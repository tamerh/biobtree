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

type chebi struct {
	source  string
	d       *DataUpdate
	ftpHost string
	ftpPath string
}

// check provides context-aware error checking for chebi processor
func (c *chebi) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// Data structures for merging multiple ChEBI files
type CompoundData struct {
	ID         string
	Name       string
	Definition string
	Stars      int32
	Source     string
	Status     int32
}

type NameData struct {
	Synonyms    []string
	IupacNames  []string
	BrandNames  []string
	InnNames    []string
}

type ChemicalData struct {
	Formula  string
	Mass     float64
	MonoMass float64
	Charge   int32
}

type StructureData struct {
	Smiles   string
	Inchi    string
	InchiKey string
}

type RelationshipData struct {
	Roles   map[string][]string // chebiID -> role IDs
	Parents map[string][]string // chebiID -> parent IDs
	Types   map[int]string      // relation_type_id -> code
}

// loadCompounds loads compound names and definitions from compounds.tsv.gz
func (c *chebi) loadCompounds() map[string]*CompoundData {
	compounds := make(map[string]*CompoundData)

	chebiPath := config.Dataconf[c.source]["path"]
	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"compounds.tsv.gz")
	c.check(err, "opening compounds.tsv.gz")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'

	lineNum := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		c.check(err, "reading compounds.tsv")

		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		// Format: id, name, status_id, source, parent_id, merge_type,
		//         chebi_accession, definition, ascii_name, stars, modified_on, release_date
		if len(record) < 10 {
			continue
		}

		// Only process active compounds
		statusID, _ := strconv.Atoi(record[2])
		if statusID != 1 {
			continue // Skip deprecated/obsolete
		}

		chebiID := record[6] // "CHEBI:xxxxx"
		if chebiID == "" || chebiID == "null" {
			continue
		}

		stars, _ := strconv.ParseInt(record[9], 10, 32)

		compounds[chebiID] = &CompoundData{
			ID:         chebiID,
			Name:       record[1],
			Definition: record[7],
			Stars:      int32(stars),
			Source:     record[3],
			Status:     int32(statusID),
		}
	}

	log.Printf("ChEBI: Loaded %d compounds", len(compounds))
	return compounds
}

// buildIDMapping creates a map from internal compound ID to CHEBI ID
func (c *chebi) buildIDMapping(compounds map[string]*CompoundData) map[string]string {
	idMap := make(map[string]string)

	chebiPath := config.Dataconf[c.source]["path"]
	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"compounds.tsv.gz")
	c.check(err, "opening compounds.tsv.gz for ID mapping")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'

	lineNum := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		c.check(err, "reading compounds.tsv for ID mapping")

		lineNum++
		if lineNum == 1 {
			continue
		}

		if len(record) < 7 {
			continue
		}

		internalID := record[0] // Internal numeric ID
		chebiID := record[6]    // "CHEBI:xxxxx"

		if chebiID != "" && chebiID != "null" {
			idMap[internalID] = chebiID
		}
	}

	log.Printf("ChEBI: Built ID mapping for %d compounds", len(idMap))
	return idMap
}

// loadNames loads synonyms and IUPAC names from names.tsv.gz
func (c *chebi) loadNames() map[string]*NameData {
	nameMap := make(map[string]*NameData)

	chebiPath := config.Dataconf[c.source]["path"]
	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"names.tsv.gz")
	c.check(err, "opening names.tsv.gz")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'

	lineNum := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		c.check(err, "reading names.tsv")

		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		// Format: id, compound_id, name, type, status_id, adapted, language_code, ascii_name
		if len(record) < 5 {
			continue
		}

		// Only active names
		statusID, _ := strconv.Atoi(record[4])
		if statusID != 1 {
			continue
		}

		compoundID := record[1]
		nameText := record[2]
		nameType := record[3]

		// Initialize if needed
		if nameMap[compoundID] == nil {
			nameMap[compoundID] = &NameData{
				Synonyms:   []string{},
				IupacNames: []string{},
				BrandNames: []string{},
				InnNames:   []string{},
			}
		}

		// Categorize by type
		switch nameType {
		case "SYNONYM":
			nameMap[compoundID].Synonyms = append(nameMap[compoundID].Synonyms, nameText)
		case "IUPAC NAME":
			nameMap[compoundID].IupacNames = append(nameMap[compoundID].IupacNames, nameText)
		case "BRAND NAME":
			nameMap[compoundID].BrandNames = append(nameMap[compoundID].BrandNames, nameText)
		case "INN":
			nameMap[compoundID].InnNames = append(nameMap[compoundID].InnNames, nameText)
		}
	}

	log.Printf("ChEBI: Loaded names for %d compounds", len(nameMap))
	return nameMap
}

