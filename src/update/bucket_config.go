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
	DefaultBucketConcatWorkers   = 16 // Higher than sort - concat is I/O bound, not memory bound
)

// Global bucket system configuration (loaded from application.param.json)
var (
	BucketEnabled         = true
	BucketReadBufferSize  = DefaultBucketReadBufferSize
	BucketWriteBufferSize = DefaultBucketWriteBufferSize
	BucketSortWorkers     = DefaultBucketSortWorkers
	BucketConcatWorkers   = DefaultBucketConcatWorkers

	// Unix sort options (external merge sort with bounded memory)
	// These options are appended to: sort -t$'\t' -k1,1 {options} | uniq
	// Note: deduplication is done by `uniq` (full-line), not sort -u (key-only)
	// Default includes: -S 4G (memory limit), --parallel=4, -T /tmp
	UnixSortOptions = "-S 4G --parallel=4 -T /tmp"

	// UnixSortCompressor specifies the compression tool for Unix sort output
	// Options: "pigz" (parallel, faster), "gzip" (standard)
	UnixSortCompressor = "pigz"

	// BucketSortMethod specifies the global sort method for all bucket files
	// Options: "unix" (external merge sort, bounded memory) or "go" (in-memory)
	// Default: "unix" - recommended for large datasets to prevent OOM
	BucketSortMethod = "unix"

	// CompressBuckets enables gzip compression for ALL bucket files globally
	// This ensures consistent compression for both forward and reverse xrefs
	// Default: true - reduces disk usage significantly for large datasets
	CompressBuckets = true

	// XrefSortLevels stores sort level configurations for reverse xrefs
	// Map: sourceDataset -> targetDataset -> []sortLevelTypes
	// Example: {"bgee": {"uberon": ["speciesPriority", "expressionScore"]}}
	// This determines how {target}/from_{source}/ bucket files are sorted
	XrefSortLevels = make(map[string]map[string][]string)
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

	// bucketConcatWorkers (for k-way merge concatenation - I/O bound, can use more workers)
	if val, ok := config.Appconf["bucketConcatWorkers"]; ok {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			BucketConcatWorkers = n
		}
	}

	// keepBucketFiles - preserve bucket files after sorting for debugging
	if val, ok := config.Appconf["keepBucketFiles"]; ok {
		KeepBucketFiles = (val == "yes" || val == "true" || val == "1")
	}

	// unixSortOptions - options for Unix sort command (appended to: sort -t$'\t' -k1,1)
	// Example: "-S 4G --parallel=4 -T /tmp" (no -u, dedup is done by uniq)
	if val, ok := config.Appconf["unixSortOptions"]; ok && val != "" {
		UnixSortOptions = val
	}

	// unixSortCompressor - compression tool for Unix sort output ("pigz" or "gzip")
	if val, ok := config.Appconf["unixSortCompressor"]; ok && val != "" {
		UnixSortCompressor = val
	}

	// bucketSortMethod - global sort method ("unix" or "go")
	// Default: "unix" (external merge sort with bounded memory)
	if val, ok := config.Appconf["bucketSortMethod"]; ok && val != "" {
		BucketSortMethod = val
	}

	// compressBuckets - global compression setting for ALL bucket files
	// Default: true (reduces disk usage significantly)
	if val, ok := config.Appconf["compressBuckets"]; ok {
		CompressBuckets = (val == "yes" || val == "true" || val == "1")
	}

	log.Printf("Bucket system config: enabled=%v sortMethod=%s compress=%v readBuffer=%d writeBuffer=%d sortWorkers=%d concatWorkers=%d keepFiles=%v compressor=%s",
		BucketEnabled, BucketSortMethod, CompressBuckets, BucketReadBufferSize, BucketWriteBufferSize, BucketSortWorkers, BucketConcatWorkers, KeepBucketFiles, UnixSortCompressor)
}

