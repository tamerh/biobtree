package update

import (
	"biobtree/pbuf"
	"bufio"
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

// fantom5 handles FANTOM5 promoter, enhancer, and gene-level expression data
type fantom5 struct {
	source string
	d      *DataUpdate
}

// Fantom5Sample represents sample metadata from SDRF
type Fantom5Sample struct {
	SampleID    string // CNhs12345
	SampleName  string // "brain, adult, donor1"
	TissueID    string // UBERON:0000955
	TissueName  string // "brain"
	CellTypeID  string // CL:0000540
	CellTypeName string // "neuron"
}

// Fantom5Promoter represents aggregated promoter data
type Fantom5Promoter struct {
	ID              int    // Our numeric ID (1, 2, 3, ...)
	PeakID          string // chr1:631073..631118,+
	PeakName        string // p1@TP53
	Chromosome      string
	Start           int
	End             int
	Strand          string
	GeneSymbol      string
	GeneID          string // Ensembl
	EntrezID        string
	UniprotID       string
	HgncID          string
	TPMValues       map[string]float64 // sample_id -> TPM
	TPMAverage      float64
	TPMMax          float64
	SamplesExpressed int
	ExpressionBreadth string
	TopTissues      []Fantom5TopExpr
	TopCellTypes    []Fantom5TopExpr
}

// Fantom5Enhancer represents aggregated enhancer data
type Fantom5Enhancer struct {
	ID              int
	EnhancerID      string // chr1:167440766-167441089
	Chromosome      string
	Start           int
	End             int
	TPMValues       map[string]float64
	TPMAverage      float64
	TPMMax          float64
	SamplesExpressed int
	AssociatedGenes []string
	TopTissues      []Fantom5TopExpr
}

// Fantom5Gene represents aggregated gene-level data
type Fantom5Gene struct {
	ID              int
	GeneID          string
	GeneSymbol      string
	EntrezID        string
	TPMValues       map[string]float64
	TPMAverage      float64
	TPMMax          float64
	SamplesExpressed int
	ExpressionBreadth string
	TopTissues      []Fantom5TopExpr
}

// Fantom5TopExpr represents a top expression entry
type Fantom5TopExpr struct {
	Name       string
	OntologyID string
	TPM        float64
	Rank       int
}

// check provides context-aware error checking
func (f *fantom5) check(err error, operation string) {
	checkWithContext(err, f.source, operation)
}

func (f *fantom5) update() {
	defer f.d.wg.Done()

	log.Printf("[%s] Starting FANTOM5 data integration...", f.source)
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(f.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, f.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("[%s] [TEST MODE] Processing up to %d entries", f.source, testLimit)
	}

	// When fantom5_promoter is called, process all FANTOM5 datasets
	// (fantom5_promoter is the parent dataset that triggers all processing)
	if f.source == "fantom5_promoter" {
		// Process promoters
		f.processPromoters(testLimit, idLogFile)
		f.d.progChan <- &progressInfo{dataset: "fantom5_promoter", done: true}

		// Process enhancers if configured
		if _, exists := config.Dataconf["fantom5_enhancer"]; exists {
			enhancerTestLimit := config.GetTestLimit("fantom5_enhancer")
			var enhancerIdLogFile *os.File
			if config.IsTestMode() {
				enhancerIdLogFile = openIDLogFile(config.TestRefDir, "fantom5_enhancer_ids.txt")
				if enhancerIdLogFile != nil {
					defer enhancerIdLogFile.Close()
				}
			}
			f.processEnhancers(enhancerTestLimit, enhancerIdLogFile)
			f.d.progChan <- &progressInfo{dataset: "fantom5_enhancer", done: true}
		}

		// Process gene-level data if configured
		if _, exists := config.Dataconf["fantom5_gene"]; exists {
			geneTestLimit := config.GetTestLimit("fantom5_gene")
			var geneIdLogFile *os.File
			if config.IsTestMode() {
				geneIdLogFile = openIDLogFile(config.TestRefDir, "fantom5_gene_ids.txt")
				if geneIdLogFile != nil {
					defer geneIdLogFile.Close()
				}
			}
			f.processGenes(geneTestLimit, geneIdLogFile)
			f.d.progChan <- &progressInfo{dataset: "fantom5_gene", done: true}
		}

		log.Printf("[%s] All FANTOM5 datasets completed in %.2fs", f.source, time.Since(startTime).Seconds())
		return
	}

	// Standalone processing (should not happen with child dataset pattern, but keep for safety)
	switch f.source {
	case "fantom5_enhancer":
		f.processEnhancers(testLimit, idLogFile)
	case "fantom5_gene":
		f.processGenes(testLimit, idLogFile)
	default:
		log.Printf("[%s] Unknown FANTOM5 dataset type", f.source)
	}

	log.Printf("[%s] Completed in %.2fs", f.source, time.Since(startTime).Seconds())
	f.d.progChan <- &progressInfo{dataset: f.source, done: true}
}

// processPromoters handles FANTOM5 promoter/CAGE peak data
func (f *fantom5) processPromoters(testLimit int, idLogFile *os.File) {
	log.Printf("[%s] Loading FANTOM5 promoter data...", f.source)

	// Step 1: Load sample metadata (for sample ID -> tissue/cell mapping)
	// Note: SDRF file is Excel format - we'll parse column headers from expression file
	sampleMeta := f.loadSampleMetadataFromExpression()
	log.Printf("[%s] Loaded metadata for %d samples", f.source, len(sampleMeta))

	// Step 2: Load peak annotations (gene associations)
	peakAnnotations := f.loadPeakAnnotations()
	log.Printf("[%s] Loaded annotations for %d peaks", f.source, len(peakAnnotations))

	// Step 3: Load peak names (p1@TP53 format)
	peakNames := f.loadPeakNames()
	log.Printf("[%s] Loaded names for %d peaks", f.source, len(peakNames))

	// Step 4: Process expression matrix and aggregate
	promoters := f.parsePromoterExpression(sampleMeta, peakAnnotations, peakNames, testLimit, idLogFile)
	log.Printf("[%s] Processed %d promoters", f.source, len(promoters))

	// Step 5: Calculate summaries and save
	f.calculatePromoterSummaries(promoters, sampleMeta)
	f.savePromoters(promoters)
}

// loadSampleMetadataFromExpression extracts sample info from expression file header
func (f *fantom5) loadSampleMetadataFromExpression() map[string]*Fantom5Sample {
	samples := make(map[string]*Fantom5Sample)

	basePath := config.Dataconf[f.source]["path"]
	exprPath := config.Dataconf[f.source]["pathExpression"]
	fullPath := basePath + exprPath

	log.Printf("[%s] Reading sample headers from: %s", f.source, fullPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(f.source, "", "", fullPath)
	f.check(err, "opening expression file for sample metadata")

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
		if gz != nil {
			gz.Close()
		}
	}()

	// Read lines until we find the header (skip ## comment lines)
	reader := bufio.NewReader(br)
	var headerLine string
	for {
		line, err := reader.ReadString('\n')
		f.check(err, "reading expression header")
		// Skip comment lines starting with ##
		if strings.HasPrefix(line, "##") {
			continue
		}
		headerLine = line
		break
	}

	// Parse header columns
	// Format: 00Annotation, short_desc, desc, transcript, entrez_id, hgnc_id, uniprot_id, then sample TPM columns
	columns := strings.Split(strings.TrimSpace(headerLine), "\t")
	sampleStartCol := 7 // Sample columns start at index 7

	for i := sampleStartCol; i < len(columns); i++ {
		sampleName := columns[i]
		// Extract sample ID from name (format varies, but often includes CNhs ID)
		sampleID := sampleName
		if strings.Contains(sampleName, ".CNhs") {
			parts := strings.Split(sampleName, ".CNhs")
			if len(parts) > 1 {
				sampleID = "CNhs" + strings.Split(parts[1], ".")[0]
			}
		}

		// Parse tissue/cell type from sample name
		tissueID, tissueName := f.parseTissueFromSampleName(sampleName)
		cellTypeID, cellTypeName := f.parseCellTypeFromSampleName(sampleName)

		samples[sampleName] = &Fantom5Sample{
			SampleID:     sampleID,
			SampleName:   sampleName,
			TissueID:     tissueID,
			TissueName:   tissueName,
			CellTypeID:   cellTypeID,
			CellTypeName: cellTypeName,
		}
	}

	return samples
}

// parseTissueFromSampleName extracts tissue info from FANTOM5 sample names
func (f *fantom5) parseTissueFromSampleName(name string) (string, string) {
	// FANTOM5 sample names often follow patterns like:
	// "brain, adult, donor1" or "liver, fetal, pool1"
	nameLower := strings.ToLower(name)

	// Map common tissue keywords to UBERON IDs
	tissueMap := map[string]string{
		"brain":       "UBERON:0000955",
		"liver":       "UBERON:0002107",
		"heart":       "UBERON:0000948",
		"lung":        "UBERON:0002048",
		"kidney":      "UBERON:0002113",
		"spleen":      "UBERON:0002106",
		"thymus":      "UBERON:0002370",
		"pancreas":    "UBERON:0001264",
		"stomach":     "UBERON:0000945",
		"colon":       "UBERON:0001155",
		"skin":        "UBERON:0002097",
		"muscle":      "UBERON:0001630",
		"bone":        "UBERON:0002481",
		"blood":       "UBERON:0000178",
		"adipose":     "UBERON:0001013",
		"testis":      "UBERON:0000473",
		"ovary":       "UBERON:0000992",
		"placenta":    "UBERON:0001987",
		"prostate":    "UBERON:0002367",
		"breast":      "UBERON:0000310",
		"thyroid":     "UBERON:0002046",
		"adrenal":     "UBERON:0002369",
	}

	for tissue, uberonID := range tissueMap {
		if strings.Contains(nameLower, tissue) {
			return uberonID, tissue
		}
	}

	return "", ""
}

// parseCellTypeFromSampleName extracts cell type info from sample names
func (f *fantom5) parseCellTypeFromSampleName(name string) (string, string) {
	nameLower := strings.ToLower(name)

	// Map common cell type keywords to CL IDs
	cellTypeMap := map[string]string{
		"neuron":       "CL:0000540",
		"astrocyte":    "CL:0000127",
		"macrophage":   "CL:0000235",
		"monocyte":     "CL:0000576",
		"t cell":       "CL:0000084",
		"b cell":       "CL:0000236",
		"nk cell":      "CL:0000623",
		"neutrophil":   "CL:0000775",
		"fibroblast":   "CL:0000057",
		"epithelial":   "CL:0000066",
		"endothelial":  "CL:0000115",
		"keratinocyte": "CL:0000312",
		"hepatocyte":   "CL:0000182",
		"cardiomyocyte":"CL:0000746",
		"adipocyte":    "CL:0000136",
		"osteoblast":   "CL:0000062",
		"chondrocyte":  "CL:0000138",
		"stem cell":    "CL:0000034",
	}

	for cellType, clID := range cellTypeMap {
		if strings.Contains(nameLower, cellType) {
			return clID, cellType
		}
	}

	return "", ""
}

// loadPeakAnnotations loads gene/protein annotations for CAGE peaks
func (f *fantom5) loadPeakAnnotations() map[string]map[string]string {
	annotations := make(map[string]map[string]string)

	basePath := config.Dataconf[f.source]["path"]
	annotPath := config.Dataconf[f.source]["pathAnnotation"]
	if annotPath == "" {
		log.Printf("[%s] No annotation path configured, skipping", f.source)
		return annotations
	}

	fullPath := basePath + annotPath
	log.Printf("[%s] Loading peak annotations from: %s", f.source, fullPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(f.source, "", "", fullPath)
	if err != nil {
		log.Printf("[%s] Warning: Could not load annotations: %v", f.source, err)
		return annotations
	}

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
		if gz != nil {
			gz.Close()
		}
	}()

	reader := csv.NewReader(br)
	reader.Comma = '\t'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read header to understand column positions
	header, err := reader.Read()
	if err != nil {
		log.Printf("[%s] Warning: Could not read annotation header: %v", f.source, err)
		return annotations
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(col)] = i
	}

	lineNum := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		lineNum++

		if len(record) < 2 {
			continue
		}

		peakID := record[0]
		annot := make(map[string]string)

		// Try to extract gene info from various possible columns
		if idx, ok := colIdx["gene_symbol"]; ok && idx < len(record) {
			annot["gene_symbol"] = record[idx]
		}
		if idx, ok := colIdx["entrezgene"]; ok && idx < len(record) {
			annot["entrez_id"] = record[idx]
		}
		if idx, ok := colIdx["uniprot"]; ok && idx < len(record) {
			annot["uniprot_id"] = record[idx]
		}
		if idx, ok := colIdx["hgnc"]; ok && idx < len(record) {
			annot["hgnc_id"] = record[idx]
		}

		// Also try alternate column names
		for i, val := range record {
			if i > 0 && val != "" {
				headerName := strings.ToLower(header[i])
				if strings.Contains(headerName, "gene") && annot["gene_symbol"] == "" {
					annot["gene_symbol"] = val
				}
				if strings.Contains(headerName, "entrez") && annot["entrez_id"] == "" {
					annot["entrez_id"] = val
				}
				if strings.Contains(headerName, "uniprot") && annot["uniprot_id"] == "" {
					annot["uniprot_id"] = val
				}
			}
		}

		if len(annot) > 0 {
			annotations[peakID] = annot
		}
	}

	log.Printf("[%s] Loaded %d peak annotations from %d lines", f.source, len(annotations), lineNum)
	return annotations
}

