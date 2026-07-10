package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/csv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// mavedb ingests MaveDB (Multiplexed Assays of Variant Effect) — the first
// functional-assay source in biobtree. It provides direct EXPERIMENTAL
// functional evidence (PS3/BS3-grade) for protein/nucleotide variants.
//
// Data shape: MaveDB publishes a biannual (May/Nov) bulk archive on Zenodo
// (mavedb-dump.<ts>.zip), plus a live API at api.mavedb.org. The unzipped
// archive is a directory containing:
//   - main.json  : {title, asOf, experimentSets:[{experiments:[{scoreSets:[...]}]}]}
//                  — score sets are nested under the Experiment Set / Experiment
//                  hierarchy. Each score set has urn, title, license.shortName,
//                  and targetGenes[] (name, category, externalIdentifiers,
//                  targetSequence.taxonomy.taxId).
//   - csv/<urn ":"→"-">.scores.csv : one scores CSV per score set. Columns:
//                  accession,hgvs_nt,hgvs_splice,hgvs_pro,score,sd,se,...
//                  (per-set *.counts.csv also exist but are not ingested).
//
// The per-score-set data LICENSE (license.shortName, e.g. "CC0") is stored on
// every variant so the KG export can treat CC0 sets as public-domain and any
// non-CC0 set under its own terms.
//
// KEY SCHEME: entries are keyed by the STABLE per-variant MAVE URN
// (e.g. "urn:mavedb:00000001-a-1#123"). The functional evidence is made
// reachable from the biological anchor by xref'ing each variant to its target
// UniProt accession (protein-HGVS targets), RefSeq/Ensembl transcript
// (nucleotide-HGVS targets), and gene/HGNC (derived from the score-set target
// metadata). Only HUMAN (tax 9606), mappable targets are ingested.
//
// SCOPE (v1): HGVS->genomic coordinate linkage is OUT of scope. The join is at
// protein / transcript / gene level only. The raw MAVE-HGVS string and the
// measured score are stored as attrs (and the score is carried as edge
// evidence onto the anchor xrefs so a gene>>mavedb traversal surfaces it).
type mavedb struct {
	source string
	d      *DataUpdate
}

func (m *mavedb) check(err error, operation string) {
	checkWithContext(err, m.source, operation)
}

// mavedbTaxonomy mirrors the targetSequence.taxonomy block of a score set.
type mavedbTaxonomy struct {
	TaxId        int    `json:"taxId"`
	OrganismName string `json:"organismName"`
}

// mavedbExtIdent mirrors one externalIdentifiers[].identifier block.
type mavedbExtIdent struct {
	Identifier struct {
		DbName     string `json:"dbName"`
		Identifier string `json:"identifier"`
	} `json:"identifier"`
}

// mavedbTargetGene mirrors one targetGenes[] block.
type mavedbTargetGene struct {
	Name                string           `json:"name"`
	Category            string           `json:"category"`
	ExternalIdentifiers []mavedbExtIdent `json:"externalIdentifiers"`
	TargetSequence      struct {
		Taxonomy mavedbTaxonomy `json:"taxonomy"`
	} `json:"targetSequence"`
}

// mavedbScoreSet mirrors one score-set metadata object.
type mavedbScoreSet struct {
	Urn     string `json:"urn"`
	Title   string `json:"title"`
	License struct {
		ShortName string `json:"shortName"` // e.g. "CC0", "CC BY-NC-SA 4.0"
	} `json:"license"`
	TargetGenes []mavedbTargetGene `json:"targetGenes"`
}

// mavedbArchive mirrors the Zenodo bulk main.json, which nests score sets under
// experimentSets[] -> experiments[] -> scoreSets[] (Experiment Set / Experiment
// / Score Set hierarchy).
type mavedbArchive struct {
	ExperimentSets []struct {
		Experiments []struct {
			ScoreSets []mavedbScoreSet `json:"scoreSets"`
		} `json:"experiments"`
	} `json:"experimentSets"`
}

