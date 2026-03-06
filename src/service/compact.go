package service

import (
	"biobtree/pbuf"
	"fmt"
	"strings"
)

// MapLiteResponse represents the LLM-friendly lite format response for map queries
type MapLiteResponse struct {
	Context    MapLiteContext  `json:"context"`
	Stats      LiteStats       `json:"stats"`
	Pagination LitePagination  `json:"pagination"`
	Schema     string          `json:"schema"`
	Mappings   []LiteMapping   `json:"mappings"`
	NotFound   []string        `json:"not_found,omitempty"`
}

// LiteMapping represents a single input-to-targets mapping group
type LiteMapping struct {
	Input   string   `json:"input"`   // Original search term
	Source  string   `json:"source"`  // Resolved source: "id|name"
	Targets []string `json:"targets"` // Target entries in pipe-delimited format
}

// MapLiteContext provides context about the map query
type MapLiteContext struct {
	Query         string `json:"query"`
	SourceDataset string `json:"source_dataset,omitempty"`
	TargetDataset string `json:"target_dataset,omitempty"`
}

// LiteStats provides summary statistics
type LiteStats struct {
	Queried int `json:"queried,omitempty"`
	Total   int `json:"total"`
	Mapped  int `json:"mapped,omitempty"`
}

// SearchLiteResponse represents the LLM-friendly lite format for search queries
type SearchLiteResponse struct {
	Context    SearchLiteContext `json:"context"`
	Stats      SearchLiteStats   `json:"stats"`
	Pagination LitePagination    `json:"pagination"`
	Schema     string            `json:"schema"`
	Data       []string          `json:"data"`
}

// SearchLiteContext provides context about the search query
type SearchLiteContext struct {
	Query         string `json:"query"`
	DatasetFilter string `json:"dataset_filter,omitempty"`
}

// SearchLiteStats provides search statistics
type SearchLiteStats struct {
	Total int `json:"total"`
}

// LitePagination handles pagination state
type LitePagination struct {
	HasNext   bool   `json:"has_next"`
	NextToken string `json:"next_token,omitempty"`
}

// EntryLiteResponse represents the LLM-friendly lite format for entry queries
// Keeps full attributes but compacts cross-references to counts only
type EntryLiteResponse struct {
	Dataset     uint32      `json:"dataset"`
	DatasetName string      `json:"dataset_name"`
	Identifier  string      `json:"identifier"`
	Attributes  interface{} `json:"Attributes,omitempty"`
	Count       uint32      `json:"count"`
	Xrefs       XrefCounts  `json:"xrefs"`
}

// XrefCounts provides cross-reference counts by dataset
type XrefCounts struct {
	Total  int      `json:"total"`
	Schema string   `json:"schema"`
	Data   []string `json:"data"`
}

// GetCompactSchema returns pipe-delimited schema for a dataset
func GetCompactSchema(compactFields []string) string {
	if len(compactFields) == 0 {
		return "id"
	}
	return "id|" + strings.Join(compactFields, "|")
}

// GetCompactRow extracts compact fields from Xref as pipe-delimited string
func GetCompactRow(xref *pbuf.Xref, compactFields []string) string {
	if xref == nil {
		return ""
	}

	values := []string{escapePipe(xref.Identifier)}
	for _, field := range compactFields {
		val := extractField(xref, field)
		values = append(values, escapePipe(val))
	}
	return strings.Join(values, "|")
}

// extractField extracts a specific field value from an Xref
func extractField(xref *pbuf.Xref, field string) string {
	if xref == nil {
		return ""
	}

	// Try each attribute type
	if a := xref.GetOntology(); a != nil {
		return extractOntologyField(a, field)
	}
	if a := xref.GetHpoAttr(); a != nil {
		return extractHPOField(a, field)
	}
	if a := xref.GetHgnc(); a != nil {
		return extractHgncField(a, field)
	}
	if a := xref.GetEnsembl(); a != nil {
		return extractEnsemblField(a, field)
	}
	if a := xref.GetTaxonomy(); a != nil {
		return extractTaxonomyField(a, field)
	}
	if a := xref.GetUniprot(); a != nil {
		return extractUniprotField(a, field)
	}
	if a := xref.GetChembl(); a != nil {
		return extractChemblField(a, field)
	}
	if a := xref.GetReactome(); a != nil {
		return extractReactomeField(a, field)
	}
	if a := xref.GetClinvar(); a != nil {
		return extractClinvarField(a, field)
	}
	if a := xref.GetDbsnp(); a != nil {
		return extractDbsnpField(a, field)
	}
	if a := xref.GetEntrez(); a != nil {
		return extractEntrezField(a, field)
	}
	if a := xref.GetPdb(); a != nil {
		return extractPdbField(a, field)
	}
	if a := xref.GetPatent(); a != nil {
		return extractPatentField(a, field)
	}
	if a := xref.GetClinicalTrials(); a != nil {
		return extractClinicalTrialsField(a, field)
	}
	if a := xref.GetRefseq(); a != nil {
		return extractRefseqField(a, field)
	}
	if a := xref.GetChebi(); a != nil {
		return extractChebiField(a, field)
	}
	if a := xref.GetHmdb(); a != nil {
		return extractHmdbField(a, field)
	}
	if a := xref.GetInterpro(); a != nil {
		return extractInterproField(a, field)
	}
	if a := xref.GetPubchem(); a != nil {
		return extractPubchemField(a, field)
	}
	if a := xref.GetStringattr(); a != nil {
		return extractStringField(a, field)
	}
	if a := xref.GetStringInteraction(); a != nil {
		return extractStringInteractionField(a, field)
	}
	if a := xref.GetAlphafold(); a != nil {
		return extractAlphafoldField(a, field)
	}
	if a := xref.GetRnacentral(); a != nil {
		return extractRnacentralField(a, field)
	}
	if a := xref.GetLipidmaps(); a != nil {
		return extractLipidmapsField(a, field)
	}
	if a := xref.GetSwisslipids(); a != nil {
		return extractSwisslipidsField(a, field)
	}
	if a := xref.GetBgee(); a != nil {
		return extractBgeeField(a, field)
	}
	if a := xref.GetRhea(); a != nil {
		return extractRheaField(a, field)
	}
	if a := xref.GetGwasStudy(); a != nil {
		return extractGwasStudyField(a, field)
	}
	if a := xref.GetGwas(); a != nil {
		return extractGwasField(a, field)
	}
	if a := xref.GetIntact(); a != nil {
		return extractIntactField(a, field)
	}
	if a := xref.GetDiamondSimilarity(); a != nil {
		return extractDiamondSimilarityField(a, field)
	}
	if a := xref.GetEsm2Similarity(); a != nil {
		return extractEsm2SimilarityField(a, field)
	}
	if a := xref.GetAntibody(); a != nil {
		return extractAntibodyField(a, field)
	}
	if a := xref.GetPubchemActivity(); a != nil {
		return extractPubchemActivityField(a, field)
	}
	if a := xref.GetPubchemAssay(); a != nil {
		return extractPubchemAssayField(a, field)
	}
	if a := xref.GetMesh(); a != nil {
		return extractMeshField(a, field)
	}
	if a := xref.GetBgeeEvidence(); a != nil {
		return extractBgeeEvidenceField(a, field)
	}
	if a := xref.GetCellxgene(); a != nil {
		return extractCellxgeneField(a, field)
	}
	if a := xref.GetCellxgeneCelltype(); a != nil {
		return extractCellxgeneCelltypeField(a, field)
	}
	if a := xref.GetScxa(); a != nil {
		return extractScxaField(a, field)
	}
	if a := xref.GetScxaExpression(); a != nil {
		return extractScxaExpressionField(a, field)
	}
	if a := xref.GetScxaGeneExperiment(); a != nil {
		return extractScxaGeneExperimentField(a, field)
	}
	if a := xref.GetBiogrid(); a != nil {
		return extractBiogridField(a, field)
	}
	if a := xref.GetBiogridInteraction(); a != nil {
		return extractBiogridInteractionField(a, field)
	}
	if a := xref.GetCtdGeneInteraction(); a != nil {
		return extractCtdGeneInteractionField(a, field)
	}
	if a := xref.GetCtdDiseaseAssociation(); a != nil {
		return extractCtdDiseaseAssociationField(a, field)
	}
	if a := xref.GetUfeature(); a != nil {
		return extractUfeatureField(a, field)
	}
	if a := xref.GetCtd(); a != nil {
		return extractCtdField(a, field)
	}
	if a := xref.GetGencc(); a != nil {
		return extractGenccField(a, field)
	}
	if a := xref.GetBindingdb(); a != nil {
		return extractBindingdbField(a, field)
	}
	if a := xref.GetMsigdb(); a != nil {
		return extractMsigdbField(a, field)
	}
	if a := xref.GetAlphamissense(); a != nil {
		return extractAlphaMissenseField(a, field)
	}
	if a := xref.GetAlphamissenseTranscript(); a != nil {
		return extractAlphaMissenseTranscriptField(a, field)
	}
	if a := xref.GetPharmgkb(); a != nil {
		return extractPharmgkbField(a, field)
	}
	if a := xref.GetPharmgkbGene(); a != nil {
		return extractPharmgkbGeneField(a, field)
	}
	if a := xref.GetPharmgkbClinical(); a != nil {
		return extractPharmgkbClinicalField(a, field)
	}
	if a := xref.GetPharmgkbVariant(); a != nil {
		return extractPharmgkbVariantField(a, field)
	}
	if a := xref.GetPharmgkbGuideline(); a != nil {
		return extractPharmgkbGuidelineField(a, field)
	}
	if a := xref.GetPharmgkbPathway(); a != nil {
		return extractPharmgkbPathwayField(a, field)
	}
	if a := xref.GetCollectri(); a != nil {
		return extractCollectriField(a, field)
	}
	if a := xref.GetSignor(); a != nil {
		return extractSignorField(a, field)
	}
	if a := xref.GetCorum(); a != nil {
		return extractCorumField(a, field)
	}
	if a := xref.GetBrenda(); a != nil {
		return extractBrendaField(a, field)
	}
	if a := xref.GetBrendaKinetics(); a != nil {
		return extractBrendaKineticsField(a, field)
	}
	if a := xref.GetBrendaInhibitor(); a != nil {
		return extractBrendaInhibitorField(a, field)
	}
	if a := xref.GetCellphonedb(); a != nil {
		return extractCellphonedbField(a, field)
	}
	if a := xref.GetOrphanet(); a != nil {
		return extractOrphanetField(a, field)
	}
	if a := xref.GetSpliceai(); a != nil {
		return extractSpliceAIField(a, field)
	}
	if a := xref.GetMirdb(); a != nil {
		return extractMiRDBField(a, field)
	}
	if a := xref.GetFantom5Promoter(); a != nil {
		return extractFantom5PromoterField(a, field)
	}
	if a := xref.GetFantom5Enhancer(); a != nil {
		return extractFantom5EnhancerField(a, field)
	}
	if a := xref.GetFantom5Gene(); a != nil {
		return extractFantom5GeneField(a, field)
	}
	if a := xref.GetJaspar(); a != nil {
		return extractJasparField(a, field)
	}
	if a := xref.GetEncodeCcre(); a != nil {
		return extractEncodeCcreField(a, field)
	}

	return ""
}

