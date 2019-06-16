package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tamerh/jsparser"
	"github.com/tamerh/xml-stream-parser"

	"github.com/krolaw/zipstream"

	"github.com/jlaffaye/ftp"
	"github.com/vbauerster/mpb"
)

const textLinkID = "0"

var mutex = &sync.Mutex{}

type dataUpdate struct {
	wg                     *sync.WaitGroup
	kvdatachan             *chan string
	invalidXrefs           HashMaper
	sampleXrefs            HashMaper
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
}

func newDataUpdate(targetDatasetMap map[string]bool, ensemblSpecies []string) *dataUpdate {

	loc := appconf["uniprot_ftp"]
	uniprotftpAddr := appconf["uniprot_ftp_"+loc]
	uniprotftpPath := appconf["uniprot_ftp_"+loc+"_path"]

	ebiftp := appconf["ebi_ftp"]
	ebiftppath := appconf["ebi_ftp_path"]

	return &dataUpdate{
		invalidXrefs:           NewHashMap(300),
		sampleXrefs:            NewHashMap(400),
		uniprotFtp:             uniprotftpAddr,
		uniprotFtpPath:         uniprotftpPath,
		ebiFtp:                 ebiftp,
		ebiFtpPath:             ebiftppath,
		targetDatasets:         targetDatasetMap,
		hasTargets:             len(targetDatasetMap) > 0,
		channelOverflowCap:     channelOverflowCap,
		stats:                  make(map[string]interface{}),
		selectedEnsemblSpecies: ensemblSpecies,
	}

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

func (d *dataUpdate) addEntryStat(source string, total uint64) {

	var entrysize = map[string]uint64{}
	entrysize["entrySize"] = total
	mutex.Lock()
	d.stats[source] = entrysize
	mutex.Unlock()

}

func (d *dataUpdate) getDataReaderNew(datatype string, ftpAddr string, ftpPath string, filePath string) (*bufio.Reader, *gzip.Reader, *ftp.Response, *os.File, string, int64) {

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

func (d *dataUpdate) addXref(key string, from string, value string, valueFrom string, isLink bool) {

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if len(key) == 0 || len(value) == 0 || len(from) == 0 {
		return
	}

	if _, ok := dataconf[valueFrom]; !isLink && !ok {
		//if appconf["debug"] == "y" {
		if val, _ := d.invalidXrefs.Get(valueFrom); val == nil {
			fmt.Println("Warn:Undefined xref name:", valueFrom, "with value", value, " skipped!. Define in data.json to be included")
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

	br, gz, ftpFile, localFile, fr, _ := d.getDataReaderNew(source, d.uniprotFtp, d.uniprotFtpPath, dataconf[source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"comment", "gene", "protein", "feature", "sequence"})

	var total uint64
	var v, x, z xmlparser.XMLElement
	var entryid string

	for r := range p.Stream() {
		entryid = r.Childs["name"][0].InnerText

		for _, v = range r.Childs["accession"] {
			d.addXref(v.InnerText, textLinkID, entryid, source, true)
		}

		for _, v = range r.Childs["dbReference"] {

			d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["type"], false)

			for _, z := range v.Childs["property"] {
				d.addXref(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], z.Attrs["value"], z.Attrs["type"], false)
			}

		}

		for _, v = range r.Childs["organism"] {
			for _, z = range v.Childs["dbReference"] {

				d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
				for _, x := range z.Childs["property"] {
					d.addXref(z.Attrs["id"], dataconf[z.Attrs["type"]]["id"], x.Attrs["value"], x.Attrs["type"], false)
				}
			}
		}

		for _, v = range r.Childs["reference"] {
			for _, z = range v.Childs["citation"] {
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

		total++

	}

	atomic.AddUint64(&d.totalParsedEntry, total)
	d.addEntryStat(source, total)
}

func (d *dataUpdate) updateUniref(unireftype string) {

	br, gz, ftpFile, localFile, fr, _ := d.getDataReaderNew(unireftype, d.uniprotFtp, d.uniprotFtpPath, dataconf[unireftype]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}

	defer gz.Close()
	defer d.wg.Done()

	p := xmlparser.NewXMLParser(br, "entry")

	// for uniref we are only interested in two references uniprot and uniparc
	validRefs := map[string]bool{}
	validRefs["UniParc ID"] = true
	validRefs["UniProtKB ID"] = true

	var total uint64
	var v, z xmlparser.XMLElement
	var ok bool
	var entryid string

	for r := range p.Stream() {
		// id
		entryid = r.Attrs["id"]

		// representativeMember--> dbreference
		for _, v = range r.Childs["representativeMember"] {
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

		// member --> dbreference
		for _, v = range r.Childs["member"] {
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

		total++

	}

	atomic.AddUint64(&d.totalParsedEntry, total)
	d.addEntryStat(unireftype, total)

}

func (d *dataUpdate) updateUniparc() {

	br, gz, ftpFile, localFile, fr, _ := d.getDataReaderNew("uniparc", d.uniprotFtp, d.uniprotFtpPath, dataconf["uniparc"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"sequence"})

	// we are excluding uniprot subreference because they are already coming from uniprot. this may be optional
	propExclusionsRefs := map[string]bool{}
	propExclusionsRefs["UniProtKB/Swiss-Prot"] = true
	propExclusionsRefs["UniProtKB/TrEMBL"] = true

	var total uint64
	var v xmlparser.XMLElement
	var ok bool
	var entryid string

	for r := range p.Stream() {
		// id
		entryid = r.Childs["accession"][0].InnerText

		//dbreference
		for _, v = range r.Childs["dbReference"] {

			d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["type"], false)

			if _, ok = propExclusionsRefs[v.Attrs["type"]]; !ok {
				for _, z := range v.Childs["property"] {
					d.addXref(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], z.Attrs["value"], z.Attrs["type"], false)
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

	br, gz, ftpFile, localFile, fr, _ := d.getDataReaderNew("taxonomy", d.ebiFtp, d.ebiFtpPath, dataconf["taxonomy"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := xmlparser.NewXMLParser(br, "taxon").SkipElements([]string{"lineage"})

	var total uint64
	var v, z xmlparser.XMLElement
	var ok bool
	var entryid string

	for r := range p.Stream() {

		// id
		entryid = r.Attrs["taxId"]

		d.addXref(r.Attrs["scientificName"], textLinkID, entryid, "taxonomy", true)

		//dbreference
		for _, v = range r.Childs["children"] {
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

func (d *dataUpdate) updateHgnc() {

	br, _, ftpFile, localFile, fr, _ := d.getDataReaderNew("hgnc", d.ebiFtp, d.ebiFtpPath, dataconf["hgnc"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer d.wg.Done()

	p := jsparser.NewJSONParser(br, "docs")

	var ok bool
	var v *jsparser.JSON
	var total uint64

	a := func(jid string, dbid string, j *jsparser.JSON, entryid string) {

		if _, ok = j.ObjectVals[jid]; ok && len(j.ObjectVals[jid].ArrayVals) > 0 {
			for _, v = range j.ObjectVals[jid].ArrayVals {
				d.addXref(entryid, fr, v.StringVal, dbid, false)
			}
		} else if _, ok = j.ObjectVals[jid]; ok {
			d.addXref(entryid, fr, j.ObjectVals[jid].StringVal, dbid, false)
		}
	}

	for j := range p.Stream() {

		entryid := j.ObjectVals["hgnc_id"].StringVal
		if len(entryid) > 0 {
			a("cosmic", "COSMIC", j, entryid)
			a("omim_id", "MIM", j, entryid)
			a("ena", "EMBL", j, entryid)
			a("ccds_id", "CCDS", j, entryid)
			a("enzyme_id", "Intenz", j, entryid)
			a("vega_id", "VEGA", j, entryid)
			a("ensembl_gene_id", "Ensembl", j, entryid)
			a("pubmed_id", "PubMed", j, entryid)
			a("refseq_accession", "RefSeq", j, entryid)
			a("uniprot_ids", "UniProtKB", j, entryid)

			if _, ok = j.ObjectVals["symbol"]; ok && len(j.ObjectVals["symbol"].ArrayVals) > 0 {
				for _, v = range j.ObjectVals["symbol"].ArrayVals {
					d.addXref(v.StringVal, textLinkID, entryid, "hgnc", true)
				}
			} else if _, ok = j.ObjectVals["symbol"]; ok && len(j.ObjectVals["symbol"].StringVal) > 0 {
				d.addXref(j.ObjectVals["symbol"].StringVal, textLinkID, entryid, "hgnc", true)
			}

			if _, ok = j.ObjectVals["name"]; ok && len(j.ObjectVals["name"].ArrayVals) > 0 {
				for _, v = range j.ObjectVals["name"].ArrayVals {
					d.addXref(v.StringVal, textLinkID, entryid, "hgnc", true)
				}
			} else if _, ok = j.ObjectVals["name"]; ok && len(j.ObjectVals["name"].StringVal) > 0 {
				d.addXref(j.ObjectVals["name"].StringVal, textLinkID, entryid, "hgnc", true)
			}

		}

		total++
	}

	atomic.AddUint64(&d.totalParsedEntry, total)

	d.addEntryStat("hgnc", total)

}

func (d *dataUpdate) updateChebi() {

	defer d.wg.Done()

	chebiPath := dataconf["chebi"]["path"]
	chebiFiles := strings.Split(dataconf["chebi"]["files"], ",")

	//xreftypes := map[string]bool{}

	for _, name := range chebiFiles {
		br, _, ftpFile, localFile, fr, _ := d.getDataReaderNew("chebi", d.ebiFtp, d.ebiFtpPath, chebiPath+name)

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

	br, gz, ftpFile, localFile, fr, _ := d.getDataReaderNew("interpro", d.ebiFtp, d.ebiFtpPath, dataconf["interpro"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer d.wg.Done()

	p := xmlparser.NewXMLParser(br, "interpro").SkipElements([]string{"abstract", "p"})

	var total uint64
	var entryid string

	for r := range p.Stream() {
		// id
		entryid = r.Attrs["id"]

		for _, x := range r.Childs["pub_list"] {
			for _, y := range x.Childs["publication"] {
				for _, z := range y.Childs["db_xref"] {
					d.addXref(entryid, fr, z.Attrs["dbkey"], z.Attrs["db"], false)
				}
			}
		}

		for _, x := range r.Childs["found_in"] {
			for _, y := range x.Childs["rel_ref"] {
				d.addXref(entryid, fr, y.Attrs["ipr_ref"], "INTERPRO", false)
			}
		}

		for _, x := range r.Childs["member_list"] {
			for _, y := range x.Childs["db_xref"] {
				d.addXref(entryid, fr, y.Attrs["dbkey"], y.Attrs["db"], false)
			}
		}

		for _, x := range r.Childs["external_doc_list"] {
			for _, y := range x.Childs["db_xref"] {
				d.addXref(entryid, fr, y.Attrs["dbkey"], y.Attrs["db"], false)
			}
		}

		for _, x := range r.Childs["structure_db_links"] {
			for _, y := range x.Childs["db_xref"] {
				d.addXref(entryid, fr, y.Attrs["dbkey"], y.Attrs["db"], false)
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
		if len(record) < 3 {
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

			// after each parsing parsing channel is closed for each file seperate one needed
			bbr := bufio.NewReaderSize(tr, fileBufSize)

			p := xmlparser.NewXMLParser(bbr, "PMC_ARTICLE").SkipElements([]string{"AuthorList,journalTitle"})

			for r := range p.Stream() {
				// accs
				var pmid, doi, pmcid string
				for _, v := range r.Childs["pmid"] {
					pmid = v.InnerText
					break
				}
				for _, v := range r.Childs["pmcid"] {
					pmcid = v.InnerText
					break
				}
				for _, v := range r.Childs["DOI"] {
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

	resp, err := http.Get(dataconf[source]["path"])
	check(err)
	defer resp.Body.Close()

	zips := zipstream.NewReader(resp.Body)

	zips.Next()

	br := bufio.NewReaderSize(zips, fileBufSize)

	p := xmlparser.NewXMLParser(br, "metabolite").SkipElements([]string{"taxonomy,ontology"})

	var total uint64
	var v, z xmlparser.XMLElement
	var ok bool
	var entryid string

	var fr = dataconf[source]["id"]
	var hmdbdis = dataconf["hmdb disease"]["id"]

	for r := range p.Stream() {

		entryid = r.Childs["accession"][0].InnerText

		// secondary accs
		for _, v = range r.Childs["secondary_accessions"] {
			for _, z = range v.Childs["accession"] {
				d.addXref(entryid, fr, z.InnerText, "hmdb", false)
			}
		}

		//name
		name := r.Childs["name"][0].InnerText
		d.addXref(name, textLinkID, entryid, "hmdb", true)

		// synonyms
		for _, v = range r.Childs["synonyms"] {
			for _, z = range v.Childs["synonym"] {
				d.addXref(z.InnerText, textLinkID, entryid, "hmdb", true)
			}
		}

		//formula
		if len(r.Childs["chemical_formula"]) > 0 {
			formula := r.Childs["chemical_formula"][0].InnerText
			d.addXref(formula, textLinkID, entryid, "hmdb", false)
		}

		if len(r.Childs["cas_registry_number"]) > 0 {
			cas := r.Childs["cas_registry_number"][0].InnerText
			d.addXref(entryid, fr, cas, "CAS", false)
		}

		for _, v := range r.Childs["pathways"] {
			for _, z := range v.Childs["pathway"] {
				for _, x := range z.Childs["kegg_map_id"] {
					if len(x.InnerText) > 0 {
						d.addXref(entryid, fr, x.InnerText, "KEGG MAP", false)
					}
				}
			}
		}

		for _, v := range r.Childs["normal_concentrations"] {
			for _, z := range v.Childs["concentration"] {
				for _, x := range z.Childs["references"] {
					for _, t := range x.Childs["reference"] {
						for _, g := range t.Childs["pubmed_id"] {
							if len(g.InnerText) > 0 {
								d.addXref(entryid, fr, g.InnerText, "PubMed", false)
							}
						}
					}
				}
			}
		}

		for _, v := range r.Childs["abnormal_concentrations"] {
			for _, z := range v.Childs["concentration"] {
				for _, x := range z.Childs["references"] {
					for _, t := range x.Childs["reference"] {
						for _, g := range t.Childs["pubmed_id"] {
							if len(g.InnerText) > 0 {
								d.addXref(entryid, fr, g.InnerText, "PubMed", false)
							}
						}
					}
				}
			}
		}

		// this is use case for graph based approach
		for _, v := range r.Childs["diseases"] {
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
						for _, t := range x.Childs["reference"] {
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

		// rest of xrefs
		for _, v := range r.Childs["drugbank_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "DrugBank", false)
			}
		}

		for _, v := range r.Childs["kegg_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "KEGG", false)
			}
		}

		for _, v := range r.Childs["biocyc_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "BioCyc", false)
			}
		}

		for _, v := range r.Childs["pubchem_compound_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, v.InnerText, "Pubchem", false)
			}
		}

		for _, v := range r.Childs["chebi_id"] {
			if len(v.InnerText) > 0 {
				d.addXref(entryid, fr, "CHEBI:"+v.InnerText, "chebi", false)
			}
		}

		for _, x := range r.Childs["general_references"] {
			for _, t := range x.Childs["reference"] {
				for _, g := range t.Childs["pubmed_id"] {
					if len(g.InnerText) > 0 {
						d.addXref(entryid, fr, g.InnerText, "PubMed", false)
					}
				}
			}
		}

		// todo in here there is also gene symbol but it also requires graph based transitive feature.
		for _, x := range r.Childs["protein_associations"] {
			for _, t := range x.Childs["protein"] {
				for _, g := range t.Childs["uniprot_id"] {
					if len(g.InnerText) > 0 {
						d.addXref(entryid, fr, g.InnerText, "UniProtKB", false)
					}
				}
			}
		}
		total++
	}

	atomic.AddUint64(&d.totalParsedEntry, total)

	d.addEntryStat(source, total)

}

func (d *dataUpdate) getEnsemblSetting(ensemblType string) (string, string, []string, []string) {

	var selectedSpecies []string
	//jsonFilePaths := map[string]string{}
	//biomartFilePaths := map[string][]string{}
	var jsonFilePaths []string
	var biomartFilePaths []string
	var fr string

	if len(d.selectedEnsemblSpecies) == 1 && d.selectedEnsemblSpecies[0] == "all" {
		d.selectedEnsemblSpecies = nil
	}
	var ftpAddress string
	var ftpJSONPath string
	var ftpMysqlPath string
	var ftpBiomartFolder string
	var branch string

	fungiAndBacteriaPaths := map[string]string{} // this is needed for fungi and bacteria because their json might under a collection

	setJSONs := func() {
		client := d.ftpClient(ftpAddress)
		entries, err := client.List(ftpJSONPath)
		check(err)

		if branch == "fungi" || branch == "bacteria" {
			for _, file := range entries {
				if strings.HasSuffix(file.Name, "_collection") {
					client := d.ftpClient(ftpAddress)
					entries2, err := client.List(ftpJSONPath+"/"+file.Name)
					check(err)
					for _, file2 := range entries2 {
						fungiAndBacteriaPaths[file2.Name] = ftpJSONPath + "/" + file.Name + "/" + file2.Name + "/" + file2.Name + ".json"
					}
				} else {
					fungiAndBacteriaPaths[file.Name] = ftpJSONPath + "/" + file.Name + "/" + file.Name + ".json"
				}
			}
		}

		if d.selectedEnsemblSpecies == nil { // if all selected

			if branch == "fungi" || branch == "bacteria" {
				for _, v := range fungiAndBacteriaPaths {
					jsonFilePaths = append(jsonFilePaths, v)
				}
			} else {
				for _, file := range entries {
					selectedSpecies = append(selectedSpecies, file.Name)
					//jsonFilePaths[file.Name] = ftpJSONPath + "/" + file.Name + "/" + file.Name + ".json"
					jsonFilePaths = append(jsonFilePaths, ftpJSONPath+"/"+file.Name+"/"+file.Name+".json")
				}
			}
		} else {
			for _, sp := range d.selectedEnsemblSpecies {
				if branch == "fungi" || branch == "bacteria" {
					if _,ok:=fungiAndBacteriaPaths[sp];!ok{
						log.Fatal("Error Species not found check the name")
						continue
					}
					jsonFilePaths = append(jsonFilePaths, fungiAndBacteriaPaths[sp])
					continue
				}
				//jsonFilePaths[sp] = ftpJSONPath + "/" + sp + "/" + sp + ".json"
				jsonFilePaths = append(jsonFilePaths, ftpJSONPath+"/"+sp+"/"+sp+".json")
			}
		}
	}

	setBiomarts := func() {

		// setup biomart release not handled at the moment
		if d.ensemblRelease == "" {
			// find the biomart folder
			client := d.ftpClient(ftpAddress)
			entries, err := client.List(ftpMysqlPath + "/" + branch + "_mart_*")
			check(err)
			if len(entries) != 1 {
				log.Fatal("Error:More than one mart folder found for biomart")
			}
			if len(entries) == 1 {
				ftpBiomartFolder = entries[0].Name
			}

		}

		var biomartSpeciesName string // this is just the shorcut name of species in biomart folder e.g homo_sapiens-> hsapiens
		for _, sp := range d.selectedEnsemblSpecies {
			
			splitted := strings.Split(sp, "_")
			if len(splitted)>1{
				biomartSpeciesName = splitted[0][:1] + splitted[len(splitted)-1]
			}else {
				panic("Unrecognized species name pattern->" + sp)
			}

			// now get list of probset files
			client := d.ftpClient(ftpAddress)
			entries, err := client.List(ftpMysqlPath + "/" + ftpBiomartFolder + "/" + biomartSpeciesName + "*__efg_*.gz")
			check(err)
			//var biomartFiles []string
			for _, file := range entries {
				biomartFilePaths = append(biomartFilePaths, ftpMysqlPath+"/"+ftpBiomartFolder+"/"+file.Name)
			}
			//biomartFilePaths[sp] = biomartFiles
		}
	}

	switch ensemblType {

	case "ensembl":
		fr = dataconf["ensembl"]["id"]
		ftpAddress = appconf["ensembl_ftp"]
		ftpJSONPath = appconf["ensembl_ftp_json_path"]
		ftpMysqlPath = appconf["ensembl_ftp_mysql_path"]
		branch = "ensembl"
		setJSONs()
		setBiomarts()
	case "ensembl_bacteria":
		fr = dataconf["ensembl_bacteria"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "bacteria", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "bacteria", 1)
		branch = "bacteria"
		setJSONs()
	case "ensembl_fungi":
		fr = dataconf["ensembl_fungi"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "fungi", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "fungi", 1)
		branch = "fungi"
		setJSONs()
		setBiomarts()
	case "ensembl_metazoa":
		fr = dataconf["ensembl_metazoa"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "metazoa", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "metazoa", 1)
		branch = "metazoa"
		setJSONs()
		setBiomarts()
	case "ensembl_plants":
		fr = dataconf["ensembl_plants"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "plants", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "plants", 1)
		branch = "plants"
		setJSONs()
		setBiomarts()
	case "ensembl_protists":
		fr = dataconf["ensembl_protists"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "protists", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "protists", 1)
		branch = "protists"
		setJSONs()
		setBiomarts()
	}

	return fr, ftpAddress, jsonFilePaths, biomartFilePaths

}

func (d *dataUpdate) updateEnsembl(ensemblType string) {

	defer d.wg.Done()

	ensemblTranscriptID := dataconf["EnsemblTranscript"]["id"]

	fr, ftpAddress, jsonPaths, biomartPaths := d.getEnsemblSetting(ensemblType)

	// if local file just ignore ftp jsons
	if dataconf[ensemblType]["useLocalFile"] == "yes" {
		jsonPaths = nil
		jsonPaths = append(jsonPaths, dataconf[ensemblType]["path"])
	}

	xref := func(j *jsparser.JSON, entryid, propName, dbid string) {

		if j.ObjectVals[propName] != nil {
			for _, val := range j.ObjectVals[propName].ArrayVals {
				d.addXref(entryid, fr, val.StringVal, dbid, false)
			}
		}
	}

	for _, path := range jsonPaths {

		br, _, ftpFile, localFile, _, _ := d.getDataReaderNew(ensemblType, ftpAddress, "", path)

		p := jsparser.NewJSONParser(br, "genes").SkipProps([]string{"description", "lineage", "start", "end", "evidence", "coord_system", "sifts", "gene_tree_id", "genome_display", "orthology_type", "genome", "seq_region_name", "strand", "xrefs"})

		for j := range p.Stream() {
			if j.ObjectVals["id"] != nil {

				entryid := j.ObjectVals["id"].StringVal

				if j.ObjectVals["name"] != nil {
					d.addXref(j.ObjectVals["name"].StringVal, textLinkID, entryid, ensemblType, true)
				}

				if j.ObjectVals["taxon_id"] != nil {
					d.addXref(entryid, fr, j.ObjectVals["taxon_id"].StringVal, "taxonomy", false)
				}

				if j.ObjectVals["homologues"] != nil {
					for _, val := range j.ObjectVals["homologues"].ArrayVals {
						d.addXref(entryid, fr, val.ObjectVals["stable_id"].StringVal, "EnsemblHomolog", false)
					}
				}

				// maybe these values from configuration
				xref(j, entryid, "Interpro", "interpro")
				xref(j, entryid, "HPA", "HPA")
				xref(j, entryid, "ArrayExpress", "ExpressionAtlas")
				xref(j, entryid, "GENE3D", "CATHGENE3D")
				xref(j, entryid, "MIM_GENE", "MIM")
				xref(j, entryid, "RefSeq_peptide", "RefSeq")
				xref(j, entryid, "EntrezGene", "GeneID")
				xref(j, entryid, "PANTHER", "PANTHER")
				xref(j, entryid, "Reactome", "Reactome")
				xref(j, entryid, "RNAcentral", "RNAcentral")
				xref(j, entryid, "Uniprot/SPTREMBL", "uniprot_unreviewed")
				xref(j, entryid, "protein_id", "EMBL")
				xref(j, entryid, "KEGG_Enzyme", "KEGG")
				xref(j, entryid, "EMBL", "EMBL")
				xref(j, entryid, "CDD", "CDD")
				xref(j, entryid, "TIGRfam", "TIGRFAMs")
				xref(j, entryid, "ChEMBL", "ChEMBL")
				xref(j, entryid, "UniParc", "uniparc")
				xref(j, entryid, "PDB", "PDB")
				xref(j, entryid, "SuperFamily", "SUPFAM")
				xref(j, entryid, "Prosite_profiles", "PROSITE")
				xref(j, entryid, "RefSeq_mRNA", "RefSeq")
				xref(j, entryid, "Pfam", "Pfam")
				xref(j, entryid, "CCDS", "RefSeq")
				xref(j, entryid, "Prosite_patterns", "PROSITE")
				xref(j, entryid, "Uniprot/SWISSPROT", "uniprot_reviewed")
				xref(j, entryid, "UCSC", "UCSC")
				xref(j, entryid, "Uniprot_gn", "uniprot_reviewed")
				xref(j, entryid, "HGNC", "hgnc")
				xref(j, entryid, "RefSeq_ncRNA_predicted", "RefSeq")
				xref(j, entryid, "HAMAP", "HAMAP")

				if j.ObjectVals["transcripts"] != nil {
					for _, val := range j.ObjectVals["transcripts"].ArrayVals {
						tentryid := val.ObjectVals["id"].StringVal

						d.addXref(entryid, fr, tentryid, "EnsemblTranscript", false)

						if val.ObjectVals["name"] != nil {
							d.addXref(val.ObjectVals["name"].StringVal, textLinkID, tentryid, "EnsemblTranscript", true)
						}

						if val.ObjectVals["exons"] != nil {
							for _, exo := range val.ObjectVals["exons"].ArrayVals {
								d.addXref(tentryid, ensemblTranscriptID, exo.ObjectVals["id"].StringVal, "EnsemblExon", false)
							}
						}

						xref(j, tentryid, "Interpro", "interpro")
						xref(j, tentryid, "HPA", "HPA")
						xref(j, tentryid, "ArrayExpress", "ExpressionAtlas")
						xref(j, tentryid, "GENE3D", "CATHGENE3D")
						xref(j, tentryid, "MIM_GENE", "MIM")
						xref(j, tentryid, "RefSeq_peptide", "RefSeq")
						xref(j, tentryid, "EntrezGene", "GeneID")
						xref(j, tentryid, "PANTHER", "PANTHER")
						xref(j, tentryid, "Reactome", "Reactome")
						xref(j, tentryid, "RNAcentral", "RNAcentral")
						xref(j, tentryid, "Uniprot/SPTREMBL", "uniprot_unreviewed")
						xref(j, tentryid, "protein_id", "EMBL")
						xref(j, tentryid, "KEGG_Enzyme", "KEGG")
						xref(j, tentryid, "EMBL", "EMBL")
						xref(j, tentryid, "CDD", "CDD")
						xref(j, tentryid, "TIGRfam", "TIGRFAMs")
						xref(j, tentryid, "ChEMBL", "ChEMBL")
						xref(j, tentryid, "UniParc", "uniparc")
						xref(j, tentryid, "PDB", "PDB")
						xref(j, tentryid, "SuperFamily", "SUPFAM")
						xref(j, tentryid, "Prosite_profiles", "PROSITE")
						xref(j, tentryid, "RefSeq_mRNA", "RefSeq")
						xref(j, tentryid, "Pfam", "Pfam")
						xref(j, tentryid, "CCDS", "RefSeq")
						xref(j, tentryid, "Prosite_patterns", "PROSITE")
						xref(j, tentryid, "Uniprot/SWISSPROT", "uniprot_reviewed")
						xref(j, tentryid, "UCSC", "UCSC")
						xref(j, tentryid, "Uniprot_gn", "uniprot_reviewed")
						xref(j, tentryid, "HGNC", "hgnc")
						xref(j, tentryid, "RefSeq_ncRNA_predicted", "RefSeq")
						xref(j, tentryid, "HAMAP", "HAMAP")

					}
				}
			}
		}

		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}

		//TODO GO
		//TODO PROTEIN FEAUTRES

	}

	// probset biomart
	for _, path := range biomartPaths {
		// first get the probset machine name
		f := strings.Split(path, "/")
		probsetMachine := strings.Split(f[len(f)-1], "__")[1][4:]
		probsetConf := dataconf[probsetMachine]

		if probsetConf != nil {

			br2, _, ftpFile2, localFile2, fr2, _ := d.getDataReaderNew(probsetMachine, ftpAddress, "", path)

			scanner := bufio.NewScanner(br2)
			for scanner.Scan() {
				t := strings.Split(scanner.Text(), "\t")
				if len(t) == 3 && t[2] != "\\N" && t[1] != "\\N" {
					d.addXref(t[2], fr2, t[1], "EnsemblTranscript", false)
				}
			}
			if ftpFile2 != nil {
				ftpFile2.Close()
			}
			if localFile2 != nil {
				localFile2.Close()
			}

		} else {
			log.Println("Warn: new prob mapping found. It must be defined in configuration", probsetMachine)
		}

	}

}
