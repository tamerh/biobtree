package query

import (
	"testing"
)

func TestNormalizeFilterSimpleField(t *testing.T) {
	tests := []struct {
		name     string
		filter   string
		dataset  string
		expected string
	}{
		{
			name:     "simple field comparison",
			filter:   "highestDevelopmentPhase==4",
			dataset:  "chembl_molecule",
			expected: "chembl_molecule.highestDevelopmentPhase==4",
		},
		{
			name:     "field with dot access already present",
			filter:   "chembl_molecule.highestDevelopmentPhase==4",
			dataset:  "chembl_molecule",
			expected: "chembl_molecule.highestDevelopmentPhase==4",
		},
		{
			name:     "field with greater than",
			filter:   "highestDevelopmentPhase>2",
			dataset:  "chembl_molecule",
			expected: "chembl_molecule.highestDevelopmentPhase>2",
		},
		{
			name:     "boolean field comparison",
			filter:   "reviewed==true",
			dataset:  "uniprot",
			expected: "uniprot.reviewed==true",
		},
		{
			name:     "string comparison with quotes",
			filter:   `status=="Approved"`,
			dataset:  "hgnc",
			expected: `hgnc.status=="Approved"`,
		},
		{
			name:     "multiple conditions with &&",
			filter:   "highestDevelopmentPhase>=3 && reviewed==true",
			dataset:  "chembl_molecule",
			expected: "chembl_molecule.highestDevelopmentPhase>=3 && chembl_molecule.reviewed==true",
		},
		{
			name:     "multiple conditions with ||",
			filter:   "status==\"Active\" || status==\"Completed\"",
			dataset:  "clinical_trials",
			expected: "clinical_trials.status==\"Active\" || clinical_trials.status==\"Completed\"",
		},
		{
			name:     "method call - contains",
			filter:   "name.contains(\"aspirin\")",
			dataset:  "chembl_molecule",
			expected: "chembl_molecule.name.contains(\"aspirin\")",
		},
		{
			name:     "negation operator",
			filter:   "!reviewed",
			dataset:  "uniprot",
			expected: "!uniprot.reviewed",
		},
		{
			name:     "complex expression with parentheses",
			filter:   "(highestDevelopmentPhase>=3) && (reviewed==true)",
			dataset:  "chembl_molecule",
			expected: "(chembl_molecule.highestDevelopmentPhase>=3) && (chembl_molecule.reviewed==true)",
		},
		{
			name:     "preserve string literals",
			filter:   `name=="chembl_molecule"`,
			dataset:  "chembl_molecule",
			expected: `chembl_molecule.name=="chembl_molecule"`,
		},
		{
			name:     "field less than float",
			filter:   "resolution<2.0",
			dataset:  "pdb",
			expected: "pdb.resolution<2.0",
		},
		{
			name:     "empty filter",
			filter:   "",
			dataset:  "uniprot",
			expected: "",
		},
		{
			name:     "empty dataset",
			filter:   "reviewed==true",
			dataset:  "",
			expected: "reviewed==true",
		},
		{
			name:     "nested field access",
			filter:   "molecule.type==\"Small molecule\"",
			dataset:  "chembl",
			expected: "chembl.molecule.type==\"Small molecule\"",
		},
		{
			name:     "already fully qualified nested field",
			filter:   "chembl.molecule.type==\"Small molecule\"",
			dataset:  "chembl",
			expected: "chembl.molecule.type==\"Small molecule\"",
		},
		{
			name:     "not equal operator",
			filter:   "status!=\"Withdrawn\"",
			dataset:  "chembl_molecule",
			expected: "chembl_molecule.status!=\"Withdrawn\"",
		},
		{
			name:     "reserved word true not prefixed",
			filter:   "reviewed==true",
			dataset:  "uniprot",
			expected: "uniprot.reviewed==true",
		},
		{
			name:     "reserved word false not prefixed",
			filter:   "reviewed==false",
			dataset:  "uniprot",
			expected: "uniprot.reviewed==false",
		},
		{
			name:     "contains function preserved",
			filter:   "description.contains(\"kinase\")",
			dataset:  "go",
			expected: "go.description.contains(\"kinase\")",
		},
		{
			name:     "type field with string value",
			filter:   `type=="biological_process"`,
			dataset:  "go",
			expected: `go.type=="biological_process"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeFilter(tt.filter, tt.dataset)
			if result != tt.expected {
				t.Errorf("normalizeFilter(%q, %q) = %q, want %q",
					tt.filter, tt.dataset, result, tt.expected)
			}
		})
	}
}

func TestNormalizeFilterEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		filter   string
		dataset  string
		expected string
	}{
		{
			name:     "underscore in field name",
			filter:   "am_pathogenicity>=0.9",
			dataset:  "alphamissense",
			expected: "alphamissense.am_pathogenicity>=0.9",
		},
		{
			name:     "number in field name",
			filter:   "score2>100",
			dataset:  "mydata",
			expected: "mydata.score2>100",
		},
		{
			name:     "mixed case field name",
			filter:   "highestDevelopmentPhase==4",
			dataset:  "chembl_molecule",
			expected: "chembl_molecule.highestDevelopmentPhase==4",
		},
		{
			name:     "single quoted string",
			filter:   "name=='test'",
			dataset:  "mydata",
			expected: "mydata.name=='test'",
		},
		{
			name:     "escaped quote in string",
			filter:   `name=="test\"value"`,
			dataset:  "mydata",
			expected: `mydata.name=="test\"value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeFilter(tt.filter, tt.dataset)
			if result != tt.expected {
				t.Errorf("normalizeFilter(%q, %q) = %q, want %q",
					tt.filter, tt.dataset, result, tt.expected)
			}
		})
	}
}
