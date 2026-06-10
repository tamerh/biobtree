package update

import (
	"biobtree/pbuf"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// hpa parses the Human Protein Atlas (https://www.proteinatlas.org/, CC BY 4.0).
// Source: proteinatlas.xml.gz (the full dataset; the TSV/JSON are a subset).
//
// One <entry> per gene (keyed by Ensembl gene ID). Produces four datasets,
// mirroring the Bgee gene-expression idiom (gene card + detail children):
//   - hpa            : gene "card" (identity, protein class, subcellular→GO,
//                      specificity calls, top expressed tissues)
//   - hpa_expression : per (gene, tissue/cell/cell-line) RNA nTPM + IHC staining,
//                      xref'd to UBERON / Cellosaurus (analog of bgee_evidence)
//   - hpa_pathology  : per (gene, cancer) prognostic survival association
//   - hpa_antibody   : per HPA validation antibody (reliability, antigen)
type hpa struct {
	source string
	d      *DataUpdate
}

func (h *hpa) check(err error, operation string) {
	checkWithContext(err, h.source, operation)
}

// ---- XML structs (one <entry> per gene) ----

type hpaXMLEntry struct {
	URL        string   `xml:"url,attr"`
	Name       string   `xml:"name"`
	Synonyms   []string `xml:"synonym"`
	Identifier struct {
		ID    string `xml:"id,attr"`
		Xrefs []struct {
			ID string `xml:"id,attr"`
			DB string `xml:"db,attr"`
		} `xml:"xref"`
	} `xml:"identifier"`
	ProteinClasses struct {
		Classes []struct {
			Name string `xml:"name,attr"`
		} `xml:"proteinClass"`
	} `xml:"proteinClasses"`
	ProteinEvidence struct {
		Evidence string `xml:"evidence,attr"`
	} `xml:"proteinEvidence"`
	PredictedLocation string `xml:"predictedLocation"`
	// Subcellular localization (immunofluorescence); locations carry GO IDs.
	CellExpression []struct {
		Verifications []hpaVerification `xml:"verification"`
		Data          struct {
			Locations []struct {
				Status string `xml:"status,attr"`
				GOId   string `xml:"GOId,attr"`
				Name   string `xml:",chardata"`
			} `xml:"location"`
		} `xml:"data"`
	} `xml:"cellExpression"`
	// RNA expression: multiple blocks by assayType (consensusTissue, ...).
	RnaExpression []struct {
		AssayType   string `xml:"assayType,attr"`
		Specificity struct {
			Specificity string `xml:"specificity,attr"`
		} `xml:"rnaSpecificity"`
		Distribution string `xml:"rnaDistribution"`
		Data         []struct {
			Tissue hpaTissueRef  `xml:"tissue"`
			Levels []hpaRNALevel `xml:"level"`
		} `xml:"data"`
		SingleCell []struct {
			Name    string `xml:"name,attr"`
			UnitRNA string `xml:"unitRNA,attr"`
			ExpRNA  string `xml:"expRNA,attr"`
		} `xml:"singleCellTypeExpression"`
	} `xml:"rnaExpression"`
	// Protein IHC staining per tissue (per cell type).
	TissueExpression []struct {
		AssayType     string            `xml:"assayType,attr"`
		Verifications []hpaVerification `xml:"verification"`
		Data          []struct {
			Tissue      hpaTissueRef `xml:"tissue"`
			Level       hpaIHCLevel  `xml:"level"`
			TissueCells []struct {
				CellType string      `xml:"cellType"`
				Level    hpaIHCLevel `xml:"level"`
			} `xml:"tissueCell"`
		} `xml:"data"`
	} `xml:"tissueExpression"`
	// Cancer prognostics (survival analysis), often TCGA-derived.
	CancerExpression []struct {
		Specificity string `xml:"rnaCancerSpecificity"`
		Data        []struct {
			Tissue   string `xml:"tissue"` // cancer type name
			Survival struct {
				PrognosticType string `xml:"prognosticType,attr"`
				IsPrognostic   string `xml:"isPrognostic,attr"`
				Prognostic     string `xml:"prognostic,attr"`
				PValue         string `xml:"pValue,attr"`
				DataSource     string `xml:"dataSource,attr"`
			} `xml:"survivalAnalysis"`
		} `xml:"data"`
	} `xml:"cancerExpression"`
	// Validation antibodies.
	Antibodies []struct {
		ID              string `xml:"id,attr"`
		ReleaseVersion  string `xml:"releaseVersion,attr"`
		AntigenSequence string `xml:"antigenSequence"`
		// IHC reliability + per-antibody validation notes are nested one level
		// deeper, inside the antibody's own <tissueExpression>.
		TissueExpression []struct {
			Verifications []hpaVerification `xml:"verification"`
			Validations   []hpaValidation   `xml:"validation"`
		} `xml:"tissueExpression"`
	} `xml:"antibody"`
}

type hpaValidation struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type hpaVerification struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type hpaTissueRef struct {
	OntologyTerms string `xml:"ontologyTerms,attr"` // comma-separated UBERON IDs
	Name          string `xml:",chardata"`
}

type hpaRNALevel struct {
	Type    string `xml:"type,attr"`    // normalizedRNAExpression / proteinCodingRNAExpression / RNAExpression
	UnitRNA string `xml:"unitRNA,attr"` // nTPM / pTPM / TPM
	ExpRNA  string `xml:"expRNA,attr"`
}

type hpaIHCLevel struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"` // "high"/"medium"/"low"/"not detected"
}

