package update

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	util "../util"
	"github.com/jlaffaye/ftp"
	"github.com/vbauerster/mpb"
)

const textLinkID = "0"
const textStoreID = "-1"

const propSep = "`"

var fileBufSize = 65536
var channelOverflowCap = 100000

var chunkLen int
var chunkIdx = "df"

var mutex = &sync.Mutex{}

var dataconf map[string]map[string]string
var appconf map[string]string

type DataUpdate struct {
	wg                     *sync.WaitGroup
	datasets               []string
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
	totalParsedEntry       uint64
	stats                  map[string]interface{}
	targetDatasets         map[string]bool
	hasTargets             bool
	channelOverflowCap     int
	selectedEnsemblSpecies []string
	ensemblSpecies         map[string]bool
	ensemblRelease         string
	progChan               chan *progressInfo
	progInterval           int64
}

type progressInfo struct {
	dataset         string
	currentKBPerSec int64
	done            bool
}

func NewDataUpdate(datasets, targetDatasets, ensemblSpecies []string, dconf map[string]map[string]string, aconf map[string]string, chkIdx string) *DataUpdate {

	dataconf = dconf
	appconf = aconf
	chunkIdx = chkIdx

	targetDatasetMap := map[string]bool{}

	if len(targetDatasets) > 0 {

		for _, dt := range targetDatasets {
			if _, ok := dataconf[dt]; ok {
				targetDatasetMap[dt] = true
				//now aliases if exist
				if _, ok := dataconf[dt]["aliases"]; ok {
					aliases := strings.Split(dataconf[dt]["aliases"], ",")
					for _, ali := range aliases {
						targetDatasetMap[ali] = true
					}
				}

			}

		}
	}

	loc := appconf["uniprot_ftp"]
	uniprotftpAddr := appconf["uniprot_ftp_"+loc]
	uniprotftpPath := appconf["uniprot_ftp_"+loc+"_path"]

	ebiftp := appconf["ebi_ftp"]
	ebiftppath := appconf["ebi_ftp_path"]

	// chunk buffer size
	var progInterval = 3
	if _, ok := appconf["progressInterval"]; ok {
		var err error
		progInterval, err = strconv.Atoi(appconf["progressInterval"])
		if err != nil {
			panic("Invalid progressInterval definition")
		}
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
		selectedEnsemblSpecies: ensemblSpecies,
		progInterval:           int64(progInterval),
		progChan:               make(chan *progressInfo, 1000),
		start:                  time.Now(),
		datasets:               datasets,
	}

}

func (d *DataUpdate) Update() (uint64, uint64) {

	var err error
	var wg sync.WaitGroup
	var e = make(chan string, channelOverflowCap)
	//var mergeGateCh = make(chan mergeInfo)
	var mergeGateCh = make(chan mergeInfo, 10000)

	d.wg = &wg
	d.kvdatachan = &e

	// chunk buffer size
	chunkLen = 10000000
	if _, ok := appconf["kvgenChunkSize"]; ok {
		var err error
		chunkLen, err = strconv.Atoi(appconf["kvgenChunkSize"])
		if err != nil {
			panic("Invalid kvgenChunkSize definition")
		}
	}

	// kv generator size
	kvGenCount := 1
	if _, ok := appconf["kvgenCount"]; ok {
		kvGenCount, err = strconv.Atoi(appconf["kvgenCount"])
		if err != nil {
			panic("Invalid kvgenCount definition")
		}
	}

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

	mydataIndex := -1
	for index, data := range d.datasets {
		switch data {
		case "uniprot_reviewed":
			d.wg.Add(1)
			u := uniprot{source: "uniprot_reviewed", d: d}
			go u.update()
			break
		case "ensembl", "ensembl_bacteria", "ensembl_fungi", "ensembl_metazoa", "ensembl_plants", "ensembl_protists":
			d.wg.Add(1)
			e := ensembl{source: data, d: d}
			go e.update()
			break
		case "taxonomy":
			d.wg.Add(1)
			t := taxonomy{source: "taxonomy", d: d}
			go t.update()
			break
		case "hgnc":
			d.wg.Add(1)
			h := hgnc{source: "hgnc", d: d}
			go h.update()
			break
		case "chebi":
			d.wg.Add(1)
			c := chebi{source: "chebi", d: d}
			go c.update()
			break
		case "interpro":
			d.wg.Add(1)
			i := interpro{source: "interpro", d: d}
			go i.update()
			break
		case "uniprot_unreviewed":
			d.wg.Add(1)
			u := uniprot{source: "uniprot_unreviewed", d: d}
			go u.update()
			break
		case "uniparc":
			d.wg.Add(1)
			u := uniparc{source: "uniparc", d: d}
			go u.update()
			break
		case "uniref50":
			d.wg.Add(1)
			u := uniref{source: "uniref50", d: d}
			go u.update()
			break
		case "uniref90":
			d.wg.Add(1)
			u := uniref{source: "uniref90", d: d}
			go u.update()
			break
		case "uniref100":
			u := uniref{source: "uniref100", d: d}
			go u.update()
			break
		case "hmdb":
			d.wg.Add(1)
			h := hmdb{source: "hmdb", d: d}
			go h.update()
			break
		case "go":
			d.wg.Add(1)
			g := gontology{source: "GO", d: d}
			go g.update()
			break
		case "my_data":
			if dataconf["my_data"]["active"] == "true" {
				d.wg.Add(1)
				u := uniprot{source: "my_data", d: d}
				go u.update()
			} else {
				mydataIndex = index
			}
			break
		case "literature_mappings":
			d.wg.Add(1)
			l := literature{source: "literature_mappings", d: d}
			go l.update()
			break
		default:
			panic("ERROR Unrecognized dataset ->" + data)
		}
	}

	if mydataIndex != -1 { // remove my_data
		d.datasets = append(d.datasets[:mydataIndex], d.datasets[mydataIndex+1:]...)
	}

	sort.Strings(d.datasets)

	log.Println("Update RUNNING...datasets->", d.datasets)

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

	ioutil.WriteFile(filepath.FromSlash(appconf["indexDir"]+"/"+chunkIdx+".meta.json"), data, 0700)

	log.Println("All done.")

	return d.totalParsedEntry, totalkv

}

