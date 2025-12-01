package update

import (
	"strings"
)

// BucketMethod extracts bucket number from an ID
type BucketMethod func(id string, numBuckets int) int

// BucketMethods registry - maps method names to implementations
// All methods preserve lexicographic order for proper k-way merge
var BucketMethods = map[string]BucketMethod{
	"numeric":    numericLexBucket, // Pure numeric IDs: uses first 2 chars for lex order
	"uniprot":    uniprotBucket,    // P12345, Q9Y6K9 → first 2 chars (letter+digit)
	"alphabetic": alphabeticBucket, // First letter: A-Z → 1-26, other → 0
	"alphanum":   alphanumBucket,   // First char: 0-9 → 1-10, A-Z → 11-36, other → 0
	"upi":        upiBucket,        // UPI00000001A2 → first 2 hex chars after UPI (0-255)
	"rnacentral": rnacentralBucket, // URS000149A9AF → first 2 hex chars after URS (0-255)
	"uniref":     unirefBucket,     // UniRef50_P12345 → alphabetic on part after "_"

	// Large dataset optimized functions (no string allocation):
	"rsid":             rsidBucket,            // rs123456789 → numeric part after "rs" (dbSNP - billions of IDs)
	"string":           stringBucket,          // 9606.ENSP00000377769 → numeric part before "." (STRING - large)
	"pubchem_activity": pubchemActivityBucket, // 10000020_21965_1 → numeric part before first "_"

	// Dataset-specific bucket methods (each handles its own format):
	"go":        goBucket,       // GO:0008150 → numeric part after "GO:"
	"mesh":      meshBucket,     // D000001 → numeric part after first char
	"chembl":    chemblBucket,   // CHEMBL123456, CHEMBL_ACT_93229, CHEMBL_TC_47, etc.
	"interpro":  interproBucket, // IPR000001 → numeric part after "IPR"
	"hmdb":      hmdbBucket,      // HMDB0000001 → numeric part after "HMDB"
	"nct":       nctBucket,       // NCT06401707 → numeric part after "NCT"
	"lipidmaps": lipidmapsBucket, // LMFA00000001 → numeric part after 4-char prefix
	"rhea":      rheaBucket,      // RHEA:16066 → numeric part after "RHEA:"
	"reactome":  reactomeBucket,  // R-HSA-12345 → numeric part after last "-"
	"patent":    patentBucket,    // US-5153197-A → numeric part between dashes
	"gwas":      gwasBucket,      // GCST000001_rs380390 → numeric from GCST prefix
	"oba":       obaBucket,       // OBA:0001234 or OBA:VT0000188 → first char after colon
	"gcst":      alphabeticBucket, // GCST010481, VT0000188 → multiple prefixes, use first letter
}

// numericLexBucket - lexicographically-preserving bucket method for numeric IDs
// Uses first 2 digits to determine bucket (0-99), ensuring lex order when concatenated.
// Examples: "1"→10, "9"→90, "10"→10, "10090"→10, "9606"→96
func numericLexBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		panic("numericLexBucket: empty id")
	}

	first := id[0]
	if first < '0' || first > '9' {
		panic("numericLexBucket: id does not start with digit: " + id)
	}

	if len(id) == 1 {
		return int(first-'0') * 10 // "1"→10, "9"→90
	}

	second := id[1]
	if second >= '0' && second <= '9' {
		return int(first-'0')*10 + int(second-'0') // "10"→10, "96"→96
	}

	return int(first-'0') * 10 // "1-foo"→10
}

// ============================================================================
// Large dataset optimized bucket functions (minimal string allocation)
// ============================================================================

// rsidBucket - optimized for dbSNP rs IDs (billions of entries)
// rs123456789 → applies numericLexBucket to digits after "rs"
func rsidBucket(id string, numBuckets int) int {
	if len(id) < 3 {
		panic("rsidBucket: id too short: " + id)
	}
	return numericLexBucket(id[2:], numBuckets)
}

// stringBucket - optimized for STRING database IDs
// 9606.ENSP00000377769 → applies numericLexBucket to taxid before "."
func stringBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		panic("stringBucket: empty id")
	}
	return numericLexBucket(id, numBuckets)
}

// pubchemActivityBucket - optimized for PubChem activity IDs
// 10000020_21965_1 → applies numericLexBucket to CID before first "_"
func pubchemActivityBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		panic("pubchemActivityBucket: empty id")
	}
	return numericLexBucket(id, numBuckets)
}

// ============================================================================
// Dataset-specific bucket functions
// Each dataset has its own bucket method to avoid cross-dataset interference
// ============================================================================

// goBucket - GO:0008150 → numeric part after "GO:"
func goBucket(id string, numBuckets int) int {
	if len(id) < 4 || id[2] != ':' {
		panic("goBucket: invalid GO id format: " + id)
	}
	return numericLexBucket(id[3:], numBuckets)
}

