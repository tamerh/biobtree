package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pquerna/ffjson/ffjson"
)

type reactome struct {
	source   string
	sourceID string
	d        *DataUpdate
}

// Species name to taxonomy ID mapping (all 16 Reactome species)
var speciesTaxMap = map[string]int32{
	"Homo sapiens":               9606,
	"Mus musculus":               10090,
	"Rattus norvegicus":          10116,
	"Bos taurus":                 9913,
	"Sus scrofa":                 9823,
	"Canis familiaris":           9615,
	"Gallus gallus":              9031,
	"Xenopus tropicalis":         8364,
	"Danio rerio":                7955,
	"Drosophila melanogaster":    7227,
	"Caenorhabditis elegans":     6239,
	"Saccharomyces cerevisiae":   4932,
	"Schizosaccharomyces pombe":  4896,
	"Dictyostelium discoideum":   44689,
	"Plasmodium falciparum":      5833,
	"Mycobacterium tuberculosis": 1773,
}

// loadGoTermMappings loads GO Biological Process terms from Pathways2GoTerms_human.txt
// Maps numeric pathway ID to GO term (e.g., "73843" -> "GO:0006015")
// These mappings apply to all species since pathways share numeric IDs across species
func (r *reactome) loadGoTermMappings() (map[string]string, error) {
	filePath := config.Dataconf[r.source]["downloadUrlGoTerms"]
	if filePath == "" {
		return make(map[string]string), nil // No GO terms configured, return empty map
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GO terms file: %v", err)
	}
	defer closeReactomeReaders(gz, ftpFile, client, localFile)

	goTermMap := make(map[string]string)
	scanner := bufio.NewScanner(br)

	// Skip header line
	if scanner.Scan() {
		// Header: Identifier	Name	GO_Term
	}

	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		pathwayID := fields[0] // e.g., "R-HSA-73843"
		goTerm := fields[2]    // e.g., "GO:0006015"

		// Extract numeric part: "R-HSA-73843" -> "73843"
		parts := strings.Split(pathwayID, "-")
		if len(parts) >= 3 {
			numericID := parts[2]
			goTermMap[numericID] = goTerm
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading GO terms file: %v", err)
	}

	return goTermMap, nil
}

// loadDiseasePathways loads disease pathway IDs from HumanDiseasePathways.txt
// Returns a set of numeric pathway IDs that are disease-related
// These IDs apply to all species (e.g., 73843 marks R-HSA-73843, R-BTA-73843, etc. as disease pathways)
func (r *reactome) loadDiseasePathways() (map[string]bool, error) {
	filePath := config.Dataconf[r.source]["downloadUrlDiseasePathways"]
	if filePath == "" {
		return make(map[string]bool), nil // No disease pathways configured, return empty set
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open disease pathways file: %v", err)
	}
	defer closeReactomeReaders(gz, ftpFile, client, localFile)

	diseasePathways := make(map[string]bool)
	scanner := bufio.NewScanner(br)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Format: pathway_id\tpathway_name
		// Example: R-HSA-1226099	Signaling by FGFR in disease
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}

		pathwayID := fields[0] // e.g., "R-HSA-1226099"

		// Extract numeric part: "R-HSA-1226099" -> "1226099"
		parts := strings.Split(pathwayID, "-")
		if len(parts) >= 3 {
			numericID := parts[2]
			diseasePathways[numericID] = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading disease pathways file: %v", err)
	}

	return diseasePathways, nil
}

