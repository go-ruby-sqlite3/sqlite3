// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package sqlite3 is a pure-Go (CGO=0) reimplementation of the Ruby sqlite3-ruby
// gem's SQLite3 API. Upstream, sqlite3-ruby is a C extension binding
// libsqlite3; this package instead binds modernc.org/sqlite — a pure-Go,
// CGO-free transpilation of the real SQLite engine — so the whole stack links
// statically with CGO_ENABLED=0 on every 64-bit target the go-* ecosystem
// supports (amd64, arm64, riscv64, loong64, ppc64le, s390x).
//
// The API mirrors SQLite3::Database and SQLite3::Statement:
//
//	db, _ := sqlite3.Open(":memory:")
//	defer db.Close()
//	db.Execute("CREATE TABLE t (a, b)", nil)
//	db.Execute("INSERT INTO t VALUES (?, ?)", []Value{1, "x"})
//	rows, _ := db.Execute("SELECT * FROM t", nil)   // [][]Value{{int64(1), "x"}}
//
// Values crossing the boundary use the gem's SQLite<->Ruby type mapping:
//
//	SQLite    Go (this package)      Ruby (rbgo binding)
//	INTEGER   int64                  Integer
//	REAL      float64                Float
//	TEXT      string                 String (UTF-8)
//	BLOB      []byte                 String (ASCII-8BIT)
//	NULL      nil                    nil
//
// A Value is any of the Go types above (plus the int / float32 / bool
// conveniences normalised on bind). Errors map SQLite result codes to the
// SQLite3::Exception hierarchy via the Error type and its ResultCode.
package sqlite3

// Value is a datum crossing the Go<->SQLite boundary. On results it is always
// one of: nil, int64, float64, string, or []byte. On binds this package also
// accepts int, int32, float32, and bool for convenience and normalises them.
type Value = any

// Row is a single result row as a slice of column Values, in column order. This
// is the shape SQLite3::Database#execute yields when results_as_hash is false
// (the default).
type Row = []Value

// HashRow is a single result row keyed by column name, the shape yielded when
// SQLite3::Database#results_as_hash= is true. Positional access is still
// available through the parallel Row returned alongside it by the hash-aware
// helpers; this map preserves the gem's string-keyed access.
type HashRow = map[string]Value
