package update

import (
	"strings"
)

// BucketMethod extracts bucket number from an ID
// Returns bucket number >= 0 on success, or -1 if the ID doesn't match this method's pattern
// (used for multi-bucket-set routing where first matching method wins)
type BucketMethod func(id string, numBuckets int) int

// BucketMethods registry - maps method names to implementations
// All methods preserve lexicographic order for proper k-way merge
// Methods return -1 if the ID doesn't match their expected pattern
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
	"msigdb":    msigdbBucket,    // M5890, M49136 → numeric part after "M"
	"patent":        patentBucket,        // Fallback: alphabetic for all formats
	"patent_us":     patentUSBucket,     // US-5153197-A → numeric, returns -1 if not US-
	"patent_ep":     patentEPBucket,     // EP-1234567-A1 → numeric, returns -1 if not EP-
	"patent_wo":     patentWOBucket,     // WO-2020123456-A1 → numeric, returns -1 if not WO-
	"patent_other":  patentOtherBucket,  // Everything else (D*, RE*, CA-*, etc.) → alphabetic
	"gwas":          gwasBucket,         // GCST000001_rs380390 → numeric from GCST prefix
	"oba":       obaBucket,       // OBA:0001234 or OBA:VT0000188 → first char after colon
	"refseq":    refseqBucket,    // NM_123456.1, NP_123456.1, NC_000001.11 → numeric after underscore
	"gcst":      alphabeticBucket, // GCST010481, VT0000188 → multiple prefixes, use first letter
	"ontology":  ontologyBucket,  // CHEBI:12345, HP:0001234, UBERON:0000001 → numeric after colon

	// Hybrid bucket methods (bucketed for known prefixes, fallback for others)
	"ensembl_hybrid": ensemblHybridBucket, // ENSG/ENSMUSG/ENSRNOG/ENSUMUG/ENSDARG/FBGN → bucketed, others → fallback
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

// msigdbBucket - M5890, M49136 → numeric part after "M"
func msigdbBucket(id string, numBuckets int) int {
	if len(id) < 2 || id[0] != 'M' {
		panic("msigdbBucket: invalid MSigDB id format (expected M followed by digits): " + id)
	}
	return numericLexBucket(id[1:], numBuckets)
}

// patentBucket - fallback alphabetic bucket for all patent formats
// Used as catch-all in multi-bucket-set config
func patentBucket(id string, numBuckets int) int {
	if len(id) < 1 {
		panic("patentBucket: empty id")
	}
	return alphabeticBucket(id, numBuckets)
}

// patentUSBucket - for US patents: US-5153197-A, US-RE42753-E1
// Returns -1 if ID doesn't start with "US-"
// Uses alphanumeric bucketing on part after "US-" for lex order
func patentUSBucket(id string, numBuckets int) int {
	if len(id) < 4 || id[0] != 'U' || id[1] != 'S' || id[2] != '-' {
		return -1 // Not a US- patent
	}
	// Bucket by first char after "US-" (alphanumeric)
	return alphanumBucket(id[3:], numBuckets)
}

// patentEPBucket - for European patents: EP-1234567-A1
// Returns -1 if ID doesn't start with "EP-"
// Uses alphanumeric bucketing on part after "EP-" for lex order
func patentEPBucket(id string, numBuckets int) int {
	if len(id) < 4 || id[0] != 'E' || id[1] != 'P' || id[2] != '-' {
		return -1 // Not an EP- patent
	}
	return alphanumBucket(id[3:], numBuckets)
}

// patentWOBucket - for WIPO/international patents: WO-2020123456-A1
// Returns -1 if ID doesn't start with "WO-"
// Uses alphanumeric bucketing on part after "WO-" for lex order
func patentWOBucket(id string, numBuckets int) int {
	if len(id) < 4 || id[0] != 'W' || id[1] != 'O' || id[2] != '-' {
		return -1 // Not a WO- patent
	}
	return alphanumBucket(id[3:], numBuckets)
}