// loadPeakNames loads peak naming (p1@TP53 format)
func (f *fantom5) loadPeakNames() map[string]string {
	names := make(map[string]string)

	basePath := config.Dataconf[f.source]["path"]
	namePath := config.Dataconf[f.source]["pathPeakNames"]
	if namePath == "" {
		return names
	}

	fullPath := basePath + namePath
	log.Printf("[%s] Loading peak names from: %s", f.source, fullPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(f.source, "", "", fullPath)
	if err != nil {
		log.Printf("[%s] Warning: Could not load peak names: %v", f.source, err)
		return names
	}

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
		if gz != nil {
			gz.Close()
		}
	}()

	scanner := bufio.NewScanner(br)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, "\t")
		if len(fields) >= 2 {
			// Format: peak_id \t peak_name (p1@TP53)
			peakID := fields[0]
			peakName := fields[1]
			names[peakID] = peakName
		}
	}

	log.Printf("[%s] Loaded %d peak names", f.source, len(names))
	return names
}

// parsePromoterExpression parses the expression matrix and creates promoter entries
func (f *fantom5) parsePromoterExpression(
	sampleMeta map[string]*Fantom5Sample,
	peakAnnotations map[string]map[string]string,
	peakNames map[string]string,
	testLimit int,
	idLogFile *os.File,
) map[int]*Fantom5Promoter {

	promoters := make(map[int]*Fantom5Promoter)

	basePath := config.Dataconf[f.source]["path"]
	exprPath := config.Dataconf[f.source]["pathExpression"]
	fullPath := basePath + exprPath

	log.Printf("[%s] Processing expression matrix: %s", f.source, fullPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(f.source, "", "", fullPath)
	f.check(err, "opening expression file")

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
		if gz != nil {
			gz.Close()
		}
	}()

	// Skip comment lines starting with ## and find header
	bufReader := bufio.NewReader(br)
	var headerLine string
	for {
		line, err := bufReader.ReadString('\n')
		f.check(err, "reading expression file")
		if strings.HasPrefix(line, "##") {
			continue
		}
		headerLine = strings.TrimSpace(line)
		break
	}

	// Parse header to get sample names
	headerFields := strings.Split(headerLine, "\t")
	sampleNames := headerFields[7:] // First 7 columns are annotation, rest are sample TPM values
	log.Printf("[%s] Expression matrix has %d samples", f.source, len(sampleNames))

	// Now use csv.Reader for the rest of the data
	reader := csv.NewReader(bufReader)
	reader.Comma = '\t'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	lineNum := 0
	promoterID := 0
	tpmThreshold := 1.0 // TPM threshold for "expressed"

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		lineNum++

		if lineNum%10000 == 0 {
			log.Printf("[%s] Processed %d peaks...", f.source, lineNum)
		}

		if len(record) < 2 {
			continue
		}

		peakID := record[0]

		// Skip FANTOM5 metadata/statistics rows (e.g., "01STAT:MAPPED", "02STAT:UNMAPPED")
		if strings.Contains(peakID, "STAT:") {
			continue
		}

		promoterID++

		// Parse coordinates from peak ID (e.g., "chr1:631073..631118,+")
		chr, start, end, strand := f.parseCoordinates(peakID)

		// Get peak name (p1@TP53) - from separate file or annotation
		peakName := peakNames[peakID]

		// Get annotations from record columns and/or separate annotation file
		// Record columns: 0=peakID, 1=short_desc, 2=desc, 3=transcript, 4=entrez_id, 5=hgnc_id, 6=uniprot_id
		var geneSymbol, geneID, entrezID, uniprotID, hgncID string

		// First try to get from annotation file
		if annot, ok := peakAnnotations[peakID]; ok {
			geneSymbol = annot["gene_symbol"]
			geneID = annot["gene_id"]
			entrezID = annot["entrez_id"]
			uniprotID = annot["uniprot_id"]
			hgncID = annot["hgnc_id"]
		}

		// Also extract from record columns if available (may override or supplement)
		// Skip "NA" placeholder values used in FANTOM5 data
		if len(record) > 4 && record[4] != "" && record[4] != "NA" && entrezID == "" {
			entrezID = record[4]
		}
		if len(record) > 5 && record[5] != "" && record[5] != "NA" && hgncID == "" {
			hgncID = record[5]
		}
		if len(record) > 6 && record[6] != "" && record[6] != "NA" && uniprotID == "" {
			uniprotID = record[6]
		}

		// Extract gene symbol from peak name if not in annotations
		if geneSymbol == "" && peakName != "" {
			parts := strings.Split(peakName, "@")
			if len(parts) > 1 {
				geneSymbol = parts[1]
			}
		}

		// Parse TPM values (columns 7+ are TPM values)
		tpmValues := make(map[string]float64)
		var tpmSum, tpmMax float64
		samplesExpressed := 0
		tpmStartCol := 7 // TPM values start at column 7

		for i := tpmStartCol; i < len(record) && (i-tpmStartCol) < len(sampleNames); i++ {
			tpm, _ := strconv.ParseFloat(record[i], 64)
			sampleName := sampleNames[i-tpmStartCol]
			tpmValues[sampleName] = tpm

			tpmSum += tpm
			if tpm > tpmMax {
				tpmMax = tpm
			}
			if tpm >= tpmThreshold {
				samplesExpressed++
			}
		}

		promoter := &Fantom5Promoter{
			ID:               promoterID,
			PeakID:           peakID,
			PeakName:         peakName,
			Chromosome:       chr,
			Start:            start,
			End:              end,
			Strand:           strand,
			GeneSymbol:       geneSymbol,
			GeneID:           geneID,
			EntrezID:         entrezID,
			UniprotID:        uniprotID,
			HgncID:           hgncID,
			TPMValues:        tpmValues,
			TPMMax:           tpmMax,
			SamplesExpressed: samplesExpressed,
		}

		if len(tpmValues) > 0 {
			promoter.TPMAverage = tpmSum / float64(len(tpmValues))
		}

		promoters[promoterID] = promoter

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, strconv.Itoa(promoterID))
		}

		// Test limit
		if testLimit > 0 && promoterID >= testLimit {
			log.Printf("[%s] [TEST MODE] Reached limit of %d promoters", f.source, testLimit)
			break
		}
	}

	log.Printf("[%s] Parsed %d promoters from %d lines", f.source, len(promoters), lineNum)
	return promoters
}

