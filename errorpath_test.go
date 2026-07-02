// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"database/sql"
	"errors"
	"testing"
)

func TestOpenSQLOpenError(t *testing.T) {
	orig := sqlOpen
	sentinel := errors.New("open boom")
	sqlOpen = func(string, string) (*sql.DB, error) { return nil, sentinel }
	defer func() { sqlOpen = orig }()
	_, err := Open(":memory:")
	if err == nil {
		t.Fatal("expected sqlOpen error to propagate")
	}
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("want *Error, got %T", err)
	}
}

// closedDB returns a database whose underlying connection pool is closed, so
// every operation on it fails at the driver. This is a real seam (no fault
// injection) for exercising the error branches of the query/exec helpers.
func closedDB(t *testing.T) *Database {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.db.Close(); err != nil {
		t.Fatalf("close underlying: %v", err)
	}
	return db
}

func TestLastInsertRowIDError(t *testing.T) {
	if _, err := closedDB(t).LastInsertRowID(); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestChangesError(t *testing.T) {
	if _, err := closedDB(t).Changes(); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestTotalChangesError(t *testing.T) {
	if _, err := closedDB(t).TotalChanges(); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestBusyTimeoutError(t *testing.T) {
	if err := closedDB(t).BusyTimeout(10); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestExecuteNonQueryError(t *testing.T) {
	// A non-query statement on a closed db fails in the Exec branch.
	if _, err := closedDB(t).Execute("CREATE TABLE x(a)", nil); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestExecuteQueryError(t *testing.T) {
	// A query statement on a closed db fails in the Query branch.
	if _, err := closedDB(t).Execute("SELECT 1", nil); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestExecuteHashQueryError(t *testing.T) {
	if _, err := closedDB(t).ExecuteHash("SELECT 1", nil); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestExecute2QueryError(t *testing.T) {
	if _, err := closedDB(t).Execute2("SELECT 1", nil); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestQueryPrepareErrorClosed(t *testing.T) {
	if _, err := closedDB(t).Query("SELECT 1", nil); err == nil {
		t.Fatal("expected prepare error on closed db")
	}
}

func TestBeginCommitRollbackErrors(t *testing.T) {
	db := closedDB(t)
	if err := db.Begin(""); err == nil {
		t.Error("Begin on closed db should error")
	}
	if err := db.Commit(); err == nil {
		t.Error("Commit on closed db should error")
	}
	if err := db.Rollback(); err == nil {
		t.Error("Rollback on closed db should error")
	}
}

func TestInTransactionBeginError(t *testing.T) {
	// InTransaction issues a probe BEGIN; on a closed db it errors, which the
	// method interprets as "already in a transaction" (true). Covers that branch.
	in, err := closedDB(t).InTransaction()
	if err != nil {
		t.Fatalf("InTransaction: %v", err)
	}
	if !in {
		t.Error("closed-db probe BEGIN failure should report in-transaction")
	}
}

func TestTransactionBlockCommitError(t *testing.T) {
	// Begin succeeds on a live db; then we close the pool inside fn so the final
	// Commit fails, exercising Transaction's commit-error branch.
	db := openMem(t)
	err := db.Transaction("", func() error {
		return db.db.Close() // returns nil; closes the pool
	})
	if err == nil {
		t.Fatal("expected Commit to fail after pool close")
	}
}

func TestStatementQueryExecErrorClosed(t *testing.T) {
	// Prepare a valid statement, then close the pool so exec's Query fails.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	st, err := db.Prepare("SELECT x FROM t")
	if err != nil {
		t.Fatal(err)
	}
	_ = db.db.Close()
	if _, err := st.Execute(); err == nil {
		t.Fatal("expected exec error after pool close")
	}
}

func TestStatementTypesQueryError(t *testing.T) {
	// Types re-queries for column metadata; closing the pool after a successful
	// first exec makes that re-query fail, covering the Types Query-error branch.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a INTEGER)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	st, err := db.Prepare("SELECT a FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Columns(); err != nil { // force a successful exec first
		t.Fatal(err)
	}
	_ = db.db.Close()
	if _, err := st.Types(); err == nil {
		t.Fatal("expected Types re-query to fail after pool close")
	}
}
