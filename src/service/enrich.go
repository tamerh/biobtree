package service

import (
	"biobtree/pbuf"
	"strings"
)

// EnrichResult adds all transient fields (dataset_name) to Result
func EnrichResult(result *pbuf.Result) *pbuf.Result {
	if result == nil {
		return nil
	}

	for _, xref := range result.Results {
		enrichXref(xref)
	}

	return result
}

// EnrichMapFilterResult adds all transient fields to MapFilterResult
func EnrichMapFilterResult(result *pbuf.MapFilterResult) *pbuf.MapFilterResult {
	if result == nil {
		return nil
	}

	for _, mapFilter := range result.Results {
		// Enrich source
		enrichXref(mapFilter.Source)

		// Enrich targets
		for _, target := range mapFilter.Targets {
			enrichXref(target)
		}
	}

	return result
}

// EnrichXref adds all transient fields to a single Xref
func EnrichXref(xref *pbuf.Xref) *pbuf.Xref {
	enrichXref(xref)
	return xref
}

// enrichXref populates all transient fields in xref (modifies in place)
// Transient fields: dataset_name
func enrichXref(xref *pbuf.Xref) {
	if xref == nil {
		return
	}

	// Set dataset_name for the xref itself
	if xref.Dataset > 0 {
		if name, ok := config.DataconfIDIntToString[xref.Dataset]; ok {
			xref.DatasetName = name
		}
	}

	// Set dataset_name for all entries
	for _, entry := range xref.Entries {
		if entry.Dataset > 0 {
			if name, ok := config.DataconfIDIntToString[entry.Dataset]; ok {
				entry.DatasetName = name
			}
		}
	}

	// URL field removed - was not functional and added unnecessary response size

	// Set id in attributes (transient - enables filtering by id)
	setAttributeId(xref)
}

