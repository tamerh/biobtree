package update

import (
	"hash/fnv"
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
	"hash":       hashBucket,       // Generic hash fallback
	"alphabetic": alphabeticBucket, // First letter: A-Z → 0-25, other → 26
}

// numeric - for pure numeric IDs (taxonomy, ncbi_gene)
func numericBucket(id string, numBuckets int) int {
	num, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return hashBucket(id, numBuckets)
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
	return hashBucket(id, numBuckets)
}

// mesh - MeSH descriptor IDs: D000001, C012345, etc.
// Extracts numeric part after first letter and uses modulo
func meshBucket(id string, numBuckets int) int {
	if len(id) < 2 {
		return hashBucket(id, numBuckets)
	}
	// Skip first letter, extract numeric part
	numPart := id[1:]
	num, err := strconv.ParseInt(numPart, 10, 64)
	if err == nil {
		return int(num % int64(numBuckets))
	}
	return hashBucket(id, numBuckets)
}

// hashBucket - FNV hash fallback for any string
func hashBucket(id string, numBuckets int) int {
	h := fnv.New32a()
	h.Write([]byte(id))
	return int(h.Sum32() % uint32(numBuckets))
}

// alphabetic - for text/keyword search, bucket by first letter
// Returns 0-25 for A-Z, 26 for other (numbers, special chars, non-ASCII)
// numBuckets is ignored - always uses 27 buckets
func alphabeticBucket(id string, numBuckets int) int {
	if len(id) == 0 {
		return 26 // "other" bucket
	}
	first := id[0]
	// Convert to uppercase if lowercase
	if first >= 'a' && first <= 'z' {
		first -= 32 // 'a'-'A' = 32
	}
	if first >= 'A' && first <= 'Z' {
		return int(first - 'A') // 0-25
	}
	return 26 // "other" bucket for numbers, special chars, etc.
}

// GetBucketMethod returns the method by name, or nil if not found
func GetBucketMethod(name string) BucketMethod {
	return BucketMethods[name]
}
