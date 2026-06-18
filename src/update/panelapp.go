package update

import (
	"biobtree/pbuf"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// panelapp ingests Genomics England PanelApp clinical gene panels.
//
// Source:  https://panelapp.genomicsengland.co.uk/api/v1/panels/   (REST/JSON)
// License: CC BY 4.0 (Genomics England)
//
// Entity model (MASTER/CHILD):
//
//	panelapp       (MASTER) one record per PANEL, keyed by the panel id (e.g.
//	               "1207"). Holds the panel's name, disease grouping, version and
//	               gene count.
//	panelapp_gene  (CHILD)  one record per (panel, gene), keyed "<panelId>_<symbol>"
//	               (e.g. "1207_HMBS"). Holds the gene symbol, owning panel name,
//	               clinical confidence (green/amber) and mode of inheritance, and is
//	               linked back to its parent panel master. Each gene record carries
//	               the real graph edges: -> hgnc, -> ensembl (GRCh38 ENSG),
//	               -> mim (OMIM gene + phenotype OMIM tokens) and -> mondo
//	               (phenotype MONDO tokens).
//
// Scope: only GREEN (confidence_level "3") and AMBER ("2") genes are ingested;
// red/level-1 and level-0 genes are low-evidence noise and dropped.
type panelapp struct {
	source string
	d      *DataUpdate
}

func (p *panelapp) check(err error, operation string) {
	checkWithContext(err, p.source, operation)
}

// panelListResponse is the paginated /panels/ listing.
type panelListResponse struct {
	Count   int            `json:"count"`
	Next    string         `json:"next"`
	Results []panelSummary `json:"results"`
}

type panelSummary struct {
	ID                int      `json:"id"`
	Name              string   `json:"name"`
	DiseaseGroup      string   `json:"disease_group"`
	DiseaseSubGroup   string   `json:"disease_sub_group"`
	RelevantDisorders []string `json:"relevant_disorders"`
	Version           string   `json:"version"`
	Stats             struct {
		NumberOfGenes int32 `json:"number_of_genes"`
	} `json:"stats"`
}

// panelDetail is the per-panel /panels/{id}/ response (genes only).
type panelDetail struct {
	Genes []panelGene `json:"genes"`
}

type panelGene struct {
	ConfidenceLevel   string   `json:"confidence_level"`
	ModeOfInheritance string   `json:"mode_of_inheritance"`
	Phenotypes        []string `json:"phenotypes"`
	GeneData          struct {
		HgncID       string                                `json:"hgnc_id"`
		GeneSymbol   string                                `json:"gene_symbol"`
		OmimGene     []string                              `json:"omim_gene"`
		// PanelApp returns ensembl_genes as the nested build dict for most genes
		// but as a bare string ("") for some — decoding into a fixed map type would
		// fail the WHOLE panel's unmarshal and drop all its genes. Keep it raw and
		// parse defensively in extractGRCh38Ensembl.
		EnsemblGenes json.RawMessage `json:"ensembl_genes"`
	} `json:"gene_data"`
}

type ensemblBuildRef struct {
	EnsemblID string `json:"ensembl_id"`
}

// Token extractors for embedded ontology references in phenotype free-text.
// PanelApp embeds "OMIM:176000" (sometimes "OMIM: 620711") and "MONDO:0008294".
var (
	panelappOmimRe  = regexp.MustCompile(`OMIM:\s*(\d+)`)
	panelappMondoRe = regexp.MustCompile(`MONDO:\s*(\d+)`)
)

func (p *panelapp) update() {
	defer p.d.wg.Done()

	log.Println("PanelApp: Starting Genomics England panel processing...")
	startTime := time.Now()

	masterSourceID := config.Dataconf[p.source]["id"]
	childSource := "panelapp_gene"
	childSourceID := config.Dataconf[childSource]["id"]
	hasChild := childSourceID != ""
	if !hasChild {
		log.Printf("PanelApp: WARNING panelapp_gene not configured - child records will be skipped")
	}

	testLimit := config.GetTestLimit(p.source)
	var idLogFile, childIDLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		if hasChild {
			childIDLogFile = openIDLogFile(config.TestRefDir, childSource+"_ids.txt")
			if childIDLogFile != nil {
				defer childIDLogFile.Close()
			}
		}
		log.Printf("PanelApp: [TEST MODE] processing up to %d panels", testLimit)
	}

	baseURL := config.Dataconf[p.source]["path"]
	if baseURL == "" {
		baseURL = "https://panelapp.genomicsengland.co.uk/api/v1/panels/"
	}

	panels := p.fetchPanelList(baseURL, testLimit)
	log.Printf("PanelApp: resolved %d panels", len(panels))

	var panelCount, geneCount uint64
	for i := range panels {
		ps := &panels[i]
		panelID := fmt.Sprintf("%d", ps.ID)

		// MASTER record per panel.
		attr := &pbuf.PanelappAttr{
			Name:              ps.Name,
			DiseaseGroup:      ps.DiseaseGroup,
			DiseaseSubGroup:   ps.DiseaseSubGroup,
			RelevantDisorders: strings.Join(ps.RelevantDisorders, ", "),
			Version:           ps.Version,
			NumberOfGenes:     ps.Stats.NumberOfGenes,
		}
		b, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("PanelApp: error marshaling panel %s: %v", panelID, err)
			continue
		}
		p.d.addProp3(panelID, masterSourceID, b)

		// Text search: panel name -> master.
		if ps.Name != "" {
			p.d.addXref(ps.Name, textLinkID, panelID, p.source, true)
		}
		if idLogFile != nil {
			logProcessedID(idLogFile, panelID)
		}
		panelCount++

		if !hasChild {
			continue
		}

		// Fetch the panel's genes and emit one child per kept gene. Pace the
		// per-panel detail calls so the API doesn't throttle the 434-panel burst
		// (proactive pacing avoids the long retry backoffs).
		if !config.IsTestMode() {
			time.Sleep(150 * time.Millisecond)
		}
		genes := p.fetchPanelGenes(baseURL, ps.ID)
		for gi := range genes {
			if p.saveGene(ps, &genes[gi], childSource, childSourceID, masterSourceID, panelID, childIDLogFile) {
				geneCount++
			}
		}
	}

	atomic.AddUint64(&p.d.totalParsedEntry, panelCount+geneCount)
	log.Printf("PanelApp: Processing complete - %d panels, %d gene records (%.2fs)",
		panelCount, geneCount, time.Since(startTime).Seconds())
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
}

