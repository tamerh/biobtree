package update

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// childDatasets maps parent datasets to their child datasets that are built during parent processing
// When a parent dataset is cleaned up (interrupted or needs update), its child datasets must also be cleaned
// Note: Additional child datasets can be defined in source*.dataset.json using the "childDatasets" attribute
var childDatasets = map[string][]string{
	"uniprot":  {"ufeature"},
	"taxonomy": {"taxchild", "taxparent"},
	"ensembl":  {"ortholog", "paralog"},
	"go":       {"gochild", "goparent"},
	"mesh":     {"meshchild", "meshparent"},
	"hpo":      {"hpochild", "hpoparent"},
	"reactome": {"reactomeparent", "reactomechild"},
	"efo":      {"efochild", "efoparent"},
	"mondo":    {"mondochild", "mondoparent"},
	"chebi":    {"chebichild", "chebiparent"},
	"uberon":   {"uberonchild", "uberonparent"},
	"eco":      {"ecochild", "ecoparent"},
}

// GetChildDatasets returns the list of child datasets for a given parent dataset.
// It merges children from both the hardcoded childDatasets map and config-defined
// "childDatasets" attribute in source*.dataset.json.
// The dataconf parameter can be nil, in which case only hardcoded children are returned.
func GetChildDatasets(datasetName string, dataconf map[string]map[string]string) []string {
	var children []string

	// First, add hardcoded children
	if hardcodedChildren, exists := childDatasets[datasetName]; exists {
		children = append(children, hardcodedChildren...)
	}

	// Then, add config-defined children (if dataconf is available)
	if dataconf != nil {
		if dsConfig, exists := dataconf[datasetName]; exists {
			if childStr, hasChildren := dsConfig["childDatasets"]; hasChildren && childStr != "" {
				configChildren := strings.Split(childStr, ",")
				for _, child := range configChildren {
					child = strings.TrimSpace(child)
					if child != "" {
						children = append(children, child)
					}
				}
			}
		}
	}

	return children
}

// CleanupForIncrementalUpdateFederated removes old bucket files and sorted files for a dataset
// across all federations. This is necessary because a dataset's xrefs may have gone to
// multiple federations (e.g., dbsnp's xrefs to hgnc go to main federation).
//
// Parameters:
// - datasetName: the dataset being updated
// - baseOutDir: the base output directory (e.g., out_prod_v4)
// - datasetFederation: map of dataset names to their federation
// - dataconf: dataset configuration (can be nil)
func CleanupForIncrementalUpdateFederated(datasetName string, baseOutDir string, datasetFederation map[string]string, dataconf map[string]map[string]string) error {
	// Get all federations (from the mapping or default to main)
	federationSet := make(map[string]bool)
	federationSet["main"] = true
	for _, fed := range datasetFederation {
		federationSet[fed] = true
	}

	// Convert to slice
	var federations []string
	for fed := range federationSet {
		federations = append(federations, fed)
	}

	log.Printf("Cleaning up dataset %s across %d federations: %v", datasetName, len(federations), federations)

	// Determine which federation this dataset belongs to
	datasetFed := "main"
	if fed, ok := datasetFederation[datasetName]; ok && fed != "" {
		datasetFed = fed
	}

	// Clean up the dataset's own files in its federation
	ownIndexDir := filepath.Join(baseOutDir, datasetFed, "index")
	if err := CleanupForIncrementalUpdate(datasetName, ownIndexDir, dataconf); err != nil {
		log.Printf("Warning: cleanup in own federation %s failed: %v", datasetFed, err)
	}

	// Clean up xref contributions FROM this dataset in OTHER federations
	// These are files like *_from_{dataset}_sorted.*.index.gz
	for _, federation := range federations {
		if federation == datasetFed {
			continue // Already cleaned above
		}

		indexDir := filepath.Join(baseOutDir, federation, "index")
		if _, err := os.Stat(indexDir); os.IsNotExist(err) {
			continue // Federation index dir doesn't exist yet
		}

		log.Printf("Cleaning xref contributions from %s in federation %s", datasetName, federation)

		// Remove *_from_{dataset}/* bucket directories
		fromPattern := fmt.Sprintf("from_%s", datasetName)
		entries, err := os.ReadDir(indexDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "_derived" || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			fromDir := filepath.Join(indexDir, entry.Name(), fromPattern)
			if _, err := removeDir(fromDir); err != nil {
				log.Printf("Warning: failed to remove from dir %s: %v", fromDir, err)
			}
		}

		// Remove *_from_{dataset}_sorted.*.index.gz files
		xrefPattern := filepath.Join(indexDir, fmt.Sprintf("*_from_%s_sorted.*.index.gz", datasetName))
		xrefFiles, _ := filepath.Glob(xrefPattern)
		for _, f := range xrefFiles {
			if err := os.Remove(f); err != nil {
				log.Printf("Warning: failed to remove xref contribution file %s: %v", f, err)
			} else {
				log.Printf("Removed old xref contribution file: %s", f)
			}
		}

		// Also clean _derived directory
		derivedDir := filepath.Join(indexDir, "_derived")
		if _, err := os.Stat(derivedDir); err == nil {
			derivedEntries, _ := os.ReadDir(derivedDir)
			for _, entry := range derivedEntries {
				if !entry.IsDir() {
					continue
				}
				fromDir := filepath.Join(derivedDir, entry.Name(), fromPattern)
				removeDir(fromDir)
			}
		}
	}

	return nil
}

