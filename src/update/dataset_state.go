package update

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// IsSourceDataset returns true if the dataset is a source dataset (has path in config)
// or is the special textsearch derived dataset which should be tracked.
// Source datasets come from source1.dataset.json and source2.dataset.json.
// Derived datasets (from xref1.dataset.json etc.) are excluded from state tracking.
func IsSourceDataset(datasetName string) bool {
	// Special case: textsearch is a derived dataset but should be tracked
	if strings.HasPrefix(datasetName, "textsearch") {
		return true
	}

	// Check if config is initialized (may be nil during early startup)
	if config == nil || config.Dataconf == nil {
		return true // If config not loaded yet, allow all datasets
	}

	// Check if dataset has a path (source datasets have download paths)
	// Some datasets use path variants like path_product1, path_product4, etc.
	if dsConfig, exists := config.Dataconf[datasetName]; exists {
		for key := range dsConfig {
			if key == "path" || strings.HasPrefix(key, "path_") {
				return true
			}
		}
	}

	return false
}

// FederationDBStats holds database write statistics for a single federation
type FederationDBStats struct {
	KeysWritten   uint64 `json:"keys_written"`   // Total keys written to database
	SpecialKeys   uint64 `json:"special_keys"`   // Total special keyword/link keys
	ValuesWritten uint64 `json:"values_written"` // Total values written to database
}

// DatasetState tracks build state for incremental updates
// Stored in the main output directory as dataset_state.json
type DatasetState struct {
	LastBuildTime time.Time                    `json:"last_build_time"`
	BuildVersion  string                       `json:"build_version"` // Biobtree version
	Datasets      map[string]*DatasetBuildInfo `json:"datasets"`
	// DB write stats per federation - populated after generate/merge phase completes
	DBStats map[string]*FederationDBStats `json:"db_stats,omitempty"` // federation name -> stats
	// Internal fields (not persisted)
	mu              sync.RWMutex    `json:"-"` // Mutex for concurrent access
	deletedDatasets map[string]bool `json:"-"` // Datasets to remove on save (for concurrent safety)
}

// DatasetBuildInfo tracks build information for a single dataset
type DatasetBuildInfo struct {
	DatasetName   string    `json:"name"`
	Federation    string    `json:"federation,omitempty"` // Federation this dataset belongs to (default: "main")
	Status        string    `json:"status"`               // processing, processed, or merged
	LastBuildTime time.Time `json:"last_build_time"`
	// Edge tracking fields
	ForwardEdges map[string]int64 `json:"forward_edges,omitempty"` // Edges from source data (target -> count, "entry" for properties)
	ReverseEdges map[string]int64 `json:"reverse_edges,omitempty"` // Edges written to other datasets (target -> count)
	TotalEdges   int64            `json:"total_edges,omitempty"`   // sum(ForwardEdges) + sum(ReverseEdges)
	// Derived views of the two counters above. These disambiguate what
	// "total_edges" means; see docs/internal/dataset-statistics.md.
	Properties  int64 `json:"properties,omitempty"`   // forward rows for "entry" (node payload, not edges)
	XrefsOut    int64 `json:"xrefs_out,omitempty"`    // entity-to-entity xrefs (forward rows minus properties)
	IndexEdges  int64 `json:"index_edges,omitempty"`  // text/interval index edges (reverse "textsearch" rows)
	StorageRows int64 `json:"storage_rows,omitempty"` // sum(forward)+sum(reverse), alias of TotalEdges
	// Source metadata
	SourceURL      string    `json:"source_url,omitempty"`      // Actual URL used for download
	SourceVersion  string    `json:"source_version,omitempty"`  // e.g., "2024.01" for UniProt release
	SourceDate     time.Time `json:"source_date,omitempty"`     // FTP file modification date
	SourceSize     int64     `json:"source_size,omitempty"`     // File size for change detection
	SourceETag     string    `json:"source_etag,omitempty"`     // HTTP ETag if available
	SourceChecksum string    `json:"source_checksum,omitempty"` // MD5/SHA if available
	// Build metadata
	TouchedDatasets []string `json:"touched_datasets,omitempty"` // Datasets that received reverse xrefs

	ProcessDuration     string `json:"process_duration,omitempty"`      // Processing/parsing phase duration (e.g., "1m30s")
	PostProcessDuration string `json:"post_process_duration,omitempty"` // Sort/concatenation phase duration (e.g., "45s")
	// DB write stats - populated after generate/merge phase completes
	DBKeys   int64 `json:"db_keys,omitempty"`   // Keys written to database for this dataset
	DBValues int64 `json:"db_values,omitempty"` // Values/xrefs written to database for this dataset
}

