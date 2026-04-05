package service

import (
	"biobtree/pbuf"
	"biobtree/query"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/cel-go/common/types/ref"
)

// mapFilterLite performs mapping and returns compact lite format response
// Returns only IDs, sorted by has_attr (entries with attributes first)
// Includes failed terms with source=nil and error field
// MapFilterLite performs mapping and returns compact lite format response
// MapFilterLite performs mapping and returns lite format response
// Returns pipe-delimited data rows optimized for LLM consumption
func (s *Service) MapFilterLite(ids []string, mapFilterQuery, page string) (*MapLiteResponse, error) {

	// Call mapFilterWithLimit with higher limit for lite mode
	fullResult, err := s.MapFilterWithLimit(ids, mapFilterQuery, page, s.maxMappingResultLite)
	if err != nil {
		return nil, err
	}

	response := &MapLiteResponse{
		Context: MapLiteContext{
			Query: mapFilterQuery,
		},
		Stats: LiteStats{
			Queried: len(ids),
			Total:   0,
			Mapped:  0,
		},
		Pagination: LitePagination{
			HasNext:   fullResult.Nextpage != "",
			NextToken: fullResult.Nextpage,
		},
	}

	// Track the target dataset for schema
	var targetDataset string
	var targetLiteFields []string

	// Track which input terms were found (for not_found calculation)
	foundInputs := make(map[string]bool)

	// Process results - group by source
	for _, mapRes := range fullResult.Results {
		if mapRes.Source == nil {
			continue
		}

		// Set context datasets from first result
		if response.Context.SourceDataset == "" {
			srcDataset := config.DataconfIDIntToString[mapRes.Source.Dataset]
			response.Context.SourceDataset = srcDataset
		}

		// Get original input term (Keyword) or fall back to Identifier
		inputTerm := mapRes.Source.Keyword
		if inputTerm == "" {
			inputTerm = mapRes.Source.Identifier
		}

		// Mark this input as found
		foundInputs[inputTerm] = true

		// Build source string: "id|name"
		sourceName := ExtractSourceName(mapRes.Source)
		sourceStr := mapRes.Source.Identifier + "|" + sourceName

		// Create mapping for this source
		mapping := LiteMapping{
			Input:   inputTerm,
			Source:  sourceStr,
			Targets: []string{},
		}

		// Extract lite rows for targets
		for _, target := range mapRes.Targets {
			// Set schema and target dataset from first target
			if targetDataset == "" {
				targetDataset = config.DataconfIDIntToString[target.Dataset]
				targetLiteFields = config.GetCompactFields(targetDataset)
				response.Schema = GetCompactSchema(targetLiteFields)
				response.Context.TargetDataset = targetDataset
			}

			row := GetCompactRow(target, targetLiteFields)
			mapping.Targets = append(mapping.Targets, row)
			response.Stats.Total++
		}

		response.Mappings = append(response.Mappings, mapping)

		if len(mapRes.Targets) > 0 {
			response.Stats.Mapped++
		}
	}

	// Calculate not_found: input terms that weren't in any result
	for _, id := range ids {
		if !foundInputs[id] {
			response.NotFound = append(response.NotFound, id)
		}
	}

	return response, nil
}

type mpPage struct {
	sourceID   string // at current level of query which source processed
	entryIndex int    // at current level of where we last left for entries
	page       int
}

type mpInPage struct {
	id      string
	source  uint32
	rawpage string
}

// rootPage is like search paging and second level paging is for each  mapping
// MapFilter performs cross-reference mapping between datasets
func (s *Service) MapFilter(ids []string, mapFilterQuery, page string) (*pbuf.MapFilterResult, error) {
	return s.MapFilterWithLimit(ids, mapFilterQuery, page, s.maxMappingResult)
}

