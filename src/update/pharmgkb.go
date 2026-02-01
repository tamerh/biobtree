package update

import (
	"archive/zip"
	"biobtree/pbuf"
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// pharmgkb handles parsing PharmGKB/ClinPGx pharmacogenomics data
// Source: https://www.clinpgx.org/
// Files processed:
// - chemicals.zip: Drug/chemical vocabulary with cross-references
// - genes.zip: Pharmacogenes with VIP flags and annotations
// - clinicalVariants.zip: Clinical variant-drug annotations
// - relationships.zip: Gene-drug relationships with evidence
type pharmgkb struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (p *pharmgkb) check(err error, operation string) {
	checkWithContext(err, p.source, operation)
}

// Chemical entry aggregating data from multiple PharmGKB files
type pharmgkbChemicalEntry struct {
	pharmgkbID            string
	name                  string
	chemType              string
	genericNames          []string
	tradeNames            []string
	brandMixtures         []string
	smiles                string
	inchi                 string
	inchiKey              string
	rxnormIDs             []string
	atcCodes              []string
	pubchemCIDs           []string
	clinicalAnnotCount    int32
	variantAnnotCount     int32
	pathwayCount          int32
	topClinicalLevel      string
	topFDALabelLevel      string
	topAnyLabelLevel      string
	hasDosingGuideline    bool
	hasPrescribingInfo    bool
	dosingGuidelineSrcs   []string
	topCPICLevel          string
	inFDAPGxSections      bool
	relatedGenes          []*pbuf.PharmgkbRelatedGene
	drugLabels            []*pbuf.PharmgkbDrugLabel
}

// Gene entry from genes.zip
type pharmgkbGeneEntry struct {
	pharmgkbID         string
	ncbiGeneID         string
	hgncID             string
	ensemblID          string
	name               string
	symbol             string
	alternateNames     []string
	alternateSymbols   []string
	isVIP              bool
	hasVariantAnnot    bool
	hasCPICGuideline   bool
	chromosome         string
	startGRCh37        int64
	endGRCh37          int64
	startGRCh38        int64
	endGRCh38          int64
}

func (p *pharmgkb) update() {
	defer p.d.wg.Done()

	log.Println("PharmGKB: Starting comprehensive data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(p.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("PharmGKB: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Phase 1: Load chemicals (drugs) - main dataset
	chemicals := p.loadChemicals(testLimit)
	log.Printf("PharmGKB: Phase 1 complete - loaded %d chemicals", len(chemicals))

	// Phase 2: Load gene-drug relationships and enrich chemical entries
	p.loadRelationships(chemicals, testLimit)
	log.Printf("PharmGKB: Phase 2 complete - loaded gene-drug relationships")

	// Phase 3: Load drug labels and enrich chemical entries
	p.loadDrugLabels(chemicals)
	log.Printf("PharmGKB: Phase 3 complete - loaded drug labels")

	// Phase 4: Save chemical entries and create cross-references
	p.saveChemicalEntries(chemicals, idLogFile)

	// Phase 5: Process genes as separate dataset (pharmgkb_gene)
	p.processGenes(testLimit)

	// Phase 6: Load phenotype mappings for cross-reference enrichment
	phenotypeMappings := p.loadPhenotypeMappings()
	log.Printf("PharmGKB: Phase 6 complete - loaded %d phenotype mappings", len(phenotypeMappings))

	// Phase 7: Process clinical annotations as separate dataset (pharmgkb_clinical)
	p.processClinicalVariants(testLimit, phenotypeMappings)

	// Phase 8: Process variants as separate dataset (pharmgkb_variant)
	// First load summary annotations for enrichment, then process variants
	summaryAnnotations := p.loadSummaryAnnotations()
	p.processVariants(testLimit, summaryAnnotations, phenotypeMappings)

	// Phase 9: Process guidelines as separate dataset (pharmgkb_guideline)
	p.processGuidelines(testLimit)

	// Phase 10: Process pathways as separate dataset (pharmgkb_pathway)
	p.processPathways(testLimit)

	log.Printf("PharmGKB: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress tracker
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
}

// openZipFile opens a zip file and returns its contents
func (p *pharmgkb) openZipFile(filename string) (*zip.Reader, []byte, error) {
	basePath := config.Dataconf[p.source]["path"]
	filePath := filepath.Join(basePath, filename)

	log.Printf("PharmGKB: Opening %s", filePath)

	zipBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, nil, err
	}

	return zipReader, zipBytes, nil
}

// loadChemicals reads chemicals.zip and creates base chemical entries
// Columns: PharmGKB Accession Id, Name, Generic Names, Trade Names, Brand Mixtures,
// Type, Cross-references, SMILES, InChI, Dosing Guideline, External Vocabulary,
// Clinical Annotation Count, Variant Annotation Count, Pathway Count, VIP Count,
// Dosing Guideline Sources, Top Clinical Annotation Level, Top FDA Label Testing Level,
// Top Any Drug Label Testing Level, Label Has Dosing Info, RxNorm Identifiers,
// ATC Identifiers, PubChem Compound Identifiers, Top CPIC Pairs Level,
// FDA Label has Prescribing Info, In FDA PGx Association Sections
func (p *pharmgkb) loadChemicals(testLimit int) map[string]*pharmgkbChemicalEntry {
	chemFile := config.Dataconf[p.source]["chemicalsFile"]
	zipReader, _, err := p.openZipFile(chemFile)
	p.check(err, "opening chemicals zip file")

	chemicals := make(map[string]*pharmgkbChemicalEntry)

	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".tsv") {
			reader, err := file.Open()
			p.check(err, "opening TSV file in chemicals zip")
			defer reader.Close()

			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 1024*1024)

			var headerParsed bool
			var colMap map[string]int
			var entryCount int

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				fields := strings.Split(line, "\t")

				// Parse header
				if !headerParsed {
					colMap = make(map[string]int)
					for i, name := range fields {
						colMap[strings.TrimSpace(name)] = i
					}
					headerParsed = true
					continue
				}

				pharmgkbID := safeFieldByCol(fields, colMap, "PharmGKB Accession Id")
				if pharmgkbID == "" {
					continue
				}

				entry := &pharmgkbChemicalEntry{
					pharmgkbID:   pharmgkbID,
					name:         safeFieldByCol(fields, colMap, "Name"),
					chemType:     safeFieldByCol(fields, colMap, "Type"),
					smiles:       safeFieldByCol(fields, colMap, "SMILES"),
					inchi:        safeFieldByCol(fields, colMap, "InChI"),
				}

				// Parse multi-value fields
				entry.genericNames = parseQuotedList(safeFieldByCol(fields, colMap, "Generic Names"))
				entry.tradeNames = parseQuotedList(safeFieldByCol(fields, colMap, "Trade Names"))
				entry.brandMixtures = parseQuotedList(safeFieldByCol(fields, colMap, "Brand Mixtures"))
				entry.rxnormIDs = splitAndClean(safeFieldByCol(fields, colMap, "RxNorm Identifiers"), ",")
				entry.atcCodes = splitAndClean(safeFieldByCol(fields, colMap, "ATC Identifiers"), ",")
				entry.pubchemCIDs = splitAndClean(safeFieldByCol(fields, colMap, "PubChem Compound Identifiers"), ",")
				entry.dosingGuidelineSrcs = splitAndClean(safeFieldByCol(fields, colMap, "Dosing Guideline Sources"), ",")

				// Parse counts
				entry.clinicalAnnotCount = parseIntField(safeFieldByCol(fields, colMap, "Clinical Annotation Count"))
				entry.variantAnnotCount = parseIntField(safeFieldByCol(fields, colMap, "Variant Annotation Count"))
				entry.pathwayCount = parseIntField(safeFieldByCol(fields, colMap, "Pathway Count"))

				// Parse levels
				entry.topClinicalLevel = safeFieldByCol(fields, colMap, "Top Clinical Annotation Level")
				entry.topFDALabelLevel = safeFieldByCol(fields, colMap, "Top FDA Label Testing Level")
				entry.topAnyLabelLevel = safeFieldByCol(fields, colMap, "Top Any Drug Label Testing Level")
				entry.topCPICLevel = safeFieldByCol(fields, colMap, "Top CPIC Pairs Level")

				// Parse boolean fields
				entry.hasDosingGuideline = safeFieldByCol(fields, colMap, "Dosing Guideline") == "Yes" ||
					safeFieldByCol(fields, colMap, "Label Has Dosing Info") == "Yes"
				entry.hasPrescribingInfo = safeFieldByCol(fields, colMap, "FDA Label has Prescribing Info") == "Yes"
				entry.inFDAPGxSections = safeFieldByCol(fields, colMap, "In FDA PGx Association Sections") == "Yes"

				// Extract InChI Key from InChI if present
				if entry.inchi != "" && strings.HasPrefix(entry.inchi, "InChI=") {
					// InChI key is typically a separate field or derived
					// Check cross-references for it
				}

				chemicals[pharmgkbID] = entry
				entryCount++

				if testLimit > 0 && entryCount >= testLimit {
					log.Printf("PharmGKB: [TEST MODE] Reached chemical limit of %d", testLimit)
					break
				}
			}

			if err := scanner.Err(); err != nil {
				log.Printf("PharmGKB: Scanner error reading chemicals: %v", err)
			}
			break
		}
	}

	return chemicals
}

