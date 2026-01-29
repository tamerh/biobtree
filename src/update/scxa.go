package update

import (
	"biobtree/pbuf"
	"bufio"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// scxa handles parsing EMBL-EBI Single Cell Expression Atlas data from FTP
// Source: https://ftp.ebi.ac.uk/pub/databases/microarray/data/atlas/sc_experiments/
type scxa struct {
	source string
	d      *DataUpdate
}

const scxaFtpBase = "https://ftp.ebi.ac.uk/pub/databases/microarray/data/atlas/sc_experiments/"

// Helper for context-aware error checking
func (s *scxa) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

// update is the main entry point for SCXA dataset processing
func (s *scxa) update() {
	defer s.d.wg.Done()

	log.Println("SCXA: Starting Single Cell Expression Atlas data processing (FTP-based)...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("SCXA: [TEST MODE] Processing up to %d experiments", testLimit)
	}

	// Get list of experiments from FTP directory listing
	experiments := s.fetchExperimentList()
	log.Printf("SCXA: Found %d experiments on FTP", len(experiments))

	// Process experiments
	count := s.processExperiments(experiments, testLimit, idLogFile)

	log.Printf("SCXA: Processed %d experiments (%.2fs)", count, time.Since(startTime).Seconds())

	// Signal completion to progress handler
	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}

// fetchExperimentList gets list of experiment directories from FTP
func (s *scxa) fetchExperimentList() []string {
	log.Printf("SCXA: Fetching experiment list from %s", scxaFtpBase)

	resp, err := http.Get(scxaFtpBase)
	if err != nil {
		log.Printf("SCXA: Error fetching FTP listing: %v", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("SCXA: Error reading FTP listing: %v", err)
		return nil
	}

	// Parse experiment IDs from HTML directory listing
	// Looking for patterns like E-MTAB-6386, E-ANND-1, E-GEOD-234602
	re := regexp.MustCompile(`E-[A-Z]+-[0-9]+`)
	matches := re.FindAllString(string(body), -1)

	// Deduplicate
	seen := make(map[string]bool)
	var experiments []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			experiments = append(experiments, m)
		}
	}

	sort.Strings(experiments)
	return experiments
}

// processExperiments processes each experiment from FTP
func (s *scxa) processExperiments(experiments []string, testLimit int, idLogFile *os.File) int64 {
	sourceID := config.Dataconf[s.source]["id"]

	var entryCount int64
	var previous int64
	var skippedNoIDF int
	var skippedNoSDRF int

	for _, expID := range experiments {
		// Progress tracking
		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: int64(entryCount / (elapsed + 1))}
		}

		// Parse experiment data from FTP files
		attr := s.parseExperiment(expID)
		if attr == nil {
			skippedNoIDF++
			continue
		}

		// Skip if no species found (invalid experiment)
		if attr.Species == "" {
			skippedNoSDRF++
			continue
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("SCXA: Error marshaling experiment %s: %v", expID, err)
			continue
		}

		// Save primary entry
		s.d.addProp3(expID, sourceID, attrBytes)

		// Create cross-references
		s.createCrossReferences(expID, sourceID, attr)

		// Log ID for testing
		if idLogFile != nil {
			logProcessedID(idLogFile, expID)
		}

		entryCount++

		// Test mode: check limit
		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("SCXA: [TEST MODE] Reached limit of %d experiments, stopping", testLimit)
			break
		}

		// Log progress every 50 experiments
		if entryCount%50 == 0 {
			log.Printf("SCXA: Processed %d experiments...", entryCount)
		}
	}

	log.Printf("SCXA: Skipped %d experiments (no IDF), %d (no SDRF/species)", skippedNoIDF, skippedNoSDRF)
	atomic.AddUint64(&s.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// parseExperiment fetches and parses all FTP files for an experiment
func (s *scxa) parseExperiment(expID string) *pbuf.ScxaAttr {
	attr := &pbuf.ScxaAttr{
		ExperimentAccession: expID,
	}

	// Parse IDF file for experiment metadata
	s.parseIDF(expID, attr)

	// Parse cell_metadata.tsv for cell types, tissues, diseases, species (streaming, no limit)
	s.parseCellMetadata(expID, attr)

	// Parse ALL marker genes (streaming, no limit)
	s.parseMarkerGenes(expID, attr)

	return attr
}

// parseIDF parses the IDF (Investigation Description Format) file
func (s *scxa) parseIDF(expID string, attr *pbuf.ScxaAttr) {
	url := scxaFtpBase + expID + "/" + expID + ".idf.txt"

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Investigation Title":
			attr.Description = value
		case "Public Release Date":
			attr.LoadDate = value
		case "Comment[EAExperimentType]":
			attr.ExperimentType = value
		case "Comment[AEExperimentType]":
			if attr.RawExperimentType == "" {
				attr.RawExperimentType = value
			}
		case "Experimental Factor Name":
			// Split by tab for multiple factors
			factors := strings.Split(value, "\t")
			for _, f := range factors {
				f = strings.TrimSpace(f)
				if f != "" {
					attr.ExperimentalFactors = append(attr.ExperimentalFactors, f)
				}
			}
		case "PubMed ID":
			if value != "" {
				attr.PubmedId = value
			}
		case "Publication DOI":
			if value != "" {
				attr.PublicationDoi = value
			}
		case "Protocol Description":
			// Try to extract technology from protocol description
			s.extractTechnology(value, attr)
		}
	}
}

