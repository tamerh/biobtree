package generate

import (
	"biobtree/configs"
	"bufio"
	"compress/gzip"
	"container/heap"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	//"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imdario/mergo"
	"github.com/pquerna/ffjson/ffjson"

	"github.com/golang/protobuf/proto"

	"biobtree/db"
	"biobtree/pbuf"
	"biobtree/update"
	"biobtree/util"
)

// LMDB cursor operation constants (from lmdb-go)
const (
	cursorFirst = 0  // MDB_FIRST - Position at first key/data item
	cursorLast  = 6  // MDB_LAST - Position at last key/data item
	cursorNext  = 8  // MDB_NEXT - Position at next data item
)

// ============================================================================
// Worker-Based K-Way Merge Implementation
// ============================================================================
// This implementation uses a worker pool instead of per-file goroutines to
// dramatically reduce memory usage. Instead of 2558 files × 40MB = 102GB,
// we use N workers × 40MB = ~320MB (with N=8 workers).
//
// Architecture:
// - fileState: Minimal per-file state (no permanent buffer)
// - Min-heap: Efficiently finds file with smallest current key
// - Worker pool: N workers with shared buffer pool read keys on demand
// ============================================================================

// fileState holds minimal state for each file in the k-way merge
// Unlike the old chunkReader, this does NOT hold a permanent 40MB buffer
type fileState struct {
	file      *os.File
	gz        *gzip.Reader
	r         *bufio.Reader
	curKey    string
	nextLine  [6]string   // Buffered next line (key, db, value, valuedb, evidence, relationship)
	eof       bool
	complete  bool
	fileName  string
	linesRead int64
	heapIndex int         // Index in the heap (for heap.Fix)
}

// fileHeap implements heap.Interface for efficient minimum key finding
type fileHeap []*fileState

func (h fileHeap) Len() int           { return len(h) }
func (h fileHeap) Less(i, j int) bool { return h[i].curKey < h[j].curKey }
func (h fileHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}

func (h *fileHeap) Push(x interface{}) {
	n := len(*h)
	fs := x.(*fileState)
	fs.heapIndex = n
	*h = append(*h, fs)
}

func (h *fileHeap) Pop() interface{} {
	old := *h
	n := len(old)
	fs := old[n-1]
	old[n-1] = nil  // avoid memory leak
	fs.heapIndex = -1
	*h = old[0 : n-1]
	return fs
}

// readJob represents a job for a worker to read the next key from a file
type readJob struct {
	fs          *fileState
	skipUntil   string                // If non-empty, skip keys <= this value
	resultCh    chan<- *fileState     // Channel to send result back
	mergeCh     *chan kvMessage       // Channel to send kv data to
	initialRead bool                  // If true, only read first key without sending to mergeCh
}

// workerPool manages a pool of workers for reading files
type workerPool struct {
	numWorkers int
	bufferPool *sync.Pool
	bufferSize int
	jobCh      chan readJob
	wg         sync.WaitGroup
}

// newWorkerPool creates a new worker pool
func newWorkerPool(numWorkers, bufferSize int) *workerPool {
	wp := &workerPool{
		numWorkers: numWorkers,
		bufferSize: bufferSize,
		jobCh:      make(chan readJob, numWorkers*2),
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return make([]rune, bufferSize)
			},
		},
	}
	return wp
}

// start launches the worker goroutines
func (wp *workerPool) start() {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// stop shuts down the worker pool
func (wp *workerPool) stop() {
	close(wp.jobCh)
	wp.wg.Wait()
}

// worker is the main loop for a worker goroutine
func (wp *workerPool) worker() {
	defer wp.wg.Done()

	for job := range wp.jobCh {
		wp.processJob(job)
	}
}

// processJob reads the next key from a file
func (wp *workerPool) processJob(job readJob) {
	fs := job.fs

	if fs.eof {
		fs.complete = true
		job.resultCh <- fs
		return
	}

	// Borrow buffer from pool
	tmprun := wp.bufferPool.Get().([]rune)
	defer wp.bufferPool.Put(tmprun)

	if job.initialRead {
		// Initial read: only read and buffer first key, don't send to merge channel
		wp.readFirstKey(fs, tmprun, job.skipUntil)
	} else {
		// Normal read: read all lines for the next key and send to merge channel
		wp.readNextKey(fs, tmprun, job.skipUntil, job.mergeCh)
	}

	job.resultCh <- fs
}

// readFirstKey reads only the first key from a file state (for initial read)
// It buffers the first line in nextLine and sets curKey, but does NOT send to mergeCh
// This is used during initial file loading to avoid flooding the merge channel
func (wp *workerPool) readFirstKey(fs *fileState, tmprun []rune, skipUntil string) {
	var line [6]string
	var c rune
	index := 0
	tabIndex := 0
	var err error

	for {
		c, _, err = fs.r.ReadRune()

		if err != nil { // EOF
			fs.eof = true
			return
		}

		switch c {
		case newliner:
			fs.linesRead++
			line[index] = string(tmprun[:tabIndex])

			// Skip keys <= skipUntil (for resume functionality)
			if skipUntil != "" && line[0] <= skipUntil {
				// Continue reading to find a key past skipUntil
				index = 0
				tabIndex = 0
				line = [6]string{}
				continue
			}

			// Found a valid first key - buffer it and return
			fs.nextLine = line
			fs.curKey = line[0]
			return

		case tabr:
			line[index] = string(tmprun[:tabIndex])
			tabIndex = 0
			index++

		default:
			if tabIndex >= len(tmprun) {
				log.Printf("FATAL: Field exceeds buffer size limit")
				log.Printf("  Buffer size: %d, File: %s", len(tmprun), fs.fileName)
				panic(fmt.Sprintf("Buffer overflow: field size exceeds buffer size=%d", len(tmprun)))
			}
			tmprun[tabIndex] = c
			tabIndex++
		}
	}
}

// readNextKey reads all lines for the next key from a file state and sends them to mergeCh
// It stops when it encounters a different key (which is buffered in nextLine)
func (wp *workerPool) readNextKey(fs *fileState, tmprun []rune, skipUntil string, mergeCh *chan kvMessage) {
	key := ""

	// If we have a buffered line from previous read, process it first
	if len(fs.nextLine[0]) > 0 {
		key = fs.nextLine[0]
		fs.curKey = key

		// Send to merge channel if not skipping
		if skipUntil == "" || key > skipUntil {
			// Validate: db field must not be empty for valid data
			if fs.nextLine[1] == "" {
				log.Printf("ERROR: Malformed line in file %s - empty db field. key='%s' (len=%d)", fs.fileName, fs.nextLine[0][:min(50, len(fs.nextLine[0]))], len(fs.nextLine[0]))
				panic("Malformed data: empty db field in file " + fs.fileName)
			}
			*mergeCh <- kvMessage{
				key:          fs.nextLine[0],
				db:           fs.nextLine[1],
				value:        fs.nextLine[2],
				valuedb:      fs.nextLine[3],
				evidence:     fs.nextLine[4],
				relationship: fs.nextLine[5],
			}
		}

		// Clear nextLine after processing
		fs.nextLine = [6]string{}
	}

	var line [6]string
	var c rune
	index := 0
	tabIndex := 0
	var err error

	for {
		c, _, err = fs.r.ReadRune()

		if err != nil { // EOF
			fs.eof = true
			return
		}

		switch c {
		case newliner:
			fs.linesRead++
			line[index] = string(tmprun[:tabIndex])

			// If we've moved to a different key, buffer this line and return
			if len(key) > 0 && line[0] != key {
				fs.nextLine = line
				fs.curKey = line[0]  // Update curKey to the new key
				return
			}

			if len(key) == 0 {
				key = line[0]
				fs.curKey = key
			}

			// Skip keys <= skipUntil (for resume functionality)
			if skipUntil != "" && key <= skipUntil {
				// Continue reading without sending to channel
				index = 0
				tabIndex = 0
				line = [6]string{}
				continue
			}

			// Validate and send this line to merge channel
			if line[1] == "" {
				log.Printf("ERROR: Malformed line in file %s - empty db field. key='%s' (len=%d)", fs.fileName, line[0][:min(50, len(line[0]))], len(line[0]))
				panic("Malformed data: empty db field in file " + fs.fileName)
			}
			*mergeCh <- kvMessage{
				key:          line[0],
				db:           line[1],
				value:        line[2],
				valuedb:      line[3],
				evidence:     line[4],
				relationship: line[5],
			}

			// Reset for next line (still same key potentially)
			index = 0
			tabIndex = 0
			line = [6]string{}

		case tabr:
			line[index] = string(tmprun[:tabIndex])
			tabIndex = 0
			index++

		default:
			if tabIndex >= len(tmprun) {
				log.Printf("FATAL: Field exceeds buffer size limit")
				log.Printf("  Buffer size: %d, Current key: '%s', File: %s", len(tmprun), key, fs.fileName)
				panic(fmt.Sprintf("Buffer overflow: field size exceeds buffer size=%d", len(tmprun)))
			}
			tmprun[tabIndex] = c
			tabIndex++
		}
	}
}

// MergeCheckpoint stores the state needed to resume a merge operation
type MergeCheckpoint struct {
	LastWrittenKey  string                 `json:"last_written_key"`
	KeysWritten     uint64                 `json:"keys_written"`
	TotalKeyWrite   uint64                 `json:"total_key_write"`
	UidIndex        uint64                 `json:"uid_index"`
	TotalLinkKey    uint64                 `json:"total_link_key"`
	TotalKey        uint64                 `json:"total_key"`
	TotalValue      uint64                 `json:"total_value"`
	Timestamp       time.Time              `json:"timestamp"`
	IndexDir        string                 `json:"index_dir"`
	Version         int                    `json:"version"` // For future compatibility
	FileStates      map[string]FileState   `json:"file_states"` // Track state of each chunk file
}

// FileState tracks the processing state of a single chunk file
type FileState struct {
	FileName    string `json:"file_name"`
	Completed   bool   `json:"completed"`    // True if file is fully processed
	LastKey     string `json:"last_key"`     // Last key read from this file (for partial progress)
	LinesRead   int64  `json:"lines_read"`   // Number of lines read
}

// DatasetMergeStats tracks per-dataset statistics during merge
type DatasetMergeStats struct {
	Keys   uint64 // Number of unique keys written for this dataset
	Values uint64 // Number of xref values written for this dataset
}

const tabr rune = '\t'
const newliner rune = '\n'
const spacestr = " "
const eof = rune(0)

var fileBufSize = 65536

// Helper function for min of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var config *configs.Conf

type Merge struct {
	wg                      *sync.WaitGroup
	wrEnv                   db.Env
	wrDbi                   db.DBI
	mergeCh                 *chan kvMessage
	mergeTotalArrLen        int
	protoBufferArrLen       int
	totalKeyWrite           uint64
	uidIndex                uint64
	totalLinkKey            uint64
	totalKey                uint64
	totalValue              uint64
	perDatasetStats         map[uint32]*DatasetMergeStats // per-dataset statistics
	batchSize               int
	pageSize                int
	batchIndex              int
	batchKeys               [][]byte
	batchVals               [][]byte
	keepUpdateFiles         bool
	pager                   *util.Pagekey
	totalkvLine             int64
	totalEntrySize          int64                 // Sum of all entry sizes for progress based on keys written
	protoResBufferPool      *chan []*pbuf.XrefEntry
	protoCountResBufferPool *chan []*pbuf.XrefDomainCount
	// Checkpoint/resume fields
	checkpointPath          string
	checkpointInterval      int
	keysSinceCheckpoint     int
	resumeFromKey           string  // If resuming, skip keys <= this
	isResuming              bool
	lastCheckpointKey       string
	completedFiles          map[string]FileState  // Track files that completed and were removed
	checkpointFileStates    map[string]FileState  // File states from loaded checkpoint
	// Worker-based merge fields (replaces goroutine-per-file approach)
	fileStates              []*fileState          // All file states
	fileHeap                *fileHeap             // Min-heap for efficient minimum key finding
	workerPool              *workerPool           // Worker pool for reading files
	numWorkers              int                   // Number of workers (default 8)
	tmprunSize              int                   // Buffer size for reading
	lastProgressTime        time.Time             // Last time progress was logged
	progressInterval        time.Duration         // Interval between progress logs
	totalLinesRead          int64                 // Total lines read across all files for progress
}

// saveCheckpoint saves the current merge progress to a checkpoint file
func (d *Merge) saveCheckpoint(lastKey string) error {
	// Build file states from current file states (worker-based approach)
	fileStatesMap := make(map[string]FileState)
	for _, fs := range d.fileStates {
		if fs != nil {
			fileStatesMap[fs.fileName] = FileState{
				FileName:  fs.fileName,
				Completed: fs.complete,
				LastKey:   fs.curKey,
				LinesRead: fs.linesRead,
			}
		}
	}
	// Also track completed files that have been removed
	for fileName, state := range d.completedFiles {
		if _, exists := fileStatesMap[fileName]; !exists {
			fileStatesMap[fileName] = state
		}
	}

	checkpoint := MergeCheckpoint{
		LastWrittenKey: lastKey,
		KeysWritten:    uint64(d.keysSinceCheckpoint),
		TotalKeyWrite:  d.totalKeyWrite,
		UidIndex:       d.uidIndex,
		TotalLinkKey:   d.totalLinkKey,
		TotalKey:       d.totalKey,
		TotalValue:     d.totalValue,
		Timestamp:      time.Now(),
		IndexDir:       config.Appconf["indexDir"],
		Version:        1,
		FileStates:     fileStatesMap,
	}

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Write to temp file first, then rename (atomic operation)
	tmpPath := d.checkpointPath + ".tmp"
	if err := ioutil.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint temp file: %w", err)
	}
	if err := os.Rename(tmpPath, d.checkpointPath); err != nil {
		return fmt.Errorf("failed to rename checkpoint file: %w", err)
	}

	d.lastCheckpointKey = lastKey

	// Calculate total lines read from all files for progress
	d.updateTotalLinesRead()

	// Calculate progress percentage based on lines read vs total kv lines
	var progressPercent float64
	if d.totalkvLine > 0 {
		progressPercent = float64(d.totalLinesRead) / float64(d.totalkvLine) * 100
	}

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapMB := memStats.HeapAlloc / 1024 / 1024
	sysMB := memStats.Sys / 1024 / 1024

	// Count active files in heap
	activeFiles := 0
	if d.fileHeap != nil {
		activeFiles = d.fileHeap.Len()
	}

	log.Printf("Checkpoint: %.1f%% | Lines: %d/%d | Last key: %s | Files: %d | Heap: %dMB | Sys: %dMB",
		progressPercent, d.totalLinesRead, d.totalkvLine, lastKey, activeFiles, heapMB, sysMB)

	// Save heap profile every 1M keys (disabled by default - uncomment for debugging)
	// if d.totalKeyWrite % 1000000 == 0 {
	// 	profilePath := filepath.Join(filepath.Dir(d.checkpointPath), fmt.Sprintf("heap_%d.pprof", d.totalKeyWrite))
	// 	f, err := os.Create(profilePath)
	// 	if err == nil {
	// 		pprof.WriteHeapProfile(f)
	// 		f.Close()
	// 		log.Printf("Heap profile saved to: %s", profilePath)
	// 	}
	// }

	return nil
}

