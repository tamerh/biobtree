package update

import (
	"archive/tar"
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
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
	// processedIDs records UniProt accessions ingested from the swissprot tar.
	// Only allocated when largeProteinBackfill is enabled, so the backfill pass
	// can skip proteins already covered (avoids duplicate props at the 2700aa
	// fragmentation boundary). nil otherwise (no overhead on normal builds).
	processedIDs map[string]bool
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

	// Enable boundary dedup tracking only when the large-protein backfill runs.
	if config.Dataconf[a.source]["largeProteinBackfill"] == "yes" {
		a.processedIDs = make(map[string]bool)
	}

	// Get data source path
	filePath := config.Dataconf[a.source]["path"]

	// Process tar.gz file (pLDDT data from FTP)
	totalProcessed, err := a.processTarFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Fatalf("Error processing AlphaFold data: %v", err)
	}

	fmt.Printf("AlphaFold pLDDT processing complete: %d structures processed\n", totalProcessed)

	// Process PAE data from GCS proteome tars (for model organisms)
	// PAE data will be merged with pLDDT data by mergeg.go
	paeProcessed := processPAEData(a.source, a.sourceID, a.d, idLogFile, testLimit)
	totalProcessed += paeProcessed

	// Backfill large/fragmented proteins absent from swissprot_pdb_v*.tar.
	// Opt-in (largeProteinBackfill=yes) because it makes live UniProt + AlphaFold
	// API calls. No-op otherwise.
	backfilled := a.processLargeProteins(idLogFile)
	totalProcessed += backfilled

	fmt.Printf("AlphaFold total processing complete: %d entries\n", totalProcessed)

	atomic.AddUint64(&a.d.totalParsedEntry, totalProcessed)
	a.d.progChan <- &progressInfo{dataset: a.source, done: true}
}

