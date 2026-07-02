// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import "database/sql"

// normalizeResult maps a value scanned from the modernc backend into the
// package's canonical result set: nil, int64, float64, string, or []byte. The
// backend already returns those types for INTEGER / REAL / TEXT / BLOB / NULL,
// so this mostly passes through; the extra cases guard against a driver that
// widens or narrows (e.g. bool, int) so results stay stable.
func normalizeResult(v any) Value {
	switch n := v.(type) {
	case nil:
		return nil
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	case string:
		return n
	case []byte:
		// A copy: sql.Rows may reuse the backing array between Scan calls.
		b := make([]byte, len(n))
		copy(b, n)
		return b
	case bool:
		// SQLite has no boolean type; it stores 0/1 as INTEGER. Preserve that.
		if n {
			return int64(1)
		}
		return int64(0)
	default:
		return n
	}
}

// normalizeBind maps a single Ruby-side bind Value to the argument type the
// modernc driver accepts, applying the reverse of the type mapping: Integer ->
// int64, Float -> float64, String -> string, ASCII-8BIT String -> []byte, nil ->
// NULL. The []byte case lets callers bind a BLOB explicitly.
func normalizeBind(v Value) any {
	switch n := v.(type) {
	case nil:
		return nil
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case float32:
		return float64(n)
	case float64:
		return n
	case bool:
		if n {
			return int64(1)
		}
		return int64(0)
	case string:
		return n
	case []byte:
		return n
	case sql.NamedArg:
		// Allow callers to pass a pre-built named argument straight through.
		return n
	default:
		return n
	}
}

// normalizeBinds maps a slice of Ruby-side binds to driver arguments. A nil or
// empty slice yields a nil argument list so a parameterless statement runs
// cleanly.
func normalizeBinds(binds []Value) []any {
	if len(binds) == 0 {
		return nil
	}
	args := make([]any, len(binds))
	for i, b := range binds {
		args[i] = normalizeBind(b)
	}
	return args
}
