package update

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// wikidata links each biobtree disease node (Mondo / Disease Ontology) to its
// own Wikidata item (QID), comprehensively: every Wikidata item carrying a Mondo
// ID (P5270) or DOID (P699) -> its QID, independent of symptoms. This replaces
// the old symptom-gated QID byproduct in wikidata_symptom (which covered only
// ~15% of diseases). Link-only: the QID becomes reachable as mondo/doid >>
// wikidata (the Wikidata page URL comes from conf `url`).
//
// Edges are keyed FROM the Wikidata item (addXref(qid, ..., mondo/doid)), so the
// reverse mondo/doid -> wikidata lands in the hub's from_wikidata bucket and
// survives incremental --only builds (same lesson as wikidata_symptom).
type wikidata struct {
	source   string
	sourceID string
	d        *DataUpdate
}

const (
	wikidataExtractScript = "src/scripts/wikidata/extract_wikidata_diseases.py"
	wikidataDataPath      = "raw_data/wikidata/wikidata_diseases.tsv"
)

func (w *wikidata) update() {
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
		log.Printf("Wikidata: %v", err)
		w.d.progChan <- &progressInfo{dataset: w.source, done: true}
		return
	}

	count, err := w.processFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Printf("Wikidata: error processing %s: %v", filePath, err)
	}
	fmt.Printf("Wikidata: processed %d disease->QID links\n", count)

	atomic.AddUint64(&w.d.totalParsedEntry, count)
	w.d.progChan <- &progressInfo{dataset: w.source, done: true}
}

// resolveDataPath: test mode reads the committed fixture (conf `path`); a real
// build ensures raw_data/wikidata/wikidata_diseases.tsv exists (running the
// SPARQL extract if absent) and reads that.
func (w *wikidata) resolveDataPath() (string, error) {
	if config.IsTestMode() {
		return config.Dataconf[w.source]["path"], nil
	}
	rootDir := config.Appconf["rootDir"]
	if rootDir == "" {
		rootDir = "./"
	}
	dataPath := wikidataDataPath
	if !filepath.IsAbs(dataPath) {
		dataPath = filepath.Join(rootDir, dataPath)
	}
	if fileExists(dataPath) {
		return dataPath, nil
	}
	scriptPath := wikidataExtractScript
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(rootDir, scriptPath)
	}
	if !fileExists(scriptPath) {
		return "", fmt.Errorf("extract script not found at %s", scriptPath)
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create output dir: %v", err)
	}
	log.Printf("Wikidata: data file not found, running extraction: python3 %s --output %s", scriptPath, dataPath)
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

// columns: 0 qid, 1 xref (MONDO:xxx or DOID:xxx)
func (w *wikidata) processFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open wikidata file: %v", err)
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 1<<20)

	var count uint64
	seen := map[string]bool{} // qid|xref already linked
	for {
		line, rerr := reader.ReadString('\n')
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line != "" && line[0] != '#' {
			fields := strings.Split(line, "\t")
			if len(fields) >= 2 {
				qid := strings.TrimSpace(fields[0])
				xref := strings.TrimSpace(fields[1])
				if strings.HasPrefix(qid, "Q") && xref != "" && !seen[qid+"|"+xref] {
					seen[qid+"|"+xref] = true
					// keyed FROM the Wikidata item so the reverse (mondo/doid ->
					// wikidata) lands in the hub's from_wikidata bucket.
					if strings.HasPrefix(xref, "MONDO:") {
						w.d.addXref(qid, w.sourceID, xref, "mondo", false)
					} else if strings.HasPrefix(xref, "DOID:") {
						w.d.addXref(qid, w.sourceID, xref, "doid", false)
					} else {
						continue
					}
					if idLogFile != nil {
						logProcessedID(idLogFile, qid)
					}
					count++
					if testLimit > 0 && int(count) >= testLimit {
						break
					}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	return count, nil
}
