package update

import (
	"biobtree/pbuf"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type uniprot struct {
	source      string
	sourceID    string
	d           *DataUpdate
	trembl      bool
	featureID   string
	ensemblID   string
	ensmeblRefs map[string][]string
}

func (u *uniprot) processDbReference(entryid string, r *xmlparser.XMLElement) {

	for _, v := range r.Childs["dbReference"] {

		switch v.Attrs["type"] {

		case "EMBL":
			emblID := strings.Split(v.Attrs["id"], ".")[0]
			u.d.addXref(entryid, u.sourceID, emblID, v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "protein sequence ID" {
					targetEmblID := strings.Split(z.Attrs["value"], ".")[0]
					u.d.addXref(emblID, config.Dataconf[v.Attrs["type"]]["id"], targetEmblID, z.Attrs["type"], false)
				} else if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "molecule type" {
					attr := pbuf.EnaAttr{}
					attr.Type = strings.ToLower(z.Attrs["value"])
					b, _ := ffjson.Marshal(attr)
					u.d.addProp3(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], b)
				}
			}
		case "RefSeq":
			refseqID := strings.Split(v.Attrs["id"], ".")[0]
			u.d.addXref(entryid, u.sourceID, refseqID, v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "nucleotide sequence ID" {
					targetRefseqID := strings.Split(z.Attrs["value"], ".")[0]
					u.d.addXref(refseqID, config.Dataconf[v.Attrs["type"]]["id"], targetRefseqID, z.Attrs["type"], false)
				}
			}
		case "PDB":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
			attr := pbuf.PdbAttr{}
			for _, z := range v.Childs["property"] {
				switch z.Attrs["type"] {
				case "method":
					attr.Method = strings.ToLower(z.Attrs["value"])
				case "chains":
					attr.Chains = z.Attrs["value"]
				case "resolution":
					attr.Resolution = z.Attrs["value"]
				}
			}
			//todo if empty
			b, _ := ffjson.Marshal(attr)
			u.d.addProp3(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], b)

		case "DrugBank":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				switch z.Attrs["type"] {
				case "generic name":
					attr := pbuf.DrugbankAttr{}
					attr.Name = strings.ToLower(z.Attrs["value"])
					b, _ := ffjson.Marshal(attr)
					u.d.addProp3(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], b)
				}
			}
		case "Ensembl", "EnsemblPlants", "EnsemblBacteria", "EnsemblProtists", "EnsemblMetazoa", "EnsemblFungi":
			// for ensembl it is indexed for swissprot only for now. if ensembl data indexed connection will come from there.
			if !u.trembl {
				for _, z := range v.Childs["property"] {
					if z.Attrs["type"] == "gene ID" {

						u.ensmeblRefs[z.Attrs["value"]] = append(u.ensmeblRefs[z.Attrs["value"]], v.Attrs["id"])

					}
				}
			}

		case "Orphanet":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				switch z.Attrs["type"] {
				case "disease":
					attr := pbuf.OrphanetAttr{}
					attr.Disease = strings.ToLower(z.Attrs["value"])
					b, _ := ffjson.Marshal(attr)
					u.d.addProp3(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], b)
				}
			}
		case "Reactome":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				switch z.Attrs["type"] {
				case "pathway name":
					attr := pbuf.ReactomeAttr{}
					attr.Pathway = z.Attrs["value"]
					b, _ := ffjson.Marshal(attr)
					u.d.addProp3(v.Attrs["id"], config.Dataconf[v.Attrs["type"]]["id"], b)
				}
			}
		case "GO":
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
			for _, z := range v.Childs["property"] {
				switch z.Attrs["type"] {
				case "evidence":
					if strings.HasPrefix(z.Attrs["value"], "ECO:") {
						u.d.addXref(entryid, u.sourceID, z.Attrs["value"], "eco", false)
					}
				}
			}
		default:
			u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		}
	}

	if !u.trembl {

		for k, v := range u.ensmeblRefs {
			u.d.addXref(entryid, u.sourceID, k, "ensembl", false)
			for _, t := range v {
				u.d.addXref(k, u.ensemblID, t, "transcript", false)
			}
		}

		for k := range u.ensmeblRefs {
			delete(u.ensmeblRefs, k)
		}

	}
}

