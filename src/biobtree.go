package main

import (
	"biobtree/configs"
	"biobtree/generate"
	"biobtree/service"
	"biobtree/update"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli"
)

const version = "1.8.0"
const versionTag = "v1.8.0"

var config *configs.Conf

var allDatasets = []string{"uniprot", "go", "eco", "efo", "hgnc", "chebi", "taxonomy",
	"interpro", "hmdb", "literature_mappings", "ensembl",
	"uniparc", "uniref50", "uniref90", "uniref100", "my_data", "uniprot_unreviewed",
	"ensembl_bacteria", "ensembl_fungi", "ensembl_metazoa", "ensembl_plants", "ensembl_protists",
	"chembl_document", "chembl_assay", "chembl_activity", "chembl_molecule", "chembl_target", "chembl_target_component", "chembl_cell_line"}

func main() {

	app := cli.NewApp()
	app.Name = "biobtree"
	app.Version = version
	app.Usage = "A tool to search and map bioinformatics identifiers and special keywords"
	app.Copyright = ""
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Tamer Gür",
			Email: "tgur@ebi.ac.uk",
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "uniprot-ftp,f",
			Value: "UK",
			Usage: "uniprot ftp mirrors. Switzerland, USA or UK",
		},
		cli.StringFlag{
			Name: "datasets,d",
			//Value: "uniprot_reviewed,taxonomy,hgnc,chebi,interpro,uniprot_unreviewed,uniparc,uniref100,uniref50,uniref90,my_data,literature_mappings,hmdb,ensembl,ensembl_bacteria,ensembl_fungi,ensembl_metazoa,ensembl_plants,ensembl_protists",
			Usage: "change default source datasets if value starts with + sign it means default dataset and given dataset. List of datasets are uniprot,taxonomy,hgnc,chebi,interpro,uniparc,uniref50,uniref90,uniref100,my_data,uniprot_unreviewed,literature_mappings,hmdb,ensembl,ensembl_bacteria,ensembl_fungi,ensembl_metazoa,ensembl_plants,ensembl_protists,go",
		},
		cli.StringFlag{
			Name:  "target-datasets,t",
			Usage: "specify target datasets. By default all datasets are included. Speeds up all process. Useful if only certain mappings are interested.",
		},
		cli.StringFlag{
			Name:  "idx,i",
			Usage: "for indexing in multiple machine. Set unique number or text for each process. No need to specify for single process",
		},
		cli.StringFlag{
			Name:  "out-dir",
			Usage: "change the output directory by default it is out folder in the same directory",
		},
		cli.StringFlag{
			Name:   "confdir",
			Hidden: true,
			Usage:  "to change default config directory while developing",
		},
		cli.StringFlag{
			Name:   "lmdb-alloc-size,lmdbsize",
			Hidden: true,
			Usage:  "specify lmdb alloc size",
		},
		cli.StringFlag{
			Name:  "pre-built,p",
			Usage: "With this command pre built data automatically installed. Please check README for values of this parameter with data that are included for each value",
		},
		cli.BoolFlag{
			Name:  "clean,c",
			Usage: "Clean existing data before update command (default keeps existing data)",
		},
		cli.BoolFlag{
			Name:  "include-optionals",
			Usage: "when this flag sets optional dataset which defined in the optional.dataset.json file includes for mapping. by default it is false",
		},
		cli.BoolFlag{
			Name:  "ensembl-orthologs,eo",
			Usage: "For ensembls by default only gff3 files are processed. For orthologs and other mappings this parameter must be set",
		},
		cli.StringFlag{
			Name:  "ensembl-orthologs-taxids,otaxids",
			Usage: "This param has same affect as 'ensembl-orthologs' but in addition fetched given comma seperated taxid for orthologs",
		},
		cli.BoolFlag{
			Name:  "ensembl-orthologs-all,eoa",
			Usage: "When ensembl-orthologs is set only given taxonomies orthologs are processed if this parameter is set all orthologs and some extra mappings are included",
		},
		cli.BoolFlag{
			Name:   "no-timezone-check",
			Usage:  "By default from user timezone it is checked that if user using the tool from US (between gmt -4 and gmt -9) if this is true then US ftp mirrors are used for applicable datasets e.g uniprot.",
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "maxcpu",
			Hidden: true,
			Usage:  "sets the maximum number of CPUs that can be executing simultaneously. By default biobtree uses all the CPUs when applicable.",
		},
		cli.StringFlag{
			Name:  "genome,s",
			Usage: "Genome names for ensembl datasets",
		},
		cli.StringFlag{
			Name: "genome-pattern,sp",
			//Value: "homo_sapiens",
			Usage: "Genome names pattern for ensembl datasets. e.g 'salmonella' which gets all genomes of salmonella species in ensembl",
		},
		cli.StringFlag{
			Name:  "genome-taxids,tax",
			Usage: "Process all the genomes belongs to given taxonomy ids seperated by comma",
		},
		cli.BoolFlag{
			Name:  "no-web-popup,np",
			Usage: "When opening the web application don't trigger opening popup",
		},
		cli.BoolFlag{
			Name:  "skip-ensembl,se",
			Usage: "During uniprot data update when taxids selected this paramater is used to just index uniprot",
		},
		cli.BoolFlag{
			Name:  "profile",
			Usage: "Enable CPU and memory profiling. Writes cpuprof.out and memprof.out files",
		},
		cli.BoolFlag{
			Name:  "lookupdb",
			Usage: "Enable loading lookup database during update. Default is false to save memory on development machines",
		},
		cli.StringFlag{
			Name:  "lmdb-safety-factor",
			Usage: "LMDB map size safety factor multiplier for generate command (default from config: 10)",
		},
		cli.IntFlag{
			Name:  "bucket-sort-workers",
			Usage: "Number of parallel workers for bucket file sorting (default from config: 8). Use lower values for large datasets like dbSNP to reduce memory usage",
		},
		cli.IntFlag{
			Name:  "pubchem-sdf-workers",
			Usage: "Number of parallel workers for PubChem SDF file parsing (default: 4). Use lower values to reduce memory usage during SDF processing",
		},
		cli.BoolFlag{
			Name:  "resume-sort",
			Usage: "Resume from sorting phase. Skip data processing and only run sort, concatenate, and write meta.json. Use after a crash during sorting",
		},
		cli.BoolFlag{
			Name:  "force",
			Usage: "Force reprocessing of datasets even if they haven't changed. Ignores the dataset state file for specified datasets",
		},
		cli.BoolFlag{
			Name:  "prod",
			Usage: "Run in production mode with alternate ports (prodHttpPort/prodGrpcPort from config, defaults: 9291/7776)",
		},
	}

	// add dataset local flags
	for _, dataset := range allDatasets {
		app.Flags = append(app.Flags, cli.StringFlag{
			Name:   dataset + ".file",
			Hidden: true,
		})
	}

	app.Commands = []cli.Command{
		{
			Name:  "start",
			Usage: "Shortcut command to execute update, generate and web",
			Action: func(c *cli.Context) error {
				return runStartCommand(c)
			},
		},
		{
			Name:  "build",
			Usage: "Shortcut command to execute update and generate. ",
			Action: func(c *cli.Context) error {
				return runBuildCommand(c)
			},
		},
		{
			Name:  "test",
			Usage: "Build database in test mode with limited datasets for testing",
			Action: func(c *cli.Context) error {
				return runTestCommand(c)
			},
		},
		{
			Name:  "update",
			Usage: "Fetch selected datsets and produce chunk files",
			Action: func(c *cli.Context) error {
				return runUpdateCommand(c)
			},
		},
		{
			Name:  "generate",
			Usage: "Merge chunks from update command and generate LMDB database",
			Action: func(c *cli.Context) error {
				return runGenerateCommand(c)
			},
		},
		{
			Name:  "web",
			Usage: "Start web server for ws, ui and grpc",
			Action: func(c *cli.Context) error {
				return runWebCommand(c)
			},
		},
		{
			Name:      "query",
			Usage:     "Query biobtree database from CLI",
			ArgsUsage: "<query_string>",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dataset,s",
					Usage: "Filter results by dataset name (e.g., uniprot, string)",
				},
				cli.StringFlag{
					Name:  "mode,m",
					Value: "full",
					Usage: "Response mode: 'full' (detailed with attributes) or 'lite' (compact IDs only)",
				},
			},
			Action: func(c *cli.Context) error {
				return runQueryCommand(c)
			},
		},
		{
			Name:  "install",
			Usage: "Install configuration files. Used for genomes and datasets listing",
			Action: func(c *cli.Context) error {
				return runInstallCommand(c)
			},
		},
		{
			Name:   "alias",
			Usage:  "Recreates alias db this is used if new aliases wants to added while keeping existing state",
			Hidden: true,
			Action: func(c *cli.Context) error {
				return runAliasCommand(c)
			},
		},
		{
			Name:  "check",
			Usage: "Check if datasets have changed at source without building. Use to preview what would be updated.",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "all,a",
					Usage: "Check all datasets defined in source*.dataset.json (ignores -d flag)",
				},
			},
			Action: func(c *cli.Context) error {
				return runCheckCommand(c)
			},
		},
		{
			Name:   "profile",
			Hidden: true,
			Usage:  "profile the",
			Action: func(c *cli.Context) error {
				return runProfileCommand(c)
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}

func runAliasCommand(c *cli.Context) error {
	log.Printf("Alias command deprecated - aliases are now loaded from conf/aliases.json at runtime")
	return nil
}

func runStartCommand(c *cli.Context) error {

	err := runUpdateCommand(c)
	err = runGenerateCommand(c)
	err = runWebCommand(c)
	return err

}

func runBuildCommand(c *cli.Context) error {

	err := runUpdateCommand(c)
	err = runGenerateCommand(c)
	return err

}

func runTestCommand(c *cli.Context) error {

	log.Println("════════════════════════════════════════════════════════════")
	log.Println("  BiobtreeV2 Test Mode")
	log.Println("════════════════════════════════════════════════════════════")
	log.Println()

	start := time.Now()

	// Initialize configuration with test output directory
	confdir := c.GlobalString("confdir")
	// Force test output directory (ignore --out-dir flag)
	testOutDir := "test_out"
	includeOptionals := c.GlobalBool("include-optionals")

	config = &configs.Conf{}
	config.Init(confdir, versionTag, testOutDir, includeOptionals)

	// Enable test mode
	config.TestMode = true
	config.TestOutputDir = testOutDir
	config.TestRefDir = testOutDir + "/reference"

	// Create test directories
	testDirs := []string{
		"test_out",
		"test_out/db",
		"test_out/aliasdb",
		"test_out/reference",
		"test_out/logs",
	}

	log.Println("Step 1: Creating test directories...")
	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Error creating directory %s: %v", dir, err)
		}
	}

	// Clear reference directory to remove old ID files from previous test runs
	refDir := testOutDir + "/reference"
	if entries, err := os.ReadDir(refDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_ids.txt") {
				filePath := refDir + "/" + entry.Name()
				if err := os.Remove(filePath); err != nil {
					log.Printf("Warning: Could not remove old reference file %s: %v", filePath, err)
				}
			}
		}
	}

	log.Println("✓ Test directories created")
	log.Println()

	// Show which datasets will be processed with limits
	// Only show datasets that have test_entries_count configured
	log.Println("Step 2: Test configuration:")
	log.Println("  Datasets with test limits configured:")
	hasTestConfig := false
	for dsName, dsConfig := range config.Dataconf {
		// Only show if test_entries_count is explicitly configured
		if _, ok := dsConfig["test_entries_count"]; ok {
			limit := config.GetTestLimit(dsName)
			if limit > 0 {
				log.Printf("    - %s: %d entries", dsName, limit)
				hasTestConfig = true
			} else if limit == -1 {
				log.Printf("    - %s: FULL dataset", dsName)
				hasTestConfig = true
			}
		}
	}

	if !hasTestConfig {
		log.Println("    (No datasets have test_entries_count configured)")
		log.Println("    (All selected datasets will be processed in FULL)")
	}

	// Show Ensembl species if configured
	testSpecies := config.GetTestSpecies()
	if len(testSpecies) > 0 {
		log.Printf("  Ensembl test species: %v", testSpecies)
	}

	// Show which datasets will actually be processed
	indataset := c.GlobalString("datasets")
	if len(indataset) > 0 {
		log.Printf("  Datasets to process (-d flag): %s", indataset)
	} else {
		log.Println("  No datasets specified with -d flag")
		log.Println("  Use: ./biobtree -d \"hgnc\" test")
	}
	log.Println()

	// Run update and generate
	log.Println("Step 3: Running data processing (update + generate)...")
	log.Println()

	err := runUpdateCommand(c)
	if err != nil {
		log.Printf("✗ Update failed: %v", err)
		return err
	}

	err = runGenerateCommand(c)
	if err != nil {
		log.Printf("✗ Generate failed: %v", err)
		return err
	}

	elapsed := time.Since(start)

	log.Println()
	log.Println("════════════════════════════════════════════════════════════")
	log.Println("✓ Test database build complete")
	log.Printf("  Time: %s", elapsed)
	log.Println("  Output: test_out")
	log.Println("  Reference IDs: test_out/reference/")
	log.Println("════════════════════════════════════════════════════════════")

	return nil
}

