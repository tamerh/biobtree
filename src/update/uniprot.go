package update

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mailru/easyjson"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type uniprot struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (u *uniprot) processDbReference(entryid string, v *xmlparser.XMLElement) {

	switch v.Attrs["type"] {

	case "EMBL":
		emblID := strings.Split(v.Attrs["id"], ".")[0]
		u.d.addXref(entryid, u.sourceID, emblID, v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "protein sequence ID" {
				targetEmblID := strings.Split(z.Attrs["value"], ".")[0]
				u.d.addXref(emblID, dataconf[v.Attrs["type"]]["id"], targetEmblID, z.Attrs["type"], false)
			} else if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "molecule type" {
				attr := CommonAttr{}
				attr.MoleculeType = z.Attrs["value"]
				b, _ := easyjson.Marshal(attr)
				u.d.addProp3(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], b)
			}
		}
	case "RefSeq":
		refseqID := strings.Split(v.Attrs["id"], ".")[0]
		u.d.addXref(entryid, u.sourceID, refseqID, v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "nucleotide sequence ID" {
				targetRefseqID := strings.Split(z.Attrs["value"], ".")[0]
				u.d.addXref(refseqID, dataconf[v.Attrs["type"]]["id"], targetRefseqID, z.Attrs["type"], false)
			}
		}
	case "PDB":
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			switch z.Attrs["type"] {
			case "method":
				attr := CommonAttr{}
				attr.Method = z.Attrs["value"]
				b, _ := easyjson.Marshal(attr)
				u.d.addProp3(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], b)
			case "chains":
				attr := CommonAttr{}
				attr.Chains = z.Attrs["value"]
				b, _ := easyjson.Marshal(attr)
				u.d.addProp3(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], b)
			case "resolution":
				attr := CommonAttr{}
				attr.Resuloution = z.Attrs["value"]
				b, _ := easyjson.Marshal(attr)
				u.d.addProp3(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], b)
			}
		}
	case "DrugBank":
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			switch z.Attrs["type"] {
			case "generic name":
				attr := CommonAttr{}
				attr.Name = z.Attrs["value"]
				b, _ := easyjson.Marshal(attr)
				u.d.addProp3(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], b)
			}
		}
	case "Ensembl":
		// nothing todo for ensembl
	case "Orphanet":
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			switch z.Attrs["type"] {
			case "disease":
				attr := CommonAttr{}
				attr.DiseaseName = z.Attrs["value"]
				b, _ := easyjson.Marshal(attr)
				u.d.addProp3(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], b)
			}
		}
	case "Reactome":
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			switch z.Attrs["type"] {
			case "pathway name":
				attr := CommonAttr{}
				attr.PathwayName = z.Attrs["value"]
				b, _ := easyjson.Marshal(attr)
				u.d.addProp3(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], b)
			}
		}
	default:
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)

	}

}

func (u *uniprot) processSequence(entryid string, r *xmlparser.XMLElement, attr *UniprotAttr) {

	if r.Childs["sequence"] != nil {
		seq := r.Childs["sequence"][0]

		attr.Sequence = UniSequence{}
		seqq := strings.Replace(seq.InnerText, "\n", "", -1)
		attr.Sequence.Seq = seqq

		if _, ok := seq.Attrs["length"]; ok {
			c, err := strconv.Atoi(seq.Attrs["length"])
			if err == nil {
				attr.Sequence.Length = c
			}
		}

		if _, ok := seq.Attrs["mass"]; ok {
			c, err := strconv.Atoi(seq.Attrs["mass"])
			if err == nil {
				attr.Sequence.Mass = c
			}
		}

		if _, ok := seq.Attrs["checksum"]; ok {
			attr.Sequence.Checksum = seq.Attrs["checksum"]
		}

	}
}