// CleanupForIncrementalUpdate removes old bucket files and sorted files for a dataset being updated
// This is called before re-parsing a dataset to ensure clean state
//
// Removes:
// 1. {dataset}/forward/* - the dataset's own forward xrefs (bucket files)
// 2. */from_{dataset}/* - reverse xrefs this dataset sent to other datasets (bucket files)
// 3. _derived/*/from_{dataset}/* - reverse xrefs to derived datasets (bucket files)
// 4. {datasetName}_sorted.*.index.gz - old sorted files for this dataset
// 5. textsearch_{datasetName}_sorted.*.index.gz - textsearch contribution files
// 6. *_from_{datasetName}_sorted.*.index.gz - xref contribution files to other datasets
//
// The dataconf parameter is optional (can be nil) and is used to look up config-defined
// child datasets via the "childDatasets" attribute in source*.dataset.json
func CleanupForIncrementalUpdate(datasetName string, indexDir string, dataconf map[string]map[string]string) error {
	log.Printf("Cleaning up bucket files for dataset %s", datasetName)
	var totalRemoved int

	// 1. Remove forward directory: {dataset}/forward/
	forwardDir := filepath.Join(indexDir, datasetName, "forward")
	if removed, err := removeDir(forwardDir); err != nil {
		log.Printf("Warning: failed to remove forward dir %s: %v", forwardDir, err)
	} else {
		totalRemoved += removed
	}

	// 2. Remove from_{dataset}/ directories from all other datasets
	// Walk the index directory looking for from_{datasetName}/ subdirectories
	fromPattern := fmt.Sprintf("from_%s", datasetName)

	entries, err := os.ReadDir(indexDir)
	if err != nil {
		return fmt.Errorf("failed to read index dir %s: %v", indexDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip the dataset being updated (already handled forward dir)
		if entry.Name() == datasetName {
			continue
		}

		// Skip special directories
		if entry.Name() == "_derived" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Check for from_{datasetName}/ subdirectory
		fromDir := filepath.Join(indexDir, entry.Name(), fromPattern)
		if removed, err := removeDir(fromDir); err != nil {
			log.Printf("Warning: failed to remove from dir %s: %v", fromDir, err)
		} else {
			totalRemoved += removed
		}
	}

	// 3. Remove from_{dataset}/ directories from derived datasets
	derivedDir := filepath.Join(indexDir, "_derived")
	if _, err := os.Stat(derivedDir); err == nil {
		derivedEntries, err := os.ReadDir(derivedDir)
		if err == nil {
			for _, entry := range derivedEntries {
				if !entry.IsDir() {
					continue
				}

				fromDir := filepath.Join(derivedDir, entry.Name(), fromPattern)
				if removed, err := removeDir(fromDir); err != nil {
					log.Printf("Warning: failed to remove derived from dir %s: %v", fromDir, err)
				} else {
					totalRemoved += removed
				}
			}
		}
	}

	// 4. Remove old sorted files for this dataset
	// Pattern: {datasetName}_sorted.*.index.gz
	sortedPattern := filepath.Join(indexDir, fmt.Sprintf("%s_sorted.*.index.gz", datasetName))
	sortedFiles, _ := filepath.Glob(sortedPattern)
	for _, f := range sortedFiles {
		if err := os.Remove(f); err != nil {
			log.Printf("Warning: failed to remove sorted file %s: %v", f, err)
		} else {
			totalRemoved++
		}
	}

	// 5. Remove old textsearch files for this dataset
	// Pattern: textsearch_{datasetName}_sorted.*.index.gz
	// This enables incremental updates - when a dataset is re-processed,
	// its textsearch contribution is rebuilt from scratch
	textsearchPattern := filepath.Join(indexDir, fmt.Sprintf("textsearch_%s_sorted.*.index.gz", datasetName))
	textsearchFiles, _ := filepath.Glob(textsearchPattern)
	for _, f := range textsearchFiles {
		if err := os.Remove(f); err != nil {
			log.Printf("Warning: failed to remove textsearch file %s: %v", f, err)
		} else {
			totalRemoved++
			log.Printf("Removed old textsearch file: %s", f)
		}
	}

	// 6. Remove xref contribution files this dataset sent TO other datasets
	// Pattern: *_from_{datasetName}_sorted.*.index.gz
	// This enables incremental updates - when a dataset is re-processed,
	// its xref contributions to other datasets are rebuilt from scratch
	xrefPattern := filepath.Join(indexDir, fmt.Sprintf("*_from_%s_sorted.*.index.gz", datasetName))
	xrefFiles, _ := filepath.Glob(xrefPattern)
	for _, f := range xrefFiles {
		if err := os.Remove(f); err != nil {
			log.Printf("Warning: failed to remove xref contribution file %s: %v", f, err)
		} else {
			totalRemoved++
			log.Printf("Removed old xref contribution file: %s", f)
		}
	}

	// Also remove legacy bucket directories (buckets_*, buckets1_*, etc.)
	legacyPatterns := []string{
		filepath.Join(indexDir, datasetName, "buckets_*"),
		filepath.Join(indexDir, datasetName, "buckets1_*"),
		filepath.Join(indexDir, datasetName, "buckets2_*"),
	}
	for _, pattern := range legacyPatterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			if removed, err := removeDir(match); err != nil {
				log.Printf("Warning: failed to remove legacy bucket dir %s: %v", match, err)
			} else {
				totalRemoved += removed
			}
		}
	}

	log.Printf("Cleanup complete for %s: removed %d files/directories", datasetName, totalRemoved)

	// Log preserved from_* sources (contributions FROM other datasets TO this dataset)
	// These are preserved because the source datasets haven't changed
	// Note: We exclude child datasets from this log since they will be cleaned next
	datasetDir := filepath.Join(indexDir, datasetName)
	if fromDirs, err := filepath.Glob(filepath.Join(datasetDir, "from_*")); err == nil && len(fromDirs) > 0 {
		// Get child datasets to filter them out of the preserved list
		children := GetChildDatasets(datasetName, dataconf)
		childSet := make(map[string]bool)
		for _, child := range children {
			childSet[child] = true
		}

		var preserved []string
		for _, fromDir := range fromDirs {
			sourceName := strings.TrimPrefix(filepath.Base(fromDir), "from_")
			// Skip if this is a child dataset (will be cleaned next)
			if !childSet[sourceName] {
				preserved = append(preserved, sourceName)
			}
		}
		if len(preserved) > 0 {
			log.Printf("Note: Preserving %d reverse xref sources for %s: %v", len(preserved), datasetName, preserved)
			log.Printf("Note: If these source datasets have also changed, they should be re-processed to update their contributions")
		}
	}

	// 7. Clean up child datasets that are built during this dataset's processing
	// e.g., when uniprot is cleaned, also clean ufeature
	// Uses both hardcoded childDatasets map and config-defined "childDatasets" attribute
	children := GetChildDatasets(datasetName, dataconf)
	if len(children) > 0 {
		for _, childName := range children {
			log.Printf("Also cleaning child dataset %s (built by %s)", childName, datasetName)
			if err := cleanupChildDataset(childName, indexDir); err != nil {
				log.Printf("Warning: failed to cleanup child dataset %s: %v", childName, err)
			}
		}
	}

	return nil
}

