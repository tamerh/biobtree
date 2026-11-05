package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// vega ingests the archived VEGA release 68 (Feb 2017) human gene set.
//
// VEGA gene names include legacy GRCh38 clone names (RP11-, CTB-) that are not
// present in current HGNC, Ensembl or NCBI Gene. The source is frozen, so this
// is a one-time import: one entry per VEGA gene, with its GRCh38 coordinates and
// an xref to the mapped Ensembl gene where one exists.
type vega struct {
	source string
	d      *DataUpdate
}

func (v *vega) check(err error, operation string) {
	checkWithContext(err, v.source, operation)
}

type vegaGene struct {
	GeneID     string
	Name       string
	Chromosome string
	Start      int32
	End        int32
	Strand     string
	Biotype    string
}

func (v *vega) update() {
	defer v.d.wg.Done()

	log.Println("VEGA: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[v.source]["id"]
	path := config.Dataconf[v.source]["path"]
	mapPath := config.Dataconf[v.source]["ensemblMapPath"]

	testLimit := config.GetTestLimit(v.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, v.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("VEGA: [TEST MODE] Processing up to %d entries", testLimit)
	}

	ensemblMap := v.loadEnsemblMap(mapPath)
	log.Printf("VEGA: Loaded %d Ensembl mappings", len(ensemblMap))

	genes, err := v.parseGTF(path)
	if err != nil {
		v.check(err, "parsing VEGA GTF")
		return
	}
	log.Printf("VEGA: Parsed %d genes", len(genes))

	// Deterministic output order.
	ids := make([]string, 0, len(genes))
	for id := range genes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var total uint64
	var previous int64

	for _, id := range ids {
		g := genes[id]

		attr := pbuf.VegaAttr{
			Name:       g.Name,
			Chromosome: g.Chromosome,
			Start:      g.Start,
			End:        g.End,
			Strand:     g.Strand,
			Biotype:    g.Biotype,
			EnsemblId:  ensemblMap[id],
			GeneId:     id,
		}

		b, merr := ffjson.Marshal(&attr)
		if merr != nil {
			log.Printf("VEGA: Error marshaling attributes for %s: %v", id, merr)
			continue
		}
		v.d.addProp3(id, sourceID, b)

		// The clone name is the search key; the entry is the VEGA gene.
		if g.Name != "" {
			v.d.addXrefWithPriority(g.Name, textLinkID, id, v.source, true, "9606")
		}

		// VEGA -> Ensembl where the archived mapping has a target.
		if ensg := ensemblMap[id]; ensg != "" {
			v.d.addXref(id, sourceID, ensg, "ensembl", false)
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, id)
		}

		total++
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
			break
		}

		elapsed := int64(time.Since(startTime).Seconds())
		if elapsed > previous+v.d.progInterval {
			previous = elapsed
			v.d.progChan <- &progressInfo{dataset: v.source, currentKBPerSec: 0}
		}
	}

	v.d.progChan <- &progressInfo{dataset: v.source, done: true}
	atomic.AddUint64(&v.d.totalParsedEntry, total)
	log.Printf("VEGA: Processing complete - %d entries saved (%.2fs)", total, time.Since(startTime).Seconds())
}

// parseGTF aggregates the GTF's exon/CDS features into one record per gene,
// using the min start / max end as the gene span.
func (v *vega) parseGTF(path string) (map[string]*vegaGene, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(v.source, "", "", path)
	if err != nil {
		return nil, err
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	genes := make(map[string]*vegaGene)
	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 9 {
			continue
		}
		attrs := parseGTFAttributes(fields[8])
		geneID := attrs["gene_id"]
		if geneID == "" {
			continue
		}

		start, serr := strconv.Atoi(fields[3])
		end, eerr := strconv.Atoi(fields[4])
		if serr != nil || eerr != nil {
			continue
		}

		g, ok := genes[geneID]
		if !ok {
			name := attrs["gene_name"]
			if name == "" {
				name = geneID
			}
			biotype := attrs["gene_biotype"]
			if biotype == "" {
				biotype = fields[1]
			}
			genes[geneID] = &vegaGene{
				GeneID:     geneID,
				Name:       name,
				Chromosome: fields[0],
				Start:      int32(start),
				End:        int32(end),
				Strand:     fields[6],
				Biotype:    biotype,
			}
			continue
		}
		if int32(start) < g.Start {
			g.Start = int32(start)
		}
		if int32(end) > g.End {
			g.End = int32(end)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return genes, nil
}

// loadEnsemblMap reads the archived VEGA68 -> Ensembl87 stable-ID mapping.
func (v *vega) loadEnsemblMap(path string) map[string]string {
	out := make(map[string]string)
	if path == "" {
		return out
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(v.source, "", "", path)
	if err != nil {
		log.Printf("VEGA: Warning - could not read Ensembl mapping %s: %v", path, err)
		return out
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "vega_id") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		vegaID := strings.TrimSpace(fields[0])
		ensemblID := strings.TrimSpace(fields[1])
		if vegaID != "" && ensemblID != "" {
			out[vegaID] = ensemblID
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("VEGA: Warning - error reading Ensembl mapping %s: %v", path, err)
	}
	return out
}

// parseGTFAttributes parses the GTF attribute column
// (key "value"; key "value"; ...).
func parseGTFAttributes(s string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, " ", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.Trim(strings.TrimSpace(kv[1]), "\"")
		if key != "" {
			out[key] = val
		}
	}
	return out
}
