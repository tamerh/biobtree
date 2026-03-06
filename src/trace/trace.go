package main

import (
	"bufio"
	"compress/gzip"
	"flag"
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
)

var fonk string
var indexDir string

func main() {

	flag.StringVar(&fonk, "fonk", "rm", "")
	flag.StringVar(&indexDir, "idir", "", "")

	flag.Parse()

	start := time.Now()

	switch fonk {
	case "kv":
		findInvalidKVMessage()
	case "rm":
		removeInvalidKVMessage()

	}

	elapsed := time.Since(start)
	log.Printf("Finished took %s", elapsed)

}

// this is a temporary workaround in same datasets like bacteria there
// links with new line chars this method remove those line and create new file.
func removeInvalidKVMessage() {

	var wg sync.WaitGroup

	processFile := func(indexdir, fname string) {

		defer wg.Done()
		newFile := "new_" + fname

		//f, err := os.Create(filepath.FromSlash(chunkFileName))
		f, err := os.OpenFile(filepath.FromSlash(newFile), os.O_RDWR|os.O_CREATE, 0700)
		if err != nil {
			panic(err)
		}
		gw, err := gzip.NewWriterLevel(f, gzip.BestSpeed)

		file, err := os.Open(filepath.FromSlash(indexDir + "/" + fname))
		gz, err := gzip.NewReader(file)
		if err == io.EOF { //zero file
			return
		}
		check(err)
		br := bufio.NewReaderSize(gz, 65536)

		var line string

		for {
			line, err = br.ReadString('\n')

			if err != nil {
				break
			}
			spil := strings.Split(line, "\t")
			if len(spil) == 4 && len(spil[0]) > 1 && len(spil[1]) > 0 && len(spil[2]) > 1 && len(spil[3]) > 1 {
				gw.Write([]byte(line))
			}

		}
		gw.Close()
		f.Close()
	}

	files, err := ioutil.ReadDir(indexDir)
	check(err)

	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".gz") {
			wg.Add(1)
			go processFile(indexDir, f.Name())
		}
	}

	wg.Wait()

}

// loop in the gz files and find and print the file and line with invalid line generated
func findInvalidKVMessage() {

	var wg sync.WaitGroup

	processFile := func(path string) {

		defer wg.Done()
		file, err := os.Open(filepath.FromSlash(path))
		gz, err := gzip.NewReader(file)
		if err == io.EOF { //zero file
			return
		}
		check(err)
		br := bufio.NewReaderSize(gz, 65536)

		var line string
		linenum := 0
		history := make([]string, 10)
		index := 0
		found := false
		for {
			line, err = br.ReadString('\n')

			if err != nil {
				break
			}
			history[index] = line
			index++

			if index == 10 {
				index = 0
			}

			if found {
				for _, l := range history {
					fmt.Println(l)
				}
				panic("invalid data file ->" + path + " input->" + line + " line number->" + strconv.Itoa(linenum))
			}
			if len(strings.Split(line, "\t")) != 4 {
				found = true
			}
			linenum++

		}
	}

	files, err := ioutil.ReadDir(indexDir)
	check(err)

	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".gz") { // todo
			wg.Add(1)
			go processFile(indexDir + "/" + f.Name())
		}
	}

	wg.Wait()

}

func check(err error) {

	if err != nil {
		panic(err)
	}

}