// loadRelationships reads relationships.zip and adds gene relationships to chemicals
// Columns: Entity1_id, Entity1_name, Entity1_type, Entity2_id, Entity2_name,
// Entity2_type, Evidence, Association, PK, PD, PMIDs
func (p *pharmgkb) loadRelationships(chemicals map[string]*pharmgkbChemicalEntry, testLimit int) {
	relFile := config.Dataconf[p.source]["relationshipsFile"]
	zipReader, _, err := p.openZipFile(relFile)
	p.check(err, "opening relationships zip file")

	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".tsv") {
			reader, err := file.Open()
			p.check(err, "opening TSV file in relationships zip")
			defer reader.Close()

			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 1024*1024)

			var headerParsed bool
			var colMap map[string]int
			var relCount int

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				fields := strings.Split(line, "\t")

				// Parse header
				if !headerParsed {
					colMap = make(map[string]int)
					for i, name := range fields {
						colMap[strings.TrimSpace(name)] = i
					}
					headerParsed = true
					continue
				}

				entity1ID := safeFieldByCol(fields, colMap, "Entity1_id")
				entity1Type := safeFieldByCol(fields, colMap, "Entity1_type")
				entity2ID := safeFieldByCol(fields, colMap, "Entity2_id")
				entity2Type := safeFieldByCol(fields, colMap, "Entity2_type")
				entity2Name := safeFieldByCol(fields, colMap, "Entity2_name")
				evidence := safeFieldByCol(fields, colMap, "Evidence")

				// We want Gene -> Chemical relationships
				if entity1Type == "Gene" && entity2Type == "Chemical" {
					if chem, exists := chemicals[entity2ID]; exists {
						// Limit to 50 related genes per chemical
						if len(chem.relatedGenes) < 50 {
							relGene := &pbuf.PharmgkbRelatedGene{
								GeneSymbol:       safeFieldByCol(fields, colMap, "Entity1_name"),
								PharmgkbGeneId:   entity1ID,
								RelationshipType: safeFieldByCol(fields, colMap, "Association"),
								EvidenceType:     evidence,
							}
							chem.relatedGenes = append(chem.relatedGenes, relGene)
						}
					}
				}

				// Also handle Chemical -> Gene relationships
				if entity1Type == "Chemical" && entity2Type == "Gene" {
					if chem, exists := chemicals[entity1ID]; exists {
						if len(chem.relatedGenes) < 50 {
							relGene := &pbuf.PharmgkbRelatedGene{
								GeneSymbol:       entity2Name,
								PharmgkbGeneId:   entity2ID,
								RelationshipType: safeFieldByCol(fields, colMap, "Association"),
								EvidenceType:     evidence,
							}
							chem.relatedGenes = append(chem.relatedGenes, relGene)
						}
					}
				}

				relCount++
				if relCount%50000 == 0 {
					log.Printf("PharmGKB: Processed %d relationships...", relCount)
				}
			}

			if err := scanner.Err(); err != nil {
				log.Printf("PharmGKB: Scanner error reading relationships: %v", err)
			}
			log.Printf("PharmGKB: Total relationships processed: %d", relCount)
			break
		}
	}
}

// saveChemicalEntries saves all chemical entries and creates cross-references
func (p *pharmgkb) saveChemicalEntries(chemicals map[string]*pharmgkbChemicalEntry, idLogFile *os.File) {
	sourceID := config.Dataconf[p.source]["id"]
	var savedCount int

	for chemID, entry := range chemicals {
		// Build protobuf attribute
		attr := p.buildChemicalAttr(entry)

		// Marshal attributes
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("PharmGKB: Error marshaling %s: %v", chemID, err)
			continue
		}

		// Save primary entry
		p.d.addProp3(chemID, sourceID, attrBytes)

		// Create cross-references
		p.createChemicalCrossRefs(chemID, entry, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(chemID + "\n")
		}

		savedCount++

		if savedCount%1000 == 0 {
			log.Printf("PharmGKB: Saved %d chemical entries...", savedCount)
		}
	}

	atomic.AddUint64(&p.d.totalParsedEntry, uint64(savedCount))
	log.Printf("PharmGKB: Total chemical entries saved: %d", savedCount)
}

// buildChemicalAttr creates the protobuf attribute for a chemical entry
func (p *pharmgkb) buildChemicalAttr(entry *pharmgkbChemicalEntry) *pbuf.PharmgkbAttr {
	return &pbuf.PharmgkbAttr{
		PharmgkbId:              entry.pharmgkbID,
		Name:                    entry.name,
		Type:                    entry.chemType,
		GenericNames:            entry.genericNames,
		TradeNames:              entry.tradeNames,
		BrandMixtures:           entry.brandMixtures,
		Smiles:                  entry.smiles,
		Inchi:                   entry.inchi,
		InchiKey:                entry.inchiKey,
		RxnormIds:               entry.rxnormIDs,
		AtcCodes:                entry.atcCodes,
		PubchemCids:             entry.pubchemCIDs,
		ClinicalAnnotationCount: entry.clinicalAnnotCount,
		VariantAnnotationCount:  entry.variantAnnotCount,
		PathwayCount:            entry.pathwayCount,
		TopClinicalLevel:        entry.topClinicalLevel,
		TopFdaLabelLevel:        entry.topFDALabelLevel,
		TopAnyLabelLevel:        entry.topAnyLabelLevel,
		HasDosingGuideline:      entry.hasDosingGuideline,
		HasPrescribingInfo:      entry.hasPrescribingInfo,
		DosingGuidelineSources:  entry.dosingGuidelineSrcs,
		RelatedGenes:            entry.relatedGenes,
		DrugLabels:              entry.drugLabels,
	}
}

