package update

import (
	"bufio"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tamerh/jsparser"
)

type ensembl struct {
	source string
	d      *DataUpdate
}

func (e *ensembl) getEnsemblSetting(ensemblType string) (string, string, []string, []string) {

	//var selectedSpecies []string
	//jsonFilePaths := map[string]string{}
	//biomartFilePaths := map[string][]string{}
	var jsonFilePaths []string
	var biomartFilePaths []string
	var fr string

	if len(e.d.selectedEnsemblSpecies) == 1 && e.d.selectedEnsemblSpecies[0] == "all" {
		e.d.selectedEnsemblSpecies = nil
	}
	var ftpAddress string
	var ftpJSONPath string
	var ftpMysqlPath string
	var ftpBiomartFolder string
	var branch string

	allSpeciesPaths := map[string]string{} // this is needed because their json might under a collection

	setJSONs := func() {
		client := e.d.ftpClient(ftpAddress)
		entries, err := client.List(ftpJSONPath)
		check(err)

		for _, file := range entries {
			if strings.HasSuffix(file.Name, "_collection") {
				client := e.d.ftpClient(ftpAddress)
				entries2, err := client.List(ftpJSONPath + "/" + file.Name)
				check(err)
				for _, file2 := range entries2 {
					allSpeciesPaths[file2.Name] = ftpJSONPath + "/" + file.Name + "/" + file2.Name + "/" + file2.Name + ".json"
				}
			} else {
				allSpeciesPaths[file.Name] = ftpJSONPath + "/" + file.Name + "/" + file.Name + ".json"
			}
		}

		if e.d.selectedEnsemblSpecies == nil { // if all selected

			for _, v := range allSpeciesPaths {
				jsonFilePaths = append(jsonFilePaths, v)
			}

		} else {
			for _, sp := range e.d.selectedEnsemblSpecies {

				if _, ok := allSpeciesPaths[sp]; !ok {
					log.Println("WARN Species ->", sp, "not found in ensembl ", branch, "if you specify multiple ensembl IGNORE this")
					continue
				} else {
					jsonFilePaths = append(jsonFilePaths, allSpeciesPaths[sp])
				}
			}
		}
	}

	setBiomarts := func() {

		// setup biomart release not handled at the moment
		if e.d.ensemblRelease == "" {
			// find the biomart folder
			client := e.d.ftpClient(ftpAddress)
			entries, err := client.List(ftpMysqlPath + "/" + branch + "_mart_*")
			check(err)
			if len(entries) != 1 {
				log.Fatal("Error:More than one mart folder found for biomart")
			}
			if len(entries) == 1 {
				ftpBiomartFolder = entries[0].Name
			}

		}

		var biomartSpeciesName string // this is just the shorcut name of species in biomart folder e.g homo_sapiens-> hsapiens
		for _, sp := range e.d.selectedEnsemblSpecies {

			splitted := strings.Split(sp, "_")
			if len(splitted) > 1 {
				biomartSpeciesName = splitted[0][:1] + splitted[len(splitted)-1]
			} else {
				panic("Unrecognized species name pattern->" + sp)
			}

			// now get list of probset files
			client := e.d.ftpClient(ftpAddress)
			entries, err := client.List(ftpMysqlPath + "/" + ftpBiomartFolder + "/" + biomartSpeciesName + "*__efg_*.gz")
			check(err)
			//var biomartFiles []string
			for _, file := range entries {
				biomartFilePaths = append(biomartFilePaths, ftpMysqlPath+"/"+ftpBiomartFolder+"/"+file.Name)
			}
			//biomartFilePaths[sp] = biomartFiles
		}
	}
	switch e.source {

	case "ensembl":
		fr = dataconf["ensembl"]["id"]
		ftpAddress = appconf["ensembl_ftp"]
		ftpJSONPath = appconf["ensembl_ftp_json_path"]
		ftpMysqlPath = appconf["ensembl_ftp_mysql_path"]
		branch = "ensembl"
		setJSONs()
		setBiomarts()
	case "ensembl_bacteria":
		fr = dataconf["ensembl_bacteria"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "bacteria", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "bacteria", 1)
		branch = "bacteria"
		setJSONs()
	case "ensembl_fungi":
		fr = dataconf["ensembl_fungi"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "fungi", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "fungi", 1)
		branch = "fungi"
		setJSONs()
		setBiomarts()
	case "ensembl_metazoa":
		fr = dataconf["ensembl_metazoa"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "metazoa", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "metazoa", 1)
		branch = "metazoa"
		setJSONs()
		setBiomarts()
	case "ensembl_plants":
		fr = dataconf["ensembl_plants"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "plants", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "plants", 1)
		branch = "plants"
		setJSONs()
		setBiomarts()
	case "ensembl_protists":
		fr = dataconf["ensembl_protists"]["id"]
		ftpAddress = appconf["ensembl_genomes_ftp"]
		ftpJSONPath = strings.Replace(appconf["ensembl_genomes_ftp_json_path"], "$(branch)", "protists", 1)
		ftpMysqlPath = strings.Replace(appconf["ensembl_genomes_ftp_mysql_path"], "$(branch)", "protists", 1)
		branch = "protists"
		setJSONs()
		setBiomarts()
	}

	return fr, ftpAddress, jsonFilePaths, biomartFilePaths

}

