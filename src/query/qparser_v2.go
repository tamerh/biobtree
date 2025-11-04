package query

import (
	"biobtree/configs"
	"fmt"
	"strings"
)

// ParserV2 implements the new >> syntax parser for mapping queries
// Phase 1: Simple >> chaining without filters
type ParserV2 struct {
	config *configs.Conf
}

// NewParserV2 creates a new V2 parser instance
func NewParserV2(conf *configs.Conf) *ParserV2 {
	return &ParserV2{config: conf}
}

// Parse converts "dataset1 >> dataset2 >> dataset3" to []Query
// This parser handles the mapping chain AFTER identifiers are extracted
// Example: "hgnc >> chembl" becomes []Query{{MapDataset: "hgnc"}, {MapDataset: "chembl"}}
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
	for i, part := range parts {
		dataset := strings.TrimSpace(part)

		// Validate non-empty
		if dataset == "" {
			return nil, fmt.Errorf("empty dataset name in mapping chain at position %d", i+1)
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

		// Create Query struct (compatible with old parser output)
		q := Query{
			MapDataset:    dataset,
			MapDatasetID:  datasetID,
			Filter:        "", // No filters in Phase 1
			IsLinkDataset: isLinkDataset,
			Program:       nil,
		}
		queries = append(queries, q)
	}

	return queries, nil
}
