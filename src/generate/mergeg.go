package generate

import (
	"biobtree/configs"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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
	"biobtree/util"
)

// LMDB cursor operation constants (from lmdb-go)
const (
	cursorFirst = 0  // MDB_FIRST - Position at first key/data item
	cursorLast  = 6  // MDB_LAST - Position at last key/data item
	cursorNext  = 8  // MDB_NEXT - Position at next data item
)

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
	chunkReaders            []*chunkReader
	mergeCh                 *chan kvMessage
	mergeTotalArrLen        int
	protoBufferArrLen       int
	totalKeyWrite           uint64
	uidIndex                uint64
	totalLinkKey            uint64
	totalKey                uint64
	totalValue              uint64
	batchSize               int
	pageSize                int
	batchIndex              int
	batchKeys               [][]byte
	batchVals               [][]byte
	keepUpdateFiles         bool
	pager                   *util.Pagekey
	totalkvLine             int64
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
}

type chunkReader struct {
	d         *Merge
	file      *os.File
	r         *bufio.Reader
	curKey    string
	complete  bool
	eof       bool
	tmprun    []rune
	nextLine  [5]string  // Extended to support evidence field (optional 5th field)
	wg        *sync.WaitGroup
	active    bool
	fileName  string     // Name of the chunk file (for tracking)
	linesRead int64      // Number of lines read from this file
}

