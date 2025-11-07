package update

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type uniparc struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for uniparc processor
func (u *uniparc) check(err error, operation string) {
	checkWithContext(err, u.source, operation)
}

func (u *uniparc) update() {

	defer u.d.wg.Done()

	fr := config.Dataconf[u.source]["id"]
	basePath := config.Dataconf[u.source]["path"]
	filePattern := config.Dataconf[u.source]["filePattern"]

	// Test mode support
	testLimit := config.GetTestLimit(u.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, u.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var files []string

	// Get list of files matching the pattern
	if config.Dataconf[u.source]["useLocalFile"] == "yes" {
		// Local file mode
		localPath := filepath.FromSlash(basePath)
		matches, err := filepath.Glob(filepath.Join(localPath, filePattern))
		check(err)
		files = matches
	} else {
		// FTP mode - list files from FTP
		client := ftpClient(u.d.uniprotFtp)
		err := client.ChangeDir(u.d.uniprotFtpPath + basePath)
		check(err)
		defer client.Quit()

		entries, err := client.List("")
		check(err)

		for _, entry := range entries {
			matched, _ := filepath.Match(filePattern, entry.Name)
			if matched {
				files = append(files, basePath+entry.Name)
			}
		}
	}

	// Sort files to ensure consistent processing order
	sort.Strings(files)

	if len(files) == 0 {
		log.Printf("Warning: No UniParc files found matching pattern %s in %s", filePattern, basePath)
		return
	}

	log.Printf("Processing %d UniParc files", len(files))

	// we are excluding uniprot subreference because they are already coming from uniprot. this may be optional
	propExclusionsRefs := map[string]bool{}
	propExclusionsRefs["UniProtKB/Swiss-Prot"] = true
	propExclusionsRefs["UniProtKB/TrEMBL"] = true

	var totalEntries uint64
	var previous int64
	var totalRead int64

	// Process each file sequentially
	for fileIdx, filePath := range files {
		log.Printf("Processing UniParc file %d/%d: %s", fileIdx+1, len(files), filepath.Base(filePath))

		br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, filePath)
		if err != nil {
			log.Printf("Warning: Failed to retrieve UniParc file %s: %v - skipping", filePath, err)
			continue
		}

		p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"sequence"})

		var v xmlparser.XMLElement
		var ok bool
		var entryid string
		var fileEntries uint64

		for r := range p.Stream() {

			elapsed := int64(time.Since(u.d.start).Seconds())
			totalRead += int64(p.TotalReadSize)
			if elapsed > previous+u.d.progInterval {
				kbytesPerSecond := totalRead / elapsed / 1024
				previous = elapsed
				u.d.progChan <- &progressInfo{
					dataset:         u.source,
					currentKBPerSec: kbytesPerSecond,
				}
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

			// Log ID in test mode
			if idLogFile != nil {
				logProcessedID(idLogFile, entryid)
			}

			fileEntries++

			// Check test limit
			if shouldStopProcessing(testLimit, int(totalEntries+fileEntries)) {
				log.Printf("Test limit reached (%d entries), stopping UniParc processing", totalEntries+fileEntries)
				break
			}
		}

		totalEntries += fileEntries
		log.Printf("Completed UniParc file %d/%d: %s (%d entries)", fileIdx+1, len(files), filepath.Base(filePath), fileEntries)

		// Check if we need to stop processing more files due to test limit
		if shouldStopProcessing(testLimit, int(totalEntries)) {
			log.Printf("Test limit reached after processing file %d/%d (%d total entries)", fileIdx+1, len(files), totalEntries)
			break
		}

		// Close resources for this file
		if gz != nil {
			gz.Close()
		}
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

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}
	atomic.AddUint64(&u.d.totalParsedEntry, totalEntries)
	u.d.addEntryStat(u.source, totalEntries)

	log.Printf("UniParc processing complete: %d total entries from %d files", totalEntries, len(files))
}
