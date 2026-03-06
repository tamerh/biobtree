package update

import (
	"bufio"
	"biobtree/pbuf"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// pubchemAssay handles PubChem Assay dataset
type pubchemAssay struct {
	source string
	d      *DataUpdate
}

// update loads and processes PubChem bioassay metadata
func (p *pubchemAssay) update(d *DataUpdate) {
	p.d = d
	p.source = "pubchem_assay"

	// Signal completion when done
	defer p.d.wg.Done()

	log.Printf("[PubChem Assay] Starting bioassay metadata dataset update")
	log.Printf("[PubChem Assay] This dataset contains assay descriptions, targets, and statistics")
	log.Printf("[PubChem Assay] Cross-references: BioAssay→Protein, BioAssay→Gene, BioAssay→BAO")

	// Load bioassays and stream to biobtree
	p.loadAndStreamBioassays()

	// Load BAO (BioAssay Ontology) annotations and create xrefs
	p.loadBAOAnnotations()

	// Signal completion
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
	log.Printf("[PubChem Assay] Dataset update complete")
}

// loadAndStreamBioassays reads bioassays.tsv.gz and streams bioassay entries
// Implements retry mechanism with resume support - on retry, skips already processed lines
func (p *pubchemAssay) loadAndStreamBioassays() {
	// Get retry configuration from application.param.json
	maxRetries := 2 // default
	retryWaitMinutes := 2 // default
	if val, ok := config.Appconf["pubchemRetryCount"]; ok {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			maxRetries = parsed
		}
	}
	if val, ok := config.Appconf["pubchemRetryWaitMinutes"]; ok {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			retryWaitMinutes = parsed
		}
	}

	basePath := config.Dataconf["pubchem_assay"]["path"]
	bioassayPath := config.Dataconf["pubchem_assay"]["pathBioassays"]
	fullURL := basePath + bioassayPath

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit("pubchem_assay")
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	if config.IsTestMode() {
		log.Printf("[PubChem Assay] [TEST MODE] Loading bioassays from %s (will stop after %d entries)", fullURL, testLimit)
	} else {
		log.Printf("[PubChem Assay] Loading bioassays from %s (20 MB compressed)", fullURL)
	}
	log.Printf("[PubChem Assay] Streaming entries directly to database")

	// State that persists across retries for resume capability
	var lastError error
	resumeFromLine := 0
	bioassayCount := 0

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[PubChem Assay] Retry attempt %d/%d - waiting %d minutes before retry...", attempt, maxRetries, retryWaitMinutes)
			time.Sleep(time.Duration(retryWaitMinutes) * time.Minute)
			log.Printf("[PubChem Assay] Resuming from line %d (skipping %d already processed lines)...", resumeFromLine, resumeFromLine)
		}

		processedLines, newBioassays, err := p.processBioassayFile(fullURL, testLimit, idLogFile, resumeFromLine)
		bioassayCount += newBioassays

		if err == nil {
			log.Printf("[PubChem Assay] Successfully completed. Total bioassays: %d", bioassayCount)
			return // Success
		}

		// Update resume point for next retry
		resumeFromLine += processedLines
		lastError = err
		log.Printf("[PubChem Assay] Attempt %d failed at line %d: %v", attempt+1, resumeFromLine, err)
	}

	// All retries exhausted - panic
	log.Panicf("[PubChem Assay] FATAL: All %d retry attempts failed. Last error: %v", maxRetries+1, lastError)
}