// logTimeBasedProgress logs progress at regular intervals (regardless of checkpoint)
func (d *Merge) logTimeBasedProgress(currentKey string) {
	if time.Since(d.lastProgressTime) < d.progressInterval {
		return
	}
	d.lastProgressTime = time.Now()

	// Calculate total lines read from all files for progress
	d.updateTotalLinesRead()

	// Calculate progress percentage based on lines read vs total kv lines
	var progressPercent float64
	if d.totalkvLine > 0 {
		progressPercent = float64(d.totalLinesRead) / float64(d.totalkvLine) * 100
	}

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapMB := memStats.HeapAlloc / 1024 / 1024

	activeFiles := 0
	if d.fileHeap != nil {
		activeFiles = d.fileHeap.Len()
	}

	log.Printf("Progress: %.1f%% | Lines: %d/%d | Current: %s | Files: %d | Mem: %dMB",
		progressPercent, d.totalLinesRead, d.totalkvLine, currentKey, activeFiles, heapMB)
}

// updateTotalLinesRead calculates total lines read from all file states
func (d *Merge) updateTotalLinesRead() {
	var total int64
	for _, fs := range d.fileStates {
		// Skip nil, completed, and EOF files (they are tracked in completedFiles)
		// Note: fs.eof is set before fs.complete, so we must check both
		if fs != nil && !fs.complete && !fs.eof {
			total += fs.linesRead
		}
	}
	// Add lines from completed files that were closed
	for _, state := range d.completedFiles {
		total += state.LinesRead
	}
	d.totalLinesRead = total
}

