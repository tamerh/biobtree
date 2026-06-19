package update

import (
	"biobtree/pbuf"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// entrez handles NCBI Entrez Gene dataset
// Source: ftp.ncbi.nlm.nih.gov/gene/DATA/gene_info.gz
// Contains ~64M genes across all species
type entrez struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for entrez processor
func (e *entrez) check(err error, operation string) {
	checkWithContext(err, e.source, operation)
}

func (e *entrez) update() {
	fr := config.Dataconf[e.source]["id"]
	path := config.Dataconf[e.source]["path"]

	defer e.d.wg.Done()

	// Test mode support
	testLimit := config.GetTestLimit(e.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, e.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Load gene summaries first (if configured)
	summaries := e.loadGeneSummaries()

	var total uint64
	var previous int64
	var totalBytesRead int64

	// Open the data source using getDataReaderNew (handles local files, HTTPS, and FTP)
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", path)
	e.check(err, "opening gene_info.gz")
	defer closeReaders(gz, ftpFile, client, localFile)

	start := time.Now()
	lineCount := 0
	headerSkipped := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			e.check(err, "reading line")
		}

		// Track bytes read (before trimming)
		totalBytesRead += int64(len(line))

		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip header line
		if !headerSkipped {
			if strings.HasPrefix(line, "#") {
				headerSkipped = true
				continue
			}
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		lineCount++

		// Progress reporting
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
		}

		// Parse tab-delimited line
		// Format: tax_id, GeneID, Symbol, LocusTag, Synonyms, dbXrefs, chromosome,
		//         map_location, description, type_of_gene, ...
		fields := strings.Split(line, "\t")
		if len(fields) < 10 {
			continue
		}

		taxID := strings.TrimSpace(fields[0])
		geneID := strings.TrimSpace(fields[1])
		symbol := strings.TrimSpace(fields[2])
		synonymsStr := strings.TrimSpace(fields[4])
		dbXrefsStr := strings.TrimSpace(fields[5])
		chromosome := strings.TrimSpace(fields[6])
		description := strings.TrimSpace(fields[8])
		geneType := strings.TrimSpace(fields[9])

		// Skip entries without a valid GeneID
		if geneID == "" || geneID == "-" {
			continue
		}

		// Parse synonyms (pipe-separated)
		var synonyms []string
		if synonymsStr != "" && synonymsStr != "-" {
			synParts := strings.Split(synonymsStr, "|")
			for _, syn := range synParts {
				syn = strings.TrimSpace(syn)
				if syn != "" && syn != "-" {
					synonyms = append(synonyms, syn)
				}
			}
		}

		// Normalize values
		if chromosome == "-" {
			chromosome = ""
		}
		if description == "-" {
			description = ""
		}
		if geneType == "-" {
			geneType = ""
		}
		if symbol == "-" {
			symbol = ""
		}

		// Create and save the entry attributes
		attr := pbuf.EntrezAttr{
			Name:       description,
			Symbol:     symbol,
			Type:       geneType,
			Synonyms:   synonyms,
			Chromosome: chromosome,
			TaxId:      taxID,
		}

		// Add summary if available
		if summaries != nil {
			if summary, ok := summaries[geneID]; ok {
				attr.Summary = summary
			}
		}

		b, _ := ffjson.Marshal(&attr)
		e.d.addProp3(geneID, fr, b)

		// Add text search for symbol (with model species priority for sorting)
		if symbol != "" {
			e.d.addXrefWithPriority(symbol, textLinkID, geneID, e.source, true, taxID)
		}

		// Add text search for each synonym (with model species priority for sorting)
		for _, syn := range synonyms {
			e.d.addXrefWithPriority(syn, textLinkID, geneID, e.source, true, taxID)
		}

		// Parse dbXrefs and create cross-references
		// Format: "MIM:138670|HGNC:HGNC:5|Ensembl:ENSG00000121410|..."
		if dbXrefsStr != "" && dbXrefsStr != "-" {
			e.parseDbXrefs(dbXrefsStr, geneID, fr)
		}

		// Create taxonomy cross-reference
		if taxID != "" && taxID != "-" {
			e.d.addXref(geneID, fr, taxID, "taxonomy", false)
		}

		total++

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, geneID)
		}

		// Check test limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	atomic.AddUint64(&e.d.totalParsedEntry, total)

	log.Printf("[Entrez Gene] Processed %d genes", total)

	// Process gene2go.gz for Gene → GO cross-references
	e.processGene2GO(fr, testLimit)

	// Process gene_orthologs.gz for ortholog cross-references
	e.processGeneOrthologs(fr, testLimit)

	// Process gene2pubmed.gz for Gene → PubMed cross-references
	e.processGene2PubMed(fr, testLimit)

	// Process gene_group.gz for typed gene relationship cross-references
	e.processGeneGroup(fr, testLimit)

	// Process gene_neighbors.gz for genomic position and neighbor cross-references
	e.processGeneNeighbors(fr, testLimit)

	// Signal completion after all processing is done
	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
}