// loadChemicalData loads formulas and molecular properties from chemical_data.tsv.gz
func (c *chebi) loadChemicalData() map[string]*ChemicalData {
	chemMap := make(map[string]*ChemicalData)

	chebiPath := config.Dataconf[c.source]["path"]
	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"chemical_data.tsv.gz")
	c.check(err, "opening chemical_data.tsv.gz")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'

	lineNum := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		c.check(err, "reading chemical_data.tsv")

		lineNum++
		if lineNum == 1 {
			continue
		}

		// Format: id, compound_id, formula, charge, mass, monoisotopic_mass,
		//         status_id, structure_id, is_autogenerated
		if len(record) < 7 {
			continue
		}

		statusID, _ := strconv.Atoi(record[6])
		if statusID != 1 {
			continue
		}

		compoundID := record[1]

		mass, _ := strconv.ParseFloat(record[4], 64)
		monoMass, _ := strconv.ParseFloat(record[5], 64)
		charge, _ := strconv.ParseInt(record[3], 10, 32)

		chemMap[compoundID] = &ChemicalData{
			Formula:  record[2],
			Mass:     mass,
			MonoMass: monoMass,
			Charge:   int32(charge),
		}
	}

	log.Printf("ChEBI: Loaded chemical data for %d compounds", len(chemMap))
	return chemMap
}

// loadStructures loads SMILES, InChI, and InChIKey from structures.tsv.gz
func (c *chebi) loadStructures() map[string]*StructureData {
	structMap := make(map[string]*StructureData)

	chebiPath := config.Dataconf[c.source]["path"]
	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"structures.tsv.gz")
	c.check(err, "opening structures.tsv.gz")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.FieldsPerRecord = -1 // Variable fields (molfile can be large)

	lineNum := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Log but continue (some lines might have formatting issues)
			log.Printf("Warning: Error reading structures.tsv line %d: %v", lineNum, err)
			continue
		}

		lineNum++
		if lineNum == 1 {
			continue
		}

		// Format: id, compound_id, status_id, molfile, smiles, standard_inchi,
		//         standard_inchi_key, dimension, default_structure
		if len(record) < 7 {
			continue
		}

		statusID, _ := strconv.Atoi(record[2])
		if statusID != 1 {
			continue
		}

		compoundID := record[1]

		// We don't store molfile (too large), just SMILES and InChI
		structMap[compoundID] = &StructureData{
			Smiles:   record[4],
			Inchi:    record[5],
			InchiKey: record[6],
		}

		// Progress reporting for large file
		if lineNum%50000 == 0 {
			log.Printf("ChEBI: Processed %d structure records...", lineNum)
		}
	}

	log.Printf("ChEBI: Loaded structures for %d compounds", len(structMap))
	return structMap
}

// loadRelations loads ontology relationships from relation.tsv.gz
func (c *chebi) loadRelations() *RelationshipData {
	rel := &RelationshipData{
		Roles:   make(map[string][]string),
		Parents: make(map[string][]string),
		Types:   make(map[int]string),
	}

	chebiPath := config.Dataconf[c.source]["path"]
	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"relation.tsv.gz")
	c.check(err, "opening relation.tsv.gz")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'

	lineNum := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		c.check(err, "reading relation.tsv")

		lineNum++
		if lineNum == 1 {
			continue
		}

		// Format: id, relation_type_id, init_id, final_id, status_id,
		//         evidence_accession, evidence_source_id
		if len(record) < 5 {
			continue
		}

		statusID, _ := strconv.Atoi(record[4])
		if statusID != 1 {
			continue
		}

		typeID, _ := strconv.Atoi(record[1])
		initID := record[2]  // Source compound ID
		finalID := record[3] // Target compound ID

		// Relation types:
		// 4 = has_role
		// 5 = is_a (hierarchy)

		if typeID == 4 { // has_role
			rel.Roles[initID] = append(rel.Roles[initID], finalID)
		} else if typeID == 5 { // is_a (parent)
			rel.Parents[initID] = append(rel.Parents[initID], finalID)
		}
	}

	log.Printf("ChEBI: Loaded %d role relationships, %d parent relationships",
		len(rel.Roles), len(rel.Parents))
	return rel
}

