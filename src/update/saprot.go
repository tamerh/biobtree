package update

import (
	"biobtree/pbuf"
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

type saprot struct {
	source string
	d      *DataUpdate
}

func (s *saprot) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

// saprotBaseAccession strips a UniProt isoform suffix ("Q14315-2" -> "Q14315").
func saprotBaseAccession(uid string) string {
	if dash := strings.Index(uid, "-"); dash > 0 {
		return uid[:dash]
	}
	return uid
}

// buildAliasMap scans the LLR keys once and returns, for each base accession that
// was scored ONLY under a single isoform key (and has no bare-accession key), that
// sole isoform. The main pass then also emits those scores under the bare-accession
// prefix, so a consumer querying the primary accession (e.g. "Q14315:D866N")
// discovers a score that physically lives under an isoform key ("Q14315-2:D866N").
//
// Ambiguous bases (scored under multiple isoforms, no bare key) are intentionally
// left out — their canonical sequence is genuinely unscored, and the consumer's
// divergence rule withholds a non-canonical hit anyway. Bases that already have a
// bare key need no alias. Derived purely from saprot's own keys, so it is
// release-independent (does not depend on any UniProt release).
func (s *saprot) buildAliasMap(filePath string, testLimit int) map[string]string {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
	s.check(err, "opening SaProt TSV for alias pre-pass")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)
	hasBare := make(map[string]bool)
	isoforms := make(map[string]map[string]bool)
	var n int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			s.check(readErr, "reading SaProt TSV (alias pre-pass)")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}
		// Only the first field (uniprot id) is needed.
		uid := line
		if t := strings.IndexByte(line, '\t'); t >= 0 {
			uid = line[:t]
		}
		uid = strings.TrimSpace(uid)
		if uid != "" && !strings.HasPrefix(uid, "#") {
			base := saprotBaseAccession(uid)
			if uid == base {
				hasBare[base] = true
			} else {
				if isoforms[base] == nil {
					isoforms[base] = make(map[string]bool)
				}
				isoforms[base][uid] = true
			}
			n++
			if testLimit > 0 && n >= int64(testLimit) {
				break
			}
		}
		if readErr == io.EOF {
			break
		}
	}

	aliasMap := make(map[string]string)
	for base, isos := range isoforms {
		if hasBare[base] || len(isos) != 1 {
			continue
		}
		for iso := range isos {
			aliasMap[base] = iso
		}
	}
	return aliasMap
}

// saprot ingests SaProt-650M structure-aware protein-LM variant-effect scores
// (computed by bioyoda from MIT weights → ours → KG-export eligible). Keyed
// "uniprot:protein_variant" (e.g. "P01116:G12D"), same scheme as esm1b. LLR <= 0,
// more negative = more damaging. Unsupervised + structure-aware → an independent
// second opinion to the (weakly supervised) AlphaMissense score.
//
// INPUT (TSV, no header; same contract as esm1b):
//
//	uniprot  protein_variant  position  llr     gene_symbol
//	P01116   G12D             12        -13.823 KRAS
func (s *saprot) update() {
	defer s.d.wg.Done()

	log.Println("SaProt: Starting data processing...")
	startTime := time.Now()

	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("SaProt: [TEST MODE] processing up to %d variants", testLimit)
	}

	sourceID := config.Dataconf[s.source]["id"]

	filePath := config.Dataconf[s.source]["path"]
	log.Printf("SaProt: Processing variants from %s", filePath)

	// Pre-pass: find base accessions scored only under a single isoform (no bare
	// key), so their scores can also be surfaced under the bare-accession key.
	aliasMap := s.buildAliasMap(filePath, testLimit)
	log.Printf("SaProt: alias pre-pass -> %d base accessions get a bare->isoform discovery alias", len(aliasMap))

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
	s.check(err, "opening SaProt TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount, entryCount, skippedCount, aliasCount int64
	var totalRead, previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			s.check(readErr, "reading SaProt TSV file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		totalRead += int64(len(line))
		lineCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" || strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// uniprot \t protein_variant \t position \t llr \t gene_symbol
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		uniprotID := fields[0]
		proteinVariant := fields[1]
		posStr := fields[2]
		llrStr := fields[3]
		geneSymbol := ""
		if len(fields) >= 5 {
			geneSymbol = fields[4]
		}

		if uniprotID == "" || proteinVariant == "" {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		pos, _ := strconv.ParseInt(posStr, 10, 64)
		llr, lerr := strconv.ParseFloat(llrStr, 64)
		if lerr != nil {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		// KEY: uniprot:protein_variant (e.g. "P01116:G12D").
		entryID := fmt.Sprintf("%s:%s", uniprotID, proteinVariant)

		attr := &pbuf.SaprotAttr{
			UniprotId:      uniprotID,
			ProteinVariant: proteinVariant,
			Position:       pos,
			SaprotLlr:      llr,
			GeneSymbol:     geneSymbol,
		}
		attrBytes, merr := ffjson.Marshal(attr)
		s.check(merr, fmt.Sprintf("marshaling attributes for %s", entryID))
		s.d.addProp3(entryID, sourceID, attrBytes)

		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Cross-reference to the UniProt protein (canonical accession, no isoform suffix).
		uniprotBase := saprotBaseAccession(uniprotID)
		s.d.addXref(entryID, sourceID, uniprotBase, "uniprot", false)

		// Discovery alias: if this base accession was scored ONLY under this single
		// isoform (no bare key), also emit the score under the bare-accession key so a
		// consumer querying the primary accession finds it. Same attr (keeps the real
		// isoform UniprotId for provenance). See buildAliasMap.
		if iso, ok := aliasMap[uniprotBase]; ok && iso == uniprotID {
			aliasID := fmt.Sprintf("%s:%s", uniprotBase, proteinVariant)
			s.d.addProp3(aliasID, sourceID, attrBytes)
			s.d.addXref(aliasID, sourceID, uniprotBase, "uniprot", false)
			aliasCount++
			if idLogFile != nil {
				logProcessedID(idLogFile, aliasID)
			}
		}

		entryCount++

		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: kbytesPerSecond}
		}
		if entryCount%10000000 == 0 {
			log.Printf("SaProt: Processed %d variants...", entryCount)
		}

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("SaProt: [TEST MODE] reached limit of %d variants", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	log.Printf("SaProt: Processed %d variants (skipped %d rows, %d bare-accession discovery aliases) (%.2fs)",
		entryCount, skippedCount, aliasCount, time.Since(startTime).Seconds())

	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}
