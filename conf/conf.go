package conf

import (
	"biobtree/util"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var fileBufSize = 65536
var channelOverflowCap = 100000

const latestReleasePath = "https://api.github.com/repos/tamerh/biobtree/releases/latest"

type Conf struct {
	Appconf               map[string]string
	Dataconf              map[string]map[string]string
	DataconfIDIntToString map[uint32]string // for now not used only validates
	DataconfIDStringToInt map[string]uint32
	DataconfIDToPageKey   map[uint32]string // uniprot -> 1 -> ab
	FilterableDatasets    map[string]bool
	githubRawPath         string
	githubContentPath     string
	versionTag            string
}

type gitLatestRelease struct {
	Tag string `json:"tag_name"`
}

type gitContent struct {
	Name   string `json:"name"`
	RawURL string `json:"download_url"`
}

func (c *Conf) Init(rootDir, versionTag string, optionalDatasetActive bool, outDir string) {

	c.githubRawPath = "https://raw.githubusercontent.com/tamerh/biobtree/" + versionTag
	c.githubContentPath = "https://api.github.com/repos/tamerh/biobtree/contents/"
	c.versionTag = versionTag

	c.checkForNewVersion()

	confdir := rootDir + "conf"
	//	if len(customconfdir) > 0 {
	//		confdir = customconfdir
	//	}

	exist, err := fileExists(confdir)

	if err != nil {
		panic("Error while checking file")
	}

	appConfFilePath := filepath.FromSlash(confdir + "/application.param.json")
	defaultDataConfFilePath := filepath.FromSlash(confdir + "/default.dataset.json")
	optionalDataConfFilePath := filepath.FromSlash(confdir + "/optional.dataset.json")
	sourcedataconfFilePath := filepath.FromSlash(confdir + "/source.dataset.json")

	if !exist {
		log.Println("Downloading configuration files.")
		err := os.Mkdir("conf", 0700)
		if err != nil {
			panic("Error while creating conf directory")
		}
		downloadFile(c.githubRawPath+"/conf/application.param.json", appConfFilePath)
		downloadFile(c.githubRawPath+"/conf/source.dataset.json", sourcedataconfFilePath)
		downloadFile(c.githubRawPath+"/conf/default.dataset.json", defaultDataConfFilePath)
		downloadFile(c.githubRawPath+"/conf/optional.dataset.json", optionalDataConfFilePath)

		downloadFile(c.githubRawPath+"/LICENSE.md", "LICENSE.md")
		downloadFile(c.githubRawPath+"/LICENSE.lmdbgo.md", "LICENSE.lmdbgo.md")
		downloadFile(c.githubRawPath+"/LICENSE.mdb.md", "LICENSE.mdb.md")

		log.Println("Files downloaded.")
	}

	exist, err = fileExists(rootDir + "website")

	if err != nil {
		panic("Error while checking file")
	}

	if !exist {
		c.retrieveWebFiles()
	}

	appconfFile := filepath.FromSlash(appConfFilePath)
	defualtconfFile := filepath.FromSlash(defaultDataConfFilePath)
	optionalconfFile := filepath.FromSlash(optionalDataConfFilePath)
	sourcedataconfFile := filepath.FromSlash(sourcedataconfFilePath)

	f, err := ioutil.ReadFile(appconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &c.Appconf); err != nil {
		panic(err)
	}

	// set root dir if passed
	if len(rootDir) > 0 {
		c.Appconf["rootDir"] = rootDir
	}

	if optionalDatasetActive {
		f, err = ioutil.ReadFile(optionalconfFile)
		if err != nil {
			fmt.Printf("Error: %v", err)
			os.Exit(1)
		}

		if err := json.Unmarshal(f, &c.Dataconf); err != nil {
			panic(err)
		}

		// this for regenerating and renumbering purpose.
		//c.toLowerCaseAndNumbered(130, "optional.dataset.json")
		// for renumbering
		//c.reNumber(500, "optional.dataset.json")
	}

	f, err = ioutil.ReadFile(defualtconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &c.Dataconf); err != nil {
		panic(err)
	}

	f, err = ioutil.ReadFile(sourcedataconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
	if err := json.Unmarshal(f, &c.Dataconf); err != nil {
		panic(err)
	}

	//c.toLowerCaseAndNumbered(30, "default.dataset.json")

	c.DataconfIDIntToString = map[uint32]string{}
	c.DataconfIDStringToInt = map[string]uint32{}
	c.DataconfIDToPageKey = map[uint32]string{}
	pager := &util.Pagekey{}
	pager.Init()

	c.FilterableDatasets = map[string]bool{}

	var aliasmap = map[string]map[string]string{}

	for key, value := range c.Dataconf {
		if _, ok := value["aliases"]; ok { // aliases
			aliases := strings.Split(value["aliases"], ",")
			for _, ali := range aliases {
				if _, ok := aliasmap[ali]; ok {
					panic("Configuration error alias has defined before ->" + ali)
				}
				aliasmap[ali] = value
			}
		}

		if _, ok := value["hasFilter"]; ok && value["hasFilter"] == "yes" {

			c.FilterableDatasets[key] = true

		}

		c.DataconfIDToPageKey[0] = pager.Key(0, 2) // for link dataset
		if _, ok := value["id"]; ok {
			id, err := strconv.Atoi(value["id"])
			if err != nil {
				panic("Invalid identifier for dataset ->" + key)
			}
			idint := uint32(id)

			if _, ok := c.DataconfIDIntToString[idint]; !ok {
				c.DataconfIDIntToString[idint] = key
				c.DataconfIDStringToInt[key] = idint
				c.DataconfIDToPageKey[idint] = pager.Key(id, 2)
			} else {
				panic("identifier for dataset ->" + key + " already used choose new unique one")
			}

		} else {
			panic("Invalid configuration dataset must have unique integer id")
		}

	}

	for k, v := range aliasmap {
		if _, ok := c.Dataconf[k]; !ok {
			c.Dataconf[k] = cloneDataConf(v)
			c.Dataconf[k]["_alias"] = "true"
		} else {
			panic("Alias cannot be same with id->" + k)
		}
	}

	if len(outDir) > 0 {
		c.Appconf["outDir"] = outDir
	}
	_, ok := c.Appconf["outDir"]
	if !ok {
		c.Appconf["outDir"] = c.Appconf["rootDir"] + "out"
	}

	_, ok = c.Appconf["dbDir"]
	if !ok {
		c.Appconf["dbDir"] = c.Appconf["outDir"] + "/db"
	}

	_, ok = c.Appconf["aliasDbDir"]
	if !ok {
		c.Appconf["aliasDbDir"] = c.Appconf["outDir"] + "/aliasdb"
	}

	_, ok = c.Appconf["indexDir"]
	if !ok {
		c.Appconf["indexDir"] = c.Appconf["outDir"] + "/index"
	}

	_, ok = c.Appconf["idDir"]
	if !ok {
		c.Appconf["idDir"] = c.Appconf["outDir"] + "/alias"
	}

	_, ok = c.Appconf["ensemblDir"]
	if !ok {
		c.Appconf["ensemblDir"] = c.Appconf["rootDir"] + "conf/ensembl"
	}

	//create dirs if missing
	//todo check error properly
	_ = os.Mkdir(filepath.FromSlash(c.Appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(c.Appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(c.Appconf["idDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(c.Appconf["dbDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(c.Appconf["aliasDbDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(c.Appconf["ensemblDir"]), 0700)

	if _, ok := c.Appconf["fileBufferSize"]; ok {
		fileBufSize, err = strconv.Atoi(c.Appconf["fileBufferSize"])
		if err != nil {
			panic("Invalid kvgenCount definition")
		}
	}

}

func (c *Conf) retrieveWebFiles() {

	_ = os.Mkdir("website", 0700)

	log.Println("Downloading web interface files.")

	indexFile := filepath.FromSlash("website/index.html")
	downloadFile(c.githubRawPath+"/web/dist/index.html", indexFile)
	mysytleFile := filepath.FromSlash("website/mystyles.css")
	downloadFile(c.githubRawPath+"/web/dist/mystyles.css", mysytleFile)

	jsFolderPath := filepath.FromSlash("website/js")
	cssFolderPath := filepath.FromSlash("website/css")
	_ = os.Mkdir(jsFolderPath, 0700)
	_ = os.Mkdir(cssFolderPath, 0700)

	jspath := c.githubContentPath + "web/dist/js?ref=" + c.versionTag

	data, err := downloadFileBytes(jspath)
	if err != nil {
		panic(err)
	}
	jsresults := []gitContent{}
	if err := json.Unmarshal(data, &jsresults); err != nil {
		panic(err)
	}

	for _, content := range jsresults {
		jsFile := filepath.FromSlash(jsFolderPath + "/" + content.Name)
		downloadFile(content.RawURL, jsFile)
	}

	csspath := c.githubContentPath + "web/dist/css?ref=" + c.versionTag

	data, err = downloadFileBytes(csspath)
	if err != nil {
		panic(err)
	}
	cssresults := []gitContent{}
	if err := json.Unmarshal(data, &cssresults); err != nil {
		panic(err)
	}

	for _, content := range cssresults {
		cssFile := filepath.FromSlash(cssFolderPath + "/" + content.Name)
		downloadFile(content.RawURL, cssFile)
	}

	log.Println("files downloaded.")

}

func (c *Conf) checkForNewVersion() {

	resp, err := http.Get(latestReleasePath)
	if err != nil {
		log.Println("Warn: Versions data could not recieved.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Warn: Versions data could not recieved from github api.")
		return
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Warn: Versions data could not recieved from github api.")
		return
	}

	latestRelease := gitLatestRelease{}
	if err := json.Unmarshal(data, &latestRelease); err != nil {
		log.Println("Warn: Versions data could not parsed.")
		return
	}

	if latestRelease.Tag != c.versionTag {
		log.Println("Warning: There is a new biobtree version available to download")
	}

}

func downloadFile(url string, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Status error: %v", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Read body: %v", err)
	}

	err = ioutil.WriteFile(dest, data, 0700)
	return err
}

func downloadFileBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Status error: %v", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Read body: %v", err)
	}

	return data, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func (c *Conf) toLowerCaseAndNumbered(start int, datasetfile string) {

	var b strings.Builder
	b.WriteString("{")

	var ids []string

	for k := range c.Dataconf {
		ids = append(ids, k)
	}

	sort.Strings(ids)

	for _, k := range ids {

		v := c.Dataconf[k]

		//id := c.Dataconf[k]["id"]

		lowerID := strings.ToLower(k)

		b.WriteString("\"" + lowerID + "\":{")

		if _, ok := v["aliases"]; ok {
			fmt.Println(k, " dataset has aliases update manually")
		}

		if lowerID != k {
			b.WriteString("\"aliases\":\"" + k + "\",")
		}
		if len(c.Dataconf[k]["name"]) > 0 {
			b.WriteString("\"name\":\"" + c.Dataconf[k]["name"] + "\",")
		} else {
			b.WriteString("\"name\":\"" + k + "\",")
		}

		b.WriteString("\"id\":\"" + strconv.Itoa(start) + "\",")

		b.WriteString("\"url\":\"" + c.Dataconf[k]["url"] + "\"},")

		start++

	}
	s := b.String()
	s = s[:len(s)-1]
	s = s + "}"
	ioutil.WriteFile("conf/new"+datasetfile, []byte(s), 0700)

}

func (c *Conf) reNumber(start int, datasetfile string) {

	var b strings.Builder
	b.WriteString("{")

	var ids []string

	for k := range c.Dataconf {
		ids = append(ids, k)
	}

	sort.Strings(ids)

	for _, k := range ids {

		v := c.Dataconf[k]

		b.WriteString("\"" + k + "\":{")

		if _, ok := v["aliases"]; ok {
			b.WriteString("\"aliases\":\"" + v["aliases"] + "\",")
		}

		if len(v["name"]) > 0 {
			b.WriteString("\"name\":\"" + v["name"] + "\",")
		} else {
			b.WriteString("\"name\":\"" + k + "\",")
		}

		if len(v["hasFilter"]) > 0 {
			b.WriteString("\"hasFilter\":\"" + v["hasFilter"] + "\",")
		}

		b.WriteString("\"id\":\"" + strconv.Itoa(start) + "\",")

		b.WriteString("\"url\":\"" + v["url"] + "\"},")

		start++

	}
	s := b.String()
	s = s[:len(s)-1]
	s = s + "}"
	ioutil.WriteFile("conf/new"+datasetfile, []byte(s), 0700)

}

func (c *Conf) createReverseConf() {

	os.Remove("conf/reverseconf.json")

	var b strings.Builder
	b.WriteString("{")
	keymap := map[string]bool{}
	for k := range c.Dataconf {
		id := c.Dataconf[k]["id"]
		if _, ok := keymap[id]; !ok {
			b.WriteString("\"" + id + "\":{")

			if len(c.Dataconf[k]["name"]) > 0 {
				b.WriteString("\"name\":\"" + c.Dataconf[k]["name"] + "\",")
			} else {
				b.WriteString("\"name\":\"" + k + "\",")
			}

			b.WriteString("\"url\":\"" + c.Dataconf[k]["url"] + "\"},")
			keymap[id] = true
		}
	}
	s := b.String()
	s = s[:len(s)-1]
	s = s + "}"
	ioutil.WriteFile("conf/reverseconf.json", []byte(s), 0700)

}

func cloneDataConf(confVal map[string]string) map[string]string {

	var clone = map[string]string{}
	for key, val := range confVal {
		clonekey := key
		cloneval := val
		clone[clonekey] = cloneval
	}
	return clone
}

func (c *Conf) CleanOutDirs() {

	err := os.RemoveAll(filepath.FromSlash(c.Appconf["outDir"]))

	if err != nil {
		log.Fatal("Error cleaning the out dir check you have right permission")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(c.Appconf["outDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", c.Appconf["outDir"], "check you have right permission ")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(c.Appconf["indexDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", c.Appconf["indexDir"], "check you have right permission ")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(c.Appconf["dbDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", c.Appconf["dbDir"], "check you have right permission ")
		panic(err)
	}

	err = os.Mkdir(filepath.FromSlash(c.Appconf["idDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", c.Appconf["dbDir"], "check you have right permission ")
		panic(err)
	}

}
