package database

import (
	"bytes"
	"encoding/json"
	"fmt"
	database2 "golangdb/database"
)

/*
   Public DB wrapper
*/

type DB struct {
	database *database2.Database
}

func NewDB(storage *database2.Database) *DB {
	return &DB{database: storage}
}

/*
   Query types
*/

type InsertQuery struct {
	db     *DB
	table  string
	values map[string]any
}

type SelectQuery struct {
	db    *DB
	table string
	where *WhereClause
}

type DeleteQuery struct {
	db    *DB
	table string
	where *WhereClause
}

type WhereClause struct {
	field    string
	operator string
	value    any
}

/*
   Constructors
*/

func (db *DB) Insert() *InsertQuery {
	return &InsertQuery{db: db}
}

func (db *DB) Select() *SelectQuery {
	return &SelectQuery{db: db}
}

func (db *DB) Delete() *DeleteQuery {
	return &DeleteQuery{db: db}
}

/*
   Insert
*/

func (q *InsertQuery) Table(name string) *InsertQuery {
	q.table = name
	return q
}

func (q *InsertQuery) Values(values map[string]any) *InsertQuery {
	q.values = values
	return q
}

// auto ID generation

func (q *InsertQuery) nextID() (int64, error) {
	metaKey := "__Meta__:" + q.table + ":next_id"

	raw, ok := q.db.database.Get(metaKey)
	if !ok {
		if err := q.db.database.Set(metaKey, mustJson(int64(2))); err != nil {
			return 0, nil
		}
		return 1, nil
	}

	var next int64

	if err := json.Unmarshal(raw, &next); err != nil {
		return 0, err
	}

	if err := q.db.database.Set(metaKey, mustJson(next+1)); err != nil {
		return 0, err
	}
	return next, nil
}

func (q *InsertQuery) Exec() error {
	if q.table == "" {
		return fmt.Errorf("table name is not set")
	}
	if len(q.values) == 0 {
		return fmt.Errorf("values are empty")
	}

	idRaw, ok := q.values["id"]
	if !ok {
		id, err := q.nextID()
		if err != nil {
			return err
		}
		idRaw = id
		q.values["id"] = idRaw
	}

	// validate values
	for k, v := range q.values {
		if !isAllowedValue(v) {
			return fmt.Errorf("unsupported value type for field %s", k)
		}
	}

	data, err := json.Marshal(q.values)
	if err != nil {
		return err
	}

	key := q.table + ":" + fmt.Sprint(idRaw)
	return q.db.database.Set(key, data)
}

/*
   Select
*/

func (s *SelectQuery) Table(name string) *SelectQuery {
	s.table = name
	return s
}

func (s *SelectQuery) Where(field, op string, value any) *SelectQuery {
	switch op {
	case "=", "!=", "<", ">":
	default:
		panic("unsupported operator")
	}
	if !isAllowedValue(value) {
		panic("unsupported where value type")
	}

	s.where = &WhereClause{
		field:    field,
		operator: op,
		value:    value,
	}
	return s
}

func (s *SelectQuery) All() ([]map[string]any, error) {
	if s.table == "" {
		return nil, fmt.Errorf("table name is not set")
	}

	raw := s.db.database.ScanPrefix(s.table + ":")
	out := make([]map[string]any, 0, len(raw))

	for _, data := range raw {
		var row map[string]any

		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()

		if err := dec.Decode(&row); err != nil {
			return nil, err
		}

		if s.where == nil || s.where.match(row) {
			out = append(out, row)
		}
	}

	return out, nil
}

/*
   Delete
*/

func (d *DeleteQuery) Table(name string) *DeleteQuery {
	d.table = name
	return d
}

func (d *DeleteQuery) Where(field, op string, value any) *DeleteQuery {
	switch op {
	case "=", "!=", "<", ">":
	default:
		panic("unsupported operator")
	}
	if !isAllowedValue(value) {
		panic("unsupported where value type")
	}

	d.where = &WhereClause{
		field:    field,
		operator: op,
		value:    value,
	}
	return d
}

func (d *DeleteQuery) Exec() error {
	if d.table == "" {
		return fmt.Errorf("table name is not set")
	}

	prefix := d.table + ":"
	raw := d.db.database.ScanPrefix(prefix)

	for key, data := range raw {
		if d.where == nil {
			if err := d.db.database.Delete(key); err != nil {
				return err
			}
			continue
		}

		var row map[string]any

		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()

		if err := dec.Decode(&row); err != nil {
			return err
		}

		if d.where.match(row) {
			if err := d.db.database.Delete(key); err != nil {
				return err
			}
		}
	}
	return nil
}

//func m() {
//
//	db, err := OpenDB(databasePath, walPath, walSizeLimit)
//	if err != nil {
//		log.Fatal(err)
//	}
//	_ = NewDB(db)
//}
