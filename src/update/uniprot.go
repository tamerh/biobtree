package update

import (
	"biobtree/src/pbuf"
	"strings"
	"sync/atomic"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type uniprot struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (u *uniprot) processDbReference(entryid string, v *xmlparser.XMLElement, attr *pbuf.XrefAttr) {

	switch v.Attrs["type"] {

	case "EMBL":
		emblID := strings.Split(v.Attrs["id"], ".")[0]
		u.d.addXref(entryid, u.sourceID, emblID, v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "protein sequence ID" {
				targetEmblID := strings.Split(z.Attrs["value"], ".")[0]
				u.d.addXref(emblID, dataconf[v.Attrs["type"]]["id"], targetEmblID, z.Attrs["type"], false)
			} else if _, ok := z.Attrs["type"]; ok && z.Attrs["type"] == "molecule type" {
				attr.Values = nil
				attr.Key = "molecule_type"
				attr.Values = append(attr.Values, z.Attrs["value"])
				u.d.addProp2(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], attr)
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
			case "method", "chains", "resolution":
				attr.Values = nil
				attr.Key = z.Attrs["type"]
				attr.Values = append(attr.Values, z.Attrs["value"])
				u.d.addProp2(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], attr)
			}
		}
	case "DrugBank":
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			switch z.Attrs["type"] {
			case "generic name":
				attr.Values = nil
				attr.Key = "name"
				attr.Values = append(attr.Values, z.Attrs["value"])
				u.d.addProp2(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], attr)
			}
		}
	case "Ensembl":
		// nothing todo for ensembl
	case "Orphanet":
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			switch z.Attrs["type"] {
			case "disease":
				attr.Values = nil
				attr.Key = "disease"
				attr.Values = append(attr.Values, z.Attrs["value"])
				u.d.addProp2(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], attr)
			}
		}
	case "Reactome":
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)
		for _, z := range v.Childs["property"] {
			switch z.Attrs["type"] {
			case "pathway name":
				attr.Values = nil
				attr.Key = "pathway_name"
				attr.Values = append(attr.Values, z.Attrs["value"])
				u.d.addProp2(v.Attrs["id"], dataconf[v.Attrs["type"]]["id"], attr)
			}
		}
	default:
		u.d.addXref(entryid, u.sourceID, v.Attrs["id"], v.Attrs["type"], false)

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

	p := xmlparser.NewXMLParser(br, "entry").SkipElements([]string{"comment", "feature", "sequence"})

	var total uint64
	var v, x, z xmlparser.XMLElement
	var entryid string
	var previous int64
	//var propVal strings.Builder
	attr := pbuf.XrefAttr{}

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

		attr.Values = nil
		attr.Key = "accession"
		for _, v = range r.Childs["accession"] {
			u.d.addXref(v.InnerText, textLinkID, entryid, u.source, true)
			attr.Values = append(attr.Values, v.InnerText)
		}
		if len(attr.Values) > 0 {
			u.d.addProp2(entryid, fr, &attr)
		}

		if trembl && r.Childs["gene"] != nil {
			// for now just for trembl since gene name can come from ensembl but think again
			//if it is not the case all the time otherwise this could be active when there is no ensembl reference
			x = r.Childs["gene"][0]
			attr.Values = nil
			attr.Key = "gene"

			for _, z = range x.Childs["name"] {
				attr.Values = append(attr.Values, z.InnerText)
			}
			if len(attr.Values) > 0 {
				u.d.addProp2(entryid, fr, &attr)
			}
		}

		if r.Childs["protein"] != nil {

			x = r.Childs["protein"][0]

			attr.Values = nil
			attr.Key = "name"
			for _, v = range x.Childs["recommendedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.Values = append(attr.Values, z.InnerText)
				}
			}
			if len(attr.Values) > 0 {
				u.d.addProp2(entryid, fr, &attr)
			}

			attr.Values = nil
			attr.Key = "alternative_name"
			for _, v = range x.Childs["alternativeName"] {
				for _, z = range v.Childs["fullName"] {
					attr.Values = append(attr.Values, z.InnerText)
				}
			}
			if len(attr.Values) > 0 {
				u.d.addProp2(entryid, fr, &attr)
			}

			attr.Values = nil
			attr.Key = "submitted_name"

			for _, v = range x.Childs["submittedName"] {
				for _, z = range v.Childs["fullName"] {
					attr.Values = append(attr.Values, z.InnerText)
				}
			}
			if len(attr.Values) > 0 {
				u.d.addProp2(entryid, fr, &attr)
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
			u.processDbReference(entryid, &ref, &attr)
		}

		// maybe  more info can be added for the literatuere for later searches e.g scope,title, interaction etc
		for _, v = range r.Childs["reference"] {
			for _, z = range v.Childs["citation"] {
				for _, x = range z.Childs["dbReference"] {
					u.d.addXref(entryid, fr, x.Attrs["id"], x.Attrs["type"], false)
				}
			}
		}

		total++

	}

	u.d.progChan <- &progressInfo{dataset: u.source, done: true}

	atomic.AddUint64(&u.d.totalParsedEntry, total)
	u.d.addEntryStat(u.source, total)

}
