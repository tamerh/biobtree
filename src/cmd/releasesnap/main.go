// releasesnap writes one release metadata snapshot for an existing output tree,
// using the same code path the generate phase runs at the end of every build.
//
// It exists for two reasons:
//   - backfill a snapshot for a release that finished before the snapshot code
//     shipped (e.g. main db_v56);
//   - verify the snapshot payload without waiting for a full generate.
//
// Usage (from src/):
//
//	go run ./cmd/releasesnap -out ../out_prod -conf ../conf -federation main -db-version db_v56
package main

import (
	"flag"
	"fmt"
	"os"

	"biobtree/configs"
	"biobtree/update"
)

func main() {
	outDir := flag.String("out", "out_prod", "output directory containing <federation>/db symlinks")
	confDir := flag.String("conf", "conf", "configuration directory")
	federation := flag.String("federation", "main", "federation this build belongs to")
	dbVersion := flag.String("db-version", "", "db_vN of the build (default: resolved from <federation>/db)")
	binaryVersion := flag.String("binary-version", "unknown", "biobtree binary version to record")
	flag.Parse()

	conf := &configs.Conf{}
	conf.Init(*confDir, *binaryVersion, *outDir, true)
	update.InitConfig(conf)

	dbv := *dbVersion
	if dbv == "" {
		if target, err := os.Readlink(*outDir + "/" + *federation + "/db"); err == nil {
			dbv = target
		}
	}
	if dbv == "" {
		fmt.Fprintln(os.Stderr, "error: no -db-version and cannot resolve <federation>/db")
		os.Exit(1)
	}
	if err := update.WriteReleaseSnapshot(*outDir, *federation, dbv, *binaryVersion); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("release snapshot written: %s/releases (%s %s)\n", *outDir, *federation, dbv)
}
