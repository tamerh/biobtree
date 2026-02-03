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

	// Process ID mapping file for cross-references
	idMappingPath := config.Dataconf[r.source]["pathIdMapping"]
	if idMappingPath != "" {
		fmt.Printf("Processing RNACentral ID mappings for cross-references...\n")
		xrefCount, err := r.processIdMappingFile(idMappingPath, testLimit)
		if err != nil {
			log.Printf("Warning: Error processing ID mapping file: %v", err)
		} else {
			fmt.Printf("RNACentral ID mapping complete: %d cross-references added\n", xrefCount)
		}
	}

	atomic.AddUint64(&r.d.totalParsedEntry, totalProcessed)
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
					if config.IsTestMode() && shouldStopProcessing(testLimit, int(totalProcessed)) {
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
	if currentID != "" && !shouldStopProcessing(testLimit, int(totalProcessed)) {
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

// Map RNACentral database names to biobtree dataset names
// Includes all datasets that exist in biobtree configuration (source*.json, xref*.json)
var rnaCentralDbMapping = map[string]string{
	// Ensembl variants
	"ENSEMBL":          "ensembl",
	"ENSEMBL_GENCODE":  "ensembl",
	"ENSEMBL_FUNGI":    "ensembl",
	"ENSEMBL_METAZOA":  "ensembl",
	"ENSEMBL_PLANTS":   "ensembl",
	"ENSEMBL_PROTISTS": "ensembl",
	// Core databases
	"REFSEQ": "refseq",
	// NOTE: INTACT excluded - RNACentral id_mapping has wrong ID format (INTACT:URS... instead of EBI-xxxxx)
	// IntAct's own parser creates correct RNACentral xrefs with 19,357 real interaction IDs
	"HGNC": "hgnc",
	"PDB":  "pdb",
	"ENA":  "ena",
	// Model organism databases
	"MGI":     "mgi",
	"RGD":     "rgd",
	"FLYBASE": "flybase",
	// NOTE: WORMBASE excluded - RNACentral uses gene IDs (WBGene...) but biobtree uses cosmid IDs (4R79.1A)
	"SGD": "sgd",
	// NOTE: POMBASE excluded - RNACentral uses RNA IDs (SPNCRNA...) but biobtree uses gene IDs (SPAC1002.01)
	"TAIR": "tair",
}

// Process id_mapping.tsv.gz file for cross-references
// Format: URS_ID\tDatabase\tExternal_ID\tTaxon\tRNA_Type\tGene_ID
func (r *rnacentralProcessor) processIdMappingFile(filePath string, testLimit int) (uint64, error) {
	// Open ID mapping file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open RNACentral ID mapping file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	// Create reader for line by line reading
	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024) // 1MB buffer
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024) // 1MB buffer
	}

	var xrefCount uint64
	var totalBytesRead int64
	var previous int64
	var uniqueUrsIDs int
	lastUrsID := ""

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return xrefCount, fmt.Errorf("error reading ID mapping: %v", err)
		}
		if len(line) == 0 && err == io.EOF {
			break
		}

		totalBytesRead += int64(len(line))
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if len(line) == 0 {
			continue
		}

		// Parse TSV: URS_ID, Database, External_ID, Taxon, RNA_Type, Gene_ID
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		ursID := fields[0]
		dbName := fields[1]
		externalID := fields[2]

		// Track unique URS IDs for test mode limit
		if ursID != lastUrsID {
			uniqueUrsIDs++
			lastUrsID = ursID
			if config.IsTestMode() && shouldStopProcessing(testLimit, uniqueUrsIDs) {
				break
			}
		}

		// Map database name to biobtree dataset
		biobtreeDataset, ok := rnaCentralDbMapping[dbName]
		if !ok {
			continue // Skip unmapped databases
		}

		// Normalize external ID based on database format
		normalizedID := externalID
		if dbName == "ENA" {
			// ENA format in RNACentral: "GU786683.1:1..200:rRNA" -> extract just "GU786683"
			// Step 1: Strip coordinates (everything after first colon)
			if colonIdx := strings.Index(externalID, ":"); colonIdx > 0 {
				normalizedID = externalID[:colonIdx]
			}
			// Step 2: Strip version (everything after dot) - biobtree ENA uses unversioned IDs
			if dotIdx := strings.LastIndex(normalizedID, "."); dotIdx > 0 {
				normalizedID = normalizedID[:dotIdx]
			}
		} else if dbName == "PDB" {
			// PDB format in RNACentral: "157D_A" (with chain) -> extract just "157D"
			if underscoreIdx := strings.Index(externalID, "_"); underscoreIdx > 0 {
				normalizedID = externalID[:underscoreIdx]
			}
		} else if dbName == "TAIR" {
			// TAIR format in RNACentral: "AT1G01270.1" (with version) -> extract just "AT1G01270"
			if dotIdx := strings.LastIndex(externalID, "."); dotIdx > 0 {
				normalizedID = externalID[:dotIdx]
			}
		}

		// Add cross-reference: RNACentral -> external database
		r.d.addXref(ursID, r.sourceID, normalizedID, biobtreeDataset, false)
		xrefCount++

		// For ENSEMBL, also add the gene ID if present (column 6)
		if strings.HasPrefix(dbName, "ENSEMBL") && len(fields) >= 6 && fields[5] != "" {
			geneID := strings.Split(fields[5], ".")[0] // Remove version
			if geneID != "" && geneID != externalID {
				r.d.addXref(ursID, r.sourceID, geneID, "ensembl", false)
				xrefCount++
			}
		}

		// Progress reporting
		elapsed := int64(time.Since(r.d.start).Seconds())
		if elapsed > previous+r.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			r.d.progChan <- &progressInfo{dataset: r.source + "_idmap", currentKBPerSec: kbytesPerSecond}
		}

		if err == io.EOF {
			break
		}
	}

	return xrefCount, nil
}
