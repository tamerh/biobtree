package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// allianceDisease ingests the Alliance of Genome Resources harmonized
// gene->disease file (DISEASE-ALLIANCE_COMBINED.tsv.gz). Each kept row becomes a
// record entity with edges to the per-species gene dataset and to the DOID
// disease term, plus the supporting PubMed citation. This adds biobtree's
// cross-species / model-organism disease layer (mouse/rat/fish/fly/worm/yeast/
// frog + human), keyed to DOID (which is now a first-class ontology).
//
// We keep only DIRECT gene-level curation: DBobjectType == "gene" and an
// association type that is NOT orthology-inferred (`*_via_orthology`, which is
// derivable from Compara + human disease) and NOT a negative assertion
// (`is_not_*`). Allele / affected_genomic_model rows are dropped (biobtree has
// no allele/AGM node).
type allianceDisease struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (a *allianceDisease) update() {
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

	filePath := config.Dataconf[a.source]["path"]
	count, err := a.processFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Printf("Alliance Disease: error processing %s: %v", filePath, err)
	}
	fmt.Printf("Alliance Disease: processed %d gene-disease associations\n", count)

	atomic.AddUint64(&a.d.totalParsedEntry, count)
	a.d.progChan <- &progressInfo{dataset: a.source, done: true}
}

func (a *allianceDisease) processFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open alliance-disease file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024)
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024)
	}

	var count uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return count, fmt.Errorf("read error: %v", rerr)
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		// The file starts with a block of '#' comment lines, then the column
		// header row (starts with "Taxon\t"); skip both, keep data rows.
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "Taxon\t") {
			f := strings.Split(line, "\t")
			if a.processRow(f, idLogFile) {
				count++
				if testLimit > 0 && int(count) >= testLimit {
					break
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	return count, nil
}

// columns: 0 Taxon, 1 SpeciesName, 2 DBobjectType, 3 DBObjectID, 4 DBObjectSymbol,
// 5 AssociationType, 6 DOID, 7 DOtermName, 8 WithOrtholog, 9 InferredFromID,
// 10 InferredFromSymbol, 11 ExperimentalCondition, 12 Modifier, 13 EvidenceCode,
// 14 EvidenceCodeName, 15 Ref
func (a *allianceDisease) processRow(f []string, idLogFile *os.File) bool {
	get := func(i int) string {
		if i < len(f) {
			return strings.TrimSpace(f[i])
		}
		return ""
	}

	if get(2) != "gene" { // gene-level rows only (drop allele / affected_genomic_model)
		return false
	}

	assoc := get(5)
	if assoc == "" || strings.Contains(assoc, "_via_orthology") || strings.HasPrefix(assoc, "is_not") {
		return false // drop orthology-inferred + negative assertions
	}

	geneDataset, geneID := allianceGeneTarget(get(3))
	doid := get(6)
	if geneID == "" || !strings.HasPrefix(doid, "DOID:") {
		return false
	}

	symbol := get(4)
	disease := get(7)

	attr := &pbuf.AllianceDiseaseAttr{
		GeneSymbol:      symbol,
		Species:         get(1),
		AssociationType: assoc,
		DiseaseName:     disease,
		EvidenceCode:    get(13),
		Source:          "Alliance",
	}

	// Deterministic id (stable across re-downloads; references are via edges).
	h := fnv.New64a()
	h.Write([]byte(geneID + "|" + doid + "|" + assoc + "|" + get(15)))
	id := fmt.Sprintf("AGRD_%016x", h.Sum64())

	b, err := ffjson.Marshal(attr)
	if err != nil {
		return false
	}
	a.d.addProp3(id, a.sourceID, b)

	// gene -> per-species gene dataset (only if that dataset is loaded; the MOD
	// datasets are optional, so guard to avoid edges into unconfigured datasets).
	if _, ok := config.Dataconf[geneDataset]; ok {
		a.d.addXref(id, a.sourceID, geneID, geneDataset, false)
	}

	// disease -> DOID (first-class ontology)
	a.d.addXref(id, a.sourceID, doid, "doid", false)

	// PubMed citation: Ref is "PMID:nnnn" (or a non-PMID curation ref); the pubmed
	// bucket requires a digit-starting id, so guard.
	if ref := get(15); strings.HasPrefix(ref, "PMID:") {
		if pmid := strings.TrimPrefix(ref, "PMID:"); isAllDigits(pmid) {
			a.d.addXref(id, a.sourceID, pmid, "pubmed", false)
		}
	}

	// Text search by gene symbol + disease name.
	if symbol != "" {
		a.d.addXref(symbol, textLinkID, id, a.source, true)
	}
	if disease != "" {
		a.d.addXref(disease, textLinkID, id, a.source, true)
	}

	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}
	return true
}

// allianceGeneTarget maps an Alliance DBObjectID (CURIE-prefixed) to biobtree's
// (dataset, id) form. MGI and HGNC keep their prefix (that is how biobtree stores
// them, matching UniProt); the others drop the Alliance CURIE prefix to the local
// accession. Returns ("","") for an unrecognized prefix.
func allianceGeneTarget(dbObjectID string) (dataset, id string) {
	switch {
	case strings.HasPrefix(dbObjectID, "HGNC:"):
		return "hgnc", dbObjectID
	case strings.HasPrefix(dbObjectID, "MGI:"):
		return "mgi", dbObjectID
	case strings.HasPrefix(dbObjectID, "RGD:"):
		return "rgd", strings.TrimPrefix(dbObjectID, "RGD:")
	case strings.HasPrefix(dbObjectID, "SGD:"):
		return "sgd", strings.TrimPrefix(dbObjectID, "SGD:")
	case strings.HasPrefix(dbObjectID, "ZFIN:"):
		return "zfin", strings.TrimPrefix(dbObjectID, "ZFIN:")
	case strings.HasPrefix(dbObjectID, "FB:"):
		return "flybase", strings.TrimPrefix(dbObjectID, "FB:")
	case strings.HasPrefix(dbObjectID, "WB:"):
		return "wormbase", strings.TrimPrefix(dbObjectID, "WB:")
	case strings.HasPrefix(dbObjectID, "Xenbase:"):
		return "xenbase", strings.TrimPrefix(dbObjectID, "Xenbase:")
	}
	return "", ""
}
