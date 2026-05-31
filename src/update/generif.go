package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
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

type generif struct {
	source string
	d      *DataUpdate
}

func (g *generif) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

func (g *generif) update() {
	defer g.d.wg.Done()

	log.Println("GeneRIF: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[g.source]["id"]
	path := config.Dataconf[g.source]["path"]

	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("GeneRIF: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Open the gzipped TSV (local or HTTP)
	var rawReader *bufio.Reader
	if config.Dataconf[g.source]["useLocalFile"] == "yes" {
		f, err := os.Open(filepath.FromSlash(path))
		g.check(err, "opening local GeneRIF file")
		defer f.Close()
		rawReader = bufio.NewReaderSize(f, fileBufSize)
	} else {
		resp, err := http.Get(path)
		g.check(err, "downloading GeneRIF data")
		defer resp.Body.Close()
		rawReader = bufio.NewReaderSize(resp.Body, fileBufSize)
	}

	gz, err := gzip.NewReader(rawReader)
	g.check(err, "opening gzip stream")
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	const maxCapacity = 512 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	var total uint64
	var entryCount int64
	var previous int64
	var skipped int
	// Disambiguate multiple RIFs that share the same gene+PMID list.
	seen := make(map[string]int)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Tax ID, Gene ID, PubMed ID (PMID) list, last update timestamp, GeneRIF text
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 5 {
			skipped++
			continue
		}
		taxID := strings.TrimSpace(fields[0])
		geneID := strings.TrimSpace(fields[1])
		pmidList := strings.TrimSpace(fields[2])
		timestamp := strings.TrimSpace(fields[3])
		text := strings.TrimSpace(fields[4])

		if geneID == "" || text == "" {
			skipped++
			continue
		}

		elapsed := int64(time.Since(g.d.start).Seconds())
		if elapsed > previous+g.d.progInterval {
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: 0}
		}

		var pmids []string
		for _, p := range strings.Split(pmidList, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				pmids = append(pmids, p)
			}
		}

		// Build a unique key: <gene_id>_<first_pmid>_<n>
		base := geneID + "_" + pmidList
		n := seen[base]
		seen[base] = n + 1
		firstPmid := "0"
		if len(pmids) > 0 {
			firstPmid = pmids[0]
		}
		key := geneID + "_" + firstPmid + "_" + strconv.Itoa(n)

		if idLogFile != nil {
			logProcessedID(idLogFile, key)
		}

		attr := pbuf.GenerifAttr{
			GeneId:    geneID,
			Pmids:     pmids,
			Text:      text,
			Timestamp: timestamp,
			TaxId:     taxID,
		}
		attrBytes, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("GeneRIF: Error marshaling %s: %v", key, err)
			continue
		}
		g.d.addProp3(key, sourceID, attrBytes)

		// Cross-reference to the gene (Entrez) and its citations (PubMed)
		g.d.addXref(key, sourceID, geneID, "entrez", false)
		for _, pmid := range pmids {
			g.d.addXref(key, sourceID, pmid, "pubmed", false)
		}

		total++
		entryCount++
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("GeneRIF: Error reading data: %v", err)
	}

	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
	atomic.AddUint64(&g.d.totalParsedEntry, total)

	log.Printf("GeneRIF: Processing complete - %d entries saved (%.2fs)", total, time.Since(startTime).Seconds())
	if skipped > 0 {
		log.Printf("GeneRIF: Skipped %d malformed/empty rows", skipped)
	}
}