// resolved target context extracted from a score set's targetGenes.
type mavedbTarget struct {
	gene     string
	category string
	uniprot  string
	refseq   string
	ensembl  string
	human    bool
}

// resolveTarget picks the first target gene and extracts the mappable anchors.
// A score set with no human target (or no mappable identifier) is skipped.
func resolveTarget(ss *mavedbScoreSet) mavedbTarget {
	var t mavedbTarget
	if len(ss.TargetGenes) == 0 {
		return t
	}
	tg := ss.TargetGenes[0]
	t.gene = strings.TrimSpace(tg.Name)
	t.category = strings.TrimSpace(tg.Category)
	// Human only: taxonomy taxId 9606.
	t.human = tg.TargetSequence.Taxonomy.TaxId == 9606
	for _, ei := range tg.ExternalIdentifiers {
		id := strings.TrimSpace(ei.Identifier.Identifier)
		if id == "" {
			continue
		}
		switch strings.ToLower(ei.Identifier.DbName) {
		case "uniprot":
			t.uniprot = id
		case "refseq":
			t.refseq = id
		case "ensembl":
			t.ensembl = id
		}
	}
	return t
}

func isMavedbNull(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "", "NA", "N/A", "NONE", "NULL", ".":
		return true
	}
	return false
}

func (m *mavedb) update() {
	defer m.d.wg.Done()
	log.Println("MaveDB: starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[m.source]["id"]
	dir := config.Dataconf[m.source]["path"]

	// Test-mode: cap variants and log processed URNs for reference extraction.
	testLimit := config.GetTestLimit(m.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, m.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("MaveDB: [TEST MODE] processing up to %d variants", testLimit)
	}

	scoreSets := m.readScoreSets(dir)
	log.Printf("MaveDB: %d score sets in archive metadata", len(scoreSets))

	var total uint64
	var scoreSetsUsed int
	for i := range scoreSets {
		ss := &scoreSets[i]
		if ss.Urn == "" {
			continue
		}
		tgt := resolveTarget(ss)
		// Filter to human + at least one mappable biological anchor.
		if !tgt.human {
			continue
		}
		if tgt.uniprot == "" && tgt.refseq == "" && tgt.ensembl == "" && tgt.gene == "" {
			continue
		}

		n := m.processScoreSet(ss, &tgt, sourceID, testLimit, idLogFile, &total)
		if n > 0 {
			scoreSetsUsed++
		}
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}
	}

	atomic.AddUint64(&m.d.totalParsedEntry, total)
	m.d.progChan <- &progressInfo{dataset: m.source, done: true}

	log.Printf("MaveDB: complete - %d variants across %d human score sets (%.2fs)",
		total, scoreSetsUsed, time.Since(startTime).Seconds())
}

// readScoreSets loads and parses main.json from the archive directory.
func (m *mavedb) readScoreSets(dir string) []mavedbScoreSet {
	metaPath := filepath.Join(filepath.FromSlash(dir), "main.json")
	f, err := os.Open(metaPath)
	m.check(err, "opening "+metaPath)
	defer f.Close()

	data, err := io.ReadAll(bufio.NewReaderSize(f, fileBufSize))
	m.check(err, "reading "+metaPath)

	var archive mavedbArchive
	if err := ffjson.Unmarshal(data, &archive); err != nil {
		m.check(err, "parsing "+metaPath)
	}
	var scoreSets []mavedbScoreSet
	for _, es := range archive.ExperimentSets {
		for _, exp := range es.Experiments {
			scoreSets = append(scoreSets, exp.ScoreSets...)
		}
	}
	return scoreSets
}

// scoreCSVName returns the scores CSV path for a score-set URN, matching the
// Zenodo bulk archive layout: csv/<urn with ":" -> "-">.scores.csv
// (e.g. "urn:mavedb:00000001-a-1" -> "csv/urn-mavedb-00000001-a-1.scores.csv").
func scoreCSVName(urn string) string {
	return "csv/" + strings.ReplaceAll(urn, ":", "-") + ".scores.csv"
}

