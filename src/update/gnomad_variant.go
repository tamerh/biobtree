package update

import (
	"biobtree/pbuf"
	"biobtree/util"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// gnomad_variant ingests gnomAD v4 per-variant, per-ancestry allele frequencies
// from the gnomAD v4 sites VCF.
//
// Purpose: adds the ACMG BA1/BS1/PM2 evidence layer (per-variant, per-population
// allele frequencies). Complements — and is DISTINCT from — the existing
// gene-level gnomad_constraint dataset (id 800) and the single global frequency
// stored on dbsnp (dbsnp.gnomad_frequency).
//
// License: ODC-ODbL (open; attribution + share-alike). The share-alike clause
// means gnomad_variant must be EXCLUDED from the CC BY-NC-SA KG export, like
// spliceai / alphamissense — documented in docs/datasets/gnomad_variant.md; no
// export logic is implemented here.
//
// Key scheme: chr:pos:ref:alt (GRCh38), identical to alphamissense / spliceai.
// VCF rows also carry rsIDs in the ID column; where present we xref to `dbsnp`
// by rsID so the frequency reaches the rsID hub.
//
// gnomAD v4 sites VCF INFO fields consumed:
//   AF                 global allele frequency
//   AF_grpmax          group-max AF (grpmax = popmax renamed in v4)
//   grpmax             ancestry group holding the grpmax AF
//   fafmax_faf95_max   filtering allele frequency (grpmax faf95)
//   AF_<pop>           per-ancestry AF: afr amr eas nfe sas fin asj ami mid remaining
//
// Production source: gnomAD v4.1 GENOMES sites VCF (~759M variants, whole-genome),
// split one file PER CHROMOSOME, hosted on the AWS Registry of Open Data:
//   https://gnomad-public-us-east-1.s3.amazonaws.com/release/4.1/vcf/genomes/
//   gnomad.genomes.v4.1.sites.chr{CHR}.vcf.bgz   (CHR = 1..22, X, Y)
// The conf "path" carries a "{CHR}" placeholder that update() expands over
// gnomadV4Chromosomes; a placeholder-free path (the test fixture) is read as a
// single file. Decision (2026-07): lives in the MAIN federation (not its own),
// since coordinate keys can't be pattern-routed to a non-main federation.
// INFO field names verified against the real v4.1 genomes VCF header.
type gnomadVariant struct {
	source string
	d      *DataUpdate
}

// gnomadV4Chromosomes are the per-chromosome sites files in the gnomAD v4.1
// genomes release: autosomes 1-22 plus X and Y. (Mitochondrial variants are a
// separate gnomAD release and are not ingested here.)
var gnomadV4Chromosomes = []string{
	"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12",
	"13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "X", "Y",
}

func (g *gnomadVariant) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

func (g *gnomadVariant) update() {
	defer g.d.wg.Done()

	log.Println("gnomAD Variant: Starting data processing...")
	startTime := time.Now()

	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("gnomAD Variant: [TEST MODE] Processing up to %d variants", testLimit)
	}

	sourceID := config.Dataconf[g.source]["id"]

	// dbsnp is optional — only xref by rsID when the dataset is configured.
	dbsnpConfigured := false
	if _, ok := config.Dataconf["dbsnp"]; ok {
		dbsnpConfigured = true
	}

	// gnomAD v4.1 genomes ships one sites VCF PER CHROMOSOME. When the configured
	// path carries the "{CHR}" placeholder we expand it over the autosomes + X + Y
	// and stream each file in turn. A path without the placeholder (the test
	// fixture) is processed as a single file.
	pathTemplate := config.Dataconf[g.source]["path"]
	var paths []string
	if strings.Contains(pathTemplate, "{CHR}") {
		for _, c := range gnomadV4Chromosomes {
			paths = append(paths, strings.ReplaceAll(pathTemplate, "{CHR}", c))
		}
	} else {
		paths = []string{pathTemplate}
	}

	var total int64
	for _, p := range paths {
		if testLimit > 0 && total >= int64(testLimit) {
			break
		}
		total += g.parseAndSaveVariants(p, testLimit, total, idLogFile, sourceID, dbsnpConfigured)
	}

	log.Printf("gnomAD Variant: Processing complete — %d variants across %d file(s) (%.2fs)",
		total, len(paths), time.Since(startTime).Seconds())
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}

