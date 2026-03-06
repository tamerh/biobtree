package update

import (
	"bufio"
	"log"
	"sort"
	"strconv"
	"strings"
)

// GeneInterval represents a gene with its genomic coordinates
type GeneInterval struct {
	Start    int64
	End      int64
	GeneID   string // Ensembl gene ID (e.g., ENSG00000141510)
	Symbol   string // Gene symbol (e.g., TP53)
}

// GeneCoordinateIndex provides efficient coordinate-based gene lookup
// It stores genes per chromosome in sorted order for binary search
type GeneCoordinateIndex struct {
	chromosomes map[string][]GeneInterval // chr -> sorted gene intervals
	geneCount   int
}

// NewGeneCoordinateIndex creates a new empty gene coordinate index
func NewGeneCoordinateIndex() *GeneCoordinateIndex {
	return &GeneCoordinateIndex{
		chromosomes: make(map[string][]GeneInterval),
	}
}

// AddGene adds a gene to the index
func (idx *GeneCoordinateIndex) AddGene(chr string, start, end int64, geneID, symbol string) {
	// Normalize chromosome (remove 'chr' prefix if present)
	chr = normalizeChromosome(chr)

	idx.chromosomes[chr] = append(idx.chromosomes[chr], GeneInterval{
		Start:  start,
		End:    end,
		GeneID: geneID,
		Symbol: symbol,
	})
	idx.geneCount++
}

// Finalize sorts all chromosome gene lists for efficient lookup
func (idx *GeneCoordinateIndex) Finalize() {
	for chr := range idx.chromosomes {
		genes := idx.chromosomes[chr]
		// Sort by start position for efficient lookup
		sort.Slice(genes, func(i, j int) bool {
			return genes[i].Start < genes[j].Start
		})
		idx.chromosomes[chr] = genes
	}
	log.Printf("GeneCoordinateIndex: Finalized with %d genes across %d chromosomes", idx.geneCount, len(idx.chromosomes))
}

// FindOverlappingGenes returns all genes that overlap with the given position
// Uses binary search to efficiently find candidate genes
func (idx *GeneCoordinateIndex) FindOverlappingGenes(chr string, position int64) []GeneInterval {
	chr = normalizeChromosome(chr)

	genes, ok := idx.chromosomes[chr]
	if !ok || len(genes) == 0 {
		return nil
	}

	var overlapping []GeneInterval

	// Binary search to find genes that might overlap
	// Find first gene where Start <= position
	// We need to search backward from there as well since genes can be long

	// Find the insertion point using binary search
	insertIdx := sort.Search(len(genes), func(i int) bool {
		return genes[i].Start > position
	})

	// Check genes before and at the insertion point
	// A gene overlaps if: gene.Start <= position <= gene.End
	// We need to check all genes where Start <= position

	// Search backward from insertIdx to find overlapping genes
	for i := insertIdx - 1; i >= 0; i-- {
		gene := genes[i]
		if gene.End < position {
			// Gene ends before our position and genes are sorted by start,
			// but gene lengths vary, so we need to check more
			// However, most genes aren't extremely long, so we can limit search
			// Check up to 500KB back (most genes are smaller)
			if position-gene.Start > 500000 {
				break
			}
			continue
		}
		if gene.Start <= position && gene.End >= position {
			overlapping = append(overlapping, gene)
		}
	}

	return overlapping
}

// GeneCount returns the total number of genes in the index
func (idx *GeneCoordinateIndex) GeneCount() int {
	return idx.geneCount
}

// FindNearbyGenes returns all genes within the specified distance from a genomic region.
// This is useful for mapping enhancers to potential target genes.
// The distance is measured from the region boundaries to the gene boundaries.
// For example, with a 500kb threshold, genes where any part of the gene
// is within 500kb of the region will be returned.
func (idx *GeneCoordinateIndex) FindNearbyGenes(chr string, regionStart, regionEnd int64, distanceThreshold int64) []GeneInterval {
	chr = normalizeChromosome(chr)

	genes, ok := idx.chromosomes[chr]
	if !ok || len(genes) == 0 {
		return nil
	}

	var nearby []GeneInterval

	// Calculate the search window
	searchStart := regionStart - distanceThreshold
	if searchStart < 0 {
		searchStart = 0
	}
	searchEnd := regionEnd + distanceThreshold

	// Binary search to find the first gene that could be in range
	// A gene is in range if: gene.End >= searchStart AND gene.Start <= searchEnd
	startIdx := sort.Search(len(genes), func(i int) bool {
		return genes[i].Start > searchStart-500000 // Account for gene length
	})

	// Step back a bit to ensure we don't miss any long genes
	if startIdx > 0 {
		startIdx--
	}

	// Scan forward from startIdx to find all genes in range
	for i := startIdx; i < len(genes); i++ {
		gene := genes[i]

		// If gene starts after our search window, we're done
		if gene.Start > searchEnd {
			break
		}

		// Check if gene overlaps with or is within distance of the region
		// Gene is nearby if:
		// 1. Gene overlaps with region: gene.Start <= regionEnd && gene.End >= regionStart
		// 2. Gene is upstream within threshold: gene.End < regionStart && regionStart - gene.End <= distanceThreshold
		// 3. Gene is downstream within threshold: gene.Start > regionEnd && gene.Start - regionEnd <= distanceThreshold

		var distance int64 = 0
		if gene.End < regionStart {
			// Gene is upstream
			distance = regionStart - gene.End
		} else if gene.Start > regionEnd {
			// Gene is downstream
			distance = gene.Start - regionEnd
		}
		// Otherwise gene overlaps with region, distance = 0

		if distance <= distanceThreshold {
			nearby = append(nearby, gene)
		}
	}

	return nearby
}