// parseDbXrefs parses the dbXrefs field and creates cross-references
// Format: "MIM:138670|HGNC:HGNC:5|Ensembl:ENSG00000121410|AllianceGenome:HGNC:5"
func (e *entrez) parseDbXrefs(dbXrefsStr string, geneID string, fr string) {
	xrefs := strings.Split(dbXrefsStr, "|")

	for _, xref := range xrefs {
		xref = strings.TrimSpace(xref)
		if xref == "" {
			continue
		}

		// Parse "source:id" format
		colonIdx := strings.Index(xref, ":")
		if colonIdx == -1 {
			continue
		}

		source := xref[:colonIdx]
		id := xref[colonIdx+1:]

		switch source {
		case "Ensembl":
			// Ensembl gene ID (e.g., ENSG00000121410)
			if strings.HasPrefix(id, "ENSG") {
				e.d.addXref(geneID, fr, id, "ensembl", false)
			}
		case "HGNC":
			// HGNC ID - format is "HGNC:HGNC:5", so id will be "HGNC:5"
			// We want the full HGNC:5 format
			if strings.HasPrefix(id, "HGNC:") {
				e.d.addXref(geneID, fr, id, "hgnc", false)
			}
		// Model-organism gene databases (MGI/RGD/ZFIN/WormBase/Xenbase/SGD/FLYBASE).
		// These connect a species' Entrez gene to its MOD gene id. Combined with the
		// existing orthologentrez links, this bridges human genes to model-organism
		// resources keyed by MOD ids (e.g. alliance_phenotype/alliance_disease):
		//   human gene >> entrez >> orthologentrez (mouse entrez) >> mgi >> alliance_phenotype
		// Id forms below are matched to alliancePhenotypeGeneTarget's key formats.
		// (MOD datasets are optional; addXref skips silently if one isn't configured.)
		case "MGI":
			// dbXref is "MGI:MGI:104537" -> id keeps the "MGI:" prefix (matches MOD key)
			if strings.HasPrefix(id, "MGI:") {
				e.d.addXref(geneID, fr, id, "mgi", false)
			}
		case "RGD":
			// bare accession (e.g. "2003")
			e.d.addXref(geneID, fr, id, "rgd", false)
		case "ZFIN":
			if strings.HasPrefix(id, "ZDB-GENE-") {
				e.d.addXref(geneID, fr, id, "zfin", false)
			}
		case "WormBase":
			if strings.HasPrefix(id, "WBGene") {
				e.d.addXref(geneID, fr, id, "wormbase", false)
			}
		case "Xenbase":
			if strings.HasPrefix(id, "XB-GENE-") {
				e.d.addXref(geneID, fr, id, "xenbase", false)
			}
		case "SGD":
			if strings.HasPrefix(id, "S0") {
				e.d.addXref(geneID, fr, id, "sgd", false)
			}
		case "FLYBASE":
			if strings.HasPrefix(id, "FBgn") {
				e.d.addXref(geneID, fr, id, "flybase", false)
			}
		// MIM (OMIM) - future integration
		// case "MIM":
		//     e.d.addXref(geneID, fr, id, "omim", false)
		}
	}
}