// cleanupChildDataset cleans up a child dataset's files without recursing to its own children
// This is a simplified version that removes bucket dirs and sorted files
func cleanupChildDataset(datasetName string, indexDir string) error {
	var totalRemoved int

	// Remove forward directory
	forwardDir := filepath.Join(indexDir, datasetName, "forward")
	if removed, err := removeDir(forwardDir); err == nil {
		totalRemoved += removed
	}

	// Remove from_{dataset}/ directories from all other datasets
	fromPattern := fmt.Sprintf("from_%s", datasetName)
	entries, _ := os.ReadDir(indexDir)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == datasetName || entry.Name() == "_derived" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fromDir := filepath.Join(indexDir, entry.Name(), fromPattern)
		if removed, err := removeDir(fromDir); err == nil {
			totalRemoved += removed
		}
	}

	// Remove from derived datasets
	derivedDir := filepath.Join(indexDir, "_derived")
	if derivedEntries, err := os.ReadDir(derivedDir); err == nil {
		for _, entry := range derivedEntries {
			if entry.IsDir() {
				fromDir := filepath.Join(derivedDir, entry.Name(), fromPattern)
				if removed, err := removeDir(fromDir); err == nil {
					totalRemoved += removed
				}
			}
		}
	}

	// Remove sorted files
	sortedPattern := filepath.Join(indexDir, fmt.Sprintf("%s_sorted.*.index.gz", datasetName))
	sortedFiles, _ := filepath.Glob(sortedPattern)
	for _, f := range sortedFiles {
		if err := os.Remove(f); err == nil {
			totalRemoved++
		}
	}

	// Remove textsearch files
	textsearchPattern := filepath.Join(indexDir, fmt.Sprintf("textsearch_%s_sorted.*.index.gz", datasetName))
	textsearchFiles, _ := filepath.Glob(textsearchPattern)
	for _, f := range textsearchFiles {
		if err := os.Remove(f); err == nil {
			totalRemoved++
		}
	}

	// Remove xref contribution files
	xrefPattern := filepath.Join(indexDir, fmt.Sprintf("*_from_%s_sorted.*.index.gz", datasetName))
	xrefFiles, _ := filepath.Glob(xrefPattern)
	for _, f := range xrefFiles {
		if err := os.Remove(f); err == nil {
			totalRemoved++
		}
	}

	if totalRemoved > 0 {
		log.Printf("Cleanup complete for child dataset %s: removed %d files/directories", datasetName, totalRemoved)
	}
	return nil
}