// setAttributeId populates the transient Id field in attribute protos
// This enables CEL filtering like: >>go[id=="GO:0005886"]
func setAttributeId(xref *pbuf.Xref) {
	if xref == nil || xref.Identifier == "" {
		return
	}

	switch attr := xref.Attributes.(type) {
	case *pbuf.Xref_Uniprot:
		if attr.Uniprot != nil {
			attr.Uniprot.Id = xref.Identifier
		}
	case *pbuf.Xref_Ufeature:
		// UniprotFeatureAttr already has id field (field 3) - no action needed
	case *pbuf.Xref_Ensembl:
		if attr.Ensembl != nil {
			attr.Ensembl.Id = xref.Identifier
		}
	case *pbuf.Xref_Taxonomy:
		if attr.Taxonomy != nil {
			attr.Taxonomy.Id = xref.Identifier
		}
	case *pbuf.Xref_Hgnc:
		if attr.Hgnc != nil {
			attr.Hgnc.Id = xref.Identifier
		}
	case *pbuf.Xref_Ontology:
		if attr.Ontology != nil {
			attr.Ontology.Id = xref.Identifier
		}
	case *pbuf.Xref_HpoAttr:
		if attr.HpoAttr != nil {
			attr.HpoAttr.Id = xref.Identifier
		}
	case *pbuf.Xref_Chembl:
		if attr.Chembl != nil {
			// Set id on nested types used in CEL
			if attr.Chembl.Molecule != nil {
				attr.Chembl.Molecule.Id = xref.Identifier
			}
			if attr.Chembl.Target != nil {
				attr.Chembl.Target.Id = xref.Identifier
			}
			if attr.Chembl.Activity != nil {
				attr.Chembl.Activity.Id = xref.Identifier
			}
			if attr.Chembl.Assay != nil {
				attr.Chembl.Assay.Id = xref.Identifier
			}
			if attr.Chembl.Doc != nil {
				attr.Chembl.Doc.Id = xref.Identifier
			}
			if attr.Chembl.CellLine != nil {
				attr.Chembl.CellLine.Id = xref.Identifier
			}
		}
	case *pbuf.Xref_Interpro:
		if attr.Interpro != nil {
			attr.Interpro.Id = xref.Identifier
		}
	case *pbuf.Xref_Ena:
		if attr.Ena != nil {
			attr.Ena.Id = xref.Identifier
		}
	case *pbuf.Xref_Hmdb:
		if attr.Hmdb != nil {
			attr.Hmdb.Id = xref.Identifier
		}
	case *pbuf.Xref_Chebi:
		if attr.Chebi != nil {
			attr.Chebi.Id = xref.Identifier
		}
	case *pbuf.Xref_Pdb:
		if attr.Pdb != nil {
			attr.Pdb.Id = xref.Identifier
		}
	case *pbuf.Xref_Drugbank:
		if attr.Drugbank != nil {
			attr.Drugbank.Id = xref.Identifier
		}
	case *pbuf.Xref_Orphanet:
		if attr.Orphanet != nil {
			attr.Orphanet.Id = xref.Identifier
		}
	case *pbuf.Xref_Reactome:
		if attr.Reactome != nil {
			attr.Reactome.Id = xref.Identifier
		}
	case *pbuf.Xref_Pubchem:
		if attr.Pubchem != nil {
			attr.Pubchem.Id = xref.Identifier
		}
	case *pbuf.Xref_PubchemActivity:
		if attr.PubchemActivity != nil {
			attr.PubchemActivity.Id = xref.Identifier
		}
	case *pbuf.Xref_PubchemAssay:
		if attr.PubchemAssay != nil {
			attr.PubchemAssay.Id = xref.Identifier
		}
	case *pbuf.Xref_Lipidmaps:
		if attr.Lipidmaps != nil {
			attr.Lipidmaps.Id = xref.Identifier
		}
	case *pbuf.Xref_Swisslipids:
		if attr.Swisslipids != nil {
			attr.Swisslipids.Id = xref.Identifier
		}
	case *pbuf.Xref_Bgee:
		if attr.Bgee != nil {
			attr.Bgee.Id = xref.Identifier
		}
	case *pbuf.Xref_BgeeEvidence:
		if attr.BgeeEvidence != nil {
			attr.BgeeEvidence.Id = xref.Identifier
		}
	case *pbuf.Xref_Rhea:
		if attr.Rhea != nil {
			attr.Rhea.Id = xref.Identifier
		}
	case *pbuf.Xref_GwasStudy:
		if attr.GwasStudy != nil {
			attr.GwasStudy.Id = xref.Identifier
		}
	case *pbuf.Xref_Gwas:
		if attr.Gwas != nil {
			attr.Gwas.Id = xref.Identifier
		}
	case *pbuf.Xref_Intact:
		if attr.Intact != nil {
			attr.Intact.Id = xref.Identifier
		}
	case *pbuf.Xref_Dbsnp:
		if attr.Dbsnp != nil {
			attr.Dbsnp.Id = xref.Identifier
		}
	case *pbuf.Xref_Clinvar:
		if attr.Clinvar != nil {
			attr.Clinvar.Id = xref.Identifier
		}
	case *pbuf.Xref_Antibody:
		if attr.Antibody != nil {
			attr.Antibody.Id = xref.Identifier
		}
	case *pbuf.Xref_Esm2Similarity:
		if attr.Esm2Similarity != nil {
			attr.Esm2Similarity.Id = xref.Identifier
		}
	case *pbuf.Xref_DiamondSimilarity:
		if attr.DiamondSimilarity != nil {
			attr.DiamondSimilarity.Id = xref.Identifier
		}
	case *pbuf.Xref_Entrez:
		if attr.Entrez != nil {
			attr.Entrez.Id = xref.Identifier
		}
	case *pbuf.Xref_Refseq:
		if attr.Refseq != nil {
			attr.Refseq.Id = xref.Identifier
		}
	case *pbuf.Xref_Gencc:
		if attr.Gencc != nil {
			attr.Gencc.Id = xref.Identifier
		}
	case *pbuf.Xref_Bindingdb:
		if attr.Bindingdb != nil {
			attr.Bindingdb.Id = xref.Identifier
		}
	case *pbuf.Xref_Ctd:
		if attr.Ctd != nil {
			attr.Ctd.Id = xref.Identifier
		}
	case *pbuf.Xref_CtdGeneInteraction:
		if attr.CtdGeneInteraction != nil {
			attr.CtdGeneInteraction.Id = xref.Identifier
		}
	case *pbuf.Xref_CtdDiseaseAssociation:
		if attr.CtdDiseaseAssociation != nil {
			attr.CtdDiseaseAssociation.Id = xref.Identifier
		}
	case *pbuf.Xref_Biogrid:
		if attr.Biogrid != nil {
			attr.Biogrid.Id = xref.Identifier
		}
	case *pbuf.Xref_BiogridInteraction:
		if attr.BiogridInteraction != nil {
			attr.BiogridInteraction.Id = xref.Identifier
		}
	case *pbuf.Xref_Msigdb:
		if attr.Msigdb != nil {
			attr.Msigdb.Id = xref.Identifier
		}
	case *pbuf.Xref_Alphamissense:
		if attr.Alphamissense != nil {
			attr.Alphamissense.Id = xref.Identifier
		}
	case *pbuf.Xref_AlphamissenseTranscript:
		if attr.AlphamissenseTranscript != nil {
			attr.AlphamissenseTranscript.Id = xref.Identifier
		}
	case *pbuf.Xref_Pharmgkb:
		if attr.Pharmgkb != nil {
			attr.Pharmgkb.Id = xref.Identifier
		}
	case *pbuf.Xref_PharmgkbGene:
		if attr.PharmgkbGene != nil {
			attr.PharmgkbGene.Id = xref.Identifier
		}
	case *pbuf.Xref_PharmgkbClinical:
		if attr.PharmgkbClinical != nil {
			attr.PharmgkbClinical.Id = xref.Identifier
		}
	case *pbuf.Xref_PharmgkbVariant:
		if attr.PharmgkbVariant != nil {
			attr.PharmgkbVariant.Id = xref.Identifier
		}
	case *pbuf.Xref_PharmgkbGuideline:
		if attr.PharmgkbGuideline != nil {
			attr.PharmgkbGuideline.Id = xref.Identifier
		}
	case *pbuf.Xref_PharmgkbPathway:
		if attr.PharmgkbPathway != nil {
			attr.PharmgkbPathway.Id = xref.Identifier
		}
	case *pbuf.Xref_Cellxgene:
		if attr.Cellxgene != nil {
			attr.Cellxgene.Id = xref.Identifier
		}
	case *pbuf.Xref_CellxgeneCelltype:
		if attr.CellxgeneCelltype != nil {
			attr.CellxgeneCelltype.Id = xref.Identifier
		}
	case *pbuf.Xref_Scxa:
		if attr.Scxa != nil {
			attr.Scxa.Id = xref.Identifier
		}
	case *pbuf.Xref_ScxaExpression:
		if attr.ScxaExpression != nil {
			attr.ScxaExpression.Id = xref.Identifier
		}
	case *pbuf.Xref_ScxaGeneExperiment:
		if attr.ScxaGeneExperiment != nil {
			attr.ScxaGeneExperiment.Id = xref.Identifier
		}
	case *pbuf.Xref_Alphafold:
		if attr.Alphafold != nil {
			attr.Alphafold.Id = xref.Identifier
		}
	case *pbuf.Xref_ClinicalTrials:
		if attr.ClinicalTrials != nil {
			attr.ClinicalTrials.Id = xref.Identifier
		}
	case *pbuf.Xref_Collectri:
		if attr.Collectri != nil {
			attr.Collectri.Id = xref.Identifier
		}
	case *pbuf.Xref_Brenda:
		if attr.Brenda != nil {
			attr.Brenda.Id = xref.Identifier
		}
	case *pbuf.Xref_BrendaKinetics:
		if attr.BrendaKinetics != nil {
			attr.BrendaKinetics.Id = xref.Identifier
		}
	case *pbuf.Xref_BrendaInhibitor:
		if attr.BrendaInhibitor != nil {
			attr.BrendaInhibitor.Id = xref.Identifier
		}
	case *pbuf.Xref_Cellphonedb:
		if attr.Cellphonedb != nil {
			attr.Cellphonedb.Id = xref.Identifier
		}
	case *pbuf.Xref_Spliceai:
		if attr.Spliceai != nil {
			attr.Spliceai.Id = xref.Identifier
		}
	case *pbuf.Xref_Mirdb:
		if attr.Mirdb != nil {
			attr.Mirdb.Id = xref.Identifier
		}
	case *pbuf.Xref_Fantom5Promoter:
		if attr.Fantom5Promoter != nil {
			attr.Fantom5Promoter.Id = xref.Identifier
		}
	case *pbuf.Xref_Fantom5Enhancer:
		if attr.Fantom5Enhancer != nil {
			attr.Fantom5Enhancer.Id = xref.Identifier
		}
	case *pbuf.Xref_Fantom5Gene:
		if attr.Fantom5Gene != nil {
			attr.Fantom5Gene.Id = xref.Identifier
		}
	case *pbuf.Xref_Jaspar:
		if attr.Jaspar != nil {
			attr.Jaspar.Id = xref.Identifier
		}
	case *pbuf.Xref_EncodeCcre:
		if attr.EncodeCcre != nil {
			attr.EncodeCcre.Id = xref.Identifier
		}
	case *pbuf.Xref_Signor:
		if attr.Signor != nil {
			attr.Signor.Id = xref.Identifier
		}
	case *pbuf.Xref_Corum:
		if attr.Corum != nil {
			attr.Corum.Id = xref.Identifier
		}
	case *pbuf.Xref_Stringattr:
		if attr.Stringattr != nil {
			attr.Stringattr.Id = xref.Identifier
		}
	case *pbuf.Xref_StringInteraction:
		if attr.StringInteraction != nil {
			attr.StringInteraction.Id = xref.Identifier
		}
	case *pbuf.Xref_Mesh:
		if attr.Mesh != nil {
			attr.Mesh.Id = xref.Identifier
		}
	case *pbuf.Xref_Patent:
		if attr.Patent != nil {
			attr.Patent.Id = xref.Identifier
		}
	case *pbuf.Xref_Rnacentral:
		if attr.Rnacentral != nil {
			attr.Rnacentral.Id = xref.Identifier
		}
	}
}

