package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bmatsuo/lmdb-go/lmdb"
)

func main() {
	fmt.Println("Testing LMDB library...")

	// Create test directory (testing on NFS for fair comparison with MDBX)
	testDir := "./test_lmdb_data_nfs"
	os.RemoveAll(testDir)
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		log.Fatal("Failed to create test directory:", err)
	}
	defer os.RemoveAll(testDir)

	// Create LMDB environment
	env, err := lmdb.NewEnv()
	if err != nil {
		log.Fatal("Failed to create LMDB env:", err)
	}
	defer env.Close()

	// Set max databases
	err = env.SetMaxDBs(1)
	if err != nil {
		log.Fatal("Failed to set max DBs:", err)
	}

	// Set map size (1GB for test)
	err = env.SetMapSize(1024 * 1024 * 1024)
	if err != nil {
		log.Fatal("Failed to set map size:", err)
	}

	// Open environment
	err = env.Open(testDir, lmdb.WriteMap, 0644)
	if err != nil {
		log.Fatal("Failed to open LMDB env:", err)
	}

	fmt.Println("✓ LMDB environment created successfully")

	// Open/create database
	var dbi lmdb.DBI
	err = env.Update(func(txn *lmdb.Txn) error {
		var err error
		dbi, err = txn.CreateDBI("testdb")
		return err
	})
	if err != nil {
		log.Fatal("Failed to create DBI:", err)
	}

	fmt.Println("✓ Database created successfully")

	// Write test data
	numRecords := 1000
	fmt.Printf("Writing %d records...\n", numRecords)
	startWrite := time.Now()

	err = env.Update(func(txn *lmdb.Txn) error {
		for i := 0; i < numRecords; i++ {
			key := fmt.Sprintf("key_%06d", i)
			value := fmt.Sprintf("value_%06d_test_data_for_lmdb", i)
			err := txn.Put(dbi, []byte(key), []byte(value), 0)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("Failed to write data:", err)
	}

	writeTime := time.Since(startWrite)
	fmt.Printf("✓ Wrote %d records in %v (%.2f records/sec)\n",
		numRecords, writeTime, float64(numRecords)/writeTime.Seconds())

	// Read test data
	fmt.Printf("Reading %d records...\n", numRecords)
	startRead := time.Now()
	readCount := 0

	err = env.View(func(txn *lmdb.Txn) error {
		for i := 0; i < numRecords; i++ {
			key := fmt.Sprintf("key_%06d", i)
			val, err := txn.Get(dbi, []byte(key))
			if err != nil {
				return err
			}
			if len(val) == 0 {
				return fmt.Errorf("empty value for key %s", key)
			}
			readCount++
		}
		return nil
	})
	if err != nil {
		log.Fatal("Failed to read data:", err)
	}

	readTime := time.Since(startRead)
	fmt.Printf("✓ Read %d records in %v (%.2f records/sec)\n",
		readCount, readTime, float64(readCount)/readTime.Seconds())

	fmt.Println("\n✓ All LMDB tests passed!")
	fmt.Println("\nLMDB is working correctly on this filesystem.")
}
