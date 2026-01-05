package service

import (
	"biobtree/query"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/ristretto"

	"biobtree/db"
	"biobtree/pbuf"
	"biobtree/util"

	"github.com/golang/protobuf/proto"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	typescelgo "github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter/functions"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

const pagingSep2 = ","
const pagingSep3 = "[]"
const pagingSep4 = "]["

type service struct {
	readEnv                  db.Env
	readDbi                  db.DBI
	aliasStore               *AliasStore
	pager                    *util.Pagekey
	pageSize                 int
	resultPageSize           int
	maxMappingResult         int
	resultPageSizeLite       int // Higher limit for lite mode
	maxMappingResultLite     int // Higher limit for lite mode
	mapFilterTimeoutDuration float64
	qparser                  *query.QueryParser
	celgoEnv                 cel.Env
	filterResultCache        *ristretto.Cache
	cacheDisabled            bool // When true, skips cache for performance testing
	celProgOpts              cel.ProgramOption
}

func (s *service) init() {

	meta := make(map[string]interface{})
	f, err := ioutil.ReadFile(filepath.FromSlash(config.Appconf["dbDir"] + "/db.meta.json"))
	if err != nil {
		log.Fatalln("Error while reading meta information file which should be produced with generate command. Please make sure you did previous steps correctly.", err)
	}

	if err := json.Unmarshal(f, &meta); err != nil {
		log.Fatal(err)
	}

	totalkvline := meta["totalKVLine"].(float64)

	db1 := db.DB{}
	s.readEnv, s.readDbi = db1.OpenDBNew(false, int64(totalkvline), config.Appconf)
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

	// Lite mode defaults: 5x higher than full mode (smaller payload)
	s.resultPageSizeLite = 50
	if _, ok := config.Appconf["maxSearchResultLite"]; ok {
		s.resultPageSizeLite, err = strconv.Atoi(config.Appconf["maxSearchResultLite"])
		if err != nil {
			panic("Invalid maxSearchResultLite definition")
		}
	}

	s.maxMappingResultLite = 150
	if _, ok := config.Appconf["maxMappingResultLite"]; ok {
		s.maxMappingResultLite, err = strconv.Atoi(config.Appconf["maxMappingResultLite"])
		if err != nil {
			panic("Invalid maxMappingResultLite definition")
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

	// init aliases from JSON config
	s.aliasStore, err = NewAliasStore(config.Appconf["confDir"])
	if err != nil {
		log.Printf("Warning: Could not load aliases: %v", err)
		// Create empty store - aliases will be disabled but service continues
		s.aliasStore = &AliasStore{
			aliases: make(map[string]*AliasEntry),
			cache:   make(map[string][]string),
		}
	}

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
		cel.Types(&pbuf.ChebiAttr{}),
		cel.Types(&pbuf.PdbAttr{}),
		cel.Types(&pbuf.DrugbankAttr{}),
		cel.Types(&pbuf.OrphanetAttr{}),
		cel.Types(&pbuf.ReactomePathwayAttr{}),
		cel.Types(&pbuf.BgeeAttr{}),
		cel.Types(&pbuf.GwasAttr{}),
		cel.Types(&pbuf.AntibodyAttr{}),
		cel.Types(&pbuf.PubchemAttr{}),
		cel.Types(&pbuf.PubchemActivityAttr{}),
		cel.Types(&pbuf.PubchemAssayAttr{}),
		cel.Types(&pbuf.EntrezAttr{}),
		cel.Types(&pbuf.RefSeqAttr{}),
		cel.Types(&pbuf.GenccAttr{}),
		cel.Types(&pbuf.BindingdbAttr{}),
		cel.Types(&pbuf.CtdAttr{}),
		cel.Types(&pbuf.CtdGeneInteraction{}),
		cel.Types(&pbuf.CtdDiseaseAssociation{}),
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
			decls.NewIdent("cds", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
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
			decls.NewIdent("mondo", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("uberon", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("oba", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("hpo", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("cl", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pato", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("obi", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("xco", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("bao", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("interpro", decls.NewObjectType("pbuf.InterproAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ena", decls.NewObjectType("pbuf.EnaAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("hmdb", decls.NewObjectType("pbuf.HmdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("chebi", decls.NewObjectType("pbuf.ChebiAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pdb", decls.NewObjectType("pbuf.PdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("drugbank", decls.NewObjectType("pbuf.DrugbankAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("orphanet", decls.NewObjectType("pbuf.OrphanetAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("reactome", decls.NewObjectType("pbuf.ReactomePathwayAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("chembl", decls.NewObjectType("pbuf.ChemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("lipidmaps", decls.NewObjectType("pbuf.LipidmapsAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("swisslipids", decls.NewObjectType("pbuf.SwisslipidsAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("bgee", decls.NewObjectType("pbuf.BgeeAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("rhea", decls.NewObjectType("pbuf.RheaAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("gwas_study", decls.NewObjectType("pbuf.GwasStudyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("gwas", decls.NewObjectType("pbuf.GwasAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("dbsnp", decls.NewObjectType("pbuf.DbsnpAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("antibody", decls.NewObjectType("pbuf.AntibodyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pubchem", decls.NewObjectType("pbuf.PubchemAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pubchem_activity", decls.NewObjectType("pbuf.PubchemActivityAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pubchem_assay", decls.NewObjectType("pbuf.PubchemAssayAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("entrez", decls.NewObjectType("pbuf.EntrezAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("refseq", decls.NewObjectType("pbuf.RefSeqAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("gencc", decls.NewObjectType("pbuf.GenccAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("bindingdb", decls.NewObjectType("pbuf.BindingdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ctd", decls.NewObjectType("pbuf.CtdAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("biogrid", decls.NewObjectType("pbuf.BiogridAttr"), nil)),
		cel.Declarations(
			decls.NewFunction("overlaps",
				decls.NewOverload("overlaps_int_int",
					[]*exprpb.Type{decls.Int, decls.Int},
					decls.Bool),
				decls.NewInstanceOverload("overlaps_int_int",
					[]*exprpb.Type{decls.NewObjectType("pbuf.EnsemblAttr"), decls.Int, decls.Int},
					decls.Bool)),
			decls.NewFunction("within",
				decls.NewOverload("within_int_int",
					[]*exprpb.Type{decls.Int, decls.Int},
					decls.Bool),
				decls.NewInstanceOverload("within_int_int",
					[]*exprpb.Type{decls.NewObjectType("pbuf.EnsemblAttr"), decls.Int, decls.Int},
					decls.Bool)),
			decls.NewFunction("covers",
				decls.NewInstanceOverload("covers_int",
					[]*exprpb.Type{decls.NewObjectType("pbuf.EnsemblAttr"), decls.Int},
					decls.Bool)),
		),
	)

	if err != nil { // handle properly
		panic(err)
	}

	s.celProgOpts = cel.Functions(
		&functions.Overload{
			Operator: "overlaps",
			Function: func(args ...ref.Val) ref.Val {

				arg1 := int32(args[1].Value().(int64))
				arg2 := int32(args[2].Value().(int64))

				if arg1 > arg2 {
					return typescelgo.Bool(false)
				}

				a := args[0].Value().(*pbuf.EnsemblAttr)

				return typescelgo.Bool(a.Start <= arg1 && arg1 <= a.End) || (a.Start <= arg2 && arg2 <= a.End)

			}},
		&functions.Overload{
			Operator: "within",
			Function: func(args ...ref.Val) ref.Val {

				arg1 := int32(args[1].Value().(int64))
				arg2 := int32(args[2].Value().(int64))

				if arg1 > arg2 {
					return typescelgo.Bool(false)
				}

				ensembl := args[0].Value().(*pbuf.EnsemblAttr)

				return typescelgo.Bool(ensembl.Start >= arg1 && ensembl.End <= arg2)

			}},
		&functions.Overload{
			Operator: "covers",
			Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {

				arg1 := int32(rhs.Value().(int64))

				ensembl := lhs.Value().(*pbuf.EnsemblAttr)

				return typescelgo.Bool(ensembl.Start <= arg1 && arg1 <= ensembl.End)

			}},
	)

	// Check if cache is disabled for performance testing
	s.cacheDisabled = false
	if val, ok := config.Appconf["disableCache"]; ok {
		if val == "y" || val == "yes" || val == "true" || val == "1" {
			s.cacheDisabled = true
			log.Println("Cache disabled via disableCache parameter")
		}
	}

	// Only initialize cache if not disabled
	if !s.cacheDisabled {
		cacheHardMaxSize := 1024
		if _, ok := config.Appconf["cacheHardMaxSize"]; ok {
			cacheHardMaxSize, err = strconv.Atoi(config.Appconf["cacheHardMaxSize"])
			if err != nil {
				panic("Invalid cacheHardMaxSize definition")
			}
		}

		s.filterResultCache, err = ristretto.NewCache(&ristretto.Config{
			NumCounters: 1e7,                                   // number of keys to track frequency of (10M).
			MaxCost:     int64(cacheHardMaxSize) * 1024 * 1024, // maximum cost of cache.
			BufferItems: 64,                                    // number of keys per Get buffer.
		})

		if err != nil {
			panic(err)
		}
	}

}

func (s *service) aliasIDs(alias string) ([]string, error) {
	return s.aliasStore.GetIDs(alias)
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
	b.WriteString(`{ "datasets":{`)
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

	// additional params
	// mark builtin db if exist
	s2 = s2 + `, "appparams":{`

	files, err := ioutil.ReadDir(config.Appconf["indexDir"])
	if err != nil {
		log.Fatal(err)
	}

loop:
	for _, f := range files {
		if !f.IsDir() {
			switch f.Name() {
			case "builtinset1.meta.json":
				s2 = s2 + `"builtinset":"1"`
				break loop
			case "builtinset2.meta.json":
				s2 = s2 + `"builtinset":"2"`
				break loop
			case "builtinset3.meta.json":
				s2 = s2 + `"builtinset":"3"`
				break loop
			case "builtinset4.meta.json":
				s2 = s2 + `"builtinset":"4"`
				break loop
			case "def.meta.json": // this used when indexing all
				s2 = s2 + `"builtinset":"0"`
				break loop
			}
		}
	}

	s2 = s2 + `}}`
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

	err = s.readEnv.View(func(txn db.Txn) (err error) {

		var target *pbuf.Xref
		for _, page := range targetPagesArr[pageInd:] {

			var r1 = pbuf.Result{}

			//k, v, err := cur.Get([]byte(id), nil, lmdb.Next)
			pageKey := id + spacestr + domainKey + spacestr + page
			v, err := txn.Get(s.readDbi, []byte(pageKey))

			if db.IsNotFound(err) {
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


func (s *service) search(ids []string, idsDomain uint32, page string, q *query.Query, detail, buildURL bool) (*pbuf.Result, error) {

	//todo remove duplicate parts
	result := &pbuf.Result{}

	if !detail {
		defer s.makeLiteAll(result)
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

							// Filter by dataset BEFORE fetching to avoid errors from missing entries in other datasets
							if idsDomain > 0 && b.Dataset != idsDomain {
								continue
							}

							xref2, err := s.getLmdbResult2(b.Identifier, b.Dataset)

							if err != nil {
								return nil, err
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

								// Filter by dataset BEFORE fetching to avoid errors from missing entries in other datasets
								if idsDomain > 0 && b.Dataset != idsDomain {
									continue
								}

								xref2, err := s.getLmdbResult2(b.Identifier, b.Dataset)

								if err != nil {
									return nil, err
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

	// Add informational message when no results found
	if len(xrefs) == 0 {
		result.Message = "No results found"
	}

	// Enrich with all transient fields (dataset_name, url)
	EnrichResult(result)

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

	// Enrich with all transient fields (dataset_name, url)
	EnrichResult(result)

	return result, nil

}

func (s *service) getLmdbResult(identifier string) (*pbuf.Result, error) {
	// TODO: Consider adding caching here for frequently accessed identifiers
	// The update layer now uses ristretto cache (see update.go:lookup()) which provides
	// significant performance benefits for repeated lookups. Similar caching could be
	// beneficial for the web service layer if query patterns show high repetition.
	// However, web queries have different access patterns (diverse, random) vs update
	// operations (same identifiers repeated millions of times), so cache tuning would differ.

	var v []byte
	err := s.readEnv.View(func(txn db.Txn) (err error) {

		v, err = txn.Get(s.readDbi, []byte(identifier))

		if db.IsNotFound(err) {
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
	err := s.readEnv.View(func(txn db.Txn) (err error) {
		//cur, err := txn.OpenCursor(s.readDbi)
		//_, v, err := cur.Get([]byte(identifier), nil, lmdb.SetKey)
		v, err = txn.Get(s.readDbi, []byte(identifier))

		if db.IsNotFound(err) {
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

	// Enrich with all transient fields (dataset_name, url)
	EnrichXref(targetXref)

	return targetXref, nil

}

// searchLite performs a search and returns compact lite format response
// Uses the main search function and converts the result to lite format
// Returns only IDs, sorted by has_attr (entries with attributes first)
func (s *service) searchLite(ids []string, idsDomain uint32, page string, datasetFilter string) (*pbuf.ResultLite, error) {
	// Use the main search function - this ensures consistent behavior
	// detail=false triggers makeLiteAll which strips attributes
	// buildURL=false since we don't need URLs in lite mode
	fullResult, err := s.search(ids, idsDomain, page, nil, false, false)
	if err != nil {
		return nil, err
	}

	// Convert full result to lite format
	result := &pbuf.ResultLite{
		Mode: "lite",
		Query: &pbuf.SearchQueryInfo{
			Terms:         ids,
			DatasetFilter: datasetFilter,
			Raw:           strings.Join(ids, ","),
		},
	}

	var liteResults []*pbuf.SearchResultLite
	statsByDataset := make(map[string]int32)

	// Track seen IDs to avoid duplicates
	seenIDs := make(map[string]bool)

	for _, xref := range fullResult.Results {
		datasetName := config.DataconfIDIntToString[xref.Dataset]
		identifier := xref.Identifier
		if identifier == "" {
			identifier = xref.Keyword
		}

		// Create unique key to avoid duplicates
		uniqueKey := identifier + ":" + datasetName
		if seenIDs[uniqueKey] {
			continue
		}
		seenIDs[uniqueKey] = true

		hasAttr := !xref.GetEmpty()

		liteResult := &pbuf.SearchResultLite{
			D:         datasetName,
			Id:        identifier,
			HasAttr:   hasAttr,
			XrefCount: xref.Count,
		}
		liteResults = append(liteResults, liteResult)
		statsByDataset[datasetName]++
	}

	// Sort: entries with attributes first
	sort.Slice(liteResults, func(i, j int) bool {
		if liteResults[i].HasAttr != liteResults[j].HasAttr {
			return liteResults[i].HasAttr // true (has attr) comes first
		}
		return false // stable sort for same has_attr value
	})

	// Apply lite mode pagination limit (5x higher than full mode)
	totalResults := int32(len(liteResults))
	hasNext := len(liteResults) > s.resultPageSizeLite
	if hasNext {
		liteResults = liteResults[:s.resultPageSizeLite]
	}

	result.Results = liteResults
	result.Stats = &pbuf.SearchStats{
		TotalResults: totalResults,
		Returned:     int32(len(liteResults)),
		ByDataset:    statsByDataset,
	}
	result.Pagination = &pbuf.PaginationInfo{
		Page:    1,
		HasNext: hasNext,
	}

	// Copy nextpage token if available
	if fullResult.Nextpage != "" {
		result.Pagination.NextToken = fullResult.Nextpage
		result.Pagination.HasNext = true
	}

	return result, nil
}