// parseCoordinates parses FANTOM5 coordinate format
func (f *fantom5) parseCoordinates(peakID string) (chr string, start, end int, strand string) {
	// Format: chr1:631073..631118,+ or chr1:631073-631118,+
	parts := strings.Split(peakID, ":")
	if len(parts) < 2 {
		return
	}

	chr = parts[0]
	rest := parts[1]

	// Parse strand
	if strings.HasSuffix(rest, ",+") {
		strand = "+"
		rest = strings.TrimSuffix(rest, ",+")
	} else if strings.HasSuffix(rest, ",-") {
		strand = "-"
		rest = strings.TrimSuffix(rest, ",-")
	}

	// Parse coordinates
	var coords []string
	if strings.Contains(rest, "..") {
		coords = strings.Split(rest, "..")
	} else if strings.Contains(rest, "-") {
		coords = strings.Split(rest, "-")
	}

	if len(coords) >= 2 {
		start, _ = strconv.Atoi(coords[0])
		end, _ = strconv.Atoi(coords[1])
	}

	return
}

// calculatePromoterSummaries computes expression breadth and top tissues
func (f *fantom5) calculatePromoterSummaries(promoters map[int]*Fantom5Promoter, sampleMeta map[string]*Fantom5Sample) {
	log.Printf("[%s] Calculating promoter summaries...", f.source)

	totalSamples := len(sampleMeta)
	tpmThreshold := 1.0

	for _, promoter := range promoters {
		// Calculate expression breadth
		expressionRatio := float64(promoter.SamplesExpressed) / float64(totalSamples)
		if expressionRatio > 0.5 {
			promoter.ExpressionBreadth = "ubiquitous"
		} else if expressionRatio > 0.1 {
			promoter.ExpressionBreadth = "broad"
		} else if promoter.SamplesExpressed > 0 {
			promoter.ExpressionBreadth = "tissue_specific"
		} else {
			promoter.ExpressionBreadth = "not_expressed"
		}

		// Aggregate by tissue
		tissueTPM := make(map[string]float64)
		tissueCounts := make(map[string]int)

		for sampleName, tpm := range promoter.TPMValues {
			if tpm < tpmThreshold {
				continue
			}
			if sample, ok := sampleMeta[sampleName]; ok && sample.TissueName != "" {
				key := sample.TissueName + "|" + sample.TissueID
				tissueTPM[key] += tpm
				tissueCounts[key]++
			}
		}

		// Calculate average TPM per tissue and sort
		type tissueAvg struct {
			name       string
			ontologyID string
			avgTPM     float64
		}
		var tissueAvgs []tissueAvg

		for key, totalTPM := range tissueTPM {
			parts := strings.Split(key, "|")
			name := parts[0]
			ontologyID := ""
			if len(parts) > 1 {
				ontologyID = parts[1]
			}
			avgTPM := totalTPM / float64(tissueCounts[key])
			tissueAvgs = append(tissueAvgs, tissueAvg{name, ontologyID, avgTPM})
		}

		sort.Slice(tissueAvgs, func(i, j int) bool {
			return tissueAvgs[i].avgTPM > tissueAvgs[j].avgTPM
		})

		// Take top 10 tissues
		for i := 0; i < 10 && i < len(tissueAvgs); i++ {
			promoter.TopTissues = append(promoter.TopTissues, Fantom5TopExpr{
				Name:       tissueAvgs[i].name,
				OntologyID: tissueAvgs[i].ontologyID,
				TPM:        tissueAvgs[i].avgTPM,
				Rank:       i + 1,
			})
		}

		// Similar aggregation for cell types (simplified)
		cellTypeTPM := make(map[string]float64)
		cellTypeCounts := make(map[string]int)

		for sampleName, tpm := range promoter.TPMValues {
			if tpm < tpmThreshold {
				continue
			}
			if sample, ok := sampleMeta[sampleName]; ok && sample.CellTypeName != "" {
				key := sample.CellTypeName + "|" + sample.CellTypeID
				cellTypeTPM[key] += tpm
				cellTypeCounts[key]++
			}
		}

		var cellTypeAvgs []tissueAvg
		for key, totalTPM := range cellTypeTPM {
			parts := strings.Split(key, "|")
			name := parts[0]
			ontologyID := ""
			if len(parts) > 1 {
				ontologyID = parts[1]
			}
			avgTPM := totalTPM / float64(cellTypeCounts[key])
			cellTypeAvgs = append(cellTypeAvgs, tissueAvg{name, ontologyID, avgTPM})
		}

		sort.Slice(cellTypeAvgs, func(i, j int) bool {
			return cellTypeAvgs[i].avgTPM > cellTypeAvgs[j].avgTPM
		})

		for i := 0; i < 10 && i < len(cellTypeAvgs); i++ {
			promoter.TopCellTypes = append(promoter.TopCellTypes, Fantom5TopExpr{
				Name:       cellTypeAvgs[i].name,
				OntologyID: cellTypeAvgs[i].ontologyID,
				TPM:        cellTypeAvgs[i].avgTPM,
				Rank:       i + 1,
			})
		}
	}
}

