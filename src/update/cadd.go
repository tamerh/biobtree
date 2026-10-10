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

// cadd ingests CADD deleteriousness scores (whole-genome SNVs + gnomAD indels).
// Stored minimally: key chr:pos:ref:alt -> {phred}, with no coord duplication
// (coords live in the key) and no reverse/text/gene links, so it is exactly 1 KV
// per variant — the smallest possible footprint for a genome-wide (~8.6B SNV)
// set. Reached by direct entry lookup of the variant key (same access pattern as
// the conservation federation). Its own `cadd` federation.
type cadd struct {
	source   string
	sourceID string
	d        *DataUpdate
}

func (c *cadd) check(err error, op string) { checkWithContext(err, c.source, op) }

func (c *cadd) update() {
	defer c.d.wg.Done()

	c.sourceID = config.Dataconf[c.source]["id"]
	testLimit := config.GetTestLimit(c.source)

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64

	if config.IsTestMode() {
		// Prod paths are remote CADD URLs (useLocalFile=no); in test mode read a
		// small local fixture directly (same approach as spliceai/pangolin).
		fx := "tests/datasets/cadd/cadd_fixture.tsv"
		f, err := os.Open(fx)
		c.check(err, "opening cadd fixture")
		defer f.Close()
		total += c.processReader(bufio.NewReaderSize(f, 1<<20), idLogFile, testLimit, total)
	} else {
		// path = whole-genome SNVs; pathIndel = gnomAD-observed indels. Same columns.
		for _, key := range []string{"path", "pathIndel"} {
			fp := config.Dataconf[c.source][key]
			if fp == "" {
				continue
			}
			n, err := c.processFile(fp, idLogFile, testLimit, total)
			if err != nil {
				log.Printf("CADD: error processing %s: %v", fp, err)
			}
			total += n
			if testLimit > 0 && total >= uint64(testLimit) {
				break
			}
		}
	}

	fmt.Printf("CADD: processed %d variants\n", total)
	atomic.AddUint64(&c.d.totalParsedEntry, total)
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

func (c *cadd) processFile(filePath string, idLogFile *os.File, testLimit int, already uint64) (uint64, error) {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	if err != nil {
		return 0, fmt.Errorf("open: %v", err)
	}
	defer closeRnacentralReaders(gz, ftpFile, client, localFile)

	var reader *bufio.Reader
	if gz != nil {
		reader = bufio.NewReaderSize(gz, 1<<20)
	} else {
		reader = bufio.NewReaderSize(br, 1<<20)
	}
	return c.processReader(reader, idLogFile, testLimit, already), nil
}

// columns: 0 Chrom, 1 Pos, 2 Ref, 3 Alt, 4 RawScore, 5 PHRED
func (c *cadd) processReader(reader *bufio.Reader, idLogFile *os.File, testLimit int, already uint64) uint64 {
	var count uint64
	for {
		line, rerr := reader.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			log.Printf("CADD: read error: %v", rerr)
			return count
		}
		if len(line) == 0 && rerr == io.EOF {
			break
		}
		line = strings.TrimRight(line, "\r\n")

		if line != "" && line[0] != '#' { // skip ## meta + #Chrom header
			f := strings.Split(line, "\t")
			if len(f) >= 6 {
				chrom := NormalizeChr(f[0])
				if _, perr := strconv.ParseInt(f[1], 10, 64); perr == nil {
					entryID := chrom + ":" + f[1] + ":" + f[2] + ":" + f[3]
					if len(entryID) <= LMDBMaxKeySize {
						attr := &pbuf.CaddAttr{Phred: f[5]}
						if b, err := ffjson.Marshal(attr); err == nil {
							c.d.addProp3(entryID, c.sourceID, b)
						}
						if idLogFile != nil {
							logProcessedID(idLogFile, entryID)
						}
						count++
						if testLimit > 0 && already+count >= uint64(testLimit) {
							break
						}
					}
				}
			}
		}
		if rerr == io.EOF {
			break
		}
	}
	return count
}