// mapFilterWithLimit is the internal implementation with configurable result limit
// MapFilterWithLimit performs cross-reference mapping with configurable result limit
func (s *Service) MapFilterWithLimit(ids []string, mapFilterQuery, page string, maxResults int) (*pbuf.MapFilterResult, error) {

	startTime := time.Now()

	result := pbuf.MapFilterResult{}

	cacheKey := s.mapFilterCacheKey(ids, mapFilterQuery, page)

	// Check cache only if not disabled
	if !s.cacheDisabled {
		resultFromCache, found := s.filterResultCache.Get(cacheKey)

		if found {

			err := proto.Unmarshal(resultFromCache.([]byte), &result)
			if err != nil {
				fmt.Println(err)
				return nil, err
			}

			return &result, nil

		}
	}

	var respaging strings.Builder

	queries, err := s.prepareQueries(mapFilterQuery)

	if err != nil {
		return nil, err
	}

	newRootPage, pages, err := s.parsePagingKey(page)

	if err != nil {
		return nil, err
	}

	// Extract lookup dataset from first query (if IsLookup == true)
	var lookupDatasetFilters []uint32
	var filterQ *query.Query

	if len(queries) > 0 && queries[0].IsLookup {
		// First query is the lookup dataset
		if queries[0].MapDatasetID > 0 {
			lookupDatasetFilters = []uint32{queries[0].MapDatasetID}
		}
		if len(queries[0].Filter) > 0 {
			filterQ = &queries[0]
		}
		// Remove lookup query from chain (it's not part of xref mapping)
		queries = queries[1:]
	} else if len(queries) > 0 && queries[0].MapDatasetID <= 0 {
		// Filter-only case (existing logic for backward compatibility)
		filterQ = &queries[0]
		queries = queries[1:]
	}

startMapping:
	inputXrefs, newRootPage, err := s.inputXrefs(ids, lookupDatasetFilters, filterQ, newRootPage, pages)

	if err != nil {
		return nil, err
	}

	if len(inputXrefs) == 0 {
		// No results in this page - check if there are more pages to search
		if newRootPage != "" {
			// Filter removed all entries from this page, continue to next page
			goto startMapping
		}
		// No more pages - return empty result with message instead of error
		result.Message = "No results found"
		EnrichMapFilterResult(&result)
		return &result, nil
	}

	for _, xref := range inputXrefs {

		if time.Since(startTime).Seconds() > s.mapFilterTimeoutDuration {
			err := fmt.Errorf("Query time out. Consider reviewing the query or use local version if this is demo version")
			return nil, err
		}

		var newpages map[int]*mpPage
		var finaltargets []*pbuf.Xref

		if len(queries) == 0 {

			//todo this case is just filter to itself for now source = target handle differently also can be outside from this loop
			finaltargets = append(finaltargets, xref)

		} else {

			if pages == nil {
				finaltargets, newpages, err = s.xrefMapping(queries, xref, nil, maxResults)
				if err != nil { // todo maybe in this case it should continue?
					return nil, err
				}
			} else {
				if _, ok := pages[xref.Identifier]; ok {
					if _, ok := pages[xref.Identifier][xref.Dataset]; ok {
						finaltargets, newpages, err = s.xrefMapping(queries, xref, pages[xref.Identifier][xref.Dataset], maxResults)
						if err != nil { // todo maybe in this case it should continue??
							return nil, err
						}
					} else {
						err := fmt.Errorf("Invalid paging request")
						return nil, err
					}
				} else {
					err := fmt.Errorf("Invalid paging request")
					return nil, err
				}
			}

		}

		if len(finaltargets) == 0 {
			continue
		}

		mapfil := pbuf.MapFilter{}
		s.makeLite(xref)
		mapfil.Source = xref
		for _, targ := range finaltargets {
			s.makeLite(targ)
		}
		mapfil.Targets = finaltargets
		// create next page string
		if len(newpages) > 0 {

			page := s.setResultPaging(xref, newpages)
			if page != "" {
				respaging.WriteString(page)
				respaging.WriteString(pagingSep4)
			}

		}

		result.Results = append(result.Results, &mapfil)

	}

	if len(result.Results) == 0 && newRootPage != "" { // todo in here maybe based on the results count
		if len(respaging.String()) == 0 { // this means while looking in the current pages everthing finished but nothing found e.g hgnc:1100 so root pages needs to be moved
			pages = nil
		}
		goto startMapping
	}

	// set cache (only if not disabled)
	setCache := func() error {
		if s.cacheDisabled {
			return nil
		}
		resultBytes, err := proto.Marshal(&result)
		if err != nil {
			err := fmt.Errorf("Error while setting result to cache")
			return err
		}

		//TODO for now with marshall and unmarshall but should be with estimated size of result to set cost
		s.filterResultCache.Set(cacheKey, resultBytes, int64(len(resultBytes)))

		return err
	}

	// return result
	// Enrich with all transient fields (dataset_name, url)
	EnrichMapFilterResult(&result)

	if newRootPage == "" && len(respaging.String()) == 0 {
		setCache()
		return &result, nil
	}

	if newRootPage == "" {
		newRootPage = "-1"
	}
	if len(respaging.String()) == 0 {
		result.Nextpage = newRootPage + pagingSep3 + "-1"
		setCache()
		return &result, nil
	}
	result.Nextpage = newRootPage + pagingSep3 + respaging.String()
	setCache()
	return &result, nil

}

func (s *Service) mapFilterCacheKey(ids []string, mapFilterQuery, page string) string {

	var str strings.Builder
	for _, id := range ids {
		str.WriteString(id)
		str.WriteString(",")
	}
	str.WriteString(mapFilterQuery)
	if len(page) > 0 {
		str.WriteString(",")
		str.WriteString(page)
	}
	return str.String()

}

func (s *Service) inputXrefs(ids []string, datasetFilters []uint32, filterq *query.Query, rootPage string, pages map[string]map[uint32]map[int]*mpPage) ([]*pbuf.Xref, string, error) {

	var inputXrefs []*pbuf.Xref

	if pages == nil {

		res, err := s.Search(ids, datasetFilters, rootPage, filterq, true, false)

		if err != nil {
			return nil, "", err
		}
		inputXrefs = res.Results
		// todo check if it needs to be nil
		return inputXrefs, res.Nextpage, nil

	}

	// paging request

	/**
	linkDatasetLen := 0
	for _, q := range queries {
		if q.IsLinkDataset {
			linkDatasetLen++
		}
	}
	**/

	for k, v := range pages {
		for k2 := range v {

			xref, err := s.LookupByDataset(k, k2)
			if err != nil {
				return nil, "", err
			}
			inputXrefs = append(inputXrefs, xref)
		}

	}

	return inputXrefs, rootPage, nil

}