// savePromoters saves promoters to database with attributes and cross-references
func (f *fantom5) savePromoters(promoters map[int]*Fantom5Promoter) {
	fr := config.Dataconf[f.source]["id"]
	savedCount := uint64(0)

	log.Printf("[%s] Saving %d promoters to database...", f.source, len(promoters))

	for _, promoter := range promoters {
		idStr := strconv.Itoa(promoter.ID)

		// Build protobuf attribute
		attr := &pbuf.Fantom5PromoterAttr{
			Fantom5PeakId:     promoter.PeakID,
			Fantom5PeakName:   promoter.PeakName,
			Chromosome:        promoter.Chromosome,
			Start:             int32(promoter.Start),
			End:               int32(promoter.End),
			Strand:            promoter.Strand,
			GeneSymbol:        promoter.GeneSymbol,
			GeneId:            promoter.GeneID,
			EntrezId:          promoter.EntrezID,
			UniprotId:         promoter.UniprotID,
			HgncId:            promoter.HgncID,
			TpmAverage:        promoter.TPMAverage,
			TpmMax:            promoter.TPMMax,
			SamplesExpressed:  int32(promoter.SamplesExpressed),
			ExpressionBreadth: promoter.ExpressionBreadth,
		}

		// Add top tissues
		for _, t := range promoter.TopTissues {
			attr.TopTissues = append(attr.TopTissues, &pbuf.Fantom5TopExpression{
				Name:       t.Name,
				OntologyId: t.OntologyID,
				Tpm:        t.TPM,
				Rank:       int32(t.Rank),
			})
		}

		// Add top cell types
		for _, c := range promoter.TopCellTypes {
			attr.TopCellTypes = append(attr.TopCellTypes, &pbuf.Fantom5TopExpression{
				Name:       c.Name,
				OntologyId: c.OntologyID,
				Tpm:        c.TPM,
				Rank:       int32(c.Rank),
			})
		}

		// Marshal and save attributes
		attrBytes, err := ffjson.Marshal(attr)
		f.check(err, "marshaling promoter attributes")
		f.d.addProp3(idStr, fr, attrBytes)

		// Create cross-references
		f.createPromoterReferences(idStr, promoter)

		savedCount++
		if savedCount%10000 == 0 {
			log.Printf("[%s] Saved %d promoters...", f.source, savedCount)
		}
	}

	log.Printf("[%s] Successfully saved %d promoters", f.source, savedCount)
	atomic.AddUint64(&f.d.totalParsedEntry, savedCount)
}

