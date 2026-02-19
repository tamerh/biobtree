package update

import (
	"biobtree/pbuf"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	xmlparser "github.com/tamerh/xml-stream-parser"
)

type orphanet struct {
	source string
	d      *DataUpdate
}

// orphaEntry holds disorder data aggregated across multiple XML files
type orphaEntry struct {
	attr       *pbuf.OrphanetAttr
	orphaCode  string
	omimIDs    []string
	mondoID    string
	meshIDs    []string
	geneCount  int
	phenoCount int
}

// check provides context-aware error checking for orphanet processor
func (o *orphanet) check(err error, operation string) {
	checkWithContext(err, o.source, operation)
}

// frequencyToValue converts Orphanet frequency string to numeric value
func frequencyToValue(freq string) float64 {
	switch freq {
	case "Obligate (100%)":
		return 1.0
	case "Very frequent (99-80%)":
		return 0.895
	case "Frequent (79-30%)":
		return 0.545
	case "Occasional (29-5%)":
		return 0.17
	case "Very rare (<4-1%)":
		return 0.025
	case "Excluded (0%)":
		return 0.0
	default:
		return -1.0 // Unknown
	}
}

func (o *orphanet) update() {
	defer o.d.wg.Done()

	log.Println("Orphanet: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(o.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, o.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("Orphanet: [TEST MODE] Processing up to %d disorders", testLimit)
	}

	// Map to aggregate data across all phases
	entries := make(map[string]*orphaEntry)

	// Phase 1: Parse product1.xml (disorder info, names, synonyms, cross-refs)
	phase1Count := o.parseProduct1(entries, testLimit, idLogFile)
	log.Printf("Orphanet: Phase 1 complete - %d disorders from product1.xml", phase1Count)

	// Phase 2: Parse product4.xml (HPO phenotype associations)
	phase2Count := o.parseProduct4(entries)
	log.Printf("Orphanet: Phase 2 complete - %d disorders with phenotypes from product4.xml", phase2Count)

	// Phase 3: Parse product6.xml (gene associations)
	phase3Count := o.parseProduct6(entries)
	log.Printf("Orphanet: Phase 3 complete - %d disorders with genes from product6.xml", phase3Count)

	// Save all entries and create cross-references
	o.saveAllEntries(entries)

	log.Printf("Orphanet: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion
	o.d.progChan <- &progressInfo{dataset: o.source, done: true}
	atomic.AddUint64(&o.d.totalParsedEntry, uint64(len(entries)))
}

// parseProduct1 parses en_product1.xml for disorder core data
// XML structure: JDBOR -> DisorderList -> Disorder
func (o *orphanet) parseProduct1(entries map[string]*orphaEntry, testLimit int, idLogFile *os.File) int {
	pathKey := "path_product1"
	filePath := config.Dataconf[o.source][pathKey]
	if filePath == "" {
		log.Printf("Orphanet: No path_product1 configured, skipping")
		return 0
	}

	log.Printf("Orphanet: Phase 1 - Downloading product1.xml from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(o.source, "", "", filePath)
	o.check(err, "opening product1.xml")
	defer closeReaders(gz, ftpFile, client, localFile)

	// Parse Disorder elements
	parser := xmlparser.NewXMLParser(br, "Disorder")

	var processedCount int
	var previous int64

	for disorder := range parser.Stream() {
		// Progress tracking
		elapsed := int64(time.Since(o.d.start).Seconds())
		if elapsed > previous+o.d.progInterval {
			kbytesPerSecond := int64(parser.TotalReadSize) / elapsed / 1024
			previous = elapsed
			o.d.progChan <- &progressInfo{dataset: o.source, currentKBPerSec: kbytesPerSecond}
		}

		// Extract OrphaCode
		orphaCode := getXMLChildText(disorder, "OrphaCode")
		if orphaCode == "" {
			continue
		}

		// Test mode: log ID and check limit
		if idLogFile != nil {
			idLogFile.WriteString(orphaCode + "\n")
		}

		processedCount++
		if testLimit > 0 && processedCount > testLimit {
			log.Printf("Orphanet: [TEST MODE] Reached limit of %d disorders, stopping phase 1", testLimit)
			break
		}

		// Create entry
		entry := &orphaEntry{
			orphaCode: orphaCode,
			attr: &pbuf.OrphanetAttr{
				Phenotypes: []*pbuf.OrphanetAttr_PhenotypeAssociation{},
			},
		}

		// Name
		if nameNode := disorder.Childs["Name"]; nameNode != nil && len(nameNode) > 0 {
			entry.attr.Name = nameNode[0].InnerText
		}

		// Disorder type
		if typeNode := disorder.Childs["DisorderType"]; typeNode != nil && len(typeNode) > 0 {
			if typeName := typeNode[0].Childs["Name"]; typeName != nil && len(typeName) > 0 {
				entry.attr.DisorderType = typeName[0].InnerText
			}
		}

		// Synonyms
		if synList := disorder.Childs["SynonymList"]; synList != nil && len(synList) > 0 {
			if synonyms := synList[0].Childs["Synonym"]; synonyms != nil {
				for _, syn := range synonyms {
					if syn.InnerText != "" {
						entry.attr.Synonyms = append(entry.attr.Synonyms, syn.InnerText)
					}
				}
			}
		}

		// Definition/summary (optional in some entries)
		if summaryNode := disorder.Childs["SummaryInformation"]; summaryNode != nil && len(summaryNode) > 0 {
			if textSec := summaryNode[0].Childs["TextSection"]; textSec != nil && len(textSec) > 0 {
				if contents := textSec[0].Childs["Contents"]; contents != nil && len(contents) > 0 {
					entry.attr.Definition = contents[0].InnerText
				}
			}
		}

		// External references (OMIM, MONDO, MeSH)
		if extRefList := disorder.Childs["ExternalReferenceList"]; extRefList != nil && len(extRefList) > 0 {
			if extRefs := extRefList[0].Childs["ExternalReference"]; extRefs != nil {
				for i := range extRefs {
					extRef := &extRefs[i]
					source := getXMLChildText(extRef, "Source")
					ref := getXMLChildText(extRef, "Reference")
					if ref == "" {
						continue
					}
					switch source {
					case "OMIM":
						entry.omimIDs = append(entry.omimIDs, ref)
					case "MONDO":
						if entry.mondoID == "" {
							entry.mondoID = ref
						}
					case "MeSH":
						entry.meshIDs = append(entry.meshIDs, ref)
					}
				}
			}
		}

		entries[orphaCode] = entry
	}

	return processedCount
}

// parseProduct4 parses en_product4.xml for HPO phenotype associations
// XML structure: JDBOR -> HPODisorderSetStatusList -> HPODisorderSetStatus -> Disorder
func (o *orphanet) parseProduct4(entries map[string]*orphaEntry) int {
	pathKey := "path_product4"
	filePath := config.Dataconf[o.source][pathKey]
	if filePath == "" {
		log.Printf("Orphanet: No path_product4 configured, skipping")
		return 0
	}

	log.Printf("Orphanet: Phase 2 - Downloading product4.xml from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(o.source, "", "", filePath)
	o.check(err, "opening product4.xml")
	defer closeReaders(gz, ftpFile, client, localFile)

	// Parse HPODisorderSetStatus elements
	parser := xmlparser.NewXMLParser(br, "HPODisorderSetStatus")

	updatedCount := 0

	for hpoSet := range parser.Stream() {
		// Get Disorder element
		disorderList := hpoSet.Childs["Disorder"]
		if disorderList == nil || len(disorderList) == 0 {
			continue
		}
		disorder := &disorderList[0]

		orphaCode := getXMLChildText(disorder, "OrphaCode")
		if orphaCode == "" {
			continue
		}

		entry, exists := entries[orphaCode]
		if !exists {
			// Entry not in phase 1 (might be filtered by test mode)
			continue
		}

		// Parse HPO associations
		if assocList := disorder.Childs["HPODisorderAssociationList"]; assocList != nil && len(assocList) > 0 {
			if assocs := assocList[0].Childs["HPODisorderAssociation"]; assocs != nil {
				for i := range assocs {
					assoc := &assocs[i]
					// HPO element
					hpoNode := assoc.Childs["HPO"]
					if hpoNode == nil || len(hpoNode) == 0 {
						continue
					}
					hpo := &hpoNode[0]

					hpoID := getXMLChildText(hpo, "HPOId")
					hpoTerm := getXMLChildText(hpo, "HPOTerm")

					if hpoID == "" {
						continue
					}

					// Frequency
					freqStr := ""
					freqVal := -1.0
					if freqNode := assoc.Childs["HPOFrequency"]; freqNode != nil && len(freqNode) > 0 {
						if freqName := freqNode[0].Childs["Name"]; freqName != nil && len(freqName) > 0 {
							freqStr = freqName[0].InnerText
							freqVal = frequencyToValue(freqStr)
						}
					}

					pa := &pbuf.OrphanetAttr_PhenotypeAssociation{
						HpoId:          hpoID,
						HpoTerm:        hpoTerm,
						Frequency:      freqStr,
						FrequencyValue: freqVal,
					}
					entry.attr.Phenotypes = append(entry.attr.Phenotypes, pa)
					entry.phenoCount++
				}
			}
		}

		if entry.phenoCount > 0 {
			updatedCount++
		}
	}

	return updatedCount
}

// parseProduct6 parses en_product6.xml for gene associations
// XML structure: JDBOR -> DisorderList -> Disorder
func (o *orphanet) parseProduct6(entries map[string]*orphaEntry) int {
	pathKey := "path_product6"
	filePath := config.Dataconf[o.source][pathKey]
	if filePath == "" {
		log.Printf("Orphanet: No path_product6 configured, skipping")
		return 0
	}

	log.Printf("Orphanet: Phase 3 - Downloading product6.xml from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(o.source, "", "", filePath)
	o.check(err, "opening product6.xml")
	defer closeReaders(gz, ftpFile, client, localFile)

	// Parse Disorder elements (different structure from product1)
	parser := xmlparser.NewXMLParser(br, "Disorder")

	sourceID := config.Dataconf[o.source]["id"]
	updatedCount := 0

	for disorder := range parser.Stream() {
		orphaCode := getXMLChildText(disorder, "OrphaCode")
		if orphaCode == "" {
			continue
		}

		entry, exists := entries[orphaCode]
		if !exists {
			continue
		}

		// Parse gene associations
		if geneAssocList := disorder.Childs["DisorderGeneAssociationList"]; geneAssocList != nil && len(geneAssocList) > 0 {
			if geneAssocs := geneAssocList[0].Childs["DisorderGeneAssociation"]; geneAssocs != nil {
				for j := range geneAssocs {
					geneAssoc := &geneAssocs[j]
					// Gene element
					geneNode := geneAssoc.Childs["Gene"]
					if geneNode == nil || len(geneNode) == 0 {
						continue
					}
					gene := &geneNode[0]

					// Association type for evidence
					assocType := ""
					if typeNode := geneAssoc.Childs["DisorderGeneAssociationType"]; typeNode != nil && len(typeNode) > 0 {
						if typeName := typeNode[0].Childs["Name"]; typeName != nil && len(typeName) > 0 {
							assocType = typeName[0].InnerText
						}
					}

					// External references (Ensembl, HGNC)
					if extRefList := gene.Childs["ExternalReferenceList"]; extRefList != nil && len(extRefList) > 0 {
						if extRefs := extRefList[0].Childs["ExternalReference"]; extRefs != nil {
							for k := range extRefs {
								extRef := &extRefs[k]
								source := getXMLChildText(extRef, "Source")
								ref := getXMLChildText(extRef, "Reference")
								if ref == "" {
									continue
								}

								switch source {
								case "Ensembl":
									// Create cross-reference to Ensembl gene
									o.d.addXrefWithEvidence(orphaCode, sourceID, ref, "ensembl", false, assocType)
								case "HGNC":
									// Create cross-reference to HGNC
									hgncID := ref
									if !strings.HasPrefix(hgncID, "HGNC:") {
										hgncID = "HGNC:" + ref
									}
									o.d.addXrefWithEvidence(orphaCode, sourceID, hgncID, "hgnc", false, assocType)
								}
							}
						}
					}

					entry.geneCount++
				}
			}
		}

		if entry.geneCount > 0 {
			updatedCount++
		}
	}

	return updatedCount
}

// saveAllEntries marshals and saves all entries, creating cross-references
func (o *orphanet) saveAllEntries(entries map[string]*orphaEntry) {
	sourceID := config.Dataconf[o.source]["id"]

	for orphaCode, entry := range entries {
		// Update counts in attr
		entry.attr.GeneCount = int32(entry.geneCount)
		entry.attr.PhenotypeCount = int32(entry.phenoCount)

		// Marshal and save attributes
		val, err := ffjson.Marshal(entry.attr)
		if err != nil {
			log.Printf("Orphanet: Error marshaling entry %s: %v", orphaCode, err)
			continue
		}
		o.d.addProp3(orphaCode, sourceID, val)

		// Create text search entries for name and synonyms
		if entry.attr.Name != "" {
			o.d.addXref(entry.attr.Name, textLinkID, orphaCode, o.source, true)
		}
		for _, syn := range entry.attr.Synonyms {
			if syn != "" {
				o.d.addXref(syn, textLinkID, orphaCode, o.source, true)
			}
		}

		// Pull MONDO synonyms for this Orphanet entry
		// This makes Orphanet entries searchable via MONDO's richer synonym vocabulary
		o.pullMondoSynonyms(orphaCode)

		// Cross-references to OMIM
		for _, omimID := range entry.omimIDs {
			o.d.addXref(orphaCode, sourceID, omimID, "mim", false)
		}

		// Cross-reference to MONDO
		if entry.mondoID != "" {
			mondoID := entry.mondoID
			if !strings.HasPrefix(mondoID, "MONDO:") {
				mondoID = "MONDO:" + mondoID
			}
			o.d.addXref(orphaCode, sourceID, mondoID, "mondo", false)
		}

		// Cross-references to MeSH
		for _, meshID := range entry.meshIDs {
			o.d.addXref(orphaCode, sourceID, meshID, "mesh", false)
		}

		// Cross-references to HPO phenotypes
		for _, pheno := range entry.attr.Phenotypes {
			evidence := pheno.Frequency
			if evidence != "" {
				o.d.addXrefWithEvidence(orphaCode, sourceID, pheno.HpoId, "hpo", false, evidence)
			} else {
				o.d.addXref(orphaCode, sourceID, pheno.HpoId, "hpo", false)
			}
		}
	}

	log.Printf("Orphanet: Saved %d entries with cross-references", len(entries))
}

// getXMLChildText extracts text from a child element
func getXMLChildText(parent *xmlparser.XMLElement, childName string) string {
	if children := parent.Childs[childName]; children != nil && len(children) > 0 {
		return children[0].InnerText
	}
	return ""
}

// parseIntSafe parses string to int, returns 0 on error
func parseIntSafe(s string) int {
	if s == "" {
		return 0
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}

// pullMondoSynonyms looks up MONDO entries that link to this Orphanet ID
// and adds their synonyms as text search terms for the Orphanet entry
func (o *orphanet) pullMondoSynonyms(orphaCode string) {
	// Check if lookup service is available and MONDO is configured
	if o.d.lookupService == nil {
		return
	}
	if _, exists := config.Dataconf["mondo"]; !exists {
		return
	}

	// Use MapFilter to find MONDO entries that link to this Orphanet ID
	// Query: orphaCode >>orphanet>>mondo - finds MONDO entries via Orphanet xrefs
	result, err := o.d.lookupService.MapFilter([]string{orphaCode}, ">>orphanet>>mondo", "")
	if err != nil {
		log.Printf("Orphanet pullMondoSynonyms: MapFilter error for %s: %v", orphaCode, err)
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
			log.Printf("Orphanet pullMondoSynonyms: found MONDO %s for Orphanet %s", mondoID, orphaCode)

			// Extract ontology attributes directly from target (contains synonyms)
			ontologyAttr := target.GetOntology()
			if ontologyAttr == nil {
				// Try getting full entry if attributes not in target
				mondoEntry, err := o.d.lookupFullEntry(mondoID, mondoDatasetID)
				if err != nil || mondoEntry == nil {
					continue
				}
				ontologyAttr = mondoEntry.GetOntology()
				if ontologyAttr == nil {
					continue
				}
			}

			log.Printf("Orphanet pullMondoSynonyms: MONDO %s has name=%s, %d synonyms", mondoID, ontologyAttr.Name, len(ontologyAttr.Synonyms))

			// Collect all phrases (name + synonyms)
			allPhrases := []string{}
			if ontologyAttr.Name != "" {
				allPhrases = append(allPhrases, ontologyAttr.Name)
			}
			allPhrases = append(allPhrases, ontologyAttr.Synonyms...)

			// Add full phrases as text search terms
			for _, phrase := range allPhrases {
				if phrase != "" {
					o.d.addXref(phrase, textLinkID, orphaCode, o.source, true)
				}
			}

			// Add individual significant words for partial matching
			for _, phrase := range allPhrases {
				for _, word := range strings.Fields(phrase) {
					word = strings.Trim(word, ",.;:'\"()-")
					if len(word) >= 4 && !isOrphanetStopWord(word) {
						o.d.addXref(word, textLinkID, orphaCode, o.source, true)
					}
				}
			}
		}
	}
}

// isOrphanetStopWord returns true for common medical terms that should not be indexed alone
func isOrphanetStopWord(word string) bool {
	word = strings.ToLower(word)
	stopWords := map[string]bool{
		// Disease type words
		"disease": true, "disorder": true, "syndrome": true, "condition": true,
		"type": true, "form": true, "variant": true,
		// Common medical terms
		"with": true, "without": true, "from": true, "that": true,
		"associated": true, "related": true, "induced": true,
		"chronic": true, "acute": true, "congenital": true,
		"familial": true, "hereditary": true, "inherited": true,
		"rare": true, "common": true,
	}
	return stopWords[word]
}