// Main update entry point
func (r *reactome) update() {
	defer r.d.wg.Done()

	r.sourceID = config.Dataconf[r.source]["id"]

	// Test mode support
	testLimit := config.GetTestLimit(r.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, r.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	fmt.Println("Starting Reactome pathway integration...")

	// Step 0: Load GO term mappings (applied to all species)
	goTermMap, err := r.loadGoTermMappings()
	if err != nil {
		log.Printf("Warning: Could not load GO term mappings: %v", err)
		goTermMap = make(map[string]string) // Continue without GO terms
	} else if len(goTermMap) > 0 {
		fmt.Printf("  Loaded GO term mappings for %d pathways\n", len(goTermMap))
	}

	// Step 0b: Load disease pathway set (applied to all species)
	diseasePathways, err := r.loadDiseasePathways()
	if err != nil {
		log.Printf("Warning: Could not load disease pathways: %v", err)
		diseasePathways = make(map[string]bool) // Continue without disease annotations
	} else if len(diseasePathways) > 0 {
		fmt.Printf("  Loaded %d disease pathways\n", len(diseasePathways))
	}

	// Step 1: Load pathways (creates pathway entries with taxonomy, GO xrefs, and disease annotations)
	pathwayMap, err := r.loadPathways(idLogFile, testLimit, goTermMap, diseasePathways)
	if err != nil {
		log.Fatalf("Error loading Reactome pathways: %v", err)
	}
	fmt.Printf("  Loaded %d pathways\n", len(pathwayMap))

	// Step 2: Process hierarchy (parent-child relationships)
	hierarchyCount, err := r.processHierarchy(pathwayMap)
	if err != nil {
		log.Printf("Warning: Error processing pathway hierarchy: %v", err)
	} else {
		fmt.Printf("  Processed %d hierarchy relationships\n", hierarchyCount)
	}

	// Step 3: Process UniProt mappings (with evidence codes)
	uniprotMappings, err := r.processUniProtMappings(pathwayMap, testLimit)
	if err != nil {
		log.Printf("Warning: Error processing UniProt mappings: %v", err)
	} else {
		fmt.Printf("  Processed %d UniProt-pathway mappings\n", uniprotMappings)
	}

	// Step 4: Process ChEBI mappings (with evidence codes)
	chebiMappings, err := r.processChEBIMappings(pathwayMap, testLimit)
	if err != nil {
		log.Printf("Warning: Error processing ChEBI mappings: %v", err)
	} else {
		fmt.Printf("  Processed %d ChEBI-pathway mappings\n", chebiMappings)
	}

	// Step 5: Process Ensembl mappings (gene-level pathway mappings)
	ensemblMappings, err := r.processEnsemblMappings(pathwayMap, testLimit)
	if err != nil {
		log.Printf("Warning: Error processing Ensembl mappings: %v", err)
	} else {
		fmt.Printf("  Processed %d Ensembl-pathway mappings\n", ensemblMappings)
	}

	totalPathways := uint64(len(pathwayMap))
	fmt.Printf("Reactome processing complete: %d pathways, %d UniProt mappings, %d ChEBI mappings, %d Ensembl mappings\n",
		totalPathways, uniprotMappings, chebiMappings, ensemblMappings)

	atomic.AddUint64(&r.d.totalParsedEntry, totalPathways)
	r.d.progChan <- &progressInfo{dataset: r.source, done: true}
}

// Load pathways from ReactomePathways.txt (streaming)
func (r *reactome) loadPathways(idLogFile *os.File, testLimit int, goTermMap map[string]string, diseasePathways map[string]bool) (map[string]bool, error) {
	var filePath string

	// Check if using local test files or remote download
	if _, ok := config.Dataconf[r.source]["useLocalFile"]; ok && config.Dataconf[r.source]["useLocalFile"] == "yes" {
		basePath := config.Dataconf[r.source]["path"]
		filePath = basePath + "ReactomePathways_test.txt"
	} else {
		filePath = config.Dataconf[r.source]["downloadUrlPathways"]
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open pathways file: %v", err)
	}
	defer closeReactomeReaders(gz, ftpFile, client, localFile)

	pathwayMap := make(map[string]bool)
	scanner := bufio.NewScanner(br)
	var processedCount int
	var previous int64
	var totalRead int

	for scanner.Scan() {
		line := scanner.Text()
		totalRead += len(line)

		// Format: pathway_id\tpathway_name\tspecies
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		pathwayID := fields[0]
		pathwayName := fields[1]
		species := fields[2]

		// Get taxonomy ID for species
		taxID, ok := speciesTaxMap[species]
		if !ok {
			log.Printf("Warning: Unknown species '%s' for pathway %s", species, pathwayID)
			continue
		}

		// Extract numeric ID from pathway ID (e.g., "R-BTA-73843" -> "73843")
		parts := strings.Split(pathwayID, "-")
		var numericID string
		if len(parts) >= 3 {
			numericID = parts[2]
		}

		// Check if this is a disease pathway
		isDiseasePathway := diseasePathways[numericID]

		// Create pathway entry
		attr := pbuf.ReactomePathwayAttr{
			Name:              pathwayName,
			TaxId:             taxID,
			IsDiseasePathway:  isDiseasePathway,
		}

		b, _ := ffjson.Marshal(&attr)
		r.d.addProp3(pathwayID, r.sourceID, b)

		// Create xref: pathway → taxonomy
		taxIDStr := strconv.Itoa(int(taxID))
		r.d.addXref(pathwayID, r.sourceID, taxIDStr, "taxonomy", false)

		// Create xref: pathway → GO Biological Process (if available)
		if numericID != "" {
			if goTerm, exists := goTermMap[numericID]; exists {
				// GO dataset ID is 4
				r.d.addXref(pathwayID, r.sourceID, goTerm, "go", false)
			}
		}

		pathwayMap[pathwayID] = true

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, pathwayID)
		}
		processedCount++
		if config.IsTestMode() && shouldStopProcessing(testLimit, processedCount) {
			break
		}

		// Progress reporting
		elapsed := int64(time.Since(r.d.start).Seconds())
		if elapsed > previous+r.d.progInterval {
			kbytesPerSecond := int64(totalRead) / elapsed / 1024
			previous = elapsed
			r.d.progChan <- &progressInfo{dataset: r.source, currentKBPerSec: kbytesPerSecond}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading pathways file: %v", err)
	}

	return pathwayMap, nil
}

