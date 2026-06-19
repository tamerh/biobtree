// dbsnp_poc -- proof-of-concept Go extractor for the dbSNP federation (#dbsnp KG layer).
//
// dbSNP is a separate BioBTree federation: one ~118 GB gzip forward index
// (dbsnp_sorted.*.index.gz, ~1.1B variants). The gene/transcript links and rich
// annotations are all in that forward stream, so we do NOT need LMDB random
// lookups -- a single sequential pass extracts everything. Decompression of one
// gzip stream caps throughput (~328 MB/s with C zlib), and Go's stdlib gzip is
// slower than that, so the fast pattern is to let `zcat` (or pigz) decompress and
// pipe the plaintext into this pure parser:
//
//	zcat dbsnp_sorted.*.index.gz | dbsnp_poc -out out/dbsnp_poc -max 5000000
//
// Per variant block (subject-contiguous in the sorted file) it emits KGX rows that
// drop straight into the existing assemble step:
//
//	node:  DBSNP:<rs>  biolink:SequenceVariant
//	edge:  variant -is_sequence_variant_of-> gene        (entrez,  ds 4)
//	edge:  variant -is_sequence_variant_of-> transcript  (refseq,  ds 8)
//	attrs: DBSNP:<rs>  {variant_type, consequence, is_common, chromosome, ...}
//
// This is a POC: bounded by -max, measures real throughput, proves correctness.
// Production wiring (id_map canonicalization of genes, sharded/full run, billion-
// scale assemble) comes after the KG shape is agreed.
package main

