package update

import (
	"biobtree/pbuf"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type clinicalTrials struct {
	source             string
	d                  *DataUpdate
	dataPath           string
	medicalTermMappings *MedicalTermMappings
	loggedMappings     map[string]bool  // Track logged conditions to avoid duplicates
	loggedMisses       map[string]bool  // Track logged misses to avoid duplicates
	mu                 sync.Mutex       // Mutex for thread-safe map access
	testTrialIDs       map[string]bool  // Track trial IDs in test mode
}

// MedicalTermMappings, QualifiersToRemove, CancerQualifiers types
// are now defined in medical_mappings.go for shared use

func (ct *clinicalTrials) update() {
	defer ct.d.wg.Done()

	log.Println("Clinical Trials: Starting clinical trials data processing...")

	// Check if data files exist, run extraction if needed
	if !ct.ensureDataFilesExist() {
		panic("Clinical Trials: Failed to ensure data files exist. Check logs/clinical_trials_prepare.log for details.")
	}

	// Initialize tracking maps for unique logging
	ct.loggedMappings = make(map[string]bool)
	ct.loggedMisses = make(map[string]bool)

	// Load medical term mappings configuration
	ct.loadMedicalTermMappings()

	// Process clinical trials (raw format has no duplicates)
	totalTrials, err := ct.processTrials()
	if err != nil {
		panic(fmt.Sprintf("Error processing clinical trials: %v", err))
	}
	fmt.Printf("Completed processing clinical trials: %d trials\n", totalTrials)

	ct.d.progChan <- &progressInfo{dataset: ct.source, done: true}
}

// ensureDataFilesExist checks if the required clinical trials data files exist and are up-to-date.
// Uses smart update checking: only downloads new data if existing data is older than configured interval.
// Returns true if files exist and are up-to-date (or were successfully created), false on error.
func (ct *clinicalTrials) ensureDataFilesExist() bool {
	// Get rootDir for resolving relative paths
	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	// Check if trials.json exists
	trialsPath := filepath.Join(ct.dataPath, "trials.json")

	// Get script path from config (with default)
	scriptPath := config.Appconf["clinicalTrialsPrepareScript"]
	if scriptPath == "" {
		scriptPath = "src/scripts/clinical_trials/clinical_trials_prepare.py"
	}
	scriptPath = ct.resolvePath(scriptPath, rootDir)

	// If file exists, check if update is needed (age-based check)
	if fileExists(trialsPath) {
		log.Println("Clinical Trials: Data files exist, checking for updates...")

		// Run Python script with --check-update to see if data is too old
		if fileExists(scriptPath) {
			checkArgs := []string{scriptPath, "--output-dir", ct.dataPath, "--check-update"}
			checkCmd := exec.Command("python3", checkArgs...)

			if err := checkCmd.Run(); err != nil {
				// Exit code 10 means no update needed
				if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 10 {
					log.Println("Clinical Trials: Data is up-to-date (within age limit)")
					return true
				}
				// Exit code 0 or error means update is needed, continue to preparation
				log.Println("Clinical Trials: Data is outdated, will update")
			} else {
				// Exit code 0 means update is needed
				log.Println("Clinical Trials: Update needed, running preparation...")
			}
		} else {
			log.Println("Clinical Trials: Data files already exist, skipping extraction")
			return true
		}
	} else {
		// Files don't exist - need to run preparation script
		log.Println("Clinical Trials: Data files not found, running preparation script...")
	}

	// Check if script exists
	if !fileExists(scriptPath) {
		log.Printf("Clinical Trials: Preparation script not found at %s", scriptPath)
		return false
	}

	// Use dataPath directly - script outputs to this location
	// dataPath comes from source1.dataset.json "path" attribute
	outputDir := ct.dataPath

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("Clinical Trials: Failed to create output directory %s: %v", outputDir, err)
		return false
	}

	// Setup log file
	logsDir := ct.resolvePath("logs", rootDir)
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("Clinical Trials: Warning - could not create logs directory: %v", err)
	}
	logFile := filepath.Join(logsDir, "clinical_trials_prepare.log")

	// Build command arguments
	args := []string{scriptPath, "--output-dir", outputDir, "--log-file", logFile}

	// Add test mode flag if in test mode
	if config.IsTestMode() {
		args = append(args, "--test-mode")
	}

	log.Printf("Clinical Trials: Running preparation: python3 %s", strings.Join(args, " "))

	// Run the Python script
	cmd := exec.Command("python3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Printf("Clinical Trials: Preparation script failed: %v", err)
		return false
	}

	// Verify files were created
	if !fileExists(trialsPath) {
		log.Printf("Clinical Trials: Preparation completed but trials.json not found at %s", trialsPath)
		return false
	}

	log.Println("Clinical Trials: Preparation completed successfully")
	return true
}

