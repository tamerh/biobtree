package util

import (
	"crypto/sha1"
	"fmt"
	"strconv"
	"strings"
)

// LMDBMaxKeySize is the maximum LMDB key length in bytes. Variant coordinate
// keys ("chr:pos:ref:alt") can exceed this for large indels (long ref/alt
// sequences), which would otherwise make the write fail.
const LMDBMaxKeySize = 511

// variantKeyAlleleCap bounds each ref/alt prefix kept in a hashed long-variant
// key, so the whole key stays well under LMDBMaxKeySize.
const variantKeyAlleleCap = 200

// VariantKey builds the storage key for a coordinate variant.
//
//   - Common case: "chr:pos:ref:alt".
//   - Large indels whose full key would exceed the LMDB key limit: a bounded,
//     deterministic key of truncated ref/alt prefixes plus a short sha1 of the
//     FULL (uppercased) ref/alt for uniqueness — e.g.
//     "1:2522791:CTATA…(≤200):C:0a1b2c3d4e5f6a7b". The complete ref/alt must
//     still be stored in the record's attributes so no data is lost.
//
// ref/alt are uppercased so the key is case-stable (matching how identifiers are
// normalized on lookup). Returns the key and whether it was hashed.
func VariantKey(chrom string, pos int64, ref, alt string) (string, bool) {
	ref = strings.ToUpper(ref)
	alt = strings.ToUpper(alt)
	key := fmt.Sprintf("%s:%d:%s:%s", chrom, pos, ref, alt)
	if len(key) <= LMDBMaxKeySize {
		return key, false
	}
	rp, ap := ref, alt
	if len(rp) > variantKeyAlleleCap {
		rp = rp[:variantKeyAlleleCap]
	}
	if len(ap) > variantKeyAlleleCap {
		ap = ap[:variantKeyAlleleCap]
	}
	h := sha1.Sum([]byte(ref + "|" + alt))
	return fmt.Sprintf("%s:%d:%s:%s:%x", chrom, pos, rp, ap, h[:8]), true
}

// NormalizeVariantLookupKey applies the SAME long-key transform used by
// VariantKey to a lookup identifier, so that querying a large indel by its full
// "chr:pos:ref:alt" coordinate resolves to the stored (hashed) key. Identifiers
// within the key-size limit (rsIDs, gene symbols, normal-length coordinates,
// already-hashed keys, …) are returned unchanged. VCF alleles never contain
// ":", so a 4-way split recovers chr/pos/ref/alt exactly.
func NormalizeVariantLookupKey(id string) string {
	if len(id) <= LMDBMaxKeySize {
		return id
	}
	parts := strings.SplitN(id, ":", 4)
	if len(parts) != 4 {
		return id
	}
	pos, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return id
	}
	key, _ := VariantKey(parts[0], pos, parts[2], parts[3])
	return key
}
