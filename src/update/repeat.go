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

// repeat ingests genomic repeat / low-complexity / segmental-duplication regions
// from UCSC hg38: Dfam-based rmskOutCurrent (RepeatMasker; NOT the RepBase-
// licensed rmsk), simpleRepeat (Tandem Repeats Finder), genomicSuperDups. Each
// region is interval-indexed so a variant chr:pos resolves to the repeats it
// falls in — giving ClinVar Microsatellite-type variants an annotation they
// otherwise lack. All three UCSC tracks are freely redistributable.
type repeat struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (r *repeat) update() {
	defer r.d.wg.Done()

	r.sourceID = config.Dataconf[r.source]["id"]
	testLimit := config.GetTestLimit(r.source)

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, r.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	// Each source is a UCSC database dump with its own column layout.
	sources := []struct {
		pathKey string
		rtype_  string
		parse   func(f []string) (chr string, start, end int, class, name, family string, ok bool)
	}{
		{"path", "repeatmasker", parseRmskRow},
		{"pathSimpleRepeat", "tandem_repeat", parseTrfRow},
		{"pathSegdup", "segmental_dup", parseSegdupRow},
	}
	for _, src := range sources {
		fp := config.Dataconf[r.source][src.pathKey]
		if fp == "" {
			continue
		}
		n, err := r.processFile(fp, src.rtype_, src.parse, idLogFile, testLimit)
		if err != nil {
			log.Printf("Repeat: error processing %s (%s): %v", src.rtype_, fp, err)
		}
		total += n
	}
	fmt.Printf("Repeat: processed %d repeat regions\n", total)

	atomic.AddUint64(&r.d.totalParsedEntry, total)
	r.d.progChan <- &progressInfo{dataset: r.source, done: true}
}

func (r *repeat) processFile(filePath, rtype string,
	parse func(f []string) (string, int, int, string, string, string, bool),
	idLogFile *os.File, testLimit int) (uint64, error) {

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(r.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("open: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1024*1024)
	} else {
		reader = bufio.NewReaderSize(br, 1024*1024)
	}

	prefix := map[string]string{"repeatmasker": "rm", "tandem_repeat": "trf", "segmental_dup": "sd"}[rtype]
	var count uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return count, fmt.Errorf("read: %v", rerr)
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line != "" && !strings.HasPrefix(line, "#") {
			f := strings.Split(line, "\t")
			chr, start, end, class, name, family, ok := parse(f)
			if ok && chr != "" {
				id := prefix + ":" + chr + ":" + strconv.Itoa(start) + "-" + strconv.Itoa(end)
				attr := &pbuf.RepeatAttr{
					Chromosome:   chr,
					Start:        int32(start),
					End:          int32(end),
					RepeatType:   rtype,
					RepeatClass:  class,
					RepeatName:   name,
					RepeatFamily: family,
				}
				if b, err := ffjson.Marshal(attr); err == nil {
					r.d.addProp3(id, r.sourceID, b)
				}
				r.d.addInterval(id, r.source, chr, int64(start), int64(end))
				if idLogFile != nil {
					logProcessedID(idLogFile, id)
				}
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

// UCSC rmskOutCurrent: bin,swScore,milliDiv,milliDel,milliIns,genoName,
// genoStart,genoEnd,genoLeft,strand,repName,repClass,repFamily,...
func parseRmskRow(f []string) (string, int, int, string, string, string, bool) {
	if len(f) < 13 {
		return "", 0, 0, "", "", "", false
	}
	s, e1 := strconv.Atoi(f[6])
	e, e2 := strconv.Atoi(f[7])
	if e1 != nil || e2 != nil {
		return "", 0, 0, "", "", "", false
	}
	return f[5], s, e, f[11], f[10], f[12], true
}

// UCSC simpleRepeat: bin,chrom,chromStart,chromEnd,name,period,copyNum,...,sequence(16)
func parseTrfRow(f []string) (string, int, int, string, string, string, bool) {
	if len(f) < 17 {
		return "", 0, 0, "", "", "", false
	}
	s, e1 := strconv.Atoi(f[2])
	e, e2 := strconv.Atoi(f[3])
	if e1 != nil || e2 != nil {
		return "", 0, 0, "", "", "", false
	}
	return f[1], s, e, "Simple_repeat", f[16], "", true // f[16] = repeat motif
}

// UCSC genomicSuperDups: bin,chrom,chromStart,chromEnd,name(otherChrom:pos),...
func parseSegdupRow(f []string) (string, int, int, string, string, string, bool) {
	if len(f) < 5 {
		return "", 0, 0, "", "", "", false
	}
	s, e1 := strconv.Atoi(f[2])
	e, e2 := strconv.Atoi(f[3])
	if e1 != nil || e2 != nil {
		return "", 0, 0, "", "", "", false
	}
	return f[1], s, e, "Segmental_duplication", f[4], "", true // f[4] = partner region
}
