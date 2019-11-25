package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"math/rand"
	"os"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

var sampleSize int

func main() {

	flag.IntVar(&sampleSize, "sample", 100, "")

	flag.Parse()

	samples := make([]string, sampleSize)

	f, err := os.Open("/Users/tgur/Downloads/uniprot_sprot.xml.gz")

	if err != nil {
		panic(err)
	}

	gz, err := gzip.NewReader(f)

	if err != nil {
		panic(err)
	}

	br := bufio.NewReaderSize(gz, 65536)

	parser := xmlparser.NewXMLParser(br, "entry")

	index := 0
	for xml := range parser.Stream() {

		if xml.Childs["name"] == nil {
			continue
		}

		entryid := xml.Childs["name"][0].InnerText

		if index > 0 {
			rand := rand.Intn(index)

			if index < sampleSize {
				samples[index] = entryid
			} else if rand < 100 {
				samples[rand] = entryid
			}

		} else {
			samples[0] = entryid
		}

		index++

	}

	for _, s := range samples {
		fmt.Print(s, ",")
	}

}