// extractTechnology extracts technology types from protocol description
func (s *scxa) extractTechnology(protocol string, attr *pbuf.ScxaAttr) {
	protocolLower := strings.ToLower(protocol)
	techMap := map[string]string{
		"10x genomics":  "10x",
		"10x 3'":        "10xv3",
		"10x 5'":        "10x5prime",
		"smart-seq2":    "smart-seq2",
		"smart-seq":     "smart-seq",
		"drop-seq":      "drop-seq",
		"cel-seq":       "cel-seq",
		"indrops":       "inDrop",
		"sci-rna-seq":   "sci-RNA-seq",
		"nextseq":       "NextSeq",
		"hiseq":         "HiSeq",
		"novaseq":       "NovaSeq",
	}

	for pattern, tech := range techMap {
		if strings.Contains(protocolLower, pattern) {
			// Check if already added
			found := false
			for _, t := range attr.TechnologyTypes {
				if t == tech {
					found = true
					break
				}
			}
			if !found {
				attr.TechnologyTypes = append(attr.TechnologyTypes, tech)
			}
		}
	}
}

// parseCellMetadata parses cell_metadata.tsv for complete metadata (streaming, no limit)
// This file has cleaner format with ontology IDs directly available
func (s *scxa) parseCellMetadata(expID string, attr *pbuf.ScxaAttr) {
	url := scxaFtpBase + expID + "/" + expID + ".cell_metadata.tsv"

	resp, err := http.Get(url)
	if err != nil {
		// Fallback to condensed-sdrf if cell_metadata not available
		s.parseCondensedSDRFFallback(expID, attr)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.parseCondensedSDRFFallback(expID, attr)
		return
	}

	// Track unique values with ontology IDs
	cellTypes := make(map[string]string)    // name -> ontology ID
	tissues := make(map[string]string)      // name -> UBERON ID
	diseases := make(map[string]string)     // name -> disease ID
	species := make(map[string]bool)

	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer size for large files - streaming, no line limit
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024) // 2MB buffer for very long lines

	var headerFields []string
	isHeader := true
	cellCount := int64(0)

	// Test mode: limit lines for speed
	maxLines := int64(0) // 0 = no limit
	if config.IsTestMode() {
		maxLines = 5000 // Sample first 5000 cells in test mode
	}

	for scanner.Scan() {
		line := scanner.Text()

		if isHeader {
			isHeader = false
			headerFields = strings.Split(line, "\t")
			continue
		}

		cellCount++

		// Test mode: stop after maxLines
		if maxLines > 0 && cellCount > maxLines {
			break
		}

		parts := strings.Split(line, "\t")
		if len(parts) < len(headerFields) {
			continue
		}

		// Parse based on header field names
		for i, field := range headerFields {
			if i >= len(parts) {
				break
			}
			value := strings.TrimSpace(parts[i])
			if value == "" {
				continue
			}

			fieldLower := strings.ToLower(field)

			switch {
			case fieldLower == "organism":
				species[value] = true
			case strings.Contains(fieldLower, "cell_type") && !strings.Contains(fieldLower, "ontology"):
				if _, exists := cellTypes[value]; !exists {
					cellTypes[value] = ""
				}
			case fieldLower == "cell_type_ontology":
				// Try to associate with cell type name
				clID := extractOntologyIDFromValue(value)
				if clID != "" {
					// Find corresponding cell type and update its ontology
					for ct := range cellTypes {
						if cellTypes[ct] == "" {
							cellTypes[ct] = clID
							break
						}
					}
				}
			case fieldLower == "organism_part" || fieldLower == "tissue":
				if _, exists := tissues[value]; !exists {
					tissues[value] = ""
				}
			case fieldLower == "organism_part_ontology":
				uberonID := extractOntologyIDFromValue(value)
				if uberonID != "" {
					for t := range tissues {
						if tissues[t] == "" {
							tissues[t] = uberonID
							break
						}
					}
				}
			case fieldLower == "disease" && value != "normal" && value != "healthy":
				if _, exists := diseases[value]; !exists {
					diseases[value] = ""
				}
			}
		}
	}

	// Set species
	for sp := range species {
		attr.Species = sp
		break
	}

	// Populate cell types
	for name, clID := range cellTypes {
		attr.CellTypes = append(attr.CellTypes, name)
		if clID != "" && strings.HasPrefix(clID, "CL:") {
			attr.CellTypeClIds = append(attr.CellTypeClIds, clID)
		}
	}

	// Populate tissues
	for name, uberonID := range tissues {
		attr.Tissues = append(attr.Tissues, name)
		if uberonID != "" && strings.HasPrefix(uberonID, "UBERON:") {
			attr.TissueUberonIds = append(attr.TissueUberonIds, uberonID)
		}
	}

	// Populate diseases
	for name, diseaseID := range diseases {
		attr.Diseases = append(attr.Diseases, name)
		if diseaseID != "" {
			attr.DiseaseIds = append(attr.DiseaseIds, diseaseID)
		}
	}

	attr.NumberOfCells = cellCount

	// If species is still empty after parsing cell_metadata, try condensed-sdrf as fallback
	// Some experiments have cell_metadata.tsv but without organism column (e.g., E-ANND-5)
	if attr.Species == "" {
		s.extractSpeciesFromCondensedSDRF(expID, attr)
	}
}

