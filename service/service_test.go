package service

import (
	"biobtree/conf"
	"fmt"
	"testing"
)

var loadConf = initConf()
var s service

func initConf() bool {

	c := conf.Conf{}
	c.Init("../", "", []string{}, []string{}, true)
	config = &c
	s = service{}
	s.init()
	return true

}

func TestMapFilter(t *testing.T) {

	ids := []string{"vav_mouse"}
	mapFilterQuery := `map(go).filter(go.type=="cellular_component").map(uniprot)`
	res := s.mapFilter(ids, mapFilterQuery)
	fmt.Println(res)

}