// processScoreSet parses the per-score-set scores CSV and emits one entry per
// variant plus its biological-anchor xrefs. Returns the number of variants
// stored for this score set.
func (m *mavedb) processScoreSet(ss *mavedbScoreSet, tgt *mavedbTarget, sourceID string,
	testLimit int, idLogFile *os.File, total *uint64) uint64 {

	dir := config.Dataconf[m.source]["path"]
	csvPath := filepath.Join(filepath.FromSlash(dir), scoreCSVName(ss.Urn))
	f, err := os.Open(csvPath)
	if err != nil {
		// A metadata entry without its score CSV in the (sampled) archive is
		// not fatal — just skip it.
		log.Printf("MaveDB: score CSV missing for %s (%v), skipping", ss.Urn, err)
		return 0
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReaderSize(f, fileBufSize))
	r.FieldsPerRecord = -1 // rows have a trailing variable set of score columns

	header, err := r.Read()
	if err != nil {
		log.Printf("MaveDB: empty/invalid CSV for %s (%v), skipping", ss.Urn, err)
		return 0
	}
	col := make(map[string]int, len(header))
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	get := func(row []string, name string) string {
		if i, ok := col[name]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}

	var n uint64
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("MaveDB: read error in %s: %v", ss.Urn, err)
			break
		}
		urn := get(row, "accession")
		if urn == "" || !strings.HasPrefix(urn, "urn:mavedb:") {
			continue
		}

		hgvsPro := get(row, "hgvs_pro")
		hgvsNt := get(row, "hgvs_nt")
		hgvsSplice := get(row, "hgvs_splice")
		score := get(row, "score")
		if isMavedbNull(hgvsPro) {
			hgvsPro = ""
		}
		if isMavedbNull(hgvsNt) {
			hgvsNt = ""
		}
		if isMavedbNull(hgvsSplice) {
			hgvsSplice = ""
		}
		if isMavedbNull(score) {
			score = ""
		}

		attr := pbuf.MavedbAttr{
			GeneSymbol:    tgt.gene,
			ScoreSet:      ss.Urn,
			ScoreSetTitle: ss.Title,
			HgvsPro:       hgvsPro,
			HgvsNt:        hgvsNt,
			HgvsSplice:    hgvsSplice,
			Score:         score,
			Uniprot:       tgt.uniprot,
			Category:      tgt.category,
			License:       ss.License.ShortName,
		}
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			continue
		}
		m.d.addProp3(urn, sourceID, b)

		// Edge evidence: carry the measured functional score onto the anchor
		// edges so a gene/uniprot >> mavedb traversal surfaces the score.
		evidence := score

		// Protein-HGVS target -> UniProt (the primary functional anchor).
		if tgt.uniprot != "" {
			m.d.addXrefWithEvidence(urn, sourceID, tgt.uniprot, "uniprot", false, evidence)
		}
		// Nucleotide/splice-HGVS target -> RefSeq/Ensembl transcript.
		if tgt.refseq != "" {
			enst := strings.SplitN(tgt.refseq, ".", 2)[0]
			m.d.addXrefWithEvidence(urn, sourceID, enst, "transcript", false, evidence)
		}
		if tgt.ensembl != "" && strings.HasPrefix(tgt.ensembl, "ENSG") {
			ensg := strings.SplitN(tgt.ensembl, ".", 2)[0]
			m.d.addXrefWithEvidence(urn, sourceID, ensg, "ensembl", false, evidence)
		}
		// Gene-hub edge via HGNC so gene-symbol / hgnc >> mavedb resolves.
		if tgt.gene != "" {
			m.d.addHumanGeneXrefsViaHGNC(tgt.gene, urn, sourceID)
		}

		// Text search: findable by the target gene symbol and score-set title.
		if tgt.gene != "" {
			m.d.addXref(tgt.gene, textLinkID, urn, m.source, true)
		}
		if ss.Title != "" {
			m.d.addXref(ss.Title, textLinkID, urn, m.source, true)
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, urn)
		}
		n++
		*total++
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(*total)) {
			break
		}
	}
	return n
}
