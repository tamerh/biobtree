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

type revel struct {
	source string
	d      *DataUpdate
}

func (r *revel) check(err error, operation string) {
	checkWithContext(err, r.source, operation)
}

// appendRevelTranscripts splits the REVEL Ensembl_transcriptid field (which is
// ';'-separated when a variant hits multiple transcripts) and appends unique,
// non-empty transcript ids to dst.
func appendRevelTranscripts(dst []string, field string) []string {
	if field == "" {
		return dst
	}
	for _, t := range strings.Split(field, ";") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		dup := false
		for _, e := range dst {
			if e == t {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, t)
		}
	}
	return dst
}

// revel ingests REVEL ensemble missense pathogenicity scores (0-1, higher = more
// pathogenic). Keyed "chr:pos:ref:alt" (GRCh38), co-locating with AlphaMissense/
// SpliceAI on the same variant node. Source = REVEL v1.3 (Ioannidis et al. 2016;
// Zenodo DOI 10.5281/zenodo.7072866, ODbL) — ingest-only, excluded from the
// CC BY-NC-SA KG export.
//
// INPUT (CSV, one header row):
//
//	chr,hg19_pos,grch38_pos,ref,alt,aaref,aaalt,REVEL,Ensembl_transcriptid
//	1,35142,35142,G,A,T,M,0.027,ENST00000417324
//
// A variant recurs on consecutive rows once per transcript it hits; REVEL is a
// genomic-position score (normally identical across those rows). The file is
// position-sorted, so we group consecutive rows sharing chr:grch38pos:ref:alt,
// keep the MAX REVEL, and collect the transcript IDs. Rows with a blank grch38_pos
// (ambiguous GRCh38 liftover — blank in REVEL since 2021) are skipped.
func (r *revel) update() {
	defer r.d.wg.Done()

	log.Println("REVEL: Starting data processing...")
	startTime := time.Now()

	testLimit := config.GetTestLimit(r.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, r.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("REVEL: [TEST MODE] processing up to %d variants", testLimit)
	}

	sourceID := config.Dataconf[r.source]["id"]

	filePath := config.Dataconf[r.source]["path"]
	log.Printf("REVEL: Processing variants from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	r.check(err, "opening REVEL CSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	reader := bufio.NewReaderSize(br, 1024*1024)

	var lineCount, entryCount, skippedCount int64
	var totalRead, previous int64

	// Current variant group accumulated for multi-transcript dedup.
	var curKey, curChrom, curRef, curAlt, curAaref, curAaalt string
	var curPos int64
	var curRevel float64
	var curTranscripts []string
	haveCur := false

	flush := func() {
		if !haveCur {
			return
		}
		attr := &pbuf.RevelAttr{
			Chromosome:    curChrom,
			Position:      curPos,
			RefAllele:     curRef,
			AltAllele:     curAlt,
			Aaref:         curAaref,
			Aaalt:         curAaalt,
			Revel:         curRevel,
			TranscriptIds: curTranscripts,
		}
		attrBytes, merr := ffjson.Marshal(attr)
		r.check(merr, fmt.Sprintf("marshaling attributes for %s", curKey))
		r.d.addProp3(curKey, sourceID, attrBytes)

		if idLogFile != nil {
			logProcessedID(idLogFile, curKey)
		}

		// Cross-reference to Ensembl transcripts (base id, no version).
		for _, t := range curTranscripts {
			if t == "" {
				continue
			}
			tb := t
			if dot := strings.Index(t, "."); dot > 0 {
				tb = t[:dot]
			}
			r.d.addXref(curKey, sourceID, tb, "transcript", false)
		}

		entryCount++
	}

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			r.check(readErr, "reading REVEL CSV file")
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		totalRead += int64(len(line))
		lineCount++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			if readErr == io.EOF {
				break
			}
			continue
		}
		// Skip the CSV header (starts with "chr,") and any comment.
		if strings.HasPrefix(line, "chr,") || strings.HasPrefix(line, "#") {
			if readErr == io.EOF {
				break
			}
			continue
		}

		// CSV: chr,hg19_pos,grch38_pos,ref,alt,aaref,aaalt,REVEL,Ensembl_transcriptid
		fields := strings.Split(line, ",")
		if len(fields) < 8 {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		chrom := fields[0]
		grch38Pos := strings.TrimSpace(fields[2])
		ref := fields[3]
		alt := fields[4]
		aaref := fields[5]
		aaalt := fields[6]
		revelStr := strings.TrimSpace(fields[7])
		transcript := ""
		if len(fields) >= 9 {
			transcript = strings.TrimSpace(fields[8])
		}

		// Skip variants without a GRCh38 position (blank/ambiguous liftover).
		if grch38Pos == "" || grch38Pos == "." {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		if strings.HasPrefix(chrom, "chr") {
			chrom = chrom[3:]
		}

		pos, perr := strconv.ParseInt(grch38Pos, 10, 64)
		if perr != nil {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		score, serr := strconv.ParseFloat(revelStr, 64)
		if serr != nil {
			skippedCount++
			if readErr == io.EOF {
				break
			}
			continue
		}

		key := fmt.Sprintf("%s:%d:%s:%s", chrom, pos, ref, alt)

		if haveCur && key == curKey {
			// Same variant, another transcript row: keep max REVEL + collect transcript(s).
			if score > curRevel {
				curRevel = score
			}
			curTranscripts = appendRevelTranscripts(curTranscripts, transcript)
		} else {
			// New variant boundary: flush the previous group, start a fresh one.
			flush()

			elapsed := int64(time.Since(r.d.start).Seconds())
			if elapsed > previous+r.d.progInterval {
				kbytesPerSecond := totalRead / elapsed / 1024
				previous = elapsed
				r.d.progChan <- &progressInfo{dataset: r.source, currentKBPerSec: kbytesPerSecond}
			}
			if entryCount > 0 && entryCount%1000000 == 0 {
				log.Printf("REVEL: Processed %d variants...", entryCount)
			}
			if testLimit > 0 && entryCount >= int64(testLimit) {
				log.Printf("REVEL: [TEST MODE] reached limit of %d variants", testLimit)
				haveCur = false
				break
			}

			curKey = key
			curChrom = chrom
			curPos = pos
			curRef = ref
			curAlt = alt
			curAaref = aaref
			curAaalt = aaalt
			curRevel = score
			curTranscripts = appendRevelTranscripts(nil, transcript)
			haveCur = true
		}

		if readErr == io.EOF {
			break
		}
	}
	// Flush the final group.
	flush()

	log.Printf("REVEL: Processed %d variants (skipped %d rows) (%.2fs)",
		entryCount, skippedCount, time.Since(startTime).Seconds())

	r.d.progChan <- &progressInfo{dataset: r.source, done: true}
}
