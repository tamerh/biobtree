package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// refseq handles NCBI RefSeq dataset
// Source: ftp.ncbi.nlm.nih.gov/genomes/refseq/ (multi-species GBFF/GPFF files)
// Contains curated reference sequences for transcripts and proteins
// Uses assembly_summary.txt to map taxids to assembly directories
type refseq struct {
	source string
	d      *DataUpdate
	// MANE data maps RefSeq accession to MANE info (human only)
	maneData map[string]*maneInfo
	// Genomic position data from gene2refseq.gz
	positionData map[string]*refseqPosition
	// Assembly paths maps taxid to list of assembly FTP paths
	assemblyPaths map[int][]string
	// Selected taxids for filtering (from --genome-taxids)
	selectedTaxids []int
}

// refseqPosition holds genomic position data from gene2refseq.gz
type refseqPosition struct {
	genomicAccession string // NC_000001.11
	start            int64
	end              int64
	orientation      string // + or -
}

// maneInfo holds MANE annotation data
type maneInfo struct {
	status            string // "MANE Select" or "MANE Plus Clinical"
	ensemblTranscript string // e.g., ENST00000263100.8
	ensemblProtein    string // e.g., ENSP00000263100.2
	hgncID            string // e.g., HGNC:5
	symbol            string // gene symbol
	geneID            string // e.g., GeneID:1
	chromosome        string // e.g., NC_000019.10
	start             int64
	end               int64
	strand            string
}

// gbffRecord represents a parsed GBFF/GPFF record
type gbffRecord struct {
	// Core identification
	accession   string // Full accession with version (from VERSION line)
	seqLength   int    // Sequence length
	molType     string // RNA, DNA, or aa
	description string // DEFINITION field
	status      string // RefSeq status from COMMENT (VALIDATED, REVIEWED, etc.)

	// Gene information
	symbol     string   // Gene symbol
	geneID     string   // Entrez Gene ID
	synonyms   []string // Gene synonyms
	chromosome string   // Chromosome
	mapLoc     string   // Map location

	// Taxonomy
	taxID    string
	organism string

	// Sequence type (mRNA, ncRNA, protein, etc.)
	seqType string

	// Cross-references
	hgncID    string
	ccdsID    string
	uniprotID string

	// Protein-specific
	proteinName    string
	molecularWt    float64
	codedBy        string // RNA accession for proteins
	proteinLength  int
	dbsourceRefseq string // DBSOURCE accession for proteins

	// Related accessions
	proteinAccession string
	rnaAccession     string
	genomicAccession string

	// Exon count
	exonCount int

	// Genomic position (from MANE or GBFF features)
	startPosition int64
	endPosition   int64
	orientation   string // + or -

	// MANE information (from KEYWORDS or COMMENT)
	isManeSelect      bool
	isManePlusClin    bool
	ensemblTranscript string
	ensemblProtein    string
}

// check provides context-aware error checking for refseq processor
func (r *refseq) check(err error, operation string) {
	checkWithContext(err, r.source, operation)
}

// getRefSeqType determines the type of RefSeq accession based on its prefix
func getRefSeqType(accession string) string {
	if len(accession) < 3 {
		return ""
	}
	prefix := accession[:3]
	switch prefix {
	case "NM_":
		return "mRNA"
	case "NR_":
		return "ncRNA"
	case "NP_":
		return "protein"
	case "NC_":
		return "genomic_chromosome"
	case "NG_":
		return "genomic_region"
	case "NT_":
		return "genomic_contig"
	case "NW_":
		return "genomic_wgs"
	case "NZ_":
		return "genomic_wgs_master"
	case "XM_":
		return "predicted_mRNA"
	case "XR_":
		return "predicted_ncRNA"
	case "XP_":
		return "predicted_protein"
	case "WP_":
		return "protein_nonredundant"
	case "YP_":
		return "protein_organelle"
	case "AP_":
		return "protein_annotated"
	default:
		return "other"
	}
}

