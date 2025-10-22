package main

import (
	"biobtree/db"
	"fmt"
	"log"
	"os"
	"time"
)

func testBackend(backend db.Backend, testDir string) error {
	fmt.Printf("\n=== Testing %s Backend ===\n", backend)

	// Clean up
	os.RemoveAll(testDir)
	defer os.RemoveAll(testDir)

	// Create config
	appconf := map[string]string{
		"dbDir":     testDir,
		"dbBackend": string(backend),
	}

	// Open database using the new abstraction
	d := db.DB{}
	env, dbi := d.OpenDBNew(true, 1000, appconf)
	defer env.Close()

	fmt.Println("✓ Database opened successfully")

	// Write test data
	numRecords := 1000
	fmt.Printf("Writing %d records...\n", numRecords)
	startWrite := time.Now()

	err := env.Update(func(txn db.Txn) error {
		for i := 0; i < numRecords; i++ {
			key := fmt.Sprintf("key_%06d", i)
			value := fmt.Sprintf("value_%06d_test_data", i)
			err := txn.Put(dbi, []byte(key), []byte(value), 0)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to write: %v", err)
	}

	writeTime := time.Since(startWrite)
	fmt.Printf("✓ Wrote %d records in %v (%.2f records/sec)\n",
		numRecords, writeTime, float64(numRecords)/writeTime.Seconds())

	// Read test data
	fmt.Printf("Reading %d records...\n", numRecords)
	startRead := time.Now()
	readCount := 0

	err = env.View(func(txn db.Txn) error {
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
		return fmt.Errorf("failed to read: %v", err)
	}

	readTime := time.Since(startRead)
	fmt.Printf("✓ Read %d records in %v (%.2f records/sec)\n",
		readCount, readTime, float64(readCount)/readTime.Seconds())

	fmt.Printf("✓ %s backend test passed!\n", backend)
	return nil
}

func main() {
	fmt.Println("Testing Database Abstraction Layer")
	fmt.Println("===================================")

	// Test LMDB backend
	if err := testBackend(db.BackendLMDB, "./test_abstraction_lmdb"); err != nil {
		log.Fatalf("LMDB test failed: %v", err)
	}

	// Test MDBX backend
	if err := testBackend(db.BackendMDBX, "./test_abstraction_mdbx"); err != nil {
		log.Fatalf("MDBX test failed: %v", err)
	}

	fmt.Println("\n=== All Tests Passed! ===")
	fmt.Println("\nThe abstraction layer works correctly with both backends.")
	fmt.Println("You can now switch between LMDB and MDBX by setting 'dbBackend'")
	fmt.Println("in your configuration file to either 'lmdb' or 'mdbx'.")
}
