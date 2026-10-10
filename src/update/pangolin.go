package update

import (
	"archive/zip"
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// pangolin ingests Pangolin precomputed splice scores (Zeng & Li 2022) — an
// independent splice predictor to SpliceAI. Source: the Zenodo bulk
// (Pangolin_hg38_snvs_masked.zip, CC BY 4.0), a zip of ~20k per-gene TSVs
// (columns: chrom,pos,ref,alt,gain_score,gain_pos,loss_score,loss_pos) covering
// every SNV in protein-coding genes (hg38, GENCODE-masked). Kept per-variant
// (chr:pos:ref:alt), filtered to largest delta >= 0.2, edged to the Ensembl gene.
type pangolin struct {
	source   string
	sourceID string
	d        *DataUpdate
}

const pangolinScoreThreshold = 0.2

func (p *pangolin) update() {
	defer p.d.wg.Done()

	p.sourceID = config.Dataconf[p.source]["id"]
	textLinkID := config.Dataconf["textlink"]["id"]
	testLimit := config.GetTestLimit(p.source)

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	if config.IsTestMode() {
		// Plain per-gene TSV fixture (named <ENSG>.tsv so the gene is inferred
		// the same way as a zip member).
		fx := "tests/datasets/pangolin/ENSG00000149596.tsv"
		f, err := os.Open(fx)
		p.check(err, "opening pangolin fixture")
		defer f.Close()
		total += p.processGene(bufio.NewReaderSize(f, 1<<20), geneFromName(fx), textLinkID, idLogFile, testLimit, total)
	} else {
		total = p.processZip(textLinkID, idLogFile, testLimit)
	}

	fmt.Printf("Pangolin: processed %d splice-altering variants\n", total)
	atomic.AddUint64(&p.d.totalParsedEntry, total)
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
}

func (p *pangolin) check(err error, op string) { checkWithContext(err, p.source, op) }

// downloadToFile streams a URL to a local file (used for the large Pangolin zip,
// which archive/zip must read with random access).
func downloadToFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return err
	}
	out.Close()
	return os.Rename(tmp, dest)
}

// geneFromName extracts the ENSG from a member/file name like
// "Pangolin_hg38_snvs_masked/ENSG00000149596.tsv.gz" -> "ENSG00000149596".
func geneFromName(name string) string {
	b := filepath.Base(name)
	b = strings.TrimSuffix(b, ".gz")
	b = strings.TrimSuffix(b, ".tsv")
	return strings.SplitN(b, ".", 2)[0]
}

// processZip downloads the Zenodo bulk (once) and iterates its per-gene members.
func (p *pangolin) processZip(textLinkID string, idLogFile *os.File, testLimit int) uint64 {
	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}
	zipPath := filepath.Join(rootDir, "raw_data", "pangolin", "Pangolin_hg38_snvs_masked.zip")
	if !fileExists(zipPath) {
		if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
			log.Printf("Pangolin: mkdir failed: %v", err)
			return 0
		}
		log.Printf("Pangolin: downloading bulk zip to %s (~13GB, one-time)...", zipPath)
		if err := downloadToFile(config.Dataconf[p.source]["path"], zipPath); err != nil {
			log.Printf("Pangolin: download failed: %v", err)
			return 0
		}
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		log.Printf("Pangolin: cannot open zip %s: %v", zipPath, err)
		return 0
	}
	defer zr.Close()

	var total uint64
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".tsv.gz") {
			continue
		}
		rc, err := f.Open() // zip deflate -> the inner .tsv.gz bytes
		if err != nil {
			log.Printf("Pangolin: open member %s: %v", f.Name, err)
			continue
		}
		gz, err := gzip.NewReader(rc)
		if err != nil {
			rc.Close()
			continue
		}
		total += p.processGene(bufio.NewReaderSize(gz, 1<<20), geneFromName(f.Name), textLinkID, idLogFile, testLimit, total)
		gz.Close()
		rc.Close()
		if testLimit > 0 && total >= uint64(testLimit) {
			break
		}
	}
	return total
}

// processGene parses one gene's SNV TSV, keeping variants with largest delta
// >= threshold. Returns the number of entries written.
func (p *pangolin) processGene(reader *bufio.Reader, gene, textLinkID string, idLogFile *os.File, testLimit int, already uint64) uint64 {
	var count uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return count
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		if line != "" && !strings.HasPrefix(line, "chrom\t") { // skip header
			f := strings.Split(line, "\t")
			if len(f) >= 8 {
				if p.processRow(f, gene, textLinkID, idLogFile) {
					count++
					if testLimit > 0 && already+count >= uint64(testLimit) {
						break
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

// columns: 0 chrom,1 pos,2 ref,3 alt,4 gain_score,5 gain_pos,6 loss_score,7 loss_pos
func (p *pangolin) processRow(f []string, gene, textLinkID string, idLogFile *os.File) bool {
	chrom := NormalizeChr(f[0]) // "chr20" -> "20", matching the chr:pos:ref:alt key scheme
	pos, perr := strconv.ParseInt(f[1], 10, 64)
	if perr != nil {
		return false
	}
	gainStr, lossStr := f[4], f[6]
	gain, _ := strconv.ParseFloat(gainStr, 64)
	loss, _ := strconv.ParseFloat(lossStr, 64) // <= 0 (loss reported negative)
	lossMag := math.Abs(loss)
	largest := gain
	effect := "gain"
	if lossMag > largest {
		largest = lossMag
		effect = "loss"
	}
	if largest < pangolinScoreThreshold {
		return false
	}
	gainPos, _ := strconv.Atoi(f[5])
	lossPos, _ := strconv.Atoi(f[7])

	entryID := fmt.Sprintf("%s:%d:%s:%s", chrom, pos, f[2], f[3])
	if len(entryID) > LMDBMaxKeySize {
		return false
	}

	attr := &pbuf.PangolinAttr{
		Chromosome: chrom,
		Position:   pos,
		RefAllele:  f[2],
		AltAllele:  f[3],
		Gene:       gene,
		Score:      float32(largest),
		Effect:     effect,
		GainScore:  gainStr,
		LossScore:  lossStr,
		GainPos:    int32(gainPos),
		LossPos:    int32(lossPos),
	}
	if b, err := ffjson.Marshal(attr); err == nil {
		p.d.addProp3(entryID, p.sourceID, b)
	}
	// chr:pos text link for position lookup.
	p.d.addXref(fmt.Sprintf("%s:%d", chrom, pos), textLinkID, entryID, p.source, true)
	// variant -> Ensembl gene (so gene >> ensembl >> pangolin resolves).
	if gene != "" {
		p.d.addXref(entryID, p.sourceID, gene, "ensembl", false)
	}
	if idLogFile != nil {
		logProcessedID(idLogFile, entryID)
	}
	return true
}
