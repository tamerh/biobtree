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

// ncrnaDrug ingests curated ncRNA <-> drug associations (ncRNADrug): drug-resistance
// (DR_Curated) and drug-target (DT_Curated). Each row is a record entity with edges
// to the ncRNA gene (ensembl/hgnc), the drug (drugbank / pubchem / chembl_molecule),
// the targeted gene, and pubmed. ncRNADrug provides ENSEMBL_ID + DrugBank_ID + CID
// directly, so most edges need no lookup. (Atlas #48 follow-on.)
type ncrnaDrug struct {
	source      string
	sourceID    string
	d           *DataUpdate
	hgncID      uint32
	ensemblID   uint32
	chemblMolID uint32
	geneCache   map[string][]geneXref // name -> hgnc/ensembl (lookup cache)
	drugCache   map[string][]string   // drug name -> chembl_molecule ids (lookup cache)
}

func (n *ncrnaDrug) update() {
	defer n.d.wg.Done()

	n.sourceID = config.Dataconf[n.source]["id"]
	n.hgncID = config.DataconfIDStringToInt["hgnc"]
	n.ensemblID = config.DataconfIDStringToInt["ensembl"]
	n.chemblMolID = config.DataconfIDStringToInt["chembl_molecule"]
	n.geneCache = make(map[string][]geneXref)
	n.drugCache = make(map[string][]string)

	testLimit := config.GetTestLimit(n.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, n.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	for _, fc := range []struct{ key, relation string }{
		{"path", "drug_resistance"},
		{"pathDT", "drug_target"},
	} {
		fp := config.Dataconf[n.source][fc.key]
		if fp == "" {
			continue
		}
		c, err := n.processFile(fp, fc.relation, idLogFile, testLimit, &total)
		if err != nil {
			log.Printf("ncRNA Drug: error processing %s: %v", fp, err)
		}
		fmt.Printf("ncRNA Drug: %d %s associations\n", c, fc.relation)
		if testLimit > 0 && int(total) >= testLimit {
			break
		}
	}

	fmt.Printf("ncRNA Drug: processed %d associations total\n", total)
	atomic.AddUint64(&n.d.totalParsedEntry, total)
	n.d.progChan <- &progressInfo{dataset: n.source, done: true}
}

func (n *ncrnaDrug) processFile(filePath, relation string, idLogFile *os.File, testLimit int, total *uint64) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(n.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open ncRNA-drug file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024)
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024)
	}

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
			if n.processRow(f, relation, idLogFile) {
				count++
				*total++
				if testLimit > 0 && int(*total) >= testLimit {
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

// columns: 0 PMID, 2 ncRNA_Name, 3 ENSEMBL_ID, 4 SYMBOL, 10 ncRNA_Type, 11 Drug_Name,
// 12 DrugBank_ID, 13 CID(PubChem), 15 FDA, 16 ncRNA_Target_Gene, 17 Pathway,
// 18 Effect/Expression, 19 Detection_Method, 23 Condition
func (n *ncrnaDrug) processRow(f []string, relation string, idLogFile *os.File) bool {
	get := func(i int) string {
		if i < len(f) {
			s := strings.TrimSpace(f[i])
			if s == "NA" {
				return ""
			}
			return s
		}
		return ""
	}

	ncName := get(2)
	drug := get(11)
	if ncName == "" || drug == "" {
		return false
	}

	attr := &pbuf.NcrnaDrugAttr{
		NcrnaName:       ncName,
		NcrnaType:       get(10),
		Symbol:          get(4),
		DrugName:        drug,
		DrugbankId:      get(12),
		Relation:        relation,
		Effect:          get(18),
		TargetGene:      get(16),
		Pathway:         get(17),
		Fda:             get(15),
		DetectionMethod: get(19),
		Condition:       get(23),
		Source:          "ncRNADrug",
	}
	h := fnv.New64a()
	h.Write([]byte(relation + "|" + get(0) + "|" + ncName + "|" + drug + "|" + get(16)))
	id := fmt.Sprintf("NCDRUG_%016x", h.Sum64())

	b, err := ffjson.Marshal(attr)
	if err != nil {
		return false
	}
	n.d.addProp3(id, n.sourceID, b)

	// ncRNA gene: ENSEMBL_ID is given directly; SYMBOL -> hgnc via cached lookup.
	if ens := get(3); strings.HasPrefix(ens, "ENSG") {
		n.d.addXref(id, n.sourceID, ens, "ensembl", false)
	}
	if sym := get(4); sym != "" {
		for _, gx := range n.resolveGene(sym) {
			n.d.addXref(id, n.sourceID, gx.id, gx.dataset, false)
		}
	}

	// Drug: DrugBank_ID + CID given directly; Drug_Name -> chembl_molecule via lookup.
	if db := get(12); db != "" && strings.HasPrefix(db, "DB") {
		n.d.addXref(id, n.sourceID, db, "drugbank", false)
	}
	if cid := get(13); cid != "" && isAllDigits(cid) {
		n.d.addXref(id, n.sourceID, cid, "pubchem", false)
	}
	for _, mol := range n.resolveDrug(drug) {
		n.d.addXref(id, n.sourceID, mol, "chembl_molecule", false)
	}

	// Gene the ncRNA targets
	if tg := get(16); tg != "" {
		for _, gx := range n.resolveGene(tg) {
			n.d.addXref(id, n.sourceID, gx.id, gx.dataset, false)
		}
	}

	// PubMed
	if pmid := get(0); pmid != "" {
		n.d.addXref(id, n.sourceID, pmid, "pubmed", false)
	}

	// Text search by ncRNA name + drug name
	n.d.addXref(ncName, textLinkID, id, n.source, true)
	n.d.addXref(drug, textLinkID, id, n.source, true)

	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}
	return true
}

func (n *ncrnaDrug) resolveGene(name string) []geneXref {
	if n.d.lookupService == nil || name == "" {
		return nil
	}
	if c, ok := n.geneCache[name]; ok {
		return c
	}
	var out []geneXref
	if result, err := n.d.lookup(name); err == nil && result != nil {
		seen := make(map[string]bool)
		classify := func(dsID uint32, ident string) {
			var ds string
			switch dsID {
			case n.hgncID:
				ds = "hgnc"
			case n.ensemblID:
				ds = "ensembl"
			default:
				return
			}
			key := ds + "\t" + ident
			if ident != "" && !seen[key] {
				seen[key] = true
				out = append(out, geneXref{dataset: ds, id: ident})
			}
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
	n.geneCache[name] = out
	return out
}

func (n *ncrnaDrug) resolveDrug(name string) []string {
	if n.d.lookupService == nil || name == "" {
		return nil
	}
	if c, ok := n.drugCache[name]; ok {
		return c
	}
	var out []string
	if result, err := n.d.lookup(name); err == nil && result != nil {
		seen := make(map[string]bool)
		consider := func(dsID uint32, ident string) {
			if dsID == n.chemblMolID && ident != "" && !seen[ident] {
				seen[ident] = true
				out = append(out, ident)
			}
		}
		for _, x := range result.Results {
			if x.IsLink {
				for _, e := range x.Entries {
					consider(e.Dataset, e.Identifier)
				}
			} else {
				consider(x.Dataset, x.Identifier)
			}
		}
	}
	n.drugCache[name] = out
	return out
}
