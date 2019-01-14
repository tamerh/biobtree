package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

	"./pbuf"
	"github.com/bmatsuo/lmdb-go/lmdb"
	pb "gopkg.in/cheggaaa/pb.v1"
)

const tabr rune = '\t'
const newliner rune = '\n'
const spacestr = " "
const eof = rune(0)

var mergebar *pb.ProgressBar

type merge struct {
	wg                      *sync.WaitGroup
	wrEnv                   *lmdb.Env
	wrDbi                   lmdb.DBI
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
	pager                   *pagekey
	totalkvLine             int64
	protoResBufferPool      *chan []*pbuf.XrefEntry
	protoCountResBufferPool *chan []*pbuf.XrefDomainCount
}

type chunkReader struct {
	d        *merge
	fileName string
	r        *bufio.Reader
	curKey   string
	complete bool
	eof      bool
	tmprun   []rune
	nextLine [4]string
	wg       *sync.WaitGroup
	active   bool
}

type kvMessage struct {
	key      string
	db       string
	value    string
	valuedb  string
	writekey bool
}

func (d *merge) Merge() (uint64, uint64, uint64) {

	d.init()

	d.wg.Add(1)
	go d.mergeg()
	d.wg.Wait()

	for _, ch := range d.chunkReaders {
		d.wg.Add(1)
		go ch.readKeyValue()
	}
	d.wg.Wait()

	activecount := 0
	minKey := ""

	for {

		activecount = 0
		minKey = ""
		for _, ch := range d.chunkReaders {
			if len(minKey) == 0 || ch.curKey < minKey {
				minKey = ch.curKey
			}
		}

		for _, ch := range d.chunkReaders {
			if ch.curKey == minKey {
				ch.active = true
			} else {
				ch.active = false
			}
		}

		*d.mergeCh <- kvMessage{
			key:      minKey,
			writekey: true,
		}

		for _, ch := range d.chunkReaders {
			if ch.active {
				d.wg.Add(1)
				go ch.readKeyValue()
				activecount++
			}
		}
		if activecount > 0 {
			d.wg.Wait()
		}

		d.removeFinished()
		if len(d.chunkReaders) == 0 {
			break
		}

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
	mergebar.Update()
	mergebar.Finish()
	log.Println("Generate finished with total key:", d.totalKey, " total special keyword keys:", d.totalLinkKey, " total value:", d.totalValue)
	return d.totalKeyWrite, d.uidIndex, d.totalLinkKey

}

func (d *merge) mergeg() {

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
	kvCounts := map[string]map[string]map[string]uint32{}

	d.wg.Done()

	for kv := range *d.mergeCh {

		if kv.writekey {

			rootResult := map[string]*[]kvMessage{}
			valueIdx := map[string]int{}

			for domain, arrIds := range keyArrIds[kv.key] {
				rootResult[domain] = &all[arrIds[0]]
				valueIdx[domain] = keyArrIndx[kv.key][domain][0]
			}

			d.batchKeys[d.batchIndex] = []byte(kv.key)
			kvcounts := kvCounts[kv.key]
			d.batchVals[d.batchIndex] = d.toProtoRoot(kv.key, rootResult, valueIdx, &kvcounts)
			d.batchIndex++

			if d.batchIndex >= d.batchSize {
				d.writeBatch()
			}

			for domain, arrIds := range keyArrIds[kv.key] {
				pageSize := len(arrIds) - 1 // 1 is root result
				datasetInt, err := strconv.Atoi(domain)
				if err != nil {
					panic("dataset id to integer conversion error. Possible invalid data generation")
				}
				keyLen := d.pager.keyLen(pageSize)

				for i := 1; i < len(arrIds); i++ {

					pageKey := kv.key + spacestr + d.pager.key(datasetInt, 2) + spacestr + d.pager.key(i-1, keyLen)
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

			delete(keyArrIds, kv.key)
			delete(keyArrIndx, kv.key)
			delete(kvCounts, kv.key)

			continue
		}

		if len(availables) < 10 {
			panic("Very few available array left for merge. Define or increase 'mergeArraySize' parameter in configuration file. This will affect of using more memory. Current array size is ->" + strconv.Itoa(d.mergeTotalArrLen))
		}

		if _, ok := keyArrIds[kv.key]; !ok {

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

func (d *merge) removeFinished() {

	var finishedReaders []*chunkReader
	for _, ch := range d.chunkReaders {
		if ch.complete {
			finishedReaders = append(finishedReaders, ch)
		}
	}

	if len(finishedReaders) > 0 {

		var updatedReaders []*chunkReader
		for _, rd := range d.chunkReaders {
			exlcuded := false
			for _, rd2 := range finishedReaders {
				if rd.r == rd2.r {
					exlcuded = true
				}
			}
			if !exlcuded {
				updatedReaders = append(updatedReaders, rd)
			} else {
				rd = nil
			}
		}
		d.chunkReaders = updatedReaders
	}

}

func (d *merge) init() {

	// batchsize
	var err error
	d.batchSize = 100000 // default
	if _, ok := appconf["batchSize"]; ok {
		d.batchSize, err = strconv.Atoi(appconf["batchSize"])
		if err != nil {
			panic("Invalid batchsize definition")
		}
	}

	// pagesize
	d.pageSize = 200 // default
	if _, ok := appconf["pageSize"]; ok {
		d.pageSize, err = strconv.Atoi(appconf["pageSize"])
		if err != nil {
			panic("Invalid batchsize definition")
		}
	}

	d.batchIndex = 0
	d.batchKeys = make([][]byte, d.batchSize)
	d.batchVals = make([][]byte, d.batchSize)

	d.protoBufferArrLen = 500
	if _, ok := appconf["protoBufPoolSize"]; ok {
		d.protoBufferArrLen, err = strconv.Atoi(appconf["protoBufPoolSize"])
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

	d.pager = &pagekey{}
	d.pager.init()

	files, err := ioutil.ReadDir(appconf["indexDir"])
	if err != nil {
		log.Fatal(err)
	}

	var cr []*chunkReader

	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".gz") {
			file, err := os.Open(filepath.FromSlash(appconf["indexDir"] + "/" + f.Name()))
			gz, err := gzip.NewReader(file)
			if err == io.EOF { //zero file
				continue
			}
			check(err)
			br := bufio.NewReaderSize(gz, fileBufSize)
			cr = append(cr, &chunkReader{
				r:        br,
				complete: false,
				tmprun:   make([]rune, 500),
				wg:       d.wg,
				d:        d,
			})
			//todo
			//defer gz.Close()
			//defer file.Close()
		}
	}
	d.chunkReaders = cr

	var totalkv float64
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".meta.json") {
			meta := make(map[string]interface{})
			f, err := ioutil.ReadFile(appconf["indexDir"] + "/" + f.Name())
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

	// before opening for write always clear first
	err = os.RemoveAll(filepath.FromSlash(appconf["dbDir"]))
	if err != nil {
		log.Fatal("Error cleaning the out dir check you have right permission")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", appconf["dbDir"], "check you have right permission ")
		panic(err)
	}

	d.wrEnv, d.wrDbi = openDB(true, d.totalkvLine)

	if _, ok := appconf["mergeArraySize"]; ok {
		d.mergeTotalArrLen, err = strconv.Atoi(appconf["mergeArraySize"])
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
		} else {
			d.mergeTotalArrLen = 300000
		}

	}

	// setup progress
	defaultRate := 2 * time.Second
	if _, ok := appconf["progressRefreshRate"]; ok {
		rate, err := strconv.Atoi(appconf["progressRefreshRate"])
		if err != nil {
			panic("Invalid refresh rate definition")
		}
		defaultRate = time.Duration(rate) * time.Second
	}

	mergebar = pb.New64(d.totalkvLine).Prefix(" generate ")
	mergebar.ShowSpeed = false
	mergebar.ShowCounters = false
	mergebar.SetRefreshRate(defaultRate)
	mergebar.ShowTimeLeft = false
	mergebar.ShowElapsedTime = true
	mergebar.Start()
}

func (d *merge) writeBatch() {

	err := d.wrEnv.Update(func(txn *lmdb.Txn) (err error) {
		i := 0
		for i = 0; i < d.batchIndex; i++ {
			txn.Put(d.wrDbi, d.batchKeys[i], d.batchVals[i], lmdb.Append)
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

func (d *merge) close() {

	d.writeBatch()
	d.wrEnv.Close()

	var keepChunks bool
	if _, ok := appconf["keepChunks"]; ok && appconf["keepChunks"] == "yes" {
		keepChunks = true
	}
	if !keepChunks {
		err := os.RemoveAll(appconf["indexDir"])

		if err != nil {
			log.Print("Warn:Error cleaning the index dir check you have right permission")
		}

	}

	mergeStats := make(map[string]interface{})
	mergeStats["totalKey"] = d.totalKey
	mergeStats["totalValue"] = d.totalValue
	mergeStats["totalKVLine"] = d.totalkvLine
	data, err := json.Marshal(mergeStats)
	if err != nil {
		fmt.Println("Error while writing merge metadata")
	}

	ioutil.WriteFile(filepath.FromSlash(appconf["dbDir"]+"/db.meta.json"), data, 0770)

}

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
			key:     ch.nextLine[0],
			db:      ch.nextLine[1],
			value:   ch.nextLine[2],
			valuedb: ch.nextLine[3],
		}

	}

	var line [4]string
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

			mergebar.Increment()

			/**
						fmt.Println(string(ch.tmprun))
						fmt.Println("tabindex", tabIndex)
						fmt.Println("tmplen", len(ch.tmprun))
						fmt.Println("inde", index)
			**/
			line[index] = string(ch.tmprun[:tabIndex])

			if len(key) > 0 && line[0] != key {
				ch.nextLine = line
				return
			}

			if len(key) == 0 { //our key
				key = line[0]
				ch.curKey = key
			}

			*ch.d.mergeCh <- kvMessage{
				key:     line[0],
				db:      line[1],
				value:   line[2],
				valuedb: line[3],
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
			ch.tmprun[tabIndex] = c
			tabIndex++
		}

	}

}

func (d *merge) toProtoRoot(id string, kv map[string]*[]kvMessage, valIdx map[string]int, kvcounts *map[string]map[string]uint32) []byte {

	var result = pbuf.Result{}
	var xrefs = make([]*pbuf.Xref, len(kv))

	index := 0
	var totalCount uint32

	entriesArr := make([][]*pbuf.XrefEntry, len(kv))
	countsArr := make([][]*pbuf.XrefDomainCount, len(kv))

	for k, v := range kv {

		var xref = pbuf.Xref{}
		//var entries = make([]*pbuf.XrefEntry, len(v))

		if len(*d.protoResBufferPool) < 10 {
			panic("Very few available proto res array left. Define or increase 'protoBufPoolSize' parameter in configuration file. This will slightly effect of using more memory. Current array size is ->" + strconv.Itoa(d.protoBufferArrLen))
		}

		entries := <-*d.protoResBufferPool
		i := 0
		for i = 0; i < valIdx[k]; i++ {
			var xentry = pbuf.XrefEntry{}
			xentry.XrefId = (*v)[i].value
			d1, err := strconv.ParseInt((*v)[i].valuedb, 10, 16)
			if err != nil {
				panic("Error while converting to int16 ")
			}
			xentry.DomainId = uint32(d1)
			entries[i] = &xentry
		}
		entriesArr[index] = entries

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
				fmt.Println("Error while writing key", id)
				panic("Error while converting to int16 val->" + x)
			}
			xcount.DomainId = uint32(did)
			xcount.Count = y
			//d.protoCountResBuffer[j] = &xcount
			counts[j] = &xcount
			totalCount = totalCount + y
			j++
		}
		countsArr[index] = counts

		xref.Entries = entries[:i]
		xref.DomainCounts = counts[:j]
		xref.Count = totalCount
		d.totalValue = d.totalValue + uint64(totalCount)
		d.uidIndex++
		//xref.Uid = d.uidIndex
		did, err := strconv.ParseInt(k, 10, 16)
		if err != nil {
			panic("Error while converting to int16 ")
		}
		xref.DomainId = uint32(did)
		if did == 0 {
			xref.IsLink = true
			d.totalLinkKey++
		}

		xrefs[index] = &xref
		index++
		d.totalKey++
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

func (d *merge) toProtoPage(id string, dataset string, v *[]kvMessage, valIdx int) []byte {

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
		xentry.XrefId = (*v)[i].value
		d1, err := strconv.ParseInt((*v)[i].valuedb, 10, 16)
		if err != nil {
			panic("Error while converting to int16 ")
		}
		xentry.DomainId = uint32(d1)
		entries[i] = &xentry
		totalCount++
	}

	xref.Entries = entries[:i]
	xref.Count = totalCount
	d.totalValue = d.totalValue + uint64(totalCount)
	did, err := strconv.ParseInt(dataset, 10, 16)
	if err != nil {
		panic("Error while converting to int16 ")
	}
	xref.DomainId = uint32(did)
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
