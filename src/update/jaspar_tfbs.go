package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// jasparTfbs ingests the Ensembl Regulatory Build motif features — JASPAR TF
// binding motifs mapped ONLY within regulatory features (the scoped alternative
// to the 177GB genome-wide JASPAR scan; ~20MB source). Child of the `jaspar`
// matrix-metadata dataset. Interval dataset: each motif is reachable by a
// variant's position via /ws/map, and links to the jaspar matrix + each TF's
// hgnc gene.
//
// Source GFF3 (type TF_binding_site), 9 cols:
//
//	chr  Ensembl  TF_binding_site  start  end  score  strand  .  ID=ENSM...;binding_matrix_id=ENSPFM...;transcription_factor=HNF4A,NR2F1,RXRA
type jasparTfbs struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (j *jasparTfbs) check(err error, op string) { checkWithContext(err, j.source, op) }

func (j *jasparTfbs) update() {
	defer j.d.wg.Done()

	j.sourceID = config.Dataconf[j.source]["id"]
	testLimit := config.GetTestLimit(j.source)

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, j.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var reader *bufio.Reader
	if config.IsTestMode() {
		// Prod path is the remote Ensembl GFF3 (useLocalFile=no); in test mode read
		// a small local fixture directly (same approach as cadd/spliceai/pangolin).
		f, ferr := os.Open("tests/datasets/jaspar_tfbs/jaspar_tfbs_fixture.gff3")
		j.check(ferr, "opening jaspar_tfbs fixture")
		defer f.Close()
		reader = bufio.NewReaderSize(f, 1<<20)
	} else {
		filePath := config.Dataconf[j.source]["path"]
		br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(j.source, "", "", filePath)
		j.check(err, "opening Ensembl motif_features GFF3")
		defer closeReaders(gz, ftpFile, client, localFile)
		if gz != nil {
			reader = bufio.NewReaderSize(gz, 1<<20)
		} else {
			reader = bufio.NewReaderSize(br, 1<<20)
		}
	}

	var count uint64
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
			if j.processRow(line, idLogFile) {
				count++
				if testLimit > 0 && int(count) >= testLimit {
					break
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}

	fmt.Printf("JASPAR TFBS: processed %d motif features\n", count)
	atomic.AddUint64(&j.d.totalParsedEntry, count)
	j.d.progChan <- &progressInfo{dataset: j.source, done: true}
}

func (j *jasparTfbs) processRow(line string, idLogFile *os.File) bool {
	f := strings.Split(line, "\t")
	if len(f) < 9 || f[2] != "TF_binding_site" {
		return false
	}
	chr := NormalizeChr(f[0])
	start, e1 := strconv.Atoi(f[3])
	end, e2 := strconv.Atoi(f[4])
	if e1 != nil || e2 != nil || chr == "" {
		return false
	}

	// attributes column: ID=...;binding_matrix_id=...;transcription_factor=A,B,C
	var motifID, matrixID string
	var tfs []string
	for _, kv := range strings.Split(f[8], ";") {
		kv = strings.TrimSpace(kv)
		if i := strings.IndexByte(kv, '='); i > 0 {
			k, v := kv[:i], kv[i+1:]
			switch k {
			case "ID":
				motifID = v
			case "binding_matrix_id":
				matrixID = v
			case "transcription_factor":
				for _, t := range strings.Split(v, ",") {
					if t = strings.TrimSpace(t); t != "" {
						tfs = append(tfs, t)
					}
				}
			}
		}
	}
	if motifID == "" {
		motifID = fmt.Sprintf("%s:%d-%d", chr, start, end)
	}
	if len(motifID) > LMDBMaxKeySize {
		return false
	}

	attr := &pbuf.JasparTfbsAttr{
		Chromosome:           chr,
		Start:                int32(start),
		End:                  int32(end),
		Score:                f[5],
		Strand:               f[6],
		BindingMatrixId:      matrixID,
		TranscriptionFactors: tfs,
	}
	if b, err := ffjson.Marshal(attr); err == nil {
		j.d.addProp3(motifID, j.sourceID, b)
	}
	// interval index -> variant-position reachable via /ws/map
	j.d.addInterval(motifID, j.source, chr, int64(start), int64(end))
	// motif -> hgnc for each TF gene symbol (keyed FROM this record, per the
	// incremental-edge-direction rule). binding_matrix_id (ENSPFM...) is kept as
	// an attr only — it is an Ensembl PFM id, which does NOT equal a JASPAR MA
	// accession, so a direct xref to the `jaspar` dataset would dangle. A proper
	// ENSPFM->MA mapping could add that link later (see JASPAR_TFBS_OPTIONS.md).
	for _, tf := range tfs {
		j.d.addXref(motifID, j.sourceID, tf, "hgnc", false)
	}
	if idLogFile != nil {
		logProcessedID(idLogFile, motifID)
	}
	return true
}
