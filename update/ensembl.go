package update

import (
	"biobtree/pbuf"
	"bufio"
	json "encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	"github.com/tamerh/jsparser"
)

type ensembl struct {
	source               string
	branch               pbuf.Ensemblbranch
	d                    *DataUpdate
	pauseDurationSeconds int
}

type ensemblPaths struct {
	Version  int                 `json:"version"`
	Jsons    map[string][]string `json:"jsons"`
	Biomarts map[string][]string `json:"biomarts"`
	Gff3s    map[string][]string `json:"gff3s"`
}

func (e *ensembl) getEnsemblPaths() (*ensemblPaths, string) {

	ensembls := ensemblPaths{}
	pathFile := filepath.FromSlash(config.Appconf["ensemblDir"] + "/" + e.source + ".paths.json")

	f, err := os.Open(pathFile)
	check(err)
	b, err := ioutil.ReadAll(f)
	check(err)
	err = json.Unmarshal(b, &ensembls)
	check(err)

	var ftpAddress string
	switch e.source {
	case "ensembl":
		ftpAddress = config.Appconf["ensembl_ftp"]
	case "ensembl_bacteria":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
	case "ensembl_fungi":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
	case "ensembl_metazoa":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
	case "ensembl_plants":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
	case "ensembl_protists":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
	}

	return &ensembls, ftpAddress

}

