package db2

import (
	"database/sql/driver"
	"errors"
)

// Result implements the database/sql/driver.Result interface.
type Result struct {
	affectedRows int64
	lastInsertId int64
}

// NewResult creates a new driver.Result with affected rows count.
func NewResult(affectedRows, lastInsertId int64) *Result {
	return &Result{
		affectedRows: affectedRows,
		lastInsertId: lastInsertId,
	}
}

// LastInsertId returns the database's auto-generated ID for the inserted row.
func (r *Result) LastInsertId() (int64, error) {
	if r.lastInsertId == 0 {
		return 0, errors.New("db2: LastInsertId is not supported without identity column retrieval")
	}
	return r.lastInsertId, nil
}

// RowsAffected returns the number of rows affected by the query.
func (r *Result) RowsAffected() (int64, error) {
	return r.affectedRows, nil
}

var _ driver.Result = (*Result)(nil)
