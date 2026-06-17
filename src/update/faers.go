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
// Entity model (MASTER/CHILD):
//
//	faers           (MASTER) one record per DRUG, keyed FAERS_<sha1(drug)>. Holds
//	                the drug's overall adverse-event summary (total_reports,
//	                distinct_reactions, serious_reports) and is the SINGLE place the
//	                drug is resolved to chembl_molecule / pubchem.
//	faers_reaction  (CHILD)  one record per (drug, reaction) co-occurrence, keyed
//	                FAERS_RX_<sha1(drug|reaction)>. Holds the per-reaction
//	                report_count, PRR, serious_count and outcome, and is linked back
//	                to its parent faers master (most-reported reaction first).
//
// IMPORTANT caveat: within a single FAERS report the listed drugs and the listed
// reactions are NOT individually linked, so a (drug, reaction) edge is report-level
// CO-OCCURRENCE, not a curated causal association. We aggregate co-occurrences
// across reports into a report_count and compute a PRR (proportional reporting
// ratio) disproportionality signal. The reaction is the MedDRA Preferred Term
// STRING only - no MedDRA dictionary is imported (license-restricted).
//
// Drug-ID normalization to chembl_molecule / pubchem is best-effort, via runtime
// name lookup of the openFDA generic_name (resolveDrugTo). biobtree has no native
// UNII/RxNorm dataset, so the drug name is the practical bridge; edges are guarded
// to only-configured datasets and only emitted when a name resolves. Because the
// drug is now a single master record, this resolution happens ONCE per drug.
//
// NOTE: individual FAERS reports are NEVER stored - only the per-drug and
// per-(drug,reaction) AGGREGATES are materialized.
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
	// reactionTotals = total reports mentioning a reaction, pairs = (drug,reaction),
	// drugSerious = total serious reports mentioning a drug (master serious_reports).
	pairs := make(map[string]*pairAgg)
	drugTotals := make(map[string]int32)
	reactionTotals := make(map[string]int32)
	drugSerious := make(map[string]int32)
	var totalReports int64

	for i, url := range partitions {
		log.Printf("FAERS: [%d/%d] downloading partition %s", i+1, len(partitions), url)
		n := f.processPartition(url, pairs, drugTotals, reactionTotals, drugSerious)
		totalReports += n
		log.Printf("FAERS: [%d/%d] partition done (%d reports, %d unique pairs so far)", i+1, len(partitions), n, len(pairs))
	}

	log.Printf("FAERS: aggregation complete - %d reports, %d unique drugs, %d unique (drug,reaction) pairs",
		totalReports, len(drugTotals), len(pairs))

	f.saveEntries(pairs, drugTotals, reactionTotals, drugSerious, totalReports)

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
func (f *faers) processPartition(url string, pairs map[string]*pairAgg, drugTotals, reactionTotals, drugSerious map[string]int32) int64 {
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
			f.foldReport(&ff.Results[ri], pairs, drugTotals, reactionTotals, drugSerious)
			reports++
		}
	}
	return reports
}