// Process tar.gz file containing PDB files
func (a *alphafoldProcessor) processTarFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	// fmt.Printf("Opening tar file from: %s\n", filePath)
	// fmt.Printf("This may take a while for large remote files...\n")

	// Open tar file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open AlphaFold tar file: %v", err)
	}
	defer closeAlphaFoldReaders(gz, ftpFile, client, localFile)

	// fmt.Printf("✓ Tar file opened successfully\n")
	// fmt.Printf("Starting to read tar entries...\n")

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

		// Log first 10 filenames to see the structure (debug only)
		// if entriesScanned <= 10 {
		// 	fmt.Printf("  DEBUG: Entry %d: %s (size: %d bytes)\n", entriesScanned, header.Name, header.Size)
		// }

		// Log progress every 5000 entries scanned
		// if entriesScanned % 5000 == 0 {
		// 	fmt.Printf("  Scanned %d tar entries, processed %d PDB files...\n", entriesScanned, totalProcessed)
		// }

		// Only process .pdb.gz files
		if !strings.HasSuffix(header.Name, ".pdb.gz") {
			continue
		}

		// Extract UniProt ID, model ID, and fragment number from filename
		uniprotID, modelID, fragmentNum := extractIDsFromFilename(header.Name)
		if uniprotID == "" {
			continue
		}

		// Log each processed structure (debug only)
		// fmt.Printf("  [%d] Processing %s → %s\n", totalProcessed+1, modelID, uniprotID)

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
			GlobalMetric:           globalMetric,
			FractionPlddtVeryHigh:  fractions.VeryHigh,
			FractionPlddtConfident: fractions.Confident,
			FractionPlddtLow:       fractions.Low,
			FractionPlddtVeryLow:   fractions.VeryLow,
			ModelEntityId:          modelID,
			Gene:                   "", // Will be populated from UniProt if needed
			FragmentNumber:         int32(fragmentNum),
			SequenceLength:         int32(len(plddt)),
		}

		// Marshal and store on UniProt ID
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("Error marshaling AlphaFold attr for %s: %v", uniprotID, err)
			continue
		}

		a.d.addProp3(uniprotID, a.sourceID, b)

		// Track for the large-protein backfill boundary dedup (when enabled).
		if a.processedIDs != nil {
			a.processedIDs[uniprotID] = true
		}

		// Create cross-reference: AlphaFold → UniProt
		// Forward: alphafold/forward/, Reverse: uniprot/from_alphafold/
		a.d.addXref(uniprotID, a.sourceID, uniprotID, "uniprot", false)

		// Create keyword: AlphaFold model ID → UniProt entry (for search endpoint)
		// isLink=true means /ws/?i=MODEL_ID will find and return the UniProt entry
		a.d.addXref(modelID, textLinkID, uniprotID, a.source, true)

		totalProcessed++
		totalBytesRead += header.Size

		// Progress message every 1000 structures
		// if totalProcessed % 1000 == 0 {
		// 	fmt.Printf("  Processed %d structures...\n", totalProcessed)
		// }

		// Test mode: log ID and check limit
		if idLogFile != nil {
			logProcessedID(idLogFile, uniprotID)
		}

		// In test mode, stop after processing enough structures
		if testLimit > 0 && int(totalProcessed) >= testLimit {
			// fmt.Printf("  [TEST MODE] Reached limit of %d structures, stopping processing\n", testLimit)
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

// Extract UniProt ID, Model ID, and fragment number from filename
// Example: AF-Q9Y6K9-F1-model_v6.pdb.gz → Q9Y6K9, AF-Q9Y6K9-F1, 1
func extractIDsFromFilename(filename string) (uniprotID string, modelID string, fragmentNum int) {
	// Remove path and extension
	base := strings.TrimSuffix(filename, ".pdb.gz")
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}

	// Format: AF-{UniProtID}-F{fragment}-model_v{version}
	// Example: AF-Q9Y6K9-F1-model_v6
	parts := strings.Split(base, "-")
	if len(parts) < 3 {
		return "", "", 0
	}

	if parts[0] != "AF" {
		return "", "", 0
	}

	uniprotID = parts[1]
	// Model ID is everything before "-model_v"
	modelID = fmt.Sprintf("%s-%s-%s", parts[0], parts[1], parts[2])

	// Extract fragment number from F{number}
	fragmentPart := parts[2] // e.g., "F1", "F2"
	if len(fragmentPart) > 1 && fragmentPart[0] == 'F' {
		fragmentNum, _ = strconv.Atoi(fragmentPart[1:])
	}

	return uniprotID, modelID, fragmentNum
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

// afPrediction is one model entry from the AlphaFold prediction API
// (https://alphafold.ebi.ac.uk/api/prediction/{accession}).
type afPrediction struct {
	EntryId          string `json:"entryId"`
	UniprotAccession string `json:"uniprotAccession"`
	PdbUrl           string `json:"pdbUrl"`
	UniprotStart     int    `json:"uniprotStart"`
	UniprotEnd       int    `json:"uniprotEnd"`
}

// processLargeProteins backfills AlphaFold structures for large proteins that the
// swissprot_pdb_v*.tar deliberately omits (AlphaFold fragments proteins above ~2700aa
// and ships those fragments only in per-proteome archives, not the SwissProt tar).
//
// It enumerates reviewed UniProt accessions at/above the length threshold, queries the
// AlphaFold prediction API per accession, and ingests whatever models actually exist
// (canonical fragments and/or isoform models), storing one aggregate AlphaFoldAttr per
// protein (length-weighted mean pLDDT, pooled confidence fractions, total residues,
// fragment count). Proteins AlphaFold no longer models (e.g. ATM, BRCA2 in v6) simply
// stay empty — no dead links are created.
//
// This is opt-in via the alphafold dataset's largeProteinBackfill="yes" flag because it
// makes ~N live HTTP calls (one UniProt query + one AF API call per large protein, plus
// PDB downloads only for those that have models). It is fully fail-soft: any network
// error logs a warning and processing continues.
func (a *alphafoldProcessor) processLargeProteins(idLogFile *os.File) uint64 {
	if config.Dataconf[a.source]["largeProteinBackfill"] != "yes" {
		return 0
	}

	minLen := 2701
	if v, ok := config.Appconf["alphafoldLargeProteinMinLength"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minLen = n
		}
	}

	accs, err := a.fetchLargeProteinAccessions(minLen)
	if err != nil {
		log.Printf("AlphaFold: large-protein backfill skipped, could not fetch UniProt list: %v", err)
		return 0
	}
	log.Printf("AlphaFold: large-protein backfill: %d reviewed proteins >= %d aa to check via AF API", len(accs), minLen)

	var processed uint64
	apiCalls := 0
	withModels := 0

	for _, acc := range accs {
		// Skip proteins already ingested from the tar (boundary overlap).
		if a.processedIDs != nil && a.processedIDs[acc] {
			continue
		}

		apiCalls++
		models, ferr := a.fetchAlphaFoldModels(acc)
		if ferr != nil {
			// Fail-soft: skip this protein, keep going.
			continue
		}
		if len(models) == 0 {
			continue
		}

		attr := a.aggregateModels(acc, models)
		if attr == nil {
			continue
		}
		withModels++

		b, merr := ffjson.Marshal(attr)
		if merr != nil {
			continue
		}
		a.d.addProp3(acc, a.sourceID, b)
		// AlphaFold → UniProt (and reverse uniprot → alphafold).
		a.d.addXref(acc, a.sourceID, acc, "uniprot", false)
		// Model ID → UniProt entry for the search endpoint, mirroring the tar pass.
		a.d.addXref(attr.ModelEntityId, textLinkID, acc, a.source, true)
		processed++

		if idLogFile != nil {
			logProcessedID(idLogFile, acc)
		}

		if apiCalls%100 == 0 {
			log.Printf("AlphaFold backfill: checked %d/%d, %d had models, %d stored", apiCalls, len(accs), withModels, processed)
		}
	}

	log.Printf("AlphaFold large-protein backfill complete: checked %d, %d had models, %d stored", apiCalls, withModels, processed)
	return processed
}

