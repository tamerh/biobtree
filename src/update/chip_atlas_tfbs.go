package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// chipAtlasTfbs ingests ChIP-Atlas Target Genes.
//
// ChIP-Atlas publishes one target-gene table per (genome, antigen, TSS window):
//
//	https://chip-atlas.dbcls.jp/data/{genome}/target/{TF}.{distance}.tsv
//
// The table is wide: one row per target gene, one column per ChIP-seq
// experiment (`SRX|cell_type`), plus an `Average` score and a trailing STRING
// column. We keep the gene->gene relationship (TF entry -> target gene) and
// summarise the experiment columns into counts + top targets, rather than
// storing every per-experiment score.
//
// Entry key: CHIPATLAS:{genome}:{TF}:{distance} (distance in kb).
// Targets: HGNC for human, Entrez (filtered by taxid) for other species.
type chipAtlasTfbs struct {
	source string
	d      *DataUpdate
}

// chipAtlasGenomeTaxID maps the ChIP-Atlas assembly name to its NCBI taxid.
var chipAtlasGenomeTaxID = map[string]string{
	"hg38":    "9606",
	"hg19":    "9606",
	"mm10":    "10090",
	"mm9":     "10090",
	"rn6":     "10116",
	"dm6":     "7227",
	"dm3":     "7227",
	"ce11":    "6239",
	"ce10":    "6239",
	"sacCer3": "559292",
	"TAIR12":  "3702",
}

func (c *chipAtlasTfbs) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// chipAtlasResolver resolves a gene symbol to an identifier in the dataset we
// want to link to. Human symbols go to HGNC; other species go to Entrez with a
// taxid filter. Results are cached because the same ~20k symbols recur across
// hundreds of TF tables.
type chipAtlasResolver struct {
	d        *DataUpdate
	hgncID   uint32
	entrezID uint32
	cache    map[string]chipAtlasResolved
}

type chipAtlasResolved struct {
	dataset string
	id      string
	ok      bool
}

func newChipAtlasResolver(d *DataUpdate) *chipAtlasResolver {
	r := &chipAtlasResolver{d: d, cache: make(map[string]chipAtlasResolved)}
	if v, ok := config.Dataconf["hgnc"]["id"]; ok {
		fmt.Sscanf(v, "%d", &r.hgncID)
	}
	if v, ok := config.Dataconf["entrez"]["id"]; ok {
		fmt.Sscanf(v, "%d", &r.entrezID)
	}
	return r
}

func (r *chipAtlasResolver) resolve(symbol, taxID string) (string, string, bool) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return "", "", false
	}
	key := taxID + "|" + symbol
	if v, ok := r.cache[key]; ok {
		return v.dataset, v.id, v.ok
	}

	if taxID == "9606" {
		if r.hgncID != 0 {
			if e, err := r.d.lookupInDataset(symbol, r.hgncID); err == nil && e != nil && e.Identifier != "" {
				r.cache[key] = chipAtlasResolved{dataset: "hgnc", id: e.Identifier, ok: true}
				return "hgnc", e.Identifier, true
			}
		}
		r.cache[key] = chipAtlasResolved{}
		return "", "", false
	}

	// Non-human: resolve by symbol with an Entrez taxid filter. This avoids
	// stapling a mouse target onto the human gene of the same symbol.
	if r.d.lookupService == nil {
		r.cache[key] = chipAtlasResolved{}
		return "", "", false
	}
	query := fmt.Sprintf(">>entrez[entrez.tax_id==\"%s\"]", taxID)
	res, err := r.d.lookupService.MapFilterLite([]string{symbol}, query, "")
	if err == nil && res != nil {
		for _, m := range res.Mappings {
			if len(m.Targets) > 0 {
				id := extractIDFromLiteTarget(m.Targets[0])
				if id != "" {
					r.cache[key] = chipAtlasResolved{dataset: "entrez", id: id, ok: true}
					return "entrez", id, true
				}
			}
		}
	}
	r.cache[key] = chipAtlasResolved{}
	return "", "", false
}

