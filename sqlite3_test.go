// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// openMem opens a fresh in-memory database and registers its cleanup.
func openMem(t *testing.T) *Database {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// mustExec runs a statement that returns no rows and fails the test on error.
func mustExec(t *testing.T, db *Database, sql string, binds ...Value) {
	t.Helper()
	if _, err := db.Execute(sql, binds); err != nil {
		t.Fatalf("Execute(%q): %v", sql, err)
	}
}

func TestOpenAndClose(t *testing.T) {
	db := openMem(t)
	if db.Closed() {
		t.Fatal("new db reports closed")
	}
	if db.Path() != ":memory:" {
		t.Errorf("Path = %q, want :memory:", db.Path())
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !db.Closed() {
		t.Error("closed db reports open")
	}
	// Idempotent close.
	if err := db.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestNewAlias(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
	if _, err := db.Execute("SELECT 1", nil); err != nil {
		t.Fatalf("Execute after New: %v", err)
	}
}

func TestOpenFileDB(t *testing.T) {
	// A real on-disk file; modernc opens actual files (CGO=0 still). This is the
	// gem's file-DB behaviour.
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer db.Close()
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(?)", 5)
	v, err := db.GetFirstValue("SELECT x FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v != int64(5) {
		t.Errorf("first value = %v, want 5", v)
	}
}

func TestOpenBadPath(t *testing.T) {
	// A directory cannot be opened as a database file — Ping fails and the error
	// is wrapped.
	dir := t.TempDir()
	_, err := Open(dir)
	if err == nil {
		t.Fatal("expected error opening a directory as a db")
	}
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("want *Error, got %T: %v", err, err)
	}
}

func TestTypeMappingResults(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(i INTEGER, r REAL, s TEXT, b BLOB, n)")
	mustExec(t, db, "INSERT INTO t VALUES(?,?,?,?,?)", 42, 3.5, "text", []byte{1, 2, 3}, nil)
	rows, err := db.Execute("SELECT i,r,s,b,n FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r[0] != int64(42) {
		t.Errorf("INTEGER -> %T(%v), want int64(42)", r[0], r[0])
	}
	if r[1] != float64(3.5) {
		t.Errorf("REAL -> %T(%v), want float64(3.5)", r[1], r[1])
	}
	if r[2] != "text" {
		t.Errorf("TEXT -> %T(%v), want string", r[2], r[2])
	}
	if b, ok := r[3].([]byte); !ok || !bytes.Equal(b, []byte{1, 2, 3}) {
		t.Errorf("BLOB -> %T(%v), want []byte{1,2,3}", r[3], r[3])
	}
	if r[4] != nil {
		t.Errorf("NULL -> %T(%v), want nil", r[4], r[4])
	}
}

func TestBindConveniences(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b,c,d,e,f)")
	// int, int32, float32, bool true/false conversions on bind.
	mustExec(t, db, "INSERT INTO t VALUES(?,?,?,?,?,?)",
		int(1), int32(2), float32(1.5), true, false, "s")
	rows, err := db.Execute("SELECT * FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := rows[0]
	want := Row{int64(1), int64(2), 1.5, int64(1), int64(0), "s"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bind conveniences = %#v, want %#v", got, want)
	}
}

func TestExecuteBlock(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	for i := 1; i <= 3; i++ {
		mustExec(t, db, "INSERT INTO t VALUES(?)", i)
	}
	var sum int64
	err := db.ExecuteBlock("SELECT x FROM t ORDER BY x", nil, func(r Row) error {
		sum += r[0].(int64)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum != 6 {
		t.Errorf("sum = %d, want 6", sum)
	}
}

func TestExecuteBlockError(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	sentinel := errors.New("stop")
	err := db.ExecuteBlock("SELECT x FROM t", nil, func(r Row) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("ExecuteBlock error = %v, want sentinel", err)
	}
}

func TestExecuteBlockQueryError(t *testing.T) {
	db := openMem(t)
	err := db.ExecuteBlock("SELECT * FROM missing", nil, func(r Row) error { return nil })
	if err == nil {
		t.Fatal("expected error from bad query")
	}
}

func TestExecuteHash(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b)")
	mustExec(t, db, "INSERT INTO t VALUES(1,'x')")
	rows, err := db.ExecuteHash("SELECT a,b FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["a"] != int64(1) || rows[0]["b"] != "x" {
		t.Errorf("hash row = %#v", rows)
	}
}

func TestExecuteHashError(t *testing.T) {
	db := openMem(t)
	if _, err := db.ExecuteHash("SELECT * FROM missing", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecute2(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b)")
	mustExec(t, db, "INSERT INTO t VALUES(1,2)")
	rows, err := db.Execute2("SELECT a,b FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("execute2 rows = %d, want 2 (header + 1 data)", len(rows))
	}
	if !reflect.DeepEqual(rows[0], Row{"a", "b"}) {
		t.Errorf("header = %#v, want [a b]", rows[0])
	}
	if !reflect.DeepEqual(rows[1], Row{int64(1), int64(2)}) {
		t.Errorf("data = %#v", rows[1])
	}
}

func TestExecute2Error(t *testing.T) {
	db := openMem(t)
	if _, err := db.Execute2("SELECT * FROM missing", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteBatch(t *testing.T) {
	db := openMem(t)
	err := db.ExecuteBatch("CREATE TABLE t(x); INSERT INTO t VALUES(1); INSERT INTO t VALUES(2);", nil)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := db.GetFirstValue("SELECT count(*) FROM t", nil)
	if v != int64(2) {
		t.Errorf("count = %v, want 2", v)
	}
}

func TestExecuteBatchError(t *testing.T) {
	db := openMem(t)
	if err := db.ExecuteBatch("NOT SQL AT ALL", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetFirstRowEmpty(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	row, err := db.GetFirstRow("SELECT x FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Errorf("first row of empty table = %#v, want nil", row)
	}
}

func TestGetFirstRowError(t *testing.T) {
	db := openMem(t)
	if _, err := db.GetFirstRow("SELECT * FROM missing", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetFirstValueEmpty(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	v, err := db.GetFirstValue("SELECT x FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Errorf("first value of empty = %#v, want nil", v)
	}
}

func TestGetFirstValueError(t *testing.T) {
	db := openMem(t)
	if _, err := db.GetFirstValue("SELECT * FROM missing", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetFirstValueNoColumns(t *testing.T) {
	db := openMem(t)
	// A statement producing a row with columns; then simulate a zero-column row
	// is not directly expressible in SQLite, so cover the len(row)==0 guard by a
	// PRAGMA that yields an empty column set is also not portable. Instead assert
	// the normal single-column path returns the value.
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(7)")
	v, err := db.GetFirstValue("SELECT x FROM t", nil)
	if err != nil || v != int64(7) {
		t.Fatalf("first value = %v (err %v)", v, err)
	}
}

func TestLastInsertRowIDAndChanges(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(10)")
	id, err := db.LastInsertRowID()
	if err != nil || id != 1 {
		t.Fatalf("LastInsertRowID = %d (err %v), want 1", id, err)
	}
	mustExec(t, db, "INSERT INTO t VALUES(20)")
	mustExec(t, db, "UPDATE t SET x = x + 1")
	ch, err := db.Changes()
	if err != nil || ch != 2 {
		t.Fatalf("Changes = %d (err %v), want 2", ch, err)
	}
	tot, err := db.TotalChanges()
	if err != nil || tot < 2 {
		t.Fatalf("TotalChanges = %d (err %v)", tot, err)
	}
}

func TestBusyTimeout(t *testing.T) {
	db := openMem(t)
	if err := db.BusyTimeout(1234); err != nil {
		t.Fatalf("BusyTimeout: %v", err)
	}
	v, _ := db.GetFirstValue("PRAGMA busy_timeout", nil)
	if v != int64(1234) {
		t.Errorf("busy_timeout pragma = %v, want 1234", v)
	}
}

func TestResultsAsHashFlag(t *testing.T) {
	db := openMem(t)
	if db.ResultsAsHash() {
		t.Error("results_as_hash defaults true, want false")
	}
	db.SetResultsAsHash(true)
	if !db.ResultsAsHash() {
		t.Error("SetResultsAsHash(true) not reflected")
	}
}

func TestTypeTranslationFlag(t *testing.T) {
	db := openMem(t)
	if db.TypeTranslation() {
		t.Error("type_translation defaults true")
	}
	db.SetTypeTranslation(true)
	if !db.TypeTranslation() {
		t.Error("SetTypeTranslation(true) not reflected")
	}
}
