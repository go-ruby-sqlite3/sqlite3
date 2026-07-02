// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// oracleRuby locates a `ruby` that (a) is on PATH, (b) has the `sqlite3` gem
// installed, and (c) reports RUBY_VERSION >= "4.0" (the prompt's version gate —
// the gem's API is pinned to the shape shipped with Ruby 4.0+). When any of
// those is unmet the oracle tests skip, and the deterministic, gem-free suite
// alone keeps coverage at 100%. The qemu cross-arch lanes and the Windows lane
// have no target ruby, so they skip too.
func oracleRuby(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping sqlite3-gem oracle")
	}
	// Probe: version gate + gem presence in one shot. Prints "OK" on success.
	probe := `exit(1) if RUBY_VERSION < "4.0"; require "sqlite3"; print "OK"`
	out, err := exec.Command(bin, "-e", probe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "OK" {
		t.Skipf("ruby oracle unavailable (need RUBY_VERSION>=4.0 + sqlite3 gem): %s", out)
	}
	return bin
}

// rubyRows runs a Ruby script that opens an in-memory SQLite3::Database, runs
// the given setup + a final `p db.execute(query)`, and returns the inspected
// rows. The script binmodes stdout (the go-ruby-erb Windows lesson).
func rubyRows(t *testing.T, bin, setup, query string) string {
	t.Helper()
	script := "$stdout.binmode\n" +
		"require 'sqlite3'\n" +
		"db = SQLite3::Database.new(':memory:')\n" +
		setup + "\n" +
		"p db.execute(" + query + ")\n"
	out, err := exec.Command(bin, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\nscript:\n%s\noutput:\n%s", err, script, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// goRowsInspect runs the same SQL through this package and renders the rows in
// Ruby `p`-style so it can be compared byte-for-byte with the gem's output.
func goRowsInspect(t *testing.T, setup []string, query string) string {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range setup {
		if _, err := db.Execute(s, nil); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	rows, err := db.Execute(query, nil)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return inspectRows(rows)
}

// inspectRows renders [][]Value the way Ruby's `p [[...],...]` would, so the Go
// result can be diffed against the gem's stdout.
func inspectRows(rows []Row) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, r := range rows {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('[')
		for j, v := range r {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString(inspectValue(v))
		}
		b.WriteByte(']')
	}
	b.WriteByte(']')
	return b.String()
}

// inspectValue renders a single Value in Ruby inspect form.
func inspectValue(v Value) string {
	switch n := v.(type) {
	case nil:
		return "nil"
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		// Ruby renders an integral float as "N.0"; Go's 'g' drops the ".0", so
		// force at least one fractional digit to match the gem's inspect.
		s := strconv.FormatFloat(n, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") {
			s += ".0"
		}
		return s
	case string:
		return strconv.Quote(n)
	case []byte:
		// The gem returns a BLOB as an ASCII-8BIT String; Ruby inspects it with
		// escapes. Our corpus avoids non-printable blobs so a plain quote matches.
		return strconv.Quote(string(n))
	default:
		return "?"
	}
}

// TestOracleRowsMatchGem runs a corpus of SELECTs through both engines and
// asserts the rows render identically.
func TestOracleRowsMatchGem(t *testing.T) {
	bin := oracleRuby(t)
	cases := []struct {
		name  string
		setup []string
		query string
	}{
		{
			name: "ints_reals_text",
			setup: []string{
				"CREATE TABLE t(i INTEGER, r REAL, s TEXT)",
				"INSERT INTO t VALUES(1, 1.5, 'a')",
				"INSERT INTO t VALUES(2, 2.5, 'b')",
			},
			query: "SELECT i, r, s FROM t ORDER BY i",
		},
		{
			name: "nulls",
			setup: []string{
				"CREATE TABLE t(a, b)",
				"INSERT INTO t VALUES(1, NULL)",
				"INSERT INTO t VALUES(NULL, 'x')",
			},
			query: "SELECT a, b FROM t",
		},
		{
			name: "aggregate",
			setup: []string{
				"CREATE TABLE t(x INTEGER)",
				"INSERT INTO t VALUES(1),(2),(3),(4)",
			},
			query: "SELECT count(*), sum(x), avg(x), max(x), min(x) FROM t",
		},
		{
			name: "expressions",
			setup: []string{
				"CREATE TABLE t(x INTEGER)",
				"INSERT INTO t VALUES(10)",
			},
			query: "SELECT x + 5, x * 2, x / 4, 'lit', 3.14 FROM t",
		},
		{
			name:  "empty",
			setup: []string{"CREATE TABLE t(x)"},
			query: "SELECT x FROM t",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := rubyRows(t, bin, strings.Join(setupLines(c.setup), "\n"),
				strconv.Quote(c.query))
			got := goRowsInspect(t, c.setup, c.query)
			if got != want {
				t.Errorf("rows differ\n  gem: %s\n  go:  %s", want, got)
			}
		})
	}
}

// setupLines wraps each setup statement as a db.execute call for the Ruby side.
func setupLines(stmts []string) []string {
	out := make([]string, len(stmts))
	for i, s := range stmts {
		out[i] = "db.execute(" + strconv.Quote(s) + ")"
	}
	return out
}

// TestOracleErrorCodesMatchGem provokes distinct SQLite errors and asserts the
// gem raises the same-numbered result code the Go binding reports.
func TestOracleErrorCodesMatchGem(t *testing.T) {
	bin := oracleRuby(t)
	cases := []struct {
		name  string
		setup []string
		bad   string
	}{
		{
			name:  "syntax_error",
			setup: nil,
			bad:   "SELECT bad syntax here",
		},
		{
			name: "unique_constraint",
			setup: []string{
				"CREATE TABLE t(x INTEGER PRIMARY KEY)",
				"INSERT INTO t VALUES(1)",
			},
			bad: "INSERT INTO t VALUES(1)",
		},
		{
			name: "not_null_constraint",
			setup: []string{
				"CREATE TABLE t(x NOT NULL)",
			},
			bad: "INSERT INTO t VALUES(NULL)",
		},
		{
			name:  "no_such_table",
			setup: nil,
			bad:   "SELECT * FROM missing_table",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gemCode := rubyErrorCode(t, bin, c.setup, c.bad)
			goCode := goErrorCode(t, c.setup, c.bad)
			if gemCode != goCode {
				t.Errorf("result code differs: gem=%d go=%d", gemCode, goCode)
			}
		})
	}
}

