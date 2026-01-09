package db_test

import (
	"golangdb/database"
	"path/filepath"
	"testing"
)

func TestSetGet(t *testing.T) {
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	db, err := database.OpenDB(dbPath, walPath, database.WalSizeLimit)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	err = db.Set("key1", []byte("value1"))
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	val, ok := db.Get("key1")
	if !ok {
		t.Fatalf("key not found")
	}

	if string(val) != "value1" {
		t.Fatalf("expected value1, got %s", val)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	db, err := database.OpenDB(dbPath, walPath, database.WalSizeLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Set("key", []byte("value"))
	db.Delete("key")

	_, ok := db.Get("key")
	if ok {
		t.Fatalf("expected key to be deleted")
	}
}

func TestRecovery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	{
		db, err := database.OpenDB(dbPath, walPath, database.WalSizeLimit)
		if err != nil {
			t.Fatal(err)
		}
		db.Set("a", []byte("1"))
		db.Set("b", []byte("2"))
		db.Close()
	}

	{
		db, err := database.OpenDB(dbPath, walPath, database.WalSizeLimit)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		val, ok := db.Get("a")
		if !ok || string(val) != "1" {
			t.Fatalf("recovery failed for key a")
		}

		val, ok = db.Get("b")
		if !ok || string(val) != "2" {
			t.Fatalf("recovery failed for key b")
		}
	}
}

func TestOverwrite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	db, _ := database.OpenDB(dbPath, walPath, database.WalSizeLimit)
	defer db.Close()

	db.Set("key", []byte("v1"))
	db.Set("key", []byte("v2"))

	val, _ := db.Get("key")
	if string(val) != "v2" {
		t.Fatalf("expected v2, got %s", val)
	}
}

func TestScanPrefix(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	db, _ := database.OpenDB(dbPath, walPath, database.WalSizeLimit)
	defer db.Close()

	db.Set("user:1", []byte("a"))
	db.Set("user:2", []byte("b"))
	db.Set("order:1", []byte("c"))

	res := db.ScanPrefix("user:")

	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
}