import (
	"bufio"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// BioBTree dataset numeric ids (global, from conf/*.dataset.json). Hardcoded for
// the POC; production reads them from the registry.
const (
	dsDbsnp    = "3"
	dsEntrez   = "4"  // gene  -> NCBIGene
	dsRefseq   = "8"  // transcript -> refseq
	dsProperty = "-1" // node property JSON
)

const (
	categoryVariant = "biolink:SequenceVariant"
	predVariantOf   = "biolink:is_sequence_variant_of"
	primary         = "infores:dbsnp"
	aggregator      = "infores:biobtree"
	knowledgeLevel  = "knowledge_assertion"
	agentType       = "manual_agent"

	nodeHeader = "id\tcategory\tname\tequivalent_identifiers\tprovided_by"
	edgeHeader = "id\tsubject\tpredicate\tobject\tprimary_knowledge_source\t" +
		"aggregator_knowledge_source\tknowledge_level\tagent_type\thas_evidence\tqualifiers"
)

// Only the property fields we surface as node attributes; json.Unmarshal ignores
// the large gnomad/1000g/hgvs_transcripts arrays (they are not in this struct).
type variantProp struct {
	RsID         string `json:"rs_id"`
	Chromosome   string `json:"chromosome"`
	Position     int64  `json:"position"`
	VariantType  string `json:"variant_type"`
	VariantClass string `json:"variant_class"`
	IsCommon     bool   `json:"is_common"`
	HgvsMane     struct {
		Consequence  string `json:"consequence"`
		IsManeSelect bool   `json:"is_mane_select"`
		GeneSymbol   string `json:"gene_symbol"`
	} `json:"hgvs_mane"`
}

func edgeID(subj, pred, obj string) string {
	sum := md5.Sum([]byte(subj + "|" + pred + "|" + obj + "|" + primary))
	return "biobtree:" + hex.EncodeToString(sum[:])[:16]
}

func curie(prefix, id string) string { return prefix + ":" + id }

type stats struct {
	variants, geneEdges, txEdges, attrRows int64
	bytesIn                                int64
}

// block holds one variant's accumulated lines (subject-contiguous). Independent of
// every other block, so blocks fan out to workers for the CPU-heavy JSON/format.
type block struct {
	id    string
	genes []string
	txs   []string
	prop  string
}

func main() {
	out := flag.String("out", "out/dbsnp_poc", "output dir for nodes/edges/attrs tsv.gz")
	max := flag.Int64("max", 0, "stop after N variants (0 = no limit)")
	withAttrs := flag.Bool("attrs", true, "parse property JSON and emit node attributes")
	workers := flag.Int("workers", 8, "parallel format/JSON workers (output is sharded into N parts)")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	if *workers < 1 {
		*workers = 1
	}

	var st stats
	t0 := time.Now()
	jobs := make(chan *block, 2048)
	var wg sync.WaitGroup

	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go worker(w, jobs, &wg, *out, *withAttrs, &st)
	}

	// Reader: cheap sequential pass building blocks, fans out to workers.
	in := bufio.NewReaderSize(os.Stdin, 1<<22)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<26) // up to 64 MB lines (property JSON is large)

	var cur string
	var genes, txs []string
	var prop string
	var nVar int64
	emit := func() {
		if cur == "" {
			return
		}
		b := &block{id: cur, prop: prop}
		b.genes = append([]string(nil), genes...)
		b.txs = append([]string(nil), txs...)
		jobs <- b
		nVar++
	}
	for sc.Scan() {
		line := sc.Bytes()
		st.bytesIn += int64(len(line)) + 1
		f := splitTab4(line)
		if len(f) < 4 {
			continue
		}
		if f[0] != cur {
			emit()
			if *max > 0 && nVar >= *max {
				cur = ""
				break
			}
			cur, genes, txs, prop = f[0], genes[:0], txs[:0], ""
		}
		switch f[3] {
		case dsProperty:
			prop = f[2]
		case dsEntrez:
			genes = append(genes, f[2])
		case dsRefseq:
			txs = append(txs, f[2])
		}
	}
	emit()
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "scan error:", err)
	}
	close(jobs)
	wg.Wait()

	dt := time.Since(t0).Seconds()
	mb := float64(st.bytesIn) / 1e6
	fmt.Fprintf(os.Stderr, "dbsnp_poc done in %.1fs (workers=%d)\n", dt, *workers)
	fmt.Fprintf(os.Stderr, "  variants=%d gene_edges=%d transcript_edges=%d attr_rows=%d\n",
		atomic.LoadInt64(&st.variants), atomic.LoadInt64(&st.geneEdges),
		atomic.LoadInt64(&st.txEdges), atomic.LoadInt64(&st.attrRows))
	fmt.Fprintf(os.Stderr, "  parsed %.1f GB plaintext  @ %.0f MB/s  %.0f variants/s\n",
		mb/1000, mb/dt, float64(atomic.LoadInt64(&st.variants))/dt)
	if *max == 0 {
		fmt.Fprintf(os.Stderr, "  (full pass)\n")
	} else {
		fmt.Fprintf(os.Stderr, "  (bounded to %d variants; extrapolate x%.0f for ~1.1B -> ~%.0f min full)\n",
			*max, 1.1e9/float64(nVar), dt*(1.1e9/float64(nVar))/60)
	}
}

// worker formats blocks into its own sharded output files (no cross-worker locks).
func worker(id int, jobs <-chan *block, wg *sync.WaitGroup, out string, withAttrs bool, st *stats) {
	defer wg.Done()
	sfx := "." + strconv.Itoa(id) + ".tsv.gz"
	nodeW := newGzTSV(out+"/dbsnp_nodes"+sfx, nodeHeader)
	edgeW := newGzTSV(out+"/dbsnp_edges"+sfx, edgeHeader)
	var attrW *gzTSV
	if withAttrs {
		attrW = newGzTSV(out+"/dbsnp_attrs"+sfx, "")
	}
	var nv, ge, te, ar int64
	for b := range jobs {
		nv++
		subj := curie("DBSNP", b.id)
		name := b.id
		if withAttrs && b.prop != "" {
			var v variantProp
			if json.Unmarshal([]byte(b.prop), &v) == nil {
				if v.RsID != "" {
					name = v.RsID
				}
				if attrW.writeAttrs(subj, &v) {
					ar++
				}
			}
		}
		nodeW.write(subj + "\t" + categoryVariant + "\t" + name + "\t" + subj + "\t" + aggregator)
		for _, g := range b.genes {
			edgeW.writeEdge(subj, predVariantOf, curie("NCBIGene", g))
			ge++
		}
		for _, tx := range b.txs {
			edgeW.writeEdge(subj, predVariantOf, curie("refseq", tx))
			te++
		}
	}
	nodeW.close()
	edgeW.close()
	if attrW != nil {
		attrW.close()
	}
	atomic.AddInt64(&st.variants, nv)
	atomic.AddInt64(&st.geneEdges, ge)
	atomic.AddInt64(&st.txEdges, te)
	atomic.AddInt64(&st.attrRows, ar)
}

