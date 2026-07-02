// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"database/sql"
	"strings"
)

// isQuery reports whether sql returns rows (SELECT / PRAGMA read / VALUES /
// WITH ... SELCT / EXPLAIN). Non-query statements go through Exec so
// last_insert_rowid / changes are meaningful. The classification is heuristic
// but covers the statements the gem's callers run through #execute.
func isQuery(query string) bool {
	s := strings.TrimLeft(query, " \t\r\n(")
	// Skip a leading -- line comment or /* */ block comment.
	for {
		if strings.HasPrefix(s, "--") {
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = strings.TrimLeft(s[i+1:], " \t\r\n(")
				continue
			}
			return false
		}
		if strings.HasPrefix(s, "/*") {
			if i := strings.Index(s, "*/"); i >= 0 {
				s = strings.TrimLeft(s[i+2:], " \t\r\n(")
				continue
			}
			return false
		}
		break
	}
	up := strings.ToUpper(s)
	switch {
	case strings.HasPrefix(up, "SELECT"),
		strings.HasPrefix(up, "VALUES"),
		strings.HasPrefix(up, "WITH"),
		strings.HasPrefix(up, "EXPLAIN"):
		return true
	case strings.HasPrefix(up, "PRAGMA"):
		// A PRAGMA that assigns (contains '=') acts as a set and returns nothing;
		// a bare PRAGMA reads and returns a row.
		return !strings.Contains(s, "=")
	default:
		return false
	}
}

// Execute runs sql with the given binds and returns the result rows as [][]Value
// (SQLite3::Database#execute). binds may be nil. Rows are positional slices; use
// ExecuteHash for the results_as_hash shape. A non-query statement (INSERT /
// UPDATE / DDL) runs and returns an empty slice.
func (d *Database) Execute(query string, binds []Value) ([]Row, error) {
	args := normalizeBinds(binds)
	if !isQuery(query) {
		if _, err := d.db.Exec(query, args...); err != nil {
			return nil, wrapError(err)
		}
		return []Row{}, nil
	}
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()
	out, _, err := scanRows(rows)
	if err != nil {
		return nil, wrapError(err)
	}
	return out, nil
}

// ExecuteBlock runs sql and calls fn once per result row, mirroring the block
// form of SQLite3::Database#execute. It stops and returns the first error fn
// returns. Non-query statements invoke fn zero times.
func (d *Database) ExecuteBlock(query string, binds []Value, fn func(Row) error) error {
	rows, err := d.Execute(query, binds)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteHash runs sql and returns rows as name-keyed maps, the shape
// SQLite3::Database#execute yields when results_as_hash is true. It works
// regardless of the results_as_hash flag; the flag governs which shape the
// gem's #execute picks.
func (d *Database) ExecuteHash(query string, binds []Value) ([]HashRow, error) {
	args := normalizeBinds(binds)
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()
	vals, cols, err := scanRows(rows)
	if err != nil {
		return nil, wrapError(err)
	}
	out := make([]HashRow, len(vals))
	for i, r := range vals {
		h := make(HashRow, len(cols))
		for j, c := range cols {
			h[c] = r[j]
		}
		out[i] = h
	}
	return out, nil
}

// Execute2 runs sql and returns the rows with the column-name header row
// prepended, matching SQLite3::Database#execute2 ("always return at least one
// row — the names of the columns"). The first element is the []Value of column
// names; the rest are data rows.
func (d *Database) Execute2(query string, binds []Value) ([]Row, error) {
	args := normalizeBinds(binds)
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()
	data, cols, err := scanRows(rows)
	if err != nil {
		return nil, wrapError(err)
	}
	header := make(Row, len(cols))
	for i, c := range cols {
		header[i] = c
	}
	out := make([]Row, 0, len(data)+1)
	out = append(out, header)
	out = append(out, data...)
	return out, nil
}

// ExecuteBatch runs every statement in sql sequentially, applying binds to each,
// and returns nothing but any error (SQLite3::Database#execute_batch). modernc's
// driver accepts multiple ';'-separated statements in a single Exec.
func (d *Database) ExecuteBatch(query string, binds []Value) error {
	args := normalizeBinds(binds)
	_, err := d.db.Exec(query, args...)
	return wrapError(err)
}

// Query prepares and runs sql, returning a *Statement positioned before the
// first row (SQLite3::Database#query). The caller steps it with Statement.Next /
// Statement.Step and must Close it. Binds are applied immediately.
func (d *Database) Query(query string, binds []Value) (*Statement, error) {
	st, err := d.Prepare(query)
	if err != nil {
		return nil, err
	}
	st.BindParams(binds)
	if err := st.exec(); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

// GetFirstRow runs sql and returns only its first row (SQLite3::Database#get_first_row),
// or nil if there are no rows.
func (d *Database) GetFirstRow(query string, binds []Value) (Row, error) {
	rows, err := d.Execute(query, binds)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// GetFirstValue runs sql and returns the first column of its first row
// (SQLite3::Database#get_first_value), or nil if there are no rows.
func (d *Database) GetFirstValue(query string, binds []Value) (Value, error) {
	row, err := d.GetFirstRow(query, binds)
	if err != nil {
		return nil, err
	}
	if len(row) == 0 {
		return nil, nil
	}
	return row[0], nil
}

// scanRows drains a *sql.Rows into positional []Value rows plus the column
// names. Values arrive from modernc as int64 / float64 / string / []byte / nil —
// exactly the gem's SQLite<->Ruby mapping — so they pass through unchanged. It
// is a var so tests can substitute a version that surfaces the (rare with
// modernc) Columns / Scan / Err failures, exercising the callers' error paths.
var scanRows = func(rows *sql.Rows) ([]Row, []string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out []Row
	for rows.Next() {
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := scanInto(rows, ptrs); err != nil {
			return nil, nil, err
		}
		row := make(Row, len(cols))
		for i, h := range holders {
			row[i] = normalizeResult(h)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, cols, nil
}

// scanInto copies one result row into the destination pointers. It is a var so a
// direct unit test can exercise the (with a live modernc row, unreachable-per-
// iteration) *sql.Rows.Scan failure by scanning a closed row.
var scanInto = func(rows *sql.Rows, ptrs []any) error {
	return rows.Scan(ptrs...)
}
