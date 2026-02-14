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

	// Count how many unique input terms were mapped
	var mapped, failed int32
	for _, found := range inputTermsMap {
		if found {
			mapped++
		} else {
			failed++
		}
	}

	result.Stats = &pbuf.MapFilterStats{
		TotalTerms:   int32(len(terms)),
		Mapped:       mapped,
		Failed:       failed,
		TotalTargets: totalTargets,
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
