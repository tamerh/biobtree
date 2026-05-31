package update

import (
	"biobtree/pbuf"
	"encoding/csv"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type clingenGeneValidity struct {
	source string
	d      *DataUpdate
}

func (c *clingenGeneValidity) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// extractAssertionID pulls the ClinGen assertion UUID out of an online-report URL
// like ".../CGGV:assertion_92de3832-c272-4993-8586-288c6331dec2-2024-03-14T16...".
// Returns the 36-char UUID, or "" if not present.
func extractAssertionID(reportURL string) string {
	const marker = "assertion_"
	idx := strings.Index(reportURL, marker)
	if idx < 0 {
		return ""
	}
	tail := reportURL[idx+len(marker):]
	if len(tail) < 36 {
		return ""
	}
	return tail[:36] // standard UUID length (8-4-4-4-12)
}

func (c *clingenGeneValidity) update() {
	defer c.d.wg.Done()

	log.Println("ClinGen GeneValidity: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[c.source]["id"]
	path := config.Dataconf[c.source]["path"]

	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("ClinGen GeneValidity: [TEST MODE] Processing up to %d entries", testLimit)
	}

	br, cleanup, err := openClingenReader(c.source, path)
	c.check(err, "opening ClinGen gene-validity data")
	defer cleanup()

	reader := csv.NewReader(br)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	// Locate the header row (first field == "GENE SYMBOL"); preceding rows are banners.
	colMap := make(map[string]int)
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			log.Printf("ClinGen GeneValidity: header row not found")
			c.d.progChan <- &progressInfo{dataset: c.source, done: true}
			return
		}
		if err != nil {
			continue
		}
		if len(rec) > 0 && strings.TrimSpace(rec[0]) == "GENE SYMBOL" {
			for i, name := range rec {
				colMap[strings.TrimSpace(name)] = i
			}
			break
		}
	}

	col := func(fields []string, name string) string {
		if idx, ok := colMap[name]; ok && idx < len(fields) {
			return strings.TrimSpace(fields[idx])
		}
		return ""
	}

	var total uint64
	var entryCount int64
	var previous int64
	var skippedNoID int

	for {
		fields, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if len(fields) == 0 {
			continue
		}
		// Skip the "+++++" separator row that follows the header.
		if strings.HasPrefix(strings.TrimSpace(fields[0]), "+") || strings.TrimSpace(fields[0]) == "" {
			continue
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: 0}
		}

		geneSymbol := col(fields, "GENE SYMBOL")
		hgncID := col(fields, "GENE ID (HGNC)")
		diseaseLabel := col(fields, "DISEASE LABEL")
		mondoID := col(fields, "DISEASE ID (MONDO)")
		reportURL := col(fields, "ONLINE REPORT")

		assertionID := extractAssertionID(reportURL)
		if assertionID == "" {
			// Fallback: synthesize a stable key from gene + disease.
			if geneSymbol != "" && mondoID != "" {
				assertionID = geneSymbol + "_" + mondoID
			} else {
				skippedNoID++
				continue
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, assertionID)
		}

		attr := pbuf.ClingenGeneValidityAttr{
			GeneSymbol:         geneSymbol,
			GeneHgncId:         hgncID,
			DiseaseLabel:       diseaseLabel,
			DiseaseMondoId:     mondoID,
			Moi:                col(fields, "MOI"),
			Sop:                col(fields, "SOP"),
			Classification:     col(fields, "CLASSIFICATION"),
			Gcep:               col(fields, "GCEP"),
			ClassificationDate: col(fields, "CLASSIFICATION DATE"),
			ReportUrl:          reportURL,
		}

		attrBytes, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("ClinGen GeneValidity: Error marshaling %s: %v", assertionID, err)
			continue
		}
		c.d.addProp3(assertionID, sourceID, attrBytes)

		// Text search
		if geneSymbol != "" {
			c.d.addXref(geneSymbol, textLinkID, assertionID, c.source, true)
		}
		if diseaseLabel != "" {
			c.d.addXref(diseaseLabel, textLinkID, assertionID, c.source, true)
		}

		// Gene cross-references (HGNC/Entrez/Ensembl via symbol lookup)
		if geneSymbol != "" {
			c.d.addHumanGeneXrefsAll(geneSymbol, assertionID, sourceID)
		}
		if strings.HasPrefix(hgncID, "HGNC:") {
			c.d.addXref(assertionID, sourceID, hgncID, "hgnc", false)
		}

		// Disease cross-reference (MONDO)
		clingenDiseaseXref(c.d, assertionID, sourceID, mondoID)

		total++
		entryCount++
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			break
		}
	}

	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
	atomic.AddUint64(&c.d.totalParsedEntry, total)

	log.Printf("ClinGen GeneValidity: Processing complete - %d entries saved (%.2fs)", total, time.Since(startTime).Seconds())
	if skippedNoID > 0 {
		log.Printf("ClinGen GeneValidity: Skipped %d rows with no usable id", skippedNoID)
	}
}
