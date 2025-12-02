package update

import (
	"log"
	"strconv"
	"strings"
)

// TextSearchDatasetID is the virtual dataset ID for text/keyword search bucketing
// This uses textLinkID ("0") from update.go
const TextSearchDatasetID = "0"

// Default bucket system configuration values
const (
	DefaultBucketReadBufferSize  = 512 * 1024 // 512KB
	DefaultBucketWriteBufferSize = 64 * 1024  // 64KB
	DefaultBucketSortWorkers     = 8
)

// Global bucket system configuration (loaded from application.param.json)
var (
	BucketEnabled         = true
	BucketReadBufferSize  = DefaultBucketReadBufferSize
	BucketWriteBufferSize = DefaultBucketWriteBufferSize
	BucketSortWorkers     = DefaultBucketSortWorkers
)

// LoadBucketSystemConfig loads bucket system configuration from Appconf
func LoadBucketSystemConfig() {
	// bucketEnabled
	if val, ok := config.Appconf["bucketEnabled"]; ok {
		BucketEnabled = (val == "yes" || val == "true" || val == "1")
	}

	// bucketReadBufferSize
	if val, ok := config.Appconf["bucketReadBufferSize"]; ok {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			BucketReadBufferSize = n
		}
	}

	// bucketWriteBufferSize
	if val, ok := config.Appconf["bucketWriteBufferSize"]; ok {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			BucketWriteBufferSize = n
		}
	}

	// bucketSortWorkers
	if val, ok := config.Appconf["bucketSortWorkers"]; ok {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			BucketSortWorkers = n
		}
	}

	log.Printf("Bucket system config: enabled=%v readBuffer=%d writeBuffer=%d sortWorkers=%d",
		BucketEnabled, BucketReadBufferSize, BucketWriteBufferSize, BucketSortWorkers)
}

// BucketConfig holds bucket configuration for a dataset
// Supports single or multiple bucket methods for multi-bucket-set routing
type BucketConfig struct {
	DatasetID      string
	DatasetName    string
	MethodName     string         // Original config value (may be comma-separated)
	NumBuckets     int            // For single method (backward compat)
	Method         BucketMethod   // For single method (backward compat)
	SkipBucketSort bool           // Skip sorting phase - for datasets that run alone and don't need sorted output

	// Multi-bucket-set support (when MethodName contains comma-separated methods)
	Methods     []BucketMethod // Ordered list of methods to try
	MethodNames []string       // Names of methods (for logging)
	NumBucketsPerSet []int     // Bucket count for each method/set
	NumSets     int            // Number of bucket sets (1 for single method)
}

// linkDatasetMap maps child dataset IDs to their parent dataset IDs
// e.g., hpoparent (ID:358) → hpo (ID:58)
var linkDatasetMap map[string]string

// GetLinkDatasetID returns the parent dataset ID for a link dataset,
// or the original ID if it's not a link dataset
func GetLinkDatasetID(datasetID string) string {
	if linkDatasetMap == nil {
		return datasetID
	}
	if parentID, ok := linkDatasetMap[datasetID]; ok {
		return parentID
	}
	return datasetID
}

// fixedBuckets defines methods with fixed bucket counts
// These methods ignore numBuckets parameter and use fixed values
var fixedBuckets = map[string]int{
	"alphabetic":     55,  // 0=<'A', 1-26=A-Z, 27=[\]^_`, 28-53=a-z, 54=high-byte
	"alphanum":       37,  // 0-9 (0-9) + A-Z (10-35) + other (36)
	"uniprot":        261, // A0-Z9 (0-259) + fallback (260)
	"upi":            256, // hex 00-FF
	"rnacentral":     256, // hex 00-FF
	"uniref":         55,  // alphabetic on member ID (uses alphabeticBucket)
	"patent_nodash":  55,  // alphabetic for no-dash patents (uses alphabeticBucket)
}