func (e *ensembl) update() {
	defer e.d.wg.Done()

	ensemblTranscriptID := dataconf["EnsemblTranscript"]["id"]

	var total uint64
	var previous int64

	fr, ftpAddress, jsonPaths, biomartPaths := e.getEnsemblSetting(e.source)

	// if local file just ignore ftp jsons
	if dataconf[e.source]["useLocalFile"] == "yes" {
		jsonPaths = nil
		jsonPaths = append(jsonPaths, dataconf[e.source]["path"])
	}

	xref := func(j *jsparser.JSON, entryid, propName, dbid string) {

		if j.ObjectVals[propName] != nil {
			for _, val := range j.ObjectVals[propName].ArrayVals {
				e.d.addXref(entryid, fr, val.StringVal, dbid, false)
			}
		}
	}

	for _, path := range jsonPaths {

		//fmt.Println(path)
		br, _, ftpFile, localFile, _, _ := e.d.getDataReaderNew(e.source, ftpAddress, "", path)

		p := jsparser.NewJSONParser(br, "genes").SkipProps([]string{"description", "lineage", "start", "end", "evidence", "coord_system", "sifts", "gene_tree_id", "genome_display", "orthology_type", "genome", "seq_region_name", "strand", "xrefs"})

		for j := range p.Stream() {
			if j.ObjectVals["id"] != nil {

				elapsed := int64(time.Since(e.d.start).Seconds())
				if elapsed > previous+e.d.progInterval {
					kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
					previous = elapsed
					e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
				}

				entryid := j.ObjectVals["id"].StringVal

				if j.ObjectVals["name"] != nil {
					e.d.addXref(j.ObjectVals["name"].StringVal, textLinkID, entryid, e.source, true)
				}

				if j.ObjectVals["taxon_id"] != nil {
					e.d.addXref(entryid, fr, j.ObjectVals["taxon_id"].StringVal, "taxonomy", false)
				}

				if j.ObjectVals["homologues"] != nil {
					for _, val := range j.ObjectVals["homologues"].ArrayVals {
						e.d.addXref(entryid, fr, val.ObjectVals["stable_id"].StringVal, "EnsemblHomolog", false)
					}
				}

				// maybe these values from configuration
				xref(j, entryid, "Interpro", "interpro")
				xref(j, entryid, "HPA", "HPA")
				xref(j, entryid, "ArrayExpress", "ExpressionAtlas")
				xref(j, entryid, "GENE3D", "CATHGENE3D")
				xref(j, entryid, "MIM_GENE", "MIM")
				xref(j, entryid, "RefSeq_peptide", "RefSeq")
				xref(j, entryid, "EntrezGene", "GeneID")
				xref(j, entryid, "PANTHER", "PANTHER")
				xref(j, entryid, "Reactome", "Reactome")
				xref(j, entryid, "RNAcentral", "RNAcentral")
				xref(j, entryid, "Uniprot/SPTREMBL", "uniprot_unreviewed")
				xref(j, entryid, "protein_id", "EMBL")
				xref(j, entryid, "KEGG_Enzyme", "KEGG")
				xref(j, entryid, "EMBL", "EMBL")
				xref(j, entryid, "CDD", "CDD")
				xref(j, entryid, "TIGRfam", "TIGRFAMs")
				xref(j, entryid, "ChEMBL", "ChEMBL")
				xref(j, entryid, "UniParc", "uniparc")
				xref(j, entryid, "PDB", "PDB")
				xref(j, entryid, "SuperFamily", "SUPFAM")
				xref(j, entryid, "Prosite_profiles", "PROSITE")
				xref(j, entryid, "RefSeq_mRNA", "RefSeq")
				xref(j, entryid, "Pfam", "Pfam")
				xref(j, entryid, "CCDS", "RefSeq")
				xref(j, entryid, "Prosite_patterns", "PROSITE")
				xref(j, entryid, "Uniprot/SWISSPROT", "uniprot_reviewed")
				xref(j, entryid, "UCSC", "UCSC")
				xref(j, entryid, "Uniprot_gn", "uniprot_reviewed")
				xref(j, entryid, "HGNC", "hgnc")
				xref(j, entryid, "RefSeq_ncRNA_predicted", "RefSeq")
				xref(j, entryid, "HAMAP", "HAMAP")

				if j.ObjectVals["transcripts"] != nil {
					for _, val := range j.ObjectVals["transcripts"].ArrayVals {
						tentryid := val.ObjectVals["id"].StringVal

						e.d.addXref(entryid, fr, tentryid, "EnsemblTranscript", false)

						if val.ObjectVals["name"] != nil {
							e.d.addXref(val.ObjectVals["name"].StringVal, textLinkID, tentryid, "EnsemblTranscript", true)
						}

						if val.ObjectVals["exons"] != nil {
							for _, exo := range val.ObjectVals["exons"].ArrayVals {
								e.d.addXref(tentryid, ensemblTranscriptID, exo.ObjectVals["id"].StringVal, "EnsemblExon", false)
							}
						}

						xref(j, tentryid, "Interpro", "interpro")
						xref(j, tentryid, "HPA", "HPA")
						xref(j, tentryid, "ArrayExpress", "ExpressionAtlas")
						xref(j, tentryid, "GENE3D", "CATHGENE3D")
						xref(j, tentryid, "MIM_GENE", "MIM")
						xref(j, tentryid, "RefSeq_peptide", "RefSeq")
						xref(j, tentryid, "EntrezGene", "GeneID")
						xref(j, tentryid, "PANTHER", "PANTHER")
						xref(j, tentryid, "Reactome", "Reactome")
						xref(j, tentryid, "RNAcentral", "RNAcentral")
						xref(j, tentryid, "Uniprot/SPTREMBL", "uniprot_unreviewed")
						xref(j, tentryid, "protein_id", "EMBL")
						xref(j, tentryid, "KEGG_Enzyme", "KEGG")
						xref(j, tentryid, "EMBL", "EMBL")
						xref(j, tentryid, "CDD", "CDD")
						xref(j, tentryid, "TIGRfam", "TIGRFAMs")
						xref(j, tentryid, "ChEMBL", "ChEMBL")
						xref(j, tentryid, "UniParc", "uniparc")
						xref(j, tentryid, "PDB", "PDB")
						xref(j, tentryid, "SuperFamily", "SUPFAM")
						xref(j, tentryid, "Prosite_profiles", "PROSITE")
						xref(j, tentryid, "RefSeq_mRNA", "RefSeq")
						xref(j, tentryid, "Pfam", "Pfam")
						xref(j, tentryid, "CCDS", "RefSeq")
						xref(j, tentryid, "Prosite_patterns", "PROSITE")
						xref(j, tentryid, "Uniprot/SWISSPROT", "uniprot_reviewed")
						xref(j, tentryid, "UCSC", "UCSC")
						xref(j, tentryid, "Uniprot_gn", "uniprot_reviewed")
						xref(j, tentryid, "HGNC", "hgnc")
						xref(j, tentryid, "RefSeq_ncRNA_predicted", "RefSeq")
						xref(j, tentryid, "HAMAP", "HAMAP")

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

		//TODO GO
		//TODO PROTEIN FEAUTRES

	}

	previous = 0
	totalRead := 0
	// probset biomart
	for _, path := range biomartPaths {
		// first get the probset machine name
		f := strings.Split(path, "/")
		probsetMachine := strings.Split(f[len(f)-1], "__")[1][4:]
		probsetConf := dataconf[probsetMachine]

		if probsetConf != nil {

			br2, _, ftpFile2, localFile2, fr2, _ := e.d.getDataReaderNew(probsetMachine, ftpAddress, "", path)

			scanner := bufio.NewScanner(br2)
			for scanner.Scan() {

				elapsed := int64(time.Since(e.d.start).Seconds())
				if elapsed > previous+e.d.progInterval {
					kbytesPerSecond := int64(totalRead) / elapsed / 1024
					previous = elapsed
					e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
				}
				s := scanner.Text()
				t := strings.Split(s, "\t")
				if len(t) == 3 && t[2] != "\\N" && t[1] != "\\N" {
					e.d.addXref(t[2], fr2, t[1], "EnsemblTranscript", false)
				}
				totalRead = totalRead + len(s)
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

	}

	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
	atomic.AddUint64(&e.d.totalParsedEntry, total)

	e.d.addEntryStat(e.source, total)

}
