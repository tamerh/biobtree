package parser

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGzXml(t *testing.T) {

	var resultChannel = make(chan XMLEntry)

	var wg sync.WaitGroup

	/*
		p := mpb.New(mpb.WithWaitGroup(&wg))
		wg.Add(1)

		bar := p.AddBar(int64(550000),
			mpb.PrependDecorators(
				// simple name decorator
				decor.Name("test"),
				// decor.DSyncWidth bit enables column width synchronization
				decor.Percentage(decor.WCSyncSpace),
			),
			mpb.AppendDecorators(
				decor.OnComplete(decor.Elapsed(decor.ET_STYLE_GO), "done",),
			),
		)
	*/
	//file, _ := os.Open("uniprot_test.xml.gz")
	file, _ := os.Open("../../test_data/uniprot_sprot.xml.gz")

	defer file.Close()

	gz, err := gzip.NewReader(file)

	if err != nil {
		fmt.Println("Error,", err)
	}

	//todo check with bigger buffer size
	br := bufio.NewReader(gz)

	var parser = XMLParser{
		R:          br,
		LoopTag:    "entry",
		OutChannel: &resultChannel,
		SkipTags:   []string{"comment", "gene", "protein", "feature", "sequence"},
		//Size:       550000,
		//ProgBar: bar,
	}

	start := time.Now()
	fmt.Println("Started...")
	go func() {
		parser.Parse()
		wg.Done()
	}()

	for range resultChannel {
		//fmt.Println(result)
	}

	elapsed := time.Since(start)
	log.Printf("Binomial took %s", elapsed)

}

func TestXml(t *testing.T) {

	var resultChannel = make(chan XMLEntry)

	file, _ := os.Open("books.xml")
	defer file.Close()

	br := bufio.NewReader(file)

	var parser = XMLParser{
		R:          br,
		LoopTag:    "book",
		OutChannel: &resultChannel,
	}

	go parser.Parse()

	var resultEntryCount int
	for range resultChannel {
		resultEntryCount++
	}

	if resultEntryCount != 12 {
		panic("Expected result count is 12 but found ->" + string(resultEntryCount))
	}

}

func TestXmlUniref(t *testing.T) {

	var resultChannel = make(chan XMLEntry)

	file, _ := os.Open("uniref.xml")
	defer file.Close()

	br := bufio.NewReader(file)

	var parser = XMLParser{
		R:          br,
		LoopTag:    "entry",
		OutChannel: &resultChannel,
	}

	go parser.Parse()

	var resultEntryCount int
	for range resultChannel {
		resultEntryCount++
	}

	if resultEntryCount != 1 {
		panic("Expected result count is 1 but found ->" + string(resultEntryCount))
	}

}

func TestArticleXml(t *testing.T) {

	var resultChannel = make(chan XMLEntry)

	file, _ := os.Open("article.xml")
	defer file.Close()

	br := bufio.NewReader(file)

	var parser = XMLParser{
		R:          br,
		LoopTag:    "article-meta",
		OutChannel: &resultChannel,
	}

	go parser.Parse()

	for entry := range resultChannel {
		if len(entry.Elements["article-id"]) != 4 {
			panic("Article should have 3 article id -> ")
		}

	}

}

func TestTaxonomyXml(t *testing.T) {

	var resultChannel = make(chan XMLEntry)

	file, _ := os.Open("taxonomy.xml")
	defer file.Close()

	br := bufio.NewReader(file)

	var parser = XMLParser{
		R:          br,
		LoopTag:    "taxon",
		OutChannel: &resultChannel,
	}

	go parser.Parse()

	for entry := range resultChannel {
		fmt.Println(entry)
	}

}

func TestUniprotXrefs(t *testing.T) {

	var resultChannel = make(chan XMLEntry)

	file, _ := os.Open("uniprot_xrefs.xml")
	defer file.Close()

	br := bufio.NewReader(file)

	var parser = XMLParser{
		R:          br,
		LoopTag:    "rdf:Description",
		OutChannel: &resultChannel,
	}

	go parser.Parse()

	var uniprotMappings = map[string]string{}
	for entry := range resultChannel {
		if len(entry.Elements["abbreviation"]) > 0 {

			abbr := entry.Elements["abbreviation"][0].InnerText

			if len(entry.Elements["urlTemplate"]) > 0 {
				url := entry.Elements["urlTemplate"][0].InnerText

				uniprotMappings[strings.ToUpper(abbr)] = url
			}

		}
	}

	dataconfFile := filepath.FromSlash("../../conf/data.json")

	dataconf := map[string]map[string]string{}

	f, err := ioutil.ReadFile(dataconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &dataconf); err != nil {
		panic(err)
	}

	for k, v := range dataconf {
		if len(v["url"]) == 0 {
			if _, ok := uniprotMappings[strings.ToUpper(k)]; ok {
				dataconf[k]["url"] = uniprotMappings[strings.ToUpper(k)]
				dataconf[k]["name"] = k
			}
		}
	}

}

func TestHmdb(t *testing.T) {

	var resultChannel = make(chan XMLEntry)

	file, _ := os.Open("hmdb.xml")
	defer file.Close()

	br := bufio.NewReader(file)

	var parser = XMLParser{
		R:          br,
		LoopTag:    "metabolite",
		SkipTags:   []string{"taxonomy,ontology"},
		OutChannel: &resultChannel,
	}

	go parser.Parse()

	for entry := range resultChannel {
		fmt.Println(entry)
	}

}
