package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krolaw/zipstream"

	"./parser"
	"github.com/jlaffaye/ftp"
	"github.com/tidwall/gjson"
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
)

const textLinkID = "0"

var mutex = &sync.Mutex{}

type dataUpdate struct {
	wg                 *sync.WaitGroup
	kvdatachan         *chan string
	invalidXrefs       HashMaper
	sampleXrefs        HashMaper
	sampleCount        int
	sampleWritten      bool
	uniprotFtp         string
	uniprotFtpPath     string
	ebiFtp             string
	ebiFtpPath         string
	uniprotEntryCounts map[string]uint64
	p                  *mpb.Progress
	totalParsedEntry   uint64
	stats              map[string]interface{}
	targetDatasets     map[string]bool
	hasTargets         bool
	channelOverflowCap int
}

func (d *dataUpdate) ftpClient(ftpAddr string) *ftp.ServerConn {

	client, err := ftp.Dial(ftpAddr)
	if err != nil {
		panic("Error in ftp connection:" + err.Error())
	}

	if err := client.Login("anonymous", ""); err != nil {
		panic("Error in ftp login with anonymous:" + err.Error())
	}
	return client
}

func (d *dataUpdate) newResultChannel() *chan parser.XMLEntry {

	var resultChannel = make(chan parser.XMLEntry, d.channelOverflowCap)
	return &resultChannel

}

func (d *dataUpdate) addEntryStat(source string, total uint64) {

	var entrysize = map[string]uint64{}
	entrysize["entrySize"] = total
	mutex.Lock()
	d.stats[source] = entrysize
	mutex.Unlock()

}

func (d *dataUpdate) createProgresIfMissing() {

	if d.p == nil {

		defaultRate := 2 * time.Second
		if _, ok := appconf["progressRefreshRate"]; ok {
			rate, err := strconv.Atoi(appconf["progressRefreshRate"])
			if err != nil {
				panic("Invalid refresh rate definition")
			}
			defaultRate = time.Duration(rate) * time.Second
		}

		d.p = mpb.New(mpb.WithWaitGroup(d.wg), mpb.WithRefreshRate(defaultRate))

	}

}

func (d *dataUpdate) addProgress(size int64, name string) *mpb.Bar {

	d.createProgresIfMissing()

	var bar *mpb.Bar

	bar = d.p.AddBar(size,
		mpb.BarClearOnComplete(),
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			// simple name decorator
			decor.Name(name),
			// decor.DSyncWidth bit enables column width synchronization
			decor.Percentage(decor.WCSyncSpace),
		),
		mpb.AppendDecorators(

			decor.OnComplete(
				decor.Elapsed(decor.ET_STYLE_GO), "done",
			),
		),
	)

	return bar

}

func (d *dataUpdate) setUniprotMeta() {

	d.uniprotEntryCounts = map[string]uint64{}

	client := d.ftpClient(d.uniprotFtp)

	relNotesFile := d.uniprotFtpPath + "/current_release/relnotes.txt"

	res2, err := client.Retr(relNotesFile)
	check(err)

	data, err := ioutil.ReadAll(res2)
	if err != nil {

		d.uniprotEntryCounts["uniprot_reviewed"] = 0
		d.uniprotEntryCounts["uniprot_unreviewed"] = 0
		d.uniprotEntryCounts["uniref100"] = 0
		d.uniprotEntryCounts["uniref90"] = 0
		d.uniprotEntryCounts["uniref50"] = 0
		d.uniprotEntryCounts["uniparc"] = 0

	}

	relnotes := string(data)
	var rel []string
	rel = strings.Split(relnotes, "entries")

	for i := range rel {
		rel[i] = strings.TrimSpace(rel[i])
	}

	var entrycounts [6]uint64
	for i := 1; i < 7; i++ {
		data := strings.Split(rel[i], " ")
		if len(data) > 0 {
			dataEntr := strings.TrimSpace(data[len(data)-1])
			u, err := strconv.ParseUint(strings.Replace(dataEntr, ",", "", -1), 10, 64)
			if err != nil {
				d.uniprotEntryCounts["uniprot_reviewed"] = 0
				d.uniprotEntryCounts["uniprot_unreviewed"] = 0
				d.uniprotEntryCounts["uniref100"] = 0
				d.uniprotEntryCounts["uniref90"] = 0
				d.uniprotEntryCounts["uniref50"] = 0
				d.uniprotEntryCounts["uniparc"] = 0
				return
			}
			entrycounts[i-1] = u
		}
	}

	d.uniprotEntryCounts["uniprot_reviewed"] = entrycounts[0]
	d.uniprotEntryCounts["uniprot_unreviewed"] = entrycounts[1]
	d.uniprotEntryCounts["uniref100"] = entrycounts[2]
	d.uniprotEntryCounts["uniref90"] = entrycounts[3]
	d.uniprotEntryCounts["uniref50"] = entrycounts[4]
	d.uniprotEntryCounts["uniparc"] = entrycounts[5]
	res2.Close()
}