// createChemicalCrossRefs builds all cross-references for a PharmGKB chemical entry
func (p *pharmgkb) createChemicalCrossRefs(chemID string, entry *pharmgkbChemicalEntry, sourceID string) {
	// Text search: chemical name and synonyms
	if entry.name != "" {
		p.d.addXref(entry.name, textLinkID, chemID, p.source, true)
	}

	// Generic names as text search
	for _, name := range entry.genericNames {
		if name != "" && len(name) > 2 && len(name) < 200 {
			p.d.addXref(name, textLinkID, chemID, p.source, true)
		}
	}

	// Trade names as text search
	for _, name := range entry.tradeNames {
		if name != "" && len(name) > 2 && len(name) < 200 {
			p.d.addXref(name, textLinkID, chemID, p.source, true)
		}
	}

	// Cross-reference: PharmGKB → PubChem
	for _, cid := range entry.pubchemCIDs {
		if cid != "" {
			if _, exists := config.Dataconf["pubchem"]; exists {
				p.d.addXref(chemID, sourceID, cid, "pubchem", false)
			}
		}
	}

	// Cross-reference: PharmGKB → ChEBI (via PubChem)
	// ChEBI has PubChem mappings

	// Cross-reference: PharmGKB → DrugBank (via name matching handled by drugbank parser)

	// Cross-references for related genes
	genesSeen := make(map[string]bool)
	for _, gene := range entry.relatedGenes {
		geneID := gene.PharmgkbGeneId
		if geneID != "" && !genesSeen[geneID] {
			genesSeen[geneID] = true
			// Link to pharmgkb_gene dataset
			if _, exists := config.Dataconf["pharmgkb_gene"]; exists {
				geneSourceID := config.Dataconf["pharmgkb_gene"]["id"]
				p.d.addXref(chemID, sourceID, geneID, "pharmgkb_gene", false)
				// Bidirectional
				p.d.addXref(geneID, geneSourceID, chemID, p.source, false)
			}
		}

		// Also link to HGNC via gene symbol
		if gene.GeneSymbol != "" {
			if _, exists := config.Dataconf["hgnc"]; exists {
				p.d.addXref(chemID, sourceID, gene.GeneSymbol, "hgnc", false)
			}
		}
	}
}

// processGenes reads genes.zip and saves gene entries to pharmgkb_gene dataset
func (p *pharmgkb) processGenes(testLimit int) {
	geneSource := "pharmgkb_gene"
	if _, exists := config.Dataconf[geneSource]; !exists {
		log.Printf("PharmGKB: Skipping genes - %s dataset not configured", geneSource)
		return
	}

	geneSourceID := config.Dataconf[geneSource]["id"]

	geneFile := config.Dataconf[p.source]["genesFile"]
	zipReader, _, err := p.openZipFile(geneFile)
	p.check(err, "opening genes zip file")

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, geneSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".tsv") {
			reader, err := file.Open()
			p.check(err, "opening TSV file in genes zip")
			defer reader.Close()

			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 1024*1024)

			var headerParsed bool
			var colMap map[string]int
			var entryCount int

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				fields := strings.Split(line, "\t")

				// Parse header
				if !headerParsed {
					colMap = make(map[string]int)
					for i, name := range fields {
						colMap[strings.TrimSpace(name)] = i
					}
					headerParsed = true
					continue
				}

				entry := p.parseGeneEntry(fields, colMap)
				if entry.pharmgkbID == "" {
					continue
				}

				// Build and save gene entry
				attr := p.buildGeneAttr(entry)
				attrBytes, err := ffjson.Marshal(attr)
				if err != nil {
					log.Printf("PharmGKB: Error marshaling gene %s: %v", entry.pharmgkbID, err)
					continue
				}

				p.d.addProp3(entry.pharmgkbID, geneSourceID, attrBytes)

				// Create gene cross-references
				p.createGeneCrossRefs(entry, geneSourceID)

				if idLogFile != nil {
					idLogFile.WriteString(entry.pharmgkbID + "\n")
				}

				entryCount++

				if testLimit > 0 && entryCount >= testLimit {
					log.Printf("PharmGKB: [TEST MODE] Reached gene limit of %d", testLimit)
					break
				}
			}

			if err := scanner.Err(); err != nil {
				log.Printf("PharmGKB: Scanner error reading genes: %v", err)
			}

			atomic.AddUint64(&p.d.totalParsedEntry, uint64(entryCount))
			log.Printf("PharmGKB: Total gene entries saved: %d", entryCount)
			break
		}
	}

	// Signal completion for gene dataset
	p.d.progChan <- &progressInfo{dataset: geneSource, done: true}
}

// parseGeneEntry parses a gene row from genes.tsv
func (p *pharmgkb) parseGeneEntry(fields []string, colMap map[string]int) *pharmgkbGeneEntry {
	entry := &pharmgkbGeneEntry{
		pharmgkbID:   safeFieldByCol(fields, colMap, "PharmGKB Accession Id"),
		ncbiGeneID:   safeFieldByCol(fields, colMap, "NCBI Gene ID"),
		hgncID:       safeFieldByCol(fields, colMap, "HGNC ID"),
		ensemblID:    safeFieldByCol(fields, colMap, "Ensembl Id"),
		name:         safeFieldByCol(fields, colMap, "Name"),
		symbol:       safeFieldByCol(fields, colMap, "Symbol"),
		chromosome:   safeFieldByCol(fields, colMap, "Chromosome"),
	}

	entry.alternateNames = parseQuotedList(safeFieldByCol(fields, colMap, "Alternate Names"))
	entry.alternateSymbols = parseQuotedList(safeFieldByCol(fields, colMap, "Alternate Symbols"))

	entry.isVIP = safeFieldByCol(fields, colMap, "Is VIP") == "Yes"
	entry.hasVariantAnnot = safeFieldByCol(fields, colMap, "Has Variant Annotation") == "Yes"
	entry.hasCPICGuideline = safeFieldByCol(fields, colMap, "Has CPIC Dosing Guideline") == "Yes"

	// Parse coordinates
	entry.startGRCh37 = parseInt64Field(safeFieldByCol(fields, colMap, "Chromosomal Start - GRCh37"))
	entry.endGRCh37 = parseInt64Field(safeFieldByCol(fields, colMap, "Chromosomal Stop - GRCh37"))
	entry.startGRCh38 = parseInt64Field(safeFieldByCol(fields, colMap, "Chromosomal Start - GRCh38"))
	entry.endGRCh38 = parseInt64Field(safeFieldByCol(fields, colMap, "Chromosomal Stop - GRCh38"))

	return entry
}

// buildGeneAttr creates the protobuf attribute for a gene entry
func (p *pharmgkb) buildGeneAttr(entry *pharmgkbGeneEntry) *pbuf.PharmgkbGeneAttr {
	return &pbuf.PharmgkbGeneAttr{
		PharmgkbId:         entry.pharmgkbID,
		Symbol:             entry.symbol,
		Name:               entry.name,
		AlternateNames:     entry.alternateNames,
		AlternateSymbols:   entry.alternateSymbols,
		IsVip:              entry.isVIP,
		HasVariantAnnotation: entry.hasVariantAnnot,
		HasCpicGuideline:   entry.hasCPICGuideline,
		Chromosome:         entry.chromosome,
		StartGrch37:        entry.startGRCh37,
		EndGrch37:          entry.endGRCh37,
		StartGrch38:        entry.startGRCh38,
		EndGrch38:          entry.endGRCh38,
		HgncId:             entry.hgncID,
		EntrezId:           entry.ncbiGeneID,
		EnsemblId:          entry.ensemblID,
	}
}