// processBioassayFile handles the actual file processing
// Returns (linesProcessed, bioassaysCreated, error) for resume capability
// resumeFromLine: skip this many data lines before processing (for retry resume)
func (p *pubchemAssay) processBioassayFile(fullURL string, testLimit int, idLogFile *os.File, resumeFromLine int) (int, int, error) {
	// Download and open file (pass full FTP URL directly)
	_, gz, _, _, localFile, _, err := getDataReaderNew("pubchem_assay", "", "", fullURL)
	if err != nil {
		return 0, 0, fmt.Errorf("could not open bioassays.tsv.gz: %v", err)
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Use bufio.Reader for robust line-by-line reading
	// 4MB buffer for better performance
	reader := bufio.NewReaderSize(gz, 4*1024*1024)

	lineCount := 0
	bioassayCount := 0
	headerSkipped := false
	skippedForResume := 0
	lastSuccessfulLine := "" // Track last line for error diagnostics

	// Log resume status
	if resumeFromLine > 0 {
		log.Printf("[PubChem Assay] Skipping first %d lines (already processed)...", resumeFromLine)
	}

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			if err == io.EOF {
				break // End of file reached successfully
			}
			// This catches the "flate: corrupt input" and network errors
			// Log detailed info to help diagnose where failure occurs
			partialLine := strings.TrimSpace(line)
			if len(partialLine) > 200 {
				partialLine = partialLine[:200] + "..."
			}
			if len(lastSuccessfulLine) > 200 {
				lastSuccessfulLine = lastSuccessfulLine[:200] + "..."
			}
			log.Printf("[PubChem Assay] ERROR at line %d (total %d):", lineCount-resumeFromLine, lineCount)
			log.Printf("[PubChem Assay]   Last successful line: %s", lastSuccessfulLine)
			log.Printf("[PubChem Assay]   Partial line read: %s", partialLine)
			return lineCount, bioassayCount, fmt.Errorf("error reading stream at line %d (total line %d): %v", lineCount-resumeFromLine, lineCount, err)
		}

		line = strings.TrimSpace(line)
		lastSuccessfulLine = line // Update for error diagnostics

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip header
		if !headerSkipped {
			headerSkipped = true
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		lineCount++

		// Resume support: skip lines already processed in previous attempt
		if lineCount <= resumeFromLine {
			skippedForResume++
			// Progress for skipping (every 100K lines)
			if skippedForResume%100000 == 0 {
				log.Printf("[PubChem Assay] Skipped %dK lines for resume...", skippedForResume/1000)
			}
			continue
		}

		// Progress tracking every 50K records
		if (lineCount-resumeFromLine)%50000 == 0 {
			log.Printf("[PubChem Assay] Processed %dK bioassays", (lineCount-resumeFromLine)/1000)
		}

		// Split by tab
		record := strings.Split(line, "\t")

		// Need at least 20 fields
		if len(record) < 20 {
			continue
		}

		// Format: AID, BioAssay Name, Deposit Date, Modify Date, Source Name, Source ID,
		//         Substance Type, Outcome Type, Project Category, BioAssay Group, BioAssay Types,
		//         Protein Accessions, UniProts IDs, Gene IDs, Target TaxIDs, Taxonomy IDs,
		//         Number of Tested SIDs, Number of Active SIDs, Number of Tested CIDs, Number of Active CIDs

		aid := strings.TrimSpace(record[0])
		if aid == "" {
			continue
		}

		// Skip non-numeric AIDs (data corruption or header line)
		// PubChem assay IDs should always be numeric (e.g., "1234567")
		if len(aid) > 0 && (aid[0] < '0' || aid[0] > '9') {
			if lineCount < 10 || lineCount%100000 == 0 {
				log.Printf("[PubChem Assay] Skipping non-numeric AID: '%s' at line %d", aid, lineCount)
			}
			continue
		}

		// Extract fields
		name := strings.TrimSpace(record[1])
		depositDate := strings.TrimSpace(record[2])
		modifyDate := strings.TrimSpace(record[3])
		sourceName := strings.TrimSpace(record[4])
		sourceID := strings.TrimSpace(record[5])
		substanceType := strings.TrimSpace(record[6])
		outcomeType := strings.TrimSpace(record[7])
		projectCategory := strings.TrimSpace(record[8])
		bioassayGroup := strings.TrimSpace(record[9])
		bioassayTypesStr := strings.TrimSpace(record[10])

		proteinAccessionsStr := strings.TrimSpace(record[11])
		uniprotIDsStr := strings.TrimSpace(record[12])
		geneIDsStr := strings.TrimSpace(record[13])
		targetTaxIDsStr := strings.TrimSpace(record[14])
		testOrgTaxIDsStr := strings.TrimSpace(record[15])

		testedSIDsStr := strings.TrimSpace(record[16])
		activeSIDsStr := strings.TrimSpace(record[17])
		testedCIDsStr := strings.TrimSpace(record[18])
		activeCIDsStr := strings.TrimSpace(record[19])

		// Parse bioassay types (semicolon-separated)
		var bioassayTypes []string
		if bioassayTypesStr != "" {
			bioassayTypes = strings.Split(bioassayTypesStr, ";")
			for i := range bioassayTypes {
				bioassayTypes[i] = strings.TrimSpace(bioassayTypes[i])
			}
		}

		// Parse protein accessions (semicolon-separated, with pipes within fields)
		var proteinAccessions []string
		if proteinAccessionsStr != "" {
			// First split by semicolon (multiple entries)
			semiSplit := strings.Split(proteinAccessionsStr, ";")
			for _, field := range semiSplit {
				field = strings.TrimSpace(field)
				if field == "" {
					continue
				}
				// Then split by pipe (multiple accessions per entry)
				pipeSplit := strings.Split(field, "|")
				for _, proteinAcc := range pipeSplit {
					proteinAcc = strings.TrimSpace(proteinAcc)
					if proteinAcc != "" {
						proteinAccessions = append(proteinAccessions, proteinAcc)
					}
				}
			}
		}

		// Parse UniProt IDs (semicolon-separated, with pipes within fields)
		var uniprotIDs []string
		if uniprotIDsStr != "" {
			// First split by semicolon (multiple entries)
			semiSplit := strings.Split(uniprotIDsStr, ";")
			for _, field := range semiSplit {
				field = strings.TrimSpace(field)
				if field == "" {
					continue
				}
				// Then split by pipe (multiple IDs per entry)
				pipeSplit := strings.Split(field, "|")
				for _, uniprotID := range pipeSplit {
					uniprotID = strings.TrimSpace(uniprotID)
					if uniprotID != "" {
						uniprotIDs = append(uniprotIDs, uniprotID)
					}
				}
			}
		}

		// Parse Gene IDs (semicolon-separated, with pipes within fields)
		var geneIDs []string
		if geneIDsStr != "" {
			// First split by semicolon (multiple genes)
			semiSplit := strings.Split(geneIDsStr, ";")
			for _, field := range semiSplit {
				field = strings.TrimSpace(field)
				if field == "" {
					continue
				}
				// Then split by pipe (multiple IDs per gene)
				pipeSplit := strings.Split(field, "|")
				for _, geneID := range pipeSplit {
					geneID = strings.TrimSpace(geneID)
					if geneID != "" {
						geneIDs = append(geneIDs, geneID)
					}
				}
			}
		}

		// Parse target taxonomy IDs (semicolon-separated)
		var targetTaxIDs []int32
		if targetTaxIDsStr != "" {
			taxIDStrs := strings.Split(targetTaxIDsStr, ";")
			for _, taxIDStr := range taxIDStrs {
				taxIDStr = strings.TrimSpace(taxIDStr)
				if taxIDStr != "" {
					if taxID, err := strconv.ParseInt(taxIDStr, 10, 32); err == nil {
						targetTaxIDs = append(targetTaxIDs, int32(taxID))
					}
				}
			}
		}

		// Parse test organism taxonomy IDs (semicolon-separated)
		var testOrgTaxIDs []int32
		if testOrgTaxIDsStr != "" {
			taxIDStrs := strings.Split(testOrgTaxIDsStr, ";")
			for _, taxIDStr := range taxIDStrs {
				taxIDStr = strings.TrimSpace(taxIDStr)
				if taxIDStr != "" {
					if taxID, err := strconv.ParseInt(taxIDStr, 10, 32); err == nil {
						testOrgTaxIDs = append(testOrgTaxIDs, int32(taxID))
					}
				}
			}
		}

		// Parse statistics
		var testedSIDs, activeSIDs, testedCIDs, activeCIDs int32
		if testedSIDsStr != "" {
			if val, err := strconv.ParseInt(testedSIDsStr, 10, 32); err == nil {
				testedSIDs = int32(val)
			}
		}
		if activeSIDsStr != "" {
			if val, err := strconv.ParseInt(activeSIDsStr, 10, 32); err == nil {
				activeSIDs = int32(val)
			}
		}
		if testedCIDsStr != "" {
			if val, err := strconv.ParseInt(testedCIDsStr, 10, 32); err == nil {
				testedCIDs = int32(val)
			}
		}
		if activeCIDsStr != "" {
			if val, err := strconv.ParseInt(activeCIDsStr, 10, 32); err == nil {
				activeCIDs = int32(val)
			}
		}

		// Calculate hit rate
		var hitRate float64
		if testedCIDs > 0 {
			hitRate = float64(activeCIDs) / float64(testedCIDs)
		}

		// Create bioassay entry
		attr := pbuf.PubchemAssayAttr{
			Aid:                  aid,
			Name:                 name,
			SourceName:           sourceName,
			SourceId:             sourceID,
			SubstanceType:        substanceType,
			OutcomeType:          outcomeType,
			ProjectCategory:      projectCategory,
			BioassayGroup:        bioassayGroup,
			BioassayTypes:        bioassayTypes,
			ProteinAccessions:    proteinAccessions,
			UniprotIds:           uniprotIDs,
			GeneIds:              geneIDs,
			TargetTaxids:         targetTaxIDs,
			TestOrganismTaxids:   testOrgTaxIDs,
			TestedSids:           testedSIDs,
			ActiveSids:           activeSIDs,
			TestedCids:           testedCIDs,
			ActiveCids:           activeCIDs,
			HitRate:              hitRate,
			DepositDate:          depositDate,
			ModifyDate:           modifyDate,
		}

		// Marshal and send to biobtree immediately
		b, _ := ffjson.Marshal(attr)
		fr := config.Dataconf["pubchem_assay"]["id"]
		p.d.addProp3(aid, fr, b)

		// Create cross-references
		// Debug logging for first few bioassays
		if bioassayCount < 5 {
			log.Printf("[PubChem Assay] DEBUG: AID=%s, Proteins=%v, UniProts=%v, Genes=%v",
				aid, proteinAccessions, uniprotIDs, geneIDs)
		}

		// BioAssay → Protein (via NCBI Protein accessions)
		for _, proteinAcc := range proteinAccessions {
			if proteinAcc != "" {
				//log.Printf("[PubChem Assay] DEBUG: Adding protein xref: AID=%s -> Protein=%s", aid, proteinAcc)
				p.d.addXref(aid, fr, proteinAcc, "protein", false)
			}
		}

		// BioAssay → UniProt
		for _, uniprotID := range uniprotIDs {
			if uniprotID != "" {
				// Validate UniProt ID format: should start with letter followed by digit
				// Skip invalid IDs like "0MID: 11262084" (corrupted PMID)
				if len(uniprotID) >= 2 {
					first := uniprotID[0]
					second := uniprotID[1]
					if (first >= 'A' && first <= 'Z') && (second >= '0' && second <= '9') {
						p.d.addXref(aid, fr, uniprotID, "uniprot", false)
					} else {
						log.Printf("[PubChem Assay] Skipping invalid UniProt ID '%s' for AID=%s", uniprotID, aid)
					}
				}
			}
		}

		// BioAssay → Gene → Ensembl (NCBI Gene ID / Entrez Gene ID)
		// Use lookup to find Entrez entry, then extract Ensembl gene ID
		for _, geneID := range geneIDs {
			if geneID != "" {
				//log.Printf("[PubChem Assay] ✓ Gene mapping: AID=%s -> Entrez Gene %s", aid, geneID)
				p.d.addXrefEnsemblViaEntrez(geneID, aid, fr)
			}
		}

		bioassayCount++

		// Test mode: log ID
		if idLogFile != nil {
			logProcessedID(idLogFile, aid)
		}

		// Test mode: stop after creating enough bioassays
		if config.IsTestMode() && bioassayCount >= testLimit {
			log.Printf("[PubChem Assay] Test mode: Stopping after creating %d bioassays", bioassayCount)
			break
		}
	}

	log.Printf("[PubChem Assay] Batch complete:")
	log.Printf("[PubChem Assay]   - Lines in this batch: %d (resumed from %d)", lineCount-resumeFromLine, resumeFromLine)
	log.Printf("[PubChem Assay]   - Bioassays created in batch: %d", bioassayCount)

	return lineCount - resumeFromLine, bioassayCount, nil // Success - return lines processed in THIS batch
}

