package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

const tab string = "\t"
const newline string = "\n"
const idkey = "id"

//const backquote string = "`"

type kvgen struct {
	wid                    string
	dbid                   string
	dataChan               *chan string
	mergeGateCh            *chan mergeInfo
	chunk                  []string
	chunkIndex             int
	chunkFileCounter       int
	chunkMergedFileCounter int
	chunkSizeCounter       uint64
	totalkv                uint64
}

func newkvgen(id string) kvgen {

	var chunkk = make([]string, chunkLen)

	return kvgen{
		wid:              id,
		chunk:            chunkk,
		chunkFileCounter: 1,
	}

}

func (k *kvgen) gen() {

	for kv := range *k.dataChan {

		if k.chunkIndex == chunkLen {
			k.flushChunk(nil, false)
		}

		k.chunk[k.chunkIndex] = kv
		k.chunkIndex++
		k.totalkv++

	}

}

func (k *kvgen) flushChunk(wg *sync.WaitGroup, last bool) {

	if wg != nil {
		defer wg.Done()
	}

	chunkFileName := appconf["indexDir"] + "/" + k.wid + "_" + strconv.Itoa(k.chunkFileCounter) + "." + chunkIdx + ".gz"

	//f, err := os.Create(filepath.FromSlash(chunkFileName))
	f, err := os.OpenFile(filepath.FromSlash(chunkFileName), os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		panic(err)
	}

	gw, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	//gw := bufio.NewWriterSize(f, fileBufSize)

	sort.Strings(k.chunk[:k.chunkIndex])

	for i := 0; i < k.chunkIndex; i++ {
		if i == 0 || k.chunk[i] != k.chunk[i-1] {
			gw.Write([]byte(k.chunk[i]))
			gw.Write([]byte(newline))
		} else {
			k.totalkv--
		}
	}

	gw.Close()
	f.Close()

	k.chunkFileCounter++
	k.chunkIndex = 0

	*k.mergeGateCh <- mergeInfo{
		fname: chunkFileName,
		level: 1,
		last:  last,
	}

}

func (k *kvgen) close(last bool) uint64 {

	if k.chunkIndex > 0 {
		k.flushChunk(nil, last)
	} else {
		// todo rare case if last true
	}

	return k.totalkv

}
