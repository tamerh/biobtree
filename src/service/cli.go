package service

import (
	"biobtree/configs"
	"encoding/json"
	"fmt"
	"strings"
)

// CLI handles command-line query interface
type CLI struct {
	service service
}

// Query executes a query from CLI and returns pretty-printed JSON
func (cli *CLI) Query(conf *configs.Conf, queryStr string, datasetFilter string) error {
	// Set package-level config (required by service.init)
	config = conf

	// Initialize service
	cli.service = service{}
	cli.service.init()
	defer cli.service.readEnv.Close()
	defer cli.service.aliasEnv.Close()

	// Parse dataset filter (optional)
	var datasetID uint32
	if datasetFilter != "" {
		var ok bool
		datasetID, ok = config.DataconfIDStringToInt[datasetFilter]
		if !ok {
			return fmt.Errorf("unknown dataset: %s", datasetFilter)
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

		result, err = cli.service.mapFilter(ids, mappingQuery, "")
	} else {
		// Simple lookup (no >>)
		ids := strings.Split(queryStr, ",")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		result, err = cli.service.search(ids, datasetID, "", nil, true, false)
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
