package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// drugcentral handles parsing DrugCentral data.
// Source: https://drugcentral.org/  (License: CC BY-SA 4.0)
//
// One entry per DrugCentral drug, keyed by DrugCentral struct_id. The dataset
// captures the curated drug->target mechanism-of-action layer plus regulatory
// approval status, built entirely from the public static downloads:
//
//   - drug.target.interaction.tsv.gz : drug->target interactions. Columns:
//     DRUG_NAME, STRUCT_ID, TARGET_NAME, TARGET_CLASS, ACCESSION, GENE,
//     SWISSPROT, ACT_VALUE, ACT_UNIT, ACT_TYPE, ACT_COMMENT, ACT_SOURCE,
//     RELATION, MOA, MOA_SOURCE, ACT_SOURCE_URL, MOA_SOURCE_URL, ACTION_TYPE,
//     TDL, ORGANISM
//   - structures.smiles.tsv : SMILES, InChI, InChIKey, ID(struct_id), INN, CAS_RN
//   - FDA/EMA/PMDA_Approved.csv : struct_id,name lists of agency-approved drugs
//
// Edges emitted:
//
//	drugcentral -> uniprot   (human target accessions; one xref per accession)
//	drugcentral -> hgnc       (target gene symbols, via canonical resolver)
//	drug name / INN / InChIKey indexed as text keywords (the InChIKey keyword is
//	also indexed by ChEMBL, so a shared structure resolves drugcentral<->chembl_molecule).
//
// Note: ATC codes and indication terms are NOT in the public static downloads
// (they live only in the DrugCentral PostgreSQL dump / live instance), so they
// are intentionally out of scope here rather than producing half-baked edges.
type drugcentral struct {
	source  string
	d       *DataUpdate
	medical *MedicalTermMappings
}

// drugcentralExtras holds the indication + ATC data parsed from the DrugCentral
// pg_dump (these are not in the static TSV/CSV downloads).
type drugcentralExtras struct {
	indications map[string][]string // struct_id -> on-label indication disease names
	atcCodes    map[string][]string // struct_id -> WHO ATC codes (e.g. P01BA06)
	atcClasses  map[string][]string // struct_id -> distinct ATC class names (l2/l3/l4)
}

func (dc *drugcentral) check(err error, operation string) {
	checkWithContext(err, dc.source, operation)
}

// drugcentralStructure holds the per-struct_id data merged from structures.smiles.tsv.
type drugcentralStructure struct {
	inn      string
	casRN    string
	inchiKey string
}

// drugcentralEntry aggregates one DrugCentral drug across interaction rows.
type drugcentralEntry struct {
	structID    string
	name        string
	inn         string
	casRN       string
	inchiKey    string
	targets     map[string]bool // human target UniProt accessions
	moaTargets  map[string]bool // accessions flagged MOA=1
	actionTypes map[string]bool // distinct action types
	genes       map[string]bool // target gene symbols (human)
}