// parseCondensedSDRFFallback is used when cell_metadata.tsv is not available
func (s *scxa) parseCondensedSDRFFallback(expID string, attr *pbuf.ScxaAttr) {
	url := scxaFtpBase + expID + "/" + expID + ".condensed-sdrf.tsv"

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	// Track unique values
	cellTypes := make(map[string]string)
	tissues := make(map[string]string)
	diseases := make(map[string]string)
	species := make(map[string]bool)

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	cellCount := int64(0)

	// Test mode: limit lines for speed
	maxLines := int64(0) // 0 = no limit
	if config.IsTestMode() {
		maxLines = 5000 // Sample first 5000 cells in test mode
	}

	for scanner.Scan() {
		cellCount++

		// Test mode: stop after maxLines
		if maxLines > 0 && cellCount > maxLines {
			break
		}

		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}

		factorType := strings.TrimSpace(parts[3])
		factorName := strings.TrimSpace(parts[4])
		value := strings.TrimSpace(parts[5])
		ontologyURI := ""
		if len(parts) > 6 {
			ontologyURI = strings.TrimSpace(parts[6])
		}

		if factorType != "characteristic" || value == "" {
			continue
		}

		switch strings.ToLower(factorName) {
		case "cell type", "inferred cell type", "inferred cell type - authors labels", "inferred cell type - ontology labels":
			if _, exists := cellTypes[value]; !exists {
				clID := extractOntologyID(ontologyURI, "CL")
				cellTypes[value] = clID
			}
		case "organism part", "tissue", "sampling site":
			if _, exists := tissues[value]; !exists {
				uberonID := extractOntologyID(ontologyURI, "UBERON")
				tissues[value] = uberonID
			}
		case "disease":
			if value != "normal" && value != "healthy" {
				if _, exists := diseases[value]; !exists {
					diseaseID := extractOntologyID(ontologyURI, "")
					diseases[value] = diseaseID
				}
			}
		case "organism":
			species[value] = true
		}
	}

	for sp := range species {
		attr.Species = sp
		break
	}

	for name, clID := range cellTypes {
		attr.CellTypes = append(attr.CellTypes, name)
		if clID != "" {
			attr.CellTypeClIds = append(attr.CellTypeClIds, clID)
		}
	}

	for name, uberonID := range tissues {
		attr.Tissues = append(attr.Tissues, name)
		if uberonID != "" {
			attr.TissueUberonIds = append(attr.TissueUberonIds, uberonID)
		}
	}

	for name, diseaseID := range diseases {
		attr.Diseases = append(attr.Diseases, name)
		if diseaseID != "" {
			attr.DiseaseIds = append(attr.DiseaseIds, diseaseID)
		}
	}

	attr.NumberOfCells = cellCount
}

