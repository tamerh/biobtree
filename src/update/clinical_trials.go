package update

import (
	"biobtree/db"
	"biobtree/pbuf"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pquerna/ffjson/ffjson"
)

type clinicalTrials struct {
	source             string
	d                  *DataUpdate
	dataPath           string
	lookupEnv          db.Env
	lookupDbi          db.DBI
	hasLookupDB        bool
	medicalTermMappings *MedicalTermMappings
	loggedMappings     map[string]bool  // Track logged conditions to avoid duplicates
	loggedMisses       map[string]bool  // Track logged misses to avoid duplicates
	mu                 sync.Mutex       // Mutex for thread-safe map access
	testTrialIDs       map[string]bool  // Track trial IDs in test mode
}

type MedicalTermMappings struct {
	SpecificPatterns    map[string]string   `json:"specific_patterns"`
	AnatomicalTerms     map[string]string   `json:"anatomical_terms"`
	QualifiersRemove    QualifiersToRemove  `json:"qualifiers_to_remove"`
	DiseaseCorrections  map[string]string   `json:"disease_corrections"`
	SpellingVariations  map[string]string   `json:"spelling_variations"`
	CancerQualifiers    CancerQualifiers    `json:"cancer_qualifiers"`
	CancerAbbreviations map[string]string   `json:"cancer_abbreviations"`
}

type QualifiersToRemove struct {
	Prefixes []string `json:"prefixes"`
	Suffixes []string `json:"suffixes"`
}

type CancerQualifiers struct {
	StageQualifiers     []string `json:"stage_qualifiers"`
	MetastasisQualifiers []string `json:"metastasis_qualifiers"`
	ReceptorPatterns    []string `json:"receptor_patterns"`
}

func (ct *clinicalTrials) update() {
	defer ct.d.wg.Done()

	// Initialize tracking maps for unique logging
	ct.loggedMappings = make(map[string]bool)
	ct.loggedMisses = make(map[string]bool)

	// Load medical term mappings configuration
	ct.loadMedicalTermMappings()

	// Initialize lookup database for ChEMBL mapping (if configured)
	ct.initLookupDB()
	defer ct.closeLookupDB()

	// Process clinical trials (raw format has no duplicates)
	totalTrials, err := ct.processTrials()
	if err != nil {
		panic(fmt.Sprintf("Error processing clinical trials: %v", err))
	}
	fmt.Printf("Completed processing clinical trials: %d trials\n", totalTrials)

	// Add entry statistics
	ct.d.addEntryStat(ct.source, uint64(totalTrials))

	ct.d.progChan <- &progressInfo{dataset: ct.source, done: true}
}

// Initialize read-only lookup database for ChEMBL mapping
func (ct *clinicalTrials) initLookupDB() {
	lookupDbDir, ok := config.Appconf["lookupDbDir"]
	if !ok {
		fmt.Println("Warning: lookupDbDir not configured, ChEMBL mapping disabled")
		ct.hasLookupDB = false
		return
	}

	// Check if meta file exists
	metaFile := filepath.FromSlash(lookupDbDir + "/db.meta.json")
	meta := make(map[string]interface{})
	f, err := ioutil.ReadFile(metaFile)
	if err != nil {
		fmt.Printf("Warning: Cannot read lookup database meta file: %v, ChEMBL mapping disabled\n", err)
		ct.hasLookupDB = false
		return
	}

	if err := json.Unmarshal(f, &meta); err != nil {
		fmt.Printf("Warning: Cannot parse lookup database meta: %v, ChEMBL mapping disabled\n", err)
		ct.hasLookupDB = false
		return
	}

	totalkvline := int64(meta["totalKVLine"].(float64))

	// Open lookup database (read-only)
	db1 := db.DB{}
	lookupConf := make(map[string]string)
	lookupConf["dbDir"] = lookupDbDir
	lookupConf["dbBackend"] = "lmdb"
	ct.lookupEnv, ct.lookupDbi = db1.OpenDBNew(false, totalkvline, lookupConf)
	ct.hasLookupDB = true
	fmt.Println("Lookup database initialized for ChEMBL mapping")
}

// Close lookup database
func (ct *clinicalTrials) closeLookupDB() {
	if ct.hasLookupDB {
		ct.lookupEnv.Close()
	}
}