func (dc *drugcentral) update() {
	defer dc.d.wg.Done()

	log.Println("DrugCentral: Starting data processing...")
	startTime := time.Now()

	testLimit := config.GetTestLimit(dc.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, dc.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("DrugCentral: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Phase 1: structures (struct_id -> INN/CAS/InChIKey)
	structures := dc.loadStructures()
	log.Printf("DrugCentral: loaded %d structure records", len(structures))

	// Phase 2: regulatory approval struct_id sets
	fdaApproved := dc.loadApprovedSet("fdaApprovedUrl")
	emaApproved := dc.loadApprovedSet("emaApprovedUrl")
	pmdaApproved := dc.loadApprovedSet("pmdaApprovedUrl")
	log.Printf("DrugCentral: approvals FDA=%d EMA=%d PMDA=%d", len(fdaApproved), len(emaApproved), len(pmdaApproved))

	// Phase 3: drug-target interactions -> aggregate per drug
	entries := dc.loadInteractions(structures, testLimit)
	log.Printf("DrugCentral: aggregated %d drugs from interactions", len(entries))

	// Phase 3b: indications + ATC classification from the pg_dump (not in the
	// static downloads). Disease-name -> ontology resolution for indication edges
	// needs the medical-term matcher and the lookup DB.
	dc.medical = LoadMedicalTermMappings()
	extras := dc.loadPgDumpExtras()
	log.Printf("DrugCentral: pg_dump extras: %d drugs w/ indications, %d w/ ATC codes",
		len(extras.indications), len(extras.atcCodes))

	// Phase 4: save entries + cross-references
	dc.saveEntries(entries, fdaApproved, emaApproved, pmdaApproved, extras, idLogFile)

	log.Printf("DrugCentral: Processing complete (%.2fs)", time.Since(startTime).Seconds())
	dc.d.progChan <- &progressInfo{dataset: dc.source, done: true}
}

// openURL opens an HTTP resource, transparently gunzipping a .gz URL.
func (dc *drugcentral) openURL(url string) (io.Reader, func()) {
	log.Printf("DrugCentral: Downloading %s", url)
	resp, err := http.Get(url)
	dc.check(err, "HTTP GET "+url)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		log.Fatalf("DrugCentral: HTTP error %d for %s", resp.StatusCode, url)
	}

	if strings.HasSuffix(url, ".gz") {
		gz, gzErr := gzip.NewReader(resp.Body)
		dc.check(gzErr, "creating gzip reader for "+url)
		return gz, func() { gz.Close(); resp.Body.Close() }
	}
	return resp.Body, func() { resp.Body.Close() }
}

// loadStructures reads structures.smiles.tsv into struct_id -> structure data.
// Columns: SMILES, InChI, InChIKey, ID, INN, CAS_RN
func (dc *drugcentral) loadStructures() map[string]*drugcentralStructure {
	out := make(map[string]*drugcentralStructure)
	url := config.Dataconf[dc.source]["path"] + config.Dataconf[dc.source]["structuresFile"]
	reader, cleanup := dc.openURL(url)
	defer cleanup()

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var headerParsed bool
	var colMap map[string]int
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if !headerParsed {
			colMap = make(map[string]int)
			for i, name := range fields {
				colMap[strings.TrimSpace(strings.ToLower(name))] = i
			}
			headerParsed = true
			continue
		}
		structID := safeFieldByColLower(fields, colMap, "id")
		if structID == "" {
			continue
		}
		out[structID] = &drugcentralStructure{
			inn:      safeFieldByColLower(fields, colMap, "inn"),
			casRN:    safeFieldByColLower(fields, colMap, "cas_rn"),
			inchiKey: safeFieldByColLower(fields, colMap, "inchikey"),
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("DrugCentral: Scanner error reading structures: %v", err)
	}
	return out
}

// loadApprovedSet reads an agency-approved CSV (struct_id,name) into a set of struct_ids.
func (dc *drugcentral) loadApprovedSet(confKey string) map[string]bool {
	out := make(map[string]bool)
	url := config.Dataconf[dc.source][confKey]
	if url == "" {
		return out
	}
	reader, cleanup := dc.openURL(url)
	defer cleanup()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if len(record) == 0 {
			continue
		}
		structID := strings.TrimSpace(record[0])
		// First column is the numeric struct_id; skip any header row.
		if structID == "" || !isAllDigits(structID) {
			continue
		}
		out[structID] = true
	}
	return out
}

// loadInteractions streams drug.target.interaction.tsv.gz, aggregating one entry
// per struct_id with its distinct human targets, MOA targets, action types and genes.
func (dc *drugcentral) loadInteractions(structures map[string]*drugcentralStructure, testLimit int) map[string]*drugcentralEntry {
	entries := make(map[string]*drugcentralEntry)
	url := config.Dataconf[dc.source]["path"] + config.Dataconf[dc.source]["interactionFile"]
	reader, cleanup := dc.openURL(url)
	defer cleanup()

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var headerParsed bool
	var colMap map[string]int
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		// DrugCentral quotes every field; strip the surrounding quotes per field.
		fields := splitTSVUnquote(line)
		if !headerParsed {
			colMap = make(map[string]int)
			for i, name := range fields {
				colMap[strings.TrimSpace(strings.ToLower(name))] = i
			}
			headerParsed = true
			continue
		}

		structID := safeFieldByColLower(fields, colMap, "struct_id")
		if structID == "" {
			continue
		}

		entry, exists := entries[structID]
		if !exists {
			// Bound test-mode size by number of distinct drugs.
			if testLimit > 0 && len(entries) >= testLimit {
				continue
			}
			entry = &drugcentralEntry{
				structID:    structID,
				name:        safeFieldByColLower(fields, colMap, "drug_name"),
				targets:     make(map[string]bool),
				moaTargets:  make(map[string]bool),
				actionTypes: make(map[string]bool),
				genes:       make(map[string]bool),
			}
			if s, ok := structures[structID]; ok {
				entry.inn = s.inn
				entry.casRN = s.casRN
				entry.inchiKey = s.inchiKey
			}
			entries[structID] = entry
		}

		organism := safeFieldByColLower(fields, colMap, "organism")
		accession := safeFieldByColLower(fields, colMap, "accession")
		moa := safeFieldByColLower(fields, colMap, "moa")
		actionType := safeFieldByColLower(fields, colMap, "action_type")
		gene := safeFieldByColLower(fields, colMap, "gene")

		// Restrict target edges to human entries — ACCESSION is the SwissProt
		// accession, but a single row may list several pipe-separated accessions.
		if organism == "Homo sapiens" && accession != "" {
			for _, acc := range strings.Split(accession, "|") {
				acc = strings.TrimSpace(acc)
				if acc == "" || !isValidUniProtAccession(acc) {
					continue
				}
				entry.targets[acc] = true
				if moa == "1" {
					entry.moaTargets[acc] = true
				}
			}
			if gene != "" {
				for _, g := range strings.Split(gene, "|") {
					if g = strings.TrimSpace(g); g != "" {
						entry.genes[g] = true
					}
				}
			}
		}

		if actionType != "" {
			entry.actionTypes[actionType] = true
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("DrugCentral: Scanner error reading interactions: %v", err)
	}
	return entries
}