func (u *uniprot) processSequence(entryid string, r *xmlparser.XMLElement, attr *pbuf.UniprotAttr) {

	if r.Childs["sequence"] != nil {
		seq := r.Childs["sequence"][0]

		attr.Sequence = &pbuf.UniSequence{}
		seqq := strings.Replace(seq.InnerText, "\n", "", -1)
		attr.Sequence.Seq = seqq

		if _, ok := seq.Attrs["mass"]; ok {
			c, err := strconv.Atoi(seq.Attrs["mass"])
			if err == nil {
				attr.Sequence.Mass = int32(c)
			}
		}

	}

}

type evidence struct {
	typee    string
	source   string
	sourceID string
}

func (u *uniprot) processFeatures(entryid string, r *xmlparser.XMLElement) {

	evidences := map[string]evidence{} // for now value is just the evidence id there is also reference to evidence
	for _, e := range r.Childs["evidence"] {
		if _, ok := e.Attrs["key"]; ok {
			if _, ok := e.Attrs["type"]; ok {

				ev := evidence{}
				ev.typee = e.Attrs["type"]
				if e.Childs["source"] != nil && e.Childs["source"][0].Childs["dbReference"] != nil {
					if _, ok := e.Childs["source"][0].Childs["dbReference"][0].Attrs["type"]; ok {
						if _, ok := e.Childs["source"][0].Childs["dbReference"][0].Attrs["id"]; ok {
							ev.source = strings.ToLower(e.Childs["source"][0].Childs["dbReference"][0].Attrs["type"])
							ev.sourceID = e.Childs["source"][0].Childs["dbReference"][0].Attrs["id"]
							if ev.source == "uniprotkb" { // this for consistency during query
								ev.source = "uniprot"
							}
						}
					}
				}

				evidences[e.Attrs["key"]] = ev
			}
		}
	}

	for index, f := range r.Childs["feature"] {

		feature := pbuf.UniprotFeatureAttr{}

		// feature id
		fentryid := entryid + "_f" + strconv.Itoa(index)

		if _, ok := f.Attrs["type"]; ok {
			feature.Type = f.Attrs["type"]
		}

		if _, ok := f.Attrs["description"]; ok {
			feature.Description = strings.ToLower(f.Attrs["description"])

			// add variants
			splitted := strings.Split(feature.Description, "dbsnp:")
			if len(splitted) == 2 {
				u.d.addXref(fentryid, u.featureID, splitted[1][:len(splitted[1])-1], "variantid", false)
			}
		}

		if _, ok := f.Attrs["id"]; ok {
			feature.Id = f.Attrs["id"]
		}

		if _, ok := f.Attrs["evidence"]; ok {
			evKeys := strings.Split(f.Attrs["evidence"], " ")
			for _, key := range evKeys {
				if _, ok := evidences[key]; ok {
					feature.Evidences = append(feature.Evidences, &pbuf.UniprotFeatureEvidence{Type: evidences[key].typee, Id: evidences[key].sourceID, Source: evidences[key].source})
					if len(evidences[key].source) > 0 && len(evidences[key].sourceID) > 0 {
						if _, ok := config.Dataconf[evidences[key].source]; ok {
							u.d.addXref(fentryid, u.featureID, evidences[key].sourceID, evidences[key].source, false)
						}
					}
				}
			}
		}

		if f.Childs["original"] != nil {
			if f.Childs["variation"] != nil {
				feature.Original = f.Childs["original"][0].InnerText
				feature.Variation = f.Childs["variation"][0].InnerText
			}
		}

		if f.Childs["location"] != nil {
			loc := f.Childs["location"][0]

			if loc.Childs["begin"] != nil && loc.Childs["end"] != nil {

				uniloc := pbuf.UniLocation{}
				if _, ok := loc.Childs["begin"][0].Attrs["position"]; ok {

					c, err := strconv.Atoi(loc.Childs["begin"][0].Attrs["position"])
					if err == nil {
						uniloc.Begin = int32(c)
					}

				}

				if _, ok := loc.Childs["end"][0].Attrs["position"]; ok {

					c, err := strconv.Atoi(loc.Childs["end"][0].Attrs["position"])
					if err == nil {
						uniloc.End = int32(c)
					}

				}
				feature.Location = &uniloc

			} else if loc.Childs["position"] != nil {

				if _, ok := loc.Childs["position"][0].Attrs["position"]; ok {

					uniloc := pbuf.UniLocation{}

					c, err := strconv.Atoi(loc.Childs["position"][0].Attrs["position"])
					if err == nil { // same for begin and end
						uniloc.Begin = int32(c)
						uniloc.End = int32(c)
					}
					feature.Location = &uniloc

				}

			}

		}

		// feature xref
		u.d.addXref(entryid, u.sourceID, fentryid, "ufeature", false)

		// feature props
		b, _ := ffjson.Marshal(feature)
		u.d.addProp3(fentryid, u.featureID, b)

	}

}

