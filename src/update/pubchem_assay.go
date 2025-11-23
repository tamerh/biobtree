package update

import (
	"bufio"
	"biobtree/pbuf"
	"github.com/pquerna/ffjson/ffjson"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
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
	log.Printf("[PubChem Assay] Cross-references: BioAssay→Protein, BioAssay→Gene")

	// Load bioassays and stream to biobtree
	p.loadAndStreamBioassays()

	// Signal completion
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
	log.Printf("[PubChem Assay] Dataset update complete")
}

// loadAndStreamBioassays reads bioassays.tsv.gz and streams bioassay entries
func (p *pubchemAssay) loadAndStreamBioassays() {
	ftpServer := config.Dataconf["pubchem_assay"]["ftpUrl"]
	basePath := "/pubchem/Bioassay/Extras/"
	bioassayPath := "bioassays.tsv.gz"

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
		log.Printf("[PubChem Assay] [TEST MODE] Loading bioassays from %s (will stop after %d entries)", bioassayPath, testLimit)
	} else {
		log.Printf("[PubChem Assay] Loading bioassays from %s (20 MB compressed)", bioassayPath)
	}
	log.Printf("[PubChem Assay] Streaming entries directly to database")

	// Download and open file
	_, gz, _, _, localFile, _, err := getDataReaderNew("pubchem_assay", ftpServer, basePath, bioassayPath)
	if err != nil {
		log.Printf("[PubChem Assay] ERROR: Could not open bioassays.tsv.gz: %v", err)
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
	// 4MB buffer for better performance
	reader := bufio.NewReaderSize(gz, 4*1024*1024)

	lineCount := 0
	bioassayCount := 0
	headerSkipped := false

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			if err == io.EOF {
				break // End of file reached successfully
			}
			log.Printf("[PubChem Assay] CRITICAL ERROR reading stream: %v", err)
			return
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

		// Progress tracking every 10K records
		if lineCount%50000 == 0 {
			log.Printf("[PubChem Assay] Processed %dK bioassays", lineCount/1000)
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
				//log.Printf("[PubChem Assay] DEBUG: Adding uniprot xref: AID=%s -> UniProt=%s", aid, uniprotID)
				p.d.addXref(aid, fr, uniprotID, "uniprot", false)
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

	log.Printf("[PubChem Assay] Complete:")
	log.Printf("[PubChem Assay]   - Total bioassays processed: %d", bioassayCount)
}

func (p *pubchemAssay) check(e error, msg string) {
	if e != nil {
		log.Panicln(msg, e)
	}
}
