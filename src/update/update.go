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
}

// Close lookup database
func (d *DataUpdate) closeLookupDB() {
	if d.hasLookupDB {
		d.lookupEnv.Close()
	}
}

// Lookup identifier in biobtree database and return results
func (d *DataUpdate) lookup(identifier string) (*pbuf.Result, error) {
	if !d.hasLookupDB {
		return nil, fmt.Errorf("lookup database not available")
	}

	// Lookup is case-insensitive (convert to uppercase like service does)
	identifier = strings.ToUpper(identifier)

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

	if len(v) == 0 {
		return nil, nil
	}

	r := pbuf.Result{}
	err = proto.Unmarshal(v, &r)
	return &r, err
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
				u := uniprot{source: data, d: d}
				d.datasets2 = append(d.datasets2, data)
				go u.update(nil)
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

	for i := 0; i < len(kvgens); i++ {
		kvgens[i].wg.Add(1)
	}
	close(e)
	wg.Wait()

	var totalkv uint64

	for i := 0; i < len(kvgens); i++ {
		totalkv = totalkv + kvgens[i].totalkv
	}

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
	*d.kvdatachan <- kup + tab + from + tab + string(attr) + tab + textStoreID

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
	*d.kvdatachan <- dataLine

	if !isLink {
		// Reverse mapping also includes evidence
		reverseDataLine := vup + tab + config.Dataconf[valueFrom]["id"] + tab + kup + tab + from
		if evidence != "" {
			reverseDataLine += tab + evidence
		}
		*d.kvdatachan <- reverseDataLine
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
	if _, ok := d.targetDatasets[valueFrom]; d.hasTargets && !ok {
		return
	}

	kup := strings.ToUpper(key)
	vup := strings.ToUpper(value)
	*d.kvdatachan <- kup + tab + from + tab + vup + tab + config.Dataconf[valueFrom]["id"]

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