func runUpdateCommand(c *cli.Context) error {

	start := time.Now()

	// Check if profiling is enabled
	profileEnabled := c.GlobalBool("profile")
	if profileEnabled {
		os.Remove("cpuprof.out")
		os.Remove("memprof.out")
		f, err := os.Create("cpuprof.out")
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer func() {
			pprof.StopCPUProfile()
			f.Close()
			log.Println("CPU profile written to cpuprof.out")

			// Write memory profile
			f2, err := os.Create("memprof.out")
			if err != nil {
				log.Fatal("could not create memory profile: ", err)
			}
			runtime.GC()
			if err := pprof.WriteHeapProfile(f2); err != nil {
				log.Fatal("could not write memory profile: ", err)
			}
			f2.Close()
			log.Println("Memory profile written to memprof.out")
		}()
		log.Println("Profiling enabled - will write cpuprof.out and memprof.out")
	}

	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	includeOptionals := c.GlobalBool("include-optionals")

	// Only initialize config if not already initialized (e.g., by test command)
	if config == nil || !config.TestMode {
		config = &configs.Conf{}
		config.Init(confdir, versionTag, outDir, includeOptionals)
	}

	indataset := c.GlobalString("datasets")

	d := map[string]bool{}

	if len(indataset) > 0 {
		for _, dt := range strings.Split(indataset, ",") {
			d[dt] = true
		}
	}

	genomeSelectionWay := 0
	s := c.GlobalString("genome")
	var sp []string
	if len(s) > 0 {
		genomeSelectionWay++
		sp = strings.Split(s, ",")
	}

	spattern := c.GlobalString("genome-pattern")
	var spatterns []string
	if len(spattern) > 0 {
		genomeSelectionWay++
		spatterns = strings.Split(spattern, ",")
	}

	genometaxidsStr := c.GlobalString("genome-taxids")
	var genometaxids []int
	if len(genometaxidsStr) > 0 {
		genomeSelectionWay++
		for _, s := range strings.Split(genometaxidsStr, ",") {
			taxid, err := strconv.Atoi(s)
			if err != nil {
				log.Fatalf("Invalid taxonomy id %s", s)
			}
			genometaxids = append(genometaxids, taxid)
		}
	}

	if genomeSelectionWay > 1 {
		log.Fatal("Genome can be selected with one way among 3 selection paramters.")
		return nil
	}

	// Resume mode doesn't need datasets - it works with existing bucket files
	if len(d) == 0 && len(sp) == 0 && len(spatterns) == 0 && len(genometaxids) == 0 && !c.GlobalBool("resume-sort") {

		log.Fatal("Datasets or genome must be selected.")
		return nil

	}

	// Clean output directory before update if --clean flag is set
	clean := c.GlobalBool("clean")
	if clean {
		config.CleanOutDirs()
	}

	config.Appconf["uniprot_ftp"] = c.GlobalString("uniprot-ftp")

	noZoneCheck := c.GlobalBool("no-timezone-check")

	if len(config.Appconf["uniprot_ftp"]) == 0 && !noZoneCheck {

		t := time.Now()
		_, offset := t.Zone()

		if offset <= -14400 && offset >= -32400 {
			log.Println("Uniprot USA ftp mirror set")
			config.Appconf["uniprot_ftp"] = "USA"
		}

	}

	t := c.GlobalString("target-datasets")
	var ts []string
	if len(t) > 0 {
		ts = strings.Split(t, ",")
	}

	chunkIdxx := c.GlobalString("idx")
	if len(chunkIdxx) == 0 {
		chunkIdxx = strconv.Itoa(time.Now().Nanosecond())
	}

	cpu := c.GlobalInt("maxcpu")
	if cpu > 1 {
		runtime.GOMAXPROCS(cpu)
	}

	// check local file path settings
	for _, dset := range allDatasets {
		if len(c.GlobalString(dset+".file")) > 0 {
			config.Dataconf[dset]["path"] = c.GlobalString(dset + ".file")
			config.Dataconf[dset]["useLocalFile"] = "yes"
		}
	}

	eo := c.GlobalBool("ensembl-orthologs")

	orthologIDs := map[int]bool{}
	if len(c.GlobalString("ensembl-orthologs-taxids")) > 0 {
		eo = true
		for _, tax := range strings.Split(c.GlobalString("ensembl-orthologs-taxids"), ",") {
			taxid, err := strconv.Atoi(tax)
			if err != nil {
				log.Fatalf("Invalid taxonomy id %s", s)
			}
			orthologIDs[taxid] = true
		}
	}

	useLookupDB := c.GlobalBool("lookupdb")

	// Override bucket sort workers if specified via CLI
	bucketSortWorkers := c.GlobalInt("bucket-sort-workers")
	if bucketSortWorkers > 0 {
		config.Appconf["bucketSortWorkers"] = strconv.Itoa(bucketSortWorkers)
	}

	// Override PubChem SDF workers if specified via CLI
	pubchemSDFWorkers := c.GlobalInt("pubchem-sdf-workers")
	if pubchemSDFWorkers > 0 {
		config.Appconf["pubchemSDFWorkers"] = strconv.Itoa(pubchemSDFWorkers)
	}

	// Create and run the data update
	dataUpdate := update.NewDataUpdate(d, ts, sp, spatterns, genometaxids, c.GlobalBool("skip-ensembl"), orthologIDs, eo, c.GlobalBool("ensembl-orthologs-all"), config, chunkIdxx, useLookupDB)

	// Resume mode: skip data processing, only run sort+concat+meta
	if c.GlobalBool("resume-sort") {
		dataUpdate.SetResumeSort(true)
	}

	// Force mode: reprocess datasets even if unchanged
	if c.GlobalBool("force") {
		dataUpdate.SetForceRebuild(true)
	}

	dataUpdate.Update()

	elapsed := time.Since(start)
	log.Printf("Update took %s", elapsed)

	return nil

}