func (c *chipAtlasTfbs) update() {
	defer c.d.wg.Done()

	log.Println("ChIP-Atlas: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[c.source]["id"]
	analysisPath := config.Dataconf[c.source]["path"]
	targetTemplate := config.Dataconf[c.source]["targetPath"]
	genomes := strings.Split(config.Dataconf[c.source]["genomes"], ",")
	distances := config.Dataconf[c.source]["distances"]
	if distances == "" {
		distances = config.Dataconf[c.source]["distance"]
	}
	if distances == "" {
		distances = "1"
	}
	if config.IsTestMode() {
		if td := config.Dataconf[c.source]["test_distances"]; td != "" {
			distances = td
		}
	}
	distanceList := strings.Split(distances, ",")

	testLimit := config.GetTestLimit(c.source)
	testTFLimit := 0
	if config.IsTestMode() {
		if v, ok := config.Dataconf[c.source]["test_tf_limit"]; ok {
			testTFLimit, _ = strconv.Atoi(v)
		}
	}

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// analysisList.tab has no header:
	//   antigen, cell_type_class, recorded_in_target_genes, genome
	byGenome, err := c.readAnalysisList(analysisPath, genomes)
	c.check(err, "reading ChIP-Atlas analysisList")
	if err != nil {
		return
	}

	resolver := newChipAtlasResolver(c.d)

	var totalEntries int64
	var totalTargets int64
	var totalResolved int64
	var previous int64

	for _, genome := range genomes {
		genome = strings.TrimSpace(genome)
		if genome == "" {
			continue
		}
		taxID := chipAtlasGenomeTaxID[genome]
		tfs := byGenome[genome]
		sort.Strings(tfs)

		for _, distance := range distanceList {
			distance = strings.TrimSpace(distance)
			if distance == "" {
				continue
			}
			processed := 0
			for _, tf := range tfs {
				if testTFLimit > 0 && processed >= testTFLimit {
					break
				}
				if testLimit > 0 && int(totalEntries) >= testLimit {
					break
				}

				url := strings.NewReplacer(
					"{genome}", genome,
					"{tf}", tf,
					"{distance}", distance,
				).Replace(targetTemplate)

				attr, targets, err := c.readTargetTable(url, tf, genome, taxID, distance)
				if err != nil {
					log.Printf("ChIP-Atlas: %s %s: %v", genome, tf, err)
					continue
				}
				if attr == nil || len(targets) == 0 {
					continue
				}

				entryID := fmt.Sprintf("CHIPATLAS:%s:%s:%s", genome, tf, distance)
				if b, merr := ffjson.Marshal(attr); merr == nil {
					c.d.addProp3(entryID, sourceID, b)
				} else {
					log.Printf("ChIP-Atlas: marshal %s: %v", entryID, merr)
				}

				// The TF itself: HGNC for human, Entrez for other species, so
				// >>hgnc>>chip_atlas_tfbs / >>entrez>>chip_atlas_tfbs resolve.
				if ds, id, ok := resolver.resolve(tf, taxID); ok {
					c.d.addXref(id, ds, entryID, sourceID, false)
				}

				// Target genes.
				for _, target := range targets {
					ds, id, ok := resolver.resolve(target, taxID)
					if !ok {
						continue
					}
					c.d.addXref(entryID, sourceID, id, ds, false)
					totalResolved++
				}

				totalEntries++
				totalTargets += int64(len(targets))
				processed++

				if idLogFile != nil {
					logProcessedID(idLogFile, entryID)
				}

				elapsed := int64(time.Since(startTime).Seconds())
				if elapsed > previous+int64(c.d.progInterval) {
					previous = elapsed
					c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: 0}
				}
			}
		}
	}

	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
	atomic.AddUint64(&c.d.totalParsedEntry, uint64(totalEntries))
	log.Printf("ChIP-Atlas: %d entries, %d target rows (%d resolved to gene ids) in %.1fs",
		totalEntries, totalTargets, totalResolved, time.Since(startTime).Seconds())
}

// readAnalysisList returns the target-gene antigens per requested genome.
func (c *chipAtlasTfbs) readAnalysisList(path string, genomes []string) (map[string][]string, error) {
	wanted := make(map[string]bool, len(genomes))
	for _, g := range genomes {
		wanted[strings.TrimSpace(g)] = true
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", path)
	if err != nil {
		return nil, err
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	out := make(map[string][]string)
	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		genome := strings.TrimSpace(f[3])
		if !wanted[genome] || strings.TrimSpace(f[2]) != "+" {
			continue
		}
		tf := strings.TrimSpace(f[0])
		if tf != "" {
			out[genome] = append(out[genome], tf)
		}
	}
	return out, scanner.Err()
}

// readTargetTable parses one ChIP-Atlas target table. It returns the entry
// attributes and the list of target gene symbols.
func (c *chipAtlasTfbs) readTargetTable(url, tf, genome, taxID, distance string) (*pbuf.ChipAtlasTfbsAttr, []string, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", url)
	if err != nil {
		return nil, nil, err
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	if !scanner.Scan() {
		return nil, nil, fmt.Errorf("empty target table")
	}
	header := strings.Split(strings.TrimRight(scanner.Text(), "\r"), "\t")
	if len(header) < 2 || header[0] != "Target_genes" {
		return nil, nil, fmt.Errorf("unexpected header %q", header[0])
	}

	// Columns: Target_genes, {TF}|Average, SRX|cell_type..., STRING
	experimentCount := len(header) - 3
	if experimentCount < 0 {
		experimentCount = 0
	}
	cellTypes := make(map[string]bool)
	for i := 2; i < len(header)-1; i++ {
		parts := strings.SplitN(header[i], "|", 2)
		if len(parts) == 2 && parts[1] != "" {
			cellTypes[parts[1]] = true
		}
	}

	type scoredTarget struct {
		name  string
		score float64
	}
	var targets []string
	var scored []scoredTarget

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		target := strings.TrimSpace(f[0])
		if target == "" {
			continue
		}
		targets = append(targets, target)
		score, _ := strconv.ParseFloat(strings.TrimSpace(f[1]), 64)
		scored = append(scored, scoredTarget{name: target, score: score})
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	topN := 20
	if len(scored) < topN {
		topN = len(scored)
	}
	top := make([]string, 0, topN)
	for _, s := range scored[:topN] {
		top = append(top, fmt.Sprintf("%s|%.3f", s.name, s.score))
	}

	cellTypeList := make([]string, 0, len(cellTypes))
	for ct := range cellTypes {
		cellTypeList = append(cellTypeList, ct)
	}
	sort.Strings(cellTypeList)
	if len(cellTypeList) > 50 {
		cellTypeList = cellTypeList[:50]
	}

	distanceKB, _ := strconv.Atoi(distance)
	attr := &pbuf.ChipAtlasTfbsAttr{
		Tf:               tf,
		Genome:           genome,
		TaxId:            taxID,
		DistanceKb:       int32(distanceKB),
		TargetCount:      int32(len(targets)),
		ExperimentCount:  int32(experimentCount),
		CellTypeCount:    int32(len(cellTypes)),
		CellTypes:        cellTypeList,
		TopTargetsSchema: "target|avg_score",
		TopTargets:       top,
	}
	return attr, targets, nil
}
