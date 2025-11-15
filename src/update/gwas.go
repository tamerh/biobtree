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

	// Group associations by SNP ID
	// Key: SNP ID, Value: list of associations for that SNP
	snpAssociations := make(map[string][]*pbuf.GwasAttr)

	var processedCount int
	var previous int64
	var skippedEmptySNP int
	var totalRowsRead int
	var uniqueSNPs int

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
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: int64(processedCount / int(elapsed))}
		}

		// Extract SNP ID (primary key)
		snpID := getField(row, colMap, "SNPS")
		if snpID == "" {
			skippedEmptySNP++
			continue
		}

		// Build association entry
		assoc := g.buildAssociation(row, colMap, snpID)
		if assoc == nil {
			continue
		}

		// Group by SNP ID
		if _, exists := snpAssociations[snpID]; !exists {
			uniqueSNPs++
		}
		snpAssociations[snpID] = append(snpAssociations[snpID], assoc)

		processedCount++

		// Test mode: check limit (limit on total associations, not unique SNPs)
		if testLimit > 0 && processedCount >= testLimit {
			log.Printf("GWAS Associations: [TEST MODE] Reached limit of %d associations, stopping", testLimit)
			break
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("GWAS Associations: Scanner error: %v", err)
	}

	log.Printf("GWAS Associations: Total rows read: %d, Associations processed: %d", totalRowsRead, processedCount)
	log.Printf("GWAS Associations: Unique SNPs: %d, Skipped (empty SNP ID): %d", uniqueSNPs, skippedEmptySNP)

	// Now save grouped SNPs
	savedSNPs := 0
	for snpID, assocs := range snpAssociations {
		// For now, take the first association as representative
		// (In production, might want to aggregate statistics across studies)
		g.saveSNP(snpID, assocs[0], sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(snpID + "\n")
		}

		savedSNPs++
	}

	log.Printf("GWAS Associations: Saved %d unique SNPs", savedSNPs)

	// Update entry statistics
	atomic.AddUint64(&g.d.totalParsedEntry, uint64(savedSNPs))
	g.d.addEntryStat(g.source, uint64(savedSNPs))
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

// saveSNP creates and saves a single SNP entry
func (g *gwas) saveSNP(snpID string, attr *pbuf.GwasAttr, sourceID string) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	g.check(err, fmt.Sprintf("marshaling GWAS attributes for %s", snpID))

	// Save entry
	g.d.addProp3(snpID, sourceID, attrBytes)

	// Create cross-references
	g.createCrossReferences(snpID, sourceID, attr)
}

// createCrossReferences builds all cross-references for a SNP
func (g *gwas) createCrossReferences(snpID, sourceID string, attr *pbuf.GwasAttr) {
	// Text search: SNP ID
	g.d.addXref(snpID, textLinkID, snpID, g.source, true)

	// Cross-reference: SNP → Study
	if attr.StudyAccession != "" {
		g.d.addXref(snpID, sourceID, attr.StudyAccession, "gwas_study", false)
	}

	// Cross-reference: Gene symbols → SNP (enables "find SNPs in BRCA1")
	maxGenes := 10 // Limit to prevent too many xrefs
	for i, gene := range attr.ReportedGenes {
		if i >= maxGenes {
			break
		}
		if gene != "" && len(gene) < 50 { // Safety limit
			g.d.addXref(gene, textLinkID, snpID, g.source, true)
		}
	}

	// Cross-reference: SNP → EFO traits
	if _, exists := config.Dataconf["efo"]; exists {
		for _, efoID := range attr.EfoTraits {
			if efoID != "" {
				g.d.addXref(snpID, sourceID, efoID, "efo", false)
			}
		}
	}

	// Text search: Disease/trait (only if not too long to avoid buffer overflow)
	if attr.DiseaseTrait != "" && len(attr.DiseaseTrait) < 200 {
		g.d.addXref(attr.DiseaseTrait, textLinkID, snpID, g.source, true)
	}

	// Text search: Mapped traits (limit to first 3 to avoid too many xrefs)
	maxTraits := 3
	for i, trait := range attr.MappedTraits {
		if i >= maxTraits {
			break
		}
		if trait != "" && len(trait) < 150 {
			g.d.addXref(trait, textLinkID, snpID, g.source, true)
		}
	}
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
