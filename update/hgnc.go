package update

import (
	"biobtree/pbuf"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	"github.com/tamerh/jsparser"
)

type hgnc struct {
	source string
	d      *DataUpdate
}

func (e *hgnc) update() {

	fr := config.Dataconf["hgnc"]["id"]
	br, _, ftpFile, client, localFile, _ := getDataReaderNew("hgnc", e.d.ebiFtp, e.d.ebiFtpPath, config.Dataconf["hgnc"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer e.d.wg.Done()

	if client != nil {
		defer client.Quit()
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

	for j := range p.Stream() {

		elapsed := int64(time.Since(e.d.start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: "hgnc", currentKBPerSec: kbytesPerSecond}
		}

		entryid := j.ObjectVals["hgnc_id"].(string)
		if len(entryid) > 0 {

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
				// e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.Names = append(attr.Names, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["name"]; ok && len(t.ArrayVals) > 0 {
					for _, v := range t.ArrayVals {
						// e.d.addXref(v.(string), textLinkID, entryid, "hgnc", true)
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
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.LocusGroup = t
			default:
			}

			switch t := j.ObjectVals["locus_type"].(type) {
			case string:
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
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
				e.d.addXref(t, textLinkID, entryid, "hgnc", true)
				attr.GeneGroups = append(attr.GeneGroups, t)
			case (*jsparser.JSON):
				if _, ok = j.ObjectVals["gene_group"]; ok && len(t.ArrayVals) > 0 {
					for _, v := range t.ArrayVals {
						e.d.addXref(v.(string), textLinkID, entryid, "hgnc", true)
						attr.GeneGroups = append(attr.GeneGroups, v.(string))
					}
				}
			default:
			}

			b, _ := ffjson.Marshal(attr)
			e.d.addProp3(entryid, fr, b)

		}

		total++
	}

	e.d.progChan <- &progressInfo{dataset: "hgnc", done: true}

	atomic.AddUint64(&e.d.totalParsedEntry, total)

	e.d.addEntryStat("hgnc", total)

}
