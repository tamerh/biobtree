package update

import (
	"biobtree/pbuf"
	"bufio"
	json "encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

//manage ensembl meta files under the ensembl directory.

type ensemblPaths struct {
	Version   int                 `json:"version"`
	Taxids    map[string]int      `json:"taxids"`
	TaxidsRev map[int][]string    `json:"-"`
	Jsons     map[string][]string `json:"jsons"`
	Biomarts  map[string][]string `json:"biomarts"`
	Gff3s     map[string][]string `json:"gff3s"`
}

type ensemblGLatestVersion struct {
	Version int `json:"version"`
}

// checkEnsemblUpdate checks for new Ensembl releases and updates all branch metadata files.
// Version check is based on Ensembl Genomes version (eg_version API, e.g., 62) since Ensembl
// releases both main Ensembl and Ensembl Genomes at the same time. When EG version changes,
// all branches are updated including main Ensembl.
// Note: The version stored in ensembl.paths.json will be the EG version number (e.g., 62),
// not the main Ensembl release number (e.g., 115). This is intentional as both are released together.
func checkEnsemblUpdate(du *DataUpdate) {

	if config.Appconf["disableEnsemblReleaseCheck"] != "y" {

		hasNewRelease, version := hasEnsemblNewRelease()
		if hasNewRelease {

			log.Println("Ensembl meta data is updating")
			ensembls := []ensembl{}
			ensembls = append(ensembls, ensembl{source: "ensembl_fungi", branch: pbuf.Ensemblbranch_FUNGI, d: du})
			ensembls = append(ensembls, ensembl{source: "ensembl", branch: pbuf.Ensemblbranch_ENSEMBL, d: du})
			ensembls = append(ensembls, ensembl{source: "ensembl_bacteria", branch: pbuf.Ensemblbranch_BACTERIA, d: du})
			ensembls = append(ensembls, ensembl{source: "ensembl_metazoa", branch: pbuf.Ensemblbranch_METAZOA, d: du})
			ensembls = append(ensembls, ensembl{source: "ensembl_plants", branch: pbuf.Ensemblbranch_PLANT, d: du})
			ensembls = append(ensembls, ensembl{source: "ensembl_protists", branch: pbuf.Ensemblbranch_PROTIST, d: du})

			for _, ens := range ensembls {
				ens.updateEnsemblMeta(version)
			}
			log.Println("Ensembl meta data update done")
		}
	}

}

// hasEnsemblNewRelease checks if there's a new Ensembl Genomes release by comparing
// local version (from ensembl_metazoa.paths.json) against the remote eg_version API.
// Returns true if any paths.json file is missing OR if remote version differs from local.
func hasEnsemblNewRelease() (bool, int) {

	// Check if ALL required paths.json files exist
	requiredFiles := []string{
		"ensembl.paths.json",
		"ensembl_fungi.paths.json",
		"ensembl_bacteria.paths.json",
		"ensembl_metazoa.paths.json",
		"ensembl_plants.paths.json",
		"ensembl_protists.paths.json",
	}

	for _, filename := range requiredFiles {
		pathFile := filepath.FromSlash(config.Appconf["ensemblDir"] + "/" + filename)
		if !fileExists(pathFile) {
			log.Printf("Missing ensembl file: %s - triggering metadata update", filename)
			return true, getLatestEnsemblVersion()
		}
	}

	// All files exist, now check version
	epaths := ensemblPaths{}
	pathFile := filepath.FromSlash(config.Appconf["ensemblDir"] + "/ensembl_metazoa.paths.json")
	f, err := os.Open(pathFile)
	check(err)
	b, err := ioutil.ReadAll(f)
	check(err)
	err = json.Unmarshal(b, &epaths)
	check(err)

	if _, ok := config.Appconf["ensembl_version_url"]; !ok {
		log.Fatal("Missing ensembl_version_url param")
	}

	latestVersion := getLatestEnsemblVersion()

	return latestVersion != epaths.Version, latestVersion

}

func getLatestEnsemblVersion() int {

	egversion := ensemblGLatestVersion{}
	res, err := http.Get(config.Appconf["ensembl_version_url"])
	if err != nil {
		log.Fatal("Error while getting ensembl release info from its rest service. This error could be temporary try again later or use param disableEnsemblReleaseCheck", err)
	}
	if res.StatusCode != 200 {
		log.Fatal("Error while getting ensembl release info from its rest service. This error could be temporary try again later or use param disableEnsemblReleaseCheck")
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal("Error while getting ensembl release info from its rest service.  This error could be temporary try again later or use param disableEnsemblReleaseCheck", err)
	}
	err = json.Unmarshal(body, &egversion)

	return egversion.Version
}

func (e *ensembl) getEnsemblPaths() *ensemblPaths {

	ensembls := ensemblPaths{}
	pathFile := filepath.FromSlash(config.Appconf["ensemblDir"] + "/" + e.source + ".paths.json")

	f, err := os.Open(pathFile)
	check(err)
	b, err := ioutil.ReadAll(f)
	check(err)
	err = json.Unmarshal(b, &ensembls)
	check(err)

	ensembls.TaxidsRev = map[int][]string{}
	for k, v := range ensembls.Taxids {
		if _, ok := ensembls.TaxidsRev[v]; ok {
			ensembls.TaxidsRev[v] = append(ensembls.TaxidsRev[v], k)
		} else {
			ensembls.TaxidsRev[v] = []string{k}
		}
	}

	return &ensembls

}

func (e *ensembl) updateEnsemblMeta(version int) (*ensemblPaths, string) {

	var branch string
	var ftpAddress string
	var ftpJSONPath string
	var ftpGFF3Path string
	var ftpMysqlPath string
	var ftpBiomartFolder string
	var err error
	isEG := true

	switch e.source {

	case "ensembl":
		ftpAddress = config.Appconf["ensembl_ftp"]
		ftpJSONPath = config.Appconf["ensembl_ftp_json_path"]
		ftpGFF3Path = config.Appconf["ensembl_ftp_gff3_path"]
		ftpMysqlPath = config.Appconf["ensembl_ftp_mysql_path"]
		branch = "ensembl"
		isEG = false
	case "ensembl_bacteria":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "bacteria", 1)
		ftpGFF3Path = strings.Replace(config.Appconf["ensembl_genomes_ftp_gff3_path"], "$(branch)", "bacteria", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "bacteria", 1)
		branch = "bacteria"
	case "ensembl_fungi":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "fungi", 1)
		ftpGFF3Path = strings.Replace(config.Appconf["ensembl_genomes_ftp_gff3_path"], "$(branch)", "fungi", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "fungi", 1)
		branch = "fungi"
	case "ensembl_metazoa":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "metazoa", 1)
		ftpGFF3Path = strings.Replace(config.Appconf["ensembl_genomes_ftp_gff3_path"], "$(branch)", "metazoa", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "metazoa", 1)
		branch = "metazoa"
	case "ensembl_plants":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "plants", 1)
		ftpGFF3Path = strings.Replace(config.Appconf["ensembl_genomes_ftp_gff3_path"], "$(branch)", "plants", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "plants", 1)
		branch = "plants"
	case "ensembl_protists":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "protists", 1)
		ftpGFF3Path = strings.Replace(config.Appconf["ensembl_genomes_ftp_gff3_path"], "$(branch)", "protists", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "protists", 1)
		branch = "protists"
	}

	ensembls := ensemblPaths{Taxids: map[string]int{}, Jsons: map[string][]string{}, Biomarts: map[string][]string{}, Gff3s: map[string][]string{}, Version: version}

	// first get taxidmap

	taxidMap := map[string]int{}
	taxidMapEG := map[string]int{}

	if isEG {
		taxidMapEG = e.taxidMapEG()
	} else {
		taxidMap = e.taxidMap()
	}

	setJSONs := func() {

		client := ftpClient(ftpAddress)
		entries, err := client.List(ftpJSONPath)

		if err != nil { // retry
			client.Quit()
			time.Sleep(time.Duration(e.pauseDurationSeconds*5) * time.Second)
			client = ftpClient(ftpAddress)
			entries, err = client.List(ftpJSONPath)
			check(err)
		}

		for _, file := range entries {
			// Skip metadata directories (symlinks like current_json, current_gff3)
			if strings.HasPrefix(file.Name, "current_") {
				continue
			}

			if strings.HasSuffix(file.Name, "_collection") {
				entries2, err := client.List(ftpJSONPath + "/" + file.Name)
				if err != nil { // retry
					client.Quit()
					time.Sleep(time.Duration(e.pauseDurationSeconds*5) * time.Second)
					client = ftpClient(ftpAddress)
					entries2, err = client.List(ftpJSONPath + "/" + file.Name)
					check(err)
				}
				for _, file2 := range entries2 {
					ensembls.Jsons[file2.Name] = append(ensembls.Jsons[file2.Name], ftpJSONPath+"/"+file.Name+"/"+file2.Name+"/"+file2.Name+".json")

					if isEG {
						if _, ok := taxidMapEG[file2.Name]; ok {
							ensembls.Taxids[file2.Name] = taxidMapEG[file2.Name]
						}
					} else {
						if _, ok := taxidMap[file2.Name]; ok {
							ensembls.Taxids[file2.Name] = taxidMap[file2.Name]
						} else if strings.HasPrefix(file2.Name, "mus_musculus") { //trick
							ensembls.Taxids[file2.Name] = taxidMap["mus_musculus"]
						}
					}

				}
			} else {
				ensembls.Jsons[file.Name] = append(ensembls.Jsons[file.Name], ftpJSONPath+"/"+file.Name+"/"+file.Name+".json")

				if isEG {
					if _, ok := taxidMapEG[file.Name]; ok {
						ensembls.Taxids[file.Name] = taxidMapEG[file.Name]
					}
				} else {
					if _, ok := taxidMap[file.Name]; ok {
						ensembls.Taxids[file.Name] = taxidMap[file.Name]
					} else if strings.HasPrefix(file.Name, "mus_musculus") { //trick
						ensembls.Taxids[file.Name] = taxidMap["mus_musculus"]
					}
				}

			}
		}
		client.Quit()

	}

	setBiomarts := func() {

		// Note: This only works for main Ensembl (not EnsemblGenomes)
		// EnsemblGenomes changed their biomart structure in recent years
		client := ftpClient(ftpAddress)

		pattern := ftpMysqlPath + "/" + branch + "_mart_*"
		entries, err := client.List(pattern)
		check(err)

		if len(entries) != 1 {
			log.Printf("Error: Expected to find 1 biomart folder but found %d for pattern: %s", len(entries), pattern)
			log.Fatal("Biomart folder mismatch")
		}

		ftpBiomartFolder = entries[0].Name

		entries, err = client.List(ftpMysqlPath + "/" + ftpBiomartFolder + "/*__efg_*.gz")
		check(err)

		for _, file := range entries {
			species := strings.Split(file.Name, "_")[0]

			ensembls.Biomarts[species] = append(ensembls.Biomarts[species], ftpMysqlPath+"/"+ftpBiomartFolder+"/"+file.Name)

		}
		client.Quit()

	}

	setGFF3 := func() {

		client := ftpClient(ftpAddress)
		entries, err := client.List(ftpGFF3Path)

		if err != nil { // retry
			client.Quit()
			time.Sleep(time.Duration(e.pauseDurationSeconds*5) * time.Second)
			client = ftpClient(ftpAddress)
			entries, err = client.List(ftpGFF3Path)
			check(err)
		}

		for _, file := range entries {
			// Skip metadata directories (symlinks like current_json, current_gff3)
			if strings.HasPrefix(file.Name, "current_") {
				continue
			}

			if strings.HasSuffix(file.Name, "_collection") {
				entriesSub, err := client.List(ftpGFF3Path + "/" + file.Name)

				if err != nil { // retry
					client.Quit()
					time.Sleep(time.Duration(e.pauseDurationSeconds*5) * time.Second)
					client = ftpClient(ftpAddress)
					entriesSub, err = client.List(ftpGFF3Path + "/" + file.Name)
					check(err)
				}
				for _, file2 := range entriesSub {

					entriesSubSub, err := client.List(ftpGFF3Path + "/" + file.Name + "/" + file2.Name)
					if err != nil { // retry
						client.Quit()
						time.Sleep(time.Duration(e.pauseDurationSeconds*5) * time.Second)
						client = ftpClient(ftpAddress)
						entriesSubSub, err = client.List(ftpGFF3Path + "/" + file.Name + "/" + file2.Name)
						check(err)
					}
					found := false
					for _, file3 := range entriesSubSub {

						if strings.HasSuffix(file3.Name, "chr.gff3.gz") || strings.HasSuffix(file3.Name, "chromosome.Chromosome.gff3.gz") {
							ensembls.Gff3s[file2.Name] = append(ensembls.Gff3s[file2.Name], ftpGFF3Path+"/"+file.Name+"/"+file2.Name+"/"+file3.Name)
							found = true
							break
						}

					}

					if !found { // if still not found retrieve the file with gff3.gz without abinitio
						for _, file3 := range entriesSubSub {
							if strings.HasSuffix(file3.Name, "gff3.gz") && !strings.Contains(file3.Name, "abinitio") {
								ensembls.Gff3s[file2.Name] = append(ensembls.Gff3s[file2.Name], ftpGFF3Path+"/"+file.Name+"/"+file2.Name+"/"+file3.Name)
								break
							}
						}
					}

				}

			} else {

				entriesSub, err := client.List(ftpGFF3Path + "/" + file.Name)

				if err != nil { // retry
					client.Quit()
					time.Sleep(time.Duration(e.pauseDurationSeconds*5) * time.Second)
					client = ftpClient(ftpAddress)
					entriesSub, err = client.List(ftpGFF3Path + "/" + file.Name)
					check(err)
				}
				found := false
				for _, file2 := range entriesSub {

					if strings.HasSuffix(file2.Name, "chr.gff3.gz") || strings.HasSuffix(file2.Name, "chromosome.Chromosome.gff3.gz") {
						ensembls.Gff3s[file.Name] = append(ensembls.Gff3s[file.Name], ftpGFF3Path+"/"+file.Name+"/"+file2.Name)
						found = true
						break
					}
				}
				if !found { // if still not found retrieve the file with gff3.gz without abinitio
					for _, file2 := range entriesSub {
						if strings.HasSuffix(file2.Name, "gff3.gz") && !strings.Contains(file2.Name, "abinitio") {
							ensembls.Gff3s[file.Name] = append(ensembls.Gff3s[file.Name], ftpGFF3Path+"/"+file.Name+"/"+file2.Name)
							break
						}
					}
				}

			}
		}
		client.Quit()

	}

	switch e.source {

	case "ensembl":
		setJSONs()
		setBiomarts() // Only main Ensembl has biomarts
		setGFF3()
	case "ensembl_bacteria":
		setJSONs()
		setGFF3()
	case "ensembl_fungi":
		setJSONs()
		// Skip biomarts - EnsemblGenomes changed biomart structure
		setGFF3()
	case "ensembl_metazoa":
		setJSONs()
		// Skip biomarts - EnsemblGenomes changed biomart structure
		setGFF3()
	case "ensembl_plants":
		setJSONs()
		// Skip biomarts - EnsemblGenomes changed biomart structure
		setGFF3()
	case "ensembl_protists":
		setJSONs()
		// Skip biomarts - EnsemblGenomes changed biomart structure
		setGFF3()
	}

	// Cleanup: Remove JSON entries that don't have corresponding GFF3 files
	// This handles Ensembl data inconsistencies where some species have JSON but no GFF3
	var missingGff3 []string
	for species := range ensembls.Jsons {
		if _, hasGff3 := ensembls.Gff3s[species]; !hasGff3 {
			missingGff3 = append(missingGff3, species)
		}
	}
	if len(missingGff3) > 0 {
		log.Printf("Warning: Found %d species with JSON but no GFF3 files, removing them: %v", len(missingGff3), missingGff3)
		for _, species := range missingGff3 {
			delete(ensembls.Jsons, species)
			delete(ensembls.Taxids, species)
		}
	}

	// Cleanup: Remove GFF3 entries that don't have corresponding JSON files
	// This handles the reverse case
	var missingJson []string
	for species := range ensembls.Gff3s {
		if _, hasJson := ensembls.Jsons[species]; !hasJson {
			missingJson = append(missingJson, species)
		}
	}
	if len(missingJson) > 0 {
		log.Printf("Warning: Found %d species with GFF3 but no JSON files, removing them: %v", len(missingJson), missingJson)
		for _, species := range missingJson {
			delete(ensembls.Gff3s, species)
			delete(ensembls.Taxids, species)
		}
	}

	data, err := json.Marshal(ensembls)
	check(err)

	ioutil.WriteFile(filepath.FromSlash(config.Appconf["ensemblDir"]+"/"+e.source+".paths.json"), data, 0770)

	if len(ensembls.Jsons) != len(ensembls.Gff3s) {
		log.Fatal("Ensembl json and gff3 file counts does not match")
	}

	return &ensembls, ftpAddress

}