// fetchLargeProteinAccessions returns reviewed UniProt accessions with sequence
// length >= minLen via the UniProt REST stream endpoint (one accession per line).
func (a *alphafoldProcessor) fetchLargeProteinAccessions(minLen int) ([]string, error) {
	query := fmt.Sprintf("reviewed:true AND length:[%d TO *]", minLen)
	reqURL := "https://rest.uniprot.org/uniprotkb/stream?query=" +
		url.QueryEscape(query) + "&fields=accession&format=list"

	resp, err := httpGetWithRetry(reqURL, 3)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("UniProt REST returned status %d", resp.StatusCode)
	}

	var accs []string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		acc := strings.TrimSpace(scanner.Text())
		if acc != "" {
			accs = append(accs, acc)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return accs, nil
}

// fetchAlphaFoldModels returns the AlphaFold models available for an accession,
// or an empty slice if AlphaFold has no structure for it.
func (a *alphafoldProcessor) fetchAlphaFoldModels(acc string) ([]afPrediction, error) {
	reqURL := "https://alphafold.ebi.ac.uk/api/prediction/" + url.PathEscape(acc)
	resp, err := httpGetWithRetry(reqURL, 2)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, nil // No model for this accession.
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("AF API status %d for %s", resp.StatusCode, acc)
	}

	var models []afPrediction
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}
	return models, nil
}

// aggregateModels downloads each model's PDB, pools per-residue pLDDT across all
// fragments/isoforms, and returns a single summary AlphaFoldAttr for the protein.
// Returns nil if no model yielded usable pLDDT data.
func (a *alphafoldProcessor) aggregateModels(acc string, models []afPrediction) *pbuf.AlphaFoldAttr {
	var totalResidues int
	var sum float64
	var veryHigh, confident, low, veryLow int
	modelCount := 0

	for _, m := range models {
		if m.PdbUrl == "" {
			continue
		}
		plddt, err := a.fetchPDBPlddt(m.PdbUrl)
		if err != nil || len(plddt) == 0 {
			continue
		}
		for _, s := range plddt {
			sum += s
			if s > 90 {
				veryHigh++
			} else if s >= 70 {
				confident++
			} else if s >= 50 {
				low++
			} else {
				veryLow++
			}
		}
		totalResidues += len(plddt)
		modelCount++
	}

	if totalResidues == 0 {
		return nil
	}
	total := float64(totalResidues)
	return &pbuf.AlphaFoldAttr{
		GlobalMetric:           sum / total,
		FractionPlddtVeryHigh:  float64(veryHigh) / total,
		FractionPlddtConfident: float64(confident) / total,
		FractionPlddtLow:       float64(low) / total,
		FractionPlddtVeryLow:   float64(veryLow) / total,
		ModelEntityId:          "AF-" + acc + "-F1",
		FragmentNumber:         int32(modelCount),
		SequenceLength:         int32(totalResidues),
		Version:                6,
	}
}

// fetchPDBPlddt downloads a (non-gzipped) AlphaFold PDB file and extracts pLDDT
// scores from the B-factor column, reusing the tar-path PDB parser.
func (a *alphafoldProcessor) fetchPDBPlddt(pdbURL string) ([]float64, error) {
	resp, err := httpGetWithRetry(pdbURL, 2)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PDB download status %d", resp.StatusCode)
	}
	return a.parsePDBFile(resp.Body)
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
