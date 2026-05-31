package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type clingenDosage struct {
	source string
	d      *DataUpdate
}

func (c *clingenDosage) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c *clingenDosage) update() {
	defer c.d.wg.Done()

	log.Println("ClinGen Dosage: Starting data processing...")
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
		log.Printf("ClinGen Dosage: [TEST MODE] Processing up to %d entries", testLimit)
	}

	br, cleanup, err := openClingenReader(c.source, path)
	c.check(err, "opening ClinGen dosage data")
	defer cleanup()

	scanner := bufio.NewScanner(br)
	const maxCapacity = 512 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	// Header is the comment line beginning "#Gene Symbol".
	colMap := make(map[string]int)
	pmidCols := []string{
		"Haploinsufficiency PMID1", "Haploinsufficiency PMID2", "Haploinsufficiency PMID3",
		"Haploinsufficiency PMID4", "Haploinsufficiency PMID5", "Haploinsufficiency PMID6",
		"Triplosensitivity PMID1", "Triplosensitivity PMID2", "Triplosensitivity PMID3",
		"Triplosensitivity PMID4", "Triplosensitivity PMID5", "Triplosensitivity PMID6",
	}

	var total uint64
	var entryCount int64
	var previous int64
	var skippedNoGene int

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#Gene Symbol") {
				header := strings.Split(strings.TrimPrefix(line, "#"), "\t")
				for i, name := range header {
					colMap[strings.TrimSpace(name)] = i
				}
			}
			continue
		}
		if len(colMap) == 0 {
			continue // header not seen yet
		}

		fields := strings.Split(line, "\t")
		col := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(fields) {
				return strings.TrimSpace(fields[idx])
			}
			return ""
		}

		geneID := col("Gene ID")
		geneSymbol := col("Gene Symbol")
		if !isNumericID(geneID) {
			skippedNoGene++
			continue
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: 0}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, geneID)
		}

		hiDisease := col("Haploinsufficiency Disease ID")
		tsDisease := col("Triplosensitivity Disease ID")

		attr := pbuf.ClingenDosageAttr{
			GeneSymbol:        geneSymbol,
			GeneId:            geneID,
			Cytoband:          col("cytoBand"),
			GenomicLocation:   col("Genomic Location"),
			HaploScore:        col("Haploinsufficiency Score"),
			HaploLabel:        col("Haploinsufficiency Description"),
			HaploDiseaseId:    hiDisease,
			TriploScore:       col("Triplosensitivity Score"),
			TriploLabel:       col("Triplosensitivity Description"),
			TriploDiseaseId:   tsDisease,
			DateLastEvaluated: col("Date Last Evaluated"),
		}

		attrBytes, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("ClinGen Dosage: Error marshaling %s: %v", geneID, err)
			continue
		}
		c.d.addProp3(geneID, sourceID, attrBytes)

		// Text search by gene symbol
		if geneSymbol != "" {
			c.d.addXref(geneSymbol, textLinkID, geneID, c.source, true)
		}

		// Gene cross-references: direct Entrez + HGNC/Ensembl via symbol lookup
		c.d.addXref(geneID, sourceID, geneID, "entrez", false)
		if geneSymbol != "" {
			c.d.addHumanGeneXrefsAll(geneSymbol, geneID, sourceID)
		}

		// Disease cross-references (HI and TS disease ids, MONDO/OMIM prefixed)
		clingenDiseaseXref(c.d, geneID, sourceID, hiDisease)
		clingenDiseaseXref(c.d, geneID, sourceID, tsDisease)

		// PubMed evidence
		for _, pc := range pmidCols {
			pmid := col(pc)
			if isNumericID(pmid) {
				c.d.addXref(geneID, sourceID, pmid, "pubmed", false)
			}
		}

		total++
		entryCount++
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ClinGen Dosage: Error reading data: %v", err)
	}

	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
	atomic.AddUint64(&c.d.totalParsedEntry, total)

	log.Printf("ClinGen Dosage: Processing complete - %d entries saved (%.2fs)", total, time.Since(startTime).Seconds())
	if skippedNoGene > 0 {
		log.Printf("ClinGen Dosage: Skipped %d rows with no numeric Gene ID", skippedNoGene)
	}
}
