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

type bgee struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for bgee processor
func (b *bgee) check(err error, operation string) {
	checkWithContext(err, b.source, operation)
}

func (b *bgee) update() {
	defer b.d.wg.Done()

	log.Printf("[%s] Starting gene expression data integration...", b.source)
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(b.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, b.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("[%s] [TEST MODE] Processing up to %d gene entries per species", b.source, testLimit)
	}

	// Get list of species to process
	speciesList := b.getSpeciesList()
	log.Printf("[%s] Processing %d species...", b.source, len(speciesList))

	totalGenesProcessed := 0

	// Process each species
	for _, species := range speciesList {
		log.Printf("[%s] Starting species: %s (taxid: %d)", b.source, species.Name, species.TaxonomyID)
		speciesStartTime := time.Now()

		// Parse TSV file and aggregate gene data
		genes := b.parseExpressionFile(species, testLimit, idLogFile)
		log.Printf("[%s] Parsed %d genes for %s (%.2fs)",
			b.source, len(genes), species.Name, time.Since(speciesStartTime).Seconds())

		// Post-process: calculate summaries
		b.calculateSummaries(genes)

		// Save to database
		b.saveGenes(genes, species)
		totalGenesProcessed += len(genes)

		log.Printf("[%s] Completed %s: %d genes (%.2fs)",
			b.source, species.Name, len(genes), time.Since(speciesStartTime).Seconds())
	}

	log.Printf("[%s] Data processing complete: %d total genes from %d species (total: %.2fs)",
		b.source, totalGenesProcessed, len(speciesList), time.Since(startTime).Seconds())

	b.d.progChan <- &progressInfo{dataset: b.source, done: true}
}

// getSpeciesList returns list of species to process (supports comma-separated values)
func (b *bgee) getSpeciesList() []SpeciesInfo {
	speciesNames := strings.Split(config.Dataconf[b.source]["species"], ",")
	taxIDs := strings.Split(config.Dataconf[b.source]["species_taxid"], ",")
	fileFormat := config.Dataconf[b.source]["file_format"]          // "simple" or "advanced"
	allConditions := config.Dataconf[b.source]["include_all_conditions"] // "yes" or "no"

	if len(speciesNames) != len(taxIDs) {
		log.Fatalf("[%s] Error: species and species_taxid lists must have same length (species=%d, taxid=%d)",
			b.source, len(speciesNames), len(taxIDs))
	}

	var speciesList []SpeciesInfo

	for i := range speciesNames {
		speciesName := strings.TrimSpace(speciesNames[i])
		taxID, err := strconv.Atoi(strings.TrimSpace(taxIDs[i]))
		if err != nil {
			log.Fatalf("[%s] Error: invalid taxonomy ID '%s' for species '%s'",
				b.source, taxIDs[i], speciesName)
		}

		// Build filename based on configuration
		filename := speciesName + "_expr_" + fileFormat
		if allConditions == "yes" {
			filename += "_all_conditions"
		}
		filename += ".tsv.gz"

		speciesList = append(speciesList, SpeciesInfo{
			Name:          strings.Replace(speciesName, "_", " ", -1),
			FileName:      filename,
			TaxonomyID:    taxID,
			FileFormat:    fileFormat,
			AllConditions: allConditions == "yes",
		})
	}

	return speciesList
}

