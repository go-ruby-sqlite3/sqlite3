// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"database/sql"
	"reflect"
	"testing"
)

func TestPrepareExecute(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b)")
	st, err := db.Prepare("INSERT INTO t VALUES(?, ?)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.SQL() != "INSERT INTO t VALUES(?, ?)" {
		t.Errorf("SQL() = %q", st.SQL())
	}
	if st.Closed() {
		t.Error("fresh statement reports closed")
	}
	st.BindParams([]Value{1, "x"})
	rows, err := st.Execute()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("insert returned %d rows", len(rows))
	}
	v, _ := db.GetFirstValue("SELECT a FROM t", nil)
	if v != int64(1) {
		t.Errorf("inserted a = %v", v)
	}
}

func TestPrepareBadSQL(t *testing.T) {
	db := openMem(t)
	if _, err := db.Prepare("SELECT bogus syntax )("); err == nil {
		t.Fatal("expected prepare error")
	}
}

func TestStatementStepLoop(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	for i := 1; i <= 3; i++ {
		mustExec(t, db, "INSERT INTO t VALUES(?)", i)
	}
	st, err := db.Prepare("SELECT x FROM t ORDER BY x")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var got []int64
	for {
		row, ok, err := st.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		got = append(got, row[0].(int64))
	}
	if !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Errorf("stepped = %v", got)
	}
	// Stepping past the end again stays exhausted.
	if _, ok, _ := st.Step(); ok {
		t.Error("step past end returned a row")
	}
}

func TestStatementColumnsAndTypes(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a INTEGER, b TEXT)")
	mustExec(t, db, "INSERT INTO t VALUES(1, 'x')")
	st, err := db.Prepare("SELECT a, b FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cols, err := st.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cols, []string{"a", "b"}) {
		t.Errorf("columns = %v", cols)
	}
	types, err := st.Types()
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 || types[0] != "INTEGER" || types[1] != "TEXT" {
		t.Errorf("types = %v, want [INTEGER TEXT]", types)
	}
}

func TestStatementTypesColumnTypesError(t *testing.T) {
	// Override the columnTypeNames seam so the ColumnTypes-error branch of Types
	// runs against the live backend (modernc never fails there naturally).
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a INTEGER)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("SELECT a FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	orig := columnTypeNames
	columnTypeNames = func(*sql.Rows) ([]string, error) {
		return nil, errMockScan
	}
	defer func() { columnTypeNames = orig }()
	if _, err := st.Types(); err == nil {
		t.Fatal("expected ColumnTypes error")
	}
}

func TestStatementTypesNonQuery(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	st, err := db.Prepare("INSERT INTO t VALUES(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	types, err := st.Types()
	if err != nil {
		t.Fatal(err)
	}
	if types != nil {
		t.Errorf("types of non-query = %v, want nil", types)
	}
}

func TestStatementReset(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("SELECT x FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Execute(); err != nil {
		t.Fatal(err)
	}
	// Second Execute without reset returns the buffered rows (no re-run).
	rows, err := st.Execute()
	if err != nil || len(rows) != 1 {
		t.Fatalf("second execute rows = %d (err %v)", len(rows), err)
	}
	if err := st.Reset(); err != nil {
		t.Fatal(err)
	}
	// After reset, insert a second row then re-run: sees both.
	mustExec(t, db, "INSERT INTO t VALUES(2)")
	rows, err = st.Execute()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Errorf("post-reset rows = %d, want 2", len(rows))
	}
}

func TestStatementExecuteHash(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b)")
	mustExec(t, db, "INSERT INTO t VALUES(1, 'y')")
	st, err := db.Prepare("SELECT a, b FROM t")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rows, err := st.ExecuteHash()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["a"] != int64(1) || rows[0]["b"] != "y" {
		t.Errorf("hash rows = %#v", rows)
	}
}

func TestStatementBindParamPositional(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b)")
	st, err := db.Prepare("INSERT INTO t VALUES(?, ?)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.BindParam(1, 100)        // int index
	st.BindParam(int64(2), "z") // int64 index
	if _, err := st.Execute(); err != nil {
		t.Fatal(err)
	}
	row, _ := db.GetFirstRow("SELECT a,b FROM t", nil)
	if !reflect.DeepEqual(row, Row{int64(100), "z"}) {
		t.Errorf("row = %#v", row)
	}
}

func TestStatementBindParamNamed(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b,c)")
	st, err := db.Prepare("INSERT INTO t VALUES(:a, $b, @c)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.BindParam(":a", 1) // colon sigil
	st.BindParam("$b", 2) // dollar sigil
	st.BindParam("@c", 3) // at sigil
	if _, err := st.Execute(); err != nil {
		t.Fatal(err)
	}
	row, _ := db.GetFirstRow("SELECT a,b,c FROM t", nil)
	if !reflect.DeepEqual(row, Row{int64(1), int64(2), int64(3)}) {
		t.Errorf("named-bound row = %#v", row)
	}
}

