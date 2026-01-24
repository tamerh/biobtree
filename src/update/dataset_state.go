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
	if dsConfig, exists := config.Dataconf[datasetName]; exists {
		if _, hasPath := dsConfig["path"]; hasPath {
			return true
		}
	}

	return false
}

// DatasetState tracks build state for incremental updates
// Stored in the main output directory as dataset_state.json
type DatasetState struct {
	LastBuildTime  time.Time                    `json:"last_build_time"`
	BuildVersion   string                       `json:"build_version"`   // Biobtree version
	TotalKVSize    uint64                       `json:"total_kv_size"`   // Total KV lines across all datasets
	Datasets       map[string]*DatasetBuildInfo `json:"datasets"`
	// DB write stats - populated after generate/merge phase completes
	DBKeysWritten   uint64 `json:"db_keys_written,omitempty"`   // Total keys written to database
	DBSpecialKeys   uint64 `json:"db_special_keys,omitempty"`   // Total special keyword/link keys
	DBValuesWritten uint64 `json:"db_values_written,omitempty"` // Total values written to database
	// Internal fields (not persisted)
	mu              sync.RWMutex    `json:"-"` // Mutex for concurrent access
	kvSizeDelta     uint64          `json:"-"` // Delta to add to TotalKVSize on save (for concurrent safety)
	deletedDatasets map[string]bool `json:"-"` // Datasets to remove on save (for concurrent safety)
}

