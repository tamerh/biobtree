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
const tab = "\t"
const newline = "\n"

// LMDBMaxKeySize is the maximum key size for LMDB (default 511 bytes)
// Keys longer than this will be skipped to prevent MDB_BAD_VALSIZE errors
const LMDBMaxKeySize = 511

var fileBufSize = 65536
var chunkIdx = "df"

var mutex = &sync.Mutex{}
var config *configs.Conf

// InitConfig initializes the package-level config for source checking
// Call this before using CheckSourceChanged outside of a DataUpdate context
func InitConfig(c *configs.Conf) {
	config = c
}

var allEnsembls = []string{"ensembl", "ensembl_fungi", "ensembl_bacteria", "ensembl_metazoa", "ensembl_plants", "ensembl_protists"}

type DataUpdate struct {
	totalParsedEntry       uint64
	wg                     *sync.WaitGroup
	inDatasets             map[string]bool
	datasets2              []string // after resolving the input datasets
	start                  time.Time
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
	targetDatasets         map[string]bool
	hasTargets             bool
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
	useLookupDB            bool              // Flag to enable/disable lookup database loading
	resumeSort             bool              // Flag to resume from sorting phase (skip data processing)
	forceRebuild           bool              // Flag to force reprocessing even if source unchanged
	datasetState           *DatasetState     // State tracking for incremental updates
}

type progressInfo struct {
	dataset         string
	currentKBPerSec int64
	done            bool
	waiting         bool
	mergeOnly       bool // true if dataset was already processed and only needs merge
}

func NewDataUpdate(datasets map[string]bool, targetDatasets, ensemblSpecies, ensemblSpeciesPattern []string, genometaxids []int, skipEnsembl bool, orthologIDs map[int]bool, orthologs, orthologsAll bool, conf *configs.Conf, chkIdx string, useLookupDB bool) *DataUpdate {

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
		useLookupDB:            useLookupDB,
	}

}

// SetResumeSort enables resume-from-sort mode (skips data processing, only does sort+concat+meta)
func (d *DataUpdate) SetResumeSort(resume bool) {
	d.resumeSort = resume
}

// SetForceRebuild enables force mode (reprocess datasets even if source unchanged)
func (d *DataUpdate) SetForceRebuild(force bool) {
	d.forceRebuild = force
}