// DatasetStateFileName is the default state file name
const DatasetStateFileName = "dataset_state.json"

// Dataset status constants for two-phase state tracking
const (
	// StatusProcessing - dataset is currently being processed (bucket files being written)
	// If found on startup, the dataset was interrupted and needs re-processing
	StatusProcessing = "processing"

	// StatusProcessed - dataset processing complete, bucket files written, awaiting merge
	// If found on startup, skip processing but include in merge
	StatusProcessed = "processed"

	// StatusMerged - dataset fully complete, data merged into final index
	// If found on startup and source unchanged, skip entirely
	StatusMerged = "merged"
)

// NewDatasetState creates a new empty state
func NewDatasetState() *DatasetState {
	return &DatasetState{
		Datasets: make(map[string]*DatasetBuildInfo),
	}
}

// LoadDatasetState reads state from JSON file in the main output directory
// Returns empty state if file doesn't exist (first build)
func LoadDatasetState(outDir string) (*DatasetState, error) {
	statePath := filepath.Join(outDir, DatasetStateFileName)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// First build - return empty state
			log.Printf("No existing dataset state found at %s, starting fresh", statePath)
			return NewDatasetState(), nil
		}
		return nil, err
	}

	state := NewDatasetState()
	if err := json.Unmarshal(data, state); err != nil {
		log.Printf("Warning: Failed to parse dataset state, starting fresh: %v", err)
		return NewDatasetState(), nil
	}

	// Migrate old state format: check for datasets with zero TotalEdges
	// Old format used kv_size and source_contributions which are now removed
	// The next build will populate forward_edges/reverse_edges/total_edges correctly
	oldFormatCount := 0
	for _, info := range state.Datasets {
		// Backfill the derived views (properties/xrefs_out/index_edges/
		// storage_rows) for state written by older binaries.
		state.recalculateTotal(info)
		if info.TotalEdges == 0 && len(info.ForwardEdges) == 0 {
			oldFormatCount++
		}
	}
	if oldFormatCount > 0 {
		log.Printf("Note: %d datasets have old state format (missing total_edges). They will be updated on next build.", oldFormatCount)
	}

	log.Printf("Loaded dataset state: %d datasets, last build: %s",
		len(state.Datasets), state.LastBuildTime.Format(time.RFC3339))

	return state, nil
}

