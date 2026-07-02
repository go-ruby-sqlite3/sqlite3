// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import "strings"

// TransactionMode selects the BEGIN mode, mirroring SQLite3::Database#transaction's
// :deferred / :immediate / :exclusive keyword.
type TransactionMode string

// The three SQLite transaction modes. Deferred is SQLite's default.
const (
	Deferred  TransactionMode = "DEFERRED"
	Immediate TransactionMode = "IMMEDIATE"
	Exclusive TransactionMode = "EXCLUSIVE"
)

// Begin starts a transaction with the given mode (SQLite3::Database#transaction
// without a block). Pass an empty mode for SQLite's default (deferred). Commit
// or Rollback ends it.
func (d *Database) Begin(mode TransactionMode) error {
	stmt := "BEGIN"
	if m := strings.TrimSpace(string(mode)); m != "" {
		stmt = "BEGIN " + strings.ToUpper(m)
	}
	_, err := d.db.Exec(stmt)
	return wrapError(err)
}

// Commit commits the current transaction (SQLite3::Database#commit).
func (d *Database) Commit() error {
	_, err := d.db.Exec("COMMIT")
	return wrapError(err)
}

// Rollback rolls back the current transaction (SQLite3::Database#rollback).
func (d *Database) Rollback() error {
	_, err := d.db.Exec("ROLLBACK")
	return wrapError(err)
}

// InTransaction reports whether a transaction is currently open
// (SQLite3::Database#transaction_active?).
func (d *Database) InTransaction() (bool, error) {
	// A no-op COMMIT-less probe: SQLite exposes autocommit state; when a
	// transaction is active, a nested BEGIN errors. We instead query the
	// connection's autocommit flag via a harmless PRAGMA-free check: attempting
	// to read `PRAGMA` is not it, so use the documented behaviour that a second
	// BEGIN inside a transaction fails.
	err := d.Begin("")
	if err != nil {
		// Already in a transaction: SQLite refuses the nested BEGIN.
		return true, nil
	}
	// We were not in a transaction; undo the probe BEGIN.
	return false, d.Rollback()
}

// Transaction runs fn inside a transaction, committing on success and rolling
// back if fn returns an error or panics (the block form of
// SQLite3::Database#transaction). mode selects the BEGIN mode.
func (d *Database) Transaction(mode TransactionMode, fn func() error) (err error) {
	if berr := d.Begin(mode); berr != nil {
		return berr
	}
	committed := false
	defer func() {
		if !committed {
			// Roll back on error or panic. A rollback error does not mask the
			// original failure.
			_ = d.Rollback()
		}
	}()
	if ferr := fn(); ferr != nil {
		return ferr
	}
	if cerr := d.Commit(); cerr != nil {
		return cerr
	}
	committed = true
	return nil
}
