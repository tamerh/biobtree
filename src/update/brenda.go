package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// brenda parses BRENDA (The Comprehensive Enzyme Information System)
// Source: https://www.brenda-enzymes.org/
// Format: JSON with EC numbers as keys in "data" object
type brenda struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (b *brenda) check(err error, operation string) {
	checkWithContext(err, b.source, operation)
}

// brendaFile represents the top-level BRENDA JSON structure
type brendaFile struct {
	Release string                    `json:"release"`
	Version string                    `json:"version"`
	Data    map[string]*brendaEC      `json:"data"`
}

// brendaEC represents an enzyme entry keyed by EC number
type brendaEC struct {
	ID              string                    `json:"id"`
	RecommendedName string                    `json:"recommended_name"`
	SystematicName  string                    `json:"systematic_name"`
	Synonyms        []brendaValue             `json:"synonyms"`
	Reaction        []brendaReaction          `json:"reaction"`
	ReactionType    []brendaValue             `json:"reaction_type"`
	Protein         map[string]*brendaProtein `json:"protein"`
	Reference       map[string]*brendaRef     `json:"reference"`
	Cofactor        []brendaCofactor          `json:"cofactor"`
	SubstratesProducts []brendaValue          `json:"substrates_products"`
	Inhibitor       []brendaValue             `json:"inhibitor"`
	KmValue         []brendaValue             `json:"km_value"`
	TurnoverNumber  []brendaValue             `json:"turnover_number"`
	Activator       []brendaValue             `json:"activating_compound"`
	KiValue         []brendaValue             `json:"ki_value"`
	IC50Value       []brendaValue             `json:"ic50_value"`
}

type brendaValue struct {
	Value      string   `json:"value"`
	Proteins   []string `json:"proteins"`
	References []string `json:"references"`
	Comment    string   `json:"comment"`
}

type brendaReaction struct {
	Value      string   `json:"value"`
	Proteins   []string `json:"proteins"`
	References []string `json:"references"`
	Comment    string   `json:"comment"`
}

type brendaCofactor struct {
	Value      string   `json:"value"`
	Proteins   []string `json:"proteins"`
	References []string `json:"references"`
	Comment    string   `json:"comment"`
}

type brendaProtein struct {
	ID         string   `json:"id"`
	Organism   string   `json:"organism"`
	References []string `json:"references"`
	Comment    string   `json:"comment"`
}

type brendaRef struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Authors []string `json:"authors"`
	Journal string   `json:"journal"`
	Year    int      `json:"year"`
	PMID    int      `json:"pmid"`
}

