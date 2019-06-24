package update

import (
	"biobtree/src/pbuf"
	"bufio"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type gontology struct {
	source string
	d      *DataUpdate
}

func (g *gontology) update() {

	var br *bufio.Reader
	fr := dataconf[g.source]["id"]

	var ontologies [2]string

	ontologies[0] = dataconf[g.source]["path"]
	ontologies[1] = dataconf[g.source]["pathEco"]

	defer g.d.wg.Done()

	var total uint64
	var entryid string
	var previous int64
	//var propVal strings.Builder
	var start time.Time

	attr := pbuf.XrefAttr{}

	for _, path := range ontologies {

		resp, err := http.Get(path)
		check(err)
		br = bufio.NewReaderSize(resp.Body, fileBufSize)

		p := xmlparser.NewXMLParser(br, "owl:Class").SkipElements([]string{"owl:Axiom"})

		for r := range p.Stream() {

			previous = 0
			start = time.Now()

			elapsed := int64(time.Since(start).Seconds())
			if elapsed > previous+g.d.progInterval {
				kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
				previous = elapsed
				g.d.progChan <- &progressInfo{dataset: "go", currentKBPerSec: kbytesPerSecond}
			}

			// id
			if r.Childs["oboInOwl:id"] != nil {

				entryid = r.Childs["oboInOwl:id"][0].InnerText

				if len(entryid) > 0 {

					// parent ontology
					if r.Childs["rdfs:subClassOf"] != nil {
						for _, parent := range r.Childs["rdfs:subClassOf"] {
							if _, ok := parent.Attrs["rdf:resource"]; ok {
								id := strings.Trim(parent.Attrs["rdf:resource"], "http://purl.obolibrary.org/obo/")
								id = strings.Replace(id, "_", ":", 1)
								if len(id) > 0 {
									g.d.addXref(entryid, fr, id, "GO", false)
								}
							} else if parent.Childs["owl:Restriction"] != nil {
								for _, res := range parent.Childs["owl:Restriction"] {
									if res.Childs["owl:someValuesFrom"] != nil {
										for _, someValue := range res.Childs["owl:someValuesFrom"] {
											id := strings.Trim(someValue.Attrs["rdf:resource"], "http://purl.obolibrary.org/obo/")
											id = strings.Replace(id, "_", ":", 1)
											if len(id) > 0 {
												g.d.addXref(entryid, fr, id, "GO", false)
											}
										}
									}
								}
							}
						}
					}

					if r.Childs["oboInOwl:hasExactSynonym"] != nil {

						attr.Values = nil
						attr.Key = "synonym"
						for _, syn := range r.Childs["oboInOwl:hasExactSynonym"] {
							attr.Values = append(attr.Values, syn.InnerText)
						}
						g.d.addProp2(entryid, fr, &attr)
					}

					if r.Childs["rdfs:label"] != nil {

						attr.Values = nil
						attr.Key = "name"
						attr.Values = append(attr.Values, r.Childs["rdfs:label"][0].InnerText)
						g.d.addProp2(entryid, fr, &attr)
					}

					if r.Childs["oboInOwl:hasOBONamespace"] != nil {

						attr.Values = nil
						attr.Key = "type"
						attr.Values = append(attr.Values, r.Childs["oboInOwl:hasOBONamespace"][0].InnerText)
						g.d.addProp2(entryid, fr, &attr)

					}

				}

			}

			total++
		}

		resp.Body.Close()
	}

	g.d.progChan <- &progressInfo{dataset: "go", done: true}
	atomic.AddUint64(&g.d.totalParsedEntry, total)

	g.d.addEntryStat(g.source, total)

}
