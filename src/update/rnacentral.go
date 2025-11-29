package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type rnacentralProcessor struct {
	source   string
	sourceID string
	d        *DataUpdate
}

// Main update entry point
func (r *rnacentralProcessor) update() {
	defer r.d.wg.Done()

	r.sourceID = config.Dataconf[r.source]["id"]

	// Test mode support
	testLimit := config.GetTestLimit(r.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, r.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	fmt.Printf("Processing RNACentral sequences...\n")

	// Get data source path
	filePath := config.Dataconf[r.source]["path"]

	// Process FASTA file
	totalProcessed, err := r.processFastaFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Fatalf("Error processing RNACentral data: %v", err)
	}

	fmt.Printf("RNACentral processing complete: %d sequences processed\n", totalProcessed)

	atomic.AddUint64(&r.d.totalParsedEntry, totalProcessed)
	r.d.addEntryStat(r.source, totalProcessed)
	r.d.progChan <- &progressInfo{dataset: r.source, done: true}
}

// Process FASTA.gz file containing RNA sequences
func (r *rnacentralProcessor) processFastaFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	// fmt.Printf("Opening FASTA file from: %s\n", filePath)
	// fmt.Printf("This may take a while for large remote files...\n")

	// Open FASTA file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open RNACentral FASTA file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	// fmt.Printf("✓ FASTA file opened successfully\n")
	// fmt.Printf("Starting to parse FASTA entries...\n")

	// Create reader for line by line reading
	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024) // 1MB buffer
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024) // 1MB buffer
	}

	var totalProcessed uint64
	var totalBytesRead int64
	var previous int64

	// Parse FASTA format
	var currentID string
	var currentDesc string
	var currentSeq strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return totalProcessed, fmt.Errorf("error reading FASTA: %v", err)
		}
		if len(line) == 0 && err == io.EOF {
			break
		}

		totalBytesRead += int64(len(line))
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Header line
		if strings.HasPrefix(line, ">") {
			// Process previous entry
			if currentID != "" {
				if procErr := r.processEntry(currentID, currentDesc, currentSeq.String(), idLogFile); procErr != nil {
					log.Printf("Warning: Error processing %s: %v", currentID, procErr)
				} else {
					totalProcessed++

					// Log progress every 1000 entries
					// if totalProcessed%1000 == 0 {
					// 	fmt.Printf("  Processed %d sequences...\n", totalProcessed)
					// }

					// In test mode, stop after processing enough sequences
					if testLimit > 0 && int(totalProcessed) >= testLimit {
						// fmt.Printf("  [TEST MODE] Reached limit of %d sequences, stopping processing\n", testLimit)
						break
					}
				}
			}

			// Parse new header: >URS000149A9AF rRNA from 1 species
			header := strings.TrimPrefix(line, ">")
			parts := strings.SplitN(header, " ", 2)
			currentID = parts[0]
			if len(parts) > 1 {
				currentDesc = strings.TrimSpace(parts[1])
			} else {
				currentDesc = ""
			}
			currentSeq.Reset()
		} else {
			// Sequence line
			currentSeq.WriteString(strings.TrimSpace(line))
		}

		// Progress reporting
		elapsed := int64(time.Since(r.d.start).Seconds())
		if elapsed > previous+r.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			r.d.progChan <- &progressInfo{dataset: r.source, currentKBPerSec: kbytesPerSecond}
		}

		if err == io.EOF {
			break
		}
	}

	// Process last entry
	if currentID != "" && (testLimit == 0 || int(totalProcessed) < testLimit) {
		if err := r.processEntry(currentID, currentDesc, currentSeq.String(), idLogFile); err != nil {
			log.Printf("Warning: Error processing %s: %v", currentID, err)
		} else {
			totalProcessed++
		}
	}

	return totalProcessed, nil
}