// setURL sets the URL field based on dataset type and identifier
// DEPRECATED: This function is no longer used. URL field was removed from responses
// as it was not functional (many URLs were broken) and added unnecessary response size.
// Kept for reference in case URL support is needed in the future.
func setURL(xref *pbuf.Xref) {
	if xref.Identifier == "" {
		return
	}

	datasetName := config.DataconfIDIntToString[xref.Dataset]

	if xref.Dataset == 72 { // ufeature
		idx := strings.Index(xref.Identifier, "_")
		if idx > 0 {
			xref.Url = strings.Replace(config.Dataconf[datasetName]["url"], "£{id}", xref.Identifier[:idx], -1)
		} else {
			xref.Url = strings.Replace(config.Dataconf[datasetName]["url"], "£{id}", xref.Identifier, -1)
		}

	} else if xref.Dataset == 73 { // variantid
		xref.Url = strings.Replace(config.Dataconf[datasetName]["url"], "£{id}", strings.ToLower(xref.Identifier), -1)

	} else if xref.Dataset == 2 || xref.Dataset == 42 || xref.Dataset == 39 { // ensembl,transcript exon
		if xref.GetEmpty() { // data not indexed
			xref.Url = "#"
		} else if xref.GetEnsembl() == nil { // Ensembl data missing - incomplete entry
			xref.Url = "#"
		} else {
			switch xref.GetEnsembl().Branch {
			case 1:
				xref.Url = strings.Replace(config.Dataconf[datasetName]["url"], "£{id}", xref.Identifier, -1)
			case 2:
				xref.Url = strings.Replace(config.Dataconf[datasetName]["bacteriaUrl"], "£{id}", xref.Identifier, -1)
			case 3:
				xref.Url = strings.Replace(config.Dataconf[datasetName]["fungiUrl"], "£{id}", xref.Identifier, -1)
			case 4:
				xref.Url = strings.Replace(config.Dataconf[datasetName]["metazoaUrl"], "£{id}", xref.Identifier, -1)
			case 5:
				xref.Url = strings.Replace(config.Dataconf[datasetName]["plantsUrl"], "£{id}", xref.Identifier, -1)
			case 6:
				xref.Url = strings.Replace(config.Dataconf[datasetName]["protistsUrl"], "£{id}", xref.Identifier, -1)
			default:
				xref.Url = "#"
			}
			xref.Url = strings.Replace(xref.Url, "£{sp}", xref.GetEnsembl().Genome, -1)
		}

	} else {
		xref.Url = strings.Replace(config.Dataconf[datasetName]["url"], "£{id}", xref.Identifier, -1)
	}
}