func (r *refseq) update(selectedTaxids []int) {
	fr := config.Dataconf[r.source]["id"]
	basePath := config.Dataconf[r.source]["path"]
	speciesGroupsStr := config.Dataconf[r.source]["speciesGroups"]
	manePath := config.Dataconf[r.source]["manePath"]

	defer r.d.wg.Done()

	r.selectedTaxids = selectedTaxids

	// Test mode support
	testLimit := config.GetTestLimit(r.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, r.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	start := time.Now()

	// Load MANE data first (enrichment for MANE Select/Plus Clinical flags, human only)
	r.maneData = make(map[string]*maneInfo)
	if manePath != "" {
		r.loadMANEData(manePath)
		log.Printf("[RefSeq] Loaded %d MANE annotations (human only)", len(r.maneData))
	}

	// Initialize position data map (will be populated from genomic GBFF)
	r.positionData = make(map[string]*refseqPosition)

	// Parse species groups to process
	speciesGroups := strings.Split(speciesGroupsStr, ",")
	if len(speciesGroups) == 0 || (len(speciesGroups) == 1 && speciesGroups[0] == "") {
		// Default to vertebrate_mammalian if not specified
		speciesGroups = []string{"vertebrate_mammalian"}
	}

	// Build taxid -> assembly paths mapping from assembly_summary.txt files
	r.assemblyPaths = make(map[int][]string)
	for _, group := range speciesGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		summaryURL := basePath + group + "/assembly_summary.txt"
		r.loadAssemblySummary(summaryURL, group)
	}

	log.Printf("[RefSeq] Loaded assembly paths for %d taxids", len(r.assemblyPaths))

	// Filter by selected taxids if provided
	var taxidsToProcess []int
	if len(r.selectedTaxids) > 0 {
		for _, taxid := range r.selectedTaxids {
			if _, ok := r.assemblyPaths[taxid]; ok {
				taxidsToProcess = append(taxidsToProcess, taxid)
			}
		}
		log.Printf("[RefSeq] Filtering to %d selected taxids", len(taxidsToProcess))
	} else {
		for taxid := range r.assemblyPaths {
			taxidsToProcess = append(taxidsToProcess, taxid)
		}
		log.Printf("[RefSeq] Processing all %d taxids", len(taxidsToProcess))
	}

	if len(taxidsToProcess) == 0 {
		log.Panic("[RefSeq] No assemblies found for selected taxids")
	}

	// Phase 1: Load genomic positions from genomic GBFF files
	// This extracts transcript_id -> genomic position mappings
	for _, taxid := range taxidsToProcess {
		paths := r.assemblyPaths[taxid]
		for _, assemblyPath := range paths {
			parts := strings.Split(strings.TrimSuffix(assemblyPath, "/"), "/")
			assemblyName := parts[len(parts)-1]

			// Load genomic GBFF for position data
			genomicURL := assemblyPath + "/" + assemblyName + "_genomic.gbff.gz"
			r.loadGenomicPositions(genomicURL)
		}
	}
	log.Printf("[RefSeq] Loaded %d genomic positions from genomic GBFF", len(r.positionData))

	// Phase 2: Process RNA and protein GBFF files (using genomic positions for enrichment)
	for _, taxid := range taxidsToProcess {
		paths := r.assemblyPaths[taxid]
		for _, assemblyPath := range paths {
			// Extract assembly name from path (last component)
			// e.g., https://ftp.ncbi.nlm.nih.gov/genomes/all/GCF/.../GCF_000001405.40_GRCh38.p14
			// -> GCF_000001405.40_GRCh38.p14
			parts := strings.Split(strings.TrimSuffix(assemblyPath, "/"), "/")
			assemblyName := parts[len(parts)-1]

			// Process RNA GBFF file
			// File naming: {assembly_name}_rna.gbff.gz
			rnaURL := assemblyPath + "/" + assemblyName + "_rna.gbff.gz"
			count := r.processGBFFFile(rnaURL, fr, false, testLimit, int(total), idLogFile)
			total += uint64(count)

			if shouldStopProcessing(testLimit, int(total)) {
				break
			}

			// Process Protein GPFF file
			// File naming: {assembly_name}_protein.gpff.gz
			proteinURL := assemblyPath + "/" + assemblyName + "_protein.gpff.gz"
			count = r.processGBFFFile(proteinURL, fr, true, testLimit, int(total), idLogFile)
			total += uint64(count)

			if shouldStopProcessing(testLimit, int(total)) {
				break
			}
		}

		if shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	elapsed := time.Since(start)
	atomic.AddUint64(&r.d.totalParsedEntry, total)

	log.Printf("[RefSeq] Processed %d entries in %v", total, elapsed)

	// Signal completion
	r.d.progChan <- &progressInfo{dataset: r.source, done: true}
}

// loadAssemblySummary parses assembly_summary.txt to build taxid -> assembly path mapping
func (r *refseq) loadAssemblySummary(summaryURL, group string) {
	log.Printf("[RefSeq] Loading assembly summary from %s", summaryURL)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", summaryURL)
	if err != nil {
		log.Panicf("[RefSeq] Could not load assembly summary for %s: %v", group, err)
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	// Increase buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	count := 0
	for scanner.Scan() {
		line := scanner.Text()

		// Skip header lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 20 {
			continue
		}

		// Column 6 = taxid, Column 20 = ftp_path
		taxidStr := fields[5]
		ftpPath := fields[19]

		if taxidStr == "" || ftpPath == "" || ftpPath == "na" {
			continue
		}

		taxid, err := strconv.Atoi(taxidStr)
		if err != nil {
			continue
		}

		// Keep FTP path as-is (FTP is more reliable for NCBI)
		// Path format: ftp://ftp.ncbi.nlm.nih.gov/genomes/all/GCF/...

		r.assemblyPaths[taxid] = append(r.assemblyPaths[taxid], ftpPath)
		count++
	}

	log.Printf("[RefSeq] Loaded %d assemblies from %s", count, group)
}

// loadMANEData loads MANE summary file for enrichment
func (r *refseq) loadMANEData(manePath string) {
	log.Printf("[RefSeq] Loading MANE data from %s", manePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", manePath)
	if err != nil {
		log.Panicf("[RefSeq] Could not load MANE data: %v", err)
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	lineNo := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNo++

		// Skip header
		if strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 14 {
			continue
		}

		// Parse fields
		geneID := fields[0]          // GeneID:1
		hgncID := fields[2]          // HGNC:5
		symbol := fields[3]          // A1BG
		refseqNuc := fields[5]       // NM_130786.4
		refseqProt := fields[6]      // NP_570602.2
		ensemblNuc := fields[7]      // ENST00000263100.8
		ensemblProt := fields[8]     // ENSP00000263100.2
		maneStatus := fields[9]      // MANE Select / MANE Plus Clinical
		chromosome := fields[10]     // NC_000019.10
		startStr := fields[11]       // 58345183
		endStr := fields[12]         // 58353492
		strand := fields[13]         // + or -

		start, _ := strconv.ParseInt(startStr, 10, 64)
		end, _ := strconv.ParseInt(endStr, 10, 64)

		info := &maneInfo{
			status:            maneStatus,
			ensemblTranscript: ensemblNuc,
			ensemblProtein:    ensemblProt,
			hgncID:            hgncID,
			symbol:            symbol,
			geneID:            geneID,
			chromosome:        chromosome,
			start:             start,
			end:               end,
			strand:            strand,
		}

		// Map both RNA and protein accessions
		if refseqNuc != "" {
			r.maneData[refseqNuc] = info
		}
		if refseqProt != "" {
			r.maneData[refseqProt] = info
		}
	}

	log.Printf("[RefSeq] Loaded %d MANE entries", len(r.maneData))
}

// loadGenomicPositions extracts transcript positions from genomic GBFF
// Parses mRNA/misc_RNA/CDS features to get transcript_id -> genomic position mapping
func (r *refseq) loadGenomicPositions(genomicURL string) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", genomicURL)
	if err != nil {
		// Genomic files may not exist for all assemblies - this is expected
		return
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024) // Larger buffer for genomic files

	var currentChromosome string
	var inFeatures bool
	var currentFeature string
	var featureLocation string
	var featureContent strings.Builder
	count := 0

	for scanner.Scan() {
		line := scanner.Text()

		// Track chromosome from VERSION line
		if strings.HasPrefix(line, "VERSION") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				currentChromosome = fields[1]
			}
			continue
		}

		// Detect FEATURES section
		if strings.HasPrefix(line, "FEATURES") {
			inFeatures = true
			continue
		}
		if strings.HasPrefix(line, "ORIGIN") || strings.HasPrefix(line, "CONTIG") || line == "//" {
			// End of features section or record
			if currentFeature != "" {
				r.extractTranscriptPosition(currentFeature, featureLocation, featureContent.String(), currentChromosome, &count)
			}
			inFeatures = false
			currentFeature = ""
			featureLocation = ""
			featureContent.Reset()
			if line == "//" {
				// End of record, reset chromosome for next record
				currentChromosome = ""
			}
			continue
		}

		if !inFeatures {
			continue
		}

		// Parse features - look for mRNA, misc_RNA, CDS with transcript_id or protein_id
		// Feature lines start at column 5 with feature type
		if len(line) > 5 && line[0] == ' ' && line[4] == ' ' && line[5] != ' ' {
			// New feature - process previous one
			if currentFeature != "" {
				r.extractTranscriptPosition(currentFeature, featureLocation, featureContent.String(), currentChromosome, &count)
			}

			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentFeature = parts[0]
				featureLocation = parts[1]
				featureContent.Reset()
			}
		} else if len(line) > 21 && strings.HasPrefix(line, "                     ") {
			// Qualifier line (continuation)
			featureContent.WriteString(strings.TrimSpace(line))
			featureContent.WriteString(" ")
		}
	}

	log.Printf("[RefSeq] Extracted %d positions from %s", count, genomicURL)
}

