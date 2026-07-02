// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"database/sql"
	"strconv"
	"strings"
)

// Statement is a prepared SQL statement, mirroring SQLite3::Statement. It holds
// a *sql.Stmt from the modernc backend plus the pending bindings; Execute (or a
// Query on the owning Database) materialises a result cursor the caller advances
// with Next / Step and reads with Row. Reset clears the cursor so the statement
// can run again with new binds; Close releases it.
type Statement struct {
	db     *Database
	stmt   *sql.Stmt
	sql    string
	closed bool

	// binds accumulates positional and named parameters set since the last
	// Reset. Positional binds land in posBinds keyed by 1-based index; named
	// binds land in namedBinds keyed by bare name (without the : $ @ sigil).
	posBinds   map[int]Value
	namedBinds map[string]Value

	// Result cursor, populated by exec(). rows is the full materialised result
	// (SQLite statements are forward-only; buffering lets Reset re-run cleanly);
	// pos is the index of the next row Next will surface, -1 before the first.
	cols []string
	rows []Row
	pos  int
	done bool // exec() has run since the last Reset
}

// Prepare compiles sql into a *Statement without running it
// (SQLite3::Database#prepare). Bind parameters with BindParam / BindParams, then
// call Execute (or step with Next).
func (d *Database) Prepare(query string) (*Statement, error) {
	stmt, err := d.db.Prepare(query)
	if err != nil {
		return nil, wrapError(err)
	}
	return &Statement{
		db:         d,
		stmt:       stmt,
		sql:        query,
		posBinds:   map[int]Value{},
		namedBinds: map[string]Value{},
		pos:        -1,
	}, nil
}

// SQL returns the statement's source text (SQLite3::Statement#sql).
func (s *Statement) SQL() string { return s.sql }

// Closed reports whether the statement has been closed (SQLite3::Statement#closed?).
func (s *Statement) Closed() bool { return s.closed }

// BindParam binds one parameter (SQLite3::Statement#bind_param). key selects the
// slot: an int is a 1-based positional index; a string is a named parameter,
// with or without a leading ':' , '$', or '@' sigil (all three SQLite name
// sigils are accepted, matching the gem). A purely numeric string binds the
// corresponding ?NNN slot.
func (s *Statement) BindParam(key any, val Value) {
	switch k := key.(type) {
	case int:
		s.posBinds[k] = val
	case int64:
		s.posBinds[int(k)] = val
	case string:
		name := strings.TrimLeft(k, ":$@")
		if n, err := strconv.Atoi(name); err == nil {
			s.posBinds[n] = val
			return
		}
		s.namedBinds[name] = val
	}
}

// BindParams binds a batch of parameters (SQLite3::Statement#bind_params). A
// positional slice binds 1..N in order; pass named parameters individually with
// BindParam. Passing nil binds nothing.
func (s *Statement) BindParams(binds []Value) {
	for i, b := range binds {
		s.posBinds[i+1] = b
	}
}

// buildArgs assembles the driver argument list from the accumulated binds. It
// merges positional and named binds: positional slots become ordered values (a
// gap defaults to NULL), named slots become sql.Named. When both are present the
// positional values precede the named ones, which SQLite resolves by index/name
// independently.
func (s *Statement) buildArgs() []any {
	var args []any
	if len(s.posBinds) > 0 {
		max := 0
		for i := range s.posBinds {
			if i > max {
				max = i
			}
		}
		for i := 1; i <= max; i++ {
			args = append(args, normalizeBind(s.posBinds[i]))
		}
	}
	for name, v := range s.namedBinds {
		args = append(args, sql.Named(name, normalizeBind(v)))
	}
	return args
}

