package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"strings"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

var sampleSize int
var uniprotLocation string

func main() {

	flag.IntVar(&sampleSize, "sample", 100, "")
	flag.StringVar(&uniprotLocation, "file", "uniprot_sprot.xml.gz", "")

	flag.Parse()

	samples := make([]string, sampleSize)

	f, err := os.Open(uniprotLocation)

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
	testFilters := []string{`map(go).filter(go.type=="molecular_function")`, `map(go).filter(go.type=="biological_process")`,
		`map(go).filter(go.type=="cellular_component")`, `map(go).filter(go.type=="cellular_component" || go.type=="biological_process")`,
		`map(go).filter(go.type=="cellular_component" && go.type=="biological_process")`,
		`map(hgnc)`,
		`map(refseq)`,
		`map(ufeature).filter(ufeature.type=="helix")`,
		`map(pdb).filter(pdb.method=="nmr")`,
		`map(ena).filter(ena.type=="mrna")`,
		`map(ena).filter(ena.type=="genomic_dna")`,
		`map(hgnc).filter(hgnc.status=="Approved")`}

	var builder strings.Builder
	for index, s := range samples {
		for _, mapf := range testFilters {
			builder.WriteString("GET http://localhost:8888/ws/map/?i=")
			builder.WriteString(s)
			builder.WriteString("&m=")
			builder.WriteString(url.QueryEscape(mapf))
			if index != len(samples)-1 {
				builder.WriteString("\n")
			}
		}
	}

	// write the whole body at once
	err = ioutil.WriteFile("output.txt", []byte(builder.String()), 0644)
	if err != nil {
		panic(err)
	}

}