// createGeneCrossRefs creates cross-references for a gene entry
func (p *pharmgkb) createGeneCrossRefs(entry *pharmgkbGeneEntry, sourceID string) {
	// Text search: gene symbol and name
	if entry.symbol != "" {
		p.d.addXref(entry.symbol, textLinkID, entry.pharmgkbID, "pharmgkb_gene", true)
	}
	if entry.name != "" && len(entry.name) > 2 {
		p.d.addXref(entry.name, textLinkID, entry.pharmgkbID, "pharmgkb_gene", true)
	}

	// Cross-reference: PharmGKB Gene → HGNC
	if entry.hgncID != "" {
		if _, exists := config.Dataconf["hgnc"]; exists {
			// HGNC IDs in biobtree use "HGNC:123" format
			hgncID := entry.hgncID
			if !strings.HasPrefix(hgncID, "HGNC:") {
				hgncID = "HGNC:" + hgncID
			}
			p.d.addXref(entry.pharmgkbID, sourceID, hgncID, "hgnc", false)
		}
	}

	// Cross-reference: PharmGKB Gene → Entrez Gene
	if entry.ncbiGeneID != "" {
		if _, exists := config.Dataconf["entrez"]; exists {
			p.d.addXref(entry.pharmgkbID, sourceID, entry.ncbiGeneID, "entrez", false)
		}
	}

	// Cross-reference: PharmGKB Gene → Ensembl
	if entry.ensemblID != "" {
		if _, exists := config.Dataconf["ensembl"]; exists {
			p.d.addXref(entry.pharmgkbID, sourceID, entry.ensemblID, "ensembl", false)
		}
	}
}

// processClinicalVariants reads clinicalVariants.zip and saves to pharmgkb_clinical dataset
func (p *pharmgkb) processClinicalVariants(testLimit int, phenotypeMappings map[string]*phenotypeMapping) {
	clinSource := "pharmgkb_clinical"
	if _, exists := config.Dataconf[clinSource]; !exists {
		log.Printf("PharmGKB: Skipping clinical variants - %s dataset not configured", clinSource)
		return
	}

	clinSourceID := config.Dataconf[clinSource]["id"]

	clinFile := config.Dataconf[p.source]["clinicalVariantsFile"]
	zipReader, _, err := p.openZipFile(clinFile)
	p.check(err, "opening clinical variants zip file")

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, clinSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".tsv") {
			reader, err := file.Open()
			p.check(err, "opening TSV file in clinical variants zip")
			defer reader.Close()

			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 1024*1024)

			var headerParsed bool
			var colMap map[string]int
			var entryCount int

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				fields := strings.Split(line, "\t")

				// Parse header
				if !headerParsed {
					colMap = make(map[string]int)
					for i, name := range fields {
						colMap[strings.TrimSpace(name)] = i
					}
					headerParsed = true
					continue
				}

				variant := safeFieldByCol(fields, colMap, "variant")
				gene := safeFieldByCol(fields, colMap, "gene")
				annotType := safeFieldByCol(fields, colMap, "type")
				level := safeFieldByCol(fields, colMap, "level of evidence")
				chemicals := parseQuotedList(safeFieldByCol(fields, colMap, "chemicals"))
				phenotypes := parseQuotedList(safeFieldByCol(fields, colMap, "phenotypes"))

				if variant == "" {
					continue
				}

				// Create unique ID combining variant and gene
				clinID := variant
				if gene != "" {
					clinID = variant + "_" + gene
				}
				// Sanitize ID
				clinID = strings.ReplaceAll(clinID, " ", "_")
				clinID = strings.ReplaceAll(clinID, ",", "_")

				attr := &pbuf.PharmgkbClinicalAttr{
					Variant:         variant,
					Gene:            gene,
					Type:            annotType,
					LevelOfEvidence: level,
					Chemicals:       chemicals,
					Phenotypes:      phenotypes,
				}

				attrBytes, err := ffjson.Marshal(attr)
				if err != nil {
					log.Printf("PharmGKB: Error marshaling clinical %s: %v", clinID, err)
					continue
				}

				p.d.addProp3(clinID, clinSourceID, attrBytes)

				// Text search for variant
				p.d.addXref(variant, textLinkID, clinID, clinSource, true)

				// Cross-reference to dbSNP if variant is rsID
				// Note: Keep "rs" prefix - dbSNP entries are stored as "RS1799853" (uppercased with prefix)
				if strings.HasPrefix(variant, "rs") {
					if _, exists := config.Dataconf["dbsnp"]; exists {
						p.d.addXref(clinID, clinSourceID, variant, "dbsnp", false)
					}
				}

				// Cross-reference to gene
				if gene != "" {
					if _, exists := config.Dataconf["hgnc"]; exists {
						p.d.addXref(clinID, clinSourceID, gene, "hgnc", false)
					}
				}

				// Cross-reference to MeSH via phenotype mappings
				if _, meshExists := config.Dataconf["mesh"]; meshExists {
					for _, phenoName := range phenotypes {
						if mapping, exists := phenotypeMappings[strings.ToLower(phenoName)]; exists {
							for _, meshID := range mapping.meshIDs {
								p.d.addXref(clinID, clinSourceID, meshID, "mesh", false)
							}
						}
					}
				}

				if idLogFile != nil {
					idLogFile.WriteString(clinID + "\n")
				}

				entryCount++

				if testLimit > 0 && entryCount >= testLimit {
					log.Printf("PharmGKB: [TEST MODE] Reached clinical variant limit of %d", testLimit)
					break
				}
			}

			if err := scanner.Err(); err != nil {
				log.Printf("PharmGKB: Scanner error reading clinical variants: %v", err)
			}

			atomic.AddUint64(&p.d.totalParsedEntry, uint64(entryCount))
			log.Printf("PharmGKB: Total clinical variant entries saved: %d", entryCount)
			break
		}
	}

	// Signal completion for clinical dataset
	p.d.progChan <- &progressInfo{dataset: clinSource, done: true}
}