func (s *Service) parsePagingKey(key string) (string, map[string]map[uint32]map[int]*mpPage, error) {

	if key == "" {
		return "", nil, nil
	}

	var rootPage string

	keys1 := strings.Split(key, pagingSep3) //[]

	if len(keys1) != 2 {
		err := fmt.Errorf("Invalid paging request. Error while getting root key " + key)
		return "", nil, err
	}

	if keys1[0] == "-1" {
		rootPage = ""
	} else {
		rootPage = keys1[0]
	}

	if keys1[1] == "-1" {
		return rootPage, nil, nil
	}

	res := map[string]map[uint32]map[int]*mpPage{}

	keys2 := strings.Split(keys1[1], pagingSep4) //][

	for _, perkey := range keys2 {

		perkeyVals := strings.Split(perkey, pagingSep2) // ,

		domainID, err := strconv.Atoi(perkeyVals[1])
		if err != nil {
			return "", nil, err
		}

		idPages := map[int]*mpPage{}

		for i := 2; i < len(perkeyVals); i += 3 {
			page := mpPage{}
			page.sourceID = perkeyVals[i]
			if i+2 < len(perkeyVals) {

				page.entryIndex, err = strconv.Atoi(perkeyVals[i+1])
				if err != nil {
					return "", nil, err
				}
				pageIndex, err := strconv.Atoi(perkeyVals[i+2])
				if err != nil {
					return "", nil, err
				}
				page.page = pageIndex
			} else {
				err := fmt.Errorf("Invalid paging request key")
				return "", nil, err
			}
			idPages[i/3] = &page
		}

		if _, ok := res[perkeyVals[0]]; !ok {
			m := map[uint32]map[int]*mpPage{}
			m[uint32(domainID)] = idPages
			res[perkeyVals[0]] = m
		} else {
			res[perkeyVals[0]][uint32(domainID)] = idPages
		}

		return rootPage, res, nil

	}

	err := fmt.Errorf("Invalid paging request paging request starts with e or r")
	return "", nil, err

}

func (s *Service) setResultPaging(source *pbuf.Xref, pages map[int]*mpPage) string {

	sorted := make([]int, len(pages))
	i := 0
	allProcessed := true
	for k, v := range pages {
		sorted[i] = k
		i++
		if v != nil {
			allProcessed = false
		}
	}
	if allProcessed {
		return ""
	}
	var b strings.Builder

	// first source id and domain_id
	b.WriteString(source.Identifier)
	b.WriteString(pagingSep2)
	b.WriteString(strconv.Itoa(int(source.Dataset)))
	b.WriteString(pagingSep2)

	sort.Ints(sorted)
	for _, i := range sorted {
		b.WriteString(pages[i].sourceID + pagingSep2 + strconv.Itoa(pages[i].entryIndex) + pagingSep2 + strconv.Itoa(pages[i].page))
		if i != len(pages)-1 {
			b.WriteString(pagingSep2)
		}
	}
	return b.String()

}

// parse mapping query and injects linkdataset queries if needed
func (s *Service) prepareQueries(mapFilterQuery string) ([]query.Query, error) {

	// Detect new syntax: doesn't start with map( or filter(
	// This routes to ParserV2 for the new intuitive syntax
	trimmed := strings.TrimSpace(mapFilterQuery)
	if !strings.HasPrefix(trimmed, "map(") && !strings.HasPrefix(trimmed, "filter(") {
		// New syntax - use V2 parser
		parser := query.NewParserV2(config)
		queries, err := parser.Parse(mapFilterQuery)
		if err != nil {
			return nil, err
		}

		// Handle link datasets (same logic as old parser)
		hasLinkDataset := false
		for _, q := range queries {
			if q.IsLinkDataset {
				hasLinkDataset = true
				break
			}
		}
		if hasLinkDataset {
			var newqueries []query.Query
			for index, q := range queries {
				if q.IsLinkDataset {
					if index == 0 {
						//err := fmt.Errorf("Query cannot start with linkdataset")
						//return nil, err
					}
					q2 := query.Query{}
					q2.MapDatasetID = config.DataconfIDStringToInt[config.Dataconf[q.MapDataset]["linkdataset"]]
					q2.MapDataset = config.Dataconf[q.MapDataset]["linkdataset"]

					if len(q.Filter) > 0 {
						q2.Filter = q.Filter
						q.Filter = ""
					}
					newqueries = append(newqueries, q)
					newqueries = append(newqueries, q2)
				} else {
					newqueries = append(newqueries, q)
				}
			}
			return newqueries, nil
		}
		return queries, nil
	}

	// Old syntax - use existing parser
	queries, err := s.qparser.Parse(mapFilterQuery)

	if err != nil {
		return nil, err
	}
	hasLinkDataset := false
	for _, q := range queries {
		if q.IsLinkDataset {
			hasLinkDataset = true
			break
		}
	}
	if hasLinkDataset {
		var newqueries []query.Query
		for index, q := range queries {

			if q.IsLinkDataset {

				if index == 0 {
					//err := fmt.Errorf("Query cannot start with linkdataset")
					//return nil, err
				}
				q2 := query.Query{}
				q2.MapDatasetID = config.DataconfIDStringToInt[config.Dataconf[q.MapDataset]["linkdataset"]]
				q2.MapDataset = config.Dataconf[q.MapDataset]["linkdataset"]

				if len(q.Filter) > 0 {
					q2.Filter = q.Filter
					// remove the filter form link one
					q.Filter = ""
				}
				newqueries = append(newqueries, q)
				newqueries = append(newqueries, q2)
			} else {
				newqueries = append(newqueries, q)
			}
		}
		return newqueries, nil
	}
	return queries, nil

}

