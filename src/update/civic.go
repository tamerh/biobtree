package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// civic ingests the CIViC (Clinical Interpretation of Variants in Cancer)
// knowledgebase. It is a parent dataset (genes/features) with three child
// datasets processed here: civic_variant, civic_evidence, civic_assertion.
//
// Molecular profiles are NOT stored as their own dataset; they are folded into
// an in-memory join map (mpID -> variantIDs) so evidence/assertion edges fan
// out to every gene/variant in the profile (including combination profiles).
type civic struct {
	source string
	d      *DataUpdate
}

func (c *civic) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// saltSuffixes are stripped from therapy names as a fallback when the full
// name does not resolve against chembl_molecule (e.g. "Imatinib Mesylate").
var saltSuffixes = []string{
	" Mesylate", " Bismesylate", " Dimesylate", " Hydrochloride", " Dihydrochloride",
	" Hydrobromide", " Sulfate", " Sulphate", " Phosphate", " Diphosphate", " Maleate",
	" Citrate", " Tartrate", " Bitartrate", " Acetate", " Succinate", " Fumarate",
	" Difumarate", " Sodium", " Disodium", " Calcium", " Potassium", " Anhydrous",
	" Trihydrate", " Dihydrate", " Monohydrate", " Hydrate", " Besylate", " Tosylate",
}

func stripSalt(name string) string {
	for _, s := range saltSuffixes {
		if strings.HasSuffix(name, s) {
			return strings.TrimSpace(strings.TrimSuffix(name, s))
		}
	}
	return name
}

// readTSV downloads a CIViC nightly TSV and returns a column->index map plus
// all data rows (the files are small enough to hold in memory).
func (c *civic) readTSV(fileName string) (map[string]int, [][]string) {
	base := config.Dataconf[c.source]["path"]
	url := base + fileName
	log.Printf("CIViC: downloading %s", url)
	resp, err := http.Get(url)
	c.check(err, "downloading "+fileName)
	defer resp.Body.Close()

	br := bufio.NewReaderSize(resp.Body, fileBufSize)
	scanner := bufio.NewScanner(br)
	const maxCapacity = 4 * 1024 * 1024 // evidence statements / descriptions can be long
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	if !scanner.Scan() {
		c.check(scanner.Err(), "reading header of "+fileName)
		return nil, nil
	}
	header := strings.Split(scanner.Text(), "\t")
	colMap := make(map[string]int, len(header))
	for i, name := range header {
		colMap[strings.Trim(strings.TrimSpace(name), "\"")] = i
	}

	var rows [][]string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}
	if err := scanner.Err(); err != nil {
		log.Printf("CIViC: error reading %s: %v", fileName, err)
	}
	return colMap, rows
}

// isCivicNull reports whether a field value is one of CIViC's null sentinels.
func isCivicNull(v string) bool {
	switch strings.ToUpper(v) {
	case "", "N/A", "NA", "NONE", "NONE FOUND", "NOT APPLICABLE", "UNKNOWN", ".":
		return true
	}
	return false
}

func getCol(row []string, col map[string]int, name string) string {
	if i, ok := col[name]; ok && i < len(row) {
		v := strings.Trim(strings.TrimSpace(row[i]), "\"")
		// CIViC uses sentinels like "N/A" / "NONE FOUND" for missing values;
		// treat them as empty so they never become a stored value or (worse)
		// an xref target into a bucketed dataset.
		if isCivicNull(v) {
			return ""
		}
		return v
	}
	return ""
}

// isAllDigits reports whether s is non-empty and entirely numeric.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// splitList splits a comma-separated CIViC list field, trimming blanks.
func splitList(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if !isCivicNull(p) {
			out = append(out, p)
		}
	}
	return out
}

// chemblMoleculeID resolves a therapy name to a chembl_molecule ID via the
// lookup database (requires --lookupdb). ChEMBL drug names are indexed as
// text-search keywords (not as chembl_molecule dataset keys), so we use the
// general keyword lookup and filter the results to the chembl_molecule dataset.
// Tries the raw name then a salt-stripped form (e.g. "Imatinib Mesylate").
func (c *civic) chemblMoleculeID(name string) string {
	if c.d.lookupService == nil || name == "" {
		return ""
	}
	cid, ok := config.Dataconf["chembl_molecule"]["id"]
	if !ok {
		return ""
	}
	var di uint32
	fmt.Sscanf(cid, "%d", &di)

	seen := map[string]bool{}
	for _, cand := range []string{name, stripSalt(name)} {
		if cand == "" || seen[cand] {
			continue
		}
		seen[cand] = true
		result, err := c.d.lookup(cand)
		if err != nil || result == nil {
			continue
		}
		for _, xref := range result.Results {
			// Direct hit: an entity keyed by this name.
			if xref.Dataset == di && xref.Identifier != "" {
				return xref.Identifier
			}
			// Keyword/text-search hit: the drug name is a link entry whose
			// chembl_molecule target sits in its linked Entries.
			for _, e := range xref.Entries {
				if e.Dataset == di && e.Identifier != "" {
					return e.Identifier
				}
			}
		}
	}
	return ""
}

