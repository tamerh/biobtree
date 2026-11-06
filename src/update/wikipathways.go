package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// wikipathways ingests the GMT release of WikiPathways.
//
// The GMT directory has one file per species, named
// wikipathways-YYYYMMDD-gmt-<Species>.gmt. Each line is:
//
//	Pathway name%WikiPathways_YYYYMMDD%WP123%Homo sapiens<TAB>url<TAB>gene...
//
// Genes are Entrez ids. One entry per pathway (WPID); human pathways also get
// direct HGNC edges where Entrez resolves, so hgnc >> wikipathways works.
type wikipathways struct {
	source string
	d      *DataUpdate
}

var wikipathwaysTaxID = map[string]string{
	"Homo sapiens":             "9606",
	"Mus musculus":             "10090",
	"Rattus norvegicus":        "10116",
	"Bos taurus":               "9913",
	"Canis familiaris":         "9615",
	"Gallus gallus":            "9031",
	"Danio rerio":              "7955",
	"Drosophila melanogaster":  "7227",
	"Caenorhabditis elegans":   "6239",
	"Saccharomyces cerevisiae": "559292",
	"Arabidopsis thaliana":     "3702",
	"Anopheles gambiae":        "7165",
	"Equus caballus":           "9796",
	"Pan troglodytes":          "9598",
	"Populus trichocarpa":      "3694",
	"Solanum lycopersicum":     "4081",
	"Sus scrofa":               "9823",
	"Zea mays":                 "4577",
}

var wikipathwaysFileRe = regexp.MustCompile(`wikipathways-([0-9]{8})-gmt-([A-Za-z0-9_]+)\.gmt`)

func (w *wikipathways) check(err error, operation string) {
	checkWithContext(err, w.source, operation)
}

type wpGMTFile struct {
	Species  string // display name, e.g. "Homo sapiens"
	Filename string
	Release  string
}