// loadCheckpoint loads a checkpoint file if it exists
func (d *Merge) loadCheckpoint() (*MergeCheckpoint, error) {
	data, err := ioutil.ReadFile(d.checkpointPath)
	if os.IsNotExist(err) {
		return nil, nil // No checkpoint, fresh start
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var checkpoint MergeCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	// Verify checkpoint is for the same index directory
	if checkpoint.IndexDir != config.Appconf["indexDir"] {
		log.Printf("Warning: Checkpoint index dir (%s) differs from current (%s). Starting fresh.",
			checkpoint.IndexDir, config.Appconf["indexDir"])
		return nil, nil
	}

	return &checkpoint, nil
}

// deleteCheckpoint removes the checkpoint file after successful completion
func (d *Merge) deleteCheckpoint() error {
	err := os.Remove(d.checkpointPath)
	if os.IsNotExist(err) {
		return nil // Already gone
	}
	return err
}

// getLastKeyFromDB retrieves the last key written to the database
// This is used to verify checkpoint consistency on resume
func (d *Merge) getLastKeyFromDB() (string, error) {
	var lastKey string
	err := d.wrEnv.View(func(txn db.Txn) error {
		cursor, err := txn.OpenCursor(d.wrDbi)
		if err != nil {
			return err
		}
		defer cursor.Close()

		key, _, err := cursor.Get(nil, nil, cursorLast)
		if err != nil {
			// Empty database is OK
			if db.IsNotFound(err) {
				return nil
			}
			return err
		}
		lastKey = string(key)
		return nil
	})
	return lastKey, err
}

type kvMessage struct {
	key          string
	db           string
	value        string
	valuedb      string
	evidence     string // Optional evidence code (e.g., TAS, IEA for Reactome)
	relationship string // Optional relationship type (e.g., "Related pseudogene" for Entrez gene_group)
	writekey     bool
}

func (d *Merge) Merge(c *configs.Conf, keep bool) (uint64, uint64, uint64) {

	config = c

	d.init()

	d.keepUpdateFiles = keep

	// Resume state is already set in init() if checkpoint exists
	if d.isResuming {
		log.Printf("Will skip all keys <= %s", d.resumeFromKey)
	}

	// Start the merge goroutine (processes kvMessages from channel)
	d.wg.Add(1)
	go d.mergeg()
	d.wg.Wait()

	// Start the worker pool
	d.workerPool.start()
	defer d.workerPool.stop()

	// Initial read: read first key from all files using worker pool
	// This is done in batches to control memory usage
	log.Printf("Reading initial keys from %d files using %d workers...", len(d.fileStates), d.numWorkers)
	d.initialReadAllFiles()

	// Build the min-heap from all file states
	fh := make(fileHeap, 0, len(d.fileStates))
	d.fileHeap = &fh
	for _, fs := range d.fileStates {
		if fs != nil && !fs.complete && !fs.eof {
			heap.Push(d.fileHeap, fs)
		}
	}
	log.Printf("Initial read complete. Files in heap: %d", d.fileHeap.Len())

	skippedKeys := uint64(0)

	// Main merge loop using heap-based minimum finding
	for d.fileHeap.Len() > 0 {
		// Get the minimum key from the heap (peek, don't pop yet)
		minFs := (*d.fileHeap)[0]
		minKey := minFs.curKey

		// Skip keys that were already written (resume mode)
		if d.isResuming && minKey <= d.resumeFromKey {
			// Collect all files with the min key
			filesToRead := d.collectFilesWithKey(minKey)

			// Read next keys from these files
			d.readNextKeysFromFiles(filesToRead, d.resumeFromKey)

			// Update heap for files that got new keys or completed
			d.updateHeapAfterRead(filesToRead)

			skippedKeys++
			if skippedKeys%100000 == 0 {
				log.Printf("Resume: Skipped %d keys so far, current key: %s", skippedKeys, minKey)
			}
			continue
		}

		// If we were resuming, we've now passed the checkpoint
		if d.isResuming {
			log.Printf("Resume complete: Skipped %d keys, now continuing from key: %s", skippedKeys, minKey)
			d.isResuming = false
		}

		// Collect all files with the min key
		filesToRead := d.collectFilesWithKey(minKey)

		// Read next keys from these files - this will:
		// 1. Send all data for minKey to merge channel (from the already-buffered nextLine)
		// 2. Read ahead until next key is found
		// 3. Buffer the first line of the next key
		d.readNextKeysFromFiles(filesToRead, "")

		// Send writekey message to trigger writing (after all data for this key is sent)
		*d.mergeCh <- kvMessage{
			key:      minKey,
			writekey: true,
		}

		// Update heap for files that got new keys or completed
		d.updateHeapAfterRead(filesToRead)

		// Log progress periodically (time-based, regardless of checkpoint)
		d.logTimeBasedProgress(minKey)
	}

	// Wait for merge channel to drain
	for len(*d.mergeCh) > 0 {
		time.Sleep(2 * time.Second)
	}

	close(*d.mergeCh)
	d.close()

	// Delete checkpoint on successful completion
	if err := d.deleteCheckpoint(); err != nil {
		log.Printf("Warning: Failed to delete checkpoint file: %v", err)
	}

	log.Println("Generate finished with total key:", d.totalKey, " total special keyword keys:", d.totalLinkKey, " total value:", d.totalValue)

	// Save DB write stats to dataset_state.json
	outDir := config.Appconf["outDir"]
	if state, err := update.LoadDatasetState(outDir); err == nil {
		state.SetDBWriteStats(d.totalKey, d.totalLinkKey, d.totalValue)
		// Save per-dataset DB stats
		// Convert perDatasetStats to the format expected by SetAllDatasetDBStats
		perDatasetStatsMap := make(map[uint32][2]uint64)
		for datasetID, stats := range d.perDatasetStats {
			perDatasetStatsMap[datasetID] = [2]uint64{stats.Keys, stats.Values}
		}
		state.SetAllDatasetDBStats(perDatasetStatsMap, config.DataconfIDIntToString)
		log.Printf("Per-dataset DB stats: %d datasets tracked", len(d.perDatasetStats))
		if err := update.SaveDatasetState(state, outDir); err != nil {
			log.Printf("Warning: Failed to save DB write stats to state file: %v", err)
		} else {
			log.Printf("DB write stats saved to dataset_state.json")
		}
	} else {
		log.Printf("Warning: Could not load state file to save DB stats: %v", err)
	}

	return d.totalKeyWrite, d.uidIndex, d.totalLinkKey

}

// initialReadAllFiles reads the first key from all files using the worker pool
// This is done in controlled batches to limit memory usage
func (d *Merge) initialReadAllFiles() {
	// Process files in batches to control memory
	batchSize := d.numWorkers * 4  // Process 4x workers at a time
	resultCh := make(chan *fileState, batchSize)

	for i := 0; i < len(d.fileStates); i += batchSize {
		end := i + batchSize
		if end > len(d.fileStates) {
			end = len(d.fileStates)
		}

		// Count non-nil files in this batch
		nonNilCount := 0
		// Submit batch of jobs
		for j := i; j < end; j++ {
			fs := d.fileStates[j]
			if fs == nil {
				continue
			}
			nonNilCount++
			skipUntil := ""
			if d.isResuming {
				skipUntil = d.resumeFromKey
			}
			d.workerPool.jobCh <- readJob{
				fs:          fs,
				skipUntil:   skipUntil,
				resultCh:    resultCh,
				mergeCh:     d.mergeCh,
				initialRead: true,
			}
		}

		// Collect results for this batch
		for j := 0; j < nonNilCount; j++ {
			<-resultCh
		}

		// Log progress at start, 25%, 50%, 75%, and end
		progress := float64(end) / float64(len(d.fileStates)) * 100
		if i == 0 || int(progress)%25 == 0 || end == len(d.fileStates) {
			log.Printf("Initial read progress: %d/%d files (%.0f%%)", end, len(d.fileStates), progress)
		}
	}
}

// collectFilesWithKey collects all files that have the given key as their current key
func (d *Merge) collectFilesWithKey(key string) []*fileState {
	var files []*fileState
	for _, fs := range *d.fileHeap {
		if fs.curKey == key {
			files = append(files, fs)
		}
	}
	return files
}

// readNextKeysFromFiles reads the next key from each file using the worker pool
func (d *Merge) readNextKeysFromFiles(files []*fileState, skipUntil string) {
	if len(files) == 0 {
		return
	}

	resultCh := make(chan *fileState, len(files))

	// Submit all read jobs
	for _, fs := range files {
		d.workerPool.jobCh <- readJob{
			fs:        fs,
			skipUntil: skipUntil,
			resultCh:  resultCh,
			mergeCh:   d.mergeCh,
		}
	}

	// Wait for all results
	for range files {
		<-resultCh
	}
}

// verifyHeapProperty checks if the heap property is maintained (DEBUG helper)
// Uncomment and call this after updateHeapAfterRead if debugging heap issues
// func (d *Merge) verifyHeapProperty() bool {
// 	h := *d.fileHeap
// 	valid := true
// 	for i := 0; i < len(h); i++ {
// 		if h[i].heapIndex != i {
// 			log.Printf("HEAP INDEX MISMATCH: h[%d].heapIndex=%d (file=%s curKey=%s)",
// 				i, h[i].heapIndex, h[i].fileName, h[i].curKey)
// 			valid = false
// 		}
// 		left := 2*i + 1
// 		right := 2*i + 2
// 		if left < len(h) && h[left].curKey < h[i].curKey {
// 			log.Printf("HEAP VIOLATION: h[%d]=%s > h[%d]=%s (left child)",
// 				i, h[i].curKey, left, h[left].curKey)
// 			valid = false
// 		}
// 		if right < len(h) && h[right].curKey < h[i].curKey {
// 			log.Printf("HEAP VIOLATION: h[%d]=%s > h[%d]=%s (right child)",
// 				i, h[i].curKey, right, h[right].curKey)
// 			valid = false
// 		}
// 	}
// 	return valid
// }

// updateHeapAfterRead updates the heap after reading next keys from files
// IMPORTANT: When multiple files are updated, we must handle heap operations carefully.
// The issue is that heap.Fix/Remove can rearrange the heap, making stored heapIndex values
// stale for subsequent files. Solution: process removals first (from highest index to lowest),
// then call heap.Init once to re-heapify all remaining elements.
func (d *Merge) updateHeapAfterRead(files []*fileState) {
	// Separate files into completed (to remove) and active (keys changed)
	var toRemove []*fileState
	var keysChanged []*fileState

	for _, fs := range files {
		if fs.complete || fs.eof {
			toRemove = append(toRemove, fs)
		} else {
			keysChanged = append(keysChanged, fs)
		}
	}

	// Remove completed files from heap - sort by heapIndex descending to avoid index shifting issues
	if len(toRemove) > 0 {
		// Sort by heapIndex descending so we remove from end first
		sort.Slice(toRemove, func(i, j int) bool {
			return toRemove[i].heapIndex > toRemove[j].heapIndex
		})

		for _, fs := range toRemove {
			if fs.heapIndex >= 0 && fs.heapIndex < d.fileHeap.Len() {
				heap.Remove(d.fileHeap, fs.heapIndex)
			}

			// Close file resources to release memory
			if fs.gz != nil {
				fs.gz.Close()
				fs.gz = nil
			}
			if fs.file != nil {
				fs.file.Close()
				fs.file = nil
			}
			fs.r = nil  // Allow GC to collect bufio reader

			// Track completed file
			if d.completedFiles == nil {
				d.completedFiles = make(map[string]FileState)
			}
			d.completedFiles[fs.fileName] = FileState{
				FileName:  fs.fileName,
				Completed: true,
				LastKey:   fs.curKey,
				LinesRead: fs.linesRead,
			}
			log.Printf("File completed: %s (lines read: %d)", fs.fileName, fs.linesRead)
		}
	}

	// ALWAYS re-heapify after any changes to ensure heap property is maintained.
	// This is critical because:
	// 1. heap.Remove can leave the heap in an inconsistent state when combined with
	//    subsequent operations
	// 2. heapIndex values can become stale after removals
	// 3. The cost of heap.Init (O(n)) is acceptable given the small number of files
	if len(toRemove) > 0 || len(keysChanged) > 0 {
		heap.Init(d.fileHeap)
	}
}

func (d *Merge) mergeg() {

	/**
	all := make([][]kvMessage, d.mergeTotalArrLen)
	for i := 0; i < d.mergeTotalArrLen; i++ {
		all[i] = make([]kvMessage, d.pageSize)
	}
	**/
	fullSingleArr := make([]kvMessage, d.mergeTotalArrLen*d.pageSize)
	batchSize := d.pageSize
	var all [][]kvMessage
	for batchSize < len(fullSingleArr) {
		fullSingleArr, all = fullSingleArr[batchSize:], append(all, fullSingleArr[0:batchSize:batchSize])
	}
	all = append(all, fullSingleArr)

	availables := make(chan int, d.mergeTotalArrLen*200)
	idx := 0
	for idx < d.mergeTotalArrLen {
		availables <- idx
		idx++
	}

	keyArrIds := map[string]map[string][]int{}
	keyArrIndx := map[string]map[string][]int{}

	keyPropArrIds := map[string]map[string][]int{}
	keyPropArrIndx := map[string]map[string][]int{}

	kvCounts := map[string]map[string]map[string]uint32{}

	d.wg.Done()

	for kv := range *d.mergeCh {

		if kv.writekey {
			rootResult := map[string]*[]kvMessage{}
			valueIdx := map[string]int{}
			rootPropResult := map[string]*[]kvMessage{}
			valuePropIdx := map[string]int{}

			//set key array ids
			for domain, arrIds := range keyArrIds[kv.key] {
				rootResult[domain] = &all[arrIds[0]]
				valueIdx[domain] = keyArrIndx[kv.key][domain][0]
			}

			// set attr array ids
			for domain, parrIds := range keyPropArrIds[kv.key] {
				rootPropResult[domain] = &all[parrIds[0]]
				valuePropIdx[domain] = keyPropArrIndx[kv.key][domain][0]
			}

			// set domain page info
			pageInfos := map[int]map[string]*pbuf.PageInfo{}
			for domain, arrIds := range keyArrIds[kv.key] {
				pageSize := len(arrIds) - 1 // 1 is root result
				datasetInt, err := strconv.Atoi(domain)
				if err != nil {
					log.Printf("ERROR: Invalid domain='%s' for key='%s', arrIds count=%d", domain, kv.key, len(arrIds))
					log.Printf("ERROR: kv.db='%s', kv.value='%s', kv.valuedb='%s'", kv.db, kv.value, kv.valuedb)
					log.Printf("ERROR: Full kv: %+v", kv)
					panic("dataset id to integer conversion error. Possible invalid data update input->" + kv.String())
				}
				keyLen := d.pager.KeyLen(pageSize)
				pageInfos[datasetInt] = map[string]*pbuf.PageInfo{}
				for i := 1; i < len(arrIds); i++ {

					tmpMap := map[string]bool{} // this used for in current page domain is included or not
					pageKey := d.pager.Key(i-1, keyLen)
					valIdx := keyArrIndx[kv.key][domain][i]
					for j := 0; j < valIdx; j++ {
						kv2 := all[arrIds[i]][j]
						if _, ok := tmpMap[kv2.valuedb]; !ok {
							if _, ok := pageInfos[datasetInt][kv2.valuedb]; !ok {
								pageInfos[datasetInt][kv2.valuedb] = &pbuf.PageInfo{Pages: []string{pageKey}}
							} else {
								pg := pageInfos[datasetInt][kv2.valuedb]
								pg.Pages = append(pg.Pages, pageKey)
							}
							tmpMap[kv2.valuedb] = true
						}
					}
				}
			}

			d.batchKeys[d.batchIndex] = []byte(kv.key)
			kvcounts := kvCounts[kv.key]
			d.batchVals[d.batchIndex] = d.toProtoRoot(kv.key, rootResult, valueIdx, rootPropResult, valuePropIdx, &kvcounts, pageInfos)
			d.batchIndex++

			if d.batchIndex >= d.batchSize {
				d.writeBatch()
			}

			// set pages
			// IMPORTANT: Sort domains by their encoded page key prefix to ensure
			// lexicographic ordering when writing to LMDB with Append mode.
			// Go map iteration order is non-deterministic, which can cause page keys
			// to be written out of order (e.g., "key yv a" before "key aa a").
			sortedDomains := make([]string, 0, len(keyArrIds[kv.key]))
			for domain := range keyArrIds[kv.key] {
				sortedDomains = append(sortedDomains, domain)
			}
			sort.Slice(sortedDomains, func(i, j int) bool {
				di, _ := strconv.Atoi(sortedDomains[i])
				dj, _ := strconv.Atoi(sortedDomains[j])
				return d.pager.Key(di, 2) < d.pager.Key(dj, 2)
			})

			for _, domain := range sortedDomains {
				arrIds := keyArrIds[kv.key][domain]
				pageSize := len(arrIds) - 1 // 1 is root result
				datasetInt, err := strconv.Atoi(domain)
				if err != nil {
					panic("dataset id to integer conversion error. Possible invalid data generation input->" + kv.String())
				}
				keyLen := d.pager.KeyLen(pageSize)

				for i := 1; i < len(arrIds); i++ {

					pageKey := kv.key + spacestr + d.pager.Key(datasetInt, 2) + spacestr + d.pager.Key(i-1, keyLen)
					d.batchKeys[d.batchIndex] = []byte(pageKey)
					valIdx := keyArrIndx[kv.key][domain][i]
					d.batchVals[d.batchIndex] = d.toProtoPage(pageKey, domain, &all[arrIds[i]], valIdx)
					d.batchIndex++

					if d.batchIndex >= d.batchSize {
						d.writeBatch()
					}

				}
			}

			// Clear only used elements (not entire pageSize) and return arrays to pool
			for domain, arrIds := range keyArrIds[kv.key] {
				indices := keyArrIndx[kv.key][domain]
				for i, arrayID := range arrIds {
					// Clear only the elements that were actually used
					usedCount := indices[i]
					for j := 0; j < usedCount; j++ {
						all[arrayID][j] = kvMessage{}
					}
					availables <- arrayID
				}
			}
			for domain, arrIds := range keyPropArrIds[kv.key] {
				indices := keyPropArrIndx[kv.key][domain]
				for i, arrayID := range arrIds {
					// Clear only the elements that were actually used
					usedCount := indices[i]
					for j := 0; j < usedCount; j++ {
						all[arrayID][j] = kvMessage{}
					}
					availables <- arrayID
				}
			}

			delete(keyArrIds, kv.key)
			delete(keyArrIndx, kv.key)
			delete(kvCounts, kv.key)
			if _, ok := keyPropArrIds[kv.key]; ok {
				delete(keyPropArrIds, kv.key)
			}
			if _, ok := keyPropArrIndx[kv.key]; ok {
				delete(keyPropArrIndx, kv.key)
			}

			// Checkpoint: save progress periodically
			d.keysSinceCheckpoint++
			if d.checkpointInterval > 0 && d.keysSinceCheckpoint >= d.checkpointInterval {
				if err := d.saveCheckpoint(kv.key); err != nil {
					log.Printf("Warning: Failed to save checkpoint: %v", err)
				}
				d.keysSinceCheckpoint = 0
			}

			continue
		}

		if len(availables) < 10 {
			panic("Very few available array left for merge. Define or increase 'mergeArraySize' parameter in configuration file. This will affect of using more memory. Current array size is ->" + strconv.Itoa(d.mergeTotalArrLen) + " last retrieved kv->" + kv.String())
		}

		if kv.valuedb == "-1" { //  prop value
			if _, ok := keyPropArrIds[kv.key]; !ok {

				keyPropArrIds[kv.key] = map[string][]int{}
				arrayID := <-availables
				arrIds := []int{arrayID}
				keyPropArrIds[kv.key][kv.db] = arrIds

				keyPropArrIndx[kv.key] = map[string][]int{}
				arrIdx := []int{1}
				keyPropArrIndx[kv.key][kv.db] = arrIdx

				all[arrayID][0] = kv

			} else if _, ok := keyPropArrIds[kv.key][kv.db]; !ok {

				arrayID := <-availables
				arrIds := []int{arrayID}
				keyPropArrIds[kv.key][kv.db] = arrIds

				arrIdx := []int{1}
				keyPropArrIndx[kv.key][kv.db] = arrIdx

				all[arrayID][0] = kv

			} else {

				lastArrayIDIdx := len(keyPropArrIds[kv.key][kv.db]) - 1
				arrayID := keyPropArrIds[kv.key][kv.db][lastArrayIDIdx]
				idx := keyPropArrIndx[kv.key][kv.db][lastArrayIDIdx]

				if idx == d.pageSize { // this is not supported a key has maximum can have pageSize of properties
					continue
				}

				all[arrayID][idx] = kv
				keyPropArrIndx[kv.key][kv.db][lastArrayIDIdx] = keyPropArrIndx[kv.key][kv.db][lastArrayIDIdx] + 1

			}

			continue
		}

		if _, ok := keyArrIds[kv.key]; !ok { // xref value

			keyArrIds[kv.key] = map[string][]int{}
			arrayID := <-availables
			arrIds := []int{arrayID}
			keyArrIds[kv.key][kv.db] = arrIds

			keyArrIndx[kv.key] = map[string][]int{}
			arrIdx := []int{1}
			keyArrIndx[kv.key][kv.db] = arrIdx

			all[arrayID][0] = kv

			// key value count
			kvCounts[kv.key] = map[string]map[string]uint32{}
			kvCounts[kv.key][kv.db] = map[string]uint32{}
			kvCounts[kv.key][kv.db][kv.valuedb] = 1

		} else if _, ok := keyArrIds[kv.key][kv.db]; !ok {

			arrayID := <-availables
			arrIds := []int{arrayID}
			keyArrIds[kv.key][kv.db] = arrIds

			arrIdx := []int{1}
			keyArrIndx[kv.key][kv.db] = arrIdx

			all[arrayID][0] = kv

			kvCounts[kv.key][kv.db] = map[string]uint32{}
			kvCounts[kv.key][kv.db][kv.valuedb] = 1

		} else {

			lastArrayIDIdx := len(keyArrIds[kv.key][kv.db]) - 1
			arrayID := keyArrIds[kv.key][kv.db][lastArrayIDIdx]
			idx := keyArrIndx[kv.key][kv.db][lastArrayIDIdx]

			if idx == d.pageSize { // if it is new page
				arrayID = <-availables
				idx = 0
				keyArrIds[kv.key][kv.db] = append(keyArrIds[kv.key][kv.db], arrayID)
				keyArrIndx[kv.key][kv.db] = append(keyArrIndx[kv.key][kv.db], 0)
				lastArrayIDIdx++
			}

			all[arrayID][idx] = kv
			keyArrIndx[kv.key][kv.db][lastArrayIDIdx] = keyArrIndx[kv.key][kv.db][lastArrayIDIdx] + 1

			// key value counts
			if _, ok = kvCounts[kv.key][kv.db][kv.valuedb]; !ok {
				kvCounts[kv.key][kv.db][kv.valuedb] = 1
			} else {
				kvCounts[kv.key][kv.db][kv.valuedb] = kvCounts[kv.key][kv.db][kv.valuedb] + 1
			}

		}

	}

}

func (d *Merge) init() {

	var wg sync.WaitGroup
	d.wg = &wg
	// batchsize
	var err error
	d.batchSize = 100000 // default
	if _, ok := config.Appconf["batchSize"]; ok {
		d.batchSize, err = strconv.Atoi(config.Appconf["batchSize"])
		if err != nil {
			panic("Invalid batchsize definition")
		}
	}

	// pagesize
	d.pageSize = 200 // default
	if _, ok := config.Appconf["pageSize"]; ok {
		d.pageSize, err = strconv.Atoi(config.Appconf["pageSize"])
		if err != nil {
			panic("Invalid batchsize definition")
		}
	}

	// Checkpoint configuration - must be set early before loadCheckpoint is called
	d.checkpointInterval = 100000 // Save checkpoint every 100K keys by default
	if _, ok := config.Appconf["mergeCheckpointInterval"]; ok {
		d.checkpointInterval, err = strconv.Atoi(config.Appconf["mergeCheckpointInterval"])
		if err != nil {
			panic("Invalid mergeCheckpointInterval definition")
		}
	}
	// Set checkpoint path - default to db directory (so deleting db dir also removes checkpoint)
	// Note: checkpoint is loaded BEFORE dbDir is cleared, so resume detection works correctly
	d.checkpointPath = filepath.FromSlash(config.Appconf["dbDir"] + "/merge_checkpoint.json")
	if _, ok := config.Appconf["mergeCheckpointPath"]; ok {
		d.checkpointPath = config.Appconf["mergeCheckpointPath"]
	}

	d.batchIndex = 0
	d.batchKeys = make([][]byte, d.batchSize)
	d.batchVals = make([][]byte, d.batchSize)

	d.protoBufferArrLen = 500
	if _, ok := config.Appconf["protoBufPoolSize"]; ok {
		d.protoBufferArrLen, err = strconv.Atoi(config.Appconf["protoBufPoolSize"])
		if err != nil {
			panic("Invalid batchsize definition")
		}
	}

	protoResPool := make(chan []*pbuf.XrefEntry, d.protoBufferArrLen*2)
	d.protoResBufferPool = &protoResPool
	protoCountResPool := make(chan []*pbuf.XrefDomainCount, d.protoBufferArrLen*2)
	d.protoCountResBufferPool = &protoCountResPool

	// initiliaze protobufferpools for results.
	protoPoolIndex := 0
	for protoPoolIndex < d.protoBufferArrLen {

		resultarr := make([]*pbuf.XrefEntry, d.pageSize)
		*d.protoResBufferPool <- resultarr
		countarr := make([]*pbuf.XrefDomainCount, 500) // todo this number must max unique dataset count
		*d.protoCountResBufferPool <- countarr
		protoPoolIndex++

	}

	ch := make(chan kvMessage, 10000)

	d.mergeCh = &ch

	d.pager = &util.Pagekey{}
	d.pager.Init()

	// Initialize completedFiles map
	d.completedFiles = make(map[string]FileState)

	// Initialize per-dataset statistics map
	d.perDatasetStats = make(map[uint32]*DatasetMergeStats)

	files, err := ioutil.ReadDir(config.Appconf["indexDir"])
	if err != nil {
		log.Fatal(err)
	}

	// Check for checkpoint FIRST before opening files
	checkpoint, checkpointErr := d.loadCheckpoint()
	if checkpointErr != nil {
		log.Printf("Warning: Failed to load checkpoint: %v. Starting fresh.", checkpointErr)
		checkpoint = nil
	}

	// Store file states from checkpoint for skipping completed files
	if checkpoint != nil && checkpoint.FileStates != nil {
		d.checkpointFileStates = checkpoint.FileStates
	}

	skippedFiles := 0

	// Configure tmpRuneSize (buffer size for reading)
	d.tmprunSize = 500000
	if _, ok := config.Appconf["tmpRuneSize"]; ok {
		d.tmprunSize, err = strconv.Atoi(config.Appconf["tmpRuneSize"])
		if err != nil {
			panic("Invalid tmpRuneSize definition")
		}
	}

	// Configure number of workers (default 8)
	d.numWorkers = 8
	if _, ok := config.Appconf["mergeWorkers"]; ok {
		d.numWorkers, err = strconv.Atoi(config.Appconf["mergeWorkers"])
		if err != nil {
			panic("Invalid mergeWorkers definition")
		}
	}

	// Initialize time-based progress logging (every 5 seconds)
	d.progressInterval = 5 * time.Second
	d.lastProgressTime = time.Now()

	// Create worker pool with shared buffer pool
	// This dramatically reduces memory: N workers × buffer size instead of N files × buffer size
	// With 8 workers and 40MB buffers = 320MB instead of 2558 files × 40MB = 102GB
	d.workerPool = newWorkerPool(d.numWorkers, d.tmprunSize)
	log.Printf("Worker pool initialized: %d workers, buffer size: %d runes = %d MB per worker",
		d.numWorkers, d.tmprunSize, d.tmprunSize*4/1024/1024)
	log.Printf("Maximum memory for buffers: %d MB (vs %d MB with old approach)",
		d.numWorkers*d.tmprunSize*4/1024/1024, 2558*d.tmprunSize*4/1024/1024)

	// Create file states (minimal state per file, no permanent buffer)
	var fss []*fileState
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".gz") {

			// Check if this file was already completed in checkpoint
			if d.checkpointFileStates != nil {
				if fileStateData, exists := d.checkpointFileStates[f.Name()]; exists && fileStateData.Completed {
					log.Printf("Skipping completed file: %s", f.Name())
					// Add to completedFiles so it's tracked in future checkpoints
					d.completedFiles[f.Name()] = fileStateData
					skippedFiles++
					continue
				}
			}

			path := filepath.FromSlash(config.Appconf["indexDir"] + "/" + f.Name())
			file, err := os.Open(path)
			if err != nil {
				log.Printf("Error opening file %s: %v", path, err)
				continue
			}

			gz, err := gzip.NewReader(file)
			if err == io.EOF { // zero file
				file.Close()
				continue
			}
			if err != nil {
				log.Printf("Error creating gzip reader for %s: %v", path, err)
				file.Close()
				continue
			}

			br := bufio.NewReaderSize(gz, fileBufSize)
			fss = append(fss, &fileState{
				file:      file,
				gz:        gz,
				r:         br,
				complete:  false,
				eof:       false,
				fileName:  f.Name(),
				heapIndex: -1,
			})
		}
	}
	d.fileStates = fss

	if skippedFiles > 0 {
		log.Printf("Resume: Skipped %d already-completed files, %d files to process", skippedFiles, len(fss))
	}

	// Read KV sizes from dataset_state.json in main output directory (replaces meta.json files)
	var totalkv float64
	var totalKVSize float64
	stateFile := filepath.Join(config.Appconf["outDir"], "dataset_state.json")
	if stateData, err := ioutil.ReadFile(stateFile); err == nil {
		var state map[string]interface{}
		if err := json.Unmarshal(stateData, &state); err != nil {
			log.Printf("Warning: failed to parse dataset_state.json: %v", err)
		} else {
			// Get total KV size (total KV lines across all datasets including derived)
			if total, ok := state["total_kv_size"].(float64); ok {
				totalkv = total
			}
			// Sum KV sizes from source datasets only
			if datasets, ok := state["datasets"].(map[string]interface{}); ok {
				for _, dsInfo := range datasets {
					if info, ok := dsInfo.(map[string]interface{}); ok {
						if kvSize, ok := info["kv_size"].(float64); ok {
							totalKVSize += kvSize
						}
					}
				}
			}
		}
	} else {
		log.Printf("Warning: could not read dataset_state.json: %v", err)
	}

	d.totalkvLine = int64(totalkv)
	d.totalEntrySize = int64(totalKVSize)

	if checkpoint != nil {
		// Resume mode: don't clear the database
		log.Printf("=== CHECKPOINT FOUND - WILL RESUME ===")
		log.Printf("Last written key: %s", checkpoint.LastWrittenKey)
		log.Printf("Keys written before interruption: %d", checkpoint.TotalKeyWrite)
		log.Printf("Checkpoint timestamp: %s", checkpoint.Timestamp.Format(time.RFC3339))
		log.Printf("Database will NOT be cleared")
		log.Printf("======================================")
		d.resumeFromKey = checkpoint.LastWrittenKey
		d.isResuming = true
		// Restore counters
		d.totalKeyWrite = checkpoint.TotalKeyWrite
		d.uidIndex = checkpoint.UidIndex
		d.totalLinkKey = checkpoint.TotalLinkKey
		d.totalKey = checkpoint.TotalKey
		d.totalValue = checkpoint.TotalValue
	} else {
		// Fresh start: clear the database directory
		err = os.RemoveAll(filepath.FromSlash(config.Appconf["dbDir"]))
		if err != nil {
			log.Fatal("Error cleaning the out dir check you have right permission")
			panic(err)
		}
		err = os.Mkdir(filepath.FromSlash(config.Appconf["dbDir"]), 0700)
		if err != nil {
			log.Fatal("Error creating dir", config.Appconf["dbDir"], "check you have right permission ")
			panic(err)
		}
	}

	database := db.DB{}

	d.wrEnv, d.wrDbi = database.OpenDBNew(true, d.totalkvLine, config.Appconf)

	// If resuming, verify checkpoint consistency with actual DB state
	if d.isResuming {
		dbLastKey, err := d.getLastKeyFromDB()
		if err != nil {
			log.Printf("Warning: Could not read last key from DB: %v", err)
		} else if dbLastKey != "" {
			if dbLastKey > d.resumeFromKey {
				// DB has more data than checkpoint - use DB's last key
				log.Printf("=== CHECKPOINT RECOVERY ===")
				log.Printf("Checkpoint last key: %s", d.resumeFromKey)
				log.Printf("Database last key:   %s", dbLastKey)
				log.Printf("Database is ahead of checkpoint (crash after write but before checkpoint save)")
				log.Printf("Using database last key for resume to avoid duplicate key errors")
				log.Printf("===========================")
				d.resumeFromKey = dbLastKey
			} else if dbLastKey < d.resumeFromKey {
				// Checkpoint is slightly ahead of DB - this can happen normally because:
				// 1. Checkpoint is saved after a key is processed
				// 2. But a key may generate multiple DB writes (root + pages)
				// 3. Crash could occur between checkpoint save and final page write
				// Use the MAX of both to ensure we don't re-process anything
				log.Printf("Checkpoint last key: %s", d.resumeFromKey)
				log.Printf("Database last key:   %s", dbLastKey)
				log.Printf("Using checkpoint key (checkpoint is authoritative for processed keys)")
			} else {
				log.Printf("Checkpoint and database are in sync (last key: %s)", dbLastKey)
			}
		} else {
			log.Printf("Database is empty - will start from beginning despite checkpoint")
			log.Printf("This may indicate the database was deleted. Starting fresh.")
			d.isResuming = false
			d.resumeFromKey = ""
		}
	}

	if _, ok := config.Appconf["mergeArraySize"]; ok {
		d.mergeTotalArrLen, err = strconv.Atoi(config.Appconf["mergeArraySize"])
		if err != nil {
			panic("Invalid mergeArraySize definition")
		}
	} else {
		// With sequential key processing, we need far fewer arrays than before
		// Arrays are for storing data of keys currently being processed
		// With 2558 files and sequential processing, we typically process one key at a time
		// But a single key can have data from many files and domains
		// Conservative estimate: max 50,000 arrays should be plenty
		// This reduces memory from 720K*200*80bytes = 11.5GB to 50K*200*80bytes = 800MB
		if d.totalkvLine < 100000000 { //100M
			d.mergeTotalArrLen = 10000
		} else if d.totalkvLine < 1000000000 { //1B
			d.mergeTotalArrLen = 30000
		} else {
			d.mergeTotalArrLen = 50000  // Reduced from 720000
		}
	}

	log.Printf("Merge array size: %d (memory for arrays: ~%d MB)",
		d.mergeTotalArrLen, d.mergeTotalArrLen*d.pageSize*80/1024/1024)

	log.Printf("Starting merge: %d total KV lines to process", d.totalkvLine)
}

