package update

import (
	"biobtree/pbuf"
	"bufio"
	"io"
	"log"
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

// check provides context-aware error checking for ontology processor
func (o *ontology) check(err error, operation string) {
	checkWithContext(err, o.source, operation)
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

	// Test mode support
	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

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

		// id - try oboInOwl:id first, then fall back to rdf:about attribute
		entryid = ""
		if r.Childs["oboInOwl:id"] != nil {
			entryid = r.Childs["oboInOwl:id"][0].InnerText
		} else if about, ok := r.Attrs["rdf:about"]; ok {
			// Extract ID from rdf:about URL like "http://purl.obolibrary.org/obo/CL_0000576"
			// Split by "/" and get last part, then replace "_" with ":"
			parts := strings.Split(about, "/")
			if len(parts) > 0 {
				id := parts[len(parts)-1]
				id = strings.Replace(id, "_", ":", 1)
				entryid = id
			}
		}

		if len(entryid) > 0 && strings.HasPrefix(entryid, g.idPrefix) {

			// always parent ontology parsed
			// Use bucketed routing for all ontologies
			if r.Childs["rdfs:subClassOf"] != nil {
				for _, parent := range r.Childs["rdfs:subClassOf"] {
					if _, ok := parent.Attrs["rdf:resource"]; ok {
						id := strings.Trim(parent.Attrs["rdf:resource"], g.prefixURL)
						id = strings.Replace(id, "_", ":", 1)
						if len(id) > 0 && entryid != id && strings.HasPrefix(id, g.idPrefix) {

							g.d.addXref2Bucketed(entryid, fr, id, frparentStr, fr)
							g.d.addXref2Bucketed(id, frparent, id, g.source, fr)

							g.d.addXref2Bucketed(id, fr, entryid, frchildStr, fr)
							g.d.addXref2Bucketed(entryid, frchild, entryid, g.source, fr)

						}
					} else if parent.Childs["owl:Restriction"] != nil {
						for _, res := range parent.Childs["owl:Restriction"] {
							if res.Childs["owl:someValuesFrom"] != nil {
								for _, someValue := range res.Childs["owl:someValuesFrom"] {
									id := strings.Trim(someValue.Attrs["rdf:resource"], g.prefixURL)
									id = strings.Replace(id, "_", ":", 1)
									if len(id) > 0 && entryid != id && strings.HasPrefix(id, g.idPrefix) {

										g.d.addXref2Bucketed(entryid, fr, id, frparentStr, fr)
										g.d.addXref2Bucketed(id, frparent, id, g.source, fr)

										g.d.addXref2Bucketed(id, fr, entryid, frchildStr, fr)
										g.d.addXref2Bucketed(entryid, frchild, entryid, g.source, fr)

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

								g.d.addXref2Bucketed(entryid, fr, id, frparentStr, fr)
								g.d.addXref2Bucketed(id, frparent, id, g.source, fr)

								g.d.addXref2Bucketed(id, fr, entryid, frchildStr, fr)
								g.d.addXref2Bucketed(entryid, frchild, entryid, g.source, fr)

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

				// Add text search for all OWL-based ontologies (GO, ECO, EFO, UBERON, CL,
				// DOID, and the cross-species phenotype ontologies MP/ZP/XPO/WBPhenotype/FYPO).
				// Enables keyword search by term names and synonyms.
				if g.source == "go" || g.source == "eco" || g.source == "efo" || g.source == "uberon" || g.source == "cl" || g.source == "doid" || g.source == "upheno" ||
					g.source == "mp" || g.source == "zp" || g.source == "xpo" || g.source == "wbphenotype" || g.source == "fypo" {
					// Index name + synonyms (full phrase + per-word) via the shared
					// helper, which also adds hyphen-normalized keys so hyphenated
					// terms are found whether or not the query uses the hyphen.
					allPhrases := []string{attr.Name}
					allPhrases = append(allPhrases, attr.Synonyms...)
					g.d.indexSearchText(g.source, textLinkID, entryid, allPhrases, 4, isOntologyStopWord)

					// Pull MONDO synonyms for EFO entries (diseases)
					// This makes EFO entries searchable via MONDO's richer synonym vocabulary
					if g.source == "efo" {
						g.pullMondoSynonyms(entryid)
					}
				}

			}

			if r.Childs["oboInOwl:hasOBONamespace"] != nil {

				attr.Type = r.Childs["oboInOwl:hasOBONamespace"][0].InnerText
			}

			b, _ := ffjson.Marshal(attr)

			// Use bucketed properties for all ontologies
			g.d.addProp3Bucketed(entryid, fr, b)

			// Log ID in test mode
			if idLogFile != nil {
				logProcessedID(idLogFile, entryid)
			}

			total++

			// Check test limit
			if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
				g.processCrossSpeciesSssom()
				g.d.progChan <- &progressInfo{dataset: g.source, done: true}
				atomic.AddUint64(&g.d.totalParsedEntry, total)
				return
			}

		}

	}

	g.processCrossSpeciesSssom()
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
	atomic.AddUint64(&g.d.totalParsedEntry, total)
}

// processCrossSpeciesSssom ingests the uPheno cross-species phenotype SSSOM
// (configured via the dataset's "pathSssom"; only uPheno has it — a no-op for every
// other ontology). It is the hub bridge: every row whose SUBJECT is a UPHENO grouping
// class and whose OBJECT is a species phenotype term in a *registered* dataset becomes a
// direct UPHENO<->species edge (addXref is bidirectional). Cross-species traversal then
// flows through the hub, e.g. >>hpo>>upheno>>mp. Rows to unregistered prefixes are
// skipped (no dangling nodes). Predicate is uniformly semapv:crossSpeciesExactMatch.
func (g *ontology) processCrossSpeciesSssom() {
	pathSssom := config.Dataconf[g.source]["pathSssom"]
	if pathSssom == "" {
		return
	}
	fr := config.Dataconf[g.source]["id"]

	// Species phenotype prefix -> biobtree dataset; keep only loaded datasets so we
	// never create edges into an unconfigured (would-be bare) dataset.
	prefixToDataset := map[string]string{
		"HP": "hpo", "MP": "mp", "ZP": "zp",
		"XPO": "xpo", "WBPhenotype": "wbphenotype", "FYPO": "fypo",
	}
	registered := map[string]string{}
	for prefix, ds := range prefixToDataset {
		if _, ok := config.Dataconf[ds]; ok {
			registered[prefix] = ds
		}
	}
	if len(registered) == 0 {
		return
	}

	resp, err := http.Get(pathSssom)
	if err != nil {
		log.Printf("uPheno: failed to fetch SSSOM %s: %v", pathSssom, err)
		return
	}
	defer resp.Body.Close()

	reader := bufio.NewReaderSize(resp.Body, fileBufSize)
	var edges uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			log.Printf("uPheno SSSOM read error: %v", rerr)
			break
		}
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			// skip '#' metadata block and the column header row
			if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "subject_id\t") {
				f := strings.Split(line, "\t")
				if len(f) >= 4 {
					subject := strings.TrimSpace(f[0]) // UPHENO:xxxx
					object := strings.TrimSpace(f[3])   // species term, e.g. MP:xxxx
					if strings.HasPrefix(subject, g.idPrefix) {
						if ci := strings.IndexByte(object, ':'); ci > 0 {
							if ds, ok := registered[object[:ci]]; ok {
								g.d.addXref(subject, fr, object, ds, false)
								edges++
							}
						}
					}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	log.Printf("uPheno: created %d cross-species phenotype hub edges", edges)
}

// isOntologyStopWord returns true for common ontology terms that should not be indexed alone
func isOntologyStopWord(word string) bool {
	stopWords := map[string]bool{
		// Disease type words
		"disease": true, "disorder": true, "syndrome": true, "condition": true,
		"cancer": true, "carcinoma": true, "neoplasm": true, "tumor": true, "tumour": true,
		"malignant": true, "benign": true, "primary": true, "secondary": true,
		"adenocarcinoma": true, "sarcoma": true, "lymphoma": true, "leukemia": true, "leukaemia": true,
		// Severity/timing words
		"acute": true, "chronic": true, "progressive": true, "recurrent": true,
		"early": true, "late": true, "onset": true,
		"mild": true, "moderate": true, "severe": true,
		// Age-related words
		"adult": true, "childhood": true, "pediatric": true, "paediatric": true,
		"infantile": true, "juvenile": true, "neonatal": true, "congenital": true,
		// Inheritance words
		"hereditary": true, "familial": true, "genetic": true, "inherited": true,
		"autosomal": true, "dominant": true, "recessive": true,
		// Location qualifiers
		"localized": true, "generalized": true, "generalised": true, "systemic": true,
		// Common prepositions/articles
		"with": true, "without": true, "associated": true, "related": true,
		"type": true, "stage": true, "grade": true, "form": true, "variant": true,
		// GO-specific common terms
		"process": true, "activity": true, "function": true, "binding": true,
		"regulation": true, "positive": true, "negative": true,
		"cellular": true, "biological": true, "molecular": true,
		// Anatomy common terms
		"cell": true, "tissue": true, "organ": true, "structure": true,
		// Other common terms
		"susceptibility": true, "modifier": true, "obsolete": true,
	}
	return stopWords[strings.ToLower(word)]
}

// pullMondoSynonyms looks up MONDO entries that link to this EFO ID
// and adds their synonyms as text search terms for the EFO entry
func (g *ontology) pullMondoSynonyms(efoID string) {
	// Check if lookup service is available and MONDO is configured
	if g.d.lookupService == nil {
		return
	}
	if _, exists := config.Dataconf["mondo"]; !exists {
		return
	}

	// Use MapFilter to find MONDO entries that link to this EFO ID
	// Query: efoID >>efo>>mondo - finds MONDO entries via EFO xrefs
	result, err := g.d.lookupService.MapFilter([]string{efoID}, ">>efo>>mondo", "")
	if err != nil {
		log.Printf("EFO pullMondoSynonyms: MapFilter error for %s: %v", efoID, err)
		return
	}
	if result == nil || len(result.Results) == 0 {
		return
	}

	mondoDatasetID := config.DataconfIDStringToInt["mondo"]

	// Process each mapping result
	for _, mapResult := range result.Results {
		// Get MONDO targets from this mapping
		for _, target := range mapResult.Targets {
			if target.Dataset != mondoDatasetID || target.Identifier == "" {
				continue
			}

			mondoID := target.Identifier
			log.Printf("EFO pullMondoSynonyms: found MONDO %s for EFO %s", mondoID, efoID)

			// Extract ontology attributes directly from target (contains synonyms)
			ontologyAttr := target.GetOntology()
			if ontologyAttr == nil {
				// Try getting full entry if attributes not in target
				mondoEntry, err := g.d.lookupFullEntry(mondoID, mondoDatasetID)
				if err != nil || mondoEntry == nil {
					continue
				}
				ontologyAttr = mondoEntry.GetOntology()
				if ontologyAttr == nil {
					continue
				}
			}

			log.Printf("EFO pullMondoSynonyms: MONDO %s has name=%s, %d synonyms", mondoID, ontologyAttr.Name, len(ontologyAttr.Synonyms))

			// Collect all phrases (name + synonyms)
			allPhrases := []string{}
			if ontologyAttr.Name != "" {
				allPhrases = append(allPhrases, ontologyAttr.Name)
			}
			allPhrases = append(allPhrases, ontologyAttr.Synonyms...)

			// Index the MONDO-sourced phrases for this EFO entry (full phrase +
			// per-word + hyphen-normalized keys) via the shared helper.
			g.d.indexSearchText(g.source, textLinkID, efoID, allPhrases, 4, isOntologyStopWord)
		}
	}
}
