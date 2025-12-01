package update

import (
	"biobtree/configs"
	"biobtree/db"
	"biobtree/pbuf"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"biobtree/util"

	"github.com/dgraph-io/ristretto"
	"github.com/golang/protobuf/proto"
	"github.com/vbauerster/mpb"
)

const textLinkID = "0"
const textStoreID = "-1"

var fileBufSize = 65536
var channelOverflowCap = 100000

var chunkLen int
var idChunkLen int
var chunkIdx = "df"

var mutex = &sync.Mutex{}
var config *configs.Conf

var allEnsembls = []string{"ensembl", "ensembl_fungi", "ensembl_bacteria", "ensembl_metazoa", "ensembl_plants", "ensembl_protists"}

type DataUpdate struct {
	totalParsedEntry       uint64
	wg                     *sync.WaitGroup
	inDatasets             map[string]bool
	datasets2              []string // after resolving the input datasets
	start                  time.Time
	kvdatachan             *chan string
	mergeGateCh            *chan mergeInfo // Channel for sending files to merge pipeline
	invalidXrefs           util.HashMaper
	sampleXrefs            util.HashMaper
	sampleCount            int
	sampleWritten          bool
	uniprotFtp             string
	uniprotFtpPath         string
	ebiFtp                 string
	ebiFtpPath             string
	uniprotEntryCounts     map[string]uint64
	p                      *mpb.Progress
	stats                  map[string]interface{}
	targetDatasets         map[string]bool
	hasTargets             bool
	channelOverflowCap     int
	selectedGenomes        []string
	selectedGenomesPattern []string
	selectedTaxids         []int
	orthologsIDs           map[int]bool
	orthologsActive        bool
	orthologsAllActive     bool
	skipEnsembl            bool
	progChan               chan *progressInfo
	progInterval           int64
	lookupEnv              db.Env
	lookupDbi              db.DBI
	hasLookupDB            bool
	lookupCache            *ristretto.Cache // Cache for lookup() to avoid repeated LMDB transactions
	bucketPool             *HybridWriterPool // Bucket writer pool for optimized datasets
	bucketWg               *sync.WaitGroup   // WaitGroup for bucket writers
}

type progressInfo struct {
	dataset         string
	currentKBPerSec int64
	done            bool
	waiting         bool
}

func NewDataUpdate(datasets map[string]bool, targetDatasets, ensemblSpecies, ensemblSpeciesPattern []string, genometaxids []int, skipEnsembl bool, orthologIDs map[int]bool, orthologs, orthologsAll bool, conf *configs.Conf, chkIdx string) *DataUpdate {

	chunkIdx = chkIdx
	config = conf

	targetDatasetMap := map[string]bool{}

	if len(targetDatasets) > 0 {

		for _, dt := range targetDatasets {
			if _, ok := config.Dataconf[dt]; ok {
				targetDatasetMap[dt] = true
				//now aliases if exist
				if _, ok := config.Dataconf[dt]["aliases"]; ok {
					aliases := strings.Split(config.Dataconf[dt]["aliases"], ",")
					for _, ali := range aliases {
						targetDatasetMap[ali] = true
					}
				}

			}

		}
	}

	loc := config.Appconf["uniprot_ftp"]
	uniprotftpAddr := config.Appconf["uniprot_ftp_"+loc]
	uniprotftpPath := config.Appconf["uniprot_ftp_"+loc+"_path"]

	ebiftp := config.Appconf["ebi_ftp"]
	ebiftppath := config.Appconf["ebi_ftp_path"]

	// chunk buffer size
	var progInterval = 3
	if _, ok := config.Appconf["progressInterval"]; ok {
		var err error
		progInterval, err = strconv.Atoi(config.Appconf["progressInterval"])
		if err != nil {
			panic("Invalid progressInterval definition")
		}
	}

	if orthologsAll {
		orthologs = true
	}

	return &DataUpdate{
		invalidXrefs:           util.NewHashMap(300),
		sampleXrefs:            util.NewHashMap(400),
		uniprotFtp:             uniprotftpAddr,
		uniprotFtpPath:         uniprotftpPath,
		ebiFtp:                 ebiftp,
		ebiFtpPath:             ebiftppath,
		targetDatasets:         targetDatasetMap,
		hasTargets:             len(targetDatasetMap) > 0,
		channelOverflowCap:     channelOverflowCap,
		stats:                  make(map[string]interface{}),
		selectedGenomes:        ensemblSpecies,
		selectedGenomesPattern: ensemblSpeciesPattern,
		selectedTaxids:         genometaxids,
		progInterval:           int64(progInterval),
		progChan:               make(chan *progressInfo, 1000),
		start:                  time.Now(),
		inDatasets:             datasets,
		orthologsIDs:           orthologIDs,
		orthologsActive:        orthologs,
		orthologsAllActive:     orthologsAll,
		skipEnsembl:            skipEnsembl,
	}

}

