package update

import (
	"biobtree/pbuf"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	xmlparser "github.com/tamerh/xml-stream-parser"
)

type clinvarXML struct {
	source string
	d      *DataUpdate
}

// parseIntOrZero converts string to int32, returning 0 for empty or invalid input
func parseIntOrZero(s string) int32 {
	if s == "" || s == "-" {
		return 0
	}
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(val)
}

// Helper for context-aware error checking
func (c *clinvarXML) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// Main update entry point
func (c *clinvarXML) update() {
	defer c.d.wg.Done()

	log.Println("ClinVar XML: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("ClinVar XML: [TEST MODE] Processing up to %d variants", testLimit)
	}

	// Process VCV XML file
	c.parseAndSaveVCVFile(testLimit, idLogFile)

	log.Printf("ClinVar XML: Processing complete (%.2fs)", time.Since(startTime).Seconds())
}

// parseAndSaveVCVFile processes the ClinVar VCV XML file using streaming parser
func (c *clinvarXML) parseAndSaveVCVFile(testLimit int, idLogFile *os.File) {
	// Build file URL
	filePath := "https://ftp.ncbi.nlm.nih.gov" + config.Dataconf[c.source]["path"] +
	            config.Dataconf[c.source]["xmlFile"]

	log.Printf("ClinVar XML: Downloading from %s", filePath)

	// Open file (handles both HTTP and local files)
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	c.check(err, "opening ClinVar VCV XML file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[c.source]["id"]

	// Create XML stream parser for VariationArchive elements
	parser := xmlparser.NewXMLParser(br, "VariationArchive")

	var processedCount int
	var previous int64

	// Stream through VariationArchive elements
	for variation := range parser.Stream() {

		// Progress tracking
		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			kbytesPerSecond := int64(parser.TotalReadSize) / elapsed / 1024
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: kbytesPerSecond}
		}

		// Extract VariationID from attributes
		variationID, ok := variation.Attrs["VariationID"]
		if !ok || variationID == "" {
			continue
		}

		// Test mode: log ID and check limit
		if idLogFile != nil {
			idLogFile.WriteString(variationID + "\n")
		}

		processedCount++
		if testLimit > 0 && processedCount >= testLimit {
			log.Printf("ClinVar XML: [TEST MODE] Reached limit of %d variants, stopping", testLimit)
			break
		}

		// Build ClinvarAttr from XML element
		attr := c.buildClinvarAttr(variation)

		// Create cross-references
		c.createXrefs(variation, variationID, sourceID, &attr)

		// Marshal and save
		val, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("ClinVar XML: Error marshaling variant %s: %v", variationID, err)
			continue
		}

		c.d.addProp3(variationID, sourceID, val)
	}

	log.Printf("ClinVar XML: Processed %d variants", processedCount)
}

