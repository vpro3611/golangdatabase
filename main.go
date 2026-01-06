// TODO :
// 1) database struct
// 2) set, delete, get methods
// 3) make a struct of what is being inserted
// 4) give it an id and increment it by one
// 5) insert into the db struct actual pointer to .db file, to .log wal file, and the map[string]RecordType{}
// 6) Tmrw - finish the basic db, add more complicated stuff.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const (
	databasePath = "./db/database.db"
	walPath      = "./db/wal.log"

	walSizeLimit = 10 * 1024 * 1024
)

type Database struct {
	dbFile  *os.File
	walFile *os.File
	mu      sync.RWMutex
	mem     map[string][]byte
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
			break // damaged
		}
		applyRecord(mem, rec)
	}

	return nil
}

func applyRecord(mem map[string][]byte, r *Record) {
	switch r.Op {
	case 'S':
		mem[string(r.Key)] = r.Value
	case 'D':
		delete(mem, string(r.Key))
	}
}

func writeRecord(w io.Writer, r *Record) error {
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
	// reading operation
	var op byte

	if err := binary.Read(r, binary.BigEndian, &op); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, err
	}
	// reading len of Key
	var keyLen uint32

	if err := binary.Read(r, binary.BigEndian, &keyLen); err != nil {
		return nil, err
	}
	// reading len of Value
	var valLen uint32

	if err := binary.Read(r, binary.BigEndian, &valLen); err != nil {
		return nil, err
	}
	// reading exact Key
	key := make([]byte, keyLen)

	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	// reading exact Value
	var value []byte

	if valLen > 0 {
		value = make([]byte, valLen)
		if _, err := io.ReadFull(r, value); err != nil {
			return nil, err
		}
	}

	return &Record{
		Op:    op,
		Key:   key,
		Value: value,
	}, nil
}

func OpenDB(dbPath, walPath string) (*Database, error) {
	filedatabase, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0644)

	if err != nil {
		return nil, errors.New("Failed to create a dbFile for database!")
	}

	fileWal, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)

	if err != nil {
		return nil, errors.New("Failed to create a walFile for database")
	}

	db := Database{
		dbFile:  filedatabase,
		walFile: fileWal,
		mu:      sync.RWMutex{},
		mem:     make(map[string][]byte),
	}
	// potential recovery
	err = loadSnapshot(dbPath, db.mem)

	if err != nil {
		return nil, err
	}

	err = replayWal(walPath, db.mem)

	if err != nil {
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

	return val, true
}

func (db *Database) Set(key string, val []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	rec := &Record{
		Op:    'S',
		Key:   []byte(key),
		Value: val,
	}

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

	if fstat.Size() > walSizeLimit {
		if err := db.snapshot(); err != nil {
			return err
		}
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

	if fstat.Size() > walSizeLimit {
		if err := db.snapshot(); err != nil {
			return err
		}
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
	tmp := databasePath + ".tmp"
	tempFile, err := os.OpenFile(tmp, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

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

	tempFile.Close()

	if err := os.Rename(tmp, databasePath); err != nil {
		return err
	}

	if err := db.dbFile.Close(); err != nil {
		return err
	}

	db.dbFile, err = os.OpenFile(databasePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	if err := db.walFile.Close(); err != nil {
		return err
	}

	db.walFile, err = os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC|os.O_APPEND, 0644)

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

func main() {
	fmt.Println("Hello Data")
}
