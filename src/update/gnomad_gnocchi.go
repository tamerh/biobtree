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

// gnomadGnocchi ingests gnomAD non-coding genomic constraint (Gnocchi, Chen et
// al. Nature 2024): genome-wide 1 kb windows each with a depletion-of-variation
// Z-score. It is the only per-position constraint signal for intronic/UTR/
// intergenic variants. Interval-indexed, so a variant chr:pos resolves to its
// covering window. Source: gnomAD v3.1 genomic_constraint (CC0, hg38).
type gnomadGnocchi struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (g *gnomadGnocchi) update() {
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
		log.Printf("gnomAD Gnocchi: error processing %s: %v", filePath, err)
	}
	fmt.Printf("gnomAD Gnocchi: processed %d constraint windows\n", count)

	atomic.AddUint64(&g.d.totalParsedEntry, count)
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}

// columns: 0 chrom, 1 start, 2 end, 3 element_id, 4 possible, 5 expected,
// 6 observed, 7 oe, 8 z
func (g *gnomadGnocchi) processFile(filePath string, idLogFile *os.File, testLimit int) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open gnomad-gnocchi file: %v", err)
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

		if line != "" && !strings.HasPrefix(line, "chrom\t") { // skip header
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

func (g *gnomadGnocchi) processRow(line string, idLogFile *os.File) bool {
	f := strings.Split(line, "\t")
	if len(f) < 9 {
		return false
	}
	chr := strings.TrimSpace(f[0])
	start, e1 := strconv.Atoi(strings.TrimSpace(f[1]))
	end, e2 := strconv.Atoi(strings.TrimSpace(f[2]))
	id := strings.TrimSpace(f[3]) // element_id, e.g. chr1-783000-784000
	if chr == "" || id == "" || e1 != nil || e2 != nil {
		return false
	}

	attr := &pbuf.GnomadGnocchiAttr{
		Chromosome: chr,
		Start:      int32(start),
		End:        int32(end),
		Z:          strings.TrimSpace(f[8]),
		Oe:         strings.TrimSpace(f[7]),
		Obs:        strings.TrimSpace(f[6]),
		Exp:        strings.TrimSpace(f[5]),
		Possible:   strings.TrimSpace(f[4]),
	}
	if b, err := ffjson.Marshal(attr); err == nil {
		g.d.addProp3(id, g.sourceID, b)
	}

	// point-in-interval index: a variant chr:pos resolves to its covering window.
	g.d.addInterval(id, g.source, chr, int64(start), int64(end))

	if idLogFile != nil {
		logProcessedID(idLogFile, id)
	}
	return true
}