func (d *dataUpdate) getDataReader(datatype string, ftpAddr string, ftpPath string, filePath string) (*bufio.Reader, *gzip.Reader, *ftp.Response, *os.File, *mpb.Bar, string) {

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	var file *os.File
	var err error
	var from string
	var entrysize uint64

	from = dataconf[datatype]["id"]

	if _, ok := dataconf[datatype]["useLocalFile"]; ok && dataconf[datatype]["useLocalFile"] == "yes" {

		file, err = os.Open(filepath.FromSlash(filePath))
		check(err)

		if filepath.Ext(file.Name()) == ".gz" {
			gz, err := gzip.NewReader(file)
			check(err)
			br := bufio.NewReaderSize(gz, fileBufSize)
			return br, gz, nil, file, nil, from
		}

		br := bufio.NewReaderSize(file, fileBufSize)
		return br, nil, nil, file, nil, from

	}

	// with ftp
	client = d.ftpClient(ftpAddr)
	entrysize = d.uniprotEntryCounts[datatype]
	path := ftpPath + filePath
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

	var bar *mpb.Bar

	if entrysize > 0 {
		bar = d.addProgress(int64(entrysize), datatype)
	}

	return br, gz, ftpfile, nil, bar, from

}

func (d *dataUpdate) addXref(key string, from string, value string, valueFrom string, isLink bool) {

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

func (d *dataUpdate) updateUniprot(source string) {

	var resultChannel = d.newResultChannel()

	br, gz, ftpFile, localFile, bar, fr := d.getDataReader(source, d.uniprotFtp, d.uniprotFtpPath, dataconf[source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := parser.XMLParser{
		R:             br,
		LoopTag:       "entry",
		OutChannel:    resultChannel,
		SkipTags:      []string{"comment", "gene", "protein", "feature", "sequence"},
		FinishMessage: source,
		ProgBar:       bar,
	}

	go p.Parse()

	var total uint64

	var r parser.XMLEntry
	var v, x, z parser.XMLElement
	var ok bool
	var entryid string

	for r = range *resultChannel {

		entryid = r.Elements["name"][0].InnerText

		for _, v = range r.Elements["accession"] {
			d.addXref(v.InnerText, textLinkID, entryid, source, true)
		}

		for _, v = range r.Elements["dbReference"] {

			d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["type"], false)

			if _, ok := v.Childs["property"]; ok {
				for _, z := range v.Childs["property"] {
					d.addXref(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], z.Attrs["value"], z.Attrs["type"], false)
				}
			}

		}
		for _, v = range r.Elements["organism"] {
			if _, ok = v.Childs["dbReference"]; ok {
				for _, z = range v.Childs["dbReference"] {

					d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)

					if _, ok := z.Childs["property"]; ok {
						for _, x := range z.Childs["property"] {
							d.addXref(z.Attrs["id"], dataconf[z.Attrs["type"]]["id"], x.Attrs["value"], x.Attrs["type"], false)
						}
					}

				}
			}
		}

		for _, v = range r.Elements["reference"] {
			if _, ok = v.Childs["citation"]; ok {
				for _, z = range v.Childs["citation"] {
					if _, ok = z.Childs["dbReference"]; ok {
						for _, x = range z.Childs["dbReference"] {

							d.addXref(entryid, fr, x.Attrs["id"], x.Attrs["type"], false)
							/**
							if _, ok := x.Childs["property"]; ok {
								for _, t := range x.Childs["property"] {
									d.addXref(entryid, fr, t.Attrs["value"], t.Attrs["type"], false)
								}
							}
							*/

						}
					}
				}
			}
		}

		total++

	}
	atomic.AddUint64(&d.totalParsedEntry, total)
	d.addEntryStat(source, total)
}