func (u *uniprot) processFeatures(entryid string, r *xmlparser.XMLElement, attr *UniprotAttr) {

	evidences := map[string]string{} // for now value is just the evidence id there is also reference to evidence
	for _, e := range r.Childs["evidence"] {
		if _, ok := e.Attrs["key"]; ok {
			if _, ok := e.Attrs["type"]; ok {
				evidences[e.Attrs["key"]] = e.Attrs["type"]
			}
		}
	}

	for _, f := range r.Childs["feature"] {

		feature := UniFeature{}
		if _, ok := f.Attrs["type"]; ok {
			feature.Type = strings.Replace(f.Attrs["type"], " ", "_", -1)
		}

		if _, ok := f.Attrs["description"]; ok {
			feature.Description = f.Attrs["description"]
		}

		if _, ok := f.Attrs["id"]; ok {
			feature.ID = f.Attrs["id"]
		}

		if _, ok := f.Attrs["evidence"]; ok {
			evKeys := strings.Split(f.Attrs["evidence"], " ")
			for _, key := range evKeys {
				if _, ok := evidences[key]; ok {
					feature.Evidences = append(feature.Evidences, evidences[key])
				}
			}
		}

		if f.Childs["original"] != nil {
			if f.Childs["variation"] != nil {
				feature.Original = f.Childs["original"][0].InnerText
				feature.Variatian = f.Childs["variation"][0].InnerText
			}
		}

		if f.Childs["location"] != nil {
			loc := f.Childs["location"][0]

			if loc.Childs["begin"] != nil && loc.Childs["end"] != nil {

				uniloc := UniLocation{}
				if _, ok := loc.Childs["begin"][0].Attrs["position"]; ok {

					c, err := strconv.Atoi(loc.Childs["begin"][0].Attrs["position"])
					if err == nil {
						uniloc.Begin = c
					}

				}

				if _, ok := loc.Childs["end"][0].Attrs["position"]; ok {

					c, err := strconv.Atoi(loc.Childs["end"][0].Attrs["position"])
					if err == nil {
						uniloc.End = c
					}

				}
				feature.Loc = uniloc

			} else if loc.Childs["position"] != nil {

				if _, ok := loc.Childs["position"][0].Attrs["position"]; ok {

					uniloc := UniLocation{}

					c, err := strconv.Atoi(loc.Childs["position"][0].Attrs["position"])
					if err == nil { // same for begin and end
						uniloc.Begin = c
						uniloc.End = c
					}
					feature.Loc = uniloc

				}

			}

		}

	}

}

func (u *uniprot) update() {

	var trembl bool

	if u.source == "uniprot_unreviewed" {
		trembl = true
	}

	br, gz, ftpFile, localFile, fr, _ := u.d.getDataReaderNew(u.source, u.d.uniprotFtp, u.d.uniprotFtpPath, dataconf[u.source]["path"])

	u.sourceID = fr

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer u.d.wg.Done()

	p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"comment"})

	var total uint64
	var v, x, z xmlparser.XMLElement
	var entryid string
	var previous int64

	for r := range p.Stream() {

		elapsed := int64(time.Since(u.d.start).Seconds())
		if elapsed > previous+u.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			u.d.progChan <- &progressInfo{dataset: u.source, currentKBPerSec: kbytesPerSecond}
		}

		if r.Childs["name"] == nil {
			continue
		}

		entryid = r.Childs["name"][0].InnerText

		attr := UniprotAttr{}

		for _, v = range r.Childs["accession"] {
			u.d.addXref(v.InnerText, textLinkID, entryid, u.source, true)
			attr.Accession = append(attr.Accession, v.InnerText)
		}

		if trembl && r.Childs["gene"] != nil {
			// for now just for trembl since gene name can come from ensembl but think again
			//if it is not the case all the time otherwise this could be active when there is no ensembl reference
			x = r.Childs["gene"][0]

			for _, z = range x.Childs["name"] {
				attr.Gene = append(attr.Gene, z.InnerText)
			}

		}

		if r.Childs["protein"] != nil {

			x = r.Childs["protein"][0]

			for _, v = range x.Childs["recommendedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.Name = append(attr.Name, z.InnerText)
				}
			}

			for _, v = range x.Childs["alternativeName"] {
				for _, z = range v.Childs["fullName"] {
					attr.AltName = append(attr.AltName, z.InnerText)
				}
			}

			for _, v = range x.Childs["submittedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.SubName = append(attr.SubName, z.InnerText)
				}
			}

		}

		for _, v = range r.Childs["organism"] {
			for _, z = range v.Childs["dbReference"] {

				u.d.addXref(entryid, fr, z.Attrs["id"], z.Attrs["type"], false)
				for _, x := range z.Childs["property"] {
					u.d.addXref(z.Attrs["id"], dataconf[z.Attrs["type"]]["id"], x.Attrs["value"], x.Attrs["type"], false)
				}
			}
		}

		for _, ref := range r.Childs["dbReference"] {
			u.processDbReference(entryid, &ref)
		}

		// maybe  more info can be added for the literatuere for later searches e.g scope,title, interaction etc
		for _, v = range r.Childs["reference"] {
			for _, z = range v.Childs["citation"] {
				for _, x = range z.Childs["dbReference"] {
					u.d.addXref(entryid, fr, x.Attrs["id"], x.Attrs["type"], false)
				}
			}
		}

		u.processFeatures(entryid, r, &attr)

		u.processSequence(entryid, r, &attr)

		b, _ := easyjson.Marshal(attr)

		u.d.addProp3(entryid, fr, b)

		total++

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}

	atomic.AddUint64(&u.d.totalParsedEntry, total)
	u.d.addEntryStat(u.source, total)

}
