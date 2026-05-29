package update

import (
	"archive/zip"
	"biobtree/pbuf"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// intogen ingests the intOGen Compendium of Cancer Genes (CC0): a computational
// catalog of somatic cancer driver genes with consensus mode of action
// (oncogene/tumor-suppressor) across tumor types and cohorts.
//
// Gene-centric: one entry per driver gene, aggregated across the cohorts where
// it is a significant driver. Disease links are resolved from the cohort's
// free-text cancer name via the shared collectOntologyIDs mapper (intOGen
// ships no DOID), so cancer -> driver-gene routes resolve through MONDO.
type intogen struct {
	source string
	d      *DataUpdate
}

func (it *intogen) check(err error, operation string) {
	checkWithContext(err, it.source, operation)
}

// readZipEntry downloads path+fileParam (a .zip) and returns the bytes of the
// first entry whose name ends with entrySuffix.
func (it *intogen) readZipEntry(fileParam, entrySuffix string) []byte {
	url := config.Dataconf[it.source]["path"] + fileParam
	log.Printf("intOGen: downloading %s", url)
	resp, err := http.Get(url)
	it.check(err, "downloading "+fileParam)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	it.check(err, "reading "+fileParam)

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	it.check(err, "opening zip "+fileParam)

	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, entrySuffix) {
			rc, err := f.Open()
			it.check(err, "opening entry "+f.Name)
			defer rc.Close()
			data, err := io.ReadAll(rc)
			it.check(err, "reading entry "+f.Name)
			return data
		}
	}
	log.Printf("intOGen: entry %q not found in %s", entrySuffix, fileParam)
	return nil
}

// parseTSVBytes parses an in-memory TSV into a column->index map and rows.
func parseTSVBytes(data []byte) (map[string]int, [][]string) {
	if len(data) == 0 {
		return nil, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	if !scanner.Scan() {
		return nil, nil
	}
	header := strings.Split(scanner.Text(), "\t")
	col := make(map[string]int, len(header))
	for i, n := range header {
		col[strings.TrimSpace(strings.Trim(n, "\""))] = i
	}
	var rows [][]string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}
	return col, rows
}

func mapBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// consensusRole returns the majority role from per-cohort calls. Ties break
// toward a definite (non-ambiguous) call, then lexically, for determinism.
func consensusRole(counts map[string]int) string {
	best, bestN := "ambiguous", -1
	for role, n := range counts {
		if n > bestN || (n == bestN && (best == "ambiguous" && role != "ambiguous" || (best != "ambiguous" && role != "ambiguous" && role < best))) {
			best, bestN = role, n
		}
	}
	return best
}

type intogenGene struct {
	roleCounts  map[string]int
	transcript  string
	cancerTypes map[string]bool
	cancerNames map[string]bool
	methods     map[string]bool
	cohorts     map[string]bool
	pmids       map[string]bool
	samples     int64
	mutations   int64
}

