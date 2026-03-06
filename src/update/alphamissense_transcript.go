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

type alphamissenseTranscript struct {
	source string
	d      *DataUpdate
}

func (amt *alphamissenseTranscript) check(err error, operation string) {
	checkWithContext(err, amt.source, operation)
}

// Main update entry point for alphamissense_transcript
func (amt *alphamissenseTranscript) update() {
	defer amt.d.wg.Done()

	log.Println("AlphaMissense Transcript: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(amt.source)
	if config.IsTestMode() {
		log.Printf("AlphaMissense Transcript: [TEST MODE] Processing up to %d entries", testLimit)
	}

	sourceID := config.Dataconf[amt.source]["id"]
	textLinkID := config.Dataconf["textlink"]["id"]

	// Process transcript-level data
	amt.parseAndSaveTranscriptData(sourceID, textLinkID, testLimit)

	log.Printf("AlphaMissense Transcript: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler
	amt.d.progChan <- &progressInfo{dataset: amt.source, done: true}
}

// parseAndSaveTranscriptData processes the transcript-level summary file
func (amt *alphamissenseTranscript) parseAndSaveTranscriptData(sourceID, textLinkID string, testLimit int) {
	filePath := config.Dataconf[amt.source]["path"]
	log.Printf("AlphaMissense Transcript: Processing transcript data from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(amt.source, "", "", filePath)
	amt.check(err, "opening AlphaMissense transcript file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount int64
	var entryCount int64

	// ID log file for transcript dataset
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, amt.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			amt.check(readErr, "reading AlphaMissense transcript file")
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

		transcriptIDVersioned := fields[0]
		meanPathogenicityStr := fields[1]

		// Parse mean pathogenicity
		meanPathogenicity, err := strconv.ParseFloat(meanPathogenicityStr, 64)
		if err != nil {
			continue
		}

		// Extract base transcript ID (without version) for entry ID
		// This enables direct linking from other datasets (RefSeq, Ensembl)
		transcriptBase := transcriptIDVersioned
		if dotIdx := strings.Index(transcriptIDVersioned, "."); dotIdx > 0 {
			transcriptBase = transcriptIDVersioned[:dotIdx]
		}

		// Entry ID is the base transcript ID (without version)
		entryID := transcriptBase

		// Create transcript-level attributes
		attr := &pbuf.AlphaMissenseTranscriptAttr{
			TranscriptId:          transcriptBase,        // Base ID (same as entry ID)
			TranscriptIdVersioned: transcriptIDVersioned, // Full ID with version
			MeanAmPathogenicity:   meanPathogenicity,
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		amt.check(err, fmt.Sprintf("marshaling transcript attributes for %s", entryID))
		amt.d.addProp3(entryID, sourceID, attrBytes)

		// Log ID for test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Create cross-reference to Ensembl transcript (bidirectional)
		amt.d.addXref(entryID, sourceID, transcriptBase, "transcript", false)

		// Text search indexing - allow searching by both versioned and base transcript ID
		amt.d.addXref(transcriptBase, textLinkID, entryID, amt.source, true)
		amt.d.addXref(transcriptIDVersioned, textLinkID, entryID, amt.source, true)

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
