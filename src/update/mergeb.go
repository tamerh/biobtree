package update

import (
	"biobtree/configs"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const newlinebyte = byte('\n')

type mergeb struct {
	wg          *sync.WaitGroup
	mergeGateCh *chan mergeInfo
	stop        bool
}
type mergeInfo struct {
	fname string
	level int
	close bool
}

type binarymerge struct {
	level        int
	outFileIndex int
	mergeGateCh  *chan mergeInfo
	ch           *chan mergeInfo
	nextch       *chan mergeInfo
	files        [2]string
	wg           *sync.WaitGroup
	conf         *configs.Conf
	//mb           *mergeb
}

// mergeState holds per-merge state to avoid race conditions
// when multiple merges run concurrently on the same binarymerge
type mergeState struct {
	filePaths [2]string
	brr       [2]*bufio.Reader
	ffiles    [2]*os.File
	gzs       [2]*gzip.Reader
	lines     [2]string
	eof       [2]bool
}

func (mb *mergeb) start() {

	defer mb.wg.Done()

	bms := map[int]*binarymerge{}

	for minfo := range *mb.mergeGateCh {

		if _, ok := bms[minfo.level]; !ok && !mb.stop {

			ch := make(chan mergeInfo, 10000)
			bm := &binarymerge{
				level:       minfo.level,
				ch:          &ch,
				mergeGateCh: mb.mergeGateCh,
				wg:          mb.wg,
				//mb:          mb,
			}
			if minfo.level > 1 {
				bms[minfo.level/2].nextch = &ch
			}
			bms[minfo.level] = bm
			mb.wg.Add(1)
			go bm.start()
			//fmt.Println("opening bmerge channel level", minfo.level)
		}

		if minfo.close {
			//fmt.Println("close merge gate")
			*bms[1].ch <- minfo
			mb.stop = true
			continue
		}

		if !mb.stop {

			*bms[minfo.level].ch <- minfo

		}

	}

}

func (bm *binarymerge) start() {

	defer bm.wg.Done()

	var wg sync.WaitGroup

	index := 0
	for minfo := range *bm.ch {

		if minfo.close {
			// If there's an odd file waiting, pass it to next level
			if index == 1 {
				if bm.nextch != nil {
					*bm.nextch <- mergeInfo{
						fname: bm.files[0],
						level: bm.level * 2,
					}
				}
				// If no next level, the file is already in final form
			}

			close(*bm.ch)
			if bm.nextch != nil {
				*bm.nextch <- mergeInfo{
					close: true,
				}
			} else {
				close(*bm.mergeGateCh)
			}

			continue
		}

		bm.files[index] = minfo.fname
		index++

		if index == 2 {

			//mergedFile := config.Appconf["indexDir"] + "/" + strconv.Itoa(bm.level) + "_" + strconv.Itoa(bm.outFileIndex) + "." + chunkIdx + ".index.gz"
			//fmt.Println("Merging 1->", bm.files[0], " 2->", bm.files[1], " Merged file-->", mergedFile)

			//wg.Add(1)
			bm.merge(&wg)
			//wg.Wait()
			index = 0
		}

	}
}

func (ms *mergeState) readLine(ind int) {

	line, err := ms.brr[ind].ReadString(newlinebyte)

	if err == io.EOF {
		ms.eof[ind] = true
		// at this stage we can also delete this file
		ms.gzs[ind].Close()
		ms.ffiles[ind].Close()

		err := os.Remove(filepath.FromSlash(ms.filePaths[ind]))
		if err != nil {
			fmt.Println("Cant remove the file", filepath.FromSlash(ms.filePaths[ind]))
			panic(err)
		}

		return
	}

	if err != nil {
		fmt.Println("Error while reading file->", ms.filePaths[ind])
		panic(err)
	}

	ms.lines[ind] = line

}

func (bm *binarymerge) merge(wg *sync.WaitGroup) {

	/**
	if bm.mb.stop {
		return
	}
	**/

	mergedFile := config.Appconf["indexDir"] + "/" + strconv.Itoa(bm.level) + "_" + strconv.Itoa(bm.outFileIndex) + "." + chunkIdx + ".index.gz"
	bm.outFileIndex++

	// Create local merge state to avoid race conditions
	// Each merge gets its own state that won't be overwritten
	ms := &mergeState{
		filePaths: [2]string{bm.files[0], bm.files[1]},
	}

	file1, err := os.Open(filepath.FromSlash(ms.filePaths[0]))
	check(err)
	ms.ffiles[0] = file1
	gz, err := gzip.NewReader(file1)
	check(err)
	ms.gzs[0] = gz

	br1 := bufio.NewReaderSize(gz, fileBufSize)
	ms.brr[0] = br1

	file2, err := os.Open(filepath.FromSlash(ms.filePaths[1]))
	check(err)
	ms.ffiles[1] = file2
	gz2, err := gzip.NewReader(file2)
	check(err)
	ms.gzs[1] = gz2
	br2 := bufio.NewReaderSize(gz2, fileBufSize)
	ms.brr[1] = br2

	f, err := os.Create(filepath.FromSlash(mergedFile))
	check(err)
	gw, err := gzip.NewWriterLevel(f, gzip.BestCompression)

	// for initial read from both
	ms.readLine(0)
	ms.readLine(1)

	sortedCh := make(chan string, 10000)

	go func() {
		for {

			if ms.lines[0] < ms.lines[1] {

				sortedCh <- ms.lines[0]
				ms.readLine(0)

				if ms.eof[0] {
					sortedCh <- ms.lines[1]
					break
				}

			} else if ms.lines[0] > ms.lines[1] {

				sortedCh <- ms.lines[1]
				ms.readLine(1)

				if ms.eof[1] {
					sortedCh <- ms.lines[0]
					break
				}

			} else {

				sortedCh <- ms.lines[1]
				ms.readLine(0)
				ms.readLine(1)

				if ms.eof[0] || ms.eof[1] {
					break
				}

			}
		}

		if !ms.eof[0] {
			for {
				ms.readLine(0)
				if ms.eof[0] {
					break
				}
				sortedCh <- ms.lines[0]
			}
		}

		if !ms.eof[1] {
			for {
				ms.readLine(1)
				if ms.eof[1] {
					break
				}
				sortedCh <- ms.lines[1]
			}
		}
		close(sortedCh)
	}()

	//var prevLine string
	for line := range sortedCh {
		gw.Write([]byte(line))
	}

	gw.Close()
	f.Close()

	mfinfo := mergeInfo{
		fname: mergedFile,
		level: bm.level * 2,
	}
	*bm.mergeGateCh <- mfinfo

}