// parseExpressionFile parses TSV file and returns gene map
func (b *bgee) parseExpressionFile(species SpeciesInfo, testLimit int, idLogFile *os.File) map[string]*BgeeGene {
	genes := make(map[string]*BgeeGene)

	// Build full URL
	basePath := config.Dataconf[b.source]["path"]
	fullPath := basePath + species.FileName

	log.Printf("[%s] Downloading: %s", b.source, fullPath)

	// Download and open file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(b.source, "", "", fullPath)
	b.check(err, "opening Bgee expression file")

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

	// Parse TSV (br is already decompressed by getDataReaderNew)
	reader := csv.NewReader(br)
	reader.Comma = '\t'
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Variable number of fields

	// Read header
	header, err := reader.Read()
	b.check(err, "reading TSV header")
	log.Printf("[%s] File format: %d columns", b.source, len(header))

	lineNum := 0
	processedGenes := 0

	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[%s] Warning - error reading line %d: %v", b.source, lineNum, err)
			continue
		}

		lineNum++

		// Progress logging every 50k lines
		if lineNum%50000 == 0 {
			log.Printf("[%s] Processed %d lines (%d genes)...", b.source, lineNum, processedGenes)
		}

		// Parse based on format
		var cond ExpressionCondition
		var geneID, geneName string

		if species.FileFormat == "advanced" {
			// Advanced format: 57 columns (or 61 with all_conditions)
			minCols := 57
			if species.AllConditions {
				minCols = 61
			}
			if len(record) < minCols {
				continue
			}

			geneID = record[0]
			geneName = strings.Trim(record[1], "\"")

			// Columns 1-9: Overall expression (same structure as simple)
			cond = ExpressionCondition{
				AnatomicalEntityID:   record[2],
				AnatomicalEntityName: strings.Trim(record[3], "\""),
				Expression:           record[4],
				CallQuality:          record[5],
				FDR:                  parseFloat(record[6]),
				ExpressionScore:      parseFloat(record[7]),
				ExpressionRank:       parseFloat(record[8]),
			}

			// Columns 10-12: Overall observation metadata
			cond.IncludingObservedData = parseBool(record[9])
			cond.SelfObservationCount = parseInt(record[10])
			cond.DescendantObservationCount = parseInt(record[11])

			// Columns 13-21: Affymetrix data
			cond.AffymetrixData = parseDataType(record[12:21])

			// Columns 22-30: EST data
			cond.ESTData = parseDataType(record[21:30])

			// Columns 31-39: in situ hybridization data
			cond.InSituData = parseDataType(record[30:39])

			// Columns 40-48: RNA-Seq data
			cond.RNASeqData = parseDataType(record[39:48])

			// Columns 49-57: single-cell RNA-Seq data
			cond.SingleCellData = parseDataType(record[48:57])

			// If all_conditions format, add developmental/demographic data
			if species.AllConditions {
				cond.DevelopmentalStageID = record[57]
				cond.DevelopmentalStageName = strings.Trim(record[58], "\"")
				cond.Sex = record[59]
				cond.Strain = record[60]
			}

		} else {
			// Simple format
			if species.AllConditions {
				// 13-column format: simple_all_conditions
				if len(record) < 13 {
					continue
				}
				geneID = record[0]
				geneName = strings.Trim(record[1], "\"")
				cond = ExpressionCondition{
					AnatomicalEntityID:      record[2],
					AnatomicalEntityName:    strings.Trim(record[3], "\""),
					DevelopmentalStageID:    record[4],
					DevelopmentalStageName:  strings.Trim(record[5], "\""),
					Sex:                     record[6],
					Strain:                  record[7],
					Expression:              record[8],
					CallQuality:             record[9],
					FDR:                     parseFloat(record[10]),
					ExpressionScore:         parseFloat(record[11]),
					ExpressionRank:          parseFloat(record[12]),
				}
			} else {
				// 9-column format: simple
				if len(record) < 9 {
					continue
				}
				geneID = record[0]
				geneName = strings.Trim(record[1], "\"")
				cond = ExpressionCondition{
					AnatomicalEntityID:   record[2],
					AnatomicalEntityName: strings.Trim(record[3], "\""),
					Expression:           record[4],
					CallQuality:          record[5],
					FDR:                  parseFloat(record[6]),
					ExpressionScore:      parseFloat(record[7]),
					ExpressionRank:       parseFloat(record[8]),
				}
			}
		}

		// Get or create gene entry
		gene, exists := genes[geneID]
		if !exists {
			gene = &BgeeGene{
				GeneID:     geneID,
				GeneName:   geneName,
				Species:    species.Name,
				TaxonomyID: species.TaxonomyID,
				Conditions: []ExpressionCondition{},
			}
			genes[geneID] = gene
			processedGenes++

			// Log ID in test mode
			if idLogFile != nil {
				logProcessedID(idLogFile, geneID)
			}

			// Test mode limit (per gene, not per line)
			if testLimit > 0 && processedGenes >= testLimit {
				log.Printf("[%s] [TEST MODE] Reached limit of %d genes", b.source, testLimit)
				break
			}
		}

		// Add expression condition
		gene.Conditions = append(gene.Conditions, cond)

		// Update statistics
		if cond.Expression == "present" {
			gene.PresentCount++
			gene.TotalExpressionScore += cond.ExpressionScore
		} else if cond.Expression == "absent" {
			gene.AbsentCount++
		}

		if cond.CallQuality == "gold quality" {
			gene.GoldQualityCount++
		}

		// Track max score
		if cond.ExpressionScore > gene.MaxExpressionScore {
			gene.MaxExpressionScore = cond.ExpressionScore
		}
	}

	log.Printf("[%s] Parsed %d lines, %d unique genes", b.source, lineNum, len(genes))
	return genes
}

