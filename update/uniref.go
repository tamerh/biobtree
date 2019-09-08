package update

import (
	"sync/atomic"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type uniref struct {
	source string
	d      *DataUpdate
}

func (u *uniref) update() {

	fr := config.Dataconf[u.source]["id"]
	br, gz, ftpFile, localFile, _ := u.d.getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, config.Dataconf[u.source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}

	defer gz.Close()
	defer u.d.wg.Done()

	p := xmlparser.NewXMLParser(br, "entry")

	// todo representative member and normal member should be differentiated.
	// for uniref uniprot and uniparc references are active
	validRefs := map[string]bool{}
	validRefs["UniParc ID"] = true
	validRefs["UniProtKB accession"] = true

	var total uint64
	var v, z xmlparser.XMLElement
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
		entryid = r.Attrs["id"]

		// representativeMember--> dbreference
		for _, v = range r.Childs["representativeMember"] {
			for _, z = range v.Childs["dbReference"] {

				/**
				if _, ok = validRefs[z.Attrs["type"]]; ok {
					u.d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
				}
				**/

			
				if _, ok = z.Childs["property"]; ok {
					for _, x := range z.Childs["property"] {
						if _, ok = validRefs[x.Attrs["type"]]; ok {
							u.d.addXref(entryid, fr, x.Attrs["value"], x.Attrs["type"], false)
						}
					}
				}
			
			}
		}

		// member --> dbreference
		for _, v = range r.Childs["member"] {
			for _, z = range v.Childs["dbReference"] {

				/**
				if _, ok = validRefs[z.Attrs["type"]]; ok {
					u.d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
				}			**/
				
				if _, ok = z.Childs["property"]; ok {
					for _, x := range z.Childs["property"] {
						if _, ok = validRefs[x.Attrs["type"]]; ok {
							u.d.addXref(entryid, fr, x.Attrs["value"], x.Attrs["type"], false)
						}
					}
				}
	
			}
		}

		total++

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}

	atomic.AddUint64(&u.d.totalParsedEntry, total)
	u.d.addEntryStat(u.source, total)

}