// Initialize read-only lookup database for keyword-to-ID resolution
func (d *DataUpdate) initLookupDB() {
	// Check if lookup database is disabled via --lookupdb flag
	if !d.useLookupDB {
		log.Println("Lookup database disabled (use --lookupdb flag to enable)")
		d.hasLookupDB = false
		return
	}

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

	// Resume mode doesn't need datasets - it works with existing bucket files
	if len(ensembls) <= 0 && len(d.inDatasets) <= 0 && !d.resumeSort {
		log.Println("No genome found for indexing")
		return 0, 0
	}

	var wg sync.WaitGroup
	var mergeGateCh = make(chan mergeInfo, 10000)

	d.wg = &wg
	d.mergeGateCh = &mergeGateCh

	// Initialize bucket system for optimized datasets
	LoadBucketSystemConfig()
	bucketConfigs := LoadBucketConfigs()

	// Auto-create bucket configs for derived datasets (datasets from xref*.json without explicit bucketMethod)
	// This enables source-tagged directories for all datasets
	bucketConfigs = LoadDerivedBucketConfigs(bucketConfigs)

	var bucketWg sync.WaitGroup
	d.bucketWg = &bucketWg
	d.bucketPool = NewHybridWriterPool(bucketConfigs, config.Appconf["indexDir"], &bucketWg)
	if len(bucketConfigs) > 0 {
		log.Printf("Bucket system initialized with %d configured datasets", len(bucketConfigs))
	}

	// Load dataset state for incremental updates (from main output directory)
	d.datasetState, _ = LoadDatasetState(config.Appconf["outDir"])
	d.datasetState.BuildVersion = "1.8.0" // TODO: use actual version

	// Clean up any datasets that were interrupted in a previous build
	// These have status "processing" and their bucket files may be corrupted/incomplete
	if err := CleanupInterruptedDatasets(d.datasetState, config.Appconf["indexDir"], config.Appconf["outDir"], config.Dataconf); err != nil {
		log.Printf("Warning: failed to cleanup interrupted datasets: %v", err)
	}

	// Resume mode: skip data processing, go directly to sort+concat+meta
	if d.resumeSort {
		log.Println("Resume mode: skipping data processing, starting from sort phase")

		// Mark existing bucket files as created
		fileCount := d.bucketPool.MarkExistingFilesCreated()
		if fileCount == 0 {
			log.Println("Warning: No existing bucket files found to resume")
			return 0, 0
		}

		// Sort all bucket files
		log.Println("Sorting bucket files...")
		if err := SortAllBuckets(d.bucketPool, 0); err != nil {
			log.Printf("Error sorting buckets: %v", err)
		}

		// Concatenate buckets and move to index directory
		log.Println("Concatenating bucket files...")
		bucketStats, err := ConcatenateBuckets(d.bucketPool, config.Appconf["indexDir"], config.Appconf["outDir"], chunkIdx, d.datasetState)
		if err != nil {
			log.Printf("Error concatenating buckets: %v", err)
			return 0, 0
		}

		var bucketLines uint64
		if bucketStats != nil {
			bucketLines = bucketStats.TotalLines
			for datasetName, lineCount := range bucketStats.PerDataset {
				d.addKVStat(datasetName, lineCount)
			}
			// Save per-source contributions for each dataset
			if d.datasetState != nil {
				for datasetName, sourceStats := range bucketStats.PerDatasetSource {
					d.datasetState.SetSourceContributions(datasetName, sourceStats)
				}
			}
		}
		log.Printf("Bucket processing complete (%d lines after deduplication)", bucketLines)

		// Update total entries in dataset state (replaces meta.json totalKV)
		if d.datasetState != nil {
			d.datasetState.AddTotalKVSize(bucketLines)
			if err := SaveDatasetState(d.datasetState, config.Appconf["outDir"]); err != nil {
				log.Printf("Warning: failed to save dataset state: %v", err)
			}
		}

		log.Println("Resume complete.")
		return 0, bucketLines
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

	// ============================================================================
	// PHASE 1: Cleanup all datasets BEFORE any processing starts
	// ============================================================================
	// This prevents race conditions where one dataset's cleanup could delete
	// files that another dataset's goroutine is actively writing.
	// Previously, cleanup and goroutine spawning were interleaved in a single loop,
	// causing files to be deleted while other datasets were still processing.
	// ============================================================================

	// Collect datasets that need processing (not skipped, not merge-only)
	var datasetsToProcess []string

	for data := range d.inDatasets {
		// Check if dataset should be skipped (already built and source unchanged)
		if d.shouldSkipDataset(data) {
			log.Printf("Skipping dataset %s (already built and source unchanged)", data)
			continue
		}

		// Check if dataset needs "merge only" (was processed but not merged)
		// In this case, skip parsing but include in datasets2 for merge
		if d.needsMergeOnly(data) {
			log.Printf("Dataset %s needs merge only (already processed), skipping parsing", data)
			d.datasets2 = append(d.datasets2, data)
			// Signal done immediately since no processing needed
			// Set mergeOnly=true to preserve original build_duration from previous run
			d.progChan <- &progressInfo{dataset: data, done: true, mergeOnly: true}
			continue
		}

		// This dataset needs processing
		datasetsToProcess = append(datasetsToProcess, data)

		// Mark dataset as "processing" BEFORE cleanup/processing starts
		// This ensures if we crash mid-processing, next run will rebuild
		// Skip meta-datasets (ontology, chembl) - they spawn sub-datasets and don't do actual work
		isMetaDataset := data == "ontology" || data == "chembl"
		if d.datasetState != nil && !isMetaDataset {
			datasetID := ""
			if props, ok := config.Dataconf[data]; ok {
				datasetID = props["id"]
			}
			d.datasetState.MarkDatasetProcessing(data, datasetID)
			// Save immediately so crash recovery works
			if err := SaveDatasetState(d.datasetState, config.Appconf["outDir"]); err != nil {
				log.Printf("Warning: failed to save processing state for %s: %v", data, err)
			}
		}

		// Clean up old bucket files and sorted files before re-processing
		// This ensures incremental updates don't leave stale data
		// Skip meta-datasets - their sub-datasets handle their own cleanup
		if !isMetaDataset {
			if err := CleanupForIncrementalUpdate(data, config.Appconf["indexDir"], config.Dataconf); err != nil {
				log.Printf("Warning: cleanup failed for %s: %v", data, err)
			}
		}
	}

	log.Printf("Phase 1 complete: cleaned up %d datasets, now starting processing phase", len(datasetsToProcess))

	// ============================================================================
	// PHASE 2: Spawn processing goroutines AFTER all cleanups are complete
	// ============================================================================

	for _, data := range datasetsToProcess {
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
			ontologyDatasets := []string{"go", "eco", "efo", "uberon", "cl", "mondo", "hpo", "oba", "pato", "obi", "xco", "bao"}
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
				case "bao":
					d.wg.Add(1)
					ba := bao{source: ontoData, d: d}
					d.datasets2 = append(d.datasets2, ontoData)
					go ba.update()
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
		case "refseq":
			d.wg.Add(1)
			rs := refseq{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go rs.update(d.selectedTaxids)
			break
		case "mesh":
			d.wg.Add(1)
			m := mesh{source: data, d: d, treeToDescriptor: make(map[string]string)}
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
		case "gencc":
			d.wg.Add(1)
			gc := gencc{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go gc.update()
			break
		case "bindingdb":
			d.wg.Add(1)
			bdb := bindingdb{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go bdb.update()
			break
		case "ctd":
			d.wg.Add(1)
			ctdParser := ctd{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ctdParser.update()
			break
		case "pharmgkb":
			d.wg.Add(1)
			pgkb := pharmgkb{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			// Also add subsidiary datasets to tracking
			if _, exists := config.Dataconf["pharmgkb_gene"]; exists {
				d.datasets2 = append(d.datasets2, "pharmgkb_gene")
			}
			if _, exists := config.Dataconf["pharmgkb_clinical"]; exists {
				d.datasets2 = append(d.datasets2, "pharmgkb_clinical")
			}
			if _, exists := config.Dataconf["pharmgkb_variant"]; exists {
				d.datasets2 = append(d.datasets2, "pharmgkb_variant")
			}
			if _, exists := config.Dataconf["pharmgkb_guideline"]; exists {
				d.datasets2 = append(d.datasets2, "pharmgkb_guideline")
			}
			if _, exists := config.Dataconf["pharmgkb_pathway"]; exists {
				d.datasets2 = append(d.datasets2, "pharmgkb_pathway")
			}
			go pgkb.update()
			break
		case "pharmgkb_gene", "pharmgkb_clinical", "pharmgkb_variant", "pharmgkb_guideline", "pharmgkb_pathway":
			// These are processed by the pharmgkb parser, skip standalone processing
			break
		case "biogrid":
			d.wg.Add(1)
			bg := biogrid{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go bg.update()
			break
		case "drugcentral":
			d.wg.Add(1)
			dc := drugcentral{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go dc.update()
			break
		case "bao":
			d.wg.Add(1)
			ba := bao{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ba.update()
			break
		case "msigdb":
			d.wg.Add(1)
			ms := msigdb{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go ms.update()
			break
		case "alphamissense":
			d.wg.Add(1)
			am := alphamissense{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go am.update()
			break
		case "alphamissense_transcript":
			d.wg.Add(1)
			amt := alphamissenseTranscript{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go amt.update()
			break
		case "cellxgene":
			d.wg.Add(1)
			cxg := cellxgene{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			// Also add cellxgene_celltype to tracking (processed by cellxgene parser)
			if _, exists := config.Dataconf["cellxgene_celltype"]; exists {
				d.datasets2 = append(d.datasets2, "cellxgene_celltype")
			}
			go cxg.update()
			break
		case "cellxgene_celltype":
			// Processed by the cellxgene parser, skip standalone processing
			break
		case "scxa":
			d.wg.Add(1)
			sx := scxa{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go sx.update()
			break
		case "scxa_expression":
			d.wg.Add(1)
			sxe := scxaExpression{source: data, d: d}
			d.datasets2 = append(d.datasets2, data)
			go sxe.update()
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

		// Validate that all created bucket files actually exist on disk
		// This helps diagnose race conditions or file creation issues
		if missingCount := d.bucketPool.ValidateCreatedFiles(); missingCount > 0 {
			log.Printf("Warning: %d bucket files marked as created but missing - check for race conditions", missingCount)
		}

		// Sort all bucket files
		log.Println("Sorting bucket files...")
		if err := SortAllBuckets(d.bucketPool, 0); err != nil { // 0 uses BucketSortWorkers from config
			log.Printf("Error sorting buckets: %v", err)
		}

		// Concatenate buckets and move to index directory
		log.Println("Concatenating bucket files...")
		bucketStats, err := ConcatenateBuckets(d.bucketPool, config.Appconf["indexDir"], config.Appconf["outDir"], chunkIdx, d.datasetState)
		if err != nil {
			log.Printf("Error concatenating buckets: %v", err)
		}
		if bucketStats != nil {
			bucketLines = bucketStats.TotalLines
			// Add entry stats for all datasets from concatenation counts
			// This is the accurate post-deduplication count
			for datasetName, lineCount := range bucketStats.PerDataset {
				d.addKVStat(datasetName, lineCount)
			}
			// Save per-source contributions for each dataset
			if d.datasetState != nil {
				for datasetName, sourceStats := range bucketStats.PerDatasetSource {
					d.datasetState.SetSourceContributions(datasetName, sourceStats)
				}
			}
		}

		// Handle "merge only" datasets (status=processed, bucket files exist but not in pool)
		// IMPORTANT: Exclude datasets that are already in the pool - they were just merged above
		// This prevents double-processing which causes sorted file corruption
		poolDatasets := make(map[string]bool)
		for _, writer := range d.bucketPool.GetBucketWriters() {
			poolDatasets[writer.config.DatasetName] = true
		}

		mergeOnlyDatasets := []string{}
		for _, dsName := range d.getMergeOnlyDatasets() {
			if !poolDatasets[dsName] {
				mergeOnlyDatasets = append(mergeOnlyDatasets, dsName)
			} else {
				log.Printf("Skipping %s for merge-only (already merged from pool)", dsName)
			}
		}

		if len(mergeOnlyDatasets) > 0 {
			log.Printf("Processing %d 'merge only' datasets...", len(mergeOnlyDatasets))
			for _, dsName := range mergeOnlyDatasets {
				// Check if dataset is already merged (crash recovery case)
				if d.datasetState != nil && d.datasetState.GetDatasetStatus(dsName) == StatusMerged {
					log.Printf("Skipping %s for merge-only (already merged from previous run)", dsName)
					continue
				}

				// Sort existing bucket files
				// Use global BucketSortMethod setting (no per-dataset override)
				isDerived := false
				useUnixSort := BucketSortMethod == "unix"
				if props, ok := config.Dataconf[dsName]; ok {
					_, hasPath := props["path"]
					isDerived = !hasPath
				}
				if err := SortBucketsForDataset(dsName, config.Appconf["indexDir"], isDerived, 0, useUnixSort); err != nil {
					log.Printf("Warning: error sorting merge-only dataset %s: %v", dsName, err)
					continue
				}
				// Concatenate
				log.Printf("Concatenating bucket files for %s...", dsName)
				lines, err := ConcatenateBucketsForDataset(dsName, config.Appconf["indexDir"], isDerived, chunkIdx)
				if err != nil {
					log.Printf("Warning: error concatenating merge-only dataset %s: %v", dsName, err)
					continue
				}
				bucketLines += lines
				d.addKVStat(dsName, lines)
				log.Printf("Merged %s: %d lines", dsName, lines)

				// Mark as merged and save state immediately for crash recovery
				if d.datasetState != nil {
					d.datasetState.MarkDatasetsMerged([]string{dsName})
					if saveErr := SaveDatasetState(d.datasetState, config.Appconf["outDir"]); saveErr != nil {
						log.Printf("Warning: failed to save merged state for %s: %v", dsName, saveErr)
					}
				}
			}
		}

		log.Printf("Bucket processing complete (%d lines after deduplication)", bucketLines)
	}

	// Total lines comes from bucket system (all data routes through buckets now)
	totalkv := bucketLines

	log.Println("Data update process completed. Making last merges...")
	// send finish signal to bmerge
	mergeGateCh <- mergeInfo{
		close: true,
		level: 1,
	}

	wgBmerge.Wait()

	// Save dataset state for incremental updates
	// Note: Each dataset is now marked as "merged" immediately after its merge completes
	// (in ConcatenateBucketsParallel and merge-only loop) for crash recovery.
	// This final save ensures any remaining state updates are persisted.
	if d.datasetState != nil {
		// Set total entries (replaces meta.json totalKV)
		d.datasetState.AddTotalKVSize(totalkv)

		// Safety net: mark any remaining "processed" datasets as merged
		// This handles edge cases where individual marking might have been missed
		d.datasetState.MarkAllProcessedAsMerged()

		// Ensure source URLs are set for all datasets
		for _, datasetName := range d.datasets2 {
			sourceURL := ""
			if props, ok := config.Dataconf[datasetName]; ok {
				sourceURL = props["path"]
			}
			if sourceURL != "" {
				if info := d.datasetState.GetDatasetInfo(datasetName); info != nil {
					info.SourceURL = sourceURL
				}
			}
		}

		if err := SaveDatasetState(d.datasetState, config.Appconf["outDir"]); err != nil {
			log.Printf("Warning: failed to save dataset state: %v", err)
		}
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

		// Two-phase state tracking:
		// Phase 1: Mark as "processed" when dataset finishes writing bucket files
		// Phase 2: Mark as "merged" after successful merge (done in Update() after wgBmerge.Wait())
		// This ensures: if merge fails, next run can skip processing and just re-merge
		if info.done && d.datasetState != nil {
			// Skip state update for merge-only datasets - they're already processed
			// and we don't want to overwrite their build_duration from the original run
			if !info.mergeOnly {
				datasetID := ""
				sourceURL := ""
				if props, ok := config.Dataconf[info.dataset]; ok {
					datasetID = props["id"]
					sourceURL = props["path"]
				}
				d.datasetState.MarkDatasetProcessed(info.dataset, datasetID, 0, 0, time.Since(d.start).Seconds())
				if sourceURL != "" {
					if dsInfo := d.datasetState.GetDatasetInfo(info.dataset); dsInfo != nil {
						dsInfo.SourceURL = sourceURL
					}
				}
				if err := SaveDatasetState(d.datasetState, config.Appconf["outDir"]); err != nil {
					log.Printf("Warning: failed to save state for %s: %v", info.dataset, err)
				}
			}
		}

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

func (d *DataUpdate) addKVStat(source string, total uint64) {
	// Only update KV size for source datasets (those explicitly requested via -d)
	// Derived datasets and those receiving bidirectional xrefs are not tracked here
	// Their lines are included in TotalEntries but not in individual dataset stats
	if d.datasetState != nil {
		// Check if this is a source dataset (requested via -d)
		if _, isSourceDataset := d.inDatasets[source]; isSourceDataset {
			d.datasetState.SetKVSize(source, total)
		}
	}
}

// shouldSkipDataset checks if a dataset should be skipped because it was already built
// and the source hasn't changed. Returns true if the dataset should be skipped.
// needsMergeOnly checks if a dataset was already processed but not yet merged
// Such datasets should skip parsing but still be included in the merge phase
func (d *DataUpdate) needsMergeOnly(datasetName string) bool {
	if d.forceRebuild {
		return false // Force mode - always re-process
	}

	if d.datasetState == nil {
		return false
	}

	info := d.datasetState.GetDatasetInfo(datasetName)
	if info == nil {
		return false
	}

	return info.Status == StatusProcessed
}

// getMergeOnlyDatasets returns datasets in datasets2 that only need merge (status=processed)
func (d *DataUpdate) getMergeOnlyDatasets() []string {
	var result []string
	if d.datasetState == nil {
		return result
	}

	for _, dsName := range d.datasets2 {
		info := d.datasetState.GetDatasetInfo(dsName)
		if info != nil && info.Status == StatusProcessed {
			result = append(result, dsName)
		}
	}
	return result
}

func (d *DataUpdate) shouldSkipDataset(datasetName string) bool {
	// Force mode - never skip
	if d.forceRebuild {
		log.Printf("Force rebuild enabled for %s", datasetName)
		return false
	}

	if d.datasetState == nil {
		return false // No state - must build
	}

	// Check if dataset was previously built
	lastBuild := d.datasetState.GetDatasetInfo(datasetName)
	if lastBuild == nil {
		// First build - fetch and store source info for future comparisons
		changeInfo, _ := CheckSourceChanged(datasetName, nil)
		if changeInfo != nil {
			d.storeSourceChangeInfo(datasetName, changeInfo)
		}
		return false // Never built - must build
	}

	// "processed" datasets are handled by needsMergeOnly(), not here
	// They should NOT be skipped, but also should not reach the parser
	if lastBuild.Status == StatusProcessed {
		return false // Don't skip - needsMergeOnly will handle it
	}

	// If status is "processing" (interrupted), must rebuild
	if lastBuild.Status == StatusProcessing {
		log.Printf("Dataset %s was interrupted during processing, will rebuild", datasetName)
		return false
	}

	// If status is empty, dataset was never actually built from source
	// (may have entries from other datasets' contributions but not from own source)
	if lastBuild.Status == "" {
		log.Printf("Dataset %s has no build status (never built from source), will build", datasetName)
		return false
	}

	// Check if source has changed
	changeInfo, err := CheckSourceChanged(datasetName, lastBuild)
	if err != nil {
		// Error checking source - rebuild to be safe
		log.Printf("Error checking source for %s: %v, will rebuild", datasetName, err)
		return false
	}

	// Handle local files specially
	if changeInfo.SourceType == SourceTypeLocal {
		log.Printf("Dataset %s uses local files and was already built at %s. Use --force to rebuild.",
			datasetName, lastBuild.LastBuildTime.Format(time.RFC3339))
		return true
	}

	// If source type is unknown (no config), skip if already built
	// This allows incremental updates to work without requiring source config for every dataset
	if changeInfo.SourceType == SourceTypeUnknown {
		log.Printf("No source config for %s, skipping (already built at %s)",
			datasetName, lastBuild.LastBuildTime.Format(time.RFC3339))
		return true
	}

	if changeInfo.HasChanged {
		log.Printf("Source changed for %s (method=%s)", datasetName, changeInfo.CheckMethod)
		// Store the new source info for later state update
		d.storeSourceChangeInfo(datasetName, changeInfo)
		return false // Source changed - must rebuild
	}

	// Source unchanged - can skip
	log.Printf("Source unchanged for %s, skipping", datasetName)
	return true

}

// storeSourceChangeInfo stores source change info for later state update
func (d *DataUpdate) storeSourceChangeInfo(datasetName string, changeInfo *SourceChangeInfo) {
	if d.datasetState == nil {
		return
	}

	// Update or create the dataset build info with source info
	info := d.datasetState.GetDatasetInfo(datasetName)
	if info == nil {
		info = &DatasetBuildInfo{
			DatasetName: datasetName,
		}
	}

	// Store source information for next comparison
	if changeInfo.SourceURL != "" {
		info.SourceURL = changeInfo.SourceURL
	}
	if !changeInfo.NewDate.IsZero() {
		info.SourceDate = changeInfo.NewDate
	}
	if changeInfo.NewSize > 0 {
		info.SourceSize = changeInfo.NewSize
	}
	if changeInfo.NewETag != "" {
		info.SourceETag = changeInfo.NewETag
	}
	if changeInfo.NewVersion != "" {
		info.SourceVersion = changeInfo.NewVersion
	}

	d.datasetState.UpdateDatasetInfo(info)
}

func (d *DataUpdate) addProp3(key, from string, attr []byte) {

	key = strings.TrimSpace(key)

	if len(key) == 0 || len(from) == 0 || len(attr) <= 2 { // empty attr {}
		return
	}

	// Get dataset name for directory naming
	datasetName := GetDatasetName(from)
	if datasetName == "" {
		datasetName = "unknown"
	}

	kup := strings.ToUpper(key)
	line := kup + tab + from + tab + string(attr) + tab + textStoreID

	// Route through bucket system (always available, auto-creates configs for all datasets)
	if !d.bucketPool.WriteForward(from, datasetName, kup, line) {
		log.Printf("ERROR: addProp3 bucket write failed for dataset %s key %s", datasetName, kup)
	}
}

func (d *DataUpdate) addXref(key string, from string, value string, valueFrom string, isLink bool) {
	// Backward compatible - calls new function with empty evidence
	d.addXrefWithEvidence(key, from, value, valueFrom, isLink, "")
}

// addXrefWithEvidence adds cross-reference with optional evidence code
// evidence: Optional evidence/quality metadata (e.g., "TAS", "IEA" for Reactome)
func (d *DataUpdate) addXrefWithEvidence(key string, from string, value string, valueFrom string, isLink bool, evidence string) {
	// Delegate to the full function with empty relationship
	d.addXrefFull(key, from, value, valueFrom, isLink, evidence, "")
}

// addXrefWithRelationship adds cross-reference with relationship type
// relationship: Type of relationship between entities (e.g., "Related pseudogene" for Entrez gene_group)
func (d *DataUpdate) addXrefWithRelationship(key string, from string, value string, valueFrom string, isLink bool, relationship string) {
	// Delegate to the full function with empty evidence
	d.addXrefFull(key, from, value, valueFrom, isLink, "", relationship)
}

// addXrefFull adds cross-reference with optional evidence code and relationship type
// evidence: Optional evidence/quality metadata (e.g., "TAS", "IEA" for Reactome)
// relationship: Optional relationship type (e.g., "Related pseudogene" for Entrez gene_group)
func (d *DataUpdate) addXrefFull(key string, from string, value string, valueFrom string, isLink bool, evidence string, relationship string) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	evidence = strings.TrimSpace(evidence)
	relationship = strings.TrimSpace(relationship)

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

	// CRITICAL: Validate that target dataset has a valid ID
	// Empty valueFromID causes "Error while converting to int16" panic during generate phase
	if !isLink {
		if config.Dataconf[valueFrom]["id"] == "" {
			log.Printf("ERROR: Dataset '%s' has empty 'id' in config - cannot create xref from %s to %s", valueFrom, key, value)
			return
		}
	}

	// now target datasets check
	if _, ok := d.targetDatasets[valueFrom]; d.hasTargets && !ok && !isLink {
		return
	}

	kup := strings.ToUpper(key)
	vup := strings.ToUpper(value)

	// Storage format: KEY <tab> FROM <tab> VALUE <tab> DATASETID <tab> EVIDENCE <tab> RELATIONSHIP
	// Empty fields are stored as empty strings to maintain column positions
	dataLine := kup + tab + from + tab + vup + tab + config.Dataconf[valueFrom]["id"]
	if evidence != "" || relationship != "" {
		dataLine += tab + evidence
		if relationship != "" {
			dataLine += tab + relationship
		}
	}

	if isLink {
		// Skip keys that exceed LMDB max key size (prevents MDB_BAD_VALSIZE errors)
		if len(kup) > LMDBMaxKeySize {
			// Log first 100 chars of skipped key for debugging
			preview := kup
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			log.Printf("[TextSearch] Skipping long key (%d bytes > %d max) from %s: %s",
				len(kup), LMDBMaxKeySize, valueFrom, preview)
			return
		}
		// Text/keyword links route to textsearch buckets (alphabetic by first letter)
		// Use WriteReverse with from_{source} subdirectory for incremental update cleanup
		// Note: For isLink, 'from' is textLinkID ("0"), actual source is in valueFrom
		d.bucketPool.WriteReverse(TextSearchDatasetID, kup, dataLine, valueFrom)
	} else {
		// Reverse mapping also includes evidence and relationship
		valueFromID := config.Dataconf[valueFrom]["id"]
		reverseDataLine := vup + tab + valueFromID + tab + kup + tab + from
		if evidence != "" || relationship != "" {
			reverseDataLine += tab + evidence
			if relationship != "" {
				reverseDataLine += tab + relationship
			}
		}

		// Get source dataset name for directory naming
		sourceDatasetName := GetDatasetName(from)
		if sourceDatasetName == "" {
			sourceDatasetName = "unknown"
		}

		// Route through bucket system (always available, auto-creates configs for all datasets)
		// Forward xref → {source}/forward/bucket_*.txt
		// Reverse xref → {target}/from_{source}/bucket_*.txt
		if !d.bucketPool.WriteForward(from, sourceDatasetName, kup, dataLine) {
			log.Printf("ERROR: addXrefFull forward bucket write failed for dataset %s key %s", sourceDatasetName, kup)
		}
		if !d.bucketPool.WriteReverse(valueFromID, vup, reverseDataLine, sourceDatasetName) {
			log.Printf("ERROR: addXrefFull reverse bucket write failed for target %s key %s", valueFrom, vup)
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

	// Get source dataset name for directory naming
	sourceDatasetName := GetDatasetName(from)
	if sourceDatasetName == "" {
		sourceDatasetName = "unknown"
	}

	// Route through bucket system (always available, auto-creates configs for all datasets)
	// Link datasets only have forward xrefs (no reverse mapping)
	// Try source dataset first, then target dataset as fallback
	if !d.bucketPool.WriteForward(from, sourceDatasetName, kup, line) {
		targetDatasetName := GetDatasetName(valueFromID)
		if targetDatasetName == "" {
			targetDatasetName = "unknown"
		}
		if !d.bucketPool.WriteForward(valueFromID, targetDatasetName, vup, line) {
			log.Printf("ERROR: addXref2 bucket write failed for both source %s and target %s", sourceDatasetName, targetDatasetName)
		}
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

	// Get dataset name for directory naming
	bucketDatasetName := GetDatasetName(bucketDatasetID)
	if bucketDatasetName == "" {
		bucketDatasetName = "unknown"
	}

	// Route through bucket pool using the specified bucket dataset
	// This allows link datasets (taxchild, taxparent) to use parent dataset's buckets
	d.bucketPool.WriteForward(bucketDatasetID, bucketDatasetName, kup, line)
	_ = vup // Suppress unused warning (not needed for link datasets)
}

// addXrefBucketed routes xrefs through bucket system for optimized datasets
// All datasets are routed through the bucket system with alphabetic fallback
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

	// Get source dataset name for directory naming
	sourceDatasetName := GetDatasetName(from)
	if sourceDatasetName == "" {
		sourceDatasetName = "unknown"
	}

	// Route through source-tagged bucket directories
	// Forward xref → {source}/forward/bucket_*.txt
	d.bucketPool.WriteForward(from, sourceDatasetName, kup, forwardLine)
	// Reverse xref → {target}/from_{source}/bucket_*.txt
	d.bucketPool.WriteReverse(valueFromID, vup, reverseLine, sourceDatasetName)

	// Text search link (routes through textsearch buckets)
	// Skip keys that exceed LMDB max key size (prevents MDB_BAD_VALSIZE errors)
	if isLink {
		if len(kup) > LMDBMaxKeySize {
			// Log first 100 chars of skipped key for debugging
			preview := kup
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			log.Printf("[TextSearch] Skipping long key (%d bytes > %d max) from %s: %s",
				len(kup), LMDBMaxKeySize, from, preview)
		} else {
			textLine := kup + tab + textLinkID + tab + vup + tab + from
			// Use WriteReverse with from_{source} subdirectory for incremental update cleanup
			d.bucketPool.WriteReverse(TextSearchDatasetID, kup, textLine, sourceDatasetName)
		}
	}
}

// addProp3Bucketed routes properties through bucket system
// Uses source-tagged directory structure for incremental updates
func (d *DataUpdate) addProp3Bucketed(key, from string, attr []byte) {

	key = strings.TrimSpace(key)

	if len(key) == 0 || len(from) == 0 || len(attr) <= 2 { // empty attr {}
		return
	}

	// Get dataset name for directory naming
	datasetName := GetDatasetName(from)
	if datasetName == "" {
		datasetName = "unknown"
	}

	kup := strings.ToUpper(key)
	line := kup + tab + from + tab + string(attr) + tab + textStoreID
	d.bucketPool.WriteForward(from, datasetName, kup, line)
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
			resolvedDatasetName, ok := config.DataconfIDIntToString[entry.Dataset]
			if !ok {
				continue
			}

			// Determine the target dataset name for the xref
			// If keywordDataset was specified, use it; otherwise use the resolved name
			xrefTargetDataset := keywordDataset
			if xrefTargetDataset == "" {
				xrefTargetDataset = resolvedDatasetName
			}

			// Create xref FROM the source (targetValue/targetDataset) TO the looked-up entry
			// This ensures forward xrefs go to the processing dataset's forward/ directory
			// e.g., dbsnp processing creates: RS123 → ENSG123
			//   Forward: dbsnp/forward/ (RS123 → ENSG123)
			//   Reverse: ensembl/from_dbsnp/ (ENSG123 → RS123)
			d.addXref(targetValue, from, entry.Identifier, xrefTargetDataset, isLink)
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
