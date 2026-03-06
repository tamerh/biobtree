package update

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

// httpClient is a custom HTTP client with timeouts configured for large file downloads
// Note: No overall Timeout set - that would limit body reading time for large files
var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second, // Connection timeout
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second, // TLS handshake timeout (fixes bgee.org issue)
		ResponseHeaderTimeout: 60 * time.Second, // Wait for response headers
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
	},
}

// httpGetWithRetry performs an HTTP GET request with retry logic for transient failures
func httpGetWithRetry(url string, maxRetries int) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 5s, 10s, 20s, ...
			backoff := time.Duration(5<<(attempt-1)) * time.Second
			log.Printf("Retry %d/%d for %s after %v...", attempt, maxRetries, url, backoff)
			time.Sleep(backoff)
		}

		resp, err := httpClient.Get(url)
		if err != nil {
			lastErr = err
			// Check if it's a transient error worth retrying
			if isTransientError(err) {
				continue
			}
			return nil, err
		}

		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		// Close body for non-OK responses before retry
		resp.Body.Close()

		// Retry on server errors (5xx) or rate limiting (429)
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			lastErr = fmt.Errorf("HTTP status %d", resp.StatusCode)
			continue
		}

		// Non-retryable client error
		return nil, fmt.Errorf("HTTP GET failed with status %d for %s", resp.StatusCode, url)
	}
	return nil, fmt.Errorf("failed after %d retries: %v", maxRetries, lastErr)
}

// isTransientError checks if an error is transient and worth retrying
// Conservative list - only clear network transient failures
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "TLS handshake timeout") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "i/o timeout")
}

// Helper function to decompress .Z (Unix compress) format using external uncompress command
func decompressZ(input io.Reader) (io.ReadCloser, error) {
	cmd := exec.Command("uncompress", "-c")
	cmd.Stdin = input
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe for uncompress: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start uncompress command: %v", err)
	}

	return stdout, nil
}

// parseFTPURL parses an FTP URL into host and path
// Supports formats: ftp://host/path, ftp.host.org/path
// Returns host with :21 port if not specified
func parseFTPURL(url string) (host, path string, err error) {
	// Remove ftp:// prefix if present
	url = strings.TrimPrefix(url, "ftp://")

	// Split on first /
	parts := strings.SplitN(url, "/", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid FTP URL: %s", url)
	}

	host = parts[0]
	// Add default FTP port if not present
	if !strings.Contains(host, ":") {
		host = host + ":21"
	}

	return host, "/" + parts[1], nil
}

func getDataReaderNew(datatype string, ftpAddr string, ftpPath string, filePath string) (*bufio.Reader, *gzip.Reader, *ftp.Response, *ftp.ServerConn, *os.File, int64, error) {

	var ftpfile *ftp.Response
	var client *ftp.ServerConn
	var file *os.File
	var err error
	var fileSize int64

	// Check if filePath is a full FTP URL - if so, parse it and use directly
	if strings.HasPrefix(filePath, "ftp://") {
		if ftpHost, ftpFullPath, parseErr := parseFTPURL(filePath); parseErr == nil {
			ftpAddr = ftpHost
			ftpPath = ""
			filePath = ftpFullPath
		}
	}

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
		ext := filepath.Ext(file.Name())

		if ext == ".gz" {
			gz, err := gzip.NewReader(file)
			if err != nil {
				file.Close()
				return nil, nil, nil, nil, nil, 0, err
			}
			br := bufio.NewReaderSize(gz, fileBufSize)
			return br, gz, nil, nil, file, fileSize, nil
		} else if ext == ".Z" {
			// Handle Unix compress format using external uncompress command
			zReader, err := decompressZ(file)
			if err != nil {
				file.Close()
				return nil, nil, nil, nil, nil, 0, fmt.Errorf("failed to decompress .Z file: %v", err)
			}
			br := bufio.NewReaderSize(zReader, fileBufSize)
			return br, nil, nil, nil, file, fileSize, nil
		}

		br := bufio.NewReaderSize(file, fileBufSize)
		return br, nil, nil, nil, file, fileSize, nil

	}

	// Handle direct HTTPS URLs (e.g., STRING, HGNC, etc.)
	if strings.HasPrefix(filePath, "https://") || strings.HasPrefix(filePath, "http://") {
		resp, err := httpGetWithRetry(filePath, 3)
		if err != nil {
			return nil, nil, nil, nil, nil, 0, fmt.Errorf("HTTP GET failed for %s: %v", filePath, err)
		}

		fileSize = resp.ContentLength

		var br *bufio.Reader
		var gz *gzip.Reader
		ext := filepath.Ext(filePath)

		if ext == ".gz" {
			gz, err = gzip.NewReader(resp.Body)
			if err != nil {
				resp.Body.Close()
				return nil, nil, nil, nil, nil, 0, err
			}
			br = bufio.NewReaderSize(gz, fileBufSize)
			return br, gz, nil, nil, nil, fileSize, nil
		} else if ext == ".Z" {
			// Handle Unix compress format using external uncompress command
			zReader, err := decompressZ(resp.Body)
			if err != nil {
				resp.Body.Close()
				return nil, nil, nil, nil, nil, 0, fmt.Errorf("failed to decompress .Z file: %v", err)
			}
			br = bufio.NewReaderSize(zReader, fileBufSize)
			return br, nil, nil, nil, nil, fileSize, nil
		} else {
			br = bufio.NewReaderSize(resp.Body, fileBufSize)
			return br, nil, nil, nil, nil, fileSize, nil
		}
	}

	// Try HTTPS for EBI (FTP protocol has been disabled)
	if strings.HasPrefix(ftpAddr, "ftp.ebi.ac.uk") {
		httpsURL := "https://ftp.ebi.ac.uk" + ftpPath + filePath
		resp, err := httpGetWithRetry(httpsURL, 3)
		if err == nil {
			fileSize = resp.ContentLength

			var br *bufio.Reader
			var gz *gzip.Reader
			ext := filepath.Ext(filePath)

			if ext == ".gz" {
				gz, err = gzip.NewReader(resp.Body)
				if err != nil {
					resp.Body.Close()
					return nil, nil, nil, nil, nil, 0, err
				}
				br = bufio.NewReaderSize(gz, fileBufSize)
				// For .gz files, gz.Close() will close the underlying resp.Body
				// so we return nil for the file parameter
				return br, gz, nil, nil, nil, fileSize, nil
			} else if ext == ".Z" {
				// Handle Unix compress format using external uncompress command
				zReader, err := decompressZ(resp.Body)
				if err != nil {
					resp.Body.Close()
					return nil, nil, nil, nil, nil, 0, fmt.Errorf("failed to decompress .Z file: %v", err)
				}
				br = bufio.NewReaderSize(zReader, fileBufSize)
				return br, nil, nil, nil, nil, fileSize, nil
			} else {
				// Non-compressed files
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

		resp, err := httpGetWithRetry(httpsURL, 3)
		if err != nil {
			log.Printf("DEBUG Ensembl: HTTPS request failed: %v\n", err)
			return nil, nil, nil, nil, nil, 0, err
		}

		log.Printf("DEBUG Ensembl: HTTPS succeeded\n")

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
		}
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		return br, nil, nil, nil, nil, fileSize, nil
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
// Returns true when currentCount >= testLimit, signaling processing should stop.
// testLimit < 0 means no limit (process all), testLimit = 0 means process nothing.
func shouldStopProcessing(testLimit int, currentCount int) bool {
	if testLimit < 0 {
		return false // No limit (negative values like -1 mean unlimited)
	}
	return currentCount >= testLimit
}