func (d *Merge) writeBatch() {

	var failedKeyIndex int
	var failedKey string
	err := d.wrEnv.Update(func(txn db.Txn) (err error) {
		for i := 0; i < d.batchIndex; i++ {
			putErr := txn.Put(d.wrDbi, d.batchKeys[i], d.batchVals[i], 0x10) // 0x10 is lmdb.Append
			if putErr != nil {
				failedKeyIndex = i
				failedKey = string(d.batchKeys[i])
				return putErr
			}
		}
		d.totalKeyWrite = d.totalKeyWrite + uint64(d.batchIndex)
		return nil
	})
	if err != nil { // if not correctly sorted gives MDB_KEYEXIST error
		// Log detailed info about the out-of-order key
		log.Printf("ERROR: LMDB write failed - keys not in lexicographic order!")
		log.Printf("  Failed key index in batch: %d", failedKeyIndex)
		log.Printf("  Failed key: %s", failedKey)
		if failedKeyIndex > 0 {
			log.Printf("  Previous key: %s", string(d.batchKeys[failedKeyIndex-1]))
		}
		log.Printf("  Total keys written so far: %d", d.totalKeyWrite)
		log.Printf("  Error: %v", err)
		panic(fmt.Sprintf("LMDB append failed: keys not in lex order. Failed key: %s, error: %v", failedKey, err))
	}

	// Clear references to allow GC to reclaim memory
	for i := 0; i < d.batchIndex; i++ {
		d.batchKeys[i] = nil
		d.batchVals[i] = nil
	}
	d.batchIndex = 0

	/**
	d.lmdbSyncIndex = d.lmdbSyncIndex + d.batchSize
	if d.lmdbSyncIndex > 10000000 {
		d.wrEnv.Sync(true)
		d.lmdbSyncIndex = 0
	}
	**/

}

