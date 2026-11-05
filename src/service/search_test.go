package service

import "testing"

func TestIdentifierShaped(t *testing.T) {
	for _, q := range []string{"CTB-180A7.6", "RP11-476M19.1", "HLA-A", "hsa-miR-21-5p"} {
		if !identifierShaped(q) {
			t.Errorf("identifierShaped(%q) = false, want true", q)
		}
	}
	for _, q := range []string{"TP53", "breast cancer", "  ", "a-b"} {
		if identifierShaped(q) {
			t.Errorf("identifierShaped(%q) = true, want false", q)
		}
	}
}

func TestSearchDatasetTier(t *testing.T) {
	cases := map[string]int{
		"hgnc":                0,
		"mirdb":               0,
		"vega":                0,
		"ncrna_interaction":   2,
		"biogrid_interaction": 2,
		"clinical_trials":     1,
		"not_a_dataset":       1,
	}
	for name, want := range cases {
		if got := searchDatasetTier(name); got != want {
			t.Errorf("searchDatasetTier(%q) = %d, want %d", name, got, want)
		}
	}
}