func (u *uniprot) update() {

	var dataPath string

	if u.trembl {
		dataPath = config.Dataconf[u.source]["pathTrembl"]
	} else {
		dataPath = config.Dataconf[u.source]["path"]
		u.ensmeblRefs = map[string][]string{}
	}

	br, gz, ftpFile, client, localFile, _ := getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, dataPath)

	fr := config.Dataconf[u.source]["id"]
	fr2 := config.Dataconf["ufeature"]["id"]
	if len(fr) <= 0 || len(fr2) <= 0 { // todo these shoud check in the conf
		panic("Uniprot or ufeature id is missing")
	}
	u.sourceID = fr
	u.featureID = fr2
	u.ensemblID = config.Dataconf["ensembl"]["id"]

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer u.d.wg.Done()

	if client != nil {
		defer client.Quit()
	}

	p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"comment"})

	var total uint64
	var v, x, z xmlparser.XMLElement
	var entryid string
	var previous int64

	//index := 0

	for r := range p.Stream() {

		elapsed := int64(time.Since(u.d.start).Seconds())
		if elapsed > previous+u.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			u.d.progChan <- &progressInfo{dataset: u.source, currentKBPerSec: kbytesPerSecond}
		}

		if r.Childs["accession"] == nil {
			log.Println("entry skipped due to the loss of accession", r)
			continue
		}
		entryid = r.Childs["accession"][0].InnerText

		attr := pbuf.UniprotAttr{}

		attr.Reviewed = !u.trembl

		for i := 1; i < len(r.Childs["accession"]); i++ {
			v = r.Childs["accession"][i]
			u.d.addXref(v.InnerText, textLinkID, entryid, u.source, true)
			attr.Accessions = append(attr.Accessions, v.InnerText)
		}

		for _, v = range r.Childs["name"] {
			u.d.addXref(v.InnerText, textLinkID, entryid, u.source, true)
		}

		/** test purpose
		if index < 4 {
			u.d.addXref("tpi1", textLinkID, entryid, u.source, true)
			index++
		}
		**/

		/** disabled because gene come from either hgnc or ensembl and uniprot name already contains the name like vav_human
		if r.Childs["gene"] != nil && len(r.Childs["gene"]) > 0 {
			x = r.Childs["gene"][0]
			for _, z = range x.Childs["name"] {
				if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "primary" {
					u.d.addXref(z.InnerText, textLinkID, entryid, u.source, true)
					attr.Genes = append(attr.Genes, z.InnerText)
				}
			}
		}**/

		if r.Childs["protein"] != nil {

			x = r.Childs["protein"][0]

			for _, v = range x.Childs["recommendedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.Names = append(attr.Names, z.InnerText)
				}
			}

			for _, v = range x.Childs["alternativeName"] {
				for _, z = range v.Childs["fullName"] {
					attr.AlternativeNames = append(attr.AlternativeNames, z.InnerText)
				}
			}

			for _, v = range x.Childs["submittedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.SubmittedNames = append(attr.SubmittedNames, z.InnerText)
				}
			}

		}

		for _, v = range r.Childs["organism"] {
			for _, z = range v.Childs["dbReference"] {

				u.d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
				for _, x := range z.Childs["property"] {
					u.d.addXref(z.Attrs["id"], config.Dataconf[z.Attrs["type"]]["id"], x.Attrs["value"], x.Attrs["type"], false)
				}
			}
		}

		u.processDbReference(entryid, r)

		// todo maybe  more info can be added for the literatuere for later searches e.g scope,title, interaction etc
		for _, v = range r.Childs["reference"] {
			for _, z = range v.Childs["citation"] {
				for _, x = range z.Childs["dbReference"] {
					u.d.addXref(entryid, fr, x.Attrs["id"], x.Attrs["type"], false)
				}
			}
		}

		u.processFeatures(entryid, r)

		u.processSequence(entryid, r, &attr)

		b, _ := ffjson.Marshal(attr)

		u.d.addProp3(entryid, fr, b)

		total++

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}

	atomic.AddUint64(&u.d.totalParsedEntry, total)
	u.d.addEntryStat(u.source, total)

}
