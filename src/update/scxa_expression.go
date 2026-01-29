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

// scxaExpression handles gene-centric expression data from SC Expression Atlas
// Source: marker_stats_*.tsv files from FTP
// Creates two types of entries:
//   - scxa_expression: Gene summary (no detailed expression)
//   - scxa_gene_experiment: Gene-experiment detail (cluster-level data)
type scxaExpression struct {
	source string
	d      *DataUpdate
}

const scxaExpressionFtpBase = "https://ftp.ebi.ac.uk/pub/databases/microarray/data/atlas/sc_experiments/"

// ScxaGeneData holds aggregated expression data for a gene
type ScxaGeneData struct {
	GeneID       string
	GeneName     string
	SpeciesTaxID int32
	// Expression grouped by experiment
	ExperimentData map[string]*ScxaGeneExpData
	// Summary stats
	MaxMean       float64
	SumMean       float64
	TotalClusters int32
	MarkerCount   int32
}

// ScxaGeneExpData holds expression data for a gene in a specific experiment
type ScxaGeneExpData struct {
	ExperimentID string
	Clusters     []*pbuf.ScxaClusterExpression
	IsMarker     bool
	MarkerCount  int32
	MaxMean      float64
}

// check provides context-aware error checking
func (s *scxaExpression) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

// update is the main entry point for scxa_expression dataset processing
func (s *scxaExpression) update() {
	defer s.d.wg.Done()

	log.Println("SCXA_EXPRESSION: Starting gene-centric expression data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("SCXA_EXPRESSION: [TEST MODE] Processing up to %d genes", testLimit)
	}

	// Get list of experiments from FTP
	experiments := s.fetchExperimentList()
	log.Printf("SCXA_EXPRESSION: Found %d experiments on FTP", len(experiments))

	// Process all experiments and aggregate gene data
	geneData := make(map[string]*ScxaGeneData)
	expCount := s.processAllExperiments(experiments, geneData, testLimit)

	log.Printf("SCXA_EXPRESSION: Aggregated data for %d genes from %d experiments", len(geneData), expCount)

	// Save gene entries and gene-experiment detail entries
	geneCount, detailCount := s.saveEntries(geneData, idLogFile, testLimit)

	log.Printf("SCXA_EXPRESSION: Saved %d gene summaries, %d gene-exp details (%.2fs)", geneCount, detailCount, time.Since(startTime).Seconds())

	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}

// fetchExperimentList gets list of experiment directories from FTP
func (s *scxaExpression) fetchExperimentList() []string {
	log.Printf("SCXA_EXPRESSION: Fetching experiment list from %s", scxaExpressionFtpBase)

	resp, err := http.Get(scxaExpressionFtpBase)
	if err != nil {
		log.Printf("SCXA_EXPRESSION: Error fetching FTP listing: %v", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("SCXA_EXPRESSION: Error reading FTP listing: %v", err)
		return nil
	}

	// Parse experiment IDs from HTML directory listing
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

// processAllExperiments processes marker_stats files from all experiments
func (s *scxaExpression) processAllExperiments(experiments []string, geneData map[string]*ScxaGeneData, testLimit int) int {
	var expCount int
	var previous int64

	// Test mode: limit number of experiments processed for speed
	maxExperiments := len(experiments)
	if config.IsTestMode() {
		// Process only 10 experiments in test mode for fast testing
		maxExperiments = min(10, len(experiments))
		log.Printf("SCXA_EXPRESSION: [TEST MODE] Limiting to %d experiments", maxExperiments)
	}

	for i, expID := range experiments {
		if i >= maxExperiments {
			break
		}

		// Progress tracking
		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: int64(len(geneData) / int(elapsed+1))}
		}

		// Process this experiment's marker_stats file
		processed := s.processExperimentMarkerStats(expID, geneData)
		if processed {
			expCount++
		}

		// Log progress every 50 experiments
		if (i+1)%50 == 0 {
			log.Printf("SCXA_EXPRESSION: Processed %d experiments, %d genes so far...", i+1, len(geneData))
		}
	}

	return expCount
}