// calculateSummaries computes summary statistics for all genes
func (b *bgee) calculateSummaries(genes map[string]*BgeeGene) {
	log.Printf("[%s] Calculating summary statistics...", b.source)

	for _, gene := range genes {
		gene.TotalConditions = len(gene.Conditions)

		// Average expression score
		if gene.PresentCount > 0 {
			gene.AverageExpressionScore = gene.TotalExpressionScore / float64(gene.PresentCount)
		}

		// Expression breadth classification
		if gene.PresentCount > 100 {
			gene.ExpressionBreadth = "ubiquitous"
		} else if gene.PresentCount > 10 {
			gene.ExpressionBreadth = "broad"
		} else if gene.PresentCount > 0 {
			gene.ExpressionBreadth = "tissue_specific"
		} else {
			gene.ExpressionBreadth = "not_expressed"
		}

		// Get top expressed tissues (top 10 by expression score)
		gene.TopExpressedTissues = b.getTopTissues(gene, 10)
	}
}

// getTopTissues returns top N tissues by expression score
func (b *bgee) getTopTissues(gene *BgeeGene, topN int) []string {
	type tissueScore struct {
		tissue string
		score  float64
	}

	var present []tissueScore
	for _, cond := range gene.Conditions {
		if cond.Expression == "present" {
			present = append(present, tissueScore{
				tissue: cond.AnatomicalEntityName,
				score:  cond.ExpressionScore,
			})
		}
	}

	// Sort by score descending
	sort.Slice(present, func(i, j int) bool {
		return present[i].score > present[j].score
	})

	// Take top N
	var topTissues []string
	for i := 0; i < topN && i < len(present); i++ {
		topTissues = append(topTissues, present[i].tissue)
	}

	return topTissues
}