func (e *ensembl) updateEnsemblPaths(version int) (*ensemblPaths, string) {

	var branch string
	var ftpAddress string
	var ftpJSONPath string
	var ftpGFF3Path string
	var ftpMysqlPath string
	var ftpBiomartFolder string
	var err error

	switch e.source {

	case "ensembl":
		ftpAddress = config.Appconf["ensembl_ftp"]
		ftpJSONPath = config.Appconf["ensembl_ftp_json_path"]
		ftpGFF3Path = config.Appconf["ensembl_ftp_gff3_path"]
		ftpMysqlPath = config.Appconf["ensembl_ftp_mysql_path"]
		branch = "ensembl"
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
		ftpGFF3Path = strings.Replace(config.Appconf["ensembl_genomes_gff3_json_path"], "$(branch)", "metazoa", 1)
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

	ensembls := ensemblPaths{Jsons: map[string][]string{}, Biomarts: map[string][]string{}, Gff3s: map[string][]string{}, Version: version}

	setJSONs := func() {

		client := e.d.ftpClient(ftpAddress)
		entries, err := client.List(ftpJSONPath)
		check(err)

		for _, file := range entries {
			if strings.HasSuffix(file.Name, "_collection") {
				//client := e.d.ftpClient(ftpAddress)
				entries2, err := client.List(ftpJSONPath + "/" + file.Name)
				check(err)
				for _, file2 := range entries2 {
					ensembls.Jsons[file2.Name] = append(ensembls.Jsons[file2.Name], ftpJSONPath+"/"+file.Name+"/"+file2.Name+"/"+file2.Name+".json")
				}
				time.Sleep(time.Duration(e.pauseDurationSeconds/2) * time.Second) // for not to kicked out from ensembl ftp

			} else {
				ensembls.Jsons[file.Name] = append(ensembls.Jsons[file.Name], ftpJSONPath+"/"+file.Name+"/"+file.Name+".json")
			}
		}
		client.Quit()

	}

	setBiomarts := func() {

		// todo setup biomart release not handled at the moment
		if e.d.ensemblRelease == "" {
			// find the biomart folder
			client := e.d.ftpClient(ftpAddress)
			entries, err := client.List(ftpMysqlPath + "/" + branch + "_mart_*")
			check(err)
			//ee := ftpMysqlPath + "/" + branch + "_mart_*"
			//fmt.Println(ee)
			if len(entries) != 1 {
				log.Fatal("Error: Expected to find 1 biomart folder but found ", +len(entries))
			}
			if len(entries) == 1 {
				ftpBiomartFolder = entries[0].Name
			}
			//fmt.Println("biomart folder name", ftpBiomartFolder)
			//fmt.Println("mysqlpath", ftpMysqlPath)

		}

		//fmt.Println("biomart folder nam", ftpBiomartFolder)
		//fmt.Println("mysqlpath", ftpMysqlPath)

		client := e.d.ftpClient(ftpAddress)
		entries, err := client.List(ftpMysqlPath + "/" + ftpBiomartFolder + "/*__efg_*.gz")
		check(err)
		//var biomartFiles []string
		for _, file := range entries {
			species := strings.Split(file.Name, "_")[0]

			ensembls.Biomarts[species] = append(ensembls.Biomarts[species], ftpMysqlPath+"/"+ftpBiomartFolder+"/"+file.Name)

		}
		client.Quit()

	}

	setGFF3 := func() {

		client := e.d.ftpClient(ftpAddress)
		entries, err := client.List(ftpGFF3Path)
		check(err)

		for _, file := range entries {
			if strings.HasSuffix(file.Name, "_collection") {
				entriesSub, err := client.List(ftpGFF3Path + "/" + file.Name)
				check(err)
				for _, file2 := range entriesSub {

					entriesSubSub, err := client.List(ftpGFF3Path + "/" + file.Name + "/" + file2.Name)
					check(err)
					for _, file3 := range entriesSubSub {

						if strings.HasSuffix(file3.Name, "chr.gff3.gz") || strings.HasSuffix(file3.Name, "chromosome.Chromosome.gff3.gz") {
							ensembls.Gff3s[file2.Name] = append(ensembls.Gff3s[file2.Name], ftpGFF3Path+"/"+file.Name+"/"+file2.Name+"/"+file3.Name)
						}

					}

				}
				//time.Sleep(time.Duration(e.pauseDurationSeconds/2) * time.Second) // for not to kicked out from ensembl ftp

			} else {

				entriesSub, err := client.List(ftpGFF3Path + "/" + file.Name)
				check(err)
				for _, file2 := range entriesSub {
					if strings.HasSuffix(file2.Name, "chr.gff3.gz") || strings.HasSuffix(file2.Name, "chromosome.Chromosome.gff3.gz") {
						ensembls.Gff3s[file.Name] = append(ensembls.Gff3s[file.Name], ftpGFF3Path+"/"+file.Name+"/"+file2.Name)
					}
				}

			}
		}
		client.Quit()

	}

	switch e.source {

	case "ensembl":
		setJSONs()
		setBiomarts()
		setGFF3()
	case "ensembl_bacteria":
		setJSONs()
		setGFF3()
	case "ensembl_fungi":
		setJSONs()
		setBiomarts()
		setGFF3()
	case "ensembl_metazoa":
		setJSONs()
		setBiomarts()
		setGFF3()
	case "ensembl_plants":
		setJSONs()
		setBiomarts()
		setGFF3()
	case "ensembl_protists":
		setJSONs()
		setBiomarts()
		setGFF3()
	}

	data, err := json.Marshal(ensembls)
	check(err)

	ioutil.WriteFile(filepath.FromSlash(config.Appconf["ensemblDir"]+"/"+e.source+".paths.json"), data, 0770)

	return &ensembls, ftpAddress

}

func (e *ensembl) getEnsemblSetting(ensemblType string) (string, string, map[string][]string, map[string][]string, []string) {

	//set files
	gff3FilePaths := map[string][]string{}
	jsonFilePaths := map[string][]string{}
	var biomartFilePaths []string
	fr := config.Dataconf["ensembl"]["id"]

	if len(e.d.selectedEnsemblSpecies) == 1 && e.d.selectedEnsemblSpecies[0] == "all" {
		e.d.selectedEnsemblSpecies = nil
		e.d.selectedEnsemblSpeciesPattern = nil
	}

	ensemblPaths, ftpAddress := e.getEnsemblPaths()

	if len(e.d.selectedEnsemblSpecies) == 0 && len(e.d.selectedEnsemblSpeciesPattern) == 0 { // if all selected

		//gff3
		for sp, v := range ensemblPaths.Gff3s {
			for _, vv := range v {
				if _, ok := gff3FilePaths[sp]; !ok {
					var paths []string
					paths = append(paths, vv)
					gff3FilePaths[sp] = paths
				} else {
					paths := gff3FilePaths[sp]
					paths = append(paths, vv)
					gff3FilePaths[sp] = paths
				}
			}
		}

		//jsons
		for sp, v := range ensemblPaths.Jsons {
			for _, vv := range v {
				if _, ok := jsonFilePaths[sp]; !ok {
					var paths []string
					paths = append(paths, vv)
					jsonFilePaths[sp] = paths
				} else {
					paths := jsonFilePaths[sp]
					paths = append(paths, vv)
					jsonFilePaths[sp] = paths
				}
			}
		}

		//biomarts
		for _, v := range ensemblPaths.Biomarts {
			for _, vv := range v {
				biomartFilePaths = append(biomartFilePaths, vv)
			}
		}

	} else {

		if len(e.d.selectedEnsemblSpeciesPattern) > 0 { // if pattern selected

			e.d.selectedEnsemblSpecies = nil

			for _, pattern := range e.d.selectedEnsemblSpeciesPattern {

				for sp, v := range ensemblPaths.Gff3s {
					if strings.Contains(strings.ToUpper(sp), strings.ToUpper(pattern)) {
						for _, vv := range v {
							if _, ok := gff3FilePaths[sp]; !ok {
								var paths []string
								paths = append(paths, vv)
								gff3FilePaths[sp] = paths
							} else {
								paths := gff3FilePaths[sp]
								paths = append(paths, vv)
								gff3FilePaths[sp] = paths
							}
						}
						e.d.selectedEnsemblSpecies = append(e.d.selectedEnsemblSpecies, sp)
					}
				}

				for sp, v := range ensemblPaths.Jsons {
					if strings.Contains(strings.ToUpper(sp), strings.ToUpper(pattern)) {
						for _, vv := range v {
							if _, ok := jsonFilePaths[sp]; !ok {
								var paths []string
								paths = append(paths, vv)
								jsonFilePaths[sp] = paths
							} else {
								paths := jsonFilePaths[sp]
								paths = append(paths, vv)
								jsonFilePaths[sp] = paths
							}
						}
					}
				}

			}

			selected := map[string]string{} // just show in file better

			for _, sp := range e.d.selectedEnsemblSpecies {
				selected[sp] = ""
			}
			data, err := json.Marshal(selected)
			check(err)

			ioutil.WriteFile(filepath.FromSlash(config.Appconf["rootDir"]+"/genomes_"+e.source+".json"), data, 0770)

			fmt.Println("Genomes are selected based on pattern. Check 'genomes_" + e.source + ".json' file in this directory for full list")

		} else { // if specific species selected

			for _, sp := range e.d.selectedEnsemblSpecies {

				if _, ok := ensemblPaths.Jsons[sp]; !ok {
					log.Println("WARN Species ->", sp, "not found in ensembl ", e.source, "if you specify multiple ensembl ignore this")
					continue
				} else {
					jsonFilePaths[sp] = ensemblPaths.Jsons[sp]
					gff3FilePaths[sp] = ensemblPaths.Gff3s[sp]
				}
			}

		}

		// set biomart for selected species
		var biomartSpeciesName string // this is just the shorcut name of species in biomart folder e.g homo_sapiens-> hsapiens
		for _, sp := range e.d.selectedEnsemblSpecies {

			splitted := strings.Split(sp, "_")
			if len(splitted) > 1 {
				biomartSpeciesName = splitted[0][:1] + splitted[len(splitted)-1]
			} else {
				panic("Unrecognized species name pattern->" + sp)
			}

			for _, vv := range ensemblPaths.Biomarts[biomartSpeciesName] {
				biomartFilePaths = append(biomartFilePaths, vv)
			}
		}

	}

	return fr, ftpAddress, gff3FilePaths, jsonFilePaths, biomartFilePaths

}

func (e *ensembl) update() {

	defer e.d.wg.Done()

	ensemblTranscriptID := config.Dataconf["transcript"]["id"]
	ensemblProteinID := config.Dataconf["cds"]["id"]
	orthologID := config.Dataconf["ortholog"]["id"]
	paralogID := config.Dataconf["paralog"]["id"]
	exonsID := config.Dataconf["exon"]["id"]

	//set pause setting
	e.pauseDurationSeconds = 2 // default
	if _, ok := config.Appconf["ensemblPauseDuration"]; ok {
		var err error
		e.pauseDurationSeconds, err = strconv.Atoi(config.Appconf["ensemblPauseDuration"])
		if err != nil {
			panic("Invalid ensemblPauseDuration definition")
		}
	}

	var total uint64
	var previous int64
	var start time.Time

	fr, ftpAddress, gff3Paths, jsonPaths, biomartPaths := e.getEnsemblSetting(e.source)

	// if local file just ignore ftp jsons
	if config.Dataconf["ensembl"]["useLocalFile"] == "yes" {
		jsonPaths = nil
		gff3Paths = map[string][]string{}
		biomartPaths = nil
		gff3Paths["local"] = []string{config.Dataconf["ensembl"]["path"]}
	}

	for genome, paths := range gff3Paths {
		for _, path := range paths {

			previous = 0
			start = time.Now()

			br, _, ftpFile, client, localFile, _ := e.d.getDataReaderNew("ensembl", ftpAddress, "", path)

			scanner := bufio.NewScanner(br)

			var currTranscript *pbuf.EnsemblAttr
			var currTranscriptID string
			var currGeneID string

			for scanner.Scan() {

				l := scanner.Text()

				if l[0] == '#' {
					continue
				}

				fields := strings.Split(string(l), tab)
				if len(fields) != 9 {
					log.Printf("Invalid line in gff3 has skipped %v\n", string(l))
					continue
				}

				if fields[1] == "." {
					continue // lines without source not used e.g biological_region
				}

				// SeqRegion = fields[0]
				// Source = fields[1]
				// Type = fields[2]
				// Start, _ = strconv.Atoi(fields[3])
				// End, _ = strconv.Atoi(fields[4])
				// Score, _ = strconv.ParseFloat(fields[5], 64)
				// Strand = fields[6][0] // one byte char: +, -, ., or ?
				// Phase, _ = strconv.Atoi(fields[7])
				// Attrs = map[string]string{}

				attrsMap := map[string]string{}
				var eqIndex int
				attrs := fields[8]

				for i := strings.Index(attrs, ";"); i > 0; i = strings.Index(attrs, ";") {
					eqIndex = strings.Index(attrs[:i], "=")
					attrsMap[attrs[:i][:eqIndex]] = attrs[:i][eqIndex+1:]
					attrs = attrs[i+1:]
				}

				eqIndex = strings.Index(attrs, "=")
				attrsMap[attrs[:eqIndex]] = attrs[eqIndex+1:]

				if _, ok := attrsMap["ID"]; ok {
					idAttr := strings.Split(attrsMap["ID"], ":")
					if len(idAttr) != 2 {
						continue
					}
					switch idAttr[0] {
					case "gene":
						attr := pbuf.EnsemblAttr{}

						currGeneID = idAttr[1]

						attr.Branch = e.branch

						if _, ok := attrsMap["Name"]; ok {
							attr.Name = attrsMap["Name"]
							e.d.addXref(attrsMap["Name"], textLinkID, currGeneID, "ensembl", true)
						}

						if _, ok := attrsMap["description"]; ok {
							attr.Description = attrsMap["description"]
						}

						if _, ok := attrsMap["biotype"]; ok {
							attr.Biotype = attrsMap["biotype"]
						}

						attr.Genome = genome

						if fields[6] != "." {
							attr.Strand = fields[6]
						}

						attr.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							attr.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							attr.End = int32(c)
						}

						b, _ := ffjson.Marshal(attr)
						e.d.addProp3(idAttr[1], fr, b)

					case "transcript":
						// first write current transcript
						if currTranscript != nil {
							b, _ := ffjson.Marshal(currTranscript)
							e.d.addProp3(currTranscriptID, ensemblTranscriptID, b)
						}

						currTranscript = &pbuf.EnsemblAttr{}

						currTranscript.Source = fields[1]

						currTranscriptID = idAttr[1]
						e.d.addXref(currGeneID, fr, idAttr[1], "transcript", false)

						if _, ok := attrsMap["Name"]; ok {
							currTranscript.Name = attrsMap["Name"]
						}

						if _, ok := attrsMap["biotype"]; ok {
							currTranscript.Biotype = attrsMap["biotype"]
						}

						if fields[6] != "." {
							currTranscript.Strand = fields[6]
						}

						currTranscript.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							currTranscript.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							currTranscript.End = int32(c)
						}

						if _, ok := attrsMap["ccdsid"]; ok {

							ccdsid := strings.Split(attrsMap["ccdsid"], ".")[0]
							e.d.addXref(currTranscriptID, ensemblTranscriptID, ccdsid, "CCDS", false)
							e.d.addXref(currGeneID, fr, ccdsid, "CCDS", false)

						}

					case "CDS":

						attr := pbuf.EnsemblAttr{}

						if fields[6] != "." {
							attr.Strand = fields[6]
						}

						attr.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							attr.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							attr.End = int32(c)
						}

						if fields[7] != "." {
							c, err = strconv.Atoi(fields[7])
							if err == nil {
								attr.Frame = int32(c)
							}
						}

						b, _ := ffjson.Marshal(attr)
						e.d.addProp3(idAttr[1], ensemblProteinID, b)

						e.d.addXref(currTranscriptID, ensemblTranscriptID, idAttr[1], "eprotein", false)

					}
				} else if fields[2] == "exon" {

					if _, ok := attrsMap["Name"]; ok {

						e.d.addXref(currTranscriptID, ensemblTranscriptID, attrsMap["Name"], "exon", false)

						attr := pbuf.EnsemblAttr{}

						if fields[6] != "." {
							attr.Strand = fields[6]
						}

						attr.SeqRegion = fields[0]

						c, err := strconv.Atoi(fields[3])
						if err == nil {
							attr.Start = int32(c)
						}

						c, err = strconv.Atoi(fields[4])
						if err == nil {
							attr.End = int32(c)
						}

						b, _ := ffjson.Marshal(attr)
						e.d.addProp3(attrsMap["Name"], exonsID, b)

					}

				} else if fields[2] == "five_prime_UTR" {

					c, err := strconv.Atoi(fields[3])
					if err == nil {
						currTranscript.Utr5Start = int32(c)
					}

					c, err = strconv.Atoi(fields[4])
					if err == nil {
						currTranscript.Utr5End = int32(c)
					}

				} else if fields[2] == "three_prime_UTR" {

					c, err := strconv.Atoi(fields[3])
					if err == nil {
						currTranscript.Utr3Start = int32(c)
					}

					c, err = strconv.Atoi(fields[4])
					if err == nil {
						currTranscript.Utr3End = int32(c)
					}

				}

			}

			if ftpFile != nil {
				ftpFile.Close()
			}
			if localFile != nil {
				localFile.Close()
			}

			if client != nil {
				client.Quit()
			}

			// time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp

		}
	}

	if _, ok := config.Appconf["ensembl_all"]; ok && config.Appconf["ensembl_all"] == "y" {

		for _, paths := range jsonPaths {

			for _, path := range paths {

				previous = 0
				start = time.Now()

				br, _, ftpFile, client, localFile, _ := e.d.getDataReaderNew("ensembl", ftpAddress, "", path)

				p := jsparser.NewJSONParser(br, "genes").SkipProps([]string{"lineage", "evidence", "coord_system", "sifts", "xrefs", "gene_tree_id", "orthology_type", "exons"})

				for j := range p.Stream() {
					if j.ObjectVals["id"] != nil {

						elapsed := int64(time.Since(start).Seconds())
						if elapsed > previous+e.d.progInterval {
							kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
							previous = elapsed
							e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
						}

						entryid := j.ObjectVals["id"].StringVal

						if j.ObjectVals["taxon_id"] != nil {
							e.d.addXref(entryid, fr, j.ObjectVals["taxon_id"].StringVal, "taxonomy", false)
						}

						if j.ObjectVals["homologues"] != nil {
							for _, val := range j.ObjectVals["homologues"].ArrayVals {
								if val.ObjectVals["stable_id"] != nil {
									stableID := val.ObjectVals["stable_id"].StringVal
									if val.ObjectVals["genome"] != nil && j.ObjectVals["genome"] != nil && val.ObjectVals["genome"].StringVal == j.ObjectVals["genome"].StringVal {
										e.d.addXref2(entryid, fr, stableID, "paralog")
										e.d.addXref2(stableID, paralogID, stableID, "ensembl")
									} else {
										e.d.addXref2(entryid, fr, stableID, "ortholog")
										e.d.addXref2(stableID, orthologID, stableID, "ensembl")
									}
								}
							}
						}

						// maybe these values from configuration
						e.xref(j, entryid, fr, "Interpro", "interpro")
						e.xref(j, entryid, fr, "HPA", "HPA")
						e.xref(j, entryid, fr, "ArrayExpress", "ExpressionAtlas")
						e.xref(j, entryid, fr, "GENE3D", "CATHGENE3D")
						e.xref(j, entryid, fr, "MIM_GENE", "MIM")
						e.xref(j, entryid, fr, "RefSeq_peptide", "RefSeq")
						e.xref(j, entryid, fr, "EntrezGene", "GeneID")
						e.xref(j, entryid, fr, "PANTHER", "PANTHER")
						e.xref(j, entryid, fr, "Reactome", "Reactome")
						e.xref(j, entryid, fr, "RNAcentral", "RNAcentral")
						e.xref(j, entryid, fr, "Uniprot/SPTREMBL", "uniprot")
						e.xref(j, entryid, fr, "protein_id", "EMBL")
						e.xref(j, entryid, fr, "KEGG_Enzyme", "KEGG")
						e.xref(j, entryid, fr, "EMBL", "EMBL")
						e.xref(j, entryid, fr, "CDD", "CDD")
						e.xref(j, entryid, fr, "TIGRfam", "TIGRFAMs")
						e.xref(j, entryid, fr, "ChEMBL", "ChEMBL")
						e.xref(j, entryid, fr, "UniParc", "uniparc")
						e.xref(j, entryid, fr, "PDB", "PDB")
						e.xref(j, entryid, fr, "SuperFamily", "SUPFAM")
						e.xref(j, entryid, fr, "Prosite_profiles", "PROSITE")
						e.xref(j, entryid, fr, "RefSeq_mRNA", "RefSeq")
						e.xref(j, entryid, fr, "Pfam", "Pfam")
						e.xref(j, entryid, fr, "CCDS", "CCDS")
						e.xref(j, entryid, fr, "Prosite_patterns", "PROSITE")
						e.xref(j, entryid, fr, "Uniprot/SWISSPROT", "uniprot")
						e.xref(j, entryid, fr, "UCSC", "UCSC")
						e.xref(j, entryid, fr, "HGNC", "hgnc")
						e.xref(j, entryid, fr, "RefSeq_ncRNA_predicted", "RefSeq")
						e.xref(j, entryid, fr, "HAMAP", "HAMAP")
						e.xrefGO(j, entryid, fr) // go terms are also under xrefs with source information.

						if j.ObjectVals["transcripts"] != nil {
							for _, val := range j.ObjectVals["transcripts"].ArrayVals {
								tentryid := val.ObjectVals["id"].StringVal

								if val.ObjectVals["translations"] != nil {
									for _, eprotein := range val.ObjectVals["translations"].ArrayVals {
										e.xref(eprotein, eprotein.ObjectVals["id"].StringVal, ensemblProteinID, "Uniprot/SWISSPROT", "uniprot")
										e.xref(eprotein, eprotein.ObjectVals["id"].StringVal, ensemblProteinID, "Uniprot/SPTREMBL", "uniprot")
									}
								}

								e.xref(val, tentryid, ensemblTranscriptID, "Interpro", "interpro")
								e.xref(val, tentryid, ensemblTranscriptID, "HPA", "HPA")
								e.xref(val, tentryid, ensemblTranscriptID, "ArrayExpress", "ExpressionAtlas")
								e.xref(val, tentryid, ensemblTranscriptID, "GENE3D", "CATHGENE3D")
								e.xref(val, tentryid, ensemblTranscriptID, "MIM_GENE", "MIM")
								e.xref(val, tentryid, ensemblTranscriptID, "RefSeq_peptide", "RefSeq")
								e.xref(val, tentryid, ensemblTranscriptID, "EntrezGene", "GeneID")
								e.xref(val, tentryid, ensemblTranscriptID, "PANTHER", "PANTHER")
								e.xref(val, tentryid, ensemblTranscriptID, "Reactome", "Reactome")
								e.xref(val, tentryid, ensemblTranscriptID, "RNAcentral", "RNAcentral")
								e.xref(val, tentryid, ensemblTranscriptID, "Uniprot/SPTREMBL", "uniprot")
								e.xref(val, tentryid, ensemblTranscriptID, "protein_id", "EMBL")
								e.xref(val, tentryid, ensemblTranscriptID, "KEGG_Enzyme", "KEGG")
								e.xref(val, tentryid, ensemblTranscriptID, "EMBL", "EMBL")
								e.xref(val, tentryid, ensemblTranscriptID, "CDD", "CDD")
								e.xref(val, tentryid, ensemblTranscriptID, "TIGRfam", "TIGRFAMs")
								e.xref(val, tentryid, ensemblTranscriptID, "ChEMBL", "ChEMBL")
								e.xref(val, tentryid, ensemblTranscriptID, "UniParc", "uniparc")
								e.xref(val, tentryid, ensemblTranscriptID, "PDB", "PDB")
								e.xref(val, tentryid, ensemblTranscriptID, "SuperFamily", "SUPFAM")
								e.xref(val, tentryid, ensemblTranscriptID, "Prosite_profiles", "PROSITE")
								e.xref(val, tentryid, ensemblTranscriptID, "RefSeq_mRNA", "RefSeq")
								e.xref(val, tentryid, ensemblTranscriptID, "Pfam", "Pfam")
								e.xref(val, tentryid, ensemblTranscriptID, "CCDS", "CCDS")
								e.xref(val, tentryid, ensemblTranscriptID, "Prosite_patterns", "PROSITE")
								e.xref(val, tentryid, ensemblTranscriptID, "Uniprot/SWISSPROT", "uniprot")
								e.xref(val, tentryid, ensemblTranscriptID, "UCSC", "UCSC")
								e.xref(val, tentryid, ensemblTranscriptID, "Uniprot_gn", "uniprot")
								e.xref(val, tentryid, ensemblTranscriptID, "HGNC", "hgnc")
								e.xref(val, tentryid, ensemblTranscriptID, "RefSeq_ncRNA_predicted", "RefSeq")
								e.xref(val, tentryid, ensemblTranscriptID, "HAMAP", "HAMAP")
								e.xrefGO(val, tentryid, ensemblTranscriptID)

							}
						}
					}
					total++
				}

				if ftpFile != nil {
					ftpFile.Close()
				}
				if localFile != nil {
					localFile.Close()
				}

				if client != nil {
					client.Quit()
				}

				time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp
			}

		}
	}

	previous = 0
	totalRead := 0
	start = time.Now()
	// probset biomart
	for _, path := range biomartPaths {
		// first get the probset machine name
		f := strings.Split(path, "/")
		probsetMachine := strings.Split(f[len(f)-1], "__")[1][4:]
		probsetConf := config.Dataconf[probsetMachine]

		if probsetConf != nil {
			fr2 := config.Dataconf[probsetMachine]["id"]
			br2, _, ftpFile2, client, localFile2, _ := e.d.getDataReaderNew(probsetMachine, ftpAddress, "", path)

			scanner := bufio.NewScanner(br2)
			for scanner.Scan() {

				elapsed := int64(time.Since(start).Seconds())
				if elapsed > previous+e.d.progInterval {
					kbytesPerSecond := int64(totalRead) / elapsed / 1024
					previous = elapsed
					e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
				}
				s := scanner.Text()
				t := strings.Split(s, "\t")
				if len(t) == 3 && t[2] != "\\N" && t[1] != "\\N" {
					e.d.addXref(t[2], fr2, t[1], "transcript", false)
				}
				totalRead = totalRead + len(s) + 1
			}
			if ftpFile2 != nil {
				ftpFile2.Close()
			}
			if localFile2 != nil {
				localFile2.Close()
			}

			if client != nil {
				client.Quit()
			}

		} else {
			log.Println("Warn: new prob mapping found. It must be defined in configuration", probsetMachine)
		}
		// time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp
	}

	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
	atomic.AddUint64(&e.d.totalParsedEntry, total)

	e.d.addEntryStat(e.source, total)

}