// createPromoterReferences creates text search and cross-reference links
func (f *fantom5) createPromoterReferences(idStr string, promoter *Fantom5Promoter) {
	fr := config.Dataconf[f.source]["id"]

	// Text search by peak ID
	f.d.addXref(promoter.PeakID, textLinkID, idStr, f.source, true)

	// Text search by peak name (p1@TP53)
	if promoter.PeakName != "" {
		f.d.addXref(promoter.PeakName, textLinkID, idStr, f.source, true)
	}

	// Text search by gene symbol
	if promoter.GeneSymbol != "" {
		f.d.addXref(promoter.GeneSymbol, textLinkID, idStr, f.source, true)
	}

	// Cross-reference to Ensembl (via gene symbol lookup)
	if promoter.GeneSymbol != "" {
		f.d.addHumanGeneXrefs(promoter.GeneSymbol, idStr, fr)
	}

	// Cross-reference to Entrez (skip "NA" placeholder values, handle space-separated multiple IDs)
	if promoter.EntrezID != "" && promoter.EntrezID != "NA" {
		for _, eid := range strings.Fields(promoter.EntrezID) {
			if eid != "" && eid != "NA" {
				f.d.addXref(idStr, fr, eid, "entrez", false)
			}
		}
	}

	// Cross-reference to UniProt (skip "NA" placeholder values, handle space-separated multiple IDs)
	if promoter.UniprotID != "" && promoter.UniprotID != "NA" {
		for _, uid := range strings.Fields(promoter.UniprotID) {
			if uid != "" && uid != "NA" {
				f.d.addXref(idStr, fr, uid, "uniprot", false)
			}
		}
	}

	// Cross-reference to HGNC (skip "NA" placeholder values, handle space-separated multiple IDs)
	if promoter.HgncID != "" && promoter.HgncID != "NA" {
		for _, hid := range strings.Fields(promoter.HgncID) {
			if hid != "" && hid != "NA" {
				f.d.addXref(idStr, fr, hid, "hgnc", false)
			}
		}
	}

	// Cross-references to UBERON (tissues where expressed)
	addedUberon := make(map[string]bool)
	for _, tissue := range promoter.TopTissues {
		if tissue.OntologyID != "" && strings.HasPrefix(tissue.OntologyID, "UBERON:") {
			if !addedUberon[tissue.OntologyID] {
				f.d.addXref(idStr, fr, tissue.OntologyID, "uberon", false)
				addedUberon[tissue.OntologyID] = true
			}
		}
	}

	// Cross-references to CL (cell types where expressed)
	addedCL := make(map[string]bool)
	for _, cellType := range promoter.TopCellTypes {
		if cellType.OntologyID != "" && strings.HasPrefix(cellType.OntologyID, "CL:") {
			if !addedCL[cellType.OntologyID] {
				f.d.addXref(idStr, fr, cellType.OntologyID, "cl", false)
				addedCL[cellType.OntologyID] = true
			}
		}
	}

	// Cross-reference to Taxonomy (human only for now)
	f.d.addXref(idStr, fr, "9606", "taxonomy", false)
}

// processEnhancers handles FANTOM5 enhancer data
func (f *fantom5) processEnhancers(testLimit int, idLogFile *os.File) {
	log.Printf("[%s] Loading FANTOM5 enhancer data...", f.source)

	// Load enhancer coordinates
	enhancers := f.loadEnhancerCoordinates(testLimit, idLogFile)
	log.Printf("[%s] Loaded %d enhancer coordinates", f.source, len(enhancers))

	// Load and process expression data
	sampleMeta := f.loadEnhancerSampleMeta()
	f.parseEnhancerExpression(enhancers, sampleMeta)

	// Calculate summaries and save
	f.calculateEnhancerSummaries(enhancers, sampleMeta)
	f.saveEnhancers(enhancers)
}

// loadEnhancerCoordinates loads enhancer BED file
func (f *fantom5) loadEnhancerCoordinates(testLimit int, idLogFile *os.File) map[int]*Fantom5Enhancer {
	enhancers := make(map[int]*Fantom5Enhancer)

	// Use fantom5_enhancer config, not parent f.source
	basePath := config.Dataconf["fantom5_enhancer"]["path"]
	coordPath := config.Dataconf["fantom5_enhancer"]["pathCoordinates"]
	fullPath := basePath + coordPath

	log.Printf("[fantom5_enhancer] Loading enhancer coordinates from: %s", fullPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("fantom5_enhancer", "", "", fullPath)
	f.check(err, "opening enhancer coordinates file")

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
		if gz != nil {
			gz.Close()
		}
	}()

	scanner := bufio.NewScanner(br)
	enhancerID := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		enhancerID++

		chr := fields[0]
		start, _ := strconv.Atoi(fields[1])
		end, _ := strconv.Atoi(fields[2])

		// Create enhancer ID from coordinates
		coordID := chr + ":" + fields[1] + "-" + fields[2]

		enhancers[enhancerID] = &Fantom5Enhancer{
			ID:         enhancerID,
			EnhancerID: coordID,
			Chromosome: chr,
			Start:      start,
			End:        end,
			TPMValues:  make(map[string]float64),
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, strconv.Itoa(enhancerID))
		}

		if testLimit > 0 && enhancerID >= testLimit {
			log.Printf("[%s] [TEST MODE] Reached limit of %d enhancers", f.source, testLimit)
			break
		}
	}

	return enhancers
}

