package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// brendaKinetics parses BRENDA kinetic measurements (Km, kcat, Ki)
// Creates entries keyed by EC|substrate with detailed kinetic data
type brendaKinetics struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (b *brendaKinetics) check(err error, operation string) {
	checkWithContext(err, b.source, operation)
}

// Regex to extract numeric value and substrate from BRENDA format
// e.g., "0.05 {benzyl alcohol}" -> value=0.05, substrate="benzyl alcohol"
var kmValueRegex = regexp.MustCompile(`^([\d.]+(?:E[+-]?\d+)?)\s*\{([^}]+)\}`)
var numericRegex = regexp.MustCompile(`^([\d.]+(?:E[+-]?\d+)?)`)

func (b *brendaKinetics) update() {
	defer b.d.wg.Done()

	log.Println("BRENDA_KINETICS: Starting kinetic data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(b.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, b.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("BRENDA_KINETICS: [TEST MODE] Processing up to %d entries", testLimit)
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

	log.Printf("BRENDA_KINETICS: Loaded release %s with %d EC entries",
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

		// Collect kinetics by substrate
		substrateKinetics := make(map[string]*substrateData)

		// Process Km values
		for _, km := range entry.KmValue {
			substrate, value := b.parseKineticValue(km.Value)
			if substrate == "" || math.IsNaN(value) {
				continue
			}

			sd := b.getOrCreateSubstrate(substrateKinetics, substrate, "substrate")
			organism := b.getOrganism(km.Proteins, proteinToOrganism)
			pubmed := b.getPubmed(km.References, refToPubmed)

			sd.kmValues = append(sd.kmValues, &pbuf.KineticMeasurement{
				Value:      value,
				Unit:       "mM",
				Organism:   organism,
				Conditions: km.Comment,
				PubmedId:   pubmed,
			})
		}

		// Process kcat (turnover number) values
		for _, kcat := range entry.TurnoverNumber {
			substrate, value := b.parseKineticValue(kcat.Value)
			if substrate == "" || math.IsNaN(value) {
				continue
			}

			sd := b.getOrCreateSubstrate(substrateKinetics, substrate, "substrate")
			organism := b.getOrganism(kcat.Proteins, proteinToOrganism)
			pubmed := b.getPubmed(kcat.References, refToPubmed)

			sd.kcatValues = append(sd.kcatValues, &pbuf.KineticMeasurement{
				Value:      value,
				Unit:       "s⁻¹",
				Organism:   organism,
				Conditions: kcat.Comment,
				PubmedId:   pubmed,
			})
		}

		// Process inhibitor values (for Ki)
		for _, inh := range entry.Inhibitor {
			// Inhibitors may have Ki values in comments or separate fields
			// For now, create entries for inhibitors with their data
			inhibitorName := strings.TrimSpace(inh.Value)
			if inhibitorName == "" || inhibitorName == "more" {
				continue
			}

			sd := b.getOrCreateSubstrate(substrateKinetics, inhibitorName, "inhibitor")
			organism := b.getOrganism(inh.Proteins, proteinToOrganism)
			pubmed := b.getPubmed(inh.References, refToPubmed)

			// Try to extract Ki from comment
			kiValue := b.extractKiFromComment(inh.Comment)
			if !math.IsNaN(kiValue) {
				sd.kiValues = append(sd.kiValues, &pbuf.KineticMeasurement{
					Value:      kiValue,
					Unit:       "mM",
					Organism:   organism,
					Conditions: inh.Comment,
					PubmedId:   pubmed,
					Comment:    inhibitorName,
				})
			}
		}

		// Create entries for each substrate
		for substrate, sd := range substrateKinetics {
			// Skip if no kinetic data
			if len(sd.kmValues) == 0 && len(sd.kcatValues) == 0 && len(sd.kiValues) == 0 {
				continue
			}

			// Create entry ID: EC|substrate
			entryID := ecID + "|" + substrate

			// Calculate min/max values
			minKm, maxKm := b.getMinMax(sd.kmValues)
			minKcat, maxKcat := b.getMinMax(sd.kcatValues)

			attr := &pbuf.BrendaKineticsAttr{
				EcNumber:      ecID,
				Substrate:     substrate,
				SubstrateType: sd.substrateType,
				KmValues:      sd.kmValues,
				KcatValues:    sd.kcatValues,
				KiValues:      sd.kiValues,
				KmCount:       int32(len(sd.kmValues)),
				KcatCount:     int32(len(sd.kcatValues)),
				KiCount:       int32(len(sd.kiValues)),
				MinKm:         minKm,
				MaxKm:         maxKm,
				MinKcat:       minKcat,
				MaxKcat:       maxKcat,
			}

			// Marshal and save
			attrBytes, err := ffjson.Marshal(attr)
			b.check(err, "marshaling BRENDA kinetics attributes")

			b.d.addProp3(entryID, sourceID, attrBytes)

			// Text search indexing - substrate name
			b.d.addXref(substrate, textLinkID, entryID, b.source, true)

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
				log.Printf("BRENDA_KINETICS: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
				goto done
			}
		}
	}

done:
	log.Printf("BRENDA_KINETICS: Saved %d kinetic entries", savedEntries)
	log.Printf("BRENDA_KINETICS: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&b.d.totalParsedEntry, uint64(savedEntries))

	// Signal completion
	b.d.progChan <- &progressInfo{dataset: b.source, done: true}
}

// substrateData holds kinetics for a single substrate
type substrateData struct {
	substrateType string
	kmValues      []*pbuf.KineticMeasurement
	kcatValues    []*pbuf.KineticMeasurement
	kiValues      []*pbuf.KineticMeasurement
}

func (b *brendaKinetics) getOrCreateSubstrate(m map[string]*substrateData, substrate, substrateType string) *substrateData {
	// Normalize substrate name
	substrate = strings.TrimSpace(substrate)
	substrate = strings.ToLower(substrate)

	if sd, ok := m[substrate]; ok {
		return sd
	}
	sd := &substrateData{substrateType: substrateType}
	m[substrate] = sd
	return sd
}

// parseKineticValue extracts numeric value and substrate from BRENDA format
// e.g., "0.05 {benzyl alcohol}" -> substrate="benzyl alcohol", value=0.05
func (b *brendaKinetics) parseKineticValue(raw string) (string, float64) {
	raw = strings.TrimSpace(raw)

	// Try to match "value {substrate}" format
	matches := kmValueRegex.FindStringSubmatch(raw)
	if len(matches) >= 3 {
		value, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return "", math.NaN()
		}
		return strings.TrimSpace(matches[2]), value
	}

	// If no substrate in braces, try to extract just the numeric value
	// and use the whole string as context
	numMatches := numericRegex.FindStringSubmatch(raw)
	if len(numMatches) >= 2 {
		value, err := strconv.ParseFloat(numMatches[1], 64)
		if err != nil {
			return "", math.NaN()
		}
		// Try to extract substrate after the number
		rest := strings.TrimSpace(raw[len(numMatches[0]):])
		if rest != "" {
			return rest, value
		}
	}

	return "", math.NaN()
}

// getOrganism gets organism name from protein IDs
func (b *brendaKinetics) getOrganism(proteinIDs []string, proteinToOrganism map[string]string) string {
	if len(proteinIDs) > 0 {
		if org, ok := proteinToOrganism[proteinIDs[0]]; ok {
			return org
		}
	}
	return ""
}

// getPubmed gets PubMed ID from reference IDs
func (b *brendaKinetics) getPubmed(refIDs []string, refToPubmed map[string]string) string {
	if len(refIDs) > 0 {
		if pmid, ok := refToPubmed[refIDs[0]]; ok {
			return pmid
		}
	}
	return ""
}

// extractKiFromComment tries to extract Ki value from inhibitor comment
// e.g., "Ki: 0.1 mM" or "IC50: 5 uM"
func (b *brendaKinetics) extractKiFromComment(comment string) float64 {
	comment = strings.ToLower(comment)

	// Look for Ki or IC50 patterns
	patterns := []string{
		`ki[:\s]+(\d+\.?\d*)\s*(mm|um|nm)`,
		`ic50[:\s]+(\d+\.?\d*)\s*(mm|um|nm)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(comment)
		if len(matches) >= 3 {
			value, err := strconv.ParseFloat(matches[1], 64)
			if err != nil {
				continue
			}
			// Convert to mM
			unit := matches[2]
			switch unit {
			case "um":
				value = value / 1000
			case "nm":
				value = value / 1000000
			}
			return value
		}
	}

	return math.NaN()
}

// getMinMax calculates min and max values from measurements
func (b *brendaKinetics) getMinMax(measurements []*pbuf.KineticMeasurement) (float64, float64) {
	if len(measurements) == 0 {
		return 0, 0
	}

	minVal := math.MaxFloat64
	maxVal := -math.MaxFloat64

	for _, m := range measurements {
		if m.Value < minVal {
			minVal = m.Value
		}
		if m.Value > maxVal {
			maxVal = m.Value
		}
	}

	return minVal, maxVal
}