// LoadBucketConfigs reads bucket configuration from loaded Dataconf
// Only loads PRIMARY dataset entries (not aliases) to avoid duplicate bucket writers
// Also builds linkDatasetMap for routing child datasets to parent buckets
// Supports comma-separated bucket methods for multi-bucket-set routing
func LoadBucketConfigs() map[string]*BucketConfig {
	cfgs := make(map[string]*BucketConfig)
	linkDatasetMap = make(map[string]string)

	// First pass: load primary bucket configs
	for datasetName, props := range config.Dataconf {
		// Skip aliases - only load primary dataset entries
		// Aliases are marked with "_alias": "true" by configs.go
		if props["_alias"] == "true" {
			continue
		}

		methodNameStr, hasMethod := props["bucketMethod"]
		if !hasMethod || methodNameStr == "" {
			continue // No bucket method → uses fallback
		}

		// Note: Link datasets with their own bucketMethod ARE loaded here
		// They override the parent's bucket routing for their own entries
		// Link datasets WITHOUT bucketMethod are handled by linkDatasetMap (second pass)

		datasetID := props["id"]

		// skipBucketSort - for datasets that run alone and don't need sorted output
		// Default: false (sorting enabled)
		skipSort := false
		if skipStr, ok := props["skipBucketSort"]; ok {
			skipSort = (skipStr == "yes" || skipStr == "true" || skipStr == "1")
		}

		// Check if this is a multi-method config (comma-separated)
		methodNames := strings.Split(methodNameStr, ",")
		for i := range methodNames {
			methodNames[i] = strings.TrimSpace(methodNames[i])
		}

		if len(methodNames) > 1 {
			// Multi-bucket-set configuration
			cfg := &BucketConfig{
				DatasetID:      datasetID,
				DatasetName:    datasetName,
				MethodName:     methodNameStr,
				SkipBucketSort: skipSort,
				NumSets:        len(methodNames),
				Methods:        make([]BucketMethod, len(methodNames)),
				MethodNames:    methodNames,
				NumBucketsPerSet: make([]int, len(methodNames)),
			}

			allValid := true
			for i, mName := range methodNames {
				method := GetBucketMethod(mName)
				if method == nil {
					log.Printf("Warning: unknown bucket method '%s' in multi-method config for dataset '%s'",
						mName, datasetName)
					allValid = false
					break
				}
				cfg.Methods[i] = method

				// Get bucket count for this method
				numBuckets := 100 // default
				if nbStr, ok := props["numBuckets"]; ok {
					if n, err := strconv.Atoi(nbStr); err == nil && n > 0 {
						numBuckets = n
					}
				}
				if fixed, hasFixed := fixedBuckets[mName]; hasFixed {
					numBuckets = fixed
				}
				cfg.NumBucketsPerSet[i] = numBuckets
			}

			if !allValid {
				continue
			}

			// Set backward-compat fields to first method
			cfg.Method = cfg.Methods[0]
			cfg.NumBuckets = cfg.NumBucketsPerSet[0]

			cfgs[datasetID] = cfg
			log.Printf("Bucket config loaded (multi-set): %s (ID:%s) methods=%v buckets=%v skipSort=%v",
				datasetName, datasetID, methodNames, cfg.NumBucketsPerSet, skipSort)
		} else {
			// Single method configuration (backward compatible)
			methodName := methodNames[0]
			method := GetBucketMethod(methodName)
			if method == nil {
				log.Printf("Warning: unknown bucket method '%s' for dataset '%s', using fallback",
					methodName, datasetName)
				continue
			}

			numBuckets := 100 // default
			if nbStr, ok := props["numBuckets"]; ok {
				if n, err := strconv.Atoi(nbStr); err == nil && n > 0 {
					numBuckets = n
				}
			}

			if fixed, hasFixed := fixedBuckets[methodName]; hasFixed {
				if numBuckets != fixed {
					log.Printf("Note: %s uses fixed bucket count %d (ignoring configured %d)",
						methodName, fixed, numBuckets)
				}
				numBuckets = fixed
			}

			cfgs[datasetID] = &BucketConfig{
				DatasetID:      datasetID,
				DatasetName:    datasetName,
				MethodName:     methodName,
				NumBuckets:     numBuckets,
				Method:         method,
				SkipBucketSort: skipSort,
				NumSets:        1,
				Methods:        []BucketMethod{method},
				MethodNames:    []string{methodName},
				NumBucketsPerSet: []int{numBuckets},
			}

			log.Printf("Bucket config loaded: %s (ID:%s) method=%s buckets=%d skipSort=%v",
				datasetName, datasetID, methodName, numBuckets, skipSort)
		}
	}

	// Second pass: build linkDatasetMap for child→parent routing
	// Only for link datasets that don't have their own bucket config
	for datasetName, props := range config.Dataconf {
		if props["_alias"] == "true" {
			continue
		}

		linkDatasetName, hasLinkDataset := props["linkdataset"]
		if !hasLinkDataset {
			continue
		}

		childID := props["id"]

		// Skip if this link dataset already has its own bucket config
		// (loaded in first pass with its own bucketMethod)
		if _, hasOwnConfig := cfgs[childID]; hasOwnConfig {
			log.Printf("Link dataset %s (ID:%s) has own bucket config, not mapping to parent",
				datasetName, childID)
			continue
		}

		// Get the parent dataset's ID
		parentProps, ok := config.Dataconf[linkDatasetName]
		if !ok {
			continue
		}
		parentID := parentProps["id"]

		// Only map if parent has bucket config
		if _, hasBucket := cfgs[parentID]; hasBucket {
			linkDatasetMap[childID] = parentID
			log.Printf("Link dataset mapped: %s (ID:%s) → %s (ID:%s)",
				datasetName, childID, linkDatasetName, parentID)
		}
	}

	// Add special textsearch bucket config for keyword/text links
	// Uses alphabetic bucketing with strict byte order (55 buckets)
	cfgs[TextSearchDatasetID] = &BucketConfig{
		DatasetID:        TextSearchDatasetID,
		DatasetName:      "textsearch",
		MethodName:       "alphabetic",
		NumBuckets:       55,
		Method:           alphabeticBucket,
		SkipBucketSort:   false,
		NumSets:          1,
		Methods:          []BucketMethod{alphabeticBucket},
		MethodNames:      []string{"alphabetic"},
		NumBucketsPerSet: []int{55},
	}
	log.Printf("Bucket config loaded: textsearch (ID:%s) method=alphabetic buckets=55",
		TextSearchDatasetID)

	return cfgs
}