// saveGenes saves all genes to database with attributes and cross-references
// Uses compact format for bgee entries and creates separate bgee_evidence entries
func (b *bgee) saveGenes(genes map[string]*BgeeGene, species SpeciesInfo) {
	fr := config.Dataconf[b.source]["id"]
	evidenceFr := config.Dataconf["bgee_evidence"]["id"]
	savedCount := uint64(0)
	evidenceCount := uint64(0)

	// Schema for compact top_conditions format
	conditionsSchema := "entity_id|name|expr|score|quality"
	topN := 30

	for geneID, gene := range genes {
		// Build BgeeAttr protobuf message (compact format)
		attr := &pbuf.BgeeAttr{
			GeneId:                 gene.GeneID,
			GeneName:               gene.GeneName,
			Species:                gene.Species,
			SpeciesTaxid:           int32(gene.TaxonomyID),
			TotalPresentCalls:      int32(gene.PresentCount),
			TotalAbsentCalls:       int32(gene.AbsentCount),
			TotalConditions:        int32(gene.TotalConditions),
			TopExpressedTissues:    gene.TopExpressedTissues,
			ExpressionBreadth:      gene.ExpressionBreadth,
			MaxExpressionScore:     gene.MaxExpressionScore,
			AverageExpressionScore: gene.AverageExpressionScore,
			GoldQualityCount:       int32(gene.GoldQualityCount),
			ConditionsSchema:       conditionsSchema,
		}

		// Sort conditions by expression score (highest first)
		sortedConditions := make([]ExpressionCondition, len(gene.Conditions))
		copy(sortedConditions, gene.Conditions)
		sort.Slice(sortedConditions, func(i, j int) bool {
			return sortedConditions[i].ExpressionScore > sortedConditions[j].ExpressionScore
		})

		// Build compact top_conditions (already sorted, take top N)
		attr.TopConditions = b.buildTopConditionsFromSorted(sortedConditions, topN)

		// Marshal and save bgee entry
		attrBytes, err := ffjson.Marshal(attr)
		b.check(err, "marshaling Bgee attributes")
		b.d.addProp3(geneID, fr, attrBytes)

		// Create bgee_evidence entries for each expression condition (sorted by score)
		taxIDStr := strconv.Itoa(gene.TaxonomyID)
		for _, cond := range sortedConditions {
			evidenceID := geneID + "|" + cond.AnatomicalEntityID
			evidenceAttr := &pbuf.BgeeEvidenceAttr{
				GeneId:               geneID,
				AnatomicalEntityId:   cond.AnatomicalEntityID,
				AnatomicalEntityName: cond.AnatomicalEntityName,
				Expression:           cond.Expression,
				CallQuality:          cond.CallQuality,
				Fdr:                  cond.FDR,
				ExpressionScore:      cond.ExpressionScore,
				ExpressionRank:       cond.ExpressionRank,
				HasAffymetrix:        cond.AffymetrixData != nil,
				HasRnaSeq:            cond.RNASeqData != nil,
				HasSingleCell:        cond.SingleCellData != nil,
				HasEst:               cond.ESTData != nil,
				HasInSitu:            cond.InSituData != nil,
			}

			evidenceBytes, err := ffjson.Marshal(evidenceAttr)
			b.check(err, "marshaling BgeeEvidence attributes")
			b.d.addProp3(evidenceID, evidenceFr, evidenceBytes)

			// Xrefs for bgee_evidence:
			// Sort levels: speciesPriority (human first) then expressionScore (highest first)
			sortLevels := []string{
				ComputeSortLevelValue(SortLevelSpeciesPriority, map[string]interface{}{"taxID": taxIDStr}),
				ComputeSortLevelValue(SortLevelExpressionScore, map[string]interface{}{"score": cond.ExpressionScore}),
			}

			// 1. bgee_evidence → bgee (enables bgee >> bgee_evidence)
			b.d.addXrefWithSortLevels(evidenceID, evidenceFr, geneID, b.source, sortLevels)

			// 2. bgee_evidence → uberon/cl (enables UBERON:xxx >> bgee_evidence)
			if strings.HasPrefix(cond.AnatomicalEntityID, "UBERON:") {
				b.d.addXrefWithSortLevels(evidenceID, evidenceFr, cond.AnatomicalEntityID, "uberon", sortLevels)
			} else if strings.HasPrefix(cond.AnatomicalEntityID, "CL:") {
				b.d.addXrefWithSortLevels(evidenceID, evidenceFr, cond.AnatomicalEntityID, "cl", sortLevels)
			}

			evidenceCount++
		}

		// Create cross-references and text search for bgee entry
		b.createReferences(geneID, gene)

		savedCount++

		// Progress logging every 1000 genes
		if savedCount%1000 == 0 {
			log.Printf("[%s] Saved %d genes, %d evidence entries...", b.source, savedCount, evidenceCount)
		}
	}

	log.Printf("[%s] Successfully saved %d genes and %d evidence entries to database", b.source, savedCount, evidenceCount)

	// Report statistics
	atomic.AddUint64(&b.d.totalParsedEntry, savedCount+evidenceCount)
}

