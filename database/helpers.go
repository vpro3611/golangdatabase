package database

import (
	"encoding/json"
)

/*
   ===== Validation =====
*/

func isAllowedValue(value any) bool {
	switch value.(type) {
	case string, int, int64, float64, bool:
		return true
	default:
		return false
	}
}

/*
   ===== Where logic =====
*/

func (w *WhereClause) match(row map[string]any) bool {
	value, ok := row[w.field]
	if !ok {
		return false
	}
	return compare(w.operator, value, w.value)
}

func compare(op string, left, right any) bool {
	l := normalizeNumber(left)
	r := normalizeNumber(right)

	switch lv := l.(type) {
	case string:
		rv, ok := r.(string)
		if !ok {
			return false
		}
		return compareStrings(op, lv, rv)

	case float64:
		rv, ok := r.(float64)
		if !ok {
			return false
		}
		return compareNumbers(op, lv, rv)

	case bool:
		rv, ok := r.(bool)
		if !ok {
			return false
		}
		return compareBools(op, lv, rv)
	}

	return false
}

func normalizeNumber(v any) any {
	switch n := v.(type) {
	case json.Number:
		f, _ := n.Float64()
		return f
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return v
	}
}

func compareNumbers(op string, a, b float64) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	}
	return false
}

func compareStrings(op, a, b string) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	}
	return false
}

func compareBools(op string, a, b bool) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	}
	return false
}

func mustJson(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
