package update

import (
	"biobtree/configs"
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

func (d *DataUpdate) Update() (uint64, uint64) {

	log.Println("Update running please wait...")

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

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

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
	*d.kvdatachan <- kup + tab + from + tab + vup + tab + config.Dataconf[valueFrom]["id"]

	if !isLink {
		*d.kvdatachan <- vup + tab + config.Dataconf[valueFrom]["id"] + tab + kup + tab + from
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

func (d *DataUpdate) selectEnsembls() map[string]ensembl {

	selectedEnsembls := []string{}
	for _, src := range allEnsembls { // this is to check command line ensembl datasets if yes genome selection will be only within those ones otherwise all
		if _, ok := d.inDatasets[src]; ok {
			selectedEnsembls = append(selectedEnsembls, src)
		}
	}

	if len(selectedEnsembls) == 0 {
		selectedEnsembls = allEnsembls
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