func (s *Service) xrefMapping(queries []query.Query, xref *pbuf.Xref, inPages map[int]*mpPage, maxResults int) ([]*pbuf.Xref, map[int]*mpPage, error) {

	var err error

	var targets []*pbuf.Xref
	targetkeys := map[string]bool{} // prevents duplication

	sourceEntries := map[int][]*pbuf.XrefEntry{} // entries by for each query level
	sources := map[int]*pbuf.Xref{}

	markNoNextPaging := func() {
		for k := range inPages {
			inPages[k] = nil
		}
	}

	qind := 0
	if inPages != nil { // paging request. init previous state

		if len(inPages) != len(queries) {
			err := fmt.Errorf("Invalid request recieved query and pages size not equal")
			return nil, nil, err
		}

		var mapDatasetID uint32
		for i := 0; i < len(inPages); i++ {

			if i == 0 {
				mapDatasetID = xref.Dataset
			} else {
				mapDatasetID = queries[i-1].MapDatasetID
			}
			source, err := s.LookupByDataset(inPages[i].sourceID, mapDatasetID)

			if err != nil {
				return nil, nil, err
			}
			sources[i] = source
			sourceEntries[i] = nil
		}
		qind = len(queries) - 1

	} else { // first request

		inPages = map[int]*mpPage{}
		inPages[0] = &mpPage{page: -1, sourceID: xref.Identifier}
		sources[0] = xref

	}

	startTime := time.Now()

	for {
	start:

		q := &queries[qind] // Use pointer to preserve cached CEL program across iterations

		if sourceEntries[qind] == nil {
			sourceEntries[qind], err = s.getEntries(sources[qind], q.MapDatasetID, inPages[qind])
			if err != nil {
				return nil, nil, err
			}
		}
		if qind == len(queries)-1 {
		searchTargets:
			for _, entry := range sourceEntries[qind] {

				if len(targets) >= maxResults {
					inPages[qind].sourceID = sources[qind].Identifier
					goto finish
				}

				inPages[qind].entryIndex++

				if entry.Dataset == q.MapDatasetID {

					// OPTIMIZATION: Check for duplicate BEFORE expensive LMDB lookup
					// This prevents redundant DB reads for entries we already have
					targetKey := config.DataconfIDIntToString[entry.Dataset] + "_" + entry.Identifier
					if _, alreadyHave := targetkeys[targetKey]; alreadyHave {
						// Already have this target, skip LMDB lookup entirely
						continue
					}

					filterRes, target, err := s.applyFilter(entry, q)
					if err != nil {
						return nil, nil, err
					}

					if filterRes {
						targets = append(targets, target)
						targetkeys[targetKey] = true
					}

				}
			}

			if time.Since(startTime).Seconds() > s.mapFilterTimeoutDuration {
				err := fmt.Errorf("Query time out. Consider reviewing the query or use local version if this is demo version")
				return nil, nil, err
			}

			//now try next page
			hasNext, err := s.moveNextPage(sourceEntries, sources[qind], inPages, qind, q.MapDatasetID)

			if err != nil {
				return nil, nil, err
			}

			if hasNext {

				goto searchTargets

			} else {

				if qind == 0 {
					markNoNextPaging()
					goto finish
				}

				// rewind
				newqind := qind
				for {
					newqind--
					switch newqind {
					case 0:
						if inPages[0].page == -2 { // everything complete
							markNoNextPaging()
							goto finish
						}
						qind = newqind
						goto start
					default:
						if inPages[newqind] != nil && inPages[newqind].page != -2 {
							qind = newqind
							goto start
						}
					}
				}

			}
		} else {
		searchNextSource:
			nextSourceFound := false
			for entryIndex, entry := range sourceEntries[qind] {
				if entry.Dataset == q.MapDatasetID {

					filterRes, nextsource, err := s.applyFilter(entry, q)

					if err != nil {
						return nil, nil, err
					}

					if filterRes {
						s.moveEntries(sourceEntries, sources[qind], inPages, qind, q.MapDatasetID, entryIndex) // move existing source entries to start from here in next iteration
						sources[qind+1] = nextsource
						// set next source paging and entries
						sourceEntries[qind+1] = nil
						nextsrcPage := mpPage{}
						nextsrcPage.page = -1
						nextsrcPage.sourceID = nextsource.Identifier
						inPages[qind+1] = &nextsrcPage
						nextSourceFound = true
						qind++
						break
					}

				}
				inPages[qind].entryIndex++
			}

			if !nextSourceFound {

				if time.Since(startTime).Seconds() > s.mapFilterTimeoutDuration {
					err := fmt.Errorf("Query time out. Consider reviewing the query or use local version if this is demo version")
					return nil, nil, err
				}

				hasNext, err := s.moveNextPage(sourceEntries, sources[qind], inPages, qind, q.MapDatasetID)

				if err != nil {
					return nil, nil, err
				}

				if hasNext {
					goto searchNextSource
				}

				if qind == 0 { // everything complete
					markNoNextPaging()
					goto finish
				}

				// next source not found

				//clear
				sources[qind+1] = nil
				sourceEntries[qind+1] = nil
				//delete(sourceEntries,qind+1)
				inPages[qind+1] = nil

				//rewind
				newqind := qind
				for {
					newqind--
					switch newqind {
					case 0:
						if inPages[0].page == -2 { // everything complete
							markNoNextPaging()
							goto finish
						}
						qind = newqind
						goto start
					default:
						if inPages[newqind] != nil && inPages[newqind].page != -2 {
							qind = newqind
							goto start
						}
					}
				}

			}
		}
	}

finish:
	/**	if len(targets) == 0 {
		err := fmt.Errorf("No mapping found")
		return nil, nil, err
	}**/
	// set paging for each

	return targets, inPages, nil

}

