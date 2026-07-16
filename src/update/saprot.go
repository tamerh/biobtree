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

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
	s.check(err, "opening SaProt TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount, entryCount, skippedCount int64
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
		uniprotBase := uniprotID
		if dash := strings.Index(uniprotID, "-"); dash > 0 {
			uniprotBase = uniprotID[:dash]
		}
		s.d.addXref(entryID, sourceID, uniprotBase, "uniprot", false)

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

	log.Printf("SaProt: Processed %d variants (skipped %d rows) (%.2fs)",
		entryCount, skippedCount, time.Since(startTime).Seconds())

	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}
