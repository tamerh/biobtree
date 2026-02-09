package update

import (
	"biobtree/pbuf"
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// maxInhibitorKeyLen is the maximum length for inhibitor names in entry IDs
// LMDB has a 511 byte key limit; we reserve space for EC number and pipe
const maxInhibitorKeyLen = 400

// brendaInhibitor parses BRENDA inhibitor data (Ki, IC50, inhibition)
// Creates entries keyed by EC|inhibitor with detailed inhibition data
type brendaInhibitor struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (b *brendaInhibitor) check(err error, operation string) {
	checkWithContext(err, b.source, operation)
}

func (b *brendaInhibitor) update() {
	defer b.d.wg.Done()

	log.Println("BRENDA_INHIBITOR: Starting inhibitor data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(b.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, b.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("BRENDA_INHIBITOR: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Get path and source ID
	path := config.Dataconf[b.source]["path"]
	sourceID := config.Dataconf[b.source]["id"]
	brendaSourceID := config.Dataconf["brenda"]["id"]

	// Open JSON file
	filePath := filepath.FromSlash(path)
	file, err := os.Open(filePath)
	b.check(err, "opening BRENDA JSON file")
	defer file.Close()

	// Create buffered reader and decode entire JSON
	br := bufio.NewReaderSize(file, fileBufSize)

	var brendaData brendaFile
	decoder := json.NewDecoder(br)
	err = decoder.Decode(&brendaData)
	b.check(err, "decoding BRENDA JSON")

	log.Printf("BRENDA_INHIBITOR: Loaded release %s with %d EC entries",
		brendaData.Release, len(brendaData.Data))

	var savedEntries int
	var previous int64

	// Sort EC numbers for deterministic processing order
	ecNumbers := make([]string, 0, len(brendaData.Data))
	for ecID := range brendaData.Data {
		ecNumbers = append(ecNumbers, ecID)
	}
	sort.Strings(ecNumbers)

	// Process each EC entry
	for _, ecID := range ecNumbers {
		entry := brendaData.Data[ecID]

		// Skip "spontaneous" entry (not a real enzyme)
		if ecID == "spontaneous" {
			continue
		}

		// Progress tracking
		elapsed := int64(time.Since(b.d.start).Seconds())
		if elapsed > previous+b.d.progInterval {
			previous = elapsed
			if elapsed > 0 {
				b.d.progChan <- &progressInfo{dataset: b.source, currentKBPerSec: int64(savedEntries / int(elapsed))}
			}
		}

		// Build protein ID to organism map
		proteinToOrganism := make(map[string]string)
		for pid, protein := range entry.Protein {
			proteinToOrganism[pid] = protein.Organism
		}

		// Build reference ID to PubMed map
		refToPubmed := make(map[string]string)
		for rid, ref := range entry.Reference {
			if ref.PMID > 0 {
				refToPubmed[rid] = strconv.Itoa(ref.PMID)
			}
		}

		// Collect inhibitors
		inhibitorData := make(map[string]*inhibitorEntry)

		// Process Ki values
		for _, ki := range entry.KiValue {
			inhibitor, value := b.parseInhibitorValue(ki.Value)
			if inhibitor == "" || math.IsNaN(value) {
				continue
			}

			ie := b.getOrCreateInhibitor(inhibitorData, inhibitor)
			organism := b.getOrganism(ki.Proteins, proteinToOrganism)
			pubmed := b.getPubmed(ki.References, refToPubmed)

			ie.kiValues = append(ie.kiValues, &pbuf.InhibitionMeasurement{
				Value:      value,
				Unit:       "mM",
				Organism:   organism,
				Conditions: ki.Comment,
				PubmedId:   pubmed,
			})
		}

		// Process IC50 values
		for _, ic50 := range entry.IC50Value {
			inhibitor, value := b.parseInhibitorValue(ic50.Value)
			if inhibitor == "" || math.IsNaN(value) {
				continue
			}

			ie := b.getOrCreateInhibitor(inhibitorData, inhibitor)
			organism := b.getOrganism(ic50.Proteins, proteinToOrganism)
			pubmed := b.getPubmed(ic50.References, refToPubmed)

			ie.ic50Values = append(ie.ic50Values, &pbuf.InhibitionMeasurement{
				Value:      value,
				Unit:       "mM",
				Organism:   organism,
				Conditions: ic50.Comment,
				PubmedId:   pubmed,
			})
		}

		// Process inhibitor entries (qualitative data)
		for _, inh := range entry.Inhibitor {
			inhibitorName := strings.TrimSpace(inh.Value)
			if inhibitorName == "" || inhibitorName == "more" {
				continue
			}

			ie := b.getOrCreateInhibitor(inhibitorData, inhibitorName)
			organism := b.getOrganism(inh.Proteins, proteinToOrganism)
			pubmed := b.getPubmed(inh.References, refToPubmed)

			// Extract inhibition type from comment if available
			inhibitionType := b.extractInhibitionType(inh.Comment)

			ie.inhibitionData = append(ie.inhibitionData, &pbuf.InhibitionMeasurement{
				Organism:       organism,
				Conditions:     inh.Comment,
				PubmedId:       pubmed,
				InhibitionType: inhibitionType,
			})
		}

		// Create entries for each inhibitor
		for inhibitor, ie := range inhibitorData {
			// Skip if no data
			if len(ie.kiValues) == 0 && len(ie.ic50Values) == 0 && len(ie.inhibitionData) == 0 {
				continue
			}

			// Create entry ID: EC|inhibitor (truncate long names to avoid LMDB key limit)
			inhibitorKey := b.truncateInhibitorName(inhibitor)
			entryID := ecID + "|" + inhibitorKey

			// Calculate min/max values
			minKi, maxKi := b.getMinMax(ie.kiValues)
			minIC50, maxIC50 := b.getMinMax(ie.ic50Values)

			attr := &pbuf.BrendaInhibitorAttr{
				EcNumber:        ecID,
				Inhibitor:       inhibitor,
				KiValues:        ie.kiValues,
				Ic50Values:      ie.ic50Values,
				InhibitionData:  ie.inhibitionData,
				KiCount:         int32(len(ie.kiValues)),
				Ic50Count:       int32(len(ie.ic50Values)),
				InhibitionCount: int32(len(ie.inhibitionData)),
				MinKi:           minKi,
				MaxKi:           maxKi,
				MinIc50:         minIC50,
				MaxIc50:         maxIC50,
			}

			// Marshal and save
			attrBytes, err := ffjson.Marshal(attr)
			b.check(err, "marshaling BRENDA inhibitor attributes")

			b.d.addProp3(entryID, sourceID, attrBytes)

			// Text search indexing - inhibitor name
			b.d.addXref(inhibitor, textLinkID, entryID, b.source, true)

			// Cross-reference to main brenda entry
			b.d.addXref(entryID, sourceID, ecID, "brenda", false)
			b.d.addXref(ecID, brendaSourceID, entryID, b.source, false)

			// Test mode: log ID
			if idLogFile != nil {
				logProcessedID(idLogFile, entryID)
			}

			savedEntries++

			// Test mode: check limit
			if testLimit > 0 && savedEntries >= testLimit {
				log.Printf("BRENDA_INHIBITOR: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
				goto done
			}
		}
	}

done:
	log.Printf("BRENDA_INHIBITOR: Saved %d inhibitor entries", savedEntries)
	log.Printf("BRENDA_INHIBITOR: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&b.d.totalParsedEntry, uint64(savedEntries))

	// Signal completion
	b.d.progChan <- &progressInfo{dataset: b.source, done: true}
}

// inhibitorEntry holds data for a single inhibitor
type inhibitorEntry struct {
	kiValues       []*pbuf.InhibitionMeasurement
	ic50Values     []*pbuf.InhibitionMeasurement
	inhibitionData []*pbuf.InhibitionMeasurement
}

func (b *brendaInhibitor) getOrCreateInhibitor(m map[string]*inhibitorEntry, inhibitor string) *inhibitorEntry {
	// Normalize inhibitor name
	inhibitor = strings.TrimSpace(inhibitor)
	inhibitor = strings.ToLower(inhibitor)

	if ie, ok := m[inhibitor]; ok {
		return ie
	}
	ie := &inhibitorEntry{}
	m[inhibitor] = ie
	return ie
}

// parseInhibitorValue extracts numeric value and inhibitor name from BRENDA format
// e.g., "1.7 {ethanol}" -> inhibitor="ethanol", value=1.7
func (b *brendaInhibitor) parseInhibitorValue(raw string) (string, float64) {
	raw = strings.TrimSpace(raw)

	// Try to match "value {inhibitor}" format
	matches := kmValueRegex.FindStringSubmatch(raw)
	if len(matches) >= 3 {
		value, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return "", math.NaN()
		}
		return strings.TrimSpace(matches[2]), value
	}

	// If no inhibitor in braces, try to extract just the numeric value
	numMatches := numericRegex.FindStringSubmatch(raw)
	if len(numMatches) >= 2 {
		value, err := strconv.ParseFloat(numMatches[1], 64)
		if err != nil {
			return "", math.NaN()
		}
		// Try to extract inhibitor after the number
		rest := strings.TrimSpace(raw[len(numMatches[0]):])
		if rest != "" {
			return rest, value
		}
	}

	return "", math.NaN()
}

// getOrganism gets organism name from protein IDs
func (b *brendaInhibitor) getOrganism(proteinIDs []string, proteinToOrganism map[string]string) string {
	if len(proteinIDs) > 0 {
		if org, ok := proteinToOrganism[proteinIDs[0]]; ok {
			return org
		}
	}
	return ""
}

// getPubmed gets PubMed ID from reference IDs
func (b *brendaInhibitor) getPubmed(refIDs []string, refToPubmed map[string]string) string {
	if len(refIDs) > 0 {
		if pmid, ok := refToPubmed[refIDs[0]]; ok {
			return pmid
		}
	}
	return ""
}

// truncateInhibitorName truncates long inhibitor names to avoid LMDB key size limit
// If name exceeds maxInhibitorKeyLen, truncate and append MD5 hash for uniqueness
func (b *brendaInhibitor) truncateInhibitorName(name string) string {
	if len(name) <= maxInhibitorKeyLen {
		return name
	}

	// Create MD5 hash of full name for uniqueness
	hash := md5.Sum([]byte(name))
	hashStr := hex.EncodeToString(hash[:8]) // Use first 8 bytes (16 hex chars)

	// Truncate and append hash
	// Leave room for "..." and hash (16 chars + 3 for "...")
	truncLen := maxInhibitorKeyLen - 19
	return name[:truncLen] + "..." + hashStr
}

// extractInhibitionType tries to extract inhibition type from comment
func (b *brendaInhibitor) extractInhibitionType(comment string) string {
	comment = strings.ToLower(comment)

	// Check for common inhibition types
	inhibitionTypes := []struct {
		pattern string
		label   string
	}{
		{"competitive", "competitive"},
		{"non-competitive", "non-competitive"},
		{"noncompetitive", "non-competitive"},
		{"uncompetitive", "uncompetitive"},
		{"mixed", "mixed"},
		{"irreversible", "irreversible"},
		{"reversible", "reversible"},
		{"allosteric", "allosteric"},
		{"suicide", "suicide"},
		{"mechanism-based", "mechanism-based"},
		{"product inhibition", "product inhibition"},
		{"substrate inhibition", "substrate inhibition"},
	}

	for _, it := range inhibitionTypes {
		if strings.Contains(comment, it.pattern) {
			return it.label
		}
	}

	return ""
}

// getMinMax calculates min and max values from measurements
func (b *brendaInhibitor) getMinMax(measurements []*pbuf.InhibitionMeasurement) (float64, float64) {
	if len(measurements) == 0 {
		return 0, 0
	}

	minVal := math.MaxFloat64
	maxVal := -math.MaxFloat64

	for _, m := range measurements {
		if m.Value > 0 { // Only consider positive values
			if m.Value < minVal {
				minVal = m.Value
			}
			if m.Value > maxVal {
				maxVal = m.Value
			}
		}
	}

	if minVal == math.MaxFloat64 {
		return 0, 0
	}

	return minVal, maxVal
}