// splitTab4 splits a line into up to 4 fields (the 4th keeps any trailing tabs,
// which is fine: the property JSON has no tabs and edge lines have exactly 4).
func splitTab4(b []byte) []string {
	s := string(b)
	out := make([]string, 0, 4)
	for i := 0; i < 3; i++ {
		j := strings.IndexByte(s, '\t')
		if j < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:j])
		s = s[j+1:]
	}
	out = append(out, s)
	return out
}

// --- gzip TSV writer -------------------------------------------------------

type gzTSV struct {
	f  *os.File
	gz *gzip.Writer
	w  *bufio.Writer
}

func newGzTSV(path, header string) *gzTSV {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	gz, err := gzip.NewWriterLevel(f, gzip.BestSpeed) // level 1: write throughput >> ratio
	if err != nil {
		fatal(err)
	}
	w := bufio.NewWriterSize(gz, 1<<20)
	t := &gzTSV{f, gz, w}
	if header != "" {
		t.write(header)
	}
	return t
}

func (t *gzTSV) write(line string) {
	t.w.WriteString(line)
	t.w.WriteByte('\n')
}

func (t *gzTSV) writeEdge(subj, pred, obj string) {
	// id \t subject \t predicate \t object \t primary \t agg \t kl \t at \t ev \t qual
	t.w.WriteString(edgeID(subj, pred, obj))
	t.w.WriteByte('\t')
	t.w.WriteString(subj)
	t.w.WriteByte('\t')
	t.w.WriteString(pred)
	t.w.WriteByte('\t')
	t.w.WriteString(obj)
	t.w.WriteByte('\t')
	t.w.WriteString(primary)
	t.w.WriteByte('\t')
	t.w.WriteString(aggregator)
	t.w.WriteByte('\t')
	t.w.WriteString(knowledgeLevel)
	t.w.WriteByte('\t')
	t.w.WriteString(agentType)
	t.w.WriteString("\t\t\n") // empty has_evidence, qualifiers
}

func (t *gzTSV) writeAttrs(node string, v *variantProp) bool {
	// minimal hand-rolled JSON of the surfaced scalar attrs
	var b strings.Builder
	b.WriteString(node)
	b.WriteString("\t{")
	first := true
	add := func(k, val string) {
		if val == "" {
			return
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteByte('"')
		b.WriteString(k)
		b.WriteString("\":")
		jb, _ := json.Marshal(val)
		b.Write(jb)
	}
	add("variant_type", v.VariantType)
	add("variant_class", v.VariantClass)
	add("chromosome", v.Chromosome)
	add("consequence", v.HgvsMane.Consequence)
	if v.IsCommon {
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString("\"is_common\":true")
	}
	if v.HgvsMane.IsManeSelect {
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString("\"mane_select\":true")
	}
	b.WriteString("}")
	if first { // no attrs surfaced
		return false
	}
	t.write(b.String())
	return true
}

func (t *gzTSV) close() {
	t.w.Flush()
	t.gz.Close()
	t.f.Close()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fatal:", err)
	os.Exit(1)
}
