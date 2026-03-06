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

// pubchemActivity handles PubChem BioActivity dataset
type pubchemActivity struct {
	source string
	d      *DataUpdate
}

// update loads and processes PubChem bioactivity data
func (p *pubchemActivity) update(d *DataUpdate) {
	p.d = d
	p.source = "pubchem_activity"

	// Signal completion when done
	defer p.d.wg.Done()

	log.Printf("[PubChem Activity] Starting bioactivity dataset update")
	log.Printf("[PubChem Activity] This dataset creates separate entries for each activity measurement")
	log.Printf("[PubChem Activity] Cross-references: Activity→Compound, Activity→Protein, Activity→Gene")

	// Load bioactivities and stream to biobtree
	p.loadAndStreamActivities()

	// Signal completion
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
	log.Printf("[PubChem Activity] Dataset update complete")
}

// loadAndStreamActivities reads bioactivities.tsv.gz and streams activity entries
// Uses bufio.Reader instead of Scanner for better handling of massive streams
// Implements retry mechanism with resume support - on retry, skips already processed lines
func (p *pubchemActivity) loadAndStreamActivities() {
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

	basePath := config.Dataconf["pubchem_activity"]["path"]
	activityPath := config.Dataconf["pubchem_activity"]["pathBioactivities"]
	fullURL := basePath + activityPath

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit("pubchem_activity")
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	if config.IsTestMode() {
		log.Printf("[PubChem Activity] [TEST MODE] Loading activities from %s (will stop after %d entries)", fullURL, testLimit)
	} else {
		log.Printf("[PubChem Activity] Loading activities from %s (3 GB compressed)", fullURL)
	}
	log.Printf("[PubChem Activity] Streaming entries directly to database (no memory accumulation)")

	// State that persists across retries for resume capability
	var lastError error
	resumeFromLine := 0                        // Line to resume from on retry
	activityIndex := make(map[string]int)      // CID_AID → count (persists across retries)
	activityCount := 0                         // Total activities created (persists across retries)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[PubChem Activity] Retry attempt %d/%d - waiting %d minutes before retry...", attempt, maxRetries, retryWaitMinutes)
			time.Sleep(time.Duration(retryWaitMinutes) * time.Minute)
			log.Printf("[PubChem Activity] Resuming from line %d (skipping %d already processed lines)...", resumeFromLine, resumeFromLine)
		}

		processedLines, newActivityCount, err := p.processActivityFile(fullURL, testLimit, idLogFile, resumeFromLine, activityIndex)
		activityCount += newActivityCount

		if err == nil {
			log.Printf("[PubChem Activity] Successfully completed. Total activities: %d", activityCount)
			return // Success
		}

		// Update resume point for next retry
		resumeFromLine += processedLines
		lastError = err
		log.Printf("[PubChem Activity] Attempt %d failed at line %d: %v", attempt+1, resumeFromLine, err)
	}

	// All retries exhausted - panic
	log.Panicf("[PubChem Activity] FATAL: All %d retry attempts failed. Last error: %v", maxRetries+1, lastError)
}

