package update

import (
	"archive/zip"
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// ncrnaInteraction ingests experimentally-supported ncRNA molecular interactions
// (NPInter v5: interaction_NPInterv5.txt.gz + .expr.txt.gz). Each row is a record
// entity with edges to the partner (uniprot for proteins / gene for RNA), the ncRNA
// gene, and the PubMed citation. The datasource field carries the evidence tier
// (literature mining / high-throughput / ...). This is the interaction/function
// layer that gives bare lncRNAs partners RNAcentral's Rfam/GO can't (Atlas #48).
//
// NPInter keys the ncRNA by NONCODE id; the gene is recovered from the ncRNA name
// via the lookup DB. Lookups are cached by name (names repeat heavily across the
// ~1.5M rows) so the number of actual lookups stays bounded.
type ncrnaInteraction struct {
	source     string
	sourceID   string
	d          *DataUpdate
	hgncID     uint32
	ensemblID  uint32
	uniprotID  uint32
	geneCache  map[string][]geneXref // name -> resolved hgnc/ensembl ids (lookup cache)
}

type geneXref struct {
	dataset string
	id      string
}

func (n *ncrnaInteraction) update() {
	defer n.d.wg.Done()

	n.sourceID = config.Dataconf[n.source]["id"]
	n.hgncID = config.DataconfIDStringToInt["hgnc"]
	n.ensemblID = config.DataconfIDStringToInt["ensembl"]
	n.uniprotID = config.DataconfIDStringToInt["uniprot"]
	n.geneCache = make(map[string][]geneXref)

	testLimit := config.GetTestLimit(n.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, n.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	// Main (literature + high-throughput) file, then the expression-derived file.
	for _, key := range []string{"path", "pathExpr"} {
		fp := config.Dataconf[n.source][key]
		if fp == "" {
			continue
		}
		c, err := n.processFile(fp, idLogFile, testLimit, &total)
		if err != nil {
			log.Printf("ncRNA Interaction: error processing %s: %v", fp, err)
		}
		fmt.Printf("ncRNA Interaction: %d interactions from %s\n", c, key)
		if testLimit > 0 && int(total) >= testLimit {
			break
		}
	}

	fmt.Printf("ncRNA Interaction: processed %d interactions total\n", total)
	atomic.AddUint64(&n.d.totalParsedEntry, total)
	n.d.progChan <- &progressInfo{dataset: n.source, done: true}
}

func (n *ncrnaInteraction) processFile(filePath string, idLogFile *os.File, testLimit int, total *uint64) (uint64, error) {
	reader, cleanup, err := n.openSource(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open ncRNA interaction file: %v", err)
	}
	defer cleanup()

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
		if line != "" {
			f := strings.Split(line, "\t")
			// skip the header row (only the base file has one)
			if len(f) > 0 && f[0] != "interID" {
				if n.processRow(f, idLogFile) {
					count++
					*total++
					if testLimit > 0 && int(*total) >= testLimit {
						break
					}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	return count, nil
}

// columns: 0 interID, 1 ncName, 2 ncID(NONCODE), 3 ncType, 4 tarName, 5 tarID,
// 6 tarType, 7 interDescription, 8 experiment, 9 reference(pubmed; ;-sep),
// 10 organism, 11 tissueOrCell, 12 tag, 13 class, 14 level, 15 datasource
func (n *ncrnaInteraction) processRow(f []string, idLogFile *os.File) bool {
	get := func(i int) string {
		if i < len(f) {
			return strings.TrimSpace(f[i])
		}
		return ""
	}

	interID := get(0)
	if interID == "" {
		return false
	}
	ncName := get(1)
	tarName := get(4)
	tarID := get(5)
	tarType := get(6)

	attr := &pbuf.NcrnaInteractionAttr{
		NcrnaName:        ncName,
		NcrnaType:        get(3),
		PartnerName:      tarName,
		PartnerType:      tarType,
		InteractionClass: get(13),
		Level:            get(14),
		Experiment:       get(8),
		Datasource:       get(15),
		Organism:         get(10),
		TissueOrCell:     get(11),
		Description:      get(7),
		Source:           "NPInter",
	}
	b, err := ffjson.Marshal(attr)
	if err != nil {
		return false
	}
	n.d.addProp3(interID, n.sourceID, b)

	// Partner edge: protein -> uniprot (tarID is the accession, no lookup needed);
	// RNA partner -> gene via cached name lookup.
	if strings.Contains(strings.ToLower(tarType), "protein") {
		if tarID != "" && isValidUniProtAccession(tarID) {
			n.d.addXref(interID, n.sourceID, tarID, "uniprot", false)
		}
	} else if tarName != "" {
		for _, gx := range n.resolveGene(tarName) {
			n.d.addXref(interID, n.sourceID, gx.id, gx.dataset, false)
		}
	}

	// ncRNA side: recover the gene from the ncRNA name so gene pages surface the interaction.
	for _, gx := range n.resolveGene(ncName) {
		n.d.addXref(interID, n.sourceID, gx.id, gx.dataset, false)
	}

	// PubMed citations (may be ;-separated). NPInter's reference column is not
	// always a PMID (prediction rows carry miRBase accessions / dataset ids), so
	// only numeric values are linked as pubmed (the pubmed bucket requires digits).
	for _, pmid := range strings.Split(get(9), ";") {
		pmid = strings.TrimSpace(pmid)
		if isAllDigits(pmid) {
			n.d.addXref(interID, n.sourceID, pmid, "pubmed", false)
		}
	}

	// Text search by ncRNA name
	if ncName != "" {
		n.d.addXref(ncName, textLinkID, interID, n.source, true)
	}

	if idLogFile != nil {
		logProcessedID(idLogFile, interID)
	}
	return true
}

// openSource fetches the NPInter file and returns a line reader. NPInter serves
// the download behind a trailing-slash redirect and (despite the .gz name) the body
// has been observed as both gzip and a single-entry ZIP across requests, so we
// download to a temp file and sniff the magic bytes (1f8b=gzip, 504b=zip).
func (n *ncrnaInteraction) openSource(url string) (*bufio.Reader, func(), error) {
	resp, err := httpGetWithRetry(url, 3)
	if err != nil {
		return nil, nil, err
	}
	tmp, err := os.CreateTemp("", "npinter-*.bin")
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}
	tmpName := tmp.Name()
	_, cErr := io.Copy(tmp, resp.Body)
	resp.Body.Close()
	tmp.Close()
	if cErr != nil {
		os.Remove(tmpName)
		return nil, nil, cErr
	}

	f, err := os.Open(tmpName)
	if err != nil {
		os.Remove(tmpName)
		return nil, nil, err
	}
	magic := make([]byte, 2)
	io.ReadFull(f, magic)
	f.Seek(0, 0)

	switch {
	case magic[0] == 0x1f && magic[1] == 0x8b: // gzip
		gz, gErr := gzip.NewReader(f)
		if gErr != nil {
			f.Close()
			os.Remove(tmpName)
			return nil, nil, gErr
		}
		return bufio.NewReaderSize(gz, 1024*1024), func() { gz.Close(); f.Close(); os.Remove(tmpName) }, nil
	case magic[0] == 0x50 && magic[1] == 0x4b: // zip (PK)
		f.Close()
		zr, zErr := zip.OpenReader(tmpName)
		if zErr != nil {
			os.Remove(tmpName)
			return nil, nil, zErr
		}
		var rc io.ReadCloser
		for _, zf := range zr.File {
			if strings.HasSuffix(zf.Name, ".txt") {
				rc, zErr = zf.Open()
				break
			}
		}
		if rc == nil && len(zr.File) > 0 {
			rc, zErr = zr.File[0].Open()
		}
		if zErr != nil || rc == nil {
			zr.Close()
			os.Remove(tmpName)
			return nil, nil, fmt.Errorf("no readable entry in zip")
		}
		return bufio.NewReaderSize(rc, 1024*1024), func() { rc.Close(); zr.Close(); os.Remove(tmpName) }, nil
	default: // plain text
		return bufio.NewReaderSize(f, 1024*1024), func() { f.Close(); os.Remove(tmpName) }, nil
	}
}

// resolveGene maps a name to hgnc/ensembl ids via the lookup DB, caching by name
// (names repeat heavily across rows, keeping the lookup count bounded).
func (n *ncrnaInteraction) resolveGene(name string) []geneXref {
	if n.d.lookupService == nil || name == "" || name == "-" {
		return nil
	}
	if cached, ok := n.geneCache[name]; ok {
		return cached
	}

	var out []geneXref
	result, err := n.d.lookup(name)
	if err == nil && result != nil {
		seen := make(map[string]bool)
		add := func(ds, id string) {
			if id == "" {
				return
			}
			key := ds + "\t" + id
			if seen[key] {
				return
			}
			seen[key] = true
			out = append(out, geneXref{dataset: ds, id: id})
		}
		classify := func(dsID uint32, ident string) {
			switch dsID {
			case n.hgncID:
				add("hgnc", ident)
			case n.ensemblID:
				add("ensembl", ident)
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
	n.geneCache[name] = out // cache even empty results to avoid repeat lookups
	return out
}