// SaveDatasetState writes state to JSON file in the main output directory with merge support for parallel runs
// When multiple biobtree instances run in parallel with different datasets,
// this function merges the current state with any existing state on disk
// to avoid overwriting other instances' entries
func SaveDatasetState(state *DatasetState, outDir string) error {
	state.mu.Lock()
	// Make DEEP copies to avoid race conditions where another goroutine
	// modifies the shared DatasetBuildInfo objects after we release the lock
	currentDatasets := make(map[string]*DatasetBuildInfo)
	for k, v := range state.Datasets {
		// Create a copy of the struct, not just the pointer
		infoCopy := *v
		currentDatasets[k] = &infoCopy
	}
	buildVersion := state.BuildVersion
	// Copy and clear deleted datasets delta
	var deletedDatasets map[string]bool
	if len(state.deletedDatasets) > 0 {
		deletedDatasets = make(map[string]bool, len(state.deletedDatasets))
		for k, v := range state.deletedDatasets {
			deletedDatasets[k] = v
		}
		state.deletedDatasets = nil // Clear after reading
	}
	// Copy DBStats map for thread-safe access
	var dbStats map[string]*FederationDBStats
	if len(state.DBStats) > 0 {
		dbStats = make(map[string]*FederationDBStats, len(state.DBStats))
		for fed, stats := range state.DBStats {
			dbStats[fed] = &FederationDBStats{
				KeysWritten:   stats.KeysWritten,
				SpecialKeys:   stats.SpecialKeys,
				ValuesWritten: stats.ValuesWritten,
			}
		}
	}
	state.mu.Unlock()

	statePath := filepath.Join(outDir, DatasetStateFileName)
	lockPath := statePath + ".lock"

	// Acquire file lock for safe parallel access
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %v", err)
	}
	defer func() {
		lockFile.Close()
		os.Remove(lockPath) // Clean up lock file
	}()

	// Use flock-style locking via exclusive file lock
	if err := acquireFileLock(lockFile); err != nil {
		return fmt.Errorf("failed to acquire lock: %v", err)
	}
	defer releaseFileLock(lockFile)

	// Re-read existing state from disk (may have been updated by another instance)
	var diskState *DatasetState
	if data, err := os.ReadFile(statePath); err == nil {
		diskState = NewDatasetState()
		if err := json.Unmarshal(data, diskState); err != nil {
			log.Printf("Warning: Failed to parse existing state, will overwrite: %v", err)
			diskState = NewDatasetState()
		}
	} else {
		diskState = NewDatasetState()
	}
	// Backfill derived views so the completeness check below always has them,
	// even when reading state written by an older binary.
	for _, info := range diskState.Datasets {
		diskState.recalculateTotal(info)
	}

	// Merge: add/update current datasets into disk state (only source datasets + textsearch)
	// COMPLETENESS REGRESSION CHECK. A large unexplained drop in a dataset's edge/
	// record count vs the previous build almost always means a SILENTLY truncated or
	// incomplete source — e.g. the 2026-07 ClinVar case where a network stall cut the
	// download to ~40% (4.5M -> 1.8M variants) and it merged partial data without an
	// error. We flag any refreshed source that shrank >30% so it can't ship unnoticed.
	var completenessWarnings []string
	var emptyCollapses []string
	for name, info := range currentDatasets {
		if IsSourceDataset(name) {
			// Compare forward rows (properties + entity xrefs) rather than
			// storage_rows: the latter double counts xrefs and includes text and
			// interval index rows, which can mask a real loss of source records.
			oldRows := forwardRows(diskState.Datasets[name])
			newRows := forwardRows(info)
			if oldRows > 1000 {
				if newRows == 0 {
					// A source that previously had substantial data now produces NOTHING.
					// Almost always a moved/empty/failed download (e.g. a redirect stub, a
					// 200-with-empty-body, or a truncation to zero) — NOT a real change. We
					// refuse to overwrite good data with nothing: record it and abort below,
					// BEFORE the new (empty) state is written, so the on-disk state and the
					// active DB are preserved.
					emptyCollapses = append(emptyCollapses, fmt.Sprintf("%s (%d rows -> 0)", name, oldRows))
					continue // do NOT overwrite this dataset's good state in memory
				}
				if ratio := float64(newRows) / float64(oldRows); ratio < 0.7 {
					drop := (1 - ratio) * 100
					log.Printf("⚠️  COMPLETENESS WARNING [%s]: forward rows %d -> %d (-%.0f%%) vs previous build — possible truncated/incomplete source; VERIFY before shipping", name, oldRows, newRows, drop)
					completenessWarnings = append(completenessWarnings, fmt.Sprintf("%s (-%.0f%%: %d->%d)", name, drop, oldRows, newRows))
				}
			}
			diskState.Datasets[name] = info
		}
	}
	// Hard stop on empty-source collapse: abort before persisting state, so the
	// update exits non-zero -> bb.sh skips generate and the live DB is never
	// clobbered with empty data. Fix the source and re-run (--force) to recover.
	if len(emptyCollapses) > 0 {
		log.Fatalf("❌ EMPTY-SOURCE ABORT: %d dataset(s) built to ZERO edges from a previously-populated source — refusing to save/ship empty data (likely a moved/empty/failed download): %s", len(emptyCollapses), strings.Join(emptyCollapses, "; "))
	}
	if len(completenessWarnings) > 0 {
		log.Printf("⚠️⚠️  COMPLETENESS: %d dataset(s) shrank >30%% vs previous build — REVIEW for truncated/incomplete downloads before activating: %s", len(completenessWarnings), strings.Join(completenessWarnings, "; "))
	}

	// Filter out any non-source datasets that might exist in disk state
	// This ensures derived datasets are removed even if they were added in previous versions
	for name := range diskState.Datasets {
		if !IsSourceDataset(name) {
			delete(diskState.Datasets, name)
		}
	}

	// Apply deletions (delta-based for concurrent safety)
	// This happens AFTER merge so deletions take precedence
	for name := range deletedDatasets {
		delete(diskState.Datasets, name)
	}

	diskState.LastBuildTime = time.Now().Truncate(time.Second)
	if buildVersion != "" {
		diskState.BuildVersion = buildVersion
	}
	// Merge DB stats per federation (OVERWRITE - these are set once at end of generate phase)
	if len(dbStats) > 0 {
		if diskState.DBStats == nil {
			diskState.DBStats = make(map[string]*FederationDBStats)
		}
		for fed, stats := range dbStats {
			diskState.DBStats[fed] = stats
		}
	}

	// Marshal merged state
	data, err := json.MarshalIndent(diskState, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then rename (atomic)
	tempPath := statePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tempPath, statePath); err != nil {
		os.Remove(tempPath)
		return err
	}

	log.Printf("Saved dataset state to %s (%d datasets, merged with existing)", statePath, len(diskState.Datasets))
	return nil
}

