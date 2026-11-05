package service

import (
	"math"
	"testing"
)

func TestHypergeomSF(t *testing.T) {
	if p := hypergeomSF(10, 43633, 100, 20); p >= 1e-15 {
		t.Fatalf("strong overlap p=%g, want < 1e-15", p)
	}
	if p := hypergeomSF(1, 43633, 100, 20); p <= 0.01 {
		t.Fatalf("weak overlap p=%g, want > 0.01", p)
	}
	if p := hypergeomSF(0, 43633, 100, 20); p != 1.0 {
		t.Fatalf("k=0 p=%g, want 1", p)
	}
}

func TestBHFDR(t *testing.T) {
	q := bhFDR([]float64{0.01, 0.02, 0.5}, 3)
	if math.Abs(q[0]-0.03) > 1e-12 || math.Abs(q[1]-0.03) > 1e-12 || math.Abs(q[2]-0.5) > 1e-12 {
		t.Fatalf("bhFDR = %v, want [0.03 0.03 0.5]", q)
	}
}

func TestParseMsigdbTarget(t *testing.T) {
	id, name, coll, size, ok := parseMsigdbTarget("M1|GOBP_X|C5:GO|description|100")
	if !ok || id != "M1" || name != "GOBP_X" || coll != "C5:GO" || size != 100 {
		t.Fatalf("got %q %q %q %d %v", id, name, coll, size, ok)
	}
	if _, _, _, _, ok := parseMsigdbTarget("bad|row"); ok {
		t.Fatal("short target should not parse")
	}
}

func TestSplitEscapedPipe(t *testing.T) {
	parts := splitEscapedPipe(`M1|GOBP_X|C5:GO|a \| b|100`)
	if len(parts) != 5 || parts[3] != "a | b" || parts[4] != "100" {
		t.Fatalf("split = %#v", parts)
	}
}

func TestNormalizeEnrichCollection(t *testing.T) {
	base, prefix := normalizeEnrichCollection("C5:GO:BP")
	if base != "C5:GO" || prefix != "GOBP_" {
		t.Fatalf("got %q %q", base, prefix)
	}
	base, prefix = normalizeEnrichCollection("C2:CP:REACTOME")
	if base != "C2:CP:REACTOME" || prefix != "" {
		t.Fatalf("got %q %q", base, prefix)
	}
}