// EnrichResultFull adds all transient fields plus full mode enhancements (query, stats, pagination)
// to Result for the full response mode
func EnrichResultFull(result *pbuf.Result, terms []string, datasetFilter, rawQuery string) *pbuf.Result {
	if result == nil {
		return nil
	}

	// First apply standard enrichment
	EnrichResult(result)

	// Add query echo
	result.Query = &pbuf.SearchQueryInfo{
		Terms:         terms,
		DatasetFilter: datasetFilter,
		Raw:           rawQuery,
	}

	// Calculate statistics
	statsByDataset := make(map[string]int32)
	for _, xref := range result.Results {
		datasetName := config.DataconfIDIntToString[xref.Dataset]
		statsByDataset[datasetName]++
	}

	result.Stats = &pbuf.SearchStats{
		TotalResults: int32(len(result.Results)),
		Returned:     int32(len(result.Results)),
		ByDataset:    statsByDataset,
	}

	// Set pagination info
	hasNext := result.Nextpage != ""
	result.Pagination = &pbuf.PaginationInfo{
		Page:      1,
		HasNext:   hasNext,
		NextToken: result.Nextpage,
	}

	return result
}

// EnrichMapFilterResultFull adds all transient fields plus full mode enhancements (query, stats, pagination)
// to MapFilterResult for the full response mode
func EnrichMapFilterResultFull(result *pbuf.MapFilterResult, terms []string, chain, rawQuery string) *pbuf.MapFilterResult {
	if result == nil {
		return nil
	}

	// First apply standard enrichment
	EnrichMapFilterResult(result)

	// Add query echo
	result.Query = &pbuf.MapFilterQueryInfo{
		Terms: terms,
		Chain: chain,
		Raw:   rawQuery,
	}

	// Calculate statistics - track which INPUT terms were successfully mapped
	// Build a map of input terms to track which ones were found
	inputTermsMap := make(map[string]bool)
	for _, term := range terms {
		inputTermsMap[strings.ToUpper(term)] = false // not found yet
	}

	var totalTargets int32
	for _, mapFilter := range result.Results {
		if len(mapFilter.Targets) > 0 {
			totalTargets += int32(len(mapFilter.Targets))
			// Mark this input term as found
			if mapFilter.Source != nil {
				// Try Keyword first (for text searches like gene symbols)
				if mapFilter.Source.Keyword != "" {
					inputTermsMap[strings.ToUpper(mapFilter.Source.Keyword)] = true
				} else if mapFilter.Source.Identifier != "" {
					// Fall back to Identifier (for exact ID queries like HP:0001250)
					inputTermsMap[strings.ToUpper(mapFilter.Source.Identifier)] = true
				}
			}
		}
	}

	// Count how many unique input terms were mapped and collect not_found
	var mapped, failed int32
	var notFound []string
	for term, found := range inputTermsMap {
		if found {
			mapped++
		} else {
			failed++
			// Find original case version of the term
			for _, origTerm := range terms {
				if strings.ToUpper(origTerm) == term {
					notFound = append(notFound, origTerm)
					break
				}
			}
		}
	}

	result.Stats = &pbuf.MapFilterStats{
		TotalTerms:   int32(len(terms)),
		Mapped:       mapped,
		Failed:       failed,
		TotalTargets: totalTargets,
		NotFound:     notFound,
	}

	// Set pagination info
	hasNext := result.Nextpage != ""
	result.Pagination = &pbuf.PaginationInfo{
		Page:      1,
		HasNext:   hasNext,
		NextToken: result.Nextpage,
	}

	return result
}
