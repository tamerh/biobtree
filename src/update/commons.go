package update

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jlaffaye/ftp"
)

func getDataReaderNew(datatype string, ftpAddr string, ftpPath string, filePath string) (*bufio.Reader, *gzip.Reader, *ftp.Response, *ftp.ServerConn, *os.File, int64, error) {

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	var file *os.File
	var err error
	var fileSize int64

	if _, ok := config.Dataconf[datatype]["useLocalFile"]; ok && config.Dataconf[datatype]["useLocalFile"] == "yes" {

		file, err = os.Open(filepath.FromSlash(filePath))
		if err != nil {
			return nil, nil, nil, nil, nil, 0, err
		}

		fileStat, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, nil, nil, nil, nil, 0, err
		}
		fileSize = fileStat.Size()

		if filepath.Ext(file.Name()) == ".gz" {
			gz, err := gzip.NewReader(file)
			if err != nil {
				file.Close()
				return nil, nil, nil, nil, nil, 0, err
			}
			br := bufio.NewReaderSize(gz, fileBufSize)
			return br, gz, nil, nil, file, fileSize, nil
		}

		br := bufio.NewReaderSize(file, fileBufSize)
		return br, nil, nil, nil, file, fileSize, nil

	}

	// Handle direct HTTPS URLs (e.g., STRING, HGNC, etc.)
	if strings.HasPrefix(filePath, "https://") || strings.HasPrefix(filePath, "http://") {
		resp, err := http.Get(filePath)
		if err != nil {
			return nil, nil, nil, nil, nil, 0, fmt.Errorf("HTTP GET failed for %s: %v", filePath, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, nil, nil, nil, nil, 0, fmt.Errorf("HTTP GET failed with status %d for %s", resp.StatusCode, filePath)
		}

		fileSize = resp.ContentLength

		var br *bufio.Reader
		var gz *gzip.Reader

		if filepath.Ext(filePath) == ".gz" {
			gz, err = gzip.NewReader(resp.Body)
			if err != nil {
				resp.Body.Close()
				return nil, nil, nil, nil, nil, 0, err
			}
			br = bufio.NewReaderSize(gz, fileBufSize)
			return br, gz, nil, nil, nil, fileSize, nil
		} else {
			br = bufio.NewReaderSize(resp.Body, fileBufSize)
			return br, nil, nil, nil, nil, fileSize, nil
		}
	}

	// Try HTTPS for EBI (FTP protocol has been disabled)
	if strings.HasPrefix(ftpAddr, "ftp.ebi.ac.uk") {
		httpsURL := "https://ftp.ebi.ac.uk" + ftpPath + filePath
		resp, err := http.Get(httpsURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			fileSize = resp.ContentLength

			var br *bufio.Reader
			var gz *gzip.Reader

			if filepath.Ext(filePath) == ".gz" {
				gz, err = gzip.NewReader(resp.Body)
				if err != nil {
					resp.Body.Close()
					return nil, nil, nil, nil, nil, 0, err
				}
				br = bufio.NewReaderSize(gz, fileBufSize)
				// For .gz files, gz.Close() will close the underlying resp.Body
				// so we return nil for the file parameter
				return br, gz, nil, nil, nil, fileSize, nil
			} else {
				// Non-.gz files: EBI datasets are typically always .gz, so this path is rarely used
				br = bufio.NewReaderSize(resp.Body, fileBufSize)
				return br, nil, nil, nil, nil, fileSize, nil
			}
		}
		// If HTTPS fails, fall through to try FTP
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}

	// For Ensembl: Try FTP first (HTTPS has certificate issues), then HTTPS as fallback
	isEnsembl := strings.HasPrefix(ftpAddr, "ftp.ensembl.org") || strings.HasPrefix(ftpAddr, "ftp.ensemblgenomes.org")

	if isEnsembl {
		// Try FTP first
		client = ftpClient(ftpAddr)
		path := ftpPath + filePath

		fileSize, err = client.FileSize(path)
		if err == nil {
			ftpfile, err = client.Retr(path)
			if err == nil {
				var br *bufio.Reader
				var gz *gzip.Reader

				if filepath.Ext(path) == ".gz" {
					gz, err = gzip.NewReader(ftpfile)
					if err != nil {
						ftpfile.Close()
						client.Quit()
						return nil, nil, nil, nil, nil, 0, err
					}
					br = bufio.NewReaderSize(gz, fileBufSize)
				} else {
					br = bufio.NewReaderSize(ftpfile, fileBufSize)
				}

				return br, gz, ftpfile, client, nil, fileSize, nil
			}
		}

		// FTP failed, clean up and try HTTPS as fallback
		if client != nil {
			client.Quit()
		}

		// Try HTTPS fallback
		hostOnly := strings.Split(ftpAddr, ":")[0]

		// Map to the correct EBI domain for HTTPS (certificate only valid for .ebi.ac.uk)
		if hostOnly == "ftp.ensembl.org" {
			hostOnly = "ftp.ensembl.ebi.ac.uk"
		} else if hostOnly == "ftp.ensemblgenomes.org" {
			hostOnly = "ftp.ensemblgenomes.ebi.ac.uk"
		}

		httpsURL := "https://" + hostOnly + ftpPath + filePath

		log.Printf("DEBUG Ensembl: FTP failed, trying HTTPS URL: %s\n", httpsURL)

		resp, err := http.Get(httpsURL)
		if err != nil {
			log.Printf("DEBUG Ensembl: HTTPS request failed: %v\n", err)
			return nil, nil, nil, nil, nil, 0, err
		}

		log.Printf("DEBUG Ensembl: HTTPS status code: %d\n", resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			fileSize = resp.ContentLength

			var br *bufio.Reader
			var gz *gzip.Reader

			if filepath.Ext(filePath) == ".gz" {
				gz, err = gzip.NewReader(resp.Body)
				if err != nil {
					resp.Body.Close()
					return nil, nil, nil, nil, nil, 0, err
				}
				br = bufio.NewReaderSize(gz, fileBufSize)
				return br, gz, nil, nil, nil, fileSize, nil
			} else {
				br = bufio.NewReaderSize(resp.Body, fileBufSize)
				return br, nil, nil, nil, nil, fileSize, nil
			}
		}

		// HTTPS also failed - return proper error
		if resp.Body != nil {
			resp.Body.Close()
		}
		return nil, nil, nil, nil, nil, 0, fmt.Errorf("HTTPS request failed with status %d for URL: %s", resp.StatusCode, httpsURL)
	}

	// For other FTP servers (not Ensembl, not EBI)
	client = ftpClient(ftpAddr)
	path := ftpPath + filePath

	fileSize, err = client.FileSize(path)
	if err != nil {
		client.Quit()
		return nil, nil, nil, nil, nil, 0, err
	}

	ftpfile, err = client.Retr(path)
	if err != nil {
		client.Quit()
		return nil, nil, nil, nil, nil, 0, err
	}

	var br *bufio.Reader
	var gz *gzip.Reader

	if filepath.Ext(path) == ".gz" {
		gz, err = gzip.NewReader(ftpfile)
		if err != nil {
			ftpfile.Close()
			client.Quit()
			return nil, nil, nil, nil, nil, 0, err
		}
		br = bufio.NewReaderSize(gz, fileBufSize)

	} else {
		br = bufio.NewReaderSize(ftpfile, fileBufSize)
	}

	return br, gz, ftpfile, client, nil, fileSize, nil

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

// Test mode utility functions

// openIDLogFile opens a file for logging processed IDs in test mode
// Returns nil if file cannot be created (non-fatal)
func openIDLogFile(referenceDir, filename string) *os.File {
	// Create reference directory if it doesn't exist
	if err := os.MkdirAll(referenceDir, 0755); err != nil {
		log.Printf("Warning: cannot create reference directory %s: %v", referenceDir, err)
		return nil
	}

	// Create log file
	filePath := filepath.Join(referenceDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Warning: cannot create ID log file %s: %v", filePath, err)
		return nil
	}

	log.Printf("[TEST MODE] Logging IDs to: %s", filePath)
	return file
}

// logProcessedID logs a single ID to the reference file
func logProcessedID(file *os.File, id string) {
	if file != nil {
		file.WriteString(id + "\n")
	}
}

// shouldStopProcessing checks if the processing should stop based on test limit
func shouldStopProcessing(testLimit int, currentCount int) bool {
	if testLimit <= 0 {
		return false // No limit
	}
	return currentCount >= testLimit
}