func (d *Merge) close() {

	d.writeBatch()
	d.wrEnv.Close()

	if !d.keepUpdateFiles {

		files, err := ioutil.ReadDir(filepath.FromSlash(config.Appconf["indexDir"]))

		if err == nil {

			for _, f := range files {

				if !f.IsDir() && strings.HasSuffix(f.Name(), ".gz") {

					err := os.Remove(filepath.FromSlash(config.Appconf["indexDir"] + "/" + f.Name()))
					if err != nil {
						log.Printf("Database successfully created but index files could not deleted please delete manually %v\n", err)
						break
					}

				}
			}

		} else {
			log.Printf("Database successfully created but index files could not deleted please delete manually %v\n", err)
		}

	}

	mergeStats := make(map[string]interface{})
	mergeStats["totalKey"] = d.totalKey
	mergeStats["totalValue"] = d.totalValue
	mergeStats["totalKVLine"] = d.totalkvLine
	data, err := json.Marshal(mergeStats)
	if err != nil {
		log.Printf("Database successfully created but meta file could not created %v\n", err)
		return
	}

	err = ioutil.WriteFile(filepath.FromSlash(config.Appconf["dbDir"]+"/db.meta.json"), data, 0770)

	if err != nil {
		log.Printf("Database successfully created but meta file could not created %v\n", err)
	}

}

