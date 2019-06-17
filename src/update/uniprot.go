package update

import (
	"sync/atomic"
	"time"

	"github.com/tamerh/xml-stream-parser"
)

type uniprot struct {
	source string
	d      *DataUpdate
}

func (u *uniprot) update() {

	u.d.datasets = append(u.d.datasets, u.source)

	br, gz, ftpFile, localFile, fr, _ := u.d.getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, dataconf[u.source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer u.d.wg.Done()

	p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"comment", "gene", "protein", "feature", "sequence"})

	var total uint64
	var v, x, z xmlparser.XMLElement
	var entryid string
	var previous int64

	for r := range p.Stream() {

		elapsed := int64(time.Since(u.d.start).Seconds())
		if elapsed > previous+u.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			u.d.progChan <- &progressInfo{dataset: u.source, currentKBPerSec: kbytesPerSecond}
		}

		entryid = r.Childs["name"][0].InnerText

		for _, v = range r.Childs["accession"] {
			u.d.addXref(v.InnerText, textLinkID, entryid, u.source, true)
		}

		for _, v = range r.Childs["dbReference"] {

			u.d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["type"], false)

			for _, z := range v.Childs["property"] {
				u.d.addXref(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], z.Attrs["value"], z.Attrs["type"], false)
			}

		}

		for _, v = range r.Childs["organism"] {
			for _, z = range v.Childs["dbReference"] {

				u.d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
				for _, x := range z.Childs["property"] {
					u.d.addXref(z.Attrs["id"], dataconf[z.Attrs["type"]]["id"], x.Attrs["value"], x.Attrs["type"], false)
				}
			}
		}

		for _, v = range r.Childs["reference"] {
			for _, z = range v.Childs["citation"] {
				for _, x = range z.Childs["dbReference"] {

					u.d.addXref(entryid, fr, x.Attrs["id"], x.Attrs["type"], false)
					/**
					if _, ok := x.Childs["property"]; ok {
						for _, t := range x.Childs["property"] {
							u.d.addXref(entryid, fr, t.Attrs["value"], t.Attrs["type"], false)
						}
					}
					*/
				}
			}
		}

		total++

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}

	atomic.AddUint64(&u.d.totalParsedEntry, total)
	u.d.addEntryStat(u.source, total)

}
