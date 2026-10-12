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

func (s *spliceai) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

// spliceaiScoreThreshold keeps only variants with a meaningful splice signal
// (parity with the delta>=0.2 set). Masking only ever zeroes a delta, so
// masked <= raw and "raw max >= 0.2" is already the union of raw/masked — no
// variant is masked-only. A kept variant may therefore carry all-zero masked
// deltas, which is exactly the negative evidence we want to expose.
const spliceaiScoreThreshold = 0.2

// Main update entry point. SpliceAI from the Ensembl MANE VCFs (MANE v1.4):
//   - path:       raw SNV scores (ClinGen SVI PP3/BP4-calibrated, Walker 2023)
//   - pathMasked: masked SNV scores (what Ensembl VEP / the Broad lookup serve);
//     carried as ds_*_masked, merged per variant via a lockstep read
//   - pathIndel:  Illumina GENCODE-based indel scores (raw-only; Ensembl is
//     SNV-only), appended as additional variants
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

	var total int64
	if config.IsTestMode() {
		total += s.processSNVs("tests/datasets/spliceai/spliceai_mane_fixture.vcf",
			"tests/datasets/spliceai/spliceai_masked_fixture.vcf", true, testLimit, total, idLogFile, sourceID, textLinkID)
		total += s.processIndels("tests/datasets/spliceai/spliceai_indel_fixture.vcf", true, testLimit, total, idLogFile, sourceID, textLinkID)
	} else {
		total += s.processSNVs(config.Dataconf[s.source]["path"], config.Dataconf[s.source]["pathMasked"],
			false, testLimit, total, idLogFile, sourceID, textLinkID)
		if ip := config.Dataconf[s.source]["pathIndel"]; ip != "" {
			total += s.processIndels(ip, false, testLimit, total, idLogFile, sourceID, textLinkID)
		}
	}

	log.Printf("SpliceAI: Processing complete (%.2fs), %d variants", time.Since(startTime).Seconds(), total)
	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}

// openVCF returns a line reader for a VCF path (local fixture or remote gz) plus
// a cleanup func.
func (s *spliceai) openVCF(path string, isFixture bool) (*bufio.Reader, func()) {
	if isFixture {
		f, err := os.Open(path)
		s.check(err, "opening SpliceAI fixture "+path)
		return bufio.NewReaderSize(f, 1<<20), func() { f.Close() }
	}
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", path)
	s.check(err, "opening SpliceAI VCF "+path)
	r := br
	if gz != nil {
		r = bufio.NewReaderSize(gz, 1<<20)
	} else {
		r = bufio.NewReaderSize(br, 1<<20)
	}
	return r, func() { closeReaders(gz, ftpFile, client, localFile) }
}

// nextData returns the next VCF data line's tab fields (>=8), skipping headers
// and blanks. Returns nil at EOF.
func nextData(reader *bufio.Reader) []string {
	for {
		line, rerr := reader.ReadString('\n')
		if len(line) == 0 && rerr == io.EOF {
			return nil
		}
		line = strings.TrimRight(line, "\r\n")
		if line != "" && line[0] != '#' {
			f := strings.Split(line, "\t")
			if len(f) >= 8 {
				return f
			}
		}
		if rerr == io.EOF {
			return nil
		}
	}
}

func variantKey(f []string) string { return f[0] + ":" + f[1] + ":" + f[3] + ":" + f[4] }
func posKey(f []string) string     { return f[0] + ":" + f[1] }

// chrRank maps a chromosome to karyotype sort order (1-22, X, Y, MT) so the
// raw/masked merge-join can decide which file is behind. Both SpliceAI VCFs are
// karyotype-sorted; an unknown contig sorts last.
func chrRank(chrom string) int {
	c := strings.TrimPrefix(strings.TrimPrefix(chrom, "chr"), "CHR")
	switch c {
	case "X":
		return 23
	case "Y":
		return 24
	case "M", "MT":
		return 25
	}
	if n, err := strconv.Atoi(c); err == nil {
		return n
	}
	return 1000
}

// cmpPos compares two VCF rows by (chrRank, position): <0 if a is before b.
func cmpPos(a, b []string) int {
	if ra, rb := chrRank(a[0]), chrRank(b[0]); ra != rb {
		if ra < rb {
			return -1
		}
		return 1
	}
	pa, _ := strconv.ParseInt(a[1], 10, 64)
	pb, _ := strconv.ParseInt(b[1], 10, 64)
	switch {
	case pa < pb:
		return -1
	case pa > pb:
		return 1
	default:
		return 0
	}
}

// groupReader yields consecutive VCF data rows that share the same (chrom,pos),
// using a one-row lookahead. Raw and masked VCFs list the same positions in the
// same order, but the ALT rows within a position can be ordered differently, so
// callers match within a group by full key rather than assuming row alignment.
type groupReader struct {
	r       *bufio.Reader
	pending []string
	done    bool
}