// buildClinvarAttr extracts variant data from VariationArchive XML element
func (c *clinvarXML) buildClinvarAttr(variation *xmlparser.XMLElement) pbuf.ClinvarAttr {
	attr := pbuf.ClinvarAttr{}

	// Basic identifiers from attributes
	attr.VariationId = variation.Attrs["VariationID"]
	attr.Name = variation.Attrs["VariationName"]
	attr.Type = variation.Attrs["VariationType"]

	// Get ClassifiedRecord
	if classifiedRecords := variation.Childs["ClassifiedRecord"]; classifiedRecords != nil && len(classifiedRecords) > 0 {
		classified := classifiedRecords[0]

		// SimpleAllele information
		if simpleAlleles := classified.Childs["SimpleAllele"]; simpleAlleles != nil && len(simpleAlleles) > 0 {
			allele := simpleAlleles[0]

			attr.AlleleId = allele.Attrs["AlleleID"]

			// Variant type
			if varType := allele.Childs["VariantType"]; varType != nil && len(varType) > 0 {
				attr.Type = varType[0].InnerText
			}

			// GeneList
			if genes := allele.Childs["GeneList"]; genes != nil && len(genes) > 0 {
				for _, gene := range genes[0].Childs["Gene"] {
					if geneID := gene.Attrs["GeneID"]; geneID != "" {
						attr.GeneId = geneID
					}
					if symbol := gene.Attrs["Symbol"]; symbol != "" {
						attr.GeneSymbol = symbol
					}
					if hgncID := gene.Attrs["HGNC_ID"]; hgncID != "" {
						attr.HgncId = hgncID
					}
					break // Use first gene
				}
			}

			// Location (prefer GRCh38)
			if locations := allele.Childs["Location"]; locations != nil {
				for _, location := range locations {
					if seqLocs := location.Childs["SequenceLocation"]; seqLocs != nil {
						for _, seqLoc := range seqLocs {
							assembly := seqLoc.Attrs["Assembly"]

							// Prefer GRCh38, but accept GRCh37 if no GRCh38
							if assembly == "GRCh38" || (attr.Assembly == "" && assembly == "GRCh37") {
								attr.Assembly = assembly
								attr.Chromosome = seqLoc.Attrs["Chr"]
								attr.Start = parseIntOrZero(seqLoc.Attrs["start"])
								attr.Stop = parseIntOrZero(seqLoc.Attrs["stop"])
								attr.ReferenceAllele = seqLoc.Attrs["referenceAllele"]
								attr.AlternateAllele = seqLoc.Attrs["alternateAllele"]

								if assembly == "GRCh38" {
									break // Found GRCh38, use it
								}
							}
						}
					}
				}
			}

			// HGVS expressions
			if hgvsList := allele.Childs["HGVSlist"]; hgvsList != nil && len(hgvsList) > 0 {
				for _, hgvs := range hgvsList[0].Childs["HGVS"] {
					if nucleotide := hgvs.Childs["NucleotideExpression"]; nucleotide != nil && len(nucleotide) > 0 {
						if expression := nucleotide[0].Childs["Expression"]; expression != nil && len(expression) > 0 {
							attr.HgvsExpressions = append(attr.HgvsExpressions, expression[0].InnerText)
						}
					}
				}
			}

			// Functional consequences
			if fcList := allele.Childs["FunctionalConsequence"]; fcList != nil {
				for _, fc := range fcList {
					consequence := &pbuf.FunctionalConsequence{}
					consequence.SoId = fc.Attrs["Value"]

					if xrefs := fc.Childs["XRef"]; xrefs != nil {
						for _, xref := range xrefs {
							if xref.Attrs["DB"] == "Sequence Ontology" {
								consequence.SoId = xref.Attrs["ID"]
								break
							}
						}
					}

					// Get human-readable name from child elements
					for _, child := range fc.Childs {
						for _, elem := range child {
							if elem.InnerText != "" {
								consequence.Consequence = elem.InnerText
								break
							}
						}
					}

					attr.FunctionalConsequences = append(attr.FunctionalConsequences, consequence)
				}
			}
		}

		// Clinical classifications
		if classifications := classified.Childs["Classifications"]; classifications != nil && len(classifications) > 0 {
			if germline := classifications[0].Childs["GermlineClassification"]; germline != nil && len(germline) > 0 {
				if reviewStatus := germline[0].Childs["ReviewStatus"]; reviewStatus != nil && len(reviewStatus) > 0 {
					attr.ReviewStatus = reviewStatus[0].InnerText
				}
				if description := germline[0].Childs["Description"]; description != nil && len(description) > 0 {
					attr.GermlineClassification = description[0].InnerText
				}
				if dateEval := germline[0].Attrs["DateLastEvaluated"]; dateEval != "" {
					attr.LastEvaluated = dateEval
				}
			}
		}

		// ClinicalAssertions (submissions)
		if assertions := classified.Childs["ClinicalAssertionList"]; assertions != nil && len(assertions) > 0 {
			for _, assertion := range assertions[0].Childs["ClinicalAssertion"] {
				submission := &pbuf.ClinicalSubmission{}
				submission.ScvId = assertion.Attrs["ID"]

				if accession := assertion.Childs["ClinVarAccession"]; accession != nil && len(accession) > 0 {
					submission.SubmitterName = accession[0].Attrs["SubmitterName"]
				}

				if classification := assertion.Childs["Classification"]; classification != nil && len(classification) > 0 {
					submission.DateLastEvaluated = classification[0].Attrs["DateLastEvaluated"]

					if reviewStatus := classification[0].Childs["ReviewStatus"]; reviewStatus != nil && len(reviewStatus) > 0 {
						submission.ReviewStatus = reviewStatus[0].InnerText
					}
					if germClass := classification[0].Childs["GermlineClassification"]; germClass != nil && len(germClass) > 0 {
						submission.Classification = germClass[0].InnerText
					}
				}

				if observed := assertion.Childs["ObservedInList"]; observed != nil && len(observed) > 0 {
					for _, observedIn := range observed[0].Childs["ObservedIn"] {
						if method := observedIn.Childs["Method"]; method != nil && len(method) > 0 {
							if methodType := method[0].Childs["MethodType"]; methodType != nil && len(methodType) > 0 {
								submission.MethodType = methodType[0].InnerText
								break
							}
						}
					}
				}

				attr.Submissions = append(attr.Submissions, submission)
			}
		}
	}

	return attr
}