// foldReport adds one report's (drug x reaction) co-occurrences to the aggregates.
func (f *faers) foldReport(r *faersReport, pairs map[string]*pairAgg, drugTotals, reactionTotals, drugSerious map[string]int32) {
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
		if serious {
			drugSerious[drug]++
		}
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

// drugAgg holds the per-drug master aggregate accumulated while iterating pairs.
// total_reports / serious_reports come straight from the report-level marginal maps
// (drugTotals / drugSerious); only distinct_reactions must be counted from the
// children that actually pass minReportCount.
type drugAgg struct {
	distinctReactions int32
}

// saveEntries materializes the aggregated pairs into the master/child records:
//
//	faers          - one MASTER record per drug, with chembl_molecule/pubchem edges
//	                 (resolved ONCE per drug) and a per-drug adverse-event summary.
//	faers_reaction - one CHILD record per (drug,reaction), linked back to its parent
//	                 master via addXrefWithSortLevels (sorted by report_count DESC so
//	                 the most-reported reactions survive the result cap).
//
// Only AGGREGATES are written; individual reports are never stored.
func (f *faers) saveEntries(pairs map[string]*pairAgg, drugTotals, reactionTotals, drugSerious map[string]int32, totalReports int64) {
	masterSource := f.source // "faers"
	masterSourceID := config.Dataconf[masterSource]["id"]

	childSource := "faers_reaction"
	childSourceID := config.Dataconf[childSource]["id"]
	hasChild := childSourceID != ""
	if !hasChild {
		log.Printf("FAERS: WARNING faers_reaction not configured - child records will be skipped")
	}

	var idLogFile, childIDLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, masterSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		if hasChild {
			childIDLogFile = openIDLogFile(config.TestRefDir, childSource+"_ids.txt")
			if childIDLogFile != nil {
				defer childIDLogFile.Close()
			}
		}
	}

	// minReportCount filters away singleton co-occurrence noise.
	minReportCount := int32(2)
	if v := config.Dataconf[masterSource]["minReportCount"]; v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			minReportCount = int32(n)
		}
	}

	// Resolve which drug-target datasets are actually configured (guarded edges).
	_, hasChembl := config.Dataconf["chembl_molecule"]
	_, hasPubchem := config.Dataconf["pubchem"]

	N := float64(totalReports)

	// Phase 1: emit CHILD records, accumulating the per-drug master aggregate from the
	// reactions that actually pass minReportCount (so master.distinct_reactions counts
	// exactly the children emitted, keeping master/child consistent).
	masters := make(map[string]*drugAgg)
	var childCount int

	for _, agg := range pairs {
		if agg.reportCount < minReportCount {
			continue
		}

		drugTotal := drugTotals[agg.drugName]
		rxTotal := reactionTotals[agg.reaction]
		prr := computePRR(agg.reportCount, drugTotal, rxTotal, N)

		masterID := faersMasterID(agg.drugName)
		childID := faersReactionID(agg.drugName, agg.reaction)

		// Accumulate the per-drug master summary.
		m, ok := masters[agg.drugName]
		if !ok {
			m = &drugAgg{}
			masters[agg.drugName] = m
		}
		m.distinctReactions++

		if hasChild {
			childAttr := &pbuf.FaersReactionAttr{
				Reaction:     agg.reaction,
				ReportCount:  agg.reportCount,
				Prr:          prr,
				SeriousCount: agg.seriousCount,
				Outcome:      topKey(agg.outcomes),
			}
			attrBytes, err := ffjson.Marshal(childAttr)
			if err != nil {
				log.Printf("FAERS: Error marshaling reaction %s: %v", childID, err)
				continue
			}
			f.d.addProp3(childID, childSourceID, attrBytes)

			// Link the child to its PARENT master, sorted by report_count DESC so the
			// most-reported reactions survive the per-query result cap. cellCount uses a
			// 12-digit inverted format (handles the full report_count range).
			sortLevels := []string{
				ComputeSortLevelValue(SortLevelCellCount, map[string]interface{}{"count": int64(agg.reportCount)}),
			}
			f.d.addXrefWithSortLevels(masterID, masterSourceID, childID, childSource, sortLevels)

			// Text search: the reaction term points at this child record.
			if len(agg.reaction) >= 3 && len(agg.reaction) < 200 {
				f.d.addXref(agg.reaction, textLinkID, childID, childSource, true)
			}

			if childIDLogFile != nil {
				childIDLogFile.WriteString(childID + "\n")
			}
		}

		childCount++
		if childCount%100000 == 0 {
			log.Printf("FAERS: saved %d reaction (child) records...", childCount)
		}
	}

	// Phase 2: emit MASTER records, one per drug, with the consolidated compound edges.
	resolvedDrug := make(map[string]bool) // drugs already master-emitted (defensive)
	var masterCount int

	for drugName, m := range masters {
		if resolvedDrug[drugName] {
			continue
		}
		resolvedDrug[drugName] = true

		masterID := faersMasterID(drugName)

		attr := &pbuf.FaersAttr{
			DrugName:          drugName,
			TotalReports:      drugTotals[drugName],
			DistinctReactions: m.distinctReactions,
			SeriousReports:    drugSerious[drugName],
		}
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("FAERS: Error marshaling master %s: %v", masterID, err)
			continue
		}
		f.d.addProp3(masterID, masterSourceID, attrBytes)

		// Text search: the drug name points at this master record.
		if len(drugName) >= 2 && len(drugName) < 200 {
			f.d.addXref(drugName, textLinkID, masterID, masterSource, true)
		}

		// Drug-ID normalization: resolve the drug name ONCE to chembl_molecule /
		// pubchem via runtime keyword lookup (best-effort), consolidating the
		// drug<->compound linkage onto the single master. Guarded to configured datasets.
		if hasChembl {
			for _, cid := range f.resolveDrugTo(drugName, "chembl_molecule") {
				f.d.addXref(masterID, masterSourceID, cid, "chembl_molecule", false)
			}
		}
		if hasPubchem {
			for _, pid := range f.resolveDrugTo(drugName, "pubchem") {
				f.d.addXref(masterID, masterSourceID, pid, "pubchem", false)
			}
		}

		if idLogFile != nil {
			idLogFile.WriteString(masterID + "\n")
		}

		masterCount++
		if masterCount%50000 == 0 {
			log.Printf("FAERS: saved %d drug (master) records...", masterCount)
		}
	}

	atomic.AddUint64(&f.d.totalParsedEntry, uint64(masterCount+childCount))
	log.Printf("FAERS: Total records saved: %d masters (drugs) + %d reactions (minReportCount=%d)",
		masterCount, childCount, minReportCount)
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

// faersMasterID builds a deterministic MASTER (per-drug) record id from the drug name.
func faersMasterID(drug string) string {
	h := sha1.Sum([]byte(drug))
	return "FAERS_" + hex.EncodeToString(h[:8]) // 16 hex chars - collision-safe at this scale
}

// faersReactionID builds a deterministic CHILD (per drug,reaction) record id.
func faersReactionID(drug, reaction string) string {
	h := sha1.Sum([]byte(drug + "\x00" + reaction))
	return "FAERS_RX_" + hex.EncodeToString(h[:8]) // 16 hex chars - collision-safe at this scale
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
// IDs so the caller can attach the same target to the drug's master record.
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
