package update

import (
	"biobtree/pbuf"
	"encoding/csv"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type clinvar struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (c *clinvar) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// Main update entry point
func (c *clinvar) update() {
	defer c.d.wg.Done()

	log.Println("ClinVar: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("ClinVar: [TEST MODE] Processing up to %d variants", testLimit)
	}

	// Step 1: Load variant summary (core data)
	variants := c.loadVariantSummary(testLimit, idLogFile)
	log.Printf("ClinVar: Loaded %d variant entries (%.2fs)", len(variants), time.Since(startTime).Seconds())

	// Step 2: Load allele-gene mappings
	checkpointTime := time.Now()
	c.loadAlleleGene(variants)
	log.Printf("ClinVar: Processed allele-gene mappings (%.2fs)", time.Since(checkpointTime).Seconds())

	// Step 3: Load cross-references
	checkpointTime = time.Now()
	c.loadCrossReferences(variants)
	log.Printf("ClinVar: Processed cross-references (%.2fs)", time.Since(checkpointTime).Seconds())

	// Step 4: Load HGVS expressions
	checkpointTime = time.Now()
	c.loadHGVS(variants)
	log.Printf("ClinVar: Processed HGVS expressions (%.2fs)", time.Since(checkpointTime).Seconds())

	// Step 5: Save all entries to database
	checkpointTime = time.Now()
	c.saveVariants(variants)
	log.Printf("ClinVar: Saved %d variants to database (%.2fs)", len(variants), time.Since(checkpointTime).Seconds())

	log.Printf("ClinVar: Data processing complete (total: %.2fs)", time.Since(startTime).Seconds())
}

// loadVariantSummary parses variant_summary.txt.gz and builds variant map
func (c *clinvar) loadVariantSummary(testLimit int, idLogFile *os.File) map[string]*ClinvarVariant {
	variants := make(map[string]*ClinvarVariant)

	// Build full HTTPS URL for NCBI FTP
	filePath := "https://ftp.ncbi.nlm.nih.gov" + config.Dataconf[c.source]["path"] + config.Dataconf[c.source]["variantSummaryFile"]
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	c.check(err, "opening variant_summary.txt.gz")
	defer closeReaders(gz, ftpFile, client, localFile)

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.LazyQuotes = true      // Handle potential quote issues
	r.FieldsPerRecord = -1   // Variable number of fields

	lineNum := 0
	processedCount := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("ClinVar: Warning - error reading line %d: %v", lineNum, err)
			continue
		}

		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		// Ensure we have enough columns (variant_summary.txt.gz has 30+ columns)
		if len(record) < 31 {
			continue
		}

		// Column 30: VariationID (our primary key)
		variationID := strings.TrimSpace(record[30])
		if variationID == "" || variationID == "-1" {
			continue
		}

		// Skip if we've already processed this variant (avoid duplicates)
		if _, exists := variants[variationID]; exists {
			continue
		}

		// Build variant entry
		variant := &ClinvarVariant{
			VariationID:       variationID,
			AlleleID:          strings.TrimSpace(record[0]),
			Type:              strings.TrimSpace(record[1]),
			Name:              strings.TrimSpace(record[2]),
			GeneID:            strings.TrimSpace(record[3]),
			GeneSymbol:        strings.TrimSpace(record[4]),
			HGNC_ID:           strings.TrimSpace(record[5]),
			GermlineClass:     strings.TrimSpace(record[6]),
			ReviewStatus:      strings.TrimSpace(record[24]),
			LastEvaluated:     strings.TrimSpace(record[8]),
			NumberSubmitters:  strings.TrimSpace(record[25]),
			Assembly:          strings.TrimSpace(record[16]),
			Chromosome:        strings.TrimSpace(record[18]),
			Start:             strings.TrimSpace(record[19]),
			Stop:              strings.TrimSpace(record[20]),
			ReferenceAllele:   strings.TrimSpace(record[21]),
			AlternateAllele:   strings.TrimSpace(record[22]),
			dbSNP:             strings.TrimSpace(record[9]),
			PhenotypeIDs:      strings.TrimSpace(record[12]),
			PhenotypeList:     strings.TrimSpace(record[13]),
		}

		// Add to map
		variants[variationID] = variant

		// Add searchable text links (like ChEBI does with names: src/update/chebi.go:457-459)
		// 1. Variant name (HGVS)
		if variant.Name != "" && variant.Name != "-" {
			c.d.addXref(variant.Name, textLinkID, variationID, c.source, true) // textLinkID = "0" for keyword search
		}

		// 2. dbSNP ID
		if variant.dbSNP != "" && variant.dbSNP != "-1" && variant.dbSNP != "-" {
			rsID := variant.dbSNP
			if !strings.HasPrefix(rsID, "rs") {
				rsID = "rs" + rsID
			}
			c.d.addXref(rsID, textLinkID, variationID, c.source, true)
		}

		// 3. Gene symbol (for text search)
		if variant.GeneSymbol != "" && variant.GeneSymbol != "-" {
			c.d.addXref(variant.GeneSymbol, textLinkID, variationID, c.source, true)
		}

		processedCount++

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, variationID)
		}

		// Test mode limit check
		if testLimit > 0 && processedCount >= testLimit {
			log.Printf("ClinVar: [TEST MODE] Reached limit of %d variants, stopping", testLimit)
			break
		}

		// Progress logging every 100k variants
		if processedCount%100000 == 0 {
			log.Printf("ClinVar: Processed %d variants...", processedCount)
		}
	}

	log.Printf("ClinVar: Parsed %d lines, kept %d variants", lineNum, len(variants))
	return variants
}

