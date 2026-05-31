package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type depmap struct {
	source string
	d      *DataUpdate
}

func (m *depmap) check(err error, operation string) {
	checkWithContext(err, m.source, operation)
}

type depmapModelInfo struct {
	name    string
	rrid    string
	lineage string
}

// depmapDownloads mirrors the relevant slice of the DepMap download API JSON.
type depmapDownloads struct {
	Table []struct {
		ReleaseName string `json:"releaseName"`
		FileName    string `json:"fileName"`
		DownloadUrl string `json:"downloadUrl"`
	} `json:"table"`
}

// parseDepmapGeneCol splits a CRISPRGeneEffect column header "SYMBOL (ENTREZ)"
// into its symbol and Entrez id. Returns entrez="" if the format doesn't match.
func parseDepmapGeneCol(h string) (sym, entrez string) {
	h = strings.TrimSpace(h)
	i := strings.LastIndex(h, " (")
	if i < 0 || !strings.HasSuffix(h, ")") {
		return h, ""
	}
	return h[:i], h[i+2 : len(h)-1]
}

// resolveURLs queries the DepMap download API and returns the gene-effect and
// model file URLs for the pinned release (prefixing relative paths with host).
func (m *depmap) resolveURLs(apiURL, host, release, geFile, moFile string) (string, string, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	var dl depmapDownloads
	if err := json.Unmarshal(body, &dl); err != nil {
		return "", "", err
	}
	fix := func(u string) string {
		if strings.HasPrefix(u, "/") {
			return host + u
		}
		return u
	}
	var geURL, moURL string
	for _, e := range dl.Table {
		if e.ReleaseName != release {
			continue
		}
		switch e.FileName {
		case geFile:
			geURL = fix(e.DownloadUrl)
		case moFile:
			moURL = fix(e.DownloadUrl)
		}
	}
	return geURL, moURL, nil
}

// ensureFile downloads url to dest unless a non-empty file is already cached.
func (m *depmap) ensureFile(url, dest string) error {
	if fi, err := os.Stat(dest); err == nil && fi.Size() > 0 {
		log.Printf("DepMap: using cached %s", dest)
		return nil
	}
	log.Printf("DepMap: downloading %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func (m *depmap) parseModels(path string) (map[string]depmapModelInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, fileBufSize))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	models := map[string]depmapModelInfo{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		id := get(rec, "ModelID")
		if id == "" {
			continue
		}
		models[id] = depmapModelInfo{
			name:    get(rec, "CellLineName"),
			rrid:    get(rec, "RRID"),
			lineage: get(rec, "OncotreeLineage"),
		}
	}
	return models, nil
}