// buildTopConditionsFromSorted creates compact pipe-delimited condition strings from pre-sorted conditions
// Format: entity_id|name|expr|score|quality
func (b *bgee) buildTopConditionsFromSorted(sortedConditions []ExpressionCondition, topN int) []string {
	var result []string
	for i := 0; i < topN && i < len(sortedConditions); i++ {
		c := sortedConditions[i]
		// Escape pipe characters in name if any
		name := strings.ReplaceAll(c.AnatomicalEntityName, "|", "/")
		row := c.AnatomicalEntityID + "|" + name + "|" + c.Expression + "|" +
			strconv.FormatFloat(c.ExpressionScore, 'f', 2, 64) + "|" + c.CallQuality
		result = append(result, row)
	}
	return result
}

// createReferences creates text search keywords and cross-references
func (b *bgee) createReferences(geneID string, gene *BgeeGene) {
	fr := config.Dataconf[b.source]["id"]

	// 1. Gene ID text search removed - entry is already findable by its primary key (Ensembl ID)
	// This was causing duplicate search results
	taxID := strconv.Itoa(gene.TaxonomyID)

	// 2. Gene name → Bgee (text search by gene symbol) with species priority
	if gene.GeneName != "" {
		b.d.addXrefWithPriority(gene.GeneName, textLinkID, geneID, b.source, true, taxID)
	}

	// 3. Create cross-reference to Ensembl
	// This enables: ENSG00000000419 >> bgee to get expression data
	// Forward: bgee/forward/ (bgee gene → ensembl)
	// Reverse: ensembl/from_bgee/ (ensembl → bgee) - this is what the query uses
	b.d.addXref(geneID, fr, geneID, "ensembl", false)

	// 3b. HGNC xref commented out - only ~65% of genes have HGNC entries (pseudogenes, lncRNAs don't)
	// This inconsistency may confuse the model. Users can still query via Ensembl ID.
	// Uncomment if needed:
	// if gene.TaxonomyID == 9606 && gene.GeneName != "" {
	// 	b.d.addHumanGeneXrefsViaHGNC(gene.GeneName, geneID, fr)
	// }

	// Get taxonomy ID for species priority sorting (human=01, mouse=02, etc.)
	taxIDStr := strconv.Itoa(gene.TaxonomyID)

	// 4. Create cross-reference to UBERON for expressed tissues
	// This enables: UBERON:XXXXX >> bgee to find genes expressed in that tissue
	// Sort levels: speciesPriority (human first), expressionScore (highest first)
	// Forward: bgee/forward/, Reverse: uberon/from_bgee/
	addedUberon := make(map[string]bool)
	for _, cond := range gene.Conditions {
		if cond.Expression == "present" && strings.HasPrefix(cond.AnatomicalEntityID, "UBERON:") {
			if !addedUberon[cond.AnatomicalEntityID] {
				// Compute sort level values
				sortLevels := []string{
					ComputeSortLevelValue(SortLevelSpeciesPriority, map[string]interface{}{"taxID": taxIDStr}),
					ComputeSortLevelValue(SortLevelExpressionScore, map[string]interface{}{"score": cond.ExpressionScore}),
				}
				b.d.addXrefWithSortLevels(geneID, fr, cond.AnatomicalEntityID, "uberon", sortLevels)
				addedUberon[cond.AnatomicalEntityID] = true
			}
		}
	}

	// 4b. Create cross-reference to CL for cell type expression
	// This enables: CL:XXXXX >> bgee to find genes expressed in that cell type
	// Sort levels: speciesPriority (human first), expressionScore (highest first)
	// Forward: bgee/forward/, Reverse: cl/from_bgee/
	addedCL := make(map[string]bool)
	for _, cond := range gene.Conditions {
		if cond.Expression == "present" && strings.HasPrefix(cond.AnatomicalEntityID, "CL:") {
			if !addedCL[cond.AnatomicalEntityID] {
				// Compute sort level values
				sortLevels := []string{
					ComputeSortLevelValue(SortLevelSpeciesPriority, map[string]interface{}{"taxID": taxIDStr}),
					ComputeSortLevelValue(SortLevelExpressionScore, map[string]interface{}{"score": cond.ExpressionScore}),
				}
				b.d.addXrefWithSortLevels(geneID, fr, cond.AnatomicalEntityID, "cl", sortLevels)
				addedCL[cond.AnatomicalEntityID] = true
			}
		}
	}

	// 5. Create cross-reference to Taxonomy
	// Forward: bgee/forward/, Reverse: taxonomy/from_bgee/
	b.d.addXref(geneID, fr, taxIDStr, "taxonomy", false)
}

