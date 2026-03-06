package update

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
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
	}

	if err := json.Unmarshal(data, &rawConfig); err != nil {
		fmt.Printf("Warning: Could not parse medical term mappings (%v), using basic normalization only\n", err)
		return &MedicalTermMappings{
			SpecificPatterns:    make(map[string]string),
			AnatomicalTerms:     make(map[string]string),
			DiseaseCorrections:  make(map[string]string),
			SpellingVariations:  make(map[string]string),
			CancerAbbreviations: make(map[string]string),
			QualifiersRemove:    QualifiersToRemove{Prefixes: []string{}, Suffixes: []string{}},
			CancerQualifiers:    CancerQualifiers{StageQualifiers: []string{}, MetastasisQualifiers: []string{}, ReceptorPatterns: []string{}},
		}
	}

	return &MedicalTermMappings{
		SpecificPatterns:    rawConfig.SpecificPatterns.Mappings,
		AnatomicalTerms:     rawConfig.AnatomicalTerms.Mappings,
		QualifiersRemove:    rawConfig.QualifiersRemove,
		DiseaseCorrections:  rawConfig.DiseaseCorrections.Mappings,
		SpellingVariations:  rawConfig.SpellingVariations.Mappings,
		CancerQualifiers:    rawConfig.CancerQualifiers,
		CancerAbbreviations: rawConfig.CancerAbbreviations.Mappings,
	}
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

// ApplySpellingVariations handles British/American spelling and common typos
func ApplySpellingVariations(m *MedicalTermMappings, condition string) string {
	if m == nil {
		return condition
	}
	lower := strings.ToLower(condition)

	for british, american := range m.SpellingVariations {
		if strings.Contains(lower, british) {
			return strings.ReplaceAll(lower, british, american)
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

	for abbrev, expanded := range m.CancerAbbreviations {
		if strings.Contains(lower, abbrev) {
			return strings.ReplaceAll(lower, abbrev, expanded)
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

	for original, synonym := range m.SpecificPatterns {
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

	for original, synonym := range m.AnatomicalTerms {
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
	for original, corrected := range m.DiseaseCorrections {
		if strings.EqualFold(condition, original) {
			return corrected, true
		}
	}
	return condition, false
}
