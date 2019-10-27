package db

import (
	"log"
	"path/filepath"
	"strconv"

	"github.com/bmatsuo/lmdb-go/lmdb"
)

type DB struct {
}

func (d *DB) OpenDB(write bool, totalKV int64, appconf map[string]string) (*lmdb.Env, lmdb.DBI) {

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
	if _, ok := appconf["lmdbAllocSize"]; ok {
		lmdbAllocSize, err = strconv.ParseInt(appconf["lmdbAllocSize"], 10, 64)
		if err != nil {
			panic("Invalid lmdbAllocSize definition")
		}
		if lmdbAllocSize <= 1 {
			panic("lmdbAllocSize must be greater than 1")
		}
	} else {

		if totalKV < 1_000_000 { //1M
			lmdbAllocSize = 1_000_000_000 // 1GB
		} else if totalKV < 50_000_000 {
			lmdbAllocSize = 5_000_000_000
		} else if totalKV < 100_000_000 {
			lmdbAllocSize = 10_000_000_000
		} else if totalKV < 150_000_000 {
			lmdbAllocSize = 15_000_000_000
		} else if totalKV < 200_000_000 {
			lmdbAllocSize = 20_000_000_000
		} else if totalKV < 300_000_000 {
			lmdbAllocSize = 30_000_000_000
		} else if totalKV < 500_000_000 {
			lmdbAllocSize = 50_000_000_000
		} else if totalKV < 1_000_000_000 {
			lmdbAllocSize = 100_000_000_000
		} else { // todo review again
			lmdbAllocSize = 1.4 * 1000 * 1000 * 1000 * 1000 // TB
		}

	}

	err = env.SetMapSize(lmdbAllocSize)
	if err != nil {
		panic("Error while setting up lmdb map size")
	}

	if write {
		err = env.Open(filepath.FromSlash(appconf["dbDir"]), lmdb.WriteMap, 0700)
	} else {
		err = env.Open(appconf["dbDir"], 0, 0700)
	}

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
	}

	return env, dbi

}

func (d *DB) OpenAliasDB(write bool, size int64, appconf map[string]string) (*lmdb.Env, lmdb.DBI) {

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

	var lmdbSize = size * 2

	err = env.SetMapSize(lmdbSize)
	if err != nil {
		panic("Error while setting up lmdb map size")
	}

	if write {
		err = env.Open(filepath.FromSlash(appconf["aliasDbDir"]), lmdb.WriteMap, 0700)
	} else {
		err = env.Open(appconf["aliasDbDir"], 0, 0700)
	}

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
		dbi, err = txn.CreateDBI("mydb2")
		return err
	})
	if err != nil {
		panic(err)
	}

	return env, dbi

}
