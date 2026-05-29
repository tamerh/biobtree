package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// cellosaurus ingests the Cellosaurus cell-line knowledge resource (SIB, CC BY
// 4.0). The flat file is in literal UniProt line format (entries terminated by
// "//"), so this parser mirrors the UniProt/GenCC style.
//
// Cross-references are split: resources that are biobtree datasets become graph
// edges (with per-resource ID normalisation); every other DR resource is kept
// in the `external_refs` attribute (nothing is skipped). The high-value gene /
// variant / anatomy links live in CC lines (Sequence variation -> HGNC/ClinVar,
// Derived from site -> UBERON, Cell type -> CL, mAb target -> UniProtKB), which
// are mined here; the raw CC text is also stored.
type cellosaurus struct {
	source       string
	d            *DataUpdate
	mappings     *MedicalTermMappings
	mondoInt     uint32
	diseaseMondo map[string][]string // disease name -> MONDO ids (cache)
}

func (c *cellosaurus) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// CC-embedded reference patterns.
var (
	reCvclHGNC    = regexp.MustCompile(`HGNC:(\d+)`)
	reCvclClinVar = regexp.MustCompile(`ClinVar=VCV0*(\d+)`)
	reCvcldbSNP   = regexp.MustCompile(`dbSNP=(rs\d+)`)
	reCvclUBERON  = regexp.MustCompile(`UBERON=UBERON_(\d+)`)
	reCvclCL      = regexp.MustCompile(`CL=CL_(\d+)`)
	reCvclChEBI   = regexp.MustCompile(`CHEBI:(\d+)`)
	reCvclUniProt = regexp.MustCompile(`UniProtKB; ([A-Z0-9]+)`)
)

// DR resources that map to an existing biobtree dataset, with the target
// dataset name. Resources NOT here are stored in external_refs.
// Value-format normalisation is applied per resource in handleDR.
var cvclDRDataset = map[string]string{
	"Cosmic":         "cosmic",
	"EFO":            "efo",
	"MeSH":           "mesh",
	"ChEMBL-Cells":   "chembl_cell_line",
	"ChEMBL-Targets": "chembl_target",
}

func (c *cellosaurus) mondoForDisease(name string) []string {
	if c.mondoInt == 0 || name == "" {
		return nil
	}
	if ids, ok := c.diseaseMondo[name]; ok {
		return ids
	}
	ids := mapBoolKeys(collectOntologyIDs(c.d, c.mappings, name, c.mondoInt))
	c.diseaseMondo[name] = ids
	return ids
}

func (c *cellosaurus) update() {
	defer c.d.wg.Done()
	log.Println("Cellosaurus: starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[c.source]["id"]
	path := config.Dataconf[c.source]["path"]
	c.mappings = LoadMedicalTermMappings()
	c.diseaseMondo = map[string][]string{}
	if id, ok := config.Dataconf["mondo"]["id"]; ok {
		fmt.Sscanf(id, "%d", &c.mondoInt)
	}

	resp, err := http.Get(path)
	c.check(err, "downloading cellosaurus")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.check(fmt.Errorf("unexpected HTTP %d for %s", resp.StatusCode, path), "downloading cellosaurus")
	}

	scanner := bufio.NewScanner(bufio.NewReaderSize(resp.Body, fileBufSize))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var total uint64
	cur := newCvclEntry()
	for scanner.Scan() {
		line := scanner.Text()
		if line == "//" {
			if cur.ac != "" {
				c.emit(cur, sourceID)
				total++
			}
			cur = newCvclEntry()
			continue
		}
		if len(line) < 5 {
			continue
		}
		code, val := line[:2], strings.TrimSpace(line[5:])
		cur.add(code, val)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Cellosaurus: scan error: %v", err)
	}
	if cur.ac != "" { // file may not end with a trailing //
		c.emit(cur, sourceID)
		total++
	}

	if total == 0 {
		c.check(fmt.Errorf("no cell lines parsed - source file may have moved"), "cellosaurus")
	}

	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
	atomic.AddUint64(&c.d.totalParsedEntry, total)
	log.Printf("Cellosaurus: complete - %d cell lines (%.2fs)", total, time.Since(startTime).Seconds())
}