// loadDrugLabels reads drugLabels.zip and enriches chemical entries
// Columns: PharmGKB ID, Name, Source, Biomarker Flag, Testing Level, Has Prescribing Info,
// Has Dosing Info, Has Alternate Drug, Has Other Prescribing Guidance, Cancer Genome,
// Prescribing, Chemicals, Genes, Variants/Haplotypes, Latest History Date
func (p *pharmgkb) loadDrugLabels(chemicals map[string]*pharmgkbChemicalEntry) {
	labelFile := config.Dataconf[p.source]["drugLabelsFile"]
	if labelFile == "" {
		log.Printf("PharmGKB: No drugLabelsFile configured, skipping drug labels")
		return
	}

	zipReader, _, err := p.openZipFile(labelFile)
	if err != nil {
		log.Printf("PharmGKB: Warning - could not open drug labels: %v", err)
		return
	}

	for _, file := range zipReader.File {
		// Only process the main drugLabels.tsv, not byGene
		if file.Name == "drugLabels.tsv" {
			reader, err := file.Open()
			p.check(err, "opening TSV file in drug labels zip")
			defer reader.Close()

			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 1024*1024)

			var headerParsed bool
			var colMap map[string]int
			var labelCount int

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				fields := strings.Split(line, "\t")

				// Parse header
				if !headerParsed {
					colMap = make(map[string]int)
					for i, name := range fields {
						colMap[strings.TrimSpace(name)] = i
					}
					headerParsed = true
					continue
				}

				// Get chemicals field and match to our entries
				chemicalsField := safeFieldByCol(fields, colMap, "Chemicals")
				if chemicalsField == "" {
					continue
				}

				// Parse drug label data
				label := &pbuf.PharmgkbDrugLabel{
					LabelId:           safeFieldByCol(fields, colMap, "PharmGKB ID"),
					Name:              safeFieldByCol(fields, colMap, "Name"),
					Source:            safeFieldByCol(fields, colMap, "Source"),
					TestingLevel:      safeFieldByCol(fields, colMap, "Testing Level"),
					HasPrescribingInfo: safeFieldByCol(fields, colMap, "Has Prescribing Info") == "Prescribing Info",
					HasDosingInfo:     safeFieldByCol(fields, colMap, "Has Dosing Info") == "Dosing Info",
					HasAlternateDrug:  safeFieldByCol(fields, colMap, "Has Alternate Drug") == "Alternate Drug",
					Genes:             parseSemicolonList(safeFieldByCol(fields, colMap, "Genes")),
					Variants:          parseSemicolonList(safeFieldByCol(fields, colMap, "Variants/Haplotypes")),
				}

				// Match label to chemicals by name
				chemNames := parseSemicolonList(chemicalsField)
				for _, chemName := range chemNames {
					chemName = strings.TrimSpace(chemName)
					// Find chemical entry by name (case-insensitive search)
					for _, entry := range chemicals {
						if strings.EqualFold(entry.name, chemName) {
							// Limit to 20 labels per drug
							if len(entry.drugLabels) < 20 {
								entry.drugLabels = append(entry.drugLabels, label)
							}
							break
						}
					}
				}

				labelCount++
			}

			if err := scanner.Err(); err != nil {
				log.Printf("PharmGKB: Scanner error reading drug labels: %v", err)
			}
			log.Printf("PharmGKB: Processed %d drug labels", labelCount)
			break
		}
	}
}

// summaryAnnotation holds aggregated data from summaryAnnotations.zip for a variant
type summaryAnnotation struct {
	levelOfEvidence     string
	score               float64
	phenotypeCategories []string
	associatedDrugs     []string
	associatedPhenotypes []string
	pmidCount           int32
}

// phenotypeMapping holds cross-reference IDs for a phenotype name
type phenotypeMapping struct {
	meshIDs   []string
	snomedIDs []string
	umlsIDs   []string
}

// processVariants reads variants.zip and saves to pharmgkb_variant dataset
// Columns: Variant ID, Variant Name, Gene IDs, Gene Symbols, Location,
// Variant Annotation count, Clinical Annotation count, Level 1/2 Clinical Annotation count,
// Guideline Annotation count, Label Annotation count, Synonyms
func (p *pharmgkb) processVariants(testLimit int, summaryAnnotations map[string]*summaryAnnotation, phenotypeMappings map[string]*phenotypeMapping) {
	varSource := "pharmgkb_variant"
	if _, exists := config.Dataconf[varSource]; !exists {
		log.Printf("PharmGKB: Skipping variants - %s dataset not configured", varSource)
		return
	}

	varSourceID := config.Dataconf[varSource]["id"]

	varFile := config.Dataconf[p.source]["variantsFile"]
	if varFile == "" {
		log.Printf("PharmGKB: No variantsFile configured, skipping variants")
		return
	}

	zipReader, _, err := p.openZipFile(varFile)
	p.check(err, "opening variants zip file")

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, varSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".tsv") {
			reader, err := file.Open()
			p.check(err, "opening TSV file in variants zip")
			defer reader.Close()

			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 1024*1024)

			var headerParsed bool
			var colMap map[string]int
			var entryCount int

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				fields := strings.Split(line, "\t")

				// Parse header
				if !headerParsed {
					colMap = make(map[string]int)
					for i, name := range fields {
						colMap[strings.TrimSpace(name)] = i
					}
					headerParsed = true
					continue
				}

				variantID := safeFieldByCol(fields, colMap, "Variant ID")
				variantName := safeFieldByCol(fields, colMap, "Variant Name")

				if variantID == "" {
					continue
				}

				attr := &pbuf.PharmgkbVariantAttr{
					VariantId:                variantID,
					VariantName:              variantName,
					GeneIds:                  parseSemicolonList(safeFieldByCol(fields, colMap, "Gene IDs")),
					GeneSymbols:              parseSemicolonList(safeFieldByCol(fields, colMap, "Gene Symbols")),
					Location:                 safeFieldByCol(fields, colMap, "Location"),
					VariantAnnotationCount:   parseIntField(safeFieldByCol(fields, colMap, "Variant Annotation count")),
					ClinicalAnnotationCount:  parseIntField(safeFieldByCol(fields, colMap, "Clinical Annotation count")),
					GuidelineAnnotationCount: parseIntField(safeFieldByCol(fields, colMap, "Guideline Annotation count")),
					LabelAnnotationCount:     parseIntField(safeFieldByCol(fields, colMap, "Label Annotation count")),
					Synonyms:                 parseSynonyms(safeFieldByCol(fields, colMap, "Synonyms")),
				}

				// Enrich with summary annotation data
				if summary, exists := summaryAnnotations[variantName]; exists {
					attr.LevelOfEvidence = summary.levelOfEvidence
					attr.Score = summary.score
					attr.PhenotypeCategories = summary.phenotypeCategories
					attr.AssociatedDrugs = summary.associatedDrugs
					attr.AssociatedPhenotypes = summary.associatedPhenotypes
					attr.PmidCount = summary.pmidCount
				}

				attrBytes, err := ffjson.Marshal(attr)
				if err != nil {
					log.Printf("PharmGKB: Error marshaling variant %s: %v", variantID, err)
					continue
				}

				p.d.addProp3(variantID, varSourceID, attrBytes)

				// Text search for variant name (rsID)
				if variantName != "" {
					p.d.addXref(variantName, textLinkID, variantID, varSource, true)
				}

				// Cross-reference to dbSNP if variant name is rsID
				// Note: Keep "rs" prefix - dbSNP entries are stored as "RS1799853" (uppercased with prefix)
				if strings.HasPrefix(variantName, "rs") {
					if _, exists := config.Dataconf["dbsnp"]; exists {
						p.d.addXref(variantID, varSourceID, variantName, "dbsnp", false)
					}
				}

				// Cross-reference to genes
				for _, geneSymbol := range attr.GeneSymbols {
					if geneSymbol != "" {
						if _, exists := config.Dataconf["hgnc"]; exists {
							p.d.addXref(variantID, varSourceID, geneSymbol, "hgnc", false)
						}
					}
				}

				// Index synonyms as text search (rsIDs)
				for _, syn := range attr.Synonyms {
					if strings.HasPrefix(syn, "rs") && len(syn) < 20 {
						p.d.addXref(syn, textLinkID, variantID, varSource, true)
					}
				}

				// Cross-reference to MeSH via phenotype mappings (from associated phenotypes)
				if _, meshExists := config.Dataconf["mesh"]; meshExists {
					for _, phenoName := range attr.AssociatedPhenotypes {
						if mapping, exists := phenotypeMappings[strings.ToLower(phenoName)]; exists {
							for _, meshID := range mapping.meshIDs {
								p.d.addXref(variantID, varSourceID, meshID, "mesh", false)
							}
						}
					}
				}

				if idLogFile != nil {
					idLogFile.WriteString(variantID + "\n")
				}

				entryCount++

				if testLimit > 0 && entryCount >= testLimit {
					log.Printf("PharmGKB: [TEST MODE] Reached variant limit of %d", testLimit)
					break
				}
			}

			if err := scanner.Err(); err != nil {
				log.Printf("PharmGKB: Scanner error reading variants: %v", err)
			}

			atomic.AddUint64(&p.d.totalParsedEntry, uint64(entryCount))
			log.Printf("PharmGKB: Total variant entries saved: %d", entryCount)
			break
		}
	}

	// Signal completion for variant dataset
	p.d.progChan <- &progressInfo{dataset: varSource, done: true}
}

