package update

import (
	"archive/zip"
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type gwas struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (g *gwas) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

// Main update entry point
func (g *gwas) update() {
	defer g.d.wg.Done()

	log.Println("GWAS Associations: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("GWAS Associations: [TEST MODE] Processing up to %d associations", testLimit)
	}

	// Process GWAS Catalog associations file
	g.parseAndSaveAssociations(testLimit, idLogFile)

	log.Printf("GWAS Associations: Processing complete (%.2fs)", time.Since(startTime).Seconds())
}

// parseAndSaveAssociations processes the GWAS Catalog associations ZIP file
func (g *gwas) parseAndSaveAssociations(testLimit int, idLogFile *os.File) {
	// Build file path
	filePath := config.Dataconf[g.source]["path"]
	log.Printf("GWAS Associations: Downloading via EBI FTP from %s", filePath)

	// Open ZIP file via EBI FTP
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, g.d.ebiFtp, g.d.ebiFtpPath, filePath)
	g.check(err, "opening GWAS Associations ZIP file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[g.source]["id"]

	// The file is a ZIP, need to extract the TSV inside
	// First, read the entire ZIP into memory (needed for zip.NewReader)
	log.Println("GWAS Associations: Reading ZIP file...")
	zipData, err := io.ReadAll(br)
	g.check(err, "reading ZIP file")

	log.Printf("GWAS Associations: ZIP file size: %.2f MB", float64(len(zipData))/1024/1024)

	// Open ZIP
	zipReader, err := zip.NewReader(readerAtFromBytes(zipData), int64(len(zipData)))
	g.check(err, "opening ZIP archive")

	if len(zipReader.File) == 0 {
		log.Fatal("GWAS Associations: ZIP file is empty")
	}

	// Find the TSV file inside (should be only one file)
	var tsvFile *zip.File
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".tsv") || strings.HasSuffix(f.Name, ".txt") {
			tsvFile = f
			break
		}
	}

	if tsvFile == nil {
		log.Fatal("GWAS Associations: No TSV file found in ZIP")
	}

	log.Printf("GWAS Associations: Found file in ZIP: %s (%.2f MB uncompressed)",
		tsvFile.Name, float64(tsvFile.UncompressedSize64)/1024/1024)

	// Open the TSV file from ZIP
	rc, err := tsvFile.Open()
	g.check(err, "opening TSV from ZIP")
	defer rc.Close()

	// Create scanner for TSV format (CRITICAL: use Scanner like gwas_study, NOT csv.Reader)
	scanner := bufio.NewScanner(rc)

	// Increase buffer size for long lines (some trait descriptions are very long)
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and parse header
	if !scanner.Scan() {
		g.check(scanner.Err(), "reading GWAS Associations header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	log.Printf("GWAS Associations: Found %d columns in header", len(colMap))

	// Save each association individually with composite key: STUDYID_RSID
	// This avoids ID collision with dbSNP and preserves all associations
	var savedAssociations int
	var previous int64
	var skippedEmptySNP int
	var skippedEmptyStudy int
	var totalRowsRead int

	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		totalRowsRead++

		// Split by tab (manual parsing like gwas_study and rhea)
		row := strings.Split(line, "\t")

		// Progress logging every 10000 rows
		if totalRowsRead%10000 == 0 {
			log.Printf("GWAS Associations: Read %d rows so far (line %d)...", totalRowsRead, lineNum)
		}

		// Progress tracking
		elapsed := int64(time.Since(g.d.start).Seconds())
		if elapsed > previous+g.d.progInterval {
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: int64(savedAssociations / int(elapsed))}
		}

		// Extract SNP ID and Study Accession
		snpID := getField(row, colMap, "SNPS")
		if snpID == "" {
			skippedEmptySNP++
			continue
		}

		studyAccession := getField(row, colMap, "STUDY ACCESSION")
		if studyAccession == "" {
			skippedEmptyStudy++
			continue
		}

		// Build association entry
		assoc := g.buildAssociation(row, colMap, snpID)
		if assoc == nil {
			continue
		}

		// Create composite key: STUDYID_RSID (e.g., "GCST000001_rs7903146")
		// This ensures uniqueness and avoids collision with dbSNP entries
		associationID := studyAccession + "_" + snpID

		// Save association with composite key
		g.saveAssociation(associationID, snpID, assoc, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(associationID + "\n")
		}

		savedAssociations++

		// Test mode: check limit
		if testLimit > 0 && savedAssociations >= testLimit {
			log.Printf("GWAS Associations: [TEST MODE] Reached limit of %d associations, stopping", testLimit)
			break
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("GWAS Associations: Scanner error: %v", err)
	}

	log.Printf("GWAS Associations: Total rows read: %d, Saved: %d associations", totalRowsRead, savedAssociations)
	log.Printf("GWAS Associations: Skipped - empty SNP: %d, empty study: %d", skippedEmptySNP, skippedEmptyStudy)

	// Update entry statistics
	atomic.AddUint64(&g.d.totalParsedEntry, uint64(savedAssociations))
	g.d.addEntryStat(g.source, uint64(savedAssociations))
}

