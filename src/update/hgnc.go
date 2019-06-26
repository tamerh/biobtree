package update

import (
	"sync/atomic"
	"time"

	"github.com/mailru/easyjson"

	"github.com/tamerh/jsparser"
)

type hgnc struct {
	source string
	d      *DataUpdate
}

func (e *hgnc) update() {

	br, _, ftpFile, localFile, fr, _ := e.d.getDataReaderNew("hgnc", e.d.ebiFtp, e.d.ebiFtpPath, dataconf["hgnc"]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer e.d.wg.Done()

	p := jsparser.NewJSONParser(br, "docs")

	var ok bool
	var v *jsparser.JSON
	var total uint64
	attr := HgncAttr{}

	a := func(jid string, dbid string, j *jsparser.JSON, entryid string) {

		if _, ok = j.ObjectVals[jid]; ok && len(j.ObjectVals[jid].ArrayVals) > 0 {
			for _, v = range j.ObjectVals[jid].ArrayVals {
				e.d.addXref(entryid, fr, v.StringVal, dbid, false)
			}
		} else if _, ok = j.ObjectVals[jid]; ok {
			e.d.addXref(entryid, fr, j.ObjectVals[jid].StringVal, dbid, false)
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

		entryid := j.ObjectVals["hgnc_id"].StringVal
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

			// todo check again most of the symbol and name has single value
			if _, ok = j.ObjectVals["symbol"]; ok && len(j.ObjectVals["symbol"].ArrayVals) > 0 {

				for _, v = range j.ObjectVals["symbol"].ArrayVals {
					e.d.addXref(v.StringVal, textLinkID, entryid, "hgnc", true)
					attr.Symbol = append(attr.Symbol, v.StringVal)
				}

			} else if _, ok = j.ObjectVals["symbol"]; ok && len(j.ObjectVals["symbol"].StringVal) > 0 {
				e.d.addXref(j.ObjectVals["symbol"].StringVal, textLinkID, entryid, "hgnc", true)
				attr.Symbol = append(attr.Symbol, j.ObjectVals["symbol"].StringVal)
			}

			if _, ok = j.ObjectVals["name"]; ok && len(j.ObjectVals["name"].ArrayVals) > 0 {
				for _, v = range j.ObjectVals["name"].ArrayVals {
					e.d.addXref(v.StringVal, textLinkID, entryid, "hgnc", true)
					attr.Name = append(attr.Name, v.StringVal)
				}

			} else if _, ok = j.ObjectVals["name"]; ok && len(j.ObjectVals["name"].StringVal) > 0 {
				e.d.addXref(j.ObjectVals["name"].StringVal, textLinkID, entryid, "hgnc", true)
				attr.Name = append(attr.Name, j.ObjectVals["name"].StringVal)
			}

			if _, ok = j.ObjectVals["locus_group"]; ok && len(j.ObjectVals["locus_group"].StringVal) > 0 {
				attr.LocusGroup = j.ObjectVals["locus_group"].StringVal
			}

			if _, ok = j.ObjectVals["location"]; ok && len(j.ObjectVals["location"].StringVal) > 0 {
				attr.Location = j.ObjectVals["location"].StringVal
			}

			b, _ := easyjson.Marshal(attr)
			e.d.addProp3(entryid, fr, b)

		}

		total++
	}

	e.d.progChan <- &progressInfo{dataset: "hgnc", done: true}

	atomic.AddUint64(&e.d.totalParsedEntry, total)

	e.d.addEntryStat("hgnc", total)

}
