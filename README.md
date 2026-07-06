<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-sqlite3/brand/main/social/go-ruby-sqlite3-sqlite3.png" alt="go-ruby-sqlite3/sqlite3" width="720"></p>

# sqlite3 — go-ruby-sqlite3

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-sqlite3.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the Ruby
[`sqlite3`](https://github.com/sparklemotion/sqlite3-ruby) gem's `SQLite3` API.**
Upstream, `sqlite3-ruby` is a C extension linking `libsqlite3`; this module binds
[`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) instead — a
CGO-free transpilation of the **real SQLite engine** into pure Go — and exposes
the gem's `SQLite3::Database` / `SQLite3::Statement` surface on top. The result is
a genuine, embedded, file-backed SQLite that links statically with
`CGO_ENABLED=0` on every 64-bit target the go-\* ecosystem supports.

It is the SQLite backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module — a sibling of
[go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (the Onigmo engine),
[go-ruby-erb](https://github.com/go-ruby-erb/erb) (the ERB compiler), and
[go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (the Psych port).

> **What it is — and isn't.** The database engine, the SQL, and the
> SQLite↔Ruby type coercions are fully deterministic and need **no interpreter**,
> so they live here as pure Go. Turning a returned row into live Ruby objects
> (a `String`, an `Integer`, an `Array`/`Hash`) is the host's job; this library
> hands back a small, explicit value model (`int64`, `float64`, `string`,
> `[]byte`, `nil`) the host maps to and from its own objects, plus an `Error`
> that names the exact `SQLite3::Exception` subclass to raise.

## Features

Faithful port of the gem's `SQLite3::Database` / `SQLite3::Statement`, validated
against the C-ext `sqlite3` gem on every platform that has it:

- **Database** — `Open` / `New` (`.new` / `.open`), `Close` (idempotent),
  `:memory:` and real file databases, `Path`, `Closed`.
- **Query** — `Execute` (positional rows), `ExecuteBlock` (the block form),
  `ExecuteHash` (`results_as_hash`), `Execute2` (column-name header row),
  `ExecuteBatch`, `Query` (a stepping `Statement`), `GetFirstRow`,
  `GetFirstValue`.
- **Prepared statements** — `Prepare` → `Statement` with `BindParam` /
  `BindParams`, `Execute` / `ExecuteHash`, `Step` / `Next`, `Columns`, `Types`,
  `Reset`, `ClearBindings`, `Close`.
- **Parameters** — positional `?`, indexed `?NNN`, and named `:name` / `$name` /
  `@name`, mixing positional and named binds.
- **Transactions** — `Begin(mode)` (`Deferred` / `Immediate` / `Exclusive`),
  `Commit`, `Rollback`, `InTransaction`, and the block form `Transaction` that
  commits on success and rolls back on error or panic.
- **Introspection** — `LastInsertRowID`, `Changes`, `TotalChanges`,
  `BusyTimeout`, `SetResultsAsHash` / `SetTypeTranslation`.
- **Exceptions** — the full `SQLite3::Exception` hierarchy
  (`SQLException`, `BusyException`, `ConstraintException`, …) mapped from SQLite
  result codes, with `ResultCode` / `PrimaryCode` (the gem's `#code`).

CGO-free, **100% test coverage**, `gofmt` + `go vet` clean, and green across the
six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le, s390x) — the
whole stack, including the transpiled SQLite engine, builds and runs pure-Go.

## Install

```sh
go get github.com/go-ruby-sqlite3/sqlite3
```

## Usage

```go
package main

import (
	"database/sql"
	"fmt"

	"github.com/go-ruby-sqlite3/sqlite3"
)

func main() {
	db, _ := sqlite3.Open(":memory:") // SQLite3::Database.new(":memory:")
	defer db.Close()

	db.Execute("CREATE TABLE hosts (id INTEGER PRIMARY KEY, name TEXT)", nil)
	db.Execute("INSERT INTO hosts (name) VALUES (?)", []sqlite3.Value{"web"})
	db.Execute("INSERT INTO hosts (name) VALUES (:n)",
		[]sqlite3.Value{sql.Named("n", "db")}) // named parameter

	rows, _ := db.Execute("SELECT id, name FROM hosts ORDER BY id", nil)
	for _, r := range rows {
		fmt.Println(r[0], r[1]) // 1 web / 2 db  (int64, string)
	}

	// Prepared statement, stepped like SQLite3::Statement.
	st, _ := db.Prepare("SELECT name FROM hosts WHERE id > ?")
	defer st.Close()
	st.BindParam(1, 0)
	for {
		row, ok, _ := st.Next()
		if !ok {
			break
		}
		fmt.Println(row[0])
	}
}
```

## Type mapping

Values crossing the boundary use the gem's SQLite↔Ruby coercions:

| SQLite    | Go (this package)  | Ruby (host binding)      |
| --------- | ------------------ | ------------------------ |
| `INTEGER` | `int64`            | `Integer`                |
| `REAL`    | `float64`          | `Float`                  |
| `TEXT`    | `string`           | `String` (UTF-8)         |
| `BLOB`    | `[]byte`           | `String` (ASCII-8BIT)    |
| `NULL`    | `nil`              | `nil`                    |

On bind, `int` / `int32` / `float32` / `bool` are normalised to the SQLite
storage classes above, and a `[]byte` binds a BLOB.

## Exceptions

Errors are `*sqlite3.Error`, carrying the SQLite result code and the mapped Ruby
class the host should raise — the same `status2klass` table the C ext uses:

```go
_, err := db.Execute("INSERT INTO t VALUES (1)", nil) // duplicate PK
var e *sqlite3.Error
if errors.As(err, &e) {
	fmt.Println(e.Class)        // SQLite3::ConstraintException
	fmt.Println(e.ResultCode()) // 1555 (SQLITE_CONSTRAINT_PRIMARYKEY)
	fmt.Println(e.PrimaryCode())// 19   (SQLITE_CONSTRAINT)
}
```

`SQLITE_ERROR → SQLException`, `SQLITE_BUSY → BusyException`,
`SQLITE_CONSTRAINT → ConstraintException`, `SQLITE_READONLY → ReadOnlyException`,
… and every other code down to `NotADatabaseException`, with unknown codes
falling back to the base `SQLite3::Exception`.

## Backend & architectures

The engine is `modernc.org/sqlite` — real SQLite, transpiled to Go, **no cgo**.
Every arch below builds and tests with `CGO_ENABLED=0`:

| arch    | CGO=0 build | notes                          |
| ------- | ----------- | ------------------------------ |
| amd64   | ✅          | native CI lane                 |
| arm64   | ✅          | native CI lane                 |
| riscv64 | ✅          | qemu-user CI lane              |
| loong64 | ✅          | qemu-user CI lane              |
| ppc64le | ✅          | qemu-user CI lane (big set)    |
| s390x   | ✅          | qemu-user CI lane (big-endian) |

The `-race` host lane keeps the default toolchain (cgo on for the race detector);
the backend is pure-Go either way.

## Tests & coverage

The suite pairs deterministic, gem-free tests (which alone hold coverage at
100%, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential oracle** versus the C-ext `sqlite3` gem (gated on
`RUBY_VERSION >= "4.0"` and the gem being installed): identical SQL scripts —
CREATE / INSERT / SELECT, aggregates, expressions, NULLs, and error-provoking
statements — run through both engines, and the rendered rows and raised result
codes are compared. The oracle scripts `$stdout.binmode` so Windows text-mode
never pollutes the bytes, and skip themselves where the gem or a 4.0+ ruby is
absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-sqlite3/sqlite3 authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
