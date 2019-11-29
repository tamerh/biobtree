package update

import (
	"biobtree/configs"
	"biobtree/generate"
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

var loadConf = initConf()

func initConf() bool {

	c := configs.Conf{}
	c.Init("../", "", true, "")
	config = &c
	return true

}

func cleanOut() {

	os.RemoveAll(config.Appconf["outDir"])
	_ = os.Mkdir(filepath.FromSlash(config.Appconf["outDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(config.Appconf["indexDir"]), 0700)
	_ = os.Mkdir(filepath.FromSlash(config.Appconf["dbDir"]), 0700)

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

	cleanOut()

	ioutil.WriteFile(filepath.FromSlash("../../test_data/hgnc_test.json"), []byte(json), 0700)

	config.Dataconf["hgnc"]["path"] = "../../test_data/hgnc_test.json"
	config.Dataconf["hgnc"]["useLocalFile"] = "yes"

	config.Appconf["kvgenCount"] = "4"
	config.Appconf["kvgenChunkSize"] = "13"

	d := NewDataUpdate(map[string]bool{"hgnc": true}, []string{}, []string{}, []string{}, []int{}, false, config, "1")

	parsed, kvs := d.Update()

	if parsed != 6 {
		panic("parsed entry is not 6")
	}

	if kvs < 25 || kvs > 27 { //this is because randomly in each file contains duplicate
		panic("key value count is not between 25 and  27 instead it is -->" + strconv.FormatUint(kvs, 10))
	}

	var m = generate.Merge{}
	j, k, _ := m.Merge(config, false)

	if j != 18 {
		panic("merge write key value is invalid")
	}

	if k != 18 {
		panic("merge uid value is invalid")
	}

	os.Remove(filepath.FromSlash("../../test_data/hgnc_test.json"))

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

	cleanOut()

	zw := gzip.NewWriter(&b)
	zw.Write([]byte(xml))
	zw.Close()

	ioutil.WriteFile(filepath.FromSlash("../../test_data/uniprot.xml.gz"), b.Bytes(), 0700)

	config.Dataconf["uniprot"]["path"] = "../../test_data/uniprot.xml.gz"
	config.Dataconf["uniprot"]["useLocalFile"] = "yes"
	config.Appconf["kvgenCount"] = "4"
	config.Appconf["kvgenChunkSize"] = "13"
	config.Appconf["pageSize"] = "2"

	d := NewDataUpdate(map[string]bool{"uniprot": true}, []string{}, []string{}, []string{}, []int{}, false, config, "1")

	parsed, kvs := d.Update()

	if parsed != 1 {
		panic("parsed entry is not 1")
	}

	if kvs != 9 {
		panic("key value count is not 8")
	}

	var m = generate.Merge{}
	j, k, l := m.Merge(config, false)

	if j != 8 {
		panic("merge write key value is invalid")
	}

	if k != 8 {
		panic("merge uid value is invalid")
	}

	if l != 6 {
		panic("link key count is invalid")
	}

	os.Remove(filepath.FromSlash("../../test_data/uniprot.xml.gz"))

}

func TestPaging(t *testing.T) {

	const json = `{"responseHeader":{"status":0,"QTime":18},"response":{"numFound":8,
	"docs":[{"hgnc_id":"HGNC:1","vega_id":"OTTHUMG00000183507","pubmed_id":"28472374","refseq_accession":["NM_130786"]}
				 ,{"hgnc_id":"HGNC:2","symbol":"tpi1"}
				 ,{"hgnc_id":"HGNC:3","vega_id":"OTTHUMG00000183508","pubmed_id":"28472375","refseq_accession":["NM_130787","NM_130788"],"ensembl_gene_id":"ENSG00000175899","cosmic":"A1BG","omim_id":["103950"]}
				 ,{"hgnc_id":"HGNC:4","vega_id":"tpi1","symbol":"tpi1"}
				 ,{"hgnc_id":"HGNC:4","vega_id":"tpi1","symbol":"tpi1"}]]
				 }}`

	cleanOut()

	ioutil.WriteFile(filepath.FromSlash("../../test_data/hgnc_test2.json"), []byte(json), 0700)
	config.Dataconf["hgnc"]["path"] = "../../test_data/hgnc_test2.json"
	config.Dataconf["hgnc"]["useLocalFile"] = "yes"
	config.Appconf["kvgenCount"] = "1"
	//c.Appconf["kvgenChunkSize"] = "13"
	config.Appconf["pageSize"] = "2"

	d := NewDataUpdate(map[string]bool{"hgnc": true}, []string{}, []string{}, []string{}, []int{}, false, config, "1")

	parsed, kvs := d.Update()

	if parsed != 5 {
		panic("parsed entry is not 5")
	}

	if kvs != 26 {
		panic("key value count not valid")
	}

	var m = generate.Merge{}
	j, k, _ := m.Merge(config, false)

	if j != 19 { // todo empty xref key hgnc:2 is not written??
		panic("merge write key value is invalid")
	}

	if k != 15 {
		panic("merge uid value is invalid")
	}

	os.Remove(filepath.FromSlash("../../test_data/hgnc_test2.json"))

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

	cleanOut()

	ioutil.WriteFile(filepath.FromSlash("../../test_data/hgnc_test.json"), []byte(json), 0700)

	config.Dataconf["hgnc"]["path"] = "../../test_data/hgnc_test.json"
	config.Dataconf["hgnc"]["useLocalFile"] = "yes"
	config.Appconf["kvgenCount"] = "4"
	config.Appconf["kvgenChunkSize"] = "13"

	d := NewDataUpdate(map[string]bool{"hgnc": true}, []string{"VEGA"}, []string{}, []string{}, []int{}, false, config, "1")

	parsed, kvs := d.Update()

	if parsed != 6 {
		panic("parsed entry is not 6")
	}

	if kvs != 10 {
		panic("key value count is not 10")
	}

	var m = generate.Merge{}
	j, k, _ := m.Merge(config, false)

	if j != 10 {
		panic("merge write key value is invalid")
	}

	if k != 10 {
		panic("merge uid value is invalid")
	}

	os.Remove(filepath.FromSlash("../../test_data/hgnc_test.json"))

}

func TestDuplicateValue(t *testing.T) {
	// entry count 6 kv count 26
	const json = `{"responseHeader":{"status":0,"QTime":18},"response":{"numFound":8,
	"docs":[
		 			{"hgnc_id":"HGNC:1","pubmed_id":"28472374","symbol":"tpi1"}
				 ,{"hgnc_id":"HGNC:1","pubmed_id":"28472374","symbol":"tpi1"}
		]}}`

	cleanOut()

	_ = os.Mkdir(filepath.FromSlash("../../test_data"), 0700)

	ioutil.WriteFile(filepath.FromSlash("../../test_data/hgnc_test.json"), []byte(json), 0700)

	config.Dataconf["hgnc"]["path"] = "../../test_data/hgnc_test.json"
	config.Dataconf["hgnc"]["useLocalFile"] = "yes"
	config.Appconf["kvgenCount"] = "1"
	config.Appconf["kvgenChunkSize"] = "20"

	d := NewDataUpdate(map[string]bool{"hgnc": true}, []string{}, []string{}, []string{}, []int{}, false, config, "1")

	parsed, kvs := d.Update()

	if parsed != 2 {
		panic("parsed entry is not 2")
	}

	if kvs != 4 {
		panic("key value count is not 4")
	}

	var m = generate.Merge{}
	j, k, _ := m.Merge(config, false)

	if j != 3 {
		panic("merge write key value is invalid")
	}

	if k != 3 {
		panic("merge uid value is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../../test_data/hgnc_test.json"))

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
            }
          ],
          "id": "ENST00000482209",
          "start": "6869548"
				}]
					} 
		 `

	cleanOut()

	ioutil.WriteFile(filepath.FromSlash("../../test_data/ensembl_test.json"), []byte(json), 0700)

	config.Dataconf["ensembl"]["path"] = "../../test_data/ensembl_test.json"

	config.Dataconf["ensembl"]["useLocalFile"] = "yes"

	config.Appconf["kvgenCount"] = "4"
	config.Appconf["kvgenChunkSize"] = "13"

	d := NewDataUpdate(map[string]bool{"ensembl": true}, []string{}, []string{}, []string{}, []int{}, false, config, "1")

	parsed, kvs := d.Update()

	if parsed != 2 {
		panic("parsed entry is not 2")
	}

	if kvs != 20 {
		panic("key value count is not 20")
	}

	var m = generate.Merge{}
	j, k, l := m.Merge(config, false)

	if j != 11 {
		panic("merge write key value is invalid")
	}

	if k != 11 {
		panic("merge uid value is invalid")
	}

	if l != 2 {
		panic("link count is invalid")
	}

	os.RemoveAll(filepath.FromSlash("../../test_data"))

}

func TestSamples(t *testing.T) {

	start := time.Now()

	cleanOut()

	config.Dataconf["uniprot"]["path"] = "../../test_data/uniprot_sprot.xml.gz"
	config.Dataconf["uniprot"]["useLocalFile"] = "yes"
	config.Dataconf["uniprot"]["pathTrembl"] = "../../test_data/uniprot_trembl.xml.gz"
	config.Dataconf["uniprot"]["useLocalFile"] = "yes"
	config.Dataconf["uniref100"]["path"] = "../../test_data/uniref100.xml.gz"
	config.Dataconf["uniref100"]["useLocalFile"] = "yes"
	config.Dataconf["uniref90"]["path"] = "../../test_data/uniref90.xml.gz"
	config.Dataconf["uniref90"]["useLocalFile"] = "yes"
	config.Dataconf["uniref50"]["path"] = "../../test_data/uniref50.xml.gz"
	config.Dataconf["uniref50"]["useLocalFile"] = "yes"
	config.Dataconf["uniparc"]["path"] = "../../test_data/uniparc.xml.gz"
	config.Dataconf["uniparc"]["useLocalFile"] = "yes"
	config.Dataconf["taxonomy"]["path"] = "../../test_data/taxonomy.xml.gz"
	config.Dataconf["taxonomy"]["useLocalFile"] = "yes"
	config.Dataconf["interpro"]["path"] = "../../test_data/interpro.xml.gz"
	config.Dataconf["interpro"]["useLocalFile"] = "yes"

	config.Appconf["kvgenCount"] = "4"
	config.Appconf["kvgenChunkSize"] = "1000000"

	d := NewDataUpdate(map[string]bool{"uniprot": true, "uniref100": true, "uniref90": true, "uniref50": true, "uniparc": true, "taxonomy": true, "interpro": true}, []string{}, []string{}, []string{}, []int{}, false, config, "1")

	d.Update()

	var m = generate.Merge{}
	i, j, _ := m.Merge(config, false)

	fmt.Println("lmdb key value size", i)
	fmt.Println("max uid", j)
	elapsed := time.Since(start)
	log.Printf("Binomial took %s", elapsed)

}