// BucketConfig holds bucket configuration for a dataset
// Supports single or multiple bucket methods for multi-bucket-set routing
type BucketConfig struct {
	DatasetID      string
	DatasetName    string
	MethodName     string         // Original config value (may be comma-separated)
	NumBuckets     int            // For single method (backward compat)
	Method         BucketMethod   // For single method (backward compat)
	SkipBucketSort  bool // Skip sorting phase - for datasets that run alone and don't need sorted output
	CompressBuckets bool // Enable gzip compression for bucket files (reduces disk usage for large datasets like dbsnp)
	UseUnixSort     bool // Use Unix sort command instead of Go in-memory sort (bounded memory, external merge sort)

	// Multi-bucket-set support (when MethodName contains comma-separated methods)
	Methods     []BucketMethod // Ordered list of methods to try
	MethodNames []string       // Names of methods (for logging)
	NumBucketsPerSet []int     // Bucket count for each method/set
	NumSets     int            // Number of bucket sets (1 for single method)

	// Hybrid mode support (bucketed for known patterns, alphabetic fallback for others)
	// When HybridMode=true and bucket method returns -1, Write() uses alphabetic fallback
	HybridMode bool

	// Incremental update support
	// IsDerived indicates this config was auto-created for a derived dataset
	// (datasets from xref*.json that have no parser, only receive reverse xrefs)
	// Derived datasets are stored under _derived/ parent directory
	IsDerived bool

	// Custom sort configuration for datasets with special ordering needs
	// SortFields: Unix sort key specification (e.g., "-k1,1 -k7,7" for key + priority)
	// If empty, uses default "-k1,1" (sort by first field only)
	SortFields string

	// StripFieldAfterSort: Field index (1-based) to remove after sorting
	// Used to remove temporary sort fields (like priority) that shouldn't be stored
	// 0 = no stripping (default)
	StripFieldAfterSort int
}

// linkDatasetMap maps child dataset IDs to their parent dataset IDs
// e.g., hpoparent (ID:358) → hpo (ID:58)
var linkDatasetMap map[string]string

// datasetIDToName maps dataset IDs to dataset names
// Built during LoadBucketConfigs from config.Dataconf
var datasetIDToName map[string]string

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

// GetDatasetName returns the dataset name for a given dataset ID
// Returns empty string if ID is not found
func GetDatasetName(datasetID string) string {
	if datasetIDToName == nil {
		return ""
	}
	return datasetIDToName[datasetID]
}

