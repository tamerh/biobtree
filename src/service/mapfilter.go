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
func (s *Service) MapFilterLite(ids []string, mapFilterQuery, page string) (*pbuf.MapFilterResultLite, error) {

	// Determine if this is a pagination request (page > 1)
	isFirstPage := page == ""
	currentPage := int32(1)
	if !isFirstPage {
		// Parse page number from the page token (format: "pageNum,..." or just use page count)
		// The page token format is complex, so we just increment from 1 for subsequent pages
		// A simple heuristic: if page param exists, it's page 2+
		currentPage = 2 // For now, mark as page 2 for any pagination request
	}

	result := &pbuf.MapFilterResultLite{
		Mode: "lite",
		Query: &pbuf.MapFilterQueryInfo{
			Terms: ids,
			Chain: mapFilterQuery,
			Raw:   strings.Join(ids, ",") + " " + mapFilterQuery,
		},
	}

	// Call mapFilterWithLimit with higher limit for lite mode (5x more results per page)
	fullResult, err := s.MapFilterWithLimit(ids, mapFilterQuery, page, s.maxMappingResultLite)
	if err != nil {
		// Return error for entire request
		return nil, err
	}

	// Track statistics
	var mapped, failed int32
	var totalTargets int32
	var warnings []string

	// Build a map of input terms to track which ones were found
	// Only track for first page - subsequent pages don't report "not found" errors
	inputTermsMap := make(map[string]bool)
	if isFirstPage {
		for _, term := range ids {
			inputTermsMap[strings.ToUpper(term)] = false // not found yet
		}
	}

	// Process results from full mapFilter
	for _, mapRes := range fullResult.Results {
		if mapRes.Source == nil {
			continue
		}

		// Mark this input term as found (use Keyword which contains original search term)
		// Only track on first page
		if isFirstPage {
			keyword := strings.ToUpper(mapRes.Source.Keyword)
			if keyword != "" {
				inputTermsMap[keyword] = true
			}
		}

		mapLite := &pbuf.MapFilterLite{
			Input: mapRes.Source.Identifier,
			Source: &pbuf.LiteEntry{
				D:       config.DataconfIDIntToString[mapRes.Source.Dataset],
				Id:      mapRes.Source.Identifier,
				HasAttr: !mapRes.Source.GetEmpty(),
			},
		}

		// Convert targets to lite format
		var liteTargets []*pbuf.LiteEntry
		for _, target := range mapRes.Targets {
			liteTarget := &pbuf.LiteEntry{
				D:       config.DataconfIDIntToString[target.Dataset],
				Id:      target.Identifier,
				HasAttr: !target.GetEmpty(),
			}
			liteTargets = append(liteTargets, liteTarget)
			totalTargets++
		}

		// Sort targets: entries with attributes first
		sort.Slice(liteTargets, func(i, j int) bool {
			if liteTargets[i].HasAttr != liteTargets[j].HasAttr {
				return liteTargets[i].HasAttr // true (has attr) comes first
			}
			return false // stable sort for same has_attr value
		})

		mapLite.Targets = liteTargets

		if len(liteTargets) > 0 {
			mapped++
		}

		result.Mappings = append(result.Mappings, mapLite)
	}

	// Add entries for input terms that weren't found (only on first page)
	if isFirstPage {
		for term, found := range inputTermsMap {
			if !found {
				failed++
				mapLite := &pbuf.MapFilterLite{
					Input: term,
					Error: "No mapping found",
				}
				result.Mappings = append(result.Mappings, mapLite)
			}
		}

		// Handle empty results message
		if len(fullResult.Results) == 0 && fullResult.Message != "" {
			// All terms failed
			failed = int32(len(ids))
			for _, term := range ids {
				mapLite := &pbuf.MapFilterLite{
					Input: term,
					Error: fullResult.Message,
				}
				result.Mappings = append(result.Mappings, mapLite)
			}
		}
	}

	// Set statistics
	result.Stats = &pbuf.MapFilterStats{
		TotalTerms:   int32(len(ids)),
		Mapped:       mapped,
		Failed:       failed,
		TotalTargets: totalTargets,
	}

	// Set pagination from full result
	hasNext := fullResult.Nextpage != ""
	result.Pagination = &pbuf.PaginationInfo{
		Page:      currentPage,
		HasNext:   hasNext,
		NextToken: fullResult.Nextpage,
	}

	if len(warnings) > 0 {
		result.Warnings = warnings
	}

	return result, nil
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
	var lookupDatasetID uint32
	var filterQ *query.Query

	if len(queries) > 0 && queries[0].IsLookup {
		// First query is the lookup dataset
		lookupDatasetID = queries[0].MapDatasetID
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
	inputXrefs, newRootPage, err := s.inputXrefs(ids, lookupDatasetID, filterQ, newRootPage, pages)

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

func (s *Service) inputXrefs(ids []string, idsDomain uint32, filterq *query.Query, rootPage string, pages map[string]map[uint32]map[int]*mpPage) ([]*pbuf.Xref, string, error) {

	var inputXrefs []*pbuf.Xref

	if pages == nil {

		res, err := s.Search(ids, idsDomain, rootPage, filterq, true, false)

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

		return xref.Entries[mpage.entryIndex:], nil

	} else {

		page := xref.DatasetPages[mapDatasetID].Pages[mpage.page]
		pageKey := xref.Identifier + spacestr + config.DataconfIDToPageKey[xref.Dataset] + spacestr + page
		source, err := s.LookupByDataset(pageKey, xref.Dataset)
		if err != nil {
			return nil, err
		}
		if mpage.entryIndex == 0 {
			return source.Entries, nil
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

	switch query.MapDataset {
	case "uniprot":
		out, _, err = query.Program.Eval(map[string]interface{}{"uniprot": targetXref.GetUniprot()})
	case "ufeature":
		out, _, err = query.Program.Eval(map[string]interface{}{"ufeature": targetXref.GetUfeature()})
	case "ensembl":
		out, _, err = query.Program.Eval(map[string]interface{}{"ensembl": targetXref.GetEnsembl()})
	case "transcript":
		out, _, err = query.Program.Eval(map[string]interface{}{"transcript": targetXref.GetEnsembl()})
	case "exon":
		out, _, err = query.Program.Eval(map[string]interface{}{"exon": targetXref.GetEnsembl()})
	case "cds":
		out, _, err = query.Program.Eval(map[string]interface{}{"cds": targetXref.GetEnsembl()})
	case "taxonomy":
		out, _, err = query.Program.Eval(map[string]interface{}{"taxonomy": targetXref.GetTaxonomy()})
	case "hgnc":
		out, _, err = query.Program.Eval(map[string]interface{}{"hgnc": targetXref.GetHgnc()})
	case "go", "efo", "eco", "mondo", "uberon", "oba", "cl", "pato", "obi", "xco", "bao":
		out, _, err = query.Program.Eval(map[string]interface{}{query.MapDataset: targetXref.GetOntology()})
	case "hpo":
		out, _, err = query.Program.Eval(map[string]interface{}{"hpo": targetXref.GetHpoAttr()})
	case "chembl_document", "chembl_assay", "chembl_activity", "chembl_molecule", "chembl_target", "chembl_target_component", "chembl_cell_line":
		out, _, err = query.Program.Eval(map[string]interface{}{"chembl": targetXref.GetChembl()})
	case "pubchem":
		out, _, err = query.Program.Eval(map[string]interface{}{"pubchem": targetXref.GetPubchem()})
	case "pubchem_activity":
		out, _, err = query.Program.Eval(map[string]interface{}{"pubchem_activity": targetXref.GetPubchemActivity()})
	case "pubchem_assay":
		out, _, err = query.Program.Eval(map[string]interface{}{"pubchem_assay": targetXref.GetPubchemAssay()})
	case "interpro":
		out, _, err = query.Program.Eval(map[string]interface{}{"interpro": targetXref.GetInterpro()})
	case "ena":
		out, _, err = query.Program.Eval(map[string]interface{}{"ena": targetXref.GetEna()})
	case "hmdb":
		out, _, err = query.Program.Eval(map[string]interface{}{"hmdb": targetXref.GetHmdb()})
	case "chebi":
		out, _, err = query.Program.Eval(map[string]interface{}{"chebi": targetXref.GetChebi()})
	case "pdb":
		out, _, err = query.Program.Eval(map[string]interface{}{"pdb": targetXref.GetPdb()})
	case "drugbank":
		out, _, err = query.Program.Eval(map[string]interface{}{"drugbank": targetXref.GetDrugbank()})
	case "orphanet":
		out, _, err = query.Program.Eval(map[string]interface{}{"orphanet": targetXref.GetOrphanet()})
	case "reactome":
		out, _, err = query.Program.Eval(map[string]interface{}{"reactome": targetXref.GetReactome()})
	case "lipidmaps":
		out, _, err = query.Program.Eval(map[string]interface{}{"lipidmaps": targetXref.GetLipidmaps()})
	case "swisslipids":
		out, _, err = query.Program.Eval(map[string]interface{}{"swisslipids": targetXref.GetSwisslipids()})
	case "bgee":
		out, _, err = query.Program.Eval(map[string]interface{}{"bgee": targetXref.GetBgee()})
	case "rhea":
		out, _, err = query.Program.Eval(map[string]interface{}{"rhea": targetXref.GetRhea()})
	case "gwas_study":
		out, _, err = query.Program.Eval(map[string]interface{}{"gwas_study": targetXref.GetGwasStudy()})
	case "gwas":
		out, _, err = query.Program.Eval(map[string]interface{}{"gwas": targetXref.GetGwas()})
	case "intact":
		out, _, err = query.Program.Eval(map[string]interface{}{"intact": targetXref.GetIntact()})
	case "dbsnp":
		out, _, err = query.Program.Eval(map[string]interface{}{"dbsnp": targetXref.GetDbsnp()})
	case "clinvar":
		out, _, err = query.Program.Eval(map[string]interface{}{"clinvar": targetXref.GetClinvar()})
	case "antibody":
		out, _, err = query.Program.Eval(map[string]interface{}{"antibody": targetXref.GetAntibody()})
	case "esm2_similarity":
		out, _, err = query.Program.Eval(map[string]interface{}{"esm2_similarity": targetXref.GetEsm2Similarity()})
	case "entrez":
		out, _, err = query.Program.Eval(map[string]interface{}{"entrez": targetXref.GetEntrez()})
	case "refseq":
		out, _, err = query.Program.Eval(map[string]interface{}{"refseq": targetXref.GetRefseq()})
	case "gencc":
		out, _, err = query.Program.Eval(map[string]interface{}{"gencc": targetXref.GetGencc()})
	case "bindingdb":
		out, _, err = query.Program.Eval(map[string]interface{}{"bindingdb": targetXref.GetBindingdb()})
	case "ctd":
		out, _, err = query.Program.Eval(map[string]interface{}{"ctd": targetXref.GetCtd()})
	case "biogrid":
		out, _, err = query.Program.Eval(map[string]interface{}{"biogrid": targetXref.GetBiogrid()})
	case "biogrid_interaction":
		out, _, err = query.Program.Eval(map[string]interface{}{"biogrid_interaction": targetXref.GetBiogridInteraction()})
	case "drugcentral":
		out, _, err = query.Program.Eval(map[string]interface{}{"drugcentral": targetXref.GetDrugcentral()})
	case "msigdb":
		out, _, err = query.Program.Eval(map[string]interface{}{"msigdb": targetXref.GetMsigdb()})
	case "alphamissense":
		out, _, err = query.Program.Eval(map[string]interface{}{"alphamissense": targetXref.GetAlphamissense()})
	case "alphamissense_transcript":
		out, _, err = query.Program.Eval(map[string]interface{}{"alphamissense_transcript": targetXref.GetAlphamissenseTranscript()})
	case "pharmgkb":
		out, _, err = query.Program.Eval(map[string]interface{}{"pharmgkb": targetXref.GetPharmgkb()})
	case "pharmgkb_gene":
		out, _, err = query.Program.Eval(map[string]interface{}{"pharmgkb_gene": targetXref.GetPharmgkbGene()})
	case "pharmgkb_clinical":
		out, _, err = query.Program.Eval(map[string]interface{}{"pharmgkb_clinical": targetXref.GetPharmgkbClinical()})
	case "pharmgkb_variant":
		out, _, err = query.Program.Eval(map[string]interface{}{"pharmgkb_variant": targetXref.GetPharmgkbVariant()})
	case "pharmgkb_guideline":
		out, _, err = query.Program.Eval(map[string]interface{}{"pharmgkb_guideline": targetXref.GetPharmgkbGuideline()})
	case "pharmgkb_pathway":
		out, _, err = query.Program.Eval(map[string]interface{}{"pharmgkb_pathway": targetXref.GetPharmgkbPathway()})
	case "cellxgene":
		out, _, err = query.Program.Eval(map[string]interface{}{"cellxgene": targetXref.GetCellxgene()})
	case "cellxgene_celltype":
		out, _, err = query.Program.Eval(map[string]interface{}{"cellxgene_celltype": targetXref.GetCellxgeneCelltype()})
	case "scxa":
		out, _, err = query.Program.Eval(map[string]interface{}{"scxa": targetXref.GetScxa()})
	case "scxa_expression":
		out, _, err = query.Program.Eval(map[string]interface{}{"scxa_expression": targetXref.GetScxaExpression()})
	case "scxa_gene_experiment":
		out, _, err = query.Program.Eval(map[string]interface{}{"scxa_gene_experiment": targetXref.GetScxaGeneExperiment()})
	case "alphafold":
		out, _, err = query.Program.Eval(map[string]interface{}{"alphafold": targetXref.GetAlphafold()})
	case "clinical_trials":
		out, _, err = query.Program.Eval(map[string]interface{}{"clinical_trials": targetXref.GetClinicalTrials()})
	case "collectri":
		out, _, err = query.Program.Eval(map[string]interface{}{"collectri": targetXref.GetCollectri()})
	case "brenda":
		out, _, err = query.Program.Eval(map[string]interface{}{"brenda": targetXref.GetBrenda()})
	case "brenda_kinetics":
		out, _, err = query.Program.Eval(map[string]interface{}{"brenda_kinetics": targetXref.GetBrendaKinetics()})
	case "brenda_inhibitor":
		out, _, err = query.Program.Eval(map[string]interface{}{"brenda_inhibitor": targetXref.GetBrendaInhibitor()})
	case "cellphonedb":
		out, _, err = query.Program.Eval(map[string]interface{}{"cellphonedb": targetXref.GetCellphonedb()})
	case "spliceai":
		out, _, err = query.Program.Eval(map[string]interface{}{"spliceai": targetXref.GetSpliceai()})
	case "mirdb":
		out, _, err = query.Program.Eval(map[string]interface{}{"mirdb": targetXref.GetMirdb()})
	case "fantom5_promoter":
		out, _, err = query.Program.Eval(map[string]interface{}{"fantom5_promoter": targetXref.GetFantom5Promoter()})
	case "fantom5_enhancer":
		out, _, err = query.Program.Eval(map[string]interface{}{"fantom5_enhancer": targetXref.GetFantom5Enhancer()})
	case "fantom5_gene":
		out, _, err = query.Program.Eval(map[string]interface{}{"fantom5_gene": targetXref.GetFantom5Gene()})
	case "jaspar":
		out, _, err = query.Program.Eval(map[string]interface{}{"jaspar": targetXref.GetJaspar()})
	case "encode_ccre":
		out, _, err = query.Program.Eval(map[string]interface{}{"encode_ccre": targetXref.GetEncodeCcre()})
	default:
		//err := fmt.Errorf("mapfilter query execution failed please check again query")
		return false, nil
	}

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