func (c *civic) update() {
	defer c.d.wg.Done()
	log.Println("CIViC: starting data processing...")
	startTime := time.Now()

	geneSourceID := config.Dataconf["civic"]["id"]
	variantSourceID := config.Dataconf["civic_variant"]["id"]
	evidenceSourceID := config.Dataconf["civic_evidence"]["id"]
	assertionSourceID := config.Dataconf["civic_assertion"]["id"]

	// ---- 1. Variants: build join maps + store variant entries -------------
	vGene := map[string]string{}    // variant_id -> gene symbol
	vEntrez := map[string]string{}  // variant_id -> entrez id
	vFeature := map[string]string{} // variant_id -> feature_id (parent gene)
	variantsDone := c.processVariants(variantSourceID, geneSourceID, vGene, vEntrez, vFeature)

	// ---- 2. Molecular profiles: mpID -> []variant_id (in-memory only) -----
	mpVariants := c.processMolecularProfiles()

	// ---- 3. Features (genes): store gene entries + gene-hub xrefs ---------
	genesDone := c.processFeatures(geneSourceID)

	// ---- 4. Evidence items ------------------------------------------------
	evidenceDone := c.processEvidence(evidenceSourceID, mpVariants, vGene, vFeature)

	// ---- 5. Assertions ----------------------------------------------------
	assertionsDone := c.processAssertions(assertionSourceID, mpVariants, vGene, vFeature)

	total := genesDone + variantsDone + evidenceDone + assertionsDone
	atomic.AddUint64(&c.d.totalParsedEntry, total)

	// Signal completion for the parent and every child dataset.
	c.d.progChan <- &progressInfo{dataset: "civic", done: true}
	c.d.progChan <- &progressInfo{dataset: "civic_variant", done: true}
	c.d.progChan <- &progressInfo{dataset: "civic_evidence", done: true}
	c.d.progChan <- &progressInfo{dataset: "civic_assertion", done: true}

	log.Printf("CIViC: complete - %d genes, %d variants, %d evidence, %d assertions (%.2fs)",
		genesDone, variantsDone, evidenceDone, assertionsDone, time.Since(startTime).Seconds())
}

func (c *civic) processVariants(sourceID, geneSourceID string, vGene, vEntrez, vFeature map[string]string) uint64 {
	col, rows := c.readTSV(config.Dataconf[c.source]["variantsFile"])
	var n uint64
	for _, row := range rows {
		vid := getCol(row, col, "variant_id")
		if !isAllDigits(vid) {
			continue
		}
		gene := getCol(row, col, "gene")
		entrez := getCol(row, col, "entrez_id")
		feature := getCol(row, col, "feature_id")

		vGene[vid] = gene
		vEntrez[vid] = entrez
		vFeature[vid] = feature

		attr := pbuf.CivicVariantAttr{
			Name:                     getCol(row, col, "variant"),
			Gene:                     gene,
			VariantTypes:             splitList(getCol(row, col, "variant_types")),
			Aliases:                  splitList(getCol(row, col, "variant_aliases")),
			HgvsDescriptions:         getCol(row, col, "hgvs_descriptions"),
			ClinvarIds:               getCol(row, col, "clinvar_ids"),
			AlleleRegistryId:         getCol(row, col, "allele_registry_id"),
			RepresentativeTranscript: getCol(row, col, "representative_transcript"),
			ReferenceBuild:           getCol(row, col, "reference_build"),
			NcitId:                   getCol(row, col, "ncit_id"),
		}
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			continue
		}
		c.d.addProp3(vid, sourceID, b)

		// Text search: variant name + aliases
		if attr.Name != "" {
			c.d.addXref(attr.Name, textLinkID, vid, "civic_variant", true)
		}
		for _, a := range attr.Aliases {
			c.d.addXref(a, textLinkID, vid, "civic_variant", true)
		}

		// Link to parent gene (civic)
		if isAllDigits(feature) {
			c.d.addXref(vid, sourceID, feature, "civic", false)
		}

		// ClinVar variation IDs (comma-separated, numeric only)
		for _, cv := range splitList(attr.ClinvarIds) {
			if isAllDigits(cv) {
				c.d.addXref(vid, sourceID, cv, "clinvar", false)
			}
		}
		n++
	}
	return n
}

