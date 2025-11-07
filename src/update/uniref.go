package update

import (
	"os"
	"sync/atomic"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type uniref struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for uniref processor
func (u *uniref) check(err error, operation string) {
	checkWithContext(err, u.source, operation)
}

func (u *uniref) update() {

	fr := config.Dataconf[u.source]["id"]

	// Test mode support
	testLimit := config.GetTestLimit(u.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, u.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, config.Dataconf[u.source]["path"])
	check(err)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}

	if client != nil {
		defer client.Quit()
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

				// In production, xrefs to UniParc/UniProt are created bidirectionally from those datasets,
				// so we don't need to create them here to avoid duplicate data for large datasets.
				// In test mode, we only process 100 entries per dataset, so the referenced IDs might not
				// be in the test subset. We create the xrefs here to make UniRef IDs searchable in tests.
				if _, ok = validRefs[z.Attrs["type"]]; ok {
					if config.IsTestMode() {
						u.d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
					}
				}

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

				// Same reasoning as representativeMember: only create in test mode to avoid
				// duplicate data in production while ensuring tests work correctly.
				if _, ok = validRefs[z.Attrs["type"]]; ok {
					if config.IsTestMode() {
						u.d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
					}
				}

				if _, ok = z.Childs["property"]; ok {
					for _, x := range z.Childs["property"] {
						if _, ok = validRefs[x.Attrs["type"]]; ok {
							u.d.addXref(entryid, fr, x.Attrs["value"], x.Attrs["type"], false)
						}
					}
				}

			}
		}

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, entryid)
		}

		total++

		// Check test limit
		if shouldStopProcessing(testLimit, int(total)) {
			break
		}

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}

	atomic.AddUint64(&u.d.totalParsedEntry, total)
	u.d.addEntryStat(u.source, total)

}