// loadEnhancerSampleMeta loads sample metadata for enhancers
func (f *fantom5) loadEnhancerSampleMeta() map[string]*Fantom5Sample {
	samples := make(map[string]*Fantom5Sample)

	// Use fantom5_enhancer config, not parent f.source
	basePath := config.Dataconf["fantom5_enhancer"]["path"]
	exprPath := config.Dataconf["fantom5_enhancer"]["pathExpression"]
	fullPath := basePath + exprPath

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("fantom5_enhancer", "", "", fullPath)
	if err != nil {
		log.Printf("[fantom5_enhancer] Warning: Could not load enhancer expression for sample meta: %v", err)
		return samples
	}

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
		if gz != nil {
			gz.Close()
		}
	}()

	reader := bufio.NewReader(br)
	headerLine, err := reader.ReadString('\n')
	if err != nil {
		return samples
	}

	columns := strings.Split(strings.TrimSpace(headerLine), "\t")
	for i := 1; i < len(columns); i++ {
		sampleName := columns[i]
		tissueID, tissueName := f.parseTissueFromSampleName(sampleName)

		samples[sampleName] = &Fantom5Sample{
			SampleID:   sampleName,
			SampleName: sampleName,
			TissueID:   tissueID,
			TissueName: tissueName,
		}
	}

	return samples
}

// parseEnhancerExpression parses enhancer expression matrix
func (f *fantom5) parseEnhancerExpression(enhancers map[int]*Fantom5Enhancer, sampleMeta map[string]*Fantom5Sample) {
	// Use fantom5_enhancer config, not parent f.source
	basePath := config.Dataconf["fantom5_enhancer"]["path"]
	exprPath := config.Dataconf["fantom5_enhancer"]["pathExpression"]
	fullPath := basePath + exprPath

	log.Printf("[fantom5_enhancer] Processing enhancer expression: %s", fullPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("fantom5_enhancer", "", "", fullPath)
	if err != nil {
		log.Printf("[fantom5_enhancer] Warning: Could not process enhancer expression: %v", err)
		return
	}

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
		if gz != nil {
			gz.Close()
		}
	}()

	reader := csv.NewReader(br)
	reader.Comma = '\t'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read header
	header, err := reader.Read()
	if err != nil {
		return
	}

	sampleNames := header[1:]
	tpmThreshold := 1.0

	// Create map from enhancer ID to our numeric ID
	enhancerIDMap := make(map[string]int)
	for id, e := range enhancers {
		enhancerIDMap[e.EnhancerID] = id
	}

	lineNum := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		lineNum++

		if len(record) < 2 {
			continue
		}

		enhancerCoordID := record[0]
		numericID, ok := enhancerIDMap[enhancerCoordID]
		if !ok {
			// Try to match by parsing coordinates
			continue
		}

		enhancer := enhancers[numericID]
		if enhancer == nil {
			continue
		}

		var tpmSum, tpmMax float64
		samplesExpressed := 0

		for i := 1; i < len(record) && i <= len(sampleNames); i++ {
			tpm, _ := strconv.ParseFloat(record[i], 64)
			sampleName := sampleNames[i-1]
			enhancer.TPMValues[sampleName] = tpm

			tpmSum += tpm
			if tpm > tpmMax {
				tpmMax = tpm
			}
			if tpm >= tpmThreshold {
				samplesExpressed++
			}
		}

		enhancer.TPMMax = tpmMax
		enhancer.SamplesExpressed = samplesExpressed
		if len(enhancer.TPMValues) > 0 {
			enhancer.TPMAverage = tpmSum / float64(len(enhancer.TPMValues))
		}
	}

	log.Printf("[%s] Processed %d expression lines for enhancers", f.source, lineNum)
}

// calculateEnhancerSummaries computes top tissues for enhancers
func (f *fantom5) calculateEnhancerSummaries(enhancers map[int]*Fantom5Enhancer, sampleMeta map[string]*Fantom5Sample) {
	log.Printf("[%s] Calculating enhancer summaries...", f.source)

	tpmThreshold := 1.0

	for _, enhancer := range enhancers {
		tissueTPM := make(map[string]float64)
		tissueCounts := make(map[string]int)

		for sampleName, tpm := range enhancer.TPMValues {
			if tpm < tpmThreshold {
				continue
			}
			if sample, ok := sampleMeta[sampleName]; ok && sample.TissueName != "" {
				key := sample.TissueName + "|" + sample.TissueID
				tissueTPM[key] += tpm
				tissueCounts[key]++
			}
		}

		type tissueAvg struct {
			name       string
			ontologyID string
			avgTPM     float64
		}
		var tissueAvgs []tissueAvg

		for key, totalTPM := range tissueTPM {
			parts := strings.Split(key, "|")
			name := parts[0]
			ontologyID := ""
			if len(parts) > 1 {
				ontologyID = parts[1]
			}
			avgTPM := totalTPM / float64(tissueCounts[key])
			tissueAvgs = append(tissueAvgs, tissueAvg{name, ontologyID, avgTPM})
		}

		sort.Slice(tissueAvgs, func(i, j int) bool {
			return tissueAvgs[i].avgTPM > tissueAvgs[j].avgTPM
		})

		for i := 0; i < 10 && i < len(tissueAvgs); i++ {
			enhancer.TopTissues = append(enhancer.TopTissues, Fantom5TopExpr{
				Name:       tissueAvgs[i].name,
				OntologyID: tissueAvgs[i].ontologyID,
				TPM:        tissueAvgs[i].avgTPM,
				Rank:       i + 1,
			})
		}
	}
}