// DatasetBuildInfo tracks build information for a single dataset
type DatasetBuildInfo struct {
	DatasetName     string    `json:"name"`
	DatasetID       string    `json:"id"`
	Status          string    `json:"status"`                     // processing, processed, or merged
	LastBuildTime   time.Time `json:"last_build_time"`
	SourceURL       string    `json:"source_url,omitempty"`       // Actual URL used for download
	SourceVersion   string    `json:"source_version,omitempty"`   // e.g., "2024.01" for UniProt release
	SourceDate      time.Time `json:"source_date,omitempty"`      // FTP file modification date
	SourceSize      int64     `json:"source_size,omitempty"`      // File size for change detection
	SourceETag      string    `json:"source_etag,omitempty"`      // HTTP ETag if available
	SourceChecksum  string    `json:"source_checksum,omitempty"`  // MD5/SHA if available
	TouchedDatasets []string  `json:"touched_datasets,omitempty"` // Datasets that received reverse xrefs
	KVSize               int64             `json:"kv_size,omitempty"`               // Number of key-value lines after deduplication
	SourceContributions  map[string]int64  `json:"source_contributions,omitempty"`  // Lines contributed by each source (forward, from_uniprot, etc.)
	XrefCount            int64             `json:"xref_count,omitempty"`            // Number of xrefs created
	BuildDuration        float64           `json:"build_duration_sec,omitempty"`    // Build time in seconds
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
	kvSizeDelta := state.kvSizeDelta
	state.kvSizeDelta = 0 // Clear delta after reading (will be added to disk value)
	// Copy and clear deleted datasets delta
	var deletedDatasets map[string]bool
	if len(state.deletedDatasets) > 0 {
		deletedDatasets = make(map[string]bool, len(state.deletedDatasets))
		for k, v := range state.deletedDatasets {
			deletedDatasets[k] = v
		}
		state.deletedDatasets = nil // Clear after reading
	}
	dbKeysWritten := state.DBKeysWritten
	dbSpecialKeys := state.DBSpecialKeys
	dbValuesWritten := state.DBValuesWritten
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

	// Merge: add/update current datasets into disk state (only source datasets + textsearch)
	for name, info := range currentDatasets {
		if IsSourceDataset(name) {
			diskState.Datasets[name] = info
		}
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

	diskState.LastBuildTime = time.Now()
	if buildVersion != "" {
		diskState.BuildVersion = buildVersion
	}
	// Merge scalar fields:
	// - total_kv_size: ADD delta to disk value (safe for concurrent processes)
	// - DB stats: OVERWRITE (these are set once at end of generate phase)
	if kvSizeDelta > 0 {
		diskState.TotalKVSize += kvSizeDelta
	}
	if dbKeysWritten > 0 {
		diskState.DBKeysWritten = dbKeysWritten
	}
	if dbSpecialKeys > 0 {
		diskState.DBSpecialKeys = dbSpecialKeys
	}
	if dbValuesWritten > 0 {
		diskState.DBValuesWritten = dbValuesWritten
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

	info.LastBuildTime = time.Now()
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
func (s *DatasetState) MarkDatasetProcessing(datasetName, datasetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}

	info, exists := s.Datasets[datasetName]
	if !exists {
		info = &DatasetBuildInfo{
			DatasetName: datasetName,
			DatasetID:   datasetID,
		}
		s.Datasets[datasetName] = info
	}

	info.Status = StatusProcessing
	info.LastBuildTime = time.Now()
}

// MarkDatasetProcessed marks a dataset as processed (bucket files written, awaiting merge)
// This is set when dataset processing completes successfully
func (s *DatasetState) MarkDatasetProcessed(datasetName, datasetID string, entryCount, xrefCount int64, duration float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Datasets == nil {
		s.Datasets = make(map[string]*DatasetBuildInfo)
	}

	info, exists := s.Datasets[datasetName]
	if !exists {
		info = &DatasetBuildInfo{
			DatasetName: datasetName,
			DatasetID:   datasetID,
		}
		s.Datasets[datasetName] = info
	}

	info.Status = StatusProcessed
	info.LastBuildTime = time.Now()
	// KVSize is set separately via SetKVSize after bucket concatenation
	// entryCount parameter is kept for API compatibility but not used
	info.XrefCount = xrefCount
	info.BuildDuration = duration
}

// MarkDatasetBuilt is deprecated - use MarkDatasetProcessed instead
// Kept for backward compatibility
func (s *DatasetState) MarkDatasetBuilt(datasetName, datasetID string, entryCount, xrefCount int64, duration float64) {
	s.MarkDatasetProcessed(datasetName, datasetID, entryCount, xrefCount, duration)
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

// SetKVSize updates the KV size for a dataset (post-deduplication line count)
// Only call this for source datasets that were explicitly requested via -d
func (s *DatasetState) SetKVSize(datasetName string, count uint64) {
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
	info.KVSize = int64(count)
}

// SetSourceContributions updates the per-source line counts for a dataset
// sourceName is "forward" for own data, or the source dataset name for reverse xrefs (e.g., "uniprot")
func (s *DatasetState) SetSourceContributions(datasetName string, contributions map[string]uint64) {
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

	// Convert uint64 to int64 for JSON compatibility
	info.SourceContributions = make(map[string]int64, len(contributions))
	for source, count := range contributions {
		info.SourceContributions[source] = int64(count)
	}
}

// AddSourceContribution adds lines to a specific source contribution (for incremental updates)
func (s *DatasetState) AddSourceContribution(datasetName, sourceName string, count uint64) {
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

	if info.SourceContributions == nil {
		info.SourceContributions = make(map[string]int64)
	}
	info.SourceContributions[sourceName] = int64(count)
}

// AddTotalKVSize records a delta to add to total KV size on next save
// This is safe for concurrent processes - each process records its delta,
// and SaveDatasetState adds it to the disk value atomically under file lock
func (s *DatasetState) AddTotalKVSize(delta uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kvSizeDelta += delta
}

// SetDBWriteStats sets the database write statistics after generate/merge completes
func (s *DatasetState) SetDBWriteStats(keysWritten, specialKeys, valuesWritten uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DBKeysWritten = keysWritten
	s.DBSpecialKeys = specialKeys
	s.DBValuesWritten = valuesWritten
}

// GetTotalKVSize returns the total KV size
func (s *DatasetState) GetTotalKVSize() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalKVSize
}

// GetKVSize returns the KV size for a dataset
func (s *DatasetState) GetKVSize(datasetName string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if info, exists := s.Datasets[datasetName]; exists {
		return info.KVSize
	}
	return 0
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