// extractOntologyField extracts a field from OntologyAttr
func extractOntologyField(a *pbuf.OntologyAttr, field string) string {
	switch field {
	case "type":
		return a.Type
	case "name":
		return a.Name
	default:
		return ""
	}
}

// extractHPOField extracts a field from HPOAttr
func extractHPOField(a *pbuf.HPOAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "definition":
		return a.Definition
	default:
		return ""
	}
}

// extractHgncField extracts a field from HgncAttr
func extractHgncField(a *pbuf.HgncAttr, field string) string {
	switch field {
	case "locus_group":
		return a.LocusGroup
	case "location":
		return a.Location
	case "locus_type":
		return a.LocusType
	case "status":
		return a.Status
	default:
		return ""
	}
}

// extractEnsemblField extracts a field from EnsemblAttr
func extractEnsemblField(a *pbuf.EnsemblAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "description":
		return a.Description
	case "biotype":
		return a.Biotype
	case "genome":
		return a.Genome
	case "strand":
		return a.Strand
	case "seq_region":
		return a.SeqRegion
	case "start":
		return fmt.Sprintf("%d", a.Start)
	case "end":
		return fmt.Sprintf("%d", a.End)
	case "frame":
		return fmt.Sprintf("%d", a.Frame)
	case "utr5Start":
		return fmt.Sprintf("%d", a.Utr5Start)
	case "utr5End":
		return fmt.Sprintf("%d", a.Utr5End)
	case "utr3Start":
		return fmt.Sprintf("%d", a.Utr3Start)
	case "utr3End":
		return fmt.Sprintf("%d", a.Utr3End)
	case "source":
		return a.Source
	default:
		return ""
	}
}

// extractTaxonomyField extracts a field from TaxoAttr
func extractTaxonomyField(a *pbuf.TaxoAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "common_name":
		return a.CommonName
	case "rank":
		return fmt.Sprintf("%d", a.Rank)
	case "taxonomic_division":
		return a.TaxonomicDivision
	default:
		return ""
	}
}