func (d *dataUpdate) updateUniref(unireftype string) {

	var resultChannel = d.newResultChannel()

	br, gz, ftpFile, localFile, bar, fr := d.getDataReader(unireftype, d.uniprotFtp, d.uniprotFtpPath, dataconf[unireftype]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}

	defer gz.Close()
	defer d.wg.Done()

	p := parser.XMLParser{
		R:             br,
		LoopTag:       "entry",
		OutChannel:    resultChannel,
		FinishMessage: unireftype,
		ProgBar:       bar,
	}

	go p.Parse()

	// for uniref we are only interested in two refernces uniprot and uniparc
	validRefs := map[string]bool{}
	validRefs["UniParc ID"] = true
	validRefs["UniProtKB ID"] = true

	var total uint64
	var r parser.XMLEntry
	var v, z parser.XMLElement
	var ok bool
	var entryid string

	for r = range *resultChannel {
		// id
		entryid = r.Attrs["id"]

		// root property
		/**
		for _, v = range r.Elements["property"] {
			d.addXref(entryid, fr, v.Attrs["value"], v.Attrs["type"], false)
		}
		**/

		// representativeMember--> dbreference
		for _, v = range r.Elements["representativeMember"] {
			if _, ok = v.Childs["dbReference"]; ok {
				for _, z = range v.Childs["dbReference"] {

					if _, ok = validRefs[z.Attrs["type"]]; ok {
						d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
					}

					/**
					if _, ok := z.Childs["property"]; ok {
						for _, x := range z.Childs["property"] {
							if _, ok = validRefs[x.Attrs["type"]]; ok {
								d.addXref(entryid, fr, x.Attrs["value"], x.Attrs["type"], false)
							}
						}
					}
					**/
				}
			}
		}
		// member --> dbreference
		for _, v = range r.Elements["member"] {
			if _, ok = v.Childs["dbReference"]; ok {
				for _, z = range v.Childs["dbReference"] {

					if _, ok = validRefs[z.Attrs["type"]]; ok {
						d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
					}
					/**
					if _, ok := z.Childs["property"]; ok {
						for _, x := range z.Childs["property"] {
							if _, ok = validRefs[x.Attrs["type"]]; ok {
								d.addXref(entryid, fr, x.Attrs["value"], x.Attrs["type"], false)
							}
						}
					}
					**/
				}
			}
		}

		total++
	}

	atomic.AddUint64(&d.totalParsedEntry, total)
	d.addEntryStat(unireftype, total)

}

func (d *dataUpdate) updateUniparc() {

	var resultChannel = d.newResultChannel()
	br, gz, ftpFile, localFile, bar, fr := d.getDataReader("uniparc", d.uniprotFtp, d.uniprotFtpPath, dataconf["uniparc"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := parser.XMLParser{
		R:             br,
		LoopTag:       "entry",
		OutChannel:    resultChannel,
		SkipTags:      []string{"sequence"},
		FinishMessage: "uniparc",
		ProgBar:       bar,
	}

	go p.Parse()

	// we are excluding uniprot subreference because they are already coming from uniprot. this may be optional
	propExclusionsRefs := map[string]bool{}
	propExclusionsRefs["UniProtKB/Swiss-Prot"] = true
	propExclusionsRefs["UniProtKB/TrEMBL"] = true

	var total uint64
	var r parser.XMLEntry
	var v parser.XMLElement
	var ok bool
	var entryid string

	for r = range *resultChannel {

		// id
		entryid = r.Elements["accession"][0].InnerText

		//dbreference
		for _, v = range r.Elements["dbReference"] {

			d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["type"], false)

			if _, ok = propExclusionsRefs[v.Attrs["type"]]; !ok {
				if _, ok := v.Childs["property"]; ok {
					for _, z := range v.Childs["property"] {
						d.addXref(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], z.Attrs["value"], z.Attrs["type"], false)
					}
				}
			}

		}
		// signatureSequenceMatch
		/**
		for _, v = range r.Elements["signatureSequenceMatch"] {

			d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["database"], false)

			if _, ok = v.Childs["ipr"]; ok {
				for _, z = range v.Childs["ipr"] {
					d.addXref(entryid, fr, z.Attrs["id"], "INTERPRO", false)
				}
			}
		}
		*/

		total++
	}

	atomic.AddUint64(&d.totalParsedEntry, total)
	d.addEntryStat("uniparc", total)

}