// CleanupAllBuckets removes all bucket files and sorted files
// Used for a full rebuild
func CleanupAllBuckets(indexDir string) error {
	log.Printf("Cleaning up all bucket files in %s", indexDir)
	var totalRemoved int

	entries, err := os.ReadDir(indexDir)
	if err != nil {
		return fmt.Errorf("failed to read index dir %s: %v", indexDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			// Remove sorted files
			if strings.HasSuffix(entry.Name(), ".index.gz") && strings.Contains(entry.Name(), "_sorted") {
				f := filepath.Join(indexDir, entry.Name())
				if err := os.Remove(f); err != nil {
					log.Printf("Warning: failed to remove sorted file %s: %v", f, err)
				} else {
					totalRemoved++
				}
			}
			continue
		}

		// Skip non-bucket directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		datasetDir := filepath.Join(indexDir, entry.Name())

		// Handle _derived directory specially
		if entry.Name() == "_derived" {
			if removed, err := removeDir(datasetDir); err != nil {
				log.Printf("Warning: failed to remove derived dir: %v", err)
			} else {
				totalRemoved += removed
			}
			continue
		}

		// Remove forward/ directory
		forwardDir := filepath.Join(datasetDir, "forward")
		if removed, err := removeDir(forwardDir); err == nil {
			totalRemoved += removed
		}

		// Remove all from_*/ directories
		fromDirs, _ := filepath.Glob(filepath.Join(datasetDir, "from_*"))
		for _, fromDir := range fromDirs {
			if removed, err := removeDir(fromDir); err != nil {
				log.Printf("Warning: failed to remove from dir %s: %v", fromDir, err)
			} else {
				totalRemoved += removed
			}
		}

		// Remove legacy bucket directories
		legacyDirs, _ := filepath.Glob(filepath.Join(datasetDir, "buckets*"))
		for _, legacyDir := range legacyDirs {
			if removed, err := removeDir(legacyDir); err != nil {
				log.Printf("Warning: failed to remove legacy dir %s: %v", legacyDir, err)
			} else {
				totalRemoved += removed
			}
		}
	}

	log.Printf("Full cleanup complete: removed %d files/directories", totalRemoved)
	return nil
}

