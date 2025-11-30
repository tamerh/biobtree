package update

import (
	"strconv"
	"strings"
)

// BucketMethod extracts bucket number from an ID
type BucketMethod func(id string, numBuckets int) int

// BucketMethods registry - maps method names to implementations
var BucketMethods = map[string]BucketMethod{
	"numeric":    numericBucket,    // Pure numeric: 9606 → 9606 % N
	"uniprot":    uniprotBucket,    // P12345, Q9Y6K9 → first 2 chars
	"go":         goBucket,         // GO:0008150 → 8150 % N
	"ontology":   ontologyBucket,   // PREFIX:NNNNN → extract numeric part
	"mesh":       meshBucket,       // D000001, C012345 → letter bucket + numeric
	"alphabetic": alphabeticBucket, // First letter: A-Z → 0-25, other → 26
	"rsid":       rsidBucket,       // rs123456789 → 123456789 % N (dbSNP)
	"gwas":       gwasBucket,       // GCST000001_RS380390 → 380390 % N
	"chembl":     chemblBucket,     // CHEMBL123456 → 123456 % N
	"reactome":   reactomeBucket,   // R-HSA-12345 → 12345 % N
	"interpro":   interproBucket,   // IPR000001 → 1 % N
	"hmdb":       hmdbBucket,       // HMDB0000001 → 1 % N
	"patent":     patentBucket,     // US-5153197-A → 5153197 % N
	"nct":        nctBucket,        // NCT06401707 → 6401707 % N
	"rnacentral": rnacentralBucket, // URS000149A9AF → hex value % N
	"lipidmaps":  lipidmapsBucket,  // LMFA00000001 → 1 % N
	"upi":        upiBucket,        // UPI00000001A2 → first 2 hex chars after UPI
	"uniref":     unirefBucket,     // UniRef50_P12345 → alphabetic on char after _
	"gcst":       gcstBucket,       // GCST010481 → 10481 % N
	"alphanum":         alphanumBucket,         // 1031_AT → first char (0-9, A-Z) preserves order
	"rhea":             rheaBucket,             // RHEA:16066 or 16066 → 16066 % N
	"string":           stringBucket,           // 9606.ENSP00000377769 → taxid % N
	"pubchem_activity": pubchemActivityBucket,  // 10000020_21965_1 → CID (first part) % N
}

// numeric - for pure numeric IDs (taxonomy, ncbi_gene)
func numericBucket(id string, numBuckets int) int {
	num, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// uniprot - bucket by first 2 characters (letter + digit)
// UniProt IDs: P12345 (old), Q9Y6K9 (new format)
// A0→0, A1→1, ... A9→9, B0→10, ... Z9→259, other→260
// numBuckets is ignored - always uses 261 buckets
func uniprotBucket(id string, numBuckets int) int {
	if len(id) < 2 {
		return 260 // fallback bucket
	}
	first := id[0]
	second := id[1]

	// Convert first char to uppercase if lowercase
	if first >= 'a' && first <= 'z' {
		first -= 32
	}

	if first >= 'A' && first <= 'Z' && second >= '0' && second <= '9' {
		return int(first-'A')*10 + int(second-'0') // 0-259
	}
	return 260 // fallback for unusual IDs
}

// go - GO:0008150 format (uses ontologyBucket internally)
func goBucket(id string, numBuckets int) int {
	return ontologyBucket(id, numBuckets)
}

// ontology - generic PREFIX:NNNNN format (ECO, EFO, MONDO, HPO, UBERON, CL, GO)
// Extracts numeric part after colon and uses modulo
func ontologyBucket(id string, numBuckets int) int {
	colonIdx := strings.Index(id, ":")
	if colonIdx > 0 && colonIdx < len(id)-1 {
		numPart := id[colonIdx+1:]
		num, err := strconv.ParseInt(numPart, 10, 64)
		if err == nil {
			return int(num % int64(numBuckets))
		}
	}
	return alphabeticBucket(id, numBuckets)
}

// mesh - MeSH descriptor IDs: D000001, C012345, etc.
// Extracts numeric part after first letter and uses modulo
func meshBucket(id string, numBuckets int) int {
	if len(id) < 2 {
		return alphabeticBucket(id, numBuckets)
	}
	// Skip first letter, extract numeric part
	numPart := id[1:]
	num, err := strconv.ParseInt(numPart, 10, 64)
	if err == nil {
		return int(num % int64(numBuckets))
	}
	return alphabeticBucket(id, numBuckets)
}

// alphabetic - for text/keyword search, bucket by first letter (lexicographic order)
// Returns 0 for special chars/numbers (they sort first in ASCII), 1-26 for A-Z
// When used standalone, numBuckets should be 27
// Respects numBuckets by using modulo to ensure bucket is always in range
func alphabeticBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		return 0 // fallback bucket
	}
	first := id[0]
	// Convert to uppercase if lowercase
	if first >= 'a' && first <= 'z' {
		first -= 32 // 'a'-'A' = 32
	}
	var bucket int
	if first >= 'A' && first <= 'Z' {
		bucket = int(first-'A') + 1 // 1-26 for A-Z
	} else {
		bucket = 0 // special chars, numbers come first in lexicographic order
	}
	// Respect numBuckets limit using modulo
	return bucket % numBuckets
}