// processExperimentMarkerStats downloads and parses marker_stats file for an experiment
func (s *scxaExpression) processExperimentMarkerStats(expID string, geneData map[string]*ScxaGeneData) bool {
	// Try marker_stats_filtered_normalised.tsv first
	url := scxaExpressionFtpBase + expID + "/" + expID + ".marker_stats_filtered_normalised.tsv"

	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Stream parse the TSV file
	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer for potentially long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	isHeader := true
	lineCount := 0

	for scanner.Scan() {
		if isHeader {
			isHeader = false
			continue
		}

		line := scanner.Text()
		lineCount++

		// Parse CSV format: "gene_id","grouping","group","cluster_id","p_value","mean","median"
		// Remove quotes and split
		line = strings.ReplaceAll(line, "\"", "")
		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			continue
		}

		geneID := strings.TrimSpace(parts[0])
		if geneID == "" {
			continue
		}

		clusterID := strings.TrimSpace(parts[3])
		markerGroup := strings.TrimSpace(parts[2]) // Which cluster it's a marker for

		// Parse expression values
		pValue, _ := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		meanExpr, _ := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)
		medianExpr, _ := strconv.ParseFloat(strings.TrimSpace(parts[6]), 64)

		// Is this gene a marker for this cluster?
		isMarker := markerGroup == clusterID

		// Get or create gene data
		gene, exists := geneData[geneID]
		if !exists {
			gene = &ScxaGeneData{
				GeneID:         geneID,
				ExperimentData: make(map[string]*ScxaGeneExpData),
			}
			geneData[geneID] = gene
		}

		// Get or create experiment data for this gene
		expData, exists := gene.ExperimentData[expID]
		if !exists {
			expData = &ScxaGeneExpData{
				ExperimentID: expID,
				Clusters:     make([]*pbuf.ScxaClusterExpression, 0),
			}
			gene.ExperimentData[expID] = expData
		}

		// Add cluster expression entry
		clusterEntry := &pbuf.ScxaClusterExpression{
			ClusterId:        clusterID,
			MeanExpression:   meanExpr,
			MedianExpression: medianExpr,
			PValue:           pValue,
			IsMarker:         isMarker,
		}
		expData.Clusters = append(expData.Clusters, clusterEntry)

		// Update experiment-level stats
		if isMarker {
			expData.IsMarker = true
			expData.MarkerCount++
			gene.MarkerCount++
		}
		if meanExpr > expData.MaxMean {
			expData.MaxMean = meanExpr
		}

		// Update gene-level summary stats
		if meanExpr > gene.MaxMean {
			gene.MaxMean = meanExpr
		}
		gene.SumMean += meanExpr
		gene.TotalClusters++
	}

	return lineCount > 0
}

// saveEntries saves gene summary entries and gene-experiment detail entries
func (s *scxaExpression) saveEntries(geneData map[string]*ScxaGeneData, idLogFile *os.File, testLimit int) (int64, int64) {
	sourceID := config.Dataconf[s.source]["id"]
	geneExpSourceID := config.Dataconf["scxa_gene_experiment"]["id"]
	scxaSourceID := config.Dataconf["scxa"]["id"]

	var geneCount int64
	var detailCount int64
	var previous int64

	// Convert map to slice for sorted processing
	genes := make([]*ScxaGeneData, 0, len(geneData))
	for _, gene := range geneData {
		genes = append(genes, gene)
	}
	// Sort by gene ID for consistent ordering
	sort.Slice(genes, func(i, j int) bool {
		return genes[i].GeneID < genes[j].GeneID
	})

	for _, gene := range genes {
		// Progress tracking
		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: int64(geneCount / (elapsed + 1))}
		}

		// Calculate summary stats
		totalExperiments := len(gene.ExperimentData)
		markerExperiments := s.countMarkerExperiments(gene)
		avgMean := 0.0
		if gene.TotalClusters > 0 {
			avgMean = gene.SumMean / float64(gene.TotalClusters)
		}

		// Create gene summary attribute (no detailed expression)
		attr := &pbuf.ScxaExpressionAttr{
			GeneId:                gene.GeneID,
			GeneName:              gene.GeneName,
			SpeciesTaxid:          gene.SpeciesTaxID,
			TotalExperiments:      int32(totalExperiments),
			MarkerExperimentCount: int32(markerExperiments),
			MaxMeanExpression:     gene.MaxMean,
			AvgMeanExpression:     avgMean,
			TotalClusters:         gene.TotalClusters,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("SCXA_EXPRESSION: Error marshaling gene %s: %v", gene.GeneID, err)
			continue
		}

		// Save gene summary entry
		s.d.addProp3(gene.GeneID, sourceID, attrBytes)

		// Save gene-experiment detail entries
		detailCount += s.saveGeneExperimentDetails(gene, geneExpSourceID, scxaSourceID, sourceID)

		// Create cross-references for gene summary
		s.createGeneCrossReferences(gene, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			logProcessedID(idLogFile, gene.GeneID)
		}

		geneCount++

		// Test mode: check limit
		if testLimit > 0 && geneCount >= int64(testLimit) {
			log.Printf("SCXA_EXPRESSION: [TEST MODE] Reached limit of %d genes, stopping", testLimit)
			break
		}

		// Log progress every 10000 genes
		if geneCount%10000 == 0 {
			log.Printf("SCXA_EXPRESSION: Saved %d genes, %d gene-exp details...", geneCount, detailCount)
		}
	}

	atomic.AddUint64(&s.d.totalParsedEntry, uint64(geneCount+detailCount))
	return geneCount, detailCount
}