func (e *ensembl) taxidMapEG() map[string]int {

	br, _, ftpFile, client, _, _, err := getDataReaderNew("ensembl", config.Appconf["ensembl_genomes_ftp"], "", config.Appconf["ensembl_genomes_ftp_meta_path"])
	check(err)

	if ftpFile != nil {
		defer ftpFile.Close()
	}

	if client != nil {
		defer client.Quit()
	}

	scanner := bufio.NewScanner(br)
	taxIDMapEG := map[string]int{}

	for scanner.Scan() {

		l := scanner.Text()

		if l[0] == '#' {
			continue
		}

		fields := strings.Split(string(l), tab)

		id, err := strconv.Atoi(fields[3])
		if err != nil {
			log.Fatal("invalid taxid " + fields[3])
		}

		taxIDMapEG[fields[1]] = id

	}
	return taxIDMapEG

}

func (e *ensembl) taxidMap() map[string]int {

	// Path in config is now a full FTP URL
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew("taxonomy", "", "", config.Dataconf["taxonomy"]["path"])
	check(err)

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()

	if client != nil {
		defer client.Quit()
	}

	p := xmlparser.NewXMLParser(br, "taxon").SkipElements([]string{"lineage"})

	taxNameIDMap := map[string]int{}

	for r := range p.Stream() {

		// id
		id, err := strconv.Atoi(r.Attrs["taxId"])
		if err != nil {
			log.Fatal("invalid taxid " + r.Attrs["taxId"])
		}

		name := strings.ToLower(strings.Replace(r.Attrs["scientificName"], " ", "_", -1))

		taxNameIDMap[name] = id
	}

	return taxNameIDMap

}
