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
	"sync"
	"time"

	"github.com/bmatsuo/lmdb-go/lmdb"
	"github.com/urfave/cli"
)

const version = "1.0.0-rc1"
const versionTag = "v1.0.0-rc1"

const confURLPath string = "https://raw.githubusercontent.com/tamerh/biobtree/" + versionTag

var dataconf map[string]map[string]string
var appconf map[string]string

var fileBufSize = 65536

const newlinebyte = byte('\n')

var chunkLen int
var chunkIdx string

func main() {

	app := cli.NewApp()
	app.Name = "biobtree"
	app.Version = version
	app.Usage = "Bioinformatics tool for search, map, and visualise bionformatics identifers and special keywords with their refered identifiers."
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
			Name: "datasets,d",
			//Value: "hgnc",
			Value: "uniprot_reviewed,taxonomy,hgnc,chebi,interpro,my_data,literature_mappings,hmdb",
			Usage: "change default source datasets. list of datasets are uniprot_reviewed,taxonomy,hgnc,chebi,interpro,uniprot_unreviewed,uniparc,uniref50,uniref90,my_data,literature_mappings,hmdb",
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

				chunkIdxx := c.GlobalString("idx")
				if len(chunkIdxx) > 0 {
					chunkIdx = chunkIdxx
				} else {
					chunkIdx = strconv.Itoa(time.Now().Nanosecond())
				}

				clean := c.GlobalBool("clean")
				if clean {
					cleanOutDirs()
				}

				cpu := c.GlobalInt(" maxcpu")
				if cpu > 1 {
					runtime.GOMAXPROCS(cpu)
				}

				updateData(d, ts)

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

				web := web{}
				web.start()

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
				if len(chunkIdxx) > 0 {
					chunkIdx = chunkIdxx
				} else {
					chunkIdx = strconv.Itoa(time.Now().Nanosecond())
				}

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

	var wg sync.WaitGroup
	var d = merge{
		wg: &wg,
	}

	keywrite, uidindx, links := d.Merge()

	wg.Wait()

	return keywrite, uidindx, links

}
func updateData(datasets []string, targetDatasets []string) (uint64, uint64) {

	log.Println("Update RUNNING...datasets->", datasets)
	log.Println("Note: Some datasets does not support progress bar. For any issue or error please contact from github page.")

	targetDatasetMap := map[string]bool{}

	if len(targetDatasets) > 0 {

		for _, dt := range targetDatasets {
			if _, ok := dataconf[dt]; ok {
				targetDatasetMap[dt] = true
				//now aliases if exist
				if _, ok := dataconf[dt]["aliases"]; ok {
					aliases := strings.Split(dataconf[dt]["aliases"], ",")
					for _, ali := range aliases {
						targetDatasetMap[ali] = true
					}
				}

			}

		}
	}
	hasTargetDatasets := len(targetDatasetMap) > 0

	var wg sync.WaitGroup

	loc := appconf["uniprot_ftp"]
	uniprotftpAddr := appconf["uniprot_ftp_"+loc]
	uniprotftpPath := appconf["uniprot_ftp_"+loc+"_path"]

	ebiftp := appconf["ebi_ftp"]
	ebiftppath := appconf["ebi_ftp_path"]

	channelOverflowCap := 100000
	var err error
	if _, ok := appconf["channelOverflowCap"]; ok {
		channelOverflowCap, err = strconv.Atoi(appconf["channelOverflowCap"])
		if err != nil {
			panic("Invalid kvgenCount definition")
		}
	}

	var d = &dataUpdate{
		invalidXrefs:       NewHashMap(300),
		sampleXrefs:        NewHashMap(400),
		uniprotFtp:         uniprotftpAddr,
		uniprotFtpPath:     uniprotftpPath,
		ebiFtp:             ebiftp,
		ebiFtpPath:         ebiftppath,
		targetDatasets:     targetDatasetMap,
		hasTargets:         hasTargetDatasets,
		channelOverflowCap: channelOverflowCap,
		stats:              make(map[string]interface{}),
	}

	var e = make(chan string, channelOverflowCap)
	//var mergeGateCh = make(chan mergeInfo)
	var mergeGateCh = make(chan mergeInfo, 10000)

	d.wg = &wg
	d.kvdatachan = &e

	// chunk buffer size
	chunkLen = 10000000
	if _, ok := appconf["kvgenChunkSize"]; ok {
		var err error
		chunkLen, err = strconv.Atoi(appconf["kvgenChunkSize"])
		if err != nil {
			panic("Invalid kvgenChunkSize definition")
		}
	}

	// kv generator size
	kvGenCount := 1
	if _, ok := appconf["kvgenCount"]; ok {
		kvGenCount, err = strconv.Atoi(appconf["kvgenCount"])
		if err != nil {
			panic("Invalid kvgenCount definition")
		}
	}

	var kvgens []*kvgen
	for i := 0; i < kvGenCount; i++ {
		kv := newkvgen(strconv.Itoa(i))
		kv.dataChan = &e
		kv.mergeGateCh = &mergeGateCh
		go kv.gen()
		kvgens = append(kvgens, &kv)
	}

	var wgBmerge sync.WaitGroup
	binarymerge := mergeb{
		wg:          &wgBmerge,
		mergeGateCh: &mergeGateCh,
	}
	wgBmerge.Add(1)
	go binarymerge.start()

	// first read uniprot metadata
	d.setUniprotMeta()

	for _, data := range datasets {
		switch data {
		case "uniprot_reviewed":
			d.wg.Add(1)
			go d.updateUniprot("uniprot_reviewed")
			break
		case "taxonomy":
			d.wg.Add(1)
			go d.updateTaxonomy()
			break
		case "hgnc":
			d.wg.Add(1)
			go d.updateHgnc()
			break
		case "chebi":
			d.wg.Add(1)
			go d.updateChebi()
			break
		case "interpro":
			d.wg.Add(1)
			go d.updateInterpro()
			break
		case "uniprot_unreviewed":
			d.wg.Add(1)
			go d.updateUniprot("uniprot_unreviewed")
			break
		case "uniparc":
			d.wg.Add(1)
			go d.updateUniparc()
			break
		case "uniref50":
			d.wg.Add(1)
			go d.updateUniref("uniref50")
			break
		case "uniref90":
			d.wg.Add(1)
			go d.updateUniref("uniref90")
			break
		case "uniref100":
			d.wg.Add(1)
			go d.updateUniref("uniref100")
			break
		case "hmdb":
			d.wg.Add(1)
			go d.updateHmdb("hmdb")
			break
		case "my_data":
			if dataconf["my_data"]["active"] == "true" {
				d.wg.Add(1)
				go d.updateUniprot("my_data")
			}
			break
		case "literature_mappings":
			d.wg.Add(1)
			go d.literatureMappings("literature_mappings")
			break
		}
	}

	d.wg.Wait()

	log.Println("Data update process completed. Making last arrangments...")

	var totalkv uint64

	for i := 0; i < len(kvgens); i++ {
		var t uint64
		if len(kvgens)-1 == i {
			t = kvgens[i].close(true)
		} else {
			t = kvgens[i].close(false)
		}
		//TODO this is workaround for now prevent race condition
		time.Sleep(4 * time.Second)

		totalkv = totalkv + t
	}

	wgBmerge.Wait()
	d.stats["totalKV"] = totalkv
	data, err := json.Marshal(d.stats)
	if err != nil {
		log.Println("Error while writing meta data")
	}

	ioutil.WriteFile(filepath.FromSlash(appconf["indexDir"]+"/"+chunkIdx+".meta.json"), data, 0700)

	log.Println("All done.")

	return d.totalParsedEntry, totalkv

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

func initConf(customconfdir string) {

	confdir := "conf"
	if len(customconfdir) > 0 {
		confdir = customconfdir
	}

	exist, err := fileExists(confdir)

	if err != nil {
		panic("Error while checking file")
	}

	appConfFilePath := confdir + "/app.json"
	dataConfFilePath := confdir + "/data.json"
	sourcedataconfFilePath := confdir + "/source.json"

	if !exist {
		log.Println("Downloading configuration and license files.")
		err := os.Mkdir("conf", 0700)
		if err != nil {
			panic("Error while creating conf directory")
		}
		downloadFile(confURLPath+"/src/conf/app.json", appConfFilePath)
		downloadFile(confURLPath+"/src/conf/source.json", sourcedataconfFilePath)
		downloadFile(confURLPath+"/src/conf/data.json", dataConfFilePath)

		downloadFile(confURLPath+"/LICENSE", "LICENSE")
		downloadFile(confURLPath+"/LICENSE.lmdbgo", "LICENSE.lmdbgo")
		downloadFile(confURLPath+"/LICENSE.mdb", "LICENSE.mdb")

		log.Println("Files downloaded.")
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

func check(err error) {

	if err != nil {
		fmt.Println("Error: ", err)
		panic(err)
	}

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

func openDB(write bool, totalKV int64) (*lmdb.Env, lmdb.DBI) {

	var err error
	var env *lmdb.Env
	var dbi lmdb.DBI
	env, err = lmdb.NewEnv()
	if err != nil {
		panic("Error while setting up lmdb env")
	}
	err = env.SetMaxDBs(1)
	if err != nil {
		panic("Error while setting up lmdb max db")
	}

	//err = env.SetMapSize(30 << 30)
	var lmdbAllocSize int64
	if _, ok := appconf["lmdbAllocSize"]; ok {
		lmdbAllocSize, err = strconv.ParseInt(appconf["lmdbAllocSize"], 10, 64)
		if err != nil {
			panic("Invalid lmdbAllocSize definition")
		}
		if lmdbAllocSize <= 1 {
			panic("lmdbAllocSize must be greater than 1")
		}
	} else {
		if totalKV < 1000000 { //1M
			lmdbAllocSize = 1000000000 // 1GB
		} else if totalKV < 50000000 { //50M
			lmdbAllocSize = 5000000000 // 5GB
		} else if totalKV < 100000000 { //100M
			lmdbAllocSize = 10000000000 // 10GB
		} else if totalKV < 500000000 { //500M
			lmdbAllocSize = 50000000000 // 50GB
		} else if totalKV < 1000000000 { //1B
			lmdbAllocSize = 100000000000 // 100GB
		} else {
			lmdbAllocSize = 1000000000000 // 1TB
		}
	}

	err = env.SetMapSize(lmdbAllocSize)
	if err != nil {
		panic("Error while setting up lmdb map size")
	}

	if write {
		err = env.Open(filepath.FromSlash(appconf["dbDir"]), lmdb.WriteMap, 0700)
	} else {
		err = env.Open(appconf["dbDir"], 0, 0700)
	}

	if err != nil {
		panic(err)
	}

	staleReaders, err := env.ReaderCheck()
	if err != nil {
		panic("Error while checking lmdb stale readers.")
	}
	if staleReaders > 0 {
		log.Printf("cleared %d reader slots from dead processes", staleReaders)
	}

	err = env.Update(func(txn *lmdb.Txn) (err error) {
		dbi, err = txn.CreateDBI("mydb")
		return err
	})
	if err != nil {
		panic(err)
		//panic("Error while creating database. Clear the directory and try again.")
	}

	return env, dbi

}