func (e *ensembl) xref(j *jsparser.JSON, entryid, from, propName, dbid string) {

	if j.ObjectVals[propName] != nil {
		for _, val := range j.ObjectVals[propName].ArrayVals {
			e.d.addXref(entryid, from, val.StringVal, dbid, false)
		}
	}
}

func (e *ensembl) xrefGO(j *jsparser.JSON, entryid, from string) {

	if j.ObjectVals["GO"] != nil {
		for _, val := range j.ObjectVals["GO"].ArrayVals {
			if _, ok := val.ObjectVals["term"]; ok {
				e.d.addXref(entryid, from, val.ObjectVals["term"].StringVal, "GO", false)
			}
		}
	}

}

//xrefProps := []string{"name", "description", "start", "end", "biotype", "genome", "strand", "seq_region_name"}
func (e *ensembl) xrefProp(j *jsparser.JSON, entryid, from string) {

	attr := pbuf.EnsemblAttr{}

	attr.Branch = e.branch

	if j.ObjectVals["name"] != nil {
		attr.Name = j.ObjectVals["name"].StringVal
	}

	if j.ObjectVals["description"] != nil {
		attr.Description = j.ObjectVals["description"].StringVal
	}

	if j.ObjectVals["biotype"] != nil {
		attr.Biotype = j.ObjectVals["biotype"].StringVal
	}

	if j.ObjectVals["genome"] != nil {
		attr.Genome = j.ObjectVals["genome"].StringVal
	}

	if j.ObjectVals["strand"] != nil {
		attr.Strand = j.ObjectVals["strand"].StringVal
	}

	if j.ObjectVals["seq_region_name"] != nil {
		attr.SeqRegion = j.ObjectVals["seq_region_name"].StringVal
	}

	if j.ObjectVals["start"] != nil {
		c, err := strconv.Atoi(j.ObjectVals["start"].StringVal)
		if err == nil {
			attr.Start = int32(c)
		}
	}

	if j.ObjectVals["end"] != nil {
		c, err := strconv.Atoi(j.ObjectVals["end"].StringVal)
		if err == nil {
			attr.End = int32(c)
		}
	}

	b, _ := ffjson.Marshal(attr)
	e.d.addProp3(entryid, from, b)

}

func fileExists(name string) bool {

	if _, err := os.Stat(name); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		// Schrodinger: file may or may not exist. See err for details.
		// Therefore, do *NOT* use !os.IsNotExist(err) to test for file existence
		check(err)
		return false
	}
}
