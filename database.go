// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

// Database is a handle to a SQLite database, mirroring SQLite3::Database. It
// wraps a modernc.org/sqlite connection. A path of ":memory:" opens a private
// in-memory database (the gem's behaviour); any other path is a real file the
// engine creates or opens.
type Database struct {
	db     *sql.DB
	path   string
	closed bool

	// resultsAsHash mirrors SQLite3::Database#results_as_hash=. When true,
	// ExecuteHash / the hash helpers key rows by column name.
	resultsAsHash bool
	// typeTranslation mirrors the (long-deprecated) #type_translation= flag. It
	// is retained for API parity; this binding always returns native Go types,
	// so the flag is advisory and does not change results.
	typeTranslation bool
}

// Open opens (creating if necessary) the SQLite database at path and returns a
// *Database, mirroring SQLite3::Database.new / .open. Use ":memory:" for a
// transient in-memory database.
func Open(path string) (*Database, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, wrapError(err)
	}
	// modernc's :memory: is per-connection; cap the pool at one connection so a
	// :memory: database behaves as the single shared database the gem exposes.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, wrapError(err)
	}
	return &Database{db: db, path: path}, nil
}

// New is an alias for Open, matching SQLite3::Database.new.
func New(path string) (*Database, error) { return Open(path) }

// Close closes the database (SQLite3::Database#close). Closing an already-closed
// database is a no-op, matching the gem's idempotent close.
func (d *Database) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	return wrapError(d.db.Close())
}

// Closed reports whether the database has been closed (SQLite3::Database#closed?).
func (d *Database) Closed() bool { return d.closed }

// Path returns the filename the database was opened with (SQLite3::Database#filename).
func (d *Database) Path() string { return d.path }

// SetResultsAsHash sets whether hash-aware helpers key rows by column name
// (SQLite3::Database#results_as_hash=).
func (d *Database) SetResultsAsHash(v bool) { d.resultsAsHash = v }

// ResultsAsHash reports the current results_as_hash setting.
func (d *Database) ResultsAsHash() bool { return d.resultsAsHash }

// SetTypeTranslation sets the advisory type_translation flag
// (SQLite3::Database#type_translation=). Retained for API parity.
func (d *Database) SetTypeTranslation(v bool) { d.typeTranslation = v }

// TypeTranslation reports the type_translation flag.
func (d *Database) TypeTranslation() bool { return d.typeTranslation }

// BusyTimeout sets how long the engine waits on a locked database before
// returning SQLITE_BUSY, in milliseconds (SQLite3::Database#busy_timeout=). It
// issues a `PRAGMA busy_timeout` on the connection.
func (d *Database) BusyTimeout(ms int) error {
	_, err := d.db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", ms))
	return wrapError(err)
}

// LastInsertRowID returns the rowid of the most recent successful INSERT
// (SQLite3::Database#last_insert_row_id). It is evaluated on the connection via
// last_insert_rowid().
func (d *Database) LastInsertRowID() (int64, error) {
	var id int64
	if err := d.db.QueryRow("SELECT last_insert_rowid()").Scan(&id); err != nil {
		return 0, wrapError(err)
	}
	return id, nil
}

// Changes returns the number of rows modified by the most recent statement
// (SQLite3::Database#changes), via changes().
func (d *Database) Changes() (int64, error) {
	var n int64
	if err := d.db.QueryRow("SELECT changes()").Scan(&n); err != nil {
		return 0, wrapError(err)
	}
	return n, nil
}

// TotalChanges returns the total rows modified since the connection was opened
// (SQLite3::Database#total_changes), via total_changes().
func (d *Database) TotalChanges() (int64, error) {
	var n int64
	if err := d.db.QueryRow("SELECT total_changes()").Scan(&n); err != nil {
		return 0, wrapError(err)
	}
	return n, nil
}