// rheaBucket - RHEA:16066 → numeric part after "RHEA:"
func rheaBucket(id string, numBuckets int) int {
	if len(id) < 6 || id[4] != ':' {
		panic("rheaBucket: invalid RHEA id format: " + id)
	}
	return numericLexBucket(id[5:], numBuckets)
}

// meshBucket - D000001, C012345 → numeric part after first letter
func meshBucket(id string, numBuckets int) int {
	if len(id) < 2 {
		panic("meshBucket: id too short: " + id)
	}
	return numericLexBucket(id[1:], numBuckets)
}

// chemblBucket - handles all ChEMBL ID formats:
// - CHEMBL123456 (molecule, target, assay, document) → numeric after "CHEMBL"
// - CHEMBL_ACT_93229 (activity) → numeric after last "_"
// - CHEMBL_TC_47 (target component) → numeric after last "_"
// - CHEMBL_BS_2617 (binding site) → numeric after last "_"
// - CHEMBL_MEC_1664 (mechanism) → numeric after last "_"
func chemblBucket(id string, numBuckets int) int {
	if len(id) < 7 || !strings.HasPrefix(id, "CHEMBL") {
		panic("chemblBucket: invalid CHEMBL id format: " + id)
	}
	// Check if it's an underscore variant (CHEMBL_XXX_NNN)
	if id[6] == '_' {
		// Find last underscore and get numeric part after it
		lastUnderscore := strings.LastIndex(id, "_")
		if lastUnderscore > 6 && lastUnderscore < len(id)-1 {
			return numericLexBucket(id[lastUnderscore+1:], numBuckets)
		}
		panic("chemblBucket: invalid CHEMBL underscore id format: " + id)
	}
	// Standard format: CHEMBL123456
	return numericLexBucket(id[6:], numBuckets)
}

// interproBucket - IPR000001 → numeric part after "IPR"
func interproBucket(id string, numBuckets int) int {
	if len(id) < 4 || !strings.HasPrefix(id, "IPR") {
		panic("interproBucket: invalid InterPro id format: " + id)
	}
	return numericLexBucket(id[3:], numBuckets)
}

// hmdbBucket - HMDB0000001 → numeric part after "HMDB"
func hmdbBucket(id string, numBuckets int) int {
	if len(id) < 5 || !strings.HasPrefix(id, "HMDB") {
		panic("hmdbBucket: invalid HMDB id format: " + id)
	}
	return numericLexBucket(id[4:], numBuckets)
}

// nctBucket - NCT06401707 → numeric part after "NCT"
func nctBucket(id string, numBuckets int) int {
	if len(id) < 4 || !strings.HasPrefix(id, "NCT") {
		panic("nctBucket: invalid NCT id format: " + id)
	}
	return numericLexBucket(id[3:], numBuckets)
}

// lipidmapsBucket - LMFA00000001 → numeric part after 4-char category prefix (LM + 2 chars)
func lipidmapsBucket(id string, numBuckets int) int {
	if len(id) < 5 || !strings.HasPrefix(id, "LM") {
		panic("lipidmapsBucket: invalid LipidMaps id format: " + id)
	}
	// LipidMaps IDs: LM + 2-char category + numeric (e.g., LMFA00000001)
	return numericLexBucket(id[4:], numBuckets)
}

// reactomeBucket - R-HSA-12345 → numeric part after last "-"
func reactomeBucket(id string, numBuckets int) int {
	if len(id) < 3 {
		panic("reactomeBucket: id too short: " + id)
	}
	lastDash := strings.LastIndex(id, "-")
	if lastDash < 0 || lastDash >= len(id)-1 {
		panic("reactomeBucket: no dash found in id: " + id)
	}
	return numericLexBucket(id[lastDash+1:], numBuckets)
}

// patentBucket - handles multiple patent ID formats, preserving lex order:
// - US-5153197-A → numeric bucket on patent number between dashes
// - RE43229 → alphanumeric bucket on first 2 chars (RE, D1, etc.)
// For dash format, numeric part preserves lex order within country code.
// For no-dash format, first 2 chars preserve lex order.
func patentBucket(id string, numBuckets int) int {
	if len(id) < 2 {
		panic("patentBucket: id too short: " + id)
	}
	firstDash := strings.Index(id, "-")
	lastDash := strings.LastIndex(id, "-")
	// Standard format with dashes: US-5153197-A
	if firstDash >= 0 && lastDash >= 0 && firstDash < lastDash {
		return numericLexBucket(id[firstDash+1:lastDash], numBuckets)
	}
	// No dashes (RE43229, D123456): use alphanumeric on first 2 chars
	// This groups RE*, D*, PP*, etc. separately while preserving lex order
	return alphanumBucket(id, numBuckets)
}

