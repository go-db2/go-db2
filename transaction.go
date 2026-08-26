package db2

import (
	"context"
	"database/sql/driver"

	"github.com/go-db2/go-db2/network"
)

// Tx represents an in-progress transaction on an IBM Db2 database.
type Tx struct {
	session *network.Session
	ctx     context.Context
}

// Commit commits the transaction.
func (tx *Tx) Commit() error {
	if tx.session == nil {
		return ErrConnectionClosed
	}
	return tx.session.Commit(tx.ctx)
}

// Rollback aborts the transaction.
func (tx *Tx) Rollback() error {
	if tx.session == nil {
		return ErrConnectionClosed
	}
	return tx.session.Rollback(tx.ctx)
}

var _ driver.Tx = (*Tx)(nil)
