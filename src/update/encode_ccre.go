package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// encode_ccre handles ENCODE cCRE (candidate cis-Regulatory Elements) data
// Source: https://screen.encodeproject.org/
// Format: BED9+1 (10 columns)
// ~2.3M regulatory regions with classifications (PLS, pELS, dELS, CA-CTCF, CA-TF, CA, TF)
type encode_ccre struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking
func (e *encode_ccre) check(err error, operation string) {
	checkWithContext(err, e.source, operation)
}

func (e *encode_ccre) update() {
	defer e.d.wg.Done()

	log.Printf("[%s] Starting ENCODE cCRE data integration...", e.source)
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(e.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, e.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("[%s] [TEST MODE] Processing up to %d entries", e.source, testLimit)
	}

	// Process the BED file
	total := e.processBedFile(testLimit, idLogFile)

	log.Printf("[%s] Completed in %.2fs, processed %d cCREs",
		e.source, time.Since(startTime).Seconds(), total)

	atomic.AddUint64(&e.d.totalParsedEntry, uint64(total))
	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
}

func (e *encode_ccre) processBedFile(testLimit int, idLogFile *os.File) int {
	filePath := config.Dataconf[e.source]["path"]
	log.Printf("[%s] Loading cCRE data from: %s", e.source, filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(
		e.source, "", "", filePath)
	e.check(err, "opening BED file")

	defer func() {
		if gz != nil {
			gz.Close()
		}
		if ftpFile != nil {
			ftpFile.Close()
		}
		if client != nil {
			client.Quit()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	sourceID := config.Dataconf[e.source]["id"]

	scanner := bufio.NewScanner(br)
	// Increase buffer for potentially long lines
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var total int
	var previous int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 10 {
			continue
		}

		// Parse BED fields
		// Format: chr, start, end, name, score, strand, thickStart, thickEnd, itemRgb, ccreClass
		chrom := fields[0]
		start, err1 := strconv.ParseInt(fields[1], 10, 64)
		end, err2 := strconv.ParseInt(fields[2], 10, 64)
		ccreID := fields[3]    // EH38E2776516
		ccreClass := fields[9] // pELS, dELS, PLS, CA-CTCF, CA-TF, CA, TF

		if err1 != nil || err2 != nil {
			continue
		}

		// Create attribute
		attr := &pbuf.EncodeCcreAttr{
			CcreId:     ccreID,
			CcreClass:  ccreClass,
			Chromosome: chrom,
			Start:      start,
			End:        end,
		}

		// Save entry
		attrBytes, err := ffjson.Marshal(attr)
		e.check(err, "marshaling cCRE attributes")
		e.d.addProp3(ccreID, sourceID, attrBytes)

		// Create cross-references
		e.createCrossReferences(ccreID, sourceID, attr)

		total++

		// Progress tracking
		if total%100000 == 0 {
			log.Printf("[%s] Processed %d cCREs...", e.source, total)
		}

		elapsed := int64(time.Since(e.d.start).Seconds())
		if elapsed > previous+e.d.progInterval {
			previous = elapsed
			e.d.progChan <- &progressInfo{
				dataset:         e.source,
				currentKBPerSec: int64(total / int(elapsed+1)),
			}
		}

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, ccreID)
		}

		// Test limit
		if testLimit > 0 && total >= testLimit {
			log.Printf("[%s] [TEST MODE] Reached limit of %d", e.source, testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[%s] Scanner error: %v", e.source, err)
	}

	return total
}

func (e *encode_ccre) createCrossReferences(ccreID, sourceID string,
	attr *pbuf.EncodeCcreAttr) {

	// 1. Text search by cCRE ID
	e.d.addXref(ccreID, textLinkID, ccreID, e.source, true)

	// 2. Text search by classification (enables searching "PLS" or "pELS")
	if attr.CcreClass != "" {
		e.d.addXref(attr.CcreClass, textLinkID, ccreID, e.source, true)
	}

	// 3. Cross-reference to taxonomy (human only - GRCh38)
	e.d.addXref(ccreID, sourceID, "9606", "taxonomy", false)
}
