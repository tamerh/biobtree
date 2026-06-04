package update

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MedicalTermMappings holds all medical term normalization mappings
// Loaded from conf/medical_term_mappings.json
type MedicalTermMappings struct {
	SpecificPatterns    map[string]string  `json:"specific_patterns"`
	AnatomicalTerms     map[string]string  `json:"anatomical_terms"`
	QualifiersRemove    QualifiersToRemove `json:"qualifiers_to_remove"`
	DiseaseCorrections  map[string]string  `json:"disease_corrections"`
	SpellingVariations  map[string]string  `json:"spelling_variations"`
	CancerQualifiers    CancerQualifiers   `json:"cancer_qualifiers"`
	CancerAbbreviations map[string]string  `json:"cancer_abbreviations"`
	// Abbreviations is a curated whole-WORD expansion dictionary used by
	// normalizeForMatch (e.g. "nmdar" -> "nmda receptor"). Unlike
	// CancerAbbreviations (whole-string replace), these expand a single token
	// wherever it appears, so "anti-NMDAR encephalitis" canonicalizes to the
	// same form as MONDO's spelled-out "anti-NMDA receptor encephalitis".
	// Keep entries unambiguous (avoid short collision-prone abbrevs like "ms").
	Abbreviations map[string]string `json:"abbreviations"`
	// abbrevRe is a precompiled case-insensitive, word-boundary alternation of
	// all Abbreviations keys, built once by compileAbbrev. Used by
	// ApplyAbbreviations for structure-preserving expansion.
	abbrevRe *regexp.Regexp
}

// compileAbbrev precompiles the whole-word abbreviation alternation regexp.
// Longest keys first so multi-token abbreviations win over their prefixes.
func (m *MedicalTermMappings) compileAbbrev() {
	if m == nil || len(m.Abbreviations) == 0 {
		return
	}
	keys := make([]string, 0, len(m.Abbreviations))
	for k := range m.Abbreviations {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j]) // longest first
		}
		return keys[i] < keys[j] // stable tie-break for determinism
	})
	for i, k := range keys {
		keys[i] = regexp.QuoteMeta(k)
	}
	m.abbrevRe = regexp.MustCompile(`(?i)\b(?:` + strings.Join(keys, "|") + `)\b`)
}

// ApplyAbbreviations expands curated whole-WORD abbreviations in place while
// preserving surrounding punctuation/structure, so the result is still a form
// the text-search index can match: "anti-NMDAR encephalitis" ->
// "anti-NMDA receptor encephalitis". Returns condition unchanged when no
// abbreviation dictionary is configured or nothing matches.
func ApplyAbbreviations(m *MedicalTermMappings, condition string) string {
	if m == nil || m.abbrevRe == nil {
		return condition
	}
	return m.abbrevRe.ReplaceAllStringFunc(condition, func(match string) string {
		if exp, ok := m.Abbreviations[strings.ToLower(match)]; ok {
			return exp
		}
		return match
	})
}

// QualifiersToRemove contains prefixes and suffixes to strip from condition names
type QualifiersToRemove struct {
	Prefixes []string `json:"prefixes"`
	Suffixes []string `json:"suffixes"`
}

// CancerQualifiers contains cancer-specific qualifiers to remove
type CancerQualifiers struct {
	StageQualifiers      []string `json:"stage_qualifiers"`
	MetastasisQualifiers []string `json:"metastasis_qualifiers"`
	ReceptorPatterns     []string `json:"receptor_patterns"`
}