// saveEnhancers saves enhancers to database
func (f *fantom5) saveEnhancers(enhancers map[int]*Fantom5Enhancer) {
	// Use fantom5_enhancer config, not parent f.source
	fr := config.Dataconf["fantom5_enhancer"]["id"]
	savedCount := uint64(0)

	log.Printf("[fantom5_enhancer] Saving %d enhancers to database...", len(enhancers))

	for _, enhancer := range enhancers {
		idStr := strconv.Itoa(enhancer.ID)

		attr := &pbuf.Fantom5EnhancerAttr{
			Fantom5EnhancerId: enhancer.EnhancerID,
			Chromosome:        enhancer.Chromosome,
			Start:             int32(enhancer.Start),
			End:               int32(enhancer.End),
			TpmAverage:        enhancer.TPMAverage,
			TpmMax:            enhancer.TPMMax,
			SamplesExpressed:  int32(enhancer.SamplesExpressed),
			AssociatedGenes:   enhancer.AssociatedGenes,
		}

		for _, t := range enhancer.TopTissues {
			attr.TopTissues = append(attr.TopTissues, &pbuf.Fantom5TopExpression{
				Name:       t.Name,
				OntologyId: t.OntologyID,
				Tpm:        t.TPM,
				Rank:       int32(t.Rank),
			})
		}

		attrBytes, err := ffjson.Marshal(attr)
		f.check(err, "marshaling enhancer attributes")
		f.d.addProp3(idStr, fr, attrBytes)

		// Create references (use "fantom5_enhancer" not parent f.source)
		f.d.addXref(enhancer.EnhancerID, textLinkID, idStr, "fantom5_enhancer", true)

		// Cross-reference to UBERON
		addedUberon := make(map[string]bool)
		for _, tissue := range enhancer.TopTissues {
			if tissue.OntologyID != "" && strings.HasPrefix(tissue.OntologyID, "UBERON:") {
				if !addedUberon[tissue.OntologyID] {
					f.d.addXref(idStr, fr, tissue.OntologyID, "uberon", false)
					addedUberon[tissue.OntologyID] = true
				}
			}
		}

		// Taxonomy
		f.d.addXref(idStr, fr, "9606", "taxonomy", false)

		savedCount++
	}

	log.Printf("[%s] Successfully saved %d enhancers", f.source, savedCount)
	atomic.AddUint64(&f.d.totalParsedEntry, savedCount)
}

// processGenes handles FANTOM5 gene-level expression data
func (f *fantom5) processGenes(testLimit int, idLogFile *os.File) {
	log.Printf("[%s] Loading FANTOM5 gene-level data...", f.source)

	sampleMeta := f.loadGeneSampleMeta()
	log.Printf("[%s] Loaded metadata for %d samples", f.source, len(sampleMeta))

	genes := f.parseGeneExpression(sampleMeta, testLimit, idLogFile)
	log.Printf("[%s] Processed %d genes", f.source, len(genes))

	f.calculateGeneSummaries(genes, sampleMeta)
	f.saveGenes(genes)
}

// loadGeneSampleMeta loads sample metadata for gene-level data
func (f *fantom5) loadGeneSampleMeta() map[string]*Fantom5Sample {
	samples := make(map[string]*Fantom5Sample)

	// Use fantom5_gene config, not parent f.source
	basePath := config.Dataconf["fantom5_gene"]["path"]
	exprPath := config.Dataconf["fantom5_gene"]["pathExpression"]
	fullPath := basePath + exprPath

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("fantom5_gene", "", "", fullPath)
	if err != nil {
		log.Printf("[fantom5_gene] Warning: Could not load gene expression for sample meta: %v", err)
		return samples
	}

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
		if gz != nil {
			gz.Close()
		}
	}()

	reader := bufio.NewReader(br)

	// Skip comment lines starting with ##
	var headerLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return samples
		}
		if strings.HasPrefix(line, "##") {
			continue
		}
		headerLine = line
		break
	}

	// Gene file format: Column 0 = gene symbol, Column 1+ = TPM values
	columns := strings.Split(strings.TrimSpace(headerLine), "\t")
	for i := 1; i < len(columns); i++ {
		sampleName := columns[i]
		tissueID, tissueName := f.parseTissueFromSampleName(sampleName)

		samples[sampleName] = &Fantom5Sample{
			SampleID:   sampleName,
			SampleName: sampleName,
			TissueID:   tissueID,
			TissueName: tissueName,
		}
	}

	return samples
}

// parseGeneExpression parses gene-level expression matrix
func (f *fantom5) parseGeneExpression(sampleMeta map[string]*Fantom5Sample, testLimit int, idLogFile *os.File) map[int]*Fantom5Gene {
	genes := make(map[int]*Fantom5Gene)

	// Use fantom5_gene config, not parent f.source
	basePath := config.Dataconf["fantom5_gene"]["path"]
	exprPath := config.Dataconf["fantom5_gene"]["pathExpression"]
	fullPath := basePath + exprPath

	log.Printf("[fantom5_gene] Processing gene expression: %s", fullPath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("fantom5_gene", "", "", fullPath)
	f.check(err, "opening gene expression file")

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
		if gz != nil {
			gz.Close()
		}
	}()

	// Skip comment lines starting with ## and find header
	bufReader := bufio.NewReader(br)
	var headerLine string
	for {
		line, err := bufReader.ReadString('\n')
		f.check(err, "reading gene expression file")
		if strings.HasPrefix(line, "##") {
			continue
		}
		headerLine = strings.TrimSpace(line)
		break
	}

	// Parse header to get sample names
	// Gene file format: Column 0 = gene symbol, Column 1+ = TPM values
	headerFields := strings.Split(headerLine, "\t")
	sampleNames := headerFields[1:] // Column 0 is gene symbol, rest are samples
	log.Printf("[%s] Gene matrix has %d samples", f.source, len(sampleNames))

	// Now use csv.Reader for the rest of the data
	reader := csv.NewReader(bufReader)
	reader.Comma = '\t'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	lineNum := 0
	geneID := 0
	tpmThreshold := 1.0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		lineNum++

		if len(record) < 2 {
			continue
		}

		geneIDStr := record[0]
		geneID++

		// Parse gene symbol from ID (format varies)
		geneSymbol := ""
		entrezID := ""

		// Try to extract gene info
		if strings.HasPrefix(geneIDStr, "ENSG") {
			// Ensembl gene ID
		} else if _, err := strconv.Atoi(geneIDStr); err == nil {
			// Numeric - likely Entrez ID
			entrezID = geneIDStr
		} else {
			// Likely gene symbol
			geneSymbol = geneIDStr
		}

		tpmValues := make(map[string]float64)
		var tpmSum, tpmMax float64
		samplesExpressed := 0

		for i := 1; i < len(record) && i <= len(sampleNames); i++ {
			tpm, _ := strconv.ParseFloat(record[i], 64)
			sampleName := sampleNames[i-1]
			tpmValues[sampleName] = tpm

			tpmSum += tpm
			if tpm > tpmMax {
				tpmMax = tpm
			}
			if tpm >= tpmThreshold {
				samplesExpressed++
			}
		}

		gene := &Fantom5Gene{
			ID:               geneID,
			GeneID:           geneIDStr,
			GeneSymbol:       geneSymbol,
			EntrezID:         entrezID,
			TPMValues:        tpmValues,
			TPMMax:           tpmMax,
			SamplesExpressed: samplesExpressed,
		}

		if len(tpmValues) > 0 {
			gene.TPMAverage = tpmSum / float64(len(tpmValues))
		}

		genes[geneID] = gene

		if idLogFile != nil {
			logProcessedID(idLogFile, strconv.Itoa(geneID))
		}

		if testLimit > 0 && geneID >= testLimit {
			log.Printf("[%s] [TEST MODE] Reached limit of %d genes", f.source, testLimit)
			break
		}
	}

	return genes
}

