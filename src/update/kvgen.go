package update

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
	wg                     *sync.WaitGroup
	//sync.Mutex
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

	defer k.wg.Done()

	for kv := range *k.dataChan {

		if k.chunkIndex == chunkLen {
			k.flushChunk()
		}

		k.chunk[k.chunkIndex] = kv
		k.chunkIndex++
		k.totalkv++

	}

	k.flushChunk()

}

func (k *kvgen) flushChunk() {

	//k.Lock()
	//defer k.Unlock()

	if k.chunkIndex > 0 {
		fileCounter := strconv.Itoa(k.chunkFileCounter)

		chunkFileName := appconf["indexDir"] + "/" + k.wid + "_" + fileCounter + "." + chunkIdx + ".gz"

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
		}
	}

}
