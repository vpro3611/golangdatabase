package database

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	databasePath = "./db/database.db"
	walPath      = "./db/wal.log"

	walSizeLimit = 10 * 1024 * 1024
)

type Database struct {
	dbFile       *os.File
	walFile      *os.File
	mu           sync.RWMutex
	mem          map[string][]byte
	databasePath string
	walPath      string
	walSizeLimit int64
}

type Record struct {
	Op    byte
	Key   []byte
	Value []byte
}

func loadSnapshot(path string, mem map[string][]byte) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	defer f.Close()
	for {

		var keyLen uint32

		if err := binary.Read(f, binary.BigEndian, &keyLen); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		var valLen uint32

		if err := binary.Read(f, binary.BigEndian, &valLen); err != nil {
			return err
		}

		key := make([]byte, keyLen)

		if _, err := io.ReadFull(f, key); err != nil {
			return err
		}

		val := make([]byte, valLen)

		if _, err := io.ReadFull(f, val); err != nil {
			return err
		}

		mem[string(key)] = val
	}

	return nil
}

func replayWal(path string, mem map[string][]byte) error {
	f, err := os.Open(path)

	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	defer f.Close()
	for {
		rec, err := ReadRecord(f)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err // damaged
		}
		applyRecord(mem, rec)
	}

	return nil
}

func applyRecord(mem map[string][]byte, r *Record) {
	switch r.Op {
	case 'S':
		v := make([]byte, len(r.Value))
		copy(v, r.Value)
		mem[string(r.Key)] = v
	case 'D':
		delete(mem, string(r.Key))
	}
}

func writeRecord(w io.Writer, r *Record) error {

	recordLen := 1 + 4 + 4 + len(r.Key) + len(r.Value)

	if err := binary.Write(w, binary.BigEndian, uint32(recordLen)); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, r.Op); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(r.Key))); err != nil {
		return err
	}

	lenVal := uint32(0)
	if r.Value != nil {
		lenVal = uint32(len(r.Value))
	}

	if err := binary.Write(w, binary.BigEndian, lenVal); err != nil {
		return err
	}

	if _, err := w.Write(r.Key); err != nil {
		return err
	}

	if lenVal > 0 {
		if _, err := w.Write(r.Value); err != nil {
			return err
		}
	}

	return nil
}

func ReadRecord(r io.Reader) (*Record, error) {

	var recordLen uint32
	if err := binary.Read(r, binary.BigEndian, &recordLen); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, err
	}

	buf := make([]byte, recordLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	br := bytes.NewReader(buf)

	// reading operation
	var op byte

	if err := binary.Read(br, binary.BigEndian, &op); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, err
	}
	// reading len of Key
	var keyLen uint32

	if err := binary.Read(br, binary.BigEndian, &keyLen); err != nil {
		return nil, err
	}
	// reading len of Value
	var valLen uint32

	if err := binary.Read(br, binary.BigEndian, &valLen); err != nil {
		return nil, err
	}
	// reading exact Key
	key := make([]byte, keyLen)

	if _, err := io.ReadFull(br, key); err != nil {
		return nil, err
	}
	// reading exact Value
	var value []byte

	if valLen > 0 {
		value = make([]byte, valLen)
		if _, err := io.ReadFull(br, value); err != nil {
			return nil, err
		}
	}

	return &Record{
		Op:    op,
		Key:   key,
		Value: value,
	}, nil
}

func OpenDB(dbPath, walPath string, walSizeLimit int64) (*Database, error) {
	filedatabase, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0644)

	if err != nil {
		return nil, errors.New("Failed to create a dbFile for database!")
	}

	fileWal, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)

	if err != nil {
		filedatabase.Close()
		return nil, errors.New("Failed to create a walFile for database")
	}

	db := Database{
		dbFile:       filedatabase,
		walFile:      fileWal,
		mu:           sync.RWMutex{},
		mem:          make(map[string][]byte),
		databasePath: dbPath,
		walPath:      walPath,
		walSizeLimit: walSizeLimit,
	}
	// potential recovery
	err = loadSnapshot(dbPath, db.mem)

	if err != nil {
		fileWal.Close()
		filedatabase.Close()
		return nil, err
	}

	err = replayWal(walPath, db.mem)

	if err != nil {
		fileWal.Close()
		filedatabase.Close()
		return nil, err
	}

	return &db, nil
}

func (db *Database) Get(key string) ([]byte, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	val, ok := db.mem[key]

	if !ok {
		return nil, false
	}
	out := make([]byte, len(val))
	copy(out, val)
	return out, true
}

func (db *Database) Set(key string, val []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	rec := &Record{
		Op:    'S',
		Key:   []byte(key),
		Value: val,
	}

	if err := applyHelper(db, rec); err != nil {
		return err
	}
	return nil
}

func (db *Database) Delete(key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	rec := &Record{
		Op:  'D',
		Key: []byte(key),
	}

	if err := applyHelper(db, rec); err != nil {
		return err
	}
	return nil
}

func (db *Database) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.walFile.Sync(); err != nil {
		return err
	}

	if err := db.dbFile.Sync(); err != nil {
		return err
	}

	if err := db.walFile.Close(); err != nil {
		return err
	}

	if err := db.dbFile.Close(); err != nil {
		return err
	}

	return nil
}

func (db *Database) snapshot() error {
	tmp := db.databasePath + ".tmp"
	tempFile, err := os.OpenFile(tmp, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	defer tempFile.Close()

	for k, v := range db.mem {
		if err := writeSnapshotRecord(tempFile, []byte(k), v); err != nil {
			tempFile.Close()
			return err
		}
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return err
	}

	if err := os.Rename(tmp, db.databasePath); err != nil {
		return err
	}

	if err := db.dbFile.Close(); err != nil {
		return err
	}

	db.dbFile, err = os.OpenFile(db.databasePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	if err := db.walFile.Close(); err != nil {
		return err
	}

	db.walFile, err = os.OpenFile(db.walPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC|os.O_APPEND, 0644)

	if err != nil {
		return err
	}

	return nil
}

func writeSnapshotRecord(w io.Writer, key, value []byte) error {
	if err := binary.Write(w, binary.BigEndian, uint32(len(key))); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(value))); err != nil {
		return err
	}

	if _, err := w.Write(key); err != nil {
		return err
	}

	if _, err := w.Write(value); err != nil {
		return err
	}

	return nil
}

func (db *Database) ScanPrefix(prefix string) map[string][]byte {
	db.mu.RLock()
	defer db.mu.RUnlock()

	res := make(map[string][]byte)
	for key, val := range db.mem {
		if strings.HasPrefix(key, prefix) {
			v := make([]byte, len(val))
			copy(v, val)
			res[key] = v
		}
	}
	return res
}

func applyHelper(db *Database, rec *Record) error {
	if err := writeRecord(db.walFile, rec); err != nil {
		return err
	}

	if err := db.walFile.Sync(); err != nil {
		return err
	}

	applyRecord(db.mem, rec)

	fstat, err := db.walFile.Stat()

	if err != nil {
		return err
	}

	if fstat.Size() > db.walSizeLimit {
		if err := db.snapshot(); err != nil {
			return err
		}
	}

	return nil
}
