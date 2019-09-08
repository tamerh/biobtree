package update

import (
	"biobtree/pbuf"
	"bufio"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/krolaw/zipstream"
	"github.com/pquerna/ffjson/ffjson"
	xmlparser "github.com/tamerh/xml-stream-parser"
)

type hmdb struct {
	source string
	d      *DataUpdate
}

func (h *hmdb) getExperimentalProps(r *xmlparser.XMLElement) *pbuf.HmdbExperimentalProps {

	result := pbuf.HmdbExperimentalProps{}

	if r.Childs["experimental_properties"] != nil {
		for _, prop := range r.Childs["experimental_properties"][0].Childs["property"] {
			if prop.Childs["kind"] != nil && prop.Childs["value"] != nil {
				val := prop.Childs["value"][0].InnerText
				switch prop.Childs["kind"][0].InnerText {
				//todo parse these values
				case "water_solubility":
					result.WaterSolubility = val
				case "melting_point":
					result.MeltingPoint = val
				case "boiling_point":
					result.BoolingPoint = val
				case "logp":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.Logp = cc
				}

			}
		}
	}
	return &result
}
func (h *hmdb) getPredictedProps(r *xmlparser.XMLElement) *pbuf.HmdbPredictedProps {

	result := pbuf.HmdbPredictedProps{}

	if r.Childs["predicted_properties"] != nil {
		for _, prop := range r.Childs["predicted_properties"][0].Childs["property"] {
			if prop.Childs["kind"] != nil && prop.Childs["value"] != nil {
				val := prop.Childs["value"][0].InnerText
				switch prop.Childs["kind"][0].InnerText {
				case "rotatable_bond_count":
					cc, err := strconv.ParseInt(strings.TrimSpace(val), 10, 32)
					check(err)
					result.RotatableBondCount = int32(cc)
				case "physiological_charge":
					cc, err := strconv.ParseInt(strings.TrimSpace(val), 10, 32)
					check(err)
					result.PhysiologicalCharge = int32(cc)
				case "rule_of_five":
					result.RuleOfFive = val
				case "pka_strongest_acidic":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.PkaStrongestAcidic = cc
				case "mono_mass":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.PkaStrongestAcidic = cc
				case "ghose_filter":
					result.GhoseFilter = val
				case "refractivity":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.Refractivity = cc
				case "formal_charge":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.FormalCharge = cc
				case "bioavailability":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.FormalCharge = cc
				case "solubility":
					result.Solubility = val
				case "pka_strongest_basic":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.PkaStrongestBasic = cc
				case "polar_surface_area":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.PolarSurfaceArea = cc
				case "veber_rule":
					result.VeberRule = val
				case "mddr_like_rule":
					result.MddrLikeRule = val
				case "logp":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.Logp = cc
				case "polarizability":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.Polarizability = cc
				case "donor_count":
					cc, err := strconv.ParseInt(strings.TrimSpace(val), 10, 32)
					check(err)
					result.DonorCount = int32(cc)
				case "average_mass":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.AverageMass = cc
				case "acceptor_count":
					cc, err := strconv.ParseInt(strings.TrimSpace(val), 10, 32)
					check(err)
					result.AcceptorCount = int32(cc)
				case "number_of_rings":
					cc, err := strconv.ParseInt(strings.TrimSpace(val), 10, 32)
					check(err)
					result.NumberOfRings = int32(cc)
				case "logs":
					cc, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
					check(err)
					result.Logs = cc
				}

			}
		}
	}
	return &result
}

func (h *hmdb) setLocations(attr *pbuf.HmdbAttr, r *xmlparser.XMLElement) {

	for _, v := range r.Childs["cellular_locations"] {
		for _, z := range v.Childs["cellular"] {
			attr.CellularLocations = append(attr.CellularLocations, z.InnerText)
		}
	}

	for _, v := range r.Childs["biospecimen_locations"] {
		for _, z := range v.Childs["biospecimen"] {
			attr.Biospecimens = append(attr.Biospecimens, z.InnerText)
		}
	}

	for _, v := range r.Childs["tissue_locations"] {
		for _, z := range v.Childs["tissue"] {
			attr.TissueLocations = append(attr.TissueLocations, z.InnerText)
		}
	}

}

func (h *hmdb) setBasics(attr *pbuf.HmdbAttr, r *xmlparser.XMLElement) {

}