// exec runs the statement with the current binds and buffers the result cursor.
// It is safe to call once per Reset; a second call without Reset is a no-op so
// Next after Execute keeps stepping the same cursor.
func (s *Statement) exec() error {
	if s.done {
		return nil
	}
	args := s.buildArgs()
	if !isQuery(s.sql) {
		if _, err := s.stmt.Exec(args...); err != nil {
			return wrapError(err)
		}
		s.cols = nil
		s.rows = nil
		s.done = true
		s.pos = -1
		return nil
	}
	rows, err := s.stmt.Query(args...)
	if err != nil {
		return wrapError(err)
	}
	defer rows.Close()
	data, cols, err := scanRows(rows)
	if err != nil {
		return wrapError(err)
	}
	s.cols = cols
	s.rows = data
	s.done = true
	s.pos = -1
	return nil
}

// Execute runs the statement and returns all result rows (SQLite3::Statement#execute).
// For a non-query statement it returns an empty slice. Bindings set beforehand
// are applied; call Reset before executing again with new binds.
func (s *Statement) Execute() ([]Row, error) {
	if err := s.exec(); err != nil {
		return nil, err
	}
	return s.rows, nil
}

// ExecuteHash runs the statement and returns its rows keyed by column name,
// the results_as_hash shape.
func (s *Statement) ExecuteHash() ([]HashRow, error) {
	if err := s.exec(); err != nil {
		return nil, err
	}
	out := make([]HashRow, len(s.rows))
	for i, r := range s.rows {
		h := make(HashRow, len(s.cols))
		for j, c := range s.cols {
			h[c] = r[j]
		}
		out[i] = h
	}
	return out, nil
}

// Step advances to the next result row and returns it, or nil when the rows are
// exhausted (SQLite3::Statement#step). It executes the statement lazily on the
// first call. The returned bool reports whether a row was produced.
func (s *Statement) Step() (Row, bool, error) {
	if err := s.exec(); err != nil {
		return nil, false, err
	}
	if s.pos+1 >= len(s.rows) {
		s.pos = len(s.rows)
		return nil, false, nil
	}
	s.pos++
	return s.rows[s.pos], true, nil
}

// Next is an alias for Step with the gem-idiomatic (row, ok) shape callers use
// to drive a step loop.
func (s *Statement) Next() (Row, bool, error) { return s.Step() }

// Columns returns the result column names (SQLite3::Statement#columns). It
// executes the statement if it has not run yet so the column list is available.
func (s *Statement) Columns() ([]string, error) {
	if err := s.exec(); err != nil {
		return nil, err
	}
	return s.cols, nil
}

// Types returns the declared column type names, one per column
// (SQLite3::Statement#types). SQLite reports the declared type of each result
// column; an expression column with no declared type yields an empty string
// (SQLite's dynamic typing), matching the gem which returns nil there.
func (s *Statement) Types() ([]string, error) {
	if err := s.exec(); err != nil {
		return nil, err
	}
	// The buffered cursor is closed, so re-derive declared types from a fresh
	// query on the same statement.
	if !isQuery(s.sql) {
		return nil, nil
	}
	rows, err := s.stmt.Query(s.buildArgs()...)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()
	names, err := columnTypeNames(rows)
	if err != nil {
		return nil, wrapError(err)
	}
	return names, nil
}

// columnTypeNames reads the declared SQL type name of each result column. It is
// a var so tests can substitute a version that surfaces the (with modernc,
// unreachable) ColumnTypes failure, covering Types' error branch.
var columnTypeNames = func(rows *sql.Rows) ([]string, error) {
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cts))
	for i, ct := range cts {
		out[i] = ct.DatabaseTypeName()
	}
	return out, nil
}

// Reset clears the result cursor so the statement can execute again
// (SQLite3::Statement#reset). Bindings are retained; call ClearBindings to drop
// them too.
func (s *Statement) Reset() error {
	s.done = false
	s.pos = -1
	s.rows = nil
	s.cols = nil
	return nil
}

// ClearBindings drops all accumulated positional and named bindings
// (SQLite3::Statement#clear_bindings!).
func (s *Statement) ClearBindings() {
	s.posBinds = map[int]Value{}
	s.namedBinds = map[string]Value{}
}

// Close releases the prepared statement (SQLite3::Statement#close). Closing an
// already-closed statement is a no-op.
func (s *Statement) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return wrapError(s.stmt.Close())
}