func (d *dataUpdate) updateTaxonomy() {

	var resultChannel = d.newResultChannel()

	br, gz, ftpFile, localFile, _, fr := d.getDataReader("taxonomy", d.ebiFtp, d.ebiFtpPath, dataconf["taxonomy"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := parser.XMLParser{
		R:             br,
		LoopTag:       "taxon",
		OutChannel:    resultChannel,
		SkipTags:      []string{"lineage"},
		FinishMessage: "taxonomy",
	}

	go p.Parse()

	var total uint64

	var r parser.XMLEntry
	var v, z parser.XMLElement
	var ok bool
	var entryid string

	for r = range *resultChannel {

		// id
		entryid = r.Attrs["taxId"]

		d.addXref(r.Attrs["scientificName"], textLinkID, entryid, "taxonomy", true)

		//dbreference
		for _, v = range r.Elements["children"] {
			if _, ok = v.Childs["taxon"]; ok {
				for _, z = range v.Childs["taxon"] {
					d.addXref(entryid, fr, z.Attrs["taxId"], "taxonomy", false)
				}
			}
		}

		total++
	}

	atomic.AddUint64(&d.totalParsedEntry, total)

	d.addEntryStat("taxonomy", total)

}

// note json parser is not efficent compare to xml_parser. it is consuming a lot ram but since hgnc is not big it can be ignored now.
func (d *dataUpdate) updateHgnc() {

	br, _, ftpFile, localFile, _, fr := d.getDataReader("hgnc", d.ebiFtp, d.ebiFtpPath, dataconf["hgnc"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer d.wg.Done()

	data, err := ioutil.ReadAll(br)
	check(err)

	result := gjson.Get(string(data), "response.docs")
	var total uint64
	result.ForEach(func(key, value gjson.Result) bool {
		//println(value.String())

		entryid := value.Get("hgnc_id").String()

		/**
		if entryid == "HGNC:12009" {
			fmt.Println("girildi..")
		}
		**/

		if len(entryid) > 0 {

			j := value.Get("cosmic").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "COSMIC", false)
				}
			}

			j = value.Get("omim_id").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "MIM", false)
				}
			}

			j = value.Get("ena").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "EMBL", false)
				}
			}

			j = value.Get("ccds_id").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "CCDS", false)
				}
			}

			j = value.Get("enzyme_id").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "Intenz", false)
				}
			}

			j = value.Get("vega_id").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "VEGA", false)
				}
			}
			j = value.Get("ensembl_gene_id").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "Ensembl", false)
				}
			}

			j = value.Get("pubmed_id").Array()

			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "PubMed", false)
				}
			}

			j = value.Get("refseq_accession").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "RefSeq", false)
				}
			}

			j = value.Get("uniprot_ids").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(entryid, fr, v.String(), "UniProtKB", false)
				}
			}

			/**
			if entryid == "HGNC:13816" {
				fmt.Println("girildi..")
			}**/

			/**
			j = value.Get("alias_symbol").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addLink(v.String(), entryid, fr)
				}
			}
			**/

			j = value.Get("symbol").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(v.String(), textLinkID, entryid, "hgnc", true)
				}
			}
			j = value.Get("name").Array()
			if len(j) > 0 {
				for _, v := range j {
					d.addXref(v.String(), textLinkID, entryid, "hgnc", true)
				}
			}
		}
		total++

		return true // keep iterating
	})

	atomic.AddUint64(&d.totalParsedEntry, total)

	d.addEntryStat("hgnc", total)

}