// processGene2GO processes gene2go.gz to create Entrez Gene → GO cross-references
// Format: tax_id, GeneID, GO_ID, Evidence, Qualifier, GO_term, PubMed, Category
func (e *entrez) processGene2GO(fr string, testLimit int) {
	gene2goPath := config.Dataconf[e.source]["gene2goPath"]
	if gene2goPath == "" {
		log.Printf("[Entrez Gene] gene2goPath not configured, skipping GO cross-references")
		return
	}

	log.Printf("[Entrez Gene] Processing gene2go.gz for GO cross-references...")

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", gene2goPath)
	if err != nil {
		log.Printf("[Entrez Gene] Warning: could not open gene2go.gz: %v", err)
		return
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	var total uint64
	var previous int64
	var totalBytesRead int64
	start := time.Now()
	headerSkipped := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[Entrez Gene] Warning: error reading gene2go.gz: %v", err)
			break
		}

		totalBytesRead += int64(len(line))
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Skip header line
		if !headerSkipped {
			if strings.HasPrefix(line, "#") {
				headerSkipped = true
				continue
			}
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
		}

		// Parse: tax_id, GeneID, GO_ID, Evidence, Qualifier, GO_term, PubMed, Category
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		geneID := strings.TrimSpace(fields[1])
		goID := strings.TrimSpace(fields[2])

		if geneID == "" || geneID == "-" || goID == "" {
			continue
		}

		// Create cross-reference: Entrez Gene → GO
		e.d.addXref(geneID, fr, goID, "go", false)

		total++

		// Check test limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	log.Printf("[Entrez Gene] Processed %d Gene → GO cross-references", total)
}

// loadGeneSummaries loads gene summaries from gene_summary.gz into memory
// Returns a map of GeneID -> Summary, or nil if not configured
func (e *entrez) loadGeneSummaries() map[string]string {
	summaryPath := config.Dataconf[e.source]["geneSummaryPath"]
	if summaryPath == "" {
		log.Printf("[Entrez Gene] geneSummaryPath not configured, skipping gene summaries")
		return nil
	}

	log.Printf("[Entrez Gene] Loading gene summaries from gene_summary.gz...")

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", summaryPath)
	if err != nil {
		log.Printf("[Entrez Gene] Warning: could not open gene_summary.gz: %v", err)
		return nil
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	summaries := make(map[string]string)
	var count uint64
	headerSkipped := false
	start := time.Now()

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[Entrez Gene] Warning: error reading gene_summary.gz: %v", err)
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip header line
		if !headerSkipped {
			if strings.HasPrefix(line, "#") {
				headerSkipped = true
				continue
			}
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Parse: tax_id, GeneID, Source, Summary
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}

		geneID := strings.TrimSpace(fields[1])
		summary := strings.TrimSpace(fields[3])

		if geneID == "" || summary == "" {
			continue
		}

		summaries[geneID] = summary
		count++

		// Log heap usage every 10,000 records
		if count%10000 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Printf("[Entrez Gene] Loaded %d summaries, heap: %.2f MB, alloc: %.2f MB",
				count, float64(m.HeapAlloc)/1024/1024, float64(m.Alloc)/1024/1024)
		}
	}

	elapsed := time.Since(start)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("[Entrez Gene] Loaded %d gene summaries in %.2fs, final heap: %.2f MB",
		count, elapsed.Seconds(), float64(m.HeapAlloc)/1024/1024)

	return summaries
}

// processGeneOrthologs processes gene_orthologs.gz to create Entrez Gene → orthologentrez cross-references
// Format: tax_id, GeneID, relationship, Other_tax_id, Other_GeneID
func (e *entrez) processGeneOrthologs(fr string, testLimit int) {
	orthologsPath := config.Dataconf[e.source]["geneOrthologsPath"]
	if orthologsPath == "" {
		log.Printf("[Entrez Gene] geneOrthologsPath not configured, skipping orthologs")
		return
	}

	log.Printf("[Entrez Gene] Processing gene_orthologs.gz for ortholog cross-references...")

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", orthologsPath)
	if err != nil {
		log.Printf("[Entrez Gene] Warning: could not open gene_orthologs.gz: %v", err)
		return
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	var total uint64
	var previous int64
	var totalBytesRead int64
	start := time.Now()
	headerSkipped := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[Entrez Gene] Warning: error reading gene_orthologs.gz: %v", err)
			break
		}

		totalBytesRead += int64(len(line))
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Skip header line
		if !headerSkipped {
			if strings.HasPrefix(line, "#") {
				headerSkipped = true
				continue
			}
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
		}

		// Parse: tax_id, GeneID, relationship, Other_tax_id, Other_GeneID
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}

		geneID := strings.TrimSpace(fields[1])
		otherGeneID := strings.TrimSpace(fields[4])

		if geneID == "" || geneID == "-" || otherGeneID == "" || otherGeneID == "-" {
			continue
		}

		// Create cross-reference: Entrez Gene → orthologentrez (which links to entrez)
		// NCBI stores each ortholog pair only ONCE (one direction). Without the reverse
		// edge, a query gene that happens to be on the "Other" side has no outgoing
		// ortholog edge and the link-view returns just the query node ("returns self").
		// Emit BOTH directions so the ortholog is reachable regardless of NCBI row order.
		// This also fixes the human->model ortholog bridge used by
		// `entrez >> orthologentrez >> mgi >> alliance_phenotype`.
		e.d.addXref(geneID, fr, otherGeneID, "orthologentrez", false)
		e.d.addXref(otherGeneID, fr, geneID, "orthologentrez", false)

		total++

		// Check test limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	log.Printf("[Entrez Gene] Processed %d ortholog cross-references", total)
}