// loadSourceMapping loads source table to map source_id to database names
func (c *chebi) loadSourceMapping() map[string]string {
	sourceMap := make(map[string]string)

	chebiPath := config.Dataconf[c.source]["path"]
	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"source.tsv.gz")
	c.check(err, "opening source.tsv.gz")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'

	lineNum := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		c.check(err, "reading source.tsv")

		lineNum++
		if lineNum == 1 {
			continue
		}

		// Format: id, name, url, prefix, description
		if len(record) < 4 {
			continue
		}

		sourceID := record[0]
		prefix := record[3] // e.g., "chembl", "hmdb", "kegg.compound"

		// Map common prefixes to biobtree dataset names
		dbName := c.mapPrefixToDataset(prefix)
		if dbName != "" {
			sourceMap[sourceID] = dbName
		}
	}

	log.Printf("ChEBI: Loaded %d source mappings", len(sourceMap))
	return sourceMap
}

// mapPrefixToDataset maps ChEBI source prefix to biobtree dataset name
// Note: ChEMBL is NOT included because ChEBI's database_accession.tsv has no valid
// ChEMBL xrefs (only 1 erroneous CAS number entry). Use ChEMBL→ChEBI direction instead.
func (c *chebi) mapPrefixToDataset(prefix string) string {
	mapping := map[string]string{
		"hmdb":          "hmdb",
		"kegg.compound": "kegg",
		"kegg.drug":     "kegg",
		"kegg.glycan":   "kegg",
		"reactome":      "reactome",
		"uniprot":       "uniprot",
		"uniprot_ft":    "uniprot",
		"go":            "go",
		"pdb":           "pdb",
		"pubmed":        "pubmed",
	}

	return mapping[prefix]
}

func (c *chebi) update() {
	defer c.d.wg.Done()

	// Get ChEBI FTP host and path from config
	// e.g., "ftp://ftp.ebi.ac.uk/pub/databases/chebi/Flat_file_tab_delimited/"
	fullURL := config.Dataconf[c.source]["path"]
	ftpHost, ftpPath, err := parseFTPURL(fullURL)
	if err != nil {
		panic("Invalid ChEBI FTP path in config: " + err.Error())
	}
	c.ftpHost = ftpHost
	c.ftpPath = ftpPath

	fr := config.Dataconf[c.source]["id"]

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	log.Printf("ChEBI: Starting enhanced data load...")

	// Step 1: Load all data files
	compounds := c.loadCompounds()
	idMap := c.buildIDMapping(compounds)
	names := c.loadNames()
	chemData := c.loadChemicalData()
	structures := c.loadStructures()
	relations := c.loadRelations()
	sourceMap := c.loadSourceMapping()

	log.Printf("ChEBI: Data load complete, merging...")

	// Step 2: Merge data and create entries
	var processedCount uint64
	var previous int64
	totalProcessed := 0

	// Sort ChEBI IDs for deterministic test mode
	sortedIDs := make([]string, 0, len(compounds))
	for chebiID := range compounds {
		sortedIDs = append(sortedIDs, chebiID)
	}
	sort.Strings(sortedIDs)

	for _, chebiID := range sortedIDs {
		compound := compounds[chebiID]
		// Get internal ID for lookups in other tables
		var internalID string
		for intID, cID := range idMap {
			if cID == chebiID {
				internalID = intID
				break
			}
		}

		// Build complete attribute
		attr := pbuf.ChebiAttr{
			Name:       compound.Name,
			Definition: compound.Definition,
			StarRating: compound.Stars,
			Source:     compound.Source,
			StatusId:   compound.Status,
		}

		// Add names if available
		if internalID != "" {
			if nameData := names[internalID]; nameData != nil {
				attr.Synonyms = nameData.Synonyms
				attr.IupacNames = nameData.IupacNames
				attr.BrandNames = nameData.BrandNames
				attr.InnNames = nameData.InnNames
			}

			// Add chemical data if available
			if chem := chemData[internalID]; chem != nil {
				attr.Formula = chem.Formula
				attr.AverageMass = chem.Mass
				attr.MonoisotopicMass = chem.MonoMass
				attr.Charge = chem.Charge
			}

			// Add structures if available
			if structData := structures[internalID]; structData != nil {
				attr.Smiles = structData.Smiles
				attr.Inchi = structData.Inchi
				attr.InchiKey = structData.InchiKey
			}

			// Add relationships if available
			if roles := relations.Roles[internalID]; roles != nil {
				// Convert internal IDs to CHEBI IDs
				for _, roleID := range roles {
					if chebiRoleID := idMap[roleID]; chebiRoleID != "" {
						attr.Roles = append(attr.Roles, chebiRoleID)
					}
				}
			}
			if parents := relations.Parents[internalID]; parents != nil {
				for _, parentID := range parents {
					if chebiParentID := idMap[parentID]; chebiParentID != "" {
						attr.ParentIds = append(attr.ParentIds, chebiParentID)
					}
				}
			}
		}

		// Marshal and store attributes
		b, err := ffjson.Marshal(&attr)
		c.check(err, "marshaling ChEBI attributes")
		c.d.addProp3(chebiID, fr, b)

		// Text search: primary name
		if compound.Name != "" {
			c.d.addXref(compound.Name, textLinkID, chebiID, "chebi", true)
		}

		// Text search: all synonyms
		if internalID != "" && names[internalID] != nil {
			for _, syn := range names[internalID].Synonyms {
				if syn != "" {
					c.d.addXref(syn, textLinkID, chebiID, "chebi", true)
				}
			}

			// Text search: IUPAC names
			for _, iupac := range names[internalID].IupacNames {
				if iupac != "" {
					c.d.addXref(iupac, textLinkID, chebiID, "chebi", true)
				}
			}

			// Text search: brand names
			for _, brand := range names[internalID].BrandNames {
				if brand != "" {
					c.d.addXref(brand, textLinkID, chebiID, "chebi", true)
				}
			}
		}

		// Text search: InChI Key (structure search!)
		if internalID != "" && structures[internalID] != nil && structures[internalID].InchiKey != "" {
			c.d.addXref(structures[internalID].InchiKey, textLinkID, chebiID, "chebi", true)
		}

		// Text search: formula
		if internalID != "" && chemData[internalID] != nil && chemData[internalID].Formula != "" {
			c.d.addXref(chemData[internalID].Formula, textLinkID, chebiID, "chebi", true)
		}

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, chebiID)
		}

		processedCount++
		totalProcessed++

		// Test mode limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, totalProcessed) {
			log.Printf("ChEBI: Reached test limit of %d compounds", testLimit)
			break
		}

		// Progress reporting
		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source}
		}
	}

	// Step 3: Process cross-references (only for processed compounds)
	processedIDs := make(map[string]bool)
	for chebiID := range compounds {
		processedIDs[chebiID] = true
		if len(processedIDs) >= int(processedCount) {
			break
		}
	}
	c.processCrossReferences(fr, sourceMap, idMap, processedIDs, idLogFile, testLimit, int(processedCount))

	// Done
	atomic.AddUint64(&c.d.totalParsedEntry, processedCount)
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}

	log.Printf("ChEBI: Processing complete - %d compounds with full data", processedCount)
}