// acquireFileLock acquires an exclusive lock on the file
func acquireFileLock(f *os.File) error {
	// Try to acquire lock with retries
	for i := 0; i < 30; i++ { // Max 30 seconds wait
		// Try exclusive lock (non-blocking first)
		err := tryLock(f)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout waiting for file lock")
}

// releaseFileLock releases the file lock
func releaseFileLock(f *os.File) error {
	return unlockFile(f)
}

// GetDatasetInfo returns build info for a dataset, or nil if not found
func (s *DatasetState) GetDatasetInfo(datasetName string) *DatasetBuildInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Datasets[datasetName]
}

// UpdateDatasetInfo updates or creates build info for a dataset
func (s *DatasetState) UpdateDatasetInfo(info *DatasetBuildInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}

	info.LastBuildTime = time.Now().Truncate(time.Second)
	s.Datasets[info.DatasetName] = info
}

// RemoveDatasetInfo removes build info for a dataset
// Note: This is for in-memory removal only. For concurrent-safe removal that
// persists across saves, use MarkDatasetForDeletion instead.
func (s *DatasetState) RemoveDatasetInfo(datasetName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.Datasets, datasetName)
}

// GetTouchedDatasets returns all datasets that received reverse xrefs from the given dataset
// These datasets need to be re-sorted when the source dataset is updated
func (s *DatasetState) GetTouchedDatasets(datasetName string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, exists := s.Datasets[datasetName]
	if !exists {
		return nil
	}
	return info.TouchedDatasets
}

// SetTouchedDatasets sets the list of datasets that received reverse xrefs
func (s *DatasetState) SetTouchedDatasets(datasetName string, touched []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if info, exists := s.Datasets[datasetName]; exists {
		info.TouchedDatasets = touched
	}
}

// GetAllTouchedBy returns all datasets that are touched by updates to the given dataset
// Includes the dataset itself and all datasets that receive its reverse xrefs
func (s *DatasetState) GetAllTouchedBy(datasetName string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	touched := []string{datasetName}
	if info, exists := s.Datasets[datasetName]; exists {
		touched = append(touched, info.TouchedDatasets...)
	}
	return touched
}

// GetDatasetsNeedingUpdate returns datasets that need updating based on source changes
// This is used during incremental update to determine which datasets to rebuild
func (s *DatasetState) GetDatasetsNeedingUpdate(changedDatasets []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Use map to track unique datasets
	needUpdate := make(map[string]bool)

	for _, changed := range changedDatasets {
		needUpdate[changed] = true

		// Also include all datasets that receive reverse xrefs from this one
		if info, exists := s.Datasets[changed]; exists {
			for _, touched := range info.TouchedDatasets {
				needUpdate[touched] = true
			}
		}
	}

	result := make([]string, 0, len(needUpdate))
	for ds := range needUpdate {
		result = append(result, ds)
	}
	return result
}

