// Copyright (c) the go-ruby-sqlite3/sqlite3 authors
//
// SPDX-License-Identifier: BSD-3-Clause

package sqlite3

import (
	"errors"

	mc "modernc.org/sqlite"
)

// SQLite primary result codes (a subset — the ones the gem's status2klass maps
// to a distinct exception). Extended codes (e.g. SQLITE_CONSTRAINT_PRIMARYKEY,
// 1555) reduce to their primary code via code & 0xff. These mirror the values
// in sqlite3.h and modernc.org/sqlite/lib.
const (
	codeOK         = 0
	codeError      = 1
	codeInternal   = 2
	codePerm       = 3
	codeAbort      = 4
	codeBusy       = 5
	codeLocked     = 6
	codeNoMem      = 7
	codeReadOnly   = 8
	codeInterrupt  = 9
	codeIOErr      = 10
	codeCorrupt    = 11
	codeNotFound   = 12
	codeFull       = 13
	codeCantOpen   = 14
	codeProtocol   = 15
	codeEmpty      = 16
	codeSchema     = 17
	codeTooBig     = 18
	codeConstraint = 19
	codeMismatch   = 20
	codeMisuse     = 21
	codeNoLFS      = 22
	codeAuth       = 23
	codeFormat     = 24
	codeRange      = 25
	codeNotADB     = 26
)

// ExceptionClass is the Ruby SQLite3::Exception subclass a result code maps to.
// The rbgo binding raises the matching Ruby class; from Go it identifies the
// error category without string matching. Every Error carries one.
type ExceptionClass string

// The SQLite3::Exception hierarchy, as raised by the gem. Names match the Ruby
// class names exactly so the rbgo binding is a direct lookup.
const (
	ExcException              ExceptionClass = "SQLite3::Exception"
	ExcSQLException           ExceptionClass = "SQLite3::SQLException"
	ExcInternalException      ExceptionClass = "SQLite3::InternalException"
	ExcPermissionException    ExceptionClass = "SQLite3::PermissionException"
	ExcAbortException         ExceptionClass = "SQLite3::AbortException"
	ExcBusyException          ExceptionClass = "SQLite3::BusyException"
	ExcLockedException        ExceptionClass = "SQLite3::LockedException"
	ExcMemoryException        ExceptionClass = "SQLite3::MemoryException"
	ExcReadOnlyException      ExceptionClass = "SQLite3::ReadOnlyException"
	ExcInterruptException     ExceptionClass = "SQLite3::InterruptException"
	ExcIOException            ExceptionClass = "SQLite3::IOException"
	ExcCorruptException       ExceptionClass = "SQLite3::CorruptException"
	ExcNotFoundException      ExceptionClass = "SQLite3::NotFoundException"
	ExcFullException          ExceptionClass = "SQLite3::FullException"
	ExcCantOpenException      ExceptionClass = "SQLite3::CantOpenException"
	ExcProtocolException      ExceptionClass = "SQLite3::ProtocolException"
	ExcEmptyException         ExceptionClass = "SQLite3::EmptyException"
	ExcSchemaChangedException ExceptionClass = "SQLite3::SchemaChangedException"
	ExcTooBigException        ExceptionClass = "SQLite3::TooBigException"
	ExcConstraintException    ExceptionClass = "SQLite3::ConstraintException"
	ExcMismatchException      ExceptionClass = "SQLite3::MismatchException"
	ExcMisuseException        ExceptionClass = "SQLite3::MisuseException"
	ExcUnsupportedException   ExceptionClass = "SQLite3::UnsupportedException"
	ExcAuthorizationException ExceptionClass = "SQLite3::AuthorizationException"
	ExcFormatException        ExceptionClass = "SQLite3::FormatException"
	ExcRangeException         ExceptionClass = "SQLite3::RangeException"
	ExcNotADatabaseException  ExceptionClass = "SQLite3::NotADatabaseException"
)

// classForCode maps a primary SQLite result code to its exception class,
// mirroring the gem's ext/sqlite3/exception.c status2klass switch. Unknown
// codes fall back to the base SQLite3::Exception.
func classForCode(primary int) ExceptionClass {
	switch primary {
	case codeError:
		return ExcSQLException
	case codeInternal:
		return ExcInternalException
	case codePerm:
		return ExcPermissionException
	case codeAbort:
		return ExcAbortException
	case codeBusy:
		return ExcBusyException
	case codeLocked:
		return ExcLockedException
	case codeNoMem:
		return ExcMemoryException
	case codeReadOnly:
		return ExcReadOnlyException
	case codeInterrupt:
		return ExcInterruptException
	case codeIOErr:
		return ExcIOException
	case codeCorrupt:
		return ExcCorruptException
	case codeNotFound:
		return ExcNotFoundException
	case codeFull:
		return ExcFullException
	case codeCantOpen:
		return ExcCantOpenException
	case codeProtocol:
		return ExcProtocolException
	case codeEmpty:
		return ExcEmptyException
	case codeSchema:
		return ExcSchemaChangedException
	case codeTooBig:
		return ExcTooBigException
	case codeConstraint:
		return ExcConstraintException
	case codeMismatch:
		return ExcMismatchException
	case codeMisuse:
		return ExcMisuseException
	case codeNoLFS:
		return ExcUnsupportedException
	case codeAuth:
		return ExcAuthorizationException
	case codeFormat:
		return ExcFormatException
	case codeRange:
		return ExcRangeException
	case codeNotADB:
		return ExcNotADatabaseException
	default:
		return ExcException
	}
}

// Error is a database error carrying the SQLite result code and the mapped
// SQLite3::Exception subclass. It corresponds to a raised SQLite3::Exception in
// the gem; ResultCode is the value exposed as Ruby's exception#code /
// #result_code, and Class names the Ruby class the rbgo binding raises.
type Error struct {
	// Code is the full SQLite result code, including any extended bits (e.g.
	// 1555 for SQLITE_CONSTRAINT_PRIMARYKEY). Ruby's #code returns this.
	Code int
	// Class is the SQLite3::Exception subclass this code maps to.
	Class ExceptionClass
	// Message is the human-readable SQLite message.
	Message string
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// ResultCode returns the SQLite result code (gem: SQLite3::Exception#result_code
// / #code). It includes extended-code bits when SQLite reported them.
func (e *Error) ResultCode() int { return e.Code }

// PrimaryCode returns the primary (low 8 bits) result code, dropping any
// extended-code bits.
func (e *Error) PrimaryCode() int { return e.Code & 0xff }

// wrapError converts an error returned by the modernc backend into a *sqlite3.Error
// with the gem-faithful exception class. A nil error passes through as nil; an
// error that does not originate from the SQLite engine is wrapped as the base
// SQLite3::Exception with the generic error code.
func wrapError(err error) error {
	if err == nil {
		return nil
	}
	var me *mc.Error
	if errors.As(err, &me) {
		code := me.Code()
		return &Error{
			Code:    code,
			Class:   classForCode(code & 0xff),
			Message: me.Error(),
		}
	}
	// Non-engine errors (bad connection, context, driver misuse) surface as the
	// base exception so callers still get a *sqlite3.Error to inspect.
	return &Error{Code: codeError, Class: ExcSQLException, Message: err.Error()}
}
