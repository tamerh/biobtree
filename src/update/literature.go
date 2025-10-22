package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	xmlparser "github.com/tamerh/xml-stream-parser"
	"github.com/vbauerster/mpb"
)

type literature struct {
	source string
	d      *DataUpdate
}

func (l *literature) update() {
	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	client = ftpClient(l.d.ebiFtp)

	// first doi pm and pmcid mappings
	path := l.d.ebiFtpPath + config.Dataconf[l.source]["path"]
	ftpfile, err := client.Retr(path)

	var gz *gzip.Reader
	var br *bufio.Reader

	gz, err = gzip.NewReader(ftpfile)
	check(err)
	br = bufio.NewReaderSize(gz, fileBufSize)

	/**
	scanner := bufio.NewScanner(gz)
	for scanner.Scan() {
		lineByParser(scanner.Text())
	}
	*/

	defer l.d.wg.Done()

	r := csv.NewReader(br)
	//r.Comma = '	'
	pubmedfr := config.Dataconf["PubMed"]["id"]
	pmcfr := config.Dataconf["PMC"]["id"]
	doiprefix := config.Dataconf[l.source]["doiPrefix"]

	var previous int64
	totalRead := 0

	i := 0
	for {
		i++
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if i == 1 {
			continue
		}
		if len(record) < 3 {
			continue
		}
		if len(record[0]) > 0 {
			if len(record[1]) > 0 {
				l.d.addXref(record[0], pubmedfr, record[1], "PMC", false)
			}
			if len(record[2]) > 0 {

				doiid := strings.TrimSpace(strings.TrimPrefix(record[2], doiprefix))
				if len(doiid) > 0 {
					l.d.addXref(record[0], pubmedfr, doiid, "DOI", false)
				}
			}
		}

		if len(record[1]) > 0 {
			if len(record[2]) > 0 {
				doiid := strings.TrimSpace(strings.TrimPrefix(record[2], doiprefix))
				if len(doiid) > 0 {
					l.d.addXref(record[1], pmcfr, doiid, "DOI", false)
				}
			}
		}

		for _, r := range record {
			totalRead = totalRead + len(r)
		}

		elapsed := int64(time.Since(l.d.start).Seconds())
		if elapsed > previous+l.d.progInterval {
			kbytesPerSecond := int64(totalRead) / elapsed / 1024
			previous = elapsed
			l.d.progChan <- &progressInfo{dataset: l.source, currentKBPerSec: kbytesPerSecond}
		}

	}

	ftpfile.Close()

	l.d.progChan <- &progressInfo{dataset: l.source, done: true}
}

func (l *literature) literatureMappings2NotUsed(source string) {

	defer l.d.wg.Done()

	if l.d.p == nil {
		l.d.p = mpb.New(mpb.WithWaitGroup(l.d.wg))
	}

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	client = ftpClient(l.d.ebiFtp)

	// first doi pm and pmcid mappings
	path := l.d.ebiFtpPath + "/pmc/PMCLiteMetadata/PMCLiteMetadata.tgz"
	ftpfile, err := client.Retr(path)

	var gz *gzip.Reader
	var br *bufio.Reader

	gz, err = gzip.NewReader(ftpfile)
	check(err)
	br = bufio.NewReaderSize(gz, fileBufSize)

	tr := tar.NewReader(br)

	pubmedfr := config.Dataconf["PubMed"]["id"]
	pmcfr := config.Dataconf["PMC"]["id"]

	for {

		hdr, err := tr.Next()

		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		if filepath.Ext(hdr.Name) == ".xml" {

			// after each parsing parsing channel is closed for each file seperate one needed
			bbr := bufio.NewReaderSize(tr, fileBufSize)

			p := xmlparser.NewXMLParser(bbr, "PMC_ARTICLE").SkipElements([]string{"AuthorList,journalTitle"})

			for r := range p.Stream() {
				// accs
				var pmid, doi, pmcid string
				for _, v := range r.Childs["pmid"] {
					pmid = v.InnerText
					break
				}
				for _, v := range r.Childs["pmcid"] {
					pmcid = v.InnerText
					break
				}
				for _, v := range r.Childs["DOI"] {
					doi = v.InnerText
					break
				}

				if len(pmid) > 0 && len(pmcid) > 0 {
					//l.d.addXrefSingle(&pmid, pubmedfr, "PMC", pmcid)
					l.d.addXref(pmid, pubmedfr, pmcid, "PMC", false)

				}

				if len(pmid) > 0 && len(doi) > 0 {
					l.d.addXref(pmid, pubmedfr, doi, "DOI", false)
				}

				if len(pmcid) > 0 && len(doi) > 0 {
					l.d.addXref(pmcid, pmcfr, doi, "DOI", false)
				}

			}

		}

	}

}