// MarkDatasetProcessing marks a dataset as currently being processed
// This is set when processing starts - if found on next run, dataset was interrupted
func (s *DatasetState) MarkDatasetProcessing(datasetName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}

	info, exists := s.Datasets[datasetName]
	if !exists {
		info = &DatasetBuildInfo{
			DatasetName: datasetName,
		}
		s.Datasets[datasetName] = info
	}

	info.Status = StatusProcessing
	info.LastBuildTime = time.Now().Truncate(time.Second)
}

// formatDuration formats a duration as human-readable string without decimals (e.g., "1h3m30s", "45s")
func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d == 0 {
		return "0s"
	}
	return d.String()
}

// MarkDatasetProcessed marks a dataset as processed (bucket files written, awaiting merge)
// This is set when dataset processing completes successfully
func (s *DatasetState) MarkDatasetProcessed(datasetName string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}

	info, exists := s.Datasets[datasetName]
	if !exists {
		info = &DatasetBuildInfo{
			DatasetName: datasetName,
		}
		s.Datasets[datasetName] = info
	}

	info.Status = StatusProcessed
	info.LastBuildTime = time.Now().Truncate(time.Second)
	info.ProcessDuration = formatDuration(duration)
}

// MarkDatasetBuilt is deprecated - use MarkDatasetProcessed instead
// Kept for backward compatibility
func (s *DatasetState) MarkDatasetBuilt(datasetName string, duration time.Duration) {
	s.MarkDatasetProcessed(datasetName, duration)
}

// SetPostProcessDuration sets the sort/concatenation phase duration for a dataset
func (s *DatasetState) SetPostProcessDuration(datasetName string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if info, exists := s.Datasets[datasetName]; exists {
		info.PostProcessDuration = formatDuration(duration)
	}
}

// MarkDatasetsMerged marks "processed" datasets as "merged"
// This should be called after successful merge completion
// Note: Does NOT mark "processing" datasets - those were interrupted and need cleanup
func (s *DatasetState) MarkDatasetsMerged(datasetNames []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mergedCount := 0
	for _, name := range datasetNames {
		if info, exists := s.Datasets[name]; exists {
			// Only mark "processed" as "merged" - "processing" means interrupted
			if info.Status == StatusProcessed {
				info.Status = StatusMerged
				mergedCount++
			}
		}
	}
	log.Printf("Marked %d datasets as merged", mergedCount)
}

// MarkAllProcessedAsMerged marks all datasets with "processed" status as "merged"
// This should be called after successful merge completion
func (s *DatasetState) MarkAllProcessedAsMerged() {
	s.mu.Lock()
	defer s.mu.Unlock()

	mergedCount := 0
	for _, info := range s.Datasets {
		if info.Status == StatusProcessed {
			info.Status = StatusMerged
			mergedCount++
		}
	}
	log.Printf("Marked %d datasets as merged", mergedCount)
}

// GetDatasetStatus returns the status of a dataset
func (s *DatasetState) GetDatasetStatus(datasetName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if info, exists := s.Datasets[datasetName]; exists {
		return info.Status
	}
	return ""
}

// GetProcessedDatasets returns all datasets with "processed" status
// These need to be included in merge even if processing is skipped
func (s *DatasetState) GetProcessedDatasets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []string
	for name, info := range s.Datasets {
		if info.Status == StatusProcessed {
			result = append(result, name)
		}
	}
	return result
}

// NeedsProcessing returns true if dataset needs processing
// Returns true if: status is "processing" (interrupted), status is empty (new), or not in state
func (s *DatasetState) NeedsProcessing(datasetName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, exists := s.Datasets[datasetName]
	if !exists {
		return true // New dataset
	}

	// If status is "processing", it was interrupted - needs re-processing
	// If status is empty (old state file), treat as needing processing
	return info.Status == StatusProcessing || info.Status == ""
}

// NeedsMergeOnly returns true if dataset was processed but not yet merged
func (s *DatasetState) NeedsMergeOnly(datasetName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if info, exists := s.Datasets[datasetName]; exists {
		return info.Status == StatusProcessed
	}
	return false
}

// GetInterruptedDatasets returns all datasets with "processing" status
// These datasets were interrupted mid-build and need cleanup before next run
func (s *DatasetState) GetInterruptedDatasets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var interrupted []string
	for name, info := range s.Datasets {
		if info.Status == StatusProcessing {
			interrupted = append(interrupted, name)
		}
	}
	return interrupted
}

