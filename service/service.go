package service

import (
	"biobtree/query"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/allegro/bigcache"

	"biobtree/db"
	"biobtree/pbuf"
	"biobtree/util"

	"github.com/bmatsuo/lmdb-go/lmdb"
	"github.com/golang/protobuf/proto"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

const pagingSep2 = ","
const pagingSep3 = "[]"
const pagingSep4 = "]["

type service struct {
	readEnv                  *lmdb.Env
	readDbi                  lmdb.DBI
	aliasEnv                 *lmdb.Env
	aliasDbi                 lmdb.DBI
	pager                    *util.Pagekey
	pageSize                 int
	resultPageSize           int
	maxMappingResult         int
	mapFilterTimeoutDuration float64
	qparser                  *query.QueryParser
	celgoEnv                 cel.Env
	filterResultCache        *bigcache.BigCache
}

func (s *service) init() {

	meta := make(map[string]interface{})
	f, err := ioutil.ReadFile(filepath.FromSlash(config.Appconf["dbDir"] + "/db.meta.json"))
	if err != nil {
		log.Fatalln("Error while reading meta information file which should be produced with generate command. Please make sure you did previous steps correctly.")
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &meta); err != nil {
		panic(err)
	}

	totalkvline := meta["totalKVLine"].(float64)

	db1 := db.DB{}
	s.readEnv, s.readDbi = db1.OpenDB(false, int64(totalkvline), config.Appconf)
	s.pager = &util.Pagekey{}
	s.pager.Init()

	s.pageSize = 200
	if _, ok := config.Appconf["pageSize"]; ok {
		s.pageSize, err = strconv.Atoi(config.Appconf["pageSize"])
		if err != nil {
			panic("Invalid pagesize definition")
		}
	}

	s.resultPageSize = 10
	if _, ok := config.Appconf["maxSearchResult"]; ok {
		s.resultPageSize, err = strconv.Atoi(config.Appconf["maxSearchResult"])
		if err != nil {
			panic("Invalid maxSearchResult definition")
		}
	}

	s.maxMappingResult = 30
	if _, ok := config.Appconf["maxMappingResult"]; ok {
		s.maxMappingResult, err = strconv.Atoi(config.Appconf["maxMappingResult"])
		if err != nil {
			panic("Invalid maxMappingResult definition")
		}
	}

	s.mapFilterTimeoutDuration = 300
	if _, ok := config.Appconf["mapFilterTimeoutDuration"]; ok {
		timeoutInt, err := strconv.Atoi(config.Appconf["mapFilterTimeoutDuration"])
		if err != nil {
			panic("Invalid mapFilterTimeoutDuration definition")
		}
		s.mapFilterTimeoutDuration = float64(timeoutInt)
	}

	// init aliases

	meta2 := make(map[string]interface{})
	f, err = ioutil.ReadFile(config.Appconf["aliasDbDir"] + "/alias.meta.json")
	if err != nil {
		log.Fatalln("Error while reading meta information file which should be produced with generate command. Please make sure you did previous steps correctly.")
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &meta2); err != nil {
		panic(err)
	}

	aliasDataSize := int64(meta2["datasize"].(float64))

	db2 := db.DB{}

	s.aliasEnv, s.aliasDbi = db2.OpenAliasDB(false, aliasDataSize, config.Appconf)

	s.qparser = &query.QueryParser{}
	s.qparser.Init(config)

	// init cel-go
	s.celgoEnv, err = cel.NewEnv(
		cel.Types(&pbuf.UniprotAttr{}),
		cel.Types(&pbuf.UniprotFeatureAttr{}),
		cel.Types(&pbuf.EnsemblAttr{}),
		cel.Types(&pbuf.TaxoAttr{}),
		cel.Types(&pbuf.HgncAttr{}),
		cel.Types(&pbuf.OntologyAttr{}),
		cel.Types(&pbuf.InterproAttr{}),
		cel.Types(&pbuf.EnaAttr{}),
		cel.Types(&pbuf.HmdbAttr{}),
		cel.Types(&pbuf.PdbAttr{}),
		cel.Types(&pbuf.DrugbankAttr{}),
		cel.Types(&pbuf.OrphanetAttr{}),
		cel.Types(&pbuf.ReactomeAttr{}),
		cel.Declarations(
			decls.NewIdent("uniprot", decls.NewObjectType("pbuf.UniprotAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ufeature", decls.NewObjectType("pbuf.UniprotFeatureAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ensembl", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("transcript", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("exon", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("taxonomy", decls.NewObjectType("pbuf.TaxoAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("hgnc", decls.NewObjectType("pbuf.HgncAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("go", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("efo", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("eco", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("interpro", decls.NewObjectType("pbuf.InterproAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ena", decls.NewObjectType("pbuf.EnaAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("hmdb", decls.NewObjectType("pbuf.HmdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pdb", decls.NewObjectType("pbuf.PdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("drugbank", decls.NewObjectType("pbuf.DrugbankAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("orphanet", decls.NewObjectType("pbuf.OrphanetAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("reactome", decls.NewObjectType("pbuf.ReactomeAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("chembl", decls.NewObjectType("pbuf.ChemblAttr"), nil)),
		cel.Declarations(
			decls.NewFunction("overlaps",
				decls.NewInstanceOverload("any_greet_int_int",
					[]*exprpb.Type{decls.Any, decls.Int, decls.Int},
					decls.Bool)),
		),
	)

	if err != nil { // handle properly
		panic(err)
	}

	maxEntrySize := 25000
	if _, ok := config.Appconf["cacheMaxEntrySize"]; ok {
		maxEntrySize, err = strconv.Atoi(config.Appconf["cacheMaxEntrySize"])
		if err != nil {
			panic("Invalid mapFilterTimeoutDuration definition")
		}
	}

	cacheHardMaxSize := 1024
	if _, ok := config.Appconf["cacheHardMaxSize"]; ok {
		cacheHardMaxSize, err = strconv.Atoi(config.Appconf["cacheHardMaxSize"])
		if err != nil {
			panic("Invalid mapFilterTimeoutDuration definition")
		}
	}

	// init filter cache
	config := bigcache.Config{
		// number of shards (must be a power of 2)
		Shards: 1024,

		// time after which entry can be evicted
		//LifeWindow: 10 * time.Minute,
		LifeWindow: 0,
		// Interval between removing expired entries (clean up).
		// If set to <= 0 then no action is performed.
		// Setting to < 1 second is counterproductive — bigcache has a one second resolution.
		CleanWindow: 0,

		// rps * lifeWindow, used only in initial memory allocation
		MaxEntriesInWindow: 1000 * 10 * 60,

		// max entry size in bytes, used only in initial memory allocation
		MaxEntrySize: maxEntrySize,

		// prints information about additional memory allocation
		Verbose: true,

		// cache will not allocate more memory than this limit, value in MB
		// if value is reached then the oldest entries can be overridden for the new ones
		// 0 value means no size limit
		HardMaxCacheSize: cacheHardMaxSize,

		// callback fired when the oldest entry is removed because of its expiration time or no space left
		// for the new entry, or because delete was called. A bitmask representing the reason will be returned.
		// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
		OnRemove: nil,

		// OnRemoveWithReason is a callback fired when the oldest entry is removed because of its expiration time or no space left
		// for the new entry, or because delete was called. A constant representing the reason will be passed through.
		// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
		// Ignored if OnRemove is specified.
		OnRemoveWithReason: nil,
	}

	s.filterResultCache, err = bigcache.NewBigCache(config)
	if err != nil {
		log.Fatal(err)
	}

}

func (s *service) aliasIDs(alias string) ([]string, error) {

	var v []byte
	err := s.aliasEnv.View(func(txn *lmdb.Txn) (err error) {

		v, err = txn.Get(s.aliasDbi, []byte(alias))

		if lmdb.IsNotFound(err) {
			err := fmt.Errorf("undefined alias ->" + alias)
			return err
		}
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	r := pbuf.Alias{}

	if len(v) > 0 {
		err = proto.Unmarshal(v, &r)
		return r.Identifiers, err
	}

	err = fmt.Errorf("empty alias content ->" + alias)
	return nil, err

}

func (s *service) meta() *pbuf.MetaResponse {

	meta := pbuf.MetaResponse{}
	results := map[string]*pbuf.MetaKeyValue{}

	for k := range config.Dataconf {
		if config.Dataconf[k]["_alias"] == "" { // not send the alias
			id := config.Dataconf[k]["id"]
			if _, ok := results[id]; !ok {
				datasetConf := map[string]string{}
				keyvals := &pbuf.MetaKeyValue{}

				if len(config.Dataconf[k]["name"]) > 0 {
					datasetConf["name"] = config.Dataconf[k]["name"]
				} else {
					datasetConf["name"] = k
				}

				if len(config.Dataconf[k]["linkdataset"]) > 0 {
					datasetConf["linkdataset"] = config.Dataconf[k]["linkdataset"]
				}

				if _, ok := config.Dataconf[k]["attrs"]; ok {
					datasetConf["attrs"] = config.Dataconf[k]["attrs"]
				}

				datasetConf["id"] = k

				keyvals.Keyvalues = datasetConf
				results[id] = keyvals
			}
		}
	}

	meta.Results = results
	return &meta

}

func (s *service) metajson() string {

	var b strings.Builder
	b.WriteString("{")
	keymap := map[string]bool{}
	for k := range config.Dataconf {
		if config.Dataconf[k]["_alias"] == "" { // not send the alias
			id := config.Dataconf[k]["id"]
			if _, ok := keymap[id]; !ok {
				b.WriteString(`"` + id + `":{`)

				if len(config.Dataconf[k]["name"]) > 0 {
					b.WriteString(`"name":"` + config.Dataconf[k]["name"] + `",`)
				} else {
					b.WriteString(`"name":"` + k + `",`)
				}

				if len(config.Dataconf[k]["linkdataset"]) > 0 {
					b.WriteString(`"linkdataset":"` + config.Dataconf[k]["linkdataset"] + `",`)
				}

				if _, ok := config.Dataconf[k]["attrs"]; ok {
					b.WriteString(`"attrs":"` + config.Dataconf[k]["attrs"] + `",`)
				}

				b.WriteString(`"id":"` + k + `"`)

				b.WriteString(`},`)

				keymap[id] = true
			}
		}
	}
	s2 := b.String()
	s2 = s2[:len(s2)-1]
	s2 = s2 + "}"
	return s2

}

func (s *service) filter(id string, src uint32, filters []uint32, pageInd int) (*pbuf.Result, error) {

	var filtered []*pbuf.XrefEntry

	id = strings.ToUpper(id)
	//first we get the rootResult
	rootRes, err := s.getLmdbResult2(id, src)
	if err != nil {
		return nil, err
	}

	if pageInd == 0 {
		for _, f := range rootRes.Entries {
			for _, filter := range filters {
				if f.Dataset == filter {
					filtered = append(filtered, f)
				}
			}
		}

		if len(filtered) >= s.pageSize { //return here
			//todo this is duplicate code
			var filteredRes = pbuf.Result{}
			//filteredRes.Identifier = "1"
			var xrefs = make([]*pbuf.Xref, 1)
			var xref = pbuf.Xref{}
			xref.Dataset = src
			xref.DatasetCounts = rootRes.DatasetCounts
			xref.Entries = filtered
			xrefs[0] = &xref
			filteredRes.Results = xrefs

			return &filteredRes, nil

		}
	}

	// now we will go throught pages that includes filtered datasets.
	targetPages := map[string]bool{}
	for _, f := range filters {
		if _, ok := rootRes.GetDatasetPages()[f]; ok {
			for _, k := range rootRes.GetDatasetPages()[f].Pages {
				targetPages[k] = true
			}
		}
	}
	targetPagesArr := make([]string, len(targetPages))

	i := 0
	for k := range targetPages {
		targetPagesArr[i] = k
		i++
	}
	sort.Strings(targetPagesArr)

	//keyLen := s.pager.KeyLen(int(rootRes.Count / uint32(s.pageSize)))
	domainKey := config.DataconfIDToPageKey[src]

	err = s.readEnv.View(func(txn *lmdb.Txn) (err error) {

		var target *pbuf.Xref
		for _, page := range targetPagesArr[pageInd:] {

			var r1 = pbuf.Result{}

			//k, v, err := cur.Get([]byte(id), nil, lmdb.Next)
			pageKey := id + spacestr + domainKey + spacestr + page
			v, err := txn.Get(s.readDbi, []byte(pageKey))

			if lmdb.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			err = proto.Unmarshal(v, &r1)
			if err != nil {
				return err
			}

			target = nil
			for _, e := range r1.Results {
				if e.Dataset == src {
					target = e
					break
				}
			}

			if target != nil {
				for _, f := range target.Entries {
					for _, filter := range filters {
						if f.Dataset == filter {
							filtered = append(filtered, f)
						}
					}
				}
			}

			pageInd++

			if len(filtered) >= s.pageSize {
				return nil
			}

		}
		return nil // todo think
	})

	if err != nil {
		return nil, err
	}

	var filteredRes = pbuf.Result{}
	var xrefs = make([]*pbuf.Xref, 1)
	var xref = pbuf.Xref{}
	xref.Dataset = src
	xref.DatasetCounts = rootRes.DatasetCounts
	xref.Entries = filtered
	xref.Identifier = strconv.Itoa(pageInd)
	xrefs[0] = &xref
	filteredRes.Results = xrefs

	return &filteredRes, nil

}

func (s *service) page(id string, src int, page int, t int) (*pbuf.Result, error) {

	keyLen := s.pager.KeyLen(t)
	pk := s.pager.Key(page, keyLen)
	srckey := s.pager.Key(src, 2)
	var key strings.Builder
	key.WriteString(strings.ToUpper(id))
	key.WriteString(spacestr)
	key.WriteString(srckey)
	key.WriteString(spacestr)
	key.WriteString(pk)

	result, err := s.getLmdbResult(key.String())
	if err != nil {
		return nil, err
	}

	for _, xref := range result.Results {
		xref.Identifier = id
	}
	//todo what if nil
	return result, nil

}

type searchPageInfo struct {
	idIndex              int
	resultIndex          int
	resultIndexProcessed bool
	linkPageIndex        int
	linkIndex            int
	linkActive           bool
	linkIndexProcessed   bool
}

func (s *service) searchPageInfo(page string) (*searchPageInfo, error) {

	pageVals := strings.Split(page, pagingSep2)

	switch len(pageVals) {

	case 2:
		pagingInfo := &searchPageInfo{}
		idIndex, err := strconv.Atoi(pageVals[0])
		if err != nil {
			return nil, err
		}
		pagingInfo.idIndex = idIndex
		resultIndex, err := strconv.Atoi(pageVals[1])
		if err != nil {
			return nil, err
		}
		pagingInfo.resultIndex = resultIndex
		return pagingInfo, nil
	case 4:

		pagingInfo := &searchPageInfo{}
		idIndex, err := strconv.Atoi(pageVals[0])
		if err != nil {
			return nil, err
		}
		pagingInfo.idIndex = idIndex
		resultIndex, err := strconv.Atoi(pageVals[1])
		if err != nil {
			return nil, err
		}
		pagingInfo.resultIndex = resultIndex

		linkPageIndex, err := strconv.Atoi(pageVals[2])
		if err != nil {
			return nil, err
		}
		pagingInfo.linkPageIndex = linkPageIndex

		linkIndex, err := strconv.Atoi(pageVals[3])
		if err != nil {
			return nil, err
		}
		pagingInfo.linkIndex = linkIndex
		pagingInfo.linkActive = true
		return pagingInfo, nil
	default:
		err := fmt.Errorf("Invalid paging request", page)
		return nil, err

	}

}

func (s *service) setURL(xref *pbuf.Xref) {

	if xref.Dataset == 72 { // ufeature
		xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["url"], "£{id}", xref.Identifier[:strings.Index(xref.Identifier, "_")], -1)

	} else if xref.Dataset == 73 { // variantid

		xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["url"], "£{id}", strings.ToLower(xref.Identifier), -1)

	} else if xref.Dataset == 2 || xref.Dataset == 42 || xref.Dataset == 39 { // ensembl,transcript exon

		if xref.GetEmpty() { // data not indexed
			xref.Url = "#"
		} else {
			switch xref.GetEnsembl().Branch {
			case 1:
				xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["url"], "£{id}", xref.Identifier, -1)
				break
			case 2:
				xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["bacteriaUrl"], "£{id}", xref.Identifier, -1)
				break
			case 3:
				xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["fungiUrl"], "£{id}", xref.Identifier, -1)
				break
			case 4:
				xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["metazoaUrl"], "£{id}", xref.Identifier, -1)
				break
			case 5:
				xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["plantsUrl"], "£{id}", xref.Identifier, -1)
				break
			case 6:
				xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["protistsUrl"], "£{id}", xref.Identifier, -1)
				break
			default:
				xref.Url = "#"
				break
			}
			xref.Url = strings.Replace(xref.Url, "£{sp}", xref.GetEnsembl().Genome, -1)
		}

	} else {
		xref.Url = strings.Replace(config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["url"], "£{id}", xref.Identifier, -1)
	}

}

func (s *service) makeLite(xref *pbuf.Xref) {
	xref.Entries = nil
	xref.DatasetPages = nil
	xref.Pages = nil
	xref.DatasetCounts = nil
}

func (s *service) makeLiteAll(res *pbuf.Result) {

	for _, xref := range res.Results {
		s.makeLite(xref)
	}

}

func (s *service) setAllURL(res *pbuf.Result) {

	for _, xref := range res.Results {
		s.setURL(xref)
	}

}

func (s *service) search(ids []string, idsDomain uint32, page string, q *query.Query, detail, buildURL bool) (*pbuf.Result, error) {

	//todo remove duplicate parts
	result := &pbuf.Result{}

	if !detail {
		defer s.makeLiteAll(result)
	}

	if buildURL {
		defer s.setAllURL(result)
	}

	var xrefs []*pbuf.Xref
	totalResult := 0
	var err error

	for i := range ids {
		ids[i] = strings.ToUpper(ids[i])
	}

	var pagingInfo *searchPageInfo

	if page != "" {
		pagingInfo, err = s.searchPageInfo(page)
		if err != nil {
			return nil, err
		}
	}

	for idIndex, id := range ids {

		if pagingInfo != nil && pagingInfo.idIndex > idIndex {
			continue
		}

		result, err := s.getLmdbResult(id)
		if err != nil {
			return nil, err
		}

		if pagingInfo != nil && !pagingInfo.resultIndexProcessed { // starts from last process id results
			result.Results = result.Results[pagingInfo.resultIndex:] //todo slice bound out of range.
			pagingInfo.resultIndexProcessed = true
		}

		resultIndex := 0
		if len(result.Results) > 0 {

			for _, xref := range result.Results {

				if xref.IsLink {

					skipRootLinks := false
					if pagingInfo != nil && pagingInfo.linkActive && !pagingInfo.linkIndexProcessed { // first paging check for root result
						if pagingInfo.linkPageIndex == -1 {
							xref.Entries = xref.Entries[pagingInfo.linkIndex:]
							pagingInfo.linkIndexProcessed = true
						} else {
							skipRootLinks = true
						}
					}

					if !skipRootLinks {
						linkIndex := 0
						for _, b := range xref.Entries { //link entries

							xref2, err := s.getLmdbResult2(b.Identifier, b.Dataset)

							if err != nil {
								return nil, err
							}

							if idsDomain > 0 && xref2.Dataset != idsDomain {
								continue
							}

							if totalResult == s.resultPageSize {

								if pagingInfo != nil {

									if idIndex == pagingInfo.idIndex {
										resultIndex = pagingInfo.resultIndex + resultIndex
										linkIndex += pagingInfo.linkIndex
									}

								}

								result.Nextpage = strconv.Itoa(idIndex) + pagingSep2 + strconv.Itoa(resultIndex) + pagingSep2 + "-1" + pagingSep2 + strconv.Itoa(linkIndex)
								result.Results = xrefs
								return result, nil
							}

							xref2.Keyword = id
							xref2.Identifier = b.Identifier
							if q != nil { // filter xref. It is repetitive can be moved to 1 place
								q.MapDataset = config.DataconfIDIntToString[xref2.Dataset]
								q.MapDatasetID = xref2.Dataset
								if len(q.Filter) > 0 {
									b, err := s.execCelGo(q, xref2)
									if err != nil {
										return nil, err
									}
									if b {
										if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
											xrefs = append(xrefs, xref2)
											totalResult++
										}
									}
								}
							} else {
								if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
									xrefs = append(xrefs, xref2)
									totalResult++
								}
							}
							linkIndex++
						}
					}
					if len(xref.Pages) > 0 { // link pages

						if pagingInfo != nil && pagingInfo.linkActive && !pagingInfo.linkIndexProcessed {
							if pagingInfo.linkPageIndex > -1 {
								xref.Pages = xref.Pages[pagingInfo.linkPageIndex:]
							}
						}

						for pageIndex, page := range xref.Pages {
							pageKey := id + spacestr + config.DataconfIDToPageKey[0] + spacestr + page
							xrefPage, err := s.getLmdbResult2(pageKey, xref.Dataset)
							if err != nil {
								return nil, err
							}

							if pagingInfo != nil && pagingInfo.linkActive && !pagingInfo.linkIndexProcessed {
								xrefPage.Entries = xrefPage.Entries[pagingInfo.linkIndex:]
								pagingInfo.linkIndexProcessed = true
							}

							linkIndex := 0
							for _, b := range xrefPage.Entries {

								xref2, err := s.getLmdbResult2(b.Identifier, b.Dataset)

								if err != nil {
									return nil, err
								}

								if idsDomain > 0 && xref2.Dataset != idsDomain {
									continue
								}

								if totalResult == s.resultPageSize {
									if pagingInfo != nil {

										if idIndex == pagingInfo.idIndex {
											resultIndex = pagingInfo.resultIndex + resultIndex

											if pagingInfo.linkPageIndex > -1 { // move pageIndex
												pageIndex += pagingInfo.linkPageIndex
											}
											if pageIndex == pagingInfo.linkPageIndex {
												linkIndex += pagingInfo.linkIndex
											}
										}

									}
									result.Nextpage = strconv.Itoa(idIndex) + pagingSep2 + strconv.Itoa(resultIndex) + pagingSep2 + strconv.Itoa(pageIndex) + pagingSep2 + strconv.Itoa(linkIndex)
									result.Results = xrefs
									return result, nil
								}

								xref2.Keyword = id
								xref2.Identifier = b.Identifier

								if q != nil {
									q.MapDataset = config.DataconfIDIntToString[xref2.Dataset]
									q.MapDatasetID = xref2.Dataset
									if len(q.Filter) > 0 {
										b, err := s.execCelGo(q, xref2)
										if err != nil {
											return nil, err
										}
										if b {

											if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
												xrefs = append(xrefs, xref2)
												totalResult++
											}

										}
									}
								} else {

									if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
										xrefs = append(xrefs, xref2)
										totalResult++
									}
								}
								linkIndex++
							}

						}
					}
				} else {

					if idsDomain > 0 && xref.Dataset != idsDomain {
						continue
					}

					if totalResult == s.resultPageSize {
						if pagingInfo != nil {
							if idIndex == pagingInfo.idIndex {
								resultIndex = pagingInfo.resultIndex + resultIndex
							}
						}
						result.Nextpage = strconv.Itoa(idIndex) + pagingSep2 + strconv.Itoa(resultIndex)
						result.Results = xrefs
						return result, nil
					}

					xref.Identifier = id

					if q != nil {
						q.MapDataset = config.DataconfIDIntToString[xref.Dataset]
						q.MapDatasetID = xref.Dataset
						if len(q.Filter) > 0 {
							b, err := s.execCelGo(q, xref)
							if err != nil {
								return nil, err
							}
							if b {
								xrefs = append(xrefs, xref)
								totalResult++
							}
						}
					} else {

						if _, ok := config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["linkdataset"]; !ok {
							xrefs = append(xrefs, xref)
							totalResult++
						}

					}
				}

				resultIndex++

			}

		}

	}

	result.Results = xrefs
	return result, nil

}

func (s *service) searchPage(ids []string, page string) (*pbuf.Result, error) {

	result := &pbuf.Result{}
	var xrefs []*pbuf.Xref
	totalResult := 0
	var err error

	pageVals := strings.Split(page, pagingSep2)
	idIndex, err := strconv.Atoi(pageVals[0])
	if err != nil {
		return nil, err
	}

	ids = ids[idIndex:]

	hasLinkEntryIndex := false
	linkEntryIndex := 0
	if len(pageVals) == 2 {
		linkEntryIndex, err = strconv.Atoi(pageVals[1])
		if err != nil {
			return nil, err
		}
		hasLinkEntryIndex = true
	}

	for idIndex, id := range ids {

		result, err := s.getLmdbResult(strings.ToUpper(id))
		if err != nil {
			return nil, err
		}

		if len(result.Results) > 0 {

			for _, xref := range result.Results {
				if xref.IsLink {
					if hasLinkEntryIndex { // first time from paging with link entry index
						xref.Entries = xref.Entries[linkEntryIndex:]
						hasLinkEntryIndex = false
					}
					for linkIndex, b := range xref.Entries {

						xref2, err := s.getLmdbResult2(b.Identifier, b.Dataset)
						if err != nil {
							return nil, err
						}

						if totalResult == s.resultPageSize {
							result.Nextpage = strconv.Itoa(idIndex) + pagingSep2 + strconv.Itoa(linkIndex)
							result.Results = xrefs
							return result, nil
						}

						xref2.Keyword = id
						xref2.Identifier = b.Identifier
						xrefs = append(xrefs, xref2)
						totalResult++
					}
				} else {

					if totalResult == s.resultPageSize {
						result.Nextpage = strconv.Itoa(idIndex)
						result.Results = xrefs
						return result, nil
					}

					xref.Identifier = id
					xrefs = append(xrefs, xref)
					totalResult++

				}
			}

		}

	}

	result.Results = xrefs
	return result, nil

}

func (s *service) getLmdbResult(identifier string) (*pbuf.Result, error) {

	var v []byte
	err := s.readEnv.View(func(txn *lmdb.Txn) (err error) {

		v, err = txn.Get(s.readDbi, []byte(identifier))

		if lmdb.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	r := pbuf.Result{}

	if len(v) > 0 {
		err = proto.Unmarshal(v, &r)
		return &r, err
	}

	return &r, nil

}

func (s *service) getLmdbResult2(identifier string, domainID uint32) (*pbuf.Xref, error) {

	var v []byte
	err := s.readEnv.View(func(txn *lmdb.Txn) (err error) {
		//cur, err := txn.OpenCursor(s.readDbi)
		//_, v, err := cur.Get([]byte(identifier), nil, lmdb.SetKey)
		v, err = txn.Get(s.readDbi, []byte(identifier))

		if lmdb.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	//todo handle empty response...like previous func
	r := pbuf.Result{}
	err = proto.Unmarshal(v, &r)
	if err != nil {
		return nil, err
	}
	// in result get target xref result
	var targetXref *pbuf.Xref
	for _, xref := range r.Results {
		if xref.Dataset == domainID {
			targetXref = xref
			targetXref.Identifier = identifier
			break
		}
	}

	if targetXref == nil {
		err := fmt.Errorf("Entry not found identifier %s dataset  %s. Make sure it is actual identifier not keyword", identifier, config.DataconfIDIntToString[domainID])
		return nil, err
	}

	return targetXref, nil

}
