package update

import (
	"biobtree/pbuf"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	xmlparser "github.com/tamerh/xml-stream-parser"
)

type taxonomy struct {
	source string
	d      *DataUpdate
}

func (t *taxonomy) update() {

	var total uint64
	var v, z xmlparser.XMLElement
	var ok bool
	var entryid string
	var previous int64

	fr := config.Dataconf[t.source]["id"]
	frparent := config.Dataconf["taxparent"]["id"]
	frchild := config.Dataconf["taxchild"]["id"]

	br, gz, ftpFile, client, localFile, _ := getDataReaderNew(t.source, t.d.ebiFtp, t.d.ebiFtpPath, config.Dataconf[t.source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer t.d.wg.Done()

	if client != nil {
		defer client.Quit()
	}

	p := xmlparser.NewXMLParser(br, "taxon").SkipElements([]string{"lineage"})

	for r := range p.Stream() {

		elapsed := int64(time.Since(t.d.start).Seconds())
		if elapsed > previous+t.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			t.d.progChan <- &progressInfo{dataset: t.source, currentKBPerSec: kbytesPerSecond}
		}

		// id
		entryid = r.Attrs["taxId"]

		t.d.addXref(r.Attrs["scientificName"], textLinkID, entryid, t.source, true)

		if strings.Contains(r.Attrs["scientificName"], " ") { // to have consistent naming with ensembl
			t.d.addXref(strings.Replace(r.Attrs["scientificName"], " ", "_", -1), textLinkID, entryid, t.source, true)
		}

		attr := pbuf.TaxoAttr{}
		attr.Name = r.Attrs["scientificName"]

		if _, ok := r.Attrs["commonName"]; ok {
			attr.CommonName = r.Attrs["commonName"]
		}

		if _, ok := r.Attrs["rank"]; ok {
			c, err := strconv.Atoi(r.Attrs["rank"])
			if err == nil {
				attr.Rank = int32(c)
			}
		}

		if _, ok := r.Attrs["taxonomicDivision"]; ok {
			attr.TaxonomicDivision = r.Attrs["taxonomicDivision"]
		}
		b, _ := ffjson.Marshal(attr)
		t.d.addProp3(entryid, fr, b)

		//child
		for _, v = range r.Childs["children"] {
			if _, ok = v.Childs["taxon"]; ok {
				for _, z = range v.Childs["taxon"] {
					t.d.addXref2(entryid, fr, z.Attrs["taxId"], "taxchild")
					t.d.addXref2(z.Attrs["taxId"], frchild, z.Attrs["taxId"], "taxonomy") // this always needs for linkdatasets like taxchild,taxparent,gochild etc. In order to automaticly expand during query time.
				}
			}
		}
		//parent
		if _, ok := r.Attrs["parentTaxId"]; ok {
			t.d.addXref2(entryid, fr, r.Attrs["parentTaxId"], "taxparent")
			t.d.addXref2(r.Attrs["parentTaxId"], frparent, r.Attrs["parentTaxId"], "taxonomy")
		}

		total++

	}

	t.d.progChan <- &progressInfo{dataset: t.source, done: true}
	atomic.AddUint64(&t.d.totalParsedEntry, total)
	t.d.addEntryStat(t.source, total)

}
