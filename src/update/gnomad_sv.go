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

// gnomadSv ingests the gnomAD v4.1 structural-variant sites VCF.
//
// One entry per SV site, keyed by the VCF ID. The VCF carries population
// allele frequencies and PREDICTED_* gene annotations; we keep those and
// link the predicted genes to HGNC. The file is large (~1.2M sites), so it
// lives in the existing `gnomad` federation.
type gnomadSv struct {
	source string
	d      *DataUpdate
}

const gnomadSvIntervalCap = 1_000_000 // index SVs up to 1 Mb; larger CNVs skip bins

func (g *gnomadSv) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

func (g *gnomadSv) update() {
	defer g.d.wg.Done()

	log.Println("gnomAD SV: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[g.source]["id"]
	path := config.Dataconf[g.source]["path"]
	testLimit := config.GetTestLimit(g.source)

	var idLogFile *os.File
	if config.IsTestMode() {
		if p := config.Dataconf[g.source]["testPath"]; p != "" {
			path = p
		}
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("gnomAD SV: [TEST MODE] Processing up to %d sites from %s", testLimit, path)
	}

	var br *bufio.Reader
	var closeFn func()
	if config.IsTestMode() && !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		f, err := os.Open(path)
		g.check(err, "opening gnomAD SV fixture")
		closeFn = func() { f.Close() }
		br = bufio.NewReaderSize(f, 1024*1024)
	} else {
		r, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", path)
		g.check(err, "opening gnomAD SV VCF")
		closeFn = func() { closeReaders(gz, ftpFile, client, localFile) }
		br = r
	}
	defer closeFn()

	resolver := newGnomadSvResolver(g.d, sourceID)
	var count int64
	var previous int64

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if testLimit > 0 && count >= int64(testLimit) {
			break
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 8 {
			continue
		}
		if g.processSite(f, sourceID, resolver, idLogFile) {
			count++
		}

		elapsed := int64(time.Since(startTime).Seconds())
		if elapsed > previous+g.d.progInterval {
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: 0}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("gnomAD SV: %v", err)
	}
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
	atomic.AddUint64(&g.d.totalParsedEntry, uint64(count))
	log.Printf("gnomAD SV: %d sites (%.1fs)", count, time.Since(startTime).Seconds())
}

func (g *gnomadSv) processSite(f []string, sourceID string, resolver *gnomadSvResolver, idLogFile *os.File) bool {
	chr := normalizeChrom(f[0])
	pos, perr := strconv.ParseInt(f[1], 10, 64)
	if perr != nil {
		return false
	}
	info := parseVCFInfo(f[7])
	end := pos
	if s := info["END"]; s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			end = n
		}
	}
	if end < pos {
		end = pos
	}
	svtype := strings.ToUpper(strings.TrimSpace(info["SVTYPE"]))
	if svtype == "" {
		svtype = altType(f[4])
	}
	id := strings.TrimSpace(f[2])
	if id == "" || id == "." {
		id = fmt.Sprintf("%s:%d:%d:%s", chr, pos, end, svtype)
	}
	coordKey := fmt.Sprintf("%s:%d:%d:%s", chr, pos, end, svtype)

	genes, effects, intergenic := gnomadSvPredictions(info)

	attr := &pbuf.GnomadSvAttr{
		Chromosome:       chr,
		Start:            pos,
		End:              end,
		Svtype:           svtype,
		Svlen:            info["SVLEN"],
		Af:               info["AF"],
		Ac:               info["AC"],
		An:               info["AN"],
		GrpmaxAf:         info["GRPMAX_AF"],
		Filters:          f[6],
		CpxType:          info["CPX_TYPE"],
		AfAfr:            info["AF_afr"],
		AfAmr:            info["AF_amr"],
		AfAsj:            info["AF_asj"],
		AfEas:            info["AF_eas"],
		AfFin:            info["AF_fin"],
		AfMid:            info["AF_mid"],
		AfNfe:            info["AF_nfe"],
		AfRmi:            info["AF_rmi"],
		AfSas:            info["AF_sas"],
		PredictedGenes:   genes,
		PredictedEffects: effects,
		Intergenic:       intergenic,
	}
	if len(id) > LMDBMaxKeySize {
		return false
	}
	if b, merr := ffjson.Marshal(attr); merr == nil {
		g.d.addProp3(id, sourceID, b)
	} else {
		log.Printf("gnomAD SV: marshal %s: %v", id, merr)
		return false
	}

	if id != coordKey {
		g.d.addTextLinkWithPriority(coordKey, textLinkID, id, g.source, textTierName)
	}
	if svtype != "BND" && end-pos <= gnomadSvIntervalCap {
		g.d.addInterval(id, g.source, chr, pos, end)
	}
	for _, gene := range genes {
		if hgncID := resolver.resolve(gene); hgncID != "" {
			g.d.addXref(id, sourceID, hgncID, "hgnc", false)
		}
	}
	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}
	return true
}