// extractTranscriptPosition extracts transcript/protein ID and position from a feature
func (r *refseq) extractTranscriptPosition(featureType, location, content, chromosome string, count *int) {
	// Only process mRNA, misc_RNA, ncRNA, CDS features
	if featureType != "mRNA" && featureType != "misc_RNA" && featureType != "ncRNA" && featureType != "CDS" && featureType != "rRNA" && featureType != "tRNA" {
		return
	}

	// Extract transcript_id or protein_id
	var accession string
	if match := regexp.MustCompile(`/transcript_id="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
		accession = match[1]
	} else if match := regexp.MustCompile(`/protein_id="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
		accession = match[1]
	}

	if accession == "" {
		return
	}

	// Parse location to get start, end, and strand
	start, end, orientation := parseGenomicLocation(location)
	if start == 0 || end == 0 {
		return
	}

	// Store position data
	r.positionData[accession] = &refseqPosition{
		genomicAccession: chromosome,
		start:            start,
		end:              end,
		orientation:      orientation,
	}
	*count++
}

// parseGenomicLocation parses GBFF location format to extract start, end, strand
// Handles formats like: 11874..14409, complement(14362..29370), join(1..100,200..300)
func parseGenomicLocation(location string) (start, end int64, orientation string) {
	orientation = "+"

	// Check for complement (minus strand)
	if strings.HasPrefix(location, "complement(") {
		orientation = "-"
		location = strings.TrimPrefix(location, "complement(")
		location = strings.TrimSuffix(location, ")")
	}

	// Handle join() - extract overall range
	if strings.HasPrefix(location, "join(") {
		location = strings.TrimPrefix(location, "join(")
		location = strings.TrimSuffix(location, ")")
	}

	// Find all numbers in the location
	re := regexp.MustCompile(`(\d+)`)
	matches := re.FindAllString(location, -1)

	if len(matches) >= 2 {
		start, _ = strconv.ParseInt(matches[0], 10, 64)
		end, _ = strconv.ParseInt(matches[len(matches)-1], 10, 64)

		// Ensure start < end
		if start > end {
			start, end = end, start
		}
	}

	return
}