func runGenerateCommand(c *cli.Context) error {

	log.Println("Generate running please wait...")

	start := time.Now()

	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	lmdbAllocSize := c.GlobalString("lmdb-alloc-size")

	// Only initialize config if not already initialized (e.g., by test command)
	if config == nil || !config.TestMode {
		config = &configs.Conf{}
		config.Init(confdir, versionTag, outDir, true)
	}

	if len(lmdbAllocSize) > 0 {
		config.Appconf["lmdbAllocSize"] = lmdbAllocSize
	}

	// Override lmdbSafetyFactor from command line if provided
	lmdbSafetyFactor := c.GlobalString("lmdb-safety-factor")
	if len(lmdbSafetyFactor) > 0 {
		config.Appconf["lmdbSafetyFactor"] = lmdbSafetyFactor
	}

	cpu := c.GlobalInt(" maxcpu")
	if cpu > 1 {
		runtime.GOMAXPROCS(cpu)
	}

	clean := c.GlobalBool("clean")
	keep := !clean // invert: clean=false means keep=true

	var d = generate.Merge{}

	d.Merge(config, keep)

	elapsed := time.Since(start)
	log.Printf("Generate took %s", elapsed)

	return nil

}

func runWebCommand(c *cli.Context) error {

	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	config = &configs.Conf{}
	config.Init(confdir, versionTag, outDir, true)

	cpu := c.GlobalInt(" maxcpu")
	if cpu > 1 {
		runtime.GOMAXPROCS(cpu)
	}

	web := service.Web{}
	web.Start(config, c.GlobalBool("no-web-popup"), c.GlobalBool("prod"))

	return nil

}