// rsid - for dbSNP rsIDs: rs123456789 → extract numeric part after "rs"
// Handles both "rs123456789" and "RS123456789" formats
func rsidBucket(id string, numBuckets int) int {
	// Remove "rs" or "RS" prefix
	numStr := id
	if len(id) > 2 {
		prefix := strings.ToLower(id[:2])
		if prefix == "rs" {
			numStr = id[2:]
		}
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// gwas - for GWAS composite IDs: GCST000001_RS380390 → extract RS numeric part
// Format: GCST{study_num}_{RS|rs}{snp_num}
func gwasBucket(id string, numBuckets int) int {
	// Find underscore and extract part after it
	underscoreIdx := strings.LastIndex(id, "_")
	if underscoreIdx > 0 && underscoreIdx < len(id)-1 {
		afterUnderscore := id[underscoreIdx+1:]
		// Check if it starts with RS/rs
		if len(afterUnderscore) > 2 {
			prefix := strings.ToUpper(afterUnderscore[:2])
			if prefix == "RS" {
				numStr := afterUnderscore[2:]
				num, err := strconv.ParseInt(numStr, 10, 64)
				if err == nil {
					return int(num % int64(numBuckets))
				}
			}
		}
	}
	// Fallback to alphabetic
	return alphabeticBucket(id, numBuckets)
}

// rhea - for Rhea reaction IDs: RHEA:16066 or just 16066
// Handles both prefixed and non-prefixed numeric IDs
func rheaBucket(id string, numBuckets int) int {
	numStr := id

	// Remove "RHEA:" prefix if present
	if strings.HasPrefix(strings.ToUpper(id), "RHEA:") {
		numStr = id[5:]
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// chembl - for ChEMBL IDs: CHEMBL123456, CHEMBL_ACT_93229, CHEMBL_TC_47
// Extracts numeric part from various ChEMBL ID formats
func chemblBucket(id string, numBuckets int) int {
	// Find last underscore or end of "CHEMBL" prefix
	// CHEMBL123456 → 123456
	// CHEMBL_ACT_93229 → 93229
	// CHEMBL_TC_47 → 47

	// Try to find numeric suffix after last underscore
	lastUnderscore := strings.LastIndex(id, "_")
	if lastUnderscore > 0 && lastUnderscore < len(id)-1 {
		numStr := id[lastUnderscore+1:]
		num, err := strconv.ParseInt(numStr, 10, 64)
		if err == nil {
			return int(num % int64(numBuckets))
		}
	}

	// Fallback: try removing "CHEMBL" prefix and parsing
	numStr := id
	if len(id) > 6 {
		prefix := strings.ToUpper(id[:6])
		if prefix == "CHEMBL" {
			numStr = id[6:]
		}
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// reactome - for Reactome IDs: R-HSA-12345, R-MMU-67890 → extract numeric part after last "-"
// Format: R-{species}-{numeric_id} where species is HSA (human), MMU (mouse), RNO (rat), etc.
func reactomeBucket(id string, numBuckets int) int {
	// Find last dash and extract numeric part
	lastDash := strings.LastIndex(id, "-")
	if lastDash > 0 && lastDash < len(id)-1 {
		numStr := id[lastDash+1:]
		num, err := strconv.ParseInt(numStr, 10, 64)
		if err == nil {
			return int(num % int64(numBuckets))
		}
	}
	return alphabeticBucket(id, numBuckets)
}

// interpro - for InterPro IDs: IPR000001 → extract numeric part after "IPR"
func interproBucket(id string, numBuckets int) int {
	// Remove "IPR" prefix (case insensitive)
	numStr := id
	if len(id) > 3 {
		prefix := strings.ToUpper(id[:3])
		if prefix == "IPR" {
			numStr = id[3:]
		}
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// hmdb - for HMDB IDs: HMDB0000001 → extract numeric part after "HMDB"
func hmdbBucket(id string, numBuckets int) int {
	// Remove "HMDB" prefix (case insensitive)
	numStr := id
	if len(id) > 4 {
		prefix := strings.ToUpper(id[:4])
		if prefix == "HMDB" {
			numStr = id[4:]
		}
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// nct - for ClinicalTrials.gov IDs: NCT06401707 → extract numeric part after "NCT"
func nctBucket(id string, numBuckets int) int {
	// Remove "NCT" prefix (case insensitive)
	numStr := id
	if len(id) > 3 {
		prefix := strings.ToUpper(id[:3])
		if prefix == "NCT" {
			numStr = id[3:]
		}
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// patent - for patent IDs: US-5153197-A, WO-2011041729-A2 → extract numeric part between dashes
// Format: COUNTRY-NUMBER-KIND where NUMBER is the patent number
func patentBucket(id string, numBuckets int) int {
	// Find first dash
	firstDash := strings.Index(id, "-")
	if firstDash < 0 || firstDash >= len(id)-1 {
		return alphabeticBucket(id, numBuckets)
	}

	// Find second dash
	rest := id[firstDash+1:]
	secondDash := strings.Index(rest, "-")
	if secondDash < 0 {
		// No second dash, try to parse everything after first dash
		num, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return alphabeticBucket(id, numBuckets)
		}
		return int(num % int64(numBuckets))
	}

	// Extract numeric part between dashes
	numStr := rest[:secondDash]
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// gcst - for GWAS Catalog Study IDs: GCST010481 → extract numeric part after "GCST"
func gcstBucket(id string, numBuckets int) int {
	// Remove "GCST" prefix (case insensitive)
	numStr := id
	if len(id) > 4 {
		prefix := strings.ToUpper(id[:4])
		if prefix == "GCST" {
			numStr = id[4:]
		}
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// upi - for UniParc IDs: UPI00000001A2 → use first 2 hex chars after "UPI"
// This preserves string sort order: bucket 00 < 01 < ... < FF (256 buckets max)
// numBuckets is ignored - always uses 256 buckets (0x00 to 0xFF)
func upiBucket(id string, numBuckets int) int {
	// Need at least "UPI" + 2 hex chars
	if len(id) < 5 {
		return 255 // fallback bucket
	}

	// Extract first 2 hex chars after "UPI"
	hexStr := id[3:5]
	num, err := strconv.ParseUint(hexStr, 16, 8)
	if err != nil {
		return 255 // fallback bucket
	}
	return int(num) // 0-255
}

// uniref - for UniRef IDs: UniRef50_P12345, UniRef100_UPI002E2621C6
// Buckets by first letter after underscore (A-Z, or U for UPI)
// Uses alphabetic bucketing on the member ID portion
func unirefBucket(id string, numBuckets int) int {
	// Find underscore and get character after it
	idx := strings.Index(id, "_")
	if idx < 0 || idx >= len(id)-1 {
		return alphabeticBucket(id, numBuckets)
	}
	// Use alphabetic bucket on the part after underscore
	return alphabeticBucket(id[idx+1:], numBuckets)
}

// alphanum - for IDs that can start with digit or letter (probe IDs like 1031_AT)
// Preserves lexicographic order: special chars → 0, 0-9 → 1-10, A-Z → 11-36
// When used standalone, numBuckets should be 37
// Respects numBuckets by using modulo to ensure bucket is always in range
func alphanumBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		return 0 // fallback bucket
	}
	first := id[0]

	var bucket int

	// Special chars come first in lexicographic order → bucket 0
	// Digits 0-9 → buckets 1-10
	if first >= '0' && first <= '9' {
		bucket = int(first-'0') + 1 // 1-10
	} else {
		// Convert to uppercase if lowercase
		if first >= 'a' && first <= 'z' {
			first -= 32
		}

		// Letters A-Z → buckets 11-36
		if first >= 'A' && first <= 'Z' {
			bucket = int(first-'A') + 11 // 11-36
		} else {
			bucket = 0 // special chars come first in lexicographic order
		}
	}

	// Respect numBuckets limit using modulo
	return bucket % numBuckets
}

// lipidmaps - for LIPID MAPS IDs: LMFA00000001 → extract numeric part after 4-char prefix
// Format: LM + 2-char category (FA, GL, GP, etc.) + 8-digit number
func lipidmapsBucket(id string, numBuckets int) int {
	// Skip first 4 chars (LM + category code like FA, GL, GP)
	if len(id) < 5 {
		return alphabeticBucket(id, numBuckets)
	}

	numStr := id[4:]
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}
	return int(num % int64(numBuckets))
}

// rnacentral - for RNAcentral IDs: URS000149A9AF → use last 2 hex chars
// RNAcentral IDs are URS + 10 hex digits, with significant digits at the end
// numBuckets is ignored - always uses 256 buckets (0x00 to 0xFF)
func rnacentralBucket(id string, numBuckets int) int {
	// Need at least "URS" + some hex chars
	if len(id) < 5 {
		return 255 // fallback bucket
	}

	// Extract last 2 hex chars of the ID
	hexStr := id[len(id)-2:]
	num, err := strconv.ParseUint(hexStr, 16, 8)
	if err != nil {
		return 255 // fallback bucket
	}
	return int(num) // 0-255
}

// string - for STRING protein IDs: {taxid}.{ENSP_ID}
// Example: 9606.ENSP00000377769 → extract 9606, then 9606 % numBuckets
// Format: taxid (numeric) + "." + Ensembl protein ID
func stringBucket(id string, numBuckets int) int {
	// Find the dot separator
	dotIdx := strings.Index(id, ".")
	if dotIdx <= 0 {
		// No dot found or dot at start, try numeric bucket as fallback
		return numericBucket(id, numBuckets)
	}

	// Extract taxonomy ID (part before the dot)
	taxidStr := id[:dotIdx]
	taxid, err := strconv.ParseInt(taxidStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}

	return int(taxid % int64(numBuckets))
}

// pubchem_activity - for PubChem activity IDs: {CID}_{AID}_{index}
// Example: 10000020_21965_1 → extract CID (10000020), then CID % numBuckets
// Format: CID (numeric) + "_" + AID + "_" + index
func pubchemActivityBucket(id string, numBuckets int) int {
	// Find the first underscore separator
	underscoreIdx := strings.Index(id, "_")
	if underscoreIdx <= 0 {
		// No underscore found, try numeric bucket as fallback
		return numericBucket(id, numBuckets)
	}

	// Extract CID (part before the first underscore)
	cidStr := id[:underscoreIdx]
	cid, err := strconv.ParseInt(cidStr, 10, 64)
	if err != nil {
		return alphabeticBucket(id, numBuckets)
	}

	return int(cid % int64(numBuckets))
}

// GetBucketMethod returns the method by name, or nil if not found
func GetBucketMethod(name string) BucketMethod {
	return BucketMethods[name]
}
