package database

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golangdb/errors_consts"
)

/*
   Public DB wrapper
*/

type DB struct {
	Database *Database
}

func NewDB(storage *Database) *DB {
	return &DB{Database: storage}
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

	raw, ok := q.db.Database.Get(metaKey)
	if !ok {
		if err := q.db.Database.Set(metaKey, mustJson(int64(2))); err != nil {
			return 0, nil
		}
		return 1, nil
	}

	var next int64

	if err := json.Unmarshal(raw, &next); err != nil {
		return 0, err
	}

	if err := q.db.Database.Set(metaKey, mustJson(next+1)); err != nil {
		return 0, err
	}
	return next, nil
}

func (q *InsertQuery) Exec() error {
	_, err := q.ExecAndReturnID()
	if err != nil {
		return err
	}
	return nil
	//if q.table == "" {
	//	return errors_consts.ErrEmptyName
	//}
	//if len(q.values) == 0 {
	//	return errors_consts.ErrEmptyValues
	//}
	//
	//idRaw, ok := q.values["id"]
	//if !ok {
	//	id, err := q.nextID()
	//	if err != nil {
	//		return err
	//	}
	//	idRaw = id
	//	q.values["id"] = idRaw
	//}
	//
	//// validate values
	//for k, v := range q.values {
	//	if !isAllowedValue(v) {
	//		return fmt.Errorf("unsupported value type for field %s", k)
	//	}
	//}
	//
	//data, err := json.Marshal(q.values)
	//if err != nil {
	//	return err
	//}
	//
	//key := q.table + ":" + fmt.Sprint(idRaw)
	//return q.db.database.Set(key, data)
}

func (q *InsertQuery) ExecAndReturnID() (int64, error) {
	if q.table == "" {
		return 0, errors_consts.ErrEmptyName
	}
	if len(q.values) == 0 {
		return 0, errors_consts.ErrEmptyValues
	}

	var id int64

	if raw, ok := q.values["id"]; ok {
		switch v := raw.(type) {
		case int:
			id = int64(v)
		case int64:
			id = v
		case json.Number:
			n, err := v.Int64()
			if err != nil {
				return 0, err
			}
			id = n
		case float64:
			id = int64(v)
		default:
			return 0, fmt.Errorf("unsupported id type %T", raw)
		}
	} else {
		var err error
		id, err := q.nextID()
		if err != nil {
			return 0, err
		}
		q.values["id"] = id
	}

	for k, v := range q.values {
		if !isAllowedValue(v) {
			return 0, fmt.Errorf("unsupported value type for field %s", k)
		}
	}

	data, err := json.Marshal(q.values)

	if err != nil {
		return 0, err
	}

	key := q.table + ":" + fmt.Sprint(id)
	if err := q.db.Database.Set(key, data); err != nil {
		return 0, err
	}

	return id, nil
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
		return nil, errors_consts.ErrEmptyName
	}

	raw := s.db.Database.ScanPrefix(s.table + ":")
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
		return errors_consts.ErrEmptyName
	}

	prefix := d.table + ":"
	raw := d.db.Database.ScanPrefix(prefix)

	for key, data := range raw {
		if d.where == nil {
			// this DELETES all the table!
			if err := d.db.Database.Delete(key); err != nil {
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
			if err := d.db.Database.Delete(key); err != nil {
				return err
			}
		}
	}
	return nil
}

//func m() {
//
//	db, err := OpenDB(databasePath, WalPath, walSizeLimit)
//	if err != nil {
//		log.Fatal(err)
//	}
//	_ = NewDB(db)
//}
