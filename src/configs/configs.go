package configs

import (
	"archive/tar"
	"archive/zip"
	"biobtree/util"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var fileBufSize = 65536
var channelOverflowCap = 100000

const latestReleasePath = "https://github.com/tamerh/biobtree/releases/latest"
const latestConfReleasePath = "https://github.com/tamerh/biobtree-conf/releases/latest"

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

	// Test mode fields
	TestMode      bool   // Whether in test mode
	TestOutputDir string // e.g., "test_out"
	TestRefDir    string // e.g., "test_out/reference"
}

func (c *Conf) Install(rootDir, bbBinaryVersion, outDir, preBuildSet string, optionalDatasetActive bool) {

	dirs := []string{}
	dirs = append(dirs, rootDir+"conf")
	dirs = append(dirs, rootDir+"website")
	dirs = append(dirs, rootDir+"out")

	for _, dir := range dirs {

		err := os.RemoveAll(filepath.FromSlash(dir))

		if err != nil {
			log.Fatalf("Error %v when deleting folder  %v\n", err, dir)
		}

	}

	latestConfVersion := c.latestConfVersion()

	c.retrConfFiles(latestConfVersion, rootDir)

	if len(preBuildSet) > 0 {
		c.retrPreBuiltDBFiles(latestConfVersion, preBuildSet, rootDir)
	}

}

func (c *Conf) Init(rootDir, bbBinaryVersion, outDir string, optionalDatasetActive bool) {

	c.versionTag = bbBinaryVersion

	confdir := rootDir + "conf"

	// STEP 1: Read application config early to check for disableRemoteConfigCheck
	appconfFile := filepath.FromSlash(filepath.FromSlash(confdir + "/application.param.json"))
	confExist, err := fileExists(appconfFile)

	disableRemoteCheck := false
	if confExist {
		f, err := ioutil.ReadFile(appconfFile)
		if err == nil {
			var tempAppconf map[string]string
			if err := json.Unmarshal(f, &tempAppconf); err == nil {
				c.Appconf = tempAppconf
				if c.Appconf["disableRemoteConfigCheck"] == "y" {
					disableRemoteCheck = true
					log.Println("Remote configuration checks disabled via config")
				}
			}
		}
	}

	// Only perform remote checks if not disabled
	var latestConfVersion string
	if !disableRemoteCheck {
		c.checkForNewVersion()
		latestConfVersion = c.latestConfVersion()
	}

	// Check if required directories exist
	confDirExist, err := fileExists(confdir)
	if err != nil {
		log.Fatal("Error while checking file")
	}

	websiteExist, err := fileExists(rootDir + "website")

	ensemblDir := confdir + "/ensembl"
	ensemblExist, err := fileExists(ensemblDir)

	// Only download if not disabled and directories are missing
	if !disableRemoteCheck && (!confDirExist || !websiteExist || !ensemblExist) {
		c.retrConfFiles(latestConfVersion, rootDir)
	}

	// Re-read application param in case it was just downloaded
	f, err := ioutil.ReadFile(appconfFile)
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &c.Appconf); err != nil {
		panic(err)
	}

	// Only check version if remote checks are enabled
	if !disableRemoteCheck && latestConfVersion != "" && c.Appconf["conf_version"] != latestConfVersion {
		log.Println("Note: There is a new data and configuration release. To use latest data install or build database again")
	}
	// set root dir if passed
	if len(rootDir) > 0 {
		c.Appconf["rootDir"] = rootDir
	}

	// STEP 2 set optional conf if activated
	if optionalDatasetActive {
		optionalconfFile := filepath.FromSlash(filepath.FromSlash(confdir + "/optional.dataset.json"))
		f, err = ioutil.ReadFile(optionalconfFile)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		if err := json.Unmarshal(f, &c.Dataconf); err != nil {
			log.Fatalf("Error: %v", err)
		}
		// this for regenerating and renumbering purpose.
		//c.toLowerCaseAndNumbered(130, "optional.dataset.json")
		// for renumbering
		//c.reNumber(500, "optional.dataset.json")
	}

	// STEP 3 read default conf file
	defualtconfFile := filepath.FromSlash(filepath.FromSlash(confdir + "/default.dataset.json"))
	f, err = ioutil.ReadFile(defualtconfFile)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	if err := json.Unmarshal(f, &c.Dataconf); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// STEP 4 read source conf files
	sourcedataconfFile := filepath.FromSlash(filepath.FromSlash(confdir + "/source.dataset.json"))
	f, err = ioutil.ReadFile(sourcedataconfFile)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	if err := json.Unmarshal(f, &c.Dataconf); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// STEP 5 set all config maps
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
					log.Fatalf("Configuration error alias has defined before -> %s", ali)
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
				log.Fatalf("Invalid identifier for dataset ->%s", key)
			}
			idint := uint32(id)

			if _, ok := c.DataconfIDIntToString[idint]; !ok {
				c.DataconfIDIntToString[idint] = key
				c.DataconfIDStringToInt[key] = idint
				c.DataconfIDToPageKey[idint] = pager.Key(id, 2)
			} else {
				log.Fatalf("identifier for dataset %s already used choose new unique one", key)
			}

		} else {
			log.Fatal("Invalid configuration dataset must have unique integer id")
		}

	}

	for k, v := range aliasmap {
		if _, ok := c.Dataconf[k]; !ok {
			c.Dataconf[k] = cloneDataConf(v)
			c.Dataconf[k]["_alias"] = "true"
		} else {
			log.Fatalf("Alias cannot be same with id -> %s", k)
		}
	}

	// STEP 6 configure output folders

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
			log.Fatal("Invalid fileBufferSize definition")
		}
	}

}