// CleanupTouchedDatasets removes sorted files for datasets that received
// reverse xrefs from an updated dataset
// These need to be re-sorted after the update
func CleanupTouchedDatasets(touchedDatasets []string, indexDir string) error {
	log.Printf("Cleaning up sorted files for %d touched datasets", len(touchedDatasets))
	var totalRemoved int

	for _, datasetName := range touchedDatasets {
		// Remove sorted files
		sortedPattern := filepath.Join(indexDir, fmt.Sprintf("%s_sorted.*.index.gz", datasetName))
		sortedFiles, _ := filepath.Glob(sortedPattern)
		for _, f := range sortedFiles {
			if err := os.Remove(f); err != nil {
				log.Printf("Warning: failed to remove sorted file %s: %v", f, err)
			} else {
				totalRemoved++
			}
		}
	}

	log.Printf("Touched datasets cleanup complete: removed %d sorted files", totalRemoved)
	return nil
}

// removeDir removes a directory and all its contents, returns count of items removed
func removeDir(dir string) (int, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, nil // Directory doesn't exist, nothing to remove
	}

	// Count files before removal
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			count++
		}
		return nil
	})

	if err := os.RemoveAll(dir); err != nil {
		return 0, err
	}

	return count, nil
}

// GetBucketDirs returns all bucket directories for a dataset
// Used for determining what needs to be processed during sort/concatenation
func GetBucketDirs(datasetName string, indexDir string, isDerived bool) ([]string, error) {
	var dirs []string

	var baseDir string
	if isDerived {
		baseDir = filepath.Join(indexDir, "_derived", datasetName)
	} else {
		baseDir = filepath.Join(indexDir, datasetName)
	}

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return dirs, nil // No bucket directories yet
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Include forward/ and from_*/ directories
		if entry.Name() == "forward" || strings.HasPrefix(entry.Name(), "from_") {
			dirs = append(dirs, filepath.Join(baseDir, entry.Name()))
		}
	}

	return dirs, nil
}