// patentOtherBucket - for all other patents (D*, RE*, CA-*, JP-*, etc.)
// Uses alphabetic bucketing as catch-all
// Returns -1 only if it matches US-, EP-, or WO- (which should go to their specific buckets)
func patentOtherBucket(id string, numBuckets int) int {
	if len(id) < 1 {
		return -1
	}
	// Reject US-, EP-, WO- patents - they should use their specific buckets
	if len(id) >= 3 && id[2] == '-' {
		if (id[0] == 'U' && id[1] == 'S') ||
			(id[0] == 'E' && id[1] == 'P') ||
			(id[0] == 'W' && id[1] == 'O') {
			return -1
		}
	}
	// Accept everything else with alphabetic bucketing
	return alphabeticBucket(id, numBuckets)
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

// refseqBucket - RefSeq accession IDs: NM_123456.1, NP_123456.1, NC_000001.11
// Format: 2-3 letters + optional underscore + numeric + optional version (.1)
// Uses numeric part for lexicographic bucket assignment
// Examples:
//   - NM_001353961.2 → bucket based on "001353961"
//   - NP_001340890.1 → bucket based on "001340890"
//   - NC_000001.11 → bucket based on "000001"
//   - XM_017000001.2 → bucket based on "017000001"
//   - CP066370.1 → bucket based on "066370" (no underscore format)
//   - NZ_CP066370.1 → bucket based on "066370" (NZ prefix with CP accession)
func refseqBucket(id string, numBuckets int) int {
	if len(id) < 4 {
		panic("refseqBucket: id too short: " + id)
	}

	// Find underscore position
	underscoreIdx := strings.IndexByte(id, '_')

	var numericStart int
	if underscoreIdx > 0 && underscoreIdx < len(id)-1 {
		// Standard format with underscore (NM_123456, NC_000001, etc.)
		numericStart = underscoreIdx + 1
		// Handle NZ_CP format - look for second underscore or first digit after underscore
		afterUnderscore := id[numericStart:]
		// Check if there's another underscore (like NZ_CP066370 which should be NZ_CP_066370-like)
		// Or if it starts with letters (like CP in NZ_CP066370)
		for i := 0; i < len(afterUnderscore); i++ {
			c := afterUnderscore[i]
			if c >= '0' && c <= '9' {
				numericStart = numericStart + i
				break
			}
		}
	} else {
		// No underscore format (CP066370, etc.) - find first digit
		for i := 0; i < len(id); i++ {
			c := id[i]
			if c >= '0' && c <= '9' {
				numericStart = i
				break
			}
		}
		if numericStart == 0 && (id[0] < '0' || id[0] > '9') {
			// No digits found, fall back to alphanumeric bucket
			return alphanumBucket(id, numBuckets)
		}
	}

	// Get numeric part (stop at dot if present)
	numericPart := id[numericStart:]
	dotIdx := strings.IndexByte(numericPart, '.')
	if dotIdx > 0 {
		numericPart = numericPart[:dotIdx]
	}

	if len(numericPart) == 0 || numericPart[0] < '0' || numericPart[0] > '9' {
		// Fall back to alphanumeric bucket for edge cases
		return alphanumBucket(id, numBuckets)
	}

	return numericLexBucket(numericPart, numBuckets)
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

// alphabeticBucket - bucket by first byte for STRICT lexicographic order
// Bucket assignment preserves byte order for proper k-way merge:
//   - Bucket 0: first byte < 'A' (0x00-0x40) - control chars, digits, special chars
//   - Buckets 1-26: first letter A-Z (0x41-0x5A)
//   - Bucket 27: chars between Z and a (0x5B-0x60): [\]^_`
//   - Buckets 28-53: first letter a-z (0x61-0x7A)
//   - Bucket 54: high-byte chars (0x7B+): {|}~ and UTF-8 multi-byte
// Total: 55 buckets (0-54) - strict byte order, case-sensitive
func alphabeticBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		panic("alphabeticBucket: empty id")
	}
	first := id[0]

	// Chars before 'A' (0x00-0x40): control, digits, special
	if first < 'A' {
		return 0
	}

	// Uppercase A-Z (0x41-0x5A)
	if first <= 'Z' {
		return int(first-'A') + 1 // A→1, B→2, ... Z→26
	}

	// Chars between Z and a (0x5B-0x60): [\]^_`
	if first < 'a' {
		return 27
	}

	// Lowercase a-z (0x61-0x7A)
	if first <= 'z' {
		return int(first-'a') + 28 // a→28, b→29, ... z→53
	}

	// High-byte chars (0x7B+): {|}~ and UTF-8 multi-byte
	return 54
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

// ontologyBucket - for ontology IDs with PREFIX:NUMBER format
// CHEBI:12345, HP:0001234, UBERON:0000001, EFO:0000001, etc.
// Uses numeric part after colon for lexicographic bucket assignment
func ontologyBucket(id string, numBuckets int) int {
	colonIdx := strings.IndexByte(id, ':')
	if colonIdx < 0 || colonIdx >= len(id)-1 {
		// No colon found - use alphabetic fallback for IDs like "HGNC:1234" without colon
		return alphabeticBucket(id, numBuckets)
	}
	afterColon := id[colonIdx+1:]
	// Check if first char after colon is a digit
	if len(afterColon) > 0 && afterColon[0] >= '0' && afterColon[0] <= '9' {
		return numericLexBucket(afterColon, numBuckets)
	}
	// Non-numeric after colon (e.g., OBA:VT0000188) - use alphanumeric
	return alphanumBucket(afterColon, numBuckets)
}

// GetBucketMethod returns the method by name, or nil if not found
func GetBucketMethod(name string) BucketMethod {
	return BucketMethods[name]
}

// ============================================================================
// Hybrid bucket methods - return encoded (setIndex, bucket) or -1 for fallback
// Used for datasets with known high-frequency prefixes + unknown prefixes
// ============================================================================

// EnsemblGenePrefixes maps Ensembl gene prefixes to set indices (0-based)
// Ordered by frequency from dbSNP cross-references
// These 6 prefixes cover ~95%+ of Ensembl gene IDs
var EnsemblGenePrefixes = []struct {
	Prefix   string
	SetIndex int
}{
	{"ENSMUSG", 1},  // Mouse genes - check longer prefix first
	{"ENSRNOG", 2},  // Rat genes
	{"ENSUMUG", 3},  // Kangaroo rat genes
	{"ENSDARG", 4},  // Zebrafish genes
	{"ENSG", 0},     // Human genes - shorter prefix checked after longer ones
	{"FBGN", 5},     // FlyBase genes
}

// EnsemblHybridNumSets is the number of bucket sets for Ensembl hybrid mode
const EnsemblHybridNumSets = 6

// ensemblHybridBucket - hybrid bucket method for Ensembl gene IDs
// Returns encoded value: setIndex * numBuckets + bucketNumber for known prefixes
// Returns -1 for unknown prefixes (triggers alphabetic fallback)
//
// Known prefixes (ordered by frequency):
//   - ENSG (Human) → set 0
//   - ENSMUSG (Mouse) → set 1
//   - ENSRNOG (Rat) → set 2
//   - ENSUMUG (Kangaroo rat) → set 3
//   - ENSDARG (Zebrafish) → set 4
//   - FBGN (FlyBase) → set 5
//
// Each set has numBuckets buckets (default 100) based on numeric suffix
// Uses ensemblNumericBucket which handles zero-padded Ensembl numeric suffixes
func ensemblHybridBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		return -1
	}

	// Try each known prefix (longer prefixes checked first to avoid partial matches)
	for _, p := range EnsemblGenePrefixes {
		if len(id) > len(p.Prefix) && strings.HasPrefix(id, p.Prefix) {
			// Extract numeric part after prefix
			numericPart := id[len(p.Prefix):]
			if len(numericPart) == 0 {
				return -1 // No numeric part
			}
			// Verify first char is a digit
			if numericPart[0] < '0' || numericPart[0] > '9' {
				return -1 // Not a valid numeric suffix
			}
			// Use ensemblNumericBucket for zero-padded numeric suffixes
			bucket := ensemblNumericBucket(numericPart, numBuckets)
			// Encode: setIndex * numBuckets + bucket
			return p.SetIndex*numBuckets + bucket
		}
	}

	// Unknown prefix → return -1 to trigger fallback
	return -1
}