// processActivityFile handles the actual file processing
// Returns (linesProcessed, activitiesCreated, error) for resume capability
// resumeFromLine: skip this many data lines before processing (for retry resume)
// activityIndex: shared map that persists across retries for consistent activity IDs
func (p *pubchemActivity) processActivityFile(fullURL string, testLimit int, idLogFile *os.File, resumeFromLine int, activityIndex map[string]int) (int, int, error) {
	// Download and open file (pass full FTP URL directly)
	br, gz, _, _, localFile, _, err := getDataReaderNew("pubchem_activity", "", "", fullURL)
	if err != nil {
		return 0, 0, fmt.Errorf("could not open bioactivities.tsv.gz: %v", err)
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Use bufio.NewReader instead of Scanner.
	// Scanner can fail on "token too long" or hide specific gzip errors.
	// 4MB buffer is generous for performance on large files.
	reader := bufio.NewReaderSize(br, 4*1024*1024)

	lineCount := 0
	activityCount := 0
	headerSkipped := false
	skippedForResume := 0
	lastSuccessfulLine := "" // Track last line for error diagnostics

	// Log resume status
	if resumeFromLine > 0 {
		log.Printf("[PubChem Activity] Skipping first %d lines (already processed)...", resumeFromLine)
	}

	for {
		// ReadString is more robust for massive ETL than Scanner
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
			log.Printf("[PubChem Activity] ERROR at line %d (total %d):", lineCount-resumeFromLine, lineCount)
			log.Printf("[PubChem Activity]   Last successful line: %s", lastSuccessfulLine)
			log.Printf("[PubChem Activity]   Partial line read: %s", partialLine)
			return lineCount, activityCount, fmt.Errorf("error reading stream at line %d (total line %d): %v", lineCount-resumeFromLine, lineCount, err)
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
			// Progress for skipping (every 10M lines)
			if skippedForResume%10000000 == 0 {
				log.Printf("[PubChem Activity] Skipped %dM lines for resume...", skippedForResume/1000000)
			}
			continue
		}

		// Progress tracking every 5M records (more frequent updates for debugging)
		if (lineCount-resumeFromLine)%5000000 == 0 {
			log.Printf("[PubChem Activity] Processed %dM records (total line %dM), created %dM activity entries",
				(lineCount-resumeFromLine)/1000000, lineCount/1000000, activityCount/1000000)
		}

		// Split by tab
		record := strings.Split(line, "\t")

		// Need at least 13 fields
		if len(record) < 13 {
			continue
		}

		// Format: AID, SID, SID Group, CID, Activity Outcome, Activity Name, Activity Qualifier, Activity Value, Activity Unit, Protein Accession, Gene ID, Target TaxID, PMID
		//         0     1    2          3    4                 5              6                   7               8              9                   10       11            12
		cid := strings.TrimSpace(record[3])
		aid := strings.TrimSpace(record[0])

		// Skip empty CID or AID
		if cid == "" || aid == "" {
			continue
		}

		// Extract fields
		activityOutcome := strings.TrimSpace(record[4])
		activityType := strings.TrimSpace(record[5])
		qualifier := strings.TrimSpace(record[6])
		valueStr := strings.TrimSpace(record[7])
		unitFromColumn := strings.TrimSpace(record[8])
		proteinAccession := strings.TrimSpace(record[9])
		geneID := strings.TrimSpace(record[10])
		targetTaxIDStr := strings.TrimSpace(record[11])
		pmid := strings.TrimSpace(record[12])

		// Parse activity value - unit is now in separate column
		var value float64
		unit := unitFromColumn
		if valueStr != "" {
			parsed, err := strconv.ParseFloat(valueStr, 64)
			if err == nil {
				value = parsed
			}
		}

        // Parse target taxonomy ID
        var targetTaxID int32
        if targetTaxIDStr != "" {
            parsed, err := strconv.ParseInt(targetTaxIDStr, 10, 32)
            if err == nil {
                targetTaxID = int32(parsed)
            }
        }

        // Generate unique activity ID: CID_AID_index
        cidAidKey := cid + "_" + aid
        activityIndex[cidAidKey]++
        activityID := fmt.Sprintf("%s_%s_%d", cid, aid, activityIndex[cidAidKey])

        // Create activity entry
        attr := pbuf.PubchemActivityAttr{
            ActivityId:       activityID,
            Cid:              cid,
            Aid:              aid,
            ActivityOutcome:  activityOutcome,
            ActivityType:     strings.ToLower(activityType), // Normalize
            Qualifier:        qualifier,
            Value:            value,
            Unit:             unit,
            ProteinAccession: proteinAccession,
            GeneId:           geneID,
            TargetTaxid:      targetTaxID,
            Pmid:             pmid,
        }

        // Marshal and send to biobtree immediately
        b, _ := ffjson.Marshal(attr)
        fr := config.Dataconf["pubchem_activity"]["id"]
        p.d.addProp3(activityID, fr, b)

        // Create cross-references
        // Activity → Compound
        p.d.addXref(activityID, fr, cid, "pubchem", false)

        // Activity → BioAssay (creates bidirectional link)
        p.d.addXref(activityID, fr, aid, "pubchem_assay", false)

        // Activity → Protein (if present)
        // Note: According to PubChem docs, this is NCBI Protein accession
        // Can be UniProt (P12345) or PDB (1A5H_A) format
        // UniProt: starts with letter + digit (e.g., P12345, Q9Y6K9)
        // PDB: starts with digit + alphanum (e.g., 1O37_A, 1A5H_A)
        if proteinAccession != "" && len(proteinAccession) >= 2 {
            first := proteinAccession[0]
            second := proteinAccession[1]
            isUniProt := (first >= 'A' && first <= 'Z') && (second >= '0' && second <= '9')
            isPDB := (first >= '0' && first <= '9') && ((second >= 'A' && second <= 'Z') || (second >= '0' && second <= '9'))
            if isUniProt {
                p.d.addXref(activityID, fr, proteinAccession, "uniprot", false)
            } else if isPDB {
                // Extract PDB ID (first 4 chars) and link to PDB dataset
                if len(proteinAccession) >= 4 {
                    pdbID := proteinAccession[:4]
                    p.d.addXref(activityID, fr, pdbID, "pdb", false)
                }
            }
            // Silently skip other formats (no more logging)
        }

        // Activity → Gene → Ensembl (if present)
        // gene_id is NCBI Gene ID (Entrez Gene ID)
        // Use lookup to find Entrez entry, then extract Ensembl gene ID
        if geneID != "" {
            //log.Printf("[PubChem Activity] ✓ Gene mapping: activity %s -> Entrez Gene %s", activityID, geneID)
            p.d.addXrefEnsemblViaEntrez(geneID, activityID, fr)
        } else {
            if activityCount < 5 { // Only log first few for debugging
                log.Printf("[PubChem Activity] DEBUG: No gene_id for activity %s", activityID)
            }
        }

        activityCount++

        // Test mode: log ID
        if idLogFile != nil {
            logProcessedID(idLogFile, activityID)
        }

        // Test mode: stop after creating enough valid activities
        if config.IsTestMode() && activityCount >= testLimit {
            log.Printf("[PubChem Activity] Test mode: Stopping after creating %d activities", activityCount)
            break
        }
    }

    log.Printf("[PubChem Activity] Batch complete:")
    log.Printf("[PubChem Activity]   - Lines in this batch: %d (resumed from %d)", lineCount-resumeFromLine, resumeFromLine)
    log.Printf("[PubChem Activity]   - Activity entries created in batch: %d", activityCount)
    log.Printf("[PubChem Activity]   - Unique CID-AID pairs (cumulative): %d", len(activityIndex))

    return lineCount - resumeFromLine, activityCount, nil // Success - return lines processed in THIS batch
}

func (p *pubchemActivity) check(e error, msg string) {
    if e != nil {
        log.Panicln(msg, e)
    }
}