// saveGeneExperimentDetails saves gene-experiment detail entries
func (s *scxaExpression) saveGeneExperimentDetails(gene *ScxaGeneData, geneExpSourceID, scxaSourceID, geneSourceID string) int64 {
	var count int64

	// Sort experiment IDs for consistent ordering
	expIDs := make([]string, 0, len(gene.ExperimentData))
	for expID := range gene.ExperimentData {
		expIDs = append(expIDs, expID)
	}
	sort.Strings(expIDs)

	for _, expID := range expIDs {
		expData := gene.ExperimentData[expID]

		// Create composite key: gene_id_experiment_id
		compositeKey := gene.GeneID + "_" + expID

		// Create gene-experiment detail attribute
		detailAttr := &pbuf.ScxaGeneExperimentAttr{
			GeneId:                gene.GeneID,
			ExperimentId:          expID,
			Clusters:              expData.Clusters,
			IsMarkerInExperiment:  expData.IsMarker,
			MarkerClusterCount:    expData.MarkerCount,
			MaxMeanExpression:     expData.MaxMean,
		}

		detailBytes, err := ffjson.Marshal(detailAttr)
		if err != nil {
			log.Printf("SCXA_EXPRESSION: Error marshaling gene-exp %s: %v", compositeKey, err)
			continue
		}

		// Save gene-experiment detail entry
		s.d.addProp3(compositeKey, geneExpSourceID, detailBytes)

		// Create cross-references for gene-experiment detail
		// Detail -> Gene (scxa_expression)
		s.d.addXref(compositeKey, geneExpSourceID, gene.GeneID, "scxa_expression", false)
		// Detail -> Experiment (scxa)
		s.d.addXref(compositeKey, geneExpSourceID, expID, "scxa", false)
		// Gene -> Detail (for navigation)
		s.d.addXref(gene.GeneID, geneSourceID, compositeKey, "scxa_gene_experiment", false)

		count++
	}

	return count
}

// countMarkerExperiments counts experiments where gene is a marker
func (s *scxaExpression) countMarkerExperiments(gene *ScxaGeneData) int {
	count := 0
	for _, expData := range gene.ExperimentData {
		if expData.IsMarker {
			count++
		}
	}
	return count
}

// createGeneCrossReferences builds cross-references for a gene summary
func (s *scxaExpression) createGeneCrossReferences(gene *ScxaGeneData, sourceID string) {
	// Cross-reference to Ensembl (if Ensembl ID)
	if strings.HasPrefix(gene.GeneID, "ENS") {
		if _, exists := config.Dataconf["ensembl"]; exists {
			s.d.addXref(gene.GeneID, sourceID, gene.GeneID, "ensembl", false)
		}
	}

	// Cross-references to SCXA experiments (for reverse lookup: gene -> experiments)
	for expID := range gene.ExperimentData {
		s.d.addXref(gene.GeneID, sourceID, expID, "scxa", false)
	}

	// Text search: gene ID
	s.d.addXref(gene.GeneID, textLinkID, gene.GeneID, s.source, true)
}