// processGBFFFile processes a single GBFF/GPFF file
// RNA files (_rna.gbff.gz) are optional - many prokaryotic genomes don't have separate RNA annotations
func (r *refseq) processGBFFFile(fileURL, fr string, isProtein bool, testLimit, currentCount int, idLogFile *os.File) int {
	log.Printf("[RefSeq] Processing: %s", fileURL)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", fileURL)
	if err != nil {
		// RNA files are optional - many genomes (especially prokaryotes) don't have them
		if !isProtein {
			log.Printf("[RefSeq] Skipped (not available): %s", fileURL)
			return 0
		}
		log.Printf("[RefSeq] Error opening %s: %v", fileURL, err)
		return 0
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	count := 0
	var recordLines []string
	scanner := bufio.NewScanner(br)

	// Increase scanner buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "//" {
			// End of record - process it
			if len(recordLines) > 0 {
				record := r.parseGBFFRecord(recordLines, isProtein)
				if record != nil && record.accession != "" {
					r.processRecord(record, fr)
					count++

					// Log ID in test mode
					if idLogFile != nil {
						logProcessedID(idLogFile, record.accession)
					}

					// Check test limit
					if shouldStopProcessing(testLimit, currentCount+count) {
						return count
					}
				}
				recordLines = nil
			}
		} else {
			recordLines = append(recordLines, line)
		}
	}

	// Process last record if file doesn't end with //
	if len(recordLines) > 0 {
		record := r.parseGBFFRecord(recordLines, isProtein)
		if record != nil && record.accession != "" {
			r.processRecord(record, fr)
			count++
		}
	}

	log.Printf("[RefSeq] Processed %d records from %s", count, fileURL)
	return count
}