// GetAllBucketFiles returns all bucket files for a dataset across all subdirectories
// Used for sort/concatenation phase
// Finds both compressed (.txt.gz) and uncompressed (.txt) bucket files
func GetAllBucketFiles(datasetName string, indexDir string, isDerived bool) ([]string, error) {
	dirs, err := GetBucketDirs(datasetName, indexDir, isDerived)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, dir := range dirs {
		// Find uncompressed bucket files
		txtMatches, err := filepath.Glob(filepath.Join(dir, "bucket_*.txt"))
		if err == nil {
			files = append(files, txtMatches...)
		}
		// Find compressed bucket files
		gzMatches, err := filepath.Glob(filepath.Join(dir, "bucket_*.txt.gz"))
		if err == nil {
			files = append(files, gzMatches...)
		}
	}

	return files, nil
}

// GetBucketFilesPerSource returns bucket files grouped by source dataset
// For derived datasets with from_{source}/ directories, returns map of source -> files
// Used for textsearch per-source file generation
func GetBucketFilesPerSource(datasetName string, indexDir string, isDerived bool) (map[string][]string, error) {
	result := make(map[string][]string)

	var baseDir string
	if isDerived {
		baseDir = filepath.Join(indexDir, "_derived", datasetName)
	} else {
		baseDir = filepath.Join(indexDir, datasetName)
	}

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return result, nil // No bucket directories yet
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		dirPath := filepath.Join(baseDir, dirName)

		// Get bucket files from this directory (both compressed and uncompressed)
		txtMatches, _ := filepath.Glob(filepath.Join(dirPath, "bucket_*.txt"))
		gzMatches, _ := filepath.Glob(filepath.Join(dirPath, "bucket_*.txt.gz"))
		matches := append(txtMatches, gzMatches...)
		if len(matches) == 0 {
			continue
		}

		// Determine source name from directory
		var sourceName string
		if strings.HasPrefix(dirName, "from_") {
			sourceName = strings.TrimPrefix(dirName, "from_")
		} else if dirName == "forward" {
			sourceName = "forward"
		} else {
			continue
		}

		result[sourceName] = matches
	}

	return result, nil
}

// CleanupInterruptedDatasets cleans up bucket files and sorted files for datasets
// that were interrupted mid-build (status = "processing")
// This should be called at the START of a new build to ensure clean state
//
// The dataconf parameter is optional (can be nil) and is used to look up config-defined
// child datasets via the "childDatasets" attribute in source*.dataset.json
// The datasetFederation parameter maps dataset names to their federation (can be nil for legacy)
func CleanupInterruptedDatasets(state *DatasetState, indexDir, outDir string, dataconf map[string]map[string]string, datasetFederation map[string]string) error {
	interrupted := state.GetInterruptedDatasets()
	if len(interrupted) == 0 {
		return nil
	}

	log.Printf("WARNING: FOUND %d INTERRUPTED DATASETS FROM PREVIOUS BUILD: %v", len(interrupted), interrupted)
	log.Printf("WARNING: These datasets will be cleaned up and must be re-processed")

	for _, datasetName := range interrupted {
		log.Printf("WARNING: Cleaning up interrupted dataset: %s", datasetName)

		// Use the federated cleanup function (handles files across all federations)
		if err := CleanupForIncrementalUpdateFederated(datasetName, outDir, datasetFederation, dataconf); err != nil {
			log.Printf("Warning: cleanup failed for interrupted dataset %s: %v", datasetName, err)
		}

		// Remove from state so it will be reprocessed
		state.RemoveDataset(datasetName)

		// Also remove child datasets from state (uses both hardcoded and config-defined)
		children := GetChildDatasets(datasetName, dataconf)
		for _, childName := range children {
			state.RemoveDataset(childName)
		}
	}

	// Save updated state to main output directory
	if err := SaveDatasetState(state, outDir); err != nil {
		log.Printf("Warning: failed to save state after cleaning interrupted datasets: %v", err)
	}

	log.Printf("WARNING: Cleaned up %d interrupted datasets - they need to be re-built", len(interrupted))
	return nil
}