// createXrefs creates cross-references from variant to other databases
func (c *clinvarXML) createXrefs(variation *xmlparser.XMLElement, variationID, sourceID string, attr *pbuf.ClinvarAttr) {

	// Track added xrefs to avoid duplicates
	addedXrefs := make(map[string]bool)

	// Helper function to add xref only if not already added
	addXrefOnce := func(fromID, fromDataset, toID, toDataset string, isTextLink bool) {
		key := fromDataset + ":" + toID + ":" + toDataset
		if !addedXrefs[key] {
			c.d.addXref(fromID, fromDataset, toID, toDataset, isTextLink)
			addedXrefs[key] = true
		}
	}

	// Gene cross-references (using HGNC ID)
	if attr.HgncId != "" && attr.HgncId != "-" {
		addXrefOnce(variationID, sourceID, attr.HgncId, "hgnc", false)
	}

	// Gene Entrez ID
	if attr.GeneId != "" && attr.GeneId != "-" {
		addXrefOnce(variationID, sourceID, attr.GeneId, "entrez", false)
	}

	// Text search links for variant name and gene symbol
	if attr.Name != "" {
		addXrefOnce(attr.Name, textLinkID, variationID, c.source, true)
	}
	if attr.GeneSymbol != "" {
		addXrefOnce(attr.GeneSymbol, textLinkID, variationID, c.source, true)

		// Gene symbol → Ensembl cross-reference via gene symbol lookup
		// Handles paralogs by creating xrefs to all matching Ensembl genes
		// Search "BRCA1" returns Ensembl entry (with embedded HGNC data), then "BRCA1 >> clinvar" returns all variants
		c.d.addXrefViaGeneSymbol(attr.GeneSymbol, attr.Chromosome, variationID, c.source, sourceID)
	}

	// Get ClassifiedRecord for XRefs
	if classifiedRecords := variation.Childs["ClassifiedRecord"]; classifiedRecords != nil && len(classifiedRecords) > 0 {
		classified := classifiedRecords[0]

		// SimpleAllele XRefs
		if simpleAlleles := classified.Childs["SimpleAllele"]; simpleAlleles != nil && len(simpleAlleles) > 0 {
			allele := simpleAlleles[0]

			if xrefLists := allele.Childs["XRefList"]; xrefLists != nil && len(xrefLists) > 0 {
				for _, xref := range xrefLists[0].Childs["XRef"] {
					db := strings.ToLower(xref.Attrs["DB"])
					id := xref.Attrs["ID"]

					// Map database names to biobtree dataset names and create xrefs
					switch db {
					case "dbsnp":
						attr.DbsnpId = "rs" + id // Store dbSNP ID with "rs" prefix in attributes
						if _, exists := config.Dataconf["snp"]; exists {
							addXrefOnce(variationID, sourceID, "rs"+id, "snp", false)
						}
					case "omim":
						if _, exists := config.Dataconf["omim"]; exists {
							addXrefOnce(variationID, sourceID, id, "omim", false)
						}
					case "medgen":
						if _, exists := config.Dataconf["medgen"]; exists {
							addXrefOnce(variationID, sourceID, id, "medgen", false)
						}
					case "mondo":
						if _, exists := config.Dataconf["mondo"]; exists {
							addXrefOnce(variationID, sourceID, id, "mondo", false)
						}
					case "orphanet":
						if _, exists := config.Dataconf["orphanet"]; exists {
							addXrefOnce(variationID, sourceID, id, "orphanet", false)
						}
					}
				}
			}
		}

		// Condition/Phenotype cross-references from RCVList
		if rcvList := classified.Childs["RCVList"]; rcvList != nil && len(rcvList) > 0 {
			for _, rcvAccession := range rcvList[0].Childs["RCVAccession"] {
				if conditionList := rcvAccession.Childs["ClassifiedConditionList"]; conditionList != nil && len(conditionList) > 0 {
					for _, condition := range conditionList[0].Childs["ClassifiedCondition"] {
						conditionDB := condition.Attrs["DB"]
						conditionID := condition.Attrs["ID"]
						conditionName := condition.InnerText

						// Store phenotype info
						if conditionName != "" {
							attr.PhenotypeList = append(attr.PhenotypeList, conditionName)
						}
						if conditionID != "" {
							attr.PhenotypeIds = append(attr.PhenotypeIds, conditionID)
						}

						// Create cross-reference to MedGen/MONDO/HPO
						datasetName := strings.ToLower(conditionDB)
						if conditionID != "" {
							// Map database names
							switch datasetName {
							case "medgen":
								if _, exists := config.Dataconf["medgen"]; exists {
									addXrefOnce(variationID, sourceID, conditionID, "medgen", false)
								}
							case "mondo":
								if _, exists := config.Dataconf["mondo"]; exists {
									addXrefOnce(variationID, sourceID, conditionID, "mondo", false)
								}
							case "human phenotype ontology", "hpo":
								if _, exists := config.Dataconf["hpo"]; exists {
									addXrefOnce(variationID, sourceID, conditionID, "hpo", false)
								}
							case "omim":
								if _, exists := config.Dataconf["omim"]; exists {
									addXrefOnce(variationID, sourceID, conditionID, "omim", false)
								}
							case "orphanet":
								if _, exists := config.Dataconf["orphanet"]; exists {
									addXrefOnce(variationID, sourceID, conditionID, "orphanet", false)
								}
							case "mesh":
								if _, exists := config.Dataconf["mesh"]; exists {
									addXrefOnce(variationID, sourceID, conditionID, "mesh", false)
								}
							case "efo":
								if _, exists := config.Dataconf["efo"]; exists {
									addXrefOnce(variationID, sourceID, conditionID, "efo", false)
								}
							}
						}
					}
				}
			}
		}

		// Additional condition cross-references from Classifications/GermlineClassification/ConditionList
		// This is where MONDO and other comprehensive XRefs are located
		if classifications := classified.Childs["Classifications"]; classifications != nil && len(classifications) > 0 {
			for _, classification := range classifications {
				if germlineList := classification.Childs["GermlineClassification"]; germlineList != nil && len(germlineList) > 0 {
					for _, germline := range germlineList {
						if conditionList := germline.Childs["ConditionList"]; conditionList != nil && len(conditionList) > 0 {
							for _, traitSet := range conditionList[0].Childs["TraitSet"] {
								if traits := traitSet.Childs["Trait"]; traits != nil {
									for _, trait := range traits {
										// Get trait name for phenotype list
										if names := trait.Childs["Name"]; names != nil && len(names) > 0 {
											for _, name := range names {
												if elements := name.Childs["ElementValue"]; elements != nil && len(elements) > 0 {
													traitName := elements[0].InnerText
													if traitName != "" {
														attr.PhenotypeList = append(attr.PhenotypeList, traitName)
													}
												}
											}
										}

										// Parse XRefs from Trait (this has MONDO, MedGen, OMIM, etc.)
										if xrefs := trait.Childs["XRef"]; xrefs != nil {
											for _, xref := range xrefs {
												db := strings.ToLower(xref.Attrs["DB"])
												id := xref.Attrs["ID"]

												if id != "" {
													// Store in phenotype_ids
													attr.PhenotypeIds = append(attr.PhenotypeIds, id)

													// Create cross-references
													switch db {
													case "medgen":
														if _, exists := config.Dataconf["medgen"]; exists {
															addXrefOnce(variationID, sourceID, id, "medgen", false)
														}
													case "mondo":
														if _, exists := config.Dataconf["mondo"]; exists {
															addXrefOnce(variationID, sourceID, id, "mondo", false)
														}
													case "omim":
														if _, exists := config.Dataconf["omim"]; exists {
															addXrefOnce(variationID, sourceID, id, "omim", false)
														}
													case "orphanet":
														if _, exists := config.Dataconf["orphanet"]; exists {
															addXrefOnce(variationID, sourceID, id, "orphanet", false)
														}
													case "human phenotype ontology", "hpo":
														if _, exists := config.Dataconf["hpo"]; exists {
															addXrefOnce(variationID, sourceID, id, "hpo", false)
														}
													case "mesh":
														if _, exists := config.Dataconf["mesh"]; exists {
															addXrefOnce(variationID, sourceID, id, "mesh", false)
														}
													case "efo":
														if _, exists := config.Dataconf["efo"]; exists {
															addXrefOnce(variationID, sourceID, id, "efo", false)
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// PubMed citations from Classifications/GermlineClassification
		if classifications := classified.Childs["Classifications"]; classifications != nil && len(classifications) > 0 {
			for _, classification := range classifications {
				if germlineList := classification.Childs["GermlineClassification"]; germlineList != nil && len(germlineList) > 0 {
					for _, germline := range germlineList {
						// Extract citations
						if citations := germline.Childs["Citation"]; citations != nil {
							for _, citation := range citations {
								if ids := citation.Childs["ID"]; ids != nil {
									for _, id := range ids {
										if id.Attrs["Source"] == "PubMed" && id.InnerText != "" {
											if _, exists := config.Dataconf["pubmed"]; exists {
												addXrefOnce(variationID, sourceID, id.InnerText, "pubmed", false)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// PubMed citations from ClinicalAssertions (individual submissions)
		if assertions := classified.Childs["ClinicalAssertionList"]; assertions != nil && len(assertions) > 0 {
			for _, assertion := range assertions[0].Childs["ClinicalAssertion"] {
				// Check in ObservedInList
				if observedList := assertion.Childs["ObservedInList"]; observedList != nil && len(observedList) > 0 {
					for _, observedIn := range observedList[0].Childs["ObservedIn"] {
						if citations := observedIn.Childs["Citation"]; citations != nil {
							for _, citation := range citations {
								if ids := citation.Childs["ID"]; ids != nil {
									for _, id := range ids {
										if id.Attrs["Source"] == "PubMed" && id.InnerText != "" {
											if _, exists := config.Dataconf["pubmed"]; exists {
												addXrefOnce(variationID, sourceID, id.InnerText, "pubmed", false)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Remove duplicates from arrays
	attr.PhenotypeList = uniqueStrings(attr.PhenotypeList)
	attr.PhenotypeIds = uniqueStrings(attr.PhenotypeIds)
	attr.HgvsExpressions = uniqueStrings(attr.HgvsExpressions)
}

// uniqueStrings removes duplicates from string slice
func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return input
	}

	seen := make(map[string]bool)
	result := []string{}

	for _, item := range input {
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	sort.Strings(result)
	return result
}
