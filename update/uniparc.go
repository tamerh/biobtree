package update

import (
	"sync/atomic"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type uniparc struct {
	source string
	d      *DataUpdate
}

func (u *uniparc) update() {

	fr := config.Dataconf[u.source]["id"]
	br, gz, ftpFile, client, localFile, _ := u.d.getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, config.Dataconf[u.source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer u.d.wg.Done()

	if client != nil {
		defer client.Quit()
	}

	p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"sequence"})

	// we are excluding uniprot subreference because they are already coming from uniprot. this may be optional
	propExclusionsRefs := map[string]bool{}
	propExclusionsRefs["UniProtKB/Swiss-Prot"] = true
	propExclusionsRefs["UniProtKB/TrEMBL"] = true

	var total uint64
	var v xmlparser.XMLElement
	var ok bool
	var entryid string
	var previous int64

	for r := range p.Stream() {

		elapsed := int64(time.Since(u.d.start).Seconds())
		if elapsed > previous+u.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			u.d.progChan <- &progressInfo{dataset: u.source, currentKBPerSec: kbytesPerSecond}
		}

		// id
		entryid = r.Childs["accession"][0].InnerText

		//dbreference
		for _, v = range r.Childs["dbReference"] {

			u.d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["type"], false)

			if _, ok = propExclusionsRefs[v.Attrs["type"]]; !ok {
				for _, z := range v.Childs["property"] {
					u.d.addXref(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], z.Attrs["value"], z.Attrs["type"], false)
				}
			}

		}
		// signatureSequenceMatch
		/**
		for _, v = range r.Elements["signatureSequenceMatch"] {

			u.d.addXref(entryid, fr, v.Attrs["id"], v.Attrs["database"], false)

			if _, ok = v.Childs["ipr"]; ok {
				for _, z = range v.Childs["ipr"] {
					u.d.addXref(entryid, fr, z.Attrs["id"], "INTERPRO", false)
				}
			}
		}
		*/

		total++

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}
	atomic.AddUint64(&u.d.totalParsedEntry, total)
	u.d.addEntryStat(u.source, total)
}
