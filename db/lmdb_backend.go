package db

import (
	"github.com/bmatsuo/lmdb-go/lmdb"
)

// LMDBEnv wraps lmdb.Env to implement our Env interface
type LMDBEnv struct {
	env *lmdb.Env
}

// LMDBTxn wraps lmdb.Txn to implement our Txn interface
type LMDBTxn struct {
	txn *lmdb.Txn
}

// LMDBCursor wraps lmdb.Cursor to implement our Cursor interface
type LMDBCursor struct {
	cursor *lmdb.Cursor
}

// LMDBDBI wraps lmdb.DBI
type LMDBDBI lmdb.DBI

// Env interface implementation for LMDB

func (e *LMDBEnv) Close() error {
	e.env.Close()
	return nil
}

func (e *LMDBEnv) Update(fn func(txn Txn) error) error {
	return e.env.Update(func(t *lmdb.Txn) error {
		return fn(&LMDBTxn{txn: t})
	})
}

func (e *LMDBEnv) View(fn func(txn Txn) error) error {
	return e.env.View(func(t *lmdb.Txn) error {
		return fn(&LMDBTxn{txn: t})
	})
}

func (e *LMDBEnv) ReaderCheck() (int, error) {
	return e.env.ReaderCheck()
}

func (e *LMDBEnv) Sync(force bool) error {
	return e.env.Sync(force)
}

// Txn interface implementation for LMDB

func (t *LMDBTxn) Get(dbi DBI, key []byte) ([]byte, error) {
	lmdbDBI := lmdb.DBI(dbi.(LMDBDBI))
	return t.txn.Get(lmdbDBI, key)
}

func (t *LMDBTxn) Put(dbi DBI, key []byte, val []byte, flags uint) error {
	lmdbDBI := lmdb.DBI(dbi.(LMDBDBI))
	return t.txn.Put(lmdbDBI, key, val, flags)
}

func (t *LMDBTxn) CreateDBI(name string) (DBI, error) {
	dbi, err := t.txn.CreateDBI(name)
	if err != nil {
		return nil, err
	}
	return LMDBDBI(dbi), nil
}

func (t *LMDBTxn) OpenCursor(dbi DBI) (Cursor, error) {
	lmdbDBI := lmdb.DBI(dbi.(LMDBDBI))
	cursor, err := t.txn.OpenCursor(lmdbDBI)
	if err != nil {
		return nil, err
	}
	return &LMDBCursor{cursor: cursor}, nil
}

// Cursor interface implementation for LMDB

func (c *LMDBCursor) Close() {
	c.cursor.Close()
}

func (c *LMDBCursor) Get(setkey, setval []byte, op uint) (key, val []byte, err error) {
	return c.cursor.Get(setkey, setval, op)
}

// NewLMDBEnv creates a new LMDB environment with the given configuration
func NewLMDBEnv(cfg *Config) (Env, DBI, error) {
	// Create directory if it doesn't exist
	err := ensureDir(cfg.Dir)
	if err != nil {
		return nil, nil, err
	}

	env, err := lmdb.NewEnv()
	if err != nil {
		return nil, nil, err
	}

	err = env.SetMaxDBs(cfg.MaxDBs)
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	// Calculate map size if not specified
	mapSize := cfg.MapSize
	if mapSize == 0 && cfg.TotalKV > 0 {
		mapSize = calculateLMDBMapSize(cfg.TotalKV)
	}
	if mapSize == 0 {
		mapSize = 1_000_000_000 // 1GB default
	}

	err = env.SetMapSize(mapSize)
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	// Open with appropriate flags
	var flags uint
	if cfg.WriteMode {
		flags = lmdb.WriteMap
	}

	err = env.Open(cfg.Dir, flags, 0700)
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	// Check stale readers
	staleReaders, _ := env.ReaderCheck()
	if staleReaders > 0 {
		// Log if needed
	}

	// Create/open database
	var dbi lmdb.DBI
	err = env.Update(func(txn *lmdb.Txn) (err error) {
		dbi, err = txn.CreateDBI("mydb")
		return err
	})
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	return &LMDBEnv{env: env}, LMDBDBI(dbi), nil
}

// calculateLMDBMapSize calculates appropriate map size based on total KV pairs
func calculateLMDBMapSize(totalKV int64) int64 {
	if totalKV < 1_000_000 {
		return 1_000_000_000 // 1GB
	} else if totalKV < 50_000_000 {
		return 5_000_000_000
	} else if totalKV < 100_000_000 {
		return 10_000_000_000
	} else if totalKV < 150_000_000 {
		return 15_000_000_000
	} else if totalKV < 200_000_000 {
		return 20_000_000_000
	} else if totalKV < 300_000_000 {
		return 30_000_000_000
	} else if totalKV < 500_000_000 {
		return 50_000_000_000
	} else if totalKV < 1_000_000_000 {
		return 100_000_000_000
	} else {
		return int64(1.4 * 1000 * 1000 * 1000 * 1000) // 1.4 TB
	}
}

// IsLMDBNotFound checks if error is a not-found error (LMDB version)
func IsLMDBNotFound(err error) bool {
	return lmdb.IsNotFound(err)
}