// loadPgDumpExtras reads the indication + ATC tables that DrugCentral ships only
// in its ~1 GB PostgreSQL dump. src/scripts/drugcentral/drugcentral_prepare.py
// extracts the three small tables once into extrasDir (atc.tsv, struct2atc.tsv,
// omop_indications.tsv); this reads those local files (verbatim pg_dump rows).
// If extrasDir / the files are absent the drug entries are still built, just
// without indications/ATC — so a build without the prep step still succeeds.
func (dc *drugcentral) loadPgDumpExtras() *drugcentralExtras {
	ex := &drugcentralExtras{
		indications: make(map[string][]string),
		atcCodes:    make(map[string][]string),
		atcClasses:  make(map[string][]string),
	}
	dir := config.Dataconf[dc.source]["extrasDir"]
	if dir == "" {
		log.Println("DrugCentral: extrasDir not configured — skipping indications/ATC")
		return ex
	}

	atcClassByCode := make(map[string][]string) // code -> [l2,l3,l4] names (mono codes only)
	structToCodes := make(map[string][]string)  // struct_id -> codes (dedup below)
	indicationSeen := make(map[string]map[string]bool)

	// atc.tsv: id,code,chem,l1c,l1n,l2c,l2n,l3c,l3n,l4c,l4n,count.
	// Class names are taken only from single-substance (mono) codes — combination
	// products (chemical_substance "X and Y") would otherwise attach misleading
	// classes to a drug (e.g. amlodipine -> "HMG CoA reductase inhibitors" from a
	// statin combo). The combo CODES are still kept (below) for completeness/search.
	dc.readExtrasFile(filepath.Join(dir, "atc.tsv"), func(f []string) {
		if len(f) < 11 {
			return
		}
		code := f[1]
		if code == "" || strings.Contains(f[2], " and ") {
			return // skip combination products for class derivation
		}
		var classes []string
		for _, i := range []int{6, 8, 10} { // l2_name, l3_name, l4_name
			if v := pgNullTrim(f[i]); v != "" {
				classes = append(classes, v)
			}
		}
		if len(classes) > 0 {
			atcClassByCode[code] = classes
		}
	})

	// struct2atc.tsv: struct_id,atc_code,id
	dc.readExtrasFile(filepath.Join(dir, "struct2atc.tsv"), func(f []string) {
		if len(f) >= 2 && f[0] != "" && f[1] != "" {
			structToCodes[f[0]] = append(structToCodes[f[0]], f[1])
		}
	})

	// omop_indications.tsv: id,struct_id,concept_id,relationship_name,concept_name,...
	// (prep already filtered to relationship_name='indication'; re-checked here).
	dc.readExtrasFile(filepath.Join(dir, "omop_indications.tsv"), func(f []string) {
		if len(f) < 5 || f[3] != "indication" {
			return
		}
		sid, name := f[1], pgNullTrim(f[4])
		if sid == "" || name == "" {
			return
		}
		if indicationSeen[sid] == nil {
			indicationSeen[sid] = make(map[string]bool)
		}
		if !indicationSeen[sid][name] {
			indicationSeen[sid][name] = true
			ex.indications[sid] = append(ex.indications[sid], name)
		}
	})

	// Post-process: per struct_id, distinct codes and the union of their class names.
	for sid, codes := range structToCodes {
		seenCode := make(map[string]bool)
		seenClass := make(map[string]bool)
		for _, code := range codes {
			if !seenCode[code] {
				seenCode[code] = true
				ex.atcCodes[sid] = append(ex.atcCodes[sid], code)
			}
			for _, cls := range atcClassByCode[code] {
				if !seenClass[cls] {
					seenClass[cls] = true
					ex.atcClasses[sid] = append(ex.atcClasses[sid], cls)
				}
			}
		}
	}
	return ex
}