func TestStatementBindParamIndexedString(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a,b)")
	st, err := db.Prepare("INSERT INTO t VALUES(?1, ?2)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// A numeric string maps to the ?NNN positional slot.
	st.BindParam("1", 7)
	st.BindParam("2", 8)
	if _, err := st.Execute(); err != nil {
		t.Fatal(err)
	}
	row, _ := db.GetFirstRow("SELECT a,b FROM t", nil)
	if !reflect.DeepEqual(row, Row{int64(7), int64(8)}) {
		t.Errorf("?NNN row = %#v", row)
	}
}

func TestStatementBindParamUnknownKeyType(t *testing.T) {
	db := openMem(t)
	st, err := db.Prepare("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// An unsupported key type is silently ignored (no panic), matching a
	// forgiving binder.
	st.BindParam(3.14, "ignored")
	if _, err := st.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestStatementPositionalGapDefaultsNull(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a, b)")
	st, err := db.Prepare("INSERT INTO t VALUES(?1, ?2)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Bind only slot 2; slot 1 is a gap and defaults to NULL.
	st.BindParam(2, "x")
	if _, err := st.Execute(); err != nil {
		t.Fatal(err)
	}
	row, _ := db.GetFirstRow("SELECT a, b FROM t", nil)
	if row[0] != nil || row[1] != "x" {
		t.Errorf("gap row = %#v, want [nil x]", row)
	}
}

func TestStatementClearBindings(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a)")
	st, err := db.Prepare("INSERT INTO t VALUES(?)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.BindParam(1, 5)
	st.ClearBindings()
	// After clearing, re-bind slot 1 and execute; the cleared value (5) is gone
	// and the freshly bound value (9) is stored.
	st.BindParam(1, 9)
	if _, err := st.Execute(); err != nil {
		t.Fatal(err)
	}
	v, _ := db.GetFirstValue("SELECT a FROM t", nil)
	if v != int64(9) {
		t.Errorf("value after ClearBindings+rebind = %v, want 9", v)
	}
}

func TestStatementExecuteError(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x INTEGER PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("INSERT INTO t VALUES(?)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.BindParam(1, 1) // duplicate PK -> constraint error
	if _, err := st.Execute(); err == nil {
		t.Fatal("expected constraint error")
	}
}

func TestStatementExecuteHashError(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x INTEGER PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("INSERT INTO t VALUES(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.ExecuteHash(); err == nil {
		t.Fatal("expected error")
	}
}

func TestStatementColumnsError(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x INTEGER PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("INSERT INTO t VALUES(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Columns(); err == nil {
		t.Fatal("expected error from failing exec")
	}
}

func TestStatementTypesExecError(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x INTEGER PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("INSERT INTO t VALUES(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// exec() fails (constraint) so Types surfaces that error before the
	// non-query short-circuit.
	if _, err := st.Types(); err == nil {
		t.Fatal("expected error")
	}
}

func TestStatementStepExecError(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x INTEGER PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("INSERT INTO t VALUES(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, _, err := st.Step(); err == nil {
		t.Fatal("expected error from Step")
	}
}

func TestStatementExecuteHashExecError(t *testing.T) {
	// ExecuteHash where exec itself errors (constraint) — covers the early
	// return branch.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x INTEGER PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, _ := db.Prepare("INSERT INTO t VALUES(1)")
	defer st.Close()
	if _, err := st.ExecuteHash(); err == nil {
		t.Fatal("expected exec error")
	}
}

func TestStatementClose(t *testing.T) {
	db := openMem(t)
	st, err := db.Prepare("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if !st.Closed() {
		t.Error("closed statement reports open")
	}
	// Idempotent.
	if err := st.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestDatabaseQuery(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	mustExec(t, db, "INSERT INTO t VALUES(2)")
	st, err := db.Query("SELECT x FROM t ORDER BY x", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var got []int64
	for {
		row, ok, err := st.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		got = append(got, row[0].(int64))
	}
	if !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("query stepped = %v", got)
	}
}

func TestDatabaseQueryWithBinds(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	mustExec(t, db, "INSERT INTO t VALUES(2)")
	st, err := db.Query("SELECT x FROM t WHERE x > ?", []Value{1})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	row, ok, _ := st.Next()
	if !ok || row[0] != int64(2) {
		t.Errorf("bound query row = %#v (ok %v)", row, ok)
	}
}

func TestDatabaseQueryPrepareError(t *testing.T) {
	db := openMem(t)
	if _, err := db.Query("SELECT bogus )(", nil); err == nil {
		t.Fatal("expected prepare error")
	}
}

func TestDatabaseQueryExecError(t *testing.T) {
	// modernc validates at Prepare, so to hit Query's post-prepare exec-error
	// branch we override the scanRows seam to fail during exec.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	orig := scanRows
	scanRows = func(*sql.Rows) ([]Row, []string, error) {
		return nil, nil, errMockScan
	}
	defer func() { scanRows = orig }()
	if _, err := db.Query("SELECT x FROM t", nil); err == nil {
		t.Fatal("expected exec error")
	}
}
