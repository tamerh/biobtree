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
	Total  int `json:"total"`
	Mapped int `json:"mapped,omitempty"`
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
	if a := xref.GetChebi(); a != nil {
		return extractChebiField(a, field)
	}
	if a := xref.GetInterpro(); a != nil {
		return extractInterproField(a, field)
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

	return ""
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
