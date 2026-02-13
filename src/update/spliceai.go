package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type spliceai struct {
	source   string
	d        *DataUpdate
	geneIdx  *GeneCoordinateIndex // Gene coordinate index for position-based lookup
}

// Helper for context-aware error checking
func (s *spliceai) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

// Main update entry point
func (s *spliceai) update() {
	defer s.d.wg.Done()

	log.Println("SpliceAI: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("SpliceAI: [TEST MODE] Processing up to %d variants", testLimit)
	}

	sourceID := config.Dataconf[s.source]["id"]
	textLinkID := config.Dataconf["textlink"]["id"]

	// Load gene coordinate index for position-based gene lookup
	s.loadGeneCoordinateIndex()

	// Process main TSV file
	s.parseAndSaveVariants(testLimit, idLogFile, sourceID, textLinkID)

	log.Printf("SpliceAI: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler
	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}

// loadGeneCoordinateIndex loads gene coordinates for position-based lookup from GFF3 file
func (s *spliceai) loadGeneCoordinateIndex() {
	gff3Path := config.Dataconf[s.source]["geneGFF3"]
	if gff3Path == "" {
		log.Println("SpliceAI: No geneGFF3 configured - variant-to-gene xrefs will not be created")
		return
	}

	idx, err := LoadHumanGeneCoordinatesFromGFF3(gff3Path)
	if err != nil {
		log.Printf("SpliceAI: Warning - could not load gene coordinates from GFF3: %v", err)
		return
	}

	if idx != nil && idx.GeneCount() > 0 {
		s.geneIdx = idx
		log.Printf("SpliceAI: Loaded %d genes from GFF3 for coordinate-based lookup", idx.GeneCount())
	}
}

// parseAndSaveVariants processes the SpliceAI TSV file
func (s *spliceai) parseAndSaveVariants(testLimit int, idLogFile *os.File, sourceID, textLinkID string) {
	filePath := config.Dataconf[s.source]["path"]
	log.Printf("SpliceAI: Processing variants from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
	s.check(err, "opening SpliceAI TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount int64
	var entryCount int64
	var skippedCount int64
	var skippedLongKeyCount int64

	// Track effect type statistics
	effectCounts := make(map[string]int64)

	// Progress tracking
	var totalRead int64
	var previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			s.check(readErr, "reading SpliceAI TSV file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		totalRead += int64(len(line))
		lineCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip header line
		if lineCount == 1 && strings.HasPrefix(line, "chromosome") {
			continue
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// TSV Format: chromosome	position	ref_allele	alt_allele	effect	score	allele_info
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("SpliceAI: SKIP line %d: Not enough fields (%d < 6)", lineCount, len(fields))
			}
			continue
		}

		chrom := fields[0]
		posStr := fields[1]
		refAllele := fields[2]
		altAllele := fields[3]
		effect := fields[4]
		scoreStr := fields[5]
		alleleInfo := ""
		if len(fields) > 6 {
			alleleInfo = fields[6]
		}

		// Parse position
		pos, err := strconv.ParseInt(posStr, 10, 64)
		if err != nil {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("SpliceAI: SKIP line %d: Invalid position %q", lineCount, posStr)
			}
			continue
		}

		// Parse score
		score, err := strconv.ParseFloat(scoreStr, 32)
		if err != nil {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("SpliceAI: SKIP line %d: Invalid score %q", lineCount, scoreStr)
			}
			continue
		}

		// Use variant notation (chr:pos:ref:alt) as entry ID for xref compatibility with dbSNP
		entryID := fmt.Sprintf("%s:%d:%s:%s", chrom, pos, refAllele, altAllele)

		// Skip entries with keys too long for LMDB
		if len(entryID) > LMDBMaxKeySize {
			skippedLongKeyCount++
			if skippedLongKeyCount <= 10 {
				log.Printf("SpliceAI: SKIP long key (%d bytes > %d max): %s",
					len(entryID), LMDBMaxKeySize, entryID[:100]+"...")
			}
			continue
		}

		// Track effect type statistics
		effectCounts[effect]++

		// Create attributes
		attr := &pbuf.SpliceAIAttr{
			Chromosome: chrom,
			Position:   pos,
			RefAllele:  refAllele,
			AltAllele:  altAllele,
			Effect:     effect,
			Score:      float32(score),
			GeneSymbol: "", // Will be populated by coordinate-based gene lookup
			AlleleInfo: alleleInfo,
		}

		// Create cross-references (this may update attr.GeneSymbol via coordinate lookup)
		s.createCrossReferences(entryID, sourceID, textLinkID, attr)

		// Index by chr:pos for position-based lookup
		posKey := fmt.Sprintf("%s:%d", chrom, pos)
		s.d.addXref(posKey, textLinkID, entryID, s.source, true)

		// Marshal and save (after xrefs so GeneSymbol is populated)
		attrBytes, err := ffjson.Marshal(attr)
		s.check(err, fmt.Sprintf("marshaling attributes for %s", entryID))
		s.d.addProp3(entryID, sourceID, attrBytes)

		// Log ID for test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		entryCount++

		// Progress reporting
		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: kbytesPerSecond}
		}

		// Progress logging
		if entryCount%1000000 == 0 {
			log.Printf("SpliceAI: Processed %d variants...", entryCount)
		}

		// Test mode limit
		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("SpliceAI: [TEST MODE] Reached limit of %d variants", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	// Log final statistics
	log.Printf("SpliceAI: Processed %d variants (skipped %d malformed lines, %d long keys > %d bytes)",
		entryCount, skippedCount, skippedLongKeyCount, LMDBMaxKeySize)

	// Log effect type distribution
	for effect, count := range effectCounts {
		pct := float64(count) / float64(entryCount) * 100
		log.Printf("SpliceAI: Effect %s: %d (%.1f%%)", effect, count, pct)
	}
}

// createCrossReferences creates cross-references for SpliceAI variant entries
func (s *spliceai) createCrossReferences(entryID, sourceID, textLinkID string, attr *pbuf.SpliceAIAttr) {

	// Also index by effect type for text search
	if attr.Effect != "" {
		s.d.addXref(attr.Effect, textLinkID, entryID, s.source, true)
	}

	// Find overlapping genes using coordinate index and create gene xrefs
	if s.geneIdx != nil {
		overlappingGenes := s.geneIdx.FindOverlappingGenes(attr.Chromosome, attr.Position)
		for _, gene := range overlappingGenes {

			// Cross-reference via gene symbol to HGNC, Entrez, Ensembl
			// This enables queries like: BRCA1 >> spliceai
			if gene.Symbol != "" {
				// Index gene symbol for text search
				s.d.addXref(gene.Symbol, textLinkID, entryID, s.source, true)

				// Add xrefs via gene databases using the standard pattern
				// This creates: gene >> hgnc >> spliceai, gene >> entrez >> spliceai, gene >> ensembl >> spliceai
				s.d.addHumanGeneXrefsAll(gene.Symbol, entryID, sourceID)

				// Store gene symbol in the attribute if not already set
				if attr.GeneSymbol == "" {
					attr.GeneSymbol = gene.Symbol
				} else if !strings.Contains(attr.GeneSymbol, gene.Symbol) {
					// Append additional gene symbols (variant may overlap multiple genes)
					attr.GeneSymbol = attr.GeneSymbol + "," + gene.Symbol
				}
			}
		}
	}
}
