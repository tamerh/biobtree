package update

import (
	"archive/tar"
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
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

type alphafoldProcessor struct {
	source   string
	sourceID string
	d        *DataUpdate
}

// Main update entry point
func (a *alphafoldProcessor) update() {
	defer a.d.wg.Done()

	a.sourceID = config.Dataconf[a.source]["id"]

	// Test mode support
	testLimit := config.GetTestLimit(a.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, a.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	fmt.Printf("Processing AlphaFold structures...\n")

	// Get data source path
	filePath := config.Dataconf[a.source]["path"]

	// Process tar.gz file
	totalProcessed, err := a.processTarFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Fatalf("Error processing AlphaFold data: %v", err)
	}

	fmt.Printf("AlphaFold processing complete: %d structures processed\n", totalProcessed)

	atomic.AddUint64(&a.d.totalParsedEntry, totalProcessed)
	a.d.addEntryStat(a.source, totalProcessed)
	a.d.progChan <- &progressInfo{dataset: a.source, done: true}
}

// Process tar.gz file containing PDB files
func (a *alphafoldProcessor) processTarFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	fmt.Printf("Opening tar file from: %s\n", filePath)
	fmt.Printf("This may take a while for large remote files...\n")

	// Open tar file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open AlphaFold tar file: %v", err)
	}
	defer closeAlphaFoldReaders(gz, ftpFile, client, localFile)

	fmt.Printf("✓ Tar file opened successfully\n")
	fmt.Printf("Starting to read tar entries...\n")

	// Create tar reader (file is tar.gz format, so gzip is already handled by getDataReaderNew)
	var tarReader *tar.Reader
	if gz != nil {
		tarReader = tar.NewReader(gz)
	} else {
		tarReader = tar.NewReader(br)
	}

	var totalProcessed uint64
	var totalBytesRead int64
	var previous int64
	var entriesScanned uint64

	// Iterate through tar entries
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return totalProcessed, fmt.Errorf("error reading tar: %v", err)
		}

		entriesScanned++

		// Log first 10 filenames to see the structure
		if entriesScanned <= 10 {
			fmt.Printf("  DEBUG: Entry %d: %s (size: %d bytes)\n", entriesScanned, header.Name, header.Size)
		}

		// Log progress every 1000 entries scanned
		if entriesScanned % 1000 == 0 {
			fmt.Printf("  Scanned %d tar entries, processed %d PDB files...\n", entriesScanned, totalProcessed)
		}

		// Only process .pdb.gz files
		if !strings.HasSuffix(header.Name, ".pdb.gz") {
			continue
		}

		// Extract UniProt ID from filename: AF-Q9Y6K9-F1-model_v6.pdb.gz → Q9Y6K9
		uniprotID, modelID := extractIDsFromFilename(header.Name)
		if uniprotID == "" {
			continue
		}

		// Log each processed structure
		fmt.Printf("  [%d] Processing %s → %s\n", totalProcessed+1, modelID, uniprotID)

		// Decompress gzip stream and parse PDB file
		gzReader, err := gzip.NewReader(tarReader)
		if err != nil {
			log.Printf("Warning: Error creating gzip reader for %s: %v", header.Name, err)
			continue
		}

		// Parse PDB file and extract pLDDT scores
		plddt, err := a.parsePDBFile(gzReader)
		gzReader.Close() // Close gzip reader after parsing
		if err != nil {
			log.Printf("Warning: Error parsing PDB file %s: %v", header.Name, err)
			continue
		}

		// Calculate pLDDT fractions
		fractions := calculatePLDDTFractions(plddt)

		// Calculate global metric (average pLDDT)
		globalMetric := calculateAverage(plddt)

		// Create AlphaFold attribute
		attr := pbuf.AlphaFoldAttr{
			GlobalMetric:             globalMetric,
			FractionPldddtVeryHigh:   fractions.VeryHigh,
			FractionPldddtConfident:  fractions.Confident,
			FractionPldddtLow:        fractions.Low,
			FractionPldddtVeryLow:    fractions.VeryLow,
			ModelEntityId:            modelID,
			Gene:                     "", // Will be populated from UniProt if needed
		}

		// Marshal and store on UniProt ID
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("Error marshaling AlphaFold attr for %s: %v", uniprotID, err)
			continue
		}

		a.d.addProp3(uniprotID, a.sourceID, b)

		// Create cross-reference: UniProt → AlphaFold
		uniprotDatasetID := config.Dataconf["uniprot"]["id"]
		a.d.addXref(uniprotID, uniprotDatasetID, uniprotID, a.source, false)

		// Create keyword: AlphaFold model ID → UniProt entry
		a.d.addXref(modelID, a.sourceID, uniprotID, a.source, true)

		totalProcessed++
		totalBytesRead += header.Size

		// Test mode: log ID and check limit
		if idLogFile != nil {
			logProcessedID(idLogFile, uniprotID)
		}

		// In test mode, stop after processing enough structures
		if testLimit > 0 && int(totalProcessed) >= testLimit {
			fmt.Printf("  [TEST MODE] Reached limit of %d structures, stopping processing\n", testLimit)
			break
		}

		// Progress reporting
		elapsed := int64(time.Since(a.d.start).Seconds())
		if elapsed > previous+a.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			a.d.progChan <- &progressInfo{dataset: a.source, currentKBPerSec: kbytesPerSecond}
		}
	}

	return totalProcessed, nil
}

