package update

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

// SourceType defines how to check for source updates
type SourceType string

const (
	SourceTypeFTPFile       SourceType = "ftp_file"       // Single FTP file - check date and size
	SourceTypeFTPFolder     SourceType = "ftp_folder"     // FTP folder - check folder mod date
	SourceTypeHTTPFile      SourceType = "http_file"      // HTTP file - use Last-Modified, ETag
	SourceTypeVersionedAPI  SourceType = "versioned_api"  // API with version endpoint
	SourceTypeReleaseFolder SourceType = "release_folder" // Release folder with version in path
	SourceTypeUnknown       SourceType = "unknown"        // No change detection available
)

// SourceChangeInfo holds information about source changes
type SourceChangeInfo struct {
	HasChanged  bool       `json:"has_changed"`
	SourceType  SourceType `json:"source_type"`
	SourceURL   string     `json:"source_url,omitempty"`
	NewVersion  string     `json:"new_version,omitempty"`
	NewDate     time.Time  `json:"new_date,omitempty"`
	NewSize     int64      `json:"new_size,omitempty"`
	NewETag     string     `json:"new_etag,omitempty"`
	CheckMethod string     `json:"check_method,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// SourceConfig holds source configuration for a dataset
type SourceConfig struct {
	SourceType     SourceType `json:"source_type"`
	SourceURL      string     `json:"source_url,omitempty"`      // FTP/HTTP URL
	VersionURL     string     `json:"version_url,omitempty"`     // API endpoint for version
	ReleasePattern string     `json:"release_pattern,omitempty"` // Pattern like "release-{version}"
}

// SourceTypeLocal is for local files (useLocalFile=yes)
const SourceTypeLocal SourceType = "local"

// SourceTypeMultiFile is for datasets with multiple source files
const SourceTypeMultiFile SourceType = "multi_file"

// GetSourceConfig returns the source configuration for a dataset
// Auto-detects source type from path if not explicitly configured
func GetSourceConfig(datasetName string) *SourceConfig {
	cfg := &SourceConfig{}

	// Check for Ensembl datasets first (by name, even if not in config)
	if isEnsemblDataset(datasetName) {
		cfg.SourceType = SourceTypeVersionedAPI
		return cfg
	}

	props, ok := config.Dataconf[datasetName]
	if !ok {
		return nil
	}

	// Check for explicit sourceType first
	if st, ok := props["sourceType"]; ok {
		cfg.SourceType = SourceType(st)
	}

	// Get explicit source URL if set
	if url, ok := props["sourceURL"]; ok {
		cfg.SourceURL = url
	}

	// Get version URL for API-based sources
	if url, ok := props["versionURL"]; ok {
		cfg.VersionURL = url
	}

	// Get release pattern
	if pattern, ok := props["releasePattern"]; ok {
		cfg.ReleasePattern = pattern
	}

	// If sourceType is not explicitly set, auto-detect from path
	if cfg.SourceType == "" || cfg.SourceType == SourceTypeUnknown {
		cfg.SourceType, cfg.SourceURL = autoDetectSourceType(datasetName, props)
	}

	return cfg
}

// autoDetectSourceType determines the source type from dataset properties
// Since all paths are now full URLs (ftp://, http://, https://), detection is straightforward
func autoDetectSourceType(datasetName string, props map[string]string) (SourceType, string) {
	// Check for Ensembl datasets first (by name)
	if isEnsemblDataset(datasetName) {
		return SourceTypeVersionedAPI, "" // Ensembl uses REST API version check
	}

	// Check for local files
	if useLocal, ok := props["useLocalFile"]; ok && (useLocal == "yes" || useLocal == "true") {
		return SourceTypeLocal, ""
	}

	path, hasPath := props["path"]
	if !hasPath || path == "" {
		return SourceTypeUnknown, ""
	}

	// Check if we have a primarySourceFile for this dataset (multi-file datasets)
	primaryFile := getPrimarySourceFileDefault(datasetName)
	if primaryFile != "" {
		if strings.HasPrefix(primaryFile, "http://") || strings.HasPrefix(primaryFile, "https://") {
			return SourceTypeHTTPFile, primaryFile
		}
		if strings.HasPrefix(primaryFile, "ftp://") {
			return SourceTypeFTPFile, primaryFile
		}
	}

	// Local path (absolute path starting with /data/ or Windows drive letter)
	if strings.HasPrefix(path, "/data/") || strings.Contains(path, ":\\") {
		return SourceTypeLocal, ""
	}

	// HTTP/HTTPS URL
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		if strings.HasSuffix(path, "/") {
			return SourceTypeHTTPFolder, path
		}
		return SourceTypeHTTPFile, path
	}

	// FTP URL (full URL starting with ftp://)
	if strings.HasPrefix(path, "ftp://") {
		if strings.HasSuffix(path, "/") {
			return SourceTypeFTPFolder, path
		}
		return SourceTypeFTPFile, path
	}

	return SourceTypeUnknown, ""
}

// isEnsemblDataset returns true if the dataset is an Ensembl variant
func isEnsemblDataset(datasetName string) bool {
	ensemblDatasets := []string{
		"ensembl", "ensembl_fungi", "ensembl_bacteria",
		"ensembl_metazoa", "ensembl_plants", "ensembl_protists",
	}
	for _, ds := range ensemblDatasets {
		if datasetName == ds {
			return true
		}
	}
	return false
}

// NOTE: getFullFTPURL and getDatasetFTPHost were removed as part of the URL simplification.
// All dataset paths in source*.dataset.json are now full URLs (ftp://, http://, https://),
// so there's no need to construct URLs from fragments.

// IsLocalDataset returns true if the dataset uses local files
func IsLocalDataset(datasetName string) bool {
	props, ok := config.Dataconf[datasetName]
	if !ok {
		return false
	}
	if useLocal, ok := props["useLocalFile"]; ok {
		return useLocal == "yes" || useLocal == "true"
	}
	// Check if path looks like a local path
	if path, ok := props["path"]; ok {
		return strings.Contains(path, "/data/") || strings.Contains(path, ":\\")
	}
	return false
}

// HasForceRebuild returns true if the dataset is configured for force rebuild
func HasForceRebuild(datasetName string) bool {
	if config == nil || config.Dataconf == nil {
		return false
	}

	props, ok := config.Dataconf[datasetName]
	if !ok {
		return false
	}
	if force, ok := props["forceRebuild"]; ok {
		return force == "yes" || force == "true"
	}
	return false
}

// GetPrimarySourceFile returns the primary file to check for multi-file datasets
// This is used when a dataset has multiple files but we want to check just one
func GetPrimarySourceFile(datasetName string) string {
	if config == nil || config.Dataconf == nil {
		return getPrimarySourceFileDefault(datasetName)
	}

	props, ok := config.Dataconf[datasetName]
	if !ok {
		return getPrimarySourceFileDefault(datasetName)
	}

	// Check for explicit primarySourceFile
	if primary, ok := props["primarySourceFile"]; ok {
		return primary
	}

	return getPrimarySourceFileDefault(datasetName)
}

// checkEnsemblVersion checks if Ensembl has a new release
// Uses the existing Ensembl version check mechanism
// Note: This ignores disableEnsemblReleaseCheck - check command always shows real status
func checkEnsemblVersion(datasetName string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeVersionedAPI,
		CheckMethod: "ensembl_rest_api",
	}

	// Get current version from REST API (ignores disableEnsemblReleaseCheck)
	latestVersion := getLatestEnsemblVersion()
	info.NewVersion = strconv.Itoa(latestVersion)

	// Compare with last build
	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	// Check if version changed
	if lastBuild.SourceVersion != "" {
		lastVersion, err := strconv.Atoi(lastBuild.SourceVersion)
		if err == nil {
			info.HasChanged = latestVersion != lastVersion
		} else {
			info.HasChanged = true
		}
	} else {
		// No previous version recorded - check paths.json file
		pathFile := filepath.Join(config.Appconf["ensemblDir"], datasetName+".paths.json")
		if _, err := os.Stat(pathFile); os.IsNotExist(err) {
			info.HasChanged = true
		} else {
			// Read version from paths.json
			data, err := os.ReadFile(pathFile)
			if err != nil {
				info.HasChanged = true
			} else {
				var paths struct {
					Version int `json:"version"`
				}
				if err := json.Unmarshal(data, &paths); err != nil {
					info.HasChanged = true
				} else {
					info.HasChanged = latestVersion != paths.Version
				}
			}
		}
	}

	log.Printf("Ensembl version check: current=%d changed=%v", latestVersion, info.HasChanged)
	return info, nil
}

// getPrimarySourceFileDefault returns default primary file mappings
func getPrimarySourceFileDefault(datasetName string) string {
	// Dataset-specific primary file mappings
	// For multi-file datasets, this specifies which single file to check for changes
	primaryFiles := map[string]string{
		// NCBI datasets
		"entrez":             "https://ftp.ncbi.nlm.nih.gov/gene/DATA/gene_info.gz",
		"dbsnp":              "https://ftp.ncbi.nlm.nih.gov/snp/latest_release/VCF/GCF_000001405.40.gz",
		"refseq":             "https://ftp.ncbi.nlm.nih.gov/genomes/refseq/vertebrate_mammalian/assembly_summary.txt",
		"literature_mappings": "https://ftp.ncbi.nlm.nih.gov/pub/pmc/DOI/PMID_PMCID_DOI.csv.gz",

		// EBI datasets
		"chebi":      "https://ftp.ebi.ac.uk/pub/databases/chebi/Flat_file_tab_delimited/compounds.tsv.gz",
		"rnacentral": "https://ftp.ebi.ac.uk/pub/databases/RNAcentral/current_release/sequences/rnacentral_active.fasta.gz",

		// Ontologies with multiple files
		"hpo": "https://github.com/obophenotype/human-phenotype-ontology/releases/download/v2025-11-24/hp-base.obo",

		// External databases
		"antibody": "https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/static/downloads/TheraSAbDab_SeqStruc_OnlineDownload.csv",
		"ctd":      "https://ctdbase.org/reports/CTD_chemicals.tsv.gz",
		"rhea":     "https://ftp.expasy.org/databases/rhea/tsv/rhea-directions.tsv",

		// STRING - check one species file
		"string": "https://stringdb-downloads.org/download/protein.info.v12.0/9606.protein.info.v12.0.txt.gz",
	}

	if primary, ok := primaryFiles[datasetName]; ok {
		return primary
	}

	return ""
}

// CheckSourceChanged checks if a remote source has changed since last build
func CheckSourceChanged(datasetName string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	// Check for dataset-level force rebuild setting first
	if HasForceRebuild(datasetName) {
		return &SourceChangeInfo{
			HasChanged:  true,
			SourceType:  SourceTypeUnknown,
			CheckMethod: "force_rebuild",
		}, nil
	}

	cfg := GetSourceConfig(datasetName)
	if cfg == nil {
		return &SourceChangeInfo{
			HasChanged: true, // Unknown source - assume changed
			SourceType: SourceTypeUnknown,
			Error:      "no source config found",
		}, nil
	}

	var result *SourceChangeInfo
	var err error
	var sourceURL string

	switch cfg.SourceType {
	case SourceTypeLocal:
		// Check local file modification time
		result, err = checkLocalFile(datasetName, lastBuild)
		// Get path for local files
		if props, ok := config.Dataconf[datasetName]; ok {
			sourceURL = props["path"]
		}
	case SourceTypeVersionedAPI:
		// Check if it's an Ensembl dataset
		if isEnsemblDataset(datasetName) {
			result, err = checkEnsemblVersion(datasetName, lastBuild)
			sourceURL = cfg.VersionURL
		} else {
			// Other versioned APIs
			result, err = checkVersionedAPI(cfg.VersionURL, lastBuild)
			sourceURL = cfg.VersionURL
		}
	case SourceTypeFTPFile:
		result, err = checkFTPFile(cfg.SourceURL, lastBuild)
		sourceURL = cfg.SourceURL
	case SourceTypeFTPFolder:
		result, err = checkFTPFolder(cfg.SourceURL, lastBuild)
		sourceURL = cfg.SourceURL
	case SourceTypeHTTPFile:
		result, err = checkHTTPFile(cfg.SourceURL, lastBuild)
		sourceURL = cfg.SourceURL
	case SourceTypeHTTPFolder:
		result, err = checkHTTPFolder(cfg.SourceURL, lastBuild)
		sourceURL = cfg.SourceURL
	case SourceTypeReleaseFolder:
		result, err = checkReleaseFolder(cfg.SourceURL, cfg.ReleasePattern, lastBuild)
		sourceURL = cfg.SourceURL
	case SourceTypeMultiFile:
		// Multi-file datasets: check primary file if available
		primaryFile := GetPrimarySourceFile(datasetName)
		if primaryFile != "" {
			sourceURL = primaryFile
			if strings.HasPrefix(primaryFile, "http://") || strings.HasPrefix(primaryFile, "https://") {
				result, err = checkHTTPFile(primaryFile, lastBuild)
			} else {
				// Assume FTP for other cases
				result, err = checkFTPFile(primaryFile, lastBuild)
			}
		} else {
			// No primary file defined - assume changed
			return &SourceChangeInfo{
				HasChanged:  true,
				SourceType:  SourceTypeMultiFile,
				CheckMethod: "no_primary_file",
				Error:       "multi-file dataset with no primary file defined",
			}, nil
		}
	default:
		return &SourceChangeInfo{
			HasChanged: true, // Unknown type - assume changed
			SourceType: cfg.SourceType,
			Error:      "unknown source type",
		}, nil
	}

	// Set the source URL on the result
	if result != nil && sourceURL != "" {
		result.SourceURL = sourceURL
	}

	return result, err
}

// checkLocalFile checks a local file or directory for changes by comparing modification time
func checkLocalFile(datasetName string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeLocal,
		CheckMethod: "local_file_date",
	}

	// Get path from config
	props, ok := config.Dataconf[datasetName]
	if !ok {
		info.Error = "dataset not found in config"
		info.HasChanged = true
		return info, nil
	}

	path := props["path"]
	if path == "" {
		info.Error = "no path defined"
		info.HasChanged = true
		return info, nil
	}

	// Stat the file or directory
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			info.Error = fmt.Sprintf("path does not exist: %s", path)
		} else {
			info.Error = fmt.Sprintf("stat failed: %v", err)
		}
		info.HasChanged = true
		return info, nil
	}

	// For directories, find the newest file modification time
	if fileInfo.IsDir() {
		newestTime, err := getNewestFileInDir(path)
		if err != nil {
			info.Error = fmt.Sprintf("dir scan failed: %v", err)
			info.HasChanged = true
			return info, nil
		}
		info.NewDate = newestTime
	} else {
		info.NewDate = fileInfo.ModTime()
	}

	// Compare with last build
	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	// Changed if file is newer than last build time
	if lastBuild.SourceDate.IsZero() {
		// No source date recorded - compare with build time
		info.HasChanged = info.NewDate.After(lastBuild.LastBuildTime)
	} else {
		info.HasChanged = info.NewDate.After(lastBuild.SourceDate)
	}

	log.Printf("Local file check %s: date=%s changed=%v",
		path, info.NewDate.Format(time.RFC3339), info.HasChanged)

	return info, nil
}

// getNewestFileInDir recursively finds the newest file modification time in a directory
func getNewestFileInDir(dirPath string) (time.Time, error) {
	var newestTime time.Time
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // Skip files we can't access
		}
		if info.IsDir() {
			return nil
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
		}
		return nil
	})
	return newestTime, err
}

// checkFTPFile checks an FTP file for changes by comparing date and size
func checkFTPFile(ftpURL string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeFTPFile,
		CheckMethod: "ftp_list",
	}

	// Parse FTP URL: ftp://host/path
	host, path, err := parseFTPURL(ftpURL)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true // Error - assume changed
		return info, err
	}

	// Connect to FTP server
	conn, err := ftp.Dial(host, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		info.Error = fmt.Sprintf("ftp dial failed: %v", err)
		info.HasChanged = true
		return info, err
	}
	defer conn.Quit()

	// Anonymous login
	if err := conn.Login("anonymous", "anonymous@"); err != nil {
		info.Error = fmt.Sprintf("ftp login failed: %v", err)
		info.HasChanged = true
		return info, err
	}

	// Get file info
	entries, err := conn.List(path)
	if err != nil || len(entries) == 0 {
		info.Error = fmt.Sprintf("ftp list failed: %v", err)
		info.HasChanged = true
		return info, err
	}

	entry := entries[0]
	info.NewDate = entry.Time
	info.NewSize = int64(entry.Size)

	// Compare with last build
	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	// Changed if date differs (fall back to LastBuildTime if SourceDate not recorded)
	compareDate := lastBuild.SourceDate
	if compareDate.IsZero() {
		compareDate = lastBuild.LastBuildTime
	}
	info.HasChanged = entry.Time.After(compareDate)

	log.Printf("FTP check %s: date=%s changed=%v (compared to %s)",
		path, entry.Time.Format(time.RFC3339), info.HasChanged, compareDate.Format(time.RFC3339))

	return info, nil
}

// checkFTPFolder checks an FTP folder for changes by checking folder modification date
func checkFTPFolder(ftpURL string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeFTPFolder,
		CheckMethod: "ftp_folder_date",
	}

	host, path, err := parseFTPURL(ftpURL)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}

	conn, err := ftp.Dial(host, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		info.Error = fmt.Sprintf("ftp dial failed: %v", err)
		info.HasChanged = true
		return info, err
	}
	defer conn.Quit()

	if err := conn.Login("anonymous", "anonymous@"); err != nil {
		info.Error = fmt.Sprintf("ftp login failed: %v", err)
		info.HasChanged = true
		return info, err
	}

	// List folder contents and find newest file
	entries, err := conn.List(path)
	if err != nil {
		info.Error = fmt.Sprintf("ftp list failed: %v", err)
		info.HasChanged = true
		return info, err
	}

	var newestTime time.Time
	for _, entry := range entries {
		if entry.Time.After(newestTime) {
			newestTime = entry.Time
		}
	}

	info.NewDate = newestTime

	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	// Fall back to LastBuildTime if SourceDate not recorded
	compareDate := lastBuild.SourceDate
	if compareDate.IsZero() {
		compareDate = lastBuild.LastBuildTime
	}
	info.HasChanged = newestTime.After(compareDate)

	log.Printf("FTP folder check %s: newest=%s changed=%v (compared to %s)",
		path, newestTime.Format(time.RFC3339), info.HasChanged, compareDate.Format(time.RFC3339))

	return info, nil
}

// SourceTypeHTTPFolder is for HTTP directories (listing files)
const SourceTypeHTTPFolder SourceType = "http_folder"

// checkHTTPFolder checks an HTTP folder by listing files and finding the newest modification date
func checkHTTPFolder(url string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeHTTPFolder,
		CheckMethod: "http_folder_listing",
	}

	// GET the directory listing
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
		info.HasChanged = true
		return info, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}

	// Parse HTML to find file links
	files := parseHTTPDirectoryListing(string(body), url)
	if len(files) == 0 {
		info.Error = "no files found in directory listing"
		info.HasChanged = true
		return info, nil
	}

	// Check each file's Last-Modified and find the newest
	var newestTime time.Time
	for _, fileURL := range files {
		fileTime, err := getHTTPFileDate(fileURL)
		if err != nil {
			continue // Skip files we can't check
		}
		if fileTime.After(newestTime) {
			newestTime = fileTime
		}
	}

	if newestTime.IsZero() {
		info.Error = "could not get modification dates for any files"
		info.HasChanged = true
		return info, nil
	}

	info.NewDate = newestTime

	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	// Changed if newest file is newer than last build
	if lastBuild.SourceDate.IsZero() {
		info.HasChanged = newestTime.After(lastBuild.LastBuildTime)
	} else {
		info.HasChanged = newestTime.After(lastBuild.SourceDate)
	}

	log.Printf("HTTP folder check %s: newest=%s changed=%v",
		url, newestTime.Format(time.RFC3339), info.HasChanged)

	return info, nil
}

// parseHTTPDirectoryListing extracts file URLs from an HTML directory listing
func parseHTTPDirectoryListing(html, baseURL string) []string {
	var files []string

	// Common patterns for directory listings:
	// <a href="filename.txt">
	// <a href="./filename.txt">
	hrefRegex := regexp.MustCompile(`href=["']([^"']+)["']`)
	matches := hrefRegex.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		href := match[1]

		// Skip parent directory links, anchors, query strings, etc.
		if href == ".." || href == "../" || href == "./" || href == "/" {
			continue
		}
		if strings.HasPrefix(href, "?") || strings.HasPrefix(href, "#") {
			continue
		}
		// Skip directory links (ending with /)
		if strings.HasSuffix(href, "/") {
			continue
		}

		// Build full URL
		var fullURL string
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			fullURL = href
		} else if strings.HasPrefix(href, "./") {
			fullURL = strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(href, "./")
		} else if strings.HasPrefix(href, "/") {
			// Absolute path - need to extract host
			// For simplicity, skip absolute paths that don't match base
			continue
		} else {
			fullURL = strings.TrimSuffix(baseURL, "/") + "/" + href
		}

		files = append(files, fullURL)
	}

	return files
}

// getHTTPFileDate gets the Last-Modified date for an HTTP file
func getHTTPFileDate(url string) (time.Time, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return time.Time{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	// Get Last-Modified header
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		return time.Parse(time.RFC1123, lm)
	}

	return time.Time{}, fmt.Errorf("no Last-Modified header")
}

// checkHTTPFile checks an HTTP file for changes using HEAD request
func checkHTTPFile(url string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeHTTPFile,
		CheckMethod: "http_head",
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}

	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
		info.HasChanged = true
		return info, nil
	}

	// Get Last-Modified header
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := time.Parse(time.RFC1123, lm); err == nil {
			info.NewDate = t
		}
	}

	// Get ETag header
	info.NewETag = resp.Header.Get("ETag")

	// Get Content-Length
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if size, err := strconv.ParseInt(cl, 10, 64); err == nil {
			info.NewSize = size
		}
	}

	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	// Compare ETag first (most reliable)
	if info.NewETag != "" && lastBuild.SourceETag != "" {
		info.HasChanged = info.NewETag != lastBuild.SourceETag
	} else if !info.NewDate.IsZero() && !lastBuild.SourceDate.IsZero() {
		// Fall back to date comparison
		info.HasChanged = info.NewDate.After(lastBuild.SourceDate)
	} else if info.NewSize != lastBuild.SourceSize {
		// Fall back to size comparison
		info.HasChanged = true
	}

	log.Printf("HTTP check %s: etag=%s date=%s size=%d changed=%v",
		url, info.NewETag, info.NewDate.Format(time.RFC3339), info.NewSize, info.HasChanged)

	return info, nil
}

// checkVersionedAPI checks a versioned API for changes
// Expects JSON response with "version" or "api_version" field
func checkVersionedAPI(url string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeVersionedAPI,
		CheckMethod: "api_version",
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
		info.HasChanged = true
		return info, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}

	// Parse JSON response
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}

	// Look for version field
	for _, key := range []string{"version", "api_version", "chembl_release", "release"} {
		if v, ok := data[key]; ok {
			info.NewVersion = fmt.Sprintf("%v", v)
			break
		}
	}

	if info.NewVersion == "" {
		info.Error = "no version field found in response"
		info.HasChanged = true
		return info, nil
	}

	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	info.HasChanged = info.NewVersion != lastBuild.SourceVersion

	log.Printf("API check %s: version=%s (was %s) changed=%v",
		url, info.NewVersion, lastBuild.SourceVersion, info.HasChanged)

	return info, nil
}

// checkReleaseFolder checks a release folder for new versions
// Looks for folders matching the release pattern (e.g., "release-113")
func checkReleaseFolder(ftpURL, pattern string, lastBuild *DatasetBuildInfo) (*SourceChangeInfo, error) {
	info := &SourceChangeInfo{
		SourceType:  SourceTypeReleaseFolder,
		CheckMethod: "release_folder",
	}

	host, path, err := parseFTPURL(ftpURL)
	if err != nil {
		info.Error = err.Error()
		info.HasChanged = true
		return info, err
	}

	conn, err := ftp.Dial(host, ftp.DialWithTimeout(30*time.Second))
	if err != nil {
		info.Error = fmt.Sprintf("ftp dial failed: %v", err)
		info.HasChanged = true
		return info, err
	}
	defer conn.Quit()

	if err := conn.Login("anonymous", "anonymous@"); err != nil {
		info.Error = fmt.Sprintf("ftp login failed: %v", err)
		info.HasChanged = true
		return info, err
	}

	entries, err := conn.List(path)
	if err != nil {
		info.Error = fmt.Sprintf("ftp list failed: %v", err)
		info.HasChanged = true
		return info, err
	}

	// Build regex from pattern (e.g., "release-{version}" -> "release-(\d+)")
	regexPattern := strings.Replace(pattern, "{version}", `(\d+)`, 1)
	re := regexp.MustCompile(regexPattern)

	var maxVersion int
	for _, entry := range entries {
		if entry.Type != ftp.EntryTypeFolder {
			continue
		}
		matches := re.FindStringSubmatch(entry.Name)
		if len(matches) >= 2 {
			if v, err := strconv.Atoi(matches[1]); err == nil && v > maxVersion {
				maxVersion = v
			}
		}
	}

	if maxVersion == 0 {
		info.Error = "no matching release folders found"
		info.HasChanged = true
		return info, nil
	}

	info.NewVersion = fmt.Sprintf("%d", maxVersion)

	if lastBuild == nil {
		info.HasChanged = true
		return info, nil
	}

	// Compare versions
	lastVersion, _ := strconv.Atoi(lastBuild.SourceVersion)
	info.HasChanged = maxVersion > lastVersion

	log.Printf("Release folder check %s: version=%d (was %d) changed=%v",
		path, maxVersion, lastVersion, info.HasChanged)

	return info, nil
}

// parseFTPURL is defined in commons.go

// CheckMultipleDatasets checks multiple datasets for changes in parallel
// Returns a map of dataset name to change info
func CheckMultipleDatasets(datasetNames []string, state *DatasetState) map[string]*SourceChangeInfo {
	results := make(map[string]*SourceChangeInfo)
	resultChan := make(chan struct {
		name string
		info *SourceChangeInfo
	}, len(datasetNames))

	// Check each dataset in parallel
	for _, name := range datasetNames {
		go func(dsName string) {
			lastBuild := state.GetDatasetInfo(dsName)
			info, _ := CheckSourceChanged(dsName, lastBuild)
			resultChan <- struct {
				name string
				info *SourceChangeInfo
			}{dsName, info}
		}(name)
	}

	// Collect results
	for i := 0; i < len(datasetNames); i++ {
		result := <-resultChan
		results[result.name] = result.info
	}

	return results
}

// GetChangedDatasets returns list of datasets that have changed
func GetChangedDatasets(datasetNames []string, state *DatasetState) []string {
	results := CheckMultipleDatasets(datasetNames, state)

	var changed []string
	for name, info := range results {
		if info.HasChanged {
			changed = append(changed, name)
		}
	}

	return changed
}