// readExtrasFile reads a tab-separated DrugCentral extras file, calling fn with
// the split fields of each row. A missing file is logged and skipped (the prep
// step may not have been run), not fatal.
func (dc *drugcentral) readExtrasFile(path string, fn func(fields []string)) {
	file, err := os.Open(filepath.FromSlash(path))
	if err != nil {
		log.Printf("DrugCentral: extras file %s not found (%v) — skipping", path, err)
		return
	}
	defer file.Close()

	br := bufio.NewReaderSize(file, 1024*1024)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			fn(strings.Split(strings.TrimRight(line, "\r\n"), "\t"))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("DrugCentral: error reading %s: %v", path, err)
			return
		}
	}
}

// pgNullTrim maps the pg_dump NULL token "\N" to the empty string.
func pgNullTrim(s string) string {
	s = strings.TrimSpace(s)
	if s == `\N` {
		return ""
	}
	return s
}

// saveEntries marshals and stores each drug entry and creates its cross-references.
func (dc *drugcentral) saveEntries(entries map[string]*drugcentralEntry, fda, ema, pmda map[string]bool, extras *drugcentralExtras, idLogFile *os.File) {
	sourceID := config.Dataconf[dc.source]["id"]
	var savedCount int

	for structID, entry := range entries {
		targets := sortedSetKeys(entry.targets)
		moaTargets := sortedSetKeys(entry.moaTargets)
		actionTypes := sortedSetKeys(entry.actionTypes)

		attr := &pbuf.DrugcentralAttr{
			Name:         entry.name,
			Inn:          entry.inn,
			CasRn:        entry.casRN,
			Inchikey:     entry.inchiKey,
			Targets:      targets,
			MoaTargets:   moaTargets,
			ActionTypes:  actionTypes,
			TargetCount:  int32(len(targets)),
			FdaApproved:  fda[structID],
			EmaApproved:  ema[structID],
			PmdaApproved: pmda[structID],
			Indications:  extras.indications[structID],
			AtcCodes:     extras.atcCodes[structID],
			AtcClasses:   extras.atcClasses[structID],
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("DrugCentral: Error marshaling %s: %v", structID, err)
			continue
		}
		dc.d.addProp3(structID, sourceID, attrBytes)

		dc.createCrossRefs(structID, entry, sourceID, targets, extras)

		if idLogFile != nil {
			idLogFile.WriteString(structID + "\n")
		}

		savedCount++
		if savedCount%1000 == 0 {
			log.Printf("DrugCentral: Saved %d drug entries...", savedCount)
		}
	}

	atomic.AddUint64(&dc.d.totalParsedEntry, uint64(savedCount))
	log.Printf("DrugCentral: Total drug entries saved: %d", savedCount)

	// Quantify drugs that carry indications/ATC but have no human-target
	// interaction entry (so they are not emitted today). Reported, not ingested —
	// expanding the entry universe to all DrugCentral drugs is a separate decision.
	var indOnly, atcOnly int
	for sid := range extras.indications {
		if _, ok := entries[sid]; !ok {
			indOnly++
		}
	}
	for sid := range extras.atcCodes {
		if _, ok := entries[sid]; !ok {
			atcOnly++
		}
	}
	if indOnly > 0 || atcOnly > 0 {
		log.Printf("DrugCentral: %d drugs with indications and %d with ATC have no interaction entry (not emitted)", indOnly, atcOnly)
	}
}