// parseSemicolonList splits a semicolon-separated list
func parseSemicolonList(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	parts := strings.Split(s, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// parseSynonyms parses the synonyms field which contains comma-separated identifiers
func parseSynonyms(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && len(part) < 50 {
			result = append(result, part)
		}
	}
	// Limit to 20 synonyms
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

// Helper functions

// safeFieldByCol returns field value by column name or empty string
func safeFieldByCol(fields []string, colMap map[string]int, colName string) string {
	if idx, exists := colMap[colName]; exists && idx < len(fields) {
		return strings.TrimSpace(fields[idx])
	}
	return ""
}

// parseQuotedList parses PharmGKB multi-value fields like: "\"value1\", \"value2\""
func parseQuotedList(s string) []string {
	if s == "" {
		return nil
	}

	var result []string
	// Split by comma and clean up quotes
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// parseIntField parses integer field with fallback to 0
func parseIntField(s string) int32 {
	if s == "" || s == "n/a" {
		return 0
	}
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(val)
}

// parseInt64Field parses int64 field with fallback to 0
func parseInt64Field(s string) int64 {
	if s == "" {
		return 0
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// loadSummaryAnnotations reads summaryAnnotations.zip and builds a map by variant name
// Columns: Summary Annotation ID, Variant/Haplotypes, Gene, Level of Evidence,
// Level Override, Level Modifiers, Score, Phenotype Category, PMID Count, Evidence Count,
// Drug(s), Phenotype(s), Latest History Date, URL, Specialty Population
func (p *pharmgkb) loadSummaryAnnotations() map[string]*summaryAnnotation {
	annotations := make(map[string]*summaryAnnotation)

	summaryFile := config.Dataconf[p.source]["summaryAnnotationsFile"]
	if summaryFile == "" {
		log.Printf("PharmGKB: No summaryAnnotationsFile configured, skipping summary annotations")
		return annotations
	}

	zipReader, _, err := p.openZipFile(summaryFile)
	if err != nil {
		log.Printf("PharmGKB: Warning - could not open summary annotations: %v", err)
		return annotations
	}

	for _, file := range zipReader.File {
		if file.Name == "summary_annotations.tsv" {
			reader, err := file.Open()
			p.check(err, "opening TSV file in summary annotations zip")
			defer reader.Close()

			scanner := bufio.NewScanner(reader)
			buf := make([]byte, 1024*1024)
			scanner.Buffer(buf, 1024*1024)

			var headerParsed bool
			var colMap map[string]int
			var annotCount int

			// Evidence levels priority (lower number = higher priority)
			levelPriority := map[string]int{
				"1A": 1, "1B": 2, "2A": 3, "2B": 4, "3": 5, "4": 6,
			}

			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				fields := strings.Split(line, "\t")

				// Parse header
				if !headerParsed {
					colMap = make(map[string]int)
					for i, name := range fields {
						colMap[strings.TrimSpace(name)] = i
					}
					headerParsed = true
					continue
				}

				variantName := safeFieldByCol(fields, colMap, "Variant/Haplotypes")
				if variantName == "" {
					continue
				}

				level := safeFieldByCol(fields, colMap, "Level of Evidence")
				score := parseFloatField(safeFieldByCol(fields, colMap, "Score"))
				phenoCategory := safeFieldByCol(fields, colMap, "Phenotype Category")
				pmidCount := parseIntField(safeFieldByCol(fields, colMap, "PMID Count"))
				drugs := parseQuotedList(safeFieldByCol(fields, colMap, "Drug(s)"))
				phenotypes := parseQuotedList(safeFieldByCol(fields, colMap, "Phenotype(s)"))

				// Get or create annotation entry
				if existing, exists := annotations[variantName]; exists {
					// Merge data - keep highest evidence level
					existingPriority := levelPriority[existing.levelOfEvidence]
					newPriority := levelPriority[level]
					if existingPriority == 0 || (newPriority > 0 && newPriority < existingPriority) {
						existing.levelOfEvidence = level
					}
					if score > existing.score {
						existing.score = score
					}
					existing.pmidCount += pmidCount

					// Aggregate unique values
					existing.phenotypeCategories = appendUnique(existing.phenotypeCategories, phenoCategory)
					existing.associatedDrugs = appendUniqueSlice(existing.associatedDrugs, drugs, 10)
					existing.associatedPhenotypes = appendUniqueSlice(existing.associatedPhenotypes, phenotypes, 10)
				} else {
					annotations[variantName] = &summaryAnnotation{
						levelOfEvidence:      level,
						score:                score,
						phenotypeCategories:  []string{phenoCategory},
						associatedDrugs:      limitSlice(drugs, 10),
						associatedPhenotypes: limitSlice(phenotypes, 10),
						pmidCount:            pmidCount,
					}
				}

				annotCount++
			}

			if err := scanner.Err(); err != nil {
				log.Printf("PharmGKB: Scanner error reading summary annotations: %v", err)
			}
			log.Printf("PharmGKB: Loaded %d summary annotations for %d variants", annotCount, len(annotations))
			break
		}
	}

	return annotations
}

// processGuidelines reads guidelineAnnotations.json.zip and saves to pharmgkb_guideline dataset
func (p *pharmgkb) processGuidelines(testLimit int) {
	guideSource := "pharmgkb_guideline"
	if _, exists := config.Dataconf[guideSource]; !exists {
		log.Printf("PharmGKB: Skipping guidelines - %s dataset not configured", guideSource)
		return
	}

	guideSourceID := config.Dataconf[guideSource]["id"]

	guideFile := config.Dataconf[p.source]["guidelineAnnotationsFile"]
	if guideFile == "" {
		log.Printf("PharmGKB: No guidelineAnnotationsFile configured, skipping guidelines")
		return
	}

	zipReader, _, err := p.openZipFile(guideFile)
	if err != nil {
		log.Printf("PharmGKB: Warning - could not open guideline annotations: %v", err)
		return
	}

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, guideSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var entryCount int

	for _, file := range zipReader.File {
		if !strings.HasSuffix(file.Name, ".json") || file.Name == "README.json" {
			continue
		}

		reader, err := file.Open()
		if err != nil {
			log.Printf("PharmGKB: Error opening %s: %v", file.Name, err)
			continue
		}

		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			log.Printf("PharmGKB: Error reading %s: %v", file.Name, err)
			continue
		}

		// Parse JSON structure: {"citations": [...], "guideline": {...}}
		var guidelineData struct {
			Guideline struct {
				ID                   string `json:"id"`
				Name                 string `json:"name"`
				Source               string `json:"source"`
				DosingInformation    bool   `json:"dosingInformation"`
				HasTestingInfo       bool   `json:"hasTestingInfo"`
				Recommendation       bool   `json:"recommendation"`
				AlternateDrugAvailable bool `json:"alternateDrugAvailable"`
				Pediatric            bool   `json:"pediatric"`
				CancerGenome         bool   `json:"cancerGenome"`
				SummaryMarkdown      struct {
					HTML string `json:"html"`
				} `json:"summaryMarkdown"`
				RelatedGenes []struct {
					ID     string `json:"id"`
					Symbol string `json:"symbol"`
				} `json:"relatedGenes"`
				RelatedChemicals []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"relatedChemicals"`
				Literature []struct {
					PMID string `json:"pmid"`
				} `json:"literature"`
			} `json:"guideline"`
		}

		if err := json.Unmarshal(data, &guidelineData); err != nil {
			log.Printf("PharmGKB: Error parsing guideline JSON %s: %v", file.Name, err)
			continue
		}

		g := guidelineData.Guideline
		if g.ID == "" {
			continue
		}

		// Extract gene symbols and IDs
		var geneSymbols, geneIDs []string
		for _, gene := range g.RelatedGenes {
			if gene.Symbol != "" {
				geneSymbols = append(geneSymbols, gene.Symbol)
			}
			if gene.ID != "" {
				geneIDs = append(geneIDs, gene.ID)
			}
		}

		// Extract chemical names and IDs
		var chemNames, chemIDs []string
		for _, chem := range g.RelatedChemicals {
			if chem.Name != "" {
				chemNames = append(chemNames, chem.Name)
			}
			if chem.ID != "" {
				chemIDs = append(chemIDs, chem.ID)
			}
		}

		// Extract PMIDs
		var pmids []string
		for _, lit := range g.Literature {
			if lit.PMID != "" && len(pmids) < 20 {
				pmids = append(pmids, lit.PMID)
			}
		}

		// Strip HTML from summary
		summary := stripHTML(g.SummaryMarkdown.HTML)
		if len(summary) > 500 {
			summary = summary[:500] + "..."
		}

		attr := &pbuf.PharmgkbGuidelineAttr{
			GuidelineId:          g.ID,
			Name:                 g.Name,
			Source:               g.Source,
			GeneSymbols:          geneSymbols,
			GeneIds:              geneIDs,
			ChemicalNames:        chemNames,
			ChemicalIds:          chemIDs,
			HasDosingInfo:        g.DosingInformation,
			HasTestingInfo:       g.HasTestingInfo,
			HasRecommendation:    g.Recommendation,
			AlternateDrugAvailable: g.AlternateDrugAvailable,
			IsPediatric:          g.Pediatric,
			IsCancerGenome:       g.CancerGenome,
			Summary:              summary,
			Pmids:                pmids,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("PharmGKB: Error marshaling guideline %s: %v", g.ID, err)
			continue
		}

		p.d.addProp3(g.ID, guideSourceID, attrBytes)

		// Text search for guideline name
		if g.Name != "" {
			p.d.addXref(g.Name, textLinkID, g.ID, guideSource, true)
		}

		// Cross-reference to genes
		for _, symbol := range geneSymbols {
			if _, exists := config.Dataconf["hgnc"]; exists {
				p.d.addXref(g.ID, guideSourceID, symbol, "hgnc", false)
			}
		}

		// Cross-reference to chemicals
		for _, chemID := range chemIDs {
			if _, exists := config.Dataconf["pharmgkb"]; exists {
				p.d.addXref(g.ID, guideSourceID, chemID, "pharmgkb", false)
			}
		}

		if idLogFile != nil {
			idLogFile.WriteString(g.ID + "\n")
		}

		entryCount++

		if testLimit > 0 && entryCount >= testLimit {
			log.Printf("PharmGKB: [TEST MODE] Reached guideline limit of %d", testLimit)
			break
		}
	}

	atomic.AddUint64(&p.d.totalParsedEntry, uint64(entryCount))
	log.Printf("PharmGKB: Total guideline entries saved: %d", entryCount)

	// Signal completion for guideline dataset
	p.d.progChan <- &progressInfo{dataset: guideSource, done: true}
}

// processPathways reads pathways.json.zip and saves to pharmgkb_pathway dataset
func (p *pharmgkb) processPathways(testLimit int) {
	pathSource := "pharmgkb_pathway"
	if _, exists := config.Dataconf[pathSource]; !exists {
		log.Printf("PharmGKB: Skipping pathways - %s dataset not configured", pathSource)
		return
	}

	pathSourceID := config.Dataconf[pathSource]["id"]

	pathFile := config.Dataconf[p.source]["pathwaysFile"]
	if pathFile == "" {
		log.Printf("PharmGKB: No pathwaysFile configured, skipping pathways")
		return
	}

	zipReader, _, err := p.openZipFile(pathFile)
	if err != nil {
		log.Printf("PharmGKB: Warning - could not open pathways: %v", err)
		return
	}

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, pathSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var entryCount int

	for _, file := range zipReader.File {
		if file.Name != "pathways.json" {
			continue
		}

		reader, err := file.Open()
		if err != nil {
			log.Printf("PharmGKB: Error opening %s: %v", file.Name, err)
			continue
		}

		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			log.Printf("PharmGKB: Error reading %s: %v", file.Name, err)
			continue
		}

		// Parse JSON array of pathways
		var pathways []struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Pharmacokinetic bool   `json:"pharmacokinetic"`
			Pharmacodynamic bool   `json:"pharmacodynamic"`
			Pediatric       bool   `json:"pediatric"`
			BiopaxLink      string `json:"biopaxLink"`
			ImageLink       string `json:"imageLink"`
			Summary         struct {
				HTML string `json:"html"`
			} `json:"summary"`
			Description struct {
				HTML string `json:"html"`
			} `json:"description"`
			Genes []struct {
				ID     string `json:"id"`
				Symbol string `json:"symbol"`
			} `json:"genes"`
			Chemicals []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"chemicals"`
			Diseases []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"diseases"`
		}

		if err := json.Unmarshal(data, &pathways); err != nil {
			log.Printf("PharmGKB: Error parsing pathways JSON: %v", err)
			continue
		}

		for _, pw := range pathways {
			if pw.ID == "" {
				continue
			}

			// Extract gene symbols and IDs
			var geneSymbols, geneIDs []string
			for _, gene := range pw.Genes {
				if gene.Symbol != "" && len(geneSymbols) < 50 {
					geneSymbols = append(geneSymbols, gene.Symbol)
				}
				if gene.ID != "" && len(geneIDs) < 50 {
					geneIDs = append(geneIDs, gene.ID)
				}
			}

			// Extract chemical names and IDs
			var chemNames, chemIDs []string
			for _, chem := range pw.Chemicals {
				if chem.Name != "" && len(chemNames) < 50 {
					chemNames = append(chemNames, chem.Name)
				}
				if chem.ID != "" && len(chemIDs) < 50 {
					chemIDs = append(chemIDs, chem.ID)
				}
			}

			// Extract disease names and IDs
			var diseaseNames, diseaseIDs []string
			for _, disease := range pw.Diseases {
				if disease.Name != "" && len(diseaseNames) < 20 {
					diseaseNames = append(diseaseNames, disease.Name)
				}
				if disease.ID != "" && len(diseaseIDs) < 20 {
					diseaseIDs = append(diseaseIDs, disease.ID)
				}
			}

			// Strip HTML from summary and description
			summary := stripHTML(pw.Summary.HTML)
			if len(summary) > 300 {
				summary = summary[:300] + "..."
			}
			description := stripHTML(pw.Description.HTML)
			if len(description) > 500 {
				description = description[:500] + "..."
			}

			attr := &pbuf.PharmgkbPathwayAttr{
				PathwayId:        pw.ID,
				Name:             pw.Name,
				IsPharmacokinetic: pw.Pharmacokinetic,
				IsPharmacodynamic: pw.Pharmacodynamic,
				IsPediatric:      pw.Pediatric,
				GeneSymbols:      geneSymbols,
				GeneIds:          geneIDs,
				ChemicalNames:    chemNames,
				ChemicalIds:      chemIDs,
				DiseaseNames:     diseaseNames,
				DiseaseIds:       diseaseIDs,
				Summary:          summary,
				Description:      description,
				BiopaxLink:       pw.BiopaxLink,
				ImageLink:        pw.ImageLink,
			}

			attrBytes, err := ffjson.Marshal(attr)
			if err != nil {
				log.Printf("PharmGKB: Error marshaling pathway %s: %v", pw.ID, err)
				continue
			}

			p.d.addProp3(pw.ID, pathSourceID, attrBytes)

			// Text search for pathway name
			if pw.Name != "" {
				p.d.addXref(pw.Name, textLinkID, pw.ID, pathSource, true)
			}

			// Cross-reference to genes
			for _, symbol := range geneSymbols {
				if _, exists := config.Dataconf["hgnc"]; exists {
					p.d.addXref(pw.ID, pathSourceID, symbol, "hgnc", false)
				}
			}

			// Cross-reference to chemicals
			for _, chemID := range chemIDs {
				if _, exists := config.Dataconf["pharmgkb"]; exists {
					p.d.addXref(pw.ID, pathSourceID, chemID, "pharmgkb", false)
				}
			}

			if idLogFile != nil {
				idLogFile.WriteString(pw.ID + "\n")
			}

			entryCount++

			if testLimit > 0 && entryCount >= testLimit {
				log.Printf("PharmGKB: [TEST MODE] Reached pathway limit of %d", testLimit)
				break
			}
		}
		break
	}

	atomic.AddUint64(&p.d.totalParsedEntry, uint64(entryCount))
	log.Printf("PharmGKB: Total pathway entries saved: %d", entryCount)

	// Signal completion for pathway dataset
	p.d.progChan <- &progressInfo{dataset: pathSource, done: true}
}

// loadPhenotypeMappings reads phenotypes.zip and builds a map of phenotype name -> external IDs
// This is used to add cross-references from clinical and variant entries to MeSH
// Columns: PharmGKB Accession Id, Name, Alternate Names, Cross-references, External Vocabulary
func (p *pharmgkb) loadPhenotypeMappings() map[string]*phenotypeMapping {
	mappings := make(map[string]*phenotypeMapping)

	phenoFile := config.Dataconf[p.source]["phenotypesFile"]
	if phenoFile == "" {
		log.Printf("PharmGKB: No phenotypesFile configured, skipping phenotype mappings")
		return mappings
	}

	zipReader, _, err := p.openZipFile(phenoFile)
	if err != nil {
		log.Printf("PharmGKB: Warning - could not open phenotypes: %v", err)
		return mappings
	}

	for _, file := range zipReader.File {
		if file.Name != "phenotypes.tsv" {
			continue
		}

		reader, err := file.Open()
		p.check(err, "opening TSV file in phenotypes zip")
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, 1024*1024)

		var headerParsed bool
		var colMap map[string]int

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			fields := strings.Split(line, "\t")

			// Parse header
			if !headerParsed {
				colMap = make(map[string]int)
				for i, name := range fields {
					colMap[strings.TrimSpace(name)] = i
				}
				headerParsed = true
				continue
			}

			name := safeFieldByCol(fields, colMap, "Name")
			if name == "" {
				continue
			}

			altNames := parseQuotedList(safeFieldByCol(fields, colMap, "Alternate Names"))

			// Parse cross-references and external vocabulary for ontology IDs
			crossRefs := safeFieldByCol(fields, colMap, "Cross-references")
			extVocab := safeFieldByCol(fields, colMap, "External Vocabulary")

			// Combine for parsing
			allRefs := crossRefs + ", " + extVocab

			// Parse MeSH, SNOMED, UMLS IDs
			meshIDs := extractIDs(allRefs, "MeSH:")
			snomedIDs := extractIDs(allRefs, "SnoMedCT:")
			umlsIDs := extractIDs(allRefs, "UMLS:")

			// Only create mapping if we have external IDs
			if len(meshIDs) > 0 || len(snomedIDs) > 0 || len(umlsIDs) > 0 {
				mapping := &phenotypeMapping{
					meshIDs:   limitSlice(meshIDs, 5),
					snomedIDs: limitSlice(snomedIDs, 5),
					umlsIDs:   limitSlice(umlsIDs, 5),
				}

				// Index by primary name (lowercase for case-insensitive matching)
				mappings[strings.ToLower(name)] = mapping

				// Also index by alternate names
				for _, altName := range altNames {
					if altName != "" {
						mappings[strings.ToLower(altName)] = mapping
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			log.Printf("PharmGKB: Scanner error reading phenotypes: %v", err)
		}
		break
	}

	return mappings
}

// Helper functions for new processing

// parseFloatField parses a float field with fallback to 0
func parseFloatField(s string) float64 {
	if s == "" {
		return 0
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

// appendUnique appends a value to a slice if not already present
func appendUnique(slice []string, value string) []string {
	if value == "" {
		return slice
	}
	for _, v := range slice {
		if v == value {
			return slice
		}
	}
	return append(slice, value)
}

// appendUniqueSlice appends values from source to dest, keeping unique values up to limit
func appendUniqueSlice(dest, source []string, limit int) []string {
	seen := make(map[string]bool)
	for _, v := range dest {
		seen[v] = true
	}
	for _, v := range source {
		if v != "" && !seen[v] && len(dest) < limit {
			dest = append(dest, v)
			seen[v] = true
		}
	}
	return dest
}

// limitSlice returns a slice with at most n elements
func limitSlice(slice []string, n int) []string {
	if len(slice) <= n {
		return slice
	}
	return slice[:n]
}

// stripHTML removes HTML tags from a string
func stripHTML(s string) string {
	if s == "" {
		return ""
	}
	// Simple regex to remove HTML tags
	re := regexp.MustCompile(`<[^>]*>`)
	result := re.ReplaceAllString(s, "")
	// Clean up whitespace
	result = strings.Join(strings.Fields(result), " ")
	return strings.TrimSpace(result)
}

// extractIDs extracts IDs with a specific prefix from a cross-reference string
// Example: "MeSH:D015746(Abdominal Pain)" -> ["D015746"]
func extractIDs(s, prefix string) []string {
	if s == "" {
		return nil
	}
	var result []string
	// Find all occurrences of prefix followed by ID
	re := regexp.MustCompile(regexp.QuoteMeta(prefix) + `([A-Za-z0-9_-]+)`)
	matches := re.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		if len(match) >= 2 && match[1] != "" {
			result = append(result, match[1])
		}
	}
	return result
}