// fetchPanelList follows the paginated panel listing until `next` is null (or the
// test-mode panel cap is reached).
func (p *panelapp) fetchPanelList(baseURL string, testLimit int) []panelSummary {
	var out []panelSummary
	url := baseURL + "?page=1"
	for url != "" {
		resp, err := httpGetWithRetry(url, 3)
		if err != nil {
			log.Printf("PanelApp: WARNING fetching panel list %s: %v", url, err)
			break
		}
		body, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			log.Printf("PanelApp: WARNING reading panel list %s: %v", url, rerr)
			break
		}
		var page panelListResponse
		if err := json.Unmarshal(body, &page); err != nil {
			log.Printf("PanelApp: WARNING parsing panel list %s: %v", url, err)
			break
		}
		out = append(out, page.Results...)
		if testLimit > 0 && len(out) >= testLimit {
			out = out[:testLimit]
			break
		}
		url = page.Next
	}
	return out
}

// fetchPanelGenes returns the genes of a single panel.
func (p *panelapp) fetchPanelGenes(baseURL string, id int) []panelGene {
	url := fmt.Sprintf("%s%d/", baseURL, id)
	resp, err := httpGetWithRetry(url, 3)
	if err != nil {
		log.Printf("PanelApp: WARNING fetching panel %d genes: %v", id, err)
		return nil
	}
	body, rerr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if rerr != nil {
		log.Printf("PanelApp: WARNING reading panel %d genes: %v", id, rerr)
		return nil
	}
	var detail panelDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		log.Printf("PanelApp: WARNING parsing panel %d genes: %v", id, err)
		return nil
	}
	return detail.Genes
}