// buildAssociation creates a single association entry from row
func (g *gwas) buildAssociation(row []string, colMap map[string]int, snpID string) *pbuf.GwasAttr {
	// Extract study accession
	studyAccession := getField(row, colMap, "STUDY ACCESSION")

	// Parse chromosome position
	var chrPos int64
	chrPosStr := getField(row, colMap, "CHR_POS")
	if chrPosStr != "" {
		if pos, err := strconv.ParseInt(chrPosStr, 10, 64); err == nil {
			chrPos = pos
		}
	}

	// Parse p-value
	var pValue float64
	pValueStr := getField(row, colMap, "P-VALUE")
	if pValueStr != "" {
		if p, err := strconv.ParseFloat(pValueStr, 64); err == nil {
			pValue = p
		}
	}

	// Parse pvalue_mlog
	var pValueMlog float64
	pValueMlogStr := getField(row, colMap, "PVALUE_MLOG")
	if pValueMlogStr != "" {
		if pm, err := strconv.ParseFloat(pValueMlogStr, 64); err == nil {
			pValueMlog = pm
		}
	}

	// Parse intergenic flag
	intergenic := false
	if getField(row, colMap, "INTERGENIC") == "1" {
		intergenic = true
	}

	// Extract reported genes (comma-separated)
	reportedGenes := splitAndClean(getField(row, colMap, "REPORTED GENE(S)"), ",")

	// Extract mapped traits (comma-separated)
	mappedTraits := splitAndClean(getField(row, colMap, "MAPPED_TRAIT"), ",")

	// Extract EFO trait URIs and convert to EFO IDs
	traitURIs := getField(row, colMap, "MAPPED_TRAIT_URI")
	var efoIDs []string
	if traitURIs != "" {
		for _, uri := range splitAndClean(traitURIs, ",") {
			efoID := extractEFOID(uri)
			if efoID != "" {
				efoIDs = append(efoIDs, efoID)
			}
		}
	}

	// Extract SNP gene IDs (Ensembl gene IDs, comma-separated)
	snpGeneIDs := splitAndClean(getField(row, colMap, "SNP_GENE_IDS"), ",")

	// Build association object
	attr := &pbuf.GwasAttr{
		SnpId:                  snpID,
		StudyAccession:         studyAccession,
		StrongestSnpRiskAllele: getField(row, colMap, "STRONGEST SNP-RISK ALLELE"),
		ChrId:                  getField(row, colMap, "CHR_ID"),
		ChrPos:                 chrPos,
		Region:                 getField(row, colMap, "REGION"),
		Context:                getField(row, colMap, "CONTEXT"),
		Intergenic:             intergenic,
		ReportedGenes:          reportedGenes,
		MappedGene:             getField(row, colMap, "MAPPED_GENE"),
		UpstreamGeneId:         getField(row, colMap, "UPSTREAM_GENE_ID"),
		DownstreamGeneId:       getField(row, colMap, "DOWNSTREAM_GENE_ID"),
		SnpGeneIds:             snpGeneIDs,
		DiseaseTrait:           getField(row, colMap, "DISEASE/TRAIT"),
		MappedTraits:           mappedTraits,
		EfoTraits:              efoIDs,
		PValue:                 pValue,
		PvalueMlog:             pValueMlog,
		OrBeta:                 getField(row, colMap, "OR or BETA"),
		Ci_95:                  getField(row, colMap, "95% CI (TEXT)"),
		RiskAlleleFrequency:    getField(row, colMap, "RISK ALLELE FREQUENCY"),
		PubmedId:               getField(row, colMap, "PUBMEDID"),
		FirstAuthor:            getField(row, colMap, "FIRST AUTHOR"),
		Date:                   getField(row, colMap, "DATE"),
		Platform:               getField(row, colMap, "PLATFORM [SNPS PASSING QC]"),
	}

	return attr
}