func (d *DataUpdate) showProgres() {

	latestProg := map[string]progressInfo{}
	var result strings.Builder

	for info := range d.progChan {

		alldone := false
		latestProg[info.dataset] = *info
		if len(d.datasets) == len(latestProg) {
			alldone = true
			elapsed := int64(time.Since(d.start).Seconds())
			result.Reset()
			result.WriteString("\r")
			result.WriteString("Processing...Elapsed ")
			result.WriteString(strconv.FormatInt(elapsed, 10))
			result.WriteString("s")
			for i, ds := range d.datasets {

				result.WriteString(" ")
				if len(d.datasets) > 5 {
					result.WriteString("d")
					result.WriteString(strconv.FormatInt(int64(i), 10))
				} else {
					result.WriteString(latestProg[ds].dataset)
				}
				result.WriteString(":")
				if latestProg[ds].done {
					result.WriteString("DONE")
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

func (d *DataUpdate) ftpClient(ftpAddr string) *ftp.ServerConn {

	client, err := ftp.Dial(ftpAddr)
	if err != nil {
		panic("Error in ftp connection:" + err.Error())
	}

	if err := client.Login("anonymous", ""); err != nil {
		panic("Error in ftp login with anonymous:" + err.Error())
	}
	return client
}

func (d *DataUpdate) addEntryStat(source string, total uint64) {

	var entrysize = map[string]uint64{}
	entrysize["entrySize"] = total
	mutex.Lock()
	d.stats[source] = entrysize
	mutex.Unlock()

}

func (d *DataUpdate) getDataReaderNew(datatype string, ftpAddr string, ftpPath string, filePath string) (*bufio.Reader, *gzip.Reader, *ftp.Response, *os.File, string, int64) {

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	var file *os.File
	var err error
	var from string
	var fileSize int64

	from = dataconf[datatype]["id"]

	if _, ok := dataconf[datatype]["useLocalFile"]; ok && dataconf[datatype]["useLocalFile"] == "yes" {

		file, err = os.Open(filepath.FromSlash(filePath))
		check(err)

		fileStat, err := file.Stat()
		check(err)
		fileSize = fileStat.Size()

		if filepath.Ext(file.Name()) == ".gz" {
			gz, err := gzip.NewReader(file)
			check(err)
			br := bufio.NewReaderSize(gz, fileBufSize)
			return br, gz, nil, file, from, fileSize
		}

		br := bufio.NewReaderSize(file, fileBufSize)
		return br, nil, nil, file, from, fileSize

	}

	// with ftp
	client = d.ftpClient(ftpAddr)
	path := ftpPath + filePath

	fileSize, err = client.FileSize(path)

	check(err)
	ftpfile, err = client.Retr(path)
	check(err)

	var br *bufio.Reader
	var gz *gzip.Reader

	if filepath.Ext(path) == ".gz" {
		gz, err = gzip.NewReader(ftpfile)
		check(err)
		br = bufio.NewReaderSize(gz, fileBufSize)

	} else {
		br = bufio.NewReaderSize(ftpfile, fileBufSize)
	}

	return br, gz, ftpfile, nil, from, fileSize

}

func (d *DataUpdate) addProp(key string, from string, value string) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	value = strings.Replace(value, tab, "", -1)
	value = strings.Replace(value, newline, "", -1)

	if len(key) == 0 || len(value) == 0 || len(from) == 0 || len(value) > 500 {
		return
	}

	kup := strings.ToUpper(key)
	*d.kvdatachan <- kup + tab + from + tab + value + tab + textStoreID

}

func (d *DataUpdate) addXref(key string, from string, value string, valueFrom string, isLink bool) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if len(key) == 0 || len(value) == 0 || len(from) == 0 {
		return
	}

	if _, ok := dataconf[valueFrom]; !isLink && !ok {
		//if appconf["debug"] == "y" {
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
	*d.kvdatachan <- kup + tab + from + tab + vup + tab + dataconf[valueFrom]["id"]

	if !isLink {
		*d.kvdatachan <- vup + tab + dataconf[valueFrom]["id"] + tab + kup + tab + from
	}

}

func check(err error) {

	if err != nil {
		fmt.Println("Error: ", err)
		panic(err)
	}

}