// extractSpeciesFromCondensedSDRF extracts only species from condensed-sdrf.tsv
// Used as fallback when cell_metadata.tsv doesn't have organism column
func (s *scxa) extractSpeciesFromCondensedSDRF(expID string, attr *pbuf.ScxaAttr) {
	url := scxaFtpBase + expID + "/" + expID + ".condensed-sdrf.tsv"

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	// Only need to find one organism entry
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}

		factorType := strings.TrimSpace(parts[3])
		factorName := strings.TrimSpace(parts[4])
		value := strings.TrimSpace(parts[5])

		if factorType == "characteristic" && strings.ToLower(factorName) == "organism" && value != "" {
			attr.Species = value
			return // Found species, we're done
		}
	}
}

// extractOntologyIDFromValue extracts ontology ID from a URI or direct value
func extractOntologyIDFromValue(value string) string {
	if value == "" {
		return ""
	}
	// If it's a URI, extract the ID
	if strings.Contains(value, "/") {
		return extractOntologyID(value, "")
	}
	// If it already looks like an ID (e.g., CL:0000123), return it
	if strings.Contains(value, ":") {
		return value
	}
	return ""
}

// parseMarkerGenes parses marker gene files for the experiment
// Stores top markers per experiment (limited to avoid oversized attributes)
// Complete gene-level data is in the scxa_expression dataset
func (s *scxa) parseMarkerGenes(expID string, attr *pbuf.ScxaAttr) {
	// Discover marker_genes files from FTP directory listing
	markerFiles := s.discoverMarkerGeneFiles(expID)

	// Limit files and markers to keep attribute size reasonable
	// Complete data is in scxa_expression dataset
	maxFiles := len(markerFiles)
	maxMarkers := 500 // Store top 500 markers per experiment

	if config.IsTestMode() {
		maxFiles = min(3, maxFiles)   // Only 3 files in test mode
		maxMarkers = 100              // Only 100 markers in test mode
	}

	var allMarkers []*pbuf.ScxaMarkerGene

	for i, fileName := range markerFiles {
		if i >= maxFiles || len(allMarkers) >= maxMarkers {
			break
		}

		url := scxaFtpBase + expID + "/" + fileName

		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		// Parse markers from file
		markers := s.parseMarkerGenesFile(resp.Body)
		resp.Body.Close()

		allMarkers = append(allMarkers, markers...)
	}

	// Trim to max if needed
	if len(allMarkers) > maxMarkers {
		allMarkers = allMarkers[:maxMarkers]
	}

	attr.TopMarkerGenes = allMarkers
}

