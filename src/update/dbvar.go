package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// dbvar ingests the clinical dbVar structural-variant calls from the GRCh38
// VCF release. One entry per dbVar call (nssv...), with phenotype/ClinVar/
// PubMed xrefs and an interval index for point-in-SV queries.
//
// Source files are the four clinical subsets:
//
//	GRCh38.variant_call.clinical.pathogenic_or_likely_pathogenic.vcf.gz
//	GRCh38.variant_call.clinical.uncertain_significance.vcf.gz
//	GRCh38.variant_call.clinical.conflicting_or_other.vcf.gz
//	GRCh38.variant_call.clinical.benign_or_likely_benign.vcf.gz
//
// The full variant_call archive (527 MB) is intentionally not ingested here.
type dbvar struct {
	source string
	d      *DataUpdate
}

func (v *dbvar) check(err error, operation string) {
	checkWithContext(err, v.source, operation)
}

func (v *dbvar) update() {
	defer v.d.wg.Done()

	log.Println("dbVar: Starting data processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[v.source]["id"]
	base := config.Dataconf[v.source]["path"]
	files := config.Dataconf[v.source]["files"]
	testLimit := config.GetTestLimit(v.source)

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, v.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("dbVar: [TEST MODE] Processing up to %d records", testLimit)
	}

	var total int64
	var previous int64
	for _, file := range strings.Split(files, ",") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if testLimit > 0 && total >= int64(testLimit) {
			break
		}
		url := strings.TrimRight(base, "/") + "/" + file
		n := v.parseFile(url, sourceID, testLimit-int(total), idLogFile)
		total += int64(n)

		elapsed := int64(time.Since(startTime).Seconds())
		if elapsed > previous+v.d.progInterval {
			previous = elapsed
			v.d.progChan <- &progressInfo{dataset: v.source, currentKBPerSec: 0}
		}
	}

	v.d.progChan <- &progressInfo{dataset: v.source, done: true}
	atomic.AddUint64(&v.d.totalParsedEntry, uint64(total))
	log.Printf("dbVar: %d clinical SV records (%.1fs)", total, time.Since(startTime).Seconds())
}

func (v *dbvar) parseFile(path, sourceID string, remaining int, idLogFile *os.File) int {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(v.source, "", "", path)
	if err != nil {
		v.check(err, "opening dbVar VCF")
		return 0
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var count int
	for scanner.Scan() {
		if remaining > 0 && count >= remaining {
			break
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 8 {
			continue
		}
		id := ""
		info := parseVCFInfo(f[7])
		if v, ok := info["DBVARID"]; ok {
			id = v
		}
		if id == "" && f[2] != "." {
			id = f[2]
		}
		if id == "" {
			continue
		}
		pos, _ := strconv.ParseInt(f[1], 10, 64)
		end := pos
		if s := info["END"]; s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
				end = n
			}
		}
		if end < pos {
			end = pos
		}
		svtype := strings.ToUpper(strings.TrimSpace(info["SVTYPE"]))
		if svtype == "" {
			svtype = altType(f[4])
		}
		svlen := info["SVLEN"]
		clnsig := cleanVCFString(info["CLNSIG"])
		origin := cleanVCFString(info["ORIGIN"])
		desc := cleanVCFString(info["DESC"])
		if u, uerr := url.QueryUnescape(desc); uerr == nil {
			desc = u
		}
		regionID := info["REGIONID"]
		_, somatic := info["SOMATIC"]

		attr := &pbuf.DbvarAttr{
			Chromosome:           normalizeChrom(f[0]),
			Start:                pos,
			End:                  end,
			Svtype:               svtype,
			Svlen:                svlen,
			ClinicalSignificance: clnsig,
			Origin:               origin,
			PhenotypeIds:         splitCSV(info["PHENO"]),
			ClinvarAccessions:    splitCSV(info["CLNACC"]),
			RegionId:             regionID,
			Description:          desc,
			Somatic:              somatic,
		}
		if b, merr := ffjson.Marshal(attr); merr == nil {
			v.d.addProp3(id, sourceID, b)
		} else {
			log.Printf("dbVar: marshal %s: %v", id, merr)
			continue
		}

		v.d.addInterval(id, v.source, attr.Chromosome, pos, end)

		// ClinVar: VCV accessions are the stable variation ids.
		for _, acc := range attr.ClinvarAccessions {
			if vid := clinvarVariationID(acc); vid != "" {
				v.d.addXref(id, sourceID, vid, "clinvar", false)
			}
		}
		// Phenotype/disease ids.
		for _, pid := range attr.PhenotypeIds {
			if ds, pid := diseaseID(pid); ds != "" && pid != "" {
				v.d.addXref(id, sourceID, pid, ds, false)
			}
		}
		// PubMed links.
		for _, link := range splitCSV(info["LINKS"]) {
			if strings.HasPrefix(strings.ToLower(link), "pubmed:") {
				pmid := strings.TrimSpace(link[7:])
				if pmid != "" {
					v.d.addXref(id, sourceID, pmid, "pubmed", false)
				}
			}
		}

		if idLogFile != nil {
			logProcessedID(idLogFile, id)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		log.Printf("dbVar: %s: %v", path, err)
	}
	return count
}

func parseVCFInfo(s string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		if part == "" {
			continue
		}
		if i := strings.Index(part, "="); i >= 0 {
			out[part[:i]] = part[i+1:]
		} else {
			out[part] = ""
		}
	}
	return out
}

func cleanVCFString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	return s
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = cleanVCFString(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func altType(alt string) string {
	switch strings.ToUpper(strings.Trim(alt, "<>")) {
	case "DEL":
		return "DEL"
	case "DUP":
		return "DUP"
	case "INS":
		return "INS"
	case "INV":
		return "INV"
	case "CNV":
		return "CNV"
	case "BND":
		return "BND"
	}
	return ""
}

func normalizeChrom(chr string) string {
	return strings.TrimPrefix(strings.TrimPrefix(chr, "chr"), "Chr")
}

func clinvarVariationID(acc string) string {
	acc = strings.TrimSpace(acc)
	if strings.HasPrefix(strings.ToUpper(acc), "VCV") {
		s := acc[3:]
		if i := strings.Index(s, "."); i >= 0 {
			s = s[:i]
		}
		if isDigits(s) {
			return s
		}
	}
	return ""
}

// diseaseID maps a dbVar PHENO token to (dataset, id).
func diseaseID(token string) (string, string) {
	token = strings.TrimSpace(token)
	upper := strings.ToUpper(token)
	switch {
	case strings.HasPrefix(upper, "MONDO:"):
		return "mondo", token
	case strings.HasPrefix(upper, "OMIM:"):
		id := token[5:]
		if i := strings.Index(id, "."); i >= 0 {
			id = id[:i]
		}
		return "mim", id
	case strings.HasPrefix(upper, "ORPHANET:"):
		return "orphanet", token[9:]
	case strings.HasPrefix(upper, "MEDGEN:"):
		return "medgen", token[7:]
	case strings.HasPrefix(upper, "MESH:"):
		return "mesh", token[5:]
	case strings.HasPrefix(upper, "HP:"):
		return "hpo", token
	case strings.HasPrefix(upper, "DOID:"):
		return "doid", token
	case strings.HasPrefix(upper, "EFO:"):
		return "efo", token
	}
	return "", ""
}
