package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// mirdb processes miRDB microRNA target prediction database
// Data format: miRNA_ID<tab>RefSeq_ID<tab>Score
// Example: hsa-miR-21-5p	NM_001234	95.5
type mirdb struct {
	source string
	d      *DataUpdate
}

// Species prefix to full name mapping
var mirdbSpeciesMap = map[string]string{
	"hsa": "Homo sapiens",
	"mmu": "Mus musculus",
	"rno": "Rattus norvegicus",
	"cfa": "Canis familiaris",
	"gga": "Gallus gallus",
}

// Species prefix to taxID mapping for sorting
var mirdbSpeciesTaxID = map[string]string{
	"hsa": "9606",  // Human
	"mmu": "10090", // Mouse
	"rno": "10116", // Rat
	"cfa": "9615",  // Dog
	"gga": "9031",  // Chicken
}

// Helper for context-aware error checking
func (m *mirdb) check(err error, operation string) {
	checkWithContext(err, m.source, operation)
}

// Main update entry point
func (m *mirdb) update() {
	defer m.d.wg.Done()

	log.Println("miRDB: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(m.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, m.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("miRDB: [TEST MODE] Processing up to %d miRNAs", testLimit)
	}

	sourceID := config.Dataconf[m.source]["id"]
	textLinkID := config.Dataconf["textlink"]["id"]

	// Process the prediction file
	m.parseAndSavePredictions(testLimit, idLogFile, sourceID, textLinkID)

	log.Printf("miRDB: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler
	m.d.progChan <- &progressInfo{dataset: m.source, done: true}
}

// parseAndSavePredictions processes the miRDB prediction result file
// Groups predictions by miRNA ID and creates entries with all targets
func (m *mirdb) parseAndSavePredictions(testLimit int, idLogFile *os.File, sourceID, textLinkID string) {
	path := config.Dataconf[m.source]["path"]

	var br *bufio.Reader
	var httpResp *http.Response
	var localFile *os.File
	var gzReader *gzip.Reader

	// Support both local files and HTTP(S) downloads
	if config.Dataconf[m.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		m.check(err, "opening local file")
		localFile = file

		// Check if gzipped
		if strings.HasSuffix(path, ".gz") {
			gzReader, err = gzip.NewReader(file)
			m.check(err, "creating gzip reader")
			br = bufio.NewReaderSize(gzReader, fileBufSize)
		} else {
			br = bufio.NewReaderSize(file, fileBufSize)
		}
	} else if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		log.Printf("miRDB: Downloading from %s", path)
		resp, err := http.Get(path)
		m.check(err, "downloading miRDB data")
		httpResp = resp

		// Check if gzipped
		if strings.HasSuffix(path, ".gz") {
			gzReader, err = gzip.NewReader(resp.Body)
			m.check(err, "creating gzip reader")
			br = bufio.NewReaderSize(gzReader, fileBufSize)
		} else {
			br = bufio.NewReaderSize(resp.Body, fileBufSize)
		}
	} else {
		// Fall back to FTP
		br2, gz, ftpFile, client, localFile2, _, err := getDataReaderNew(m.source, "", "", path)
		m.check(err, "opening FTP file")
		br = br2
		gzReader = gz
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

	// Close resources
	defer func() {
		if gzReader != nil {
			gzReader.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if httpResp != nil {
			httpResp.Body.Close()
		}
	}()

	// First pass: group all targets by miRNA ID
	// miRDB file format: miRNA_ID<tab>RefSeq_ID<tab>Score
	log.Println("miRDB: Reading and grouping predictions by miRNA...")

	type targetInfo struct {
		RefSeqID string
		Score    float32
	}

	mirnaTargets := make(map[string][]targetInfo)
	var totalLines int64
	var skippedLines int64

	reader := bufio.NewReaderSize(br, 1024*1024)

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			m.check(err, "reading miRDB file")
		}
		if len(line) == 0 && err == io.EOF {
			break
		}

		totalLines++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip empty lines
		if line == "" {
			if err == io.EOF {
				break
			}
			continue
		}

		// Parse tab-separated: miRNA_ID<tab>RefSeq_ID<tab>Score
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			skippedLines++
			if skippedLines <= 5 {
				log.Printf("miRDB: SKIP line %d: Not enough fields (%d < 3)", totalLines, len(fields))
			}
			continue
		}

		mirnaID := strings.TrimSpace(fields[0])
		refseqID := strings.TrimSpace(fields[1])
		scoreStr := strings.TrimSpace(fields[2])

		if mirnaID == "" || refseqID == "" {
			skippedLines++
			continue
		}

		score, err := strconv.ParseFloat(scoreStr, 32)
		if err != nil {
			skippedLines++
			continue
		}

		mirnaTargets[mirnaID] = append(mirnaTargets[mirnaID], targetInfo{
			RefSeqID: refseqID,
			Score:    float32(score),
		})

		if err == io.EOF {
			break
		}
	}

	log.Printf("miRDB: Read %d lines, found %d unique miRNAs (skipped %d malformed)",
		totalLines, len(mirnaTargets), skippedLines)

	// Second pass: create entries for each miRNA with all targets
	log.Println("miRDB: Creating entries for each miRNA...")

	var entryCount int64
	var totalTargets int64
	var previous int64

	// Track species distribution
	speciesCount := make(map[string]int)

	for mirnaID, targets := range mirnaTargets {
		// Progress reporting
		elapsed := int64(time.Since(m.d.start).Seconds())
		if elapsed > previous+m.d.progInterval {
			previous = elapsed
			m.d.progChan <- &progressInfo{dataset: m.source, currentKBPerSec: 0}
		}

		// Extract species from miRNA ID (e.g., "hsa-miR-21-5p" -> "hsa")
		species := ""
		if dashIdx := strings.Index(mirnaID, "-"); dashIdx > 0 {
			species = mirnaID[:dashIdx]
		}
		speciesCount[species]++

		// Calculate score statistics
		var minScore, maxScore, sumScore float32
		minScore = 100
		for i, t := range targets {
			if i == 0 || t.Score < minScore {
				minScore = t.Score
			}
			if t.Score > maxScore {
				maxScore = t.Score
			}
			sumScore += t.Score
		}
		avgScore := sumScore / float32(len(targets))

		// Sort targets by score descending for top N selection
		sort.Slice(targets, func(i, j int) bool {
			return targets[i].Score > targets[j].Score
		})

		// Build compact top 50 targets: "refseq|score"
		topN := 50
		if len(targets) < topN {
			topN = len(targets)
		}
		topTargets := make([]string, 0, topN)
		for i := 0; i < topN; i++ {
			t := targets[i]
			topTargets = append(topTargets, fmt.Sprintf("%s|%.1f", t.RefSeqID, t.Score))
		}

		// Create attributes with compact top targets
		attr := &pbuf.MiRDBAttr{
			MirnaId:          mirnaID,
			Species:          species,
			TargetCount:      int32(len(targets)),
			AvgScore:         avgScore,
			MaxScore:         maxScore,
			MinScore:         minScore,
			TopTargetsSchema: "refseq|score",
			TopTargets:       topTargets,
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		m.check(err, "marshaling attributes for "+mirnaID)
		m.d.addProp3(mirnaID, sourceID, attrBytes)

		// Create cross-references

		// Text search by miRNA ID
		m.d.addXref(mirnaID, textLinkID, mirnaID, m.source, true)

		// Also index without species prefix for easier search
		// e.g., "miR-21-5p" in addition to "hsa-miR-21-5p"
		if dashIdx := strings.Index(mirnaID, "-"); dashIdx > 0 && dashIdx < len(mirnaID)-1 {
			shortName := mirnaID[dashIdx+1:]
			m.d.addXref(shortName, textLinkID, mirnaID, m.source, true)
		}

		// Cross-reference to RefSeq targets with sorting (species priority + prediction score)
		// Get taxID for species priority sorting
		taxID := mirdbSpeciesTaxID[species]
		if taxID == "" {
			taxID = "0" // Unknown species gets lowest priority
		}

		for _, t := range targets {
			// Scale score from 0-100 to 0-1000 for interactionScore
			scoreInt := int(t.Score * 10)
			if scoreInt > 1000 {
				scoreInt = 1000
			}

			sortLevels := []string{
				ComputeSortLevelValue(SortLevelSpeciesPriority, map[string]interface{}{"taxID": taxID}),
				ComputeSortLevelValue(SortLevelInteractionScore, map[string]interface{}{"score": scoreInt}),
			}
			m.d.addXrefWithSortLevels(mirnaID, sourceID, t.RefSeqID, "refseq", sortLevels)
		}

		// Log ID for test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, mirnaID)
		}

		entryCount++
		totalTargets += int64(len(targets))

		// Test mode: check if limit reached
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			log.Printf("miRDB: [TEST MODE] Reached limit of %d miRNAs", testLimit)
			break
		}
	}

	atomic.AddUint64(&m.d.totalParsedEntry, uint64(entryCount))

	log.Printf("miRDB: Saved %d miRNA entries with %d total target predictions", entryCount, totalTargets)
	log.Printf("miRDB: Species distribution:")
	for sp, count := range speciesCount {
		if fullName, ok := mirdbSpeciesMap[sp]; ok {
			log.Printf("  %s (%s): %d miRNAs", sp, fullName, count)
		} else {
			log.Printf("  %s: %d miRNAs", sp, count)
		}
	}
}
