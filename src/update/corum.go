package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// corumOrgToTaxID maps CORUM organism names to NCBI Taxonomy IDs
var corumOrgToTaxID = map[string]string{
	"Human":    "9606",
	"Mouse":    "10090",
	"Rat":      "10116",
	"Bovine":   "9913",
	"Pig":      "9823",
	"Dog":      "9615",
	"Rabbit":   "9986",
	"Hamster":  "10029",
	"MINK":     "452646",
	"Mammalia": "40674",
}

// corum parses CORUM (Comprehensive Resource of Mammalian Protein Complexes)
// Source: https://mips.helmholtz-muenchen.de/corum/
// Format: JSON array of complex objects
type corum struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (c *corum) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// corumComplex represents a single protein complex from CORUM JSON
type corumComplex struct {
	ComplexID            int                    `json:"complex_id"`
	ComplexName          string                 `json:"complex_name"`
	Synonyms             interface{}            `json:"synonyms"` // Can be string or null
	Organism             string                 `json:"organism"`
	CellLine             string                 `json:"cell_line"`
	PMID                 interface{}            `json:"pmid"` // Can be int or null
	CommentComplex       string                 `json:"comment_complex"`
	CommentMembers       string                 `json:"comment_members"`
	CommentDisease       string                 `json:"comment_disease"`
	CommentDrug          string                 `json:"comment_drug"`
	PurificationMethods  []corumMethod          `json:"purification_methods"`
	Subunits             []corumSubunit         `json:"subunits"`
	Functions            []corumFunction        `json:"functions"`
}

type corumMethod struct {
	MIID string `json:"mi_id"`
	Name string `json:"name"`
}

type corumSubunit struct {
	Stoichiometry interface{}     `json:"stoechiometrie"` // Can be string or null
	SwissProt     *corumSwissProt `json:"swissprot"`
}

type corumSwissProt struct {
	UniProtID          string      `json:"uniprot_id"`
	GeneName           string      `json:"gene_name"`
	GeneNameSynonyms   string      `json:"gene_name_synonyms"`
	ProteinName        string      `json:"protein_name"`
	ProteinNameSynonyms string     `json:"protein_name_synonyms"`
	Organism           string      `json:"organism"`
	EntrezID           interface{} `json:"entrez_id"` // Can be string or null
	Gene               *corumGene  `json:"gene"`
}

type corumGene struct {
	GeneName string       `json:"genname"`
	Drugs    []corumDrug  `json:"drugs"`
	OMIM     []corumOMIM  `json:"omim"`
}

type corumDrug struct {
	Name string `json:"name"`
}

type corumOMIM struct {
	MIMID   int    `json:"mim_id"`
	Acronym string `json:"acronym"`
	Name    string `json:"name"`
}

type corumFunction struct {
	Evidence string    `json:"evi"`
	PMID     int       `json:"pmid"`
	GO       *corumGO  `json:"go"`
}

type corumGO struct {
	Name     string `json:"name"`
	Ontology string `json:"ontology"`
	GOID     string `json:"go_id"`
}

