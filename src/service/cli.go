package service

import (
	"biobtree/configs"
	"encoding/json"
	"fmt"
	"strings"
)

// CLI handles command-line query interface
type CLI struct {
	service *Service
}

// Query executes a query from CLI and returns pretty-printed JSON
// mode: "full" for detailed response with attributes, "lite" for compact IDs only
func (cli *CLI) Query(conf *configs.Conf, queryStr string, datasetFilter string, mode string) error {
	// Set package-level config (required by service.init)
	config = conf

	// Initialize service
	cli.service = &Service{}
	cli.service.init()
	defer cli.service.Close()

	// Parse dataset filter (optional) - supports comma-separated: uniprot,ensembl,hgnc
	var datasetFilters []uint32
	if datasetFilter != "" {
		for _, ds := range strings.Split(datasetFilter, ",") {
			ds = strings.TrimSpace(ds)
			if ds == "" {
				continue
			}
			if id, ok := config.DataconfIDStringToInt[ds]; ok {
				datasetFilters = append(datasetFilters, id)
			} else {
				return fmt.Errorf("unknown dataset: %s", ds)
			}
		}
	}

	// Detect query type and execute
	var result interface{}
	var err error

	if strings.Contains(queryStr, ">>") {
		// Chain/map query: "P27348 >> hgnc" or "cas9 >> uniprot >> hgnc"
		// Split by first >> to separate IDs from mapping chain
		parts := strings.SplitN(queryStr, ">>", 2)

		// Part 1: Identifiers (can be comma-separated)
		idsStr := strings.TrimSpace(parts[0])
		ids := strings.Split(idsStr, ",")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}

		// Part 2: Mapping query (may have more >>)
		mappingQuery := ""
		if len(parts) > 1 {
			mappingQuery = strings.TrimSpace(parts[1])
		}

		if mode == "lite" {
			result, err = cli.service.MapFilterLite(ids, mappingQuery, "")
		} else {
			// Full mode - get result and enrich with query echo and stats
			res, e := cli.service.MapFilter(ids, mappingQuery, "")
			if e == nil {
				rawQuery := idsStr + " >>" + mappingQuery
				EnrichMapFilterResultFull(res, ids, mappingQuery, rawQuery)
			}
			result, err = res, e
		}
	} else {
		// Simple lookup (no >>)
		ids := strings.Split(queryStr, ",")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}

		if mode == "lite" {
			result, err = cli.service.searchLite(ids, datasetFilters, "", datasetFilter)
		} else {
			// Full mode - get result and enrich with query echo and stats
			res, e := cli.service.Search(ids, datasetFilters, "", nil, true, false)
			if e == nil {
				rawQuery := queryStr
				if datasetFilter != "" {
					rawQuery += " s=" + datasetFilter
				}
				EnrichResultFull(res, ids, datasetFilter, rawQuery)
			}
			result, err = res, e
		}
	}

	if err != nil {
		return err
	}

	// Pretty print JSON (always)
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonBytes))

	return nil
}