func (c *civic) processMolecularProfiles() map[string][]string {
	col, rows := c.readTSV(config.Dataconf[c.source]["molecularProfilesFile"])
	mp := make(map[string][]string, len(rows))
	for _, row := range rows {
		id := getCol(row, col, "molecular_profile_id")
		if id == "" {
			continue
		}
		mp[id] = splitList(getCol(row, col, "variant_ids"))
	}
	return mp
}

func (c *civic) processFeatures(sourceID string) uint64 {
	col, rows := c.readTSV(config.Dataconf[c.source]["featuresFile"])
	var n uint64
	for _, row := range rows {
		fid := getCol(row, col, "feature_id")
		if !isAllDigits(fid) {
			continue
		}
		name := getCol(row, col, "name")
		entrez := getCol(row, col, "entrez_id")

		attr := pbuf.CivicGeneAttr{
			Name:        name,
			FeatureType: getCol(row, col, "feature_type"),
			Description: getCol(row, col, "description"),
			EntrezId:    entrez,
			NcitId:      getCol(row, col, "ncit_id"),
			Aliases:     splitList(getCol(row, col, "feature_aliases")),
		}
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			continue
		}
		c.d.addProp3(fid, sourceID, b)

		// Text search: gene symbol + aliases
		if name != "" {
			c.d.addXref(name, textLinkID, fid, "civic", true)
		}
		for _, a := range attr.Aliases {
			c.d.addXref(a, textLinkID, fid, "civic", true)
		}

		// Gene hub. Prefer the authoritative Entrez ID (exact) for entrez +
		// ensembl, and resolve HGNC via the (HGNC-approved) symbol.
		if isAllDigits(entrez) {
			c.d.addXref(fid, sourceID, entrez, "entrez", false)
			c.d.addXrefEnsemblViaEntrez(entrez, fid, sourceID)
			c.d.addHumanGeneXrefsViaHGNC(name, fid, sourceID)
		} else if name != "" {
			// Fusions / features without an Entrez ID: best-effort by symbol.
			c.d.addHumanGeneXrefsAll(name, fid, sourceID)
		}
		n++
	}
	return n
}

// genesForProfile resolves a molecular_profile_id to the distinct feature IDs,
// gene symbols, and variant IDs of its constituent variants (covers
// combination profiles, which reference more than one variant/gene).
func genesForProfile(mpID string, mpVariants map[string][]string, vGene, vFeature map[string]string) (featureIDs, geneSymbols, variantIDs []string) {
	seenF := map[string]bool{}
	seenG := map[string]bool{}
	for _, vid := range mpVariants[mpID] {
		variantIDs = append(variantIDs, vid)
		if f := vFeature[vid]; f != "" && !seenF[f] {
			seenF[f] = true
			featureIDs = append(featureIDs, f)
		}
		if g := vGene[vid]; g != "" && !seenG[g] {
			seenG[g] = true
			geneSymbols = append(geneSymbols, g)
		}
	}
	return featureIDs, geneSymbols, variantIDs
}

func (c *civic) processEvidence(sourceID string, mpVariants map[string][]string, vGene, vFeature map[string]string) uint64 {
	col, rows := c.readTSV(config.Dataconf[c.source]["evidenceFile"])
	var n uint64
	for _, row := range rows {
		eid := getCol(row, col, "evidence_id")
		if !isAllDigits(eid) {
			continue
		}
		mpID := getCol(row, col, "molecular_profile_id")
		doid := getCol(row, col, "doid")
		therapies := splitList(getCol(row, col, "therapies"))

		attr := pbuf.CivicEvidenceAttr{
			MolecularProfile:       getCol(row, col, "molecular_profile"),
			Disease:                getCol(row, col, "disease"),
			Doid:                   doid,
			Therapies:              therapies,
			TherapyInteractionType: getCol(row, col, "therapy_interaction_type"),
			EvidenceType:           getCol(row, col, "evidence_type"),
			EvidenceDirection:      getCol(row, col, "evidence_direction"),
			EvidenceLevel:          getCol(row, col, "evidence_level"),
			Significance:           getCol(row, col, "significance"),
			EvidenceStatement:      getCol(row, col, "evidence_statement"),
			Phenotypes:             splitList(getCol(row, col, "phenotypes")),
			CitationId:             getCol(row, col, "citation_id"),
			Rating:                 getCol(row, col, "rating"),
			EvidenceStatus:         getCol(row, col, "evidence_status"),
			VariantOrigin:          getCol(row, col, "variant_origin"),
		}
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			continue
		}
		c.d.addProp3(eid, sourceID, b)

		c.emitClinicalEdges(eid, sourceID, "civic_evidence", mpID, doid, attr.Disease,
			attr.MolecularProfile, therapies, mpVariants, vGene, vFeature)

		// Literature: PMID only when the source is PubMed.
		if getCol(row, col, "source_type") == "PubMed" {
			if cid := attr.CitationId; cid != "" && cid[0] >= '0' && cid[0] <= '9' {
				c.d.addXref(eid, sourceID, cid, "pubmed", false)
			}
		}
		// Clinical trials (NCT identifiers only)
		for _, nct := range splitList(getCol(row, col, "nct_ids")) {
			if strings.HasPrefix(nct, "NCT") {
				c.d.addXref(eid, sourceID, nct, "clinical_trials", false)
			}
		}
		n++
	}
	return n
}

