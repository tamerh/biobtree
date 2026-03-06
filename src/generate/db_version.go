package generate

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// DBVersionConfig holds configuration for database versioning
type DBVersionConfig struct {
	KeepVersions int  // Number of versions to keep (default: 2)
	DryRun       bool // If true, don't actually create/delete anything
}

// DefaultDBVersionConfig returns the default versioning configuration
func DefaultDBVersionConfig() DBVersionConfig {
	return DBVersionConfig{
		KeepVersions: 2,
		DryRun:       false,
	}
}

// dbVersionRegex matches db_v{N} directories
var dbVersionRegex = regexp.MustCompile(`^db_v(\d+)$`)

// GetDBVersions returns a sorted list of version numbers found in the federation directory
// e.g., if there are db_v1, db_v3 directories, returns [1, 3]
func GetDBVersions(federationDir string) ([]int, error) {
	entries, err := os.ReadDir(federationDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []int{}, nil
		}
		return nil, err
	}

	var versions []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		matches := dbVersionRegex.FindStringSubmatch(entry.Name())
		if matches != nil {
			v, _ := strconv.Atoi(matches[1])
			versions = append(versions, v)
		}
	}

	sort.Ints(versions)
	return versions, nil
}

// GetLatestVersion returns the highest version number, or 0 if none exist
func GetLatestVersion(federationDir string) (int, error) {
	versions, err := GetDBVersions(federationDir)
	if err != nil {
		return 0, err
	}
	if len(versions) == 0 {
		return 0, nil
	}
	return versions[len(versions)-1], nil
}

// GetCurrentSymlinkVersion returns the version number that the 'db' symlink points to
// Returns -1 if symlink doesn't exist or doesn't point to a versioned directory
func GetCurrentSymlinkVersion(federationDir string) (int, error) {
	symlinkPath := filepath.Join(federationDir, "db")

	target, err := os.Readlink(symlinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		// Not a symlink (might be a real directory from old format)
		return -1, nil
	}

	// Extract version from target (e.g., "db_v3" -> 3)
	matches := dbVersionRegex.FindStringSubmatch(filepath.Base(target))
	if matches == nil {
		return -1, nil
	}

	v, _ := strconv.Atoi(matches[1])
	return v, nil
}

// GetNextVersionDir returns the path for the next version directory
// e.g., if db_v3 is the latest, returns "/path/to/federation/db_v4"
func GetNextVersionDir(federationDir string) (string, int, error) {
	latest, err := GetLatestVersion(federationDir)
	if err != nil {
		return "", 0, err
	}

	nextVersion := latest + 1
	nextDir := filepath.Join(federationDir, fmt.Sprintf("db_v%d", nextVersion))
	return nextDir, nextVersion, nil
}

// CreateVersionedDBDir creates a new versioned db directory for the current build
// Returns the path to the new directory and its version number
func CreateVersionedDBDir(federationDir string) (string, int, error) {
	// Ensure federation directory exists
	if err := os.MkdirAll(federationDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create federation dir: %w", err)
	}

	nextDir, version, err := GetNextVersionDir(federationDir)
	if err != nil {
		return "", 0, err
	}

	log.Printf("Creating versioned database directory: %s (version %d)", nextDir, version)

	if err := os.MkdirAll(nextDir, 0700); err != nil {
		return "", 0, fmt.Errorf("failed to create db version dir: %w", err)
	}

	return nextDir, version, nil
}

// UpdateDBSymlink updates the 'db' symlink to point to a specific version
// If there's an existing symlink, it's removed first
// If there's an existing 'db' directory (old format), it's renamed to db_v0
func UpdateDBSymlink(federationDir string, version int) error {
	symlinkPath := filepath.Join(federationDir, "db")
	targetDir := fmt.Sprintf("db_v%d", version)
	targetPath := filepath.Join(federationDir, targetDir)

	// Verify target exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return fmt.Errorf("target db directory does not exist: %s", targetPath)
	}

	// Check what currently exists at the symlink path
	info, err := os.Lstat(symlinkPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - remove it
			if err := os.Remove(symlinkPath); err != nil {
				return fmt.Errorf("failed to remove existing symlink: %w", err)
			}
		} else if info.IsDir() {
			// It's a real directory (legacy format) - rename it to db_v0
			legacyPath := filepath.Join(federationDir, "db_v0")
			log.Printf("Migrating legacy db directory to %s", legacyPath)
			if err := os.Rename(symlinkPath, legacyPath); err != nil {
				return fmt.Errorf("failed to rename legacy db dir: %w", err)
			}
		}
	}

	// Create new symlink (using relative path so it works after directory moves)
	log.Printf("Updating db symlink: %s -> %s", symlinkPath, targetDir)
	if err := os.Symlink(targetDir, symlinkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// CleanupOldVersions removes old db versions, keeping the most recent N
func CleanupOldVersions(federationDir string, keepCount int) error {
	if keepCount < 1 {
		keepCount = 1 // Always keep at least the current version
	}

	versions, err := GetDBVersions(federationDir)
	if err != nil {
		return err
	}

	if len(versions) <= keepCount {
		return nil
	}

	// Get current symlink version to ensure we don't delete it
	currentVersion, _ := GetCurrentSymlinkVersion(federationDir)

	// Find versions to delete (oldest ones, but never delete current)
	toDelete := versions[:len(versions)-keepCount]

	for _, v := range toDelete {
		if v == currentVersion {
			log.Printf("Skipping deletion of db_v%d (current version)", v)
			continue
		}
		dirPath := filepath.Join(federationDir, fmt.Sprintf("db_v%d", v))
		log.Printf("Removing old db version: %s", dirPath)
		if err := os.RemoveAll(dirPath); err != nil {
			log.Printf("Warning: failed to remove old version %s: %v", dirPath, err)
		}
	}

	return nil
}

// ListDBVersions returns info about all available database versions
type DBVersionInfo struct {
	Version   int
	Path      string
	IsCurrent bool
	SizeBytes int64
}

func ListDBVersions(federationDir string) ([]DBVersionInfo, error) {
	versions, err := GetDBVersions(federationDir)
	if err != nil {
		return nil, err
	}

	currentVersion, _ := GetCurrentSymlinkVersion(federationDir)

	var result []DBVersionInfo
	for _, v := range versions {
		dirPath := filepath.Join(federationDir, fmt.Sprintf("db_v%d", v))

		// Get directory size (sum of all files)
		var size int64
		filepath.Walk(dirPath, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				size += info.Size()
			}
			return nil
		})

		result = append(result, DBVersionInfo{
			Version:   v,
			Path:      dirPath,
			IsCurrent: v == currentVersion,
			SizeBytes: size,
		})
	}

	return result, nil
}

// SwitchDBVersion updates the symlink to point to a different version
func SwitchDBVersion(federationDir string, version int) error {
	versions, err := GetDBVersions(federationDir)
	if err != nil {
		return err
	}

	// Check if version exists
	found := false
	for _, v := range versions {
		if v == version {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("version %d not found. Available versions: %v", version, versions)
	}

	return UpdateDBSymlink(federationDir, version)
}
