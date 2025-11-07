package update

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type chebi struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for chebi processor
func (c *chebi) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

func (c *chebi) update() {

	defer c.d.wg.Done()
	fr := config.Dataconf[c.source]["id"]
	chebiPath := config.Dataconf[c.source]["path"]
	chebiFiles := strings.Split(config.Dataconf[c.source]["files"], ",")

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Track unique ChEBI IDs (same ID may appear for multiple xrefs)
	seenIDs := make(map[string]bool)
	var uniqueCount uint64

	//xreftypes := map[string]bool{}

	var previous int64
	totalRead := 0

	for _, name := range chebiFiles {
		br, _, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, c.d.ebiFtp, c.d.ebiFtpPath, chebiPath+name)
		check(err)

		r := csv.NewReader(br)
		r.Comma = '	'

		for {
			record, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			// Skip header row
			if record[3] == "TYPE" || record[1] == "compound_id" {
				continue
			}

			entryid := "CHEBI:" + record[1]

			// Log ID in test mode (only once per unique ID)
			// Do this BEFORE checking if target database is configured
			if idLogFile != nil && !seenIDs[entryid] {
				logProcessedID(idLogFile, entryid)
				seenIDs[entryid] = true
				uniqueCount++

				// Check test limit right after logging new ID
				if shouldStopProcessing(testLimit, int(uniqueCount)) {
					goto done
				}
			}

			if _, ok := config.Dataconf[record[3]]; !ok {
				continue
			}

			c.d.addXref(entryid, fr, record[4], record[3], false)

			for _, r := range record {
				totalRead = totalRead + len(r)
			}

			elapsed := int64(time.Since(c.d.start).Seconds())
			if elapsed > previous+c.d.progInterval {
				kbytesPerSecond := int64(totalRead) / elapsed / 1024
				previous = elapsed
				c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
			}

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
		if client != nil {
			client.Quit()
		}

	}

done:
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}

	atomic.AddUint64(&c.d.totalParsedEntry, uniqueCount)
	c.d.addEntryStat(c.source, uniqueCount)
}
