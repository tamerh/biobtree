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

// gtexEgene ingests GTEx v10 gene-level eGene (eQTL) / sGene (sQTL) summaries,
// keyed by Ensembl gene (ENSG, version stripped). One entry per gene holds its
// per-tissue call (is-eGene, q-value, lead variant, slope). This is the
// gene-anchored view the variant-keyed `gtex` dataset cannot be aggregated into
// (you can't walk every variant of a gene), so Atlas can reach it as
// ensembl >> gtex_egene. Each gene gets a gtex_egene -> ensembl xref.
//
// Input is the prepared, gene-sorted TSV (src/scripts/gtex/gtex_prepare.py):
//
//	gene_id  qtl_type  tissue  gene_name  is_egene  qval  lead_variant  slope
//
// Sorted by gene_id, so consecutive rows for one gene aggregate in a single
// streaming pass.
type gtexEgene struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (g *gtexEgene) check(err error, op string) { checkWithContext(err, g.source, op) }

func (g *gtexEgene) update() {
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
		f, ferr := os.Open("tests/datasets/gtex_egene/gtex_egene_fixture.tsv")
		g.check(ferr, "opening gtex_egene fixture")
		defer f.Close()
		reader = bufio.NewReaderSize(f, 1<<20)
	} else {
		filePath := config.Dataconf[g.source]["path"]
		br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", filePath)
		g.check(err, "opening prepared GTEx eGene TSV")
		defer closeReaders(gz, ftpFile, client, localFile)
		if gz != nil {
			reader = bufio.NewReaderSize(gz, 1<<20)
		} else {
			reader = bufio.NewReaderSize(br, 1<<20)
		}
	}

	var count uint64
	var curGene string
	var attr *pbuf.GtexEgeneAttr

	flush := func() {
		if curGene == "" || attr == nil {
			return
		}
		if b, err := ffjson.Marshal(attr); err == nil {
			g.d.addProp3(curGene, g.sourceID, b)
		}
		// gene-level record -> its Ensembl gene, so ensembl >> gtex_egene resolves.
		// Keyed FROM the gtex_egene record (its own bucket) so the reverse edge on
		// ensembl survives incremental --only builds.
		g.d.addXref(curGene, g.sourceID, curGene, "ensembl", false)
		if idLogFile != nil {
			logProcessedID(idLogFile, curGene)
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
			if len(f) >= 8 {
				gene := f[0]
				if gene != curGene {
					flush()
					if testLimit > 0 && int(count) >= testLimit {
						curGene = ""
						break
					}
					curGene = gene
					attr = &pbuf.GtexEgeneAttr{GeneName: f[3]}
				}
				attr.Tissues = append(attr.Tissues, &pbuf.GtexEgeneTissue{
					QtlType:     f[1],
					Tissue:      f[2],
					IsEgene:     f[4] == "true" || f[4] == "1",
					Qval:        f[5],
					LeadVariant: f[6],
					Slope:       f[7],
				})
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	flush() // last gene

	fmt.Printf("GTEx eGene: processed %d genes with tissue-level QTL summaries\n", count)
	atomic.AddUint64(&g.d.totalParsedEntry, count)
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}