// createCrossRefs builds text-search keywords and database edges for a drug.
func (dc *drugcentral) createCrossRefs(structID string, entry *drugcentralEntry, sourceID string, targets []string, extras *drugcentralExtras) {
	// Text search: drug name and INN.
	if entry.name != "" {
		dc.d.addXref(entry.name, textLinkID, structID, dc.source, true)
	}
	if entry.inn != "" && !strings.EqualFold(entry.inn, entry.name) {
		dc.d.addXref(entry.inn, textLinkID, structID, dc.source, true)
	}

	// InChIKey: index as a text keyword (searchable) AND resolve it through the
	// lookup DB to the real chembl_molecule / pubchem nodes, adding graph edges so
	// the drug is edge-reachable via map chains (e.g. chembl_molecule >> drugcentral),
	// not merely co-findable by a shared keyword.
	if entry.inchiKey != "" {
		dc.d.addXref(entry.inchiKey, textLinkID, structID, dc.source, true)
		dc.linkStructure(structID, entry.inchiKey, sourceID)
	}

	// Drug -> target (human UniProt accessions).
	if _, exists := config.Dataconf["uniprot"]; exists {
		for _, acc := range targets {
			dc.d.addXref(structID, sourceID, acc, "uniprot", false)
		}
	}

	// Drug -> target gene (canonical HGNC / Entrez / Ensembl resolution).
	for g := range entry.genes {
		dc.d.addHumanGeneXrefsAll(g, structID, sourceID)
	}

	// ATC codes: index as searchable text keywords, so "P01BA06" finds the drug.
	for _, code := range extras.atcCodes[structID] {
		dc.d.addXref(code, textLinkID, structID, dc.source, true)
	}

	// On-label indications: resolve each disease name to the MONDO/EFO ontology
	// (precise whole-term matcher, same path ncrna_disease uses) and add a
	// drug -> disease edge, so approved indications surface on both drug and
	// disease pages (the donepezil/memantine -> Alzheimer's gap Atlas reported).
	dc.linkIndications(structID, sourceID, extras.indications[structID])
}

// linkIndications resolves indication disease names to ontology ids and adds
// drug -> mondo / drug -> efo edges.
func (dc *drugcentral) linkIndications(structID, sourceID string, indications []string) {
	if len(indications) == 0 || dc.d.lookupService == nil {
		return
	}
	mondoID := config.DataconfIDStringToInt["mondo"]
	efoID := config.DataconfIDStringToInt["efo"]
	mapTo := func(ontID uint32, ds string) {
		if ontID == 0 {
			return
		}
		seen := make(map[string]bool)
		for _, name := range indications {
			for _, dc2 := range diseaseNameVariants(name) {
				for o := range collectOntologyIDs(dc.d, dc.medical, dc2, ontID) {
					if !seen[o] {
						seen[o] = true
						dc.d.addXref(structID, sourceID, o, ds, false)
					}
				}
			}
		}
	}
	mapTo(mondoID, "mondo")
	mapTo(efoID, "efo")
}

// linkStructure resolves a drug's InChIKey through the lookup DB to its
// chembl_molecule / pubchem nodes and adds real xref edges, so a DrugCentral drug
// is reachable via map chains (chembl_molecule >> drugcentral, drugcentral >>
// pubchem, ...) — not only co-findable through a shared text keyword. ChEMBL
// indexes molecule InChIKeys (and pubchem its synonyms), so the lookup returns
// the matching compound nodes; we keep only chembl_molecule / pubchem targets.
func (dc *drugcentral) linkStructure(structID, inchiKey, sourceID string) {
	if dc.d.lookupService == nil {
		return
	}
	result, err := dc.d.lookup(inchiKey)
	if err != nil || result == nil {
		return
	}
	chemblID := config.DataconfIDStringToInt["chembl_molecule"]
	pubchemID := config.DataconfIDStringToInt["pubchem"]
	seen := make(map[string]bool)
	add := func(ds uint32, ident string) {
		var name string
		switch ds {
		case chemblID:
			name = "chembl_molecule"
		case pubchemID:
			name = "pubchem"
		default:
			return
		}
		key := name + "\t" + ident
		if seen[key] {
			return
		}
		seen[key] = true
		dc.d.addXref(structID, sourceID, ident, name, false)
	}
	for _, x := range result.Results {
		if x.IsLink {
			for _, e := range x.Entries {
				add(e.Dataset, e.Identifier)
			}
		} else {
			add(x.Dataset, x.Identifier)
		}
	}
}

// sortedKeys returns the keys of a string-set in deterministic order.
func sortedSetKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// splitTSVUnquote splits a tab-separated line and strips a single layer of
// surrounding double quotes from each field (DrugCentral quotes every column).
func splitTSVUnquote(line string) []string {
	fields := strings.Split(line, "\t")
	for i, f := range fields {
		f = strings.TrimSpace(f)
		if len(f) >= 2 && strings.HasPrefix(f, "\"") && strings.HasSuffix(f, "\"") {
			f = f[1 : len(f)-1]
		}
		fields[i] = f
	}
	return fields
}