func runQueryCommand(c *cli.Context) error {
	// 1. Check arguments
	if c.NArg() == 0 {
		return fmt.Errorf("query string required\nUsage: biobtree --out-dir <dir> query \"<query_string>\"\nExamples:\n  biobtree query P27348\n  biobtree query \"CHEMBL203 >> surechembl >> patent\"\n  biobtree query -m lite P27348")
	}
	queryStr := c.Args().First()

	// 2. Initialize config (same as web command)
	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	config = &configs.Conf{}
	config.Init(confdir, versionTag, outDir, true)

	// 3. Execute query via CLI handler
	cliHandler := service.CLI{}
	datasetFilter := c.String("dataset")
	mode := c.String("mode")
	// Validate mode
	if mode != "full" && mode != "lite" {
		return fmt.Errorf("invalid mode '%s': must be 'full' or 'lite'", mode)
	}
	return cliHandler.Query(config, queryStr, datasetFilter, mode)
}

func runInstallCommand(c *cli.Context) error {

	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	preBuildSet := c.GlobalString("pre-built")
	config = &configs.Conf{}
	config.Install(confdir, versionTag, outDir, preBuildSet, true)

	return nil

}

func runCheckCommand(c *cli.Context) error {
	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	config = &configs.Conf{}
	config.Init(confdir, versionTag, outDir, true)

	// Initialize update package config for source checking
	update.InitConfig(config)

	// Get datasets to check
	var datasets []string
	if c.Bool("all") {
		// Get all datasets from config (only those with path defined - source datasets)
		for name, props := range config.Dataconf {
			// Skip aliases
			if props["_alias"] == "true" {
				continue
			}
			// Skip link datasets (no path)
			if _, hasPath := props["path"]; !hasPath {
				continue
			}
			datasets = append(datasets, name)
		}
		sort.Strings(datasets) // Sort for consistent output
		fmt.Printf("Checking all %d source datasets...\n", len(datasets))
	} else {
		datasetsStr := c.GlobalString("datasets")
		if datasetsStr == "" {
			log.Println("Error: datasets must be specified with -d flag, or use --all to check all datasets")
			return nil
		}
		datasets = strings.Split(datasetsStr, ",")
	}

	indexDir := config.Appconf["indexDir"]

	// Load state
	state, err := update.LoadDatasetState(indexDir)
	if err != nil {
		log.Printf("Warning: Could not load dataset state: %v", err)
		state = update.NewDatasetState()
	}

	fmt.Println("\n=== Dataset Source Change Check ===\n")
	fmt.Printf("%-25s %-15s %-10s %s\n", "DATASET", "SOURCE TYPE", "CHANGED", "DETAILS")
	fmt.Printf("%-25s %-15s %-10s %s\n", strings.Repeat("-", 25), strings.Repeat("-", 15), strings.Repeat("-", 10), strings.Repeat("-", 40))

	var changedCount, unchangedCount, unknownCount int

	var notBuiltCount int

	for _, dsName := range datasets {
		dsName = strings.TrimSpace(dsName)
		lastBuild := state.GetDatasetInfo(dsName)

		// If dataset was never built, no need to check remote - just show "not built"
		if lastBuild == nil {
			fmt.Printf("%-25s %-15s %-10s %s\n", dsName, "not_built", "-", "never built - will build on first run")
			notBuiltCount++
			continue
		}

		changeInfo, err := update.CheckSourceChanged(dsName, lastBuild)
		if err != nil {
			fmt.Printf("%-25s %-15s %-10s %s\n", dsName, "ERROR", "-", err.Error())
			unknownCount++
			continue
		}

		// Determine details based on source type and check method
		var details string
		var displayType string
		isUnknown := false

		// Handle force_rebuild specially
		if changeInfo.CheckMethod == "force_rebuild" {
			displayType = "force_rebuild"
			details = "always rebuilds"
		} else {
			displayType = string(changeInfo.SourceType)
			switch changeInfo.SourceType {
			case update.SourceTypeLocal:
				if changeInfo.Error != "" {
					details = changeInfo.Error
				} else if !changeInfo.NewDate.IsZero() {
					details = fmt.Sprintf("modified: %s", changeInfo.NewDate.Format("2006-01-02 15:04"))
				} else {
					details = "local file"
				}
			case update.SourceTypeFTPFile, update.SourceTypeFTPFolder:
				if !changeInfo.NewDate.IsZero() {
					details = fmt.Sprintf("date: %s", changeInfo.NewDate.Format("2006-01-02"))
				}
			case update.SourceTypeHTTPFile:
				if changeInfo.NewETag != "" {
					details = fmt.Sprintf("etag: %s", changeInfo.NewETag[:min(20, len(changeInfo.NewETag))])
				} else if !changeInfo.NewDate.IsZero() {
					details = fmt.Sprintf("date: %s", changeInfo.NewDate.Format("2006-01-02"))
				}
			case update.SourceTypeHTTPFolder:
				if !changeInfo.NewDate.IsZero() {
					details = fmt.Sprintf("date: %s", changeInfo.NewDate.Format("2006-01-02"))
				}
			case update.SourceTypeVersionedAPI:
				details = fmt.Sprintf("version: %s", changeInfo.NewVersion)
			case update.SourceTypeUnknown:
				isUnknown = true
				if changeInfo.Error != "" {
					details = changeInfo.Error
				} else {
					details = "no source config"
				}
			default:
				details = string(changeInfo.SourceType)
			}
		}

		// Count appropriately
		status := "NO"
		if changeInfo.HasChanged {
			status = "YES"
			if isUnknown {
				unknownCount++
			} else {
				changedCount++
			}
		} else {
			unchangedCount++
		}

		fmt.Printf("%-25s %-15s %-10s %s\n", dsName, displayType, status, details)
	}

	fmt.Println()
	if notBuiltCount > 0 {
		fmt.Printf("Summary: %d changed, %d unchanged, %d not built, %d unknown/error\n", changedCount, unchangedCount, notBuiltCount, unknownCount)
	} else {
		fmt.Printf("Summary: %d changed, %d unchanged, %d unknown/error\n", changedCount, unchangedCount, unknownCount)
	}

	if changedCount > 0 {
		fmt.Println("\nTo rebuild changed datasets, run: ./biobtree build -d <datasets>")
	}
	if unknownCount > 0 {
		fmt.Println("Note: Unknown datasets will be skipped if already built, or built if new.")
	}

	return nil
}

