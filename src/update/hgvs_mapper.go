package update

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"biobtree/pbuf"
)

// HGVSMapper computes HGVS nomenclature for variants using RefSeq GFF3 data
type HGVSMapper struct {
	// geneTranscripts maps gene_symbol -> list of transcripts
	geneTranscripts map[string][]*TranscriptInfo

	// transcriptsByID maps transcript_id -> TranscriptInfo
	transcriptsByID map[string]*TranscriptInfo

	// chromosomeTranscripts maps chr:start-end intervals -> transcripts for position-based lookup
	// This is used when we need to find all transcripts overlapping a position
	chromosomeTranscripts map[string][]*TranscriptInfo

	loaded bool
	mu     sync.RWMutex
}

// TranscriptInfo holds all information needed to compute HGVS for a transcript
type TranscriptInfo struct {
	TranscriptID       string   // e.g., "NM_001005484.2"
	GeneSymbol         string   // e.g., "OR4F5"
	GeneID             string   // Entrez Gene ID
	Chromosome         string   // e.g., "NC_000001.11"
	Strand             string   // "+" or "-"
	TxStart            int64    // Transcript start (genomic)
	TxEnd              int64    // Transcript end (genomic)
	CdsStart           int64    // CDS start (genomic), 0 if non-coding
	CdsEnd             int64    // CDS end (genomic), 0 if non-coding
	Exons              []Exon   // Exon coordinates (sorted by genomic position)
	IsMANESelect       bool     // MANE Select transcript
	IsMANEPlusClinical bool     // MANE Plus Clinical transcript
	ProteinID          string   // e.g., "NP_001005484.2"
}

// Exon represents an exon with genomic coordinates
type Exon struct {
	Start int64 // Genomic start (1-based)
	End   int64 // Genomic end (1-based, inclusive)
	Number int  // Exon number (1-based)
}

// GFF3 URL for human RefSeq annotation
const RefSeqGFF3URL = "https://ftp.ncbi.nlm.nih.gov/genomes/all/annotation_releases/9606/GCF_000001405.40-RS_2024_08/GCF_000001405.40_GRCh38.p14_genomic.gff.gz"

// NewHGVSMapper creates a new HGVS mapper instance
func NewHGVSMapper() *HGVSMapper {
	return &HGVSMapper{
		geneTranscripts:       make(map[string][]*TranscriptInfo),
		transcriptsByID:       make(map[string]*TranscriptInfo),
		chromosomeTranscripts: make(map[string][]*TranscriptInfo),
	}
}

// Load downloads and parses the RefSeq GFF3 file
func (m *HGVSMapper) Load(cacheDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loaded {
		return nil
	}

	// Check for cached file
	cacheFile := filepath.Join(cacheDir, "refseq_grch38.gff.gz")

	var reader io.Reader
	var err error

	if _, err = os.Stat(cacheFile); err == nil {
		// Use cached file
		log.Printf("HGVS: Loading cached GFF3 from %s", cacheFile)
		f, err := os.Open(cacheFile)
		if err != nil {
			return fmt.Errorf("failed to open cached GFF3: %v", err)
		}
		defer f.Close()

		gzReader, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %v", err)
		}
		defer gzReader.Close()
		reader = gzReader
	} else {
		// Download from NCBI
		log.Printf("HGVS: Downloading RefSeq GFF3 from %s", RefSeqGFF3URL)

		resp, err := http.Get(RefSeqGFF3URL)
		if err != nil {
			return fmt.Errorf("failed to download GFF3: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download GFF3: HTTP %d", resp.StatusCode)
		}

		// Save to cache while parsing
		os.MkdirAll(cacheDir, 0755)
		cacheWriter, err := os.Create(cacheFile)
		if err != nil {
			log.Printf("HGVS: Warning - could not create cache file: %v", err)
			// Continue without caching
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to create gzip reader: %v", err)
			}
			defer gzReader.Close()
			reader = gzReader
		} else {
			defer cacheWriter.Close()
			// Tee the response to both cache file and parser
			teeReader := io.TeeReader(resp.Body, cacheWriter)
			gzReader, err := gzip.NewReader(teeReader)
			if err != nil {
				return fmt.Errorf("failed to create gzip reader: %v", err)
			}
			defer gzReader.Close()
			reader = gzReader
		}
	}

	// Parse the GFF3
	err = m.parseGFF3(reader)
	if err != nil {
		return fmt.Errorf("failed to parse GFF3: %v", err)
	}

	m.loaded = true
	log.Printf("HGVS: Loaded %d genes, %d transcripts", len(m.geneTranscripts), len(m.transcriptsByID))

	return nil
}

