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
// Implements retry mechanism (configurable via pubchemRetryCount/pubchemRetryWaitMinutes) for network/corruption errors
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

	ftpServer := config.Dataconf["pubchem"]["ftpUrl"]
	basePath := "/pubchem/Bioassay/Extras/"
	activityPath := "bioactivities.tsv.gz"

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
		log.Printf("[PubChem Activity] [TEST MODE] Loading activities from %s (will stop after %d entries)", activityPath, testLimit)
	} else {
		log.Printf("[PubChem Activity] Loading activities from %s (3 GB compressed)", activityPath)
	}
	log.Printf("[PubChem Activity] Streaming entries directly to database (no memory accumulation)")

	var lastError error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[PubChem Activity] Retry attempt %d/%d - waiting %d minutes before retry...", attempt, maxRetries, retryWaitMinutes)
			time.Sleep(time.Duration(retryWaitMinutes) * time.Minute)
			log.Printf("[PubChem Activity] Retrying download and processing...")
		}

		err := p.processActivityFile(ftpServer, basePath, activityPath, testLimit, idLogFile)
		if err == nil {
			return // Success
		}

		lastError = err
		log.Printf("[PubChem Activity] Attempt %d failed: %v", attempt+1, err)
	}

	// All retries exhausted - panic
	log.Panicf("[PubChem Activity] FATAL: All %d retry attempts failed. Last error: %v", maxRetries+1, lastError)
}

// processActivityFile handles the actual file processing
// Returns error if processing fails (for retry mechanism)
func (p *pubchemActivity) processActivityFile(ftpServer, basePath, activityPath string, testLimit int, idLogFile *os.File) error {
	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew("pubchem_activity", ftpServer, basePath, activityPath)
	if err != nil {
		return fmt.Errorf("could not open bioactivities.tsv.gz: %v", err)
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
	activityIndex := make(map[string]int) // CID_AID → count (for unique IDs)

	for {
		// ReadString is more robust for massive ETL than Scanner
		line, err := reader.ReadString('\n')

		if err != nil {
			if err == io.EOF {
				break // End of file reached successfully
			}
			// This catches the "flate: corrupt input" and network errors
			return fmt.Errorf("error reading stream: %v", err)
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

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		lineCount++

		// Progress tracking every 5M records (more frequent updates for debugging)
		if lineCount%5000000 == 0 {
			log.Printf("[PubChem Activity] Processed %dM records, created %dM activity entries",
				lineCount/1000000, activityCount/1000000)
		}

		// Split by tab
		record := strings.Split(line, "\t")

		// Need at least 12 fields
		if len(record) < 12 {
			continue
		}

		// Format: AID, SID, SID Group, CID, Activity Outcome, Activity Name, Qualifier, Value, Protein Acc, Gene ID, TaxID, PMID
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
		proteinAccession := strings.TrimSpace(record[8])
		geneID := strings.TrimSpace(record[9])
		targetTaxIDStr := strings.TrimSpace(record[10])
		pmid := strings.TrimSpace(record[11])

		// Parse activity value and extract unit
		var value float64
		var unit string
		if valueStr != "" {
			parts := strings.Fields(valueStr)
			if len(parts) > 0 {
				parsed, err := strconv.ParseFloat(parts[0], 64)
				if err == nil {
					value = parsed
					if len(parts) > 1 {
						unit = parts[1]
                    }
                }
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
        // We map to "uniprot" dataset as we don't have separate NCBI Protein dataset
        // Some NCBI Protein accessions contain underscores (e.g., "1A5H_A")
        if proteinAccession != "" {
            p.d.addXref(activityID, fr, proteinAccession, "uniprot", false)
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

    log.Printf("[PubChem Activity] Complete:")
    log.Printf("[PubChem Activity]   - Total records processed: %d", lineCount)
    log.Printf("[PubChem Activity]   - Activity entries created: %d", activityCount)
    log.Printf("[PubChem Activity]   - Unique CID-AID pairs: %d", len(activityIndex))

    return nil // Success
}

func (p *pubchemActivity) check(e error, msg string) {
    if e != nil {
        log.Panicln(msg, e)
    }
}