package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type gencc struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for gencc processor
func (g *gencc) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

func (g *gencc) update() {
	defer g.d.wg.Done()

	log.Println("GenCC: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[g.source]["id"]
	path := config.Dataconf[g.source]["path"]

	// Test mode support
	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("GenCC: [TEST MODE] Processing up to %d entries", testLimit)
	}

	var br *bufio.Reader
	var httpResp *http.Response
	var localFile *os.File

	// Support both local files and HTTP(S) downloads
	if config.Dataconf[g.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		g.check(err, "opening local file")
		br = bufio.NewReaderSize(file, fileBufSize)
		localFile = file
		defer localFile.Close()
	} else if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		log.Printf("GenCC: Downloading from %s", path)
		resp, err := http.Get(path)
		g.check(err, "downloading GenCC data")
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		httpResp = resp
		defer httpResp.Body.Close()
	} else {
		// Fall back to FTP for backward compatibility
		// Path is now a full URL
		br2, _, ftpFile, client, localFile2, _, err := getDataReaderNew(g.source, "", "", path)
		g.check(err, "opening FTP file")
		br = br2
		if ftpFile != nil {
			defer ftpFile.Close()
		}
		if localFile2 != nil {
			defer localFile2.Close()
		}
		if client != nil {
			defer client.Quit()
		}
	}

	// Create scanner for TSV format
	scanner := bufio.NewScanner(br)

	// Increase buffer size for long lines
	const maxCapacity = 512 * 1024 // 512KB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and parse header
	if !scanner.Scan() {
		g.check(scanner.Err(), "reading GenCC header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices (strip quotes from column names)
	colMap := make(map[string]int)
	for i, name := range header {
		// Strip quotes and whitespace from column names
		name = strings.TrimSpace(name)
		name = strings.Trim(name, "\"")
		colMap[name] = i
	}

	log.Printf("GenCC: Found %d columns in header", len(colMap))

	var total uint64
	var entryCount int64
	var previous int64
	var skippedNoUUID int
	var skippedNoGene int
	var skippedMalformed int

	// Expected number of columns in GenCC TSV
	const expectedColumns = 30

	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Quick check: skip lines that are likely continuation lines from multiline notes
		// These won't have enough tabs to be valid rows
		tabCount := strings.Count(line, "\t")
		if tabCount < expectedColumns-1 {
			skippedMalformed++
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(g.d.start).Seconds())
		if elapsed > previous+g.d.progInterval {
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: 0}
		}

		// Split by tab
		fields := strings.Split(line, "\t")

		// Helper function to safely get field value (strips quotes)
		getField := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(fields) {
				val := strings.TrimSpace(fields[idx])
				val = strings.Trim(val, "\"")
				return val
			}
			return ""
		}

		// Get UUID as entry ID
		uuid := getField("uuid")
		if uuid == "" {
			skippedNoUUID++
			continue
		}

		// Get gene information
		geneCurie := getField("gene_curie")
		geneSymbol := getField("gene_symbol")
		if geneCurie == "" && geneSymbol == "" {
			skippedNoGene++
			continue
		}

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, uuid)
		}

		// Build attributes
		attr := pbuf.GenccAttr{
			Uuid:                       uuid,
			GeneCurie:                  geneCurie,
			GeneSymbol:                 geneSymbol,
			DiseaseCurie:               getField("disease_curie"),
			DiseaseTitle:               getField("disease_title"),
			ClassificationCurie:        getField("classification_curie"),
			ClassificationTitle:        getField("classification_title"),
			MoiCurie:                   getField("moi_curie"),
			MoiTitle:                   getField("moi_title"),
			SubmitterCurie:             getField("submitter_curie"),
			SubmitterTitle:             getField("submitter_title"),
			SubmittedAsDate:            getField("submitted_as_date"),
			SubmittedAsPublicReportUrl: getField("submitted_as_public_report_url"),
		}

		// Parse PMIDs (can be comma or semicolon separated)
		pmidsStr := getField("submitted_as_pmids")
		if pmidsStr != "" {
			// Replace semicolons with commas to normalize
			pmidsStr = strings.ReplaceAll(pmidsStr, ";", ",")
			pmids := strings.Split(pmidsStr, ",")
			for _, pmid := range pmids {
				pmid = strings.TrimSpace(pmid)
				// Skip empty values and non-numeric PMIDs
				if pmid != "" && len(pmid) > 0 && pmid[0] >= '0' && pmid[0] <= '9' {
					attr.SubmittedAsPmids = append(attr.SubmittedAsPmids, pmid)
				}
			}
		}

		// Serialize attributes
		attrBytes, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("GenCC: Error marshaling attributes for %s: %v", uuid, err)
			continue
		}

		// Save entry with attributes
		g.d.addProp3(uuid, sourceID, attrBytes)

		// Create cross-references

		// Gene symbol as text search
		if geneSymbol != "" {
			g.d.addXref(geneSymbol, textLinkID, uuid, g.source, true)
		}

		// Disease title as text search
		diseaseTitle := getField("disease_title")
		if diseaseTitle != "" {
			g.d.addXref(diseaseTitle, textLinkID, uuid, g.source, true)
		}

		// Classification title as text search
		classificationTitle := getField("classification_title")
		if classificationTitle != "" {
			g.d.addXref(classificationTitle, textLinkID, uuid, g.source, true)
		}

		// Cross-reference to Ensembl via HGNC (using gene symbol lookup)
		// In production, HGNC is part of Ensembl, so we use addXrefEnsemblViaHgnc
		if geneSymbol != "" {
			g.d.addXrefEnsemblViaHgnc(geneSymbol, uuid, sourceID)
		}

		// Direct cross-reference to HGNC (gene_curie field contains "HGNC:1100" format)
		if geneCurie != "" && strings.HasPrefix(geneCurie, "HGNC:") {
			g.d.addXref(uuid, sourceID, geneCurie, "hgnc", false)
		}

		// Cross-reference to disease ontologies
		diseaseCurie := getField("disease_curie")
		if diseaseCurie != "" {
			if strings.HasPrefix(diseaseCurie, "MONDO:") {
				g.d.addXref(uuid, sourceID, diseaseCurie, "mondo", false)
			} else if strings.HasPrefix(diseaseCurie, "OMIM:") {
				g.d.addXref(uuid, sourceID, diseaseCurie, "MIM", false)
			} else if strings.HasPrefix(diseaseCurie, "Orphanet:") {
				g.d.addXref(uuid, sourceID, diseaseCurie, "orphanet", false)
			}
		}

		// Cross-reference to HPO (mode of inheritance)
		moiCurie := getField("moi_curie")
		if moiCurie != "" && strings.HasPrefix(moiCurie, "HP:") {
			g.d.addXref(uuid, sourceID, moiCurie, "hpo", false)
		}

		// Cross-reference to PubMed
		for _, pmid := range attr.SubmittedAsPmids {
			g.d.addXref(uuid, sourceID, pmid, "PubMed", false)
		}

		total++
		entryCount++

		// Test mode: check if limit reached
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("GenCC: Error reading data: %v", err)
	}

	g.d.progChan <- &progressInfo{dataset: g.source, done: true}

	atomic.AddUint64(&g.d.totalParsedEntry, total)

	log.Printf("GenCC: Processing complete - %d entries saved (%.2fs)", total, time.Since(startTime).Seconds())
	if skippedMalformed > 0 {
		log.Printf("GenCC: Skipped %d malformed lines (multiline notes continuation)", skippedMalformed)
	}
	if skippedNoUUID > 0 {
		log.Printf("GenCC: Skipped %d entries with no UUID", skippedNoUUID)
	}
	if skippedNoGene > 0 {
		log.Printf("GenCC: Skipped %d entries with no gene information", skippedNoGene)
	}
}
