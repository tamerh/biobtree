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

func (e *ensembl) updateEnsemblPaths() (*ensemblPaths, string) {

	var branch string
	var ftpAddress string
	var ftpJSONPath string
	var ftpMysqlPath string
	var ftpBiomartFolder string
	var version int
	var err error

	switch e.source {

	case "ensembl":
		ftpAddress = config.Appconf["ensembl_ftp"]
		ftpJSONPath = config.Appconf["ensembl_ftp_json_path"]
		ftpMysqlPath = config.Appconf["ensembl_ftp_mysql_path"]
		branch = "ensembl"
	case "ensembl_bacteria":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "bacteria", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "bacteria", 1)
		branch = "bacteria"
	case "ensembl_fungi":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "fungi", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "fungi", 1)
		branch = "fungi"
	case "ensembl_metazoa":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "metazoa", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "metazoa", 1)
		branch = "metazoa"
	case "ensembl_plants":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "plants", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "plants", 1)
		branch = "plants"
	case "ensembl_protists":
		ftpAddress = config.Appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "protists", 1)
		ftpMysqlPath = strings.Replace(config.Appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "protists", 1)
		branch = "protists"
	}

	ensembls := ensemblPaths{Jsons: map[string][]string{}, Biomarts: map[string][]string{}, Version: version}

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

	}

	switch e.source {

	case "ensembl":
		setJSONs()
		setBiomarts()
	case "ensembl_bacteria":
		setJSONs()
	case "ensembl_fungi":
		setJSONs()
		setBiomarts()
	case "ensembl_metazoa":
		setJSONs()
		setBiomarts()
	case "ensembl_plants":
		setJSONs()
		setBiomarts()
	case "ensembl_protists":
		setJSONs()
		setBiomarts()
	}

	data, err := json.Marshal(ensembls)
	check(err)

	ioutil.WriteFile(filepath.FromSlash(config.Appconf["ensemblDir"]+"/"+e.source+".paths.json"), data, 0770)

	return &ensembls, ftpAddress

}

func (e *ensembl) getEnsemblSetting(ensemblType string) (string, string, map[string][]string, []string) {

	//set pause setting
	e.pauseDurationSeconds = 2 // default
	if _, ok := config.Appconf["ensemblPauseDuration"]; ok {
		var err error
		e.pauseDurationSeconds, err = strconv.Atoi(config.Appconf["ensemblPauseDuration"])
		if err != nil {
			panic("Invalid ensemblPauseDuration definition")
		}
	}

	//set files
	jsonFilePaths := map[string][]string{}
	var biomartFilePaths []string
	fr := config.Dataconf["ensembl"]["id"]

	if len(e.d.selectedEnsemblSpecies) == 1 && e.d.selectedEnsemblSpecies[0] == "all" {
		e.d.selectedEnsemblSpecies = nil
		e.d.selectedEnsemblSpeciesPattern = nil
	}

	ensemblPaths, ftpAddress := e.getEnsemblPaths()

	if len(e.d.selectedEnsemblSpecies) == 0 && len(e.d.selectedEnsemblSpeciesPattern) == 0 { // if all selected

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

		for _, v := range ensemblPaths.Biomarts {
			for _, vv := range v {
				biomartFilePaths = append(biomartFilePaths, vv)
			}
		}

	} else {

		if len(e.d.selectedEnsemblSpeciesPattern) > 0 { // if pattern selected

			e.d.selectedEnsemblSpecies = nil

			for _, pattern := range e.d.selectedEnsemblSpeciesPattern {

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
						e.d.selectedEnsemblSpecies = append(e.d.selectedEnsemblSpecies, sp)
					}
				}

			}

			/**if len(e.d.selectedEnsemblSpecies) == 0 {
				log.Fatal(" ERROR No genome found based on given pattern")
			}**/

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

	/** this is for regenerating the biomart probe conf
	m := map[string]bool{}
	index := 76
	for _, path := range biomartFilePaths {
		f := strings.Split(path, "/")
		probsetMachine := strings.Split(f[len(f)-1], "__")[1][4:]
		//probsetConf := config.Dataconf[probsetMachine]
		//if probsetConf == nil {
		if _, ok := m[probsetMachine]; !ok {
			m[probsetMachine] = true
		}
		//}
	}
	marr := make([]string, len(m))
	i := 0
	for k := range m {
		marr[i] = k
		i++
	}
	sort.Strings(marr)
	for _, k := range marr {
		s := strconv.Itoa(index)
		fmt.Println(`"` + k + `": {"name": "` + k + `","id": "` + s + `","url": ""},`)
		index++
	}

	fmt.Println("Check done")
	**/

	return fr, ftpAddress, jsonFilePaths, biomartFilePaths

}

