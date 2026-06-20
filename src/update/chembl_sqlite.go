package update

import (
	"archive/tar"
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// chemblSqlite handles parsing ChEMBL data extracted from SQLite database.
// This is a simplified parser that creates DIRECT edges:
//   - chembl_target → uniprot (no intermediate target_component)
//   - chembl_molecule → chembl_target (drug to targets)
//
// Note: molecule → uniprot was removed as it was semantically confusing.
// Use the clearer path: molecule >> target >> uniprot
//
// Data is extracted via Python script (scripts/chembl/extract_chembl_data.py)
// If data files don't exist, the script is automatically invoked.
//
// Output datasets:
//   - chembl_target: Drug targets with direct UniProt xrefs
//   - chembl_molecule: Compounds/drugs with target xrefs
//   - chembl_activity: Bioactivity measurements (optional)
type chemblSqlite struct {
	source string
	d      *DataUpdate
}

// JSON structures matching extracted data format

// chemblTargetJSON matches the extracted target JSON format
type chemblTargetJSON struct {
	TargetID              string                    `json:"target_id"`
	Name                  string                    `json:"name"`
	TargetType            string                    `json:"target_type"`
	Organism              string                    `json:"organism"`
	TaxID                 *int                      `json:"tax_id"`
	IsSpeciesGroup        bool                      `json:"is_species_group"`
	UniprotIDs            []string                  `json:"uniprot_ids"`
	ComponentDescriptions []string                  `json:"component_descriptions"`
	BindingSites          []string                  `json:"binding_sites"`
	Relations             *chemblTargetRelationsJSON `json:"relations"`
	ProteinClasses        []chemblProteinClassJSON  `json:"protein_classes"`
	Mechanisms            []chemblMechanismJSON     `json:"mechanisms"`
}

type chemblTargetRelationsJSON struct {
	SubsetOf   []string `json:"subset_of"`
	SupersetOf []string `json:"superset_of"`
	Overlaps   []string `json:"overlaps"`
	Equivalent []string `json:"equivalent"`
}

type chemblProteinClassJSON struct {
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Level     *int   `json:"level"`
	Path      string `json:"path"`
}

type chemblMechanismJSON struct {
	Description string `json:"description"`
	Action      string `json:"action"`
}

// chemblMechanismRecordJSON matches one extracted drug_mechanism row
// (the chembl_mechanism dataset — curated molecule→target MOA edge).
type chemblMechanismRecordJSON struct {
	MechanismID        string `json:"mechanism_id"`
	MoleculeID         string `json:"molecule_id"`
	TargetID           string `json:"target_id"`
	TargetName         string `json:"target_name"`
	TargetType         string `json:"target_type"`
	MechanismOfAction  string `json:"mechanism_of_action"`
	ActionType         string `json:"action_type"`
	DirectInteraction  bool   `json:"direct_interaction"`
	MolecularMechanism bool   `json:"molecular_mechanism"`
	MechanismComment   string `json:"mechanism_comment"`
}

// chemblMoleculeJSON matches the extracted molecule JSON format
type chemblMoleculeJSON struct {
	MoleculeID        string   `json:"molecule_id"`
	Name              string   `json:"name"`
	MaxPhase          *float64 `json:"max_phase"`
	MoleculeType      string   `json:"molecule_type"`
	FirstApproval     *int     `json:"first_approval"`
	Oral              *bool    `json:"oral"`
	Parenteral        *bool    `json:"parenteral"`
	Topical           *bool    `json:"topical"`
	BlackBoxWarning   *bool    `json:"black_box_warning"`
	NaturalProduct    *bool    `json:"natural_product"`
	Prodrug           *bool    `json:"prodrug"`
	Smiles            string   `json:"smiles"`
	Inchi             string   `json:"inchi"`
	InchiKey          string   `json:"inchi_key"`
	MolecularWeight   *float64 `json:"molecular_weight"`
	Alogp             *float64 `json:"alogp"`
	Hba               *int     `json:"hba"`
	Hbd               *int     `json:"hbd"`
	Psa               *float64 `json:"psa"`
	Rtb               *int     `json:"rtb"`
	FullMolWeight     *float64 `json:"full_molecular_weight"`
	Formula           string   `json:"formula"`
	AromaticRings     *int     `json:"aromatic_rings"`
	HeavyAtoms        *int     `json:"heavy_atoms"`
	QedWeighted       *float64 `json:"qed_weighted"`
	Ro3Pass           string   `json:"ro3_pass"`
	LipinskiViolations *int      `json:"lipinski_violations"`
	UniprotIDs         []string `json:"uniprot_ids"`
	TargetIDs         []string `json:"target_ids"`
	Indications       []chemblIndicationJSON     `json:"indications"`
	Synonyms          []chemblSynonymJSON        `json:"synonyms"`
	AtcClassifications []chemblAtcJSON           `json:"atc_classifications"`
	ParentChemblID    string   `json:"parent_chembl_id"`
	ChildChemblIDs    []string `json:"child_chembl_ids"`
}

type chemblIndicationJSON struct {
	MeshID   string   `json:"mesh_id"`
	EfoID    string   `json:"efo_id"`
	MaxPhase *float64 `json:"max_phase"`
}

type chemblSynonymJSON struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type chemblAtcJSON struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Level1      string `json:"level1"`
	Level1Desc  string `json:"level1_desc"`
	Level2      string `json:"level2"`
	Level2Desc  string `json:"level2_desc"`
	Level3      string `json:"level3"`
	Level3Desc  string `json:"level3_desc"`
	Level4      string `json:"level4"`
	Level4Desc  string `json:"level4_desc"`
}