// parseAndSaveVariants streams one gnomAD sites VCF (a single per-chromosome
// file in production, or the whole fixture in tests) and emits one entry per
// variant keyed chr:pos:ref:alt. priorCount is the number of variants already
// processed from earlier files, so the global testLimit is honored across the
// per-chromosome loop. Returns the number of variants processed from THIS file.
func (g *gnomadVariant) parseAndSaveVariants(filePath string, testLimit int, priorCount int64, idLogFile *os.File, sourceID string, dbsnpConfigured bool) int64 {
	log.Printf("gnomAD Variant: Processing variants from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", filePath)
	g.check(err, "opening gnomAD Variant VCF file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount int64
	var entryCount int64
	var skippedCount int64
	var longKeyCount int64
	var dbsnpXrefCount int64

	var totalRead int64
	var previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			g.check(readErr, "reading gnomAD Variant VCF file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		totalRead += int64(len(line))
		lineCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip VCF header / meta lines (## meta, # column header).
		if strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}
		if line == "" {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// VCF columns: CHROM POS ID REF ALT QUAL FILTER INFO ...
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("gnomAD Variant: SKIP line %d: not enough VCF columns (%d < 8)", lineCount, len(fields))
			}
			continue
		}

		chrom := fields[0]
		posStr := fields[1]
		idField := fields[2]
		refAllele := fields[3]
		altAllele := fields[4]
		info := fields[7]

		// Normalize chromosome (strip "chr" prefix if present).
		if strings.HasPrefix(chrom, "chr") {
			chrom = chrom[3:]
		}

		pos, perr := strconv.ParseInt(posStr, 10, 64)
		if perr != nil {
			skippedCount++
			if skippedCount <= 5 {
				log.Printf("gnomAD Variant: SKIP line %d: invalid position %q", lineCount, posStr)
			}
			continue
		}

		// Multi-allelic rows: gnomAD sites VCFs are split (one ALT per row).
		// If a comma slips through, keep only the first ALT for the key.
		if idx := strings.IndexByte(altAllele, ','); idx > 0 {
			altAllele = altAllele[:idx]
		}

		// Large indels whose full chr:pos:ref:alt key would exceed the LMDB key
		// limit get a bounded hashed key (util.VariantKey); the full ref/alt stay
		// in the attributes below, so no data is lost, and the same transform on
		// the lookup path (util.NormalizeVariantLookupKey) keeps them findable by
		// their full coordinate.
		entryID, hashed := util.VariantKey(chrom, pos, refAllele, altAllele)
		if hashed {
			longKeyCount++
			if longKeyCount <= 5 {
				log.Printf("gnomAD Variant: long-allele indel at %s:%d -> hashed key (full ref/alt kept in attrs)", chrom, pos)
			}
		}

		kv := parseVcfInfo(info)

		attr := &pbuf.GnomadVariantAttr{
			Chromosome:     chrom,
			Position:       pos,
			RefAllele:      refAllele,
			AltAllele:      altAllele,
			Af:             infoFloat(kv, "AF"),
			AfGrpmax:       infoFloat(kv, "AF_grpmax"),
			GrpmaxAncestry: kv["grpmax"],
			Faf:            infoFloat(kv, "fafmax_faf95_max"),
			AfAfr:          infoFloat(kv, "AF_afr"),
			AfAmr:          infoFloat(kv, "AF_amr"),
			AfEas:          infoFloat(kv, "AF_eas"),
			AfNfe:          infoFloat(kv, "AF_nfe"),
			AfSas:          infoFloat(kv, "AF_sas"),
			AfFin:          infoFloat(kv, "AF_fin"),
			AfAsj:          infoFloat(kv, "AF_asj"),
			AfAmi:          infoFloat(kv, "AF_ami"),
			AfMid:          infoFloat(kv, "AF_mid"),
			AfRemaining:    infoFloat(kv, "AF_remaining"),
		}

		attrBytes, merr := ffjson.Marshal(attr)
		g.check(merr, fmt.Sprintf("marshaling attributes for %s", entryID))
		g.d.addProp3(entryID, sourceID, attrBytes)

		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Xref to dbsnp by rsID (ID column may hold "rs..." or "." or a
		// semicolon-separated list). Lets the frequency reach the rsID hub.
		if dbsnpConfigured && idField != "." && idField != "" {
			for _, rsID := range strings.Split(idField, ";") {
				rsID = strings.TrimSpace(rsID)
				if isValidRsID(rsID) {
					g.d.addXref(entryID, sourceID, rsID, "dbsnp", false)
					dbsnpXrefCount++
				}
			}
		}

		entryCount++

		elapsed := int64(time.Since(g.d.start).Seconds())
		if elapsed > previous+g.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: kbytesPerSecond}
		}

		if entryCount%1000000 == 0 {
			log.Printf("gnomAD Variant: Processed %d variants...", entryCount)
		}

		if testLimit > 0 && priorCount+entryCount >= int64(testLimit) {
			log.Printf("gnomAD Variant: [TEST MODE] Reached limit of %d variants", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	log.Printf("gnomAD Variant: Processed %d variants from %s (skipped %d malformed lines, %d long-allele indels hashed, %d dbsnp xrefs)",
		entryCount, filePath, skippedCount, longKeyCount, dbsnpXrefCount)
	return entryCount
}

// parseVcfInfo splits a VCF INFO column ("A=1;B=2;FLAG") into a key->value map.
// Flag keys (no "=") are stored with an empty value.
func parseVcfInfo(info string) map[string]string {
	kv := make(map[string]string)
	if info == "" || info == "." {
		return kv
	}
	for _, field := range strings.Split(info, ";") {
		if field == "" {
			continue
		}
		if eq := strings.IndexByte(field, '='); eq >= 0 {
			kv[field[:eq]] = field[eq+1:]
		} else {
			kv[field] = ""
		}
	}
	return kv
}

// infoFloat parses a float INFO value; missing / "." / unparseable yields 0.
func infoFloat(kv map[string]string, key string) float64 {
	v, ok := kv[key]
	if !ok || v == "" || v == "." {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}