func (e *ensembl) update() {

	defer e.d.wg.Done()

	ensemblTranscriptID := config.Dataconf["transcript"]["id"]
	ensemblProteinID := config.Dataconf["eprotein"]["id"]
	orthologID := config.Dataconf["ortholog"]["id"]
	paralogID := config.Dataconf["paralog"]["id"]
	exonsID := config.Dataconf["exon"]["id"]

	var total uint64
	var previous int64
	var start time.Time

	fr, ftpAddress, jsonPaths, biomartPaths := e.getEnsemblSetting(e.source)

	// if local file just ignore ftp jsons
	if config.Dataconf["ensembl"]["useLocalFile"] == "yes" {
		jsonPaths = map[string][]string{}
		biomartPaths = nil
		jsonPaths["local"] = []string{config.Dataconf["ensembl"]["path"]}
	}

	xref := func(j *jsparser.JSON, entryid, from, propName, dbid string) {

		if j.ObjectVals[propName] != nil {
			for _, val := range j.ObjectVals[propName].ArrayVals {
				e.d.addXref(entryid, from, val.StringVal, dbid, false)
			}
		}
	}

	xrefGO := func(j *jsparser.JSON, entryid, from string) {

		if j.ObjectVals["GO"] != nil {
			for _, val := range j.ObjectVals["GO"].ArrayVals {
				if _, ok := val.ObjectVals["term"]; ok {
					e.d.addXref(entryid, from, val.ObjectVals["term"].StringVal, "GO", false)
				}
			}
		}

	}

	//xrefProps := []string{"name", "description", "start", "end", "biotype", "genome", "strand", "seq_region_name"}
	xrefProp := func(j *jsparser.JSON, entryid, from string) {

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
			attr.SeqRegionName = j.ObjectVals["seq_region_name"].StringVal
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

	for _, paths := range jsonPaths {

		for _, path := range paths {

			previous = 0
			start = time.Now()

			// todo each file creates new ftp connection maybe it can be reused like init in bacteria paths
			br, _, ftpFile, localFile, _ := e.d.getDataReaderNew("ensembl", ftpAddress, "", path)

			p := jsparser.NewJSONParser(br, "genes").SkipProps([]string{"lineage", "evidence", "coord_system", "sifts", "xrefs", "gene_tree_id", "orthology_type"})

			for j := range p.Stream() {
				if j.ObjectVals["id"] != nil {

					elapsed := int64(time.Since(start).Seconds())
					if elapsed > previous+e.d.progInterval {
						kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
						previous = elapsed
						e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
					}

					entryid := j.ObjectVals["id"].StringVal

					if j.ObjectVals["name"] != nil {
						e.d.addXref(j.ObjectVals["name"].StringVal, textLinkID, entryid, "ensembl", true)
					}

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

					// store texts
					xrefProp(j, entryid, fr)

					// maybe these values from configuration
					xref(j, entryid, fr, "Interpro", "interpro")
					xref(j, entryid, fr, "HPA", "HPA")
					xref(j, entryid, fr, "ArrayExpress", "ExpressionAtlas")
					xref(j, entryid, fr, "GENE3D", "CATHGENE3D")
					xref(j, entryid, fr, "MIM_GENE", "MIM")
					xref(j, entryid, fr, "RefSeq_peptide", "RefSeq")
					xref(j, entryid, fr, "EntrezGene", "GeneID")
					xref(j, entryid, fr, "PANTHER", "PANTHER")
					xref(j, entryid, fr, "Reactome", "Reactome")
					xref(j, entryid, fr, "RNAcentral", "RNAcentral")
					xref(j, entryid, fr, "Uniprot/SPTREMBL", "uniprot")
					xref(j, entryid, fr, "protein_id", "EMBL")
					xref(j, entryid, fr, "KEGG_Enzyme", "KEGG")
					xref(j, entryid, fr, "EMBL", "EMBL")
					xref(j, entryid, fr, "CDD", "CDD")
					xref(j, entryid, fr, "TIGRfam", "TIGRFAMs")
					xref(j, entryid, fr, "ChEMBL", "ChEMBL")
					xref(j, entryid, fr, "UniParc", "uniparc")
					xref(j, entryid, fr, "PDB", "PDB")
					xref(j, entryid, fr, "SuperFamily", "SUPFAM")
					xref(j, entryid, fr, "Prosite_profiles", "PROSITE")
					xref(j, entryid, fr, "RefSeq_mRNA", "RefSeq")
					xref(j, entryid, fr, "Pfam", "Pfam")
					xref(j, entryid, fr, "CCDS", "RefSeq")
					xref(j, entryid, fr, "Prosite_patterns", "PROSITE")
					xref(j, entryid, fr, "Uniprot/SWISSPROT", "uniprot")
					xref(j, entryid, fr, "UCSC", "UCSC")
					//xref(j, entryid, fr, "Uniprot_gn", "uniprot")
					xref(j, entryid, fr, "HGNC", "hgnc")
					xref(j, entryid, fr, "RefSeq_ncRNA_predicted", "RefSeq")
					xref(j, entryid, fr, "HAMAP", "HAMAP")
					xrefGO(j, entryid, fr) // go terms are also under xrefs with source information.

					if j.ObjectVals["transcripts"] != nil {
						for _, val := range j.ObjectVals["transcripts"].ArrayVals {
							tentryid := val.ObjectVals["id"].StringVal

							e.d.addXref(entryid, fr, tentryid, "transcript", false)

							/** this is excluded for now can be included if wanted
							if val.ObjectVals["name"] != nil {
								e.d.addXref(val.ObjectVals["name"].StringVal, textLinkID, tentryid, "transcript", true)
							}
							**/

							if val.ObjectVals["exons"] != nil {
								for _, exo := range val.ObjectVals["exons"].ArrayVals {
									e.d.addXref(tentryid, ensemblTranscriptID, exo.ObjectVals["id"].StringVal, "exon", false)
									attr := pbuf.EnsemblAttr{}
									if exo.ObjectVals["seq_region_name"] != nil {
										attr.SeqRegionName = exo.ObjectVals["seq_region_name"].StringVal
									}

									if exo.ObjectVals["strand"] != nil {
										attr.Strand = exo.ObjectVals["strand"].StringVal
									}

									if exo.ObjectVals["start"] != nil {
										c, err := strconv.Atoi(exo.ObjectVals["start"].StringVal)
										if err == nil {
											attr.Start = int32(c)
										}
									}

									if exo.ObjectVals["end"] != nil {
										c, err := strconv.Atoi(exo.ObjectVals["end"].StringVal)
										if err == nil {
											attr.End = int32(c)
										}
									}
									b, _ := ffjson.Marshal(attr)
									e.d.addProp3(exo.ObjectVals["id"].StringVal, exonsID, b)

								}
							}

							if val.ObjectVals["translations"] != nil {
								for _, eprotein := range val.ObjectVals["translations"].ArrayVals {
									e.d.addXref(tentryid, ensemblTranscriptID, eprotein.ObjectVals["id"].StringVal, "eprotein", false)
									xref(eprotein, eprotein.ObjectVals["id"].StringVal, ensemblProteinID, "Uniprot/SWISSPROT", "uniprot")
									xref(eprotein, eprotein.ObjectVals["id"].StringVal, ensemblProteinID, "Uniprot/SPTREMBL", "uniprot")
								}
							}

							// store texts
							xrefProp(val, tentryid, ensemblTranscriptID)

							xref(val, tentryid, ensemblTranscriptID, "Interpro", "interpro")
							xref(val, tentryid, ensemblTranscriptID, "HPA", "HPA")
							xref(val, tentryid, ensemblTranscriptID, "ArrayExpress", "ExpressionAtlas")
							xref(val, tentryid, ensemblTranscriptID, "GENE3D", "CATHGENE3D")
							xref(val, tentryid, ensemblTranscriptID, "MIM_GENE", "MIM")
							xref(val, tentryid, ensemblTranscriptID, "RefSeq_peptide", "RefSeq")
							xref(val, tentryid, ensemblTranscriptID, "EntrezGene", "GeneID")
							xref(val, tentryid, ensemblTranscriptID, "PANTHER", "PANTHER")
							xref(val, tentryid, ensemblTranscriptID, "Reactome", "Reactome")
							xref(val, tentryid, ensemblTranscriptID, "RNAcentral", "RNAcentral")
							xref(val, tentryid, ensemblTranscriptID, "Uniprot/SPTREMBL", "uniprot")
							xref(val, tentryid, ensemblTranscriptID, "protein_id", "EMBL")
							xref(val, tentryid, ensemblTranscriptID, "KEGG_Enzyme", "KEGG")
							xref(val, tentryid, ensemblTranscriptID, "EMBL", "EMBL")
							xref(val, tentryid, ensemblTranscriptID, "CDD", "CDD")
							xref(val, tentryid, ensemblTranscriptID, "TIGRfam", "TIGRFAMs")
							xref(val, tentryid, ensemblTranscriptID, "ChEMBL", "ChEMBL")
							xref(val, tentryid, ensemblTranscriptID, "UniParc", "uniparc")
							xref(val, tentryid, ensemblTranscriptID, "PDB", "PDB")
							xref(val, tentryid, ensemblTranscriptID, "SuperFamily", "SUPFAM")
							xref(val, tentryid, ensemblTranscriptID, "Prosite_profiles", "PROSITE")
							xref(val, tentryid, ensemblTranscriptID, "RefSeq_mRNA", "RefSeq")
							xref(val, tentryid, ensemblTranscriptID, "Pfam", "Pfam")
							xref(val, tentryid, ensemblTranscriptID, "CCDS", "RefSeq")
							xref(val, tentryid, ensemblTranscriptID, "Prosite_patterns", "PROSITE")
							xref(val, tentryid, ensemblTranscriptID, "Uniprot/SWISSPROT", "uniprot")
							xref(val, tentryid, ensemblTranscriptID, "UCSC", "UCSC")
							xref(val, tentryid, ensemblTranscriptID, "Uniprot_gn", "uniprot")
							xref(val, tentryid, ensemblTranscriptID, "HGNC", "hgnc")
							xref(val, tentryid, ensemblTranscriptID, "RefSeq_ncRNA_predicted", "RefSeq")
							xref(val, tentryid, ensemblTranscriptID, "HAMAP", "HAMAP")
							xrefGO(val, tentryid, ensemblTranscriptID)

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

			time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp
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
			br2, _, ftpFile2, localFile2, _ := e.d.getDataReaderNew(probsetMachine, ftpAddress, "", path)

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

		} else {
			log.Println("Warn: new prob mapping found. It must be defined in configuration", probsetMachine)
		}
		time.Sleep(time.Duration(e.pauseDurationSeconds) * time.Second) // for not to kicked out from ensembl ftp
	}

	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
	atomic.AddUint64(&e.d.totalParsedEntry, total)

	e.d.addEntryStat(e.source, total)

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