// saveCheckpoint saves the current merge progress to a checkpoint file
func (d *Merge) saveCheckpoint(lastKey string) error {
	// Build file states from current chunk readers
	fileStates := make(map[string]FileState)
	for _, ch := range d.chunkReaders {
		fileStates[ch.fileName] = FileState{
			FileName:  ch.fileName,
			Completed: ch.complete,
			LastKey:   ch.curKey,
			LinesRead: ch.linesRead,
		}
	}
	// Also track completed files that have been removed from chunkReaders
	for fileName, state := range d.completedFiles {
		if _, exists := fileStates[fileName]; !exists {
			fileStates[fileName] = state
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
		FileStates:     fileStates,
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

	// Calculate progress percentage
	var progressPercent float64
	if d.totalkvLine > 0 {
		// Count total lines read across all files (active + completed)
		var totalLinesRead int64
		for _, ch := range d.chunkReaders {
			totalLinesRead += ch.linesRead
		}
		for _, state := range d.completedFiles {
			totalLinesRead += state.LinesRead
		}
		progressPercent = float64(totalLinesRead) / float64(d.totalkvLine) * 100
	}

	log.Printf("Progress: %.1f%% | Keys written: %d | Last key: %s | Active files: %d",
		progressPercent, d.totalKeyWrite, lastKey, len(d.chunkReaders))
	return nil
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
	key      string
	db       string
	value    string
	valuedb  string
	evidence string  // Optional evidence code (e.g., TAS, IEA for Reactome)
	writekey bool
}

func (d *Merge) Merge(c *configs.Conf, keep bool) (uint64, uint64, uint64) {

	config = c

	d.init()

	d.keepUpdateFiles = keep

	// Resume state is already set in init() if checkpoint exists
	if d.isResuming {
		log.Printf("Will skip all keys <= %s", d.resumeFromKey)
	}

	d.wg.Add(1)
	go d.mergeg()
	d.wg.Wait()

	// Initial read with skip support if resuming
	for _, ch := range d.chunkReaders {
		d.wg.Add(1)
		if d.isResuming {
			go ch.readKeyValueWithSkip(d.resumeFromKey)
		} else {
			go ch.readKeyValue()
		}
	}
	d.wg.Wait()

	d.removeFinished()

	skippedKeys := uint64(0)

	for len(d.chunkReaders) > 0 {
		// Find minimum key using simple linear scan (faster for small k)
		minKey := d.chunkReaders[0].curKey
		for _, ch := range d.chunkReaders[1:] {
			if ch.curKey < minKey {
				minKey = ch.curKey
			}
		}

		// Skip keys that were already written (resume mode)
		if d.isResuming && minKey <= d.resumeFromKey {
			// Advance readers with min key without writing
			for _, ch := range d.chunkReaders {
				if ch.curKey == minKey {
					d.wg.Add(1)
					go ch.readKeyValueWithSkip(d.resumeFromKey)
				}
			}
			d.wg.Wait()
			d.removeFinished()

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

		// Send writekey message
		*d.mergeCh <- kvMessage{
			key:      minKey,
			writekey: true,
		}

		// Advance all readers that have this key
		for _, ch := range d.chunkReaders {
			if ch.curKey == minKey {
				d.wg.Add(1)
				go ch.readKeyValue()
			}
		}
		d.wg.Wait()

		d.removeFinished()
	}

	for { // to wait last batch to finish
		if len(*d.mergeCh) > 0 {
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}

	close(*d.mergeCh)
	d.close()

	// Delete checkpoint on successful completion
	if err := d.deleteCheckpoint(); err != nil {
		log.Printf("Warning: Failed to delete checkpoint file: %v", err)
	}

	log.Println("Generate finished with total key:", d.totalKey, " total special keyword keys:", d.totalLinkKey, " total value:", d.totalValue)
	return d.totalKeyWrite, d.uidIndex, d.totalLinkKey

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
			for domain, arrIds := range keyArrIds[kv.key] {
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

			for _, v := range keyArrIds[kv.key] {
				for _, arrayID := range v {

					for i := 0; i < d.pageSize; i++ {
						var emptymes kvMessage
						all[arrayID][i] = emptymes
					}

					availables <- arrayID
				}
			}
			for _, v := range keyPropArrIds[kv.key] {
				for _, arrayID := range v {

					for i := 0; i < d.pageSize; i++ {
						var emptymes kvMessage
						all[arrayID][i] = emptymes
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

func (d *Merge) removeFinished() {
	var finishedReaders []*chunkReader
	for _, ch := range d.chunkReaders {
		if ch.complete {
			finishedReaders = append(finishedReaders, ch)
			// Track completed file for checkpoint
			if d.completedFiles == nil {
				d.completedFiles = make(map[string]FileState)
			}
			d.completedFiles[ch.fileName] = FileState{
				FileName:  ch.fileName,
				Completed: true,
				LastKey:   ch.curKey,
				LinesRead: ch.linesRead,
			}
			log.Printf("File completed: %s (lines read: %d)", ch.fileName, ch.linesRead)
		}
	}

	if len(finishedReaders) > 0 {
		// More efficient O(n) removal using filter-in-place
		writeIdx := 0
		for _, rd := range d.chunkReaders {
			if !rd.complete {
				d.chunkReaders[writeIdx] = rd
				writeIdx++
			}
		}
		d.chunkReaders = d.chunkReaders[:writeIdx]
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

	var cr []*chunkReader
	skippedFiles := 0

	tmpRuneSize := 500000
	if _, ok := config.Appconf["tmpRuneSize"]; ok {
		tmpRuneSize, err = strconv.Atoi(config.Appconf["tmpRuneSize"])
		if err != nil {
			panic("Invalid tmpRuneSize definition")
		}
	}

	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".gz") {

			// Check if this file was already completed in checkpoint
			if d.checkpointFileStates != nil {
				if fileState, exists := d.checkpointFileStates[f.Name()]; exists && fileState.Completed {
					log.Printf("Skipping completed file: %s", f.Name())
					// Add to completedFiles so it's tracked in future checkpoints
					d.completedFiles[f.Name()] = fileState
					skippedFiles++
					continue
				}
			}

			path := filepath.FromSlash(config.Appconf["indexDir"] + "/" + f.Name())
			file, err := os.Open(path)
			gz, err := gzip.NewReader(file)

			if err == io.EOF { //zero file
				continue
			}

			check(err)
			br := bufio.NewReaderSize(gz, fileBufSize)
			cr = append(cr, &chunkReader{
				r:        br,
				complete: false,
				tmprun:   make([]rune, tmpRuneSize),
				wg:       d.wg,
				d:        d,
				file:     file,
				fileName: f.Name(),
			})

		}
	}
	d.chunkReaders = cr

	if skippedFiles > 0 {
		log.Printf("Resume: Skipped %d already-completed files, %d files to process", skippedFiles, len(cr))
	}

	var totalkv float64
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".meta.json") {
			meta := make(map[string]interface{})
			f, err := ioutil.ReadFile(config.Appconf["indexDir"] + "/" + f.Name())
			if err != nil {
				fmt.Printf("Error: %v", err)
				os.Exit(1)
			}

			if err := json.Unmarshal(f, &meta); err != nil {
				panic(err)
			}
			totalkv = totalkv + meta["totalKV"].(float64)

		}
	}

	d.totalkvLine = int64(totalkv)

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
	} else { // estimate the size of array
		if d.totalkvLine < 100000000 { //100M
			d.mergeTotalArrLen = 20000
		} else if d.totalkvLine < 200000000 { //200M
			d.mergeTotalArrLen = 30000
		} else if d.totalkvLine < 1000000000 { //1B
			d.mergeTotalArrLen = 100000
		} else { // todo review again
			d.mergeTotalArrLen = 720000
		}

	}

	log.Printf("Starting merge: %d total KV lines to process", d.totalkvLine)
}

func (d *Merge) writeBatch() {

	err := d.wrEnv.Update(func(txn db.Txn) (err error) {
		i := 0
		for i = 0; i < d.batchIndex; i++ { // todo missing error check??
			txn.Put(d.wrDbi, d.batchKeys[i], d.batchVals[i], 0x10) // 0x10 is lmdb.Append
		}
		d.totalKeyWrite = d.totalKeyWrite + uint64(i)
		return err
	})
	if err != nil { // if not correctly sorted gives MDB_KEYEXIST error
		panic(err)
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

var totalLine = 0

func (ch *chunkReader) readKeyValue() {

	defer ch.wg.Done()

	if ch.eof {
		ch.complete = true
		return
	}

	key := ""
	if len(ch.nextLine[0]) > 0 {
		key = ch.nextLine[0]
		ch.curKey = key
		//ch.newDomainKey(ch.nextLine[1], ch.nextLine[2], ch.nextLine[3])

		*ch.d.mergeCh <- kvMessage{
			key:      ch.nextLine[0],
			db:       ch.nextLine[1],
			value:    ch.nextLine[2],
			valuedb:  ch.nextLine[3],
			evidence: ch.nextLine[4], // Optional evidence field
		}

	}

	var line [5]string  // Extended to support optional evidence field
	var c rune
	index := 0
	tabIndex := 0
	lineIndex := 0
	var err error

	for {

		c, _, err = ch.r.ReadRune()
		lineIndex++

		if err != nil { // this is eof
			ch.eof = true
			return
		}

		switch c {

		case newliner:
			ch.linesRead++  // Track lines read for checkpoint

			line[index] = string(ch.tmprun[:tabIndex])

			/*
				totalLine++
				if totalLine > 3000000 {
					fmt.Println(line)
					//fmt.Println(string(ch.tmprun))
					//fmt.Println("tabindex", tabIndex)
					//fmt.Println("tmplen", len(ch.tmprun))
					//fmt.Println("inde", index)
					fmt.Println(len(*ch.d.mergeCh))
				}*/

			if len(key) > 0 && line[0] != key {
				ch.nextLine = line
				return
			}

			if len(key) == 0 { //our key
				key = line[0]
				ch.curKey = key
			}

			*ch.d.mergeCh <- kvMessage{
				key:      line[0],
				db:       line[1],
				value:    line[2],
				valuedb:  line[3],
				evidence: line[4], // Include evidence field (optional 5th field)
			}

			index = 0
			tabIndex = 0
			lineIndex = 0
			break
		case tabr:
			line[index] = string(ch.tmprun[:tabIndex])
			tabIndex = 0
			index++
			break

		default:
			// Log warning for unusually large fields (before hitting the limit)
			// This helps identify potential issues early
			bufferSize := len(ch.tmprun)
			if tabIndex == bufferSize/2 || tabIndex == bufferSize*3/4 || tabIndex == bufferSize*9/10 {
				log.Printf("WARNING: Large field detected")
				log.Printf("  Field size: %d characters (%.1f%% of buffer)", tabIndex, float64(tabIndex)/float64(bufferSize)*100)
				log.Printf("  Buffer size: %d characters", bufferSize)
				log.Printf("  Current key: '%s'", key)
				log.Printf("  Field index: %d", index)
				log.Printf("  Field content (first 200 chars): %s...", string(ch.tmprun[:min(200, tabIndex)]))
				log.Printf("  File: %s", ch.file.Name())
			}

			// Bounds check to prevent buffer overflow and provide debugging info
			if tabIndex >= bufferSize {
				// Log detailed information for debugging before panicking
				log.Printf("FATAL: Field exceeds buffer size limit")
				log.Printf("  Buffer size (tmpRuneSize): %d characters", bufferSize)
				log.Printf("  Current tabIndex: %d", tabIndex)
				log.Printf("  Current key: '%s'", key)
				log.Printf("  Field index: %d", index)
				log.Printf("  Current field content (first 500 chars): %s...", string(ch.tmprun[:min(500, bufferSize)]))
				log.Printf("  File: %s", ch.file.Name())
				log.Printf("")
				log.Printf("  This usually happens when a field (key, value, or xref) is exceptionally large.")
				log.Printf("  To fix: Increase tmpRuneSize in conf/application.param.json")
				log.Printf("  Suggested value: \"tmpRuneSize\": \"10000000\" (10M characters)")
				panic(fmt.Sprintf("Buffer overflow: field size exceeds tmpRuneSize=%d", bufferSize))
			}
			ch.tmprun[tabIndex] = c
			tabIndex++
		}

	}

}

// readKeyValueWithSkip reads key-values but skips all data until we pass skipUntil key
// This is used for resume functionality - we need to advance through the file
// without sending data to the merge channel
func (ch *chunkReader) readKeyValueWithSkip(skipUntil string) {

	defer ch.wg.Done()

	if ch.eof {
		ch.complete = true
		return
	}

	key := ""
	// If we have a buffered line from previous read
	if len(ch.nextLine[0]) > 0 {
		key = ch.nextLine[0]
		ch.curKey = key

		// Skip sending to channel if key <= skipUntil
		if key > skipUntil {
			*ch.d.mergeCh <- kvMessage{
				key:      ch.nextLine[0],
				db:       ch.nextLine[1],
				value:    ch.nextLine[2],
				valuedb:  ch.nextLine[3],
				evidence: ch.nextLine[4],
			}
		}
	}

	var line [5]string
	var c rune
	index := 0
	tabIndex := 0
	var err error

	for {
		c, _, err = ch.r.ReadRune()

		if err != nil { // EOF
			ch.eof = true
			return
		}

		switch c {
		case newliner:
			ch.linesRead++  // Track lines read for checkpoint
			line[index] = string(ch.tmprun[:tabIndex])

			// If we've moved to a different key
			if len(key) > 0 && line[0] != key {
				ch.nextLine = line
				return
			}

			if len(key) == 0 {
				key = line[0]
				ch.curKey = key
			}

			// Only send to channel if we're past the skip point
			if key > skipUntil {
				*ch.d.mergeCh <- kvMessage{
					key:      line[0],
					db:       line[1],
					value:    line[2],
					valuedb:  line[3],
					evidence: line[4],
				}
			}

			index = 0
			tabIndex = 0

		case tabr:
			line[index] = string(ch.tmprun[:tabIndex])
			tabIndex = 0
			index++

		default:
			bufferSize := len(ch.tmprun)
			if tabIndex >= bufferSize {
				log.Printf("FATAL: Field exceeds buffer size limit during skip")
				panic(fmt.Sprintf("Buffer overflow: field size exceeds tmpRuneSize=%d", bufferSize))
			}
			ch.tmprun[tabIndex] = c
			tabIndex++
		}
	}
}

func (k *kvMessage) String() string {
	return "key:" + k.key + " db:" + k.db + " value:" + k.value + " valuedb" + k.valuedb
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

	for k, v := range kv {

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
			xentry.Evidence = (*v)[i].evidence  // Set evidence code if present
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
			case "go", "eco", "efo", "mondo", "hpo", "uberon", "cl":
				attr := &pbuf.OntologyAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Ontology{attr}
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
				attr := &pbuf.AlphaFoldAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_Alphafold{attr}
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
			case "protein_similarity":
				attr := &pbuf.ProteinSimilarityAttr{}
				barr := []byte((*kvProp[k])[0].value)
				ffjson.Unmarshal(barr, attr)
				xref.Attributes = &pbuf.Xref_ProteinSimilarity{attr}
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
				case "go", "eco", "efo", "mondo", "hpo", "cl":
					attr := &pbuf.OntologyAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Ontology{attr}
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
					attr := &pbuf.AlphaFoldAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_Alphafold{attr}
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
				case "protein_similarity":
					attr := &pbuf.ProteinSimilarityAttr{}
					barr := []byte((*kvProp[k])[0].value)
					ffjson.Unmarshal(barr, attr)
					xref.Attributes = &pbuf.Xref_ProteinSimilarity{attr}
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
				}
			}

			// No entries or counts for property-only entries
			xref.Entries = []*pbuf.XrefEntry{}
			xref.DatasetCounts = []*pbuf.XrefDomainCount{}
			xref.Count = 0

			xrefs[index] = &xref
			index++
			d.totalKey++
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
		xentry.Evidence = (*v)[i].evidence  // Set evidence code if present
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
