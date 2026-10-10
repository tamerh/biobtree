package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type spliceai struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (s *spliceai) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

// Main update entry point. Source: Ensembl MANE SpliceAI VCF
// (spliceai_scores.raw.snv.ensembl_mane_v1.4.grch38.vcf.gz) — MANE v1.4 (current
// annotation, superseding the old GENCODE-v24 Illumina tables), all SNV alts,
// and the four delta scores. The gene symbol comes from the VCF, so no GFF3
// coordinate index is needed.
func (s *spliceai) update() {
	defer s.d.wg.Done()

	log.Println("SpliceAI: Starting data processing...")
	startTime := time.Now()

	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("SpliceAI: [TEST MODE] Processing up to %d variants", testLimit)
	}

	sourceID := config.Dataconf[s.source]["id"]
	textLinkID := config.Dataconf["textlink"]["id"]

	s.parseAndSaveVariants(testLimit, idLogFile, sourceID, textLinkID)

	log.Printf("SpliceAI: Processing complete (%.2fs)", time.Since(startTime).Seconds())
	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}

// spliceaiScoreThreshold keeps only variants with a meaningful splice signal
// (parity with the previously-ingested delta>=0.2 set); the raw MANE VCF carries
// every SNV genome-wide, almost all with all-zero deltas.
const spliceaiScoreThreshold = 0.2

// extractSpliceaiInfo pulls the SpliceAI annotation out of a VCF INFO column and
// splits it into its 10 pipe fields:
//
//	ALLELE|SYMBOL|DS_AG|DS_AL|DS_DG|DS_DL|DP_AG|DP_AL|DP_DG|DP_DL
//
// Returns nil if absent/malformed. If several comma-separated annotations are
// present (one per overlapping gene) the first is used.
func extractSpliceaiInfo(info string) []string {
	for _, kv := range strings.Split(info, ";") {
		if strings.HasPrefix(kv, "SpliceAI=") {
			v := strings.SplitN(strings.TrimPrefix(kv, "SpliceAI="), ",", 2)[0]
			parts := strings.Split(v, "|")
			if len(parts) >= 10 {
				return parts
			}
			return nil
		}
	}
	return nil
}

// parseAndSaveVariants streams the MANE SpliceAI VCF, keeps variants whose max
// delta >= threshold, and stores the four deltas + positions + dominant effect.
func (s *spliceai) parseAndSaveVariants(testLimit int, idLogFile *os.File, sourceID, textLinkID string) {
	var reader *bufio.Reader
	if config.IsTestMode() {
		// Read the committed plain-text fixture directly (conf useLocalFile=no is
		// for the remote prod VCF, so getDataReaderNew can't open a local path).
		file, ferr := os.Open("tests/datasets/spliceai/spliceai_mane_fixture.vcf")
		s.check(ferr, "opening SpliceAI fixture")
		defer file.Close()
		log.Printf("SpliceAI: [TEST MODE] Processing fixture VCF")
		reader = bufio.NewReaderSize(file, 1024*1024)
	} else {
		filePath := config.Dataconf[s.source]["path"]
		log.Printf("SpliceAI: Processing MANE VCF from %s", filePath)
		br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
		s.check(err, "opening SpliceAI VCF")
		defer closeReaders(gz, ftpFile, client, localFile)
		if gz != nil {
			reader = bufio.NewReaderSize(gz, 1024*1024)
		} else {
			reader = bufio.NewReaderSize(br, 1024*1024)
		}
	}

	var lineCount, entryCount, skippedCount int64
	effectCounts := make(map[string]int64)
	var totalRead, previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			s.check(readErr, "reading SpliceAI VCF")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}
		totalRead += int64(len(line))
		lineCount++
		line = strings.TrimRight(line, "\r\n")

		if line == "" || line[0] == '#' { // VCF headers / meta
			if readErr == io.EOF {
				break
			}
			continue
		}

		// VCF: CHROM POS ID REF ALT QUAL FILTER INFO
		f := strings.Split(line, "\t")
		if len(f) < 8 {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}
		chrom, posStr, refAllele, altAllele, info := f[0], f[1], f[3], f[4], f[7]

		pos, perr := strconv.ParseInt(posStr, 10, 64)
		if perr != nil {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		sa := extractSpliceaiInfo(info)
		if sa == nil {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}
		symbol := sa[1]
		dsAG, _ := strconv.ParseFloat(sa[2], 64)
		dsAL, _ := strconv.ParseFloat(sa[3], 64)
		dsDG, _ := strconv.ParseFloat(sa[4], 64)
		dsDL, _ := strconv.ParseFloat(sa[5], 64)

		maxDS, effect := dsAG, "acceptor_gain"
		if dsAL > maxDS {
			maxDS, effect = dsAL, "acceptor_loss"
		}
		if dsDG > maxDS {
			maxDS, effect = dsDG, "donor_gain"
		}
		if dsDL > maxDS {
			maxDS, effect = dsDL, "donor_loss"
		}
		if maxDS < spliceaiScoreThreshold {
			if readErr == io.EOF {
				break
			}
			continue
		}

		entryID := fmt.Sprintf("%s:%d:%s:%s", chrom, pos, refAllele, altAllele)
		if len(entryID) > LMDBMaxKeySize {
			if readErr == io.EOF {
				break
			}
			continue
		}

		effectCounts[effect]++
		attr := &pbuf.SpliceAIAttr{
			Chromosome: chrom,
			Position:   pos,
			RefAllele:  refAllele,
			AltAllele:  altAllele,
			Effect:     effect,
			Score:      float32(maxDS),
			GeneSymbol: symbol,
			DsAg:       sa[2], DsAl: sa[3], DsDg: sa[4], DsDl: sa[5],
			DpAg: sa[6], DpAl: sa[7], DpDg: sa[8], DpDl: sa[9],
		}

		s.createCrossReferences(entryID, sourceID, textLinkID, attr)

		// chr:pos text link for position-based lookup.
		s.d.addXref(fmt.Sprintf("%s:%d", chrom, pos), textLinkID, entryID, s.source, true)

		attrBytes, merr := ffjson.Marshal(attr)
		s.check(merr, fmt.Sprintf("marshaling attributes for %s", entryID))
		s.d.addProp3(entryID, sourceID, attrBytes)

		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}
		entryCount++

		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: totalRead / elapsed / 1024}
		}
		if entryCount%1000000 == 0 {
			log.Printf("SpliceAI: Processed %d variants...", entryCount)
		}
		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("SpliceAI: [TEST MODE] Reached limit of %d variants", testLimit)
			break
		}
		if readErr == io.EOF {
			break
		}
	}

	log.Printf("SpliceAI: Processed %d variants (skipped %d non-SpliceAI/malformed/below-threshold)", entryCount, skippedCount)
	for effect, count := range effectCounts {
		log.Printf("SpliceAI: Effect %s: %d", effect, count)
	}
}

// createCrossReferences wires the gene (from the VCF symbol) and the effect so
// gene >> spliceai and effect text search resolve.
func (s *spliceai) createCrossReferences(entryID, sourceID, textLinkID string, attr *pbuf.SpliceAIAttr) {
	if attr.Effect != "" {
		s.d.addXref(attr.Effect, textLinkID, entryID, s.source, true)
	}
	if attr.GeneSymbol != "" {
		s.d.addXref(attr.GeneSymbol, textLinkID, entryID, s.source, true)
		s.d.addHumanGeneXrefsAll(attr.GeneSymbol, entryID, sourceID)
	}
}