// Initialize read-only lookup database for keyword-to-ID resolution
func (d *DataUpdate) initLookupDB() {
	lookupDbDir, ok := config.Appconf["lookupDbDir"]
	if !ok {
		d.hasLookupDB = false
		return
	}

	// Check if meta file exists
	metaFile := filepath.FromSlash(lookupDbDir + "/db.meta.json")
	meta := make(map[string]interface{})
	f, err := ioutil.ReadFile(metaFile)
	if err != nil {
		fmt.Printf("Warning: Cannot read lookup database meta file: %v, keyword lookup disabled\n", err)
		d.hasLookupDB = false
		return
	}

	if err := json.Unmarshal(f, &meta); err != nil {
		fmt.Printf("Warning: Cannot parse lookup database meta: %v, keyword lookup disabled\n", err)
		d.hasLookupDB = false
		return
	}

	totalkvline := int64(meta["totalKVLine"].(float64))

	// Open lookup database (read-only)
	db1 := db.DB{}
	lookupConf := make(map[string]string)
	lookupConf["dbDir"] = lookupDbDir
	lookupConf["dbBackend"] = "lmdb"
	d.lookupEnv, d.lookupDbi = db1.OpenDBNew(false, totalkvline, lookupConf)
	d.hasLookupDB = true

	// Initialize lookup cache to avoid repeated LMDB transactions
	// During update operations, the same gene names are looked up millions of times
	// Cache is optimized for high hit rate (e.g., same ~20K genes repeated across 600M+ SNPs)
	// DISABLED: Cache was causing memory growth due to storing full Result objects with nested Xrefs
	// TODO: Re-enable with fixed cost calculation or cache only gene existence (not full Results)
	d.lookupCache = nil
	/*
	var err2 error
	d.lookupCache, err2 = ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e6,      // 1M counters to track frequency
		MaxCost:     100 << 20, // 100MB max cache size
		BufferItems: 64,       // number of keys per Get buffer
	})
	if err2 != nil {
		log.Printf("Warning: Failed to initialize lookup cache: %v, will use uncached lookups\n", err2)
		d.lookupCache = nil
	}
	*/
}

// Close lookup database
func (d *DataUpdate) closeLookupDB() {
	if d.hasLookupDB {
		d.lookupEnv.Close()
	}
	if d.lookupCache != nil {
		d.lookupCache.Close()
	}
}