// parseGBFFRecord parses a single GBFF/GPFF record
func (r *refseq) parseGBFFRecord(lines []string, isProtein bool) *gbffRecord {
	record := &gbffRecord{}

	var inFeatures, inComment bool
	var currentFeature string
	var featureContent strings.Builder
	var commentContent strings.Builder

	for _, line := range lines {
		// Detect section transitions
		if strings.HasPrefix(line, "FEATURES") {
			inFeatures = true
			inComment = false
			continue
		}
		if strings.HasPrefix(line, "COMMENT") {
			inComment = true
			inFeatures = false
			commentContent.WriteString(strings.TrimPrefix(line, "COMMENT"))
			commentContent.WriteString(" ")
			continue
		}
		if strings.HasPrefix(line, "ORIGIN") || strings.HasPrefix(line, "CONTIG") {
			inFeatures = false
			inComment = false
			continue
		}

		// Parse header section
		if !inFeatures && !inComment {
			r.parseHeaderLine(line, record, isProtein)
		}

		// Accumulate comment
		if inComment {
			if len(line) > 12 && line[0] == ' ' {
				commentContent.WriteString(strings.TrimSpace(line))
				commentContent.WriteString(" ")
			}
		}

		// Parse features section
		if inFeatures {
			// Check for new feature (starts at column 5)
			if len(line) > 5 && line[0] == ' ' && line[4] == ' ' && line[5] != ' ' {
				// Process previous feature
				if currentFeature != "" {
					r.parseFeature(currentFeature, featureContent.String(), record, isProtein)
				}
				// Start new feature
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					currentFeature = parts[0]
					featureContent.Reset()
					featureContent.WriteString(strings.Join(parts[1:], " "))
					featureContent.WriteString(" ")
				}
			} else if len(line) > 21 && strings.HasPrefix(line, "                     ") {
				// Continuation of feature (qualifier lines start at column 21)
				featureContent.WriteString(strings.TrimSpace(line))
				featureContent.WriteString(" ")
			}
		}
	}

	// Process last feature
	if currentFeature != "" {
		r.parseFeature(currentFeature, featureContent.String(), record, isProtein)
	}

	// Parse accumulated comment for status and MANE info
	r.parseComment(commentContent.String(), record)

	// Determine sequence type
	if record.seqType == "" {
		record.seqType = getRefSeqType(record.accession)
	}

	// Enrich with MANE data if available
	if mane, ok := r.maneData[record.accession]; ok {
		record.isManeSelect = mane.status == "MANE Select"
		record.isManePlusClin = mane.status == "MANE Plus Clinical"
		record.ensemblTranscript = mane.ensemblTranscript
		record.ensemblProtein = mane.ensemblProtein
		if record.hgncID == "" {
			record.hgncID = mane.hgncID
		}
		if record.symbol == "" {
			record.symbol = mane.symbol
		}
		if record.geneID == "" {
			record.geneID = strings.TrimPrefix(mane.geneID, "GeneID:")
		}
		if record.genomicAccession == "" {
			record.genomicAccession = mane.chromosome
		}
		// Copy genomic positions from MANE
		if record.startPosition == 0 && mane.start > 0 {
			record.startPosition = mane.start
		}
		if record.endPosition == 0 && mane.end > 0 {
			record.endPosition = mane.end
		}
		if record.orientation == "" && mane.strand != "" {
			record.orientation = mane.strand
		}
	}

	// Fallback: Enrich with position data from genomic GBFF (if not already set by MANE)
	if pos, ok := r.positionData[record.accession]; ok {
		if record.startPosition == 0 && pos.start > 0 {
			record.startPosition = pos.start
		}
		if record.endPosition == 0 && pos.end > 0 {
			record.endPosition = pos.end
		}
		if record.orientation == "" && pos.orientation != "" {
			record.orientation = pos.orientation
		}
		if record.genomicAccession == "" && pos.genomicAccession != "" {
			record.genomicAccession = pos.genomicAccession
		}
	}

	return record
}