func gnomadSvPredictions(info map[string]string) ([]string, []string, bool) {
	genes := make(map[string]bool)
	effects := make(map[string]bool)
	intergenic := false
	for k, v := range info {
		if !strings.HasPrefix(k, "PREDICTED_") {
			continue
		}
		if k == "PREDICTED_INTERGENIC" {
			intergenic = true
			continue
		}
		// Nearest TSS is not an affected gene; keep it out of both lists.
		if k == "PREDICTED_NEAREST_TSS" {
			continue
		}
		effect := strings.TrimPrefix(k, "PREDICTED_")
		for _, gene := range strings.Split(v, ",") {
			gene = cleanVCFString(gene)
			if gene == "" {
				continue
			}
			genes[gene] = true
			effects[effect+":"+gene] = true
		}
	}
	return sortedSet(genes), sortedSet(effects), intergenic
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// gnomadSvResolver resolves a predicted gene symbol to one exact HGNC id.
// Text search is alias-broad, so require the official symbol to match.
type gnomadSvResolver struct {
	d      *DataUpdate
	hgncID uint32
	cache  map[string]string
}

func newGnomadSvResolver(d *DataUpdate, _ string) *gnomadSvResolver {
	r := &gnomadSvResolver{d: d, cache: make(map[string]string)}
	if v, ok := config.Dataconf["hgnc"]["id"]; ok {
		fmt.Sscanf(v, "%d", &r.hgncID)
	}
	return r
}

func (r *gnomadSvResolver) resolve(symbol string) string {
	if v, ok := r.cache[symbol]; ok {
		return v
	}
	if r.d.lookupService == nil || r.hgncID == 0 {
		r.cache[symbol] = ""
		return ""
	}
	result, err := r.d.lookup(symbol)
	if err != nil || result == nil {
		r.cache[symbol] = ""
		return ""
	}
	checkEntries := func(entries []*pbuf.XrefEntry) string {
		for _, e := range entries {
			if e == nil || e.Dataset != r.hgncID {
				continue
			}
			full, ferr := r.d.lookupFullEntry(e.Identifier, r.hgncID)
			if ferr != nil || full == nil {
				continue
			}
			a := full.GetHgnc()
			if a == nil {
				continue
			}
			for _, s := range a.Symbols {
				if strings.EqualFold(s, symbol) {
					return e.Identifier
				}
			}
		}
		return ""
	}
	for _, x := range result.Results {
		if x.IsLink {
			if id := checkEntries(x.Entries); id != "" {
				r.cache[symbol] = id
				return id
			}
			if pi, ok := x.DatasetPages[r.hgncID]; ok && pi != nil {
				for _, page := range pi.Pages {
					pageKey := strings.ToUpper(symbol) + pageKeySep + config.DataconfIDToPageKey[0] + page
					pageResult, perr := r.d.lookupPage(pageKey, r.hgncID)
					if perr != nil || pageResult == nil {
						continue
					}
					if id := checkEntries(pageResult.Entries); id != "" {
						r.cache[symbol] = id
						return id
					}
				}
			}
		} else if x.Dataset == r.hgncID {
			if a := x.GetHgnc(); a != nil {
				for _, s := range a.Symbols {
					if strings.EqualFold(s, symbol) {
						r.cache[symbol] = x.Identifier
						return x.Identifier
					}
				}
			}
		}
	}
	r.cache[symbol] = ""
	return ""
}
