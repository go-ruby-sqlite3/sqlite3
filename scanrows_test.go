// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
)

// The scanRows helper's internal error branches (Columns error, Scan error,
// rows.Err error) are effectively unreachable through the pure-Go modernc
// backend, which buffers a whole result set into memory before iteration. To
// exercise them deterministically we register a tiny mock driver here whose rows
// can be told to fail at each of those points, then run real *sql.Rows through
// scanRows.

type mockMode int

const (
	modeOK mockMode = iota
	modeScanErr
	modeRowsErr
)

var errMockScan = errors.New("mock scan failure")
var errMockRows = errors.New("mock rows failure")

type mockDriver struct{ mode mockMode }

func (d mockDriver) Open(string) (driver.Conn, error) { return mockConn{d.mode}, nil }

type mockConn struct{ mode mockMode }

func (c mockConn) Prepare(string) (driver.Stmt, error) { return mockStmt{c.mode}, nil }
func (c mockConn) Close() error                        { return nil }
func (c mockConn) Begin() (driver.Tx, error)           { return nil, errors.New("no tx") }

type mockStmt struct{ mode mockMode }

func (s mockStmt) Close() error                                    { return nil }
func (s mockStmt) NumInput() int                                   { return 0 }
func (s mockStmt) Exec([]driver.Value) (driver.Result, error)      { return nil, errors.New("no exec") }
func (s mockStmt) Query(_ []driver.Value) (driver.Rows, error)     { return &mockRows{mode: s.mode}, nil }

type mockRows struct {
	mode  mockMode
	count int
}

func (r *mockRows) Columns() []string { return []string{"c"} }
func (r *mockRows) Close() error      { return nil }

func (r *mockRows) Next(dest []driver.Value) error {
	switch r.mode {
	case modeScanErr:
		// Return a value driver.Rows cannot convert into the *any target through
		// a normal path; instead we signal a scan-time failure by returning a
		// value that the convertAssign fails on. Simpler: return an error here on
		// the first row, which surfaces as rows.Err / Scan error to the caller.
		return errMockScan
	case modeRowsErr:
		if r.count == 0 {
			r.count++
			dest[0] = int64(1)
			return nil
		}
		return errMockRows
	default:
		if r.count == 0 {
			r.count++
			dest[0] = int64(1)
			return nil
		}
		return io.EOF
	}
}

var mockCounter atomic.Int64

func mockRowsFor(t *testing.T, mode mockMode) *sql.Rows {
	t.Helper()
	// Unique driver name per call; sql.Register panics on a duplicate name.
	name := fmt.Sprintf("mockscan_%d", mockCounter.Add(1))
	sql.Register(name, mockDriver{mode: mode})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rows, err := db.Query("SELECT c")
	if err != nil {
		t.Fatalf("query mock: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	return rows
}

// The real scanRows is captured once so a test that overrides the package var
// can restore it.
var realScanRows = scanRows

func TestScanRowsScanError(t *testing.T) {
	rows := mockRowsFor(t, modeScanErr)
	if _, _, err := realScanRows(rows); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestScanRowsRowsErr(t *testing.T) {
	rows := mockRowsFor(t, modeRowsErr)
	if _, _, err := realScanRows(rows); err == nil {
		t.Fatal("expected rows.Err")
	}
}

func TestScanRowsColumnsError(t *testing.T) {
	// A closed *sql.Rows makes rows.Columns() fail, covering that branch.
	rows := mockRowsFor(t, modeOK)
	_ = rows.Close()
	if _, _, err := realScanRows(rows); err == nil {
		t.Fatal("expected columns error on closed rows")
	}
}

func TestScanIntoClosedRowsError(t *testing.T) {
	// scanInto on a closed *sql.Rows makes rows.Scan fail, covering scanRows'
	// per-row Scan-error branch (unreachable per-iteration with modernc, which
	// buffers eagerly).
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	rows, err := db.db.Query("SELECT x FROM t")
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	var v any
	if err := scanInto(rows, []any{&v}); err == nil {
		t.Fatal("expected Scan error on closed rows")
	}
}

func TestColumnTypeNamesClosedRowsError(t *testing.T) {
	// columnTypeNames on a closed *sql.Rows makes ColumnTypes fail, covering the
	// Types metadata-error branch.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a INTEGER)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	rows, err := db.db.Query("SELECT a FROM t")
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if _, err := columnTypeNames(rows); err == nil {
		t.Fatal("expected ColumnTypes error on closed rows")
	}
}

func TestScanIntoOK(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	rows, err := db.db.Query("SELECT x FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no row")
	}
	var v any
	if err := scanInto(rows, []any{&v}); err != nil {
		t.Fatalf("scanInto: %v", err)
	}
	if v != int64(1) {
		t.Errorf("scanned = %v", v)
	}
}

func TestColumnTypeNamesOK(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a INTEGER)")
	rows, err := db.db.Query("SELECT a FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	names, err := columnTypeNames(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "INTEGER" {
		t.Errorf("names = %v", names)
	}
}

func TestScanRowsScanIntoError(t *testing.T) {
	// Override the inner scanInto seam so realScanRows hits its per-row
	// Scan-error branch against a live modernc result set.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	rows, err := db.db.Query("SELECT x FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	orig := scanInto
	scanInto = func(*sql.Rows, []any) error { return errMockScan }
	defer func() { scanInto = orig }()
	if _, _, err := realScanRows(rows); err == nil {
		t.Fatal("expected scanInto error to propagate")
	}
}

func TestScanRowsOK(t *testing.T) {
	rows := mockRowsFor(t, modeOK)
	out, cols, err := realScanRows(rows)
	if err != nil {
		t.Fatalf("scanRows: %v", err)
	}
	if len(cols) != 1 || cols[0] != "c" || len(out) != 1 || out[0][0] != int64(1) {
		t.Errorf("scanRows result cols=%v out=%#v", cols, out)
	}
}

// TestExecuteScanError overrides the scanRows seam so the Execute / ExecuteHash /
// Execute2 / statement.exec callers hit their post-scan error branch against the
// live modernc backend.
func TestExecuteScanErrorBranches(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")

	orig := scanRows
	scanRows = func(*sql.Rows) ([]Row, []string, error) {
		return nil, nil, errMockScan
	}
	defer func() { scanRows = orig }()

	if _, err := db.Execute("SELECT x FROM t", nil); err == nil {
		t.Error("Execute: expected scan error")
	}
	if _, err := db.ExecuteHash("SELECT x FROM t", nil); err == nil {
		t.Error("ExecuteHash: expected scan error")
	}
	if _, err := db.Execute2("SELECT x FROM t", nil); err == nil {
		t.Error("Execute2: expected scan error")
	}
	st, err := db.Prepare("SELECT x FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Execute(); err == nil {
		t.Error("Statement.Execute: expected scan error")
	}
}
