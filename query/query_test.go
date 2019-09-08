package query

import (
	"biobtree/conf"
	"testing"
)

var c = initConf()
var qparser = QueryParser{}
var res []Query
var err error

func initConf() bool {

	c := conf.Conf{}
	c.Init("../", "", []string{}, []string{}, true)
	qparser.Init(&c)
	return true

}

func TestBasics(t *testing.T) {

	query := `map(go).filter(type="biological_process")`
	res, err = qparser.Parse(query)

	if err != nil {
		panic(err)
	}

	if len(res) != 1 {
		panic("result must be 1")
	}
	if res[0].MapDataset != "go" {
		panic("parsing error invalid mapdataset expected go actual->" + res[0].MapDataset)
	}
	if res[0].Filter != `type="biological_process"` {
		panic("invalid filter")
	}

	// filter with parantghesis
	query = `map(go).filter(size(type) > 10 && size(type) < 50 )`
	res, err = qparser.Parse(query)

	if err != nil {
		panic(err)
	}
	if res[0].MapDataset != "go" {
		panic("parsing error invalid mapdataset expected go actual->" + res[0].MapDataset)
	}
	if res[0].Filter != `size(type) > 10 && size(type) < 50` {
		panic("invalid filter")
	}

	// test without initial map
	query = `filter(type="biological_process")`
	res, err = qparser.Parse(query)

	if err != nil {
		panic(err)
	}

	if res[0].MapDataset != "" {
		panic("parsing error invalid mapdataset expected empty found->" + res[0].MapDataset)
	}
	if res[0].Filter != `type="biological_process"` {
		panic("invalid filter")
	}

	// filter cant followed by filter
	query = `filter(type="biological_process").filter()`
	res, err = qparser.Parse(query)

	if err == nil {
		panic("Filter must follow by map error is expected")
	}

}

func TestMultiQuery(t *testing.T) {

	query := `filter(type="biological_process").map(uniprot).filter(reviewed=true).map(taxonomy).filter(name=="abcde").map(hgnc).map(uniprot)`
	res, err = qparser.Parse(query)

	if err != nil {
		panic(err)
	}

	if len(res) != 5 {
		panic("query must be total 5")
	}

	if res[0].MapDataset != "" {
		panic("parsing error invalid mapdataset expected empty found->" + res[0].MapDataset)
	}
	if res[0].Filter != `type="biological_process"` {
		panic("invalid filter")
	}

	if res[1].MapDataset != "uniprot" && res[1].Filter != "reviewed=true" {
		panic("Invalid first map filter")
	}

	if res[2].MapDataset != "taxonomy" && res[2].Filter != `name=="abcde"` {
		panic("Invalid second map filter")
	}

	if res[3].MapDataset != "hgnc" && res[3].Filter != `` {
		panic("Invalid third map filter")
	}

	if res[4].MapDataset != "uniprot" && res[4].Filter != `` {
		panic("Invalid second map filter")
	}

}

func TestFilterNotAllowed(t *testing.T) {

	// filter cant followed by filter
	query := `filter(type="biological_process").map(wikipedia).filter(name== "tpi1")`
	res, err = qparser.Parse(query)

	if err == nil {
		panic("dataset is not filterable error is expected")
	}
}

func TestSimple(t *testing.T) {

	// filter cant followed by filter
	query := `map(ufeature).filter(ufeature.type=="mutagenesis site" && ufeature.description.contains("cancer"))`
	res, err = qparser.Parse(query)

	if err != nil {
		panic("parse error")
	}
}