// extractUniprotField extracts a field from UniprotAttr
func extractUniprotField(a *pbuf.UniprotAttr, field string) string {
	switch field {
	case "reviewed":
		if a.Reviewed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// extractChemblField extracts a field from ChemblAttr
func extractChemblField(a *pbuf.ChemblAttr, field string) string {
	// ChemblAttr has nested messages: molecule, target, assay, etc.
	// Most compact fields will be from molecule
	if mol := a.GetMolecule(); mol != nil {
		switch field {
		case "name":
			return mol.Name
		case "type":
			return mol.Type
		case "highestDevelopmentPhase":
			return fmt.Sprintf("%d", mol.HighestDevelopmentPhase)
		case "desc":
			return mol.Desc
		case "formula":
			return mol.Formula
		}
	}
	if tgt := a.GetTarget(); tgt != nil {
		switch field {
		case "title":
			return tgt.Title
		case "type":
			return tgt.Type
		}
	}
	if assay := a.GetAssay(); assay != nil {
		switch field {
		case "desc":
			return assay.Desc
		case "type":
			return assay.Type
		}
	}
	if act := a.GetActivity(); act != nil {
		switch field {
		case "type", "standard_type":
			return act.StandardType
		case "standard_value":
			return fmt.Sprintf("%.4g", act.StandardValue)
		case "standard_units":
			return act.StandardUnits
		case "pchembl":
			return fmt.Sprintf("%.2f", act.PChembl)
		}
	}
	if cl := a.GetCellLine(); cl != nil {
		switch field {
		case "desc":
			return cl.Desc
		case "cellosaurus_id":
			return cl.CellosaurusId
		case "efo":
			return cl.Efo
		case "clo":
			return cl.Clo
		case "tax":
			return cl.Tax
		}
	}
	return ""
}

// extractReactomeField extracts a field from ReactomePathwayAttr
func extractReactomeField(a *pbuf.ReactomePathwayAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "tax_id":
		return fmt.Sprintf("%d", a.TaxId)
	case "is_disease_pathway":
		if a.IsDiseasePathway {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// extractClinvarField extracts a field from ClinvarAttr
func extractClinvarField(a *pbuf.ClinvarAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "type":
		return a.Type
	case "germline_classification":
		return a.GermlineClassification
	case "variation_id":
		return a.VariationId
	case "chromosome":
		return a.Chromosome
	case "review_status":
		return a.ReviewStatus
	case "gene_symbol":
		return a.GeneSymbol
	default:
		return ""
	}
}

// extractDbsnpField extracts a field from DbsnpAttr
func extractDbsnpField(a *pbuf.DbsnpAttr, field string) string {
	switch field {
	case "chromosome":
		return a.Chromosome
	case "position":
		return fmt.Sprintf("%d", a.Position)
	case "ref_allele":
		return a.RefAllele
	case "alt_allele":
		return a.AltAllele
	case "rs_id":
		return a.RsId
	case "clinical_significance":
		return a.ClinicalSignificance
	case "variant_type":
		return a.VariantType
	default:
		return ""
	}
}

// extractEntrezField extracts a field from EntrezAttr
func extractEntrezField(a *pbuf.EntrezAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "symbol":
		return a.Symbol
	case "type":
		return a.Type
	case "chromosome":
		return a.Chromosome
	default:
		return ""
	}
}

// extractPdbField extracts a field from PdbAttr
func extractPdbField(a *pbuf.PdbAttr, field string) string {
	switch field {
	case "title":
		return a.Title
	case "method":
		return a.Method
	case "resolution":
		return a.Resolution
	case "header":
		return a.Header
	case "molecule_type":
		return a.MoleculeType
	case "release_date":
		return a.ReleaseDate
	case "source_organism":
		return a.SourceOrganism
	case "chain_count":
		return fmt.Sprintf("%d", a.ChainCount)
	case "chains":
		return a.Chains
	default:
		return ""
	}
}

// extractClinicalTrialsField extracts a field from ClinicalTrialAttr
func extractClinicalTrialsField(a *pbuf.ClinicalTrialAttr, field string) string {
	switch field {
	case "brief_title":
		return a.BriefTitle
	case "overall_status":
		return a.OverallStatus
	case "phase":
		return a.Phase
	case "study_type":
		return a.StudyType
	case "conditions":
		return strings.Join(a.Conditions, ";")
	default:
		return ""
	}
}

// extractPatentField extracts a field from PatentAttr
func extractPatentField(a *pbuf.PatentAttr, field string) string {
	switch field {
	case "title":
		return a.Title
	case "country":
		return a.Country
	case "publication_date":
		return a.PublicationDate
	case "family_id":
		return a.FamilyId
	case "assignee":
		return strings.Join(a.Asignee, ";")
	default:
		return ""
	}
}

// extractRefseqField extracts a field from RefSeqAttr
func extractRefseqField(a *pbuf.RefSeqAttr, field string) string {
	switch field {
	case "symbol":
		return a.Symbol
	case "type":
		return a.Type
	case "status":
		return a.Status
	case "description":
		return a.Description
	case "chromosome":
		return a.Chromosome
	case "organism":
		return a.Organism
	case "is_mane_select":
		if a.IsManeSelect {
			return "true"
		}
		return "false"
	case "is_mane_plus_clinical":
		if a.IsManePlusClinical {
			return "true"
		}
		return "false"
	case "seq_length":
		return fmt.Sprintf("%d", a.SeqLength)
	case "protein_length":
		return fmt.Sprintf("%d", a.ProteinLength)
	default:
		return ""
	}
}

// extractChebiField extracts a field from ChebiAttr
func extractChebiField(a *pbuf.ChebiAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "formula":
		return a.Formula
	case "definition":
		return a.Definition
	case "smiles":
		return a.Smiles
	case "inchi_key":
		return a.InchiKey
	case "star_rating":
		return fmt.Sprintf("%d", a.StarRating)
	default:
		return ""
	}
}

// extractHmdbField extracts a field from HmdbAttr
func extractHmdbField(a *pbuf.HmdbAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "formula":
		return a.Formula
	case "iupac_name":
		return a.IupacName
	case "smiles":
		return a.Smiles
	case "inchi_key":
		return a.InchiKey
	case "diseases":
		return strings.Join(a.Diseases, ";")
	case "pathways":
		return strings.Join(a.Pathways, ";")
	case "biospecimens":
		return strings.Join(a.Biospecimens, ";")
	case "tissue_locations":
		return strings.Join(a.TissueLocations, ";")
	default:
		return ""
	}
}

// extractInterproField extracts a field from InterproAttr
func extractInterproField(a *pbuf.InterproAttr, field string) string {
	switch field {
	case "short_name":
		return a.ShortName
	case "type":
		return a.Type
	case "protein_count":
		return fmt.Sprintf("%d", a.ProteinCount)
	default:
		return ""
	}
}

// extractPubchemField extracts a field from PubchemAttr
func extractPubchemField(a *pbuf.PubchemAttr, field string) string {
	switch field {
	case "name", "title":
		return a.Title
	case "iupac_name":
		return a.IupacName
	case "formula", "molecular_formula":
		return a.MolecularFormula
	case "smiles":
		return a.Smiles
	case "inchi_key":
		return a.InchiKey
	case "is_fda_approved":
		if a.IsFdaApproved {
			return "true"
		}
		return "false"
	case "compound_type":
		return a.CompoundType
	case "synonyms":
		return strings.Join(a.Synonyms, ";")
	case "mesh_terms":
		return strings.Join(a.MeshTerms, ";")
	default:
		return ""
	}
}

// extractStringField extracts a field from StringAttr
func extractStringField(a *pbuf.StringAttr, field string) string {
	switch field {
	case "string_id":
		return a.StringId
	case "organism_taxid":
		return fmt.Sprintf("%d", a.OrganismTaxid)
	case "preferred_name":
		return a.PreferredName
	case "protein_size":
		return fmt.Sprintf("%d", a.ProteinSize)
	case "annotation":
		return a.Annotation
	case "interaction_count":
		return fmt.Sprintf("%d", a.InteractionCount)
	default:
		return ""
	}
}