// ensemblNumericBucket - bucket method for zero-padded Ensembl numeric suffixes
// Ensembl IDs have format: PREFIX + 11-digit zero-padded number (e.g., ENSG00000000003)
//
// For lexicographic ordering of zero-padded numbers, we CANNOT strip leading zeros
// because "00000000003" < "00000141510" lexicographically (comparing char by char).
// Instead, we use the first 2 digits directly (positions 0 and 1 of the numeric suffix).
//
// Examples (11-digit suffix):
//   - "00000000003" → first two = "00" → bucket 0
//   - "00000141510" → first two = "00" → bucket 0
//   - "00000283245" → first two = "00" → bucket 0
//   - "00100283245" → first two = "00" → bucket 0
//   - "01000283245" → first two = "01" → bucket 1
//   - "10000283245" → first two = "10" → bucket 10
//   - "28324500000" → first two = "28" → bucket 28
//
// This preserves lexicographic order: bucket 0 < bucket 1 < ... < bucket 99
// All IDs in bucket N have numeric suffixes starting with digits that map to N.
//
// Distribution note: Ensembl gene IDs typically range from ~00000000001 to ~00000300000
// for human, so most will be in buckets 0-3. For better distribution, we look at
// positions 5-6 (the 6th and 7th digits) which have more variation.
func ensemblNumericBucket(numericPart string, numBuckets int) int {
	// For Ensembl 11-digit suffixes, positions 5-6 typically have good distribution
	// (e.g., ENSG00000141510 → position 5='1', position 6='4' → bucket 14)
	// This gives us ~100 buckets with reasonable distribution while maintaining lex order

	// Use positions 5-6 for better distribution (if available)
	if len(numericPart) >= 7 {
		d5 := numericPart[5]
		d6 := numericPart[6]
		if d5 >= '0' && d5 <= '9' && d6 >= '0' && d6 <= '9' {
			return int(d5-'0')*10 + int(d6-'0')
		}
	}

	// Fallback to first two digits for shorter suffixes
	if len(numericPart) >= 2 {
		d0 := numericPart[0]
		d1 := numericPart[1]
		if d0 >= '0' && d0 <= '9' && d1 >= '0' && d1 <= '9' {
			return int(d0-'0')*10 + int(d1-'0')
		}
	}

	// Single digit or invalid
	if len(numericPart) >= 1 && numericPart[0] >= '0' && numericPart[0] <= '9' {
		return int(numericPart[0]-'0') * 10
	}

	return 0
}
