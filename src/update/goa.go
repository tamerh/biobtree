package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// goa ingests the human GO annotation file (GOA/GOC GAF 2.2) and exposes one
// entry per GO term: the term name, namespace and the set of annotated human
// genes. It reuses MsigdbAttr so the gene-set payload (name/collection/size)
// matches what the enrichment service already understands, but the dataset is
// separate from MSigDB and is not limited to the C5 collections.
type goa struct {
	source string
	d      *DataUpdate
}

func (g *goa) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

type goaTerm struct {
	name  string
	ns    string // BP, MF or CC
	genes map[string]bool
}

// goaGeneResolver maps a GAF gene symbol to its official HGNC id, caching
// results because the same symbols repeat across hundreds of thousands of rows.
type goaGeneResolver struct {
	d      *DataUpdate
	hgncID uint32
	cache  map[string]string
}

func newGoaGeneResolver(d *DataUpdate) *goaGeneResolver {
	r := &goaGeneResolver{d: d, cache: make(map[string]string)}
	if idStr, ok := config.Dataconf["hgnc"]["id"]; ok {
		fmt.Sscanf(idStr, "%d", &r.hgncID)
	}
	return r
}

func (r *goaGeneResolver) resolve(symbol string) (string, bool) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" || r.d.lookupService == nil || r.hgncID == 0 {
		return "", false
	}
	key := strings.ToUpper(symbol)
	if v, ok := r.cache[key]; ok {
		return v, v != ""
	}
	r.cache[key] = "" // unresolved sentinel

	entries, err := r.d.lookupService.LookupAllInDataset(symbol, r.hgncID)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		full, ferr := r.d.lookupFullEntry(entry.Identifier, r.hgncID)
		if ferr != nil || full == nil {
			continue
		}
		a := full.GetHgnc()
		if a == nil {
			continue
		}
		exact := false
		for _, candidate := range a.Symbols {
			if strings.EqualFold(candidate, symbol) {
				exact = true
				break
			}
		}
		if exact {
			r.cache[key] = entry.Identifier
			return entry.Identifier, true
		}
	}
	return "", false
}

func (g *goa) update() {
	defer g.d.wg.Done()

	log.Println("GOA: Starting human GO annotation processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[g.source]["id"]
	path := config.Dataconf[g.source]["path"]

	var goDatasetID uint32
	if idStr, ok := config.Dataconf["go"]["id"]; ok {
		fmt.Sscanf(idStr, "%d", &goDatasetID)
	}

	testLimit := config.GetTestLimit(g.source)
	if config.IsTestMode() {
		log.Printf("GOA: [TEST MODE] processing up to %d annotation rows", testLimit)
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", path)
	g.check(err, "opening GOA GAF")
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)

	terms := make(map[string]*goaTerm)
	uniqueGenes := make(map[string]bool)
	geneResolver := newGoaGeneResolver(g.d)
	termNames := make(map[string]string)

	rows := 0
	skipped := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 13 {
			continue
		}
		// Human only (GAF column 13 is taxon:9606).
		if !strings.Contains(fields[12], "taxon:9606") {
			continue
		}
		// NOT qualifiers describe negative annotations, not membership.
		qualifier := strings.ToUpper(fields[3])
		if strings.Contains(qualifier, "NOT") {
			skipped++
			continue
		}
		goID := strings.TrimSpace(fields[4])
		if !strings.HasPrefix(goID, "GO:") {
			continue
		}
		symbol := strings.TrimSpace(fields[2])
		hgncID, ok := geneResolver.resolve(symbol)
		if !ok {
			skipped++
			continue
		}

		ns := "BP"
		switch strings.ToUpper(strings.TrimSpace(fields[8])) {
		case "F":
			ns = "MF"
		case "C":
			ns = "CC"
		}

		t := terms[goID]
		if t == nil {
			name := termNames[goID]
			if name == "" {
				if full, ferr := g.d.lookupFullEntry(goID, goDatasetID); ferr == nil && full != nil {
					if a := full.GetOntology(); a != nil {
						name = a.Name
					}
				}
				if name == "" {
					name = goID
				}
				termNames[goID] = name
			}
			t = &goaTerm{name: name, ns: ns, genes: make(map[string]bool)}
			terms[goID] = t
		}
		t.genes[hgncID] = true
		uniqueGenes[hgncID] = true
		rows++

		if testLimit > 0 && rows >= testLimit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("GOA: read error: %v", err)
	}

	log.Printf("GOA: %d annotations, %d GO terms, %d unique HGNC genes (skipped %d)",
		rows, len(terms), len(uniqueGenes), skipped)

	// Universe entry lets enrichment use the correct background size.
	universe := &pbuf.MsigdbAttr{
		SystematicName: "GOA:UNIVERSE",
		StandardName:   "GOA human gene universe",
		Collection:     "GOA",
		GeneCount:      int32(len(uniqueGenes)),
		SourceSpecies:  "Hs",
	}
	if b, merr := ffjson.Marshal(universe); merr == nil {
		g.d.addProp3("GOA:UNIVERSE", sourceID, b)
	}

	saved := 0
	for goID, t := range terms {
		attr := &pbuf.MsigdbAttr{
			SystematicName: goID,
			StandardName:   t.name,
			Collection:     "GOA:" + t.ns,
			GeneCount:      int32(len(t.genes)),
			SourceSpecies:  "Hs",
		}
		b, merr := ffjson.Marshal(attr)
		if merr != nil {
			log.Printf("GOA: marshal %s: %v", goID, merr)
			continue
		}
		g.d.addProp3(goID, sourceID, b)
		for hgncID := range t.genes {
			g.d.addXref(goID, sourceID, hgncID, "hgnc", false)
		}
		saved++
	}

	atomic.AddUint64(&g.d.totalParsedEntry, uint64(saved))
	log.Printf("GOA: saved %d GO term entries with human gene sets (%.2fs)", saved, time.Since(startTime).Seconds())

	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}