func (b *brenda) update() {
	defer b.d.wg.Done()

	log.Println("BRENDA: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(b.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, b.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("BRENDA: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Get path and source ID
	path := config.Dataconf[b.source]["path"]
	sourceID := config.Dataconf[b.source]["id"]

	// Open JSON file
	filePath := filepath.FromSlash(path)
	file, err := os.Open(filePath)
	b.check(err, "opening BRENDA JSON file")
	defer file.Close()

	// Create buffered reader and decode entire JSON
	// Note: BRENDA JSON structure requires loading "data" object to iterate keys
	br := bufio.NewReaderSize(file, fileBufSize)

	var brendaData brendaFile
	decoder := json.NewDecoder(br)
	err = decoder.Decode(&brendaData)
	b.check(err, "decoding BRENDA JSON")

	log.Printf("BRENDA: Loaded release %s, version %s with %d EC entries",
		brendaData.Release, brendaData.Version, len(brendaData.Data))

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

		// Test mode: log ID
		if idLogFile != nil {
			logProcessedID(idLogFile, ecID)
		}

		// Build attribute
		attr := b.buildAttr(ecID, entry)

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		b.check(err, "marshaling BRENDA attributes")

		b.d.addProp3(ecID, sourceID, attrBytes)

		// Text search indexing
		// EC number itself
		b.d.addXref(ecID, textLinkID, ecID, b.source, true)

		// Recommended name
		if entry.RecommendedName != "" {
			b.d.addXref(entry.RecommendedName, textLinkID, ecID, b.source, true)
		}

		// Systematic name
		if entry.SystematicName != "" {
			b.d.addXref(entry.SystematicName, textLinkID, ecID, b.source, true)
		}

		// Synonyms (limit to avoid explosion)
		synCount := 0
		for _, syn := range entry.Synonyms {
			if syn.Value != "" && synCount < 50 {
				b.d.addXref(syn.Value, textLinkID, ecID, b.source, true)
				synCount++
			}
		}

		// Cross-references
		// PubMed references
		for _, ref := range entry.Reference {
			if ref.PMID > 0 {
				pmidStr := strconv.Itoa(ref.PMID)
				b.d.addXref(ecID, sourceID, pmidStr, "pubmed", false)
			}
		}

		// Taxonomy via organism names (using lookup)
		// Note: This creates xrefs to organisms found in the protein entries
		organisms := b.getUniqueOrganisms(entry.Protein)
		for _, org := range organisms {
			// Try to find taxonomy ID via lookup if available
			// For now, we index organism names for text search
			if org != "" {
				b.d.addXref(org, textLinkID, ecID, b.source, true)
			}
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("BRENDA: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
			break
		}
	}

	log.Printf("BRENDA: Saved %d enzyme entries", savedEntries)
	log.Printf("BRENDA: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&b.d.totalParsedEntry, uint64(savedEntries))

	// Signal completion
	b.d.progChan <- &progressInfo{dataset: b.source, done: true}
}

// buildAttr constructs the BrendaAttr protobuf message from an EC entry
func (b *brenda) buildAttr(ecID string, entry *brendaEC) *pbuf.BrendaAttr {
	attr := &pbuf.BrendaAttr{
		EcNumber:        ecID,
		RecommendedName: entry.RecommendedName,
		SystematicName:  entry.SystematicName,
	}

	// Synonyms (limit to 100)
	for i, syn := range entry.Synonyms {
		if i >= 100 {
			break
		}
		if syn.Value != "" {
			attr.Synonyms = append(attr.Synonyms, syn.Value)
		}
	}

	// Reactions (limit to 50)
	for i, rxn := range entry.Reaction {
		if i >= 50 {
			break
		}
		if rxn.Value != "" {
			attr.Reactions = append(attr.Reactions, rxn.Value)
		}
	}

	// Reaction types
	for _, rt := range entry.ReactionType {
		if rt.Value != "" {
			attr.ReactionTypes = append(attr.ReactionTypes, rt.Value)
		}
	}

	// Cofactors
	cofactorSet := make(map[string]bool)
	for _, cof := range entry.Cofactor {
		if cof.Value != "" && !cofactorSet[cof.Value] {
			attr.Cofactors = append(attr.Cofactors, cof.Value)
			cofactorSet[cof.Value] = true
		}
	}

	// Organisms (unique, limit to 50)
	organisms := b.getUniqueOrganisms(entry.Protein)
	attr.OrganismCount = int32(len(organisms))
	if len(organisms) > 50 {
		organisms = organisms[:50]
	}
	attr.Organisms = organisms

	// Counts (detailed data available via brenda_kinetics and brenda_inhibitor subdatasets)
	attr.SubstrateCount = int32(len(entry.SubstratesProducts))
	attr.InhibitorCount = int32(len(entry.Inhibitor))

	// Kinetic data counts
	attr.KmCount = int32(len(entry.KmValue))
	attr.KcatCount = int32(len(entry.TurnoverNumber))

	// Reference count (PubMed IDs are stored as xrefs, not attributes)
	attr.ReferenceCount = int32(len(entry.Reference))

	return attr
}

// getUniqueOrganisms extracts unique organism names from protein entries
func (b *brenda) getUniqueOrganisms(proteins map[string]*brendaProtein) []string {
	orgSet := make(map[string]bool)
	for _, p := range proteins {
		if p.Organism != "" {
			orgSet[p.Organism] = true
		}
	}

	organisms := make([]string, 0, len(orgSet))
	for org := range orgSet {
		organisms = append(organisms, org)
	}
	sort.Strings(organisms)
	return organisms
}
