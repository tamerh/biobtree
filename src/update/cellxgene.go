package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// cellxgene handles parsing CELLxGENE Census single-cell data
// Source: CZ CELLxGENE Census (https://cellxgene.cziscience.com/)
// Data is extracted via Python script (scripts/cellxgene/extract_cellxgene_data.py)
// If data files don't exist, the script is automatically invoked.
//
// CONSOLIDATED VERSION: 2 datasets
//   - cellxgene_datasets.json: Dataset metadata with cell types, tissues, diseases
//   - cellxgene_celltype.json: Comprehensive cell type data (merged cellguide/markers/expression/counts)
//
// Configuration (application.param.json):
//   - cellxgeneExtractScript: Path to extraction script (default: scripts/cellxgene/extract_cellxgene_data.py)
//   - cellxgeneDataDir: Output directory for extracted data (default: cache/cellxgene)
//   - cellxgeneCensusVersion: Census API version to use (default: stable)
type cellxgene struct {
	source string
	d      *DataUpdate
}

// JSON structures matching extracted data format

// cellxgeneDatasetJSON matches the extracted dataset JSON format
type cellxgeneDatasetJSON struct {
	DatasetID       string   `json:"dataset_id"`
	Title           string   `json:"title"`
	CollectionName  string   `json:"collection_name"`
	CollectionID    string   `json:"collection_id"`
	CollectionDOI   string   `json:"collection_doi"`
	CellCount       int64    `json:"cell_count"`
	Citation        string   `json:"citation"`
	Organism        string   `json:"organism"`
	OrganismTaxid   string   `json:"organism_taxid"`
	AssayTypes      []string `json:"assay_types"`
	AssayEfoIDs     []string `json:"assay_efo_ids"`
	CellTypes       []string `json:"cell_types"`
	CellTypeCLIDs   []string `json:"cell_type_cl_ids"`
	Tissues         []string `json:"tissues"`
	TissueUberonIDs []string `json:"tissue_uberon_ids"`
	Diseases        []string `json:"diseases"`
	DiseaseMondoIDs []string `json:"disease_mondo_ids"`
}

// cellxgeneCelltypeJSON matches the consolidated cell type JSON format
type cellxgeneCelltypeJSON struct {
	ID                  string                       `json:"id"`
	CellTypeCL          string                       `json:"cell_type_cl"`
	Name                string                       `json:"name"`
	Definition          string                       `json:"definition"`
	Synonyms            []string                     `json:"synonyms"`
	CanonicalMarkers    []string                     `json:"canonical_markers"`
	CanonicalMarkerIDs  []string                     `json:"canonical_marker_ids"`
	DevelopsFrom        []string                     `json:"develops_from"`
	DevelopsFromIDs     []string                     `json:"develops_from_ids"`
	FoundInTissues      []string                     `json:"found_in_tissues"`
	FoundInTissueIDs    []string                     `json:"found_in_tissue_ids"`
	AssociatedDiseases  []string                     `json:"associated_diseases"`
	AssociatedDiseaseIDs []string                    `json:"associated_disease_ids"`
	TotalCells          int64                        `json:"total_cells"`
	TotalCellCount      int64                        `json:"total_cell_count"`
	UniqueCellCount     int64                        `json:"unique_cell_count"`
	ExpressionByTissue  []cellxgeneTissueExprJSON    `json:"expression_by_tissue"`
}

type cellxgeneTissueExprJSON struct {
	TissueUberon string `json:"tissue_uberon"`
	TissueName   string `json:"tissue_name"`
	CellCount    int64  `json:"cell_count"`
}

