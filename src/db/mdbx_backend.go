package db

import (
	"github.com/erigontech/mdbx-go/mdbx"
)

// MDBXEnv wraps mdbx.Env to implement our Env interface
type MDBXEnv struct {
	env *mdbx.Env
}

// MDBXTxn wraps mdbx.Txn to implement our Txn interface
type MDBXTxn struct {
	txn *mdbx.Txn
}

// MDBXCursor wraps mdbx.Cursor to implement our Cursor interface
type MDBXCursor struct {
	cursor *mdbx.Cursor
}

// MDBXDBI wraps mdbx.DBI
type MDBXDBI mdbx.DBI

// Env interface implementation for MDBX

func (e *MDBXEnv) Close() error {
	e.env.Close()
	return nil
}

func (e *MDBXEnv) Update(fn func(txn Txn) error) error {
	return e.env.Update(func(t *mdbx.Txn) error {
		return fn(&MDBXTxn{txn: t})
	})
}

func (e *MDBXEnv) View(fn func(txn Txn) error) error {
	return e.env.View(func(t *mdbx.Txn) error {
		return fn(&MDBXTxn{txn: t})
	})
}

func (e *MDBXEnv) ReaderCheck() (int, error) {
	// MDBX doesn't have ReaderCheck, return 0
	return 0, nil
}

func (e *MDBXEnv) Sync(force bool) error {
	return e.env.Sync(force, false)
}

// Txn interface implementation for MDBX

func (t *MDBXTxn) Get(dbi DBI, key []byte) ([]byte, error) {
	mdbxDBI := mdbx.DBI(dbi.(MDBXDBI))
	return t.txn.Get(mdbxDBI, key)
}

func (t *MDBXTxn) Put(dbi DBI, key []byte, val []byte, flags uint) error {
	mdbxDBI := mdbx.DBI(dbi.(MDBXDBI))
	return t.txn.Put(mdbxDBI, key, val, flags)
}

func (t *MDBXTxn) CreateDBI(name string) (DBI, error) {
	dbi, err := t.txn.OpenDBI(name, mdbx.Create, nil, nil)
	if err != nil {
		return nil, err
	}
	return MDBXDBI(dbi), nil
}

func (t *MDBXTxn) OpenCursor(dbi DBI) (Cursor, error) {
	mdbxDBI := mdbx.DBI(dbi.(MDBXDBI))
	cursor, err := t.txn.OpenCursor(mdbxDBI)
	if err != nil {
		return nil, err
	}
	return &MDBXCursor{cursor: cursor}, nil
}

// Cursor interface implementation for MDBX

func (c *MDBXCursor) Close() {
	c.cursor.Close()
}

func (c *MDBXCursor) Get(setkey, setval []byte, op uint) (key, val []byte, err error) {
	return c.cursor.Get(setkey, setval, op)
}

// NewMDBXEnv creates a new MDBX environment with the given configuration
func NewMDBXEnv(cfg *Config) (Env, DBI, error) {
	// Create directory if it doesn't exist
	err := ensureDir(cfg.Dir)
	if err != nil {
		return nil, nil, err
	}

	env, err := mdbx.NewEnv(mdbx.Label("biobtree"))
	if err != nil {
		return nil, nil, err
	}

	err = env.SetOption(mdbx.OptMaxDB, uint64(cfg.MaxDBs))
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	// Set geometry (MDBX auto-grows, but we set initial size)
	initialSize := cfg.MapSize
	if initialSize == 0 && cfg.TotalKV > 0 {
		initialSize = calculateLMDBMapSize(cfg.TotalKV) // Reuse same size logic
	}
	if initialSize == 0 {
		initialSize = 1_000_000_000 // 1GB default
	}

	// SetGeometry(sizeMin, sizeNow, sizeMax, growthStep, shrinkThreshold, pageSize)
	// -1 means use default
	err = env.SetGeometry(-1, -1, int(initialSize), -1, -1, -1)
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	// Open with Exclusive flag for NFS compatibility
	// Exclusive ensures single-process access and avoids file locking issues on NFS
	var flags uint = mdbx.Exclusive
	if cfg.WriteMode {
		// WriteMap can be added if needed, but Exclusive is key for NFS
		// flags |= mdbx.WriteMap
	}

	err = env.Open(cfg.Dir, flags, 0700)
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	// Create/open database
	var dbi mdbx.DBI
	err = env.Update(func(txn *mdbx.Txn) (err error) {
		dbi, err = txn.OpenDBI("mydb", mdbx.Create, nil, nil)
		return err
	})
	if err != nil {
		env.Close()
		return nil, nil, err
	}

	return &MDBXEnv{env: env}, MDBXDBI(dbi), nil
}

// IsMDBXNotFound checks if error is a not-found error (MDBX version)
func IsMDBXNotFound(err error) bool {
	return mdbx.IsNotFound(err)
}