// BuildDatasetIDToNameMap builds the reverse lookup from ID to name
// Called during initialization
func BuildDatasetIDToNameMap() {
	datasetIDToName = make(map[string]string)
	for name, props := range config.Dataconf {
		if props["_alias"] == "true" {
			continue
		}
		if id, ok := props["id"]; ok {
			datasetIDToName[id] = name
		}
	}
	log.Printf("Built dataset ID to name map: %d entries", len(datasetIDToName))
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

// hybridBucketMethods defines methods that support hybrid mode
// These methods return -1 for unrecognized patterns, triggering alphabetic fallback
// Map value is the number of sets the hybrid method uses
var hybridBucketMethods = map[string]int{
	"ensembl_hybrid": EnsemblHybridNumSets, // 6 sets: ENSG, ENSMUSG, ENSRNOG, ENSUMUG, ENSDARG, FBGN
}

// LoadBucketConfigs reads bucket configuration from loaded Dataconf
// Only loads PRIMARY dataset entries (not aliases) to avoid duplicate bucket writers
// Also builds linkDatasetMap for routing child datasets to parent buckets
// Supports comma-separated bucket methods for multi-bucket-set routing
func LoadBucketConfigs() map[string]*BucketConfig {
	cfgs := make(map[string]*BucketConfig)
	linkDatasetMap = make(map[string]string)

	// Build reverse lookup from dataset ID to name
	BuildDatasetIDToNameMap()

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

		// Note: compressBuckets and useUnixSort are now GLOBAL settings (not per-dataset)
		// This fixes the reverse xref inheritance bug where reverse xrefs used target's config
		// Global settings are loaded in LoadBucketSystemConfig(): CompressBuckets, BucketSortMethod

		// Check if this is a multi-method config (comma-separated)
		methodNames := strings.Split(methodNameStr, ",")
		for i := range methodNames {
			methodNames[i] = strings.TrimSpace(methodNames[i])
		}

		if len(methodNames) > 1 {
			// Multi-bucket-set configuration
			cfg := &BucketConfig{
				DatasetID:       datasetID,
				DatasetName:     datasetName,
				MethodName:      methodNameStr,
				SkipBucketSort:  skipSort,
				CompressBuckets: CompressBuckets,               // Global setting
				UseUnixSort:     BucketSortMethod == "unix",    // Global setting
				NumSets:         len(methodNames),
				Methods:         make([]BucketMethod, len(methodNames)),
				MethodNames:     methodNames,
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

			// Read custom sortFields from config (e.g., "-k1,1 -k5,5r" for evidence-based sorting)
			if sortFields, ok := props["sortFields"]; ok && sortFields != "" {
				cfg.SortFields = sortFields
			}
			// Read stripFieldAfterSort (1-based field index to remove after sorting)
			if stripField, ok := props["stripFieldAfterSort"]; ok && stripField != "" {
				if fieldNum, err := strconv.Atoi(stripField); err == nil && fieldNum > 0 {
					cfg.StripFieldAfterSort = fieldNum
				}
			}

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

			// Check if this is a hybrid bucket method
			hybridNumSets, isHybrid := hybridBucketMethods[methodName]
			if isHybrid {
				// Hybrid mode: create multi-set config with fallback support
				// Total buckets = numSets * numBuckets (encoded as setIndex * numBuckets + bucket)
				totalBuckets := hybridNumSets * numBuckets

				// Create per-set bucket counts (all sets use same numBuckets)
				bucketsPerSet := make([]int, hybridNumSets)
				for i := 0; i < hybridNumSets; i++ {
					bucketsPerSet[i] = numBuckets
				}

				cfgs[datasetID] = &BucketConfig{
					DatasetID:        datasetID,
					DatasetName:      datasetName,
					MethodName:       methodName,
					NumBuckets:       totalBuckets,
					Method:           method,
					SkipBucketSort:   skipSort,
					CompressBuckets:  CompressBuckets,             // Global setting
					UseUnixSort:      BucketSortMethod == "unix",  // Global setting
					NumSets:          hybridNumSets,
					Methods:          []BucketMethod{method},
					MethodNames:      []string{methodName},
					NumBucketsPerSet: bucketsPerSet,
					HybridMode:       true,
				}

				// Read custom sortFields from config (e.g., "-k1,1 -k5,5r" for evidence-based sorting)
				if sortFields, ok := props["sortFields"]; ok && sortFields != "" {
					cfgs[datasetID].SortFields = sortFields
				}
				// Read stripFieldAfterSort (1-based field index to remove after sorting)
				if stripField, ok := props["stripFieldAfterSort"]; ok && stripField != "" {
					if fieldNum, err := strconv.Atoi(stripField); err == nil && fieldNum > 0 {
						cfgs[datasetID].StripFieldAfterSort = fieldNum
					}
				}

				log.Printf("Bucket config loaded (hybrid): %s (ID:%s) method=%s sets=%d buckets=%d totalBuckets=%d skipSort=%v compress=%v",
					datasetName, datasetID, methodName, hybridNumSets, numBuckets, totalBuckets, skipSort, CompressBuckets)
			} else {
				// Standard single-method config
				cfgs[datasetID] = &BucketConfig{
					DatasetID:        datasetID,
					DatasetName:      datasetName,
					MethodName:       methodName,
					NumBuckets:       numBuckets,
					Method:           method,
					SkipBucketSort:   skipSort,
					CompressBuckets:  CompressBuckets,             // Global setting
					UseUnixSort:      BucketSortMethod == "unix",  // Global setting
					NumSets:          1,
					Methods:          []BucketMethod{method},
					MethodNames:      []string{methodName},
					NumBucketsPerSet: []int{numBuckets},
					HybridMode:       false,
				}

				// Read custom sortFields from config (e.g., "-k1,1 -k5,5r" for evidence-based sorting)
				if sortFields, ok := props["sortFields"]; ok && sortFields != "" {
					cfgs[datasetID].SortFields = sortFields
				}
				// Read stripFieldAfterSort (1-based field index to remove after sorting)
				if stripField, ok := props["stripFieldAfterSort"]; ok && stripField != "" {
					if fieldNum, err := strconv.Atoi(stripField); err == nil && fieldNum > 0 {
						cfgs[datasetID].StripFieldAfterSort = fieldNum
					}
				}

				if cfgs[datasetID].SortFields != "" {
					log.Printf("Bucket config loaded: %s (ID:%s) method=%s buckets=%d skipSort=%v sortFields=%s stripField=%d",
						datasetName, datasetID, methodName, numBuckets, skipSort, cfgs[datasetID].SortFields, cfgs[datasetID].StripFieldAfterSort)
				} else if CompressBuckets {
					log.Printf("Bucket config loaded: %s (ID:%s) method=%s buckets=%d skipSort=%v compress=true",
						datasetName, datasetID, methodName, numBuckets, skipSort)
				} else {
					log.Printf("Bucket config loaded: %s (ID:%s) method=%s buckets=%d skipSort=%v",
						datasetName, datasetID, methodName, numBuckets, skipSort)
				}
			}
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
	// Marked as IsDerived because it receives data via WriteReverse (from_{source}/ directories)
	// from multiple source datasets, not via WriteForward from a single parser
	// SortFields: Sort by key (field 1) then priority (field 7) for model species ordering
	// StripFieldAfterSort: Remove priority field (7) after sorting to save storage
	cfgs[TextSearchDatasetID] = &BucketConfig{
		DatasetID:           TextSearchDatasetID,
		DatasetName:         "textsearch",
		MethodName:          "alphabetic",
		NumBuckets:          55,
		Method:              alphabeticBucket,
		SkipBucketSort:      false,
		CompressBuckets:     CompressBuckets,            // Global setting
		UseUnixSort:         BucketSortMethod == "unix", // Global setting
		NumSets:             1,
		Methods:             []BucketMethod{alphabeticBucket},
		MethodNames:         []string{"alphabetic"},
		NumBucketsPerSet:    []int{55},
		IsDerived:           true,       // Uses from_{source}/ subdirs, not forward/
		SortFields:          "-k1,1 -k7,7", // Sort by key, then model species priority
		StripFieldAfterSort: 7,          // Strip priority field after sorting
	}
	log.Printf("Bucket config loaded: textsearch (ID:%s) method=alphabetic buckets=55 (derived-style) compress=%v sortFields=%s stripField=%d",
		TextSearchDatasetID, CompressBuckets, "-k1,1 -k7,7", 7)

	// Load xrefSort configurations
	// This specifies how reverse xrefs (target/from_source/) should be sorted
	LoadXrefSortConfigs()

	return cfgs
}

// LoadXrefSortConfigs loads sort level configurations for reverse xrefs
// Config format in source.dataset.json:
//
//	"bgee": {
//	  "xrefSort": {
//	    "uberon": ["speciesPriority", "expressionScore"],
//	    "cl": ["speciesPriority", "expressionScore"]
//	  }
//	}
//
// This means: when sorting uberon/from_bgee/, use these sort levels
func LoadXrefSortConfigs() {
	for datasetName, props := range config.Dataconf {
		// Skip aliases
		if props["_alias"] == "true" {
			continue
		}

		xrefSortStr, ok := props["xrefSort"]
		if !ok || xrefSortStr == "" {
			continue
		}

		// Parse the JSON-like config: {"uberon": ["speciesPriority", "expressionScore"]}
		// Format: target1:level1,level2;target2:level1,level2
		// Example: uberon:speciesPriority,expressionScore;cl:speciesPriority,expressionScore
		if XrefSortLevels[datasetName] == nil {
			XrefSortLevels[datasetName] = make(map[string][]string)
		}

		// Parse semicolon-separated target configs
		targetConfigs := strings.Split(xrefSortStr, ";")
		for _, targetConfig := range targetConfigs {
			targetConfig = strings.TrimSpace(targetConfig)
			if targetConfig == "" {
				continue
			}

			// Parse target:levels format
			parts := strings.SplitN(targetConfig, ":", 2)
			if len(parts) != 2 {
				continue
			}

			targetDataset := strings.TrimSpace(parts[0])
			levelsStr := strings.TrimSpace(parts[1])

			// Parse comma-separated levels
			levels := strings.Split(levelsStr, ",")
			var sortLevels []string
			for _, level := range levels {
				level = strings.TrimSpace(level)
				if level != "" {
					sortLevels = append(sortLevels, level)
				}
			}

			if len(sortLevels) > 0 {
				XrefSortLevels[datasetName][targetDataset] = sortLevels
				log.Printf("Reverse xref sort config: %s -> %s: %v", datasetName, targetDataset, sortLevels)
			}
		}
	}
}

// GetXrefSortLevels returns the sort levels for a source->target xref
// Returns nil if no sort levels are configured
func GetXrefSortLevels(sourceDataset, targetDataset string) []string {
	if targets, ok := XrefSortLevels[sourceDataset]; ok {
		if levels, ok := targets[targetDataset]; ok {
			return levels
		}
	}
	return nil
}

// HasSortLevels returns true if the dataset has any sort levels configured
// Used to determine if entry lines need dummy sort values
func HasSortLevels(datasetName string) bool {
	if targets, ok := XrefSortLevels[datasetName]; ok {
		return len(targets) > 0
	}
	return false
}

// GetSortLevelCount returns the max sort level count for a dataset
// Returns 0 if no sort levels are configured
func GetSortLevelCount(datasetName string) int {
	if targets, ok := XrefSortLevels[datasetName]; ok {
		maxCount := 0
		for _, levels := range targets {
			if len(levels) > maxCount {
				maxCount = len(levels)
			}
		}
		return maxCount
	}
	return 0
}

// GetDummySortValues returns dummy sort level values for a dataset
// These ensure entries/xrefs without real sort values have consistent field count
// Returns empty string if no sorting configured
// Dummy values use "0000" which sorts before all real values (both priority "01"+ and scores "0001"+)
func GetDummySortValues(datasetName string) string {
	count := GetSortLevelCount(datasetName)
	if count == 0 {
		return ""
	}
	// Build dummy values: "0000" for each level (sorts first for both priority and score types)
	result := ""
	for i := 0; i < count; i++ {
		result += "\t0000"
	}
	return result
}

// LoadDerivedBucketConfigs auto-creates bucket configs for derived datasets
// (datasets from xref*.json that have no parser/path defined)
// These datasets only receive reverse xrefs from other datasets (no parser, no forward xrefs)
// Uses alphabetic bucketing with 1 bucket (low data volume) and stores under _derived/
//
// Also creates bucket configs for source datasets without bucketMethod (like bgee)
// These are NOT marked as derived since they have parsers
func LoadDerivedBucketConfigs(existingConfigs map[string]*BucketConfig) map[string]*BucketConfig {
	derivedCount := 0
	sourceCount := 0

	for datasetName, props := range config.Dataconf {
		// Skip aliases
		if props["_alias"] == "true" {
			continue
		}

		datasetID := props["id"]

		// Skip if already has bucket config (source dataset or link dataset with own config)
		if _, exists := existingConfigs[datasetID]; exists {
			continue
		}

		// Skip if this is a link dataset mapped to a parent
		if linkDatasetMap != nil {
			if _, isLink := linkDatasetMap[datasetID]; isLink {
				continue
			}
		}

		// Check if this is a source dataset (has path) or derived dataset (no path)
		// Source datasets have parsers and create forward xrefs
		// Derived datasets only receive reverse xrefs from other datasets
		_, hasPath := props["path"]
		isDerived := !hasPath

		// Auto-create config
		// Uses alphabetic method with 55 buckets for source datasets, 1 bucket for derived
		numBuckets := 1
		if !isDerived {
			numBuckets = 55 // Source datasets may have more data
		}

		cfg := &BucketConfig{
			DatasetID:        datasetID,
			DatasetName:      datasetName,
			MethodName:       "alphabetic",
			NumBuckets:       numBuckets,
			Method:           alphabeticBucket,
			SkipBucketSort:   false,
			CompressBuckets:  CompressBuckets,             // Global setting
			UseUnixSort:      BucketSortMethod == "unix",  // Global setting
			NumSets:          1,
			Methods:          []BucketMethod{alphabeticBucket},
			MethodNames:      []string{"alphabetic"},
			NumBucketsPerSet: []int{numBuckets},
			IsDerived:        isDerived,
		}

		existingConfigs[datasetID] = cfg
		if isDerived {
			derivedCount++
		} else {
			sourceCount++
		}
	}

	if derivedCount > 0 {
		log.Printf("Auto-created bucket configs for %d derived datasets (stored under _derived/)", derivedCount)
	}
	if sourceCount > 0 {
		log.Printf("Auto-created bucket configs for %d source datasets without bucketMethod", sourceCount)
	}

	return existingConfigs
}
