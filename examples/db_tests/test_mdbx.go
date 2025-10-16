package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/erigontech/mdbx-go/mdbx"
)

func main() {
	fmt.Println("Testing MDBX-go library...")

	// Create test directory (testing on NFS with Exclusive flag)
	testDir := "./test_mdbx_data_nfs"
	os.RemoveAll(testDir)
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		log.Fatal("Failed to create test directory:", err)
	}
	defer os.RemoveAll(testDir)

	// Create MDBX environment with label
	env, err := mdbx.NewEnv(mdbx.Label("test"))
	if err != nil {
		log.Fatal("Failed to create MDBX env:", err)
	}
	defer env.Close()

	// Set options - max databases
	err = env.SetOption(mdbx.OptMaxDB, 1)
	if err != nil {
		log.Fatal("Failed to set max DBs:", err)
	}

	// Set map size (1GB for test)
	// Note: MDBX can auto-grow, but we set initial size
	err = env.SetGeometry(-1, -1, 1024*1024*1024, -1, -1, -1)
	if err != nil {
		log.Fatal("Failed to set geometry:", err)
	}

	// Open environment with Exclusive flag (works on NFS!)
	err = env.Open(testDir, mdbx.Exclusive, 0644)
	if err != nil {
		log.Fatal("Failed to open MDBX env:", err)
	}

	fmt.Println("✓ MDBX environment created successfully")

	// Open/create database
	var dbi mdbx.DBI
	err = env.Update(func(txn *mdbx.Txn) error {
		var err error
		dbi, err = txn.OpenDBI("testdb", mdbx.Create, nil, nil)
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

	err = env.Update(func(txn *mdbx.Txn) error {
		for i := 0; i < numRecords; i++ {
			key := fmt.Sprintf("key_%06d", i)
			value := fmt.Sprintf("value_%06d_test_data_for_mdbx", i)
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

	err = env.View(func(txn *mdbx.Txn) error {
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

	// Test cursor iteration
	fmt.Println("Testing cursor iteration...")
	startIter := time.Now()
	iterCount := 0

	err = env.View(func(txn *mdbx.Txn) error {
		cur, err := txn.OpenCursor(dbi)
		if err != nil {
			return err
		}
		defer cur.Close()

		for {
			k, v, err := cur.Get(nil, nil, mdbx.Next)
			if mdbx.IsNotFound(err) {
				break
			}
			if err != nil {
				return err
			}
			if len(k) > 0 && len(v) > 0 {
				iterCount++
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("Failed to iterate:", err)
	}

	iterTime := time.Since(startIter)
	fmt.Printf("✓ Iterated %d records in %v (%.2f records/sec)\n",
		iterCount, iterTime, float64(iterCount)/iterTime.Seconds())

	// Get stats
	var stat *mdbx.Stat
	err = env.View(func(txn *mdbx.Txn) error {
		var err error
		stat, err = txn.StatDBI(dbi)
		return err
	})
	if err != nil {
		log.Fatal("Failed to get stats:", err)
	}

	fmt.Println("\n=== Database Statistics ===")
	fmt.Printf("Entries: %d\n", stat.Entries)
	fmt.Printf("Branch pages: %d\n", stat.BranchPages)
	fmt.Printf("Leaf pages: %d\n", stat.LeafPages)
	fmt.Printf("Overflow pages: %d\n", stat.OverflowPages)

	fmt.Println("\n✓ All MDBX tests passed!")
	fmt.Println("\nMDBX is working correctly and ready to use.")
}
