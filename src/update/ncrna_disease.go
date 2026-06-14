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

// ncrnaDisease ingests curated ncRNA->disease associations (LncRNADisease v3.0,
// website_alldata.tsv — experimentally supported, lncRNA + circRNA). Each row is
// a record entity with edges to the ncRNA gene (hgnc/ensembl), the disease
// (mondo/efo, via the shared matcher) and the supporting PubMed citation. This is
// the disease layer that RNAcentral's Rfam/GO cannot give bare lncRNAs (Atlas #48).
type ncrnaDisease struct {
	source   string
	sourceID string
	d        *DataUpdate
	medical  *MedicalTermMappings
}

func (n *ncrnaDisease) update() {
	defer n.d.wg.Done()

	n.sourceID = config.Dataconf[n.source]["id"]
	n.medical = LoadMedicalTermMappings()

	testLimit := config.GetTestLimit(n.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, n.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	filePath := config.Dataconf[n.source]["path"]
	count, err := n.processFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Printf("ncRNA Disease: error processing %s: %v", filePath, err)
	}
	fmt.Printf("ncRNA Disease: processed %d LncRNADisease associations\n", count)

	// HMDD v4 — miRNA->disease, folded into this dataset with source="HMDD".
	var hmddCount uint64
	if hmddPath := config.Dataconf[n.source]["pathHmdd"]; hmddPath != "" {
		hmddCount, err = n.processHmddFile(hmddPath, idLogFile, testLimit)
		if err != nil {
			log.Printf("ncRNA Disease: error processing HMDD %s: %v", hmddPath, err)
		}
		fmt.Printf("ncRNA Disease: processed %d HMDD (miRNA) associations\n", hmddCount)
	}

	atomic.AddUint64(&n.d.totalParsedEntry, count+hmddCount)
	n.d.progChan <- &progressInfo{dataset: n.source, done: true}
}

