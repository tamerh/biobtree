package update

import (
	"biobtree/pbuf"
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	xmlparser "github.com/tamerh/xml-stream-parser"
)

type ontology struct {
	source    string
	idPrefix  string
	prefixURL string
	d         *DataUpdate
}

func (g *ontology) update() {

	var br *bufio.Reader
	fr := config.Dataconf[g.source]["id"]
	path := config.Dataconf[g.source]["path"]
	frparentStr := g.source + "parent"
	frchildStr := g.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

	defer g.d.wg.Done()

	var total uint64
	var entryid string
	var previous int64
	var start time.Time

	if config.Dataconf[g.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		check(err)
		br = bufio.NewReaderSize(file, fileBufSize)
	} else {
		resp, err := http.Get(path)
		check(err)
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		defer resp.Body.Close()
	}

	p := xmlparser.NewXMLParser(br, "owl:Class").SkipElements([]string{"owl:Axiom"})

	for r := range p.Stream() {

		previous = 0
		start = time.Now()

		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+g.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: kbytesPerSecond}
		}

		// id
		if r.Childs["oboInOwl:id"] != nil {

			entryid = r.Childs["oboInOwl:id"][0].InnerText

			if len(entryid) > 0 && strings.HasPrefix(entryid, g.idPrefix) {

				// always parent ontology parsed
				if r.Childs["rdfs:subClassOf"] != nil {
					for _, parent := range r.Childs["rdfs:subClassOf"] {
						if _, ok := parent.Attrs["rdf:resource"]; ok {
							id := strings.Trim(parent.Attrs["rdf:resource"], g.prefixURL)
							id = strings.Replace(id, "_", ":", 1)
							if len(id) > 0 && entryid != id && strings.HasPrefix(id, g.idPrefix) {

								g.d.addXref2(entryid, fr, id, frparentStr)
								g.d.addXref2(id, frparent, id, g.source)

								g.d.addXref2(id, fr, entryid, frchildStr)
								g.d.addXref2(entryid, frchild, entryid, g.source)

							}
						} else if parent.Childs["owl:Restriction"] != nil {
							for _, res := range parent.Childs["owl:Restriction"] {
								if res.Childs["owl:someValuesFrom"] != nil {
									for _, someValue := range res.Childs["owl:someValuesFrom"] {
										id := strings.Trim(someValue.Attrs["rdf:resource"], g.prefixURL)
										id = strings.Replace(id, "_", ":", 1)
										if len(id) > 0 && entryid != id && strings.HasPrefix(id, g.idPrefix) {

											g.d.addXref2(entryid, fr, id, frparentStr)
											g.d.addXref2(id, frparent, id, g.source)

											g.d.addXref2(id, fr, entryid, frchildStr)
											g.d.addXref2(entryid, frchild, entryid, g.source)

										}
									}
								}
							}
						}
					}
				}

				if r.Childs["owl:equivalentClass"] != nil && r.Childs["owl:equivalentClass"][0].Childs["owl:Class"] != nil && r.Childs["owl:equivalentClass"][0].Childs["owl:Class"][0].Childs["owl:intersectionOf"] != nil {

					for _, res := range r.Childs["owl:equivalentClass"][0].Childs["owl:Class"][0].Childs["owl:intersectionOf"][0].Childs["owl:Restriction"] {
						if res.Childs["owl:someValuesFrom"] != nil {
							for _, someValue := range res.Childs["owl:someValuesFrom"] {
								id := strings.Trim(someValue.Attrs["rdf:resource"], "http://purl.obolibrary.org/obo/")
								id = strings.Replace(id, "_", ":", 1)
								if len(id) > 0 && entryid != id && strings.HasPrefix(id, g.idPrefix) {

									g.d.addXref2(entryid, fr, id, frparentStr)
									g.d.addXref2(id, frparent, id, g.source)

									g.d.addXref2(id, fr, entryid, frchildStr)
									g.d.addXref2(entryid, frchild, entryid, g.source)

								}
							}
						}
					}
				}

				attr := pbuf.OntologyAttr{}

				if r.Childs["oboInOwl:hasExactSynonym"] != nil {
					for _, syn := range r.Childs["oboInOwl:hasExactSynonym"] {
						attr.Synonyms = append(attr.Synonyms, syn.InnerText)
					}
				}

				if r.Childs["rdfs:label"] != nil {
					attr.Name = r.Childs["rdfs:label"][0].InnerText
				}

				if r.Childs["oboInOwl:hasOBONamespace"] != nil {

					attr.Type = r.Childs["oboInOwl:hasOBONamespace"][0].InnerText
				}

				b, _ := ffjson.Marshal(attr)

				g.d.addProp3(entryid, fr, b)

			}

		}

		total++
	}

	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
	atomic.AddUint64(&g.d.totalParsedEntry, total)

	g.d.addEntryStat(g.source, total)

}
