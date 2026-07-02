// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"errors"
	"testing"
)

func TestTransactionCommit(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	if err := db.Begin(""); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	if err := db.Commit(); err != nil {
		t.Fatal(err)
	}
	v, _ := db.GetFirstValue("SELECT count(*) FROM t", nil)
	if v != int64(1) {
		t.Errorf("committed count = %v, want 1", v)
	}
}

func TestTransactionRollback(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	if err := db.Begin(Immediate); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	if err := db.Rollback(); err != nil {
		t.Fatal(err)
	}
	v, _ := db.GetFirstValue("SELECT count(*) FROM t", nil)
	if v != int64(0) {
		t.Errorf("rolled-back count = %v, want 0", v)
	}
}

func TestTransactionModes(t *testing.T) {
	for _, m := range []TransactionMode{Deferred, Immediate, Exclusive, ""} {
		db := openMem(t)
		if err := db.Begin(m); err != nil {
			t.Fatalf("Begin(%q): %v", m, err)
		}
		if err := db.Commit(); err != nil {
			t.Fatalf("Commit after Begin(%q): %v", m, err)
		}
	}
}

func TestTransactionBlockCommit(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	err := db.Transaction(Deferred, func() error {
		mustExec(t, db, "INSERT INTO t VALUES(1)")
		mustExec(t, db, "INSERT INTO t VALUES(2)")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := db.GetFirstValue("SELECT count(*) FROM t", nil)
	if v != int64(2) {
		t.Errorf("block-committed count = %v, want 2", v)
	}
}

func TestTransactionBlockRollbackOnError(t *testing.T) {
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x)")
	sentinel := errors.New("boom")
	err := db.Transaction("", func() error {
		mustExec(t, db, "INSERT INTO t VALUES(1)")
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("Transaction error = %v, want sentinel", err)
	}
	v, _ := db.GetFirstValue("SELECT count(*) FROM t", nil)
	if v != int64(0) {
		t.Errorf("count after rolled-back block = %v, want 0", v)
	}
}

func TestTransactionBlockBeginError(t *testing.T) {
	db := openMem(t)
	// Start a transaction manually so the block's inner Begin fails.
	if err := db.Begin(""); err != nil {
		t.Fatal(err)
	}
	defer db.Rollback()
	err := db.Transaction("", func() error { return nil })
	if err == nil {
		t.Fatal("expected nested Begin to error")
	}
}

func TestInTransaction(t *testing.T) {
	db := openMem(t)
	in, err := db.InTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if in {
		t.Error("fresh connection reports in-transaction")
	}
	if err := db.Begin(""); err != nil {
		t.Fatal(err)
	}
	in, err = db.InTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if !in {
		t.Error("open transaction not detected")
	}
	_ = db.Rollback()
}