// expr accumulates RNA + IHC for one (gene, tissue) so they merge into one entry.
type hpaExprAccum struct {
	entityID   string
	entityName string
	axis       string
	ntpm       float64
	hasNtpm    bool
	level      string
	cellLevels []string
}

func (h *hpa) update() {
	defer h.d.wg.Done()

	log.Println("HPA: Starting Human Protein Atlas processing...")
	start := time.Now()

	hpaID := config.Dataconf[h.source]["id"]
	exprID := datasetID("hpa_expression")
	pathID := datasetID("hpa_pathology")
	abID := datasetID("hpa_antibody")

	testLimit := config.GetTestLimit(h.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, h.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	basePath := config.Dataconf[h.source]["downloadBaseUrl"]
	if basePath == "" {
		basePath = config.Dataconf[h.source]["path"]
	}
	filePath := basePath + config.Dataconf[h.source]["hpaFile"]
	if config.Dataconf[h.source]["useLocalFile"] == "yes" {
		filePath = config.Dataconf[h.source]["path"] + config.Dataconf[h.source]["hpaFile"]
	}
	log.Printf("HPA: reading %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(h.source, "", "", filePath)
	h.check(err, "opening proteinatlas.xml.gz")
	defer closeReaders(gz, ftpFile, client, localFile)

	decoder := xml.NewDecoder(br)
	var geneCount, exprCount, pathCount, abCount int64
	var previous int64

	for {
		tok, err := decoder.Token()
		if err != nil {
			break // EOF or read error ends the stream
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "entry" {
			continue
		}

		var e hpaXMLEntry
		if err := decoder.DecodeElement(&e, &se); err != nil {
			log.Printf("HPA: skip entry (decode error): %v", err)
			continue
		}

		geneID := e.Identifier.ID
		if geneID == "" {
			continue
		}

		// Progress
		elapsed := int64(time.Since(h.d.start).Seconds())
		if elapsed > previous+h.d.progInterval {
			previous = elapsed
			h.d.progChan <- &progressInfo{dataset: h.source, currentKBPerSec: int64(decoder.InputOffset()) / max64(elapsed, 1) / 1024}
		}

		h.processEntry(&e, geneID, hpaID, exprID, pathID, abID, &exprCount, &pathCount, &abCount)

		if idLogFile != nil {
			idLogFile.WriteString(geneID + "\n")
		}
		geneCount++
		if testLimit > 0 && geneCount >= int64(testLimit) {
			log.Printf("HPA: [TEST MODE] Reached gene limit of %d", testLimit)
			break
		}
	}

	atomic.AddUint64(&h.d.totalParsedEntry, uint64(geneCount+exprCount+pathCount+abCount))
	log.Printf("HPA: processed %d genes, %d expression, %d pathology, %d antibody (%.1fs)",
		geneCount, exprCount, pathCount, abCount, time.Since(start).Seconds())

	// Signal completion for the parent and every child dataset.
	for _, ds := range []string{h.source, "hpa_expression", "hpa_pathology", "hpa_antibody"} {
		if _, ok := config.Dataconf[ds]; ok {
			h.d.progChan <- &progressInfo{dataset: ds, done: true}
		}
	}
}

func (h *hpa) processEntry(e *hpaXMLEntry, geneID, hpaID, exprID, pathID, abID string,
	exprCount, pathCount, abCount *int64) {

	// ---- gene identifiers from the identifier xrefs ----
	var uniprot, entrez string
	for _, x := range e.Identifier.Xrefs {
		switch {
		case strings.HasPrefix(x.DB, "Uniprot"):
			if uniprot == "" {
				uniprot = x.ID
			}
		case x.DB == "NCBI GeneID":
			entrez = x.ID
		}
	}

	// ---- parent HpaAttr ----
	attr := &pbuf.HpaAttr{
		GeneId:            geneID,
		GeneName:          e.Name,
		Synonyms:          e.Synonyms,
		Uniprot:           uniprot,
		Entrez:            entrez,
		ProteinEvidence:   e.ProteinEvidence.Evidence,
		PredictedLocation: e.PredictedLocation,
	}

	// protein classes (dedup names)
	seenClass := map[string]bool{}
	for _, c := range e.ProteinClasses.Classes {
		if c.Name != "" && !seenClass[c.Name] {
			seenClass[c.Name] = true
			attr.ProteinClasses = append(attr.ProteinClasses, c.Name)
		}
	}

	// subcellular locations + GO xrefs
	goSeen := map[string]bool{}
	for _, ce := range e.CellExpression {
		for _, loc := range ce.Data.Locations {
			name := strings.TrimSpace(loc.Name)
			if name == "" {
				continue
			}
			if loc.Status == "additional" {
				attr.SubcellularAdditional = append(attr.SubcellularAdditional, name)
			} else {
				attr.SubcellularMain = append(attr.SubcellularMain, name)
			}
			if strings.HasPrefix(loc.GOId, "GO:") && !goSeen[loc.GOId] {
				goSeen[loc.GOId] = true
				if _, ok := config.Dataconf["go"]; ok {
					h.d.addXref(geneID, hpaID, loc.GOId, "go", false)
				}
			}
		}
	}

	// RNA / IHC specificity calls + per-tissue accumulation
	exprByEntity := map[string]*hpaExprAccum{} // entityID -> accum
	order := []string{}                        // preserve discovery order for stable output

	for _, rna := range e.RnaExpression {
		spec := rna.Specificity.Specificity
		switch rna.AssayType {
		case "consensusTissue", "tissue":
			if attr.RnaTissueSpecificity == "" {
				attr.RnaTissueSpecificity = spec
				attr.RnaTissueDistribution = rna.Distribution
			}
		case "blood", "bloodCell":
			if attr.RnaBloodSpecificity == "" {
				attr.RnaBloodSpecificity = spec
			}
		}
		for _, dt := range rna.Data {
			entID, entName := tissueEntity(dt.Tissue)
			if entName == "" {
				continue
			}
			ntpm := pickNTPM(dt.Levels)
			ac := getAccum(exprByEntity, &order, entID, entName, "tissue")
			if ntpm >= 0 {
				ac.ntpm, ac.hasNtpm = ntpm, true
			}
		}
		// single-cell types (names only; CL mapping deferred)
		for _, sc := range rna.SingleCell {
			if sc.Name == "" {
				continue
			}
			ac := getAccum(exprByEntity, &order, "name:"+sc.Name, sc.Name, "single_cell")
			if v, err := strconv.ParseFloat(sc.ExpRNA, 64); err == nil {
				ac.ntpm, ac.hasNtpm = v, true
			}
		}
	}
	if len(e.CancerExpression) > 0 {
		attr.RnaCancerSpecificity = e.CancerExpression[0].Specificity
	}

	// IHC protein staining per tissue (merge into the tissue accumulators)
	for _, te := range e.TissueExpression {
		for _, dt := range te.Data {
			entID, entName := tissueEntity(dt.Tissue)
			if entName == "" {
				continue
			}
			ac := getAccum(exprByEntity, &order, entID, entName, "tissue")
			if dt.Level.Value != "" {
				ac.level = strings.TrimSpace(dt.Level.Value)
			}
			for _, tc := range dt.TissueCells {
				if tc.CellType != "" {
					ac.cellLevels = append(ac.cellLevels, tc.CellType+"|"+strings.TrimSpace(tc.Level.Value))
				}
			}
		}
	}

	// top expressed tissues / single-cell types for the gene card
	attr.TopTissues = topExpressed(exprByEntity, order, "tissue", 10)
	attr.TopCellTypes = topExpressed(exprByEntity, order, "single_cell", 10)

	// Gene reliability (Enhanced/Supported/Approved/Uncertain): IHC from the
	// entry-level tissueExpression verification, IF from cellExpression verification.
	for _, te := range e.TissueExpression {
		if v := pickReliability(te.Verifications); v != "" {
			attr.ReliabilityIh = v
			break
		}
	}
	for _, ce := range e.CellExpression {
		if v := pickReliability(ce.Verifications); v != "" {
			attr.ReliabilityIf = v
			break
		}
	}

	// ---- save parent + gene-hub xrefs ----
	if b, err := ffjson.Marshal(attr); err == nil {
		h.d.addProp3(geneID, hpaID, b)
	}
	if e.Name != "" {
		h.d.addXref(e.Name, textLinkID, geneID, h.source, true)
	}
	if _, ok := config.Dataconf["ensembl"]; ok {
		h.d.addXref(geneID, hpaID, geneID, "ensembl", false)
	}
	if uniprot != "" {
		if _, ok := config.Dataconf["uniprot"]; ok {
			h.d.addXref(geneID, hpaID, uniprot, "uniprot", false)
		}
	}
	if e.Name != "" {
		if _, ok := config.Dataconf["hgnc"]; ok {
			h.d.addHumanGeneXrefsAll(e.Name, geneID, hpaID)
		}
	}
	if entrez != "" {
		if _, ok := config.Dataconf["entrez"]; ok {
			h.d.addXref(geneID, hpaID, entrez, "entrez", false)
		}
	}

	// ---- hpa_expression children ----
	if _, ok := config.Dataconf["hpa_expression"]; ok {
		_, uberonOK := config.Dataconf["uberon"]
		_, cvclOK := config.Dataconf["cellosaurus"]
		for _, entID := range order {
			ac := exprByEntity[entID]
			eattr := &pbuf.HpaExpressionAttr{
				GeneId:     geneID,
				EntityId:   ac.entityID,
				EntityName: ac.entityName,
				Axis:       ac.axis,
				Ntpm:       ac.ntpm,
				ProteinLevel: ac.level,
				CellLevels: ac.cellLevels,
			}
			key := geneID + "|" + ac.entityID
			if b, err := ffjson.Marshal(eattr); err == nil {
				h.d.addProp3(key, exprID, b)
				*exprCount++
			}
			sortLevels := []string{
				ComputeSortLevelValue(SortLevelExpressionScore, map[string]interface{}{"score": ac.ntpm}),
			}
			// expression → gene (enables hpa >> hpa_expression)
			h.d.addXrefWithSortLevels(key, exprID, geneID, h.source, sortLevels)
			// expression → anatomy / cell line (enables UBERON:.. >> hpa_expression)
			if uberonOK && strings.HasPrefix(ac.entityID, "UBERON:") {
				h.d.addXrefWithSortLevels(key, exprID, ac.entityID, "uberon", sortLevels)
				// also link the gene directly to the tissue (gene >> uberon), like Bgee
				h.d.addXrefWithSortLevels(geneID, hpaID, ac.entityID, "uberon", sortLevels)
			} else if cvclOK && strings.HasPrefix(ac.entityID, "CVCL_") {
				h.d.addXrefWithSortLevels(key, exprID, ac.entityID, "cellosaurus", sortLevels)
			}
		}
	}

	// ---- hpa_pathology children ----
	if _, ok := config.Dataconf["hpa_pathology"]; ok {
		for _, ce := range e.CancerExpression {
			for _, dt := range ce.Data {
				cancer := strings.TrimSpace(dt.Tissue)
				if cancer == "" {
					continue
				}
				pv, _ := strconv.ParseFloat(dt.Survival.PValue, 64)
				pattr := &pbuf.HpaPathologyAttr{
					GeneId:         geneID,
					Cancer:         cancer,
					IsPrognostic:   dt.Survival.IsPrognostic == "true",
					PrognosticType: dt.Survival.PrognosticType,
					Prognostic:     dt.Survival.Prognostic,
					PValue:         pv,
					DataSource:     dt.Survival.DataSource,
				}
				key := geneID + "|" + sanitizeKeyPart(cancer)
				if b, err := ffjson.Marshal(pattr); err == nil {
					h.d.addProp3(key, pathID, b)
					*pathCount++
				}
				h.d.addXref(key, pathID, geneID, h.source, false)
			}
		}
	}

	// ---- hpa_antibody children ----
	if _, ok := config.Dataconf["hpa_antibody"]; ok {
		for _, ab := range e.Antibodies {
			if ab.ID == "" {
				continue
			}
			aattr := &pbuf.HpaAntibodyAttr{
				AntibodyId:      ab.ID,
				Gene:            e.Name,
				GeneId:          geneID,
				AntigenSequence: ab.AntigenSequence,
				ReleaseVersion:  ab.ReleaseVersion,
			}
			// Per-antibody IHC reliability + validation notes live inside the
			// antibody's own <tissueExpression>.
			for _, te := range ab.TissueExpression {
				if aattr.ReliabilityIh == "" {
					aattr.ReliabilityIh = pickReliability(te.Verifications)
				}
				for _, v := range te.Validations {
					if v.Value != "" {
						aattr.Validations = append(aattr.Validations, v.Type+": "+strings.TrimSpace(v.Value))
					}
				}
			}
			if b, err := ffjson.Marshal(aattr); err == nil {
				h.d.addProp3(ab.ID, abID, b)
				*abCount++
			}
			h.d.addXref(ab.ID, textLinkID, ab.ID, "hpa_antibody", true)
			// antibody → gene and gene → antibody (enables hpa/gene >> hpa_antibody)
			if _, ok := config.Dataconf["ensembl"]; ok {
				h.d.addXref(ab.ID, abID, geneID, "ensembl", false)
			}
			h.d.addXref(geneID, hpaID, ab.ID, "hpa_antibody", false)
		}
	}
}

// ---- helpers ----

// tissueEntity returns (entityID, name): the first UBERON term if present, else a name-key.
func tissueEntity(t hpaTissueRef) (string, string) {
	name := strings.TrimSpace(t.Name)
	if t.OntologyTerms != "" {
		first := strings.TrimSpace(strings.SplitN(t.OntologyTerms, ",", 2)[0])
		if first != "" {
			return first, name
		}
	}
	if name == "" {
		return "", ""
	}
	return "name:" + name, name
}

// pickNTPM returns the normalized RNA expression (nTPM) value, or -1 if absent.
func pickNTPM(levels []hpaRNALevel) float64 {
	for _, l := range levels {
		if l.Type == "normalizedRNAExpression" {
			if v, err := strconv.ParseFloat(l.ExpRNA, 64); err == nil {
				return v
			}
		}
	}
	return -1
}

func getAccum(m map[string]*hpaExprAccum, order *[]string, entID, entName, axis string) *hpaExprAccum {
	if ac, ok := m[entID]; ok {
		return ac
	}
	ac := &hpaExprAccum{entityID: entID, entityName: entName, axis: axis, ntpm: -1}
	m[entID] = ac
	*order = append(*order, entID)
	return ac
}

// topExpressed returns up to n "name|nTPM" strings for the given axis, highest nTPM first.
func topExpressed(m map[string]*hpaExprAccum, order []string, axis string, n int) []string {
	type kv struct {
		name string
		v    float64
	}
	var items []kv
	for _, id := range order {
		ac := m[id]
		if ac.axis == axis && ac.hasNtpm {
			items = append(items, kv{ac.entityName, ac.ntpm})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].v > items[j].v })
	var out []string
	for i := 0; i < len(items) && i < n; i++ {
		out = append(out, fmt.Sprintf("%s|%.1f", items[i].name, items[i].v))
	}
	return out
}

// pickReliability returns the reliability level (Enhanced/Supported/Approved/
// Uncertain) from a set of <verification> elements — preferring type="reliability"
// or type="validation", else the first non-empty value.
func pickReliability(verifs []hpaVerification) string {
	for _, v := range verifs {
		if v.Type == "reliability" || v.Type == "validation" {
			if s := strings.TrimSpace(v.Value); s != "" {
				return s
			}
		}
	}
	for _, v := range verifs {
		if s := strings.TrimSpace(v.Value); s != "" {
			return s
		}
	}
	return ""
}

// sanitizeKeyPart removes characters that would break composite keys.
func sanitizeKeyPart(s string) string {
	return strings.NewReplacer("|", "_", "\t", " ", "\n", " ").Replace(strings.TrimSpace(s))
}

// datasetID returns the configured numeric id for a dataset, or "" if absent.
func datasetID(name string) string {
	if cfg, ok := config.Dataconf[name]; ok {
		return cfg["id"]
	}
	return ""
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