// Lookup identifier in biobtree database and return results
// Uses in-memory cache to avoid repeated LMDB transactions for the same identifier
func (d *DataUpdate) lookup(identifier string) (*pbuf.Result, error) {
	if !d.hasLookupDB {
		return nil, fmt.Errorf("lookup database not available")
	}

	// Lookup is case-insensitive (convert to uppercase like service does)
	identifier = strings.ToUpper(identifier)

	// Check cache first (if available)
	if d.lookupCache != nil {
		if cached, found := d.lookupCache.Get(identifier); found {
			// Cache hit - return cached result
			if cached == nil {
				return nil, nil // Cached "not found" result
			}
			return cached.(*pbuf.Result), nil
		}
	}

	// Cache miss - perform LMDB lookup
	var v []byte
	err := d.lookupEnv.View(func(txn db.Txn) (err error) {
		v, err = txn.Get(d.lookupDbi, []byte(identifier))
		if db.IsNotFound(err) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	// Handle not found
	if len(v) == 0 {
		// Store nil in cache to avoid repeated lookups for non-existent identifiers
		if d.lookupCache != nil {
			d.lookupCache.Set(identifier, nil, 1)
		}
		return nil, nil
	}

	// Unmarshal result
	r := pbuf.Result{}
	err = proto.Unmarshal(v, &r)
	if err != nil {
		return nil, err
	}

	// Store in cache (cost = approximate size)
	if d.lookupCache != nil {
		d.lookupCache.Set(identifier, &r, int64(len(v)))
	}

	return &r, nil
}

func (d *DataUpdate) Update() (uint64, uint64) {

	log.Println("Update running please wait...")

	// Initialize lookup database for keyword-based xref resolution
	d.initLookupDB()
	defer d.closeLookupDB()

	// first check update for ensembl meta
	checkEnsemblUpdate(d)

	// select ensembls
	ensembls := d.selectEnsembls()

	for _,ens := range allEnsembls { // remove from here because ensembl handled differently after selection
		if _, ok := d.inDatasets[ens]; ok {
			delete(d.inDatasets, ens)
		}
	}

	if len(ensembls) <= 0 && len(d.inDatasets) <= 0 {
		log.Println("No genome found for indexing")
		return 0, 0
	}

	var err error
	var wg sync.WaitGroup
	var e = make(chan string, channelOverflowCap)
	//var mergeGateCh = make(chan mergeInfo)
	var mergeGateCh = make(chan mergeInfo, 10000)

	d.wg = &wg
	d.kvdatachan = &e
	d.mergeGateCh = &mergeGateCh

	// Initialize bucket system for optimized datasets
	LoadBucketSystemConfig()
	bucketConfigs := LoadBucketConfigs()
	var bucketWg sync.WaitGroup
	d.bucketWg = &bucketWg
	d.bucketPool = NewHybridWriterPool(bucketConfigs, &e, config.Appconf["indexDir"], &bucketWg)
	if len(bucketConfigs) > 0 {
		log.Printf("Bucket system initialized with %d configured datasets", len(bucketConfigs))
	}

	// chunk buffer size
	chunkLen = 10000000
	if _, ok := config.Appconf["kvgenChunkSize"]; ok {
		var err error
		chunkLen, err = strconv.Atoi(config.Appconf["kvgenChunkSize"])
		if err != nil {
			panic("Invalid kvgenChunkSize definition")
		}
	}

	idChunkLen = 10000
	if _, ok := config.Appconf["idgenChunkSize"]; ok {
		var err error
		idChunkLen, err = strconv.Atoi(config.Appconf["idgenChunkSize"])
		if err != nil {
			panic("Invalid idgenChunkSize definition")
		}
	}

	// kv generator size
	kvGenCount := 1
	if _, ok := config.Appconf["kvgenCount"]; ok {
		kvGenCount, err = strconv.Atoi(config.Appconf["kvgenCount"])
		if err != nil {
			panic("Invalid kvgenCount definition")
		}
	}

	// create kvgens
	var kvgens []*kvgen
	for i := 0; i < kvGenCount; i++ {
		kv := newkvgen(strconv.Itoa(i))
		kv.dataChan = &e
		kv.mergeGateCh = &mergeGateCh
		kv.wg = &wg
		go kv.gen()
		kvgens = append(kvgens, &kv)
	}

	var wgBmerge sync.WaitGroup
	binarymerge := mergeb{
		wg:          &wgBmerge,
		mergeGateCh: &mergeGateCh,
	}
	wgBmerge.Add(1)
	go binarymerge.start()

	// first start ensembls
	if len(ensembls) > 0 && !d.skipEnsembl {
		for ens := range ensembls {
			d.datasets2 = append(d.datasets2, ens) // for the progress bar
			d.progChan <- &progressInfo{dataset: ens, waiting: true}
			d.wg.Add(1)
		}
		go d.updateEnsembls(ensembls)
	}

	for data := range d.inDatasets {
		switch data {
		case "uniprot":
			d.wg.Add(1)
			u := uniprot{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go u.update(d.selectedTaxids)
			break
		case "taxonomy":
			d.wg.Add(1)
			t := taxonomy{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go t.update()
			break
		case "hgnc":
			d.wg.Add(1)
			h := hgnc{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go h.update()
			break
		case "chebi":
			d.wg.Add(1)
			c := chebi{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go c.update()
			break
		case "interpro":
			d.wg.Add(1)
			i := interpro{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go i.update()
			break
		case "uniparc":
			d.wg.Add(1)
			u := uniparc{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go u.update()
			break
		case "uniref50":
			d.wg.Add(1)
			u := uniref{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go u.update()
			break
		case "uniref90":
			d.wg.Add(1)
			u := uniref{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go u.update()
			break
		case "uniref100":
			d.wg.Add(1)
			u := uniref{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go u.update()
			break
		case "hmdb":
			d.wg.Add(1)
			h := hmdb{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go h.update()
			break
		case "patent":
			d.wg.Add(1)
			p := patents{source: data, d: d, dataPath: config.Dataconf[data]["path"]}
			d.datasets2 = append(d.datasets2, data)
			go p.update()
			break
		case "clinical_trials":
			d.wg.Add(1)
			ct := clinicalTrials{source: data, d: d, dataPath: config.Dataconf[data]["path"]}
			d.datasets2 = append(d.datasets2, data)
			go ct.update()
			break
		case "string":
			d.wg.Add(1)
			str := stringProcessor{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go str.update(d.selectedTaxids)
			break
		case "reactome":
			d.wg.Add(1)
			r := reactome{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go r.update()
			break
		case "ontology":
			// Process all ontology datasets at once
			ontologyDatasets := []string{"go", "eco", "efo", "uberon", "cl", "mondo", "hpo", "oba", "pato", "obi", "xco"}
			for _, ontoData := range ontologyDatasets {
				switch ontoData {
				case "go":
					d.wg.Add(1)
					g := ontology{source: ontoData, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "GO:"}
					d.datasets2 = append(d.datasets2, ontoData)
					go g.update()
				case "efo":
					d.wg.Add(1)
					g := ontology{source: ontoData, d: d, prefixURL: "http://www.ebi.ac.uk/efo/", idPrefix: "EFO:"}
					d.datasets2 = append(d.datasets2, ontoData)
					go g.update()
				case "eco":
					d.wg.Add(1)
					g := ontology{source: ontoData, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "ECO:"}
					d.datasets2 = append(d.datasets2, ontoData)
					go g.update()
				case "uberon":
					d.wg.Add(1)
					u := ontology{source: ontoData, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "UBERON:"}
					d.datasets2 = append(d.datasets2, ontoData)
					go u.update()
				case "cl":
					d.wg.Add(1)
					c := ontology{source: ontoData, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "CL:"}
					d.datasets2 = append(d.datasets2, ontoData)
					go c.update()
				case "mondo":
					d.wg.Add(1)
					m := mondo{source: ontoData, d: d}
					d.datasets2 = append(d.datasets2, ontoData)
					go m.update()
				case "hpo":
					d.wg.Add(1)
					h := hpo{source: ontoData, d: d}
					d.datasets2 = append(d.datasets2, ontoData)
					go h.update()
				case "oba":
					d.wg.Add(1)
					ob := oba{source: ontoData, d: d}
					d.datasets2 = append(d.datasets2, ontoData)
					go ob.update()
				case "pato":
					d.wg.Add(1)
					pt := pato{source: ontoData, d: d}
					d.datasets2 = append(d.datasets2, ontoData)
					go pt.update()
				case "obi":
					d.wg.Add(1)
					ob := obi{source: ontoData, d: d}
					d.datasets2 = append(d.datasets2, ontoData)
					go ob.update()
				case "xco":
					d.wg.Add(1)
					xc := xco{source: ontoData, d: d}
					d.datasets2 = append(d.datasets2, ontoData)
					go xc.update()
				}
			}
			break
		case "go":
			d.wg.Add(1)
			g := ontology{source: data, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "GO:"}
			d.datasets2 = append(d.datasets2, data)
			go g.update()
			break
		case "efo":
			d.wg.Add(1)
			g := ontology{source: data, d: d, prefixURL: "http://www.ebi.ac.uk/efo/", idPrefix: "EFO:"}
			d.datasets2 = append(d.datasets2, data)
			go g.update()
			break
		case "eco":
			d.wg.Add(1)
			g := ontology{source: data, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "ECO:"}
			d.datasets2 = append(d.datasets2, data)
			go g.update()
			break
		case "uberon":
			d.wg.Add(1)
			u := ontology{source: data, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "UBERON:"}
			d.datasets2 = append(d.datasets2, data)
			go u.update()
			break
		case "cl":
			d.wg.Add(1)
			c := ontology{source: data, d: d, prefixURL: "http://purl.obolibrary.org/obo/", idPrefix: "CL:"}
			d.datasets2 = append(d.datasets2, data)
			go c.update()
			break
		case "bgee":
			d.wg.Add(1)
			b := bgee{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go b.update()
			break
		case "rhea":
			d.wg.Add(1)
			r := rhea{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go r.update()
			break
		case "gwas_study":
			d.wg.Add(1)
			g := gwasStudy{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go g.update()
			break
		case "gwas":
			d.wg.Add(1)
			gw := gwas{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go gw.update()
			break
		case "dbsnp":
			d.wg.Add(1)
			db := dbsnp{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go db.update()
			break
		case "intact":
			d.wg.Add(1)
			ia := intact{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ia.update()
			break
		case "protein_similarity":
			d.wg.Add(1)
			ps := proteinSimilarity{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ps.update()
			break
		case "antibody":
			d.wg.Add(1)
			ab := &antibody{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ab.update()
			break
		case "pubchem":
			d.wg.Add(1)
			pc := &pubchem{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go pc.update()
			break
		case "pubchem_activity":
			d.wg.Add(1)
			pca := &pubchemActivity{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go pca.update(d)
			break
		case "pubchem_assay":
			d.wg.Add(1)
			pcb := &pubchemAssay{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go pcb.update(d)
			break
		case "mondo":
			d.wg.Add(1)
			m := mondo{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go m.update()
			break
		case "hpo":
			d.wg.Add(1)
			h := hpo{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go h.update()
			break
		case "oba":
			d.wg.Add(1)
			ob := oba{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ob.update()
			break
		case "pato":
			d.wg.Add(1)
			pt := pato{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go pt.update()
			break
		case "obi":
			d.wg.Add(1)
			ob := obi{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ob.update()
			break
		case "xco":
			d.wg.Add(1)
			xc := xco{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go xc.update()
			break
		case "entrez":
			d.wg.Add(1)
			en := entrez{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go en.update()
			break
		case "mesh":
			d.wg.Add(1)
			m := mesh{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go m.update()
			break
		case "alphafold":
			d.wg.Add(1)
			af := alphafoldProcessor{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go af.update()
			break
		case "rnacentral":
			d.wg.Add(1)
			rc := rnacentralProcessor{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go rc.update()
			break
		case "clinvar":
			d.wg.Add(1)
			cv := clinvarXML{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go cv.update()
			break
		case "lipidmaps":
			d.wg.Add(1)
			lm := lipidmaps{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go lm.update()
			break
		case "swisslipids":
			d.wg.Add(1)
			sl := swisslipids{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go sl.update()
			break
		case "my_data":

			if len(config.Dataconf[data]["path"]) > 0 {
				d.wg.Add(1)
				m := mydata{source: data, d: d}
				d.datasets2 = append(d.datasets2, data)
				go m.update()
			} else {
				log.Fatal("Missing source path for my_data ")
			}

			break
		case "literature_mappings":
			d.wg.Add(1)
			l := literature{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go l.update()
			break
		case "uniprot_unreviewed":
			d.wg.Add(1)
			u2 := uniprot{source: data, d: d, trembl: true}
			d.datasets2 = append(d.datasets2, data)
			go u2.update(d.selectedTaxids)
			break
		case "chembl":
			chemblDatasets := []string{"chembl_document", "chembl_assay", "chembl_activity", "chembl_molecule", "chembl_target", "chembl_target_component", "chembl_cell_line"}
			for _, chembldata := range chemblDatasets {
				d.wg.Add(1)
				c := chembl{source: chembldata, d: d}
				d.datasets2 = append(d.datasets2, chembldata)
				go c.update()
			}
			break
		case "chembl_document", "chembl_assay", "chembl_activity", "chembl_molecule", "chembl_target", "chembl_target_component", "chembl_cell_line":
			d.wg.Add(1)
			c := chembl{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go c.update()
			break
		default:
			log.Fatal("ERROR Unrecognized dataset ->" + data)
		}
	}

	sort.Strings(d.datasets2)

	for i, data := range d.datasets2 {
		fmt.Print(strconv.Itoa(i)+"-", data, " ")
	}
	fmt.Println("")

	go d.showProgres()

	d.wg.Wait()

	// Close bucket writers and wait for them to finish
	var bucketLines uint64
	if d.bucketPool != nil && len(bucketConfigs) > 0 {
		log.Println("Closing bucket writers...")
		d.bucketPool.Close()
		d.bucketWg.Wait()
		log.Println("Bucket writers closed")

		// Sort all bucket files
		log.Println("Sorting bucket files...")
		if err := SortAllBuckets(d.bucketPool, 0); err != nil { // 0 uses BucketSortWorkers from config
			log.Printf("Error sorting buckets: %v", err)
		}

		// Concatenate buckets and move to index directory
		log.Println("Concatenating bucket files...")
		var err error
		bucketLines, err = ConcatenateBuckets(d.bucketPool, config.Appconf["indexDir"], chunkIdx)
		if err != nil {
			log.Printf("Error concatenating buckets: %v", err)
		}
		log.Printf("Bucket processing complete (%d lines after deduplication)", bucketLines)
	}

	for i := 0; i < len(kvgens); i++ {
		kvgens[i].wg.Add(1)
	}
	close(e)
	wg.Wait()

	var totalkv uint64

	for i := 0; i < len(kvgens); i++ {
		totalkv = totalkv + kvgens[i].totalkv
	}

	// Add bucket lines to total (post-deduplication count from concatenation)
	totalkv = totalkv + bucketLines

	log.Println("Data update process completed. Making last merges...")
	// send finish signal to bmerge
	mergeGateCh <- mergeInfo{
		close: true,
		level: 1,
	}

	wgBmerge.Wait()
	d.stats["totalKV"] = totalkv
	data, err := json.Marshal(d.stats)
	if err != nil {
		log.Println("Error while writing meta data")
	}

	ioutil.WriteFile(filepath.FromSlash(config.Appconf["indexDir"]+"/"+chunkIdx+".meta.json"), data, 0700)

	// write id file infos
	if len(idMeta) > 0 {
		data, err = json.Marshal(idMeta)
		if err != nil {
			log.Println("Error while writing id meta data")
		}
		ioutil.WriteFile(filepath.FromSlash(config.Appconf["idDir"]+"/"+chunkIdx+".meta.json"), data, 0700)
	}

	log.Println("All done.")

	return d.totalParsedEntry, totalkv

}

func (d *DataUpdate) showProgres() {

	latestProg := map[string]progressInfo{}
	var result strings.Builder

	for info := range d.progChan {

		alldone := false
		latestProg[info.dataset] = *info
		if len(d.datasets2) == len(latestProg) {
			alldone = true
			elapsed := int64(time.Since(d.start).Seconds())
			result.Reset()
			result.WriteString("\r")
			result.WriteString("Processing...Elapsed ")
			result.WriteString(strconv.FormatInt(elapsed, 10))
			result.WriteString("s")
			for i, ds := range d.datasets2 {

				result.WriteString(" ")
				if len(d.datasets2) > 7 {
					result.WriteString("d")
					result.WriteString(strconv.FormatInt(int64(i), 10))
				} else {
					result.WriteString(latestProg[ds].dataset)
				}
				result.WriteString(":")
				if latestProg[ds].done {
					result.WriteString("DONE")
				} else if latestProg[ds].waiting {
					result.WriteString("Waiting")
					alldone = false
				} else {
					//result.WriteString(string(latestProg[ds].currentKBPerSec))
					alldone = false
					result.WriteString(strconv.FormatInt(latestProg[ds].currentKBPerSec, 10))
					delete(latestProg, ds)
				}

			}
			if alldone {
				close(d.progChan)
				result.WriteString(" KB/s")
				fmt.Printf(result.String())
				fmt.Println("")
				return
			}

			result.WriteString(" KB/s")
			fmt.Printf(result.String())

		}

	}

}

func (d *DataUpdate) addEntryStat(source string, total uint64) {

	var entrysize = map[string]interface{}{}
	entrysize["entrySize"] = total
	mutex.Lock()
	d.stats[source] = entrysize
	mutex.Unlock()

}

func (d *DataUpdate) addMeta(source string, meta map[string]interface{}) {

	mutex.Lock()

	if _, ok := d.stats[source]; ok {

		for k, v := range meta {
			d.stats[source].(map[string]interface{})[k] = v
		}

	} else {
		d.stats[source] = meta
	}

	mutex.Unlock()

}

func (d *DataUpdate) addProp3(key, from string, attr []byte) {

	key = strings.TrimSpace(key)

	if len(key) == 0 || len(from) == 0 || len(attr) <= 2 { // empty attr {}
		return
	}

	kup := strings.ToUpper(key)
	line := kup + tab + from + tab + string(attr) + tab + textStoreID

	// Check if dataset has bucket config - route to buckets if so
	if d.bucketPool != nil && d.bucketPool.HasBucketConfig(from) {
		d.bucketPool.Write(from, kup, line)
	} else {
		*d.kvdatachan <- line
	}

}

func (d *DataUpdate) addXref(key string, from string, value string, valueFrom string, isLink bool) {
	// Backward compatible - calls new function with empty evidence
	d.addXrefWithEvidence(key, from, value, valueFrom, isLink, "")
}

// addXrefWithEvidence adds cross-reference with optional evidence code
// evidence: Optional evidence/quality metadata (e.g., "TAS", "IEA" for Reactome)
func (d *DataUpdate) addXrefWithEvidence(key string, from string, value string, valueFrom string, isLink bool, evidence string) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	evidence = strings.TrimSpace(evidence)

	if len(key) == 0 || len(value) == 0 || len(from) == 0 {
		return
	}

	if _, ok := config.Dataconf[valueFrom]; !isLink && !ok {
		//if config.Appconf["debug"] == "y" {
		if val, _ := d.invalidXrefs.Get(valueFrom); val == nil {
			//fmt.Println("Warn:Undefined xref name:", valueFrom, "with value", value, " skipped!. Define in data.json to be included")
			//not to print again.
			d.invalidXrefs.Set(valueFrom, "true")
		}
		//}
		return
	}

	// now target datasets check
	if _, ok := d.targetDatasets[valueFrom]; d.hasTargets && !ok && !isLink {
		return
	}

	kup := strings.ToUpper(key)
	vup := strings.ToUpper(value)

	// Storage format: KEY <tab> FROM <tab> VALUE <tab> DATASETID <tab> EVIDENCE (optional)
	dataLine := kup + tab + from + tab + vup + tab + config.Dataconf[valueFrom]["id"]
	if evidence != "" {
		dataLine += tab + evidence
	}

	if isLink {
		// Text/keyword links route to textsearch buckets (alphabetic by first letter)
		d.bucketPool.Write(TextSearchDatasetID, kup, dataLine)
	} else {
		// Reverse mapping also includes evidence
		valueFromID := config.Dataconf[valueFrom]["id"]
		reverseDataLine := vup + tab + valueFromID + tab + kup + tab + from
		if evidence != "" {
			reverseDataLine += tab + evidence
		}

		// Check if source or target dataset has bucket config
		// Forward xref: keyed by source ID (kup), goes to source dataset's buckets
		// Reverse xref: keyed by target ID (vup), goes to target dataset's buckets
		hasBucketPool := d.bucketPool != nil
		sourceHasBucket := hasBucketPool && d.bucketPool.HasBucketConfig(from)
		targetHasBucket := hasBucketPool && d.bucketPool.HasBucketConfig(valueFromID)

		// Route forward xref (keyed by source ID kup)
		if sourceHasBucket {
			d.bucketPool.Write(from, kup, dataLine)
		} else {
			*d.kvdatachan <- dataLine
		}

		// Route reverse xref (keyed by target ID vup)
		if targetHasBucket {
			d.bucketPool.Write(valueFromID, vup, reverseDataLine)
		} else {
			*d.kvdatachan <- reverseDataLine
		}
	}

}

// this is similar with addXref but for only link datasets like orthologes,paralogs where no need text link checking and reverse mapping creation
func (d *DataUpdate) addXref2(key string, from string, value string, valueFrom string) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if len(key) == 0 || len(value) == 0 || len(from) == 0 {
		return
	}

	if _, ok := config.Dataconf[valueFrom]; !ok {
		if val, _ := d.invalidXrefs.Get(valueFrom); val == nil {
			d.invalidXrefs.Set(valueFrom, "true")
		}
		return
	}

	// now target datasets check
	if _, ok := d.targetDatasets[valueFrom]; d.hasTargets && !ok {
		return
	}

	kup := strings.ToUpper(key)
	vup := strings.ToUpper(value)
	valueFromID := config.Dataconf[valueFrom]["id"]
	line := kup + tab + from + tab + vup + tab + valueFromID

	// Check if source or target dataset has bucket config
	// For link datasets (hpochild, taxparent, etc.), the key comes from the source dataset
	// which typically has bucket config (hpo, taxonomy, etc.)
	if d.bucketPool != nil {
		if d.bucketPool.HasBucketConfig(from) {
			// Source dataset has bucket config - route by key (source ID)
			d.bucketPool.Write(from, kup, line)
		} else if d.bucketPool.HasBucketConfig(valueFromID) {
			// Target dataset has bucket config - route by value (target ID)
			d.bucketPool.Write(valueFromID, vup, line)
		} else {
			*d.kvdatachan <- line
		}
	} else {
		*d.kvdatachan <- line
	}

}

// addXref2Bucketed routes link dataset xrefs through bucket system
// For link datasets (taxchild, taxparent, gochild, goparent, etc.), the key is from
// the parent dataset (taxonomy, go, etc.) which has bucket config.
// bucketDatasetID: the dataset ID to use for bucket routing (e.g., taxonomy's ID for taxchild/taxparent)
func (d *DataUpdate) addXref2Bucketed(key string, from string, value string, valueFrom string, bucketDatasetID string) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if len(key) == 0 || len(value) == 0 || len(from) == 0 {
		return
	}

	if _, ok := config.Dataconf[valueFrom]; !ok {
		if val, _ := d.invalidXrefs.Get(valueFrom); val == nil {
			d.invalidXrefs.Set(valueFrom, "true")
		}
		return
	}

	// Target datasets check
	if _, ok := d.targetDatasets[valueFrom]; d.hasTargets && !ok {
		return
	}

	kup := strings.ToUpper(key)
	vup := strings.ToUpper(value)

	line := kup + tab + from + tab + vup + tab + config.Dataconf[valueFrom]["id"]

	// Route through bucket pool using the specified bucket dataset ID
	// This allows link datasets (taxchild, taxparent) to use parent dataset's buckets
	d.bucketPool.Write(bucketDatasetID, kup, line)
}

// addXrefBucketed routes xrefs through bucket system for optimized datasets
// Use this for datasets with bucketMethod configured in source.dataset.json
// Falls back to kvdatachan for datasets without bucket configuration
func (d *DataUpdate) addXrefBucketed(key, from, value, valueFrom string, isLink bool) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if len(key) == 0 || len(value) == 0 || len(from) == 0 {
		return
	}

	if _, ok := config.Dataconf[valueFrom]; !isLink && !ok {
		if val, _ := d.invalidXrefs.Get(valueFrom); val == nil {
			d.invalidXrefs.Set(valueFrom, "true")
		}
		return
	}

	// Target datasets check
	if _, ok := d.targetDatasets[valueFrom]; d.hasTargets && !ok && !isLink {
		return
	}

	kup := strings.ToUpper(key)
	vup := strings.ToUpper(value)

	valueFromID := config.Dataconf[valueFrom]["id"]

	// Build forward and reverse lines (same format as addXref)
	forwardLine := kup + tab + from + tab + vup + tab + valueFromID
	reverseLine := vup + tab + valueFromID + tab + kup + tab + from

	// Route through bucket pool (handles bucket vs fallback automatically)
	d.bucketPool.WriteXref(from, kup, forwardLine, valueFromID, vup, reverseLine)

	// Text search link (always goes to kvdatachan)
	if isLink {
		*d.kvdatachan <- kup + tab + textLinkID + tab + vup + tab + from
	}
}

// addProp3Bucketed routes properties through bucket system
// Falls back to kvdatachan for datasets without bucket configuration
func (d *DataUpdate) addProp3Bucketed(key, from string, attr []byte) {

	key = strings.TrimSpace(key)

	if len(key) == 0 || len(from) == 0 || len(attr) <= 2 { // empty attr {}
		return
	}

	kup := strings.ToUpper(key)
	line := kup + tab + from + tab + string(attr) + tab + textStoreID
	d.bucketPool.Write(from, kup, line)
}

// addXrefViaKeyword resolves keyword to database identifiers via lookup, then creates xrefs
// keyword: The keyword to lookup (e.g., "BRCA1")
// keywordDataset: Dataset to filter results (empty string = accept all matches for auto-enrichment)
// targetValue: The value to link to (e.g., HPO ID)
// targetDataset: The dataset of the target value
// from: Source dataset name
// isLink: Whether this is a link-only relationship
func (d *DataUpdate) addXrefViaKeyword(keyword string, keywordDataset string, targetValue string, targetDataset string, from string, isLink bool) {
	if !d.hasLookupDB {
		return
	}

	// Lookup keyword in database
	result, err := d.lookup(keyword)
	if err != nil || result == nil || len(result.Results) == 0 {
		return
	}

	//fmt.Println("=== Lookup result for keyword:", keyword, "===")
	//fmt.Println("Total results:", len(result.Results))
	//for i, r := range result.Results {
	//	fmt.Printf("Result[%d]: Identifier=%s, Dataset=%d, IsLink=%v, Entries count=%d\n",
	//		i, r.Identifier, r.Dataset, r.IsLink, len(r.Entries))
	//}
	//fmt.Println("==========================================")

	// When lookup returns a keyword result, the actual identifiers are in the entries
	// We need to extract them from all results
	for _, r := range result.Results {
		// Check if this is a link result (keyword lookup)
		if !r.IsLink || len(r.Entries) == 0 {
			continue
		}

		// Loop through entries to get actual dataset IDs and identifiers
		for _, entry := range r.Entries {
			// Filter by keywordDataset if specified
			if keywordDataset != "" {
				datasetID, ok := config.Dataconf[keywordDataset]["id"]
				if ok {
					var targetID uint32
					fmt.Sscanf(datasetID, "%d", &targetID)
					if entry.Dataset != targetID {
						continue // Skip entries that don't match the filter
					}
				}
			}

			// Verify dataset ID exists in config
			if _, ok := config.DataconfIDIntToString[entry.Dataset]; !ok {
				continue
			}

			// Convert dataset ID to string
			datasetIDStr := strconv.Itoa(int(entry.Dataset))
			d.addXref(entry.Identifier, datasetIDStr, targetValue, targetDataset, isLink)
		}
	}
}

// addXrefViaGeneSymbol creates cross-reference from variant to Ensembl gene via gene symbol lookup
// Handles paralog cases by creating xrefs to all matching Ensembl genes
// chromosome parameter is kept for future enhancement but not used currently
// geneSymbol: Gene symbol (e.g., "BRCA1", "DDX11L16")
// chromosome: Chromosome name/ID (reserved for future filtering)
// variantID: Variant ID (SNP, GWAS association, etc.)
// variantDataset: Variant dataset name (e.g., "dbsnp", "gwas")
// variantDatasetID: Variant dataset ID string
func (d *DataUpdate) addXrefViaGeneSymbol(geneSymbol, chromosome, variantID, variantDataset, variantDatasetID string) {
	// Use addXrefViaKeyword to lookup symbol in Ensembl (instead of HGNC)
	// This will create xrefs to all matching Ensembl genes
	// For paralogs like DDX11L16, this creates xrefs to all copies (chr1, chrX, chrY)
	// This follows biobtree's deterministic principle: show all or none
	d.addXrefViaKeyword(geneSymbol, "ensembl", variantID, variantDataset, variantDatasetID, false)
}

// addXrefEnsemblViaHgnc creates ClinVar → Ensembl cross-reference via HGNC
// This ensures we only get human Ensembl genes by going through HGNC first
// geneSymbol: Gene symbol (e.g., "BRCA1")
// clinvarID: ClinVar variant ID
// clinvarDatasetID: ClinVar dataset ID
func (d *DataUpdate) addXrefEnsemblViaHgnc(geneSymbol, clinvarID, clinvarDatasetID string) {
	if !d.hasLookupDB {
		return
	}

	// Step 1: Lookup gene symbol to find HGNC entry
	result, err := d.lookup(geneSymbol)
	if err != nil || result == nil || len(result.Results) == 0 {
		return
	}

	// Step 2: Find HGNC entry in the results
	hgncDatasetID, ok := config.Dataconf["hgnc"]["id"]
	if !ok {
		return
	}

	var hgncDatasetInt uint32
	fmt.Sscanf(hgncDatasetID, "%d", &hgncDatasetInt)

	var hgncIdentifier string
	for _, r := range result.Results {
		// Look for link results (keyword search results)
		if !r.IsLink || len(r.Entries) == 0 {
			continue
		}

		// Find HGNC entry
		for _, entry := range r.Entries {
			if entry.Dataset == hgncDatasetInt {
				hgncIdentifier = entry.Identifier
				break
			}
		}

		if hgncIdentifier != "" {
			break
		}
	}

	if hgncIdentifier == "" {
		return // No HGNC entry found
	}

	// Step 3: Lookup HGNC entry directly to get its cross-references
	hgncResult, err := d.lookup(hgncIdentifier)
	if err != nil || hgncResult == nil || len(hgncResult.Results) == 0 {
		return
	}

	// Step 4: Find Ensembl cross-reference in HGNC entry
	ensemblDatasetID, ok := config.Dataconf["ensembl"]["id"]
	if !ok {
		return
	}

	var ensemblDatasetInt uint32
	fmt.Sscanf(ensemblDatasetID, "%d", &ensemblDatasetInt)

	// Look through HGNC's cross-references for Ensembl
	for _, hgncRes := range hgncResult.Results {
		if len(hgncRes.Entries) == 0 {
			continue
		}

		for _, entry := range hgncRes.Entries {
			if entry.Dataset == ensemblDatasetInt {
				// Found human Ensembl gene - create cross-reference
				d.addXref(clinvarID, clinvarDatasetID, entry.Identifier, "ensembl", false)
				return // Only need one Ensembl reference
			}
		}
	}
}

// addXrefEnsemblViaEntrez creates a cross-reference to Ensembl gene via Entrez Gene ID
// entrezGeneID: NCBI Gene ID (Entrez Gene ID) as string
// sourceID: The identifier of the source entity (e.g., activity ID, bioassay ID)
// sourceDatasetID: The dataset ID of the source entity
func (d *DataUpdate) addXrefEnsemblViaEntrez(entrezGeneID, sourceID, sourceDatasetID string) {
	if !d.hasLookupDB {
		return
	}

	// Get Entrez dataset ID
	entrezDatasetID, ok := config.Dataconf["entrez"]["id"]
	if !ok {
		return // Entrez dataset not configured
	}

	var entrezDatasetInt uint32
	fmt.Sscanf(entrezDatasetID, "%d", &entrezDatasetInt)

	// Step 1: Lookup Entrez Gene ID
	result, err := d.lookup(entrezGeneID)
	if err != nil || result == nil || len(result.Results) == 0 {
		return // Entrez Gene ID not found in database
	}

	// Step 2: Find Entrez entry in results (filter by dataset)
	var entrezEntry *pbuf.Xref
	for _, xref := range result.Results {
		if xref.Dataset == entrezDatasetInt {
			entrezEntry = xref
			break
		}
	}

	if entrezEntry == nil || len(entrezEntry.Entries) == 0 {
		return // No Entrez entry found or no cross-references
	}

	// Step 3: Find Ensembl cross-reference in Entrez entry
	ensemblDatasetID, ok := config.Dataconf["ensembl"]["id"]
	if !ok {
		return
	}

	var ensemblDatasetInt uint32
	fmt.Sscanf(ensemblDatasetID, "%d", &ensemblDatasetInt)

	// Look for Ensembl gene in Entrez entry's cross-references
	for _, entry := range entrezEntry.Entries {
		if entry.Dataset == ensemblDatasetInt {
			// Found Ensembl gene - create cross-reference
			d.addXref(sourceID, sourceDatasetID, entry.Identifier, "ensembl", false)
			return // Only need one Ensembl reference
		}
	}
}


func (d *DataUpdate) selectEnsembls() map[string]ensembl {

	selectedEnsembls := []string{}
	for _, src := range allEnsembls { // this is to check command line ensembl datasets if yes genome selection will be only within those ones otherwise all
		if _, ok := d.inDatasets[src]; ok {
			selectedEnsembls = append(selectedEnsembls, src)
		}
	}

	// Only process ensembl if explicitly included in datasets (-d parameter)
	if len(selectedEnsembls) == 0 {
		return map[string]ensembl{}
	}

	ensembls := map[string]ensembl{}

	for _, src := range selectedEnsembls {
		switch src {
		case "ensembl":
			ensembls["ensembl"] = ensembl{source: src, d: d, branch: pbuf.Ensemblbranch_ENSEMBL, ftpAddress: config.Appconf["ensembl_ftp"]}
		case "ensembl_bacteria":
			ensembls[src] = ensembl{source: src, d: d, branch: pbuf.Ensemblbranch_BACTERIA, ftpAddress: config.Appconf["ensembl_genomes_ftp"]}
		case "ensembl_fungi":
			ensembls[src] = ensembl{source: src, d: d, branch: pbuf.Ensemblbranch_FUNGI, ftpAddress: config.Appconf["ensembl_genomes_ftp"]}
		case "ensembl_metazoa":
			ensembls[src] = ensembl{source: src, d: d, branch: pbuf.Ensemblbranch_METAZOA, ftpAddress: config.Appconf["ensembl_genomes_ftp"]}
		case "ensembl_plants":
			ensembls[src] = ensembl{source: src, d: d, branch: pbuf.Ensemblbranch_PLANT, ftpAddress: config.Appconf["ensembl_genomes_ftp"]}
		case "ensembl_protists":
			ensembls[src] = ensembl{source: src, d: d, branch: pbuf.Ensemblbranch_PROTIST, ftpAddress: config.Appconf["ensembl_genomes_ftp"]}
		}
	}

	// select genomes
	res := map[string]ensembl{}
	for src, ens := range ensembls {

		if ens.selectGenomes() {
			res[src] = ens
		}

	}

	return res
}

func check(err error) {

	if err != nil {
		log.Fatal("Error: ", err)
	}

}

// checkWithContext provides error context with dataset and operation information
func checkWithContext(err error, dataset string, operation string) {
	if err != nil {
		log.Fatalf("[%s] Error during %s: %v", dataset, operation, err)
	}
}
