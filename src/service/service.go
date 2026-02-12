package service

import (
	"biobtree/configs"
	"biobtree/query"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/ristretto"

	"biobtree/db"
	"biobtree/pbuf"
	"biobtree/util"

	"github.com/golang/protobuf/proto"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	typescelgo "github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter/functions"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

const pagingSep2 = ","
const pagingSep3 = "[]"
const pagingSep4 = "]["

// federationDB holds the database connection for a single federation
type federationDB struct {
	env  db.Env
	dbi  db.DBI
	meta map[string]interface{}
}

// Service provides database lookup and query functionality.
// Can be used by both web/CLI (with outDir) and update package (with lookupDbDir).
type Service struct {
	// Federation support: maps federation name to its database
	federations              map[string]*federationDB
	datasetFederation        map[uint32]string // cached: datasetID -> federation name
	// Legacy fields for backward compatibility (point to main federation)
	readEnv                  db.Env
	readDbi                  db.DBI
	aliasStore               *AliasStore
	pager                    *util.Pagekey
	pageSize                 int
	resultPageSize           int
	maxMappingResult         int
	resultPageSizeLite       int // Higher limit for lite mode
	maxMappingResultLite     int // Higher limit for lite mode
	mapFilterTimeoutDuration float64
	qparser                  *query.QueryParser
	celgoEnv                 cel.Env
	filterResultCache        *ristretto.Cache
	cacheDisabled            bool // When true, skips cache for performance testing
	celProgOpts              cel.ProgramOption
}

// NewService creates a new Service instance with a configurable database directory.
// dbDir: Base directory containing federation databases (e.g., outDir or lookupDbDir)
// conf: Global configuration
func NewService(dbDir string, conf *configs.Conf) (*Service, error) {
	config = conf // Set package-level config
	s := &Service{}
	if err := s.initWithDbDir(dbDir); err != nil {
		return nil, err
	}
	return s, nil
}

// IsAvailable returns true if the service has a valid database connection
func (s *Service) IsAvailable() bool {
	return s.federations != nil && len(s.federations) > 0
}

// init initializes the service using outDir from config (backward compatibility)
func (s *Service) init() {
	outDir := config.Appconf["outDir"]
	s.initWithDbDir(outDir)
}

// initWithDbDir initializes the service with a configurable database directory
func (s *Service) initWithDbDir(dbDir string) error {
	// Initialize federation support
	s.federations = make(map[string]*federationDB)
	s.datasetFederation = config.DatasetFederation

	// Load all federations from the specified directory
	s.loadFederations(dbDir)

	// Set legacy fields to main federation for backward compatibility
	if mainFed, ok := s.federations["main"]; ok {
		s.readEnv = mainFed.env
		s.readDbi = mainFed.dbi
	} else {
		return fmt.Errorf("main federation not found in %s - please make sure database was generated successfully", dbDir)
	}

	s.pager = &util.Pagekey{}
	s.pager.Init()

	var err error

	s.pageSize = 200
	if _, ok := config.Appconf["pageSize"]; ok {
		s.pageSize, err = strconv.Atoi(config.Appconf["pageSize"])
		if err != nil {
			panic("Invalid pagesize definition")
		}
	}

	s.resultPageSize = 10
	if _, ok := config.Appconf["maxSearchResult"]; ok {
		s.resultPageSize, err = strconv.Atoi(config.Appconf["maxSearchResult"])
		if err != nil {
			panic("Invalid maxSearchResult definition")
		}
	}

	s.maxMappingResult = 30
	if _, ok := config.Appconf["maxMappingResult"]; ok {
		s.maxMappingResult, err = strconv.Atoi(config.Appconf["maxMappingResult"])
		if err != nil {
			panic("Invalid maxMappingResult definition")
		}
	}

	// Lite mode defaults: 5x higher than full mode (smaller payload)
	s.resultPageSizeLite = 50
	if _, ok := config.Appconf["maxSearchResultLite"]; ok {
		s.resultPageSizeLite, err = strconv.Atoi(config.Appconf["maxSearchResultLite"])
		if err != nil {
			panic("Invalid maxSearchResultLite definition")
		}
	}

	s.maxMappingResultLite = 150
	if _, ok := config.Appconf["maxMappingResultLite"]; ok {
		s.maxMappingResultLite, err = strconv.Atoi(config.Appconf["maxMappingResultLite"])
		if err != nil {
			panic("Invalid maxMappingResultLite definition")
		}
	}

	s.mapFilterTimeoutDuration = 300
	if _, ok := config.Appconf["mapFilterTimeoutDuration"]; ok {
		timeoutInt, err := strconv.Atoi(config.Appconf["mapFilterTimeoutDuration"])
		if err != nil {
			panic("Invalid mapFilterTimeoutDuration definition")
		}
		s.mapFilterTimeoutDuration = float64(timeoutInt)
	}

	// init aliases from JSON config
	s.aliasStore, err = NewAliasStore(config.Appconf["confDir"])
	if err != nil {
		log.Printf("Warning: Could not load aliases: %v", err)
		// Create empty store - aliases will be disabled but service continues
		s.aliasStore = &AliasStore{
			aliases: make(map[string]*AliasEntry),
			cache:   make(map[string][]string),
		}
	}

	s.qparser = &query.QueryParser{}
	s.qparser.Init(config)

	// init cel-go
	s.celgoEnv, err = cel.NewEnv(
		cel.Types(&pbuf.UniprotAttr{}),
		cel.Types(&pbuf.UniprotFeatureAttr{}),
		cel.Types(&pbuf.EnsemblAttr{}),
		cel.Types(&pbuf.TaxoAttr{}),
		cel.Types(&pbuf.HgncAttr{}),
		cel.Types(&pbuf.OntologyAttr{}),
		cel.Types(&pbuf.HPOAttr{}),
		cel.Types(&pbuf.HPOAttr_DiseaseAssoc{}),
		cel.Types(&pbuf.InterproAttr{}),
		cel.Types(&pbuf.EnaAttr{}),
		cel.Types(&pbuf.HmdbAttr{}),
		cel.Types(&pbuf.ChebiAttr{}),
		cel.Types(&pbuf.PdbAttr{}),
		cel.Types(&pbuf.DrugbankAttr{}),
		cel.Types(&pbuf.OrphanetAttr{}),
		cel.Types(&pbuf.OrphanetAttr_PhenotypeAssociation{}),
		cel.Types(&pbuf.ReactomePathwayAttr{}),
		cel.Types(&pbuf.BgeeAttr{}),
		cel.Types(&pbuf.GwasAttr{}),
		cel.Types(&pbuf.AntibodyAttr{}),
		cel.Types(&pbuf.Esm2SimilarityAttr{}),
		cel.Types(&pbuf.Esm2Similarity{}),
		cel.Types(&pbuf.PubchemAttr{}),
		cel.Types(&pbuf.PubchemActivityAttr{}),
		cel.Types(&pbuf.PubchemAssayAttr{}),
		cel.Types(&pbuf.EntrezAttr{}),
		cel.Types(&pbuf.RefSeqAttr{}),
		cel.Types(&pbuf.GenccAttr{}),
		cel.Types(&pbuf.BindingdbAttr{}),
		cel.Types(&pbuf.CtdAttr{}),
		cel.Types(&pbuf.CtdGeneInteraction{}),
		cel.Types(&pbuf.CtdDiseaseAssociation{}),
		cel.Types(&pbuf.DrugcentralAttr{}),
		cel.Types(&pbuf.DrugcentralTarget{}),
		cel.Types(&pbuf.MsigdbAttr{}),
		cel.Types(&pbuf.AlphaMissenseAttr{}),
		cel.Types(&pbuf.AlphaMissenseTranscriptAttr{}),
		cel.Types(&pbuf.PharmgkbAttr{}),
		cel.Types(&pbuf.PharmgkbRelatedGene{}),
		cel.Types(&pbuf.PharmgkbGeneAttr{}),
		cel.Types(&pbuf.PharmgkbClinicalAttr{}),
		cel.Types(&pbuf.PharmgkbDrugLabel{}),
		cel.Types(&pbuf.PharmgkbVariantAttr{}),
		cel.Types(&pbuf.PharmgkbGuidelineAttr{}),
		cel.Types(&pbuf.PharmgkbPathwayAttr{}),
		cel.Types(&pbuf.CellxgeneAttr{}),
		cel.Types(&pbuf.CellxgeneCelltypeAttr{}),
		cel.Types(&pbuf.CellxgeneTissueExpression{}),
		cel.Types(&pbuf.ScxaAttr{}),
		cel.Types(&pbuf.ScxaExpressionAttr{}),
		cel.Types(&pbuf.ScxaGeneExperimentAttr{}),
		cel.Types(&pbuf.ScxaClusterExpression{}),
		cel.Types(&pbuf.ClinvarAttr{}),
		cel.Types(&pbuf.BiogridInteractionAttr{}),
		cel.Types(&pbuf.AlphaFoldAttr{}),
		cel.Types(&pbuf.ClinicalTrialAttr{}),
		cel.Types(&pbuf.CollecTriAttr{}),
		cel.Types(&pbuf.SignorAttr{}),
		cel.Types(&pbuf.CorumAttr{}),
		cel.Types(&pbuf.CorumSubunit{}),
		cel.Types(&pbuf.BrendaAttr{}),
		cel.Types(&pbuf.BrendaKineticsAttr{}),
		cel.Types(&pbuf.KineticMeasurement{}),
		cel.Types(&pbuf.BrendaInhibitorAttr{}),
		cel.Types(&pbuf.InhibitionMeasurement{}),
		cel.Types(&pbuf.CellphonedbAttr{}),
		cel.Types(&pbuf.SpliceAIAttr{}),
		cel.Declarations(
			decls.NewIdent("uniprot", decls.NewObjectType("pbuf.UniprotAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ufeature", decls.NewObjectType("pbuf.UniprotFeatureAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ensembl", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("transcript", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("exon", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("cds", decls.NewObjectType("pbuf.EnsemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("taxonomy", decls.NewObjectType("pbuf.TaxoAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("hgnc", decls.NewObjectType("pbuf.HgncAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("go", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("efo", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("eco", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("mondo", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("uberon", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("oba", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("hpo", decls.NewObjectType("pbuf.HPOAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("cl", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pato", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("obi", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("xco", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("bao", decls.NewObjectType("pbuf.OntologyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("interpro", decls.NewObjectType("pbuf.InterproAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ena", decls.NewObjectType("pbuf.EnaAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("hmdb", decls.NewObjectType("pbuf.HmdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("chebi", decls.NewObjectType("pbuf.ChebiAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pdb", decls.NewObjectType("pbuf.PdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("drugbank", decls.NewObjectType("pbuf.DrugbankAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("orphanet", decls.NewObjectType("pbuf.OrphanetAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("reactome", decls.NewObjectType("pbuf.ReactomePathwayAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("chembl", decls.NewObjectType("pbuf.ChemblAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("lipidmaps", decls.NewObjectType("pbuf.LipidmapsAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("swisslipids", decls.NewObjectType("pbuf.SwisslipidsAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("bgee", decls.NewObjectType("pbuf.BgeeAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("rhea", decls.NewObjectType("pbuf.RheaAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("gwas_study", decls.NewObjectType("pbuf.GwasStudyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("gwas", decls.NewObjectType("pbuf.GwasAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("dbsnp", decls.NewObjectType("pbuf.DbsnpAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("clinvar", decls.NewObjectType("pbuf.ClinvarAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("antibody", decls.NewObjectType("pbuf.AntibodyAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("esm2_similarity", decls.NewObjectType("pbuf.Esm2SimilarityAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pubchem", decls.NewObjectType("pbuf.PubchemAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pubchem_activity", decls.NewObjectType("pbuf.PubchemActivityAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pubchem_assay", decls.NewObjectType("pbuf.PubchemAssayAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("entrez", decls.NewObjectType("pbuf.EntrezAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("refseq", decls.NewObjectType("pbuf.RefSeqAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("gencc", decls.NewObjectType("pbuf.GenccAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("bindingdb", decls.NewObjectType("pbuf.BindingdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("ctd", decls.NewObjectType("pbuf.CtdAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("biogrid", decls.NewObjectType("pbuf.BiogridAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("biogrid_interaction", decls.NewObjectType("pbuf.BiogridInteractionAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("drugcentral", decls.NewObjectType("pbuf.DrugcentralAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("msigdb", decls.NewObjectType("pbuf.MsigdbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("alphamissense", decls.NewObjectType("pbuf.AlphaMissenseAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("alphamissense_transcript", decls.NewObjectType("pbuf.AlphaMissenseTranscriptAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pharmgkb", decls.NewObjectType("pbuf.PharmgkbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pharmgkb_gene", decls.NewObjectType("pbuf.PharmgkbGeneAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pharmgkb_clinical", decls.NewObjectType("pbuf.PharmgkbClinicalAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pharmgkb_variant", decls.NewObjectType("pbuf.PharmgkbVariantAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pharmgkb_guideline", decls.NewObjectType("pbuf.PharmgkbGuidelineAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("pharmgkb_pathway", decls.NewObjectType("pbuf.PharmgkbPathwayAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("cellxgene", decls.NewObjectType("pbuf.CellxgeneAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("cellxgene_celltype", decls.NewObjectType("pbuf.CellxgeneCelltypeAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("scxa", decls.NewObjectType("pbuf.ScxaAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("scxa_expression", decls.NewObjectType("pbuf.ScxaExpressionAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("scxa_gene_experiment", decls.NewObjectType("pbuf.ScxaGeneExperimentAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("alphafold", decls.NewObjectType("pbuf.AlphaFoldAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("clinical_trials", decls.NewObjectType("pbuf.ClinicalTrialAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("collectri", decls.NewObjectType("pbuf.CollecTriAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("signor", decls.NewObjectType("pbuf.SignorAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("corum", decls.NewObjectType("pbuf.CorumAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("brenda", decls.NewObjectType("pbuf.BrendaAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("brenda_kinetics", decls.NewObjectType("pbuf.BrendaKineticsAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("brenda_inhibitor", decls.NewObjectType("pbuf.BrendaInhibitorAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("cellphonedb", decls.NewObjectType("pbuf.CellphonedbAttr"), nil)),
		cel.Declarations(
			decls.NewIdent("spliceai", decls.NewObjectType("pbuf.SpliceAIAttr"), nil)),
		cel.Declarations(
			decls.NewFunction("overlaps",
				decls.NewOverload("overlaps_int_int",
					[]*exprpb.Type{decls.Int, decls.Int},
					decls.Bool),
				decls.NewInstanceOverload("overlaps_int_int",
					[]*exprpb.Type{decls.NewObjectType("pbuf.EnsemblAttr"), decls.Int, decls.Int},
					decls.Bool)),
			decls.NewFunction("within",
				decls.NewOverload("within_int_int",
					[]*exprpb.Type{decls.Int, decls.Int},
					decls.Bool),
				decls.NewInstanceOverload("within_int_int",
					[]*exprpb.Type{decls.NewObjectType("pbuf.EnsemblAttr"), decls.Int, decls.Int},
					decls.Bool)),
			decls.NewFunction("covers",
				decls.NewInstanceOverload("covers_int",
					[]*exprpb.Type{decls.NewObjectType("pbuf.EnsemblAttr"), decls.Int},
					decls.Bool)),
		),
	)

	if err != nil { // handle properly
		panic(err)
	}

	s.celProgOpts = cel.Functions(
		&functions.Overload{
			Operator: "overlaps",
			Function: func(args ...ref.Val) ref.Val {

				arg1 := int32(args[1].Value().(int64))
				arg2 := int32(args[2].Value().(int64))

				if arg1 > arg2 {
					return typescelgo.Bool(false)
				}

				a := args[0].Value().(*pbuf.EnsemblAttr)

				return typescelgo.Bool(a.Start <= arg1 && arg1 <= a.End) || (a.Start <= arg2 && arg2 <= a.End)

			}},
		&functions.Overload{
			Operator: "within",
			Function: func(args ...ref.Val) ref.Val {

				arg1 := int32(args[1].Value().(int64))
				arg2 := int32(args[2].Value().(int64))

				if arg1 > arg2 {
					return typescelgo.Bool(false)
				}

				ensembl := args[0].Value().(*pbuf.EnsemblAttr)

				return typescelgo.Bool(ensembl.Start >= arg1 && ensembl.End <= arg2)

			}},
		&functions.Overload{
			Operator: "covers",
			Binary: func(lhs ref.Val, rhs ref.Val) ref.Val {

				arg1 := int32(rhs.Value().(int64))

				ensembl := lhs.Value().(*pbuf.EnsemblAttr)

				return typescelgo.Bool(ensembl.Start <= arg1 && arg1 <= ensembl.End)

			}},
	)

	// Check if cache is disabled for performance testing
	s.cacheDisabled = false
	if val, ok := config.Appconf["disableCache"]; ok {
		if val == "y" || val == "yes" || val == "true" || val == "1" {
			s.cacheDisabled = true
			log.Println("Cache disabled via disableCache parameter")
		}
	}

	// Only initialize cache if not disabled
	if !s.cacheDisabled {
		cacheHardMaxSize := 1024
		if _, ok := config.Appconf["cacheHardMaxSize"]; ok {
			cacheHardMaxSize, err = strconv.Atoi(config.Appconf["cacheHardMaxSize"])
			if err != nil {
				panic("Invalid cacheHardMaxSize definition")
			}
		}

		s.filterResultCache, err = ristretto.NewCache(&ristretto.Config{
			NumCounters: 1e7,                                   // number of keys to track frequency of (10M).
			MaxCost:     int64(cacheHardMaxSize) * 1024 * 1024, // maximum cost of cache.
			BufferItems: 64,                                    // number of keys per Get buffer.
		})

		if err != nil {
			panic(err)
		}
	}

	return nil
}

// loadFederations loads all federations that have database files
func (s *Service) loadFederations(outDir string) {
	// Always try to load main federation first
	mainDir := filepath.Join(outDir, "main")
	if err := s.loadFederation("main", mainDir); err != nil {
		// Fallback: try to load from legacy location (direct outDir)
		log.Printf("Main federation not found at %s, trying legacy location", mainDir)
		if err := s.loadFederationLegacy("main", outDir); err != nil {
			log.Fatalf("Could not load main federation: %v", err)
		}
	}

	// Load other federations if they exist
	for _, fed := range config.GetFederations() {
		if fed == "main" {
			continue
		}
		fedDir := filepath.Join(outDir, fed)
		if err := s.loadFederation(fed, fedDir); err != nil {
			log.Printf("Federation '%s' not loaded (may not be generated yet): %v", fed, err)
		}
	}

	log.Printf("Loaded %d federation(s): %v", len(s.federations), s.getFederationNames())
}

// loadFederation loads a single federation from its directory
func (s *Service) loadFederation(name, dir string) error {
	// Check if db.meta.json exists
	metaPath := filepath.Join(dir, "db", "db.meta.json")
	metaData, err := ioutil.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("meta file not found: %w", err)
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("failed to parse meta: %w", err)
	}

	totalKV := int64(meta["totalKVLine"].(float64))

	// Create appconf copy with federation-specific dbDir
	fedAppconf := make(map[string]string)
	for k, v := range config.Appconf {
		fedAppconf[k] = v
	}
	fedAppconf["dbDir"] = filepath.Join(dir, "db")

	db1 := db.DB{}
	env, dbi := db1.OpenDBNew(false, totalKV, fedAppconf)

	s.federations[name] = &federationDB{
		env:  env,
		dbi:  dbi,
		meta: meta,
	}

	log.Printf("Loaded federation '%s' from %s (totalKV: %d)", name, dir, totalKV)
	return nil
}

// loadFederationLegacy loads from the legacy flat directory structure (pre-federation)
func (s *Service) loadFederationLegacy(name, outDir string) error {
	metaPath := filepath.Join(outDir, "db", "db.meta.json")
	metaData, err := ioutil.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("legacy meta file not found: %w", err)
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("failed to parse legacy meta: %w", err)
	}

	totalKV := int64(meta["totalKVLine"].(float64))

	db1 := db.DB{}
	env, dbi := db1.OpenDBNew(false, totalKV, config.Appconf)

	s.federations[name] = &federationDB{
		env:  env,
		dbi:  dbi,
		meta: meta,
	}

	log.Printf("Loaded federation '%s' from legacy location %s (totalKV: %d)", name, outDir, totalKV)
	return nil
}

// getFederationNames returns the names of all loaded federations
func (s *Service) getFederationNames() []string {
	names := make([]string, 0, len(s.federations))
	for name := range s.federations {
		names = append(names, name)
	}
	return names
}

// getDBForDataset returns the database connection for a specific dataset ID
func (s *Service) getDBForDataset(datasetID uint32) (db.Env, db.DBI) {
	if s.datasetFederation != nil {
		if fed, ok := s.datasetFederation[datasetID]; ok {
			if fedDB, exists := s.federations[fed]; exists {
				return fedDB.env, fedDB.dbi
			}
		}
	}
	// Default to main federation
	return s.federations["main"].env, s.federations["main"].dbi
}

// getDBForIdentifier returns the database connection based on identifier pattern
// This is used for search queries where we need to route by identifier
//
// LIMITATION: This pattern-based routing only works for federations with distinct
// ID prefixes (e.g., dbSNP IDs start with "RS"). It does NOT handle:
// 1. Keyword/text search - keywords like "BRCA1" won't find entries in non-main federations
// 2. Federations without clear ID patterns - datasets without unique prefixes can't be routed
//
// SOLUTION: For future federations or keyword search support, use searchAllFederations()
// which tries all federations. This is slower but handles all cases correctly.
// To enable: change getLmdbResult() to call searchAllFederations() instead of
// using getDBForIdentifier() directly.
func (s *Service) getDBForIdentifier(identifier string) (db.Env, db.DBI) {
	// Check if identifier looks like an rsID (dbSNP variant)
	// Add new federation ID patterns here as needed (e.g., "CHEMBL" prefix for chembl federation)
	if isRsID(identifier) {
		if fedDB, exists := s.federations["dbsnp"]; exists {
			return fedDB.env, fedDB.dbi
		}
	}
	// Default to main federation
	return s.federations["main"].env, s.federations["main"].dbi
}

// isRsID checks if the identifier looks like a dbSNP rsID
func isRsID(id string) bool {
	if len(id) < 3 {
		return false
	}
	upper := strings.ToUpper(id)
	return strings.HasPrefix(upper, "RS")
}

// searchAllFederations searches for an identifier across all federations
// Returns the first match found
//
// Use this function when:
// 1. Keyword/text search is needed (keywords don't have federation-specific patterns)
// 2. New federation added without distinct ID prefix pattern
// 3. Fallback search when pattern-based routing fails
//
// Performance note: This is slower than getDBForIdentifier() as it may query
// multiple LMDBs. For high-volume queries with known ID patterns, use
// getDBForIdentifier() directly.
func (s *Service) searchAllFederations(identifier string) (*pbuf.Result, error) {
	// First try the likely federation based on identifier pattern
	env, dbi := s.getDBForIdentifier(identifier)
	result, err := s.getLmdbResultFrom(identifier, env, dbi)
	if err == nil && result != nil && len(result.Results) > 0 {
		return result, nil
	}

	// If not found, try all other federations
	for name, fedDB := range s.federations {
		if fedDB.env == env {
			continue // Skip the one we already tried
		}
		result, err := s.getLmdbResultFrom(identifier, fedDB.env, fedDB.dbi)
		if err == nil && result != nil && len(result.Results) > 0 {
			log.Printf("Found %s in federation '%s' (unexpected)", identifier, name)
			return result, nil
		}
	}

	return &pbuf.Result{}, nil
}

// getLmdbResultFrom retrieves a result from a specific database
func (s *Service) getLmdbResultFrom(identifier string, env db.Env, dbi db.DBI) (*pbuf.Result, error) {
	var v []byte
	err := env.View(func(txn db.Txn) (err error) {
		v, err = txn.Get(dbi, []byte(identifier))
		if db.IsNotFound(err) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	r := pbuf.Result{}
	if len(v) > 0 {
		err = proto.Unmarshal(v, &r)
		return &r, err
	}

	return &r, nil
}

func (s *Service) aliasIDs(alias string) ([]string, error) {
	return s.aliasStore.GetIDs(alias)
}

func (s *Service) meta() *pbuf.MetaResponse {

	meta := pbuf.MetaResponse{}
	results := map[string]*pbuf.MetaKeyValue{}

	for k := range config.Dataconf {
		if config.Dataconf[k]["_alias"] == "" { // not send the alias
			id := config.Dataconf[k]["id"]
			if _, ok := results[id]; !ok {
				datasetConf := map[string]string{}
				keyvals := &pbuf.MetaKeyValue{}

				if len(config.Dataconf[k]["name"]) > 0 {
					datasetConf["name"] = config.Dataconf[k]["name"]
				} else {
					datasetConf["name"] = k
				}

				if len(config.Dataconf[k]["linkdataset"]) > 0 {
					datasetConf["linkdataset"] = config.Dataconf[k]["linkdataset"]
				}

				if _, ok := config.Dataconf[k]["attrs"]; ok {
					datasetConf["attrs"] = config.Dataconf[k]["attrs"]
				}

				datasetConf["id"] = k

				keyvals.Keyvalues = datasetConf
				results[id] = keyvals
			}
		}
	}

	meta.Results = results
	return &meta

}

func (s *Service) metajson() string {

	var b strings.Builder
	b.WriteString(`{ "datasets":{`)
	keymap := map[string]bool{}
	for k := range config.Dataconf {
		if config.Dataconf[k]["_alias"] == "" { // not send the alias
			id := config.Dataconf[k]["id"]
			if _, ok := keymap[id]; !ok {
				b.WriteString(`"` + id + `":{`)

				if len(config.Dataconf[k]["name"]) > 0 {
					b.WriteString(`"name":"` + config.Dataconf[k]["name"] + `",`)
				} else {
					b.WriteString(`"name":"` + k + `",`)
				}

				if len(config.Dataconf[k]["linkdataset"]) > 0 {
					b.WriteString(`"linkdataset":"` + config.Dataconf[k]["linkdataset"] + `",`)
				}

				if _, ok := config.Dataconf[k]["attrs"]; ok {
					b.WriteString(`"attrs":"` + config.Dataconf[k]["attrs"] + `",`)
				}

				b.WriteString(`"id":"` + k + `"`)

				b.WriteString(`},`)

				keymap[id] = true
			}
		}
	}
	s2 := b.String()
	s2 = s2[:len(s2)-1]
	s2 = s2 + "}"

	// additional params
	// mark builtin db if exist
	s2 = s2 + `, "appparams":{`

	// Check main federation's index dir for special meta files
	mainIndexDir := filepath.Join(config.Appconf["outDir"], "main", "index")
	files, err := ioutil.ReadDir(mainIndexDir)
	if err != nil {
		log.Fatal(err)
	}

loop:
	for _, f := range files {
		if !f.IsDir() {
			switch f.Name() {
			case "builtinset1.meta.json":
				s2 = s2 + `"builtinset":"1"`
				break loop
			case "builtinset2.meta.json":
				s2 = s2 + `"builtinset":"2"`
				break loop
			case "builtinset3.meta.json":
				s2 = s2 + `"builtinset":"3"`
				break loop
			case "builtinset4.meta.json":
				s2 = s2 + `"builtinset":"4"`
				break loop
			case "def.meta.json": // this used when indexing all
				s2 = s2 + `"builtinset":"0"`
				break loop
			}
		}
	}

	s2 = s2 + `}}`
	return s2

}

type searchPageInfo struct {
	idIndex              int
	resultIndex          int
	resultIndexProcessed bool
	linkPageIndex        int
	linkIndex            int
	linkActive           bool
	linkIndexProcessed   bool
}

func (s *Service) searchPageInfo(page string) (*searchPageInfo, error) {

	pageVals := strings.Split(page, pagingSep2)

	switch len(pageVals) {

	case 2:
		pagingInfo := &searchPageInfo{}
		idIndex, err := strconv.Atoi(pageVals[0])
		if err != nil {
			return nil, err
		}
		pagingInfo.idIndex = idIndex
		resultIndex, err := strconv.Atoi(pageVals[1])
		if err != nil {
			return nil, err
		}
		pagingInfo.resultIndex = resultIndex
		return pagingInfo, nil
	case 4:

		pagingInfo := &searchPageInfo{}
		idIndex, err := strconv.Atoi(pageVals[0])
		if err != nil {
			return nil, err
		}
		pagingInfo.idIndex = idIndex
		resultIndex, err := strconv.Atoi(pageVals[1])
		if err != nil {
			return nil, err
		}
		pagingInfo.resultIndex = resultIndex

		linkPageIndex, err := strconv.Atoi(pageVals[2])
		if err != nil {
			return nil, err
		}
		pagingInfo.linkPageIndex = linkPageIndex

		linkIndex, err := strconv.Atoi(pageVals[3])
		if err != nil {
			return nil, err
		}
		pagingInfo.linkIndex = linkIndex
		pagingInfo.linkActive = true
		return pagingInfo, nil
	default:
		err := fmt.Errorf("Invalid paging request", page)
		return nil, err

	}

}


func (s *Service) makeLite(xref *pbuf.Xref) {
	xref.Entries = nil
	xref.DatasetPages = nil
	xref.Pages = nil
	xref.DatasetCounts = nil
}

func (s *Service) makeLiteAll(res *pbuf.Result) {

	for _, xref := range res.Results {
		s.makeLite(xref)
	}

}


// Search performs a search across datasets with optional filtering
func (s *Service) Search(ids []string, idsDomain uint32, page string, q *query.Query, detail, buildURL bool) (*pbuf.Result, error) {

	//todo remove duplicate parts
	result := &pbuf.Result{}

	if !detail {
		defer s.makeLiteAll(result)
	}


	var xrefs []*pbuf.Xref
	totalResult := 0
	var err error

	for i := range ids {
		ids[i] = strings.ToUpper(ids[i])
	}

	var pagingInfo *searchPageInfo

	if page != "" {
		pagingInfo, err = s.searchPageInfo(page)
		if err != nil {
			return nil, err
		}
	}

	for idIndex, id := range ids {

		if pagingInfo != nil && pagingInfo.idIndex > idIndex {
			continue
		}

		result, err := s.Lookup(id)
		if err != nil {
			return nil, err
		}

		if pagingInfo != nil && !pagingInfo.resultIndexProcessed { // starts from last process id results
			result.Results = result.Results[pagingInfo.resultIndex:] //todo slice bound out of range.
			pagingInfo.resultIndexProcessed = true
		}

		resultIndex := 0
		if len(result.Results) > 0 {

			for _, xref := range result.Results {

				if xref.IsLink {

					skipRootLinks := false
					if pagingInfo != nil && pagingInfo.linkActive && !pagingInfo.linkIndexProcessed { // first paging check for root result
						if pagingInfo.linkPageIndex == -1 {
							xref.Entries = xref.Entries[pagingInfo.linkIndex:]
							pagingInfo.linkIndexProcessed = true
						} else {
							skipRootLinks = true
						}
					}

					if !skipRootLinks {
						linkIndex := 0
						for _, b := range xref.Entries { //link entries

							// Filter by dataset BEFORE fetching to avoid errors from missing entries in other datasets
							if idsDomain > 0 && b.Dataset != idsDomain {
								continue
							}

							xref2, err := s.LookupByDataset(b.Identifier, b.Dataset)

							if err != nil {
								return nil, err
							}

							if totalResult == s.resultPageSize {

								if pagingInfo != nil {

									if idIndex == pagingInfo.idIndex {
										resultIndex = pagingInfo.resultIndex + resultIndex
										linkIndex += pagingInfo.linkIndex
									}

								}

								result.Nextpage = strconv.Itoa(idIndex) + pagingSep2 + strconv.Itoa(resultIndex) + pagingSep2 + "-1" + pagingSep2 + strconv.Itoa(linkIndex)
								result.Results = xrefs
								return result, nil
							}

							xref2.Keyword = id
							xref2.Identifier = b.Identifier
							if q != nil { // filter xref. It is repetitive can be moved to 1 place
								q.MapDataset = config.DataconfIDIntToString[xref2.Dataset]
								q.MapDatasetID = xref2.Dataset
								if len(q.Filter) > 0 {
									b, err := s.execCelGo(q, xref2)
									if err != nil {
										return nil, err
									}
									if b {
										if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
											xrefs = append(xrefs, xref2)
											totalResult++
										}
									}
								}
							} else {
								if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
									xrefs = append(xrefs, xref2)
									totalResult++
								}
							}
							linkIndex++
						}
					}
					if len(xref.DatasetPages) > 0 { // link pages - use DatasetPages for correct page key prefix

						// Iterate over DatasetPages to use correct target dataset prefix
						for targetDatasetID, pageInfo := range xref.DatasetPages {
							// If filtering by dataset, only process pages for that dataset
							if idsDomain > 0 && targetDatasetID != idsDomain {
								continue
							}

							for pageIndex, page := range pageInfo.Pages {
								// Build page key with SOURCE dataset prefix (0 for text links)
								// Pages are stored under text link key prefix, not target dataset prefix
								pageKey := id + spacestr + config.DataconfIDToPageKey[0] + spacestr + page
								// Try target dataset first, then main (0) - pages may be stored in either
								xrefPage, err := s.LookupPage(pageKey, targetDatasetID)
								if err != nil {
									return nil, err
								}
								if xrefPage == nil {
									// Try main federation (pages might be stored there during text link merge)
									xrefPage, err = s.LookupPage(pageKey, 0)
									if err != nil {
										return nil, err
									}
								}
								if xrefPage == nil {
									continue
								}

							if pagingInfo != nil && pagingInfo.linkActive && !pagingInfo.linkIndexProcessed {
								xrefPage.Entries = xrefPage.Entries[pagingInfo.linkIndex:]
								pagingInfo.linkIndexProcessed = true
							}

							linkIndex := 0
							for _, b := range xrefPage.Entries {

								// Filter by dataset BEFORE fetching to avoid errors from missing entries in other datasets
								if idsDomain > 0 && b.Dataset != idsDomain {
									continue
								}

								xref2, err := s.LookupByDataset(b.Identifier, b.Dataset)

								if err != nil {
									return nil, err
								}

								if totalResult == s.resultPageSize {
									if pagingInfo != nil {

										if idIndex == pagingInfo.idIndex {
											resultIndex = pagingInfo.resultIndex + resultIndex

											if pagingInfo.linkPageIndex > -1 { // move pageIndex
												pageIndex += pagingInfo.linkPageIndex
											}
											if pageIndex == pagingInfo.linkPageIndex {
												linkIndex += pagingInfo.linkIndex
											}
										}

									}
									result.Nextpage = strconv.Itoa(idIndex) + pagingSep2 + strconv.Itoa(resultIndex) + pagingSep2 + strconv.Itoa(pageIndex) + pagingSep2 + strconv.Itoa(linkIndex)
									result.Results = xrefs
									return result, nil
								}

								xref2.Keyword = id
								xref2.Identifier = b.Identifier

								if q != nil {
									q.MapDataset = config.DataconfIDIntToString[xref2.Dataset]
									q.MapDatasetID = xref2.Dataset
									if len(q.Filter) > 0 {
										b, err := s.execCelGo(q, xref2)
										if err != nil {
											return nil, err
										}
										if b {

											if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
												xrefs = append(xrefs, xref2)
												totalResult++
											}

										}
									}
								} else {

									if _, ok := config.Dataconf[config.DataconfIDIntToString[xref2.Dataset]]["linkdataset"]; !ok {
										xrefs = append(xrefs, xref2)
										totalResult++
									}
								}
								linkIndex++
							}

						}
						}
					}
				} else {

					if idsDomain > 0 && xref.Dataset != idsDomain {
						continue
					}

					if totalResult == s.resultPageSize {
						if pagingInfo != nil {
							if idIndex == pagingInfo.idIndex {
								resultIndex = pagingInfo.resultIndex + resultIndex
							}
						}
						result.Nextpage = strconv.Itoa(idIndex) + pagingSep2 + strconv.Itoa(resultIndex)
						result.Results = xrefs
						return result, nil
					}

					xref.Identifier = id

					if q != nil {
						q.MapDataset = config.DataconfIDIntToString[xref.Dataset]
						q.MapDatasetID = xref.Dataset
						if len(q.Filter) > 0 {
							b, err := s.execCelGo(q, xref)
							if err != nil {
								return nil, err
							}
							if b {
								xrefs = append(xrefs, xref)
								totalResult++
							}
						}
					} else {

						if _, ok := config.Dataconf[config.DataconfIDIntToString[xref.Dataset]]["linkdataset"]; !ok {
							xrefs = append(xrefs, xref)
							totalResult++
						}

					}
				}

				resultIndex++

			}

		}

	}

	result.Results = xrefs

	// Add informational message when no results found
	if len(xrefs) == 0 {
		result.Message = "No results found"
	}

	// Enrich with all transient fields (dataset_name, url)
	EnrichResult(result)

	return result, nil

}

// Lookup performs a case-insensitive lookup with federation routing
func (s *Service) Lookup(identifier string) (*pbuf.Result, error) {
	// LMDB keys are stored uppercase - normalize input
	identifier = strings.ToUpper(identifier)

	// Use federation routing based on identifier pattern
	env, dbi := s.getDBForIdentifier(identifier)

	var v []byte
	err := env.View(func(txn db.Txn) (err error) {

		v, err = txn.Get(dbi, []byte(identifier))

		if db.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	r := pbuf.Result{}

	if len(v) > 0 {
		err = proto.Unmarshal(v, &r)
		return &r, err
	}

	return &r, nil

}

// LookupByDataset retrieves a specific Xref entry for an identifier in a dataset
func (s *Service) LookupByDataset(identifier string, domainID uint32) (*pbuf.Xref, error) {
	// LMDB keys are stored uppercase - normalize input
	identifier = strings.ToUpper(identifier)

	// Use federation routing based on domain (dataset) ID
	env, dbi := s.getDBForDataset(domainID)

	var v []byte
	err := env.View(func(txn db.Txn) (err error) {
		//cur, err := txn.OpenCursor(s.readDbi)
		//_, v, err := cur.Get([]byte(identifier), nil, lmdb.SetKey)
		v, err = txn.Get(dbi, []byte(identifier))

		if db.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	//todo handle empty response...like previous func
	r := pbuf.Result{}
	err = proto.Unmarshal(v, &r)
	if err != nil {
		return nil, err
	}
	// in result get target xref result
	var targetXref *pbuf.Xref
	for _, xref := range r.Results {
		if xref.Dataset == domainID {
			targetXref = xref
			targetXref.Identifier = identifier
			break
		}
	}

	if targetXref == nil {
		err := fmt.Errorf("Entry not found identifier %s dataset  %s. Make sure it is actual identifier not keyword", identifier, config.DataconfIDIntToString[domainID])
		return nil, err
	}

	// Enrich with all transient fields (dataset_name, url)
	EnrichXref(targetXref)

	return targetXref, nil

}

// searchLite performs a search and returns compact lite format response
// Uses the main search function and converts the result to lite format
// Returns only IDs, sorted by has_attr (entries with attributes first)
func (s *Service) searchLite(ids []string, idsDomain uint32, page string, datasetFilter string) (*pbuf.ResultLite, error) {
	// Use the main search function - this ensures consistent behavior
	// detail=false triggers makeLiteAll which strips attributes
	// buildURL=false since we don't need URLs in lite mode
	fullResult, err := s.Search(ids, idsDomain, page, nil, false, false)
	if err != nil {
		return nil, err
	}

	// Convert full result to lite format
	result := &pbuf.ResultLite{
		Mode: "lite",
		Query: &pbuf.SearchQueryInfo{
			Terms:         ids,
			DatasetFilter: datasetFilter,
			Raw:           strings.Join(ids, ","),
		},
	}

	var liteResults []*pbuf.SearchResultLite
	statsByDataset := make(map[string]int32)

	// Track seen IDs to avoid duplicates
	seenIDs := make(map[string]bool)

	for _, xref := range fullResult.Results {
		datasetName := config.DataconfIDIntToString[xref.Dataset]
		identifier := xref.Identifier
		if identifier == "" {
			identifier = xref.Keyword
		}

		// Create unique key to avoid duplicates
		uniqueKey := identifier + ":" + datasetName
		if seenIDs[uniqueKey] {
			continue
		}
		seenIDs[uniqueKey] = true

		hasAttr := !xref.GetEmpty()

		liteResult := &pbuf.SearchResultLite{
			D:         datasetName,
			Id:        identifier,
			HasAttr:   hasAttr,
			XrefCount: xref.Count,
		}
		liteResults = append(liteResults, liteResult)
		statsByDataset[datasetName]++
	}

	// Sort: entries with attributes first
	sort.Slice(liteResults, func(i, j int) bool {
		if liteResults[i].HasAttr != liteResults[j].HasAttr {
			return liteResults[i].HasAttr // true (has attr) comes first
		}
		return false // stable sort for same has_attr value
	})

	// Apply lite mode pagination limit (5x higher than full mode)
	totalResults := int32(len(liteResults))
	hasNext := len(liteResults) > s.resultPageSizeLite
	if hasNext {
		liteResults = liteResults[:s.resultPageSizeLite]
	}

	result.Results = liteResults
	result.Stats = &pbuf.SearchStats{
		TotalResults: totalResults,
		Returned:     int32(len(liteResults)),
		ByDataset:    statsByDataset,
	}
	result.Pagination = &pbuf.PaginationInfo{
		Page:    1,
		HasNext: hasNext,
	}

	// Copy nextpage token if available
	if fullResult.Nextpage != "" {
		result.Pagination.NextToken = fullResult.Nextpage
		result.Pagination.HasNext = true
	}

	return result, nil
}

// =============================================================================
// EXPORTED HELPER METHODS FOR UPDATE PACKAGE
// =============================================================================

// Close closes all database connections
func (s *Service) Close() {
	for _, fed := range s.federations {
		if fed.env != nil {
			fed.env.Close()
		}
	}
}

// LookupInDataset looks up identifier (keyword) and returns the entry from the specified dataset.
// This searches through text link results (IsLink=true) and finds entries matching the dataset.
// Handles both inline entries and paginated entries.
func (s *Service) LookupInDataset(identifier string, datasetID uint32) (*pbuf.XrefEntry, error) {
	result, err := s.Lookup(identifier)
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Results) == 0 {
		return nil, nil
	}

	identifier = strings.ToUpper(identifier)

	// Search through results - look in IsLink entries for the target dataset
	for _, xref := range result.Results {
		if xref.IsLink {
			// Check inline entries first
			for _, entry := range xref.Entries {
				if entry.Dataset == datasetID {
					return entry, nil
				}
			}

			// Check paginated entries if not found inline
			if len(xref.Pages) > 0 {
				for _, page := range xref.Pages {
					// Build page key: identifier + " " + pageKeyPart + " " + page
					pageKey := identifier + " " + config.DataconfIDToPageKey[0] + " " + page
					pageResult, err := s.LookupPage(pageKey, xref.Dataset)
					if err != nil {
						continue
					}
					if pageResult != nil {
						for _, entry := range pageResult.Entries {
							if entry.Dataset == datasetID {
								return entry, nil
							}
						}
					}
				}
			}
		} else if xref.Dataset == datasetID {
			// Direct match (non-link result)
			return &pbuf.XrefEntry{
				Dataset:    xref.Dataset,
				Identifier: xref.Identifier,
			}, nil
		}
	}

	return nil, nil // No entry found for this dataset
}

// LookupFullEntry fetches the full Xref with attributes for a specific identifier in a dataset.
// This is used when you need to access attributes (e.g., to filter by genome).
func (s *Service) LookupFullEntry(identifier string, datasetID uint32) (*pbuf.Xref, error) {
	// LMDB keys are stored uppercase - normalize input
	identifier = strings.ToUpper(identifier)

	env, dbi := s.getDBForDataset(datasetID)

	var v []byte
	err := env.View(func(txn db.Txn) (err error) {
		v, err = txn.Get(dbi, []byte(identifier))
		if db.IsNotFound(err) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	if len(v) == 0 {
		return nil, nil
	}

	// Unmarshal the result
	result := &pbuf.Result{}
	err = proto.Unmarshal(v, result)
	if err != nil {
		return nil, err
	}

	// Find the Xref matching the dataset
	for _, xref := range result.Results {
		if xref.Dataset == datasetID {
			xref.Identifier = identifier
			return xref, nil
		}
	}

	return nil, nil
}

// LookupPage fetches a page of entries from the appropriate federation database.
// Pages are stored as pbuf.Result containing a single Xref with the page entries.
func (s *Service) LookupPage(pageKey string, datasetID uint32) (*pbuf.Xref, error) {
	env, dbi := s.getDBForDataset(datasetID)

	var v []byte
	err := env.View(func(txn db.Txn) (err error) {
		v, err = txn.Get(dbi, []byte(pageKey))
		if db.IsNotFound(err) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	if len(v) == 0 {
		return nil, nil
	}

	// Pages are stored as pbuf.Result, not pbuf.Xref
	result := &pbuf.Result{}
	err = proto.Unmarshal(v, result)
	if err != nil {
		return nil, err
	}

	// Extract the first Xref from Results
	if len(result.Results) == 0 {
		return nil, nil
	}

	return result.Results[0], nil
}

// GetDBForDataset returns the database connection for a specific dataset ID (exported version)
func (s *Service) GetDBForDataset(datasetID uint32) (db.Env, db.DBI) {
	return s.getDBForDataset(datasetID)
}