// update is the main entry point for cellxgene dataset processing
func (c *cellxgene) update() {
	defer c.d.wg.Done()

	log.Println("CELLxGENE: Starting single-cell data processing (consolidated 2-dataset version)...")

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	if config.IsTestMode() {
		log.Printf("CELLxGENE: [TEST MODE] Processing up to %d entries per dataset", testLimit)
	}

	// Check if data files exist, run extraction if needed
	if !c.ensureDataFilesExist(testLimit) {
		log.Println("CELLxGENE: Failed to ensure data files exist, aborting")
		c.d.progChan <- &progressInfo{dataset: c.source, done: true}
		return
	}

	// Process main datasets file
	datasetsCount := c.processDatasets(testLimit)
	log.Printf("CELLxGENE: Processed %d datasets", datasetsCount)

	// Process consolidated cell type data
	if _, exists := config.Dataconf["cellxgene_celltype"]; exists {
		celltypeCount := c.processCelltype(testLimit)
		log.Printf("CELLxGENE: Processed %d cell type entries", celltypeCount)
	}

	log.Println("CELLxGENE: Processing complete")

	// Signal completion for main dataset
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

// ensureDataFilesExist checks if the required data files exist.
// If they don't exist, it runs the Python extraction script to generate them.
// Returns true if files exist (or were successfully created), false on error.
func (c *cellxgene) ensureDataFilesExist(testLimit int) bool {
	// Get rootDir for resolving relative paths
	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	// Get file paths from config and resolve relative paths
	datasetsPath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	celltypePath := ""
	if _, exists := config.Dataconf["cellxgene_celltype"]; exists {
		celltypePath = c.resolvePath(config.Dataconf["cellxgene_celltype"]["path"], rootDir)
	}

	// Check if both files exist
	datasetsExists := fileExists(datasetsPath)
	celltypeExists := celltypePath == "" || fileExists(celltypePath)

	if datasetsExists && celltypeExists {
		log.Println("CELLxGENE: Data files already exist, skipping extraction")
		return true
	}

	// Files don't exist - need to run extraction
	log.Println("CELLxGENE: Data files not found, running extraction script...")

	// Get script path from config (with default)
	scriptPath := config.Appconf["cellxgeneExtractScript"]
	if scriptPath == "" {
		scriptPath = "scripts/cellxgene/extract_cellxgene_data.py"
	}

	// Resolve script path relative to rootDir
	scriptPath = c.resolvePath(scriptPath, rootDir)

	// Check if script exists
	if !fileExists(scriptPath) {
		log.Printf("CELLxGENE: Extraction script not found at %s", scriptPath)
		return false
	}

	// Get output directory - use parent directory of the datasets path
	outputDir := filepath.Dir(datasetsPath)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("CELLxGENE: Failed to create output directory %s: %v", outputDir, err)
		return false
	}

	// Get census version from config (with default)
	censusVersion := config.Appconf["cellxgeneCensusVersion"]
	if censusVersion == "" {
		censusVersion = "stable"
	}

	// Build command arguments
	args := []string{scriptPath, "--output-dir", outputDir, "--census-version", censusVersion}

	// Add test mode flag if in test mode
	if config.IsTestMode() {
		args = append(args, "--test-mode")
	}

	log.Printf("CELLxGENE: Running extraction: python3 %s", strings.Join(args, " "))

	// Run the Python script
	cmd := exec.Command("python3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Printf("CELLxGENE: Extraction script failed: %v", err)
		return false
	}

	// Verify files were created
	if !fileExists(datasetsPath) {
		log.Printf("CELLxGENE: Extraction completed but datasets file not found at %s", datasetsPath)
		return false
	}
	if celltypePath != "" && !fileExists(celltypePath) {
		log.Printf("CELLxGENE: Extraction completed but celltype file not found at %s", celltypePath)
		return false
	}

	log.Println("CELLxGENE: Extraction completed successfully")
	return true
}

// resolvePath resolves a path relative to rootDir if it's not absolute
func (c *cellxgene) resolvePath(path, rootDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootDir, path)
}