type cvclEntry struct {
	ac        string
	name      string
	synonyms  []string
	sex       string
	age       string
	category  string
	diseases  []string // "NCIt:Cxxxx=name"
	species   []string // "9606=Homo sapiens (Human)"
	parent    []string // CVCL_ ids
	sameInd   []string // CVCL_ ids
	extRefs   []string // "Resource:id"
	comments  []string // raw CC
	di        [][3]string
	ox        []string // taxids
	dr        [][2]string // (resource, id)
	rx        []string // raw RX values
}

func newCvclEntry() *cvclEntry { return &cvclEntry{} }

func (e *cvclEntry) add(code, val string) {
	switch code {
	case "AC":
		if e.ac == "" {
			e.ac = val
		}
	case "ID":
		if e.name == "" {
			e.name = val
		}
	case "SY":
		for _, s := range strings.Split(val, ";") {
			if s = strings.TrimSpace(s); s != "" {
				e.synonyms = append(e.synonyms, s)
			}
		}
	case "SX":
		e.sex = val
	case "AG":
		e.age = val
	case "CA":
		e.category = val
	case "OX":
		// "NCBI_TaxID=9606; ! Homo sapiens (Human)"
		if strings.HasPrefix(val, "NCBI_TaxID=") {
			rest := strings.TrimPrefix(val, "NCBI_TaxID=")
			taxid := rest
			label := ""
			if i := strings.Index(rest, ";"); i >= 0 {
				taxid = strings.TrimSpace(rest[:i])
				if j := strings.Index(rest, "!"); j >= 0 {
					label = strings.TrimSpace(rest[j+1:])
				}
			}
			e.ox = append(e.ox, taxid)
			e.species = append(e.species, taxid+"="+label)
		}
	case "DI":
		// "NCIt; C4878; Lung carcinoma"
		parts := strings.SplitN(val, ";", 3)
		if len(parts) == 3 {
			src := strings.TrimSpace(parts[0])
			cd := strings.TrimSpace(parts[1])
			nm := strings.TrimSpace(parts[2])
			e.di = append(e.di, [3]string{src, cd, nm})
			e.diseases = append(e.diseases, src+":"+cd+"="+nm)
		}
	case "DR":
		// "Resource; id"
		if i := strings.Index(val, ";"); i >= 0 {
			res := strings.TrimSpace(val[:i])
			id := strings.TrimSpace(val[i+1:])
			if res != "" && id != "" {
				e.dr = append(e.dr, [2]string{res, id})
			}
		}
	case "RX":
		e.rx = append(e.rx, val)
	case "HI":
		if cv := cvclID(val); cv != "" {
			e.parent = append(e.parent, cv)
		}
	case "OI":
		if cv := cvclID(val); cv != "" {
			e.sameInd = append(e.sameInd, cv)
		}
	case "CC":
		e.comments = append(e.comments, val)
	}
}