// Process a single FASTA entry
func (r *rnacentralProcessor) processEntry(id, description, sequence string, idLogFile *os.File) error {
	// Extract RNA type from description (e.g., "rRNA from 1 species" → rRNA)
	rnaType := extractRNAType(description)

	// Calculate sequence length
	seqLength := int32(len(sequence))

	// Extract organism count from description (e.g., "from 1 species" → 1)
	organismCount := extractOrganismCount(description)

	// Create RNACentral attribute (metadata only, not storing sequence)
	attr := pbuf.RnacentralAttr{
		RnaType:       rnaType,
		Description:   description,
		Length:        seqLength,
		OrganismCount: organismCount,
		Databases:     []string{}, // Will be populated from id_mapping.tsv if needed
		IsActive:      true,       // Assume active from FASTA (rnacentral_active.fasta.gz)
		Md5:           "",         // Will be calculated if needed
	}

	// Marshal and store
	b, err := ffjson.Marshal(&attr)
	if err != nil {
		return fmt.Errorf("error marshaling RNACentral attr: %v", err)
	}

	r.d.addProp3(id, r.sourceID, b)

	// Test mode: log ID
	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}

	return nil
}

// Extract RNA type from description
// Example: "rRNA from 1 species" → "rRNA"
// Example: "Homo sapiens (human) microRNA hsa-mir-21" → "microRNA"
func extractRNAType(description string) string {
	desc := strings.ToLower(description)

	// Common RNA type keywords (in order of specificity)
	rnaTypes := []string{
		"microrna", "mirna", "pre-mirna",
		"lncrna", "lincrna", "long non-coding rna",
		"snorna", "snrna", "snorna", "snrna",
		"pirna",
		"sirna",
		"rrna", "ribosomal rna",
		"trna", "transfer rna",
		"scrna", "tmrna", "telomerase rna",
		"rnase p rna", "rnase mrp rna",
		"vault rna", "y rna",
		"ribozyme", "hammerhead ribozyme",
		"antisense rna", "antisense",
		"autocatalytically spliced intron",
		"other",
	}

	for _, rnaType := range rnaTypes {
		if strings.Contains(desc, rnaType) {
			// Normalize to standard form
			switch rnaType {
			case "microrna", "mirna", "pre-mirna":
				return "miRNA"
			case "lncrna", "lincrna", "long non-coding rna":
				return "lncRNA"
			case "snorna":
				return "snoRNA"
			case "snrna":
				return "snRNA"
			case "pirna":
				return "piRNA"
			case "sirna":
				return "siRNA"
			case "rrna", "ribosomal rna":
				return "rRNA"
			case "trna", "transfer rna":
				return "tRNA"
			case "scrna":
				return "scRNA"
			case "tmrna":
				return "tmRNA"
			default:
				return strings.ToUpper(rnaType[:1]) + rnaType[1:]
			}
		}
	}

	return "ncRNA" // Default: non-coding RNA
}

// Extract organism count from description
// Example: "rRNA from 1 species" → 1
// Example: "rRNA from 10 species" → 10
func extractOrganismCount(description string) int32 {
	// Look for pattern "from N species"
	parts := strings.Split(strings.ToLower(description), "from")
	if len(parts) < 2 {
		return 1 // Default to 1 if not specified
	}

	speciesPart := strings.TrimSpace(parts[1])
	if strings.HasPrefix(speciesPart, "1 species") {
		return 1
	}

	// Try to extract number
	words := strings.Fields(speciesPart)
	if len(words) >= 2 && words[1] == "species" {
		var count int32
		fmt.Sscanf(words[0], "%d", &count)
		if count > 0 {
			return count
		}
	}

	return 1 // Default
}

// Helper to close readers
func closeRnacentralReaders(gz *gzip.Reader, ftpFile interface{}, client interface{}, localFile *os.File) {
	if gz != nil {
		gz.Close()
	}
	if localFile != nil {
		localFile.Close()
	}
}
