package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	"github.com/tamerh/jsparser"
)

type hgnc struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for hgnc processor
func (h *hgnc) check(err error, operation string) {
	checkWithContext(err, h.source, operation)
}

func (e *hgnc) update() {

	fr := config.Dataconf["hgnc"]["id"]
	path := config.Dataconf["hgnc"]["path"]

	var br *bufio.Reader
	var httpResp *http.Response
	var localFile *os.File

	defer e.d.wg.Done()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit("hgnc")
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, "hgnc_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Support both local files and HTTP(S) downloads
	if config.Dataconf["hgnc"]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		check(err)
		br = bufio.NewReaderSize(file, fileBufSize)
		localFile = file
		defer localFile.Close()
	} else if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		// HTTP(S) download (like ontology.go)
		resp, err := http.Get(path)
		check(err)
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		httpResp = resp
		defer httpResp.Body.Close()
	} else {
		// Fall back to FTP for backward compatibility
		// Path is now a full URL
		br2, _, ftpFile, client, localFile2, _, err := getDataReaderNew("hgnc", "", "", path)
		check(err)
		br = br2
		if ftpFile != nil {
			defer ftpFile.Close()
		}
		if localFile2 != nil {
			defer localFile2.Close()
		}
		if client != nil {
			defer client.Quit()
		}
	}

	p := jsparser.NewJSONParser(br, "docs")

	var ok bool
	var total uint64
	attr := pbuf.HgncAttr{}

	a := func(jid string, dbid string, j *jsparser.JSON, entryid string) {

		switch t := j.ObjectVals[jid].(type) {
		case string:
			e.d.addXref(entryid, fr, t, dbid, false)
		case (*jsparser.JSON):
			if _, ok = j.ObjectVals[jid]; ok && len(j.ObjectVals[jid].(*jsparser.JSON).ArrayVals) > 0 {
				for _, v := range j.ObjectVals[jid].(*jsparser.JSON).ArrayVals {
					e.d.addXref(entryid, fr, v.(string), dbid, false)
				}
			}
		default:
		}
	}

	var previous int64
	var entryCount int64

	// Accumulate HGNC gene-family membership so we can emit "related gene" edges
	// between co-members of the same family (change #5). HGNC gene_group names are
	// the family label; members sharing a family are functionally related. We
	// accumulate family name -> set(entrez_id) and flush all-pairs edges after the
	// stream completes. Families larger than familyMemberCap are skipped to avoid
	// O(n^2) edge explosion on huge groups (e.g. "Zinc fingers").
	familyMembers := make(map[string]map[string]struct{})

	for j := range p.Stream() {

		elapsed := int64(time.Since(e.d.start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: "hgnc", currentKBPerSec: kbytesPerSecond}
		}

		entryid := j.ObjectVals["hgnc_id"].(string)
		if len(entryid) > 0 {

			// Test mode: log ID
			if idLogFile != nil {
				logProcessedID(idLogFile, entryid)
			}

			a("cosmic", "COSMIC", j, entryid)
			a("omim_id", "MIM", j, entryid)
			a("ena", "EMBL", j, entryid)
			a("ccds_id", "CCDS", j, entryid)
			a("enzyme_id", "Intenz", j, entryid)
			a("vega_id", "VEGA", j, entryid)
			a("ensembl_gene_id", "Ensembl", j, entryid)
			a("pubmed_id", "PubMed", j, entryid)
			a("refseq_accession", "RefSeq", j, entryid)
			a("uniprot_ids", "UniProtKB", j, entryid)
			a("entrez_id", "entrez", j, entryid)
			// Note: STRING uses its own format (9606.ENSP...), not UniProt IDs
			// STRING xrefs are created via UniProt → STRING mapping in string.go
			a("uniprot_ids", "alphafold", j, entryid)

			attr.Reset()

			switch t := j.ObjectVals["symbol"].(type) {
			case string:
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.Symbols = append(attr.Symbols, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["symbol"]; ok && len(t.ArrayVals) > 0 { // this line maybe althogether not necessary
					for _, v := range t.ArrayVals {
						e.d.addXref(v.(string), textLinkID, entryid, "hgnc", true)
						attr.Symbols = append(attr.Symbols, v.(string))
					}
				}
			default:
			}

			switch t := j.ObjectVals["alias_symbol"].(type) {
			case string:
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.Aliases = append(attr.Aliases, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["alias_symbol"]; ok && len(t.ArrayVals) > 0 {
					for _, v := range t.ArrayVals {
						e.d.addXref(v.(string), textLinkID, entryid, "hgnc", true)
						attr.Aliases = append(attr.Aliases, v.(string))
					}
				}
			default:
			}

			switch t := j.ObjectVals["prev_symbol"].(type) {
			case string:
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.PrevSymbols = append(attr.PrevSymbols, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["prev_symbol"]; ok && len(t.ArrayVals) > 0 {
					for _, v := range t.ArrayVals {
						e.d.addXref(v.(string), textLinkID, entryid, "hgnc", true)
						attr.PrevSymbols = append(attr.PrevSymbols, v.(string))
					}
				}
			default:
			}

			switch t := j.ObjectVals["name"].(type) {
			case string:
				// Enable protein name search: "insulin" should find INS gene
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.Names = append(attr.Names, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["name"]; ok && len(t.ArrayVals) > 0 {
					for _, v := range t.ArrayVals {
						e.d.addXref(v.(string), textLinkID, entryid, "hgnc", true)
						attr.Names = append(attr.Names, v.(string))
					}
				}
			default:
			}

			switch t := j.ObjectVals["prev_name"].(type) {
			case string:
				attr.PrevNames = append(attr.PrevNames, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["prev_name"]; ok && len(t.ArrayVals) > 0 {
					for _, v := range t.ArrayVals {
						attr.PrevNames = append(attr.PrevNames, v.(string))
					}
				}
			default:
			}

			switch t := j.ObjectVals["locus_group"].(type) {
			case string:
				// NOTE: Removed from text search - "protein-coding gene" returned 50+ entries
				// Still stored in attr for filtering
				attr.LocusGroup = t
			default:
			}

			switch t := j.ObjectVals["locus_type"].(type) {
			case string:
				// NOTE: Removed from text search - same pollution issue as locus_group
				attr.LocusType = t
			default:
			}

			switch t := j.ObjectVals["location"].(type) {
			case string:
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.Location = t
			default:
			}

			switch t := j.ObjectVals["status"].(type) {
			case string:
				attr.Status = t
			default:
			}

			switch t := j.ObjectVals["gene_group"].(type) {
			case string:
				// NOTE: Removed from text search - "Ring finger proteins" etc. caused pollution
				attr.GeneGroups = append(attr.GeneGroups, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["gene_group"]; ok && len(t.ArrayVals) > 0 {
					for _, v := range t.ArrayVals {
						attr.GeneGroups = append(attr.GeneGroups, v.(string))
					}
				}
			default:
			}

			// Accumulate gene-family membership keyed by entrez_id for relatedentrez
			// family enrichment (flushed after the stream, below). Only this gene's
			// own entrez_id is known here; we cross-link members per family at the end.
			if entrezID := hgncEntrezID(j); entrezID != "" {
				for _, group := range attr.GeneGroups {
					group = strings.TrimSpace(group)
					if group == "" {
						continue
					}
					if familyMembers[group] == nil {
						familyMembers[group] = make(map[string]struct{})
					}
					familyMembers[group][entrezID] = struct{}{}
				}
			}

			b, _ := ffjson.Marshal(attr)
			e.d.addProp3(entryid, fr, b)

		}

		total++
		entryCount++

		// Test mode: check if limit reached
		if config.IsTestMode() && shouldStopProcessing(testLimit, int(entryCount)) {
			e.d.progChan <- &progressInfo{dataset: "hgnc", done: true}
			break
		}
	}

	// Flush HGNC gene-family co-member edges into relatedentrez (change #5).
	// For each family, emit all-pairs bidirectional edges between member entrez ids.
	// Edges are OWNED by hgnc (from = hgnc id) so re-running hgnc cleans them up.
	// relatedentrez is a linkdataset:entrez target, so an entrez-keyed edge written
	// from hgnc is valid and reachable via `entrez >> relatedentrez`.
	e.flushFamilyRelatedEdges(fr, familyMembers)

	e.d.progChan <- &progressInfo{dataset: "hgnc", done: true}

	atomic.AddUint64(&e.d.totalParsedEntry, total)
}

// familyMemberCap is the maximum number of members in an HGNC gene family for
// which we emit co-member relatedentrez edges. Above this, all-pairs growth is
// O(n^2) and dominated by huge generic groups (zinc fingers, olfactory receptors,
// etc.) whose "relatedness" is weak, so we skip them.
const familyMemberCap = 50

// hgncEntrezID extracts a single entrez_id string from an HGNC record.
// HGNC stores entrez_id as a scalar string; arrays are handled defensively
// (first valid element). Returns "" if absent/blank.
func hgncEntrezID(j *jsparser.JSON) string {
	switch t := j.ObjectVals["entrez_id"].(type) {
	case string:
		return strings.TrimSpace(t)
	case (*jsparser.JSON):
		for _, v := range t.ArrayVals {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// flushFamilyRelatedEdges emits bidirectional relatedentrez edges between all
// entrez members of each HGNC gene family, skipping families over familyMemberCap.
func (e *hgnc) flushFamilyRelatedEdges(fr string, familyMembers map[string]map[string]struct{}) {
	var families, edges, skipped uint64
	relID := config.Dataconf["relatedentrez"]["id"]
	for group, members := range familyMembers {
		if len(members) < 2 {
			continue // a single-member family has no co-member edges
		}
		if len(members) > familyMemberCap {
			skipped++
			continue
		}

		// Materialize member ids for stable all-pairs iteration.
		ids := make([]string, 0, len(members))
		for id := range members {
			ids = append(ids, id)
		}

		// Register each member's link-view resolution back to entrez once, so
		// `>>relatedentrez>>entrez` resolves (same fix as orthologentrez).
		for _, id := range ids {
			e.d.addXref2(id, relID, id, "entrez")
		}

		rel := "gene family: " + group
		for i := 0; i < len(ids); i++ {
			for k := i + 1; k < len(ids); k++ {
				// Bidirectional: emit both directions so either gene reaches the other.
				e.d.addXrefWithRelationship(ids[i], fr, ids[k], "relatedentrez", false, rel)
				e.d.addXrefWithRelationship(ids[k], fr, ids[i], "relatedentrez", false, rel)
				edges += 2
			}
		}
		families++
	}
	log.Printf("[HGNC] gene-family relatedentrez enrichment: %d families, %d edges, %d families skipped (>%d members)",
		families, edges, skipped, familyMemberCap)
}