// discoverMarkerGeneFiles finds all marker_genes_*.tsv files for an experiment
func (s *scxa) discoverMarkerGeneFiles(expID string) []string {
	url := scxaFtpBase + expID + "/"

	resp, err := http.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	// Find all marker_genes_*.tsv files
	re := regexp.MustCompile(expID + `\.marker_genes_\d+\.tsv`)
	matches := re.FindAllString(string(body), -1)

	// Deduplicate
	seen := make(map[string]bool)
	var files []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			files = append(files, m)
		}
	}

	return files
}

// parseMarkerGenesFile parses a single marker genes TSV file (streaming, no limit)
func (s *scxa) parseMarkerGenesFile(reader io.Reader) []*pbuf.ScxaMarkerGene {
	var markers []*pbuf.ScxaMarkerGene

	scanner := bufio.NewScanner(reader)
	// Larger buffer for streaming
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	isHeader := true

	// Stream ALL lines - no limit
	for scanner.Scan() {
		if isHeader {
			isHeader = false
			continue
		}

		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}

		// Format: cluster, ref, rank, genes, scores, logfoldchanges, pvals, pvals_adj
		cluster := strings.TrimSpace(parts[0])
		geneID := strings.TrimSpace(parts[3])
		scoreStr := strings.TrimSpace(parts[4])

		if geneID == "" {
			continue
		}

		// Parse score
		score, _ := strconv.ParseFloat(scoreStr, 32)

		// Parse log fold change if available
		var logFC float32
		if len(parts) > 5 {
			lfc, _ := strconv.ParseFloat(strings.TrimSpace(parts[5]), 32)
			logFC = float32(lfc)
		}

		markers = append(markers, &pbuf.ScxaMarkerGene{
			EnsemblId:     geneID,
			Cluster:       cluster,
			Score:         float32(score),
			LogFoldChange: logFC,
		})
	}

	return markers
}

// extractOntologyID extracts ontology ID from URI
func extractOntologyID(uri string, prefix string) string {
	if uri == "" {
		return ""
	}

	// http://purl.obolibrary.org/obo/CL_0000787 -> CL:0000787
	// http://www.ebi.ac.uk/efo/EFO_0001272 -> EFO:0001272
	parts := strings.Split(uri, "/")
	if len(parts) == 0 {
		return ""
	}
	lastPart := parts[len(parts)-1]

	// Convert underscore to colon
	if strings.Contains(lastPart, "_") {
		id := strings.Replace(lastPart, "_", ":", 1)
		if prefix == "" || strings.HasPrefix(id, prefix+":") {
			return id
		}
	}

	return ""
}

