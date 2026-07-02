// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"errors"
	"testing"
)

func TestClassForCodeAll(t *testing.T) {
	cases := map[int]ExceptionClass{
		codeOK:         ExcException, // default fall-through
		codeError:      ExcSQLException,
		codeInternal:   ExcInternalException,
		codePerm:       ExcPermissionException,
		codeAbort:      ExcAbortException,
		codeBusy:       ExcBusyException,
		codeLocked:     ExcLockedException,
		codeNoMem:      ExcMemoryException,
		codeReadOnly:   ExcReadOnlyException,
		codeInterrupt:  ExcInterruptException,
		codeIOErr:      ExcIOException,
		codeCorrupt:    ExcCorruptException,
		codeNotFound:   ExcNotFoundException,
		codeFull:       ExcFullException,
		codeCantOpen:   ExcCantOpenException,
		codeProtocol:   ExcProtocolException,
		codeEmpty:      ExcEmptyException,
		codeSchema:     ExcSchemaChangedException,
		codeTooBig:     ExcTooBigException,
		codeConstraint: ExcConstraintException,
		codeMismatch:   ExcMismatchException,
		codeMisuse:     ExcMisuseException,
		codeNoLFS:      ExcUnsupportedException,
		codeAuth:       ExcAuthorizationException,
		codeFormat:     ExcFormatException,
		codeRange:      ExcRangeException,
		codeNotADB:     ExcNotADatabaseException,
		999:            ExcException, // unknown -> base
	}
	for code, want := range cases {
		if got := classForCode(code); got != want {
			t.Errorf("classForCode(%d) = %s, want %s", code, got, want)
		}
	}
}

func TestErrorInterface(t *testing.T) {
	e := &Error{Code: 1555, Class: ExcConstraintException, Message: "UNIQUE failed"}
	if e.Error() != "UNIQUE failed" {
		t.Errorf("Error() = %q", e.Error())
	}
	if e.ResultCode() != 1555 {
		t.Errorf("ResultCode() = %d, want 1555", e.ResultCode())
	}
	if e.PrimaryCode() != 19 {
		t.Errorf("PrimaryCode() = %d, want 19", e.PrimaryCode())
	}
}

func TestWrapErrorNil(t *testing.T) {
	if err := wrapError(nil); err != nil {
		t.Errorf("wrapError(nil) = %v, want nil", err)
	}
}

func TestWrapErrorEngineError(t *testing.T) {
	// A real constraint violation from the engine keeps its extended code and
	// maps to the constraint exception.
	db := openMem(t)
	mustExec(t, db, "CREATE TABLE t(x INTEGER PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO t VALUES(1)")
	_, err := db.Execute("INSERT INTO t VALUES(1)", nil)
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("want *Error, got %T", err)
	}
	if se.Class != ExcConstraintException {
		t.Errorf("class = %s, want ConstraintException", se.Class)
	}
	if se.PrimaryCode() != codeConstraint {
		t.Errorf("primary code = %d, want %d", se.PrimaryCode(), codeConstraint)
	}
	if se.ResultCode() != 1555 { // SQLITE_CONSTRAINT_PRIMARYKEY
		t.Errorf("result code = %d, want 1555", se.ResultCode())
	}
}

func TestWrapErrorSyntaxIsSQLException(t *testing.T) {
	db := openMem(t)
	_, err := db.Execute("SELECT bad syntax here", nil)
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("want *Error, got %T: %v", err, err)
	}
	if se.Class != ExcSQLException {
		t.Errorf("syntax error class = %s, want SQLException", se.Class)
	}
}

func TestWrapErrorNonEngine(t *testing.T) {
	// A non-engine error wraps as the base SQLException with codeError.
	err := wrapError(errors.New("plain go error"))
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("want *Error, got %T", err)
	}
	if se.Class != ExcSQLException || se.Code != codeError {
		t.Errorf("non-engine wrap = %+v", se)
	}
	if se.Error() != "plain go error" {
		t.Errorf("message = %q", se.Error())
	}
}
