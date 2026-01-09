package errors_consts

import "errors"

var (
	ErrEmptyName   = errors.New("table name is not set")
	ErrEmptyValues = errors.New("values are empty, they cannot be empty")
)
