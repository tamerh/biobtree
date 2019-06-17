package update

import (
	"bufio"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/krolaw/zipstream"
	"github.com/tamerh/xml-stream-parser"
)

type hmdb struct {
	source string
	d      *DataUpdate
}

func (h *hmdb) update() {

	defer h.d.wg.Done()

	resp, err := http.Get(dataconf[h.source]["path"])
	check(err)
	defer resp.Body.Close()

	zips := zipstream.NewReader(resp.Body)

	zips.Next()

	br := bufio.NewReaderSize(zips, fileBufSize)

	p := xmlparser.NewXMLParser(br, "metabolite").SkipElements([]string{"taxonomy,ontology"})

	var total uint64
	var v, z xmlparser.XMLElement
	var ok bool
	var entryid string

	var fr = dataconf[h.source]["id"]
	var hmdbdis = dataconf["hmdb disease"]["id"]
	var previous int64

	for r := range p.Stream() {

		elapsed := int64(time.Since(h.d.start).Seconds())
		if elapsed > previous+h.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			h.d.progChan <- &progressInfo{dataset: h.source, currentKBPerSec: kbytesPerSecond}
		}

		entryid = r.Childs["accession"][0].InnerText

		// secondary accs
		for _, v = range r.Childs["secondary_accessions"] {
			for _, z = range v.Childs["accession"] {
				h.d.addXref(entryid, fr, z.InnerText, h.source, false)
			}
		}

		//name
		name := r.Childs["name"][0].InnerText
		h.d.addXref(name, textLinkID, entryid, h.source, true)

		// synonyms
		for _, v = range r.Childs["synonyms"] {
			for _, z = range v.Childs["synonym"] {
				h.d.addXref(z.InnerText, textLinkID, entryid, h.source, true)
			}
		}

		//formula
		if len(r.Childs["chemical_formula"]) > 0 {
			formula := r.Childs["chemical_formula"][0].InnerText
			h.d.addXref(formula, textLinkID, entryid, h.source, false)
		}

		if len(r.Childs["cas_registry_number"]) > 0 {
			cas := r.Childs["cas_registry_number"][0].InnerText
			h.d.addXref(entryid, fr, cas, "CAS", false)
		}

		for _, v := range r.Childs["pathways"] {
			for _, z := range v.Childs["pathway"] {
				for _, x := range z.Childs["kegg_map_id"] {
					if len(x.InnerText) > 0 {
						h.d.addXref(entryid, fr, x.InnerText, "KEGG MAP", false)
					}
				}
			}
		}

		for _, v := range r.Childs["normal_concentrations"] {
			for _, z := range v.Childs["concentration"] {
				for _, x := range z.Childs["references"] {
					for _, t := range x.Childs["reference"] {
						for _, g := range t.Childs["pubmed_id"] {
							if len(g.InnerText) > 0 {
								h.d.addXref(entryid, fr, g.InnerText, "PubMed", false)
							}
						}
					}
				}
			}
		}

		for _, v := range r.Childs["abnormal_concentrations"] {
			for _, z := range v.Childs["concentration"] {
				for _, x := range z.Childs["references"] {
					for _, t := range x.Childs["reference"] {
						for _, g := range t.Childs["pubmed_id"] {
							if len(g.InnerText) > 0 {
								h.d.addXref(entryid, fr, g.InnerText, "PubMed", false)
							}
						}
					}
				}
			}
		}

		// this is use case for graph based approach
		for _, v := range r.Childs["diseases"] {
			for _, z := range v.Childs["disease"] {

				if _, ok = z.Childs["name"]; ok {
					diseaseName := z.Childs["name"][0].InnerText
					h.d.addXref(entryid, fr, diseaseName, "hmdb disease", false)

					if _, ok = z.Childs["omim_id"]; ok {
						for _, x := range z.Childs["omim_id"] {
							if len(x.InnerText) > 0 {
								h.d.addXref(diseaseName, hmdbdis, x.InnerText, "MIM", false)
							}
						}
					}

					//disase pubmed references
					for _, x := range z.Childs["references"] {
						for _, t := range x.Childs["reference"] {
							for _, g := range t.Childs["pubmed_id"] {
								if len(g.InnerText) > 0 {
									h.d.addXref(diseaseName, hmdbdis, g.InnerText, "PubMed", false)
								}
							}
						}
					}

				}
			}
		}

		// rest of xrefs
		for _, v := range r.Childs["drugbank_id"] {
			if len(v.InnerText) > 0 {
				h.d.addXref(entryid, fr, v.InnerText, "DrugBank", false)
			}
		}

		for _, v := range r.Childs["kegg_id"] {
			if len(v.InnerText) > 0 {
				h.d.addXref(entryid, fr, v.InnerText, "KEGG", false)
			}
		}

		for _, v := range r.Childs["biocyc_id"] {
			if len(v.InnerText) > 0 {
				h.d.addXref(entryid, fr, v.InnerText, "BioCyc", false)
			}
		}

		for _, v := range r.Childs["pubchem_compound_id"] {
			if len(v.InnerText) > 0 {
				h.d.addXref(entryid, fr, v.InnerText, "Pubchem", false)
			}
		}

		for _, v := range r.Childs["chebi_id"] {
			if len(v.InnerText) > 0 {
				h.d.addXref(entryid, fr, "CHEBI:"+v.InnerText, "chebi", false)
			}
		}

		for _, x := range r.Childs["general_references"] {
			for _, t := range x.Childs["reference"] {
				for _, g := range t.Childs["pubmed_id"] {
					if len(g.InnerText) > 0 {
						h.d.addXref(entryid, fr, g.InnerText, "PubMed", false)
					}
				}
			}
		}

		// todo in here there is also gene symbol but it also requires graph based transitive feature.
		for _, x := range r.Childs["protein_associations"] {
			for _, t := range x.Childs["protein"] {
				for _, g := range t.Childs["uniprot_id"] {
					if len(g.InnerText) > 0 {
						h.d.addXref(entryid, fr, g.InnerText, "UniProtKB", false)
					}
				}
			}
		}
		total++
	}

	h.d.progChan <- &progressInfo{dataset: h.source, done: true}
	atomic.AddUint64(&h.d.totalParsedEntry, total)

	h.d.addEntryStat(h.source, total)

}