func (s *Service) getEntries(xref *pbuf.Xref, mapDatasetID uint32, mpage *mpPage) ([]*pbuf.XrefEntry, error) {

	if mpage == nil { // root page first time
		return xref.Entries, nil
	}
	var entries []*pbuf.XrefEntry

	if mpage.page == -2 {

		return entries, nil

	} else if mpage.page == -1 { // root page

		if mpage.entryIndex == 0 {
			return xref.Entries, nil
		}

		// Bounds check to prevent panic if entryIndex exceeds entries length
		if mpage.entryIndex >= len(xref.Entries) {
			return entries, nil // Return empty slice
		}

		return xref.Entries[mpage.entryIndex:], nil

	} else {
		// Build page key: rootKey + \x00 + datasetKey(2 chars) + pageIndex
		page := xref.DatasetPages[mapDatasetID].Pages[mpage.page]
		pageKey := xref.Identifier + pageKeySep + config.DataconfIDToPageKey[xref.Dataset] + page
		source, err := s.LookupByDataset(pageKey, xref.Dataset)
		if err != nil {
			return nil, err
		}
		if mpage.entryIndex == 0 {
			return source.Entries, nil
		}

		// Bounds check to prevent panic if entryIndex exceeds entries length
		if mpage.entryIndex >= len(source.Entries) {
			return entries, nil // Return empty slice
		}

		return source.Entries[mpage.entryIndex:], nil

	}

	return entries, nil

}

