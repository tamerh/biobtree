package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"
)

// this creates example json file for the usecase in the web interface

type queryExample struct {
	Name           string `json:"name"`
	Typee          string `json:"type"`
	Source         string `json:"source"`
	SearchTerm     string `json:"searchTerm"`
	MapfFilterTerm string `json:"mapFilterTerm"`
}

//var categories = []string{"mix", "gene", "protein", "chembl", "taxonomy"}

var categories = map[string]bool{}
var results = map[string][]queryExample{}
var db, categoriesStr string

func newResult(category, name, res string) {

	if len(category) == 0 || len(name) == 0 || len(res) == 0 {
		return
	}

	if _, ok := categories[category]; !ok {
		return
	}

	category = strings.Split(category, "_")[0]

	resSplit := strings.Split(res, " ")

	if len(resSplit) != 6 {
		fmt.Println(resSplit)
		log.Fatal("invalid result" + res)
	}

	if _, ok := results[category]; !ok {
		results[category] = []queryExample{}
	}

	typee, searchTerm, mapfFilterTerm, source, ok := getTestParams(resSplit[1])

	if !ok {
		return
	}

	newExample := queryExample{
		Name:           name,
		Typee:          typee,
		Source:         source,
		SearchTerm:     searchTerm,
		MapfFilterTerm: mapfFilterTerm,
	}
	results[category] = append(results[category], newExample)

}
func main() {

	flag.StringVar(&db, "db", "builtin1", "")
	flag.StringVar(&categoriesStr, "cat", "", "")
	flag.Parse()

	for _, cat := range strings.Split(categoriesStr, ",") {
		categories[cat] = true
	}

	file, err := os.Open("newman_result.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	alt := '↳'
	tick := '✓'
	box := '❏'
	isfirst := true
	var curCategory, curName, curRes string
	curSuccess := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		liner := []rune(line)

		if len(liner) == 0 {
			continue
		}

		if liner[0] == box {

			if len(curCategory) > 0 && curCategory != string(liner[2:]) {
				newResult(curCategory, curName, curRes)
				curName = ""
				curRes = ""
			}
			curCategory = string(liner[2:])

		} else if liner[0] == alt { // new test

			if !isfirst { // print previous test
				if !curSuccess {
					panic("Failed tests check generated file")
				}
				newResult(curCategory, curName, curRes)
				curName = ""
				curRes = ""
				curSuccess = false
			} else {
				isfirst = false
			}
			curName = string(liner[2:])

		} else if line[0:3] == "GET" {

			curRes = line

		} else if liner[0] == tick { //test result

			curSuccess = true

		}

	}

	// last test
	if !curSuccess {
		panic("Failed tests check generated file")
	}
	newResult(curCategory, curName, curRes)

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	for k, v := range results {
		fmt.Println("cat:" + k)
		fmt.Println()
		for _, q := range v {
			fmt.Println()
			fmt.Println("#" + q.Name)
			if q.Typee == "0" {
				if len(q.Source) > 0 {
					fmt.Println("bb.search('" + strings.ToLower(q.SearchTerm) + "',source='" + q.Source + "')")
				} else {
					fmt.Println("bb.search('" + strings.ToLower(q.SearchTerm) + "')")
				}

			} else if q.Typee == "1" {
				if len(q.Source) > 0 {
					fmt.Println("bb.mapping('" + strings.ToLower(q.SearchTerm) + "','" + q.MapfFilterTerm + "',source='" + q.Source + "')")
				} else {
					fmt.Println("bb.mapping('" + strings.ToLower(q.SearchTerm) + "','" + q.MapfFilterTerm + "')")
				}

			}

		}
	}

	data, err := json.Marshal(results)
	if err != nil {
		panic(err)
	}

	ioutil.WriteFile(db+".json", data, 0770)

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
