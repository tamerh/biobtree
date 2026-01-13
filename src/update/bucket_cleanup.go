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
func CleanupForIncrementalUpdate(datasetName string, indexDir string) error {
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

	// 7. Clean up child datasets that are built during this dataset's processing
	// e.g., when uniprot is cleaned, also clean ufeature
	if children, hasChildren := childDatasets[datasetName]; hasChildren {
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
func GetAllBucketFiles(datasetName string, indexDir string, isDerived bool) ([]string, error) {
	dirs, err := GetBucketDirs(datasetName, indexDir, isDerived)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "bucket_*.txt"))
		if err != nil {
			continue
		}
		files = append(files, matches...)
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

		// Get bucket files from this directory
		matches, err := filepath.Glob(filepath.Join(dirPath, "bucket_*.txt"))
		if err != nil || len(matches) == 0 {
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
func CleanupInterruptedDatasets(state *DatasetState, indexDir string) error {
	interrupted := state.GetInterruptedDatasets()
	if len(interrupted) == 0 {
		return nil
	}

	log.Printf("Found %d interrupted datasets from previous build: %v", len(interrupted), interrupted)

	for _, datasetName := range interrupted {
		log.Printf("Cleaning up interrupted dataset: %s", datasetName)

		// Use the existing cleanup function (also cleans child datasets)
		if err := CleanupForIncrementalUpdate(datasetName, indexDir); err != nil {
			log.Printf("Warning: cleanup failed for interrupted dataset %s: %v", datasetName, err)
		}

		// Remove from state so it will be reprocessed
		state.RemoveDataset(datasetName)

		// Also remove child datasets from state
		if children, hasChildren := childDatasets[datasetName]; hasChildren {
			for _, childName := range children {
				state.RemoveDataset(childName)
			}
		}
	}

	// Save updated state
	if err := SaveDatasetState(state, indexDir); err != nil {
		log.Printf("Warning: failed to save state after cleaning interrupted datasets: %v", err)
	}

	log.Printf("Cleaned up %d interrupted datasets", len(interrupted))
	return nil
}
