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

type alphamissense struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (am *alphamissense) check(err error, operation string) {
	checkWithContext(err, am.source, operation)
}

// Main update entry point
func (am *alphamissense) update() {
	defer am.d.wg.Done()

	log.Println("AlphaMissense: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(am.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, am.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("AlphaMissense: [TEST MODE] Processing up to %d variants", testLimit)
	}

	sourceID := config.Dataconf[am.source]["id"]
	textLinkID := config.Dataconf["textlink"]["id"]

	// Get transcript dataset source ID (separate dataset)
	transcriptSourceID := ""
	if conf, ok := config.Dataconf["alphamissense_transcript"]; ok {
		transcriptSourceID = conf["id"]
	}

	// Phase 1: Process main variants file (has UniProt links)
	am.parseAndSaveVariants(testLimit, idLogFile, sourceID, textLinkID, transcriptSourceID)

	// Phase 2: Stream isoforms file to add additional transcript xrefs (memory efficient)
	if pathIsoforms, ok := config.Dataconf[am.source]["pathIsoforms"]; ok && pathIsoforms != "" {
		am.streamIsoformsXrefs(sourceID, transcriptSourceID, testLimit)
	}

	// Note: alphamissense_transcript is now a separate dataset with its own parser

	log.Printf("AlphaMissense: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler
	am.d.progChan <- &progressInfo{dataset: am.source, done: true}
}

// streamIsoformsXrefs streams through isoforms file and adds xrefs directly (memory efficient)
func (am *alphamissense) streamIsoformsXrefs(sourceID, transcriptSourceID string, testLimit int) {
	pathIsoforms := config.Dataconf[am.source]["pathIsoforms"]
	log.Printf("AlphaMissense: Streaming isoforms xrefs from %s", pathIsoforms)
	if testLimit > 0 {
		log.Printf("AlphaMissense: [TEST MODE] Processing up to %d isoform entries", testLimit)
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(am.source, "", "", pathIsoforms)
	am.check(err, "opening AlphaMissense isoforms file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var entryCount int64
	var xrefCount int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			am.check(readErr, "reading AlphaMissense isoforms file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		entryCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// Format: CHROM	POS	REF	ALT	genome	transcript_id	protein_variant	am_pathogenicity	am_class
		fields := strings.Split(line, "\t")
		if len(fields) < 9 {
			continue
		}

		chrom := fields[0]
		posStr := fields[1]
		refAllele := fields[2]
		altAllele := fields[3]
		transcriptID := fields[5]

		// Normalize chromosome
		if strings.HasPrefix(chrom, "chr") {
			chrom = chrom[3:]
		}

		// Create variant ID (entry ID)
		variantID := fmt.Sprintf("%s:%s:%s:%s", chrom, posStr, refAllele, altAllele)

		// Add xrefs for this isoform directly (no memory storage)
		if transcriptID != "" {
			// Xref to transcript dataset (base ID without version)
			transcriptBase := transcriptID
			if dotIdx := strings.Index(transcriptID, "."); dotIdx > 0 {
				transcriptBase = transcriptID[:dotIdx]
			}
			am.d.addXref(variantID, sourceID, transcriptBase, "transcript", false)

			// Xref to alphamissense_transcript dataset (base ID - matches new entry format)
			if transcriptSourceID != "" {
				am.d.addXref(variantID, sourceID, transcriptBase, "alphamissense_transcript", false)
			}
			xrefCount++
		}

		// Progress logging
		if entryCount%10000000 == 0 {
			log.Printf("AlphaMissense: Processed %d isoform entries (%d xrefs)...", entryCount, xrefCount)
		}

		// Test mode limit
		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("AlphaMissense: [TEST MODE] Reached isoform limit of %d entries", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	log.Printf("AlphaMissense: Processed %d isoform entries, created %d transcript xrefs", entryCount, xrefCount)
}

// parseAndSaveVariants processes the main AlphaMissense TSV file
func (am *alphamissense) parseAndSaveVariants(testLimit int, idLogFile *os.File, sourceID, textLinkID, transcriptSourceID string) {
	filePath := config.Dataconf[am.source]["path"]
	log.Printf("AlphaMissense: Processing variants from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(am.source, "", "", filePath)
	am.check(err, "opening AlphaMissense TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount int64
	var entryCount int64
	var skippedCount int64

	// Track statistics
	var likelyBenign, ambiguous, likelyPathogenic int64

	// Progress tracking
	var totalRead int64
	var previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			am.check(readErr, "reading AlphaMissense TSV file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		totalRead += int64(len(line))
		lineCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// Format: CHROM	POS	REF	ALT	genome	uniprot_id	transcript_id	protein_variant	am_pathogenicity	am_class
		fields := strings.Split(line, "\t")
		if len(fields) < 10 {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("AlphaMissense: SKIP line %d: Not enough fields (%d < 10)", lineCount, len(fields))
			}
			continue
		}

		chrom := fields[0]
		posStr := fields[1]
		refAllele := fields[2]
		altAllele := fields[3]
		uniprotID := fields[5]
		transcriptID := fields[6]
		proteinVariant := fields[7]
		pathogenicityStr := fields[8]
		amClass := fields[9]

		// Normalize chromosome
		if strings.HasPrefix(chrom, "chr") {
			chrom = chrom[3:]
		}

		// Parse position
		pos, err := strconv.ParseInt(posStr, 10, 64)
		if err != nil {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("AlphaMissense: SKIP line %d: Invalid position %q", lineCount, posStr)
			}
			continue
		}

		// Parse pathogenicity score
		pathogenicity, err := strconv.ParseFloat(pathogenicityStr, 64)
		if err != nil {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("AlphaMissense: SKIP line %d: Invalid pathogenicity %q", lineCount, pathogenicityStr)
			}
			continue
		}

		// Create entry ID
		entryID := fmt.Sprintf("%s:%d:%s:%s", chrom, pos, refAllele, altAllele)

		// Track classification statistics
		switch amClass {
		case "likely_benign":
			likelyBenign++
		case "ambiguous":
			ambiguous++
		case "likely_pathogenic":
			likelyPathogenic++
		}

		// Build transcript IDs list (just canonical from main file)
		// Additional isoform transcripts are added via xrefs in Phase 2
		var transcriptIDs []string
		if transcriptID != "" {
			transcriptIDs = []string{transcriptID}
		}

		// Create attributes
		attr := &pbuf.AlphaMissenseAttr{
			Chromosome:      chrom,
			Position:        pos,
			RefAllele:       refAllele,
			AltAllele:       altAllele,
			UniprotId:       uniprotID,
			TranscriptIds:   transcriptIDs,
			ProteinVariant:  proteinVariant,
			GeneSymbol:      "",
			AmPathogenicity: pathogenicity,
			AmClass:         amClass,
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		am.check(err, fmt.Sprintf("marshaling attributes for %s", entryID))
		am.d.addProp3(entryID, sourceID, attrBytes)

		// Log ID for test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Create cross-references
		am.createCrossReferences(entryID, sourceID, textLinkID, transcriptSourceID, attr)

		entryCount++

		// Progress reporting
		elapsed := int64(time.Since(am.d.start).Seconds())
		if elapsed > previous+am.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			am.d.progChan <- &progressInfo{dataset: am.source, currentKBPerSec: kbytesPerSecond}
		}

		// Progress logging
		if entryCount%1000000 == 0 {
			log.Printf("AlphaMissense: Processed %d variants...", entryCount)
		}

		// Test mode limit
		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("AlphaMissense: [TEST MODE] Reached limit of %d variants", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	// Log final statistics
	log.Printf("AlphaMissense: Processed %d variants (skipped %d malformed lines)", entryCount, skippedCount)
	log.Printf("AlphaMissense: Classification: likely_benign=%d (%.1f%%), ambiguous=%d (%.1f%%), likely_pathogenic=%d (%.1f%%)",
		likelyBenign, float64(likelyBenign)/float64(entryCount)*100,
		ambiguous, float64(ambiguous)/float64(entryCount)*100,
		likelyPathogenic, float64(likelyPathogenic)/float64(entryCount)*100)
}

// parseAndSaveTranscriptData processes the transcript-level summary file (alphamissense_transcript dataset)
func (am *alphamissense) parseAndSaveTranscriptData(transcriptSourceID, textLinkID, variantSourceID string, testLimit int) {
	pathGene := config.Dataconf["alphamissense_transcript"]["path"]
	log.Printf("AlphaMissense Transcript: Processing transcript data from %s", pathGene)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("alphamissense_transcript", "", "", pathGene)
	am.check(err, "opening AlphaMissense transcript file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount int64
	var entryCount int64

	// ID log file for transcript dataset
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, "alphamissense_transcript_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			am.check(readErr, "reading AlphaMissense transcript file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		lineCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// Format: transcript_id	mean_am_pathogenicity
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}

		transcriptID := fields[0]
		meanPathogenicityStr := fields[1]

		// Parse mean pathogenicity
		meanPathogenicity, err := strconv.ParseFloat(meanPathogenicityStr, 64)
		if err != nil {
			continue
		}

		// Entry ID is the transcript ID directly
		entryID := transcriptID

		// Create transcript-level attributes
		attr := &pbuf.AlphaMissenseTranscriptAttr{
			TranscriptId:        transcriptID,
			MeanAmPathogenicity: meanPathogenicity,
		}

		// Marshal and save using alphamissense_transcript source ID
		attrBytes, err := ffjson.Marshal(attr)
		am.check(err, fmt.Sprintf("marshaling transcript attributes for %s", entryID))
		am.d.addProp3(entryID, transcriptSourceID, attrBytes)

		// Log ID for test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Create cross-reference to Ensembl transcript
		// Strip version for cross-referencing
		transcriptBase := transcriptID
		if dotIdx := strings.Index(transcriptID, "."); dotIdx > 0 {
			transcriptBase = transcriptID[:dotIdx]
		}
		am.d.addXref(entryID, transcriptSourceID, transcriptBase, "transcript", false)

		// Text search indexing - allow searching by transcript ID
		am.d.addXref(transcriptBase, textLinkID, entryID, "alphamissense_transcript", true)
		am.d.addXref(transcriptID, textLinkID, entryID, "alphamissense_transcript", true)

		entryCount++

		// Test mode limit
		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("AlphaMissense Transcript: [TEST MODE] Reached limit of %d entries", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	log.Printf("AlphaMissense Transcript: Processed %d transcript entries", entryCount)
}

// createCrossReferences creates cross-references for AlphaMissense variant entries
func (am *alphamissense) createCrossReferences(entryID, sourceID, textLinkID, transcriptSourceID string, attr *pbuf.AlphaMissenseAttr) {
	// Cross-reference to UniProt protein
	if attr.UniprotId != "" {
		am.d.addXref(entryID, sourceID, attr.UniprotId, "uniprot", false)
	}

	// Cross-reference to Ensembl transcripts (all isoforms)
	for _, transcriptID := range attr.TranscriptIds {
		if transcriptID != "" {
			transcriptBase := transcriptID
			if dotIdx := strings.Index(transcriptID, "."); dotIdx > 0 {
				transcriptBase = transcriptID[:dotIdx]
			}
			am.d.addXref(entryID, sourceID, transcriptBase, "transcript", false)

			// Cross-reference to alphamissense_transcript dataset (base ID - matches new entry format)
			if transcriptSourceID != "" {
				am.d.addXref(entryID, sourceID, transcriptBase, "alphamissense_transcript", false)
			}
		}
	}

	// Text search indexing
	if attr.UniprotId != "" {
		am.d.addXref(attr.UniprotId, textLinkID, entryID, am.source, true)
	}

	if attr.ProteinVariant != "" && len(attr.ProteinVariant) <= 20 {
		am.d.addXref(attr.ProteinVariant, textLinkID, entryID, am.source, true)
	}
}