func (w *wikipathways) update() {
	defer w.d.wg.Done()

	log.Println("WikiPathways: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[w.source]["id"]
	base := config.Dataconf[w.source]["path"]
	testLimit := config.GetTestLimit(w.source)
	testSpeciesLimit := 0
	if config.IsTestMode() {
		if v, ok := config.Dataconf[w.source]["test_species_limit"]; ok {
			testSpeciesLimit, _ = strconv.Atoi(v)
		}
	}

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, w.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	files, err := w.listFiles(base)
	w.check(err, "listing WikiPathways GMT files")
	if err != nil {
		return
	}

	resolver := newWikipathwaysResolver(w.d)
	var totalEntries int64
	var totalGenes int64
	var previous int64

	for _, f := range files {
		if testLimit > 0 && int(totalEntries) >= testLimit {
			break
		}
		url := strings.TrimRight(base, "/") + "/" + f.Filename
		remaining := 0
		if testLimit > 0 {
			remaining = testLimit - int(totalEntries)
		}
		processed, genes, err := w.processFile(url, f, sourceID, testSpeciesLimit, remaining, idLogFile, resolver)
		if err != nil {
			log.Printf("WikiPathways: %s: %v", f.Filename, err)
			continue
		}
		totalEntries += int64(processed)
		totalGenes += genes

		elapsed := int64(time.Since(startTime).Seconds())
		if elapsed > previous+w.d.progInterval {
			previous = elapsed
			w.d.progChan <- &progressInfo{dataset: w.source, currentKBPerSec: 0}
		}
	}

	w.d.progChan <- &progressInfo{dataset: w.source, done: true}
	atomic.AddUint64(&w.d.totalParsedEntry, uint64(totalEntries))
	log.Printf("WikiPathways: %d pathways, %d gene edges (%.1fs)", totalEntries, totalGenes, time.Since(startTime).Seconds())
}

// listFiles reads the GMT directory index and returns the newest file per
// species. The directory contains monthly releases for the last 12 months, so
// choose the lexicographically greatest YYYYMMDD for each species.
func (w *wikipathways) listFiles(base string) ([]wpGMTFile, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(w.source, "", "", base)
	if err != nil {
		return nil, err
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	latest := make(map[string]wpGMTFile)
	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := 0
	for scanner.Scan() {
		lines++
		for _, m := range wikipathwaysFileRe.FindAllStringSubmatch(scanner.Text(), -1) {
			release, speciesFile := m[1], m[2]
			species := strings.ReplaceAll(speciesFile, "_", " ")
			cur, ok := latest[species]
			if !ok || release > cur.Release {
				latest[species] = wpGMTFile{
					Species:  species,
					Filename: m[0],
					Release:  "WikiPathways_" + release,
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	log.Printf("WikiPathways: listing %s: %d lines, %d species files", base, lines, len(latest))

	files := make([]wpGMTFile, 0, len(latest))
	for _, f := range latest {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Species < files[j].Species })
	return files, nil
}

func (w *wikipathways) processFile(url string, f wpGMTFile, sourceID string, speciesLimit, remaining int, idLogFile *os.File, resolver *wikipathwaysResolver) (int, int64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(w.source, "", "", url)
	if err != nil {
		return 0, 0, err
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	taxID := wikipathwaysTaxID[f.Species]
	entries := 0
	var genes int64

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if speciesLimit > 0 && entries >= speciesLimit {
			break
		}
		if remaining > 0 && entries >= remaining {
			break
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		meta := strings.Split(fields[0], "%")
		if len(meta) < 4 {
			continue
		}
		// Parse from the right: name%release%WPID%species. The name itself may
		// contain a percent sign, so join any leading fragments.
		n := len(meta)
		species := strings.TrimSpace(meta[n-1])
		wpid := strings.TrimSpace(meta[n-2])
		release := strings.TrimSpace(meta[n-3])
		name := strings.TrimSpace(strings.Join(meta[:n-3], "%"))
		if species == "" {
			species = f.Species
		}
		if wpid == "" || name == "" {
			continue
		}
		if taxID == "" {
			taxID = wikipathwaysTaxID[species]
		}

		seen := make(map[string]bool)
		var geneIDs []string
		for _, g := range fields[2:] {
			g = strings.TrimSpace(g)
			if g == "" || !isDigits(g) || seen[g] {
				continue
			}
			seen[g] = true
			geneIDs = append(geneIDs, g)
		}

		attr := &pbuf.WikipathwaysAttr{
			Name:      name,
			Species:   species,
			TaxId:     taxID,
			GeneCount: int32(len(geneIDs)),
			Release:   release,
		}
		if b, merr := ffjson.Marshal(attr); merr == nil {
			w.d.addProp3(wpid, sourceID, b)
		} else {
			log.Printf("WikiPathways: marshal %s: %v", wpid, merr)
			continue
		}

		// Search by pathway name. The WPID itself is the primary key, so it
		// must NOT also be a self text-link: that would make Lookup(WPID)
		// return both the entry and a link and duplicate every map result.
		w.d.indexSearchText(w.source, textLinkID, wpid, []string{name}, 3, nil)

		for _, g := range geneIDs {
			// Human pathways use HGNC as the gene namespace so
			// hgnc >> wikipathways resolves directly. Keep Entrez only as a
			// fallback for genes that do not resolve to HGNC. Mixing both
			// namespaces on one entry also duplicates map result groups, so
			// pick one per gene.
			if taxID == "9606" {
				if hgncID := resolver.resolve(g); hgncID != "" {
					w.d.addXref(wpid, sourceID, hgncID, "hgnc", false)
					continue
				}
			}
			w.d.addXref(wpid, sourceID, g, "entrez", false)
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, wpid)
		}
		entries++
		genes += int64(len(geneIDs))
	}
	return entries, genes, scanner.Err()
}

// wikipathwaysResolver caches Entrez -> HGNC lookups. The same human genes
// recur across hundreds of pathways, so the cache keeps this to ~10k queries.
type wikipathwaysResolver struct {
	d     *DataUpdate
	cache map[string]string
}

func newWikipathwaysResolver(d *DataUpdate) *wikipathwaysResolver {
	return &wikipathwaysResolver{d: d, cache: make(map[string]string)}
}

func (r *wikipathwaysResolver) resolve(entrezID string) string {
	if v, ok := r.cache[entrezID]; ok {
		return v
	}
	if r.d.lookupService == nil {
		r.cache[entrezID] = ""
		return ""
	}
	res, err := r.d.lookupService.MapFilterLite([]string{entrezID}, ">>entrez>>hgnc", "")
	if err != nil || res == nil {
		r.cache[entrezID] = ""
		return ""
	}
	for _, m := range res.Mappings {
		if len(m.Targets) > 0 {
			if id := extractIDFromLiteTarget(m.Targets[0]); id != "" {
				r.cache[entrezID] = id
				return id
			}
		}
	}
	r.cache[entrezID] = ""
	return ""
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