// Process hierarchy from ReactomePathwaysRelation.txt (streaming)
func (r *reactome) processHierarchy(pathwayMap map[string]bool) (int, error) {
	var filePath string

	// Check if using local test files or remote download
	if _, ok := config.Dataconf[r.source]["useLocalFile"]; ok && config.Dataconf[r.source]["useLocalFile"] == "yes" {
		basePath := config.Dataconf[r.source]["path"]
		filePath = basePath + "ReactomePathwaysRelation_test.txt"
	} else {
		filePath = config.Dataconf[r.source]["downloadUrlRelation"]
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open hierarchy file: %v", err)
	}
	defer closeReactomeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	hierarchyCount := 0

	// Get dataset IDs for parent/child
	parentDatasetID := config.Dataconf["reactomeparent"]["id"]
	childDatasetID := config.Dataconf["reactomechild"]["id"]

	for scanner.Scan() {
		line := scanner.Text()

		// Format: parent_id\tchild_id
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}

		parentID := fields[0]
		childID := fields[1]

		// Only process if both pathways exist in our map
		if !pathwayMap[parentID] || !pathwayMap[childID] {
			continue
		}

		// Create hierarchy xrefs (following GO/EFO pattern)
		// Child → Parent
		r.d.addXref2(childID, r.sourceID, parentID, "reactomeparent")
		r.d.addXref2(parentID, parentDatasetID, parentID, r.source)

		// Parent → Child
		r.d.addXref2(parentID, r.sourceID, childID, "reactomechild")
		r.d.addXref2(childID, childDatasetID, childID, r.source)

		hierarchyCount++
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading hierarchy file: %v", err)
	}

	return hierarchyCount, nil
}

// Process UniProt mappings from UniProt2Reactome.txt (streaming)
func (r *reactome) processUniProtMappings(pathwayMap map[string]bool, testLimit int) (int, error) {
	var filePath string

	// Check if using local test files or remote download
	if _, ok := config.Dataconf[r.source]["useLocalFile"]; ok && config.Dataconf[r.source]["useLocalFile"] == "yes" {
		basePath := config.Dataconf[r.source]["path"]
		filePath = basePath + "UniProt2Reactome_test.txt"
	} else {
		filePath = config.Dataconf[r.source]["downloadUrlUniProt"]
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open UniProt mappings file: %v", err)
	}
	defer closeReactomeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	mappingCount := 0
	uniprotDatasetID := config.Dataconf["uniprot"]["id"]

	for scanner.Scan() {
		line := scanner.Text()

		// Format: uniprot_id\tpathway_id\turl\tpathway_name\tevidence_code\tspecies
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			continue
		}

		uniprotID := fields[0]
		pathwayID := fields[1]
		evidenceCode := fields[4] // TAS, IEA, IEP

		// Only process if pathway exists in our map
		if !pathwayMap[pathwayID] {
			continue
		}

		// Create xref: UniProt → Reactome with evidence code
		// Evidence code indicates curation quality: TAS (curated), IEA (inferred)
		r.d.addXrefWithEvidence(uniprotID, uniprotDatasetID, pathwayID, r.source, false, evidenceCode)

		mappingCount++

		// In test mode, limit number of mappings
		if testLimit > 0 && mappingCount >= testLimit*10 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading UniProt mappings file: %v", err)
	}

	return mappingCount, nil
}

