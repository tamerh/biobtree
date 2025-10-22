package main

import (
	"fmt"
	"log"
	"os"

	"github.com/erigontech/mdbx-go/mdbx"
)

func main() {
	fmt.Println("Testing MDBX on NFS with different flag combinations...")

	// Test on NFS
	testDir := "./test_mdbx_nfs_data"
	os.RemoveAll(testDir)

	// Try different flag combinations
	flagTests := []struct{
		name string
		flags uint
	}{
		{"Default (0)", 0},
		{"Accede", mdbx.Accede},
		{"SafeNoSync", mdbx.SafeNoSync},
		{"Accede|SafeNoSync", mdbx.Accede | mdbx.SafeNoSync},
		{"WriteMap", mdbx.WriteMap},
		{"NoMetaSync", mdbx.NoMetaSync},
		{"Accede|NoMetaSync", mdbx.Accede | mdbx.NoMetaSync},
		{"Accede|WriteMap", mdbx.Accede | mdbx.WriteMap},
		{"SafeNoSync|WriteMap", mdbx.SafeNoSync | mdbx.WriteMap},
		{"NoMetaSync|WriteMap", mdbx.NoMetaSync | mdbx.WriteMap},
		{"NoSubdir", mdbx.NoSubdir},
		{"Exclusive", mdbx.Exclusive},
	}

	for _, test := range flagTests {
		fmt.Printf("\n[%s] Testing flags: %s (0x%x)...\n", testDir, test.name, test.flags)

		os.RemoveAll(testDir)
		err := os.MkdirAll(testDir, 0755)
		if err != nil {
			log.Printf("  ✗ Failed to create directory: %v\n", err)
			continue
		}

		env, err := mdbx.NewEnv(mdbx.Label("test"))
		if err != nil {
			log.Printf("  ✗ Failed to create env: %v\n", err)
			continue
		}

		err = env.SetOption(mdbx.OptMaxDB, 1)
		if err != nil {
			env.Close()
			log.Printf("  ✗ Failed to set options: %v\n", err)
			continue
		}

		// Try with and without geometry
		err = env.SetGeometry(-1, -1, 100*1024*1024, -1, -1, -1) // 100MB
		if err != nil {
			env.Close()
			log.Printf("  ✗ Failed to set geometry: %v\n", err)
			continue
		}

		err = env.Open(testDir, test.flags, 0644)
		if err != nil {
			env.Close()
			log.Printf("  ✗ Failed to open: %v\n", err)
			continue
		}

		// Try to write something
		var dbi mdbx.DBI
		err = env.Update(func(txn *mdbx.Txn) error {
			var err error
			dbi, err = txn.OpenDBI("testdb", mdbx.Create, nil, nil)
			if err != nil {
				return err
			}
			return txn.Put(dbi, []byte("test"), []byte("value"), 0)
		})

		env.Close()

		if err != nil {
			log.Printf("  ✗ Failed to write: %v\n", err)
		} else {
			fmt.Printf("  ✓ SUCCESS! This flag combination works on NFS!\n")
		}

		os.RemoveAll(testDir)
	}

	fmt.Println("\n=== Test Complete ===")
}
