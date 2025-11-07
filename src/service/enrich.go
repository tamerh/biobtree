package service

import (
	"biobtree/pbuf"
	"strings"
)

// EnrichResult adds all transient fields (dataset_name, url) to Result
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
// Transient fields: dataset_name, url, identifier, keyword
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

	// Set URL based on dataset type and identifier
	setURL(xref)
}

// setURL sets the URL field based on dataset type and identifier
func setURL(xref *pbuf.Xref) {
	if xref.Identifier == "" {
		return
	}

	datasetName := config.DataconfIDIntToString[xref.Dataset]

	if xref.Dataset == 72 { // ufeature
		xref.Url = strings.Replace(config.Dataconf[datasetName]["url"], "£{id}", xref.Identifier[:strings.Index(xref.Identifier, "_")], -1)

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
