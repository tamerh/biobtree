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

// normalizeFilter prepends the dataset name to field references in a filter expression
// if they don't already have the dataset prefix.
//
// This allows users to write:
//
//	>>chembl_molecule[highestDevelopmentPhase==4]
//
// instead of:
//
//	>>chembl_molecule[chembl_molecule.highestDevelopmentPhase==4]
//
// The function handles:
//   - Simple field comparisons: `field==value` → `dataset.field==value`
//   - Multiple conditions: `field1>5 && field2<10` → `dataset.field1>5 && dataset.field2<10`
//   - Method calls: `field.contains("x")` → `dataset.field.contains("x")`
//   - Preserves existing prefixes: `dataset.field==4` remains unchanged
//   - Preserves string literals: `"text"` is not modified
func normalizeFilter(filter string, dataset string) string {
	if filter == "" || dataset == "" {
		return filter
	}

	// If filter already starts with dataset prefix, return as-is
	if strings.HasPrefix(filter, dataset+".") {
		return filter
	}

	// Pattern to match identifiers that could be field references:
	// - Start of string or after operators/spaces: (&& || ! ( , space)
	// - Identifier: [a-zA-Z_][a-zA-Z0-9_]*
	// - Followed by: dot, comparison operator, or opening paren (method call)
	//
	// We need to NOT match:
	// - Things already prefixed with dataset name
	// - String literals
	// - Boolean literals (true, false)
	// - Numbers
	// - CEL built-in functions

	// Reserved words that shouldn't be prefixed
	// Note: "type" is NOT included here because it's commonly used as a field name
	// in biobtree datasets (e.g., go.type=="biological_process"). CEL's type()
	// function is called with parentheses which our logic handles separately.
	reserved := map[string]bool{
		"true": true, "false": true, "null": true,
		"in": true, "has": true, "all": true, "exists": true, "exists_one": true,
		"map": true, "filter": true, "size": true,
		"int": true, "uint": true, "double": true, "bool": true, "string": true,
		"bytes": true, "list": true, "duration": true, "timestamp": true,
		"dyn": true, "getDate": true, "getFullYear": true, "getMonth": true,
		"getDayOfMonth": true, "getDayOfWeek": true, "getDayOfYear": true,
		"getHours": true, "getMinutes": true, "getSeconds": true,
		"getMilliseconds": true, "startsWith": true, "endsWith": true,
		"matches": true, "contains": true, "overlaps": true, "within": true, "covers": true,
	}

	// Regex to find identifier.something or identifier followed by operator
	// This matches:
	// - ^identifier or after boundary: start of expression or after ( && || ! , space
	// - identifier: [a-zA-Z_][a-zA-Z0-9_]*
	// - followed by: . or comparison operator or (
	//
	// We'll process token by token, being careful about quotes
	var result strings.Builder
	i := 0
	n := len(filter)

	for i < n {
		ch := filter[i]

		// Handle string literals - copy them as-is
		if ch == '"' || ch == '\'' {
			quote := ch
			result.WriteByte(ch)
			i++
			for i < n && filter[i] != quote {
				if filter[i] == '\\' && i+1 < n {
					result.WriteByte(filter[i])
					i++
				}
				result.WriteByte(filter[i])
				i++
			}
			if i < n {
				result.WriteByte(filter[i]) // closing quote
				i++
			}
			continue
		}

		// Handle identifiers
		if isIdentStart(ch) {
			start := i
			for i < n && isIdentPart(filter[i]) {
				i++
			}
			ident := filter[start:i]

			// Check if this identifier should be prefixed
			shouldPrefix := false

			// Skip reserved words
			if !reserved[ident] {
				// Check what comes before (should be start or boundary)
				isBoundary := start == 0
				if !isBoundary && start > 0 {
					prev := filter[start-1]
					isBoundary = prev == '(' || prev == ' ' || prev == '\t' ||
						prev == '&' || prev == '|' || prev == '!' ||
						prev == ',' || prev == '[' || prev == '>'
				}

				// Check what comes after
				if isBoundary && i < n {
					next := filter[i]
					// Field reference: followed by dot, comparison, or is alone
					if next == '.' || next == '=' || next == '!' ||
						next == '<' || next == '>' || next == ')' ||
						next == ' ' || next == '\t' || next == '&' ||
						next == '|' || next == ',' || next == ']' {
						// Make sure it's not already the dataset name
						if ident != dataset {
							shouldPrefix = true
						}
					}
				} else if isBoundary && i == n {
					// At end of filter
					if ident != dataset {
						shouldPrefix = true
					}
				}
			}

			if shouldPrefix {
				result.WriteString(dataset)
				result.WriteByte('.')
			}
			result.WriteString(ident)
			continue
		}

		// Copy other characters as-is
		result.WriteByte(ch)
		i++
	}

	return result.String()
}

// isIdentStart checks if a byte can start an identifier
func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

// isIdentPart checks if a byte can be part of an identifier
func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

// parseDatasetWithFilter extracts dataset name and optional filter from "dataset[filter]"
// Returns: dataset name, filter expression (normalized), error
//
// The filter is automatically normalized: field references without the dataset prefix
// will have the prefix auto-prepended. This allows simpler filter syntax.
//
// Examples:
//
//	"hgnc" -> "hgnc", "", nil
//	"hgnc[status=='Approved']" -> "hgnc", "hgnc.status=='Approved'", nil (prefix auto-added)
//	"hgnc[hgnc.status=='Approved']" -> "hgnc", "hgnc.status=='Approved'", nil (unchanged)
//	"chembl_molecule[highestDevelopmentPhase==4]" -> "chembl_molecule", "chembl_molecule.highestDevelopmentPhase==4", nil
//	"[uniprot.reviewed==true]" -> "", "uniprot.reviewed==true", nil (filter-only, not normalized)
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

	// Normalize filter: auto-prepend dataset prefix to field references if missing
	// This allows: >>chembl_molecule[highestDevelopmentPhase==4]
	// instead of:  >>chembl_molecule[chembl_molecule.highestDevelopmentPhase==4]
	//
	// Uses filterName from config (set at config load time) which handles:
	// - Link datasets (ortholog -> ensembl)
	// - CEL reserved word conflicts (string -> stringdb)
	// - Default: dataset name itself
	if dataset != "" && p.config != nil {
		if datasetConf, exists := p.config.Dataconf[dataset]; exists {
			filter = normalizeFilter(filter, datasetConf["filterName"])
		}
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
