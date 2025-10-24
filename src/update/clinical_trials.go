package update

import (
	"biobtree/db"
	"biobtree/pbuf"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pquerna/ffjson/ffjson"
)

type clinicalTrials struct {
	source      string
	d           *DataUpdate
	dataPath    string
	lookupEnv   db.Env
	lookupDbi   db.DBI
	hasLookupDB bool
}

func (ct *clinicalTrials) update() {
	defer ct.d.wg.Done()

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

func (ct *clinicalTrials) processTrials() (int, error) {
	trialsFile := filepath.Join(ct.dataPath, "trials.json")
	fmt.Printf("Opening clinical trials file: %s\n", trialsFile)
	file, err := os.Open(trialsFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open trials file: %w", err)
	}
	defer file.Close()

	fr := config.Dataconf[ct.source]["id"]
	chemblDatasetID := uint32(18) // chembl_molecule dataset ID

	// Use json.Decoder for streaming (handles array format)
	decoder := json.NewDecoder(file)

	// Read opening bracket [
	if _, err := decoder.Token(); err != nil {
		return 0, fmt.Errorf("failed to read opening bracket: %w", err)
	}

	uniqueTrials := 0
	var previous int64
	startTime := time.Now()

	// Iterate through array elements (no deduplication needed - raw format has unique NCT_IDs)
	for decoder.More() {
		uniqueTrials++

		elapsed := int64(time.Since(startTime).Seconds())
		if elapsed > previous+ct.d.progInterval {
			previous = elapsed
			ct.d.progChan <- &progressInfo{dataset: ct.source, currentKBPerSec: 0}
		}

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

		// TODO: Map conditions to disease ontology (DisGeNET or similar)
		// Once disease ontology is integrated into biobtree, create proper cross-references:
		// for _, condition := range conditions {
		//     if condition != "" {
		//         // Map condition name to disease ontology ID (e.g., UMLS, MESH, OMIM)
		//         // ct.d.addXref(diseaseID, linkID, nctID, ct.source, false)
		//     }
		// }

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
