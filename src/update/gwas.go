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

	// Save each association with unique key: STUDYID_N (where N is row counter per study)
	// This avoids long keys while preserving all SNP associations via xrefs
	var savedAssociations int
	var previous int64
	var skippedEmptySNP int
	var skippedEmptyStudy int
	var totalRowsRead int

	// Track association count per study for unique key generation
	studyAssocCount := make(map[string]int)

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

		// Extract SNP IDs and Study Accession
		snpsField := getField(row, colMap, "SNPS")
		if snpsField == "" {
			skippedEmptySNP++
			continue
		}

		studyAccession := getField(row, colMap, "STUDY ACCESSION")
		if studyAccession == "" {
			skippedEmptyStudy++
			continue
		}

		// SNPS field can contain multiple SNP IDs separated by semicolons, commas, or "x"
		// e.g., "rs387673; rs12413638; rs7096965" or "rs123 x rs456"
		snpIDs := splitSNPs(snpsField)
		if len(snpIDs) == 0 {
			skippedEmptySNP++
			continue
		}

		// Generate unique key: STUDYID_N (e.g., "GCST000001_1", "GCST000001_2")
		studyAssocCount[studyAccession]++
		associationID := studyAccession + "_" + strconv.Itoa(studyAssocCount[studyAccession])

		// Build association entry with first SNP as primary (others linked via xrefs)
		assoc := g.buildAssociation(row, colMap, snpIDs[0])
		if assoc == nil {
			continue
		}

		// Save association with unique short key
		g.saveAssociation(associationID, snpIDs, assoc, sourceID)

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
// associationID: Unique key in format "STUDYID_N" (e.g., "GCST000001_1")
// snpIDs: All SNP IDs from this association row (for cross-references)
func (g *gwas) saveAssociation(associationID string, snpIDs []string, attr *pbuf.GwasAttr, sourceID string) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	g.check(err, fmt.Sprintf("marshaling GWAS attributes for %s", associationID))

	// Save entry with unique key
	g.d.addProp3(associationID, sourceID, attrBytes)

	// Create cross-references for all SNPs
	g.createCrossReferences(associationID, snpIDs, sourceID, attr)
}

// createCrossReferences builds all cross-references for a GWAS association
// associationID: Unique key "STUDYID_N" (e.g., "GCST000001_1")
// snpIDs: All SNP IDs from this association row
func (g *gwas) createCrossReferences(associationID string, snpIDs []string, sourceID string, attr *pbuf.GwasAttr) {
	// Text search: association ID searchable (e.g., "GCST000001_1")
	g.d.addXref(associationID, textLinkID, associationID, g.source, true)

	// Text search: study accession searchable (e.g., search "GCST000001" finds all associations)
	if attr.StudyAccession != "" {
		g.d.addXref(attr.StudyAccession, textLinkID, associationID, g.source, true)
	}

	// Create xrefs for ALL SNPs in this association
	for _, snpID := range snpIDs {
		// Text search: SNP ID searchable (finds all associations for this SNP)
		g.d.addXref(snpID, textLinkID, associationID, g.source, true)

		// Bidirectional cross-reference: Association ↔ dbSNP
		// "rs7903146 >> gwas" returns all associations containing this SNP
		// Only create xref if snpID is a valid rsID format (starts with "rs" followed by digits)
		if _, exists := config.Dataconf["dbsnp"]; exists {
			if isValidRsID(snpID) {
				g.d.addXref(associationID, sourceID, snpID, "dbsnp", false)
			}
		}
	}

	// Bidirectional cross-reference: Association ↔ GWAS Study
	// "GCST000001 >> gwas" returns all associations in this study
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

	// Cross-reference: Association → ontology traits (EFO, MONDO, HP, OBA, etc.)
	// GWAS Catalog includes traits from multiple ontologies, route each to correct dataset
	for _, traitID := range attr.EfoTraits {
		if traitID == "" {
			continue
		}
		// Route to correct dataset based on prefix
		targetDataset := getOntologyDataset(traitID)
		if targetDataset != "" {
			if _, exists := config.Dataconf[targetDataset]; exists {
				g.d.addXref(associationID, sourceID, traitID, targetDataset, false)
			}
		}
	}
}

// isValidRsID checks if a SNP ID is in valid rsID format (rs followed by digits)
func isValidRsID(snpID string) bool {
	if len(snpID) < 3 {
		return false
	}
	if snpID[0] != 'r' || snpID[1] != 's' {
		return false
	}
	// Check remaining characters are digits
	for i := 2; i < len(snpID); i++ {
		if snpID[i] < '0' || snpID[i] > '9' {
			return false
		}
	}
	return true
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

// splitSNPs splits a compound SNP field into individual SNP IDs
// The GWAS Catalog SNPS field can contain multiple SNPs separated by:
// - semicolons: "rs123; rs456; rs789"
// - " x " (interaction): "rs123 x rs456"
// - commas: "rs123, rs456"
func splitSNPs(snpsField string) []string {
	// Replace all delimiters with semicolon for uniform splitting
	normalized := snpsField
	normalized = strings.ReplaceAll(normalized, " x ", ";")
	normalized = strings.ReplaceAll(normalized, ",", ";")

	parts := strings.Split(normalized, ";")
	var snpIDs []string
	for _, part := range parts {
		snpID := strings.TrimSpace(part)
		if snpID != "" {
			snpIDs = append(snpIDs, snpID)
		}
	}
	return snpIDs
}

// getOntologyDataset maps ontology ID prefixes to biobtree dataset names
// Returns empty string if ontology is not supported
func getOntologyDataset(id string) string {
	colonIdx := strings.Index(id, ":")
	if colonIdx <= 0 {
		return ""
	}
	prefix := strings.ToUpper(id[:colonIdx])

	// Map ontology prefixes to biobtree dataset names
	switch prefix {
	case "EFO":
		return "efo"
	case "MONDO":
		return "mondo"
	case "HP":
		return "hpo"
	case "GO":
		return "go"
	case "UBERON":
		return "uberon"
	case "CL":
		return "cl"
	case "ECO":
		return "eco"
	case "CHEBI":
		return "chebi"
	case "ORPHANET", "ORPHA":
		return "orphanet"
	case "OBA":
		return "oba"
	case "PATO":
		return "pato"
	case "OBI":
		return "obi"
	case "XCO":
		return "xco"
	// Unsupported ontologies - skip silently
	case "NCIT", "VT", "CMO":
		return ""
	default:
		// Log unknown prefixes for debugging (uncomment if needed)
		// log.Printf("Unknown ontology prefix: %s", prefix)
		return ""
	}
}