// RemoveDataset marks a dataset for removal from state (used after cleanup of interrupted datasets)
// This is concurrent-safe: the actual deletion from disk state happens in SaveDatasetState
// under file lock, ensuring parallel processes don't re-add removed datasets.
func (s *DatasetState) RemoveDataset(datasetName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from in-memory state
	delete(s.Datasets, datasetName)

	// Mark for deletion on save (delta-based approach for concurrent safety)
	if s.deletedDatasets == nil {
		s.deletedDatasets = make(map[string]bool)
	}
	s.deletedDatasets[datasetName] = true
}

// SetDBWriteStats sets the database write statistics for a federation after generate/merge completes
func (s *DatasetState) SetDBWriteStats(federation string, keysWritten, specialKeys, valuesWritten uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.DBStats == nil {
		s.DBStats = make(map[string]*FederationDBStats)
	}
	s.DBStats[federation] = &FederationDBStats{
		KeysWritten:   keysWritten,
		SpecialKeys:   specialKeys,
		ValuesWritten: valuesWritten,
	}
}

// SetDatasetDBStats sets the per-dataset database write statistics after generate/merge completes
// datasetID is the numeric dataset ID (as used in the database), keys and values are the counts
func (s *DatasetState) SetDatasetDBStats(datasetName string, keys, values uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}

	info, exists := s.Datasets[datasetName]
	if !exists {
		info = &DatasetBuildInfo{
			DatasetName: datasetName,
		}
		s.Datasets[datasetName] = info
	}
	info.DBKeys = int64(keys)
	info.DBValues = int64(values)
}

// SetAllDatasetDBStats sets the per-dataset database write statistics for all datasets
// perDatasetStats is a map of dataset ID (uint32) to {keys, values}
// datasetIDToName is a map converting dataset IDs to names (from config.DataconfIDIntToString)
// Only updates existing dataset entries - does NOT create new entries for derived datasets
func (s *DatasetState) SetAllDatasetDBStats(federation string, perDatasetStats map[uint32][2]uint64, datasetIDToName map[uint32]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		return // No datasets to update
	}

	seen := make(map[string]bool, len(perDatasetStats))
	for datasetID, stats := range perDatasetStats {
		// Skip dataset ID 0 (textsearch/link dataset - not a regular dataset)
		if datasetID == 0 {
			continue
		}

		// Convert dataset ID to name
		datasetName, exists := datasetIDToName[datasetID]
		if !exists {
			continue // Skip unknown dataset IDs
		}

		// Only update existing entries - don't create new ones for derived datasets
		info, exists := s.Datasets[datasetName]
		if !exists {
			continue // Skip datasets not already tracked (derived datasets)
		}
		info.DBKeys = int64(stats[0])
		info.DBValues = int64(stats[1])
		seen[datasetName] = true
	}

	// A dataset in this federation that contributed no rows must be reset, not
	// left with the counts from a previous generate. Without this, a dataset
	// that shrinks to zero keeps stale db_keys/db_values forever.
	zeroed := 0
	for datasetName, info := range s.Datasets {
		if seen[datasetName] {
			continue
		}
		fed := info.Federation
		if fed == "" {
			fed = "main"
		}
		if fed != federation {
			continue
		}
		if info.DBKeys != 0 || info.DBValues != 0 {
			zeroed++
		}
		info.DBKeys = 0
		info.DBValues = 0
	}
	if zeroed > 0 {
		log.Printf("Reset DB stats for %d dataset(s) in federation '%s' that wrote no rows this generate", zeroed, federation)
	}
}

// getOrCreateDataset is a helper to get or create a dataset entry (caller must hold lock)
func (s *DatasetState) getOrCreateDataset(name string) *DatasetBuildInfo {
	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}
	info, exists := s.Datasets[name]
	if !exists {
		info = &DatasetBuildInfo{DatasetName: name}
		s.Datasets[name] = info
	}
	return info
}

// forwardRows is the stable record count used by the completeness check:
// entry properties plus entity-to-entity xrefs. It excludes text/interval index
// rows and the reverse copies, so it does not shrink or grow for reasons
// unrelated to the source data.
func forwardRows(info *DatasetBuildInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Properties + info.XrefsOut
}

