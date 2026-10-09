package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// wikidataSymptom ingests disease -> symptom/sign statements from Wikidata
// (property P780), gated to disease subjects that carry a Mondo (P5270) or DOID
// (P699) mapping so every edge anchors to a disease node biobtree already has.
//
// It fills biobtree's COMMON-disease symptom gap. The authoritative structured
// disease->phenotype source (HPO's phenotype.hpoa, already ingested) is curated
// via OMIM/Orphanet/DECIPHER and so is rare/Mendelian-skewed; common diseases
// (type 2 diabetes, asthma, migraine) carry few HPO phenotypes. SNOMED CT covers
// them but is license-restricted. Wikidata P780 is CC0 and editor-curated from
// general medical knowledge, so it complements (not duplicates) HPO. It is
// general-knowledge data, not clinical-grade curation.
//
// Each symptom term becomes a wikidata_symptom entry (keyed by its Wikidata QID,
// reusing OntologyAttr with type="symptom"); each kept statement adds a
// disease->symptom cross-reference onto the mondo and/or doid term.
type wikidataSymptom struct {
	source   string
	sourceID string
	d        *DataUpdate
}

const (
	wikidataSymptomExtractScript = "src/scripts/wikidata/extract_wikidata_symptoms.py"
	wikidataSymptomDataPath      = "raw_data/wikidata/wikidata_symptoms.tsv"
)

func (w *wikidataSymptom) update() {
	defer w.d.wg.Done()

	w.sourceID = config.Dataconf[w.source]["id"]

	testLimit := config.GetTestLimit(w.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, w.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	filePath, err := w.resolveDataPath()
	if err != nil {
		log.Printf("Wikidata Symptom: %v", err)
		w.d.progChan <- &progressInfo{dataset: w.source, done: true}
		return
	}

	count, err := w.processFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Printf("Wikidata Symptom: error processing %s: %v", filePath, err)
	}
	fmt.Printf("Wikidata Symptom: processed %d disease-symptom edges\n", count)

	atomic.AddUint64(&w.d.totalParsedEntry, count)
	w.d.progChan <- &progressInfo{dataset: w.source, done: true}
}

// resolveDataPath returns the TSV to parse. In test mode the committed fixture
// (conf `path`) is used so the suite runs offline. In a real build the parser
// owns its source: it ensures raw_data/wikidata/wikidata_symptoms.tsv exists
// (running the SPARQL extract script if absent) and reads that, so the conf
// `path` never needs a fixture<->live toggle.
func (w *wikidataSymptom) resolveDataPath() (string, error) {
	if config.IsTestMode() {
		return config.Dataconf[w.source]["path"], nil
	}

	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}

	dataPath := wikidataSymptomDataPath
	if !filepath.IsAbs(dataPath) {
		dataPath = filepath.Join(rootDir, dataPath)
	}
	if fileExists(dataPath) {
		return dataPath, nil
	}

	scriptPath := wikidataSymptomExtractScript
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(rootDir, scriptPath)
	}
	if !fileExists(scriptPath) {
		return "", fmt.Errorf("extract script not found at %s", scriptPath)
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create output dir: %v", err)
	}

	log.Printf("Wikidata Symptom: data file not found, running extraction: python3 %s --output %s", scriptPath, dataPath)
	cmd := exec.Command("python3", scriptPath, "--output", dataPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("extract script failed: %v", err)
	}
	if !fileExists(dataPath) {
		return "", fmt.Errorf("extraction completed but data file missing at %s", dataPath)
	}
	return dataPath, nil
}

func (w *wikidataSymptom) processFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(w.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open wikidata-symptom file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024)
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024)
	}

	wikidataID := config.Dataconf["wikidata"]["id"]
	saved := map[string]bool{}  // symptom QID -> attr already written
	linked := map[string]bool{} // disease QID|mondo|doid -> disease->wikidata edge already written

	var count uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return count, fmt.Errorf("read error: %v", rerr)
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		if line != "" && !strings.HasPrefix(line, "symptom_qid\t") { // skip header
			if w.processRow(line, wikidataID, saved, linked, idLogFile) {
				count++
				if testLimit > 0 && int(count) >= testLimit {
					break
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	return count, nil
}

// columns: 0 symptom_qid, 1 symptom_label, 2 mondo, 3 doid, 4 disease_qid
func (w *wikidataSymptom) processRow(line, wikidataID string, saved, linked map[string]bool, idLogFile *os.File) bool {
	f := strings.Split(line, "\t")
	if len(f) < 4 {
		return false
	}
	sym := strings.TrimSpace(f[0])
	label := strings.TrimSpace(f[1])
	mondo := strings.TrimSpace(f[2])
	doid := strings.TrimSpace(f[3])
	disease := "" // disease's own Wikidata QID (column added later; tolerate 4-col rows)
	if len(f) > 4 {
		disease = strings.TrimSpace(f[4])
	}

	if sym == "" || (mondo == "" && doid == "") {
		return false
	}

	// Store the symptom term once per QID.
	if !saved[sym] {
		attr := &pbuf.OntologyAttr{Name: label, Type: "symptom"}
		if b, err := ffjson.Marshal(attr); err == nil {
			w.d.addProp3(sym, w.sourceID, b)
		}
		if label != "" {
			w.d.addXref(label, textLinkID, sym, w.source, true) // text search by symptom name
		}
		saved[sym] = true
		if idLogFile != nil {
			logProcessedID(idLogFile, sym)
		}
	}

	// symptom -> disease edges, keyed FROM the wikidata_symptom record (same
	// pattern as alliance_disease -> doid). addXref writes the forward edge into
	// wikidata_symptom's own bucket and the reverse into the disease's
	// from_wikidata_symptom bucket, so BOTH directions are owned by this parser's
	// update. Keying from the disease instead (addXref(mondo, mondoID, sym, ...))
	// routes the forward edge into mondo/doid's forward bucket, which is lost in
	// incremental --only builds when the hub dataset is merged before this one.
	if strings.HasPrefix(mondo, "MONDO:") {
		w.d.addXref(sym, w.sourceID, mondo, "mondo", false)
	}
	if strings.HasPrefix(doid, "DOID:") {
		w.d.addXref(sym, w.sourceID, doid, "doid", false)
	}

	// disease -> its own Wikidata item (e.g. DOID:6364 -> Q11081), so a disease
	// page links straight to Wikidata. Keyed FROM the Wikidata item (which lives
	// in the `wikidata` namespace, a childDataset of wikidata_symptom), so the
	// reverse disease->wikidata edge lands in mondo/doid's from_wikidata bucket
	// and survives incremental --only builds. Emitted once per disease.
	if strings.HasPrefix(disease, "Q") && wikidataID != "" {
		lk := disease + "|" + mondo + "|" + doid
		if !linked[lk] {
			if strings.HasPrefix(mondo, "MONDO:") {
				w.d.addXref(disease, wikidataID, mondo, "mondo", false)
			}
			if strings.HasPrefix(doid, "DOID:") {
				w.d.addXref(disease, wikidataID, doid, "doid", false)
			}
			linked[lk] = true
		}
	}
	return true
}
