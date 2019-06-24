package update

import (
	"biobtree/src/pbuf"
	"sync/atomic"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type interpro struct {
	source string
	d      *DataUpdate
}

func (i *interpro) update() {

	br, gz, ftpFile, localFile, fr, _ := i.d.getDataReaderNew(i.source, i.d.ebiFtp, i.d.ebiFtpPath, dataconf[i.source]["path"])

	if ftpFile != nil {
		defer ftpFile.Close()
	}
	if localFile != nil {
		defer localFile.Close()
	}
	defer gz.Close()
	defer i.d.wg.Done()

	p := xmlparser.NewXMLParser(br, i.source).SkipElements([]string{"abstract", "p"})

	var total uint64
	var entryid string
	var previous int64
	attr := pbuf.XrefAttr{}

	for r := range p.Stream() {

		elapsed := int64(time.Since(i.d.start).Seconds())
		if elapsed > previous+i.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			i.d.progChan <- &progressInfo{dataset: i.source, currentKBPerSec: kbytesPerSecond}
		}

		// id
		entryid = r.Attrs["id"]

		if _, ok := r.Attrs["short_name"]; ok {
			i.d.addXref(r.Attrs["short_name"], textLinkID, entryid, i.source, true)
			attr.Values = nil
			attr.Key = "short_name"
			attr.Values = append(attr.Values, r.Attrs["short_name"])
			i.d.addProp2(entryid, fr, &attr)
		}

		if _, ok := r.Attrs["type"]; ok {
			attr.Values = nil
			attr.Key = "type"
			attr.Values = append(attr.Values, r.Attrs["type"])
			i.d.addProp2(entryid, fr, &attr)
		}

		if _, ok := r.Attrs["protein_count"]; ok {

			attr.Values = nil
			attr.Key = "protein_count"
			attr.Values = append(attr.Values, r.Attrs["protein_count"])
			i.d.addProp2(entryid, fr, &attr)
		}

		attr.Values = nil
		attr.Key = "name"
		for _, v := range r.Childs["name"] {
			attr.Values = append(attr.Values, v.InnerText)
		}
		i.d.addProp2(entryid, fr, &attr)

		for _, x := range r.Childs["pub_list"] {
			for _, y := range x.Childs["publication"] {
				for _, z := range y.Childs["db_xref"] {
					i.d.addXref(entryid, fr, z.Attrs["dbkey"], z.Attrs["db"], false)
				}
			}
		}

		for _, x := range r.Childs["found_in"] {
			for _, y := range x.Childs["rel_ref"] {
				i.d.addXref(entryid, fr, y.Attrs["ipr_ref"], i.source, false)
			}
		}

		for _, x := range r.Childs["member_list"] {
			for _, y := range x.Childs["db_xref"] {
				i.d.addXref(entryid, fr, y.Attrs["dbkey"], y.Attrs["db"], false)
			}
		}

		for _, x := range r.Childs["external_doc_list"] {
			for _, y := range x.Childs["db_xref"] {
				i.d.addXref(entryid, fr, y.Attrs["dbkey"], y.Attrs["db"], false)
			}
		}

		for _, x := range r.Childs["structure_db_links"] {
			for _, y := range x.Childs["db_xref"] {
				i.d.addXref(entryid, fr, y.Attrs["dbkey"], y.Attrs["db"], false)
			}
		}

		/**
		// representativeMember--> dbreference
		for _, v = range r.Elements["pub_list"] {
			if _, ok = v.Childs["publication"]; ok {
				for _, z = range v.Childs["publication"] {
					i.d.addXref(&entryid, fr, &z)
				}
			}
		}
		// member --> dbreference
		for _, v = range r.Elements["member"] {
			if _, ok = v.Childs["dbReference"]; ok {
				for _, z = range v.Childs["dbReference"] {
					i.d.addXref(&entryid, fr, &z)
				}
			}
		}
		**/

		total++

	}

	i.d.progChan <- &progressInfo{dataset: i.source, done: true}
	atomic.AddUint64(&i.d.totalParsedEntry, total)

	i.d.addEntryStat(i.source, total)

}