// saveGene emits one CHILD (panel, gene) record and its edges. Returns false when
// the gene is dropped (red/level-0 or missing symbol).
func (p *panelapp) saveGene(ps *panelSummary, g *panelGene, childSource, childSourceID, masterSourceID, panelID string, childIDLogFile *os.File) bool {
	confidence := mapPanelappConfidence(g.ConfidenceLevel)
	if confidence == "" { // keep only green (3) + amber (2)
		return false
	}
	symbol := strings.TrimSpace(g.GeneData.GeneSymbol)
	if symbol == "" {
		return false
	}

	geneID := panelID + "_" + symbol

	attr := &pbuf.PanelappGeneAttr{
		GeneSymbol:        symbol,
		PanelName:         ps.Name,
		Confidence:        confidence,
		ModeOfInheritance: g.ModeOfInheritance,
	}
	b, err := ffjson.Marshal(attr)
	if err != nil {
		log.Printf("PanelApp: error marshaling gene %s: %v", geneID, err)
		return false
	}
	p.d.addProp3(geneID, childSourceID, b)

	// MASTER -> CHILD: panel -> its gene records.
	p.d.addXref(panelID, masterSourceID, geneID, childSource, false)

	// gene -> hgnc
	if hgnc := strings.TrimSpace(g.GeneData.HgncID); strings.HasPrefix(hgnc, "HGNC:") {
		p.d.addXref(geneID, childSourceID, hgnc, "hgnc", false)
	}

	// gene -> ensembl (GRCh38 ENSG; tolerate missing / variant build keys)
	if ensg := extractGRCh38Ensembl(g.GeneData.EnsemblGenes); strings.HasPrefix(ensg, "ENSG") {
		p.d.addXref(geneID, childSourceID, ensg, "ensembl", false)
	}

	// gene -> mim : explicit omim_gene list + OMIM tokens parsed from phenotypes
	mimSeen := make(map[string]bool)
	addMim := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || !isAllDigits(s) || mimSeen[s] {
			return
		}
		mimSeen[s] = true
		p.d.addXref(geneID, childSourceID, s, "mim", false)
	}
	for _, om := range g.GeneData.OmimGene {
		addMim(om)
	}

	// gene -> mondo + gene -> mim (from free-text phenotype tokens)
	mondoSeen := make(map[string]bool)
	for _, ph := range g.Phenotypes {
		for _, m := range panelappOmimRe.FindAllStringSubmatch(ph, -1) {
			addMim(m[1])
		}
		for _, m := range panelappMondoRe.FindAllStringSubmatch(ph, -1) {
			id := "MONDO:" + m[1]
			if !mondoSeen[id] {
				mondoSeen[id] = true
				p.d.addXref(geneID, childSourceID, id, "mondo", false)
			}
		}
	}

	// Text search: gene symbol -> this gene record.
	p.d.addXref(symbol, textLinkID, geneID, childSource, true)

	if childIDLogFile != nil {
		logProcessedID(childIDLogFile, geneID)
	}
	return true
}

// mapPanelappConfidence maps PanelApp's confidence_level to biobtree's stored
// label, keeping only green (3) and amber (2). Returns "" for dropped levels.
func mapPanelappConfidence(level string) string {
	switch strings.TrimSpace(level) {
	case "3":
		return "green"
	case "2":
		return "amber"
	default:
		return ""
	}
}

// extractGRCh38Ensembl pulls the GRCh38 ENSG id out of PanelApp's nested
// ensembl_genes dict (ensembl_genes.<build>.<release>.ensembl_id), tolerating the
// "GRch38" key casing variants and any release number under it. Falls back to ""
// when no GRCh38 build is present.
func extractGRCh38Ensembl(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Tolerate the bare-string variant ("") PanelApp sometimes returns.
	var eg map[string]map[string]ensemblBuildRef
	if err := json.Unmarshal(raw, &eg); err != nil {
		return ""
	}
	for build, releases := range eg {
		if !strings.EqualFold(build, "GRCh38") {
			continue
		}
		for _, ref := range releases {
			if strings.HasPrefix(ref.EnsemblID, "ENSG") {
				return ref.EnsemblID
			}
		}
	}
	return ""
}