// resolvePath resolves a path relative to rootDir if it's not absolute
func (ct *clinicalTrials) resolvePath(path, rootDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootDir, path)
}

// Load medical term mappings from JSON configuration file
// Uses the shared LoadMedicalTermMappings function from medical_mappings.go
func (ct *clinicalTrials) loadMedicalTermMappings() {
	ct.medicalTermMappings = LoadMedicalTermMappings()
}

// Map intervention name to ChEMBL molecules (multi-attempt with splitting)
func (ct *clinicalTrials) mapInterventionToChEMBL(nctID string, interventionName string, chemblDatasetID uint32, fr string) {
	if ct.d.lookupService == nil {
		return
	}


	// Track found ChEMBL IDs to prevent duplicates
	foundChEMBLs := make(map[string]bool)

	// ATTEMPT 1: Try full normalized name
	ct.lookupAndCollectChEMBL(interventionName, chemblDatasetID, foundChEMBLs)
	if len(foundChEMBLs) > 0 {
		ct.createChEMBLXrefs(nctID, fr, foundChEMBLs)
		return
	}

	// ATTEMPT 2: Try base name (remove chemical suffixes/numbers)
	baseName := removeChemicalSuffixes(interventionName)
	if baseName != interventionName {
		ct.lookupAndCollectChEMBL(baseName, chemblDatasetID, foundChEMBLs)
		if len(foundChEMBLs) > 0 {
			ct.createChEMBLXrefs(nctID, fr, foundChEMBLs)
			return
		}
	}

	// ATTEMPT 3: Try splitting into drug combinations
	components := splitDrugCombination(interventionName)
	if len(components) > 1 {
		for _, component := range components {
			component = strings.TrimSpace(component)
			if component == "" {
				continue
			}

			// Track before this component
			countBefore := len(foundChEMBLs)

			// Try full component name
			ct.lookupAndCollectChEMBL(component, chemblDatasetID, foundChEMBLs)

			// If this specific component found nothing, try base name
			if len(foundChEMBLs) == countBefore {
				baseComp := removeChemicalSuffixes(component)
				if baseComp != component && baseComp != "" {
					ct.lookupAndCollectChEMBL(baseComp, chemblDatasetID, foundChEMBLs)
				}
			}
		}
	}

	// Create all unique xrefs found
	if len(foundChEMBLs) > 0 {
		ct.createChEMBLXrefs(nctID, fr, foundChEMBLs)
	}
}

// Lookup name and collect ChEMBL IDs into the map
func (ct *clinicalTrials) lookupAndCollectChEMBL(name string, chemblDatasetID uint32, chemblIDs map[string]bool) {
	result, err := ct.d.lookup(name)
	if err != nil || result == nil || len(result.Results) == 0 {
		return
	}

	for _, xref := range result.Results {
		if xref.IsLink {
			// Text link - actual xrefs are in Entries
			for _, entry := range xref.Entries {
				if entry.Dataset == chemblDatasetID {
					chemblIDs[entry.Identifier] = true
				}
			}
		} else if xref.Dataset == chemblDatasetID {
			chemblIDs[xref.Identifier] = true
		}
	}
}

// Create ChEMBL cross-references
func (ct *clinicalTrials) createChEMBLXrefs(nctID string, fr string, chemblIDs map[string]bool) {
	for chemblID := range chemblIDs {
		ct.d.addXref(nctID, fr, chemblID, "chembl_molecule", false)
	}
}