func (n *ncrnaDisease) processFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(n.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open ncRNA-disease file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024)
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024)
	}

	hgncID := config.DataconfIDStringToInt["hgnc"]
	ensemblID := config.DataconfIDStringToInt["ensembl"]
	mondoID := config.DataconfIDStringToInt["mondo"]
	efoID := config.DataconfIDStringToInt["efo"]

	var count uint64
	header := true

	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return count, fmt.Errorf("read error: %v", rerr)
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		if header {
			header = false // skip the column header row
		} else if line != "" {
			f := strings.Split(line, "\t")
			if n.processRow(f, mondoID, efoID, hgncID, ensemblID, idLogFile) {
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

// columns: 0 Symbol, 1 Category, 2 Species, 3 Disease, 4 Sample, 5 Dysfunction,
// 6 Validated Method, 7 Description, 8 Clinical Application, 9 Causality,
// 10 Causal Description, 11 PubMed ID
func (n *ncrnaDisease) processRow(f []string, mondoID, efoID, hgncID, ensemblID uint32, idLogFile *os.File) bool {
	get := func(i int) string {
		if i < len(f) {
			return strings.TrimSpace(f[i])
		}
		return ""
	}

	symbol := get(0)
	disease := get(3)
	if symbol == "" || disease == "" {
		return false
	}

	attr := &pbuf.NcrnaDiseaseAttr{
		NcrnaSymbol:         symbol,
		NcrnaCategory:       get(1),
		Species:             get(2),
		DiseaseName:         disease,
		DysfunctionPattern:  get(5),
		ValidatedMethod:     get(6),
		Description:         get(7),
		ClinicalApplication: get(8),
		Causality:           get(9),
		Source:              "LncRNADisease",
	}

	// Deterministic id (stable across re-downloads; references are via edges anyway).
	h := fnv.New64a()
	h.Write([]byte(symbol + "|" + disease + "|" + get(11) + "|" + get(6)))
	id := fmt.Sprintf("LNCRD_%016x", h.Sum64())

	b, err := ffjson.Marshal(attr)
	if err != nil {
		return false
	}
	n.d.addProp3(id, n.sourceID, b)

	// ncRNA gene -> hgnc / ensembl (lncRNA genes are in HGNC/Ensembl)
	n.linkGene(id, []string{symbol}, hgncID, ensemblID)

	// disease name -> MONDO / EFO via the shared matcher
	n.mapDisease(id, disease, mondoID, efoID)

	// PubMed citation
	if pmid := get(11); pmid != "" && pmid != "nan" {
		n.d.addXref(id, n.sourceID, pmid, "pubmed", false)
	}

	// Text search by ncRNA symbol + disease name
	n.d.addXref(symbol, textLinkID, id, n.source, true)
	n.d.addXref(disease, textLinkID, id, n.source, true)

	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}
	return true
}

// mapDisease maps a disease name to MONDO/EFO via the shared matcher (same as
// clinical_trials/civic), trying comma de-inverted MeSH variants too.
func (n *ncrnaDisease) mapDisease(id, disease string, mondoID, efoID uint32) {
	candidates := diseaseNameVariants(disease)
	mapTo := func(ontID uint32, ds string) {
		if ontID == 0 {
			return
		}
		seen := make(map[string]bool)
		for _, dc := range candidates {
			for o := range collectOntologyIDs(n.d, n.medical, dc, ontID) {
				if !seen[o] {
					seen[o] = true
					n.d.addXref(id, n.sourceID, o, ds, false)
				}
			}
		}
	}
	mapTo(mondoID, "mondo")
	mapTo(efoID, "efo")
}

// processHmddFile ingests HMDD v4 (miRNA->disease) into this dataset with
// source="HMDD". Columns: code, PMID, miRNA, disease, description.
func (n *ncrnaDisease) processHmddFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(n.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open HMDD file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024)
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024)
	}

	hgncID := config.DataconfIDStringToInt["hgnc"]
	ensemblID := config.DataconfIDStringToInt["ensembl"]
	mondoID := config.DataconfIDStringToInt["mondo"]
	efoID := config.DataconfIDStringToInt["efo"]

	var count uint64
	header := true
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return count, fmt.Errorf("read error: %v", rerr)
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if header {
			header = false
		} else if line != "" {
			f := strings.Split(line, "\t")
			if n.processHmddRow(f, mondoID, efoID, hgncID, ensemblID, idLogFile) {
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

// columns: 0 code(evidence category), 1 PMID, 2 miRNA, 3 disease, 4 description
func (n *ncrnaDisease) processHmddRow(f []string, mondoID, efoID, hgncID, ensemblID uint32, idLogFile *os.File) bool {
	get := func(i int) string {
		if i < len(f) {
			return strings.TrimSpace(f[i])
		}
		return ""
	}
	mirna := get(2)
	disease := get(3)
	if mirna == "" || disease == "" {
		return false
	}

	attr := &pbuf.NcrnaDiseaseAttr{
		NcrnaSymbol:     mirna,
		NcrnaCategory:   "miRNA",
		Species:         "Homo sapiens", // HMDD is human
		DiseaseName:     disease,
		ValidatedMethod: get(0), // HMDD evidence category (genetics/epigenetics/tissue/...)
		Description:     get(4),
		Source:          "HMDD",
	}
	h := fnv.New64a()
	h.Write([]byte("hmdd|" + mirna + "|" + disease + "|" + get(1)))
	id := fmt.Sprintf("HMDD_%016x", h.Sum64())

	b, err := ffjson.Marshal(attr)
	if err != nil {
		return false
	}
	n.d.addProp3(id, n.sourceID, b)

	n.linkGene(id, mirnaGeneCandidates(mirna), hgncID, ensemblID)
	n.mapDisease(id, disease, mondoID, efoID)

	if pmid := get(1); pmid != "" && pmid != "nan" {
		n.d.addXref(id, n.sourceID, pmid, "pubmed", false)
	}
	n.d.addXref(mirna, textLinkID, id, n.source, true)
	n.d.addXref(disease, textLinkID, id, n.source, true)

	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}
	return true
}

// mirnaGeneCandidates derives lookup candidates from a miRBase name so the miRNA
// can resolve to its HGNC/Ensembl gene (best-effort): the raw name plus a gene-style
// form ("hsa-mir-29b" -> "MIR29B", "hsa-let-7a" -> "MIRLET7A").
func mirnaGeneCandidates(name string) []string {
	out := []string{name}
	s := strings.ToUpper(strings.TrimSpace(name))
	if i := strings.Index(s, "-"); i >= 0 && i <= 4 { // strip species prefix (HSA-, MMU-, ...)
		s = s[i+1:]
	}
	s = strings.ReplaceAll(s, "-", "")
	if strings.HasPrefix(s, "LET") {
		s = "MIR" + s
	}
	if s != "" && s != strings.ToUpper(name) {
		out = append(out, s)
	}
	return out
}

// diseaseNameVariants returns the disease name plus, for comma-inverted MeSH names
// ("Carcinoma, Hepatocellular"), the de-inverted form ("Hepatocellular Carcinoma").
func diseaseNameVariants(name string) []string {
	out := []string{name}
	if strings.Contains(name, ", ") {
		parts := strings.Split(name, ", ")
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
		if deinv := strings.Join(parts, " "); deinv != name {
			out = append(out, deinv)
		}
	}
	return out
}

// linkGene resolves ncRNA name candidate(s) to hgnc/ensembl via the lookup DB and
// links the association to them, so gene pages surface the disease and disease pages
// surface the ncRNA. Multiple candidates support miRNA name normalization
// (e.g. "hsa-mir-29b" -> "MIR29B").
func (n *ncrnaDisease) linkGene(id string, names []string, hgncID, ensemblID uint32) {
	if n.d.lookupService == nil {
		return
	}
	seen := make(map[string]bool)
	add := func(ds, val string) {
		if val == "" {
			return
		}
		key := ds + "\t" + val
		if seen[key] {
			return
		}
		seen[key] = true
		n.d.addXref(id, n.sourceID, val, ds, false)
	}
	classify := func(dsID uint32, ident string) {
		switch dsID {
		case hgncID:
			add("hgnc", ident)
		case ensemblID:
			add("ensembl", ident)
		}
	}
	tried := make(map[string]bool)
	for _, name := range names {
		if name == "" || tried[name] {
			continue
		}
		tried[name] = true
		result, err := n.d.lookup(name)
		if err != nil || result == nil {
			continue
		}
		for _, x := range result.Results {
			if x.IsLink {
				for _, e := range x.Entries {
					classify(e.Dataset, e.Identifier)
				}
			} else {
				classify(x.Dataset, x.Identifier)
			}
		}
	}
}
