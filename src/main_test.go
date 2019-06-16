package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type teststruct struct {
	amap2 map[string][]string
}

func TestScratch(t *testing.T) {

	test := teststruct{}

	amap := map[string]teststruct{}
	amap["1"] = teststruct{}

	for _, v := range amap {
		fmt.Println(v)
		for _, l := range v.amap2["23"] {
			fmt.Println(l)
		}
	}

	for _, v := range test.amap2 {
		fmt.Println(v)
	}
}

func clearDirs() {
	os.RemoveAll(appconf["outDir"])
	_ = os.Mkdir(filepath.FromSlash(appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)

}
func TestConfiguration(t *testing.T) {
	initConf("")
}

func TestFullSample(t *testing.T) {

	start := time.Now()

	initConf("")
	clearDirs()

	dataconf["uniprot_reviewed"]["path"] = "../raw/uniprot/uniprot_sprot.xml.gz"
	dataconf["uniprot_reviewed"]["useLocalFile"] = "yes"
	dataconf["uniprot_unreviewed"]["path"] = "../raw/uniprot/uniprot_trembl.xml.gz"
	dataconf["uniprot_unreviewed"]["useLocalFile"] = "yes"
	dataconf["uniref100"]["path"] = "../raw/uniprot/uniref100.xml.gz"
	dataconf["uniref100"]["useLocalFile"] = "yes"
	dataconf["uniref90"]["path"] = "../raw/uniprot/uniref90.xml.gz"
	dataconf["uniref90"]["useLocalFile"] = "yes"
	dataconf["uniref50"]["path"] = "../raw/uniprot/uniref50.xml.gz"
	dataconf["uniref50"]["useLocalFile"] = "yes"
	dataconf["uniparc"]["path"] = "../raw/uniprot/uniparc_all.xml.gz"
	dataconf["uniparc"]["useLocalFile"] = "yes"
	dataconf["taxonomy"]["path"] = "../raw/taxonomy/taxonomy.xml.gz"
	dataconf["taxonomy"]["useLocalFile"] = "yes"
	dataconf["hgnc"]["path"] = "../raw/hgnc/hgnc_complete_set.json"
	dataconf["hgnc"]["useLocalFile"] = "yes"
	dataconf["interpro"]["path"] = "../raw/interpro/interpro.xml.gz"
	dataconf["interpro"]["useLocalFile"] = "yes"

	appconf["kvgenCount"] = "4"
	appconf["kvgenChunkSize"] = "1000000"

	updateData([]string{"hgnc", "uniprot_reviewed", "uniprot_unreviewed", "uniref100", "uniref90", "uniref50", "uniparc", "taxonomy", "hgnc", "interpro"}, []string{})

	i, j, _ := mergeData()

	fmt.Println("lmdb key value size", i)
	fmt.Println("max uid", j)
	elapsed := time.Since(start)
	log.Printf("Binomial took %s", elapsed)

}

func TestHgnc(t *testing.T) {

	// entry count 6 kv count 26
	const json = `{"responseHeader":{"status":0,"QTime":18},"response":{"numFound":8,
	"docs":[{"hgnc_id":"HGNC:1","vega_id":"OTTHUMG00000183507","pubmed_id":"28472374","refseq_accession":["NM_130786"]}
				 ,{"hgnc_id":"HGNC:2"}
				 ,{"hgnc_id":"HGNC:3","vega_id":"OTTHUMG00000183508","pubmed_id":"28472375","refseq_accession":["NM_130787"]}
				 ,{"hgnc_id":"HGNC:4","vega_id":"OTTHUMG00000183509","pubmed_id":"28472376","refseq_accession":["NM_130788"]}
				 ,{"hgnc_id":"HGNC:5","vega_id":"OTTHUMG00000183500","pubmed_id":"28472377","refseq_accession":["NM_130789"]}
				 ,{"hgnc_id":"HGNC:6","vega_id":"OTTHUMG00000183600"}]
				 }}`

	initConf("")

	os.RemoveAll(appconf["outDir"])
	_ = os.Mkdir(filepath.FromSlash(appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)

	_ = os.Mkdir(filepath.FromSlash("../tmp"), 0700)

	ioutil.WriteFile(filepath.FromSlash("../tmp/hgnc_test.json"), []byte(json), 0700)

	dataconf["hgnc"]["path"] = "../tmp/hgnc_test.json"

	dataconf["hgnc"]["useLocalFile"] = "yes"

	appconf["kvgenCount"] = "4"
	appconf["kvgenChunkSize"] = "13"

	parsed, kvs := updateData([]string{"hgnc"}, []string{})

	if parsed != 6 {
		panic("parsed entry is not 6")
	}

	if kvs < 25 || kvs > 27 { //this is because randomly in each file contains duplicate
		panic("key value count is not between 25 and  27 instead it is -->" + strconv.FormatUint(kvs, 10))
	}

	j, k, _ := mergeData()

	if j != 18 {
		panic("merge write key value is invalid")
	}

	if k != 18 {
		panic("merge uid value is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../tmp"))

}

func TestKeyLink(t *testing.T) {

	var b bytes.Buffer

	const xml = `<entry dataset="Swiss-Prot" created="1987-08-13" modified="2018-11-07" version="271">
	<accession>P04637</accession>
	<accession>Q15086</accession>
	<accession>Q15087</accession>
	<accession>Q15088</accession>
	<accession>Q16535</accession>
	<accession>Q16807</accession>
	<name>P53_HUMAN</name>
	<dbReference type="NCBI taxonomy" id="9606"/></entry>`

	initConf("")
	clearDirs()

	_ = os.Mkdir(filepath.FromSlash("../tmp"), 0700)

	zw := gzip.NewWriter(&b)
	zw.Write([]byte(xml))
	zw.Close()

	ioutil.WriteFile(filepath.FromSlash("../tmp/uniprot.xml.gz"), b.Bytes(), 0700)

	dataconf["uniprot_reviewed"]["path"] = "../tmp/uniprot.xml.gz"
	dataconf["uniprot_reviewed"]["useLocalFile"] = "yes"

	appconf["kvgenCount"] = "4"
	appconf["kvgenChunkSize"] = "13"
	appconf["pageSize"] = "2"

	parsed, kvs := updateData([]string{"uniprot_reviewed"}, []string{})

	if parsed != 1 {
		panic("parsed entry is not 1")
	}

	if kvs != 8 {
		panic("key value count is not 8")
	}

	j, k, l := mergeData()

	if j != 8 {
		panic("merge write key value is invalid")
	}

	if k != 8 {
		panic("merge uid value is invalid")
	}

	if l != 6 {
		panic("link key count is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../tmp"))

}
func TestPaging(t *testing.T) {

	const json = `{"responseHeader":{"status":0,"QTime":18},"response":{"numFound":8,
	"docs":[{"hgnc_id":"HGNC:1","vega_id":"OTTHUMG00000183507","pubmed_id":"28472374","refseq_accession":["NM_130786"]}
				 ,{"hgnc_id":"HGNC:2","symbol":"tpi1"}
				 ,{"hgnc_id":"HGNC:3","vega_id":"OTTHUMG00000183508","pubmed_id":"28472375","refseq_accession":["NM_130787","NM_130788"],"ensembl_gene_id":"ENSG00000175899","cosmic":"A1BG","omim_id":["103950"]}
				 ,{"hgnc_id":"HGNC:4","vega_id":"tpi1","symbol":"tpi1"}
				 ,{"hgnc_id":"HGNC:4","vega_id":"tpi1","symbol":"tpi1"}]]
				 }}`

	initConf("")
	clearDirs()

	_ = os.Mkdir(filepath.FromSlash("../tmp"), 0700)

	ioutil.WriteFile(filepath.FromSlash("../tmp/hgnc_test.json"), []byte(json), 0700)

	dataconf["hgnc"]["path"] = "../tmp/hgnc_test.json"
	dataconf["hgnc"]["useLocalFile"] = "yes"

	appconf["kvgenCount"] = "1"
	//appconf["kvgenChunkSize"] = "13"
	appconf["pageSize"] = "2"

	parsed, kvs := updateData([]string{"hgnc"}, []string{})

	if parsed != 5 {
		panic("parsed entry is not 5")
	}

	if kvs != 24 {
		panic("key value count is not 24")
	}

	j, k, _ := mergeData()

	if j != 18 {
		panic("merge write key value is invalid")
	}

	if k != 15 {
		panic("merge uid value is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../tmp"))

}

func TestPageKey(t *testing.T) {

	p := &pagekey{}
	p.init()

	page := 25
	keyLen := p.keyLen(page)
	first := p.key(0, keyLen)
	last := p.key(25, keyLen)

	if first != "a" {
		panic("invalid page key")
	}

	if last != "z" {
		panic("invalid page key")
	}

	page = 676
	keyLen = p.keyLen(page)
	first = p.key(0, keyLen)
	last = p.key(25, keyLen)

	if first != "aa" {
		panic("invalid page key")
	}

	if last != "az" {
		panic("invalid page key")
	}

}

func TestTargetDbs(t *testing.T) {

	const json = `{"responseHeader":{"status":0,"QTime":18},"response":{"numFound":8,
	"docs":[{"hgnc_id":"HGNC:1","vega_id":"OTTHUMG00000183507","pubmed_id":"28472374","refseq_accession":["NM_130786"]}
				 ,{"hgnc_id":"HGNC:2"}
				 ,{"hgnc_id":"HGNC:3","vega_id":"OTTHUMG00000183508","pubmed_id":"28472375","refseq_accession":["NM_130787"]}
				 ,{"hgnc_id":"HGNC:4","vega_id":"OTTHUMG00000183509","pubmed_id":"28472376","refseq_accession":["NM_130788"]}
				 ,{"hgnc_id":"HGNC:5","vega_id":"OTTHUMG00000183500","pubmed_id":"28472377","refseq_accession":["NM_130789"]}
				 ,{"hgnc_id":"HGNC:6","vega_id":"OTTHUMG00000183600"}]
				 }}`

	initConf("")

	os.RemoveAll(appconf["outDir"])
	_ = os.Mkdir(filepath.FromSlash(appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)

	_ = os.Mkdir(filepath.FromSlash("../tmp"), 0700)

	ioutil.WriteFile(filepath.FromSlash("../tmp/hgnc_test.json"), []byte(json), 0700)

	dataconf["hgnc"]["path"] = "../tmp/hgnc_test.json"

	dataconf["hgnc"]["useLocalFile"] = "yes"

	appconf["kvgenCount"] = "4"
	appconf["kvgenChunkSize"] = "13"

	parsed, kvs := updateData([]string{"hgnc"}, []string{"VEGA"})

	if parsed != 6 {
		panic("parsed entry is not 6")
	}

	if kvs != 10 {
		panic("key value count is not 10")
	}

	j, k, _ := mergeData()

	if j != 10 {
		panic("merge write key value is invalid")
	}

	if k != 10 {
		panic("merge uid value is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../tmp"))

}

func TestDuplicateValue(t *testing.T) {
	// entry count 6 kv count 26
	const json = `{"responseHeader":{"status":0,"QTime":18},"response":{"numFound":8,
	"docs":[
		 			{"hgnc_id":"HGNC:1","pubmed_id":"28472374","symbol":"tpi1"}
				 ,{"hgnc_id":"HGNC:1","pubmed_id":"28472374","symbol":"tpi1"}
		]}}`

	initConf("")

	os.RemoveAll(appconf["outDir"])
	_ = os.Mkdir(filepath.FromSlash(appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)

	_ = os.Mkdir(filepath.FromSlash("../tmp"), 0700)

	ioutil.WriteFile(filepath.FromSlash("../tmp/hgnc_test.json"), []byte(json), 0700)

	dataconf["hgnc"]["path"] = "../tmp/hgnc_test.json"

	dataconf["hgnc"]["useLocalFile"] = "yes"

	appconf["kvgenCount"] = "1"
	appconf["kvgenChunkSize"] = "20"

	parsed, kvs := updateData([]string{"hgnc"}, []string{})

	if parsed != 2 {
		panic("parsed entry is not 2")
	}

	if kvs != 3 {
		panic("key value count is not 3")
	}

	j, k, _ := mergeData()

	if j != 18 {
		panic("merge write key value is invalid")
	}

	if k != 18 {
		panic("merge uid value is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../tmp"))

}
func TestIndexMerge(t *testing.T) {

	initConf("")

}

func TestSize(t *testing.T) {

	i := 30 << 30
	fmt.Println(i)

}

func TestEnsembl(t *testing.T) {

	// entry count 6 kv count 26
	const json = ` {
		"is_reference": "false",
		"genes": [ { 
		"id": "ENSG00000111669",
		"name": "TPI1",
		"Interpro": [
      "IPR020861",
      "IPR013785",
      "IPR022896",
      "IPR000652",
      "IPR035990"
    ],
    "HPA": [
      "HPA053568",
      "50924",
      "53568",
      "HPA050924"
    ],
    "ArrayExpress": [
      "ENSG00000111669"
    ],
    "Gene3D": [
      "3.20.20.70"
    ],
    "MIM_GENE": [
      "190450"
    ],  
      "homologues": [
        {
          "gene_tree_id": "ENSGT00390000013354",
          "stable_id": "ENSGGOG00000002623",
          "genome_display": "Gorilla",
          "orthology_type": "ortholog_one2one",
          "genome": "gorilla_gorilla"
        },
        {
          "gene_tree_id": "ENSGT00390000013354",
          "stable_id": "ENSPTRG00000004595",
          "genome_display": "Chimpanzee",
          "orthology_type": "ortholog_one2one",
          "genome": "pan_troglodytes"
				}],
				"transcripts": [
        {
          "RNAcentral": [
            "URS0000D3B3F5"
          ],
          "name": "TPI1-205",
          "HGNC_trans_name": [
            "TPI1-205"
          ],
          "end": "6870137",
          "biotype": "retained_intron",
          "seq_region_name": "12",
          "UCSC": [
            "ENST00000482209.1"
          ],
          "strand": "1",
          "exons": [
            {
              "seq_region_name": "12",
              "strand": "1",
              "id": "ENSE00001883408",
              "end": "6869773",
              "start": "6869548"
            },
            {
              "seq_region_name": "12",
              "strand": "1",
              "id": "ENSE00001887401",
              "end": "6870137",
              "start": "6870036"
            }
          ],
          "id": "ENST00000482209",
          "xrefs": [
            {
              "display_id": "ENST00000482209.1",
              "primary_id": "ENST00000482209.1",
              "db_display": "UCSC Stable ID",
              "info_type": "COORDINATE_OVERLAP",
              "info_text": "",
              "dbname": "UCSC"
            }
          ],
          "start": "6869548"
				}]
					} 
		 `

	initConf("")

	os.RemoveAll(appconf["outDir"])
	_ = os.Mkdir(filepath.FromSlash(appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(appconf["dbDir"]), 0700)

	_ = os.Mkdir(filepath.FromSlash("../tmp"), 0700)

	ioutil.WriteFile(filepath.FromSlash("../tmp/ensembl_test.json"), []byte(json), 0700)

	dataconf["ensembl"]["path"] = "../tmp/ensembl_test.json"

	dataconf["ensembl"]["useLocalFile"] = "yes"

	appconf["kvgenCount"] = "4"
	appconf["kvgenChunkSize"] = "13"

	parsed, kvs := updateData([]string{"ensembl"}, []string{})

	if parsed != 6 {
		panic("parsed entry is not 6")
	}

	if kvs != 26 {
		panic("key value count is not 26")
	}

	j, k, _ := mergeData()

	if j != 18 {
		panic("merge write key value is invalid")
	}

	if k != 18 {
		panic("merge uid value is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../tmp"))

}
