package main

import (
	"fmt"
	"log"
	"strings"

	"../../../src/pbuf"

	"github.com/bmatsuo/lmdb-go/lmdb"
	"github.com/golang/protobuf/proto"
)

// First specify the lmdb directory.
const lmdbdir = ""

var readEnv *lmdb.Env
var readDbi lmdb.DBI

func main() {

	// init lmdb

	readEnv, readDbi = openDB(false, 1000000)

	// key to retrieve
	key := "tpi1"

	var values []*pbuf.Result

	id := strings.ToUpper(key)
	idres := getResult(id)
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

						jres := getResult(b.XrefId)
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
				values = append(values, &r2)
			}

		}

	}

	// print the result

	fmt.Println(values)

}

func getResult(key string) []byte {

	var v []byte
	err := readEnv.View(func(txn *lmdb.Txn) (err error) {
		//cur, err := txn.OpenCursor(s.readDbi)

		//_, v, err := cur.Get([]byte(identifier), nil, lmdb.SetKey)
		v, err = txn.Get(readDbi, []byte(key))

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
	return v

}

func openDB(write bool, totalKV int64) (*lmdb.Env, lmdb.DBI) {

	var err error
	var env *lmdb.Env
	var dbi lmdb.DBI
	env, err = lmdb.NewEnv()
	if err != nil {
		panic("Error while setting up lmdb env")
	}
	err = env.SetMaxDBs(1)
	if err != nil {
		panic("Error while setting up lmdb max db")
	}

	//err = env.SetMapSize(30 << 30)
	var lmdbAllocSize int64

	lmdbAllocSize = 1000000000 // 1GB

	err = env.SetMapSize(lmdbAllocSize)
	if err != nil {
		panic("Error while setting up lmdb map size")
	}

	if len(lmdbdir) <= 0 {
		panic("Specify the lmdb directory variable")
	}
	err = env.Open(lmdbdir, 0, 0700)

	if err != nil {
		panic(err)
	}

	staleReaders, err := env.ReaderCheck()
	if err != nil {
		panic("Error while checking lmdb stale readers.")
	}
	if staleReaders > 0 {
		log.Printf("cleared %d reader slots from dead processes", staleReaders)
	}

	err = env.Update(func(txn *lmdb.Txn) (err error) {
		dbi, err = txn.CreateDBI("mydb")
		return err
	})
	if err != nil {
		panic(err)
		//panic("Error while creating database. Clear the directory and try again.")
	}

	return env, dbi

}
