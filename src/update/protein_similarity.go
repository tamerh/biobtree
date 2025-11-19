package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type proteinSimilarity struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (p *proteinSimilarity) check(err error, operation string) {
	checkWithContext(err, p.source, operation)
}

// Main update entry point
func (p *proteinSimilarity) update() {
	defer p.d.wg.Done()

	log.Println("Protein Similarity: Starting DIAMOND data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(p.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("Protein Similarity: [TEST MODE] Processing up to %d proteins", testLimit)
	}

	// Process DIAMOND TSV file
	p.parseAndSaveSimilarities(testLimit, idLogFile)

	log.Printf("Protein Similarity: Processing complete (%.2fs)", time.Since(startTime).Seconds())
}

// parseAndSaveSimilarities processes the DIAMOND BLASTP TSV file
func (p *proteinSimilarity) parseAndSaveSimilarities(testLimit int, idLogFile *os.File) {
	// Get file path (testPath for test mode, path for production)
	var filePath string
	if config.IsTestMode() {
		testPath := config.Dataconf[p.source]["testPath"]
		if testPath != "" {
			filePath = testPath
			log.Printf("Protein Similarity: [TEST MODE] Using test file: %s", filePath)
		} else {
			filePath = config.Dataconf[p.source]["path"]
			log.Printf("Protein Similarity: [TEST MODE] No testPath found, using: %s", filePath)
		}
	} else {
		filePath = config.Dataconf[p.source]["path"]
		log.Printf("Protein Similarity: Processing file: %s", filePath)
	}

	// Open TSV file
	file, err := os.Open(filePath)
	p.check(err, "opening DIAMOND TSV file")
	defer file.Close()

	// Use bufio.Scanner for line-by-line reading
	scanner := bufio.NewScanner(file)
	const maxCapacity = 1024 * 1024 // 1MB buffer for long lines
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and skip header
	if !scanner.Scan() {
		p.check(scanner.Err(), "reading DIAMOND header")
		return
	}
	headerLine := scanner.Text()
	log.Printf("Protein Similarity: Header: %s", headerLine)

	// Group similarities by query protein (with "d" prefix)
	// Key: dP01942, Value: list of similarity hits for that protein
	proteinSimilarities := make(map[string][]*pbuf.DiamondSimilarity)

	var processedLines int
	var previous int64
	var totalRowsRead int
	var uniqueProteins int
	var skippedLines int

	// Source ID for cross-references
	sourceID := config.Dataconf[p.source]["id"]

	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		totalRowsRead++

		// Split by tab
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			log.Printf("Protein Similarity: Line %d has only %d fields, skipping", lineNum, len(fields))
			skippedLines++
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(p.d.start).Seconds())
		if elapsed > previous+p.d.progInterval {
			previous = elapsed
			p.d.progChan <- &progressInfo{dataset: p.source, currentKBPerSec: int64(processedLines / int(elapsed))}
		}

		// Extract query and target UniProt IDs from pipe-delimited format
		// Format: sp|P01942|HBA_MOUSE → P01942
		queryUniProt := p.extractUniProtID(fields[0])
		targetUniProt := p.extractUniProtID(fields[1])
		targetName := p.extractProteinName(fields[1])

		if queryUniProt == "" || targetUniProt == "" {
			skippedLines++
			continue
		}

		// Add "d" prefix for DIAMOND IDs
		queryDiamondID := "d" + queryUniProt

		// Parse alignment statistics
		identity, err := strconv.ParseFloat(fields[2], 32)
		if err != nil {
			skippedLines++
			continue
		}

		alignmentLength, _ := strconv.Atoi(fields[3])
		mismatches, _ := strconv.Atoi(fields[4])
		gapOpens, _ := strconv.Atoi(fields[5])
		qStart, _ := strconv.Atoi(fields[6])
		qEnd, _ := strconv.Atoi(fields[7])
		sStart, _ := strconv.Atoi(fields[8])
		sEnd, _ := strconv.Atoi(fields[9])
		evalue, _ := strconv.ParseFloat(fields[10], 64)
		bitscore, _ := strconv.ParseFloat(fields[11], 32)

		// Build similarity object
		similarity := &pbuf.DiamondSimilarity{
			TargetUniprot:   targetUniProt,
			TargetName:      targetName,
			Identity:        float32(identity),
			AlignmentLength: int32(alignmentLength),
			Mismatches:      int32(mismatches),
			GapOpens:        int32(gapOpens),
			QStart:          int32(qStart),
			QEnd:            int32(qEnd),
			SStart:          int32(sStart),
			SEnd:            int32(sEnd),
			Evalue:          evalue,
			Bitscore:        float32(bitscore),
		}

		// Store similarity for query protein
		if _, exists := proteinSimilarities[queryDiamondID]; !exists {
			uniqueProteins++

			// In test mode, check if we've reached the protein limit
			if testLimit > 0 && uniqueProteins > testLimit {
				log.Printf("Protein Similarity: [TEST MODE] Reached limit of %d proteins, stopping", testLimit)
				break
			}
		}
		proteinSimilarities[queryDiamondID] = append(proteinSimilarities[queryDiamondID], similarity)

		processedLines++
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("Protein Similarity: Scanner error: %v", err)
	}

	log.Printf("Protein Similarity: Total rows read: %d, Lines processed: %d", totalRowsRead, processedLines)
	log.Printf("Protein Similarity: Unique proteins: %d, Skipped: %d", uniqueProteins, skippedLines)

	// Now save grouped protein similarities
	savedProteins := 0
	for diamondID, similarities := range proteinSimilarities {
		// Calculate summary statistics
		var topIdentity float32
		var topBitscore float32

		for _, sim := range similarities {
			if sim.Identity > topIdentity {
				topIdentity = sim.Identity
			}
			if sim.Bitscore > topBitscore {
				topBitscore = sim.Bitscore
			}
		}

		// Save protein
		p.saveProtein(diamondID, similarities, topIdentity, topBitscore, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(diamondID + "\n")
		}

		savedProteins++
	}

	log.Printf("Protein Similarity: Saved %d unique proteins with similarities", savedProteins)

	// Update entry statistics
	atomic.AddUint64(&p.d.totalParsedEntry, uint64(savedProteins))
	p.d.addEntryStat(p.source, uint64(savedProteins))
}