// gwasBucket - GCST000001_rs380390 → numeric from GCST prefix (after "GCST")
func gwasBucket(id string, numBuckets int) int {
	if len(id) < 5 {
		panic("gwasBucket: id too short: " + id)
	}
	// GWAS association IDs are GCST000001_rs380390
	// Bucket by the GCST numeric part
	if strings.HasPrefix(id, "GCST") {
		// Find end of GCST number (before underscore or end of string)
		endIdx := strings.Index(id, "_")
		if endIdx < 0 {
			endIdx = len(id)
		}
		if endIdx > 4 {
			return numericLexBucket(id[4:endIdx], numBuckets)
		}
	}
	panic("gwasBucket: invalid GWAS id format: " + id)
}

// obaBucket - OBA:0001234 or OBA:VT0000188 → alphanumeric bucket on first char after colon
func obaBucket(id string, numBuckets int) int {
	if len(id) < 5 || id[3] != ':' {
		panic("obaBucket: invalid OBA id format: " + id)
	}
	return alphanumBucket(id[4:], numBuckets)
}

// isNumeric checks if string contains only digits
func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// uniprotBucket - for UniProt IDs: P12345, Q9Y6K9
// Uses first 2 chars (letter+digit) for 260 buckets (A0-Z9)
func uniprotBucket(id string, numBuckets int) int {
	if len(id) < 2 {
		panic("uniprotBucket: id too short: " + id)
	}
	first := id[0]
	if first >= 'a' && first <= 'z' {
		first -= 32
	}
	second := id[1]
	if first >= 'A' && first <= 'Z' && second >= '0' && second <= '9' {
		return int(first-'A')*10 + int(second-'0')
	}
	panic("uniprotBucket: invalid format (expected letter+digit): " + id)
}

// alphabeticBucket - bucket by first letter: 0 for non-letters, 1-26 for A-Z
// Preserves lex order: special/digits→0, A→1, B→2, ... Z→26
func alphabeticBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		panic("alphabeticBucket: empty id")
	}
	first := id[0]
	if first >= 'a' && first <= 'z' {
		first -= 32
	}
	if first >= 'A' && first <= 'Z' {
		return int(first-'A') + 1 // A→1, B→2, ... Z→26
	}
	return 0 // special chars, digits, etc. go to bucket 0
}

// alphanumBucket - for IDs starting with digit or letter (preserves lex order)
// 1-10 for 0-9, 11-36 for A-Z (no modulo to maintain order)
func alphanumBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		panic("alphanumBucket: empty id")
	}
	first := id[0]
	if first >= '0' && first <= '9' {
		return int(first-'0') + 1 // 0→1, 1→2, ... 9→10
	}
	if first >= 'a' && first <= 'z' {
		first -= 32
	}
	if first >= 'A' && first <= 'Z' {
		return int(first-'A') + 11 // A→11, B→12, ... Z→36
	}
	panic("alphanumBucket: id does not start with alphanumeric: " + id)
}

// upiBucket - for UniParc IDs: UPI00000001A2 → first 2 hex chars after "UPI" (0-255)
func upiBucket(id string, numBuckets int) int {
	if len(id) < 5 {
		panic("upiBucket: id too short: " + id)
	}
	hexStr := id[3:5]
	num := hexToInt(hexStr)
	if num < 0 {
		panic("upiBucket: invalid hex chars: " + id)
	}
	return num
}

// rnacentralBucket - for RNAcentral IDs: URS000149A9AF → first 2 hex chars after "URS" (0-255)
// Uses first 2 hex chars to preserve lex order (not last 2 which would break lex order)
func rnacentralBucket(id string, numBuckets int) int {
	// URS + 10 hex chars, e.g., URS000149A9AF
	if len(id) < 5 {
		panic("rnacentralBucket: id too short: " + id)
	}
	hexStr := id[3:5] // first 2 hex chars after "URS"
	num := hexToInt(hexStr)
	if num < 0 {
		panic("rnacentralBucket: invalid hex chars: " + id)
	}
	return num
}

// unirefBucket - for UniRef IDs: UniRef50_P12345 → alphabetic on part after "_"
func unirefBucket(id string, numBuckets int) int {
	idx := strings.Index(id, "_")
	if idx < 0 || idx >= len(id)-1 {
		panic("unirefBucket: invalid format (expected underscore): " + id)
	}
	return alphabeticBucket(id[idx+1:], numBuckets)
}

// hexToInt converts 2-char hex string to int (0-255), returns -1 on error
func hexToInt(s string) int {
	if len(s) != 2 {
		return -1
	}
	val := 0
	for _, c := range s {
		val *= 16
		if c >= '0' && c <= '9' {
			val += int(c - '0')
		} else if c >= 'A' && c <= 'F' {
			val += int(c-'A') + 10
		} else if c >= 'a' && c <= 'f' {
			val += int(c-'a') + 10
		} else {
			return -1
		}
	}
	return val
}

// GetBucketMethod returns the method by name, or nil if not found
func GetBucketMethod(name string) BucketMethod {
	return BucketMethods[name]
}