// extractStringInteractionField extracts a field from StringInteractionAttr
func extractStringInteractionField(a *pbuf.StringInteractionAttr, field string) string {
	switch field {
	case "protein_a":
		return a.ProteinA
	case "protein_b":
		return a.ProteinB
	case "uniprot_a":
		return a.UniprotA
	case "uniprot_b":
		return a.UniprotB
	case "score":
		return fmt.Sprintf("%d", a.Score)
	case "has_experimental":
		if a.HasExperimental {
			return "true"
		}
		return "false"
	case "has_database":
		if a.HasDatabase {
			return "true"
		}
		return "false"
	case "has_textmining":
		if a.HasTextmining {
			return "true"
		}
		return "false"
	case "has_coexpression":
		if a.HasCoexpression {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// escapePipe escapes pipe characters in values
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// ExtractSourceName gets the primary name from source Xref
func ExtractSourceName(xref *pbuf.Xref) string {
	if xref == nil {
		return ""
	}

	// Try each attribute type for name
	if a := xref.GetOntology(); a != nil {
		return a.Name
	}
	if a := xref.GetHpoAttr(); a != nil {
		return a.Name
	}
	if a := xref.GetHgnc(); a != nil {
		if len(a.Names) > 0 {
			return a.Names[0]
		}
	}
	if a := xref.GetEnsembl(); a != nil {
		return a.Name
	}
	if a := xref.GetUniprot(); a != nil {
		if len(a.Names) > 0 {
			return a.Names[0]
		}
	}
	if a := xref.GetChembl(); a != nil {
		if mol := a.GetMolecule(); mol != nil {
			return mol.Name
		}
	}
	if a := xref.GetReactome(); a != nil {
		return a.Name
	}
	if a := xref.GetClinvar(); a != nil {
		return a.Name
	}
	if a := xref.GetEntrez(); a != nil {
		return a.Name
	}
	if a := xref.GetPdb(); a != nil {
		return a.Title
	}
	if a := xref.GetChebi(); a != nil {
		return a.Name
	}
	if a := xref.GetInterpro(); a != nil {
		return a.ShortName
	}
	if a := xref.GetTaxonomy(); a != nil {
		return a.Name
	}
	if a := xref.GetBgee(); a != nil {
		return a.GeneName
	}
	if a := xref.GetPharmgkbGene(); a != nil {
		return a.Name
	}
	if a := xref.GetPharmgkb(); a != nil {
		return a.Name
	}
	if a := xref.GetPubchem(); a != nil {
		return a.Title
	}
	if a := xref.GetMesh(); a != nil {
		return a.DescriptorName
	}
	if a := xref.GetCtd(); a != nil {
		return a.ChemicalName
	}
	if a := xref.GetClinicalTrials(); a != nil {
		return a.BriefTitle
	}
	if a := xref.GetOrphanet(); a != nil {
		return a.Name
	}
	if a := xref.GetScxa(); a != nil {
		return a.Description
	}
	if a := xref.GetCtdGeneInteraction(); a != nil {
		// Format: "ChemicalName → GeneSymbol (Organism): Interaction"
		return fmt.Sprintf("%s → %s (%s): %s", a.ChemicalName, a.GeneSymbol, a.Organism, a.Interaction)
	}
	if a := xref.GetCtdDiseaseAssociation(); a != nil {
		// Format: "ChemicalName → DiseaseName [Evidence]: InferenceGenes"
		evidence := strings.Join(a.DirectEvidence, ";")
		genes := strings.Join(a.InferenceGeneSymbols, ";")
		return fmt.Sprintf("%s → %s [%s]: %s", a.ChemicalName, a.DiseaseName, evidence, genes)
	}

	return ""
}

// extractBgeeEvidenceField extracts a field from BgeeEvidenceAttr
func extractBgeeEvidenceField(a *pbuf.BgeeEvidenceAttr, field string) string {
	switch field {
	case "gene_id":
		return a.GeneId
	case "anatomical_entity_id":
		return a.AnatomicalEntityId
	case "anatomical_entity_name":
		return a.AnatomicalEntityName
	case "expression":
		return a.Expression
	case "call_quality":
		return a.CallQuality
	case "expression_score":
		return fmt.Sprintf("%.2f", a.ExpressionScore)
	case "expression_rank":
		return fmt.Sprintf("%.0f", a.ExpressionRank)
	case "fdr":
		return fmt.Sprintf("%.2e", a.Fdr)
	default:
		return ""
	}
}

// extractCellxgeneField extracts a field from CellxgeneAttr
func extractCellxgeneField(a *pbuf.CellxgeneAttr, field string) string {
	switch field {
	case "title":
		return a.Title
	case "collection_name":
		return a.CollectionName
	case "organism":
		return a.Organism
	case "cell_count":
		return fmt.Sprintf("%d", a.CellCount)
	default:
		return ""
	}
}

// extractCellxgeneCelltypeField extracts a field from CellxgeneCelltypeAttr
func extractCellxgeneCelltypeField(a *pbuf.CellxgeneCelltypeAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "definition":
		return a.Definition
	case "total_cells":
		return fmt.Sprintf("%d", a.TotalCells)
	default:
		return ""
	}
}

// extractScxaField extracts a field from ScxaAttr
func extractScxaField(a *pbuf.ScxaAttr, field string) string {
	switch field {
	case "experiment_accession":
		return a.ExperimentAccession
	case "description":
		return a.Description
	case "species":
		return a.Species
	case "number_of_cells":
		return fmt.Sprintf("%d", a.NumberOfCells)
	case "experiment_type":
		return a.ExperimentType
	default:
		return ""
	}
}

// extractScxaExpressionField extracts a field from ScxaExpressionAttr
func extractScxaExpressionField(a *pbuf.ScxaExpressionAttr, field string) string {
	switch field {
	case "gene_id":
		return a.GeneId
	case "gene_name":
		return a.GeneName
	case "total_experiments":
		return fmt.Sprintf("%d", a.TotalExperiments)
	case "marker_experiment_count":
		return fmt.Sprintf("%d", a.MarkerExperimentCount)
	case "max_mean_expression":
		return fmt.Sprintf("%.2f", a.MaxMeanExpression)
	default:
		return ""
	}
}

// extractScxaGeneExperimentField extracts a field from ScxaGeneExperimentAttr
func extractScxaGeneExperimentField(a *pbuf.ScxaGeneExperimentAttr, field string) string {
	switch field {
	case "gene_id":
		return a.GeneId
	case "experiment_id":
		return a.ExperimentId
	case "is_marker_in_experiment":
		if a.IsMarkerInExperiment {
			return "true"
		}
		return "false"
	case "marker_cluster_count":
		return fmt.Sprintf("%d", a.MarkerClusterCount)
	case "max_mean_expression":
		return fmt.Sprintf("%.2f", a.MaxMeanExpression)
	default:
		return ""
	}
}

// extractBiogridField extracts a field from BiogridAttr
func extractBiogridField(a *pbuf.BiogridAttr, field string) string {
	switch field {
	case "biogrid_id":
		return a.BiogridId
	case "interaction_count":
		return fmt.Sprintf("%d", a.InteractionCount)
	case "unique_partners":
		return fmt.Sprintf("%d", a.UniquePartners)
	case "physical_count":
		return fmt.Sprintf("%d", a.PhysicalCount)
	case "genetic_count":
		return fmt.Sprintf("%d", a.GeneticCount)
	default:
		return ""
	}
}

// extractBiogridInteractionField extracts a field from BiogridInteractionAttr
func extractBiogridInteractionField(a *pbuf.BiogridInteractionAttr, field string) string {
	switch field {
	case "interaction_id":
		return a.InteractionId
	case "interactor_b_symbol":
		return a.InteractorBSymbol
	case "interactor_b_id":
		return a.InteractorBId
	case "experimental_system":
		return a.ExperimentalSystem
	case "experimental_system_type":
		return a.ExperimentalSystemType
	case "author":
		return a.Author
	case "publication":
		return a.Publication
	case "score":
		return fmt.Sprintf("%.2f", a.Score)
	default:
		return ""
	}
}

// extractCtdGeneInteractionField extracts a field from CtdGeneInteractionAttr
func extractCtdGeneInteractionField(a *pbuf.CtdGeneInteractionAttr, field string) string {
	switch field {
	case "interaction_id":
		return a.InteractionId
	case "chemical_id":
		return a.ChemicalId
	case "chemical_name":
		return a.ChemicalName
	case "gene_symbol":
		return a.GeneSymbol
	case "gene_id":
		return a.GeneId
	case "organism":
		return a.Organism
	case "organism_id":
		return fmt.Sprintf("%d", a.OrganismId)
	case "interaction":
		return a.Interaction
	case "interaction_actions":
		return strings.Join(a.InteractionActions, ";")
	case "gene_forms":
		return a.GeneForms
	case "pubmed_count":
		return fmt.Sprintf("%d", a.PubmedCount)
	default:
		return ""
	}
}

// extractCtdDiseaseAssociationField extracts a field from CtdDiseaseAssociationAttr
func extractCtdDiseaseAssociationField(a *pbuf.CtdDiseaseAssociationAttr, field string) string {
	switch field {
	case "association_id":
		return a.AssociationId
	case "chemical_id":
		return a.ChemicalId
	case "chemical_name":
		return a.ChemicalName
	case "disease_name":
		return a.DiseaseName
	case "disease_id":
		return a.DiseaseId
	case "direct_evidence":
		return strings.Join(a.DirectEvidence, ";")
	case "inference_gene_symbols":
		return strings.Join(a.InferenceGeneSymbols, ";")
	case "inference_score":
		return fmt.Sprintf("%.2f", a.InferenceScore)
	case "pubmed_count":
		return fmt.Sprintf("%d", a.PubmedCount)
	default:
		return ""
	}
}

// extractAlphafoldField extracts a field from AlphaFoldAttr
func extractAlphafoldField(a *pbuf.AlphaFoldAttr, field string) string {
	switch field {
	case "global_metric":
		return fmt.Sprintf("%.2f", a.GlobalMetric)
	case "fraction_plddt_very_high":
		return fmt.Sprintf("%.2f", a.FractionPlddtVeryHigh)
	case "fraction_plddt_confident":
		return fmt.Sprintf("%.2f", a.FractionPlddtConfident)
	case "fraction_plddt_low":
		return fmt.Sprintf("%.2f", a.FractionPlddtLow)
	case "fraction_plddt_very_low":
		return fmt.Sprintf("%.2f", a.FractionPlddtVeryLow)
	case "model_entity_id":
		return a.ModelEntityId
	case "gene":
		return a.Gene
	case "max_pae":
		return fmt.Sprintf("%.2f", a.MaxPae)
	case "mean_pae":
		return fmt.Sprintf("%.2f", a.MeanPae)
	case "fraction_pae_confident":
		return fmt.Sprintf("%.2f", a.FractionPaeConfident)
	case "version":
		return fmt.Sprintf("%d", a.Version)
	case "fragment_number":
		return fmt.Sprintf("%d", a.FragmentNumber)
	case "sequence_length":
		return fmt.Sprintf("%d", a.SequenceLength)
	default:
		return ""
	}
}

// extractRnacentralField extracts a field from RnacentralAttr
func extractRnacentralField(a *pbuf.RnacentralAttr, field string) string {
	switch field {
	case "rna_type":
		return a.RnaType
	case "description":
		return a.Description
	case "length":
		return fmt.Sprintf("%d", a.Length)
	case "organism_count":
		return fmt.Sprintf("%d", a.OrganismCount)
	case "databases":
		return strings.Join(a.Databases, ";")
	case "is_active":
		if a.IsActive {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// extractLipidmapsField extracts a field from LipidmapsAttr
func extractLipidmapsField(a *pbuf.LipidmapsAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "systematic_name":
		return a.SystematicName
	case "abbreviation":
		return a.Abbreviation
	case "category":
		return a.Category
	case "main_class":
		return a.MainClass
	case "sub_class":
		return a.SubClass
	case "exact_mass":
		return a.ExactMass
	case "formula":
		return a.Formula
	default:
		return ""
	}
}

// extractSwisslipidsField extracts a field from SwisslipidsAttr
func extractSwisslipidsField(a *pbuf.SwisslipidsAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "abbreviation":
		return a.Abbreviation
	case "category":
		return a.Category
	case "main_class":
		return a.MainClass
	case "sub_class":
		return a.SubClass
	case "level":
		return a.Level
	case "formula":
		return a.Formula
	case "mass":
		return a.Mass
	default:
		return ""
	}
}

// extractBgeeField extracts a field from BgeeAttr
func extractBgeeField(a *pbuf.BgeeAttr, field string) string {
	switch field {
	case "gene_name":
		return a.GeneName
	case "species":
		return a.Species
	case "expression_breadth":
		return a.ExpressionBreadth
	case "total_present_calls":
		return fmt.Sprintf("%d", a.TotalPresentCalls)
	case "total_absent_calls":
		return fmt.Sprintf("%d", a.TotalAbsentCalls)
	case "total_conditions":
		return fmt.Sprintf("%d", a.TotalConditions)
	case "max_expression_score":
		return fmt.Sprintf("%.2f", a.MaxExpressionScore)
	case "average_expression_score":
		return fmt.Sprintf("%.2f", a.AverageExpressionScore)
	case "gold_quality_count":
		return fmt.Sprintf("%d", a.GoldQualityCount)
	case "top_expressed_tissues":
		return strings.Join(a.TopExpressedTissues, ";")
	default:
		return ""
	}
}

// extractRheaField extracts a field from RheaAttr
func extractRheaField(a *pbuf.RheaAttr, field string) string {
	switch field {
	case "equation":
		return a.Equation
	case "direction":
		return a.Direction
	case "status":
		return a.Status
	case "is_transport":
		if a.IsTransport {
			return "true"
		}
		return "false"
	case "uniprot_count":
		return fmt.Sprintf("%d", a.UniprotCount)
	case "ec_numbers":
		return strings.Join(a.EcNumbers, ";")
	default:
		return ""
	}
}

// extractGwasStudyField extracts a field from GwasStudyAttr
func extractGwasStudyField(a *pbuf.GwasStudyAttr, field string) string {
	switch field {
	case "study_accession":
		return a.StudyAccession
	case "pubmed_id":
		return a.PubmedId
	case "first_author":
		return a.FirstAuthor
	case "publication_date":
		return a.PublicationDate
	case "journal":
		return a.Journal
	case "study":
		return a.Study
	case "disease_trait":
		return a.DiseaseTrait
	case "initial_sample_size":
		return a.InitialSampleSize
	default:
		return ""
	}
}

// extractGwasField extracts a field from GwasAttr
func extractGwasField(a *pbuf.GwasAttr, field string) string {
	switch field {
	case "snp_id":
		return a.SnpId
	case "study_accession":
		return a.StudyAccession
	case "disease_trait":
		return a.DiseaseTrait
	case "mapped_gene":
		return a.MappedGene
	case "chr_id":
		return a.ChrId
	case "chr_pos":
		return fmt.Sprintf("%d", a.ChrPos)
	case "p_value":
		return fmt.Sprintf("%e", a.PValue)
	case "or_beta":
		return fmt.Sprintf("%.4f", a.OrBeta)
	case "risk_allele_frequency":
		return fmt.Sprintf("%.4f", a.RiskAlleleFrequency)
	case "context":
		return a.Context
	default:
		return ""
	}
}

// extractIntactField extracts a field from IntactAttr
func extractIntactField(a *pbuf.IntactAttr, field string) string {
	switch field {
	case "interaction_id":
		return a.InteractionId
	case "protein_a":
		return a.ProteinA
	case "protein_a_gene":
		return a.ProteinAGene
	case "protein_b":
		return a.ProteinB
	case "protein_b_gene":
		return a.ProteinBGene
	case "detection_method":
		return a.DetectionMethod
	case "interaction_type":
		return a.InteractionType
	case "confidence_score":
		return fmt.Sprintf("%.3f", a.ConfidenceScore)
	default:
		return ""
	}
}

// extractDiamondSimilarityField extracts a field from DiamondSimilarityAttr
func extractDiamondSimilarityField(a *pbuf.DiamondSimilarityAttr, field string) string {
	switch field {
	case "protein_id":
		return a.ProteinId
	case "similarity_count":
		return fmt.Sprintf("%d", a.SimilarityCount)
	case "top_identity":
		return fmt.Sprintf("%.2f", a.TopIdentity)
	case "top_bitscore":
		return fmt.Sprintf("%.2f", a.TopBitscore)
	default:
		return ""
	}
}

// extractEsm2SimilarityField extracts a field from Esm2SimilarityAttr
func extractEsm2SimilarityField(a *pbuf.Esm2SimilarityAttr, field string) string {
	switch field {
	case "protein_id":
		return a.ProteinId
	case "similarity_count":
		return fmt.Sprintf("%d", a.SimilarityCount)
	case "top_similarity":
		return fmt.Sprintf("%.4f", a.TopSimilarity)
	case "avg_similarity":
		return fmt.Sprintf("%.4f", a.AvgSimilarity)
	default:
		return ""
	}
}

// extractAntibodyField extracts a field from AntibodyAttr
func extractAntibodyField(a *pbuf.AntibodyAttr, field string) string {
	switch field {
	case "source":
		return a.Source
	case "antibody_type":
		return a.AntibodyType
	case "inn_name":
		return a.InnName
	case "format":
		return a.Format
	case "isotype":
		return a.Isotype
	case "clinical_stage":
		return a.ClinicalStage
	case "status":
		return a.Status
	case "targets":
		return strings.Join(a.Targets, ";")
	default:
		return ""
	}
}

// extractPubchemActivityField extracts a field from PubchemActivityAttr
func extractPubchemActivityField(a *pbuf.PubchemActivityAttr, field string) string {
	switch field {
	case "activity_id":
		return a.ActivityId
	case "cid":
		return a.Cid
	case "aid":
		return a.Aid
	case "activity_outcome":
		return a.ActivityOutcome
	case "activity_type":
		return a.ActivityType
	case "value":
		return fmt.Sprintf("%.4f", a.Value)
	case "unit":
		return a.Unit
	case "protein_accession":
		return a.ProteinAccession
	default:
		return ""
	}
}

// extractPubchemAssayField extracts a field from PubchemAssayAttr
func extractPubchemAssayField(a *pbuf.PubchemAssayAttr, field string) string {
	switch field {
	case "aid":
		return a.Aid
	case "name":
		return a.Name
	case "source_name":
		return a.SourceName
	case "substance_type":
		return a.SubstanceType
	case "outcome_type":
		return a.OutcomeType
	case "tested_sids":
		return fmt.Sprintf("%d", a.TestedSids)
	case "active_sids":
		return fmt.Sprintf("%d", a.ActiveSids)
	case "hit_rate":
		return fmt.Sprintf("%.2f", a.HitRate)
	default:
		return ""
	}
}

// extractMeshField extracts a field from MeshAttr
func extractMeshField(a *pbuf.MeshAttr, field string) string {
	switch field {
	case "descriptor_ui":
		return a.DescriptorUi
	case "descriptor_name":
		return a.DescriptorName
	case "descriptor_class":
		return a.DescriptorClass
	case "scope_note":
		return a.ScopeNote
	case "is_supplementary":
		if a.IsSupplementary {
			return "true"
		}
		return "false"
	case "registry_number":
		return a.RegistryNumber
	case "tree_numbers":
		return strings.Join(a.TreeNumbers, ";")
	case "entry_terms":
		return strings.Join(a.EntryTerms, ";")
	case "pharmacological_actions":
		return strings.Join(a.PharmacologicalActions, ";")
	default:
		return ""
	}
}

// extractUfeatureField extracts a field from UniprotFeatureAttr
func extractUfeatureField(a *pbuf.UniprotFeatureAttr, field string) string {
	switch field {
	case "type":
		return a.Type
	case "description":
		return a.Description
	case "feature_id":
		return a.Id
	case "original":
		return a.Original
	case "variation":
		return a.Variation
	case "location_begin":
		if a.Location != nil {
			return fmt.Sprintf("%d", a.Location.Begin)
		}
		return ""
	case "location_end":
		if a.Location != nil {
			return fmt.Sprintf("%d", a.Location.End)
		}
		return ""
	default:
		return ""
	}
}

// extractCtdField extracts a field from CtdAttr
func extractCtdField(a *pbuf.CtdAttr, field string) string {
	switch field {
	case "chemical_name":
		return a.ChemicalName
	case "chemical_id":
		return a.ChemicalId
	case "cas_rn":
		return a.CasRn
	case "definition":
		return a.Definition
	case "pubchem_cid":
		return a.PubchemCid
	case "inchi_key":
		return a.InchiKey
	case "gene_interaction_count":
		return fmt.Sprintf("%d", a.GeneInteractionCount)
	case "disease_association_count":
		return fmt.Sprintf("%d", a.DiseaseAssociationCount)
	case "synonyms":
		return strings.Join(a.Synonyms, ";")
	default:
		return ""
	}
}

// extractGenccField extracts a field from GenccAttr
func extractGenccField(a *pbuf.GenccAttr, field string) string {
	switch field {
	case "gene_symbol":
		return a.GeneSymbol
	case "gene_curie":
		return a.GeneCurie
	case "disease_title":
		return a.DiseaseTitle
	case "disease_curie":
		return a.DiseaseCurie
	case "classification_title":
		return a.ClassificationTitle
	case "classification_curie":
		return a.ClassificationCurie
	case "moi_title":
		return a.MoiTitle
	case "moi_curie":
		return a.MoiCurie
	case "submitter_title":
		return a.SubmitterTitle
	case "uuid":
		return a.Uuid
	default:
		return ""
	}
}

// extractBindingdbField extracts a field from BindingdbAttr
func extractBindingdbField(a *pbuf.BindingdbAttr, field string) string {
	switch field {
	case "bindingdb_id":
		return a.BindingdbId
	case "ligand_name":
		return a.LigandName
	case "target_name":
		return a.TargetName
	case "target_source_organism":
		return a.TargetSourceOrganism
	case "ki":
		return a.Ki
	case "ic50":
		return a.Ic50
	case "kd":
		return a.Kd
	case "ec50":
		return a.Ec50
	case "ph":
		return a.Ph
	case "temp_c":
		return a.TempC
	default:
		return ""
	}
}

// extractMsigdbField extracts a field from MsigdbAttr
func extractMsigdbField(a *pbuf.MsigdbAttr, field string) string {
	switch field {
	case "standard_name":
		return a.StandardName
	case "systematic_name":
		return a.SystematicName
	case "collection":
		return a.Collection
	case "description":
		return a.Description
	case "gene_count":
		return fmt.Sprintf("%d", a.GeneCount)
	case "pmid":
		return a.Pmid
	default:
		return ""
	}
}

// extractAlphaMissenseField extracts a field from AlphaMissenseAttr
func extractAlphaMissenseField(a *pbuf.AlphaMissenseAttr, field string) string {
	switch field {
	case "gene_symbol":
		return a.GeneSymbol
	case "protein_variant":
		return a.ProteinVariant
	case "am_pathogenicity":
		return fmt.Sprintf("%.3f", a.AmPathogenicity)
	case "am_class":
		return a.AmClass
	case "chromosome":
		return a.Chromosome
	case "position":
		return fmt.Sprintf("%d", a.Position)
	case "uniprot_id":
		return a.UniprotId
	default:
		return ""
	}
}

// extractAlphaMissenseTranscriptField extracts a field from AlphaMissenseTranscriptAttr
func extractAlphaMissenseTranscriptField(a *pbuf.AlphaMissenseTranscriptAttr, field string) string {
	switch field {
	case "transcript_id":
		return a.TranscriptId
	case "mean_am_pathogenicity":
		return fmt.Sprintf("%.3f", a.MeanAmPathogenicity)
	default:
		return ""
	}
}

// extractPharmgkbField extracts a field from PharmgkbAttr
func extractPharmgkbField(a *pbuf.PharmgkbAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "pharmgkb_id":
		return a.PharmgkbId
	case "type":
		return a.Type
	case "clinical_annotation_count":
		return fmt.Sprintf("%d", a.ClinicalAnnotationCount)
	case "variant_annotation_count":
		return fmt.Sprintf("%d", a.VariantAnnotationCount)
	case "pathway_count":
		return fmt.Sprintf("%d", a.PathwayCount)
	default:
		return ""
	}
}

// extractPharmgkbGeneField extracts a field from PharmgkbGeneAttr
func extractPharmgkbGeneField(a *pbuf.PharmgkbGeneAttr, field string) string {
	switch field {
	case "symbol":
		return a.Symbol
	case "name":
		return a.Name
	case "pharmgkb_id":
		return a.PharmgkbId
	case "is_vip":
		if a.IsVip {
			return "true"
		}
		return "false"
	case "has_variant_annotation":
		if a.HasVariantAnnotation {
			return "true"
		}
		return "false"
	case "has_cpic_guideline":
		if a.HasCpicGuideline {
			return "true"
		}
		return "false"
	case "chromosome":
		return a.Chromosome
	default:
		return ""
	}
}

// extractPharmgkbClinicalField extracts a field from PharmgkbClinicalAttr
func extractPharmgkbClinicalField(a *pbuf.PharmgkbClinicalAttr, field string) string {
	switch field {
	case "variant":
		return a.Variant
	case "gene":
		return a.Gene
	case "type":
		return a.Type
	case "level_of_evidence":
		return a.LevelOfEvidence
	case "chemicals":
		return strings.Join(a.Chemicals, ";")
	case "phenotypes":
		return strings.Join(a.Phenotypes, ";")
	default:
		return ""
	}
}

// extractPharmgkbVariantField extracts a field from PharmgkbVariantAttr
func extractPharmgkbVariantField(a *pbuf.PharmgkbVariantAttr, field string) string {
	switch field {
	case "variant_name":
		return a.VariantName
	case "variant_id":
		return a.VariantId
	case "gene_symbols":
		return strings.Join(a.GeneSymbols, ";")
	case "location":
		return a.Location
	case "level_of_evidence":
		return a.LevelOfEvidence
	case "score":
		return fmt.Sprintf("%.2f", a.Score)
	case "clinical_annotation_count":
		return fmt.Sprintf("%d", a.ClinicalAnnotationCount)
	case "guideline_annotation_count":
		return fmt.Sprintf("%d", a.GuidelineAnnotationCount)
	case "associated_drugs":
		return strings.Join(a.AssociatedDrugs, ";")
	default:
		return ""
	}
}

// extractPharmgkbGuidelineField extracts a field from PharmgkbGuidelineAttr
func extractPharmgkbGuidelineField(a *pbuf.PharmgkbGuidelineAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "guideline_id":
		return a.GuidelineId
	case "source":
		return a.Source
	case "gene_symbols":
		return strings.Join(a.GeneSymbols, ";")
	case "chemical_names":
		return strings.Join(a.ChemicalNames, ";")
	case "has_dosing_info":
		if a.HasDosingInfo {
			return "true"
		}
		return "false"
	case "has_recommendation":
		if a.HasRecommendation {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// extractPharmgkbPathwayField extracts a field from PharmgkbPathwayAttr
func extractPharmgkbPathwayField(a *pbuf.PharmgkbPathwayAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "pathway_id":
		return a.PathwayId
	case "is_pharmacokinetic":
		if a.IsPharmacokinetic {
			return "true"
		}
		return "false"
	case "is_pharmacodynamic":
		if a.IsPharmacodynamic {
			return "true"
		}
		return "false"
	case "gene_symbols":
		return strings.Join(a.GeneSymbols, ";")
	case "chemical_names":
		return strings.Join(a.ChemicalNames, ";")
	case "disease_names":
		return strings.Join(a.DiseaseNames, ";")
	default:
		return ""
	}
}

// extractCollectriField extracts a field from CollecTriAttr
func extractCollectriField(a *pbuf.CollecTriAttr, field string) string {
	switch field {
	case "tf_gene":
		return a.TfGene
	case "target_gene":
		return a.TargetGene
	case "regulation":
		return a.Regulation
	case "confidence":
		return a.Confidence
	case "sources":
		return strings.Join(a.Sources, ";")
	default:
		return ""
	}
}

// extractSignorField extracts a field from SignorAttr
func extractSignorField(a *pbuf.SignorAttr, field string) string {
	switch field {
	case "entity_a":
		return a.EntityA
	case "entity_b":
		return a.EntityB
	case "effect":
		return a.Effect
	case "mechanism":
		return a.Mechanism
	case "direct":
		if a.Direct {
			return "true"
		}
		return "false"
	case "score":
		return fmt.Sprintf("%.2f", a.Score)
	case "type_a":
		return a.TypeA
	case "type_b":
		return a.TypeB
	default:
		return ""
	}
}

// extractCorumField extracts a field from CorumAttr
func extractCorumField(a *pbuf.CorumAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "organism":
		return a.Organism
	case "subunit_count":
		return fmt.Sprintf("%d", a.SubunitCount)
	case "subunit_genes":
		return strings.Join(a.SubunitGenes, ";")
	case "cell_line":
		return a.CellLine
	case "comment_disease":
		return a.CommentDisease
	case "has_drug_targets":
		if a.HasDrugTargets {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// extractBrendaField extracts a field from BrendaAttr
func extractBrendaField(a *pbuf.BrendaAttr, field string) string {
	switch field {
	case "recommended_name":
		return a.RecommendedName
	case "systematic_name":
		return a.SystematicName
	case "organism_count":
		return fmt.Sprintf("%d", a.OrganismCount)
	case "substrate_count":
		return fmt.Sprintf("%d", a.SubstrateCount)
	case "inhibitor_count":
		return fmt.Sprintf("%d", a.InhibitorCount)
	case "km_count":
		return fmt.Sprintf("%d", a.KmCount)
	case "kcat_count":
		return fmt.Sprintf("%d", a.KcatCount)
	default:
		return ""
	}
}

// extractBrendaKineticsField extracts a field from BrendaKineticsAttr
// formatFloat returns "0" for zero values, otherwise formats with precision
func formatFloat(v float64) string {
	if v == 0 {
		return "0"
	}
	return fmt.Sprintf("%.4f", v)
}

func extractBrendaKineticsField(a *pbuf.BrendaKineticsAttr, field string) string {
	switch field {
	case "ec_number":
		return a.EcNumber
	case "substrate":
		return a.Substrate
	case "substrate_type":
		return a.SubstrateType
	case "km_count":
		return fmt.Sprintf("%d", a.KmCount)
	case "kcat_count":
		return fmt.Sprintf("%d", a.KcatCount)
	case "min_km":
		return formatFloat(a.MinKm)
	case "max_km":
		return formatFloat(a.MaxKm)
	default:
		return ""
	}
}

// extractBrendaInhibitorField extracts a field from BrendaInhibitorAttr
func extractBrendaInhibitorField(a *pbuf.BrendaInhibitorAttr, field string) string {
	switch field {
	case "ec_number":
		return a.EcNumber
	case "inhibitor":
		return a.Inhibitor
	case "ki_count":
		return fmt.Sprintf("%d", a.KiCount)
	case "ic50_count":
		return fmt.Sprintf("%d", a.Ic50Count)
	case "min_ki":
		return formatFloat(a.MinKi)
	case "max_ki":
		return formatFloat(a.MaxKi)
	case "min_ic50":
		return formatFloat(a.MinIc50)
	case "max_ic50":
		return formatFloat(a.MaxIc50)
	default:
		return ""
	}
}

// extractCellphonedbField extracts a field from CellphonedbAttr
func extractCellphonedbField(a *pbuf.CellphonedbAttr, field string) string {
	switch field {
	case "partner_a":
		return a.PartnerA
	case "partner_b":
		return a.PartnerB
	case "directionality":
		return a.Directionality
	case "classification":
		return a.Classification
	case "genes_a":
		return strings.Join(a.GenesA, ";")
	case "genes_b":
		return strings.Join(a.GenesB, ";")
	case "receptor_a":
		if a.ReceptorA {
			return "true"
		}
		return "false"
	case "receptor_b":
		if a.ReceptorB {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// extractOrphanetField extracts a field from OrphanetAttr
func extractOrphanetField(a *pbuf.OrphanetAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "disorder_type":
		return a.DisorderType
	case "definition":
		return a.Definition
	case "gene_count":
		return fmt.Sprintf("%d", a.GeneCount)
	case "phenotype_count":
		return fmt.Sprintf("%d", a.PhenotypeCount)
	case "synonyms":
		return strings.Join(a.Synonyms, ";")
	default:
		return ""
	}
}

// extractSpliceAIField extracts a field from SpliceAIAttr
func extractSpliceAIField(a *pbuf.SpliceAIAttr, field string) string {
	switch field {
	case "chromosome":
		return a.Chromosome
	case "position":
		return fmt.Sprintf("%d", a.Position)
	case "ref_allele":
		return a.RefAllele
	case "alt_allele":
		return a.AltAllele
	case "effect":
		return a.Effect
	case "score":
		if a.Score == 0 {
			return "0"
		}
		return fmt.Sprintf("%.4f", a.Score)
	case "gene_symbol":
		return a.GeneSymbol
	case "allele_info":
		return a.AlleleInfo
	default:
		return ""
	}
}

// extractMiRDBField extracts a field from MiRDBAttr
func extractMiRDBField(a *pbuf.MiRDBAttr, field string) string {
	switch field {
	case "mirna_id":
		return a.MirnaId
	case "species":
		return a.Species
	case "target_count":
		return fmt.Sprintf("%d", a.TargetCount)
	case "avg_score":
		if a.AvgScore == 0 {
			return "0"
		}
		return fmt.Sprintf("%.2f", a.AvgScore)
	case "max_score":
		if a.MaxScore == 0 {
			return "0"
		}
		return fmt.Sprintf("%.2f", a.MaxScore)
	case "min_score":
		if a.MinScore == 0 {
			return "0"
		}
		return fmt.Sprintf("%.2f", a.MinScore)
	default:
		return ""
	}
}

// extractFantom5PromoterField extracts a field from Fantom5PromoterAttr
func extractFantom5PromoterField(a *pbuf.Fantom5PromoterAttr, field string) string {
	switch field {
	case "gene_symbol":
		return a.GeneSymbol
	case "chromosome":
		return a.Chromosome
	case "start":
		return fmt.Sprintf("%d", a.Start)
	case "end":
		return fmt.Sprintf("%d", a.End)
	case "strand":
		return a.Strand
	case "tpm_average":
		return formatFloat(a.TpmAverage)
	case "tpm_max":
		return formatFloat(a.TpmMax)
	case "samples_expressed":
		return fmt.Sprintf("%d", a.SamplesExpressed)
	case "expression_breadth":
		return a.ExpressionBreadth
	default:
		return ""
	}
}

// extractFantom5EnhancerField extracts a field from Fantom5EnhancerAttr
func extractFantom5EnhancerField(a *pbuf.Fantom5EnhancerAttr, field string) string {
	switch field {
	case "chromosome":
		return a.Chromosome
	case "start":
		return fmt.Sprintf("%d", a.Start)
	case "end":
		return fmt.Sprintf("%d", a.End)
	case "tpm_average":
		return formatFloat(a.TpmAverage)
	case "tpm_max":
		return formatFloat(a.TpmMax)
	case "samples_expressed":
		return fmt.Sprintf("%d", a.SamplesExpressed)
	case "associated_genes":
		return strings.Join(a.AssociatedGenes, ";")
	default:
		return ""
	}
}

// extractFantom5GeneField extracts a field from Fantom5GeneAttr
func extractFantom5GeneField(a *pbuf.Fantom5GeneAttr, field string) string {
	switch field {
	case "gene_symbol":
		return a.GeneSymbol
	case "gene_id":
		return a.GeneId
	case "tpm_average":
		return formatFloat(a.TpmAverage)
	case "tpm_max":
		return formatFloat(a.TpmMax)
	case "samples_expressed":
		return fmt.Sprintf("%d", a.SamplesExpressed)
	case "expression_breadth":
		return a.ExpressionBreadth
	default:
		return ""
	}
}

// extractJasparField extracts a field from JasparAttr
func extractJasparField(a *pbuf.JasparAttr, field string) string {
	switch field {
	case "name":
		return a.Name
	case "collection":
		return a.Collection
	case "class":
		return a.Class
	case "family":
		return a.Family
	case "tax_group":
		return a.TaxGroup
	case "type":
		return a.Type
	case "species":
		return a.Species
	case "version":
		return fmt.Sprintf("%d", a.Version)
	default:
		return ""
	}
}

// extractEncodeCcreField extracts a field from EncodeCcreAttr
func extractEncodeCcreField(a *pbuf.EncodeCcreAttr, field string) string {
	switch field {
	case "ccre_class":
		return a.CcreClass
	case "chromosome":
		return a.Chromosome
	case "start":
		return fmt.Sprintf("%d", a.Start)
	case "end":
		return fmt.Sprintf("%d", a.End)
	default:
		return ""
	}
}

// GetSearchCompactRow creates a pipe-delimited row for search results
// Format: id|dataset|name|xref_count
func GetSearchCompactRow(xref *pbuf.Xref, datasetName string) string {
	if xref == nil {
		return ""
	}

	id := xref.Identifier
	if id == "" {
		id = xref.Keyword
	}

	name := ExtractSourceName(xref)
	xrefCount := fmt.Sprintf("%d", xref.Count)

	return escapePipe(id) + "|" + escapePipe(datasetName) + "|" + escapePipe(name) + "|" + xrefCount
}
