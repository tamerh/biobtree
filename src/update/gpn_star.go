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

// gpnStar ingests GPN-Star phylogeny-aware genomic language-model scores
// (Benegas et al., Nature 2026; supersedes GPN-MSA) — specifically the V
// (100-vertebrate) model's mutation-rate-CALIBRATED scores. V is shipped FIRST
// as a product choice: it leads the ClinVar/coding AUPRC (0.907) that a germline
// variant reference mostly answers. This is NOT the paper's default — the paper
// frames model choice as timescale-dependent and names no canonical model. M
// (447-mammal) is a planned SECOND model for non-coding (V alone is incomplete
// there); P (243-primate) is skipped (never wins a clinically relevant
// benchmark). Note M is NOT redundant with our mammalian phyloP470way: GPN-Star
// beats phyloP at every matched depth (redundancy of timescale != of signal).
// The calibration removes the local-mutation-rate confound (what Gnocchi corrects).
// Stored minimally like CADD: key chr:pos:ref:alt -> {score}, no coord
// duplication and no links, so exactly 1 KV per variant — smallest footprint for
// a genome-wide (~9B SNV) set. Direct entry lookup (same as cadd/conservation).
// Own `gpn_star` federation. Apache-2.0 (export-clean). SNV-only (no indels).
// NOT ACMG-calibrated -> context/evidence, not a weighted signal.
//
// Input is the prepared, sorted TSV (src/scripts/gpn_star/gpn_star_prepare.py,
// which converts the HF parquet shards): chrom<TAB>pos<TAB>ref<TAB>alt<TAB>score.
type gpnStar struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (g *gpnStar) check(err error, op string) { checkWithContext(err, g.source, op) }

func (g *gpnStar) update() {
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
		fx := "tests/datasets/gpn_star/gpn_star_fixture.tsv"
		f, err := os.Open(fx)
		g.check(err, "opening gpn_star fixture")
		defer f.Close()
		total += g.processReader(bufio.NewReaderSize(f, 1<<20), idLogFile, testLimit, total)
	} else {
		fp := config.Dataconf[g.source]["path"]
		n, err := g.processFile(fp, idLogFile, testLimit, total)
		if err != nil {
			log.Printf("GPN-Star: error processing %s: %v", fp, err)
		}
		total += n
	}

	fmt.Printf("GPN-Star: processed %d variants\n", total)
	atomic.AddUint64(&g.d.totalParsedEntry, total)
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}

func (g *gpnStar) processFile(filePath string, idLogFile *os.File, testLimit int, already uint64) (uint64, error) {
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

// columns: 0 chrom, 1 pos, 2 ref, 3 alt, 4 score (llr_calibrated), 5 abs_llr
func (g *gpnStar) processReader(reader *bufio.Reader, idLogFile *os.File, testLimit int, already uint64) uint64 {
	var count uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			log.Printf("GPN-Star: read error: %v", rerr)
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
						absLlr := ""
						if len(f) >= 6 {
							absLlr = f[5]
						}
						attr := &pbuf.GpnStarAttr{Score: f[4], AbsLlr: absLlr}
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
