package main

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/tamerh/jsparser"
)

// this creates example json file for the usecase in the web interface

type queryExample struct {
	Name           string `json:"name"`
	Typee          string `json:"type"`
	Source         string `json:"source"`
	SearchTerm     string `json:"searchTerm"`
	MapfFilterTerm string `json:"mapFilterTerm"`
}

var categories = []string{"mix", "gene", "protein", "chembl", "taxonomy"}

func main() {

	f, err := os.Open("biobtree_default.postman_test_run.json")

	if err != nil {
		panic(err)
	}

	br := bufio.NewReader(f)

	jsparser := jsparser.NewJSONParser(br, "results")

	results := map[string][]queryExample{}

	for json := range jsparser.Stream() {

		ok, category := getCategory(json.ObjectVals["name"].StringVal)

		if ok {

			if _, ok := results[category]; !ok {
				results[category] = []queryExample{}
			}
			typee, searchTerm, mapfFilterTerm, source, ok := getTestParams(json.ObjectVals["url"].StringVal)
			if !ok {
				continue
			}
			newExample := queryExample{
				Name:           json.ObjectVals["name"].StringVal[strings.Index(json.ObjectVals["name"].StringVal, " ")+1:],
				Typee:          typee,
				Source:         source,
				SearchTerm:     searchTerm,
				MapfFilterTerm: mapfFilterTerm,
			}
			results[category] = append(results[category], newExample)

		}
	}

	sortedresults := map[string][]queryExample{}

	for i, cat := range categories {
		sortedresults[strconv.Itoa(i)+"_"+cat] = results[cat]
	}

	data, err := json.Marshal(sortedresults)
	if err != nil {
		panic(err)
	}

	ioutil.WriteFile("examples.json", data, 0770)

	//fmt.Println(results)

}

func getTestParams(urlval string) (string, string, string, string, bool) {

	u, err := url.Parse(urlval)
	if err != nil {
		log.Fatal(err)
	}

	params := u.Query()

	if len(params.Get("i")) > 0 && len(params.Get("m")) > 0 && len(params.Get("s")) > 0 {
		return "1", params.Get("i"), params.Get("m"), params.Get("s"), true
	} else if len(params.Get("i")) > 0 && len(params.Get("m")) > 0 {
		return "1", params.Get("i"), params.Get("m"), "", true
	} else if len(params.Get("i")) > 0 && len(params.Get("s")) > 0 {
		return "0", params.Get("i"), "", params.Get("s"), true
	} else if len(params.Get("i")) > 0 {
		return "0", params.Get("i"), "", "", true
	}

	return "", "", "", "", false

}

func getCategory(testname string) (bool, string) {

	for _, cat := range categories {
		if strings.HasPrefix(testname, cat) {
			return true, cat
		}
	}
	return false, ""
}
