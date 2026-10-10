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
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// gnomadRmc ingests gnomAD v4.1.1 regional missense constraint (missense-
// constrained regions, MCRs). Each transcript is split into sub-regions with
// observed/expected missense and the ClinGen-calibrated O/E (region_oe); a
// region with low O/E is under strong missense constraint. Interval-indexed so
// a variant chr:pos resolves to its covering region, and edged to the Ensembl
// transcript. Source: gs://gcp-public-data--gnomad/papers/2026-rmc (CC0, hg38).
type gnomadRmc struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (g *gnomadRmc) update() {
	defer g.d.wg.Done()

	g.sourceID = config.Dataconf[g.source]["id"]

	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	filePath := config.Dataconf[g.source]["path"]
	count, err := g.processFile(filePath, idLogFile, testLimit)
	if err != nil {
		log.Printf("gnomAD RMC: error processing %s: %v", filePath, err)
	}
	fmt.Printf("gnomAD RMC: processed %d constrained regions\n", count)

	atomic.AddUint64(&g.d.totalParsedEntry, count)
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}

// columns: 0 transcript, 1 n_total_transcript_regions, 2 chr, 3 region_start,
// 4 region_end, 5 region_obs, 6 region_exp, 7 region_oe, 8 region_oe_chisq,
// 9 region_oe_chisq_p, 10 is_high_coverage
func (g *gnomadRmc) processFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open gnomad-rmc file: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024)
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024)
	}

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

		if line != "" && !strings.HasPrefix(line, "transcript\t") { // skip header
			if g.processRow(line, idLogFile) {
				count++
				if testLimit > 0 && int(count) >= testLimit {
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

func (g *gnomadRmc) processRow(line string, idLogFile *os.File) bool {
	f := strings.Split(line, "\t")
	if len(f) < 11 {
		return false
	}
	transcript := strings.SplitN(strings.TrimSpace(f[0]), ".", 2)[0] // ENST, versionless
	chr := strings.TrimSpace(f[2])
	start, e1 := strconv.Atoi(strings.TrimSpace(f[3]))
	end, e2 := strconv.Atoi(strings.TrimSpace(f[4]))
	if transcript == "" || chr == "" || e1 != nil || e2 != nil {
		return false
	}
	obs := strings.TrimSpace(f[5])
	exp := strings.TrimSpace(f[6])
	oeStr := strings.TrimSpace(f[7])
	chisq := strings.TrimSpace(f[8])
	chisqP := strings.TrimSpace(f[9])
	highCov := strings.EqualFold(strings.TrimSpace(f[10]), "true")

	// constrained = oe < 0.36 (ClinGen moderate-pathogenic regional threshold).
	constrained := false
	if oeVal, err := strconv.ParseFloat(oeStr, 64); err == nil && oeVal < 0.36 {
		constrained = true
	}

	// id = ENST:region_start (unique per region of a transcript)
	id := transcript + ":" + strconv.Itoa(start)

	attr := &pbuf.GnomadRmcAttr{
		Transcript:     transcript,
		Chromosome:     chr,
		Start:          int32(start),
		End:            int32(end),
		Obs:            obs,
		Exp:            exp,
		Oe:             oeStr,
		OeChisq:        chisq,
		OeChisqP:       chisqP,
		Constrained:    constrained,
		IsHighCoverage: highCov,
	}
	if b, err := ffjson.Marshal(attr); err == nil {
		g.d.addProp3(id, g.sourceID, b)
	}

	// region -> Ensembl transcript (keyed from the RMC record so the edge is
	// owned by this parser and survives incremental builds).
	g.d.addXref(id, g.sourceID, transcript, "transcript", false)

	// point-in-interval index: a variant chr:pos resolves to the covering region.
	g.d.addInterval(id, g.source, chr, int64(start), int64(end))

	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}
	return true
}