func (m *depmap) update() {
	defer m.d.wg.Done()

	log.Println("DepMap: Starting data processing...")
	startTime := time.Now()

	depmapID := config.Dataconf[m.source]["id"]
	depDepID := config.Dataconf["depmap_dependency"]["id"]

	release := config.Dataconf[m.source]["depmapRelease"]
	host := config.Dataconf[m.source]["depmapHost"]
	geFile := config.Dataconf[m.source]["geneEffectFile"]
	moFile := config.Dataconf[m.source]["modelFile"]
	threshold := -0.5
	if t, err := strconv.ParseFloat(config.Dataconf[m.source]["dependencyThreshold"], 64); err == nil {
		threshold = t
	}

	testLimit := config.GetTestLimit(m.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, m.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("DepMap: [TEST MODE] Processing up to %d genes", testLimit)
	}

	// Resolve the gene-effect + model files (local override or cached download)
	var gePath, moPath string
	if localDir := config.Dataconf[m.source]["depmapLocalDir"]; localDir != "" {
		gePath = filepath.Join(localDir, geFile)
		moPath = filepath.Join(localDir, moFile)
	} else {
		geURL, moURL, err := m.resolveURLs(config.Dataconf[m.source]["path"], host, release, geFile, moFile)
		m.check(err, "resolving DepMap download URLs")
		if geURL == "" || moURL == "" {
			m.check(io.ErrUnexpectedEOF, "DepMap release '"+release+"' files not found in download API")
		}
		cacheDir := filepath.Join(config.Appconf["outDir"], "depmap_cache")
		os.MkdirAll(cacheDir, 0755)
		gePath = filepath.Join(cacheDir, geFile)
		moPath = filepath.Join(cacheDir, moFile)
		m.check(m.ensureFile(moURL, moPath), "downloading Model.csv")
		m.check(m.ensureFile(geURL, gePath), "downloading CRISPRGeneEffect.csv")
	}

	models, err := m.parseModels(moPath)
	m.check(err, "parsing Model.csv")
	log.Printf("DepMap: loaded %d cell-line models", len(models))

	// Stream the gene-effect matrix (rows = cell lines, cols = "SYMBOL (ENTREZ)")
	f, err := os.Open(gePath)
	m.check(err, "opening CRISPRGeneEffect.csv")
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, fileBufSize))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = true

	header, err := r.Read()
	m.check(err, "reading gene-effect header")

	// Build per-column gene identity (col 0 is the ModelID label)
	nCol := len(header)
	geneSym := make([]string, nCol)
	geneID := make([]string, nCol)
	keep := make([]bool, nCol)
	geneCols := 0
	maxGenes := -1
	if config.IsTestMode() && testLimit > 0 {
		maxGenes = testLimit
	}
	for j := 1; j < nCol; j++ {
		sym, ent := parseDepmapGeneCol(header[j])
		if ent == "" {
			continue
		}
		if maxGenes >= 0 && geneCols >= maxGenes {
			break
		}
		geneSym[j] = sym
		geneID[j] = ent
		keep[j] = true
		geneCols++
	}
	log.Printf("DepMap: %d gene columns", geneCols)

	sum := make([]float64, nCol)
	cnt := make([]int32, nCol)
	dep := make([]int32, nCol)

	var depEdges uint64
	var previous int64
	rows := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		rows++
		modelID := strings.TrimSpace(rec[0])
		mi := models[modelID]

		elapsed := int64(time.Since(m.d.start).Seconds())
		if elapsed > previous+m.d.progInterval {
			previous = elapsed
			m.d.progChan <- &progressInfo{dataset: m.source, currentKBPerSec: 0}
		}

		for j := 1; j < nCol; j++ {
			if !keep[j] || j >= len(rec) {
				continue
			}
			v := strings.TrimSpace(rec[j])
			if v == "" {
				continue
			}
			f64, perr := strconv.ParseFloat(v, 64)
			if perr != nil {
				continue
			}
			cnt[j]++
			sum[j] += f64
			if f64 < threshold {
				dep[j]++
				// Emit a per-cell-line dependency edge
				key := modelID + "_" + geneID[j]
				dattr := pbuf.DepmapDependencyAttr{
					GeneSymbol:      geneSym[j],
					GeneId:          geneID[j],
					ModelId:         modelID,
					CellLineName:    mi.name,
					Rrid:            mi.rrid,
					OncotreeLineage: mi.lineage,
					GeneEffect:      f64,
				}
				if db, merr := ffjson.Marshal(&dattr); merr == nil {
					m.d.addProp3(key, depDepID, db)
					m.d.addXref(key, depDepID, geneID[j], "entrez", false)
					if mi.rrid != "" {
						m.d.addXref(key, depDepID, mi.rrid, "cellosaurus", false)
					}
					depEdges++
				}
			}
		}
	}
	log.Printf("DepMap: matrix parsed (%d cell lines, %d dependency edges)", rows, depEdges)

	// Emit per-gene aggregates
	var genes uint64
	for j := 1; j < nCol; j++ {
		if !keep[j] || cnt[j] == 0 {
			continue
		}
		mean := sum[j] / float64(cnt[j])
		pct := 100.0 * float64(dep[j]) / float64(cnt[j])
		attr := pbuf.DepmapAttr{
			GeneSymbol:        geneSym[j],
			GeneId:            geneID[j],
			MeanGeneEffect:    mean,
			NumLines:          cnt[j],
			NumDependent:      dep[j],
			PctDependent:      pct,
			CommonEssential:   pct >= 90.0,
			StronglySelective: dep[j] > 0 && pct <= 5.0,
		}
		ab, merr := ffjson.Marshal(&attr)
		if merr != nil {
			continue
		}
		m.d.addProp3(geneID[j], depmapID, ab)
		m.d.addXref(geneID[j], depmapID, geneID[j], "entrez", false)
		if geneSym[j] != "" {
			m.d.addXref(geneSym[j], textLinkID, geneID[j], m.source, true)
			m.d.addHumanGeneXrefsAll(geneSym[j], geneID[j], depmapID)
		}
		if idLogFile != nil {
			logProcessedID(idLogFile, geneID[j])
		}
		genes++
	}

	m.d.progChan <- &progressInfo{dataset: "depmap_dependency", done: true}
	m.d.progChan <- &progressInfo{dataset: m.source, done: true}
	atomic.AddUint64(&m.d.totalParsedEntry, genes+depEdges)

	log.Printf("DepMap: complete - %d gene aggregates, %d dependency edges (%.1fs)", genes, depEdges, time.Since(startTime).Seconds())
}