// saveAssociation creates and saves a single GWAS association entry
// associationID: Composite key in format "STUDYID_RSID" (e.g., "GCST000001_rs7903146")
// snpID: The rs ID for cross-reference purposes
func (g *gwas) saveAssociation(associationID string, snpID string, attr *pbuf.GwasAttr, sourceID string) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	g.check(err, fmt.Sprintf("marshaling GWAS attributes for %s", associationID))

	// Save entry with composite key
	g.d.addProp3(associationID, sourceID, attrBytes)

	// Create cross-references
	g.createCrossReferences(associationID, snpID, sourceID, attr)
}

// createCrossReferences builds all cross-references for a GWAS association
// associationID: Composite key "STUDYID_RSID" (e.g., "GCST000001_rs7903146")
// snpID: The rs ID part (e.g., "rs7903146")
func (g *gwas) createCrossReferences(associationID string, snpID string, sourceID string, attr *pbuf.GwasAttr) {
	// Text search: SNP ID searchable (finds all associations for this SNP)
	// Entry ID (associationID) is already searchable by default, no need to add as keyword
	g.d.addXref(snpID, textLinkID, associationID, g.source, true)

	// Bidirectional cross-reference: Association ↔ dbSNP
	// "rs7903146 >> gwas" returns all associations (GCST000001_rs7903146, GCST000002_rs7903146, ...)
	if _, exists := config.Dataconf["dbsnp"]; exists {
		g.d.addXref(associationID, sourceID, snpID, "dbsnp", false)
	}

	// Bidirectional cross-reference: Association ↔ GWAS Study
	// "GCST000001 >> gwas" returns all SNP associations in this study
	if attr.StudyAccession != "" {
		g.d.addXref(associationID, sourceID, attr.StudyAccession, "gwas_study", false)
	}

	// Gene symbols → Association cross-reference via Ensembl lookup
	// Handles paralogs by creating xrefs to all matching Ensembl genes
	// Search "BRCA1" returns Ensembl entry, then "BRCA1 >> gwas" returns all associations
	for _, gene := range attr.ReportedGenes {
		if gene != "" && len(gene) < 50 {
			g.d.addXrefViaGeneSymbol(gene, attr.ChrId, associationID, g.source, sourceID)
		}
	}

	// Cross-reference: Association → EFO traits (structured ontology)
	if _, exists := config.Dataconf["efo"]; exists {
		for _, efoID := range attr.EfoTraits {
			if efoID != "" {
				g.d.addXref(associationID, sourceID, efoID, "efo", false)
			}
		}
	}

	// TODO: Consider mapping disease_trait and mapped_traits to ontologies
	// Currently these are kept as display-only attributes:
	// - attr.DiseaseTrait (e.g., "Type 2 diabetes") → Could map to MONDO/EFO via keyword lookup
	// - attr.MappedTraits (e.g., ["insulin resistance", "glucose metabolism"]) → Could map to ontologies
	// For now: Users should search via EFO entries, then use "EFO:0001360 >> gwas" to find SNPs
	// This keeps search clean and uses structured ontology mappings already present in EFO cross-references
}

// Helper: readerAtFromBytes creates a ReaderAt from byte slice (needed for zip.NewReader)
func readerAtFromBytes(b []byte) io.ReaderAt {
	return &bytesReaderAt{b: b}
}

type bytesReaderAt struct {
	b []byte
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n = copy(p, r.b[off:])
	if n < len(p) {
		err = io.EOF
	}
	return
}