func (c *Conf) checkForNewVersion() {

	resp, err := http.Get(latestReleasePath)
	if err != nil {
		log.Println("Warn: Versions data could not recieved.")
		return
	}

	finalURL := resp.Request.URL.String()
	splitteURL := strings.Split(finalURL, "/")

	if len(splitteURL) > 0 && splitteURL[len(splitteURL)-1] != c.versionTag {
		log.Println("New biobtree version " + splitteURL[len(splitteURL)-1] + " is available to download")
	}

}

func (c *Conf) retrConfFiles(confVersion, confDir string) {

	confPath := "https://github.com/tamerh/biobtree-conf/archive/" + confVersion + ".zip"

	log.Printf("Pulling configuration files from %s \n", confPath)

	err := c.unzip(confPath, confDir, confVersion)
	if err != nil {
		log.Fatal("Unzip file", err)
	}

	log.Println("Pulling done.")

}

func (c *Conf) retrPreBuiltDBFiles(confVersion, preBuildSet, confDir string) {

	if len(preBuildSet) > 0 {

		preDataPath := "https://github.com/tamerh/biobtree-conf/releases/download/" + confVersion + "/biobtree-conf-" + confVersion + "-set" + preBuildSet + ".tar.gz"

		log.Printf("Pulling pre-build db files from %s \n", preDataPath)

		resp, err := http.Get(preDataPath)

		br := bufio.NewReaderSize(resp.Body, fileBufSize)

		uncompressedStream, err := gzip.NewReader(br)
		if err != nil {
			log.Fatal("ExtractTarGz: NewReader failed")
		}

		tarReader := tar.NewReader(uncompressedStream)

		for true {
			header, err := tarReader.Next()

			if err == io.EOF {
				break
			}

			if err != nil {
				log.Fatalf("ExtractTarGz: Next() failed: %s", err.Error())
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if err := os.Mkdir(header.Name, 0755); err != nil {
					log.Fatalf("ExtractTarGz: Mkdir() failed: %s", err.Error())
				}
			case tar.TypeReg:
				outFile, err := os.Create(header.Name)
				if err != nil {
					log.Fatalf("ExtractTarGz: Create() failed: %s", err.Error())
				}
				if _, err := io.Copy(outFile, tarReader); err != nil {
					log.Fatalf("ExtractTarGz: Copy() failed: %s", err.Error())
				}
				outFile.Close()
			default:
				log.Fatalf("ExtractTarGz: uknown type: %x in %s", header.Typeflag, header.Name)
			}

		}

		log.Println("Pulling done.")

	}

}