// parseHeaderLine parses LOCUS, DEFINITION, VERSION, etc.
func (r *refseq) parseHeaderLine(line string, record *gbffRecord, isProtein bool) {
	if strings.HasPrefix(line, "LOCUS") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			// Parse length
			lenStr := fields[2]
			record.seqLength, _ = strconv.Atoi(lenStr)
			// Parse molecule type
			for _, f := range fields {
				if f == "aa" {
					record.molType = "protein"
				} else if f == "RNA" || f == "mRNA" {
					record.molType = "RNA"
				} else if f == "DNA" {
					record.molType = "DNA"
				}
			}
		}
	} else if strings.HasPrefix(line, "DEFINITION") {
		record.description = strings.TrimSpace(strings.TrimPrefix(line, "DEFINITION"))
	} else if strings.HasPrefix(line, "            ") && record.description != "" && record.accession == "" {
		// Continuation of DEFINITION
		record.description += " " + strings.TrimSpace(line)
	} else if strings.HasPrefix(line, "VERSION") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			record.accession = fields[1]
		}
	} else if strings.HasPrefix(line, "DBSOURCE") {
		// For proteins, this contains the reference to RNA
		match := regexp.MustCompile(`accession\s+(\S+)`).FindStringSubmatch(line)
		if len(match) > 1 {
			record.dbsourceRefseq = match[1]
			record.rnaAccession = match[1]
		}
	} else if strings.HasPrefix(line, "KEYWORDS") {
		keywords := strings.TrimSpace(strings.TrimPrefix(line, "KEYWORDS"))
		if strings.Contains(keywords, "MANE Select") {
			record.isManeSelect = true
		}
		if strings.Contains(keywords, "MANE Plus Clinical") {
			record.isManePlusClin = true
		}
		if strings.Contains(keywords, "RefSeq Select") {
			record.status = "VALIDATED"
		}
	}
}

