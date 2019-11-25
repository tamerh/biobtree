package main

import (
	"bufio"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"
	"strconv"
	"strings"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

func main() {

	f, err := os.Open("/Users/tgur/Downloads/taxonomy.xml.gz")

	if err != nil {
		panic(err)
	}

	gz, err := gzip.NewReader(f)

	if err != nil {
		panic(err)
	}

	br := bufio.NewReaderSize(gz, 65536)

	p := xmlparser.NewXMLParser(br, "taxon").SkipElements([]string{"lineage"})

	taxNameIDMap := map[string]int{}

	for r := range p.Stream() {

		// id
		id, err := strconv.Atoi(r.Attrs["taxId"])
		if err != nil {
			panic("invalid taxid")
		}

		name := strings.ToLower(strings.Replace(r.Attrs["scientificName"], " ", "_", -1))

		taxNameIDMap[name] = id

	}

	// Create a file for IO
	encodeFile, err := os.Create("taxids.gob")
	if err != nil {
		panic(err)
	}

	// Since this is a binary format large parts of it will be unreadable
	encoder := gob.NewEncoder(encodeFile)

	// Write to the file
	if err := encoder.Encode(taxNameIDMap); err != nil {
		panic(err)
	}
	encodeFile.Close()

	fmt.Println(len(taxNameIDMap))
}
