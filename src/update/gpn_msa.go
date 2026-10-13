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
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// gpnMsa ingests GPN-MSA allele-specific genomic language-model scores (Benegas
// et al., Nat Biotechnol 2025) — the allele-aware complement to phyloP/phastCons
// (which are position-only). Stored minimally like CADD: key chr:pos:ref:alt ->
// {score}, no coord duplication (coords live in the key) and no reverse/text/gene
// links, so exactly 1 KV per variant — the smallest footprint for a genome-wide
// (~9B SNV) set. Reached by direct entry lookup of the variant key (same access
// pattern as cadd/conservation). Its own `gpn_msa` federation. MIT-licensed
// (export-clean). SNV-only (no indels). Raw score, NOT ACMG-calibrated.
type gpnMsa struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (g *gpnMsa) check(err error, op string) { checkWithContext(err, g.source, op) }

func (g *gpnMsa) update() {
	defer g.d.wg.Done()

	g.sourceID = config.Dataconf[g.source]["id"]
	testLimit := config.GetTestLimit(g.source)

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64

	if config.IsTestMode() {
		// Prod path is the remote HuggingFace bgz (useLocalFile=no); in test mode
		// read a small local fixture directly (same approach as cadd/spliceai).
		fx := "tests/datasets/gpn_msa/gpn_msa_fixture.tsv"
		f, err := os.Open(fx)
		g.check(err, "opening gpn_msa fixture")
		defer f.Close()
		total += g.processReader(bufio.NewReaderSize(f, 1<<20), idLogFile, testLimit, total)
	} else {
		fp := config.Dataconf[g.source]["path"]
		n, err := g.processFile(fp, idLogFile, testLimit, total)
		if err != nil {
			log.Printf("GPN-MSA: error processing %s: %v", fp, err)
		}
		total += n
	}

	fmt.Printf("GPN-MSA: processed %d variants\n", total)
	atomic.AddUint64(&g.d.totalParsedEntry, total)
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}

func (g *gpnMsa) processFile(filePath string, idLogFile *os.File, testLimit int, already uint64) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("open: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1<<20)
	} else {
		reader = bufio.NewReaderSize(br, 1<<20)
	}
	return g.processReader(reader, idLogFile, testLimit, already), nil
}

// columns: 0 chrom, 1 pos, 2 ref, 3 alt, 4 score
func (g *gpnMsa) processReader(reader *bufio.Reader, idLogFile *os.File, testLimit int, already uint64) uint64 {
	var count uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			log.Printf("GPN-MSA: read error: %v", rerr)
			return count
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		if line != "" && line[0] != '#' { // skip header/meta
			f := strings.Split(line, "\t")
			if len(f) >= 5 {
				chrom := NormalizeChr(f[0])
				if _, perr := strconv.ParseInt(f[1], 10, 64); perr == nil {
					entryID := chrom + ":" + f[1] + ":" + f[2] + ":" + f[3]
					if len(entryID) <= LMDBMaxKeySize {
						attr := &pbuf.GpnMsaAttr{Score: f[4]}
						if b, err := ffjson.Marshal(attr); err == nil {
							g.d.addProp3(entryID, g.sourceID, b)
						}
						if idLogFile != nil {
							logProcessedID(idLogFile, entryID)
						}
						count++
						if testLimit > 0 && already+count >= uint64(testLimit) {
							break
						}
					}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	return count
}