// processGene2PubMed processes gene2pubmed.gz to create Entrez Gene → PubMed cross-references
// Format: tax_id, GeneID, PubMed_ID
// NOTE: This is a large file (~71.6M lines, ~237MB compressed) - will generate many cross-references
func (e *entrez) processGene2PubMed(fr string, testLimit int) {
	gene2pubmedPath := config.Dataconf[e.source]["gene2pubmedPath"]
	if gene2pubmedPath == "" {
		log.Printf("[Entrez Gene] gene2pubmedPath not configured, skipping PubMed cross-references")
		return
	}

	log.Printf("[Entrez Gene] Processing gene2pubmed.gz for PubMed cross-references...")

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", gene2pubmedPath)
	if err != nil {
		log.Printf("[Entrez Gene] Warning: could not open gene2pubmed.gz: %v", err)
		return
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	var total uint64
	var previous int64
	var totalBytesRead int64
	start := time.Now()
	headerSkipped := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[Entrez Gene] Warning: error reading gene2pubmed.gz: %v", err)
			break
		}

		totalBytesRead += int64(len(line))
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Skip header line
		if !headerSkipped {
			if strings.HasPrefix(line, "#") {
				headerSkipped = true
				continue
			}
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
		}

		// Parse: tax_id, GeneID, PubMed_ID
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		geneID := strings.TrimSpace(fields[1])
		pubmedID := strings.TrimSpace(fields[2])

		if geneID == "" || geneID == "-" || pubmedID == "" || pubmedID == "-" {
			continue
		}

		// Create cross-reference: Entrez Gene → PubMed
		e.d.addXref(geneID, fr, pubmedID, "pubmed", false)

		total++

		// Check test limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	log.Printf("[Entrez Gene] Processed %d Gene → PubMed cross-references", total)
}

// processGeneGroup processes gene_group.gz to create gene relationship cross-references
// Format: tax_id, GeneID, relationship, Other_tax_id, Other_GeneID
// Uses single relatedentrez linkdataset with relationship type stored in evidence field
// NOTE: Small file (~51K lines) with specialized gene relationship data
func (e *entrez) processGeneGroup(fr string, testLimit int) {
	geneGroupPath := config.Dataconf[e.source]["geneGroupPath"]
	if geneGroupPath == "" {
		log.Printf("[Entrez Gene] geneGroupPath not configured, skipping gene group relationships")
		return
	}

	log.Printf("[Entrez Gene] Processing gene_group.gz for gene relationship cross-references...")

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", geneGroupPath)
	if err != nil {
		log.Printf("[Entrez Gene] Warning: could not open gene_group.gz: %v", err)
		return
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	var total uint64
	relationshipCounts := make(map[string]uint64)
	var previous int64
	var totalBytesRead int64
	start := time.Now()
	headerSkipped := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[Entrez Gene] Warning: error reading gene_group.gz: %v", err)
			break
		}

		totalBytesRead += int64(len(line))
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Skip header line
		if !headerSkipped {
			if strings.HasPrefix(line, "#") {
				headerSkipped = true
				continue
			}
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
		}

		// Parse: tax_id, GeneID, relationship, Other_tax_id, Other_GeneID
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}

		geneID := strings.TrimSpace(fields[1])
		relationship := strings.TrimSpace(fields[2])
		otherGeneID := strings.TrimSpace(fields[4])

		if geneID == "" || geneID == "-" || otherGeneID == "" || otherGeneID == "-" {
			continue
		}

		// Log unknown relationship types but still process them
		if _, known := relationshipCounts[relationship]; !known {
			log.Printf("[Entrez Gene] Found relationship type: %s", relationship)
		}
		relationshipCounts[relationship]++

		// Create cross-reference with relationship type.
		// gene_group, like gene_orthologs, lists each pair only ONCE. Emit BOTH
		// directions so either gene reaches the other (otherwise the "Other" gene
		// has no outgoing edge and the link-view returns just the query node).
		// The relationship label describes the pair, so it applies to both directions.
		e.d.addXrefWithRelationship(geneID, fr, otherGeneID, "relatedentrez", false, relationship)
		e.d.addXrefWithRelationship(otherGeneID, fr, geneID, "relatedentrez", false, relationship)

		total++

		// Check test limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	// Log counts per relationship type
	log.Printf("[Entrez Gene] Processed %d gene group relationships:", total)
	for rel, count := range relationshipCounts {
		log.Printf("[Entrez Gene]   - %s: %d", rel, count)
	}
}

