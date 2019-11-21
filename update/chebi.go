package update

import (
	"encoding/csv"
	"io"
	"log"
	"strings"
	"time"
)

type chebi struct {
	source string
	d      *DataUpdate
}

func (c *chebi) update() {

	defer c.d.wg.Done()
	fr := config.Dataconf[c.source]["id"]
	chebiPath := config.Dataconf[c.source]["path"]
	chebiFiles := strings.Split(config.Dataconf[c.source]["files"], ",")

	//xreftypes := map[string]bool{}

	var previous int64
	totalRead := 0

	for _, name := range chebiFiles {
		br, _, ftpFile, client, localFile, _ := c.d.getDataReaderNew(c.source, c.d.ebiFtp, c.d.ebiFtpPath, chebiPath+name)

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

	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}