// Load medical term mappings from JSON configuration file
func (ct *clinicalTrials) loadMedicalTermMappings() {
	configPath := filepath.FromSlash("conf/medical_term_mappings.json")

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Warning: Could not load medical term mappings (%v), using basic normalization only\n", err)
		ct.medicalTermMappings = &MedicalTermMappings{
			SpecificPatterns:    make(map[string]string),
			AnatomicalTerms:     make(map[string]string),
			DiseaseCorrections:  make(map[string]string),
			SpellingVariations:  make(map[string]string),
			CancerAbbreviations: make(map[string]string),
			QualifiersRemove:    QualifiersToRemove{Prefixes: []string{}, Suffixes: []string{}},
			CancerQualifiers:    CancerQualifiers{StageQualifiers: []string{}, MetastasisQualifiers: []string{}, ReceptorPatterns: []string{}},
		}
		return
	}

	// Parse the JSON structure
	var rawConfig struct {
		SpecificPatterns struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"specific_patterns"`
		AnatomicalTerms struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"anatomical_terms"`
		QualifiersRemove  QualifiersToRemove `json:"qualifiers_to_remove"`
		DiseaseCorrections struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"disease_corrections"`
		SpellingVariations struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"spelling_variations"`
		CancerQualifiers    CancerQualifiers `json:"cancer_qualifiers"`
		CancerAbbreviations struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"cancer_abbreviations"`
	}

	if err := json.Unmarshal(data, &rawConfig); err != nil {
		fmt.Printf("Warning: Could not parse medical term mappings (%v), using basic normalization only\n", err)
		return
	}

	ct.medicalTermMappings = &MedicalTermMappings{
		SpecificPatterns:    rawConfig.SpecificPatterns.Mappings,
		AnatomicalTerms:     rawConfig.AnatomicalTerms.Mappings,
		QualifiersRemove:    rawConfig.QualifiersRemove,
		DiseaseCorrections:  rawConfig.DiseaseCorrections.Mappings,
		SpellingVariations:  rawConfig.SpellingVariations.Mappings,
		CancerQualifiers:    rawConfig.CancerQualifiers,
		CancerAbbreviations: rawConfig.CancerAbbreviations.Mappings,
	}

	fmt.Printf("Loaded medical term mappings: %d specific, %d anatomical, %d qualifiers, %d cancer qualifiers\n",
		len(ct.medicalTermMappings.SpecificPatterns),
		len(ct.medicalTermMappings.AnatomicalTerms),
		len(ct.medicalTermMappings.QualifiersRemove.Prefixes)+len(ct.medicalTermMappings.QualifiersRemove.Suffixes),
		len(ct.medicalTermMappings.CancerQualifiers.StageQualifiers)+
		len(ct.medicalTermMappings.CancerQualifiers.MetastasisQualifiers)+
		len(ct.medicalTermMappings.CancerQualifiers.ReceptorPatterns))
}

// Lookup identifier in biobtree database and return results
func (ct *clinicalTrials) lookup(identifier string) (*pbuf.Result, error) {
	if !ct.hasLookupDB {
		return nil, fmt.Errorf("lookup database not available")
	}

	// Lookup is case-insensitive (convert to uppercase like service does)
	identifier = strings.ToUpper(identifier)

	var v []byte
	err := ct.lookupEnv.View(func(txn db.Txn) (err error) {
		v, err = txn.Get(ct.lookupDbi, []byte(identifier))
		if db.IsNotFound(err) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	if len(v) == 0 {
		return nil, nil
	}

	r := pbuf.Result{}
	err = proto.Unmarshal(v, &r)
	return &r, err
}

// Map intervention name to ChEMBL molecules (multi-attempt with splitting)
func (ct *clinicalTrials) mapInterventionToChEMBL(nctID string, interventionName string, chemblDatasetID uint32, fr string) {
	if !ct.hasLookupDB {
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
	result, err := ct.lookup(name)
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
	if !ct.hasLookupDB {
		return
	}

	// Track found MONDO IDs to prevent duplicates
	foundMONDOs := make(map[string]bool)

	// ATTEMPT 1: Try exact condition name
	ct.lookupAndCollectMONDO(condition, mondoDatasetID, foundMONDOs)
	if len(foundMONDOs) > 0 {
		ct.logMappingSuccess(condition, "1_EXACT", condition, len(foundMONDOs))
		ct.createMONDOXrefs(nctID, fr, foundMONDOs)
		return
	}

	// ATTEMPT 2: Try disease corrections (covid19 → COVID-19, hiv → HIV infection)
	if ct.medicalTermMappings != nil {
		for original, corrected := range ct.medicalTermMappings.DiseaseCorrections {
			if strings.EqualFold(condition, original) {
				ct.lookupAndCollectMONDO(corrected, mondoDatasetID, foundMONDOs)
				if len(foundMONDOs) > 0 {
					ct.logMappingSuccess(condition, "2_CORRECTION", corrected, len(foundMONDOs))
					ct.createMONDOXrefs(nctID, fr, foundMONDOs)
					return
				}
			}
		}
	}

	// ATTEMPT 3: Try spelling variations (British/American, common typos)
	if ct.medicalTermMappings != nil {
		spellingVariant := ct.applySpellingVariations(condition)
		if spellingVariant != condition {
			ct.lookupAndCollectMONDO(spellingVariant, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				ct.logMappingSuccess(condition, "3_SPELLING", spellingVariant, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 3b: Try cancer abbreviations (NSCLC → non-small cell lung cancer)
	if ct.medicalTermMappings != nil {
		cancerAbbrevVariant := ct.applyCancerAbbreviations(condition)
		if cancerAbbrevVariant != condition {
			ct.lookupAndCollectMONDO(cancerAbbrevVariant, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				ct.logMappingSuccess(condition, "3b_CANCER_ABBREV", cancerAbbrevVariant, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 3c: Try removing cancer-specific qualifiers (stage, receptor, metastatic)
	// This is BEFORE general qualifiers to be more aggressive with cancer terms
	if ct.medicalTermMappings != nil {
		withoutCancerQualifiers := ct.removeCancerQualifiers(condition)
		if withoutCancerQualifiers != condition {
			ct.lookupAndCollectMONDO(withoutCancerQualifiers, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				ct.logMappingSuccess(condition, "3c_CANCER_QUALIFIERS", withoutCancerQualifiers, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 4: Remove parentheses and their contents
	// Example: "Heart Arrest (Cardiac)" → "Heart Arrest"
	simplifiedCondition := removeParentheses(condition)
	if simplifiedCondition != condition {
		ct.lookupAndCollectMONDO(simplifiedCondition, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			ct.logMappingSuccess(condition, "4_NO_PARENS", simplifiedCondition, len(foundMONDOs))
			ct.createMONDOXrefs(nctID, fr, foundMONDOs)
			return
		}
	}

	// ATTEMPT 5: Try slash/or splitting (HIV/AIDS → try both)
	slashVariations := ct.splitSlashOr(condition)
	for _, variation := range slashVariations {
		ct.lookupAndCollectMONDO(variation, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			ct.logMappingSuccess(condition, "5_SLASH_SPLIT", variation, len(foundMONDOs))
			ct.createMONDOXrefs(nctID, fr, foundMONDOs)
			return
		}
	}

	// ATTEMPT 6: Try specific medical term patterns (heart attack → myocardial infarction)
	if ct.medicalTermMappings != nil {
		variations := ct.applySpecificPatterns(condition)
		for _, variation := range variations {
			ct.lookupAndCollectMONDO(variation, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				ct.logMappingSuccess(condition, "6_SPECIFIC_PATTERN", variation, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 7: Remove medical qualifiers (Acute, Chronic, Mild, etc.)
	if ct.medicalTermMappings != nil {
		withoutQualifiers := ct.removeQualifiers(condition)
		if withoutQualifiers != condition {
			ct.lookupAndCollectMONDO(withoutQualifiers, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				ct.logMappingSuccess(condition, "7_NO_QUALIFIERS", withoutQualifiers, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 8: Try word order normalization (Amyloidosis Cardiac → Cardiac Amyloidosis)
	wordOrderVariation := ct.tryWordOrderSwap(condition)
	if wordOrderVariation != condition {
		ct.lookupAndCollectMONDO(wordOrderVariation, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			ct.logMappingSuccess(condition, "8_WORD_ORDER", wordOrderVariation, len(foundMONDOs))
			ct.createMONDOXrefs(nctID, fr, foundMONDOs)
			return
		}
	}

	// ATTEMPT 9: Try anatomical term variations (heart → cardiac, kidney → renal)
	if ct.medicalTermMappings != nil {
		anatomicalVariations := ct.applyAnatomicalTerms(condition)
		for _, variation := range anatomicalVariations {
			ct.lookupAndCollectMONDO(variation, mondoDatasetID, foundMONDOs)
			if len(foundMONDOs) > 0 {
				ct.logMappingSuccess(condition, "9_ANATOMICAL", variation, len(foundMONDOs))
				ct.createMONDOXrefs(nctID, fr, foundMONDOs)
				return
			}
		}
	}

	// ATTEMPT 10: Try singular/plural variations
	// "Seizures" → "Seizure", "Cardiovascular Diseases" → "Cardiovascular Disease"
	singularCondition := toSingular(condition)
	if singularCondition != condition {
		ct.lookupAndCollectMONDO(singularCondition, mondoDatasetID, foundMONDOs)
		if len(foundMONDOs) > 0 {
			ct.logMappingSuccess(condition, "10_SINGULAR", singularCondition, len(foundMONDOs))
		}
	}

	// Create all unique xrefs found
	if len(foundMONDOs) > 0 {
		ct.createMONDOXrefs(nctID, fr, foundMONDOs)
	} else {
		// Log conditions that failed all mapping attempts (unique only)
		ct.logMappingMiss(condition)
	}
}

// Lookup condition name and collect MONDO IDs into the map
func (ct *clinicalTrials) lookupAndCollectMONDO(condition string, mondoDatasetID uint32, mondoIDs map[string]bool) {
	result, err := ct.lookup(condition)
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

// removeParentheses removes text in parentheses
// Example: "Heart Arrest (Cardiac)" → "Heart Arrest"
func removeParentheses(s string) string {
	reParens := regexp.MustCompile(`\s*\([^)]*\)`)
	return strings.TrimSpace(reParens.ReplaceAllString(s, ""))
}

// toSingular attempts simple plural → singular conversion
// Example: "Seizures" → "Seizure", "Diseases" → "Disease"
func toSingular(s string) string {
	// Handle "Diseases" → "Disease"
	if strings.HasSuffix(s, "eases") {
		return s[:len(s)-1] // Remove 's'
	}
	// Handle "Injuries" → "Injury"
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	// Handle "Tumors" → "Tumor", but keep "-sis" (Sepsis, Thrombosis)
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "sis") && !strings.HasSuffix(s, "us") {
		return s[:len(s)-1]
	}
	return s
}

// applySpecificPatterns tries high-priority exact phrase replacements
func (ct *clinicalTrials) applySpecificPatterns(condition string) []string {
	var variations []string
	lower := strings.ToLower(condition)

	for original, synonym := range ct.medicalTermMappings.SpecificPatterns {
		if strings.Contains(lower, original) {
			variation := strings.ReplaceAll(lower, original, synonym)
			if variation != lower {
				variations = append(variations, variation)
			}
		}
		// Also try reverse mapping
		if strings.Contains(lower, synonym) {
			variation := strings.ReplaceAll(lower, synonym, original)
			if variation != lower {
				variations = append(variations, variation)
			}
		}
	}

	return variations
}

// applyAnatomicalTerms tries general anatomical term replacements
func (ct *clinicalTrials) applyAnatomicalTerms(condition string) []string {
	var variations []string
	lower := strings.ToLower(condition)

	for original, synonym := range ct.medicalTermMappings.AnatomicalTerms {
		// Use word boundaries to avoid partial replacements
		// "heart disease" → "cardiac disease", but not "sheart" → "scardiac"
		if strings.Contains(lower, " "+original+" ") ||
		   strings.HasPrefix(lower, original+" ") ||
		   strings.HasSuffix(lower, " "+original) ||
		   lower == original {
			variation := strings.ReplaceAll(lower, original, synonym)
			if variation != lower {
				variations = append(variations, variation)
			}
		}
	}

	return variations
}

// removeQualifiers strips temporal/severity modifiers from condition names
func (ct *clinicalTrials) removeQualifiers(condition string) string {
	result := condition
	lower := strings.ToLower(condition)

	// Remove prefixes
	for _, prefix := range ct.medicalTermMappings.QualifiersRemove.Prefixes {
		prefixPattern := prefix + " "
		if strings.HasPrefix(lower, prefixPattern) {
			// Preserve original case for the rest of the string
			result = condition[len(prefixPattern):]
			lower = strings.ToLower(result)
		}
	}

	// Remove suffixes
	for _, suffix := range ct.medicalTermMappings.QualifiersRemove.Suffixes {
		if strings.Contains(lower, " "+suffix) {
			idx := strings.Index(lower, " "+suffix)
			if idx > 0 {
				result = condition[:idx]
				lower = strings.ToLower(result)
			}
		}
	}

	return strings.TrimSpace(result)
}

// applySpellingVariations handles British/American spelling and common typos
func (ct *clinicalTrials) applySpellingVariations(condition string) string {
	lower := strings.ToLower(condition)

	for british, american := range ct.medicalTermMappings.SpellingVariations {
		if strings.Contains(lower, british) {
			return strings.ReplaceAll(lower, british, american)
		}
	}

	return condition
}

// splitSlashOr splits conditions like "HIV/AIDS" or "Recurrent/Advanced Cancer"
func (ct *clinicalTrials) splitSlashOr(condition string) []string {
	var variations []string

	// Split on slash
	if strings.Contains(condition, "/") {
		parts := strings.Split(condition, "/")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" && trimmed != condition {
				variations = append(variations, trimmed)
			}
		}
	}

	// Split on " or "
	if strings.Contains(strings.ToLower(condition), " or ") {
		parts := strings.Split(condition, " or ")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" && trimmed != condition {
				variations = append(variations, trimmed)
			}
		}
	}

	return variations
}

// tryWordOrderSwap handles reversed word order like "Amyloidosis Cardiac" → "Cardiac Amyloidosis"
func (ct *clinicalTrials) tryWordOrderSwap(condition string) string {
	words := strings.Fields(condition)

	// Only swap if exactly 2 words
	if len(words) == 2 {
		// Swap if second word looks like an adjective (ends in -ic, -al, -ous, etc.)
		secondLower := strings.ToLower(words[1])
		if strings.HasSuffix(secondLower, "ic") ||
		   strings.HasSuffix(secondLower, "al") ||
		   strings.HasSuffix(secondLower, "ous") ||
		   strings.HasSuffix(secondLower, "ar") {
			return words[1] + " " + words[0]
		}
	}

	return condition
}

// applyCancerAbbreviations expands cancer abbreviations and normalizes hyphenation
// Examples: "NSCLC" → "non-small cell lung cancer", "head-and-neck" → "head and neck"
func (ct *clinicalTrials) applyCancerAbbreviations(condition string) string {
	lower := strings.ToLower(condition)

	for abbrev, expanded := range ct.medicalTermMappings.CancerAbbreviations {
		if strings.Contains(lower, abbrev) {
			return strings.ReplaceAll(lower, abbrev, expanded)
		}
	}

	return condition
}

// removeCancerQualifiers removes cancer-specific qualifiers (stage, receptor markers, metastatic)
// This is more aggressive than general qualifier removal and runs BEFORE it
// Examples:
//   "Stage III Colorectal Cancer" → "Colorectal Cancer"
//   "HER2 Positive Metastatic Breast Cancer" → "Breast Cancer"
//   "Early-stage Non-small Cell Lung Cancer" → "Non-small Cell Lung Cancer"
func (ct *clinicalTrials) removeCancerQualifiers(condition string) string {
	result := strings.TrimSpace(condition)
	lower := strings.ToLower(result)

	// Remove stage qualifiers
	for _, stageQual := range ct.medicalTermMappings.CancerQualifiers.StageQualifiers {
		stageQualLower := strings.ToLower(stageQual)
		// Try as prefix
		if strings.HasPrefix(lower, stageQualLower+" ") {
			result = strings.TrimSpace(result[len(stageQual)+1:])
			lower = strings.ToLower(result)
		}
		// Try as suffix
		if strings.HasSuffix(lower, " "+stageQualLower) {
			result = strings.TrimSpace(result[:len(result)-len(stageQual)-1])
			lower = strings.ToLower(result)
		}
		// Try in middle (with spaces)
		if strings.Contains(lower, " "+stageQualLower+" ") {
			result = strings.ReplaceAll(result, " "+stageQual+" ", " ")
			result = strings.TrimSpace(result)
			lower = strings.ToLower(result)
		}
	}

	// Remove metastasis qualifiers
	for _, metaQual := range ct.medicalTermMappings.CancerQualifiers.MetastasisQualifiers {
		metaQualLower := strings.ToLower(metaQual)
		// Try as prefix
		if strings.HasPrefix(lower, metaQualLower+" ") {
			result = strings.TrimSpace(result[len(metaQual)+1:])
			lower = strings.ToLower(result)
		}
		// Try as suffix
		if strings.HasSuffix(lower, " "+metaQualLower) {
			result = strings.TrimSpace(result[:len(result)-len(metaQual)-1])
			lower = strings.ToLower(result)
		}
		// Try in middle (with spaces)
		if strings.Contains(lower, " "+metaQualLower+" ") {
			result = strings.ReplaceAll(result, " "+metaQual+" ", " ")
			result = strings.TrimSpace(result)
			lower = strings.ToLower(result)
		}
	}

	// Remove receptor patterns (more complex as they can be anywhere)
	for _, receptorPattern := range ct.medicalTermMappings.CancerQualifiers.ReceptorPatterns {
		receptorLower := strings.ToLower(receptorPattern)
		// Try as prefix
		if strings.HasPrefix(lower, receptorLower+" ") {
			result = strings.TrimSpace(result[len(receptorPattern)+1:])
			lower = strings.ToLower(result)
		}
		// Try as suffix
		if strings.HasSuffix(lower, " "+receptorLower) {
			result = strings.TrimSpace(result[:len(result)-len(receptorPattern)-1])
			lower = strings.ToLower(result)
		}
		// Try in middle (with spaces)
		if strings.Contains(lower, " "+receptorLower+" ") {
			result = strings.ReplaceAll(result, " "+receptorPattern+" ", " ")
			result = strings.TrimSpace(result)
			lower = strings.ToLower(result)
		}
	}

	return result
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
	chemblDatasetID := uint32(18)   // chembl_molecule dataset ID
	mondoDatasetID := uint32(26)    // mondo dataset ID

	// Process each JSON file in the directory
	for _, fileInfo := range files {
		if fileInfo.IsDir() || !strings.HasSuffix(fileInfo.Name(), ".json") {
			continue
		}

		trialsFile := filepath.Join(ct.dataPath, fileInfo.Name())
		fmt.Printf("Processing clinical trials file: %s\n", trialsFile)

		trialsProcessed, err := ct.processTrialsFile(trialsFile, fr, chemblDatasetID, mondoDatasetID, idLogFile)
		if err != nil {
			fmt.Printf("Warning: Error processing file %s: %v\n", trialsFile, err)
			continue
		}

		totalTrials += trialsProcessed
		fmt.Printf("Processed %d trials from %s (total so far: %d)\n", trialsProcessed, fileInfo.Name(), totalTrials)

		// Test mode: Check if we've reached the limit
		if shouldStopProcessing(testLimit, len(ct.testTrialIDs)) {
			break
		}
	}

	fmt.Printf("Completed processing all clinical trials files: %d total trials\n", totalTrials)
	return totalTrials, nil
}

func (ct *clinicalTrials) processTrialsFile(trialsFile string, fr string, chemblDatasetID uint32, mondoDatasetID uint32, idLogFile *os.File) (int, error) {
	file, err := os.Open(trialsFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open trials file: %w", err)
	}
	defer file.Close()

	// Use json.Decoder for streaming (handles array format)
	decoder := json.NewDecoder(file)

	// Read opening bracket [
	if _, err := decoder.Token(); err != nil {
		return 0, fmt.Errorf("failed to read opening bracket: %w", err)
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
			if shouldStopProcessing(testLimit, len(ct.testTrialIDs)) {
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
			}
		}

		// Extract and link publications (PMIDs)
		publications := ct.extractPublications(trialData)
		for _, pmid := range publications {
			if pmid != "" {
				// Create cross-reference: NCT_ID ↔ PMID
				ct.d.addXref(pmid, config.Dataconf["pubmed"]["id"], nctID, ct.source, false)
			}
		}

		// TODO: Extract and process sponsors
		// Sponsors need normalization strategy (e.g., "Pfizer Inc" vs "Pfizer")
		// This will enable linking clinical trials to patents (same organizations)
		// Once we have organization normalization:
		// sponsors := ct.extractSponsors(trialData)
		// for _, sponsor := range sponsors {
		//     if sponsor != "" {
		//         normalizedName := normalizeSponsorName(sponsor)
		//         ct.d.addXref(normalizedName, textLinkID, nctID, ct.source, true)
		//     }
		// }

		// TODO: Extract and store facilities (locations)
		// Consider adding to general biobtree roadmap for geographic search
		// facilities := ct.extractFacilities(trialData)
		// Store in attributes but don't create xrefs yet
	}

	fmt.Printf("Processed %d clinical trials\n", uniqueTrials)

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
