package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type clingenVariant struct {
	source string
	d      *DataUpdate
}

func (c *clingenVariant) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// splitCodes splits a comma/semicolon-separated code/id list into trimmed tokens,
// normalizing multi-word ACMG strength modifiers to their canonical single-token form
// (e.g. "PM3_Very Strong" -> "PM3_VeryStrong"). NOTE: the upstream ClinGen erepo
// summary-CSV endpoint itself corrupts most Very-Strong rows before we see them,
// dropping the word "Strong" (so "PM3_Very Strong" arrives as "PM3_Very" and "PS4_Very
// Strong" as bare "PS4"). We cannot recover a word absent from the source; this only
// canonicalizes the rows that arrive intact and guards against a downstream whitespace
// re-split. The truncated rows are an upstream ClinGen data-quality bug (see KNOWN_ISSUES).
func splitCodes(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		tok = strings.ReplaceAll(tok, "_Very Strong", "_VeryStrong")
		tok = strings.ReplaceAll(tok, "_Stand Alone", "_Standalone")
		// Deterministic completion of partial truncations: the ClinGen CSV sometimes
		// drops only the second word, leaving "_Very" / "_Stand". No ACMG strength
		// other than "Very Strong" starts with "Very" (nor any but "Stand Alone" with
		// "Stand"), so these suffixes complete unambiguously without needing the summary.
		if strings.HasSuffix(tok, "_Very") {
			tok = strings.TrimSuffix(tok, "_Very") + "_VeryStrong"
		} else if strings.HasSuffix(tok, "_Stand") {
			tok = strings.TrimSuffix(tok, "_Stand") + "_Standalone"
		}
		out = append(out, tok)
	}
	return out
}

// codeBase returns an ACMG criterion code without any (possibly truncated) strength
// suffix: "PS4"->"PS4", "PM3_Very"->"PM3", "PP1_Strong"->"PP1".
func codeBase(code string) string {
	if i := strings.IndexByte(code, '_'); i >= 0 {
		return code[:i]
	}
	return code
}

// needsStrengthRecovery reports whether a structured ACMG code may have had its strength
// dropped by the upstream ClinGen CSV export: a bare code (no suffix) or one truncated to
// the "_Very" / "_Stand" artifact. Codes already carrying a full suffix are left alone.
func needsStrengthRecovery(code string) bool {
	if !strings.Contains(code, "_") {
		return true
	}
	return strings.HasSuffix(code, "_Very") || strings.HasSuffix(code, "_Stand")
}

// recoverStrengthFromSummary backfills a dropped ACMG strength from the interpretation
// summary, which (unlike the upstream CSV's Applied-Evidence-Codes column) preserves the
// full "<CODE>_Very Strong" form. ClinGen only truncates the two MULTI-WORD strengths
// ("Very Strong", "Stand Alone") — single-word strengths are never dropped — so recovery
// only ever restores VeryStrong / Standalone, and only on an unambiguous summary match
// (present as exactly one of the two). Otherwise the code is returned unchanged.
func recoverStrengthFromSummary(code, summary string) string {
	if summary == "" || !needsStrengthRecovery(code) {
		return code
	}
	base := codeBase(code)
	if base == "" {
		return code
	}
	vs := strings.Contains(summary, base+"_VeryStrong") || strings.Contains(summary, base+"_Very Strong")
	sa := strings.Contains(summary, base+"_Standalone") || strings.Contains(summary, base+"_Stand Alone")
	switch {
	case vs && !sa:
		return base + "_VeryStrong"
	case sa && !vs:
		return base + "_Standalone"
	}
	return code
}

// recoverStrengths applies recoverStrengthFromSummary across a code list.
func recoverStrengths(codes []string, summary string) []string {
	if len(codes) == 0 || summary == "" {
		return codes
	}
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = recoverStrengthFromSummary(c, summary)
	}
	return out
}

