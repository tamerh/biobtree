package query

import (
	"biobtree/configs"
	"fmt"
	"strings"
)

// ParserV2 implements the new >> syntax parser for mapping queries
// Phase 2: >> chaining with [] filter support
type ParserV2 struct {
	config *configs.Conf
}

// NewParserV2 creates a new V2 parser instance
func NewParserV2(conf *configs.Conf) *ParserV2 {
	return &ParserV2{config: conf}
}

// parseDatasetWithFilter extracts dataset name and optional filter from "dataset[filter]"
// Returns: dataset name, filter expression, error
// Examples:
//   "hgnc" -> "hgnc", "", nil
//   "hgnc[hgnc.status=='Approved']" -> "hgnc", "hgnc.status=='Approved'", nil
//   "[uniprot.reviewed==true]" -> "", "uniprot.reviewed==true", nil (filter-only)
func (p *ParserV2) parseDatasetWithFilter(part string) (string, string, error) {
	part = strings.TrimSpace(part)

	// Find the first '[' that's not inside quotes
	bracketStart := -1
	inQuote := false
	quoteChar := rune(0)

	for i, ch := range part {
		if ch == '"' || ch == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuote = false
			}
		} else if ch == '[' && !inQuote {
			bracketStart = i
			break
		}
	}

	// No filter - just dataset name
	if bracketStart == -1 {
		return part, "", nil
	}

	// Extract dataset name (empty if starts with '[')
	dataset := strings.TrimSpace(part[:bracketStart])

	// Find matching closing bracket
	bracketEnd := -1
	depth := 0
	inQuote = false
	quoteChar = 0

	for i := bracketStart; i < len(part); i++ {
		ch := rune(part[i])
		if ch == '"' || ch == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuote = false
			}
		} else if !inQuote {
			if ch == '[' {
				depth++
			} else if ch == ']' {
				depth--
				if depth == 0 {
					bracketEnd = i
					break
				}
			}
		}
	}

	if bracketEnd == -1 {
		return "", "", fmt.Errorf("unclosed '[' in filter expression: %s", part)
	}

	// Extract filter (content between brackets)
	filter := strings.TrimSpace(part[bracketStart+1 : bracketEnd])

	if filter == "" {
		return "", "", fmt.Errorf("empty filter expression in brackets: %s", part)
	}

	return dataset, filter, nil
}

// Parse converts "dataset1 >> dataset2 >> dataset3" to []Query
// This parser handles the mapping chain AFTER identifiers are extracted
// Phase 2: Now supports filters in [] brackets
// Examples:
//   "hgnc >> chembl" -> []Query{{MapDataset: "hgnc"}, {MapDataset: "chembl"}}
//   "hgnc[hgnc.status=='Approved'] >> chembl" -> with filter on hgnc
//   "[uniprot.reviewed==true] >> hgnc" -> filter-only first step
func (p *ParserV2) Parse(queryString string) ([]Query, error) {
	// Trim spaces
	queryString = strings.TrimSpace(queryString)

	// Empty query means no mapping (just lookup)
	if queryString == "" {
		return []Query{}, nil
	}

	// Split by >>
	parts := strings.Split(queryString, ">>")

	// Build Query structs for each dataset in the chain
	var queries []Query
	firstDatasetFound := false

	for i, part := range parts {
		part = strings.TrimSpace(part)

		// Skip empty parts (allows ">>dataset" syntax)
		if part == "" {
			continue
		}

		// Parse dataset and optional filter
		dataset, filter, err := p.parseDatasetWithFilter(part)
		if err != nil {
			return nil, fmt.Errorf("error parsing part %d ('%s'): %v", i+1, part, err)
		}

		// Handle filter-only case (starts with '[')
		if dataset == "" {
			// Filter-only: MapDatasetID = 0, which mapFilter interprets as filter on source
			q := Query{
				MapDataset:    "",
				MapDatasetID:  0,
				Filter:        filter,
				IsLinkDataset: false,
				IsLookup:      false,
				Program:       nil,
			}
			queries = append(queries, q)
			continue
		}

		// Handle wildcard "*" - means search everywhere
		if dataset == "*" {
			q := Query{
				MapDataset:    "*",
				MapDatasetID:  0, // 0 means search everywhere
				Filter:        filter,
				IsLinkDataset: false,
				IsLookup:      true, // Wildcard is always a lookup operation
				Program:       nil,
			}
			queries = append(queries, q)
			firstDatasetFound = true
			continue
		}

		// Validate dataset exists
		datasetID, ok := p.config.DataconfIDStringToInt[dataset]
		if !ok {
			return nil, fmt.Errorf("unknown dataset: '%s'", dataset)
		}

		// Check if this is a link dataset (special handling by mapFilter)
		isLinkDataset := false
		if datasetConf, exists := p.config.Dataconf[dataset]; exists {
			if linkDataset, hasLink := datasetConf["linkdataset"]; hasLink && linkDataset != "" {
				isLinkDataset = true
			}
		}

		// Mark first dataset as lookup operation
		isLookup := false
		if !firstDatasetFound {
			isLookup = true
			firstDatasetFound = true
		}

		// Create Query struct
		q := Query{
			MapDataset:    dataset,
			MapDatasetID:  datasetID,
			Filter:        filter,
			IsLinkDataset: isLinkDataset,
			IsLookup:      isLookup,
			Program:       nil,
		}
		queries = append(queries, q)
	}

	// Single >>target is now allowed - it will resolve keyword/ID in target dataset directly
	// This enables queries like: ?i=INCHIKEY&m=>>chembl_molecule
	// The keyword resolves to entries in the target dataset without requiring source>>target format

	return queries, nil
}
