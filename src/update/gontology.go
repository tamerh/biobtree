package update

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tamerh/xml-stream-parser"
)

type gontology struct {
	source string
	d      *DataUpdate
}

func (g *gontology) update() {

	var br *bufio.Reader
	fr := dataconf[g.source]["id"]

	if _, ok := dataconf[g.source]["useLocalFile"]; ok && dataconf[g.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(dataconf[g.source]["path"]))
		check(err)
		br = bufio.NewReaderSize(file, fileBufSize)
	} else {
		resp, err := http.Get(dataconf[g.source]["path"])
		check(err)
		defer resp.Body.Close()
		br = bufio.NewReader(resp.Body)

		/**
		out, err := os.Create("output.txt")
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		check(err)
		**/

	}
	defer g.d.wg.Done()

	p := xmlparser.NewXMLParser(br, "owl:Class").SkipElements([]string{"owl:Axiom"})

	var total uint64
	var entryid string
	var previous int64
	var propVal strings.Builder

	for r := range p.Stream() {

		elapsed := int64(time.Since(g.d.start).Seconds())
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
					propVal.Reset()
					for _, syn := range r.Childs["oboInOwl:hasExactSynonym"] {
						propVal.WriteString(syn.InnerText)
						propVal.WriteString(propSep)
					}
					g.d.addProp(entryid, fr, "synonym:"+propVal.String()[:len(propVal.String())-1])
				}
				if r.Childs["rdfs:label"] != nil {
					g.d.addProp(entryid, fr, "label:"+r.Childs["rdfs:label"][0].InnerText)
				}

				if r.Childs["oboInOwl:hasOBONamespace"] != nil {
					g.d.addProp(entryid, fr, "type:"+r.Childs["oboInOwl:hasOBONamespace"][0].InnerText)
				}

			}

		}

		total++

	}

	g.d.progChan <- &progressInfo{dataset: "go", done: true}
	atomic.AddUint64(&g.d.totalParsedEntry, total)

	g.d.addEntryStat(g.source, total)

}
