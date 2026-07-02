// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"bytes"
	"database/sql"
	"reflect"
	"testing"
)

func TestIsQuery(t *testing.T) {
	cases := map[string]bool{
		"SELECT 1":                       true,
		"  select * from t":              true,
		"\n\tSELECT x":                   true,
		"(SELECT 1)":                     true,
		"VALUES (1),(2)":                 true,
		"WITH c AS (SELECT 1) SELECT *":  true,
		"EXPLAIN SELECT 1":               true,
		"PRAGMA table_info(t)":           true,  // bare pragma reads
		"PRAGMA busy_timeout = 5":        false, // assigning pragma sets
		"INSERT INTO t VALUES(1)":        false,
		"UPDATE t SET x=1":               false,
		"DELETE FROM t":                  false,
		"CREATE TABLE t(x)":              false,
		"DROP TABLE t":                   false,
		"-- a comment\nSELECT 1":         true,
		"/* block */ SELECT 1":           true,
		"-- only a comment":              false, // no statement after comment
		"/* unterminated":                false, // no closing */
		"-- c1\n-- c2\nINSERT INTO t V":  false,
		"/* c */ /* d */ INSERT":         false,
		"":                               false,
	}
	for sql, want := range cases {
		if got := isQuery(sql); got != want {
			t.Errorf("isQuery(%q) = %v, want %v", sql, got, want)
		}
	}
}

func TestNormalizeResult(t *testing.T) {
	blob := []byte{1, 2, 3}
	cases := []struct {
		in   any
		want Value
	}{
		{nil, nil},
		{int64(5), int64(5)},
		{int(5), int64(5)},
		{int32(5), int64(5)},
		{float64(1.5), float64(1.5)},
		{float32(1.5), float64(1.5)},
		{"s", "s"},
		{true, int64(1)},
		{false, int64(0)},
		{uint8(7), uint8(7)}, // default pass-through
	}
	for _, c := range cases {
		got := normalizeResult(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("normalizeResult(%#v) = %#v, want %#v", c.in, got, c.want)
		}
	}
	// []byte is copied, not aliased.
	got := normalizeResult(blob).([]byte)
	if !bytes.Equal(got, blob) {
		t.Errorf("blob = %v, want %v", got, blob)
	}
	blob[0] = 99
	if got[0] == 99 {
		t.Error("normalizeResult did not copy the blob (aliased backing array)")
	}
}

func TestNormalizeBind(t *testing.T) {
	named := sql.Named("a", 1)
	cases := []struct {
		in   Value
		want any
	}{
		{nil, nil},
		{int(3), int64(3)},
		{int32(3), int64(3)},
		{int64(3), int64(3)},
		{float32(2.5), float64(2.5)},
		{float64(2.5), float64(2.5)},
		{true, int64(1)},
		{false, int64(0)},
		{"str", "str"},
		{[]byte{9}, []byte{9}},
		{named, named},
		{complex(1, 2), complex(1, 2)}, // default pass-through
	}
	for _, c := range cases {
		got := normalizeBind(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("normalizeBind(%#v) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestNormalizeBinds(t *testing.T) {
	if normalizeBinds(nil) != nil {
		t.Error("normalizeBinds(nil) should be nil")
	}
	if normalizeBinds([]Value{}) != nil {
		t.Error("normalizeBinds(empty) should be nil")
	}
	got := normalizeBinds([]Value{1, "x", nil})
	want := []any{int64(1), "x", nil}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeBinds = %#v, want %#v", got, want)
	}
}

func TestNamedBindThroughExecute(t *testing.T) {
	// Bind a BLOB via []byte and a NamedArg straight through Execute's bind path.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(a, b)")
	mustExec(t, db, "INSERT INTO t VALUES(:a, ?)", sql.Named("a", 5), []byte{7})
	row, _ := db.GetFirstRow("SELECT a, b FROM t", nil)
	if row[0] != int64(5) {
		t.Errorf("named bind a = %v, want 5", row[0])
	}
	if b, ok := row[1].([]byte); !ok || !bytes.Equal(b, []byte{7}) {
		t.Errorf("blob bind = %#v", row[1])
	}
}
