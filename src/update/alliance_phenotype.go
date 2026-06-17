package update

import (
	"biobtree/pbuf"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// alliancePhenotype ingests the Alliance of Genome Resources per-MOD PHENOTYPE
// files (PHENOTYPE_<MOD>.json.gz). Each kept annotation becomes a record entity
// with edges to the per-species gene dataset and to the model-organism phenotype
// ontology term, plus the supporting PubMed citation. This is the natural sibling
// of allianceDisease: the experimentally-observed phenotype layer of model-organism
// genes (mouse gene->MP, rat gene->MP, worm gene->WBPhenotype, xenopus gene->XPO).
//
// SCOPE - only edges that join existing biobtree nodes are kept:
//   - gene end: objectId routed by CURIE prefix to mgi/rgd/wormbase/xenbase
//     (same routing as allianceGeneTarget). Only GENE-level objectIds are kept;
//     allele / affected_genomic_model / genotype / transgene objectIds are dropped
//     (biobtree has no node for those).
//   - phenotype end: each phenotypeTermIdentifiers[].termId routed by ontology
//     prefix to a biobtree ontology we have: MP->mp, ZP->zp, WBPhenotype->wbphenotype,
//     XPO->xpo. Any term whose ontology we do not have a node for is skipped.
//
// Files processed: MGI, RGD, WB, XBXL, XBXT (built from path + files config). ZFIN
// is intentionally NOT processed: its PHENOTYPE file is composed of ZFA (anatomy) +
// PATO (quality) terms, not ZP, and we have no zfa/pato phenotype node to join (so it
// would yield zero ingestible edges). FB (DPO/FBcv), SGD (APO) and HUMAN (HP, already
// covered by HPOA) are likewise excluded by the file list / ontology guard.
type alliancePhenotype struct {
	source   string
	sourceID string
	d        *DataUpdate
}

// phenotypeAnnotation mirrors the AGR phenotype JSON record (only the fields we use).
type phenotypeAnnotation struct {
	ObjectID                 string `json:"objectId"`
	PhenotypeTermIdentifiers []struct {
		TermID string `json:"termId"`
	} `json:"phenotypeTermIdentifiers"`
	PhenotypeStatement string `json:"phenotypeStatement"`
	Evidence           struct {
		PublicationID string `json:"publicationId"`
	} `json:"evidence"`
}

func (a *alliancePhenotype) update() {
	defer a.d.wg.Done()

	a.sourceID = config.Dataconf[a.source]["id"]

	testLimit := config.GetTestLimit(a.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, a.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	basePath := config.Dataconf[a.source]["path"]
	files := strings.Split(config.Dataconf[a.source]["files"], ",")

	var total uint64
	for _, fn := range files {
		fn = strings.TrimSpace(fn)
		if fn == "" {
			continue
		}
		filePath := basePath + fn
		count, err := a.processFile(filePath, idLogFile, testLimit, &total)
		if err != nil {
			log.Printf("Alliance Phenotype: error processing %s: %v", filePath, err)
		}
		fmt.Printf("Alliance Phenotype: %s -> %d gene-phenotype associations\n", fn, count)
		if testLimit > 0 && int(total) >= testLimit {
			break
		}
	}
	fmt.Printf("Alliance Phenotype: processed %d gene-phenotype associations total\n", total)

	atomic.AddUint64(&a.d.totalParsedEntry, total)
	a.d.progChan <- &progressInfo{dataset: a.source, done: true}
}

func (a *alliancePhenotype) processFile(filePath string, idLogFile *os.File, testLimit int, total *uint64) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open alliance-phenotype file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var r io.Reader
	if gz != nil {
		r = gz
	} else {
		r = br
	}

	dec := json.NewDecoder(r)

	// The file is a single JSON object: {"metaData":{...},"data":[ {...}, ... ]}.
	// Stream the "data" array element-by-element to avoid loading 1M+ records.
	if err := seekToDataArray(dec); err != nil {
		return 0, err
	}

	var count uint64
	for dec.More() {
		var ann phenotypeAnnotation
		if err := dec.Decode(&ann); err != nil {
			return count, fmt.Errorf("json decode error: %v", err)
		}
		if a.processAnnotation(&ann, idLogFile) {
			count++
			atomic.AddUint64(total, 1)
			if testLimit > 0 && int(*total) >= testLimit {
				break
			}
		}
	}
	return count, nil
}

// seekToDataArray advances the decoder past the opening object brace and the
// "data" key up to (and including) the opening '[' of the data array.
func seekToDataArray(dec *json.Decoder) error {
	// consume the opening '{'
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("failed reading json start: %v", err)
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return fmt.Errorf("failed reading json key: %v", err)
		}
		key, _ := t.(string)
		if key == "data" {
			// next token is the '[' opening the array
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("failed reading data array start: %v", err)
			}
			return nil
		}
		// skip this key's value (metaData object)
		if err := skipValue(dec); err != nil {
			return err
		}
	}
	return fmt.Errorf("no data array found in alliance-phenotype file")
}