func (s *Service) moveNextPage(entryMap map[int][]*pbuf.XrefEntry, source *pbuf.Xref, inPages map[int]*mpPage, index int, MapDatasetID uint32) (bool, error) {

	var err error
	if _, ok := source.DatasetPages[MapDatasetID]; ok && inPages[index].page+1 < len(source.DatasetPages[MapDatasetID].Pages) {
		inPages[index].page = inPages[index].page + 1
		inPages[index].entryIndex = 0
		entryMap[index], err = s.getEntries(source, MapDatasetID, inPages[index])
		if err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil

}

func (s *Service) moveEntries(sourceEntries map[int][]*pbuf.XrefEntry, source *pbuf.Xref, inPages map[int]*mpPage, qind int, MapDatasetID uint32, entryIndex int) {

	if entryIndex+1 < len(sourceEntries[qind]) {
		sourceEntries[qind] = sourceEntries[qind][entryIndex+1:]
		inPages[qind].entryIndex = inPages[qind].entryIndex + 1
	} else {
		if _, ok := source.DatasetPages[MapDatasetID]; ok && inPages[qind].page+1 < len(source.DatasetPages[MapDatasetID].Pages) {
			inPages[qind].page = inPages[qind].page + 1
			inPages[qind].entryIndex = 0
			sourceEntries[qind] = nil // because maybe not needed to fill
		} else {
			inPages[qind].page = -2 // no more page
		}
	}

}

func (s *Service) applyFilter(entry *pbuf.XrefEntry, q *query.Query) (bool, *pbuf.Xref, error) {

	// OPTIMIZATION: Check filter cache BEFORE doing expensive LMDB lookup
	// This prevents repeated DB reads for entries we already know will fail the filter
	if len(q.Filter) > 0 && !s.cacheDisabled && s.filterResultCache != nil {
		cacheKey := "f_" + entry.Identifier + "_" + strconv.Itoa(int(entry.Dataset)) + q.Filter
		if cached, found := s.filterResultCache.Get(cacheKey); found {
			if !cached.(bool) {
				// Already know this entry fails the filter - skip DB lookup entirely
				return false, nil, nil
			}
			// If true, we still need to fetch the target data, so continue
		}
	}

	target, err := s.LookupByDataset(entry.Identifier, entry.Dataset)
	if err != nil {
		// Entry not found as primary entry in target dataset - skip it
		// This can happen when xrefs point to identifiers that aren't indexed
		// (e.g., LRG_321 listed as ensembl xref but not stored as primary entry)
		return false, nil, nil
	}

	if len(q.Filter) == 0 {
		return true, target, nil
	}

	filterRes, err := s.execCelGo(q, target)
	if err != nil {
		return false, nil, err
	}
	return filterRes, target, nil

}

func (s *Service) execCelGo(query *query.Query, targetXref *pbuf.Xref) (bool, error) {

	if targetXref.GetEmpty() {
		//err := fmt.Errorf("Filtered entry has not indexed for filtering->" + targetXref.Identifier)
		// think again this
		return false, nil
	}

	// look in cache f_ is just differentiate with mapfilter can be better...
	cacheKey := "f_" + targetXref.Identifier + "_" + strconv.Itoa(int(targetXref.Dataset)) + query.Filter
	if !s.cacheDisabled && s.filterResultCache != nil {
		entry, found := s.filterResultCache.Get(cacheKey)
		if found {
			if entry.(bool) {
				return true, nil
			}
			return false, nil
		}
	}

	/**
	if s.filterResultCache.Len() > 100000 {
		fmt.Println("cache size->", s.filterResultCache.Len())
	}
	**/

	var err error

	if query.Program == nil {

		parsed, issues := s.celgoEnv.Parse(query.Filter)
		if issues != nil && issues.Err() != nil {
			err := fmt.Errorf("parse error: %s", issues.Err())
			return false, err
		}

		checked, issues := s.celgoEnv.Check(parsed)
		if issues != nil && issues.Err() != nil {
			err := fmt.Errorf("type-check error: %s", issues.Err())
			return false, err
		}

		prg, err := s.celgoEnv.Program(checked, s.celProgOpts)

		if err != nil {
			err := fmt.Errorf("program construction error: %s", err)
			return false, err
		}

		query.Program = prg

	}

	var out ref.Val

	// Build evaluation map for CEL filtering
	// Set transient id field on attributes for filtering like: >>go[id=="GO:0005886"]
	evalMap := map[string]interface{}{}
	id := targetXref.Identifier

	switch query.MapDataset {
	case "uniprot":
		if attr := targetXref.GetUniprot(); attr != nil {
			attr.Id = id
			if len(attr.Names) > 0 {
				attr.Name = attr.Names[0]
			}
			evalMap["uniprot"] = attr
		}
	case "ufeature":
		// UniprotFeatureAttr already has id field (field 3)
		evalMap["ufeature"] = targetXref.GetUfeature()
	case "ensembl":
		if attr := targetXref.GetEnsembl(); attr != nil {
			attr.Id = id
			evalMap["ensembl"] = attr
		}
	case "transcript":
		if attr := targetXref.GetEnsembl(); attr != nil {
			attr.Id = id
			evalMap["transcript"] = attr
		}
	case "exon":
		if attr := targetXref.GetEnsembl(); attr != nil {
			attr.Id = id
			evalMap["exon"] = attr
		}
	case "cds":
		if attr := targetXref.GetEnsembl(); attr != nil {
			attr.Id = id
			evalMap["cds"] = attr
		}
	case "taxonomy":
		if attr := targetXref.GetTaxonomy(); attr != nil {
			attr.Id = id
			evalMap["taxonomy"] = attr
		}
	case "hgnc":
		if attr := targetXref.GetHgnc(); attr != nil {
			attr.Id = id
			if len(attr.Names) > 0 {
				attr.Name = attr.Names[0]
			}
			evalMap["hgnc"] = attr
		}
	case "go", "efo", "eco", "mondo", "uberon", "oba", "cl", "pato", "obi", "xco", "bao":
		if attr := targetXref.GetOntology(); attr != nil {
			attr.Id = id
			evalMap[query.MapDataset] = attr
		}
	case "hpo":
		if attr := targetXref.GetHpoAttr(); attr != nil {
			attr.Id = id
			evalMap["hpo"] = attr
		}
	case "chembl_molecule":
		if attr := targetXref.GetChembl().GetMolecule(); attr != nil {
			attr.Id = id
			evalMap["chembl_molecule"] = attr
		}
	case "chembl_target":
		if attr := targetXref.GetChembl().GetTarget(); attr != nil {
			attr.Id = id
			evalMap["chembl_target"] = attr
		}
	case "chembl_activity":
		if attr := targetXref.GetChembl().GetActivity(); attr != nil {
			attr.Id = id
			evalMap["chembl_activity"] = attr
		}
	case "chembl_assay":
		if attr := targetXref.GetChembl().GetAssay(); attr != nil {
			attr.Id = id
			evalMap["chembl_assay"] = attr
		}
	case "chembl_document":
		if attr := targetXref.GetChembl().GetDoc(); attr != nil {
			attr.Id = id
			evalMap["chembl_document"] = attr
		}
	case "chembl_cell_line":
		if attr := targetXref.GetChembl().GetCellLine(); attr != nil {
			attr.Id = id
			evalMap["chembl_cell_line"] = attr
		}
	case "pubchem":
		if attr := targetXref.GetPubchem(); attr != nil {
			attr.Id = id
			evalMap["pubchem"] = attr
		}
	case "pubchem_activity":
		if attr := targetXref.GetPubchemActivity(); attr != nil {
			attr.Id = id
			evalMap["pubchem_activity"] = attr
		}
	case "pubchem_assay":
		if attr := targetXref.GetPubchemAssay(); attr != nil {
			attr.Id = id
			evalMap["pubchem_assay"] = attr
		}
	case "interpro":
		if attr := targetXref.GetInterpro(); attr != nil {
			attr.Id = id
			evalMap["interpro"] = attr
		}
	case "ena":
		if attr := targetXref.GetEna(); attr != nil {
			attr.Id = id
			evalMap["ena"] = attr
		}
	case "hmdb":
		if attr := targetXref.GetHmdb(); attr != nil {
			attr.Id = id
			evalMap["hmdb"] = attr
		}
	case "chebi":
		if attr := targetXref.GetChebi(); attr != nil {
			attr.Id = id
			evalMap["chebi"] = attr
		}
	case "pdb":
		if attr := targetXref.GetPdb(); attr != nil {
			attr.Id = id
			evalMap["pdb"] = attr
		}
	case "drugbank":
		if attr := targetXref.GetDrugbank(); attr != nil {
			attr.Id = id
			evalMap["drugbank"] = attr
		}
	case "orphanet":
		if attr := targetXref.GetOrphanet(); attr != nil {
			attr.Id = id
			evalMap["orphanet"] = attr
		}
	case "reactome":
		if attr := targetXref.GetReactome(); attr != nil {
			attr.Id = id
			evalMap["reactome"] = attr
		}
	case "lipidmaps":
		if attr := targetXref.GetLipidmaps(); attr != nil {
			attr.Id = id
			evalMap["lipidmaps"] = attr
		}
	case "swisslipids":
		if attr := targetXref.GetSwisslipids(); attr != nil {
			attr.Id = id
			evalMap["swisslipids"] = attr
		}
	case "bgee":
		if attr := targetXref.GetBgee(); attr != nil {
			attr.Id = id
			evalMap["bgee"] = attr
		}
	case "bgee_evidence":
		if attr := targetXref.GetBgeeEvidence(); attr != nil {
			attr.Id = id
			evalMap["bgee_evidence"] = attr
		}
	case "rhea":
		if attr := targetXref.GetRhea(); attr != nil {
			attr.Id = id
			evalMap["rhea"] = attr
		}
	case "gwas_study":
		if attr := targetXref.GetGwasStudy(); attr != nil {
			attr.Id = id
			evalMap["gwas_study"] = attr
		}
	case "gwas":
		if attr := targetXref.GetGwas(); attr != nil {
			attr.Id = id
			evalMap["gwas"] = attr
		}
	case "intact":
		if attr := targetXref.GetIntact(); attr != nil {
			attr.Id = id
			evalMap["intact"] = attr
		}
	case "dbsnp":
		if attr := targetXref.GetDbsnp(); attr != nil {
			attr.Id = id
			evalMap["dbsnp"] = attr
		}
	case "clinvar":
		if attr := targetXref.GetClinvar(); attr != nil {
			attr.Id = id
			evalMap["clinvar"] = attr
		}
	case "antibody":
		if attr := targetXref.GetAntibody(); attr != nil {
			attr.Id = id
			evalMap["antibody"] = attr
		}
	case "esm2_similarity":
		if attr := targetXref.GetEsm2Similarity(); attr != nil {
			attr.Id = id
			evalMap["esm2_similarity"] = attr
		}
	case "entrez":
		if attr := targetXref.GetEntrez(); attr != nil {
			attr.Id = id
			evalMap["entrez"] = attr
		}
	case "refseq":
		if attr := targetXref.GetRefseq(); attr != nil {
			attr.Id = id
			evalMap["refseq"] = attr
		}
	case "gencc":
		if attr := targetXref.GetGencc(); attr != nil {
			attr.Id = id
			evalMap["gencc"] = attr
		}
	case "bindingdb":
		if attr := targetXref.GetBindingdb(); attr != nil {
			attr.Id = id
			evalMap["bindingdb"] = attr
		}
	case "ctd":
		if attr := targetXref.GetCtd(); attr != nil {
			attr.Id = id
			evalMap["ctd"] = attr
		}
	case "ctd_gene_interaction":
		if attr := targetXref.GetCtdGeneInteraction(); attr != nil {
			attr.Id = id
			evalMap["ctd_gene_interaction"] = attr
		}
	case "ctd_disease_association":
		if attr := targetXref.GetCtdDiseaseAssociation(); attr != nil {
			attr.Id = id
			evalMap["ctd_disease_association"] = attr
		}
	case "biogrid":
		if attr := targetXref.GetBiogrid(); attr != nil {
			attr.Id = id
			evalMap["biogrid"] = attr
		}
	case "biogrid_interaction":
		if attr := targetXref.GetBiogridInteraction(); attr != nil {
			attr.Id = id
			evalMap["biogrid_interaction"] = attr
		}
	case "msigdb":
		if attr := targetXref.GetMsigdb(); attr != nil {
			attr.Id = id
			evalMap["msigdb"] = attr
		}
	case "alphamissense":
		if attr := targetXref.GetAlphamissense(); attr != nil {
			attr.Id = id
			evalMap["alphamissense"] = attr
		}
	case "alphamissense_transcript":
		if attr := targetXref.GetAlphamissenseTranscript(); attr != nil {
			attr.Id = id
			evalMap["alphamissense_transcript"] = attr
		}
	case "pharmgkb":
		if attr := targetXref.GetPharmgkb(); attr != nil {
			attr.Id = id
			evalMap["pharmgkb"] = attr
		}
	case "pharmgkb_gene":
		if attr := targetXref.GetPharmgkbGene(); attr != nil {
			attr.Id = id
			evalMap["pharmgkb_gene"] = attr
		}
	case "pharmgkb_clinical":
		if attr := targetXref.GetPharmgkbClinical(); attr != nil {
			attr.Id = id
			evalMap["pharmgkb_clinical"] = attr
		}
	case "pharmgkb_variant":
		if attr := targetXref.GetPharmgkbVariant(); attr != nil {
			attr.Id = id
			evalMap["pharmgkb_variant"] = attr
		}
	case "pharmgkb_guideline":
		if attr := targetXref.GetPharmgkbGuideline(); attr != nil {
			attr.Id = id
			evalMap["pharmgkb_guideline"] = attr
		}
	case "pharmgkb_pathway":
		if attr := targetXref.GetPharmgkbPathway(); attr != nil {
			attr.Id = id
			evalMap["pharmgkb_pathway"] = attr
		}
	case "cellxgene":
		if attr := targetXref.GetCellxgene(); attr != nil {
			attr.Id = id
			evalMap["cellxgene"] = attr
		}
	case "cellxgene_celltype":
		if attr := targetXref.GetCellxgeneCelltype(); attr != nil {
			attr.Id = id
			evalMap["cellxgene_celltype"] = attr
		}
	case "scxa":
		if attr := targetXref.GetScxa(); attr != nil {
			attr.Id = id
			evalMap["scxa"] = attr
		}
	case "scxa_expression":
		if attr := targetXref.GetScxaExpression(); attr != nil {
			attr.Id = id
			evalMap["scxa_expression"] = attr
		}
	case "scxa_gene_experiment":
		if attr := targetXref.GetScxaGeneExperiment(); attr != nil {
			attr.Id = id
			evalMap["scxa_gene_experiment"] = attr
		}
	case "alphafold":
		if attr := targetXref.GetAlphafold(); attr != nil {
			attr.Id = id
			evalMap["alphafold"] = attr
		}
	case "clinical_trials":
		if attr := targetXref.GetClinicalTrials(); attr != nil {
			attr.Id = id
			evalMap["clinical_trials"] = attr
		}
	case "collectri":
		if attr := targetXref.GetCollectri(); attr != nil {
			attr.Id = id
			evalMap["collectri"] = attr
		}
	case "brenda":
		if attr := targetXref.GetBrenda(); attr != nil {
			attr.Id = id
			evalMap["brenda"] = attr
		}
	case "brenda_kinetics":
		if attr := targetXref.GetBrendaKinetics(); attr != nil {
			attr.Id = id
			evalMap["brenda_kinetics"] = attr
		}
	case "brenda_inhibitor":
		if attr := targetXref.GetBrendaInhibitor(); attr != nil {
			attr.Id = id
			evalMap["brenda_inhibitor"] = attr
		}
	case "cellphonedb":
		if attr := targetXref.GetCellphonedb(); attr != nil {
			attr.Id = id
			evalMap["cellphonedb"] = attr
		}
	case "spliceai":
		if attr := targetXref.GetSpliceai(); attr != nil {
			attr.Id = id
			evalMap["spliceai"] = attr
		}
	case "mirdb":
		if attr := targetXref.GetMirdb(); attr != nil {
			attr.Id = id
			evalMap["mirdb"] = attr
		}
	case "fantom5_promoter":
		if attr := targetXref.GetFantom5Promoter(); attr != nil {
			attr.Id = id
			evalMap["fantom5_promoter"] = attr
		}
	case "fantom5_enhancer":
		if attr := targetXref.GetFantom5Enhancer(); attr != nil {
			attr.Id = id
			evalMap["fantom5_enhancer"] = attr
		}
	case "fantom5_gene":
		if attr := targetXref.GetFantom5Gene(); attr != nil {
			attr.Id = id
			evalMap["fantom5_gene"] = attr
		}
	case "jaspar":
		if attr := targetXref.GetJaspar(); attr != nil {
			attr.Id = id
			evalMap["jaspar"] = attr
		}
	case "encode_ccre":
		if attr := targetXref.GetEncodeCcre(); attr != nil {
			attr.Id = id
			evalMap["encode_ccre"] = attr
		}
	case "signor":
		if attr := targetXref.GetSignor(); attr != nil {
			attr.Id = id
			evalMap["signor"] = attr
		}
	case "corum":
		if attr := targetXref.GetCorum(); attr != nil {
			attr.Id = id
			evalMap["corum"] = attr
		}
	case "string":
		if attr := targetXref.GetStringattr(); attr != nil {
			attr.Id = id
			evalMap["stringdb"] = attr
		}
	case "string_interaction":
		if attr := targetXref.GetStringInteraction(); attr != nil {
			attr.Id = id
			evalMap["string_interaction"] = attr
		}
	case "gtopdb":
		if attr := targetXref.GetGtopdb(); attr != nil {
			attr.Id = id
			evalMap["gtopdb"] = attr
		}
	case "gtopdb_ligand":
		if attr := targetXref.GetGtopdbLigand(); attr != nil {
			attr.Id = id
			evalMap["gtopdb_ligand"] = attr
		}
	case "gtopdb_interaction":
		if attr := targetXref.GetGtopdbInteraction(); attr != nil {
			attr.Id = id
			evalMap["gtopdb_interaction"] = attr
		}
	default:
		//err := fmt.Errorf("mapfilter query execution failed please check again query")
		return false, nil
	}

	out, _, err = query.Program.Eval(evalMap)

	if err != nil {
		err := fmt.Errorf("mapfilter query execution failed: %s", err)
		return false, err
	}

	if out != nil {

		var res, ok bool
		if res, ok = out.Value().(bool); !ok {
			if !s.cacheDisabled && s.filterResultCache != nil {
				s.filterResultCache.Set(cacheKey, false, 1)
			}
			return false, nil
		}

		if res { // todo think again conversion
			if !s.cacheDisabled && s.filterResultCache != nil {
				s.filterResultCache.Set(cacheKey, true, 1)
			}
			return true, nil
		}

	}

	if !s.cacheDisabled && s.filterResultCache != nil {
		s.filterResultCache.Set(cacheKey, false, 1)
	}
	return false, nil

}