func (d *dataUpdate) updateChebi() {

	defer d.wg.Done()

	chebiPath := dataconf["chebi"]["path"]
	chebiFiles := strings.Split(dataconf["chebi"]["files"], ",")

	//xreftypes := map[string]bool{}

	for _, name := range chebiFiles {
		br, _, ftpFile, localFile, _, fr := d.getDataReader("chebi", d.ebiFtp, d.ebiFtpPath, chebiPath+name)

		r := csv.NewReader(br)
		r.Comma = '	'

		for {
			record, err := r.Read()
			if err == io.EOF {
				break
			}
			if record[3] == "TYPE" {
				continue
			}
			if err != nil {
				log.Fatal(err)
			}

			entryid := "CHEBI:" + record[1]

			if _, ok := dataconf[record[3]]; !ok {
				continue
			}

			d.addXref(entryid, fr, record[4], record[3], false)

		}

		/*
			var ind int64
			ind = 191
			var xbuf strings.Builder

			for k := range xreftypes {
				var a string
				if strings.HasSuffix(k, "Registry Number") {
					a = "\"" + strings.TrimSpace(strings.TrimRight(k, "Registry Number")) + "\": {\"id\": \"" + strconv.FormatInt(ind, 10) + "\",\"name\": \"\",\"url\": \"\",\"aliases\": \"" + k + "\"}"
				} else if strings.HasSuffix(k, "accession") {
					a = "\"" + strings.TrimSpace(strings.TrimRight(k, "accession")) + "\": {\"id\": \"" + strconv.FormatInt(ind, 10) + "\",\"name\": \"\",\"url\": \"\",\"aliases\": \"" + k + "\"}"
				} else if strings.HasSuffix(k, "citation") {
					a = "\"" + strings.TrimSpace(strings.TrimRight(k, "citiation")) + "\": {\"id\": \"" + strconv.FormatInt(ind, 10) + "\",\"name\": \"\",\"url\": \"\",\"aliases\": \"" + k + "\"}"
				}

				xbuf.WriteString(a)
				xbuf.WriteString(",")
				ind++
			}
			fmt.Println(xbuf.String())
		*/

		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}

	}

}

