package update

import (
	"log"
	"strconv"
)

// TextSearchDatasetID is the virtual dataset ID for text/keyword search bucketing
// This uses textLinkID ("0") from update.go
const TextSearchDatasetID = "0"

// BucketConfig holds bucket configuration for a dataset
type BucketConfig struct {
	DatasetID   string
	DatasetName string
	MethodName  string
	NumBuckets  int
	Method      BucketMethod
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

// LoadBucketConfigs reads bucket configuration from loaded Dataconf
// Only loads PRIMARY dataset entries (not aliases) to avoid duplicate bucket writers
// Also builds linkDatasetMap for routing child datasets to parent buckets
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

		methodName, hasMethod := props["bucketMethod"]
		if !hasMethod || methodName == "" {
			continue // No bucket method → uses fallback
		}

		// Skip link datasets - they route to their parent's buckets
		if _, hasLinkDataset := props["linkdataset"]; hasLinkDataset {
			continue
		}

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

		datasetID := props["id"]

		cfgs[datasetID] = &BucketConfig{
			DatasetID:   datasetID,
			DatasetName: datasetName,
			MethodName:  methodName,
			NumBuckets:  numBuckets,
			Method:      method,
		}

		log.Printf("Bucket config loaded: %s (ID:%s) method=%s buckets=%d",
			datasetName, datasetID, methodName, numBuckets)
	}

	// Second pass: build linkDatasetMap for child→parent routing
	for datasetName, props := range config.Dataconf {
		if props["_alias"] == "true" {
			continue
		}

		linkDatasetName, hasLinkDataset := props["linkdataset"]
		if !hasLinkDataset {
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
			childID := props["id"]
			linkDatasetMap[childID] = parentID
			log.Printf("Link dataset mapped: %s (ID:%s) → %s (ID:%s)",
				datasetName, childID, linkDatasetName, parentID)
		}
	}

	// Add special textsearch bucket config for keyword/text links
	// Uses alphabetic bucketing (A-Z + other = 27 buckets)
	cfgs[TextSearchDatasetID] = &BucketConfig{
		DatasetID:   TextSearchDatasetID,
		DatasetName: "textsearch",
		MethodName:  "alphabetic",
		NumBuckets:  27,
		Method:      alphabeticBucket,
	}
	log.Printf("Bucket config loaded: textsearch (ID:%s) method=alphabetic buckets=27",
		TextSearchDatasetID)

	return cfgs
}