// Helper struct for in-memory variant data
type ClinvarVariant struct {
	VariationID      string
	AlleleID         string
	Type             string
	Name             string
	GeneID           string
	GeneSymbol       string
	HGNC_ID          string
	GermlineClass    string
	ReviewStatus     string
	LastEvaluated    string
	NumberSubmitters string
	Assembly         string
	Chromosome       string
	Start            string
	Stop             string
	ReferenceAllele  string
	AlternateAllele  string
	dbSNP            string
	PhenotypeIDs     string
	PhenotypeList    string
	HGVSExpressions  []string // Added from hgvs4variation.txt.gz
}

func parseIntOrZero(s string) int32 {
	if s == "" || s == "-" {
		return 0
	}
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(val)
}

// loadAlleleGene parses allele_gene.txt.gz and creates gene cross-references
func (c *clinvar) loadAlleleGene(variants map[string]*ClinvarVariant) {
	// Step 1: Build reverse index for fast lookup (O(n) instead of O(n*m))
	log.Println("ClinVar: Building AlleleID index for gene mappings...")
	alleleIndex := make(map[string][]*ClinvarVariant)
	for _, variant := range variants {
		if variant.AlleleID != "" && variant.AlleleID != "-1" {
			alleleIndex[variant.AlleleID] = append(alleleIndex[variant.AlleleID], variant)
		}
	}
	log.Printf("ClinVar: Indexed %d unique AlleleIDs", len(alleleIndex))

	// Step 2: Load and process allele_gene file
	filePath := "https://ftp.ncbi.nlm.nih.gov" + config.Dataconf[c.source]["path"] + config.Dataconf[c.source]["alleleGeneFile"]
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	c.check(err, "opening allele_gene.txt.gz")
	defer closeReaders(gz, ftpFile, client, localFile)

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	lineNum := 0
	mappingsAdded := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("ClinVar: Warning - error reading allele_gene line %d: %v", lineNum, err)
			continue
		}

		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		// Ensure we have enough columns
		// Format: AlleleID, GeneID, Symbol, RelationshipType, etc.
		if len(record) < 3 {
			continue
		}

		alleleID := strings.TrimSpace(record[0])
		geneID := strings.TrimSpace(record[1])
		// geneSymbol at record[2] not used - we use HGNC ID from variant data instead

		if alleleID == "" || alleleID == "-1" || geneID == "" || geneID == "-1" {
			continue
		}

		// Fast O(1) lookup instead of O(n) scan
		if matchedVariants, exists := alleleIndex[alleleID]; exists {
			for _, variant := range matchedVariants {
				// Create bidirectional cross-references (pattern from Reactome: src/update/reactome.go)
				// Variant -> Gene (Entrez Gene ID)
				if geneID != "" && geneID != "-" {
					c.d.addXref(variant.VariationID, config.Dataconf[c.source]["id"], geneID, "gene", false)
				}

				// Variant -> HGNC (use HGNC ID from variant data, not gene symbol)
				if variant.HGNC_ID != "" && variant.HGNC_ID != "-" {
					c.d.addXref(variant.VariationID, config.Dataconf[c.source]["id"], variant.HGNC_ID, "hgnc", false)
				}

				mappingsAdded++
			}
		}

		// Progress logging every 500k lines
		if lineNum%500000 == 0 {
			log.Printf("ClinVar: Processed %d allele_gene lines, %d mappings added...", lineNum, mappingsAdded)
		}
	}

	log.Printf("ClinVar: Added %d gene mappings from %d lines", mappingsAdded, lineNum)
}

// loadCrossReferences parses cross_references.txt and maps external database IDs
func (c *clinvar) loadCrossReferences(variants map[string]*ClinvarVariant) {
	filePath := "https://ftp.ncbi.nlm.nih.gov" + config.Dataconf[c.source]["path"] + config.Dataconf[c.source]["crossReferencesFile"]
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	c.check(err, "opening cross_references.txt")
	defer closeReaders(gz, ftpFile, client, localFile)

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	lineNum := 0
	xrefsAdded := 0

	// Database name mapping to biobtree keyids
	dbMap := map[string]string{
		"OMIM":      "omim",
		"UniProtKB": "uniprot",
		"dbSNP":     "snp",
		"MONDO":     "mondo",
		"Orphanet":  "orphanet",
	}

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("ClinVar: Warning - error reading cross_references line %d: %v", lineNum, err)
			continue
		}

		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		// Format: VariationID, Database, ID
		if len(record) < 3 {
			continue
		}

		variationID := strings.TrimSpace(record[0])
		database := strings.TrimSpace(record[1])
		externalID := strings.TrimSpace(record[2])

		if variationID == "" || database == "" || externalID == "" {
			continue
		}

		// Only process if we have this variant loaded
		if _, exists := variants[variationID]; !exists {
			continue
		}

		// Map database to biobtree dataset name
		datasetName, found := dbMap[database]
		if !found {
			continue // Skip unknown databases
		}

		// Verify dataset exists in config
		if _, exists := config.Dataconf[datasetName]; !exists {
			continue
		}

		// Add cross-reference
		c.d.addXref(variationID, config.Dataconf[c.source]["id"], externalID, datasetName, false)
		xrefsAdded++
	}

	log.Printf("ClinVar: Added %d cross-references from %d lines", xrefsAdded, lineNum)
}