// processCrossReferences creates cross-references from ChEBI to other databases
func (c *chebi) processCrossReferences(fr string, sourceMap map[string]string, idMap map[string]string, processedIDs map[string]bool, idLogFile *os.File, testLimit int, alreadyProcessed int) {
	log.Printf("ChEBI: Processing cross-references...")

	chebiPath := config.Dataconf[c.source]["path"]

	br, _, ftpFile, client, localFile, _, err := getDataReaderNew(
		c.source, c.ftpHost, c.ftpPath, chebiPath+"database_accession.tsv.gz")
	c.check(err, "opening database_accession.tsv.gz")
	defer func() {
		if ftpFile != nil {
			ftpFile.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
		if client != nil {
			client.Quit()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'

	var xrefCount int
	lineNum := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		c.check(err, "reading database_accession.tsv")

		lineNum++
		// Skip header
		if lineNum == 1 {
			continue
		}

		// Format: id, compound_id, accession_number, type, status_id, source_id
		if len(record) < 6 {
			continue
		}

		// Only active xrefs
		statusID, _ := strconv.Atoi(record[4])
		if statusID != 1 {
			continue
		}

		internalID := record[1]   // Internal compound ID
		accessionNum := record[2] // External database ID
		sourceID := record[5]     // Links to source.tsv

		// Convert internal ID to CHEBI ID
		chebiID := idMap[internalID]
		if chebiID == "" {
			continue
		}

		// Only create xrefs for ChEBI IDs we actually processed
		if !processedIDs[chebiID] {
			continue
		}

		// Look up actual database name from source_id
		targetDB := sourceMap[sourceID]
		if targetDB == "" {
			// Not a database we care about
			continue
		}

		// Check if target database is configured in biobtree
		if _, ok := config.Dataconf[targetDB]; !ok {
			continue
		}

		// Validate accession format for specific databases
		if targetDB == "hmdb" && !strings.HasPrefix(accessionNum, "HMDB") {
			log.Printf("ChEBI: Skipping invalid HMDB ID '%s' for %s", accessionNum, chebiID)
			continue // Skip non-HMDB IDs (e.g., CAS numbers)
		}

		// Create cross-reference: CHEBI → Target Database
		c.d.addXref(chebiID, fr, accessionNum, targetDB, false)
		xrefCount++
	}

	log.Printf("ChEBI: Created %d cross-references to external databases", xrefCount)
}