func (it *intogen) update() {
	defer it.d.wg.Done()
	log.Println("intOGen: starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[it.source]["id"]
	mappings := LoadMedicalTermMappings()
	var mondoInt uint32
	if id, ok := config.Dataconf["mondo"]["id"]; ok {
		fmt.Sscanf(id, "%d", &mondoInt)
	}

	// 1. Cohorts: cohort -> cancer name + supporting PMID.
	ccol, crows := parseTSVBytes(it.readZipEntry(config.Dataconf[it.source]["cohortsFile"], "cohorts.tsv"))
	cohortName := make(map[string]string)
	cohortPMID := make(map[string]string)
	for _, r := range crows {
		ch := getCol(r, ccol, "COHORT")
		if ch == "" {
			continue
		}
		cohortName[ch] = getCol(r, ccol, "CANCER_NAME")
		if ref := getCol(r, ccol, "REFERENCE"); strings.HasPrefix(ref, "PMID:") {
			cohortPMID[ch] = strings.TrimPrefix(ref, "PMID:")
		}
	}

	// 2. Drivers: aggregate per gene across cohorts.
	dcol, drows := parseTSVBytes(it.readZipEntry(config.Dataconf[it.source]["driversFile"], "Compendium_Cancer_Genes.tsv"))
	genes := make(map[string]*intogenGene)
	var order []string
	for _, r := range drows {
		sym := getCol(r, dcol, "SYMBOL")
		if sym == "" {
			continue
		}
		g := genes[sym]
		if g == nil {
			g = &intogenGene{
				roleCounts:  map[string]int{},
				cancerTypes: map[string]bool{}, cancerNames: map[string]bool{},
				methods: map[string]bool{}, cohorts: map[string]bool{}, pmids: map[string]bool{},
			}
			genes[sym] = g
			order = append(order, sym)
		}
		if g.transcript == "" {
			g.transcript = getCol(r, dcol, "TRANSCRIPT")
		}
		// ROLE is a per-cohort call; the gene's consensus is the majority vote.
		if role := getCol(r, dcol, "ROLE"); role != "" {
			g.roleCounts[role]++
		}
		if ct := getCol(r, dcol, "CANCER_TYPE"); ct != "" {
			g.cancerTypes[ct] = true
		}
		if ch := getCol(r, dcol, "COHORT"); ch != "" {
			g.cohorts[ch] = true
			if n := cohortName[ch]; n != "" {
				g.cancerNames[n] = true
			}
			if p := cohortPMID[ch]; p != "" {
				g.pmids[p] = true
			}
		}
		for _, m := range strings.Split(getCol(r, dcol, "METHODS"), ",") {
			if m = strings.TrimSpace(m); m != "" {
				g.methods[m] = true
			}
		}
		if s, err := strconv.ParseInt(getCol(r, dcol, "SAMPLES"), 10, 64); err == nil {
			g.samples += s
		}
		if mu, err := strconv.ParseInt(getCol(r, dcol, "MUTATIONS"), 10, 64); err == nil {
			g.mutations += mu
		}
	}

	nameMondoCache := map[string][]string{}
	var total uint64
	for _, sym := range order {
		g := genes[sym]
		attr := pbuf.IntogenAttr{
			Symbol:         sym,
			Role:           consensusRole(g.roleCounts),
			Transcript:     g.transcript,
			CancerTypes:    mapBoolKeys(g.cancerTypes),
			CancerNames:    mapBoolKeys(g.cancerNames),
			Methods:        mapBoolKeys(g.methods),
			NumCohorts:     int64(len(g.cohorts)),
			TotalSamples:   g.samples,
			TotalMutations: g.mutations,
		}
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			continue
		}
		it.d.addProp3(sym, sourceID, b)

		// Text search by gene symbol.
		it.d.addXref(sym, textLinkID, sym, it.source, true)

		// Gene hub: symbol -> HGNC / Entrez / Ensembl.
		it.d.addHumanGeneXrefsAll(sym, sym, sourceID)

		// Disease: cohort cancer name -> MONDO via the shared mapper (cached;
		// cancer names repeat heavily across genes).
		mondoSeen := map[string]bool{}
		for name := range g.cancerNames {
			ids, ok := nameMondoCache[name]
			if !ok {
				ids = mapBoolKeys(collectOntologyIDs(it.d, mappings, name, mondoInt))
				nameMondoCache[name] = ids
			}
			for _, mid := range ids {
				if !mondoSeen[mid] {
					mondoSeen[mid] = true
					it.d.addXref(sym, sourceID, mid, "mondo", false)
				}
			}
		}

		// Supporting literature.
		for p := range g.pmids {
			if isAllDigits(p) {
				it.d.addXref(sym, sourceID, p, "pubmed", false)
			}
		}
		total++
	}

	it.d.progChan <- &progressInfo{dataset: it.source, done: true}
	atomic.AddUint64(&it.d.totalParsedEntry, total)
	log.Printf("intOGen: complete - %d driver genes (%.2fs)", total, time.Since(startTime).Seconds())
}