func (c *clingenVariant) update() {
	defer c.d.wg.Done()

	log.Println("ClinGen Variant: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[c.source]["id"]
	path := config.Dataconf[c.source]["path"]

	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("ClinGen Variant: [TEST MODE] Processing up to %d entries", testLimit)
	}

	br, cleanup, err := openClingenReader(c.source, path)
	c.check(err, "opening ClinGen variant data")
	defer cleanup()

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024 // summaries can be long
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	if !scanner.Scan() {
		c.check(scanner.Err(), "reading ClinGen variant header")
		c.d.progChan <- &progressInfo{dataset: c.source, done: true}
		return
	}
	header := strings.Split(scanner.Text(), "\t")
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}
	expectedColumns := len(colMap)

	var total uint64
	var entryCount int64
	var previous int64
	var skippedNoID int

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		// Guard against continuation lines from multiline summaries.
		if strings.Count(line, "\t") < expectedColumns-1 {
			continue
		}
		fields := strings.Split(line, "\t")
		col := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(fields) {
				return strings.TrimSpace(fields[idx])
			}
			return ""
		}

		caID := col("Allele Registry Id")
		uuid := col("Uuid")
		key := caID
		if key == "" {
			key = uuid // fall back to the Evidence Repo UUID
		}
		if key == "" {
			skippedNoID++
			continue
		}

		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: 0}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, key)
		}

		clinvarID := col("ClinVar Variation Id")
		geneSymbol := col("HGNC Gene Symbol")
		mondoID := col("Mondo Id")

		// The upstream ClinGen CSV drops multi-word ACMG strengths (Very Strong,
		// Stand Alone) from the evidence-code columns, but the interpretation summary
		// preserves them — recover the dropped strengths from it.
		summaryText := col("Summary of interpretation")

		attr := pbuf.ClingenVariantAttr{
			VariationName:       col("Variation"),
			ClinvarVariationId:  clinvarID,
			AlleleRegistryId:    caID,
			GeneSymbol:          geneSymbol,
			Disease:             col("Disease"),
			DiseaseMondoId:      mondoID,
			Moi:                 col("Mode of Inheritance"),
			Assertion:           col("Assertion"),
			EvidenceCodesMet:    recoverStrengths(splitCodes(col("Applied Evidence Codes (Met)")), summaryText),
			EvidenceCodesNotMet: recoverStrengths(splitCodes(col("Applied Evidence Codes (Not Met)")), summaryText),
			Summary:             summaryText,
			Vcep:                col("Expert Panel"),
			Guideline:           col("Guideline"),
			ApprovalDate:        col("Approval Date"),
			PublishedDate:       col("Published Date"),
			EvidenceRepoLink:    col("Evidence Repo Link"),
			Uuid:                uuid,
		}

		attrBytes, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("ClinGen Variant: Error marshaling %s: %v", key, err)
			continue
		}
		c.d.addProp3(key, sourceID, attrBytes)

		// Text search
		if geneSymbol != "" {
			c.d.addXref(geneSymbol, textLinkID, key, c.source, true)
		}
		if name := attr.VariationName; name != "" {
			c.d.addXref(name, textLinkID, key, c.source, true)
		}

		// Bridge to the ClinVar hub (inherits dbSNP/gene/disease links)
		if isNumericID(clinvarID) {
			c.d.addXref(key, sourceID, clinvarID, "clinvar", false)
		}

		// Gene cross-references via symbol lookup (HGNC/Entrez/Ensembl)
		if geneSymbol != "" {
			c.d.addHumanGeneXrefsAll(geneSymbol, key, sourceID)
		}

		// Disease cross-reference (MONDO)
		clingenDiseaseXref(c.d, key, sourceID, mondoID)

		// PubMed
		for _, pmid := range splitCodes(col("PubMed Articles")) {
			if isNumericID(pmid) {
				c.d.addXref(key, sourceID, pmid, "pubmed", false)
			}
		}

		total++
		entryCount++
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ClinGen Variant: Error reading data: %v", err)
	}

	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
	atomic.AddUint64(&c.d.totalParsedEntry, total)

	log.Printf("ClinGen Variant: Processing complete - %d entries saved (%.2fs)", total, time.Since(startTime).Seconds())
	if skippedNoID > 0 {
		log.Printf("ClinGen Variant: Skipped %d rows with no usable id", skippedNoID)
	}
}
