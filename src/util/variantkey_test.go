package util

import (
	"strings"
	"testing"
)

func TestVariantKeyShort(t *testing.T) {
	k, hashed := VariantKey("1", 69094, "G", "A")
	if hashed {
		t.Fatalf("short variant should not be hashed, got %q", k)
	}
	if k != "1:69094:G:A" {
		t.Fatalf("want 1:69094:G:A, got %q", k)
	}
}

func TestVariantKeyLongIsBoundedAndHashed(t *testing.T) {
	longAlt := strings.Repeat("ACGT", 200) // 800 bases -> full key > 511
	k, hashed := VariantKey("1", 2522791, "C", longAlt)
	if !hashed {
		t.Fatalf("long variant should be hashed, got %q (len %d)", k, len(k))
	}
	if len(k) > LMDBMaxKeySize {
		t.Fatalf("hashed key must be <= %d bytes, got %d (%q)", LMDBMaxKeySize, len(k), k)
	}
}

// The core guarantee: a large indel written under VariantKey is found when
// looked up by its FULL chr:pos:ref:alt coordinate, because the read-side
// NormalizeVariantLookupKey applies the identical transform.
func TestNormalizeMatchesWriteKey(t *testing.T) {
	longAlt := strings.Repeat("ACGTTGCA", 100) // 800 bases
	writeKey, hashed := VariantKey("1", 2522791, "C", longAlt)
	if !hashed {
		t.Fatal("expected long variant to hash")
	}
	full := "1:2522791:C:" + longAlt // how a user would query it
	readKey := NormalizeVariantLookupKey(full)
	if readKey != writeKey {
		t.Fatalf("read/write key mismatch:\n write=%q\n read =%q", writeKey, readKey)
	}
	// Case-insensitivity: lowercase alleles must resolve to the same key.
	if got := NormalizeVariantLookupKey(strings.ToLower(full)); got != writeKey {
		t.Fatalf("lowercase query mismatch:\n write=%q\n read =%q", writeKey, got)
	}
}

func TestNormalizeShortPassthrough(t *testing.T) {
	for _, id := range []string{"rs2691305", "1:69094:G:A", "BRCA1", "P38398"} {
		if got := NormalizeVariantLookupKey(id); got != id {
			t.Fatalf("short id %q should pass through unchanged, got %q", id, got)
		}
	}
}

// Two distinct long alleles at the same locus must get distinct keys.
func TestNormalizeDistinctLongAlleles(t *testing.T) {
	a := "1:2522791:C:" + strings.Repeat("A", 600)
	b := "1:2522791:C:" + strings.Repeat("A", 599) + "T"
	if NormalizeVariantLookupKey(a) == NormalizeVariantLookupKey(b) {
		t.Fatal("distinct long alleles collided to the same key")
	}
}
