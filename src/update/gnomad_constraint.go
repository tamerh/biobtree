package update

import (
	"biobtree/pbuf"
	"bufio"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type gnomadConstraint struct {
	source string
	d      *DataUpdate
}

func (g *gnomadConstraint) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

// gcRow holds the constraint metrics selected for one gene (one transcript).
type gcRow struct {
	rec      []string
	mane     bool
	canonand bool // canonical (used only as fallback when no MANE row exists)
}

// gnomadParseFloat parses a constraint metric, tolerating empty / "NA" values.
// Returns ok=false when the field should be skipped.
func gnomadParseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" || s == "NaN" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func (g *gnomadConstraint) update() {
	defer g.d.wg.Done()

	log.Println("gnomAD Constraint: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[g.source]["id"]
	path := config.Dataconf[g.source]["path"]

	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("gnomAD Constraint: [TEST MODE] Processing up to %d genes", testLimit)
	}

	// Open the plain TSV (local or HTTP)
	var rawReader *bufio.Reader
	if config.Dataconf[g.source]["useLocalFile"] == "yes" {
		f, err := os.Open(filepath.FromSlash(path))
		g.check(err, "opening local gnomAD constraint file")
		defer f.Close()
		rawReader = bufio.NewReaderSize(f, fileBufSize)
	} else {
		resp, err := http.Get(path)
		g.check(err, "downloading gnomAD constraint data")
		defer resp.Body.Close()
		rawReader = bufio.NewReaderSize(resp.Body, fileBufSize)
	}

	scanner := bufio.NewScanner(rawReader)
	const maxCapacity = 512 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	// Read header and resolve column indices by name.
	if !scanner.Scan() {
		g.check(io.ErrUnexpectedEOF, "reading gnomAD constraint header")
	}
	header := strings.Split(scanner.Text(), "\t")
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	idxGene := col["gene"]
	idxGeneID := col["gene_id"]
	idxTranscript := col["transcript"]
	idxCanonical := col["canonical"]
	idxMane := col["mane_select"]
	idxPLI := col["lof.pLI"]
	idxLoeuf := col["lof.oe_ci.upper"]
	idxOeLof := col["lof.oe"]
	idxLofZ := col["lof.z_score"]
	idxOeMis := col["mis.oe"]
	idxMisZ := col["mis.z_score"]
	idxOeSyn := col["syn.oe"]
	idxSynZ := col["syn.z_score"]
	idxObsLof := col["lof.obs"]
	idxExpLof := col["lof.exp"]
	idxFlags := col["constraint_flags"]

	get := func(rec []string, i int) string {
		if i >= 0 && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	// First pass: pick one transcript per ENSG (prefer MANE-select, else canonical).
	best := map[string]gcRow{}
	var previous int64
	scanned := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		rec := strings.Split(line, "\t")
		ensg := get(rec, idxGeneID)
		if !strings.HasPrefix(ensg, "ENSG") {
			continue // skip RefSeq rows (and any non-Ensembl gene ids)
		}
		scanned++

		elapsed := int64(time.Since(g.d.start).Seconds())
		if elapsed > previous+g.d.progInterval {
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: 0}
		}

		mane := strings.EqualFold(get(rec, idxMane), "true")
		canon := strings.EqualFold(get(rec, idxCanonical), "true")
		cur := gcRow{rec: rec, mane: mane, canonand: canon}

		prev, exists := best[ensg]
		if !exists {
			best[ensg] = cur
			continue
		}
		// Prefer MANE-select over anything; otherwise prefer canonical.
		if mane && !prev.mane {
			best[ensg] = cur
		} else if !prev.mane && canon && !prev.canonand {
			best[ensg] = cur
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("gnomAD Constraint: Error reading data: %v", err)
	}
	log.Printf("gnomAD Constraint: %d ENSG rows scanned, %d unique genes", scanned, len(best))

	// Second pass: emit one record per gene.
	var total uint64
	var entryCount int64
	for ensg, row := range best {
		rec := row.rec
		symbol := get(rec, idxGene)

		attr := pbuf.GnomadConstraintAttr{
			GeneSymbol:      symbol,
			GeneId:          ensg,
			Transcript:      get(rec, idxTranscript),
			ConstraintFlags: get(rec, idxFlags),
		}
		if v, ok := gnomadParseFloat(get(rec, idxPLI)); ok {
			attr.Pli = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxLoeuf)); ok {
			attr.Loeuf = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxOeLof)); ok {
			attr.OeLof = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxLofZ)); ok {
			attr.LofZ = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxOeMis)); ok {
			attr.OeMis = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxMisZ)); ok {
			attr.MisZ = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxOeSyn)); ok {
			attr.OeSyn = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxSynZ)); ok {
			attr.SynZ = v
		}
		if v, ok := gnomadParseFloat(get(rec, idxObsLof)); ok {
			attr.ObsLof = int32(v)
		}
		if v, ok := gnomadParseFloat(get(rec, idxExpLof)); ok {
			attr.ExpLof = v
		}

		attrBytes, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("gnomAD Constraint: Error marshaling %s: %v", ensg, err)
			continue
		}

		// Key the record by ENSG; link to ensembl and reach the wider gene
		// namespace (hgnc/entrez) via the symbol.
		g.d.addProp3(ensg, sourceID, attrBytes)
		g.d.addXref(ensg, sourceID, ensg, "ensembl", false)
		// Link the constraint record to its representative Ensembl transcript so
		// gene constraint is reachable via transcript (strip any version suffix).
		if enst := strings.Split(attr.Transcript, ".")[0]; strings.HasPrefix(enst, "ENST") {
			g.d.addXref(ensg, sourceID, enst, "transcript", false)
		}
		if symbol != "" {
			g.d.addXref(symbol, textLinkID, ensg, g.source, true)
			g.d.addHumanGeneXrefsAll(symbol, ensg, sourceID)
		}
		if idLogFile != nil {
			logProcessedID(idLogFile, ensg)
		}

		total++
		entryCount++
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			break
		}
	}

	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
	atomic.AddUint64(&g.d.totalParsedEntry, total)

	log.Printf("gnomAD Constraint: Processing complete - %d genes saved (%.2fs)", total, time.Since(startTime).Seconds())
}
