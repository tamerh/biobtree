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

// conservation ingests per-position evolutionary conservation scores
// (phyloP / GERP++ / phastCons) — biobtree's per-base conservation layer.
//
// KEY SCHEME: genomic position "chr:pos" (GRCh38/hg38), ref/alt-agnostic.
//
// IMPORTANT (design flag for coordinator / merge review):
//   Conservation is keyed "chr:pos" whereas the variant datasets
//   (alphamissense / spliceai / gnomad_variant / clinvar) key
//   "chr:pos:ref:alt". Those keys are NOT identical, so a variant xref will
//   NOT auto-join to a conservation record. The intended positional join is:
//     variant "chr:pos:ref:alt"  ->  strip ref/alt  ->  "chr:pos"  ->  conservation lookup.
//   This wiring is deliberately NOT implemented here (would require touching the
//   variant parsers, which is out of scope). Left as a decision for merge review.
//
// PRIMARY, FREELY-REDISTRIBUTABLE SOURCES (dbNSFP itself is CC BY-NC-ND /
// No-Derivatives and cannot be redistributed as a subset, so scores are sourced
// from the upstream providers):
//   phylop     UCSC hg38 phyloP470way multiz track   (hg38.phyloP470way.bw)
//   phastcons  UCSC hg38 phastCons470way multiz track (hg38.phastCons470way.bw)
//   gerp       GERP++ RS score (Ensembl Compara / UCSC)
// See docs/datasets/conservation.md for URLs and the dbNSFP-ND reasoning.
//
// INPUT FORMAT (pre-merged TSV, one row per position; the coordinator produces
// this by joining the three bigWig tracks — biobtree does not parse bigWig):
//
//	# chrom	pos	phylop	gerp	phastcons
//	1	69094	1.234	4.56	0.987
//
// A missing score for a provider is represented by an empty field or "NA" and
// stored as 0 (callers should treat 0 phastcons / phylop as "no value" only in
// combination with the source manifest).
type conservation struct {
	source string
	d      *DataUpdate
}

func (c *conservation) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

func (c *conservation) update() {
	defer c.d.wg.Done()

	log.Println("Conservation: Starting per-position conservation processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[c.source]["id"]

	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("Conservation: [TEST MODE] processing up to %d positions", testLimit)
	}

	filePath := config.Dataconf[c.source]["path"]
	log.Printf("Conservation: Processing positions from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	c.check(err, "opening Conservation TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount, entryCount, skippedCount int64
	var totalRead, previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			c.check(readErr, "reading Conservation TSV file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		totalRead += int64(len(line))
		lineCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip comment / header lines.
		if line == "" || strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// Format: chrom \t pos \t phylop \t gerp \t phastcons
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("Conservation: SKIP line %d: not enough fields (%d < 5)", lineCount, len(fields))
			}
			if readErr == io.EOF {
				break
			}
			continue
		}

		chrom := fields[0]
		posStr := fields[1]

		// Normalize chromosome (strip "chr" prefix to match variant datasets).
		if strings.HasPrefix(chrom, "chr") {
			chrom = chrom[3:]
		}

		pos, err := strconv.ParseInt(posStr, 10, 64)
		if err != nil {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("Conservation: SKIP line %d: invalid position %q", lineCount, posStr)
			}
			if readErr == io.EOF {
				break
			}
			continue
		}

		phylop := parseConservationScore(fields[2])
		gerp := parseConservationScore(fields[3])
		phastcons := parseConservationScore(fields[4])

		// KEY: chr:pos only (ref/alt-agnostic) — see design flag above.
		entryID := fmt.Sprintf("%s:%d", chrom, pos)

		attr := &pbuf.ConservationAttr{
			Chromosome: chrom,
			Position:   pos,
			Phylop:     phylop,
			Gerp:       gerp,
			Phastcons:  phastcons,
		}

		attrBytes, err := ffjson.Marshal(attr)
		c.check(err, fmt.Sprintf("marshaling attributes for %s", entryID))
		c.d.addProp3(entryID, sourceID, attrBytes)

		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		entryCount++

		// Progress reporting.
		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}
		if entryCount%10000000 == 0 {
			log.Printf("Conservation: Processed %d positions...", entryCount)
		}

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("Conservation: [TEST MODE] reached limit of %d positions", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	log.Printf("Conservation: Processed %d positions (skipped %d malformed lines) (%.2fs)",
		entryCount, skippedCount, time.Since(startTime).Seconds())

	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

// parseConservationScore parses a conservation score field, treating empty / "NA"
// / "." as 0 (absent).
func parseConservationScore(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" || s == "." || s == "nan" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