// parseGFF3 parses the RefSeq GFF3 file and builds the transcript database
func (m *HGVSMapper) parseGFF3(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	// Increase buffer size for long lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	// Temporary storage for building transcripts
	transcripts := make(map[string]*TranscriptInfo)  // transcript_id -> info
	exonsByTranscript := make(map[string][]Exon)     // transcript_id -> exons
	cdsByTranscript := make(map[string][]Exon)       // transcript_id -> CDS regions

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Parse GFF3 line
		fields := strings.Split(line, "\t")
		if len(fields) < 9 {
			continue
		}

		chrom := fields[0]
		featureType := fields[2]
		startStr := fields[3]
		endStr := fields[4]
		strand := fields[6]
		attributes := fields[8]

		// Only process main chromosomes (NC_*)
		if !strings.HasPrefix(chrom, "NC_") {
			continue
		}

		start, _ := strconv.ParseInt(startStr, 10, 64)
		end, _ := strconv.ParseInt(endStr, 10, 64)

		// Parse attributes
		attrMap := parseGFF3Attributes(attributes)

		switch featureType {
		case "mRNA", "transcript", "primary_transcript", "lnc_RNA":
			// This is a transcript record
			transcriptID := extractTranscriptID(attrMap["ID"])
			if transcriptID == "" || !strings.HasPrefix(transcriptID, "NM_") {
				// Only process NM_ transcripts for now (protein-coding mRNAs)
				// Could extend to NR_ for non-coding
				continue
			}

			geneSymbol := attrMap["gene"]
			geneID := ""
			if dbxref, ok := attrMap["Dbxref"]; ok {
				for _, ref := range strings.Split(dbxref, ",") {
					if strings.HasPrefix(ref, "GeneID:") {
						geneID = strings.TrimPrefix(ref, "GeneID:")
						break
					}
				}
			}

			isMANE := strings.Contains(attrMap["tag"], "MANE Select")
			isMANEClinical := strings.Contains(attrMap["tag"], "MANE Plus Clinical")

			tx := &TranscriptInfo{
				TranscriptID:       transcriptID,
				GeneSymbol:         geneSymbol,
				GeneID:             geneID,
				Chromosome:         chrom,
				Strand:             strand,
				TxStart:            start,
				TxEnd:              end,
				IsMANESelect:       isMANE,
				IsMANEPlusClinical: isMANEClinical,
			}
			transcripts[transcriptID] = tx

		case "exon":
			// Exon record
			parent := attrMap["Parent"]
			transcriptID := extractTranscriptID(parent)
			if transcriptID == "" || !strings.HasPrefix(transcriptID, "NM_") {
				continue
			}

			// Extract exon number from ID if available
			exonNum := 0
			if id, ok := attrMap["ID"]; ok {
				// Format: exon-NM_001005484.2-1
				parts := strings.Split(id, "-")
				if len(parts) >= 3 {
					exonNum, _ = strconv.Atoi(parts[len(parts)-1])
				}
			}

			exon := Exon{Start: start, End: end, Number: exonNum}
			exonsByTranscript[transcriptID] = append(exonsByTranscript[transcriptID], exon)

		case "CDS":
			// CDS record (coding sequence)
			parent := attrMap["Parent"]
			transcriptID := extractTranscriptID(parent)
			if transcriptID == "" || !strings.HasPrefix(transcriptID, "NM_") {
				continue
			}

			// Extract protein ID
			if proteinID, ok := attrMap["protein_id"]; ok {
				if tx, exists := transcripts[transcriptID]; exists {
					tx.ProteinID = proteinID
				}
			}

			cds := Exon{Start: start, End: end}
			cdsByTranscript[transcriptID] = append(cdsByTranscript[transcriptID], cds)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading GFF3: %v", err)
	}

	// Build final transcript database
	for txID, tx := range transcripts {
		// Add exons
		if exons, ok := exonsByTranscript[txID]; ok {
			// Sort exons by genomic position
			sort.Slice(exons, func(i, j int) bool {
				return exons[i].Start < exons[j].Start
			})

			// Renumber exons if needed
			for i := range exons {
				if exons[i].Number == 0 {
					exons[i].Number = i + 1
				}
			}
			tx.Exons = exons
		}

		// Calculate CDS boundaries
		if cdsRegions, ok := cdsByTranscript[txID]; ok && len(cdsRegions) > 0 {
			// Find overall CDS start and end
			tx.CdsStart = cdsRegions[0].Start
			tx.CdsEnd = cdsRegions[0].End
			for _, cds := range cdsRegions {
				if cds.Start < tx.CdsStart {
					tx.CdsStart = cds.Start
				}
				if cds.End > tx.CdsEnd {
					tx.CdsEnd = cds.End
				}
			}
		}

		// Skip transcripts without exons
		if len(tx.Exons) == 0 {
			continue
		}

		// Add to gene lookup
		m.geneTranscripts[tx.GeneSymbol] = append(m.geneTranscripts[tx.GeneSymbol], tx)

		// Add to transcript ID lookup
		m.transcriptsByID[txID] = tx

		// Add to chromosome lookup
		m.chromosomeTranscripts[tx.Chromosome] = append(m.chromosomeTranscripts[tx.Chromosome], tx)
	}

	// Sort transcripts by MANE status (MANE Select first)
	for gene := range m.geneTranscripts {
		txList := m.geneTranscripts[gene]
		sort.Slice(txList, func(i, j int) bool {
			// MANE Select first
			if txList[i].IsMANESelect != txList[j].IsMANESelect {
				return txList[i].IsMANESelect
			}
			// Then MANE Plus Clinical
			if txList[i].IsMANEPlusClinical != txList[j].IsMANEPlusClinical {
				return txList[i].IsMANEPlusClinical
			}
			// Then by transcript ID
			return txList[i].TranscriptID < txList[j].TranscriptID
		})
	}

	return nil
}