// processDatasets processes the main datasets file (cellxgene_datasets.json)
func (c *cellxgene) processDatasets(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	// Resolve file path relative to rootDir
	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}
	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("CELLxGENE: Error opening datasets file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	// Increase buffer for potentially long JSON lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry cellxgeneDatasetJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("CELLxGENE: Error parsing dataset JSON: %v", err)
			continue
		}

		if entry.DatasetID == "" {
			continue
		}

		// Build protobuf attribute
		attr := &pbuf.CellxgeneAttr{
			DatasetId:        entry.DatasetID,
			Title:            entry.Title,
			CollectionName:   entry.CollectionName,
			CollectionDoi:    entry.CollectionDOI,
			CellCount:        entry.CellCount,
			Organism:         entry.Organism,
			OrganismTaxid:    entry.OrganismTaxid,
			AssayTypes:       entry.AssayTypes,
			AssayEfoIds:      entry.AssayEfoIDs,
			CellTypes:        entry.CellTypes,
			CellTypeClIds:    entry.CellTypeCLIDs,
			Tissues:          entry.Tissues,
			TissueUberonIds:  entry.TissueUberonIDs,
			Diseases:         entry.Diseases,
			DiseaseMondoIds:  entry.DiseaseMondoIDs,
			Citation:         entry.Citation,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("CELLxGENE: Error marshaling dataset %s: %v", entry.DatasetID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(entry.DatasetID, sourceID, attrBytes)

		// Text search: title and collection name
		if entry.Title != "" && len(entry.Title) > 2 && len(entry.Title) < 500 {
			c.d.addXref(entry.Title, textLinkID, entry.DatasetID, c.source, true)
		}
		if entry.CollectionName != "" && len(entry.CollectionName) > 2 && len(entry.CollectionName) < 500 {
			c.d.addXref(entry.CollectionName, textLinkID, entry.DatasetID, c.source, true)
		}

		// Cross-references to ontologies
		c.createDatasetCrossRefs(entry, sourceID)

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.DatasetID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("CELLxGENE: [TEST MODE] Reached datasets limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("CELLxGENE: Scanner error reading datasets: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// createDatasetCrossRefs creates cross-references from dataset to ontologies
func (c *cellxgene) createDatasetCrossRefs(entry cellxgeneDatasetJSON, sourceID string) {
	// Cross-reference to Cell Ontology (CL)
	for _, clID := range entry.CellTypeCLIDs {
		if clID != "" && strings.HasPrefix(clID, "CL:") {
			if _, exists := config.Dataconf["cl"]; exists {
				c.d.addXref(entry.DatasetID, sourceID, clID, "cl", false)
			}
		}
	}

	// Cross-reference to UBERON (anatomy)
	for _, uberonID := range entry.TissueUberonIDs {
		if uberonID != "" && strings.HasPrefix(uberonID, "UBERON:") {
			if _, exists := config.Dataconf["uberon"]; exists {
				c.d.addXref(entry.DatasetID, sourceID, uberonID, "uberon", false)
			}
		}
	}

	// Cross-reference to MONDO (diseases)
	for _, mondoID := range entry.DiseaseMondoIDs {
		if mondoID != "" && strings.HasPrefix(mondoID, "MONDO:") {
			if _, exists := config.Dataconf["mondo"]; exists {
				c.d.addXref(entry.DatasetID, sourceID, mondoID, "mondo", false)
			}
		}
	}

	// Cross-reference to EFO (assays)
	for _, efoID := range entry.AssayEfoIDs {
		if efoID != "" && strings.HasPrefix(efoID, "EFO:") {
			if _, exists := config.Dataconf["efo"]; exists {
				c.d.addXref(entry.DatasetID, sourceID, efoID, "efo", false)
			}
		}
	}

	// Cross-reference to taxonomy (strip NCBITaxon: prefix if present)
	if entry.OrganismTaxid != "" {
		if _, exists := config.Dataconf["taxonomy"]; exists {
			taxid := entry.OrganismTaxid
			// Remove NCBITaxon: prefix (case-insensitive)
			if strings.HasPrefix(strings.ToUpper(taxid), "NCBITAXON:") {
				taxid = taxid[10:] // len("NCBITaxon:") = 10
			}
			if taxid != "" {
				c.d.addXref(entry.DatasetID, sourceID, taxid, "taxonomy", false)
			}
		}
	}
}

// processCelltype processes consolidated cell type data (cellxgene_celltype.json)
func (c *cellxgene) processCelltype(testLimit int) int64 {
	celltypeSource := "cellxgene_celltype"
	sourceID := config.Dataconf[celltypeSource]["id"]

	// Resolve file path relative to rootDir
	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}
	filePath := c.resolvePath(config.Dataconf[celltypeSource]["path"], rootDir)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("CELLxGENE: Error opening celltype file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, celltypeSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry cellxgeneCelltypeJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("CELLxGENE: Error parsing celltype JSON: %v", err)
			continue
		}

		if entry.ID == "" {
			continue
		}

		// Convert expression_by_tissue to protobuf format
		var exprByTissue []*pbuf.CellxgeneTissueExpression
		for _, expr := range entry.ExpressionByTissue {
			exprByTissue = append(exprByTissue, &pbuf.CellxgeneTissueExpression{
				TissueUberon: expr.TissueUberon,
				TissueName:   expr.TissueName,
				CellCount:    expr.CellCount,
			})
		}

		attr := &pbuf.CellxgeneCelltypeAttr{
			CellTypeCl:          entry.CellTypeCL,
			Name:                entry.Name,
			Definition:          entry.Definition,
			Synonyms:            entry.Synonyms,
			CanonicalMarkers:    entry.CanonicalMarkers,
			CanonicalMarkerIds:  entry.CanonicalMarkerIDs,
			DevelopsFrom:        entry.DevelopsFrom,
			DevelopsFromIds:     entry.DevelopsFromIDs,
			FoundInTissues:      entry.FoundInTissues,
			FoundInTissueIds:    entry.FoundInTissueIDs,
			AssociatedDiseases:  entry.AssociatedDiseases,
			AssociatedDiseaseIds: entry.AssociatedDiseaseIDs,
			TotalCells:          entry.TotalCells,
			TotalCellCount:      entry.TotalCellCount,
			UniqueCellCount:     entry.UniqueCellCount,
			ExpressionByTissue:  exprByTissue,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("CELLxGENE: Error marshaling celltype %s: %v", entry.ID, err)
			continue
		}

		c.d.addProp3(entry.ID, sourceID, attrBytes)

		// Text search for cell type name and synonyms
		if entry.Name != "" && len(entry.Name) > 2 && len(entry.Name) < 200 {
			c.d.addXref(entry.Name, textLinkID, entry.ID, celltypeSource, true)
		}
		for _, syn := range entry.Synonyms {
			if syn != "" && len(syn) > 2 && len(syn) < 200 {
				c.d.addXref(syn, textLinkID, entry.ID, celltypeSource, true)
			}
		}

		// Cross-reference to Cell Ontology (the entry ID IS the CL ID)
		if entry.CellTypeCL != "" && strings.HasPrefix(entry.CellTypeCL, "CL:") {
			if _, exists := config.Dataconf["cl"]; exists {
				c.d.addXref(entry.ID, sourceID, entry.CellTypeCL, "cl", false)
			}
		}

		// Cross-reference parent cell types (develops_from)
		for _, parentCL := range entry.DevelopsFromIDs {
			if parentCL != "" && strings.HasPrefix(parentCL, "CL:") {
				if _, exists := config.Dataconf["cl"]; exists {
					c.d.addXref(entry.ID, sourceID, parentCL, "cl", false)
				}
			}
		}

		// Cross-reference to UBERON for tissues (from found_in_tissues)
		for _, tissueID := range entry.FoundInTissueIDs {
			if tissueID != "" && strings.HasPrefix(tissueID, "UBERON:") {
				if _, exists := config.Dataconf["uberon"]; exists {
					c.d.addXref(entry.ID, sourceID, tissueID, "uberon", false)
				}
			}
		}

		// Cross-reference to UBERON for expression_by_tissue
		for _, expr := range entry.ExpressionByTissue {
			if expr.TissueUberon != "" && strings.HasPrefix(expr.TissueUberon, "UBERON:") {
				if _, exists := config.Dataconf["uberon"]; exists {
					c.d.addXref(entry.ID, sourceID, expr.TissueUberon, "uberon", false)
				}
			}
		}

		// Cross-reference to MONDO for diseases
		for _, diseaseID := range entry.AssociatedDiseaseIDs {
			if diseaseID != "" && strings.HasPrefix(diseaseID, "MONDO:") {
				if _, exists := config.Dataconf["mondo"]; exists {
					c.d.addXref(entry.ID, sourceID, diseaseID, "mondo", false)
				}
			}
		}

		// Cross-reference to Ensembl for canonical markers
		if _, ensemblExists := config.Dataconf["ensembl"]; ensemblExists {
			for _, markerID := range entry.CanonicalMarkerIDs {
				if markerID != "" && strings.HasPrefix(markerID, "ENSG") {
					c.d.addXref(entry.ID, sourceID, markerID, "ensembl", false)
				}
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.ID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			break
		}
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))

	// Signal completion for celltype dataset
	c.d.progChan <- &progressInfo{dataset: celltypeSource, done: true}

	return entryCount
}