// parseFeature parses a single feature block
func (r *refseq) parseFeature(featureType, content string, record *gbffRecord, isProtein bool) {
	switch featureType {
	case "source":
		// Extract organism and taxonomy
		if match := regexp.MustCompile(`/organism="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.organism = match[1]
		}
		if match := regexp.MustCompile(`/db_xref="taxon:(\d+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.taxID = match[1]
		}
		if match := regexp.MustCompile(`/chromosome="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.chromosome = match[1]
		}
		if match := regexp.MustCompile(`/map="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.mapLoc = match[1]
		}
		if match := regexp.MustCompile(`/mol_type="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			molType := match[1]
			if strings.Contains(molType, "mRNA") {
				record.seqType = "mRNA"
			} else if strings.Contains(molType, "RNA") {
				record.seqType = "ncRNA"
			}
		}

	case "gene":
		// Extract gene information
		if match := regexp.MustCompile(`/gene="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.symbol = match[1]
		}
		if match := regexp.MustCompile(`/db_xref="GeneID:(\d+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.geneID = match[1]
		}
		if match := regexp.MustCompile(`/db_xref="HGNC:([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			// HGNC db_xref format is "HGNC:HGNC:11998" - capture already includes "HGNC:" prefix
			record.hgncID = match[1]
		}
		// Extract synonyms
		synonymMatches := regexp.MustCompile(`/gene_synonym="([^"]+)"`).FindAllStringSubmatch(content, -1)
		for _, m := range synonymMatches {
			if len(m) > 1 {
				// Synonyms may be semicolon-separated
				syns := strings.Split(m[1], "; ")
				record.synonyms = append(record.synonyms, syns...)
			}
		}

	case "CDS":
		// Extract coding sequence info (for RNA) or protein info
		if match := regexp.MustCompile(`/db_xref="GeneID:(\d+)"`).FindStringSubmatch(content); len(match) > 1 {
			if record.geneID == "" {
				record.geneID = match[1]
			}
		}
		if match := regexp.MustCompile(`/db_xref="CCDS:([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.ccdsID = match[1]
		}
		if match := regexp.MustCompile(`/db_xref="HGNC:([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			// HGNC db_xref format is "HGNC:HGNC:11998" - capture already includes "HGNC:" prefix
			record.hgncID = match[1]
		}
		if match := regexp.MustCompile(`/protein_id="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.proteinAccession = match[1]
		}
		if match := regexp.MustCompile(`/coded_by="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.codedBy = match[1]
			// Extract RNA accession from coded_by (e.g., "NM_001368254.1:47..1195")
			if idx := strings.Index(record.codedBy, ":"); idx > 0 {
				record.rnaAccession = record.codedBy[:idx]
			}
		}
		if match := regexp.MustCompile(`/gene="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			if record.symbol == "" {
				record.symbol = match[1]
			}
		}
		// Extract UniProt cross-reference
		// Format in RefSeq: "UniProtKB/Swiss-Prot (P01023.3)" or "UniProtKB:P45952"
		if record.uniprotID == "" {
			// Format: UniProtKB/Swiss-Prot (ACCESSION.VERSION) or UniProtKB/TrEMBL (ACCESSION.VERSION)
			if match := regexp.MustCompile(`UniProtKB/(?:Swiss-Prot|TrEMBL)\s+\(([A-Z0-9]+)(?:\.\d+)?\)`).FindStringSubmatch(content); len(match) > 1 {
				record.uniprotID = match[1]
			} else if match := regexp.MustCompile(`UniProtKB:([A-Z0-9]+)`).FindStringSubmatch(content); len(match) > 1 {
				// Format: UniProtKB:ACCESSION (in evidence strings)
				record.uniprotID = match[1]
			}
		}

	case "Protein":
		// Protein-specific attributes
		if match := regexp.MustCompile(`/product="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			record.proteinName = match[1]
		}
		if match := regexp.MustCompile(`/calculated_mol_wt=(\d+)`).FindStringSubmatch(content); len(match) > 1 {
			record.molecularWt, _ = strconv.ParseFloat(match[1], 64)
		}
		// Parse location for length
		if match := regexp.MustCompile(`^(\d+)\.\.(\d+)`).FindStringSubmatch(content); len(match) > 2 {
			start, _ := strconv.Atoi(match[1])
			end, _ := strconv.Atoi(match[2])
			record.proteinLength = end - start + 1
		}
		// Extract UniProt cross-reference from protein features
		if record.uniprotID == "" {
			if match := regexp.MustCompile(`UniProtKB/(?:Swiss-Prot|TrEMBL)\s+\(([A-Z0-9]+)(?:\.\d+)?\)`).FindStringSubmatch(content); len(match) > 1 {
				record.uniprotID = match[1]
			} else if match := regexp.MustCompile(`UniProtKB:([A-Z0-9]+)`).FindStringSubmatch(content); len(match) > 1 {
				record.uniprotID = match[1]
			}
		}

	case "mRNA", "ncRNA", "misc_RNA", "rRNA":
		if match := regexp.MustCompile(`/product="([^"]+)"`).FindStringSubmatch(content); len(match) > 1 {
			if record.description == "" {
				record.description = match[1]
			}
		}

	case "exon":
		record.exonCount++

	case "misc_feature":
		// Extract UniProt cross-reference from misc_feature (most common location)
		// Format: "propagated from UniProtKB/Swiss-Prot (P01023.3)"
		if record.uniprotID == "" {
			if match := regexp.MustCompile(`UniProtKB/(?:Swiss-Prot|TrEMBL)\s+\(([A-Z0-9]+)(?:\.\d+)?\)`).FindStringSubmatch(content); len(match) > 1 {
				record.uniprotID = match[1]
			}
		}
	}
}

// parseComment extracts status and MANE info from COMMENT section
func (r *refseq) parseComment(comment string, record *gbffRecord) {
	// Extract status
	if strings.Contains(comment, "VALIDATED REFSEQ") {
		record.status = "VALIDATED"
	} else if strings.Contains(comment, "REVIEWED REFSEQ") {
		record.status = "REVIEWED"
	} else if strings.Contains(comment, "PROVISIONAL REFSEQ") {
		record.status = "PROVISIONAL"
	} else if strings.Contains(comment, "PREDICTED REFSEQ") || strings.Contains(comment, "MODEL REFSEQ") {
		record.status = "PREDICTED"
	}

	// Extract MANE Ensembl match
	if match := regexp.MustCompile(`MANE Ensembl match\s*::\s*(\S+)/\s*(\S+)`).FindStringSubmatch(comment); len(match) > 2 {
		record.ensemblTranscript = match[1]
		record.ensemblProtein = match[2]
		record.isManeSelect = true
	}
}

