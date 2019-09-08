package query

import (
	"biobtree/conf"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
)

var config *conf.Conf

type QueryParser struct {
}

type Query struct {
	MapDataset    string
	MapDatasetID  uint32
	Filter        string
	IsLinkDataset bool
	Program       cel.Program
}

func (q *QueryParser) Init(c *conf.Conf) {

	config = c

}

func (q *QueryParser) Parse(queryStr string) ([]Query, error) {

	var err error
	var result []Query
	currentQuery := Query{}
	var curPos int
	begining := true // this indicate begining of query set false after first map or filter
	var dataset string
	var val string

	// this set the current query to the end of query
	setNextQuery := func() {

		if begining {
			begining = false
		}

		result = append(result, currentQuery)

	}
	// this func get value between () from current position
	getValue := func() (string, error) {

		startFound := false
		startPos := 0
		var vals []rune

		for pos, c := range queryStr[curPos:] { // find start position

			if q.isWS(c) {
				continue
			}
			if c != '(' {
				err = fmt.Errorf("Invalid query. char ( not found near map ")
				return "", err
			}
			startFound = true
			startPos = curPos + pos + 1
			break
		}

		if !startFound || len(queryStr) < startPos {
			err = fmt.Errorf("Invalid query")
			return "", err
		}

		depth := 0
		for pos, c := range queryStr[startPos:] { // read value

			if c == '(' {
				depth++
			}
			if c == ')' {
				if depth > 0 {
					depth--
				} else {
					curPos = startPos + pos + 1
					break
				}
			}

			vals = append(vals, c)
		}

		return string(vals), nil
	}

	// START initial scan which can be filter or map
	queryStr = strings.TrimSpace(queryStr)
	if strings.HasPrefix(queryStr, "map") {

		curPos = 3
		goto scanMap

	} else if strings.HasPrefix(queryStr, "filter") {

		curPos = 6
		goto scanFilter

	} else {
		err = fmt.Errorf("Invalid query. query needs to start with map or filter")
		return nil, err
	}

scanMap:

	val, err = getValue()

	val = strings.TrimSpace(val)

	if err != nil {
		return nil, err
	}

	if len(val) == 0 {
		err = fmt.Errorf("Invalid query map cannot be empty")
		return nil, err
	}
	// set the dataset
	dataset = string(val)
	if _, ok := config.Dataconf[dataset]; !ok {
		err = fmt.Errorf("Invalid query. '" + dataset + "' is not a dataset")
		return nil, err
	}

	if _, ok := config.Dataconf[dataset]["linkdataset"]; ok {
		currentQuery.IsLinkDataset = true
	}

	currentQuery.MapDataset = dataset
	currentQuery.MapDatasetID = config.DataconfIDStringToInt[dataset]

	// either finish or go reading filter
	if curPos == len(queryStr) {
		setNextQuery()
		return result, nil
	}
	// next can be either map or filter
	if len(queryStr) >= curPos+4 && queryStr[curPos:curPos+4] == ".map" {
		setNextQuery()
		curPos = curPos + 4
		currentQuery = Query{}
		goto scanMap
	} else if len(queryStr) >= curPos+7 && queryStr[curPos:curPos+7] == ".filter" {
		curPos = curPos + 7
		goto scanFilter
	} else {
		err = fmt.Errorf("Invalid query")
		return nil, err
	}

scanFilter:

	val, err = getValue()

	val = strings.TrimSpace(val)

	if err != nil {
		return nil, err
	}

	if len(val) == 0 {
		err = fmt.Errorf("Invalid query filter cannot be empty")
		return nil, err
	}

	currentQuery.Filter = val

	// check that mapped dataset is filterable
	if ok := config.FilterableDatasets[currentQuery.MapDataset]; !ok && !begining {

		err = fmt.Errorf("Invalid query. " + currentQuery.MapDataset + " is not filterable dataset. It can be only mapped")
		return nil, err

	}

	if curPos == len(queryStr) {
		setNextQuery()
		return result, nil
	}

	// in this case next can be only map and it will be new query.
	if len(queryStr) < curPos+4 || queryStr[curPos:curPos+4] != ".map" {
		err = fmt.Errorf("Invalid query. Filter must follow by map ")
		return nil, err
	}
	curPos = curPos + 4
	setNextQuery()
	currentQuery = Query{} // now new query
	goto scanMap

}

func (q *QueryParser) isWS(in rune) bool {

	if in == ' ' || in == '\n' || in == '\t' || in == '\r' {
		return true
	}

	return false

}
