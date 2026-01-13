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

// bao parses BioAssay Ontology (BAO) OWL files
// BAO uses a different URL format than standard OBO ontologies:
// - Standard: http://purl.obolibrary.org/obo/GO_0000001 → GO:0000001
// - BAO: http://www.bioassayontology.org/bao#BAO_0000001 → BAO:0000001
type bao struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for BAO processor
func (b *bao) check(err error, operation string) {
	checkWithContext(err, b.source, operation)
}

// extractBAOID extracts BAO ID from rdf:about URL
// e.g., "http://www.bioassayontology.org/bao#BAO_0000001" → "BAO:0000001"
func extractBAOID(url string) string {
	// Try splitting on '#' first (BAO format)
	if idx := strings.LastIndex(url, "#"); idx >= 0 {
		id := url[idx+1:]
		// Replace first underscore with colon for consistency
		return strings.Replace(id, "_", ":", 1)
	}
	// Fall back to splitting on '/' (standard OBO format)
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		id := parts[len(parts)-1]
		return strings.Replace(id, "_", ":", 1)
	}
	return ""
}

func (b *bao) update() {
	var br *bufio.Reader
	fr := config.Dataconf[b.source]["id"]
	path := config.Dataconf[b.source]["path"]
	frparentStr := b.source + "parent"
	frchildStr := b.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

	defer b.d.wg.Done()

	var total uint64
	var entryid string
	var previous int64
	var start time.Time

	// Test mode support
	testLimit := config.GetTestLimit(b.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, b.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	if config.Dataconf[b.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		b.check(err, "opening local file")
		br = bufio.NewReaderSize(file, fileBufSize)
	} else {
		resp, err := http.Get(path)
		b.check(err, "fetching URL")
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		defer resp.Body.Close()
	}

	p := xmlparser.NewXMLParser(br, "owl:Class").SkipElements([]string{"owl:Axiom"})

	for r := range p.Stream() {
		previous = 0
		start = time.Now()

		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+b.d.progInterval {
			kbytesPerSecond := int64(p.TotalReadSize) / elapsed / 1024
			previous = elapsed
			b.d.progChan <- &progressInfo{dataset: b.source, currentKBPerSec: kbytesPerSecond}
		}

		// Extract ID from rdf:about attribute
		entryid = ""
		if about, ok := r.Attrs["rdf:about"]; ok {
			entryid = extractBAOID(about)
		}

		// Only process BAO entries
		if len(entryid) > 0 && strings.HasPrefix(entryid, "BAO:") {

			// Parse parent/child relationships from rdfs:subClassOf
			if r.Childs["rdfs:subClassOf"] != nil {
				for _, parent := range r.Childs["rdfs:subClassOf"] {
					if resource, ok := parent.Attrs["rdf:resource"]; ok {
						parentID := extractBAOID(resource)
						if len(parentID) > 0 && entryid != parentID && strings.HasPrefix(parentID, "BAO:") {
							// Add parent relationship
							b.d.addXref2Bucketed(entryid, fr, parentID, frparentStr, fr)
							b.d.addXref2Bucketed(parentID, frparent, parentID, b.source, fr)

							// Add child relationship (reverse)
							b.d.addXref2Bucketed(parentID, fr, entryid, frchildStr, fr)
							b.d.addXref2Bucketed(entryid, frchild, entryid, b.source, fr)
						}
					} else if parent.Childs["owl:Restriction"] != nil {
						// Handle complex subClassOf with owl:Restriction
						for _, res := range parent.Childs["owl:Restriction"] {
							if res.Childs["owl:someValuesFrom"] != nil {
								for _, someValue := range res.Childs["owl:someValuesFrom"] {
									if resource, ok := someValue.Attrs["rdf:resource"]; ok {
										parentID := extractBAOID(resource)
										if len(parentID) > 0 && entryid != parentID && strings.HasPrefix(parentID, "BAO:") {
											b.d.addXref2Bucketed(entryid, fr, parentID, frparentStr, fr)
											b.d.addXref2Bucketed(parentID, frparent, parentID, b.source, fr)

											b.d.addXref2Bucketed(parentID, fr, entryid, frchildStr, fr)
											b.d.addXref2Bucketed(entryid, frchild, entryid, b.source, fr)
										}
									}
								}
							}
						}
					}
				}
			}

			// Build attributes
			attr := pbuf.OntologyAttr{}

			// Get synonyms from obo:IAO_0000118 (BAO uses this for synonyms)
			// Note: Some BAO synonym fields contain newlines separating multiple synonyms
			// We split on newlines and add each as a separate synonym
			if r.Childs["obo:IAO_0000118"] != nil {
				for _, syn := range r.Childs["obo:IAO_0000118"] {
					if syn.InnerText != "" {
						// Split on newlines in case multiple synonyms are in one field
						for _, s := range strings.Split(syn.InnerText, "\n") {
							s = strings.TrimSpace(s)
							if s != "" {
								attr.Synonyms = append(attr.Synonyms, s)
							}
						}
					}
				}
			}

			// Also check oboInOwl:hasExactSynonym for compatibility
			if r.Childs["oboInOwl:hasExactSynonym"] != nil {
				for _, syn := range r.Childs["oboInOwl:hasExactSynonym"] {
					if syn.InnerText != "" {
						// Split on newlines in case multiple synonyms are in one field
						for _, s := range strings.Split(syn.InnerText, "\n") {
							s = strings.TrimSpace(s)
							if s != "" {
								attr.Synonyms = append(attr.Synonyms, s)
							}
						}
					}
				}
			}

			// Get name from rdfs:label
			if r.Childs["rdfs:label"] != nil {
				attr.Name = r.Childs["rdfs:label"][0].InnerText

				// Add text search for name and synonyms
				if attr.Name != "" {
					b.d.addXref(attr.Name, textLinkID, entryid, b.source, true)
				}
				for _, syn := range attr.Synonyms {
					if syn != "" {
						b.d.addXref(syn, textLinkID, entryid, b.source, true)
					}
				}
			}

			// Get type/namespace if available
			if r.Childs["oboInOwl:hasOBONamespace"] != nil {
				attr.Type = r.Childs["oboInOwl:hasOBONamespace"][0].InnerText
			}

			// Serialize and store attributes
			attrBytes, _ := ffjson.Marshal(attr)
			b.d.addProp3Bucketed(entryid, fr, attrBytes)

			// Log ID in test mode
			if idLogFile != nil {
				logProcessedID(idLogFile, entryid)
			}

			total++

			// Check test limit
			if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
				b.d.progChan <- &progressInfo{dataset: b.source, done: true}
				atomic.AddUint64(&b.d.totalParsedEntry, total)
				return
			}
		}
	}

	b.d.progChan <- &progressInfo{dataset: b.source, done: true}
	atomic.AddUint64(&b.d.totalParsedEntry, total)
}