func (k *kvMessage) String() string {
	return "key:" + k.key + " db:" + k.db + " value:" + k.value + " valuedb:" + k.valuedb
}

func (d *Merge) toProtoRoot(id string, kv map[string]*[]kvMessage, valIdx map[string]int, kvProp map[string]*[]kvMessage, valPropIdx map[string]int, kvcounts *map[string]map[string]uint32, pageInfos map[int]map[string]*pbuf.PageInfo) []byte {

	var result = pbuf.Result{}

	// Calculate total number of xrefs needed (kv entries + property-only entries)
	xrefCount := len(kv)
	if xrefCount == 0 && len(kvProp) > 0 {
		xrefCount = len(kvProp)
	}
	var xrefs = make([]*pbuf.Xref, xrefCount)

	index := 0
	var totalCount uint32

	// Keep original sizes for entriesArr and countsArr for kv loop
	entriesArr := make([][]*pbuf.XrefEntry, len(kv))
	countsArr := make([][]*pbuf.XrefDomainCount, len(kv))

	// Sort dataset keys by ID (ascending) so main datasets appear before derived datasets in pagination
	sortedDatasetKeys := make([]string, 0, len(kv))
	for k := range kv {
		sortedDatasetKeys = append(sortedDatasetKeys, k)
	}
	sort.Slice(sortedDatasetKeys, func(i, j int) bool {
		di, _ := strconv.Atoi(sortedDatasetKeys[i])
		dj, _ := strconv.Atoi(sortedDatasetKeys[j])
		return di < dj
	})

	for _, k := range sortedDatasetKeys {
		v := kv[k]

		var xref = pbuf.Xref{}
		did, err := strconv.ParseInt(k, 10, 16)
		if err != nil {
			//fmt.Println(kv)
			// this is mostly because of invalid data e.g extra tab or kvmessage broken
			panic("Error while converting to int16 for domain id->" + k)
		}
		xref.Dataset = uint32(did)

		if len(*d.protoResBufferPool) < 10 {
			panic("Very few available proto res array left. Define or increase 'protoBufPoolSize' parameter in configuration file. This will slightly effect of using more memory. Current array size is ->" + strconv.Itoa(d.protoBufferArrLen))
		}

		entries := <-*d.protoResBufferPool
		xref.Attributes = &pbuf.Xref_Empty{Empty: true}
		i := 0
		for i = 0; i < valIdx[k]; i++ {
			var xentry = pbuf.XrefEntry{}
			xentry.Identifier = (*v)[i].value
			d1, err := strconv.ParseInt((*v)[i].valuedb, 10, 16)
			if err != nil {
				panic("Error while converting to int16 ->" + (*v)[i].String())
			}
			xentry.Dataset = uint32(d1)
			xentry.Evidence = (*v)[i].evidence         // Set evidence code if present
			xentry.Relationship = (*v)[i].relationship // Set relationship type if present
			entries[i] = &xentry
		}
		entriesArr[index] = entries

		if _, ok := kvProp[k]; ok && valPropIdx[k] > 0 { // xref attributes

			// todo think again following 2 switch with interface
			switch config.DataconfIDIntToString[xref.Dataset] {
			case "uniprot":
				attr := &pbuf.UniprotAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Uniprot{attr}
			case "ufeature":
				attr := &pbuf.UniprotFeatureAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Ufeature{attr}
			case "ensembl", "transcript", "exon", "cds":
				attr := &pbuf.EnsemblAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Ensembl{attr}
			case "taxonomy":
				attr := &pbuf.TaxoAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Taxonomy{attr}
			case "hgnc":
				attr := &pbuf.HgncAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Hgnc{attr}
			case "go", "eco", "efo", "mondo", "uberon", "cl", "oba", "pato", "obi", "xco", "bao":
				attr := &pbuf.OntologyAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Ontology{attr}
			case "hpo":
				attr := &pbuf.HPOAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_HpoAttr{attr}
			case "patent":
				attr := &pbuf.PatentAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Patent{attr}
			case "clinical_trials":
				attr := &pbuf.ClinicalTrialAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_ClinicalTrials{attr}
			case "string":
				attr := &pbuf.StringAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Stringattr{attr}
			case "alphafold":
				// Merge multiple AlphaFoldAttr properties (FTP pLDDT + GCS PAE)
				if valPropIdx[k] > 1 {
					finalAttr := pbuf.AlphaFoldAttr{}
					for a := 0; a < valPropIdx[k]; a++ {
						barr := []byte((*kvProp[k])[a].value)
						attr := &pbuf.AlphaFoldAttr{}
						ffjson.Unmarshal(barr, attr)
						if err := mergo.Merge(&finalAttr, attr, mergo.WithAppendSlice); err != nil {
							panic(err)
						}
					}
					xref.Attributes = &pbuf.Xref_Alphafold{&finalAttr}
				} else {
					attr := &pbuf.AlphaFoldAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Alphafold{attr}
				}
			case "rnacentral":
				attr := &pbuf.RnacentralAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Rnacentral{attr}
			case "clinvar":
				attr := &pbuf.ClinvarAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Clinvar{attr}
			case "lipidmaps":
				attr := &pbuf.LipidmapsAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Lipidmaps{attr}
			case "swisslipids":
				attr := &pbuf.SwisslipidsAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Swisslipids{attr}
			case "bgee":
				attr := &pbuf.BgeeAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Bgee{attr}
			case "rhea":
				attr := &pbuf.RheaAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Rhea{attr}
			case "gwas_study":
				attr := &pbuf.GwasStudyAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_GwasStudy{attr}
			case "gwas":
				attr := &pbuf.GwasAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Gwas{attr}
			case "dbsnp":
				attr := &pbuf.DbsnpAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Dbsnp{attr}
			case "intact":
				attr := &pbuf.IntactAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Intact{attr}
			case "biogrid":
				attr := &pbuf.BiogridAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Biogrid{attr}
			case "biogrid_interaction":
				attr := &pbuf.BiogridInteractionAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_BiogridInteraction{attr}
			case "protein_similarity":
				attr := &pbuf.ProteinSimilarityAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_ProteinSimilarity{attr}
			case "esm2_similarity":
				attr := &pbuf.Esm2SimilarityAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Esm2Similarity{attr}
			case "antibody":
				attr := &pbuf.AntibodyAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Antibody{attr}
			case "pubchem":
				attr := &pbuf.PubchemAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Pubchem{attr}
			case "pubchem_activity":
				attr := &pbuf.PubchemActivityAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_PubchemActivity{attr}
			case "pubchem_assay":
				attr := &pbuf.PubchemAssayAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_PubchemAssay{attr}
			case "mesh":
				attr := &pbuf.MeshAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Mesh{attr}
			case "entrez":
				// Merge multiple EntrezAttr properties (gene_info + gene_neighbors)
				if valPropIdx[k] > 1 {
					finalAttr := pbuf.EntrezAttr{}
					for a := 0; a < valPropIdx[k]; a++ {
						barr := []byte((*kvProp[k])[a].value)
						attr := &pbuf.EntrezAttr{}
						ffjson.Unmarshal(barr, attr)
						if err := mergo.Merge(&finalAttr, attr, mergo.WithAppendSlice); err != nil {
							panic(err)
						}
					}
					xref.Attributes = &pbuf.Xref_Entrez{&finalAttr}
				} else {
					attr := &pbuf.EntrezAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Entrez{attr}
				}
			case "refseq":
				attr := &pbuf.RefSeqAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Refseq{attr}
			case "gencc":
				attr := &pbuf.GenccAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Gencc{attr}
			case "bindingdb":
				attr := &pbuf.BindingdbAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Bindingdb{attr}
			case "ctd":
				attr := &pbuf.CtdAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Ctd{attr}
			case "pharmgkb":
				attr := &pbuf.PharmgkbAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Pharmgkb{attr}
			case "pharmgkb_gene":
				attr := &pbuf.PharmgkbGeneAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_PharmgkbGene{attr}
			case "pharmgkb_clinical":
				attr := &pbuf.PharmgkbClinicalAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_PharmgkbClinical{attr}
			case "pharmgkb_variant":
				attr := &pbuf.PharmgkbVariantAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_PharmgkbVariant{attr}
			case "pharmgkb_guideline":
				attr := &pbuf.PharmgkbGuidelineAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_PharmgkbGuideline{attr}
			case "pharmgkb_pathway":
				attr := &pbuf.PharmgkbPathwayAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_PharmgkbPathway{attr}
			case "cellxgene":
				attr := &pbuf.CellxgeneAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Cellxgene{attr}
			case "cellxgene_celltype":
				attr := &pbuf.CellxgeneCelltypeAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_CellxgeneCelltype{attr}
			case "scxa":
				attr := &pbuf.ScxaAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Scxa{attr}
			case "scxa_expression":
				attr := &pbuf.ScxaExpressionAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_ScxaExpression{attr}
			case "scxa_gene_experiment":
				attr := &pbuf.ScxaGeneExperimentAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_ScxaGeneExperiment{attr}
			case "collectri":
				attr := &pbuf.CollecTriAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Collectri{attr}
			case "signor":
				attr := &pbuf.SignorAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Signor{attr}
			case "corum":
				attr := &pbuf.CorumAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Corum{attr}
			case "brenda":
				attr := &pbuf.BrendaAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Brenda{attr}
			case "brenda_kinetics":
				attr := &pbuf.BrendaKineticsAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_BrendaKinetics{attr}
			case "brenda_inhibitor":
				attr := &pbuf.BrendaInhibitorAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_BrendaInhibitor{attr}
			case "cellphonedb":
				attr := &pbuf.CellphonedbAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Cellphonedb{attr}
			case "drugcentral":
				attr := &pbuf.DrugcentralAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Drugcentral{attr}
			case "msigdb":
				attr := &pbuf.MsigdbAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Msigdb{attr}
			case "alphamissense":
				attr := &pbuf.AlphaMissenseAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Alphamissense{attr}
			case "alphamissense_transcript":
				attr := &pbuf.AlphaMissenseTranscriptAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_AlphamissenseTranscript{attr}
			case "interpro":
				attr := &pbuf.InterproAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Interpro{attr}
			case "ena":
				attr := &pbuf.EnaAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Ena{attr}
			case "hmdb":
				attr := &pbuf.HmdbAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Hmdb{attr}
			case "chebi":
				attr := &pbuf.ChebiAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Chebi{attr}
			case "pdb":
				attr := &pbuf.PdbAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Pdb{attr}
			case "drugbank":
				attr := &pbuf.DrugbankAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Drugbank{attr}
			case "orphanet":
				attr := &pbuf.OrphanetAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Orphanet{attr}
			case "reactome":
				attr := &pbuf.ReactomePathwayAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Reactome{attr}
			case "chembl_document", "chembl_assay", "chembl_activity", "chembl_molecule", "chembl_target", "chembl_target_component", "chembl_cell_line":

				if valPropIdx[k] > 1 {
					finalAttr := pbuf.ChemblAttr{}
					for a := 0; a < valPropIdx[k]; a++ {
						barr := []byte((*kvProp[k])[a].value)
						attr := &pbuf.ChemblAttr{}
						ffjson.Unmarshal(barr, attr)
						if err := mergo.Merge(&finalAttr, attr, mergo.WithAppendSlice); err != nil {
							panic(err)
						}
					}
					xref.Attributes = &pbuf.Xref_Chembl{&finalAttr}
				} else {
					barr := []byte((*kvProp[k])[0].value)
					attr := &pbuf.ChemblAttr{}
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Chembl{attr}
				}
			}

		}

		j := 0
		totalCount = 0

		if len(*d.protoResBufferPool) < 10 {
			panic("Very few available proto res array left. Define or increase 'protoBufPoolSize' parameter in configuration file. This will slightly effect of using more memory. Current array size is ->" + strconv.Itoa(d.protoBufferArrLen))
		}

		counts := <-*d.protoCountResBufferPool
		for x, y := range (*kvcounts)[k] {
			var xcount = pbuf.XrefDomainCount{}
			did, err := strconv.ParseInt(x, 10, 16)
			if err != nil {
				panic("Error while converting to int16 val->" + x + " key->" + id)
			}
			xcount.Dataset = uint32(did)
			xcount.Count = y
			//d.protoCountResBuffer[j] = &xcount
			counts[j] = &xcount
			totalCount = totalCount + y
			j++
		}
		countsArr[index] = counts

		xref.Entries = entries[:i]
		xref.DatasetCounts = counts[:j]
		xref.Count = totalCount
		d.totalValue = d.totalValue + uint64(totalCount)
		// Track per-dataset values
		datasetID := uint32(did)
		if d.perDatasetStats[datasetID] == nil {
			d.perDatasetStats[datasetID] = &DatasetMergeStats{}
		}
		d.perDatasetStats[datasetID].Values += uint64(totalCount)
		d.uidIndex++
		//xref.Uid = d.uidIndex
		if did == 0 {
			xref.IsLink = true
			d.totalLinkKey++
		}

		// set page infos
		if _, ok := pageInfos[int(did)]; ok {
			pageinfoFinal := map[uint32]*pbuf.PageInfo{}
			pages := map[string]bool{}
			for pdomain, v := range pageInfos[int(did)] { // this is for converting string to int
				dint, err := strconv.ParseInt(pdomain, 10, 16)
				if err != nil {
					panic("Error while converting to int16 " + pdomain)
				}
				pageinfoFinal[uint32(dint)] = v
				for _, page := range v.Pages { // todo these unqiue pages can come directly and separetely
					if _, ok := pages[page]; !ok {
						pages[page] = true
					}
				}
			}
			if len(pages) > 0 {
				pagesArr := make([]string, len(pages))
				ind := 0
				for k := range pages {
					pagesArr[ind] = k
					ind++
				}
				sort.Strings(pagesArr)
				xref.Pages = pagesArr
			}
			xref.DatasetPages = pageinfoFinal
		}

		xrefs[index] = &xref
		index++
		d.totalKey++
		// Track per-dataset keys (datasetID already set above)
		d.perDatasetStats[datasetID].Keys++
	}

	// Handle entries with only properties but no xrefs (e.g., MONDO entries)
	if len(kv) == 0 && len(kvProp) > 0 {
		for k := range kvProp {
			var xref = pbuf.Xref{}
			did, err := strconv.ParseInt(k, 10, 16)
			if err != nil {
				panic("Error while converting to int16 for domain id->" + k)
			}
			xref.Dataset = uint32(did)
			xref.Attributes = &pbuf.Xref_Empty{Empty: true}

			// Set attributes based on dataset type
			if valPropIdx[k] > 0 {
				switch config.DataconfIDIntToString[xref.Dataset] {
				case "go", "eco", "efo", "mondo", "cl", "oba", "pato", "obi", "xco", "bao":
					attr := &pbuf.OntologyAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Ontology{attr}
				case "hpo":
					attr := &pbuf.HPOAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_HpoAttr{attr}
				case "chebi":
					attr := &pbuf.ChebiAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Chebi{attr}
				case "patent":
					attr := &pbuf.PatentAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Patent{attr}
				case "clinical_trials":
					attr := &pbuf.ClinicalTrialAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_ClinicalTrials{attr}
				case "string":
					attr := &pbuf.StringAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Stringattr{attr}
				case "alphafold":
					// Merge multiple AlphaFoldAttr properties (FTP pLDDT + GCS PAE)
					if valPropIdx[k] > 1 {
						finalAttr := pbuf.AlphaFoldAttr{}
						for a := 0; a < valPropIdx[k]; a++ {
							barr := []byte((*kvProp[k])[a].value)
							attr := &pbuf.AlphaFoldAttr{}
							ffjson.Unmarshal(barr, attr)
							if err := mergo.Merge(&finalAttr, attr, mergo.WithAppendSlice); err != nil {
								panic(err)
							}
						}
						xref.Attributes = &pbuf.Xref_Alphafold{&finalAttr}
					} else {
						attr := &pbuf.AlphaFoldAttr{}
						barr := []byte((*kvProp[k])[0].value)
						ffjson.Unmarshal(barr, attr)
						xref.Attributes = &pbuf.Xref_Alphafold{attr}
					}
				case "rnacentral":
					attr := &pbuf.RnacentralAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Rnacentral{attr}
				case "clinvar":
					attr := &pbuf.ClinvarAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Clinvar{attr}
				case "lipidmaps":
					attr := &pbuf.LipidmapsAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Lipidmaps{attr}
				case "swisslipids":
					attr := &pbuf.SwisslipidsAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Swisslipids{attr}
				case "rhea":
					attr := &pbuf.RheaAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Rhea{attr}
				case "gwas_study":
					attr := &pbuf.GwasStudyAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_GwasStudy{attr}
				case "gwas":
					attr := &pbuf.GwasAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Gwas{attr}
				case "dbsnp":
					attr := &pbuf.DbsnpAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Dbsnp{attr}
				case "intact":
					attr := &pbuf.IntactAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Intact{attr}
				case "biogrid":
					attr := &pbuf.BiogridAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Biogrid{attr}
				case "biogrid_interaction":
					attr := &pbuf.BiogridInteractionAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_BiogridInteraction{attr}
				case "protein_similarity":
					attr := &pbuf.ProteinSimilarityAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_ProteinSimilarity{attr}
				case "esm2_similarity":
					attr := &pbuf.Esm2SimilarityAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Esm2Similarity{attr}
				case "antibody":
					attr := &pbuf.AntibodyAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Antibody{attr}
				case "pubchem":
					attr := &pbuf.PubchemAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Pubchem{attr}
				case "pubchem_activity":
					attr := &pbuf.PubchemActivityAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_PubchemActivity{attr}
				case "pubchem_assay":
					attr := &pbuf.PubchemAssayAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_PubchemAssay{attr}
				case "mesh":
					attr := &pbuf.MeshAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Mesh{attr}
				case "entrez":
					// Merge multiple EntrezAttr properties (gene_info + gene_neighbors)
					if valPropIdx[k] > 1 {
						finalAttr := pbuf.EntrezAttr{}
						for a := 0; a < valPropIdx[k]; a++ {
							barr := []byte((*kvProp[k])[a].value)
							attr := &pbuf.EntrezAttr{}
							ffjson.Unmarshal(barr, attr)
							if err := mergo.Merge(&finalAttr, attr, mergo.WithAppendSlice); err != nil {
								panic(err)
							}
						}
						xref.Attributes = &pbuf.Xref_Entrez{&finalAttr}
					} else {
						attr := &pbuf.EntrezAttr{}
						barr := []byte((*kvProp[k])[0].value)
						ffjson.Unmarshal(barr, attr)
						xref.Attributes = &pbuf.Xref_Entrez{attr}
					}
				case "refseq":
					attr := &pbuf.RefSeqAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Refseq{attr}
				case "gencc":
					attr := &pbuf.GenccAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Gencc{attr}
				case "bindingdb":
					attr := &pbuf.BindingdbAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Bindingdb{attr}
				case "ctd":
					attr := &pbuf.CtdAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Ctd{attr}
				case "pharmgkb":
					attr := &pbuf.PharmgkbAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Pharmgkb{attr}
				case "pharmgkb_gene":
					attr := &pbuf.PharmgkbGeneAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_PharmgkbGene{attr}
				case "pharmgkb_clinical":
					attr := &pbuf.PharmgkbClinicalAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_PharmgkbClinical{attr}
				case "pharmgkb_variant":
					attr := &pbuf.PharmgkbVariantAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_PharmgkbVariant{attr}
				case "pharmgkb_guideline":
					attr := &pbuf.PharmgkbGuidelineAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_PharmgkbGuideline{attr}
				case "pharmgkb_pathway":
					attr := &pbuf.PharmgkbPathwayAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_PharmgkbPathway{attr}
				case "cellxgene":
					attr := &pbuf.CellxgeneAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Cellxgene{attr}
				case "cellxgene_celltype":
					attr := &pbuf.CellxgeneCelltypeAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_CellxgeneCelltype{attr}
				case "scxa":
					attr := &pbuf.ScxaAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Scxa{attr}
				case "scxa_expression":
					attr := &pbuf.ScxaExpressionAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_ScxaExpression{attr}
				case "scxa_gene_experiment":
					attr := &pbuf.ScxaGeneExperimentAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_ScxaGeneExperiment{attr}
				case "collectri":
					attr := &pbuf.CollecTriAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Collectri{attr}
				case "signor":
					attr := &pbuf.SignorAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Signor{attr}
				case "cellphonedb":
					attr := &pbuf.CellphonedbAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Cellphonedb{attr}
				case "drugcentral":
					attr := &pbuf.DrugcentralAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Drugcentral{attr}
				case "msigdb":
					attr := &pbuf.MsigdbAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Msigdb{attr}
				case "alphamissense":
					attr := &pbuf.AlphaMissenseAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Alphamissense{attr}
				case "alphamissense_transcript":
					attr := &pbuf.AlphaMissenseTranscriptAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_AlphamissenseTranscript{attr}
				}
			}

			// No entries or counts for property-only entries
			xref.Entries = []*pbuf.XrefEntry{}
			xref.DatasetCounts = []*pbuf.XrefDomainCount{}
			xref.Count = 0

			xrefs[index] = &xref
			index++
			d.totalKey++
			// Track per-dataset keys for property-only entries
			datasetID := uint32(did)
			if d.perDatasetStats[datasetID] == nil {
				d.perDatasetStats[datasetID] = &DatasetMergeStats{}
			}
			d.perDatasetStats[datasetID].Keys++
		}
	}

	//result.Identifier = id
	result.Results = xrefs
	data, err := proto.Marshal(&result)
	if err != nil {
		panic(err)
	}

	for _, arr := range entriesArr {
		*d.protoResBufferPool <- arr
	}
	for _, arr := range countsArr {
		*d.protoCountResBufferPool <- arr
	}

	return data

}

