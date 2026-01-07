package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertExec(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	storage, err := OpenDB(dbPath, walPath, walSizeLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	db := NewDB(storage)

	err = db.Insert().
		Table("users").
		Values(map[string]any{
			"id":   "1",
			"name": "Alex",
		}).
		Exec()

	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	val, ok := storage.Get("users:1")
	if !ok {
		t.Fatalf("record not found")
	}

	if !strings.Contains(string(val), "Alex") {
		t.Fatalf("unexpected value: %s", string(val))
	}
}

func TestSelectAll(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	storage, err := OpenDB(dbPath, walPath, walSizeLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	db := NewDB(storage)

	db.Insert().
		Table("users").
		Values(map[string]any{
			"id":   "1",
			"name": "Alex",
		}).
		Exec()

	db.Insert().
		Table("users").
		Values(map[string]any{
			"id":   "2",
			"name": "Bob",
		}).
		Exec()

	rows, err := db.Select().
		Table("users").
		All()

	if err != nil {
		t.Fatalf("select failed: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestSelectWhere(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	storage, _ := OpenDB(dbPath, walPath, walSizeLimit)
	defer storage.Close()

	db := NewDB(storage)

	db.Insert().Table("users").Values(map[string]any{
		"id": "1", "age": 20,
	}).Exec()

	db.Insert().Table("users").Values(map[string]any{
		"id": "2", "age": 15,
	}).Exec()

	rows, err := db.Select().
		Table("users").
		Where("age", ">", 18).
		All()

	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestDeleteWhere(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	storage, err := OpenDB(dbPath, walPath, walSizeLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	db := NewDB(storage)

	db.Insert().Table("users").Values(map[string]any{
		"id": "1", "age": 20,
	}).Exec()

	db.Insert().Table("users").Values(map[string]any{
		"id": "2", "age": 15,
	}).Exec()

	err = db.Delete().
		Table("users").
		Where("age", "<", 18).
		Exec()

	if err != nil {
		t.Fatal(err)
	}

	rows, _ := db.Select().Table("users").All()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestInsertAutoIncrement(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db.data")
	walPath := filepath.Join(dir, "db.wal")

	storage, err := OpenDB(dbPath, walPath, walSizeLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()

	db := NewDB(storage)

	// insert без id
	err = db.Insert().
		Table("users").
		Values(map[string]any{
			"name": "Alice",
			"age":  20,
		}).
		Exec()
	if err != nil {
		t.Fatal(err)
	}

	err = db.Insert().
		Table("users").
		Values(map[string]any{
			"name": "Bob",
			"age":  25,
		}).
		Exec()
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.Select().
		Table("users").
		All()
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	ids := make(map[int64]bool)

	for _, row := range rows {
		rawID, ok := row["id"]
		if !ok {
			t.Fatalf("row has no id field")
		}

		var id int64

		switch v := rawID.(type) {
		case json.Number:
			n, err := v.Int64()
			if err != nil {
				t.Fatalf("invalid json.Number id: %v", err)
			}
			id = n

		case float64:
			id = int64(v)

		default:
			t.Fatalf("unexpected id type %T", rawID)
		}

		ids[id] = true
	}

	if !ids[1] || !ids[2] {
		t.Fatalf("expected ids {1,2}, got %v", ids)
	}

}