// chemblActivityJSON matches the extracted activity JSON format
type chemblActivityJSON struct {
	ActivityID       int64    `json:"activity_id"`
	MoleculeID       string   `json:"molecule_id"`
	TargetID         string   `json:"target_id"`
	UniprotID        string   `json:"uniprot_id"`
	StandardType     string   `json:"standard_type"`
	StandardRelation string   `json:"standard_relation"`
	StandardValue    *float64 `json:"standard_value"`
	StandardUnits    string   `json:"standard_units"`
	PchemblValue     *float64 `json:"pchembl_value"`
	AssayType        string   `json:"assay_type"`
	ConfidenceScore  *int     `json:"confidence_score"`
	BaoEndpoint      string   `json:"bao_endpoint"` // BAO ontology term for activity endpoint (e.g., BAO:0000190)
}

// chemblAssayJSON matches the extracted assay JSON format
type chemblAssayJSON struct {
	AssayID             string `json:"assay_id"`
	Description         string `json:"description"`
	AssayType           string `json:"assay_type"`
	TestType            string `json:"test_type"`
	ConfidenceScore     *int   `json:"confidence_score"`
	Category            string `json:"category"`
	CellType            string `json:"cell_type"`
	Tissue              string `json:"tissue"`
	SubcellularFraction string `json:"subcellular_fraction"`
	Strain              string `json:"strain"`
	TargetID            string `json:"target_id"`
	DocumentID          string `json:"document_id"`
	Source              string `json:"source"`
	BaoFormat           string `json:"bao_format"` // BAO ontology term for assay format (e.g., BAO:0000019)
}

// chemblDocumentJSON matches the extracted document JSON format
type chemblDocumentJSON struct {
	DocumentID string `json:"document_id"`
	Title      string `json:"title"`
	DocType    string `json:"doc_type"`
	PubmedID   *int64 `json:"pubmed_id"`
	DOI        string `json:"doi"`
	Journal    string `json:"journal"`
	Year       *int   `json:"year"`
	Volume     string `json:"volume"`
	FirstPage  string `json:"first_page"`
	LastPage   string `json:"last_page"`
	Authors    string `json:"authors"`
}