// normalizeChromosome removes 'chr' prefix and normalizes chromosome names
func normalizeChromosome(chr string) string {
	if strings.HasPrefix(chr, "chr") {
		chr = chr[3:]
	}
	return chr
}

// LoadHumanGeneCoordinatesFromGFF3 loads human gene coordinates from an Ensembl GFF3 file
// This parses only gene features and extracts coordinates and identifiers
func LoadHumanGeneCoordinatesFromGFF3(gff3Path string) (*GeneCoordinateIndex, error) {
	log.Printf("Loading human gene coordinates from %s", gff3Path)

	// Get data reader (supports FTP, HTTP, local files)
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("ensembl", "", "", gff3Path)
	if err != nil {
		return nil, err
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	idx := NewGeneCoordinateIndex()
	scanner := bufio.NewScanner(br)

	// Use larger buffer for long lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	var lineCount, geneCount int64
	var skippedNoID, skippedFeatureType int64
	featureTypeCounts := make(map[string]int64)

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		// Log progress every 1M lines
		if lineCount%1000000 == 0 {
			log.Printf("GeneCoordinateIndex: Processed %d lines, found %d genes so far...", lineCount, geneCount)
		}

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || len(line) == 0 {
			continue
		}

		// GFF3 format: seqid source type start end score strand phase attributes
		fields := strings.Split(line, "\t")
		if len(fields) < 9 {
			continue
		}

		featureType := fields[2]
		featureTypeCounts[featureType]++

		// Only process gene features (gene, ncRNA_gene, pseudogene, etc.)
		if !strings.HasSuffix(featureType, "gene") {
			skippedFeatureType++
			continue
		}

		chr := fields[0]
		startStr := fields[3]
		endStr := fields[4]
		attributes := fields[8]

		// Parse start and end
		start, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			continue
		}
		end, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			continue
		}

		// Parse attributes to extract gene ID and name
		// Format: ID=gene:ENSG00000223972;Name=DDX11L1;biotype=...
		geneID := ""
		symbol := ""

		for _, attr := range strings.Split(attributes, ";") {
			if strings.HasPrefix(attr, "ID=gene:") {
				geneID = strings.TrimPrefix(attr, "ID=gene:")
			} else if strings.HasPrefix(attr, "ID=") {
				// Some GFF3 files might not have "gene:" prefix
				id := strings.TrimPrefix(attr, "ID=")
				if strings.HasPrefix(id, "ENSG") {
					geneID = id
				}
			} else if strings.HasPrefix(attr, "Name=") {
				symbol = strings.TrimPrefix(attr, "Name=")
			}
		}

		if geneID == "" {
			skippedNoID++
			// Log first few skipped entries for debugging
			if skippedNoID <= 5 {
				attrPreview := attributes
			if len(attrPreview) > 100 {
				attrPreview = attrPreview[:100]
			}
			log.Printf("GeneCoordinateIndex: Skipped gene with no ID, featureType=%s, attrs=%s", featureType, attrPreview)
			}
			continue
		}

		idx.AddGene(chr, start, end, geneID, symbol)
		geneCount++

		if geneCount%10000 == 0 {
			log.Printf("GeneCoordinateIndex: Loaded %d genes...", geneCount)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("GeneCoordinateIndex: Scanner error: %v", err)
		return nil, err
	}

	// Log summary statistics
	log.Printf("GeneCoordinateIndex: Parsed %d lines total", lineCount)
	log.Printf("GeneCoordinateIndex: Skipped %d non-gene features, %d genes with no ID", skippedFeatureType, skippedNoID)

	// Log top feature types
	log.Printf("GeneCoordinateIndex: Feature type counts (top 10):")
	for ft, count := range featureTypeCounts {
		if strings.HasSuffix(ft, "gene") || count > 10000 {
			log.Printf("  %s: %d", ft, count)
		}
	}

	idx.Finalize()
	log.Printf("GeneCoordinateIndex: Loaded %d genes from %d lines", geneCount, lineCount)

	return idx, nil
}