func (h *hmdb) update() {

	defer h.d.wg.Done()

	resp, err := http.Get(config.Dataconf[h.source]["path"])
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

	var fr = config.Dataconf[h.source]["id"]
	var hmdbdis = config.Dataconf["hmdb disease"]["id"]
	var previous int64

	for r := range p.Stream() {

		elapsed := int64(time.Since(h.d.start).Seconds())
		if elapsed > previous+h.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			h.d.progChan <- &progressInfo{dataset: h.source, currentKBPerSec: kbytesPerSecond}
		}

		if r.Childs["accession"] == nil {
			continue
		}

		attr := pbuf.HmdbAttr{}

		entryid = r.Childs["accession"][0].InnerText

		// secondary accs
		for _, v = range r.Childs["secondary_accessions"] {
			for _, z = range v.Childs["accession"] {
				h.d.addXref(z.InnerText, textLinkID, entryid, h.source, true)
				attr.Accessions = append(attr.Accessions, z.InnerText)
			}
		}

		//name
		name := r.Childs["name"][0].InnerText
		h.d.addXref(name, textLinkID, entryid, h.source, true)
		attr.Name = name

		// description
		if r.Childs["description"] != nil {
			attr.Desc = r.Childs["description"][0].InnerText
		}

		// synonyms
		for _, v = range r.Childs["synonyms"] {
			for _, z = range v.Childs["synonym"] {
				h.d.addXref(z.InnerText, textLinkID, entryid, h.source, true)
				attr.Synonyms = append(attr.Synonyms, z.InnerText)
			}
		}

		//formula
		if len(r.Childs["chemical_formula"]) > 0 {
			h.d.addXref(r.Childs["chemical_formula"][0].InnerText, textLinkID, entryid, h.source, true)
			attr.Formula = r.Childs["chemical_formula"][0].InnerText
		}

		if len(r.Childs["cas_registry_number"]) > 0 {
			cas := r.Childs["cas_registry_number"][0].InnerText
			h.d.addXref(entryid, fr, cas, "CAS", false)
		}

		// moleculer weight
		if len(r.Childs["average_molecular_weight"]) > 0 && len(r.Childs["average_molecular_weight"][0].InnerText) > 0 {
			cc, err := strconv.ParseFloat(r.Childs["average_molecular_weight"][0].InnerText, 64)
			check(err)
			attr.AverageWeight = cc
		}

		// monisotopic weight
		if len(r.Childs["monisotopic_molecular_weight"]) > 0 && len(r.Childs["monisotopic_molecular_weight"][0].InnerText) > 0 {
			cc, err := strconv.ParseFloat(r.Childs["monisotopic_molecular_weight"][0].InnerText, 64)
			check(err)
			attr.MonisotopicWeight = cc
		}

		//smiles
		if len(r.Childs["smiles"]) > 0 {
			h.d.addXref(r.Childs["smiles"][0].InnerText, textLinkID, entryid, h.source, true)
			attr.Smiles = r.Childs["smiles"][0].InnerText
		}

		//inchi
		if len(r.Childs["inchi"]) > 0 {
			h.d.addXref(r.Childs["inchi"][0].InnerText, textLinkID, entryid, h.source, true)
			attr.Inchi = r.Childs["inchi"][0].InnerText
		}

		//inchi key
		if len(r.Childs["inchikey"]) > 0 {
			h.d.addXref(r.Childs["inchikey"][0].InnerText, textLinkID, entryid, h.source, true)
			attr.InchiKey = r.Childs["inchikey"][0].InnerText
		}

		// experimental && predicted properties
		attr.ExperimentalProps = h.getExperimentalProps(r)
		attr.Props = h.getPredictedProps(r)

		// set cell,biospecimen and tissue locations
		h.setLocations(&attr, r)

		for _, v := range r.Childs["pathways"] {
			for _, z := range v.Childs["pathway"] {
				for _, x := range z.Childs["kegg_map_id"] {
					if len(x.InnerText) > 0 {
						h.d.addXref(entryid, fr, x.InnerText, "KEGG MAP", false)
					}
				}
				for _, x := range z.Childs["name"] {
					if len(x.InnerText) > 0 {
						attr.Pathways = append(attr.Pathways, x.InnerText)
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
					// todo lower case
					diseaseName := z.Childs["name"][0].InnerText
					attr.Diseases = append(attr.Diseases, diseaseName)
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

		b, _ := ffjson.Marshal(attr)
		h.d.addProp3(entryid, fr, b)
		total++
	}

	h.d.progChan <- &progressInfo{dataset: h.source, done: true}
	atomic.AddUint64(&h.d.totalParsedEntry, total)

	h.d.addEntryStat(h.source, total)

}
