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

type esm2Similarity struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (e *esm2Similarity) check(err error, operation string) {
	checkWithContext(err, e.source, operation)
}

// Main update entry point
func (e *esm2Similarity) update() {
	defer e.d.wg.Done()

	log.Println("ESM2 Similarity: Starting ESM2 embedding similarity processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(e.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, e.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("ESM2 Similarity: [TEST MODE] Processing up to %d proteins", testLimit)
	}

	// Process ESM2 TSV file
	e.parseAndSaveSimilarities(testLimit, idLogFile)

	log.Printf("ESM2 Similarity: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler
	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
}

// parseAndSaveSimilarities processes the ESM2 similarity TSV file
// Format: query_id	target_id	cosine_similarity	rank
func (e *esm2Similarity) parseAndSaveSimilarities(testLimit int, idLogFile *os.File) {
	filePath := config.Dataconf[e.source]["path"]
	if config.IsTestMode() {
		log.Printf("ESM2 Similarity: [TEST MODE] Processing file: %s (will stop after %d proteins)", filePath, testLimit)
	} else {
		log.Printf("ESM2 Similarity: Processing file: %s", filePath)
	}

	// Open TSV file
	file, err := os.Open(filePath)
	e.check(err, "opening ESM2 TSV file")
	defer file.Close()

	// Use bufio.Scanner for line-by-line reading
	scanner := bufio.NewScanner(file)
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and skip header
	if !scanner.Scan() {
		e.check(scanner.Err(), "reading ESM2 header")
		return
	}
	headerLine := scanner.Text()
	log.Printf("ESM2 Similarity: Header: %s", headerLine)

	// Group similarities by query protein
	proteinSimilarities := make(map[string][]*pbuf.Esm2Similarity)

	var processedLines int
	var previous int64
	var totalRowsRead int
	var uniqueProteins int
	var skippedLines int

	sourceID := config.Dataconf[e.source]["id"]
	lineNum := 1

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if line == "" {
			continue
		}

		totalRowsRead++

		// Split by tab: query_id, target_id, cosine_similarity, rank
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			log.Printf("ESM2 Similarity: Line %d has only %d fields, skipping", lineNum, len(fields))
			skippedLines++
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(e.d.start).Seconds())
		if elapsed > previous+e.d.progInterval {
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: int64(processedLines / int(elapsed))}
		}

		queryID := strings.TrimSpace(fields[0])
		targetID := strings.TrimSpace(fields[1])

		if queryID == "" || targetID == "" {
			skippedLines++
			continue
		}

		// Parse cosine similarity
		cosineSim, err := strconv.ParseFloat(fields[2], 32)
		if err != nil {
			skippedLines++
			continue
		}

		// Parse rank
		rank, err := strconv.Atoi(fields[3])
		if err != nil {
			skippedLines++
			continue
		}

		// Build similarity object
		similarity := &pbuf.Esm2Similarity{
			TargetUniprot:    targetID,
			CosineSimilarity: float32(cosineSim),
			Rank:             int32(rank),
		}

		// Track unique proteins
		if _, exists := proteinSimilarities[queryID]; !exists {
			uniqueProteins++

			// In test mode, check if we've reached the protein limit
			if testLimit > 0 && uniqueProteins > testLimit {
				log.Printf("ESM2 Similarity: [TEST MODE] Reached limit of %d proteins, stopping", testLimit)
				break
			}
		}
		proteinSimilarities[queryID] = append(proteinSimilarities[queryID], similarity)

		processedLines++
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("ESM2 Similarity: Scanner error: %v", err)
	}

	log.Printf("ESM2 Similarity: Total rows read: %d, Lines processed: %d", totalRowsRead, processedLines)
	log.Printf("ESM2 Similarity: Unique proteins: %d, Skipped: %d", uniqueProteins, skippedLines)

	// Save grouped protein similarities
	savedProteins := 0
	for proteinID, similarities := range proteinSimilarities {
		// Calculate summary statistics
		var topSimilarity float32
		var totalSimilarity float32

		for _, sim := range similarities {
			totalSimilarity += sim.CosineSimilarity
			if sim.CosineSimilarity > topSimilarity {
				topSimilarity = sim.CosineSimilarity
			}
		}

		avgSimilarity := totalSimilarity / float32(len(similarities))

		// Save protein
		e.saveProtein(proteinID, similarities, topSimilarity, avgSimilarity, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(proteinID + "\n")
		}

		savedProteins++
	}

	log.Printf("ESM2 Similarity: Saved %d unique proteins with similarities", savedProteins)

	// Update entry statistics
	atomic.AddUint64(&e.d.totalParsedEntry, uint64(savedProteins))
}

// saveProtein saves a protein with all its ESM2 similarity hits
func (e *esm2Similarity) saveProtein(proteinID string, similarities []*pbuf.Esm2Similarity,
	topSimilarity, avgSimilarity float32, sourceID string) {

	attr := &pbuf.Esm2SimilarityAttr{
		ProteinId:       proteinID,
		Similarities:    similarities,
		SimilarityCount: int32(len(similarities)),
		TopSimilarity:   topSimilarity,
		AvgSimilarity:   avgSimilarity,
	}

	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	e.check(err, fmt.Sprintf("marshaling ESM2 Similarity attributes for %s", proteinID))

	// Save entry
	e.d.addProp3(proteinID, sourceID, attrBytes)

	// Create cross-references
	e.createCrossReferences(proteinID, similarities, sourceID)
}

// createCrossReferences creates cross-references from ESM2 similarity to UniProt
func (e *esm2Similarity) createCrossReferences(proteinID string, similarities []*pbuf.Esm2Similarity, sourceID string) {
	textLinkID := "0"

	// Protein ID → Text search
	e.d.addXref(proteinID, textLinkID, proteinID, e.source, true)

	// ESM2 similarity → UniProt (same ID)
	e.d.addXref(proteinID, sourceID, proteinID, "uniprot", false)

	// Track unique similar proteins to avoid duplicate xrefs
	uniqueSimilar := make(map[string]bool)
	uniqueSimilar[proteinID] = true // Mark query protein as seen

	for _, sim := range similarities {
		// ESM2 similarity → Similar protein (UniProt)
		if sim.TargetUniprot != "" && !uniqueSimilar[sim.TargetUniprot] {
			e.d.addXref(proteinID, sourceID, sim.TargetUniprot, "uniprot", false)
			uniqueSimilar[sim.TargetUniprot] = true
		}
	}
}