func (c *civic) processAssertions(sourceID string, mpVariants map[string][]string, vGene, vFeature map[string]string) uint64 {
	col, rows := c.readTSV(config.Dataconf[c.source]["assertionsFile"])
	var n uint64
	for _, row := range rows {
		aid := getCol(row, col, "assertion_id")
		if !isAllDigits(aid) {
			continue
		}
		mpID := getCol(row, col, "molecular_profile_id")
		doid := getCol(row, col, "doid")
		therapies := splitList(getCol(row, col, "therapies"))

		attr := pbuf.CivicAssertionAttr{
			MolecularProfile:   getCol(row, col, "molecular_profile"),
			Disease:            getCol(row, col, "disease"),
			Doid:               doid,
			Therapies:          therapies,
			AssertionType:      getCol(row, col, "assertion_type"),
			AssertionDirection: getCol(row, col, "assertion_direction"),
			Significance:       getCol(row, col, "significance"),
			AmpCategory:        getCol(row, col, "amp_category"),
			AcmgCodes:          splitList(getCol(row, col, "acmg_codes")),
			NccnGuideline:      getCol(row, col, "nccn_guideline"),
			RegulatoryApproval: getCol(row, col, "regulatory_approval"),
			FdaCompanionTest:   getCol(row, col, "fda_companion_test"),
			AssertionSummary:   getCol(row, col, "assertion_summary"),
			EvidenceStatus:     getCol(row, col, "status"),
		}
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			continue
		}
		c.d.addProp3(aid, sourceID, b)

		c.emitClinicalEdges(aid, sourceID, "civic_assertion", mpID, doid, attr.Disease,
			attr.MolecularProfile, therapies, mpVariants, vGene, vFeature)
		n++
	}
	return n
}

// emitClinicalEdges creates the shared edges for an evidence item or assertion:
// fan-out to constituent variants/genes, disease (DOID), therapies (ChEMBL),
// plus disease/therapy/profile text search.
func (c *civic) emitClinicalEdges(id, sourceID, dataset, mpID, doid, disease, profile string,
	therapies []string, mpVariants map[string][]string, vGene, vFeature map[string]string) {

	featureIDs, geneSymbols, variantIDs := genesForProfile(mpID, mpVariants, vGene, vFeature)
	for _, vid := range variantIDs {
		c.d.addXref(id, sourceID, vid, "civic_variant", false)
	}
	for _, fid := range featureIDs {
		c.d.addXref(id, sourceID, fid, "civic", false)
	}
	// Direct gene-hub edges so disease -> evidence -> hgnc resolves in one hop.
	for _, g := range geneSymbols {
		c.d.addHumanGeneXrefsViaHGNC(g, id, sourceID)
	}

	// Disease -> DOID (bridged to MONDO via mondo.obo DOID xrefs)
	if isAllDigits(doid) {
		c.d.addXref(id, sourceID, "DOID:"+doid, "doid", false)
	}

	// Therapies -> chembl_molecule (best-effort name match) + always text.
	for _, t := range therapies {
		if cid := c.chemblMoleculeID(t); cid != "" {
			c.d.addXref(id, sourceID, cid, "chembl_molecule", false)
		}
		c.d.addXref(t, textLinkID, id, dataset, true)
	}

	// Disease + molecular profile name text search
	if disease != "" {
		c.d.addXref(disease, textLinkID, id, dataset, true)
	}
	if profile != "" {
		c.d.addXref(profile, textLinkID, id, dataset, true)
	}
}