// parseGFF3Attributes parses the attributes column of a GFF3 line
func parseGFF3Attributes(attrs string) map[string]string {
	result := make(map[string]string)
	for _, attr := range strings.Split(attrs, ";") {
		parts := strings.SplitN(attr, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

// extractTranscriptID extracts the transcript ID from a GFF3 ID or Parent field
// Input: "rna-NM_001005484.2" -> Output: "NM_001005484.2"
func extractTranscriptID(id string) string {
	if strings.HasPrefix(id, "rna-") {
		return strings.TrimPrefix(id, "rna-")
	}
	// Already clean
	if strings.HasPrefix(id, "NM_") || strings.HasPrefix(id, "NR_") {
		return id
	}
	return ""
}

// ComputeHGVS computes HGVS annotations for a variant
// Parameters:
//   - chromosome: NC_* accession (e.g., "NC_000001.11")
//   - position: 1-based genomic position
//   - ref: reference allele
//   - alt: alternate allele
//   - geneSymbols: list of gene symbols from GENEINFO (can be empty)
// Returns: MANE annotation and list of all transcript annotations
func (m *HGVSMapper) ComputeHGVS(chromosome string, position int64, ref, alt string, geneSymbols []string) (*pbuf.HgvsAnnotation, []*pbuf.HgvsAnnotation) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.loaded {
		return nil, nil
	}

	var maneAnnotation *pbuf.HgvsAnnotation
	var allAnnotations []*pbuf.HgvsAnnotation

	// Collect all transcripts to check
	var transcriptsToCheck []*TranscriptInfo

	// First, use gene symbols if provided
	for _, gene := range geneSymbols {
		if txList, ok := m.geneTranscripts[gene]; ok {
			transcriptsToCheck = append(transcriptsToCheck, txList...)
		}
	}

	// If no gene symbols or no matches, try position-based lookup
	if len(transcriptsToCheck) == 0 {
		if txList, ok := m.chromosomeTranscripts[chromosome]; ok {
			for _, tx := range txList {
				// Check if position overlaps transcript region (with some buffer for upstream/downstream)
				buffer := int64(5000) // 5kb buffer for upstream/downstream variants
				if position >= tx.TxStart-buffer && position <= tx.TxEnd+buffer {
					transcriptsToCheck = append(transcriptsToCheck, tx)
				}
			}
		}
	}

	// Compute HGVS for each transcript
	seen := make(map[string]bool)
	for _, tx := range transcriptsToCheck {
		if seen[tx.TranscriptID] {
			continue
		}
		seen[tx.TranscriptID] = true

		annotation := m.computeTranscriptHGVS(tx, chromosome, position, ref, alt)
		if annotation == nil {
			continue
		}

		allAnnotations = append(allAnnotations, annotation)

		// Track MANE Select for the dedicated field
		if tx.IsMANESelect && maneAnnotation == nil {
			maneAnnotation = annotation
		}
	}

	return maneAnnotation, allAnnotations
}

// computeTranscriptHGVS computes HGVS for a specific transcript
func (m *HGVSMapper) computeTranscriptHGVS(tx *TranscriptInfo, chromosome string, position int64, ref, alt string) *pbuf.HgvsAnnotation {
	// Check if variant is on the same chromosome
	if tx.Chromosome != chromosome {
		return nil
	}

	// Determine the consequence and compute c. notation
	hgvsC, consequence := m.computeCNotation(tx, position, ref, alt)
	if hgvsC == "" {
		return nil
	}

	// Compute protein change if coding
	hgvsP := ""
	if consequence == "coding" && tx.ProteinID != "" {
		// TODO: Compute protein change (requires codon table and sequence)
		// For now, leave empty - would need RefSeq sequence data
	}

	return &pbuf.HgvsAnnotation{
		TranscriptId:       tx.TranscriptID,
		GeneSymbol:         tx.GeneSymbol,
		HgvsC:              hgvsC,
		HgvsP:              hgvsP,
		Consequence:        consequence,
		IsManeSelect:       tx.IsMANESelect,
		IsManePlusClinical: tx.IsMANEPlusClinical,
	}
}

// computeCNotation computes the c. notation for a variant
// Returns (c_notation, consequence)
func (m *HGVSMapper) computeCNotation(tx *TranscriptInfo, position int64, ref, alt string) (string, string) {
	// Check if position is within transcript bounds (with buffer)
	buffer := int64(5000)
	if position < tx.TxStart-buffer || position > tx.TxEnd+buffer {
		return "", ""
	}

	// Handle strand
	isReverse := tx.Strand == "-"

	// Find the exon or intron containing this position
	var cPos int64
	var consequence string
	var inExon bool
	var exonNum int
	var distToExon int64

	// Calculate position in transcript coordinates
	if tx.CdsStart > 0 && tx.CdsEnd > 0 {
		// Coding transcript
		cPos, inExon, exonNum, distToExon = m.genomicToTranscriptPos(tx, position, isReverse)

		if position < tx.TxStart {
			consequence = "upstream"
			return "", consequence
		} else if position > tx.TxEnd {
			consequence = "downstream"
			return "", consequence
		} else if position < tx.CdsStart || (isReverse && position > tx.CdsStart) {
			consequence = "5_utr"
		} else if position > tx.CdsEnd || (isReverse && position < tx.CdsEnd) {
			consequence = "3_utr"
		} else if inExon {
			consequence = "coding"
		} else {
			consequence = "intronic"
		}
	} else {
		// Non-coding transcript
		cPos, inExon, exonNum, distToExon = m.genomicToTranscriptPos(tx, position, isReverse)
		if inExon {
			consequence = "non_coding_exon"
		} else {
			consequence = "intronic"
		}
	}

	// Build HGVS c. notation
	var hgvsC string

	// Handle allele representation for HGVS
	refLen := len(ref)
	altLen := len(alt)

	// For reverse strand, complement the alleles
	if isReverse {
		ref = reverseComplement(ref)
		alt = reverseComplement(alt)
	}

	if inExon {
		// Exonic variant
		if refLen == 1 && altLen == 1 {
			// SNV: c.123A>G
			hgvsC = fmt.Sprintf("c.%d%s>%s", cPos, ref, alt)
		} else if refLen > altLen {
			// Deletion
			delLen := refLen - altLen
			if delLen == 1 {
				hgvsC = fmt.Sprintf("c.%ddel", cPos)
			} else {
				hgvsC = fmt.Sprintf("c.%d_%ddel", cPos, cPos+int64(delLen)-1)
			}
		} else if refLen < altLen {
			// Insertion
			inserted := alt[refLen:]
			hgvsC = fmt.Sprintf("c.%d_%dins%s", cPos, cPos+1, inserted)
		} else {
			// Substitution/indel
			hgvsC = fmt.Sprintf("c.%d_%ddelins%s", cPos, cPos+int64(refLen)-1, alt)
		}
	} else {
		// Intronic variant: c.123+10A>G or c.124-5A>G
		if distToExon > 0 {
			// After exon (in intron, closer to previous exon)
			if refLen == 1 && altLen == 1 {
				hgvsC = fmt.Sprintf("c.%d+%d%s>%s", cPos, distToExon, ref, alt)
			} else {
				hgvsC = fmt.Sprintf("c.%d+%d", cPos, distToExon)
			}
		} else {
			// Before exon (in intron, closer to next exon)
			if refLen == 1 && altLen == 1 {
				hgvsC = fmt.Sprintf("c.%d%d%s>%s", cPos, distToExon, ref, alt)
			} else {
				hgvsC = fmt.Sprintf("c.%d%d", cPos, distToExon)
			}
		}
	}

	_ = exonNum // Could use for additional annotation

	return hgvsC, consequence
}

// genomicToTranscriptPos converts genomic position to transcript position
// Returns: (cPos, inExon, exonNumber, distanceToNearestExon)
func (m *HGVSMapper) genomicToTranscriptPos(tx *TranscriptInfo, genomicPos int64, isReverse bool) (int64, bool, int, int64) {
	// Sort exons by genomic position
	exons := tx.Exons

	// If reverse strand, we count from the end
	if isReverse {
		// Make a copy and reverse
		reversedExons := make([]Exon, len(exons))
		for i, e := range exons {
			reversedExons[len(exons)-1-i] = e
		}
		exons = reversedExons
	}

	var transcriptPos int64 = 0
	var cdsOffset int64 = 0

	// Calculate CDS offset (for UTR handling)
	if tx.CdsStart > 0 {
		for _, exon := range exons {
			cdsStartInExon := tx.CdsStart
			if isReverse {
				cdsStartInExon = tx.CdsEnd
			}

			if exon.Start <= cdsStartInExon && cdsStartInExon <= exon.End {
				// CDS starts in this exon
				if isReverse {
					cdsOffset += exon.End - cdsStartInExon + 1
				} else {
					cdsOffset += cdsStartInExon - exon.Start
				}
				break
			} else if exon.End < cdsStartInExon {
				// Entire exon is before CDS start (5' UTR)
				cdsOffset += exon.End - exon.Start + 1
			}
		}
	}

	// Find position in transcript
	for i, exon := range exons {
		exonLen := exon.End - exon.Start + 1

		if genomicPos >= exon.Start && genomicPos <= exon.End {
			// Position is in this exon
			var posInExon int64
			if isReverse {
				posInExon = exon.End - genomicPos
			} else {
				posInExon = genomicPos - exon.Start
			}

			transcriptPos += posInExon + 1

			// Adjust for CDS (convert to c. position)
			cPos := transcriptPos - cdsOffset

			return cPos, true, i + 1, 0
		}

		// Check if position is in intron after this exon
		if i < len(exons)-1 {
			nextExon := exons[i+1]

			var intronStart, intronEnd int64
			if isReverse {
				intronStart = nextExon.End + 1
				intronEnd = exon.Start - 1
			} else {
				intronStart = exon.End + 1
				intronEnd = nextExon.Start - 1
			}

			if genomicPos >= intronStart && genomicPos <= intronEnd {
				// Position is in this intron
				distFromPrevExon := genomicPos - exon.End
				distToNextExon := nextExon.Start - genomicPos

				if isReverse {
					distFromPrevExon = exon.Start - genomicPos
					distToNextExon = genomicPos - nextExon.End
				}

				transcriptPos += exonLen
				cPos := transcriptPos - cdsOffset

				if distFromPrevExon <= distToNextExon {
					// Closer to previous exon: c.X+N
					return cPos, false, i + 1, distFromPrevExon
				} else {
					// Closer to next exon: c.X-N
					cPosNext := cPos + 1
					return cPosNext, false, i + 2, -distToNextExon
				}
			}
		}

		transcriptPos += exonLen
	}

	// Position outside transcript
	return 0, false, 0, 0
}

// reverseComplement returns the reverse complement of a DNA sequence
func reverseComplement(seq string) string {
	complement := map[byte]byte{
		'A': 'T', 'T': 'A', 'G': 'C', 'C': 'G',
		'a': 't', 't': 'a', 'g': 'c', 'c': 'g',
		'N': 'N', 'n': 'n',
	}

	result := make([]byte, len(seq))
	for i := 0; i < len(seq); i++ {
		c := seq[len(seq)-1-i]
		if comp, ok := complement[c]; ok {
			result[i] = comp
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// IsLoaded returns true if the HGVS mapper has been loaded
func (m *HGVSMapper) IsLoaded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded
}

// GetTranscriptsForGene returns all transcripts for a gene symbol
func (m *HGVSMapper) GetTranscriptsForGene(geneSymbol string) []*TranscriptInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.geneTranscripts[geneSymbol]
}

// GetMANETranscript returns the MANE Select transcript for a gene, if available
func (m *HGVSMapper) GetMANETranscript(geneSymbol string) *TranscriptInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	txList := m.geneTranscripts[geneSymbol]
	for _, tx := range txList {
		if tx.IsMANESelect {
			return tx
		}
	}
	return nil
}