// skipValue consumes one full JSON value (object/array/scalar) from the decoder.
func skipValue(dec *json.Decoder) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); ok && (d == '{' || d == '[') {
		for dec.More() {
			if d == '{' {
				if _, err := dec.Token(); err != nil { // key
					return err
				}
			}
			if err := skipValue(dec); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil { // closing delim
			return err
		}
	}
	return nil
}

func (a *alliancePhenotype) processAnnotation(ann *phenotypeAnnotation, idLogFile *os.File) bool {
	// Gene end: only GENE-level objectIds; allele/AGM/genotype/transgene are dropped.
	geneDataset, geneID := alliancePhenotypeGeneTarget(ann.ObjectID)
	if geneID == "" {
		return false
	}

	if len(ann.PhenotypeTermIdentifiers) == 0 {
		return false
	}

	ref := ann.Evidence.PublicationID // e.g. "PMID:12345" or a MOD reference curie

	added := false
	for _, term := range ann.PhenotypeTermIdentifiers {
		ontDataset, termID := alliancePhenotypeTermTarget(term.TermID)
		if ontDataset == "" || termID == "" {
			continue // ontology we don't have a node for - skip
		}

		attr := &pbuf.AlliancePhenotypeAttr{
			GeneSymbol:         geneID,
			Species:            geneDataset,
			PhenotypeTerm:      termID,
			PhenotypeStatement: ann.PhenotypeStatement,
			Source:             "Alliance",
		}

		// Deterministic id (stable across re-downloads; references are via edges).
		h := fnv.New64a()
		h.Write([]byte(geneID + "|" + termID + "|" + ref))
		id := fmt.Sprintf("AGRP_%016x", h.Sum64())

		b, err := ffjson.Marshal(attr)
		if err != nil {
			continue
		}
		a.d.addProp3(id, a.sourceID, b)

		// record -> per-species gene dataset (only if loaded; MOD gene datasets are
		// optional, so guard to avoid edges into unconfigured datasets).
		if _, ok := config.Dataconf[geneDataset]; ok {
			a.d.addXref(id, a.sourceID, geneID, geneDataset, false)
		}

		// record -> phenotype ontology term (mp/zp/wbphenotype/xpo, colon form).
		a.d.addXref(id, a.sourceID, termID, ontDataset, false)

		// PubMed citation: publicationId is "PMID:nnnn" (or a MOD curation ref). The
		// pubmed bucket requires a digit-starting id, so guard.
		if pmid := strings.TrimPrefix(ref, "PMID:"); pmid != ref && isAllDigits(pmid) {
			a.d.addXref(id, a.sourceID, pmid, "pubmed", false)
		}

		// Text search by gene symbol/id + phenotype statement.
		a.d.addXref(geneID, textLinkID, id, a.source, true)
		if ann.PhenotypeStatement != "" {
			a.d.addXref(ann.PhenotypeStatement, textLinkID, id, a.source, true)
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, id)
		}
		added = true
	}
	return added
}

// alliancePhenotypeGeneTarget maps an Alliance phenotype objectId (CURIE) to
// biobtree's (dataset, id) form, GENE-level only. MGI keeps its prefix (how biobtree
// stores it); RGD/WB/Xenbase drop the CURIE prefix to the local accession. Allele,
// affected_genomic_model, genotype and transgene objectIds (e.g. ZFIN:ZDB-ALT-*,
// ZFIN:ZDB-FISH-*, WB:WBVar*, WB:WBTransgene*) return ("","") and are dropped.
func alliancePhenotypeGeneTarget(objectID string) (dataset, id string) {
	switch {
	case strings.HasPrefix(objectID, "MGI:"):
		return "mgi", objectID
	case strings.HasPrefix(objectID, "RGD:"):
		return "rgd", strings.TrimPrefix(objectID, "RGD:")
	case strings.HasPrefix(objectID, "ZFIN:ZDB-GENE-"):
		return "zfin", strings.TrimPrefix(objectID, "ZFIN:")
	case strings.HasPrefix(objectID, "WB:WBGene"):
		return "wormbase", strings.TrimPrefix(objectID, "WB:")
	case strings.HasPrefix(objectID, "Xenbase:XB-GENE-"):
		return "xenbase", strings.TrimPrefix(objectID, "Xenbase:")
	}
	return "", ""
}

// alliancePhenotypeTermTarget maps a phenotype termId to (ontology dataset, term id
// in colon form) for the model-organism phenotype ontologies biobtree has. WB terms
// arrive double-prefixed as "WB:WBPhenotype:nnnn"; strip the leading "WB:" so the id
// matches the wbphenotype ontology node. Terms from ontologies we do not have a node
// for (ZFA/PATO/GO/CHEBI/BSPO/DPO/FBcv/APO/HP/...) return ("","") and are skipped.
func alliancePhenotypeTermTarget(termID string) (dataset, id string) {
	termID = strings.TrimPrefix(termID, "WB:") // WB:WBPhenotype:nnnn -> WBPhenotype:nnnn
	switch {
	case strings.HasPrefix(termID, "MP:"):
		return "mp", termID
	case strings.HasPrefix(termID, "ZP:"):
		return "zp", termID
	case strings.HasPrefix(termID, "WBPhenotype:"):
		return "wbphenotype", termID
	case strings.HasPrefix(termID, "XPO:"):
		return "xpo", termID
	}
	return "", ""
}