func (d *dataUpdate) updateInterpro() {

	var resultChannel = d.newResultChannel()
	br, gz, ftpFile, localFile, bar, fr := d.getDataReader("interpro", d.ebiFtp, d.ebiFtpPath, dataconf["interpro"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := parser.XMLParser{
		R:             br,
		LoopTag:       "interpro",
		OutChannel:    resultChannel,
		SkipTags:      []string{"abstract"},
		FinishMessage: "interpro",
		ProgBar:       bar,
	}

	go p.Parse()

	var total uint64
	var r parser.XMLEntry
	var ok bool
	var entryid string

	for r = range *resultChannel {
		// id
		entryid = r.Attrs["id"]

		for _, v := range r.Elements["pub_list"] {
			if _, ok = v.Childs["publication"]; ok {
				for _, z := range v.Childs["publication"] {
					if _, ok = z.Childs["db_xref"]; ok {
						for _, x := range z.Childs["db_xref"] {
							d.addXref(entryid, fr, x.Attrs["dbkey"], x.Attrs["db"], false)
						}
					}
				}
			}
		}

		for _, v := range r.Elements["found_in"] {
			if _, ok = v.Childs["rel_ref"]; ok {
				for _, z := range v.Childs["rel_ref"] {
					d.addXref(entryid, fr, z.Attrs["ipr_ref"], "INTERPRO", false)
				}
			}
		}

		for _, v := range r.Elements["member_list"] {
			if _, ok = v.Childs["db_xref"]; ok {
				for _, z := range v.Childs["db_xref"] {
					d.addXref(entryid, fr, z.Attrs["dbkey"], z.Attrs["db"], false)
				}
			}
		}

		for _, v := range r.Elements["external_doc_list"] {
			if _, ok = v.Childs["db_xref"]; ok {
				for _, z := range v.Childs["db_xref"] {
					d.addXref(entryid, fr, z.Attrs["dbkey"], z.Attrs["db"], false)
				}
			}
		}

		for _, v := range r.Elements["structure_db_links"] {
			if _, ok = v.Childs["db_xref"]; ok {
				for _, z := range v.Childs["db_xref"] {
					d.addXref(entryid, fr, z.Attrs["dbkey"], z.Attrs["db"], false)
				}
			}
		}

		/**
		// representativeMember--> dbreference
		for _, v = range r.Elements["pub_list"] {
			if _, ok = v.Childs["publication"]; ok {
				for _, z = range v.Childs["publication"] {
					d.addXref(&entryid, fr, &z)
				}
			}
		}
		// member --> dbreference
		for _, v = range r.Elements["member"] {
			if _, ok = v.Childs["dbReference"]; ok {
				for _, z = range v.Childs["dbReference"] {
					d.addXref(&entryid, fr, &z)
				}
			}
		}
		**/

		total++
	}

	atomic.AddUint64(&d.totalParsedEntry, total)

	d.addEntryStat("interpro", total)

}

func (d *dataUpdate) literatureMappings(source string) {

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	client = d.ftpClient(d.ebiFtp)

	// first doi pm and pmcid mappings
	path := d.ebiFtpPath + dataconf[source]["path"]
	ftpfile, err := client.Retr(path)

	var gz *gzip.Reader
	var br *bufio.Reader

	gz, err = gzip.NewReader(ftpfile)
	check(err)
	br = bufio.NewReaderSize(gz, fileBufSize)

	/**
	scanner := bufio.NewScanner(gz)
	for scanner.Scan() {
		lineByParser(scanner.Text())
	}
	*/

	defer d.wg.Done()

	r := csv.NewReader(br)
	//r.Comma = '	'
	pubmedfr := dataconf["PubMed"]["id"]
	pmcfr := dataconf["PMC"]["id"]
	doiprefix := dataconf[source]["doiPrefix"]

	i := 0
	for {
		i++
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if i == 1 {
			continue
		}
		if len(record[0]) > 0 {
			if len(record[1]) > 0 {
				d.addXref(record[0], pubmedfr, record[1], "PMC", false)
			}
			if len(record[2]) > 0 {

				doiid := strings.TrimSpace(strings.TrimPrefix(record[2], doiprefix))
				if len(doiid) > 0 {
					d.addXref(record[0], pubmedfr, doiid, "DOI", false)
				}
			}
		}

		if len(record[1]) > 0 {
			if len(record[2]) > 0 {
				doiid := strings.TrimSpace(strings.TrimPrefix(record[2], doiprefix))
				if len(doiid) > 0 {
					d.addXref(record[1], pmcfr, doiid, "DOI", false)
				}
			}
		}

	}

	ftpfile.Close()

}

/**
pmc part not used
*/
func (d *dataUpdate) literatureMappings2(source string) {

	defer d.wg.Done()

	if d.p == nil {
		d.p = mpb.New(mpb.WithWaitGroup(d.wg))
	}

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	client = d.ftpClient(d.ebiFtp)

	// first doi pm and pmcid mappings
	path := d.ebiFtpPath + "/pmc/PMCLiteMetadata/PMCLiteMetadata.tgz"
	ftpfile, err := client.Retr(path)

	var gz *gzip.Reader
	var br *bufio.Reader

	gz, err = gzip.NewReader(ftpfile)
	check(err)
	br = bufio.NewReaderSize(gz, fileBufSize)

	tr := tar.NewReader(br)

	pubmedfr := dataconf["PubMed"]["id"]
	pmcfr := dataconf["PMC"]["id"]

	for {

		hdr, err := tr.Next()

		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		if filepath.Ext(hdr.Name) == ".xml" {

			bar := d.p.AddBar(hdr.FileInfo().Size(),
				mpb.BarClearOnComplete(),
				mpb.PrependDecorators(
					// simple name decorator
					decor.Name("literature "+hdr.Name),
					// decor.DSyncWidth bit enables column width synchronization
					decor.Percentage(decor.WCSyncSpace),
				),
				mpb.AppendDecorators(

					decor.OnComplete(
						decor.Elapsed(decor.ET_STYLE_GO), "done",
					),
				),
			)

			// after each parsing parsing channel is closed for each file seperate one needed
			var resultChannel = d.newResultChannel()
			bbr := bufio.NewReaderSize(tr, fileBufSize)

			p := parser.XMLParser{
				R:          bbr,
				LoopTag:    "PMC_ARTICLE",
				OutChannel: resultChannel,
				SkipTags:   []string{"AuthorList,journalTitle"},
				//FinishMessage: "literature mappings " + hdr.Name,
				ProgBar:    bar,
				ProgBySize: true,
			}

			go p.Parse()

			for r := range *resultChannel {
				// accs
				var pmid, doi, pmcid string
				for _, v := range r.Elements["pmid"] {
					pmid = v.InnerText
					break
				}
				for _, v := range r.Elements["pmcid"] {
					pmcid = v.InnerText
					break
				}
				for _, v := range r.Elements["DOI"] {
					doi = v.InnerText
					break
				}

				if len(pmid) > 0 && len(pmcid) > 0 {
					//d.addXrefSingle(&pmid, pubmedfr, "PMC", pmcid)
					d.addXref(pmid, pubmedfr, pmcid, "PMC", false)

				}

				if len(pmid) > 0 && len(doi) > 0 {
					d.addXref(pmid, pubmedfr, doi, "DOI", false)
				}

				if len(pmcid) > 0 && len(doi) > 0 {
					d.addXref(pmcid, pmcfr, doi, "DOI", false)
				}

			}

		}

	}

}

func (d *dataUpdate) updateHmdb(source string) {

	defer d.wg.Done()

	var resultChannel = d.newResultChannel()

	resp, err := http.Get(dataconf[source]["path"])
	check(err)
	defer resp.Body.Close()

	zips := zipstream.NewReader(resp.Body)

	zips.Next()

	br := bufio.NewReaderSize(zips, fileBufSize)

	p := parser.XMLParser{
		R:             br,
		LoopTag:       "metabolite",
		OutChannel:    resultChannel,
		SkipTags:      []string{"taxonomy,ontology"},
		FinishMessage: "HMDB",
		//ProgBar:       bar,
	}

	go p.Parse()

	var total uint64
	var r parser.XMLEntry
	var v, z parser.XMLElement
	var ok bool
	var entryid string

	var fr = dataconf[source]["id"]
	var hmdbdis = dataconf["hmdb disease"]["id"]

	for r = range *resultChannel {

		entryid = r.Elements["accession"][0].InnerText

		// secondary accs
		for _, v = range r.Elements["secondary_accessions"] {
			if _, ok = v.Childs["accession"]; ok {
				for _, z = range v.Childs["accession"] {
					d.addXref(entryid, fr, z.InnerText, "hmdb", false)
				}
			}
		}
		//name
		name := r.Elements["name"][0].InnerText
		d.addXref(name, textLinkID, entryid, "hmdb", true)

		// synonyms
		for _, v = range r.Elements["synonyms"] {
			if _, ok = v.Childs["synonym"]; ok {
				for _, z = range v.Childs["synonym"] {
					d.addXref(z.InnerText, textLinkID, entryid, "hmdb", true)
				}
			}
		}
		//formula
		if len(r.Elements["chemical_formula"]) > 0 {
			formula := r.Elements["chemical_formula"][0].InnerText
			d.addXref(formula, textLinkID, entryid, "hmdb", false)
		}

		if len(r.Elements["cas_registry_number"]) > 0 {
			cas := r.Elements["cas_registry_number"][0].InnerText
			d.addXref(entryid, fr, cas, "CAS", false)
		}

		for _, v := range r.Elements["pathways"] {
			if _, ok = v.Childs["pathway"]; ok {
				for _, z := range v.Childs["pathway"] {
					if _, ok = z.Childs["kegg_map_id"]; ok {
						for _, x := range z.Childs["kegg_map_id"] {
							if len(x.InnerText) > 0 {
								d.addXref(entryid, fr, x.InnerText, "KEGG MAP", false)
							}
						}
					}
				}
			}
		}

		for _, v := range r.Elements["normal_concentrations"] {
			if _, ok = v.Childs["concentration"]; ok {
				for _, z := range v.Childs["concentration"] {
					if _, ok = z.Childs["references"]; ok {
						for _, x := range z.Childs["references"] {
							if _, ok = x.Childs["reference"]; ok {
								for _, t := range x.Childs["reference"] {
									if _, ok = t.Childs["pubmed_id"]; ok {
										for _, g := range t.Childs["pubmed_id"] {
											if len(g.InnerText) > 0 {
												d.addXref(entryid, fr, g.InnerText, "PubMed", false)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		for _, v := range r.Elements["abnormal_concentrations"] {
			if _, ok = v.Childs["concentration"]; ok {
				for _, z := range v.Childs["concentration"] {
					for _, x := range z.Childs["references"] {
						if _, ok = x.Childs["reference"]; ok {
							for _, t := range x.Childs["reference"] {
								if _, ok = t.Childs["pubmed_id"]; ok {
									for _, g := range t.Childs["pubmed_id"] {
										if len(g.InnerText) > 0 {
											d.addXref(entryid, fr, g.InnerText, "PubMed", false)
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// this is use case for graph based approach
		for _, v := range r.Elements["diseases"] {
			if _, ok = v.Childs["disease"]; ok {
				for _, z := range v.Childs["disease"] {

					if _, ok = z.Childs["name"]; ok {
						diseaseName := z.Childs["name"][0].InnerText
						d.addXref(entryid, fr, diseaseName, "hmdb disease", false)

						if _, ok = z.Childs["omim_id"]; ok {
							for _, x := range z.Childs["omim_id"] {
								if len(x.InnerText) > 0 {
									d.addXref(diseaseName, hmdbdis, x.InnerText, "MIM", false)
								}
							}
						}

						//disase pubmed references
						for _, x := range z.Childs["references"] {
							if _, ok = x.Childs["reference"]; ok {
								for _, t := range x.Childs["reference"] {
									if _, ok = t.Childs["pubmed_id"]; ok {
										for _, g := range t.Childs["pubmed_id"] {
											if len(g.InnerText) > 0 {
												d.addXref(diseaseName, hmdbdis, g.InnerText, "PubMed", false)
											}
										}
									}
								}
							}
						}

					}
				}
			}
		}

		// rest of xrefs
		for _, v := range r.Elements["drugbank_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "DrugBank", false)
			}
		}

		for _, v := range r.Elements["kegg_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "KEGG", false)
			}
		}

		for _, v := range r.Elements["biocyc_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "BioCyc", false)
			}
		}

		for _, v := range r.Elements["pubchem_compound_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "Pubchem", false)
			}
		}

		for _, v := range r.Elements["chebi_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, "CHEBI:"+v.InnerText, "chebi", false)
			}
		}

		for _, x := range r.Elements["general_references"] {
			if _, ok = x.Childs["reference"]; ok {
				for _, t := range x.Childs["reference"] {
					if _, ok = t.Childs["pubmed_id"]; ok {
						for _, g := range t.Childs["pubmed_id"] {
							if len(g.InnerText) > 0 {
								d.addXref(entryid, fr, g.InnerText, "PubMed", false)
							}
						}
					}
				}
			}
		}

		// todo in here there is also gene symbol but it also requires graph based transitive feature.
		for _, x := range r.Elements["protein_associations"] {
			if _, ok = x.Childs["protein"]; ok {
				for _, t := range x.Childs["protein"] {
					if _, ok = t.Childs["uniprot_id"]; ok {
						for _, g := range t.Childs["uniprot_id"] {
							if len(g.InnerText) > 0 {
								d.addXref(entryid, fr, g.InnerText, "UniProtKB", false)
							}
						}
					}
				}
			}
		}

		total++
	}

	atomic.AddUint64(&d.totalParsedEntry, total)

	d.addEntryStat(source, total)

}
