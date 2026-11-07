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

// proteinPosition extracts the numeric position from a one-letter protein
// substitution such as "G12D" -> 12. Returns 0 when no digits are present.
func proteinPosition(variant string) int64 {
	start := -1
	for i := 0; i < len(variant); i++ {
		c := variant[i]
		if c >= '0' && c <= '9' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0
	}
	end := start
	for end < len(variant) && variant[end] >= '0' && variant[end] <= '9' {
		end++
	}
	pos, err := strconv.ParseInt(variant[start:end], 10, 64)
	if err != nil {
		return 0
	}
	return pos
}

// alphamissenseAA ingests the protein-centric AlphaMissense table
// (AlphaMissense_aa_substitutions.tsv.gz). Keyed "uniprot:protein_variant"
// (e.g. "P01116:G12D"), the same key shape as ESM1b and SaProt, so protein-level
// joins do not need to go through genomic coordinates.
//
// INPUT:
//
//	uniprot_id  protein_variant  am_pathogenicity  am_class
//	P01116      G12D             0.998             likely_pathogenic
type alphamissenseAA struct {
	source string
	d      *DataUpdate
}

func (a *alphamissenseAA) check(err error, operation string) {
	checkWithContext(err, a.source, operation)
}

func (a *alphamissenseAA) update() {
	defer a.d.wg.Done()

	log.Println("AlphaMissense-AA: Starting protein-centric data processing...")
	startTime := time.Now()

	testLimit := config.GetTestLimit(a.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, a.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("AlphaMissense-AA: [TEST MODE] processing up to %d substitutions", testLimit)
	}

	sourceID := config.Dataconf[a.source]["id"]
	filePath := config.Dataconf[a.source]["path"]
	log.Printf("AlphaMissense-AA: Processing substitutions from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(a.source, "", "", filePath)
	a.check(err, "opening AlphaMissense substitutions file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var entryCount, skippedCount int64
	var totalRead, previous int64

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			a.check(readErr, "reading AlphaMissense substitutions file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		totalRead += int64(len(line))
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "uniprot_id\t") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		uniprotID := strings.TrimSpace(fields[0])
		proteinVariant := strings.TrimSpace(fields[1])
		scoreStr := strings.TrimSpace(fields[2])
		amClass := strings.TrimSpace(fields[3])

		if uniprotID == "" || proteinVariant == "" {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		score, serr := strconv.ParseFloat(scoreStr, 64)
		if serr != nil {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		entryID := fmt.Sprintf("%s:%s", uniprotID, proteinVariant)
		attr := &pbuf.AlphaMissenseAttr{
			UniprotId:       uniprotID,
			ProteinVariant:  proteinVariant,
			Position:        proteinPosition(proteinVariant),
			AmPathogenicity: score,
			AmClass:         amClass,
		}
		attrBytes, merr := ffjson.Marshal(attr)
		a.check(merr, fmt.Sprintf("marshaling attributes for %s", entryID))
		a.d.addProp3(entryID, sourceID, attrBytes)

		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		// Canonical UniProt accession (no isoform suffix).
		uniprotBase := uniprotID
		if dash := strings.Index(uniprotID, "-"); dash > 0 {
			uniprotBase = uniprotID[:dash]
		}
		a.d.addXref(entryID, sourceID, uniprotBase, "uniprot", false)

		entryCount++

		elapsed := int64(time.Since(a.d.start).Seconds())
		if elapsed > previous+a.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			a.d.progChan <- &progressInfo{dataset: a.source, currentKBPerSec: kbytesPerSecond}
		}
		if entryCount%10000000 == 0 {
			log.Printf("AlphaMissense-AA: Processed %d substitutions...", entryCount)
		}

		if testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("AlphaMissense-AA: [TEST MODE] reached limit of %d substitutions", testLimit)
			break
		}

		if readErr == io.EOF {
			break
		}
	}

	log.Printf("AlphaMissense-AA: Processed %d substitutions (skipped %d rows) (%.2fs)",
		entryCount, skippedCount, time.Since(startTime).Seconds())

	a.d.progChan <- &progressInfo{dataset: a.source, done: true}
}