// cvclID extracts a leading CVCL_ accession from "CVCL_xxxx ! name".
func cvclID(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "CVCL_") {
		return ""
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

func (c *cellosaurus) emit(e *cvclEntry, sourceID string) {
	ac := e.ac

	// Sets to dedup edges discovered across multiple lines.
	hgnc := map[string]bool{}
	clinvar := map[string]bool{}
	dbsnp := map[string]bool{}
	uberon := map[string]bool{}
	cl := map[string]bool{}
	chebi := map[string]bool{}
	uniprot := map[string]bool{}

	// Mine CC lines for embedded references (the bulk of the gene/anatomy value).
	for _, cc := range e.comments {
		for _, m := range reCvclHGNC.FindAllStringSubmatch(cc, -1) {
			hgnc["HGNC:"+m[1]] = true
		}
		for _, m := range reCvclClinVar.FindAllStringSubmatch(cc, -1) {
			clinvar[m[1]] = true
		}
		for _, m := range reCvcldbSNP.FindAllStringSubmatch(cc, -1) {
			dbsnp[m[1]] = true
		}
		for _, m := range reCvclUBERON.FindAllStringSubmatch(cc, -1) {
			uberon["UBERON:"+m[1]] = true
		}
		for _, m := range reCvclCL.FindAllStringSubmatch(cc, -1) {
			cl["CL:"+m[1]] = true
		}
		for _, m := range reCvclChEBI.FindAllStringSubmatch(cc, -1) {
			chebi["CHEBI:"+m[1]] = true
		}
		for _, m := range reCvclUniProt.FindAllStringSubmatch(cc, -1) {
			uniprot[m[1]] = true
		}
	}

	attr := pbuf.CellosaurusAttr{
		Name:           e.name,
		Synonyms:       e.synonyms,
		Sex:            e.sex,
		Age:            e.age,
		Category:       e.category,
		Diseases:       e.diseases,
		Species:        e.species,
		Parent:         e.parent,
		SameIndividual: e.sameInd,
		Comments:       e.comments,
	}

	// Species -> taxonomy
	for _, taxid := range e.ox {
		if isAllDigits(taxid) {
			c.d.addXref(ac, sourceID, taxid, "taxonomy", false)
		}
	}

	// Disease (DI): ORDO -> orphanet (bare numeric); both -> MONDO via name; text.
	for _, di := range e.di {
		src, code, name := di[0], di[1], di[2]
		if src == "ORDO" {
			ordo := strings.TrimPrefix(code, "Orphanet_")
			if isAllDigits(ordo) {
				c.d.addXref(ac, sourceID, ordo, "orphanet", false)
			}
		}
		if name != "" {
			c.d.addXref(name, textLinkID, ac, c.source, true)
			for _, mid := range c.mondoForDisease(name) {
				c.d.addXref(ac, sourceID, mid, "mondo", false)
			}
		}
	}

	// DR: matched resources -> edges; everything else -> external_refs.
	for _, dr := range e.dr {
		res, id := dr[0], dr[1]
		if ds, ok := cvclDRDataset[res]; ok {
			tid := id
			if res == "EFO" { // EFO_0001185 -> EFO:0001185
				tid = strings.Replace(id, "_", ":", 1)
			}
			c.d.addXref(ac, sourceID, tid, ds, false)
		} else {
			attr.ExternalRefs = append(attr.ExternalRefs, res+":"+id)
		}
	}

	// RX: PubMed / DOI / Patent
	for _, rx := range e.rx {
		switch {
		case strings.HasPrefix(rx, "PubMed="):
			if pmid := strings.TrimRight(strings.TrimPrefix(rx, "PubMed="), ";"); isAllDigits(pmid) {
				c.d.addXref(ac, sourceID, pmid, "pubmed", false)
			}
		case strings.HasPrefix(rx, "DOI="):
			if doi := strings.TrimRight(strings.TrimPrefix(rx, "DOI="), ";"); doi != "" {
				c.d.addXref(ac, sourceID, doi, "doi", false)
			}
		case strings.HasPrefix(rx, "Patent="):
			if pt := strings.TrimRight(strings.TrimPrefix(rx, "Patent="), ";"); pt != "" {
				c.d.addXref(ac, sourceID, pt, "patent", false)
			}
		}
	}

	// CC-mined edges
	for v := range hgnc {
		c.d.addXref(ac, sourceID, v, "hgnc", false)
	}
	for v := range clinvar {
		c.d.addXref(ac, sourceID, v, "clinvar", false)
	}
	for v := range dbsnp {
		c.d.addXref(ac, sourceID, v, "dbsnp", false)
	}
	for v := range uberon {
		c.d.addXref(ac, sourceID, v, "uberon", false)
	}
	for v := range cl {
		c.d.addXref(ac, sourceID, v, "cl", false)
	}
	for v := range chebi {
		c.d.addXref(ac, sourceID, v, "chebi", false)
	}
	for v := range uniprot {
		c.d.addXref(ac, sourceID, v, "uniprot", false)
	}

	// Hierarchy self-edges (CVCL -> CVCL)
	for _, p := range e.parent {
		c.d.addXref(ac, sourceID, p, "cellosaurus", false)
	}
	for _, o := range e.sameInd {
		c.d.addXref(ac, sourceID, o, "cellosaurus", false)
	}

	// Text search: name + synonyms
	if e.name != "" {
		c.d.addXref(e.name, textLinkID, ac, c.source, true)
	}
	for _, s := range e.synonyms {
		c.d.addXref(s, textLinkID, ac, c.source, true)
	}

	b, err := ffjson.Marshal(&attr)
	if err != nil {
		return
	}
	c.d.addProp3(ac, sourceID, b)
}