func runProfileCommand(c *cli.Context) error {

	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	config = &configs.Conf{}
	config.Init(confdir, versionTag, outDir, true)

	os.Remove("memprof.out")
	os.Remove("cpuprof.out")

	start := time.Now()

	d := strings.Split(c.GlobalString("datasets"), ",")
	config.Appconf["uniprot_ftp"] = c.GlobalString("uniprot-ftp")

	if len(d) == 0 {
		log.Println("Error:datasets must be specified")
		return nil
	}

	f, err := os.Create("cpuprof.out")
	if err != nil {
		log.Fatal("could not create CPU profile: ", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatal("could not start CPU profile: ", err)
	}
	defer pprof.StopCPUProfile()

	chunkIdxx := c.GlobalString("idx")
	if len(chunkIdxx) == 0 {
		chunkIdxx = strconv.Itoa(time.Now().Nanosecond())
	}
	//fmt.Println("chunkindex", chunkIdx)

	///updateData(d, []string{})

	elapsed := time.Since(start)
	log.Printf("finished took %s", elapsed)

	//mergeData()

	f2, err := os.Create("memprof.out")
	if err != nil {
		log.Fatal("could not create memory profile: ", err)
	}
	runtime.GC() // get up-to-date statistics
	if err := pprof.WriteHeapProfile(f2); err != nil {
		log.Fatal("could not write memory profile: ", err)
	}
	f2.Close()

	return nil

}

func check(err error) {

	if err != nil {
		fmt.Println("Error: ", err)
		panic(err)
	}

}
