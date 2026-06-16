package update

import (
	"archive/zip"
	"biobtree/pbuf"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// faers handles parsing the FDA Adverse Event Reporting System (FAERS) data
// exposed through openFDA's drug/event bulk download.
//
// Source:  https://download.open.fda.gov/drug/event/   (zipped JSON, partitioned
//
//	per calendar quarter; ~1700 partitions / ~20M reports total)
//
// Manifest: https://api.fda.gov/download.json  (lists every partition URL)
// License: CC0 (https://open.fda.gov/license/)
//
// Entity model (mirrors ctd_disease_association's chemical->disease record):
//
//	one entry per (drug, adverse-reaction) AGGREGATE, keyed FAERS_<sha1(drug|reaction)>.
//
// IMPORTANT caveat: within a single FAERS report the listed drugs and the listed
// reactions are NOT individually linked, so a (drug, reaction) edge is report-level
// CO-OCCURRENCE, not a curated causal association. We aggregate co-occurrences
// across reports into a report_count and compute a PRR (proportional reporting
// ratio) disproportionality signal. The reaction is the MedDRA Preferred Term
// STRING only - no MedDRA dictionary is imported (license-restricted).
//
// Drug-ID normalization to chembl_molecule / pubchem is best-effort, via runtime
// name lookup of the openFDA generic_name (addXrefViaKeyword). biobtree has no
// native UNII/RxNorm dataset, so the drug name is the practical bridge; edges are
// guarded to only-configured datasets and only emitted when a name resolves.
type faers struct {
	source string
	d      *DataUpdate
}

func (f *faers) check(err error, operation string) {
	checkWithContext(err, f.source, operation)
}

// faersReport is the subset of an openFDA drug/event report we parse.
type faersReport struct {
	Serious string `json:"serious"`
	Patient struct {
		Drug []struct {
			MedicinalProduct string `json:"medicinalproduct"`
			OpenFDA          struct {
				RxCUI         []string `json:"rxcui"`
				UNII          []string `json:"unii"`
				GenericName   []string `json:"generic_name"`
				BrandName     []string `json:"brand_name"`
				SubstanceName []string `json:"substance_name"`
			} `json:"openfda"`
		} `json:"drug"`
		Reaction []struct {
			ReactionMeddraPt string `json:"reactionmeddrapt"`
			ReactionOutcome  string `json:"reactionoutcome"`
		} `json:"reaction"`
	} `json:"patient"`
}

type faersFile struct {
	Results []faersReport `json:"results"`
}

// pairAgg accumulates statistics for one (drug, reaction) co-occurrence pair.
type pairAgg struct {
	drugName     string
	reaction     string
	reportCount  int32
	seriousCount int32
	outcomes     map[string]int32 // reactionoutcome code -> count
}

func (f *faers) update() {
	defer f.d.wg.Done()

	log.Println("FAERS: Starting openFDA drug/event processing...")
	startTime := time.Now()

	if config.IsTestMode() {
		log.Printf("FAERS: [TEST MODE] capped partition sampling enabled")
	}

	// Resolve the partition URLs from the openFDA download manifest. In test mode
	// (or whenever testPartitions is set) we cap to the first N partitions so the
	// focused build only pulls a few hundred MB instead of the multi-GB corpus.
	partitions, err := f.resolvePartitions()
	if err != nil {
		log.Panicf("FAERS: FATAL: cannot resolve partition list: %v", err)
	}
	if len(partitions) == 0 {
		log.Panicf("FAERS: FATAL: no partitions to process")
	}
	log.Printf("FAERS: processing %d partition file(s)", len(partitions))

	// Aggregation maps. drugTotals = total reports mentioning a drug (PRR denom),
	// reactionTotals = total reports mentioning a reaction, pairs = (drug,reaction).
	pairs := make(map[string]*pairAgg)
	drugTotals := make(map[string]int32)
	reactionTotals := make(map[string]int32)
	var totalReports int64

	for i, url := range partitions {
		log.Printf("FAERS: [%d/%d] downloading partition %s", i+1, len(partitions), url)
		n := f.processPartition(url, pairs, drugTotals, reactionTotals)
		totalReports += n
		log.Printf("FAERS: [%d/%d] partition done (%d reports, %d unique pairs so far)", i+1, len(partitions), n, len(pairs))
	}

	log.Printf("FAERS: aggregation complete - %d reports, %d unique drugs, %d unique (drug,reaction) pairs",
		totalReports, len(drugTotals), len(pairs))

	f.saveEntries(pairs, drugTotals, reactionTotals, totalReports)

	log.Printf("FAERS: Processing complete (%.2fs)", time.Since(startTime).Seconds())
	f.d.progChan <- &progressInfo{dataset: f.source, done: true}
}

// resolvePartitions fetches the openFDA download manifest and returns the drug/event
// partition URLs. Honors the testPartitions cap (and, in test mode, defaults to a
// small sample) so the focused build never pulls the full corpus.
func (f *faers) resolvePartitions() ([]string, error) {
	manifestURL := config.Dataconf[f.source]["manifestUrl"]
	if manifestURL == "" {
		manifestURL = "https://api.fda.gov/download.json"
	}

	resp, err := httpGetWithRetry(manifestURL, 3)
	if err != nil {
		return nil, fmt.Errorf("GET manifest %s: %v", manifestURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %v", err)
	}

	// Minimal struct - we only need results.drug.event.partitions[].file
	var manifest struct {
		Results struct {
			Drug struct {
				Event struct {
					Partitions []struct {
						File string `json:"file"`
					} `json:"partitions"`
				} `json:"event"`
			} `json:"drug"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %v", err)
	}

	parts := manifest.Results.Drug.Event.Partitions
	if len(parts) == 0 {
		return nil, fmt.Errorf("manifest has no drug/event partitions")
	}

	urls := make([]string, 0, len(parts))
	for _, p := range parts {
		if p.File != "" {
			urls = append(urls, p.File)
		}
	}

	// Most-recent-first so a small sample reflects current data. Quarter is encoded
	// in the URL (.../event/YYYYqN/drug-event-NNNN-of-MMMM.json.zip).
	sort.Slice(urls, func(i, j int) bool { return partitionRank(urls[i]) > partitionRank(urls[j]) })

	// Cap. testPartitions takes precedence; otherwise in test mode default to 2.
	cap := 0
	if v := config.Dataconf[f.source]["testPartitions"]; v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			cap = n
		}
	}
	if cap == 0 && config.IsTestMode() {
		cap = 2
	}
	if cap > 0 && cap < len(urls) {
		log.Printf("FAERS: capping to first %d of %d partitions (testPartitions/test mode)", cap, len(urls))
		urls = urls[:cap]
	}
	return urls, nil
}

var partitionQuarterRe = regexp.MustCompile(`/event/(\d{4})q(\d)/`)

// partitionRank yields a sortable integer (year*10+quarter) from a partition URL.
func partitionRank(url string) int {
	m := partitionQuarterRe.FindStringSubmatch(url)
	if len(m) != 3 {
		return 0
	}
	y, _ := strconv.Atoi(m[1])
	q, _ := strconv.Atoi(m[2])
	return y*10 + q
}

// processPartition downloads one zipped-JSON partition and folds its reports into
// the aggregation maps. Returns the number of reports processed.
func (f *faers) processPartition(url string, pairs map[string]*pairAgg, drugTotals, reactionTotals map[string]int32) int64 {
	resp, err := httpGetWithRetry(url, 3)
	if err != nil {
		log.Printf("FAERS: WARNING skipping partition %s: %v", url, err)
		return 0
	}
	defer resp.Body.Close()

	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("FAERS: WARNING reading partition %s: %v", url, err)
		return 0
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		log.Printf("FAERS: WARNING bad zip %s: %v", url, err)
		return 0
	}

	var reports int64
	for _, zf := range zr.File {
		if !strings.HasSuffix(zf.Name, ".json") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			log.Printf("FAERS: WARNING opening %s in zip: %v", zf.Name, err)
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Printf("FAERS: WARNING reading %s: %v", zf.Name, err)
			continue
		}

		var ff faersFile
		if err := json.Unmarshal(data, &ff); err != nil {
			log.Printf("FAERS: WARNING parsing %s: %v", zf.Name, err)
			continue
		}

		for ri := range ff.Results {
			f.foldReport(&ff.Results[ri], pairs, drugTotals, reactionTotals)
			reports++
		}
	}
	return reports
}

// foldReport adds one report's (drug x reaction) co-occurrences to the aggregates.
func (f *faers) foldReport(r *faersReport, pairs map[string]*pairAgg, drugTotals, reactionTotals map[string]int32) {
	serious := r.Serious == "1"

	// Distinct, normalized drug names for this report.
	drugSet := make(map[string]bool)
	for di := range r.Patient.Drug {
		d := &r.Patient.Drug[di]
		name := normalizeFaersDrug(d.OpenFDA.GenericName, d.OpenFDA.SubstanceName, d.OpenFDA.BrandName, d.MedicinalProduct)
		if name != "" {
			drugSet[name] = true
		}
	}
	if len(drugSet) == 0 {
		return
	}

	// Distinct reactions (MedDRA PT string) + their outcome codes for this report.
	type rxInfo struct{ outcome string }
	rxSet := make(map[string]rxInfo)
	for _, rx := range r.Patient.Reaction {
		pt := strings.TrimSpace(rx.ReactionMeddraPt)
		if pt == "" {
			continue
		}
		ptKey := strings.ToLower(pt)
		// keep canonical display casing of first seen, store outcome
		if _, ok := rxSet[ptKey]; !ok {
			rxSet[ptKey] = rxInfo{outcome: rx.ReactionOutcome}
		}
	}
	if len(rxSet) == 0 {
		return
	}

	// Per-report marginal totals (count each drug / reaction once per report).
	for drug := range drugSet {
		drugTotals[drug]++
	}
	for rxKey := range rxSet {
		reactionTotals[rxKey]++
	}

	// Cross product = report-level co-occurrence edges.
	for drug := range drugSet {
		for rxKey, info := range rxSet {
			pairKey := drug + "\x00" + rxKey
			agg, ok := pairs[pairKey]
			if !ok {
				agg = &pairAgg{
					drugName: drug,
					reaction: rxKey,
					outcomes: make(map[string]int32),
				}
				pairs[pairKey] = agg
			}
			agg.reportCount++
			if serious {
				agg.seriousCount++
			}
			if info.outcome != "" {
				agg.outcomes[info.outcome]++
			}
		}
	}
}

// normalizeFaersDrug picks the best normalized drug key for a report's drug entry,
// preferring the openFDA generic_name (resolves to chembl/pubchem), then substance,
// brand, then the verbatim medicinalproduct. Returns lowercased trimmed name.
func normalizeFaersDrug(generic, substance, brand []string, medicinalProduct string) string {
	pick := func(arr []string) string {
		for _, s := range arr {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
		return ""
	}
	name := pick(generic)
	if name == "" {
		name = pick(substance)
	}
	if name == "" {
		name = pick(brand)
	}
	if name == "" {
		name = strings.TrimSpace(medicinalProduct)
	}
	name = strings.ToLower(name)
	// Guard against absurd / multi-ingredient blobs blowing up the key space.
	if len(name) < 2 || len(name) > 120 {
		return ""
	}
	return name
}

// saveEntries materializes the aggregated pairs into FAERS records, computing PRR,
// adding cross-references to chembl_molecule/pubchem (best-effort, name-resolved)
// and text-indexing the drug name + reaction term.
func (f *faers) saveEntries(pairs map[string]*pairAgg, drugTotals, reactionTotals map[string]int32, totalReports int64) {
	sourceID := config.Dataconf[f.source]["id"]

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, f.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// minReportCount filters away singleton co-occurrence noise.
	minReportCount := int32(2)
	if v := config.Dataconf[f.source]["minReportCount"]; v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			minReportCount = int32(n)
		}
	}

	// Resolve which drug-target datasets are actually configured (guarded edges).
	_, hasChembl := config.Dataconf["chembl_molecule"]
	_, hasPubchem := config.Dataconf["pubchem"]

	// Cache per-unique-drug the chembl/pubchem identifiers a name resolved to, so we
	// run ONE runtime lookup per drug instead of once per (drug,reaction) pair, then
	// attach the cached targets to every record for that drug.
	type drugTargets struct {
		chembl  []string
		pubchem []string
	}
	resolvedDrug := make(map[string]*drugTargets)

	var savedCount int
	N := float64(totalReports)

	for _, agg := range pairs {
		if agg.reportCount < minReportCount {
			continue
		}

		drugTotal := drugTotals[agg.drugName]
		rxTotal := reactionTotals[agg.reaction]

		prr := computePRR(agg.reportCount, drugTotal, rxTotal, N)

		// Deterministic id from drug+reaction (kept short; key is uppercased on store).
		id := faersID(agg.drugName, agg.reaction)

		topOutcome := topKey(agg.outcomes)

		attr := &pbuf.FaersAttr{
			DrugName:        agg.drugName,
			Reaction:        agg.reaction,
			ReportCount:     agg.reportCount,
			Prr:             prr,
			SeriousCount:    agg.seriousCount,
			TopOutcome:      topOutcome,
			DrugReportTotal: drugTotal,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("FAERS: Error marshaling %s: %v", id, err)
			continue
		}
		f.d.addProp3(id, sourceID, attrBytes)

		// Sort edges so higher-PRR records surface first.
		sortLevels := []string{
			ComputeSortLevelValue(SortLevelInteractionScore, map[string]interface{}{"score": int(prr * 1000)}),
		}

		// Text search: drug name + reaction term, both pointing at this record.
		if len(agg.drugName) >= 2 && len(agg.drugName) < 200 {
			f.d.addXref(agg.drugName, textLinkID, id, f.source, true)
		}
		if len(agg.reaction) >= 3 && len(agg.reaction) < 200 {
			f.d.addXref(agg.reaction, textLinkID, id, f.source, true)
		}

		// Drug-ID normalization: resolve the drug name to chembl_molecule / pubchem
		// identifiers via runtime keyword lookup (best-effort), ONCE per unique drug,
		// then attach the cached targets to this record. Guarded to configured datasets.
		dt, seen := resolvedDrug[agg.drugName]
		if !seen {
			dt = &drugTargets{}
			if hasChembl {
				dt.chembl = f.resolveDrugTo(agg.drugName, "chembl_molecule")
			}
			if hasPubchem {
				dt.pubchem = f.resolveDrugTo(agg.drugName, "pubchem")
			}
			resolvedDrug[agg.drugName] = dt
		}
		for _, cid := range dt.chembl {
			f.d.addXrefWithSortLevels(id, sourceID, cid, "chembl_molecule", sortLevels)
		}
		for _, pid := range dt.pubchem {
			f.d.addXrefWithSortLevels(id, sourceID, pid, "pubchem", sortLevels)
		}

		if idLogFile != nil {
			idLogFile.WriteString(id + "\n")
		}

		savedCount++
		if savedCount%100000 == 0 {
			log.Printf("FAERS: saved %d records...", savedCount)
		}
	}

	atomic.AddUint64(&f.d.totalParsedEntry, uint64(savedCount))
	log.Printf("FAERS: Total records saved: %d (minReportCount=%d)", savedCount, minReportCount)
}

// computePRR returns the proportional reporting ratio for a (drug, reaction) pair.
//
//	          a / (a + b)
//	PRR = -------------------
//	          c / (c + d)
//
// a = reports with drug AND reaction        (pairCount)
// a+b = reports with drug                    (drugTotal)
// c = reports with reaction but not drug     (rxTotal - a)
// c+d = reports without drug                 (N - drugTotal)
//
// Returns 0 when undefined (insufficient denominator).
func computePRR(pairCount, drugTotal, rxTotal int32, N float64) float64 {
	a := float64(pairCount)
	aPlusB := float64(drugTotal)
	c := float64(rxTotal) - a
	cPlusD := N - float64(drugTotal)
	if aPlusB <= 0 || cPlusD <= 0 {
		return 0
	}
	target := a / aPlusB
	background := c / cPlusD
	if background <= 0 {
		return 0
	}
	prr := target / background
	if math.IsInf(prr, 0) || math.IsNaN(prr) {
		return 0
	}
	return prr
}

// faersID builds a deterministic record id from the drug name and reaction.
func faersID(drug, reaction string) string {
	h := sha1.Sum([]byte(drug + "\x00" + reaction))
	return "FAERS_" + hex.EncodeToString(h[:8]) // 16 hex chars - collision-safe at this scale
}

// topKey returns the map key with the highest count (deterministic tie-break by key).
func topKey(m map[string]int32) string {
	best := ""
	var bestN int32 = -1
	for k, n := range m {
		if n > bestN || (n == bestN && k < best) {
			best = k
			bestN = n
		}
	}
	return best
}

// resolveDrugTo looks the drug name up in the prior-build index and returns the
// identifiers it resolves to within targetDataset (e.g. chembl_molecule / pubchem).
// Best-effort: returns nil when the lookup service is unavailable or no match. This
// mirrors the dataset filtering inside addXrefViaKeyword but captures the resolved
// IDs so the caller can attach the same target to every record for that drug.
func (f *faers) resolveDrugTo(drugName, targetDataset string) []string {
	if f.d.lookupService == nil {
		return nil
	}
	idStr, ok := config.Dataconf[targetDataset]["id"]
	if !ok {
		return nil
	}
	var filterID uint32
	fmt.Sscanf(idStr, "%d", &filterID)

	result, err := f.d.lookup(drugName)
	if err != nil || result == nil || len(result.Results) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var ids []string
	const maxTargets = 10 // bound fan-out for ambiguous drug names
	for _, r := range result.Results {
		if !r.IsLink {
			continue
		}
		for _, e := range r.Entries {
			if e.Dataset != filterID || seen[e.Identifier] {
				continue
			}
			seen[e.Identifier] = true
			ids = append(ids, e.Identifier)
			if len(ids) >= maxTargets {
				return ids
			}
		}
	}
	return ids
}
