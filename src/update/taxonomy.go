package update

import (
	"biobtree/src/pbuf"
	"sync/atomic"
	"time"

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
	attr := pbuf.XrefAttr{}

	br, gz, ftpFile, localFile, fr, _ := t.d.getDataReaderNew(t.source, t.d.ebiFtp, t.d.ebiFtpPath, dataconf[t.source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer t.d.wg.Done()

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

		attr.Values = nil
		attr.Key = "name"
		attr.Values = append(attr.Values, r.Attrs["scientificName"])
		t.d.addProp2(entryid, fr, &attr)

		if _, ok := r.Attrs["commonName"]; ok {
			attr.Values = nil
			attr.Key = "commonName"
			attr.Values = append(attr.Values, r.Attrs["commonName"])
			t.d.addProp2(entryid, fr, &attr)
		}

		if _, ok := r.Attrs["rank"]; ok {
			attr.Values = nil
			attr.Key = "rank"
			attr.Values = append(attr.Values, r.Attrs["rank"])
			t.d.addProp2(entryid, fr, &attr)
		}

		if _, ok := r.Attrs["taxonomicDivision"]; ok {
			attr.Values = nil
			attr.Key = "taxonomicDivision"
			attr.Values = append(attr.Values, r.Attrs["taxonomicDivision"])
			t.d.addProp2(entryid, fr, &attr)
		}

		//dbreference
		for _, v = range r.Childs["children"] {
			if _, ok = v.Childs["taxon"]; ok {
				for _, z = range v.Childs["taxon"] {
					t.d.addXref(entryid, fr, z.Attrs["taxId"], t.source, false)
				}
			}
		}

		total++

	}

	t.d.progChan <- &progressInfo{dataset: t.source, done: true}
	atomic.AddUint64(&t.d.totalParsedEntry, total)
	t.d.addEntryStat(t.source, total)

}