// processRecord creates index entries for a parsed record
func (r *refseq) processRecord(record *gbffRecord, fr string) {
	// Clean description (remove trailing periods, normalize whitespace)
	record.description = strings.TrimSpace(record.description)
	record.description = strings.TrimSuffix(record.description, ".")

	// Create attributes
	attr := pbuf.RefSeqAttr{
		Accession:         record.accession,
		Type:              record.seqType,
		Status:            record.status,
		Symbol:            record.symbol,
		Description:       record.description,
		Synonyms:          record.synonyms,
		Chromosome:        record.chromosome,
		StartPosition:     record.startPosition,
		EndPosition:       record.endPosition,
		Orientation:       record.orientation,
		GenomicAccession:  record.genomicAccession,
		ProteinAccession:  record.proteinAccession,
		RnaAccession:      record.rnaAccession,
		IsManeSelect:      record.isManeSelect,
		IsManePlusClinical: record.isManePlusClin,
		EnsemblTranscript: record.ensemblTranscript,
		EnsemblProtein:    record.ensemblProtein,
		HgncId:            record.hgncID,
		CcdsId:            record.ccdsID,
		UniprotId:         record.uniprotID,
		ProteinLength:     int32(record.proteinLength),
		MolecularWeight:   record.molecularWt,
		ProteinName:       record.proteinName,
		Organism:          record.organism,
		SeqLength:         int32(record.seqLength),
		MolType:           record.molType,
		ExonCount:         int32(record.exonCount),
	}

	// Parse taxonomy ID
	if record.taxID != "" {
		if taxid, err := strconv.Atoi(record.taxID); err == nil {
			attr.Taxid = int32(taxid)
		}
	}

	b, _ := ffjson.Marshal(&attr)
	r.d.addProp3(record.accession, fr, b)

	// Add text search for symbol
	if record.symbol != "" {
		r.d.addXref(record.symbol, textLinkID, record.accession, r.source, true)
	}

	// Add text search for gene synonyms
	for _, syn := range record.synonyms {
		if syn != "" {
			r.d.addXref(syn, textLinkID, record.accession, r.source, true)
		}
	}

	// Create cross-reference to Entrez Gene
	if record.geneID != "" {
		r.d.addXref(record.accession, fr, record.geneID, "entrez", false)
	}

	// Create cross-reference to taxonomy
	if record.taxID != "" {
		r.d.addXref(record.accession, fr, record.taxID, "taxonomy", false)
	}

	// Create cross-reference to Ensembl via HGNC lookup (requires lookup DB)
	// This links RefSeq → Ensembl by looking up gene symbol → HGNC → Ensembl
	if record.symbol != "" {
		r.d.addXrefEnsemblViaHgnc(record.symbol, record.accession, fr)
	}

	// Create cross-reference to Ensembl transcript
	if record.ensemblTranscript != "" {
		// Remove version for cross-reference
		ensemblID := record.ensemblTranscript
		if idx := strings.Index(ensemblID, "."); idx > 0 {
			ensemblID = ensemblID[:idx]
		}
		r.d.addXref(record.accession, fr, ensemblID, "ensembl", false)
	}

	// Create cross-reference to Ensembl protein
	if record.ensemblProtein != "" {
		ensemblID := record.ensemblProtein
		if idx := strings.Index(ensemblID, "."); idx > 0 {
			ensemblID = ensemblID[:idx]
		}
		r.d.addXref(record.accession, fr, ensemblID, "ensembl", false)
	}

	// Create cross-references between RefSeq RNA and protein
	if record.proteinAccession != "" && record.proteinAccession != record.accession {
		r.d.addXref(record.accession, fr, record.proteinAccession, r.source, false)
	}
	if record.rnaAccession != "" && record.rnaAccession != record.accession {
		r.d.addXref(record.accession, fr, record.rnaAccession, r.source, false)
	}

	// Cross-reference to CCDS
	if record.ccdsID != "" {
		r.d.addXref(record.accession, fr, record.ccdsID, "ccds", false)
	}

	// Cross-reference to UniProt
	if record.uniprotID != "" {
		r.d.addXref(record.accession, fr, record.uniprotID, "uniprot", false)
	}
}
