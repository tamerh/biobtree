package update

import (
	"sync/atomic"
	"time"

	"github.com/tamerh/xml-stream-parser"
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
