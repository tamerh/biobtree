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

	// "runtime"
	// "runtime/pprof"
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
			Name:  "keep",
			Usage: "Keep existing data from update command",
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
			Usage:     "Query biobtree database from CLI (always detailed, pretty-printed)",
			ArgsUsage: "<query_string>",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "dataset,s",
					Usage: "Filter results by dataset name (e.g., uniprot, string)",
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

	log.Println("Alias running...")
	start := time.Now()
	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	includeOptionals := c.GlobalBool("include-optionals")
	config = &configs.Conf{}
	config.Init(confdir, versionTag, outDir, includeOptionals)

	var ali = update.Alias{}
	ali.Merge(config)

	elapsed := time.Since(start)
	log.Printf("Alias took %s", elapsed)

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

	if len(d) == 0 && len(sp) == 0 && len(spatterns) == 0 && len(genometaxids) == 0 {

		log.Fatal("Datasets or genome must be selected.")
		return nil

	}

	keep := c.GlobalBool("keep")
	if !keep {
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

	update.NewDataUpdate(d, ts, sp, spatterns, genometaxids, c.GlobalBool("skip-ensembl"), orthologIDs, eo, c.GlobalBool("ensembl-orthologs-all"), config, chunkIdxx).Update()

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

	cpu := c.GlobalInt(" maxcpu")
	if cpu > 1 {
		runtime.GOMAXPROCS(cpu)
	}

	keep := c.GlobalBool("keep")

	var ali = update.Alias{}
	ali.Merge(config)

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
	web.Start(config, c.GlobalBool("no-web-popup"))

	return nil

}

func runQueryCommand(c *cli.Context) error {
	// 1. Check arguments
	if c.NArg() == 0 {
		return fmt.Errorf("query string required\nUsage: biobtree --out-dir <dir> query \"<query_string>\"\nExamples:\n  biobtree query P27348\n  biobtree query \"CHEMBL203 >> surechembl >> patent\"")
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
	return cliHandler.Query(config, queryStr, datasetFilter)
}

func runInstallCommand(c *cli.Context) error {

	confdir := c.GlobalString("confdir")
	outDir := c.GlobalString("out-dir")
	preBuildSet := c.GlobalString("pre-built")
	config = &configs.Conf{}
	config.Install(confdir, versionTag, outDir, preBuildSet, true)

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