// rubyErrorCode runs the failing SQL through the gem and prints the raised
// SQLite3::Exception#code (the SQLite result code).
func rubyErrorCode(t *testing.T, bin string, setup []string, bad string) int {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("$stdout.binmode\nrequire 'sqlite3'\n")
	sb.WriteString("db = SQLite3::Database.new(':memory:')\n")
	for _, s := range setup {
		sb.WriteString("db.execute(" + strconv.Quote(s) + ")\n")
	}
	sb.WriteString("begin\n  db.execute(" + strconv.Quote(bad) + ")\n")
	sb.WriteString("rescue SQLite3::Exception => e\n  print e.code\nend\n")
	out, err := exec.Command(bin, "-e", sb.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\noutput:\n%s", err, out)
	}
	code, cerr := strconv.Atoi(strings.TrimSpace(string(out)))
	if cerr != nil {
		t.Fatalf("could not parse gem error code %q: %v", out, cerr)
	}
	// The gem's #code is the primary result code; reduce any extended bits so
	// the comparison is against the same primary code the Go side reports.
	return code & 0xff
}

// goErrorCode runs the same failing SQL through this package and returns the
// primary SQLite result code of the raised error.
func goErrorCode(t *testing.T, setup []string, bad string) int {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range setup {
		if _, err := db.Execute(s, nil); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	_, err = db.Execute(bad, nil)
	se, ok := err.(*Error)
	if !ok {
		t.Fatalf("want *Error from %q, got %T: %v", bad, err, err)
	}
	// The gem's #code returns the primary result code; compare primaries.
	return se.PrimaryCode()
}