// Helper structures
type BgeeGene struct {
	GeneID                 string
	GeneName               string
	Species                string
	TaxonomyID             int
	Conditions             []ExpressionCondition
	PresentCount           int
	AbsentCount            int
	TotalConditions        int
	GoldQualityCount       int
	TotalExpressionScore   float64
	MaxExpressionScore     float64
	AverageExpressionScore float64
	TopExpressedTissues    []string
	ExpressionBreadth      string
}

type ExpressionCondition struct {
	AnatomicalEntityID     string
	AnatomicalEntityName   string
	Expression             string
	CallQuality            string
	FDR                    float64
	ExpressionScore        float64
	ExpressionRank         float64
	DevelopmentalStageID   string
	DevelopmentalStageName string
	Sex                    string
	Strain                 string
	// Advanced format fields
	IncludingObservedData       bool
	SelfObservationCount        int
	DescendantObservationCount  int
	AffymetrixData              *DataTypeExpression
	ESTData                     *DataTypeExpression
	InSituData                  *DataTypeExpression
	RNASeqData                  *DataTypeExpression
	SingleCellData              *DataTypeExpression
}

type DataTypeExpression struct {
	Expression                 string
	CallQuality                string
	FDR                        float64
	ExpressionScore            float64
	ExpressionRank             float64
	Weight                     float64
	IncludingObservedData      bool
	SelfObservationCount       int
	DescendantObservationCount int
}

type SpeciesInfo struct {
	Name          string
	FileName      string
	TaxonomyID    int
	FileFormat    string // "simple" or "advanced"
	AllConditions bool
}

// Helper function to parse float safely
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// Helper function to parse boolean safely
func parseBool(s string) bool {
	return s == "yes" || s == "t" || s == "true" || s == "1"
}

// Helper function to parse int safely
func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// Helper function to parse data type expression (9 columns)
func parseDataType(fields []string) *DataTypeExpression {
	if len(fields) < 9 {
		return nil
	}
	// Check if this data type has any data (expression field not empty)
	if fields[0] == "" || fields[0] == "no data" {
		return nil
	}
	return &DataTypeExpression{
		Expression:                 fields[0],
		CallQuality:                fields[1],
		FDR:                        parseFloat(fields[2]),
		ExpressionScore:            parseFloat(fields[3]),
		ExpressionRank:             parseFloat(fields[4]),
		Weight:                     parseFloat(fields[5]),
		IncludingObservedData:      parseBool(fields[6]),
		SelfObservationCount:       parseInt(fields[7]),
		DescendantObservationCount: parseInt(fields[8]),
	}
}

// Helper function to convert DataTypeExpression to protobuf
func dataTypeToProto(dt *DataTypeExpression) *pbuf.BgeeDataTypeExpression {
	if dt == nil {
		return nil
	}
	return &pbuf.BgeeDataTypeExpression{
		Expression:                 dt.Expression,
		CallQuality:                dt.CallQuality,
		Fdr:                        dt.FDR,
		ExpressionScore:            dt.ExpressionScore,
		ExpressionRank:             dt.ExpressionRank,
		Weight:                     dt.Weight,
		IncludingObservedData:      dt.IncludingObservedData,
		SelfObservationCount:       int32(dt.SelfObservationCount),
		DescendantObservationCount: int32(dt.DescendantObservationCount),
	}
}