// extractUniProtID extracts UniProt accession from pipe-delimited format
// Input: "sp|P01942|HBA_MOUSE"
// Output: "P01942"
func (p *proteinSimilarity) extractUniProtID(pipeID string) string {
	parts := strings.Split(pipeID, "|")
	if len(parts) >= 2 {
		return parts[1]
	}
	return pipeID // Return as-is if not pipe-delimited
}

// extractProteinName extracts protein name from pipe-delimited format
// Input: "sp|P01942|HBA_MOUSE"
// Output: "HBA_MOUSE"
func (p *proteinSimilarity) extractProteinName(pipeID string) string {
	parts := strings.Split(pipeID, "|")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

// saveProtein saves a protein with all its similarity hits
func (p *proteinSimilarity) saveProtein(diamondID string, similarities []*pbuf.DiamondSimilarity,
	topIdentity, topBitscore float32, sourceID string) {

	attr := &pbuf.ProteinSimilarityAttr{
		ProteinId:       diamondID,
		Similarities:    similarities,
		SimilarityCount: int32(len(similarities)),
		TopIdentity:     topIdentity,
		TopBitscore:     topBitscore,
	}

	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	p.check(err, fmt.Sprintf("marshaling Protein Similarity attributes for %s", diamondID))

	// Save entry (CRITICAL: second param is dataset ID)
	p.d.addProp3(diamondID, sourceID, attrBytes)

	// Create cross-references
	p.createCrossReferences(diamondID, similarities, sourceID)
}

// createCrossReferences creates cross-references from DIAMOND IDs to UniProt
func (p *proteinSimilarity) createCrossReferences(diamondID string, similarities []*pbuf.DiamondSimilarity, sourceID string) {
	textLinkID := "0" // Text search link ID

	// DIAMOND ID → Text search (dP01942 searchable)
	p.d.addXref(diamondID, textLinkID, diamondID, p.source, true)

	// DIAMOND ID → Original UniProt ID (without "d" prefix)
	// This allows queries like: dP01942 >> uniprot
	uniprotID := strings.TrimPrefix(diamondID, "d")
	if uniprotID != diamondID {
		// CRITICAL: Second param is dataset ID, fourth is dataset name
		p.d.addXref(diamondID, sourceID, uniprotID, "uniprot", false)
	}

	// Track unique similar proteins to avoid duplicate xrefs
	uniqueSimilar := make(map[string]bool)

	for _, sim := range similarities {
		// DIAMOND ID → Similar protein (UniProt)
		// This allows bidirectional queries: dP01942 >> protein_similarity >> uniprot
		if sim.TargetUniprot != "" && !uniqueSimilar[sim.TargetUniprot] {
			p.d.addXref(diamondID, sourceID, sim.TargetUniprot, "uniprot", false)
			uniqueSimilar[sim.TargetUniprot] = true
		}
	}
}
