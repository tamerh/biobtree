package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"./generate"
	"./service"
	"./update"
	"github.com/urfave/cli"
)

const version = "1.0.0-rc2"
const versionTag = "v1.0.0-rc2"

const latestReleasePath = "https://api.github.com/repos/tamerh/biobtree/releases/latest"

const confURLPath string = "https://raw.githubusercontent.com/tamerh/biobtree/" + versionTag

// for now they are static
var webuicssfiles = []string{"app.95380e253f42e1540222c408041dc917.css", "app.95380e253f42e1540222c408041dc917.css.map"}
var webuijsfiles = []string{"app.9310f7ea5073d514af7e.js", "app.9310f7ea5073d514af7e.js.map", "manifest.153f892e737d563221fa.js", "manifest.153f892e737d563221fa.js.map", "vendor.12f6f6a0a52ad7f1cd87.js", "vendor.12f6f6a0a52ad7f1cd87.js.map"}

var dataconf map[string]map[string]string
var appconf map[string]string

var fileBufSize = 65536
var channelOverflowCap = 100000

func main() {

	app := cli.NewApp()
	app.Name = "biobtree"
	app.Version = version
	app.Usage = "A tool to search, map and visualize bioinformatics identifiers and special keywords"
	app.Copyright = ""
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Tamer Gür",
			Email: "tgur@ebi.ac.uk",
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "uniprot_ftp,f",
			Value: "UK",
			Usage: "uniprot ftp mirrors. Switzerland, USA or UK",
		},
		cli.StringFlag{
			Name:  "datasets,d",
			Value: "ensembl_fungi",
			//Value: "uniprot_reviewed,taxonomy,hgnc,chebi,interpro,literature_mappings,hmdb",
			Usage: "change default source datasets. list of datasets are uniprot_reviewed,ensembl,taxonomy,hgnc,chebi,interpro,uniprot_unreviewed,uniparc,uniref50,uniref90,my_data,literature_mappings,hmdb,ensembl,ensembl_bacteria,ensembl_fungi,ensembl_metazoa,ensembl_plants,ensembl_protists",
		},
		cli.StringFlag{
			Name:  "target_datasets,t",
			Usage: "specify target datasets. By default all datasets are included. Speeds up process. See data.json files for full list of datasets",
		},
		cli.StringFlag{
			Name:  "idx,i",
			Usage: "for indexing in multiple machine. Set unique number or text for each process. No need to specify for single process",
		},
		cli.StringFlag{
			Name:   "confdir",
			Hidden: true,
			Usage:  "to change default config directory while developing",
		},
		cli.BoolFlag{
			Name:  "clean",
			Usage: "before starts clean all output directories.",
		},
		cli.IntFlag{
			Name:   "maxcpu",
			Hidden: true,
			Usage:  "sets the maximum number of CPUs that can be executing simultaneously. By default biobtree uses all the CPUs when applicable.",
		},
		cli.StringFlag{
			Name: "species,s",
			//Value: "acremonium_chrysogenum_atcc_11550_gca_000769265",
			Usage: "Species names for ensembl dataset",
		},
	}

	app.Commands = []cli.Command{
		{
			Name: "update",

			Usage: "update produce chunk files for selected datasets",
			Action: func(c *cli.Context) error {

				start := time.Now()

				confdir := c.GlobalString("confdir")

				initConf(confdir)

				d := strings.Split(c.GlobalString("datasets"), ",")
				appconf["uniprot_ftp"] = c.GlobalString("uniprot_ftp")

				if len(d) == 0 {
					log.Fatal("Error:datasets must be specified")
					return nil
				}

				t := c.GlobalString("target_datasets")
				var ts []string
				if len(t) > 0 {
					ts = strings.Split(t, ",")
				}

				s := c.GlobalString("species")
				var sp []string
				if len(s) > 0 {
					sp = strings.Split(s, ",")
				}

				/**
				for _, dd := range d {
					if dd == "ensembl" && len(sp) == 0 {
						log.Fatal("ERROR:When processing ensembl species must be specified")
						return nil
					}
				}
				**/

				chunkIdxx := c.GlobalString("idx")
				if len(chunkIdxx) == 0 {
					chunkIdxx = strconv.Itoa(time.Now().Nanosecond())
				}
				//fmt.Println("chunkindex", chunkIdx)

				clean := c.GlobalBool("clean")
				if clean {
					cleanOutDirs()
				}

				cpu := c.GlobalInt(" maxcpu")
				if cpu > 1 {
					runtime.GOMAXPROCS(cpu)
				}

				updateData(d, ts, sp, chunkIdxx)

				elapsed := time.Since(start)
				log.Printf("Finished took %s", elapsed)

				return nil
			},
		},
		{
			Name:  "generate",
			Usage: "merge data from update phase and generate LMDB database",
			Action: func(c *cli.Context) error {

				start := time.Now()

				confdir := c.GlobalString("confdir")

				initConf(confdir)

				cpu := c.GlobalInt(" maxcpu")
				if cpu > 1 {
					runtime.GOMAXPROCS(cpu)
				}

				mergeData()

				elapsed := time.Since(start)
				log.Printf("finished took %s", elapsed)

				return nil
			},
		},
		{
			Name:  "web",
			Usage: "runs web services and web interface",
			Action: func(c *cli.Context) error {

				confdir := c.GlobalString("confdir")

				initConf(confdir)

				cpu := c.GlobalInt(" maxcpu")
				if cpu > 1 {
					runtime.GOMAXPROCS(cpu)
				}

				web := service.Web{}
				web.Start(appconf, dataconf)

				return nil
			},
		},
		{
			Name:   "profile",
			Hidden: true,
			Usage:  "profile the",
			Action: func(c *cli.Context) error {

				confdir := c.GlobalString("confdir")

				initConf(confdir)

				os.Remove("memprof.out")
				os.Remove("cpuprof.out")

				start := time.Now()

				d := strings.Split(c.GlobalString("datasets"), ",")
				appconf["uniprot_ftp"] = c.GlobalString("uniprot_ftp")

				if len(d) == 0 {
					log.Println("Error:datasets must be specified")
					return nil
				}

				f, err := os.Create("cpuprof.out")
				if err != nil {
					log.Fatal("could not create CPU profile: ", err)
				}
				if err := pprof.StartCPUProfile(f); err != nil {
					log.Fatal("could not start CPU profile: ", err)
				}
				defer pprof.StopCPUProfile()

				chunkIdxx := c.GlobalString("idx")
				if len(chunkIdxx) == 0 {
					chunkIdxx = strconv.Itoa(time.Now().Nanosecond())
				}
				//fmt.Println("chunkindex", chunkIdx)

				///updateData(d, []string{})

				elapsed := time.Since(start)
				log.Printf("finished took %s", elapsed)

				mergeData()

				f2, err := os.Create("memprof.out")
				if err != nil {
					log.Fatal("could not create memory profile: ", err)
				}
				runtime.GC() // get up-to-date statistics
				if err := pprof.WriteHeapProfile(f2); err != nil {
					log.Fatal("could not write memory profile: ", err)
				}
				f2.Close()

				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}

func mergeData() (uint64, uint64, uint64) {

	log.Println("Generate running...")

	//var wg sync.WaitGroup
	var d = generate.Merge{
		//	wg: &wg,
	}

	keywrite, uidindx, links := d.Merge(appconf, dataconf)

	//wg.Wait()

	return keywrite, uidindx, links

}
func updateData(datasets, targetDatasets, ensemblSpecies []string, chunkIdx string) (uint64, uint64) {

	return update.NewDataUpdate(datasets, targetDatasets, ensemblSpecies, dataconf, appconf, chunkIdx).Update()

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

type gitLatestRelease struct {
	Tag string `json:"tag_name"`
}

func checkForNewVersion() {

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

	if latestRelease.Tag != versionTag {
		log.Println("ATTENTION: There is a new biobtree version available to download.")
	}

}
func initConf(customconfdir string) {

	checkForNewVersion()

	confdir := "conf"
	if len(customconfdir) > 0 {
		confdir = customconfdir
	}

	exist, err := fileExists(confdir)

	if err != nil {
		panic("Error while checking file")
	}

	appConfFilePath := filepath.FromSlash(confdir + "/app.json")
	dataConfFilePath := filepath.FromSlash(confdir + "/data.json")
	sourcedataconfFilePath := filepath.FromSlash(confdir + "/source.json")

	if !exist {
		log.Println("Downloading configuration and license files.")
		err := os.Mkdir("conf", 0700)
		if err != nil {
			panic("Error while creating conf directory")
		}
		downloadFile(confURLPath+"/src/conf/app.json", appConfFilePath)
		downloadFile(confURLPath+"/src/conf/source.json", sourcedataconfFilePath)
		downloadFile(confURLPath+"/src/conf/data.json", dataConfFilePath)

		downloadFile(confURLPath+"/LICENSE.md", "LICENSE.md")
		downloadFile(confURLPath+"/LICENSE.lmdbgo.md", "LICENSE.lmdbgo.md")
		downloadFile(confURLPath+"/LICENSE.mdb.md", "LICENSE.mdb.md")

		log.Println("Files downloaded.")
	}

	exist, err = fileExists("webui")

	if err != nil {
		panic("Error while checking file")
	}

	if !exist {
		log.Println("Downloading web interface files.")
		err := os.Mkdir("webui", 0700)
		if err != nil {
			panic("Error while creating conf directory")
		}
		staticFolderPath := filepath.FromSlash("webui/static")
		jsFolderPath := filepath.FromSlash("webui/static/js")
		cssFolderPath := filepath.FromSlash("webui/static/css")

		err = os.Mkdir(staticFolderPath, 0700)
		if err != nil {
			panic("Error while creating static directory")
		}

		err = os.Mkdir(jsFolderPath, 0700)
		if err != nil {
			panic("Error while creating js directory")
		}
		for _, file := range webuijsfiles {
			jsFile := filepath.FromSlash(jsFolderPath + "/" + file)
			downloadFile(confURLPath+"/src/webui/static/js/"+file, jsFile)
		}
		err = os.Mkdir(cssFolderPath, 0700)
		if err != nil {
			panic("Error while creating css directory")
		}
		for _, file := range webuicssfiles {
			cssFile := filepath.FromSlash(cssFolderPath + "/" + file)
			downloadFile(confURLPath+"/src/webui/static/css/"+file, cssFile)
		}
		indexFile := filepath.FromSlash("webui/index.html")
		downloadFile(confURLPath+"/src/webui/index.html", indexFile)
		log.Println("files downloaded.")
	}

	appconfFile := filepath.FromSlash(appConfFilePath)
	dataconfFile := filepath.FromSlash(dataConfFilePath)
	sourcedataconfFile := filepath.FromSlash(sourcedataconfFilePath)

	f, err := ioutil.ReadFile(appconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &appconf); err != nil {
		panic(err)
	}

	f, err = ioutil.ReadFile(dataconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &dataconf); err != nil {
		panic(err)
	}

	f, err = ioutil.ReadFile(sourcedataconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
	if err := json.Unmarshal(f, &dataconf); err != nil {
		panic(err)
	}

	//createReverseConf()

	var aliasmap = map[string]map[string]string{}
	for _, value := range dataconf {
		if _, ok := value["aliases"]; ok {
			aliases := strings.Split(value["aliases"], ",")
			for _, ali := range aliases {
				if _, ok := aliasmap[ali]; ok {
					panic("Configuration error alias has defined before ->" + ali)
				}
				aliasmap[ali] = value
			}
		}
	}

	for k, v := range aliasmap {
		dataconf[k] = cloneDataConf(v)
		dataconf[k]["_alias"] = "true"
	}

	_, ok := appconf["outDir"]
	if !ok {
		appconf["outDir"] = appconf["rootDir"] + "/out"
	}

	_, ok = appconf["rawDir"]
	if !ok {
		appconf["rawDir"] = appconf["rootDir"] + "/raw"
	}

	_, ok = appconf["dbDir"]
	if !ok {
		appconf["dbDir"] = appconf["outDir"] + "/db"
	}

	_, ok = appconf["indexDir"]
	if !ok {
		appconf["indexDir"] = appconf["outDir"] + "/index"
	}

	//create dirs if missing
	_ = os.Mkdir(filepath.FromSlash(appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)

	if _, ok := appconf["fileBufferSize"]; ok {
		fileBufSize, err = strconv.Atoi(appconf["fileBufferSize"])
		if err != nil {
			panic("Invalid kvgenCount definition")
		}
	}

	if _, ok := appconf["channelOverflowCap"]; ok {
		channelOverflowCap, err = strconv.Atoi(appconf["channelOverflowCap"])
		if err != nil {
			panic("Invalid channelOverflowCap definition")
		}
	}

}

func createReverseConf() {

	os.Remove("conf/reverseconf.json")

	var b strings.Builder
	b.WriteString("{")
	keymap := map[string]bool{}
	for k := range dataconf {
		id := dataconf[k]["id"]
		if _, ok := keymap[id]; !ok {
			b.WriteString("\"" + id + "\":{")

			if len(dataconf[k]["name"]) > 0 {
				b.WriteString("\"name\":\"" + dataconf[k]["name"] + "\",")
			} else {
				b.WriteString("\"name\":\"" + k + "\",")
			}

			b.WriteString("\"url\":\"" + dataconf[k]["url"] + "\"},")
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

func cleanOutDirs() {

	err := os.RemoveAll(filepath.FromSlash(appconf["outDir"]))

	if err != nil {
		log.Fatal("Error cleaning the out dir check you have right permission")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(appconf["outDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", appconf["outDir"], "check you have right permission ")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(appconf["indexDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", appconf["indexDir"], "check you have right permission ")
		panic(err)
	}
	err = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)
	if err != nil {
		log.Fatal("Error creating dir", appconf["dbDir"], "check you have right permission ")
		panic(err)
	}

}

func check(err error) {

	if err != nil {
		fmt.Println("Error: ", err)
		panic(err)
	}

}