func (g *groupReader) next() [][]string {
	if g.done {
		return nil
	}
	first := g.pending
	g.pending = nil
	if first == nil {
		if first = nextData(g.r); first == nil {
			g.done = true
			return nil
		}
	}
	group := [][]string{first}
	pk := posKey(first)
	for {
		row := nextData(g.r)
		if row == nil {
			g.done = true
			break
		}
		if posKey(row) == pk {
			group = append(group, row)
		} else {
			g.pending = row
			break
		}
	}
	return group
}

// extractSpliceaiInfo pulls the SpliceAI annotation out of a VCF INFO column and
// splits it into its 10 pipe fields:
//
//	ALLELE|SYMBOL|DS_AG|DS_AL|DS_DG|DS_DL|DP_AG|DP_AL|DP_DG|DP_DL
//
// Returns nil if absent/malformed; the first annotation is used if several.
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

// processSNVs streams the raw SNV VCF and, in lockstep, the masked SNV VCF (same
// MANE SNV rows in the same order), matching by full variant key. A kept variant
// (raw max >= threshold) stores raw deltas + the matched masked deltas.
func (s *spliceai) processSNVs(rawPath, maskedPath string, isFixture bool, testLimit int, already int64, idLogFile *os.File, sourceID, textLinkID string) int64 {
	raw, closeRaw := s.openVCF(rawPath, isFixture)
	defer closeRaw()

	var masked *bufio.Reader
	if maskedPath != "" {
		var closeMasked func()
		masked, closeMasked = s.openVCF(maskedPath, isFixture)
		defer closeMasked()
	}

	rawG := &groupReader{r: raw}
	var maskedG *groupReader
	var mg [][]string // current masked position-group (lookahead)
	if masked != nil {
		maskedG = &groupReader{r: masked}
		mg = maskedG.next()
	}

	// Merge-join by (chrRank, pos). For each raw position-group: advance masked past
	// any masked-only positions (behind raw), then if masked is AT the same position
	// build the per-key masked-delta map (alts within the group may be reordered),
	// else masked is ahead -> this is a raw-only position (masked stays empty).
	var count, rawOnly int64
	for {
		rg := rawG.next()
		if rg == nil {
			break
		}
		mmap := map[string][4]string{}
		if maskedG != nil {
			for mg != nil && cmpPos(mg[0], rg[0]) < 0 {
				mg = maskedG.next()
			}
			if mg != nil && cmpPos(mg[0], rg[0]) == 0 {
				for _, mr := range mg {
					if msa := extractSpliceaiInfo(mr[7]); msa != nil {
						mmap[variantKey(mr)] = [4]string{msa[2], msa[3], msa[4], msa[5]}
					}
				}
				mg = maskedG.next()
			} else {
				rawOnly++
			}
		}
		for _, rr := range rg {
			md := mmap[variantKey(rr)] // zero value [4]string{} if no masked match
			if s.storeVariant(rr, md, idLogFile, sourceID, textLinkID) {
				count++
			}
		}
		if testLimit > 0 && already+count >= int64(testLimit) {
			break
		}
	}
	log.Printf("SpliceAI: SNVs — stored %d variants (%d raw-only positions had no masked row)", count, rawOnly)
	return count
}

// processIndels streams the Illumina indel VCF (raw-only; no masked file exists).
func (s *spliceai) processIndels(path string, isFixture bool, testLimit int, already int64, idLogFile *os.File, sourceID, textLinkID string) int64 {
	r, cl := s.openVCF(path, isFixture)
	defer cl()
	var count int64
	for {
		f := nextData(r)
		if f == nil {
			break
		}
		if s.storeVariant(f, [4]string{}, idLogFile, sourceID, textLinkID) {
			count++
			if testLimit > 0 && already+count >= int64(testLimit) {
				break
			}
		}
	}
	log.Printf("SpliceAI: indels — stored %d variants", count)
	return count
}

// storeVariant parses one VCF row, applies the raw>=threshold filter, and stores
// the SpliceAIAttr (raw deltas + the supplied masked deltas) plus cross-refs.
func (s *spliceai) storeVariant(f []string, maskedDeltas [4]string, idLogFile *os.File, sourceID, textLinkID string) bool {
	chrom, posStr, refAllele, altAllele, info := f[0], f[1], f[3], f[4], f[7]
	pos, perr := strconv.ParseInt(posStr, 10, 64)
	if perr != nil {
		return false
	}
	sa := extractSpliceaiInfo(info)
	if sa == nil {
		return false
	}
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
		return false
	}

	entryID := fmt.Sprintf("%s:%d:%s:%s", chrom, pos, refAllele, altAllele)
	if len(entryID) > LMDBMaxKeySize {
		return false
	}

	attr := &pbuf.SpliceAIAttr{
		Chromosome: chrom,
		Position:   pos,
		RefAllele:  refAllele,
		AltAllele:  altAllele,
		Effect:     effect,
		Score:      float32(maxDS),
		GeneSymbol: sa[1],
		DsAg:       sa[2], DsAl: sa[3], DsDg: sa[4], DsDl: sa[5],
		DpAg: sa[6], DpAl: sa[7], DpDg: sa[8], DpDl: sa[9],
		DsAgMasked: maskedDeltas[0], DsAlMasked: maskedDeltas[1],
		DsDgMasked: maskedDeltas[2], DsDlMasked: maskedDeltas[3],
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
	return true
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
