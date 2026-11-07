package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// refseqExon ingests exon coordinates from the human GRCh38 RefSeq GFF3.
// One entry per exon, keyed "<transcriptBase>:exon<rank>", xref to refseq.
// This complements the Ensembl exon dataset for RefSeq accessions, which the
// transcript pages need for exon numbering and NMD-context calculations.
type refseqExon struct {
	source string
	d      *DataUpdate
}

func (r *refseqExon) check(err error, operation string) {
	checkWithContext(err, r.source, operation)
}

func (r *refseqExon) update() {
	defer r.d.wg.Done()

	log.Println("RefSeqExon: Starting GFF processing...")
	startTime := time.Now()

	sourceID := config.Dataconf[r.source]["id"]
	path := config.Dataconf[r.source]["path"]

	testLimit := config.GetTestLimit(r.source)
	if config.IsTestMode() {
		log.Printf("RefSeqExon: [TEST MODE] processing up to %d exons", testLimit)
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", path)
	r.check(err, "opening RefSeq GFF")
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)

	seen := make(map[string]bool)
	var total, saved, skipped int64
	var totalRead, previous int64

	for scanner.Scan() {
		line := scanner.Text()
		totalRead += int64(len(line)) + 1
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 9 || fields[2] != "exon" {
			continue
		}

		attrs := parseGFFAttrs(fields[8])
		transcript := attrs["transcript_id"]
		if transcript == "" {
			if parent := attrs["Parent"]; strings.HasPrefix(parent, "rna-") {
				transcript = strings.TrimPrefix(parent, "rna-")
			}
		}
		if !strings.HasPrefix(transcript, "NM_") && !strings.HasPrefix(transcript, "NR_") &&
			!strings.HasPrefix(transcript, "XM_") && !strings.HasPrefix(transcript, "XR_") {
			continue
		}

		rank := 0
		if exonID := attrs["ID"]; exonID != "" {
			if i := strings.LastIndex(exonID, "-"); i >= 0 {
				rank, _ = strconv.Atoi(exonID[i+1:])
			}
		}
		if rank == 0 {
			rank, _ = strconv.Atoi(attrs["exon_number"])
		}
		if rank == 0 {
			skipped++
			continue
		}

		start, serr := strconv.ParseInt(fields[3], 10, 64)
		end, eerr := strconv.ParseInt(fields[4], 10, 64)
		if serr != nil || eerr != nil || start <= 0 || end < start {
			skipped++
			continue
		}

		base := stripRefSeqVersion(transcript)
		entryID := fmt.Sprintf("%s:exon%d", base, rank)
		if seen[entryID] {
			continue
		}
		seen[entryID] = true

		attr := &pbuf.RefSeqExonAttr{
			Transcript:     transcript,
			TranscriptBase: base,
			ExonId:         attrs["ID"],
			ExonNumber:     int32(rank),
			Chromosome:     fields[0],
			Start:          start,
			End:            end,
			Strand:         fields[6],
		}
		b, merr := ffjson.Marshal(attr)
		if merr != nil {
			log.Printf("RefSeqExon: marshal %s: %v", entryID, merr)
			continue
		}
		r.d.addProp3(entryID, sourceID, b)
		r.d.addXref(entryID, sourceID, base, "refseq", false)
		saved++

		elapsed := int64(time.Since(r.d.start).Seconds())
		if elapsed > previous+r.d.progInterval {
			kbytesPerSecond := totalRead / elapsed / 1024
			previous = elapsed
			r.d.progChan <- &progressInfo{dataset: r.source, currentKBPerSec: kbytesPerSecond}
		}
		if saved%1000000 == 0 {
			log.Printf("RefSeqExon: saved %d exons...", saved)
		}
		if testLimit > 0 && saved >= int64(testLimit) {
			log.Printf("RefSeqExon: [TEST MODE] reached limit of %d exons", testLimit)
			break
		}
		total++
	}
	if err := scanner.Err(); err != nil {
		log.Printf("RefSeqExon: read error: %v", err)
	}

	atomic.AddUint64(&r.d.totalParsedEntry, uint64(saved))
	log.Printf("RefSeqExon: saved %d exons (skipped %d) (%.2fs)", saved, skipped, time.Since(startTime).Seconds())
	r.d.progChan <- &progressInfo{dataset: r.source, done: true}
}

// parseGFFAttrs parses a GFF3 attribute column into a key/value map.
func parseGFFAttrs(s string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		if part == "" {
			continue
		}
		i := strings.IndexByte(part, '=')
		if i < 0 {
			continue
		}
		out[part[:i]] = part[i+1:]
	}
	return out
}
