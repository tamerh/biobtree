package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// gtex ingests GTEx v10 cis-QTL significant variant-gene pairs (eQTL + sQTL),
// keyed by variant chr:pos:ref:alt. One entry per variant holds ALL its
// significant associations across the 49 tissues (full per-tissue), each tagged
// eqtl/sqtl. Each distinct affected gene gets a variant->ensembl xref so
// gene >> gtex resolves too.
//
// Input is the prepared, variant-sorted TSV (src/scripts/gtex/gtex_prepare.py):
//
//	variant_key  qtl_type  gene_id  tissue  slope  pval  phenotype_id
//
// Sorted by variant_key, so consecutive rows for one variant aggregate in a
// single streaming pass.
type gtex struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (g *gtex) check(err error, op string) { checkWithContext(err, g.source, op) }

func (g *gtex) update() {
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

	var reader *bufio.Reader
	if config.IsTestMode() {
		f, ferr := os.Open("tests/datasets/gtex/gtex_fixture.tsv")
		g.check(ferr, "opening gtex fixture")
		defer f.Close()
		reader = bufio.NewReaderSize(f, 1<<20)
	} else {
		filePath := config.Dataconf[g.source]["path"]
		br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", filePath)
		g.check(err, "opening prepared GTEx TSV")
		defer closeReaders(gz, ftpFile, client, localFile)
		if gz != nil {
			reader = bufio.NewReaderSize(gz, 1<<20)
		} else {
			reader = bufio.NewReaderSize(br, 1<<20)
		}
	}

	var count uint64
	var curKey string
	var attr *pbuf.GtexAttr
	genes := map[string]bool{} // distinct genes for the current variant

	flush := func() {
		if curKey == "" || attr == nil {
			return
		}
		if b, err := ffjson.Marshal(attr); err == nil {
			g.d.addProp3(curKey, g.sourceID, b)
		}
		for gene := range genes {
			if gene != "" {
				g.d.addXref(curKey, g.sourceID, gene, "ensembl", false)
			}
		}
		if idLogFile != nil {
			logProcessedID(idLogFile, curKey)
		}
		count++
	}

	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			break
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line != "" && line[0] != '#' {
			f := strings.Split(line, "\t")
			if len(f) >= 6 {
				key := f[0]
				if key != curKey {
					flush()
					if testLimit > 0 && int(count) >= testLimit {
						curKey = ""
						break
					}
					curKey = key
					attr = &pbuf.GtexAttr{}
					genes = map[string]bool{}
				}
				phenotype := ""
				if len(f) >= 7 {
					phenotype = f[6]
				}
				attr.Associations = append(attr.Associations, &pbuf.GtexAssoc{
					QtlType:     f[1],
					GeneId:      f[2],
					Tissue:      f[3],
					Slope:       f[4],
					Pval:        f[5],
					PhenotypeId: phenotype,
				})
				genes[f[2]] = true
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	flush() // last variant

	fmt.Printf("GTEx: processed %d variants with cis-QTL associations\n", count)
	atomic.AddUint64(&g.d.totalParsedEntry, count)
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}
