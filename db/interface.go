package db

// Database abstraction interface to support both LMDB and MDBX
// This allows easy switching between backends or rollback if needed

// Env represents a database environment (LMDB Env or MDBX Env)
type Env interface {
	// Close closes the environment and releases resources
	Close() error

	// Update executes a read-write transaction
	Update(fn func(txn Txn) error) error

	// View executes a read-only transaction
	View(fn func(txn Txn) error) error

	// ReaderCheck clears stale reader locks
	ReaderCheck() (int, error)

	// Sync flushes system buffers to disk
	Sync(force bool) error
}

// Txn represents a database transaction (LMDB Txn or MDBX Txn)
type Txn interface {
	// Get retrieves a value by key
	Get(dbi DBI, key []byte) ([]byte, error)

	// Put stores a key-value pair
	Put(dbi DBI, key []byte, val []byte, flags uint) error

	// CreateDBI creates or opens a named database
	CreateDBI(name string) (DBI, error)

	// OpenCursor opens a cursor for iteration
	OpenCursor(dbi DBI) (Cursor, error)
}

// Cursor represents a database cursor for iteration
type Cursor interface {
	// Close closes the cursor
	Close()

	// Get retrieves the current or next key-value pair
	Get(setkey, setval []byte, op uint) (key, val []byte, err error)
}

// DBI represents a database handle (opaque identifier)
type DBI interface{}

// Backend type for database selection
type Backend string

const (
	BackendLMDB Backend = "lmdb"
	BackendMDBX Backend = "mdbx"
)

// Config holds database configuration
type Config struct {
	Backend     Backend // "lmdb" or "mdbx"
	Dir         string  // Database directory
	MapSize     int64   // Map size (for LMDB) or initial size (for MDBX)
	MaxDBs      int     // Maximum number of named databases
	WriteMode   bool    // Open for writing
	TotalKV     int64   // Total key-value pairs (for size estimation)
	AppConf     map[string]string // Application configuration
}

// IsNotFound checks if an error is a "not found" error for any backend
func IsNotFound(err error) bool {
	return IsLMDBNotFound(err) || IsMDBXNotFound(err)
}
