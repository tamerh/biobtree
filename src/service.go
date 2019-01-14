package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"./pbuf"
	"github.com/bmatsuo/lmdb-go/lmdb"
	"github.com/golang/protobuf/proto"
)

type service struct {
	readEnv  *lmdb.Env
	readDbi  lmdb.DBI
	pager    *pagekey
	pageSize int
}

func (s *service) init() {

	meta := make(map[string]interface{})
	f, err := ioutil.ReadFile(appconf["dbDir"] + "/db.meta.json")
	if err != nil {
		log.Fatalln("Error while reading meta information file which should be produced with generate command. Please make sure you did previous steps correctly.")
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	if err := json.Unmarshal(f, &meta); err != nil {
		panic(err)
	}

	totalkvline := meta["totalKVLine"].(float64)

	s.readEnv, s.readDbi = openDB(false, int64(totalkvline))
	s.pager = &pagekey{}
	s.pager.init()

	s.pageSize = 200
	if _, ok := appconf["pageSize"]; ok {
		s.pageSize, err = strconv.Atoi(appconf["pageSize"])
		if err != nil {
			panic("Invalid batchsize definition")
		}
	}

}

func (s *service) meta() *pbuf.BiobtreeMetaResponse {
	return nil
}

func (s *service) filter(id string, src uint32, filters []uint32, pageInd int) *pbuf.Result {

	//first we get the rootResult
	var filtered []*pbuf.XrefEntry
	idres := *s.getLmdbResult(id)
	var rootRes *pbuf.Xref
	if len(idres) > 0 {
		r1 := pbuf.Result{}
		err := proto.Unmarshal(idres, &r1)
		if err != nil {
			log.Fatal("unmarshaling error: ", err)
		}

		for _, e := range r1.Results {
			if e.DomainId == src {
				rootRes = e
				break
			}
		}
	}

	if pageInd == 0 {
		for _, f := range rootRes.Entries {
			for _, filter := range filters {
				if f.DomainId == filter {
					filtered = append(filtered, f)
				}
			}
		}

		if len(filtered) >= s.pageSize { //return here
			//todo this is duplicate code
			var filteredRes = pbuf.Result{}
			//filteredRes.Identifier = "1"
			var xrefs = make([]*pbuf.Xref, 1)
			var xref = pbuf.Xref{}
			xref.DomainId = src
			xref.DomainCounts = rootRes.DomainCounts
			xref.Entries = filtered
			xrefs[0] = &xref
			filteredRes.Results = xrefs

			return &filteredRes

		}
	}

	// now we will search in the pages
	keyLen := s.pager.keyLen(int(rootRes.Count / uint32(s.pageSize)))
	domainKey := s.pager.key(int(src), 2)
	var pageKey string
	err := s.readEnv.View(func(txn *lmdb.Txn) (err error) {

		var target *pbuf.Xref
		for {

			var r1 = pbuf.Result{}

			//k, v, err := cur.Get([]byte(id), nil, lmdb.Next)
			pageKey = id + spacestr + domainKey + spacestr + s.pager.key(pageInd, keyLen)
			v, err := txn.Get(s.readDbi, []byte(pageKey))

			if lmdb.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}

			err = proto.Unmarshal(v, &r1)
			if err != nil {
				log.Fatal("unmarshaling error: ", err)
			}

			target = nil
			for _, e := range r1.Results {
				if e.DomainId == src {
					target = e
					break
				}
			}

			if target != nil {
				for _, f := range target.Entries {
					for _, filter := range filters {
						if f.DomainId == filter {
							filtered = append(filtered, f)
						}
					}
				}
			}

			pageInd++

			if len(filtered) >= s.pageSize {
				return nil
			}

		}
	})

	if err != nil {
		// todo handle err
	}

	var filteredRes = pbuf.Result{}
	var xrefs = make([]*pbuf.Xref, 1)
	var xref = pbuf.Xref{}
	xref.DomainId = src
	xref.DomainCounts = rootRes.DomainCounts
	xref.Entries = filtered
	xref.Identifier = strconv.Itoa(pageInd)
	xrefs[0] = &xref
	filteredRes.Results = xrefs

	return &filteredRes

}

func (s *service) page(id string, src int, page int, t int) *pbuf.Result {

	keyLen := s.pager.keyLen(t)
	pk := s.pager.key(page, keyLen)
	srckey := s.pager.key(src, 2)
	var key strings.Builder
	key.WriteString(id)
	key.WriteString(spacestr)
	key.WriteString(srckey)
	key.WriteString(spacestr)
	key.WriteString(pk)
	idres := *s.getLmdbResult(key.String())
	if len(idres) > 0 {
		r1 := pbuf.Result{}

		err := proto.Unmarshal(idres, &r1)
		if err != nil {
			log.Fatal("unmarshaling error: ", err)
		}

		for _, xref := range r1.Results {
			xref.Identifier = id
		}
		return &r1
	}
	//todo handle this case
	return nil
}

func (s *service) search(ids []string) []*pbuf.Result {

	var res []*pbuf.Result
	for _, id := range ids {

		id = strings.ToUpper(id)
		idres := *s.getLmdbResult(id)
		var xrefs []*pbuf.Xref
		if len(idres) > 0 {
			r1 := pbuf.Result{}

			err := proto.Unmarshal(idres, &r1)
			if err != nil {
				log.Fatal("unmarshaling error: ", err)
			}

			if len(r1.Results) > 0 {

				for _, xref := range r1.Results {
					if xref.IsLink {
						for _, b := range xref.Entries {

							jres := *s.getLmdbResult(b.XrefId)
							r2 := pbuf.Result{}

							err = proto.Unmarshal(jres, &r2)
							if err != nil {
								log.Fatal("unmarshaling error: ", err)
							}
							//resultIndex := 0
							for _, rs2 := range r2.Results {
								//rs2.ExpandedQuery = b.XrefId
								if rs2.DomainId == b.DomainId {
									rs2.Identifier = b.XrefId
									rs2.SpecialKeyword = id
									xrefs = append(xrefs, rs2)
								}
							}
							//res = append(res, r2)
						}
					} else {
						xref.Identifier = id
						xrefs = append(xrefs, xref)
					}
				}

				if len(xrefs) > 0 {
					r2 := pbuf.Result{}
					r2.Results = xrefs
					res = append(res, &r2)
				}

			}

		}

	}

	return res

}

func (s *service) getLmdbResult(identifier string) *[]byte {

	var v []byte
	err := s.readEnv.View(func(txn *lmdb.Txn) (err error) {
		//cur, err := txn.OpenCursor(s.readDbi)

		//_, v, err := cur.Get([]byte(identifier), nil, lmdb.SetKey)
		v, err = txn.Get(s.readDbi, []byte(identifier))

		if lmdb.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		panic(err)
	}
	return &v

}
