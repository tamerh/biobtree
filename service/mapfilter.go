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
func (s *service) mapFilter(ids []string, idsDomain uint32, mapFilterQuery, page string) (*pbuf.MapFilterResult, error) {

	startTime := time.Now()

	result := pbuf.MapFilterResult{}

	cacheKey := s.mapFilterCacheKey(ids, idsDomain, mapFilterQuery, page)

	if resultFromCache, err := s.filterResultCache.Get(cacheKey); err == nil {

		err := proto.Unmarshal(resultFromCache, &result)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
		//fmt.Println("Coming from cacheeee")
		return &result, nil

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

	var filterQ *query.Query

	if queries[0].MapDatasetID <= 0 { // if starts with filter
		filterQ = &queries[0]
		queries = queries[1:]
	}

startMapping:
	inputXrefs, newRootPage, err := s.inputXrefs(ids, idsDomain, filterQ, newRootPage, pages)

	if err != nil {
		return nil, err
	}

	if len(inputXrefs) == 0 {
		err := fmt.Errorf("No mapping found")
		return nil, err
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
				finaltargets, newpages, err = s.xrefMapping(queries, xref, nil)
				if err != nil { // todo maybe in this case it should continue?
					return nil, err
				}
			} else {
				if _, ok := pages[xref.Identifier]; ok {
					if _, ok := pages[xref.Identifier][xref.DomainId]; ok {
						finaltargets, newpages, err = s.xrefMapping(queries, xref, pages[xref.Identifier][xref.DomainId])
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
		xref.Entries = nil
		xref.DomainCounts = nil
		xref.Count = 0
		xref.DomainPages = nil
		xref.Pages = nil
		mapfil.Source = xref
		for _, tar := range finaltargets {
			tar.Entries = nil
			tar.DomainCounts = nil
			tar.Count = 0
			tar.DomainPages = nil
			tar.Pages = nil
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

	// set cache
	setCache := func() error {
		resultBytes, err := proto.Marshal(&result) // saving as json also can be option
		if err != nil {
			err := fmt.Errorf("Error while setting result to cache")
			return err
		}
		//fmt.Println("cache size->", strconv.Itoa(len(resultBytes)))
		err = s.filterResultCache.Set(cacheKey, resultBytes)
		return err
	}

	// return result
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

func (s *service) mapFilterCacheKey(ids []string, idsDomain uint32, mapFilterQuery, page string) string {

	var str strings.Builder
	for _, id := range ids {
		str.WriteString(id)
		str.WriteString(",")
	}
	if idsDomain > 0 {
		str.WriteString(strconv.Itoa(int(idsDomain)))
		str.WriteString(",")
	}
	str.WriteString(mapFilterQuery)
	if len(page) > 0 {
		str.WriteString(",")
		str.WriteString(page)
	}
	return str.String()

}

func (s *service) inputXrefs(ids []string, idsDomain uint32, filterq *query.Query, rootPage string, pages map[string]map[uint32]map[int]*mpPage) ([]*pbuf.Xref, string, error) {

	var inputXrefs []*pbuf.Xref

	if pages == nil {

		res, err := s.search(ids, idsDomain, rootPage, filterq)

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

			xref, err := s.getLmdbResult2(strings.ToUpper(k), k2)
			if err != nil {
				return nil, "", err
			}
			inputXrefs = append(inputXrefs, xref)
		}

	}

	return inputXrefs, rootPage, nil

}

func (s *service) parsePagingKey(key string) (string, map[string]map[uint32]map[int]*mpPage, error) {

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

func (s *service) setResultPaging(source *pbuf.Xref, pages map[int]*mpPage) string {

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
	b.WriteString(strconv.Itoa(int(source.DomainId)))
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
func (s *service) prepareQueries(mapFilterQuery string) ([]query.Query, error) {

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

func (s *service) xrefMapping(queries []query.Query, xref *pbuf.Xref, inPages map[int]*mpPage) ([]*pbuf.Xref, map[int]*mpPage, error) {

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
				mapDatasetID = xref.DomainId
			} else {
				mapDatasetID = queries[i-1].MapDatasetID
			}
			source, err := s.getLmdbResult2(inPages[i].sourceID, mapDatasetID)

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

		q := queries[qind]

		if sourceEntries[qind] == nil {
			sourceEntries[qind], err = s.getEntries(sources[qind], q.MapDatasetID, inPages[qind])
			if err != nil {
				return nil, nil, err
			}
		}
		if qind == len(queries)-1 {
		searchTargets:
			for _, entry := range sourceEntries[qind] {

				if len(targets) >= s.maxMappingResult {
					inPages[qind].sourceID = sources[qind].Identifier
					goto finish
				}

				inPages[qind].entryIndex++

				if entry.DomainId == q.MapDatasetID {

					filterRes, target, err := s.applyFilter(entry, &q)
					if err != nil {
						return nil, nil, err
					}

					if filterRes {
						if _, ok := targetkeys[config.DataconfIDIntToString[target.DomainId]+"_"+target.Identifier]; !ok {
							targets = append(targets, target)
							targetkeys[config.DataconfIDIntToString[target.DomainId]+"_"+target.Identifier] = true
						}
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
				if entry.DomainId == q.MapDatasetID {

					filterRes, nextsource, err := s.applyFilter(entry, &q)

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

func (s *service) getEntries(xref *pbuf.Xref, mapDatasetID uint32, mpage *mpPage) ([]*pbuf.XrefEntry, error) {

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

		page := xref.DomainPages[mapDatasetID].Pages[mpage.page]
		pageKey := xref.Identifier + spacestr + config.DataconfIDToPageKey[xref.DomainId] + spacestr + page
		source, err := s.getLmdbResult2(pageKey, xref.DomainId)
		if err != nil {
			return nil, err
		}
		if mpage.entryIndex == 0 {
			return source.Entries, nil
		}

		return xref.Entries[mpage.entryIndex:], nil

	}

	return entries, nil

}

func (s *service) moveNextPage(entryMap map[int][]*pbuf.XrefEntry, source *pbuf.Xref, inPages map[int]*mpPage, index int, MapDatasetID uint32) (bool, error) {

	var err error
	if _, ok := source.DomainPages[MapDatasetID]; ok && inPages[index].page+1 < len(source.DomainPages[MapDatasetID].Pages) {
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

func (s *service) moveEntries(sourceEntries map[int][]*pbuf.XrefEntry, source *pbuf.Xref, inPages map[int]*mpPage, qind int, MapDatasetID uint32, entryIndex int) {

	if entryIndex+1 < len(sourceEntries[qind]) {
		sourceEntries[qind] = sourceEntries[qind][entryIndex+1:]
		inPages[qind].entryIndex = inPages[qind].entryIndex + 1
	} else {
		if _, ok := source.DomainPages[MapDatasetID]; ok && inPages[qind].page+1 < len(source.DomainPages[MapDatasetID].Pages) {
			inPages[qind].page = inPages[qind].page + 1
			inPages[qind].entryIndex = 0
			sourceEntries[qind] = nil // because maybe not needed to fill
		} else {
			inPages[qind].page = -2 // no more page
		}
	}

}

func (s *service) applyFilter(entry *pbuf.XrefEntry, q *query.Query) (bool, *pbuf.Xref, error) {

	target, err := s.getLmdbResult2(entry.XrefId, entry.DomainId)
	if err != nil {
		return false, nil, err
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

func (s *service) execCelGo(query *query.Query, targetXref *pbuf.Xref) (bool, error) {

	if targetXref.GetEmpty() {
		//err := fmt.Errorf("Filtered entry has not indexed for filtering->" + targetXref.Identifier)
		// think again this
		return false, nil
	}

	// look in cache f_ is just differentiate with mapfilter can be better...
	cacheKey := "f_" + targetXref.Identifier + "_" + strconv.Itoa(int(targetXref.DomainId)) + query.Filter
	if entry, err := s.filterResultCache.Get(cacheKey); err == nil {
		if entry[0] == '1' {
			return true, nil
		} else if entry[0] == '0' {
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

		prg, err := s.celgoEnv.Program(checked)

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
	case "taxonomy":
		out, _, err = query.Program.Eval(map[string]interface{}{"taxonomy": targetXref.GetTaxonomy()})
	case "hgnc":
		out, _, err = query.Program.Eval(map[string]interface{}{"hgnc": targetXref.GetHgnc()})
	case "go":
		out, _, err = query.Program.Eval(map[string]interface{}{"go": targetXref.GetGontology()})
	case "chembl_document", "chembl_assay", "chembl_activity", "chembl_molecule", "chembl_target", "chembl_target_component", "chembl_cell_line":
		out, _, err = query.Program.Eval(map[string]interface{}{"chembl": targetXref.GetChembl()})
	case "interpro":
		out, _, err = query.Program.Eval(map[string]interface{}{"interpro": targetXref.GetInterpro()})
	case "ena":
		out, _, err = query.Program.Eval(map[string]interface{}{"ena": targetXref.GetEna()})
	case "hmdb":
		out, _, err = query.Program.Eval(map[string]interface{}{"hmdb": targetXref.GetHmdb()})
	case "pdb":
		out, _, err = query.Program.Eval(map[string]interface{}{"pdb": targetXref.GetPdb()})
	case "drugbank":
		out, _, err = query.Program.Eval(map[string]interface{}{"drugbank": targetXref.GetDrugbank()})
	case "orphanet":
		out, _, err = query.Program.Eval(map[string]interface{}{"orphanet": targetXref.GetOrphanet()})
	case "reactome":
		out, _, err = query.Program.Eval(map[string]interface{}{"reactome": targetXref.GetReactome()})
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
			s.filterResultCache.Set(cacheKey, []byte{'0'})
			return false, nil
		}

		if res { // todo think again conversion
			s.filterResultCache.Set(cacheKey, []byte{'1'})
			return true, nil
		}

	}

	s.filterResultCache.Set(cacheKey, []byte{'0'})
	return false, nil

}