// Extract UniProt ID and Model ID from filename
// Example: AF-Q9Y6K9-F1-model_v6.pdb.gz → Q9Y6K9, AF-Q9Y6K9-F1
func extractIDsFromFilename(filename string) (string, string) {
	// Remove path and extension
	base := strings.TrimSuffix(filename, ".pdb.gz")
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}

	// Format: AF-{UniProtID}-F{fragment}-model_v{version}
	// Example: AF-Q9Y6K9-F1-model_v6
	parts := strings.Split(base, "-")
	if len(parts) < 3 {
		return "", ""
	}

	if parts[0] != "AF" {
		return "", ""
	}

	uniprotID := parts[1]
	// Model ID is everything before "-model_v"
	modelID := fmt.Sprintf("%s-%s-%s", parts[0], parts[1], parts[2])

	return uniprotID, modelID
}

// Parse PDB file and extract pLDDT scores from B-factor column
func (a *alphafoldProcessor) parsePDBFile(reader io.Reader) ([]float64, error) {
	var plddtScores []float64

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// Only process ATOM records
		if !strings.HasPrefix(line, "ATOM") {
			continue
		}

		// PDB format: B-factor is in columns 61-66 (0-indexed: 60-66)
		if len(line) < 66 {
			continue
		}

		bFactorStr := strings.TrimSpace(line[60:66])
		bFactor, err := strconv.ParseFloat(bFactorStr, 64)
		if err != nil {
			continue
		}

		plddtScores = append(plddtScores, bFactor)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(plddtScores) == 0 {
		return nil, fmt.Errorf("no pLDDT scores found")
	}

	return plddtScores, nil
}

// PLDDTFractions holds fraction of residues in each confidence category
type PLDDTFractions struct {
	VeryHigh  float64 // > 90
	Confident float64 // 70-90
	Low       float64 // 50-70
	VeryLow   float64 // < 50
}

// Calculate pLDDT fractions based on thresholds
func calculatePLDDTFractions(scores []float64) PLDDTFractions {
	if len(scores) == 0 {
		return PLDDTFractions{}
	}

	var veryHigh, confident, low, veryLow int

	for _, score := range scores {
		if score > 90 {
			veryHigh++
		} else if score >= 70 {
			confident++
		} else if score >= 50 {
			low++
		} else {
			veryLow++
		}
	}

	total := float64(len(scores))
	return PLDDTFractions{
		VeryHigh:  float64(veryHigh) / total,
		Confident: float64(confident) / total,
		Low:       float64(low) / total,
		VeryLow:   float64(veryLow) / total,
	}
}

// Calculate average of scores
func calculateAverage(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}

	var sum float64
	for _, score := range scores {
		sum += score
	}

	return sum / float64(len(scores))
}

// Helper to close readers
func closeAlphaFoldReaders(gz *gzip.Reader, ftpFile interface{}, client interface{}, localFile *os.File) {
	if gz != nil {
		gz.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
}