// loadHGVS parses hgvs4variation.txt.gz and adds HGVS expressions to variants
func (c *clinvar) loadHGVS(variants map[string]*ClinvarVariant) {
	filePath := "https://ftp.ncbi.nlm.nih.gov" + config.Dataconf[c.source]["path"] + config.Dataconf[c.source]["hgvsFile"]
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	c.check(err, "opening hgvs4variation.txt.gz")
	defer closeReaders(gz, ftpFile, client, localFile)

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	lineNum := 0
	hgvsAdded := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("ClinVar: Warning - error reading hgvs4variation line %d: %v", lineNum, err)
			continue
		}

		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		// Format: VariationID, HGVS_Name, Type, Assembly, etc.
		if len(record) < 2 {
			continue
		}

		variationID := strings.TrimSpace(record[0])
		hgvsName := strings.TrimSpace(record[1])

		if variationID == "" || hgvsName == "" || hgvsName == "-" {
			continue
		}

		// Add HGVS expression to variant
		if variant, exists := variants[variationID]; exists {
			variant.HGVSExpressions = append(variant.HGVSExpressions, hgvsName)
			hgvsAdded++
		}
	}

	log.Printf("ClinVar: Added %d HGVS expressions from %d lines", hgvsAdded, lineNum)
}

// saveVariants marshals variants to protobuf and saves to database
func (c *clinvar) saveVariants(variants map[string]*ClinvarVariant) {
	// Sort variant IDs for consistent ordering (pattern from ChEBI: src/update/chebi.go:530-545)
	variantIDs := make([]string, 0, len(variants))
	for id := range variants {
		variantIDs = append(variantIDs, id)
	}
	sort.Strings(variantIDs)

	savedCount := int32(0)

	for _, variationID := range variantIDs {
		variant := variants[variationID]

		// Build ClinvarAttr protobuf message
		attr := &pbuf.ClinvarAttr{
			VariationId:             variationID,
			AlleleId:                variant.AlleleID,
			Name:                    variant.Name,
			HgvsExpressions:         variant.HGVSExpressions,
			DbsnpId:                 variant.dbSNP,
			Type:                    variant.Type,
			Chromosome:              variant.Chromosome,
			Start:                   parseIntOrZero(variant.Start),
			Stop:                    parseIntOrZero(variant.Stop),
			ReferenceAllele:         variant.ReferenceAllele,
			AlternateAllele:         variant.AlternateAllele,
			Assembly:                variant.Assembly,
			GermlineClassification: variant.GermlineClass,
			ReviewStatus:            variant.ReviewStatus,
			LastEvaluated:           variant.LastEvaluated,
			NumberSubmitters:        parseIntOrZero(variant.NumberSubmitters),
			GeneId:                  variant.GeneID,
			GeneSymbol:              variant.GeneSymbol,
			HgncId:                  variant.HGNC_ID,
		}

		// Parse phenotype lists (pattern from clinical trials: src/update/clinicalt.go)
		if variant.PhenotypeList != "" && variant.PhenotypeList != "-" {
			// Split semicolon-separated phenotypes
			phenotypes := strings.Split(variant.PhenotypeList, ";")
			for i, p := range phenotypes {
				phenotypes[i] = strings.TrimSpace(p)
			}
			attr.PhenotypeList = phenotypes
		}

		if variant.PhenotypeIDs != "" && variant.PhenotypeIDs != "-" {
			// Split semicolon-separated IDs
			ids := strings.Split(variant.PhenotypeIDs, ";")
			for i, id := range ids {
				ids[i] = strings.TrimSpace(id)
			}
			attr.PhenotypeIds = ids
		}

		// Save to database (pattern from ChEBI: src/update/chebi.go:645-648)
		b, err := ffjson.Marshal(&attr)
		c.check(err, "marshaling ClinVar attributes")
		c.d.addProp3(variationID, config.Dataconf[c.source]["id"], b)

		atomic.AddInt32(&savedCount, 1)

		// Progress logging every 10k variants
		if savedCount%10000 == 0 {
			log.Printf("ClinVar: Saved %d variants...", savedCount)
		}
	}

	log.Printf("ClinVar: Successfully saved %d variants to database", savedCount)

	// Report statistics
	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedCount))
}