// calculateGeneSummaries computes expression breadth and top tissues for genes
func (f *fantom5) calculateGeneSummaries(genes map[int]*Fantom5Gene, sampleMeta map[string]*Fantom5Sample) {
	log.Printf("[%s] Calculating gene summaries...", f.source)

	totalSamples := len(sampleMeta)
	tpmThreshold := 1.0

	for _, gene := range genes {
		expressionRatio := float64(gene.SamplesExpressed) / float64(totalSamples)
		if expressionRatio > 0.5 {
			gene.ExpressionBreadth = "ubiquitous"
		} else if expressionRatio > 0.1 {
			gene.ExpressionBreadth = "broad"
		} else if gene.SamplesExpressed > 0 {
			gene.ExpressionBreadth = "tissue_specific"
		} else {
			gene.ExpressionBreadth = "not_expressed"
		}

		tissueTPM := make(map[string]float64)
		tissueCounts := make(map[string]int)

		for sampleName, tpm := range gene.TPMValues {
			if tpm < tpmThreshold {
				continue
			}
			if sample, ok := sampleMeta[sampleName]; ok && sample.TissueName != "" {
				key := sample.TissueName + "|" + sample.TissueID
				tissueTPM[key] += tpm
				tissueCounts[key]++
			}
		}

		type tissueAvg struct {
			name       string
			ontologyID string
			avgTPM     float64
		}
		var tissueAvgs []tissueAvg

		for key, totalTPM := range tissueTPM {
			parts := strings.Split(key, "|")
			name := parts[0]
			ontologyID := ""
			if len(parts) > 1 {
				ontologyID = parts[1]
			}
			avgTPM := totalTPM / float64(tissueCounts[key])
			tissueAvgs = append(tissueAvgs, tissueAvg{name, ontologyID, avgTPM})
		}

		sort.Slice(tissueAvgs, func(i, j int) bool {
			return tissueAvgs[i].avgTPM > tissueAvgs[j].avgTPM
		})

		for i := 0; i < 10 && i < len(tissueAvgs); i++ {
			gene.TopTissues = append(gene.TopTissues, Fantom5TopExpr{
				Name:       tissueAvgs[i].name,
				OntologyID: tissueAvgs[i].ontologyID,
				TPM:        tissueAvgs[i].avgTPM,
				Rank:       i + 1,
			})
		}
	}
}

// saveGenes saves genes to database
func (f *fantom5) saveGenes(genes map[int]*Fantom5Gene) {
	// Use fantom5_gene config, not parent f.source
	fr := config.Dataconf["fantom5_gene"]["id"]
	savedCount := uint64(0)

	log.Printf("[fantom5_gene] Saving %d genes to database...", len(genes))

	for _, gene := range genes {
		idStr := strconv.Itoa(gene.ID)

		attr := &pbuf.Fantom5GeneAttr{
			GeneId:            gene.GeneID,
			GeneSymbol:        gene.GeneSymbol,
			EntrezId:          gene.EntrezID,
			TpmAverage:        gene.TPMAverage,
			TpmMax:            gene.TPMMax,
			SamplesExpressed:  int32(gene.SamplesExpressed),
			ExpressionBreadth: gene.ExpressionBreadth,
		}

		for _, t := range gene.TopTissues {
			attr.TopTissues = append(attr.TopTissues, &pbuf.Fantom5TopExpression{
				Name:       t.Name,
				OntologyId: t.OntologyID,
				Tpm:        t.TPM,
				Rank:       int32(t.Rank),
			})
		}

		attrBytes, err := ffjson.Marshal(attr)
		f.check(err, "marshaling gene attributes")
		f.d.addProp3(idStr, fr, attrBytes)

		// Text search by gene ID and symbol (use "fantom5_gene" not parent f.source)
		f.d.addXref(gene.GeneID, textLinkID, idStr, "fantom5_gene", true)
		if gene.GeneSymbol != "" {
			f.d.addXref(gene.GeneSymbol, textLinkID, idStr, "fantom5_gene", true)
		}

		// Cross-reference via gene symbol lookup
		if gene.GeneSymbol != "" {
			f.d.addHumanGeneXrefs(gene.GeneSymbol, idStr, fr)
		}

		// Cross-reference to Entrez (skip "NA" placeholder values)
		if gene.EntrezID != "" && gene.EntrezID != "NA" {
			f.d.addXref(idStr, fr, gene.EntrezID, "entrez", false)
		}

		// Cross-reference to UBERON
		addedUberon := make(map[string]bool)
		for _, tissue := range gene.TopTissues {
			if tissue.OntologyID != "" && strings.HasPrefix(tissue.OntologyID, "UBERON:") {
				if !addedUberon[tissue.OntologyID] {
					f.d.addXref(idStr, fr, tissue.OntologyID, "uberon", false)
					addedUberon[tissue.OntologyID] = true
				}
			}
		}

		// Taxonomy
		f.d.addXref(idStr, fr, "9606", "taxonomy", false)

		savedCount++
	}

	log.Printf("[%s] Successfully saved %d genes", f.source, savedCount)
	atomic.AddUint64(&f.d.totalParsedEntry, savedCount)
}
