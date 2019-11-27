package update

import (
	"bufio"
	"compress/gzip"
	"os"
	"path/filepath"

	"github.com/jlaffaye/ftp"
)

func getDataReaderNew(datatype string, ftpAddr string, ftpPath string, filePath string) (*bufio.Reader, *gzip.Reader, *ftp.Response, *ftp.ServerConn, *os.File, int64) {

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	var file *os.File
	var err error
	var fileSize int64

	if _, ok := config.Dataconf[datatype]["useLocalFile"]; ok && config.Dataconf[datatype]["useLocalFile"] == "yes" {

		file, err = os.Open(filepath.FromSlash(filePath))
		check(err)

		fileStat, err := file.Stat()
		check(err)
		fileSize = fileStat.Size()

		if filepath.Ext(file.Name()) == ".gz" {
			gz, err := gzip.NewReader(file)
			check(err)
			br := bufio.NewReaderSize(gz, fileBufSize)
			return br, gz, nil, nil, file, fileSize
		}

		br := bufio.NewReaderSize(file, fileBufSize)
		return br, nil, nil, nil, file, fileSize

	}

	// with ftp
	client = ftpClient(ftpAddr)
	path := ftpPath + filePath

	fileSize, err = client.FileSize(path)

	check(err)
	ftpfile, err = client.Retr(path)
	check(err)

	var br *bufio.Reader
	var gz *gzip.Reader

	if filepath.Ext(path) == ".gz" {
		gz, err = gzip.NewReader(ftpfile)
		check(err)
		br = bufio.NewReaderSize(gz, fileBufSize)

	} else {
		br = bufio.NewReaderSize(ftpfile, fileBufSize)
	}

	return br, gz, ftpfile, client, nil, fileSize

}

func ftpClient(ftpAddr string) *ftp.ServerConn {

	client, err := ftp.Dial(ftpAddr)
	if err != nil {
		panic("Error in ftp connection:" + err.Error())
	}

	if err := client.Login("anonymous", ""); err != nil {
		panic("Error in ftp login with anonymous:" + err.Error())
	}
	return client
}

func fileExists(name string) bool {

	if _, err := os.Stat(name); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		// Schrodinger: file may or may not exist. See err for details.
		// Therefore, do *NOT* use !os.IsNotExist(err) to test for file existence
		check(err)
		return false
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