// LoadMedicalTermMappings loads mappings from JSON configuration file
func LoadMedicalTermMappings() *MedicalTermMappings {
	configPath := filepath.FromSlash("conf/medical_term_mappings.json")

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Warning: Could not load medical term mappings (%v), using basic normalization only\n", err)
		return &MedicalTermMappings{
			SpecificPatterns:    make(map[string]string),
			AnatomicalTerms:     make(map[string]string),
			DiseaseCorrections:  make(map[string]string),
			SpellingVariations:  make(map[string]string),
			CancerAbbreviations: make(map[string]string),
			Abbreviations:       make(map[string]string),
			QualifiersRemove:    QualifiersToRemove{Prefixes: []string{}, Suffixes: []string{}},
			CancerQualifiers:    CancerQualifiers{StageQualifiers: []string{}, MetastasisQualifiers: []string{}, ReceptorPatterns: []string{}},
		}
	}

	// Parse the JSON structure
	var rawConfig struct {
		SpecificPatterns struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"specific_patterns"`
		AnatomicalTerms struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"anatomical_terms"`
		QualifiersRemove   QualifiersToRemove `json:"qualifiers_to_remove"`
		DiseaseCorrections struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"disease_corrections"`
		SpellingVariations struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"spelling_variations"`
		CancerQualifiers    CancerQualifiers `json:"cancer_qualifiers"`
		CancerAbbreviations struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"cancer_abbreviations"`
		Abbreviations struct {
			Mappings map[string]string `json:"mappings"`
		} `json:"abbreviations"`
	}

	if err := json.Unmarshal(data, &rawConfig); err != nil {
		fmt.Printf("Warning: Could not parse medical term mappings (%v), using basic normalization only\n", err)
		return &MedicalTermMappings{
			SpecificPatterns:    make(map[string]string),
			AnatomicalTerms:     make(map[string]string),
			DiseaseCorrections:  make(map[string]string),
			SpellingVariations:  make(map[string]string),
			CancerAbbreviations: make(map[string]string),
			Abbreviations:       make(map[string]string),
			QualifiersRemove:    QualifiersToRemove{Prefixes: []string{}, Suffixes: []string{}},
			CancerQualifiers:    CancerQualifiers{StageQualifiers: []string{}, MetastasisQualifiers: []string{}, ReceptorPatterns: []string{}},
		}
	}

	m := &MedicalTermMappings{
		SpecificPatterns:    rawConfig.SpecificPatterns.Mappings,
		AnatomicalTerms:     rawConfig.AnatomicalTerms.Mappings,
		QualifiersRemove:    rawConfig.QualifiersRemove,
		DiseaseCorrections:  rawConfig.DiseaseCorrections.Mappings,
		SpellingVariations:  rawConfig.SpellingVariations.Mappings,
		CancerQualifiers:    rawConfig.CancerQualifiers,
		CancerAbbreviations: rawConfig.CancerAbbreviations.Mappings,
		Abbreviations:       rawConfig.Abbreviations.Mappings,
	}
	m.compileAbbrev()
	return m
}

// RemoveParentheses removes text in parentheses
// Example: "Heart Arrest (Cardiac)" -> "Heart Arrest"
func RemoveParentheses(s string) string {
	reParens := regexp.MustCompile(`\s*\([^)]*\)`)
	return strings.TrimSpace(reParens.ReplaceAllString(s, ""))
}

// ToSingular attempts simple plural -> singular conversion
// Example: "Seizures" -> "Seizure", "Diseases" -> "Disease"
func ToSingular(s string) string {
	// Handle "Diseases" -> "Disease"
	if strings.HasSuffix(s, "eases") {
		return s[:len(s)-1] // Remove 's'
	}
	// Handle "Injuries" -> "Injury"
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	// Handle "Tumors" -> "Tumor", but keep "-sis" (Sepsis, Thrombosis)
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "sis") && !strings.HasSuffix(s, "us") {
		return s[:len(s)-1]
	}
	return s
}

// TryWordOrderSwap handles reversed word order like "Amyloidosis Cardiac" -> "Cardiac Amyloidosis"
func TryWordOrderSwap(condition string) string {
	words := strings.Fields(condition)

	// Only swap if exactly 2 words
	if len(words) == 2 {
		// Swap if second word looks like an adjective (ends in -ic, -al, -ous, etc.)
		secondLower := strings.ToLower(words[1])
		if strings.HasSuffix(secondLower, "ic") ||
			strings.HasSuffix(secondLower, "al") ||
			strings.HasSuffix(secondLower, "ous") ||
			strings.HasSuffix(secondLower, "ar") {
			return words[1] + " " + words[0]
		}
	}

	return condition
}

// SplitSlashOr splits conditions like "HIV/AIDS" or "Recurrent/Advanced Cancer"
func SplitSlashOr(condition string) []string {
	var variations []string

	// Split on slash
	if strings.Contains(condition, "/") {
		parts := strings.Split(condition, "/")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" && trimmed != condition {
				variations = append(variations, trimmed)
			}
		}
	}

	// Split on " or "
	if strings.Contains(strings.ToLower(condition), " or ") {
		parts := strings.Split(condition, " or ")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" && trimmed != condition {
				variations = append(variations, trimmed)
			}
		}
	}

	return variations
}

// sortedKeys returns the keys of a string map in deterministic (sorted) order.
// The normalization helpers below iterate the mapping tables and stop at / order
// by the first match; iterating a Go map directly is randomized, so two
// otherwise-identical runs could pick different variants and produce different
// mappings (a source of run-to-run nondeterminism). Sorting the keys makes the
// whole cascade deterministic.
func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// ApplySpellingVariations handles British/American spelling and common typos
func ApplySpellingVariations(m *MedicalTermMappings, condition string) string {
	if m == nil {
		return condition
	}
	lower := strings.ToLower(condition)

	for _, british := range sortedKeys(m.SpellingVariations) {
		if strings.Contains(lower, british) {
			return strings.ReplaceAll(lower, british, m.SpellingVariations[british])
		}
	}

	return condition
}

// ApplyCancerAbbreviations expands cancer abbreviations and normalizes hyphenation
// Examples: "NSCLC" -> "non-small cell lung cancer", "head-and-neck" -> "head and neck"
func ApplyCancerAbbreviations(m *MedicalTermMappings, condition string) string {
	if m == nil {
		return condition
	}
	lower := strings.ToLower(condition)

	for _, abbrev := range sortedKeys(m.CancerAbbreviations) {
		if strings.Contains(lower, abbrev) {
			return strings.ReplaceAll(lower, abbrev, m.CancerAbbreviations[abbrev])
		}
	}

	return condition
}

// ApplySpecificPatterns tries high-priority exact phrase replacements
func ApplySpecificPatterns(m *MedicalTermMappings, condition string) []string {
	if m == nil {
		return nil
	}
	var variations []string
	lower := strings.ToLower(condition)

	for _, original := range sortedKeys(m.SpecificPatterns) {
		synonym := m.SpecificPatterns[original]
		if strings.Contains(lower, original) {
			variation := strings.ReplaceAll(lower, original, synonym)
			if variation != lower {
				variations = append(variations, variation)
			}
		}
		// Also try reverse mapping
		if strings.Contains(lower, synonym) {
			variation := strings.ReplaceAll(lower, synonym, original)
			if variation != lower {
				variations = append(variations, variation)
			}
		}
	}

	return variations
}

// ApplyAnatomicalTerms tries general anatomical term replacements
func ApplyAnatomicalTerms(m *MedicalTermMappings, condition string) []string {
	if m == nil {
		return nil
	}
	var variations []string
	lower := strings.ToLower(condition)

	for _, original := range sortedKeys(m.AnatomicalTerms) {
		synonym := m.AnatomicalTerms[original]
		// Use word boundaries to avoid partial replacements
		// "heart disease" -> "cardiac disease", but not "sheart" -> "scardiac"
		if strings.Contains(lower, " "+original+" ") ||
			strings.HasPrefix(lower, original+" ") ||
			strings.HasSuffix(lower, " "+original) ||
			lower == original {
			variation := strings.ReplaceAll(lower, original, synonym)
			if variation != lower {
				variations = append(variations, variation)
			}
		}
	}

	return variations
}

// RemoveQualifiers strips temporal/severity modifiers from condition names
func RemoveQualifiers(m *MedicalTermMappings, condition string) string {
	if m == nil {
		return condition
	}
	result := condition
	lower := strings.ToLower(condition)

	// Remove prefixes
	for _, prefix := range m.QualifiersRemove.Prefixes {
		prefixPattern := prefix + " "
		if strings.HasPrefix(lower, prefixPattern) {
			// Preserve original case for the rest of the string
			result = condition[len(prefixPattern):]
			lower = strings.ToLower(result)
		}
	}

	// Remove suffixes
	for _, suffix := range m.QualifiersRemove.Suffixes {
		if strings.Contains(lower, " "+suffix) {
			idx := strings.Index(lower, " "+suffix)
			if idx > 0 {
				result = condition[:idx]
				lower = strings.ToLower(result)
			}
		}
	}

	return strings.TrimSpace(result)
}

// RemoveCancerQualifiers removes cancer-specific qualifiers (stage, receptor markers, metastatic)
// This is more aggressive than general qualifier removal and runs BEFORE it
// Examples:
//
//	"Stage III Colorectal Cancer" -> "Colorectal Cancer"
//	"HER2 Positive Metastatic Breast Cancer" -> "Breast Cancer"
//	"Early-stage Non-small Cell Lung Cancer" -> "Non-small Cell Lung Cancer"
func RemoveCancerQualifiers(m *MedicalTermMappings, condition string) string {
	if m == nil {
		return condition
	}
	result := strings.TrimSpace(condition)
	lower := strings.ToLower(result)

	// Remove stage qualifiers
	for _, stageQual := range m.CancerQualifiers.StageQualifiers {
		stageQualLower := strings.ToLower(stageQual)
		// Try as prefix
		if strings.HasPrefix(lower, stageQualLower+" ") {
			result = strings.TrimSpace(result[len(stageQual)+1:])
			lower = strings.ToLower(result)
		}
		// Try as suffix
		if strings.HasSuffix(lower, " "+stageQualLower) {
			result = strings.TrimSpace(result[:len(result)-len(stageQual)-1])
			lower = strings.ToLower(result)
		}
		// Try in middle (with spaces)
		if strings.Contains(lower, " "+stageQualLower+" ") {
			result = strings.ReplaceAll(result, " "+stageQual+" ", " ")
			result = strings.TrimSpace(result)
			lower = strings.ToLower(result)
		}
	}

	// Remove metastasis qualifiers
	for _, metaQual := range m.CancerQualifiers.MetastasisQualifiers {
		metaQualLower := strings.ToLower(metaQual)
		// Try as prefix
		if strings.HasPrefix(lower, metaQualLower+" ") {
			result = strings.TrimSpace(result[len(metaQual)+1:])
			lower = strings.ToLower(result)
		}
		// Try as suffix
		if strings.HasSuffix(lower, " "+metaQualLower) {
			result = strings.TrimSpace(result[:len(result)-len(metaQual)-1])
			lower = strings.ToLower(result)
		}
		// Try in middle (with spaces)
		if strings.Contains(lower, " "+metaQualLower+" ") {
			result = strings.ReplaceAll(result, " "+metaQual+" ", " ")
			result = strings.TrimSpace(result)
			lower = strings.ToLower(result)
		}
	}

	// Remove receptor patterns (more complex as they can be anywhere)
	for _, receptorPattern := range m.CancerQualifiers.ReceptorPatterns {
		receptorLower := strings.ToLower(receptorPattern)
		// Try as prefix
		if strings.HasPrefix(lower, receptorLower+" ") {
			result = strings.TrimSpace(result[len(receptorPattern)+1:])
			lower = strings.ToLower(result)
		}
		// Try as suffix
		if strings.HasSuffix(lower, " "+receptorLower) {
			result = strings.TrimSpace(result[:len(result)-len(receptorPattern)-1])
			lower = strings.ToLower(result)
		}
		// Try in middle (with spaces)
		if strings.Contains(lower, " "+receptorLower+" ") {
			result = strings.ReplaceAll(result, " "+receptorPattern+" ", " ")
			result = strings.TrimSpace(result)
			lower = strings.ToLower(result)
		}
	}

	return result
}

// ApplyDiseaseCorrections applies disease name corrections (misspellings, alternative names)
func ApplyDiseaseCorrections(m *MedicalTermMappings, condition string) (string, bool) {
	if m == nil {
		return condition, false
	}
	for _, original := range sortedKeys(m.DiseaseCorrections) {
		if strings.EqualFold(condition, original) {
			return m.DiseaseCorrections[original], true
		}
	}
	return condition, false
}

// normalizeForMatch reduces a disease/condition string to a canonical
// comparison form: lowercase, every non-alphanumeric run collapsed to a single
// space, surrounding whitespace trimmed, and — when m is non-nil — curated
// whole-WORD abbreviation expansion (e.g. "nmdar" -> "nmda receptor").
//
// It is applied symmetrically to the trial condition and to each ontology
// name/synonym, so spelling/punctuation/case/abbreviation variants of the SAME
// term collapse together ("Anti-NMDAR Encephalitis" == "anti-NMDA receptor
// encephalitis"). Crucially it is still a WHOLE-STRING comparison: a substring
// of a longer term never matches (condition "cataract" still cannot hit Vici
// syndrome's "absent corpus callosum cataract immunodeficiency" synonym), so it
// does not reopen the word-token over-linking that #28 closed.
func normalizeForMatch(s string, m *MedicalTermMappings) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	fields := strings.Fields(b.String())
	if m != nil && len(m.Abbreviations) > 0 {
		out := make([]string, 0, len(fields))
		for _, w := range fields {
			if exp, ok := m.Abbreviations[w]; ok {
				out = append(out, strings.Fields(exp)...)
			} else {
				out = append(out, w)
			}
		}
		fields = out
	}
	return strings.Join(fields, " ")
}

// collectOntologyIDs resolves a free-text disease/condition name to ontology
// IDs (MONDO or EFO — pass the target dataset's numeric id) using a
// multi-strategy normalization cascade, stopping at the first strategy that
// yields any hit. Returns the deduplicated set of matched identifiers.
//
// This is the shared core extracted from clinical_trials' condition mapping so
// that clinical_trials, intogen and civic all resolve disease names the same
// way. Callers decide how to emit the resulting xrefs.
func collectOntologyIDs(d *DataUpdate, m *MedicalTermMappings, condition string, ontologyDatasetID uint32) map[string]bool {
	found := make(map[string]bool)
	if d == nil || d.lookupService == nil || strings.TrimSpace(condition) == "" {
		return found
	}

	collect := func(name string) {
		if strings.TrimSpace(name) == "" {
			return
		}
		// Canonical form of the looked-up name (abbreviation-expanded). An
		// ontology id is accepted only when one of its own name/synonyms reduces
		// to this SAME canonical form — recovering punctuation/case/abbreviation
		// variants (trial "Anti-NMDAR Encephalitis" vs MONDO "anti-NMDA receptor
		// encephalitis") while still requiring whole-term equality, so the
		// word-token text-search hits #28 removed (condition "cataract" ->
		// Vici syndrome synonym "...cataract...") stay excluded.
		nameNorm := normalizeForMatch(name, m)
		add := func(id string) {
			for termName := range d.ontologyTermNames(id, ontologyDatasetID) {
				if normalizeForMatch(termName, m) == nameNorm {
					found[id] = true
					return
				}
			}
		}
		// One text-search lookup of this cascade variant. We deliberately do NOT
		// strip punctuation for the lookup query: the text index matches on
		// hyphenated tokens, so "anti-NMDA receptor encephalitis" surfaces the
		// term but the hyphen-stripped "anti nmda receptor encephalitis" does
		// not. Abbreviation recall is handled by the structure-preserving
		// expansion step in the cascade below (it keeps the hyphen the index
		// needs); the normalized guard above then accepts the resulting hit.
		result, err := d.lookup(name)
		if err != nil {
			// The lookup DB is static and read-only; a read error here is
			// transient. Retry once rather than silently dropping the mapping
			// (silent drops were a source of run-to-run nondeterminism). Log on
			// persistent failure so it is observable instead of invisible.
			result, err = d.lookup(name)
			if err != nil {
				log.Printf("collectOntologyIDs: lookup %q failed after retry: %v", name, err)
			}
		}
		if err != nil || result == nil {
			return
		}
		for _, xref := range result.Results {
			// Top-level entity directly in the ontology (e.g. an exact MONDO hit).
			if xref.Dataset == ontologyDatasetID {
				add(xref.Identifier)
			}
			// Ontology targets nested in Entries: the common case for a
			// text-search link, but also when another dataset (clinical_trials,
			// ctd, ...) is the top-level result with the ontology nested under it.
			for _, entry := range xref.Entries {
				if entry.Dataset == ontologyDatasetID {
					add(entry.Identifier)
				}
			}
		}
	}

	// 1: exact name
	collect(condition)
	if len(found) > 0 {
		return found
	}
	// 1b: hyphen-normalized form — matches the hyphen-insensitive index keys
	// added by indexSearchText, so odd hyphenation in trial conditions resolves
	// (e.g. "anti-NMDA-receptor encephalitis" -> "anti NMDA receptor
	// encephalitis"). Scoped to the disease cascade; the global lookup path is
	// unchanged, so hyphenated identifier lookups (HLA-A, ...) are unaffected.
	if v := normalizeSearchHyphens(condition); v != condition {
		collect(v)
		if len(found) > 0 {
			return found
		}
	}
	// 2: disease corrections (covid19 -> COVID-19, hiv -> HIV infection)
	if m != nil {
		for _, original := range sortedKeys(m.DiseaseCorrections) {
			if strings.EqualFold(condition, original) {
				collect(m.DiseaseCorrections[original])
				if len(found) > 0 {
					return found
				}
			}
		}
	}
	// 3: spelling variations
	if m != nil {
		if v := ApplySpellingVariations(m, condition); v != condition {
			collect(v)
			if len(found) > 0 {
				return found
			}
		}
	}
	// 3b: cancer abbreviations (NSCLC -> non-small cell lung cancer)
	if m != nil {
		if v := ApplyCancerAbbreviations(m, condition); v != condition {
			collect(v)
			if len(found) > 0 {
				return found
			}
		}
	}
	// 3b2: general whole-word abbreviations (NMDAR -> NMDA receptor),
	// structure-preserving so the hyphenated form the text index needs survives.
	if m != nil {
		if v := ApplyAbbreviations(m, condition); v != condition {
			collect(v)
			if len(found) > 0 {
				return found
			}
		}
	}
	// 3c: remove cancer-specific qualifiers (stage, receptor, metastatic)
	if m != nil {
		if v := RemoveCancerQualifiers(m, condition); v != condition {
			collect(v)
			if len(found) > 0 {
				return found
			}
		}
	}
	// 4: remove parentheses
	if v := RemoveParentheses(condition); v != condition {
		collect(v)
		if len(found) > 0 {
			return found
		}
	}
	// 5: slash/or split (HIV/AIDS)
	for _, v := range SplitSlashOr(condition) {
		collect(v)
		if len(found) > 0 {
			return found
		}
	}
	// 6: specific medical term patterns (heart attack -> myocardial infarction)
	if m != nil {
		for _, v := range ApplySpecificPatterns(m, condition) {
			collect(v)
			if len(found) > 0 {
				return found
			}
		}
	}
	// 7: remove general qualifiers (Acute, Chronic, ...)
	if m != nil {
		if v := RemoveQualifiers(m, condition); v != condition {
			collect(v)
			if len(found) > 0 {
				return found
			}
		}
	}
	// 8: word order swap (Amyloidosis Cardiac -> Cardiac Amyloidosis)
	if v := TryWordOrderSwap(condition); v != condition {
		collect(v)
		if len(found) > 0 {
			return found
		}
	}
	// 9: anatomical term variations (heart -> cardiac, kidney -> renal)
	if m != nil {
		for _, v := range ApplyAnatomicalTerms(m, condition) {
			collect(v)
			if len(found) > 0 {
				return found
			}
		}
	}
	// 10: singular/plural
	if v := ToSingular(condition); v != condition {
		collect(v)
	}
	return found
}
