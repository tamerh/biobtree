package update

import (
	"biobtree/pbuf"
	"bufio"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	xmlparser "github.com/tamerh/xml-stream-parser"
)

type efo struct {
	source string
	d      *DataUpdate
}

func (e *efo) update() {

	var br *bufio.Reader
	fr := config.Dataconf[e.source]["id"]
	frparent := config.Dataconf["efoparent"]["id"]
	frchild := config.Dataconf["efochild"]["id"]

	defer e.d.wg.Done()

	var total uint64
	var entryid string
	var previous int64
	var start time.Time

	entrystarts := "EFO:"
	resp, err := http.Get(config.Dataconf[e.source]["path"])
	check(err)
	br = bufio.NewReaderSize(resp.Body, fileBufSize)

	p := xmlparser.NewXMLParser(br, "owl:Class").SkipElements([]string{"owl:Axiom"})

	for r := range p.Stream() {

		previous = 0
		start = time.Now()

		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: "efo", currentKBPerSec: kbytesPerSecond}
		}

		// id
		if r.Childs["oboInOwl:id"] != nil {

			entryid = r.Childs["oboInOwl:id"][0].InnerText

			if len(entryid) > 0 && strings.HasPrefix(entryid, entrystarts) {
				entryidcomma := entryid
				entryid = strings.Replace(entryid, ":", "_", -1)

				e.d.addXref(entryidcomma, textLinkID, entryid, e.source, true)

				// always parent ontology parsed
				if r.Childs["rdfs:subClassOf"] != nil {
					for _, parent := range r.Childs["rdfs:subClassOf"] {
						if _, ok := parent.Attrs["rdf:resource"]; ok {
							id := strings.Trim(parent.Attrs["rdf:resource"], "http://www.ebi.ac.uk/efo/")
							id = strings.Replace(id, "_", ":", 1)
							if len(id) > 0 && entryid != id && strings.HasPrefix(id, entrystarts) {

								e.d.addXref2(entryid, fr, id, "efoparent")
								e.d.addXref2(id, frparent, id, "EFO")

								e.d.addXref2(id, fr, entryid, "efochild")
								e.d.addXref2(entryid, frchild, entryid, "EFO")

							}
						} else if parent.Childs["owl:Restriction"] != nil {
							for _, res := range parent.Childs["owl:Restriction"] {
								if res.Childs["owl:onProperty"] != nil {
									for _, onprop := range res.Childs["owl:onProperty"] {
										id := strings.Trim(onprop.Attrs["rdf:resource"], "http://www.ebi.ac.uk/efo/")
										id = strings.Replace(id, "_", ":", 1)
										if len(id) > 0 && entryid != id && strings.HasPrefix(id, entrystarts) {

											e.d.addXref2(entryid, fr, id, "efoparent")
											e.d.addXref2(id, frparent, id, "EFO")

											e.d.addXref2(id, fr, entryid, "efochild")
											e.d.addXref2(entryid, frchild, entryid, "EFO")

										}
									}
								}
							}
						}
					}
				}

				attr := pbuf.EfoAttr{}

				if r.Childs["oboInOwl:hasExactSynonym"] != nil {
					for _, syn := range r.Childs["oboInOwl:hasExactSynonym"] {
						attr.Synonyms = append(attr.Synonyms, syn.InnerText)
					}
				}

				if r.Childs["rdfs:label"] != nil {
					attr.Name = r.Childs["rdfs:label"][0].InnerText
					e.d.addXref(attr.Name, textLinkID, entryid, e.source, true)
				}

				if r.Childs["oboInOwl:hasOBONamespace"] != nil {

					attr.Type = r.Childs["oboInOwl:hasOBONamespace"][0].InnerText
				}

				b, _ := ffjson.Marshal(attr)

				e.d.addProp3(entryid, fr, b)

			}
		}
		total++
	}

	resp.Body.Close()

	e.d.progChan <- &progressInfo{dataset: "efo", done: true}
	atomic.AddUint64(&e.d.totalParsedEntry, total)

	e.d.addEntryStat(e.source, total)

}