// createCrossReferences builds all cross-references for an SCXA experiment
func (s *scxa) createCrossReferences(expID, sourceID string, attr *pbuf.ScxaAttr) {
	// Text search: experiment accession
	s.d.addXref(expID, textLinkID, expID, s.source, true)

	// Text search: description (if not too long)
	if attr.Description != "" && len(attr.Description) > 2 && len(attr.Description) < 500 {
		s.d.addXref(attr.Description, textLinkID, expID, s.source, true)
	}

	// Text search: species name
	if attr.Species != "" && len(attr.Species) > 2 && len(attr.Species) < 100 {
		s.d.addXref(attr.Species, textLinkID, expID, s.source, true)
	}

	// Cross-reference to taxonomy based on species name
	s.createTaxonomyCrossRef(expID, sourceID, attr)

	// Cross-references to Cell Ontology
	for _, clID := range attr.CellTypeClIds {
		if clID != "" && strings.HasPrefix(clID, "CL:") {
			if _, exists := config.Dataconf["cl"]; exists {
				s.d.addXref(expID, sourceID, clID, "cl", false)
			}
		}
	}

	// Cross-references to UBERON (tissues)
	for _, uberonID := range attr.TissueUberonIds {
		if uberonID != "" && strings.HasPrefix(uberonID, "UBERON:") {
			if _, exists := config.Dataconf["uberon"]; exists {
				s.d.addXref(expID, sourceID, uberonID, "uberon", false)
			}
		}
	}

	// Cross-references to disease ontologies
	for _, diseaseID := range attr.DiseaseIds {
		if diseaseID == "" {
			continue
		}
		targetDataset := ""
		if strings.HasPrefix(diseaseID, "MONDO:") {
			targetDataset = "mondo"
		} else if strings.HasPrefix(diseaseID, "EFO:") {
			targetDataset = "efo"
		}
		if targetDataset != "" {
			if _, exists := config.Dataconf[targetDataset]; exists {
				s.d.addXref(expID, sourceID, diseaseID, targetDataset, false)
			}
		}
	}

	// Cross-references to Ensembl genes (marker genes)
	ensemblSeen := make(map[string]bool)
	for _, marker := range attr.TopMarkerGenes {
		if marker.EnsemblId != "" && !ensemblSeen[marker.EnsemblId] {
			ensemblSeen[marker.EnsemblId] = true
			if _, exists := config.Dataconf["ensembl"]; exists {
				s.d.addXref(expID, sourceID, marker.EnsemblId, "ensembl", false)
			}
		}
	}

	// Text search: cell type names
	for _, ct := range attr.CellTypes {
		if ct != "" && len(ct) > 2 && len(ct) < 100 {
			s.d.addXref(ct, textLinkID, expID, s.source, true)
		}
	}

	// Text search: tissue names
	for _, tissue := range attr.Tissues {
		if tissue != "" && len(tissue) > 2 && len(tissue) < 100 {
			s.d.addXref(tissue, textLinkID, expID, s.source, true)
		}
	}

	// Text search: experimental factors
	for _, factor := range attr.ExperimentalFactors {
		if factor != "" && len(factor) > 2 && len(factor) < 100 {
			s.d.addXref(factor, textLinkID, expID, s.source, true)
		}
	}
}

// createTaxonomyCrossRef creates cross-reference to taxonomy based on species name
func (s *scxa) createTaxonomyCrossRef(expID, sourceID string, attr *pbuf.ScxaAttr) {
	// Map common species names to NCBI taxonomy IDs
	speciesTaxIDMap := map[string]string{
		"homo sapiens":                  "9606",
		"mus musculus":                  "10090",
		"rattus norvegicus":             "10116",
		"danio rerio":                   "7955",
		"drosophila melanogaster":       "7227",
		"caenorhabditis elegans":        "6239",
		"arabidopsis thaliana":          "3702",
		"saccharomyces cerevisiae":      "4932",
		"gallus gallus":                 "9031",
		"sus scrofa":                    "9823",
		"macaca fascicularis":           "9541",
		"macaca mulatta":                "9544",
		"oryctolagus cuniculus":         "9986",
		"plasmodium falciparum":         "5833",
		"xenopus tropicalis":            "8364",
		"anopheles gambiae":             "7165",
		"schistosoma mansoni":           "6183",
		"strongyloides stercoralis":     "6248",
		"trypanosoma brucei":            "5691",
	}

	if attr.Species != "" {
		speciesLower := strings.ToLower(strings.TrimSpace(attr.Species))
		if taxID, ok := speciesTaxIDMap[speciesLower]; ok {
			if _, exists := config.Dataconf["taxonomy"]; exists {
				s.d.addXref(expID, sourceID, taxID, "taxonomy", false)
			}
		}
	}
}
