package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

type mergeb struct {
	wg          *sync.WaitGroup
	mergeGateCh *chan mergeInfo
	stop        bool
}
type mergeInfo struct {
	fname string
	level int
	last  bool
	close bool
}

type binarymerge struct {
	level        int
	outFileIndex int
	mergeGateCh  *chan mergeInfo
	ch           *chan mergeInfo
	nextch       *chan mergeInfo
	files        [2]string
	brr          [2]*bufio.Reader
	ffiles       [2]*os.File
	gzs          [2]*gzip.Reader
	lines        [2]string
	complete     [2]bool
	eof          [2]bool
	wg           *sync.WaitGroup
	//mb           *mergeb
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

		if minfo.last {

			*bms[minfo.level].ch <- mergeInfo{
				close: true,
			}
			mb.stop = true

		} else {

			if !mb.stop {
				*bms[minfo.level].ch <- minfo
			}

		}

	}

}

func (bm *binarymerge) start() {

	defer bm.wg.Done()

	var wg sync.WaitGroup

	index := 0
	for minfo := range *bm.ch {

		if minfo.close {
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
			//wg.Add(1)
			bm.merge(&wg)
			//wg.Wait()
			index = 0
		}

	}
}

func (bm *binarymerge) readLine(ind int) {

	line, err := bm.brr[ind].ReadString(newlinebyte)

	if err == io.EOF {
		bm.eof[ind] = true
		// at this stage we can also delete this file
		bm.gzs[ind].Close()
		bm.ffiles[ind].Close()
		err := os.Remove(filepath.FromSlash(bm.files[ind]))
		if err != nil {
			panic(err)
		}
		return
	}

	if err != nil {
		fmt.Println("Error while reading file->", bm.files[ind])
		panic(err)
	}

	bm.lines[ind] = line

}

func (bm *binarymerge) merge(wg *sync.WaitGroup) {

	/**
	if bm.mb.stop {
		return
	}
	**/
	mergedFile := appconf["indexDir"] + "/" + strconv.Itoa(bm.level) + "_" + strconv.Itoa(bm.outFileIndex) + "." + chunkIdx + ".index.gz"
	bm.outFileIndex++

	bm.complete[0] = false
	bm.complete[1] = false
	bm.eof[0] = false
	bm.eof[1] = false

	file1, err := os.Open(filepath.FromSlash(bm.files[0]))
	check(err)
	bm.ffiles[0] = file1
	gz, err := gzip.NewReader(file1)
	check(err)
	bm.gzs[0] = gz

	br1 := bufio.NewReaderSize(gz, fileBufSize)
	bm.brr[0] = br1

	file2, err := os.Open(filepath.FromSlash(bm.files[1]))
	check(err)
	bm.ffiles[1] = file2
	gz2, err := gzip.NewReader(file2)
	check(err)
	bm.gzs[1] = gz2
	br2 := bufio.NewReaderSize(gz2, fileBufSize)
	bm.brr[1] = br2

	f, err := os.Create(filepath.FromSlash(mergedFile))
	check(err)
	gw, err := gzip.NewWriterLevel(f, gzip.BestSpeed)

	// for initial read from both
	bm.readLine(0)
	bm.readLine(1)

	var prevLine string

	// a lot duplicate code which needs refactor but working.
	for {
	start:

		if bm.complete[0] && bm.complete[1] {
			gw.Close()
			break
		}

		if bm.lines[0] < bm.lines[1] {

			gw.Write([]byte(bm.lines[0]))
			if bm.eof[0] {
				bm.complete[0] = true
				gw.Write([]byte(bm.lines[1]))
				for {
					bm.readLine(1)
					if bm.eof[1] {
						bm.complete[1] = true
						prevLine = ""
						goto start
					}
					if len(prevLine) == 0 || prevLine != bm.lines[1] {
						gw.Write([]byte(bm.lines[1]))
						prevLine = bm.lines[1]
					}
				}
			}
			bm.readLine(0)

		} else if bm.lines[1] < bm.lines[0] {

			gw.Write([]byte(bm.lines[1]))
			if bm.eof[1] {
				bm.complete[1] = true
				gw.Write([]byte(bm.lines[0]))
				for {
					bm.readLine(0)
					if bm.eof[0] {
						bm.complete[0] = true
						prevLine = ""
						goto start
					}

					if len(prevLine) == 0 || prevLine != bm.lines[0] {
						gw.Write([]byte(bm.lines[0]))
						prevLine = bm.lines[0]
					}

				}
			}
			bm.readLine(1)

		} else {

			gw.Write([]byte(bm.lines[0]))

			if bm.eof[0] && bm.eof[1] {
				bm.complete[0] = true
				bm.complete[1] = true
				gw.Close()
				break
			}

			if bm.eof[0] {
				bm.complete[0] = true
				for {
					bm.readLine(1)
					if bm.eof[1] {
						bm.complete[1] = true
						prevLine = ""
						goto start
					}
					if len(prevLine) == 0 || prevLine != bm.lines[1] {
						gw.Write([]byte(bm.lines[1]))
						prevLine = bm.lines[1]
					}

				}
			}

			if bm.eof[1] {
				bm.complete[1] = true
				for {
					bm.readLine(0)
					if bm.eof[0] {
						bm.complete[0] = true
						prevLine = ""
						goto start
					}

					if len(prevLine) == 0 || prevLine != bm.lines[0] {
						gw.Write([]byte(bm.lines[0]))
						prevLine = bm.lines[0]
					}

				}
			}

			bm.readLine(0)
			bm.readLine(1)

			if bm.eof[0] && bm.eof[1] {
				bm.complete[0] = true
				bm.complete[1] = true
				gw.Close()
				break
			}

			if bm.eof[0] {
				bm.complete[0] = true
				gw.Write([]byte(bm.lines[1]))
				for {
					bm.readLine(1)
					if bm.eof[1] {
						bm.complete[1] = true
						prevLine = ""
						goto start
					}
					if len(prevLine) == 0 || prevLine != bm.lines[1] {
						gw.Write([]byte(bm.lines[1]))
						prevLine = bm.lines[1]
					}

				}
			}

			if bm.eof[1] {
				bm.complete[1] = true
				gw.Write([]byte(bm.lines[0]))
				for {
					bm.readLine(0)
					if bm.eof[0] {
						bm.complete[0] = true
						prevLine = ""
						goto start
					}

					if len(prevLine) == 0 || prevLine != bm.lines[0] {
						gw.Write([]byte(bm.lines[0]))
						prevLine = bm.lines[0]
					}

				}
			}

		}

	}

	gw.Close()
	f.Close()

	mfinfo := mergeInfo{
		fname: mergedFile,
		level: bm.level * 2,
	}
	*bm.mergeGateCh <- mfinfo

}