// encodeNeighborDistance turns a raw gene_neighbors distance string into the
// zero-padded, ascending-sortable evidence value used for neighborentrez edges.
// Empty/"-"/unparseable distances are clamped to the max (sort last).
func encodeNeighborDistance(distStr string) string {
	dist := distanceSortMax
	distStr = strings.TrimSpace(distStr)
	if distStr != "" && distStr != "-" {
		if d, err := strconv.ParseInt(distStr, 10, 64); err == nil && d >= 0 {
			dist = d
		}
	}
	return ComputeSortLevelValue(SortLevelDistance, map[string]interface{}{"distance": dist})
}

// processGeneNeighbors processes gene_neighbors.gz for genomic position and neighbor cross-references
// Format: tax_id, GeneID, genomic_accession.version, genomic_gi, start_position, end_position,
//
//	orientation, chromosome, GeneIDs_on_left, distance_to_left, GeneIDs_on_right,
//	distance_to_right, overlapping_GeneIDs, assembly
//
// NOTE: Large file (~64M lines) - one entry per gene with position and neighbor data
func (e *entrez) processGeneNeighbors(fr string, testLimit int) {
	neighborsPath := config.Dataconf[e.source]["geneNeighborsPath"]
	if neighborsPath == "" {
		log.Printf("[Entrez Gene] geneNeighborsPath not configured, skipping neighbor data")
		return
	}

	log.Printf("[Entrez Gene] Processing gene_neighbors.gz for genomic position and neighbor cross-references...")

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", neighborsPath)
	if err != nil {
		log.Printf("[Entrez Gene] Warning: could not open gene_neighbors.gz: %v", err)
		return
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	var total uint64
	var leftNeighbors, rightNeighbors, overlapping uint64
	var previous int64
	var totalBytesRead int64
	start := time.Now()
	headerSkipped := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[Entrez Gene] Warning: error reading gene_neighbors.gz: %v", err)
			break
		}

		totalBytesRead += int64(len(line))
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Skip header line
		if !headerSkipped {
			if strings.HasPrefix(line, "#") {
				headerSkipped = true
				continue
			}
		}

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Progress reporting
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := totalBytesRead / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
		}

		// Parse: tax_id, GeneID, genomic_accession.version, genomic_gi, start_position, end_position,
		//        orientation, chromosome, GeneIDs_on_left, distance_to_left, GeneIDs_on_right,
		//        distance_to_right, overlapping_GeneIDs, assembly
		fields := strings.Split(line, "\t")
		if len(fields) < 13 {
			continue
		}

		geneID := strings.TrimSpace(fields[1])
		genomicAccession := strings.TrimSpace(fields[2])
		startPosStr := strings.TrimSpace(fields[4])
		endPosStr := strings.TrimSpace(fields[5])
		orientation := strings.TrimSpace(fields[6])
		leftGenesStr := strings.TrimSpace(fields[8])
		leftDistStr := strings.TrimSpace(fields[9])
		rightGenesStr := strings.TrimSpace(fields[10])
		rightDistStr := strings.TrimSpace(fields[11])
		overlappingGenesStr := strings.TrimSpace(fields[12])

		// Get assembly info (field 13) - only process Primary Assembly
		assembly := ""
		if len(fields) >= 14 {
			assembly = strings.TrimSpace(fields[13])
		}

		// Skip non-primary assemblies to avoid duplicate entries per gene
		if !strings.Contains(assembly, "Primary Assembly") {
			continue
		}

		if geneID == "" || geneID == "-" {
			continue
		}

		// Parse positions
		var startPos, endPos int64
		if startPosStr != "" && startPosStr != "-" {
			startPos, _ = strconv.ParseInt(startPosStr, 10, 64)
		}
		if endPosStr != "" && endPosStr != "-" {
			endPos, _ = strconv.ParseInt(endPosStr, 10, 64)
		}

		// Normalize values
		if genomicAccession == "-" {
			genomicAccession = ""
		}
		if orientation == "-" {
			orientation = ""
		}

		// Create position attributes (only if we have valid position data)
		if startPos > 0 || endPos > 0 || genomicAccession != "" {
			attr := pbuf.EntrezAttr{
				StartPosition:    startPos,
				EndPosition:      endPos,
				Orientation:      orientation,
				GenomicAccession: genomicAccession,
			}
			b, _ := ffjson.Marshal(&attr)
			e.d.addProp3(geneID, fr, b)
		}

		// Create cross-reference to RefSeq genomic accession
		// Strip version suffix (e.g., NC_000017.11 -> NC_000017)
		// RefSeq IDs are indexed by base accession
		if genomicAccession != "" {
			refseqID := genomicAccession
			if idx := strings.LastIndex(refseqID, "."); idx > 0 {
				refseqID = refseqID[:idx]
			}
			e.d.addXref(geneID, fr, refseqID, "refseq", false)
		}

		// Neighbor edges carry a machine-sortable, zero-padded genomic distance in
		// the EVIDENCE field (via SortLevelDistance) and the side in the RELATIONSHIP
		// field. The distance is encoded ascending (nearest == smallest string) so a
		// client/Atlas can rank neighbors nearest-first and take the closest N.
		//
		// NOTE on server-side sort: we intentionally do NOT route neighborentrez
		// through addXrefWithSortLevels. The queried (forward) direction lives in the
		// shared entrez/forward/ bucket alongside entrez->refseq/go/ensembl/... which
		// carry NO sort columns; enabling forward sort levels on entrez (the only knob
		// available, GetSortLevelCount("entrez")) would mis-strip those other edges and
		// corrupt the merge. The reverse (neighborentrez/from_entrez/) could be sorted,
		// but the link-view for `entrez >> neighborentrez` reads entrez's forward edges,
		// so reverse sorting would not affect the served order. Hence distance is kept
		// as evidence (consumer-sortable) rather than server-ordered. See report.

		// Process left neighbor genes (distance to the left neighbor)
		if leftGenesStr != "" && leftGenesStr != "-" {
			leftDist := encodeNeighborDistance(leftDistStr)
			leftGenes := strings.Split(leftGenesStr, "|")
			for _, neighborID := range leftGenes {
				neighborID = strings.TrimSpace(neighborID)
				if neighborID != "" && neighborID != "-" {
					e.d.addXrefFull(geneID, fr, neighborID, "neighborentrez", false, leftDist, "left_neighbor")
					leftNeighbors++
				}
			}
		}

		// Process right neighbor genes (distance to the right neighbor)
		if rightGenesStr != "" && rightGenesStr != "-" {
			rightDist := encodeNeighborDistance(rightDistStr)
			rightGenes := strings.Split(rightGenesStr, "|")
			for _, neighborID := range rightGenes {
				neighborID = strings.TrimSpace(neighborID)
				if neighborID != "" && neighborID != "-" {
					e.d.addXrefFull(geneID, fr, neighborID, "neighborentrez", false, rightDist, "right_neighbor")
					rightNeighbors++
				}
			}
		}

		// Process overlapping genes - distance 0 (closest possible), encoded so they
		// rank first when a consumer sorts neighbors by the evidence distance.
		if overlappingGenesStr != "" && overlappingGenesStr != "-" {
			overlapDist := ComputeSortLevelValue(SortLevelDistance, map[string]interface{}{"distance": int64(0)})
			overlappingGenes := strings.Split(overlappingGenesStr, "|")
			for _, neighborID := range overlappingGenes {
				neighborID = strings.TrimSpace(neighborID)
				if neighborID != "" && neighborID != "-" {
					e.d.addXrefFull(geneID, fr, neighborID, "neighborentrez", false, overlapDist, "overlapping")
					overlapping++
				}
			}
		}

		total++

		// Check test limit
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	log.Printf("[Entrez Gene] Processed %d gene neighbor entries:", total)
	log.Printf("[Entrez Gene]   - left_neighbor: %d cross-refs", leftNeighbors)
	log.Printf("[Entrez Gene]   - right_neighbor: %d cross-refs", rightNeighbors)
	log.Printf("[Entrez Gene]   - overlapping: %d cross-refs", overlapping)
}