func (c *corum) update() {
	defer c.d.wg.Done()

	log.Println("CORUM: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("CORUM: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Get path and source ID
	path := config.Dataconf[c.source]["path"]
	sourceID := config.Dataconf[c.source]["id"]

	// Open JSON file
	filePath := filepath.FromSlash(path)
	file, err := os.Open(filePath)
	c.check(err, "opening CORUM JSON file")
	defer file.Close()

	// Create buffered reader
	br := bufio.NewReaderSize(file, fileBufSize)

	// Decode JSON array
	decoder := json.NewDecoder(br)

	// Read opening bracket
	_, err = decoder.Token()
	c.check(err, "reading JSON opening bracket")

	var savedEntries int
	var previous int64

	// Process each complex
	for decoder.More() {
		var complex corumComplex
		err := decoder.Decode(&complex)
		if err != nil {
			log.Printf("CORUM: Error decoding complex: %v", err)
			continue
		}

		// Progress tracking
		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: int64(savedEntries / int(elapsed))}
		}

		// Entry ID is the complex_id as string
		entryID := strconv.Itoa(complex.ComplexID)

		// Test mode: log ID
		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Build attribute
		attr := c.buildAttr(&complex)

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		c.check(err, "marshaling CORUM attributes")

		c.d.addProp3(entryID, sourceID, attrBytes)

		// Text search indexing
		// Complex name
		c.d.addXref(complex.ComplexName, textLinkID, entryID, c.source, true)

		// Synonyms
		if synonyms := c.getSynonyms(complex.Synonyms); synonyms != nil {
			for _, syn := range synonyms {
				if syn != "" {
					c.d.addXref(syn, textLinkID, entryID, c.source, true)
				}
			}
		}

		// Gene names for text search
		for _, gene := range attr.SubunitGenes {
			if gene != "" {
				c.d.addXref(gene, textLinkID, entryID, c.source, true)
			}
		}

		// Cross-references
		// UniProt subunits
		for _, sub := range complex.Subunits {
			if sub.SwissProt != nil && sub.SwissProt.UniProtID != "" {
				c.d.addXref(entryID, sourceID, sub.SwissProt.UniProtID, "uniprot", false)
			}
		}

		// Entrez Gene IDs
		for _, sub := range complex.Subunits {
			if sub.SwissProt != nil {
				entrezID := c.getEntrezID(sub.SwissProt.EntrezID)
				if entrezID != "" {
					c.d.addXref(entryID, sourceID, entrezID, "entrez", false)
				}
			}
		}

		// GO terms
		for _, fn := range complex.Functions {
			if fn.GO != nil && fn.GO.GOID != "" {
				c.d.addXref(entryID, sourceID, fn.GO.GOID, "go", false)
			}
		}

		// PubMed
		if pmid := c.getPMID(complex.PMID); pmid != "" {
			c.d.addXref(entryID, sourceID, pmid, "pubmed", false)
		}

		// Taxonomy via organism mapping
		if taxID, ok := corumOrgToTaxID[complex.Organism]; ok {
			c.d.addXref(entryID, sourceID, taxID, "taxonomy", false)
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("CORUM: [TEST MODE] Reached limit, stopping")
			break
		}
	}

	log.Printf("CORUM: Saved %d protein complexes", savedEntries)
	log.Printf("CORUM: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedEntries))

	// Signal completion
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

// buildAttr constructs the CorumAttr protobuf message from a complex
func (c *corum) buildAttr(complex *corumComplex) *pbuf.CorumAttr {
	attr := &pbuf.CorumAttr{
		Name:           complex.ComplexName,
		Organism:       complex.Organism,
		CellLine:       complex.CellLine,
		Comment:        complex.CommentComplex,
		CommentDisease: complex.CommentDisease,
		CommentDrug:    complex.CommentDrug,
		SubunitCount:   int32(len(complex.Subunits)),
	}

	// Synonyms
	attr.Synonyms = c.getSynonyms(complex.Synonyms)

	// PMID
	if pmid := c.getPMID(complex.PMID); pmid != "" {
		if pmidInt, err := strconv.Atoi(pmid); err == nil {
			attr.Pmid = int32(pmidInt)
		}
	}

	// Purification methods
	for _, method := range complex.PurificationMethods {
		if method.Name != "" {
			attr.PurificationMethods = append(attr.PurificationMethods, method.Name)
		}
	}

	// Subunits
	hasDrugTargets := false
	geneSet := make(map[string]bool)

	for _, sub := range complex.Subunits {
		if sub.SwissProt == nil {
			continue
		}

		// Build protobuf subunit
		pbSub := &pbuf.CorumSubunit{
			UniprotId:   sub.SwissProt.UniProtID,
			GeneName:    sub.SwissProt.GeneName,
			ProteinName: sub.SwissProt.ProteinName,
			EntrezId:    c.getEntrezID(sub.SwissProt.EntrezID),
		}

		// Stoichiometry
		if sub.Stoichiometry != nil {
			if stoich, ok := sub.Stoichiometry.(string); ok {
				pbSub.Stoichiometry = stoich
			}
		}

		// Drugs from gene
		if sub.SwissProt.Gene != nil {
			for _, drug := range sub.SwissProt.Gene.Drugs {
				if drug.Name != "" {
					pbSub.Drugs = append(pbSub.Drugs, drug.Name)
					hasDrugTargets = true
				}
			}
		}

		attr.Subunits = append(attr.Subunits, pbSub)

		// Collect unique genes for text search
		if sub.SwissProt.GeneName != "" {
			geneSet[sub.SwissProt.GeneName] = true
		}
	}

	// Flatten gene list (for text search filtering)
	for gene := range geneSet {
		attr.SubunitGenes = append(attr.SubunitGenes, gene)
	}

	// Subset flags
	attr.HasDrugTargets = hasDrugTargets
	attr.HasSpliceVariants = strings.Contains(strings.ToLower(complex.CommentMembers), "splice")

	return attr
}

// getSynonyms extracts synonyms from the JSON value (can be string or null)
func (c *corum) getSynonyms(value interface{}) []string {
	if value == nil {
		return nil
	}
	if syn, ok := value.(string); ok && syn != "" {
		// Split by common delimiters
		return strings.Split(syn, ";")
	}
	return nil
}

// getPMID extracts PMID from the JSON value (can be int or null)
func (c *corum) getPMID(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	case string:
		return v
	}
	return ""
}

// getEntrezID extracts Entrez ID from the JSON value (can be string, int, or null)
func (c *corum) getEntrezID(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	case string:
		return v
	}
	return ""
}