func (d *Merge) toProtoPage(id string, dataset string, v *[]kvMessage, valIdx int) []byte {

	var result = pbuf.Result{}
	var xrefs [1]*pbuf.Xref

	var totalCount uint32
	var xref = pbuf.Xref{}

	//var entries = make([]*pbuf.XrefEntry, len(v))
	i := 0
	entries := <-*d.protoResBufferPool

	if len(*d.protoResBufferPool) < 10 {
		panic("Very few available proto res array left. Define or increase 'protoBufPoolSize' parameter in configuration file. This will slightly effect of using more memory. Current array size is ->" + strconv.Itoa(d.protoBufferArrLen))
	}

	for i = 0; i < valIdx; i++ {
		var xentry = pbuf.XrefEntry{}
		xentry.Identifier = (*v)[i].value
		d1, err := strconv.ParseInt((*v)[i].valuedb, 10, 16)
		if err != nil {
			panic("Error while converting to int16 ->" + (*v)[i].valuedb)
		}
		xentry.Dataset = uint32(d1)
		xentry.Evidence = (*v)[i].evidence         // Set evidence code if present
		xentry.Relationship = (*v)[i].relationship // Set relationship type if present
		entries[i] = &xentry
		totalCount++
	}

	xref.Entries = entries[:i]
	xref.Count = totalCount
	d.totalValue = d.totalValue + uint64(totalCount)
	did, err := strconv.ParseInt(dataset, 10, 16)
	if err != nil {
		panic("Error while converting to int16 ->" + dataset)
	}
	// Track per-dataset values for paginated entries
	datasetID := uint32(did)
	if d.perDatasetStats[datasetID] == nil {
		d.perDatasetStats[datasetID] = &DatasetMergeStats{}
	}
	d.perDatasetStats[datasetID].Values += uint64(totalCount)
	xref.Dataset = uint32(did)
	xrefs[0] = &xref

	//	result.Identifier = id
	result.Results = xrefs[:]
	data, err := proto.Marshal(&result)
	if err != nil {
		panic(err)
	}

	*d.protoResBufferPool <- entries

	return data

}

func check(err error) {

	if err != nil {
		fmt.Println("Error: ", err)
		panic(err)
	}

}
