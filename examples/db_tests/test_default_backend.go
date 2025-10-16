package main

import (
	"biobtree/db"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	fmt.Println("Testing Default Backend (should be MDBX)")
	fmt.Println("========================================\n")

	testDir := "./test_default_backend_db"
	os.RemoveAll(testDir)
	defer os.RemoveAll(testDir)

	// Create config WITHOUT specifying backend (should default to MDBX)
	appconf := map[string]string{
		"dbDir": testDir,
		// Note: NOT setting "dbBackend" - should default to MDBX
	}

	// Detect which backend will be used
	backend := db.GetBackendFromConfig(appconf)
	fmt.Printf("Default backend detected: %s\n", backend)

	if backend != db.BackendMDBX {
		log.Fatalf("❌ Expected MDBX as default, got %s", backend)
	}
	fmt.Println("✓ MDBX is the default backend\n")

	// Open database using the new abstraction
	d := db.DB{}
	env, dbi := d.OpenDBNew(true, 5000, appconf)
	defer env.Close()

	fmt.Println("✓ Database opened successfully with MDBX")

	// Perform a real test
	numRecords := 5000
	fmt.Printf("\nWriting %d records...\n", numRecords)
	startWrite := time.Now()

	err := env.Update(func(txn db.Txn) error {
		for i := 0; i < numRecords; i++ {
			key := fmt.Sprintf("biobtree_key_%06d", i)
			value := fmt.Sprintf("biobtree_value_%06d_with_some_test_data", i)
			err := txn.Put(dbi, []byte(key), []byte(value), 0)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to write: %v", err)
	}

	writeTime := time.Since(startWrite)
	fmt.Printf("✓ Wrote %d records in %v\n", numRecords, writeTime)
	fmt.Printf("  Performance: %.2f records/sec\n", float64(numRecords)/writeTime.Seconds())

	// Read test
	fmt.Printf("\nReading %d records...\n", numRecords)
	startRead := time.Now()
	readCount := 0

	err = env.View(func(txn db.Txn) error {
		for i := 0; i < numRecords; i++ {
			key := fmt.Sprintf("biobtree_key_%06d", i)
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
		log.Fatalf("Failed to read: %v", err)
	}

	readTime := time.Since(startRead)
	fmt.Printf("✓ Read %d records in %v\n", readCount, readTime)
	fmt.Printf("  Performance: %.2f records/sec\n", float64(readCount)/readTime.Seconds())

	fmt.Println("\n==================================================")
	fmt.Println("✅ SUCCESS! MDBX is working as the default backend")
	fmt.Println("==================================================")
	fmt.Println("\nTo use LMDB instead, add to your config:")
	fmt.Println(`  "dbBackend": "lmdb"`)
}