// recalculateTotal updates TotalEdges and the derived views from the two raw
// counters. TotalEdges/storage_rows count both storage directions, so they are
// row counts, not logical edge counts; the distinction is intentional.
func (s *DatasetState) recalculateTotal(info *DatasetBuildInfo) {
	var forward, reverse int64
	for _, count := range info.ForwardEdges {
		forward += count
	}
	for _, count := range info.ReverseEdges {
		reverse += count
	}

	info.TotalEdges = forward + reverse
	info.StorageRows = info.TotalEdges
	info.Properties = info.ForwardEdges["entry"]
	info.XrefsOut = forward - info.Properties
	info.IndexEdges = info.ReverseEdges["textsearch"]
}

// SetForwardEdges sets the forward edge counts for a dataset (target -> count)
// "entry" key represents dataset ID -1 (properties/attributes)
func (s *DatasetState) SetForwardEdges(datasetName string, forwardEdges map[string]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info := s.getOrCreateDataset(datasetName)
	info.ForwardEdges = forwardEdges
	s.recalculateTotal(info)
}

// SetReverseEdges sets reverse edge counts for all targets for a dataset
func (s *DatasetState) SetReverseEdges(datasetName string, reverseEdges map[string]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info := s.getOrCreateDataset(datasetName)
	info.ReverseEdges = reverseEdges
	s.recalculateTotal(info)
}

// AddReverseEdges adds/updates reverse edge count for a specific target
func (s *DatasetState) AddReverseEdges(datasetName, targetName string, count int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info := s.getOrCreateDataset(datasetName)
	if info.ReverseEdges == nil {
		info.ReverseEdges = make(map[string]int64)
	}
	info.ReverseEdges[targetName] += count
	s.recalculateTotal(info)
}

// GetTotalEdges returns the total edges for a dataset
func (s *DatasetState) GetTotalEdges(datasetName string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if info, exists := s.Datasets[datasetName]; exists {
		return info.TotalEdges
	}
	return 0
}

// GetTotalEdgesForFederation returns sum of TotalEdges for all datasets in federation
func (s *DatasetState) GetTotalEdgesForFederation(federation string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var total int64
	for _, info := range s.Datasets {
		fed := info.Federation
		if fed == "" {
			fed = "main"
		}
		if fed == federation {
			total += info.TotalEdges
		}
	}
	return total
}

// SetFederation sets the federation for a dataset
func (s *DatasetState) SetFederation(datasetName, federation string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}

	info, exists := s.Datasets[datasetName]
	if !exists {
		info = &DatasetBuildInfo{
			DatasetName: datasetName,
		}
		s.Datasets[datasetName] = info
	}
	info.Federation = federation
}

// GetFederation returns the federation for a dataset (defaults to "main" if not set)
func (s *DatasetState) GetFederation(datasetName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if info, exists := s.Datasets[datasetName]; exists {
		if info.Federation != "" {
			return info.Federation
		}
	}
	return "main"
}

// GetDatasetsForFederation returns all dataset names belonging to a specific federation
func (s *DatasetState) GetDatasetsForFederation(federation string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []string
	for name, info := range s.Datasets {
		dsFed := info.Federation
		if dsFed == "" {
			dsFed = "main"
		}
		if dsFed == federation {
			result = append(result, name)
		}
	}
	return result
}

// GetLastBuildTime returns the last build time for a dataset
// Returns zero time if dataset was never built
func (s *DatasetState) GetLastBuildTime(datasetName string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if info, exists := s.Datasets[datasetName]; exists {
		return info.LastBuildTime
	}
	return time.Time{}
}

// WasBuiltAfter returns true if the dataset was built after the given time
func (s *DatasetState) WasBuiltAfter(datasetName string, t time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if info, exists := s.Datasets[datasetName]; exists {
		return info.LastBuildTime.After(t)
	}
	return false
}

// Summary returns a summary of the state for logging
func (s *DatasetState) Summary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return fmt.Sprintf("DatasetState: %d datasets, last build: %s",
		len(s.Datasets), s.LastBuildTime.Format(time.RFC3339))
}

// tryLock attempts to acquire an exclusive lock on the file (Unix implementation)
func tryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlockFile releases the lock on the file (Unix implementation)
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