func (p *pubchemAssay) check(e error, msg string) {
	if e != nil {
		log.Panicln(msg, e)
	}
}

// loadBAOAnnotations loads BAO (BioAssay Ontology) annotations from Aid2CategorizedComment.gz
// and creates cross-references from PubChem Assay AIDs to BAO terms
func (p *pubchemAssay) loadBAOAnnotations() {
	// Check if BAO dataset is configured
	if _, exists := config.Dataconf["bao"]; !exists {
		log.Printf("[PubChem Assay] BAO dataset not configured, skipping BAO annotations")
		return
	}

	basePath := config.Dataconf["pubchem_assay"]["path"]
	annotationPath := "Aid2CategorizedComment.gz"
	fullURL := basePath + annotationPath

	log.Printf("[PubChem Assay] Loading BAO annotations from %s", fullURL)

	// Download and open file (pass full FTP URL directly)
	br, gz, _, _, localFile, _, err := getDataReaderNew("pubchem_assay", "", "", fullURL)
	if err != nil {
		log.Printf("[PubChem Assay] Warning: Could not load BAO annotations: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Use bufio.Reader for robust line-by-line reading
	reader := bufio.NewReaderSize(br, 4*1024*1024)

	lineCount := 0
	baoXrefCount := 0
	headerSkipped := false
	fr := config.Dataconf["pubchem_assay"]["id"]

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[PubChem Assay] Error reading BAO annotations: %v", err)
			break
		}

		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip header
		if !headerSkipped {
			headerSkipped = true
			continue
		}

		lineCount++

		// Split by tab: AID \t Title \t Comment
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}

		aid := strings.TrimSpace(parts[0])
		title := strings.TrimSpace(parts[1])
		value := strings.TrimSpace(parts[2])

		// Only process BAO annotations
		if !strings.HasPrefix(title, "BAO:") {
			continue
		}

		if aid == "" || value == "" {
			continue
		}

		// Try multiple matching strategies for BAO term lookup
		// Values can be like "luminescence: bioluminescence" or just "primary"

		// Strategy 1: Try the full value
		p.d.addXrefViaKeyword(value, "bao", aid, p.source, fr, false)

		// Strategy 2: If value contains ":", try the last part (e.g., "bioluminescence" from "luminescence: bioluminescence")
		if strings.Contains(value, ":") {
			parts := strings.Split(value, ":")
			lastPart := strings.TrimSpace(parts[len(parts)-1])
			if lastPart != "" && lastPart != value {
				p.d.addXrefViaKeyword(lastPart, "bao", aid, p.source, fr, false)
			}
		}

		// Strategy 3: Try with "assay" suffix (e.g., "primary" -> "primary assay")
		withAssay := value + " assay"
		p.d.addXrefViaKeyword(withAssay, "bao", aid, p.source, fr, false)

		baoXrefCount++

		// Progress reporting every 1000 BAO xrefs
		if baoXrefCount%1000 == 0 {
			log.Printf("[PubChem Assay] Created %d BAO xrefs...", baoXrefCount)
		}

		// Test mode: stop early
		if config.IsTestMode() && baoXrefCount >= 500 {
			log.Printf("[PubChem Assay] Test mode: Stopping after %d BAO xrefs", baoXrefCount)
			break
		}
	}

	log.Printf("[PubChem Assay] BAO annotation loading complete:")
	log.Printf("[PubChem Assay]   - Total lines processed: %d", lineCount)
	log.Printf("[PubChem Assay]   - BAO xrefs created: %d", baoXrefCount)
}