// chemblCellLineJSON matches the extracted cell line JSON format
type chemblCellLineJSON struct {
	CellLineID    string `json:"cell_line_id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Tissue        string `json:"tissue"`
	Organism      string `json:"organism"`
	TaxID         *int   `json:"tax_id"`
	CLOID         string `json:"clo_id"`
	EFOID         string `json:"efo_id"`
	CellosaurusID string `json:"cellosaurus_id"`
}

// update is the main entry point for ChEMBL SQLite-based dataset processing
func (c *chemblSqlite) update() {
	defer c.d.wg.Done()

	log.Printf("ChEMBL-SQLite: Starting processing for %s...", c.source)

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	if config.IsTestMode() {
		log.Printf("ChEMBL-SQLite: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Check if data files exist, run extraction if needed
	if !c.ensureDataFilesExist() {
		log.Println("ChEMBL-SQLite: Failed to ensure data files exist, aborting")
		c.d.progChan <- &progressInfo{dataset: c.source, done: true}
		return
	}

	// Process based on source
	switch c.source {
	case "chembl_target":
		count := c.processTargets(testLimit)
		log.Printf("ChEMBL-SQLite: Processed %d targets", count)
	case "chembl_molecule":
		count := c.processMolecules(testLimit)
		log.Printf("ChEMBL-SQLite: Processed %d molecules", count)
	case "chembl_activity":
		count := c.processActivities(testLimit)
		log.Printf("ChEMBL-SQLite: Processed %d activities", count)
	case "chembl_assay":
		count := c.processAssays(testLimit)
		log.Printf("ChEMBL-SQLite: Processed %d assays", count)
	case "chembl_document":
		count := c.processDocuments(testLimit)
		log.Printf("ChEMBL-SQLite: Processed %d documents", count)
	case "chembl_cell_line":
		count := c.processCellLines(testLimit)
		log.Printf("ChEMBL-SQLite: Processed %d cell lines", count)
	case "chembl_mechanism":
		count := c.processMechanisms(testLimit)
		log.Printf("ChEMBL-SQLite: Processed %d mechanisms", count)
	default:
		log.Printf("ChEMBL-SQLite: Unknown source %s", c.source)
	}

	log.Printf("ChEMBL-SQLite: Processing complete for %s", c.source)
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

// ensureDataFilesExist checks if the required data files exist.
// If they don't exist, it runs the Python extraction script to generate them.
func (c *chemblSqlite) ensureDataFilesExist() bool {
	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	// Get the data file path for current source
	dataPath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if dataPath == "" {
		// Fall back to default naming convention
		switch c.source {
		case "chembl_target":
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_targets.jsonl", rootDir)
		case "chembl_molecule":
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_molecules.jsonl", rootDir)
		case "chembl_activity":
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_activities.jsonl", rootDir)
		case "chembl_assay":
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_assays.jsonl", rootDir)
		case "chembl_document":
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_documents.jsonl", rootDir)
		case "chembl_cell_line":
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_cell_lines.jsonl", rootDir)
		case "chembl_mechanism":
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_mechanisms.jsonl", rootDir)
		default:
			dataPath = c.resolvePath("raw_data/chembl/extracted/chembl_targets.jsonl", rootDir)
		}
	}

	if fileExists(dataPath) {
		log.Printf("ChEMBL-SQLite: Data file exists: %s", dataPath)
		return true
	}

	// Files don't exist - need to run extraction
	log.Printf("ChEMBL-SQLite: Data file not found at %s, running extraction...", dataPath)

	// Get script path from config (with default)
	scriptPath := config.Appconf["chemblExtractScript"]
	if scriptPath == "" {
		scriptPath = "src/scripts/chembl/extract_chembl_data.py"
	}
	scriptPath = c.resolvePath(scriptPath, rootDir)

	if !fileExists(scriptPath) {
		log.Printf("ChEMBL-SQLite: Extraction script not found at %s", scriptPath)
		return false
	}

	// Resolve the SQLite DB. With an explicit chemblSqliteDb it must already exist
	// (no download); otherwise it's the configured version, auto-downloaded + cached.
	dbPath, chemblDir, version, autoDownload := c.chemblDBPath(rootDir)
	if autoDownload {
		if !c.ensureChemblDB(dbPath, chemblDir, version) {
			log.Printf("ChEMBL-SQLite: ChEMBL %s SQLite DB unavailable at %s", version, dbPath)
			return false
		}
	} else if !fileExists(dbPath) {
		log.Printf("ChEMBL-SQLite: chemblSqliteDb points to %s but it does not exist (unset it to auto-download ChEMBL %s)", dbPath, version)
		return false
	}

	// Get output directory
	outputDir := filepath.Dir(dataPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("ChEMBL-SQLite: Failed to create output directory: %v", err)
		return false
	}

	// Build command
	args := []string{scriptPath, "--db", dbPath, "--output-dir", outputDir}
	if config.IsTestMode() {
		args = append(args, "--test-mode")
	}

	log.Printf("ChEMBL-SQLite: Running extraction: python3 %s", strings.Join(args, " "))

	cmd := exec.Command("python3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Printf("ChEMBL-SQLite: Extraction script failed: %v", err)
		return false
	}

	if !fileExists(dataPath) {
		log.Printf("ChEMBL-SQLite: Extraction completed but data file not found at %s", dataPath)
		return false
	}

	log.Println("ChEMBL-SQLite: Extraction completed successfully")
	return true
}

// resolvePath resolves a path relative to rootDir if it's not absolute
func (c *chemblSqlite) resolvePath(path, rootDir string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootDir, path)
}

const chemblFTPBase = "https://ftp.ebi.ac.uk/pub/databases/chembl/ChEMBLdb/"

var (
	// chemblDBMutex serializes the (large) SQLite download/extract across the 6
	// chembl sub-datasets, which run as parallel goroutines.
	chemblDBMutex sync.Mutex
	// chemblReleaseOnce ensures the upstream-release check logs only once per run.
	chemblReleaseOnce sync.Once
)

// chemblDBPath returns the SQLite path, the cache directory, the version, and
// whether biobtree may auto-download it.
//
//   - If chemblSqliteDb is set (explicit override), use that path as-is and do
//     NOT auto-download — for testing or pointing at a pre-existing DB.
//   - Otherwise derive <chemblDir>/chembl_<v>/chembl_<v>_sqlite/chembl_<v>.db from
//     the configurable chemblVersion / chemblDir and auto-download (cached) if
//     missing. No version is hardcoded.
func (c *chemblSqlite) chemblDBPath(rootDir string) (dbPath, chemblDir, version string, autoDownload bool) {
	version = config.Appconf["chemblVersion"]
	if version == "" {
		version = "37"
	}
	chemblDir = config.Appconf["chemblDir"]
	if chemblDir == "" {
		chemblDir = "raw_data/chembl"
	}
	chemblDir = c.resolvePath(chemblDir, rootDir)

	if override := config.Appconf["chemblSqliteDb"]; override != "" {
		return c.resolvePath(override, rootDir), chemblDir, version, false
	}

	name := "chembl_" + version
	dbPath = filepath.Join(chemblDir, name, name+"_sqlite", name+".db")
	return dbPath, chemblDir, version, true
}

// ensureChemblDB returns true once the configured-version SQLite DB is available,
// downloading+extracting it (cached, kept for later builds) if missing. Safe to
// call concurrently from the parallel chembl datasets.
func (c *chemblSqlite) ensureChemblDB(dbPath, chemblDir, version string) bool {
	chemblReleaseOnce.Do(func() { checkChemblRelease(version) })

	if fileExists(dbPath) {
		return true
	}

	// Only one goroutine downloads; the rest wait then find the DB present.
	chemblDBMutex.Lock()
	defer chemblDBMutex.Unlock()
	if fileExists(dbPath) {
		return true
	}

	log.Printf("ChEMBL-SQLite: ChEMBL %s SQLite not found at %s, downloading (~6GB compressed, kept cached)...", version, dbPath)
	if err := downloadAndExtractChembl(version, chemblDir); err != nil {
		log.Printf("ChEMBL-SQLite: download/extract failed: %v", err)
		return false
	}
	if !fileExists(dbPath) {
		log.Printf("ChEMBL-SQLite: extracted archive but DB not at expected path %s", dbPath)
		return false
	}
	log.Printf("ChEMBL-SQLite: ChEMBL %s ready at %s", version, dbPath)
	return true
}

// checkChemblRelease logs (informational, non-fatal) whether a newer ChEMBL
// release than `current` is available upstream.
func checkChemblRelease(current string) {
	resp, err := httpGetWithRetry(chemblFTPBase+"latest/", 2)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	m := regexp.MustCompile(`chembl_(\d+)_sqlite\.tar\.gz`).FindStringSubmatch(string(body))
	if len(m) < 2 {
		return
	}
	latest := m[1]
	li, _ := strconv.Atoi(latest)
	ci, _ := strconv.Atoi(current)
	if li > ci {
		log.Printf("ChEMBL-SQLite: NOTE — ChEMBL %s is available upstream (currently using %s). To upgrade, set chemblVersion=%s in application.param.json.", latest, current, latest)
	} else {
		log.Printf("ChEMBL-SQLite: ChEMBL %s is the latest upstream release", current)
	}
}

// downloadAndExtractChembl fetches chembl_<version>_sqlite.tar.gz into chemblDir,
// extracts it, and removes the tarball (keeping the extracted .db cached).
func downloadAndExtractChembl(version, chemblDir string) error {
	if err := os.MkdirAll(chemblDir, 0755); err != nil {
		return err
	}
	name := "chembl_" + version + "_sqlite.tar.gz"
	url := chemblFTPBase + "releases/chembl_" + version + "/" + name
	tarPath := filepath.Join(chemblDir, name)

	resp, err := httpGetWithRetry(url, 3)
	if err != nil {
		return fmt.Errorf("download %s: %v", url, err)
	}
	out, err := os.Create(tarPath)
	if err != nil {
		resp.Body.Close()
		return err
	}
	n, copyErr := io.Copy(out, resp.Body)
	resp.Body.Close()
	out.Close()
	if copyErr != nil {
		os.Remove(tarPath)
		return fmt.Errorf("download interrupted after %d bytes: %v", n, copyErr)
	}
	log.Printf("ChEMBL-SQLite: downloaded %s (%d bytes), extracting...", name, n)

	if err := extractTarGz(tarPath, chemblDir); err != nil {
		return err
	}
	// Keep the extracted .db cached; only the (large) tarball is removed.
	os.Remove(tarPath)
	return nil
}

// extractTarGz extracts a .tar.gz into destDir, guarding against path traversal.
func extractTarGz(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	cleanDest := filepath.Clean(destDir)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, hdr.Name)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in tar: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			w, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, tr); err != nil {
				w.Close()
				return err
			}
			w.Close()
		}
	}
	return nil
}

// processTargets processes the targets file (chembl_targets.jsonl)
func (c *chemblSqlite) processTargets(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if filePath == "" {
		filePath = c.resolvePath("raw_data/chembl/extracted/chembl_targets.jsonl", rootDir)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ChEMBL-SQLite: Error opening targets file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry chemblTargetJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("ChEMBL-SQLite: Error parsing target JSON: %v", err)
			continue
		}

		if entry.TargetID == "" {
			continue
		}

		// Build protobuf attribute
		attr := &pbuf.ChemblAttr{
			Target: &pbuf.ChemblTarget{
				Title: entry.Name,
				Type:  entry.TargetType,
			},
		}

		if entry.TaxID != nil {
			attr.Target.Tax = itoa(*entry.TaxID)
		}

		if entry.IsSpeciesGroup {
			attr.Target.IsSpeciesGroup = "true"
		}

		// Binding sites
		if len(entry.BindingSites) > 0 {
			attr.Target.BindingSite = &pbuf.ChemblBindingSite{
				Name: strings.Join(entry.BindingSites, "; "),
			}
		}

		// Target relations
		if entry.Relations != nil {
			attr.Target.Subsetofs = entry.Relations.SubsetOf
			attr.Target.Subsets = entry.Relations.SupersetOf
			attr.Target.Overlaps = entry.Relations.Overlaps
			attr.Target.Equivalents = entry.Relations.Equivalent
		}

		// Protein classifications - DISABLED: redundant with UniProt/GO/InterPro
		// for _, pc := range entry.ProteinClasses {
		// 	ptc := &pbuf.ChemblProteinTargetClassification{
		// 		ClassName: pc.Name,
		// 		ClassPath: pc.Path,
		// 	}
		// 	if pc.Level != nil {
		// 		ptc.ClassLevel = "L" + itoa(*pc.Level)
		// 	}
		// 	attr.Target.Ptclassifications = append(attr.Target.Ptclassifications, ptc)
		// }

		// Drug mechanisms
		if len(entry.Mechanisms) > 0 {
			// Use first mechanism as primary
			attr.Target.Mechanism = &pbuf.ChemblMechanism{
				Desc:   entry.Mechanisms[0].Description,
				Action: entry.Mechanisms[0].Action,
			}
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("ChEMBL-SQLite: Error marshaling target %s: %v", entry.TargetID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(entry.TargetID, sourceID, attrBytes)

		// Text search: target name
		if entry.Name != "" && len(entry.Name) > 2 && len(entry.Name) < 500 {
			c.d.addXref(entry.Name, textLinkID, entry.TargetID, c.source, true)
		}

		// DIRECT target → uniprot cross-references
		// This is the key simplification: no intermediate target_component
		if _, uniprotExists := config.Dataconf["uniprot"]; uniprotExists {
			for _, uniprotID := range entry.UniprotIDs {
				if uniprotID != "" {
					c.d.addXref(entry.TargetID, sourceID, uniprotID, "uniprot", false)
				}
			}
		}

		// Cross-reference to taxonomy
		if entry.TaxID != nil {
			if _, taxExists := config.Dataconf["taxonomy"]; taxExists {
				c.d.addXref(entry.TargetID, sourceID, itoa(*entry.TaxID), "taxonomy", false)
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.TargetID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ChEMBL-SQLite: [TEST MODE] Reached target limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ChEMBL-SQLite: Scanner error reading targets: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// processMolecules processes the molecules file (chembl_molecules.jsonl)
func (c *chemblSqlite) processMolecules(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if filePath == "" {
		filePath = c.resolvePath("raw_data/chembl/extracted/chembl_molecules.jsonl", rootDir)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ChEMBL-SQLite: Error opening molecules file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry chemblMoleculeJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("ChEMBL-SQLite: Error parsing molecule JSON: %v", err)
			continue
		}

		if entry.MoleculeID == "" {
			continue
		}

		// Build protobuf attribute
		attr := &pbuf.ChemblAttr{
			Molecule: &pbuf.ChemblMolecule{
				Name: entry.Name,
				Type: entry.MoleculeType,
				// SMILES kept inline so it can be a compact field (saves Atlas a
				// >>chembl_molecule>>pubchem hop). Other molecular properties stay
				// DISABLED - redundant with PubChem (use >>chembl_molecule>>pubchem).
				Smiles: entry.Smiles,
				// Inchi:            entry.Inchi,
				// InchiKey:         entry.InchiKey,
				// Formula:          entry.Formula,
				// Ro3Pass:          entry.Ro3Pass,
			},
		}

		if entry.MaxPhase != nil {
			attr.Molecule.HighestDevelopmentPhase = int32(int(*entry.MaxPhase))
		}

		// Molecular properties DISABLED - redundant with PubChem
		// if entry.MolecularWeight != nil {
		// 	attr.Molecule.WeightFreebase = *entry.MolecularWeight
		// }
		// if entry.FullMolWeight != nil {
		// 	attr.Molecule.Weight = *entry.FullMolWeight
		// }
		// if entry.Alogp != nil {
		// 	attr.Molecule.Alogp = *entry.Alogp
		// }
		// if entry.Hba != nil {
		// 	attr.Molecule.Hba = float64(*entry.Hba)
		// }
		// if entry.Hbd != nil {
		// 	attr.Molecule.Hbd = float64(*entry.Hbd)
		// }
		// if entry.Psa != nil {
		// 	attr.Molecule.Psa = *entry.Psa
		// }
		// if entry.Rtb != nil {
		// 	attr.Molecule.Rtb = float64(*entry.Rtb)
		// }
		// if entry.AromaticRings != nil {
		// 	attr.Molecule.AromaticRings = float64(*entry.AromaticRings)
		// }
		// if entry.HeavyAtoms != nil {
		// 	attr.Molecule.HeavyAtoms = float64(*entry.HeavyAtoms)
		// }
		// if entry.QedWeighted != nil {
		// 	attr.Molecule.QedWeighted = *entry.QedWeighted
		// }
		// if entry.LipinskiViolations != nil {
		// 	attr.Molecule.NumRo5Violations = float64(*entry.LipinskiViolations)
		// }

		// Parent molecule
		if entry.ParentChemblID != "" {
			attr.Molecule.Parent = entry.ParentChemblID
		}

		// Child molecules
		if len(entry.ChildChemblIDs) > 0 {
			attr.Molecule.Childs = entry.ChildChemblIDs
		}

		// Hierarchical parent/child edges (issue #15): ChEMBL stores salt/anhydrous
		// forms as separate molecule IDs. Following the reactome/ontology linkdataset
		// pattern, lay traversable edges so >>chembl_molecule>>chembl_moleculeparent
		// (salt -> parent) and >>chembl_molecule>>chembl_moleculechild (parent ->
		// salts) resolve in a single map hop. Driven by each child's parent_chembl_id.
		if entry.ParentChemblID != "" && entry.ParentChemblID != entry.MoleculeID {
			parentLinkID := config.Dataconf["chembl_moleculeparent"]["id"]
			childLinkID := config.Dataconf["chembl_moleculechild"]["id"]
			// child (this molecule) -> parent
			c.d.addXref2(entry.MoleculeID, sourceID, entry.ParentChemblID, "chembl_moleculeparent")
			c.d.addXref2(entry.ParentChemblID, parentLinkID, entry.ParentChemblID, c.source)
			// parent -> child
			c.d.addXref2(entry.ParentChemblID, sourceID, entry.MoleculeID, "chembl_moleculechild")
			c.d.addXref2(entry.MoleculeID, childLinkID, entry.MoleculeID, c.source)
		}

		// Synonyms as altNames (deduplicated)
		seenNames := make(map[string]bool)
		for _, syn := range entry.Synonyms {
			if syn.Name != "" && !seenNames[syn.Name] {
				seenNames[syn.Name] = true
				attr.Molecule.AltNames = append(attr.Molecule.AltNames, syn.Name)
			}
		}

		// ATC classifications (just the codes for simplicity)
		for _, atc := range entry.AtcClassifications {
			if atc.Code != "" {
				attr.Molecule.AtcClassification = append(attr.Molecule.AtcClassification, atc.Code)
			}
		}

		// Indications (IDs and phase only, names available via xrefs to efo/mesh)
		for _, ind := range entry.Indications {
			indication := &pbuf.ChemblIndication{
				Mesh: ind.MeshID,
				Efo:  ind.EfoID,
			}
			if ind.MaxPhase != nil {
				indication.HighestDevelopmentPhase = int32(int(*ind.MaxPhase))
			}
			attr.Molecule.Indications = append(attr.Molecule.Indications, indication)
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("ChEMBL-SQLite: Error marshaling molecule %s: %v", entry.MoleculeID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(entry.MoleculeID, sourceID, attrBytes)

		// Text search: molecule name
		if entry.Name != "" && len(entry.Name) > 2 && len(entry.Name) < 500 {
			c.d.addXref(entry.Name, textLinkID, entry.MoleculeID, c.source, true)
		}

		// Text search: InChI Key (useful for structure lookup)
		if entry.InchiKey != "" {
			c.d.addXref(entry.InchiKey, textLinkID, entry.MoleculeID, c.source, true)

			// Cross-reference to ChEBI via InChI Key
			// This creates chembl_molecule → chebi links for structurally identical compounds
			if _, chebiExists := config.Dataconf["chebi"]; chebiExists {
				c.d.addXrefViaKeyword(entry.InchiKey, "chebi", entry.MoleculeID, c.source, sourceID, false)
			}
		}

		// Text search: synonyms (trade names, research codes, etc.)
		seenKeywords := make(map[string]bool)
		if entry.Name != "" {
			seenKeywords[entry.Name] = true // already indexed above
		}
		for _, syn := range entry.Synonyms {
			if syn.Name != "" && len(syn.Name) > 2 && len(syn.Name) < 200 && !seenKeywords[syn.Name] {
				seenKeywords[syn.Name] = true
				c.d.addXref(syn.Name, textLinkID, entry.MoleculeID, c.source, true)
			}
		}

		// COMMENTED OUT: Direct molecule → uniprot was confusing
		// The semantic path molecule >> target >> uniprot is clearer
		// (drug acts on targets, target is protein)
		//
		// if _, uniprotExists := config.Dataconf["uniprot"]; uniprotExists {
		// 	for _, uniprotID := range entry.UniprotIDs {
		// 		if uniprotID != "" {
		// 			c.d.addXref(entry.MoleculeID, sourceID, uniprotID, "uniprot", false)
		// 		}
		// 	}
		// }

		// Cross-reference to targets
		if _, targetExists := config.Dataconf["chembl_target"]; targetExists {
			for _, targetID := range entry.TargetIDs {
				if targetID != "" {
					c.d.addXref(entry.MoleculeID, sourceID, targetID, "chembl_target", false)
				}
			}
		}

		// Cross-reference to indications (EFO and MeSH)
		for _, ind := range entry.Indications {
			if ind.EfoID != "" {
				if _, efoExists := config.Dataconf["efo"]; efoExists {
					c.d.addXref(entry.MoleculeID, sourceID, ind.EfoID, "efo", false)
				}
			}
			if ind.MeshID != "" {
				if _, meshExists := config.Dataconf["mesh"]; meshExists {
					c.d.addXref(entry.MoleculeID, sourceID, ind.MeshID, "mesh", false)
				}
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.MoleculeID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ChEMBL-SQLite: [TEST MODE] Reached molecule limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ChEMBL-SQLite: Scanner error reading molecules: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// processActivities processes the activities file (chembl_activities.jsonl)
func (c *chemblSqlite) processActivities(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if filePath == "" {
		filePath = c.resolvePath("raw_data/chembl/extracted/chembl_activities.jsonl", rootDir)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ChEMBL-SQLite: Error opening activities file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry chemblActivityJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("ChEMBL-SQLite: Error parsing activity JSON: %v", err)
			continue
		}

		if entry.ActivityID == 0 {
			continue
		}

		// Generate activity ID string
		activityID := "CHEMBL_ACT_" + itoa64(entry.ActivityID)

		// Build protobuf attribute
		attr := &pbuf.ChemblAttr{
			Activity: &pbuf.ChemblActivity{
				StandardType:     entry.StandardType,
				StandardRelation: entry.StandardRelation,
				StandardUnits:    entry.StandardUnits,
				Bao:              entry.BaoEndpoint, // BAO ontology term for activity endpoint
			},
		}

		if entry.StandardValue != nil {
			attr.Activity.StandardValue = *entry.StandardValue
		}
		if entry.PchemblValue != nil {
			attr.Activity.PChembl = *entry.PchemblValue
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("ChEMBL-SQLite: Error marshaling activity %d: %v", entry.ActivityID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(activityID, sourceID, attrBytes)

		// Compute pChembl sort level (higher pChembl = stronger binding = sorts first)
		pchembl := 0.0
		if entry.PchemblValue != nil {
			pchembl = *entry.PchemblValue
		}
		sortLevels := []string{
			ComputeSortLevelValue(SortLevelPchemblScore, map[string]interface{}{"score": pchembl}),
		}

		// Cross-reference to molecule (sorted by pChembl)
		if entry.MoleculeID != "" {
			if _, molExists := config.Dataconf["chembl_molecule"]; molExists {
				c.d.addXrefWithSortLevels(activityID, sourceID, entry.MoleculeID, "chembl_molecule", sortLevels)
			}
		}

		// Cross-reference to target (sorted by pChembl)
		if entry.TargetID != "" {
			if _, targetExists := config.Dataconf["chembl_target"]; targetExists {
				c.d.addXrefWithSortLevels(activityID, sourceID, entry.TargetID, "chembl_target", sortLevels)
			}
		}

		// Cross-reference to uniprot (sorted by pChembl)
		if entry.UniprotID != "" {
			if _, uniprotExists := config.Dataconf["uniprot"]; uniprotExists {
				c.d.addXrefWithSortLevels(activityID, sourceID, entry.UniprotID, "uniprot", sortLevels)
			}
		}

		// Cross-reference to BAO (BioAssay Ontology) - no pChembl sorting needed
		if entry.BaoEndpoint != "" {
			if _, baoExists := config.Dataconf["bao"]; baoExists {
				c.d.addXref(activityID, sourceID, entry.BaoEndpoint, "bao", false)
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, activityID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ChEMBL-SQLite: [TEST MODE] Reached activity limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ChEMBL-SQLite: Scanner error reading activities: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// processAssays processes the assays file (chembl_assays.jsonl)
// processMechanisms builds the chembl_mechanism dataset from chembl_mechanisms.jsonl.
// Each record is one curated drug_mechanism row (MOA + action_type) linking a molecule
// to a target. Edges: chembl_molecule <-> chembl_mechanism <-> chembl_target. This is
// the curated drug→target ChEMBL bioactivity misses — notably RNA therapeutics, which
// have no bioactivity edge at all (Atlas #49).
func (c *chemblSqlite) processMechanisms(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if filePath == "" {
		filePath = c.resolvePath("raw_data/chembl/extracted/chembl_mechanisms.jsonl", rootDir)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ChEMBL-SQLite: Error opening mechanisms file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry chemblMechanismRecordJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("ChEMBL-SQLite: Error parsing mechanism JSON: %v", err)
			continue
		}

		if entry.MechanismID == "" || entry.MoleculeID == "" {
			continue
		}

		attr := &pbuf.ChemblAttr{
			Mechanism: &pbuf.ChemblMechanismRecord{
				MechanismOfAction:  entry.MechanismOfAction,
				ActionType:         entry.ActionType,
				DirectInteraction:  entry.DirectInteraction,
				MolecularMechanism: entry.MolecularMechanism,
				MechanismComment:   entry.MechanismComment,
				TargetName:         entry.TargetName,
				TargetType:         entry.TargetType,
			},
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("ChEMBL-SQLite: Error marshaling mechanism %s: %v", entry.MechanismID, err)
			continue
		}
		c.d.addProp3(entry.MechanismID, sourceID, attrBytes)

		// Edge: chembl_mechanism <-> chembl_molecule
		// (reverse enables >>chembl_molecule>>chembl_mechanism)
		if _, ok := config.Dataconf["chembl_molecule"]; ok {
			c.d.addXref(entry.MechanismID, sourceID, entry.MoleculeID, "chembl_molecule", false)
		}

		// Edge: chembl_mechanism <-> chembl_target (protein targets reach the gene
		// transitively via chembl_target -> uniprot)
		if entry.TargetID != "" {
			if _, ok := config.Dataconf["chembl_target"]; ok {
				c.d.addXref(entry.MechanismID, sourceID, entry.TargetID, "chembl_target", false)
			}
		}

		// Searchable: the MOA phrase links to this mechanism
		if entry.MechanismOfAction != "" {
			c.d.addXref(entry.MechanismOfAction, textLinkID, entry.MechanismID, c.source, true)
		}

		// Phase 2: RNA / nucleic-acid targets carry no UniProt protein, so the
		// chembl_target -> uniprot bridge can't reach the gene. The gene is in the
		// target name ("PCSK9 mRNA"); resolve it so gene pages surface RNA
		// therapeutics (PCSK9 -> inclisiran, TTR -> patisiran).
		c.linkMechanismGeneFromRNATarget(&entry, sourceID)

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.MechanismID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ChEMBL-SQLite: [TEST MODE] Reached mechanism limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ChEMBL-SQLite: Scanner error reading mechanisms: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// linkMechanismGeneFromRNATarget (Phase 2) handles nucleic-acid (RNA) mechanism
// targets, which have no UniProt protein. It recovers the gene/protein name from
// the ChEMBL target name (e.g. "PCSK9 mRNA" -> "PCSK9") and resolves it via the
// lookup DB to hgnc / ensembl / uniprot, adding edges so the gene page surfaces the
// RNA therapeutic. Protein targets are skipped (they already reach the gene via
// chembl_target -> uniprot).
func (c *chemblSqlite) linkMechanismGeneFromRNATarget(entry *chemblMechanismRecordJSON, sourceID string) {
	if c.d.lookupService == nil || entry.TargetName == "" {
		return
	}
	tt := strings.ToUpper(entry.TargetType)
	lname := strings.ToLower(entry.TargetName)
	if !strings.Contains(tt, "NUCLEIC") && !strings.Contains(tt, "RNA") &&
		!strings.Contains(lname, "mrna") && !strings.Contains(lname, " rna") {
		return
	}

	gene := strings.TrimSpace(entry.TargetName)
	for _, suf := range []string{" mRNA", " pre-mRNA", " RNA", " mrna", " rna"} {
		gene = strings.TrimSuffix(gene, suf)
	}
	gene = strings.TrimSpace(gene)
	if gene == "" {
		return
	}

	result, err := c.d.lookup(gene)
	if err != nil || result == nil || len(result.Results) == 0 {
		return
	}

	hgncID := config.DataconfIDStringToInt["hgnc"]
	ensemblID := config.DataconfIDStringToInt["ensembl"]
	uniprotID := config.DataconfIDStringToInt["uniprot"]
	seen := make(map[string]bool)
	add := func(ds, id string) {
		if id == "" {
			return
		}
		key := ds + "\t" + id
		if seen[key] {
			return
		}
		seen[key] = true
		c.d.addXref(entry.MechanismID, sourceID, id, ds, false)
	}
	classify := func(ds uint32, id string) {
		switch ds {
		case hgncID:
			add("hgnc", id)
		case ensemblID:
			add("ensembl", id)
		case uniprotID:
			add("uniprot", id)
		}
	}
	for _, xref := range result.Results {
		if xref.IsLink {
			for _, e := range xref.Entries {
				classify(e.Dataset, e.Identifier)
			}
		} else {
			classify(xref.Dataset, xref.Identifier)
		}
	}
}

func (c *chemblSqlite) processAssays(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if filePath == "" {
		filePath = c.resolvePath("raw_data/chembl/extracted/chembl_assays.jsonl", rootDir)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ChEMBL-SQLite: Error opening assays file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry chemblAssayJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("ChEMBL-SQLite: Error parsing assay JSON: %v", err)
			continue
		}

		if entry.AssayID == "" {
			continue
		}

		// Build protobuf attribute
		attr := &pbuf.ChemblAttr{
			Assay: &pbuf.ChemblAssay{
				Desc:        entry.Description,
				Type:        entry.AssayType,
				TestType:    entry.TestType,
				Category:    entry.Category,
				CellType:    entry.CellType,
				Tissue:      entry.Tissue,
				SubCellFrac: entry.SubcellularFraction,
				Strain:      entry.Strain,
				Source:      entry.Source,
				Bao:         entry.BaoFormat, // BAO ontology term for assay format
			},
		}

		if entry.ConfidenceScore != nil {
			attr.Assay.TargetConfScore = int32(*entry.ConfidenceScore)
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("ChEMBL-SQLite: Error marshaling assay %s: %v", entry.AssayID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(entry.AssayID, sourceID, attrBytes)

		// Text search: assay description (if short enough)
		if entry.Description != "" && len(entry.Description) > 2 && len(entry.Description) < 200 {
			c.d.addXref(entry.Description, textLinkID, entry.AssayID, c.source, true)
		}

		// Cross-reference to target
		if entry.TargetID != "" {
			if _, targetExists := config.Dataconf["chembl_target"]; targetExists {
				c.d.addXref(entry.AssayID, sourceID, entry.TargetID, "chembl_target", false)
			}
		}

		// Cross-reference to document
		if entry.DocumentID != "" {
			if _, docExists := config.Dataconf["chembl_document"]; docExists {
				c.d.addXref(entry.AssayID, sourceID, entry.DocumentID, "chembl_document", false)
			}
		}

		// Cross-reference to BAO (BioAssay Ontology)
		if entry.BaoFormat != "" {
			if _, baoExists := config.Dataconf["bao"]; baoExists {
				c.d.addXref(entry.AssayID, sourceID, entry.BaoFormat, "bao", false)
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.AssayID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ChEMBL-SQLite: [TEST MODE] Reached assay limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ChEMBL-SQLite: Scanner error reading assays: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// processDocuments processes the documents file (chembl_documents.jsonl)
func (c *chemblSqlite) processDocuments(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if filePath == "" {
		filePath = c.resolvePath("raw_data/chembl/extracted/chembl_documents.jsonl", rootDir)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ChEMBL-SQLite: Error opening documents file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry chemblDocumentJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("ChEMBL-SQLite: Error parsing document JSON: %v", err)
			continue
		}

		if entry.DocumentID == "" {
			continue
		}

		// Build protobuf attribute
		attr := &pbuf.ChemblAttr{
			Doc: &pbuf.ChemblDocument{
				Title:   entry.Title,
				Type:    entry.DocType,
				Journal: entry.Journal,
			},
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("ChEMBL-SQLite: Error marshaling document %s: %v", entry.DocumentID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(entry.DocumentID, sourceID, attrBytes)

		// Text search: document title
		if entry.Title != "" && len(entry.Title) > 2 && len(entry.Title) < 500 {
			c.d.addXref(entry.Title, textLinkID, entry.DocumentID, c.source, true)
		}

		// Cross-reference to PubMed
		if entry.PubmedID != nil && *entry.PubmedID > 0 {
			if _, pmidExists := config.Dataconf["literature_mappings"]; pmidExists {
				c.d.addXref(entry.DocumentID, sourceID, itoa64(*entry.PubmedID), "literature_mappings", false)
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.DocumentID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ChEMBL-SQLite: [TEST MODE] Reached document limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ChEMBL-SQLite: Scanner error reading documents: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// processCellLines processes the cell lines file (chembl_cell_lines.jsonl)
func (c *chemblSqlite) processCellLines(testLimit int) int64 {
	sourceID := config.Dataconf[c.source]["id"]

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	filePath := c.resolvePath(config.Dataconf[c.source]["path"], rootDir)
	if filePath == "" {
		filePath = c.resolvePath("raw_data/chembl/extracted/chembl_cell_lines.jsonl", rootDir)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ChEMBL-SQLite: Error opening cell lines file %s: %v", filePath, err)
		return 0
	}
	defer file.Close()

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var entryCount int64

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry chemblCellLineJSON
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			log.Printf("ChEMBL-SQLite: Error parsing cell line JSON: %v", err)
			continue
		}

		if entry.CellLineID == "" {
			continue
		}

		// Build protobuf attribute
		attr := &pbuf.ChemblAttr{
			CellLine: &pbuf.ChemblCellLine{
				Desc:          entry.Description,
				CellosaurusId: entry.CellosaurusID,
				Efo:           entry.EFOID,
				Clo:           entry.CLOID,
			},
		}

		if entry.TaxID != nil {
			attr.CellLine.Tax = itoa(*entry.TaxID)
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("ChEMBL-SQLite: Error marshaling cell line %s: %v", entry.CellLineID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(entry.CellLineID, sourceID, attrBytes)

		// Text search: cell line name
		if entry.Name != "" && len(entry.Name) > 2 && len(entry.Name) < 200 {
			c.d.addXref(entry.Name, textLinkID, entry.CellLineID, c.source, true)
		}

		// Cross-reference to taxonomy
		if entry.TaxID != nil {
			if _, taxExists := config.Dataconf["taxonomy"]; taxExists {
				c.d.addXref(entry.CellLineID, sourceID, itoa(*entry.TaxID), "taxonomy", false)
			}
		}

		// Cross-reference to EFO
		if entry.EFOID != "" {
			if _, efoExists := config.Dataconf["efo"]; efoExists {
				c.d.addXref(entry.CellLineID, sourceID, entry.EFOID, "efo", false)
			}
		}

		// Cross-reference to Cellosaurus (if dataset exists)
		if entry.CellosaurusID != "" {
			if _, celloExists := config.Dataconf["cellosaurus"]; celloExists {
				c.d.addXref(entry.CellLineID, sourceID, entry.CellosaurusID, "cellosaurus", false)
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, entry.CellLineID)
		}

		entryCount++

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ChEMBL-SQLite: [TEST MODE] Reached cell line limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ChEMBL-SQLite: Scanner error reading cell lines: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(entryCount))
	return entryCount
}

// Helper functions

func itoa(i int) string {
	return strconv.Itoa(i)
}

func itoa64(i int64) string {
	return strconv.FormatInt(i, 10)
}