func (c *Conf) unzip(path string, dest, confVersion string) error {

	resp, err := http.Get(path)
	if err != nil {
		log.Fatalf("GET error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Status error: %v", resp.StatusCode)
	}

	zipcontent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Read body: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(zipcontent), int64(len(zipcontent)))

	if err != nil {
		// Show first 500 bytes of response to help debug what was actually downloaded
		preview := string(zipcontent)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		log.Printf("ERROR: Downloaded content is not a valid ZIP file")
		log.Printf("URL: %s", path)
		log.Printf("Content length: %d bytes", len(zipcontent))
		log.Printf("Content preview:\n%s", preview)
		return fmt.Errorf("zip: not a valid zip file (downloaded %d bytes from %s)", len(zipcontent), path)
	}

	if len(dest) > 0 {
		os.MkdirAll(dest, 0755)
	}

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, strings.TrimPrefix(f.Name, "biobtree-conf-"+confVersion+"/"))

		if len(path) <= 1 { // Root folder TODO CHECK windows
			return nil
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Conf) latestConfVersion() string {

	resp, err := http.Get(latestConfReleasePath)
	if err != nil {
		log.Fatal("Error while connecting github for conf files")
	}

	finalURL := resp.Request.URL.String()
	splitteURL := strings.Split(finalURL, "/")

	return splitteURL[len(splitteURL)-1]

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

func cloneDataConf(confVal map[string]string) map[string]string {

	var clone = map[string]string{}
	for key, val := range confVal {
		clonekey := key
		cloneval := val
		clone[clonekey] = cloneval
	}
	return clone
}

// Test mode helper methods

// IsTestMode checks if test mode is enabled
func (c *Conf) IsTestMode() bool {
	return c.TestMode
}

// GetTestLimit returns the test entry limit for a dataset
// Returns -1 if no limit (process full dataset) or if not in test mode
func (c *Conf) GetTestLimit(dataset string) int {
	if !c.TestMode {
		return -1 // No limit in production mode
	}

	// Normalize Ensembl division names to "ensembl"
	// e.g., "ensembl_plants" -> "ensembl", "ensembl_bacteria" -> "ensembl"
	lookupDataset := dataset
	if strings.HasPrefix(dataset, "ensembl_") {
		lookupDataset = "ensembl"
	}

	dsConfig, exists := c.Dataconf[lookupDataset]
	if !exists {
		return -1 // Dataset not found, no limit
	}

	// Check for test_entries_count parameter
	if testCountStr, ok := dsConfig["test_entries_count"]; ok {
		testCount, err := strconv.Atoi(testCountStr)
		if err != nil {
			log.Printf("Warning: invalid test_entries_count for %s: %s", dataset, testCountStr)
			return -1
		}
		return testCount
	}

	return -1 // No limit specified = full dataset
}

// GetTestSpecies returns the list of test species for Ensembl
// Returns nil if not in test mode or no species specified
func (c *Conf) GetTestSpecies() []string {
	if !c.TestMode {
		return nil
	}

	ensemblConfig, exists := c.Dataconf["ensembl"]
	if !exists {
		return nil
	}

	// Check for test_species parameter
	if speciesStr, ok := ensemblConfig["test_species"]; ok {
		// Parse comma-separated species list
		species := strings.Split(speciesStr, ",")
		// Trim whitespace
		for i := range species {
			species[i] = strings.TrimSpace(species[i])
		}
		return species
	}

	return nil
}
