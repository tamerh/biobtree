package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// ensureDir creates directory if it doesn't exist
func ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// NewEnv creates a new database environment based on the backend type
// Returns: Env, DBI, error
func NewEnv(backend Backend, cfg *Config) (Env, DBI, error) {
	switch backend {
	case BackendLMDB:
		return NewLMDBEnv(cfg)
	case BackendMDBX:
		return NewMDBXEnv(cfg)
	default:
		return nil, nil, fmt.Errorf("unknown backend: %s", backend)
	}
}

// OpenDB opens a database with the specified backend (compatible with existing code)
// This maintains backward compatibility while adding backend selection
func (d *DB) OpenDBWithBackend(backend Backend, write bool, totalKV int64, appconf map[string]string) (Env, DBI, error) {
	// Determine map size
	var lmdbAllocSize int64
	if val, ok := appconf["lmdbAllocSize"]; ok {
		size, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid lmdbAllocSize: %v", err)
		}
		if size <= 1 {
			return nil, nil, fmt.Errorf("lmdbAllocSize must be greater than 1")
		}
		lmdbAllocSize = size
	}

	// Create config
	cfg := &Config{
		Backend:   backend,
		Dir:       filepath.FromSlash(appconf["dbDir"]),
		MapSize:   lmdbAllocSize,
		MaxDBs:    1,
		WriteMode: write,
		TotalKV:   totalKV,
		AppConf:   appconf,
	}

	return NewEnv(backend, cfg)
}

// OpenAliasDBWithBackend opens an alias database with the specified backend
func (d *DB) OpenAliasDBWithBackend(backend Backend, write bool, size int64, appconf map[string]string) (Env, DBI, error) {
	lmdbSize := size * 2

	cfg := &Config{
		Backend:   backend,
		Dir:       filepath.FromSlash(appconf["aliasDbDir"]),
		MapSize:   lmdbSize,
		MaxDBs:    1,
		WriteMode: write,
		TotalKV:   0, // Not used for alias DB
		AppConf:   appconf,
	}

	return NewEnv(backend, cfg)
}

// GetBackendFromConfig returns the backend type from application configuration
// Defaults to LMDB (proven stability), but can be overridden to MDBX via config
func GetBackendFromConfig(appconf map[string]string) Backend {
	if backend, ok := appconf["dbBackend"]; ok {
		switch backend {
		case "mdbx":
			return BackendMDBX
		case "lmdb":
			return BackendLMDB
		default:
			return BackendLMDB // Default to LMDB (proven stability)
		}
	}
	return BackendLMDB // Default to LMDB
}