// Process ChEBI mappings from ChEBI2Reactome.txt (streaming)
func (r *reactome) processChEBIMappings(pathwayMap map[string]bool, testLimit int) (int, error) {
	var filePath string

	// Check if using local test files or remote download
	if _, ok := config.Dataconf[r.source]["useLocalFile"]; ok && config.Dataconf[r.source]["useLocalFile"] == "yes" {
		basePath := config.Dataconf[r.source]["path"]
		filePath = basePath + "ChEBI2Reactome_test.txt"
	} else {
		filePath = config.Dataconf[r.source]["downloadUrlChEBI"]
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open ChEBI mappings file: %v", err)
	}
	defer closeReactomeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	mappingCount := 0
	chebiDatasetID := config.Dataconf["chebi"]["id"]

	for scanner.Scan() {
		line := scanner.Text()

		// Format: chebi_id\tpathway_id\turl\tpathway_name\tevidence_code\tspecies
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			continue
		}

		chebiID := fields[0]
		pathwayID := fields[1]
		evidenceCode := fields[4] // TAS, IEA, IEP

		// Only process if pathway exists in our map
		if !pathwayMap[pathwayID] {
			continue
		}

		// ChEBI IDs in Reactome are numeric - need to prepend "CHEBI:"
		fullChebiID := "CHEBI:" + chebiID

		// Create xref: ChEBI → Reactome with evidence code
		r.d.addXrefWithEvidence(fullChebiID, chebiDatasetID, pathwayID, r.source, false, evidenceCode)

		mappingCount++

		// In test mode, limit number of mappings
		if testLimit > 0 && mappingCount >= testLimit*5 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading ChEBI mappings file: %v", err)
	}

	return mappingCount, nil
}

// Process Ensembl mappings from Ensembl2Reactome.txt (streaming)
func (r *reactome) processEnsemblMappings(pathwayMap map[string]bool, testLimit int) (int, error) {
	var filePath string

	// Check if using local test files or remote download
	if _, ok := config.Dataconf[r.source]["useLocalFile"]; ok && config.Dataconf[r.source]["useLocalFile"] == "yes" {
		basePath := config.Dataconf[r.source]["path"]
		filePath = basePath + "Ensembl2Reactome_test.txt"
	} else {
		filePath = config.Dataconf[r.source]["downloadUrlEnsembl"]
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open Ensembl mappings file: %v", err)
	}
	defer closeReactomeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	mappingCount := 0

	// Check if ensembl dataset exists in config
	if _, exists := config.Dataconf["ensembl"]["id"]; !exists {
		log.Printf("Warning: Ensembl dataset not found in config, skipping Ensembl mappings")
		return 0, nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Format: ensembl_id\tpathway_id\turl\tpathway_name\tevidence_code\tspecies
		// Example: ENSG00000157764	R-HSA-73843	https://...	5-Phosphoribose...	TAS	Homo sapiens
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			continue
		}

		ensemblID := fields[0]
		pathwayID := fields[1]
		evidenceCode := fields[4] // TAS, IEA, IEP

		// Only process if pathway exists in our map
		if !pathwayMap[pathwayID] {
			continue
		}

		// Create xref: Reactome pathway → Ensembl gene with evidence code
		// Forward: reactome/forward/, Reverse: ensembl/from_reactome/ (enables ENSG >> reactome queries)
		r.d.addXrefWithEvidence(pathwayID, r.sourceID, ensemblID, "ensembl", false, evidenceCode)

		mappingCount++

		// In test mode, limit number of mappings
		if testLimit > 0 && mappingCount >= testLimit*20 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading Ensembl mappings file: %v", err)
	}

	return mappingCount, nil
}

// Helper to close readers
func closeReactomeReaders(gz *gzip.Reader, ftpFile *ftp.Response, client *ftp.ServerConn, localFile *os.File) {
	if gz != nil {
		gz.Close()
	}
	if ftpFile != nil {
		ftpFile.Close()
	}
	if client != nil {
		client.Quit()
	}
	if localFile != nil {
		localFile.Close()
	}
}
