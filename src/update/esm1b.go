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

type esm1b struct {
	source string
	d      *DataUpdate
}

func (e *esm1b) check(err error, operation string) {
	checkWithContext(err, e.source, operation)
}

// esm1b ingests ESM1b protein-language-model variant-effect scores (Brandes et al.
// 2023). Keyed "uniprot:protein_variant" (e.g. "P01116:G12D") — protein-level, NOT
// genomic; it joins to Atlas variants via (uniprot_id, protein_variant), the same
// single-letter WT+pos+mut notation AlphaMissense uses. LLR <= 0, more negative =
// more damaging. Published scores are CC-BY-NC → ingest-only / KG-export excluded.
//
// INPUT (pre-melted TSV from src/scripts/esm1b/esm1b_prepare.py; biobtree does not
// melt the 42k-CSV LLR matrix archive):
//
//	uniprot  protein_variant  position  llr     gene_symbol
//	P01116   G12D             12        -7.412  KRAS
func (e *esm1b) update() {
	defer e.d.wg.Done()

	log.Println("ESM1b: Starting data processing...")
	startTime := time.Now()

	testLimit := config.GetTestLimit(e.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, e.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("ESM1b: [TEST MODE] processing up to %d variants", testLimit)
	}

	sourceID := config.Dataconf[e.source]["id"]

	filePath := config.Dataconf[e.source]["path"]
	log.Printf("ESM1b: Processing variants from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(e.source, "", "", filePath)
	e.check(err, "opening ESM1b TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount, entryCount, skippedCount int64
	var totalRead, previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			e.check(readErr, "reading ESM1b TSV file")
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

		attr := &pbuf.Esm1BAttr{
			UniprotId:      uniprotID,
			ProteinVariant: proteinVariant,
			Position:       pos,
			Esm1BLlr:       llr,
			GeneSymbol:     geneSymbol,
		}
		attrBytes, merr := ffjson.Marshal(attr)
		e.check(merr, fmt.Sprintf("marshaling attributes for %s", entryID))
		e.d.addProp3(entryID, sourceID, attrBytes)

		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Cross-reference to the UniProt protein (canonical accession, no isoform suffix).
		uniprotBase := uniprotID
		if dash := strings.Index(uniprotID, "-"); dash > 0 {
			uniprotBase = uniprotID[:dash]
		}
		e.d.addXref(entryID, sourceID, uniprotBase, "uniprot", false)

		entryCount++

		elapsed := int64(time.Since(e.d.start).Seconds())
		if elapsed > previous+e.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			e.d.progChan <- &progressInfo{dataset: e.source, currentKBPerSec: kbytesPerSecond}
		}
		if entryCount%10000000 == 0 {
			log.Printf("ESM1b: Processed %d variants...", entryCount)
		}

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("ESM1b: [TEST MODE] reached limit of %d variants", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	log.Printf("ESM1b: Processed %d variants (skipped %d rows) (%.2fs)",
		entryCount, skippedCount, time.Since(startTime).Seconds())

	e.d.progChan <- &progressInfo{dataset: e.source, done: true}
}
