package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AliasEntry represents a single alias definition
type AliasEntry struct {
	Description string   `json:"description,omitempty"`
	IDs         []string `json:"ids,omitempty"`  // Inline IDs for small aliases
	File        string   `json:"file,omitempty"` // External file for large aliases
}

// AliasStore manages alias definitions loaded from JSON
type AliasStore struct {
	aliases  map[string]*AliasEntry
	confDir  string
	cache    map[string][]string // Cache for file-based aliases
	cacheMu  sync.RWMutex
}

// NewAliasStore creates a new alias store and loads aliases from the config directory
func NewAliasStore(confDir string) (*AliasStore, error) {
	store := &AliasStore{
		aliases: make(map[string]*AliasEntry),
		confDir: confDir,
		cache:   make(map[string][]string),
	}

	aliasFile := filepath.Join(confDir, "aliases.json")

	// Check if file exists
	if _, err := os.Stat(aliasFile); os.IsNotExist(err) {
		log.Printf("No aliases.json found at %s, aliases feature disabled", aliasFile)
		return store, nil
	}

	data, err := ioutil.ReadFile(aliasFile)
	if err != nil {
		return nil, fmt.Errorf("reading aliases.json: %w", err)
	}

	// Parse JSON - keys starting with "_" are comments/examples, skip them
	var rawAliases map[string]*AliasEntry
	if err := json.Unmarshal(data, &rawAliases); err != nil {
		return nil, fmt.Errorf("parsing aliases.json: %w", err)
	}

	// Filter out entries starting with "_" (comments/examples)
	for name, entry := range rawAliases {
		if !strings.HasPrefix(name, "_") {
			store.aliases[name] = entry
		}
	}

	log.Printf("Loaded %d aliases from %s", len(store.aliases), aliasFile)
	return store, nil
}

// GetIDs returns the IDs for the given alias name
func (s *AliasStore) GetIDs(aliasName string) ([]string, error) {
	entry, ok := s.aliases[aliasName]
	if !ok {
		return nil, fmt.Errorf("undefined alias: %s", aliasName)
	}

	// If inline IDs, return directly
	if len(entry.IDs) > 0 {
		return entry.IDs, nil
	}

	// If file reference, load from file (with caching)
	if entry.File != "" {
		return s.loadFromFile(aliasName, entry.File)
	}

	return nil, fmt.Errorf("alias %s has no ids or file defined", aliasName)
}

// loadFromFile loads IDs from an external file, with caching
func (s *AliasStore) loadFromFile(aliasName, filename string) ([]string, error) {
	// Check cache first
	s.cacheMu.RLock()
	if cached, ok := s.cache[aliasName]; ok {
		s.cacheMu.RUnlock()
		return cached, nil
	}
	s.cacheMu.RUnlock()

	// Resolve file path relative to conf directory
	filePath := filepath.Join(s.confDir, filename)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening alias file %s: %w", filePath, err)
	}
	defer file.Close()

	var ids []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			ids = append(ids, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading alias file %s: %w", filePath, err)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("alias file %s is empty", filePath)
	}

	// Cache the result
	s.cacheMu.Lock()
	s.cache[aliasName] = ids
	s.cacheMu.Unlock()

	log.Printf("Loaded %d IDs from alias file %s for alias %s", len(ids), filename, aliasName)
	return ids, nil
}

// ListAliases returns all available alias names
func (s *AliasStore) ListAliases() []string {
	names := make([]string, 0, len(s.aliases))
	for name := range s.aliases {
		names = append(names, name)
	}
	return names
}

// HasAlias checks if an alias exists
func (s *AliasStore) HasAlias(name string) bool {
	_, ok := s.aliases[name]
	return ok
}