// Map clinical trial condition to MONDO disease ontology
func (ct *clinicalTrials) mapConditionToMONDO(nctID string, condition string, mondoDatasetID uint32, fr string) {
	if ct.d.lookupService == nil {
		return
	}

	// Track found MONDO IDs to prevent duplicates
	foundMONDOs := make(map[string]bool)

	// ATTEMPT 1: Try exact condition name
	ct.lookupAndCollectMONDO(condition, mondoDatasetID, foundMONDOs)
	if len(foundMONDOs) > 0 {
		// ct.logMappingSuccess(condition, "1_EXACT", condition, len(foundMONDOs))
		ct.createMONDOXrefs(nctID, fr, foundMONDOs)
		return
	}

	// ATTEMPT 2: Try disease corrections (covid19 → COVID-19, hiv → HIV infection)
	if ct.medicalTermMappings != nil {
		for original, corrected := range ct.medicalTermMappings.DiseaseCorrections {
			if strings.EqualFold(condition, original) {
				ct.lookupAndCollectMONDO(corrected, mondoDatasetID, foundMONDOs)
				if len(foundMONDOs) > 0 {
					// ct.logMappingSuccess(condition, "2_CORRECTION", corrected, len(foundMONDOs))
					ct.createMONDOXrefs(nctID, fr, foundMONDOs)
					return
				}
			}
		}
	}

	// ATTEMPT 3: Try spelling variations (British/American, common typos)
	if ct.medicalTermMappings != nil {
		spellingVariant := ApplySpellingVariations(ct.medicalTermMappings, condition)
		if spellingVariant != condition {
			ct.lookupAndCollectMONDO(spellingVariant, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				// ct.logMappingSuccess(condition, "3_SPELLING", spellingVariant, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 3b: Try cancer abbreviations (NSCLC → non-small cell lung cancer)
	if ct.medicalTermMappings != nil {
		cancerAbbrevVariant := ApplyCancerAbbreviations(ct.medicalTermMappings, condition)
		if cancerAbbrevVariant != condition {
			ct.lookupAndCollectMONDO(cancerAbbrevVariant, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				// ct.logMappingSuccess(condition, "3b_CANCER_ABBREV", cancerAbbrevVariant, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 3c: Try removing cancer-specific qualifiers (stage, receptor, metastatic)
	// This is BEFORE general qualifiers to be more aggressive with cancer terms
	if ct.medicalTermMappings != nil {
		withoutCancerQualifiers := RemoveCancerQualifiers(ct.medicalTermMappings, condition)
		if withoutCancerQualifiers != condition {
			ct.lookupAndCollectMONDO(withoutCancerQualifiers, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				// ct.logMappingSuccess(condition, "3c_CANCER_QUALIFIERS", withoutCancerQualifiers, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 4: Remove parentheses and their contents
	// Example: "Heart Arrest (Cardiac)" → "Heart Arrest"
	simplifiedCondition := RemoveParentheses(condition)
	if simplifiedCondition != condition {
		ct.lookupAndCollectMONDO(simplifiedCondition, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			// ct.logMappingSuccess(condition, "4_NO_PARENS", simplifiedCondition, len(foundMONDOs))
			ct.createMONDOXrefs(nctID, fr, foundMONDOs)
			return
		}
	}

	// ATTEMPT 5: Try slash/or splitting (HIV/AIDS → try both)
	slashVariations := SplitSlashOr(condition)
	for _, variation := range slashVariations {
		ct.lookupAndCollectMONDO(variation, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			// ct.logMappingSuccess(condition, "5_SLASH_SPLIT", variation, len(foundMONDOs))
			ct.createMONDOXrefs(nctID, fr, foundMONDOs)
			return
		}
	}

	// ATTEMPT 6: Try specific medical term patterns (heart attack → myocardial infarction)
	if ct.medicalTermMappings != nil {
		variations := ApplySpecificPatterns(ct.medicalTermMappings, condition)
		for _, variation := range variations {
			ct.lookupAndCollectMONDO(variation, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				// ct.logMappingSuccess(condition, "6_SPECIFIC_PATTERN", variation, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 7: Remove medical qualifiers (Acute, Chronic, Mild, etc.)
	if ct.medicalTermMappings != nil {
		withoutQualifiers := RemoveQualifiers(ct.medicalTermMappings, condition)
		if withoutQualifiers != condition {
			ct.lookupAndCollectMONDO(withoutQualifiers, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				// ct.logMappingSuccess(condition, "7_NO_QUALIFIERS", withoutQualifiers, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 8: Try word order normalization (Amyloidosis Cardiac → Cardiac Amyloidosis)
	wordOrderVariation := TryWordOrderSwap(condition)
	if wordOrderVariation != condition {
		ct.lookupAndCollectMONDO(wordOrderVariation, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			// ct.logMappingSuccess(condition, "8_WORD_ORDER", wordOrderVariation, len(foundMONDOs))
			ct.createMONDOXrefs(nctID, fr, foundMONDOs)
			return
		}
	}

	// ATTEMPT 9: Try anatomical term variations (heart → cardiac, kidney → renal)
	if ct.medicalTermMappings != nil {
		anatomicalVariations := ApplyAnatomicalTerms(ct.medicalTermMappings, condition)
		for _, variation := range anatomicalVariations {
			ct.lookupAndCollectMONDO(variation, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				// ct.logMappingSuccess(condition, "9_ANATOMICAL", variation, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 10: Try singular/plural variations
	// "Seizures" → "Seizure", "Cardiovascular Diseases" → "Cardiovascular Disease"
	singularCondition := ToSingular(condition)
	if singularCondition != condition {
		ct.lookupAndCollectMONDO(singularCondition, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			// ct.logMappingSuccess(condition, "10_SINGULAR", singularCondition, len(foundMONDOs))
		}
	}

	// Create all unique xrefs found
	if len(foundMONDOs) > 0 {
		ct.createMONDOXrefs(nctID, fr, foundMONDOs)
	} else {
		// Log conditions that failed all mapping attempts (unique only)
		// ct.logMappingMiss(condition)
	}
}

// Lookup condition name and collect MONDO IDs into the map
func (ct *clinicalTrials) lookupAndCollectMONDO(condition string, mondoDatasetID uint32, mondoIDs map[string]bool) {
	result, err := ct.d.lookup(condition)
	if err != nil || result == nil || len(result.Results) == 0 {
		return
	}

	for _, xref := range result.Results {
		if xref.IsLink {
			// Text link - actual xrefs are in Entries
			for _, entry := range xref.Entries {
				if entry.Dataset == mondoDatasetID {
					mondoIDs[entry.Identifier] = true
				}
			}
		} else if xref.Dataset == mondoDatasetID {
			mondoIDs[xref.Identifier] = true
		}
	}
}

// Create MONDO cross-references
func (ct *clinicalTrials) createMONDOXrefs(nctID string, fr string, mondoIDs map[string]bool) {
	for mondoID := range mondoIDs {
		ct.d.addXref(nctID, fr, mondoID, "mondo", false)
	}
}

// Map condition to EFO disease ontology (parallel to MONDO)
// Uses the same multi-attempt mapping strategies
func (ct *clinicalTrials) mapConditionToEFO(nctID string, condition string, efoDatasetID uint32, fr string) {
	if ct.d.lookupService == nil {
		return
	}

	// Track found EFO IDs to prevent duplicates
	foundEFOs := make(map[string]bool)

	// ATTEMPT 1: Try exact condition name
	ct.lookupAndCollectEFO(condition, efoDatasetID, foundEFOs)
	if len(foundEFOs) > 0 {
		ct.createEFOXrefs(nctID, fr, foundEFOs)
		return
	}

	// ATTEMPT 2: Try disease corrections
	if ct.medicalTermMappings != nil {
		for original, corrected := range ct.medicalTermMappings.DiseaseCorrections {
			if strings.EqualFold(condition, original) {
				ct.lookupAndCollectEFO(corrected, efoDatasetID, foundEFOs)
				if len(foundEFOs) > 0 {
					ct.createEFOXrefs(nctID, fr, foundEFOs)
					return
				}
			}
		}
	}

	// ATTEMPT 3: Try spelling variations
	if ct.medicalTermMappings != nil {
		spellingVariant := ApplySpellingVariations(ct.medicalTermMappings, condition)
		if spellingVariant != condition {
			ct.lookupAndCollectEFO(spellingVariant, efoDatasetID, foundEFOs)
			if len(foundEFOs) > 0 {
				ct.createEFOXrefs(nctID, fr, foundEFOs)
				return
			}
		}
	}

	// ATTEMPT 4: Remove parentheses
	simplifiedCondition := RemoveParentheses(condition)
	if simplifiedCondition != condition {
		ct.lookupAndCollectEFO(simplifiedCondition, efoDatasetID, foundEFOs)
		if len(foundEFOs) > 0 {
			ct.createEFOXrefs(nctID, fr, foundEFOs)
			return
		}
	}

	// ATTEMPT 5: Try general qualifiers removal
	if ct.medicalTermMappings != nil {
		withoutQualifiers := RemoveQualifiers(ct.medicalTermMappings, condition)
		if withoutQualifiers != condition {
			ct.lookupAndCollectEFO(withoutQualifiers, efoDatasetID, foundEFOs)
			if len(foundEFOs) > 0 {
				ct.createEFOXrefs(nctID, fr, foundEFOs)
				return
			}
		}
	}
}

// Lookup condition and collect EFO IDs into the map
func (ct *clinicalTrials) lookupAndCollectEFO(condition string, efoDatasetID uint32, efoIDs map[string]bool) {
	result, err := ct.d.lookup(condition)
	if err != nil || result == nil || len(result.Results) == 0 {
		return
	}

	// Check if any result entries are EFO IDs
	for _, xref := range result.Results {
		if xref.Dataset == 0 {
			// Text link - actual xrefs are in Entries
			for _, entry := range xref.Entries {
				if entry.Dataset == efoDatasetID {
					efoIDs[entry.Identifier] = true
				}
			}
		} else if xref.Dataset == efoDatasetID {
			efoIDs[xref.Identifier] = true
		}
	}
}

// Create EFO cross-references
func (ct *clinicalTrials) createEFOXrefs(nctID string, fr string, efoIDs map[string]bool) {
	for efoID := range efoIDs {
		ct.d.addXref(nctID, fr, efoID, "efo", false)
	}
}

// Extract sponsor names from trial data
func (ct *clinicalTrials) extractSponsors(trialData map[string]interface{}) []string {
	var sponsors []string

	if trialData["sponsors"] == nil {
		return sponsors
	}

	sponsorList, ok := trialData["sponsors"].([]interface{})
	if !ok {
		return sponsors
	}

	for _, item := range sponsorList {
		sponsorMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract sponsor name
		name := getStringFromMap(sponsorMap, "name")
		if name != "" {
			sponsors = append(sponsors, name)
		}
	}

	return sponsors
}

// Normalize sponsor/company names for consistent cross-referencing
// Adapted from normalizeCompanyName in patents.go
func normalizeSponsorName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Convert to uppercase for consistent matching
	name = strings.ToUpper(name)

	// Remove country codes in parentheses: (US), (GB), (DE), etc.
	countryCodes := []string{
		" (US)", " (GB)", " (DE)", " (FR)", " (JP)", " (CN)",
		" (CH)", " (NL)", " (DK)", " (SE)", " (BM)", " (CA)",
		" (AU)", " (IT)", " (ES)", " (KR)", " (IN)", " (BE)",
	}
	for _, code := range countryCodes {
		name = strings.ReplaceAll(name, code, "")
	}

	// Remove legal suffixes
	suffixes := []string{
		" INC.", " INC", " INCORPORATED",
		" LTD.", " LTD", " LIMITED",
		" LLC", " LLC.",
		" GMBH", " GMBH.",
		" AG", " AG.",
		" SA", " SA.",
		" NV", " NV.",
		" CO.", " CO", " COMPANY",
		" CORPORATION", " CORP.", " CORP",
		" AND COMPANY",
		" PLC", " PLC.",
		" LP", " L.P.",
		" S.A.", " S.A",
		" S.R.L.", " SRL",
		" PTY", " PTY.",
		" BV", " B.V.",
	}

	for _, suffix := range suffixes {
		name = strings.TrimSuffix(name, suffix)
	}

	// Handle common abbreviation patterns
	name = strings.ReplaceAll(name, "E. I. ", "EI ")
	name = strings.ReplaceAll(name, "E.I. ", "EI ")

	// Clean up multiple spaces
	name = strings.Join(strings.Fields(name), " ")

	return strings.TrimSpace(name)
}

// Log mapping success (unique conditions only)
func (ct *clinicalTrials) logMappingSuccess(original string, attempt string, mapped string, count int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	key := fmt.Sprintf("%s|%s", original, attempt)
	if !ct.loggedMappings[key] {
		ct.loggedMappings[key] = true
		fmt.Printf("MONDO_MAP_SUCCESS: ATTEMPT=%s ORIGINAL='%s' MAPPED='%s' FOUND=%d\n", attempt, original, mapped, count)
	}
}

// Log mapping miss (unique conditions only)
func (ct *clinicalTrials) logMappingMiss(condition string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if !ct.loggedMisses[condition] {
		ct.loggedMisses[condition] = true
		fmt.Printf("MONDO_MAPPING_MISS: CONDITION='%s'\n", condition)
	}
}

func (ct *clinicalTrials) processTrials() (int, error) {
	// Read all JSON files from the directory
	files, err := ioutil.ReadDir(ct.dataPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read clinical trials directory: %w", err)
	}

	// Test mode setup
	testLimit := config.GetTestLimit(ct.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		ct.testTrialIDs = make(map[string]bool)
		idLogFile = openIDLogFile(config.TestRefDir, ct.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	totalTrials := 0
	fr := config.Dataconf[ct.source]["id"]

	// Get dataset IDs from config (not hardcoded)
	chemblDatasetID := config.DataconfIDStringToInt["chembl_molecule"]
	mondoDatasetID := config.DataconfIDStringToInt["mondo"]
	efoDatasetID := config.DataconfIDStringToInt["efo"]

	// Process each JSON file in the directory
	for _, fileInfo := range files {
		if fileInfo.IsDir() || !strings.HasSuffix(fileInfo.Name(), ".json") {
			continue
		}

		trialsFile := filepath.Join(ct.dataPath, fileInfo.Name())
		log.Printf("Clinical Trials: Processing %s (%.1f MB)...", fileInfo.Name(), float64(fileInfo.Size())/1024/1024)

		trialsProcessed, err := ct.processTrialsFile(trialsFile, fr, chemblDatasetID, mondoDatasetID, efoDatasetID, idLogFile)
		if err != nil {
			fmt.Printf("Warning: Error processing file %s: %v\n", trialsFile, err)
			continue
		}

		totalTrials += trialsProcessed
		log.Printf("Clinical Trials: Processed %d trials from %s", trialsProcessed, fileInfo.Name())

		// Test mode: Check if we've reached the limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, len(ct.testTrialIDs)) {
			break
		}
	}

	fmt.Printf("Completed processing all clinical trials files: %d total trials\n", totalTrials)
	return totalTrials, nil
}

func (ct *clinicalTrials) processTrialsFile(trialsFile string, fr string, chemblDatasetID uint32, mondoDatasetID uint32, efoDatasetID uint32, idLogFile *os.File) (int, error) {
	file, err := os.Open(trialsFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open trials file: %w", err)
	}
	defer file.Close()

	// Use json.Decoder for streaming
	decoder := json.NewDecoder(file)

	// Read first token - could be '[' (array) or '{' (wrapped object with "trials" key)
	token, err := decoder.Token()
	if err != nil {
		return 0, fmt.Errorf("failed to read first token: %w", err)
	}

	// Handle wrapped format: {"trials":[...]}
	if delim, ok := token.(json.Delim); ok && delim == '{' {
		// Read "trials" key
		keyToken, err := decoder.Token()
		if err != nil {
			return 0, fmt.Errorf("failed to read trials key: %w", err)
		}
		if key, ok := keyToken.(string); !ok || key != "trials" {
			return 0, fmt.Errorf("expected 'trials' key, got %v", keyToken)
		}
		// Read opening bracket of trials array
		if _, err := decoder.Token(); err != nil {
			return 0, fmt.Errorf("failed to read trials array start: %w", err)
		}
	} else if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return 0, fmt.Errorf("expected '[' or '{', got %v", token)
	}

	// Test mode setup
	testLimit := config.GetTestLimit(ct.source)

	uniqueTrials := 0
	var previous int64
	startTime := time.Now()

	// Iterate through array elements (no deduplication needed - raw format has unique NCT_IDs)
	for decoder.More() {
		// Read trial object
		var trialData map[string]interface{}
		if err := decoder.Decode(&trialData); err != nil {
			continue
		}

		// Extract NCT_ID
		nctID := getStringFromMap(trialData, "nct_id")
		if nctID == "" {
			continue
		}

		// Test mode: Track trial IDs and check limit
		if config.IsTestMode() {
			if _, exists := ct.testTrialIDs[nctID]; !exists {
				ct.testTrialIDs[nctID] = true
				if idLogFile != nil {
					logProcessedID(idLogFile, nctID)
				}
			}
			// Check if we've reached the limit
			if config.IsTestMode() && shouldStopProcessing(testLimit, len(ct.testTrialIDs)) {
				break
			}
		}

		uniqueTrials++

		elapsed := int64(time.Since(startTime).Seconds())
		if elapsed > previous+ct.d.progInterval {
			previous = elapsed
			ct.d.progChan <- &progressInfo{dataset: ct.source, currentKBPerSec: 0}
		}

		// Extract core trial metadata
		briefTitle := getStringFromMap(trialData, "brief_title")
		overallStatus := getStringFromMap(trialData, "overall_status")
		phase := getStringFromMap(trialData, "phase")
		studyType := getStringFromMap(trialData, "study_type")

		// Extract conditions (diseases/medical conditions)
		conditions := extractStringArrayFromMap(trialData, "conditions")

		// Store trial attributes
		attr := pbuf.ClinicalTrialAttr{
			BriefTitle:    briefTitle,
			OverallStatus: overallStatus,
			Phase:         phase,
			StudyType:     studyType,
			Conditions:    conditions,
		}

		b, _ := ffjson.Marshal(attr)
		ct.d.addProp3(nctID, fr, b)

		// Extract and process interventions
		interventions := ct.extractInterventionsFromMap(trialData)
		if len(interventions) > 0 {
			// Store interventions as part of attributes
			attr.Interventions = interventions
			b, _ = ffjson.Marshal(attr)
			ct.d.addProp3(nctID, fr, b)

			// Create cross-references for intervention names
			// This allows searching trials by drug name
			for _, interv := range interventions {
				if interv.Name != "" {
					// Normalize intervention name for searchability
					normalizedName := normalizeInterventionName(interv.Name)
					if normalizedName != "" {
						// Create text-based xref: intervention_name → NCT_ID
						ct.d.addXref(normalizedName, textLinkID, nctID, ct.source, true)

						// Map intervention to ChEMBL molecules if lookup DB available
						ct.mapInterventionToChEMBL(nctID, normalizedName, chemblDatasetID, fr)
					}
				}
			}
		}

		// Phase as searchable attribute
		if phase != "" && phase != "nan" {
			ct.d.addXref(phase, textLinkID, nctID, ct.source, true)
		}

		// Status as searchable attribute
		if overallStatus != "" {
			ct.d.addXref(overallStatus, textLinkID, nctID, ct.source, true)
		}

		// Study type as searchable attribute
		if studyType != "" {
			ct.d.addXref(studyType, textLinkID, nctID, ct.source, true)
		}

		// Map conditions to MONDO disease ontology
		for _, condition := range conditions {
			if condition != "" {
				// Create text search xref for condition
				ct.d.addXref(condition, textLinkID, nctID, ct.source, true)

				// Map condition to MONDO disease IDs if lookup DB available
				ct.mapConditionToMONDO(nctID, condition, mondoDatasetID, fr)
				// Also map to EFO (parallel disease ontology)
				ct.mapConditionToEFO(nctID, condition, efoDatasetID, fr)
			}
		}

		// Extract and link publications (PMIDs)
		// Forward: clinical_trials/forward/, Reverse: pubmed/from_clinical_trials/
		publications := ct.extractPublications(trialData)
		for _, pmid := range publications {
			if pmid != "" {
				// Create cross-reference: NCT_ID → PMID (reverse enables PMID >> clinical_trials queries)
				ct.d.addXref(nctID, fr, pmid, "pubmed", false)
			}
		}

		// Extract and process sponsors with normalization
		// Enables linking clinical trials to patents (same organizations)
		sponsors := ct.extractSponsors(trialData)
		for _, sponsor := range sponsors {
			if sponsor != "" {
				normalizedName := normalizeSponsorName(sponsor)
				if normalizedName != "" {
					// Add as text link for searchability (search "Pfizer" → find trials)
					ct.d.addXref(normalizedName, textLinkID, nctID, ct.source, true)
				}
			}
		}

		// TODO: Extract and store facilities (locations)
		// Consider adding to general biobtree roadmap for geographic search
		// facilities := ct.extractFacilities(trialData)
		// Store in attributes but don't create xrefs yet
	}

	// fmt.Printf("Processed %d clinical trials\n", uniqueTrials)

	return uniqueTrials, nil
}

func (ct *clinicalTrials) extractInterventionsFromMap(trialData map[string]interface{}) []*pbuf.Intervention {
	var interventions []*pbuf.Intervention

	// Check if interventions field exists
	if trialData["interventions"] == nil {
		return interventions
	}

	// Parse interventions array
	intervList, ok := trialData["interventions"].([]interface{})
	if !ok {
		return interventions
	}

	for _, item := range intervList {
		intervMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		intervention := &pbuf.Intervention{}

		if intervType, ok := intervMap["intervention_type"].(string); ok {
			intervention.Type = intervType
		}

		if name, ok := intervMap["name"].(string); ok {
			intervention.Name = name
		}

		if desc, ok := intervMap["description"].(string); ok {
			intervention.Description = desc
		}

		// Only add if we have at least a name
		if intervention.Name != "" {
			interventions = append(interventions, intervention)
		}
	}

	return interventions
}

// Helper function to extract string from map[string]interface{}
func getStringFromMap(m map[string]interface{}, key string) string {
	if m[key] == nil {
		return ""
	}

	switch v := m[key].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return ""
	}
}

// Helper function to extract string array from map[string]interface{}
func extractStringArrayFromMap(m map[string]interface{}, key string) []string {
	var result []string

	if m[key] == nil {
		return result
	}

	arr, ok := m[key].([]interface{})
	if !ok {
		return result
	}

	for _, item := range arr {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}

	return result
}

// Extract PMIDs from publications array
func (ct *clinicalTrials) extractPublications(trialData map[string]interface{}) []string {
	var pmids []string

	if trialData["publications"] == nil {
		return pmids
	}

	pubList, ok := trialData["publications"].([]interface{})
	if !ok {
		return pmids
	}

	for _, item := range pubList {
		pubMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract PMID
		pmid := getStringFromMap(pubMap, "pmid")
		if pmid != "" {
			pmids = append(pmids, pmid)
		}
	}

	return pmids
}

func normalizeInterventionName(name string) string {
	// Normalize intervention names for consistent searching
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Convert to lowercase for case-insensitive matching
	name = strings.ToLower(name)

	// Remove common phrases that aren't drug names
	name = strings.ReplaceAll(name, "receive ", "")
	name = strings.ReplaceAll(name, "treatment with ", "")
	name = strings.ReplaceAll(name, "administration of ", "")

	// Remove formulation details
	formulations := []string{
		"concentrated solution for injection",
		"for injection",
		"oral solution",
		"tablets",
		"capsules",
		"oral",
		"intravenous",
		"subcutaneous",
		"topical",
		"transdermal",
	}
	for _, form := range formulations {
		name = strings.ReplaceAll(name, form, "")
	}

	// Extract drug name from dosage patterns
	// "30 mg of CoQ10" → "coq10"
	if strings.Contains(name, " mg of ") {
		parts := strings.Split(name, " mg of ")
		if len(parts) > 1 {
			name = strings.TrimSpace(parts[1])
		}
	} else if strings.Contains(name, "mg ") {
		parts := strings.Split(name, "mg ")
		if len(parts) > 1 {
			name = strings.TrimSpace(parts[1])
		}
	}

	// Clean up whitespace
	name = strings.Join(strings.Fields(name), " ")
	name = strings.TrimSpace(name)

	// Skip if too short (likely noise)
	if len(name) < 3 {
		return ""
	}

	return name
}

// Remove chemical suffixes and numbers to get base drug name
func removeChemicalSuffixes(name string) string {
	name = strings.TrimSpace(name)

	// Remove common chemical suffixes with numbers
	// "medroxyprogesterone 17-acetate" → "medroxyprogesterone"
	// Split on spaces and take first significant word
	words := strings.Fields(name)
	if len(words) > 1 {
		// Check if subsequent words contain numbers or are common suffixes
		for i, word := range words {
			// If word contains number or is a common suffix, take everything before it
			if strings.ContainsAny(word, "0123456789") || isChemicalSuffix(word) {
				if i > 0 {
					return strings.Join(words[:i], " ")
				}
			}
		}
	}

	return name
}

// Check if word is a common chemical suffix
func isChemicalSuffix(word string) bool {
	suffixes := []string{
		"acetate", "sulfate", "chloride", "hydrochloride", "phosphate",
		"sodium", "potassium", "calcium", "magnesium",
		"hcl", "na", "k", "ca", "mg",
	}
	word = strings.ToLower(strings.Trim(word, ",-"))
	for _, suffix := range suffixes {
		if word == suffix {
			return true
		}
	}
	return false
}

// Split drug combination into individual components
func splitDrugCombination(name string) []string {
	// Try different separators
	separators := []string{"/", " and ", "+", ",", " & "}

	for _, sep := range separators {
		if strings.Contains(name, sep) {
			parts := strings.Split(name, sep)
			if len(parts) > 1 {
				// Clean each part
				var cleaned []string
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" && len(part) >= 3 {
						cleaned = append(cleaned, part)
					}
				}
				if len(cleaned) > 1 {
					return cleaned
				}
			}
		}
	}

	// Also try splitting on spaces if it looks like a combination
	// "Edaravone Dexborneol" → ["Edaravone", "Dexborneol"]
	words := strings.Fields(name)
	if len(words) == 2 {
		// Both words capitalized or both reasonable length = likely combination
		if len(words[0]) >= 4 && len(words[1]) >= 4 {
			// Check if both start with capital (after normalization they're lowercase)
			// Or if both are substantial words
			return words
		}
	}

	// No split found, return as single component
	return []string{name}
}
