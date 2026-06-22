package batchflow

import (
	"context"
	"errors"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

const (
	ErrorReasonUnknown         = "unknown"
	ErrorReasonContextCanceled = "context_canceled"
	ErrorReasonContextDeadline = "context_deadline"
	ErrorReasonDuplicateKey    = "duplicate_key"
	ErrorReasonDeadlock        = "deadlock"
	ErrorReasonLockTimeout     = "lock_timeout"
	ErrorReasonTimeout         = "timeout"
	ErrorReasonConnection      = "connection"
	ErrorReasonIO              = "io"
	ErrorReasonSyntax          = "syntax"
	ErrorReasonNonRetryable    = "non_retryable"
)

// ClassifyError returns a low-cardinality retry decision and reason label for logs and metrics.
// It unwraps BatchError and SQLError before classifying the underlying cause.
func ClassifyError(err error) (retryable bool, reason string) {
	if err == nil {
		return false, ErrorReasonUnknown
	}
	err = unwrapBatchCause(err)
	if errors.Is(err, context.Canceled) {
		return false, ErrorReasonContextCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false, ErrorReasonContextDeadline
	}
	if retryable, reason, ok := classifyMySQLError(err); ok {
		return retryable, reason
	}
	if retryable, reason, ok := classifyPostgreSQLError(err); ok {
		return retryable, reason
	}

	s := strings.ToLower(err.Error())
	switch {
	case containsAny(s, "duplicate key", "duplicate entry", "unique constraint", "unique violation"):
		return false, ErrorReasonDuplicateKey
	case strings.Contains(s, "deadlock"):
		return true, ErrorReasonDeadlock
	case containsAny(s, "lock wait timeout", "lock timeout", "could not obtain lock"):
		return true, ErrorReasonLockTimeout
	case strings.Contains(s, "timeout"):
		return true, ErrorReasonTimeout
	case strings.Contains(s, "connection") && containsAny(s, "refused", "reset", "closed", "failure", "unavailable"):
		return true, ErrorReasonConnection
	case strings.Contains(s, "too many connections"):
		return true, ErrorReasonConnection
	case containsAny(s, "broken pipe", "unexpected eof", "eof"):
		return true, ErrorReasonIO
	case strings.Contains(s, "syntax"):
		return false, ErrorReasonSyntax
	default:
		return false, ErrorReasonNonRetryable
	}
}

func classifyMySQLError(err error) (retryable bool, reason string, ok bool) {
	var mysqlErr *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false, "", false
	}
	switch mysqlErr.Number {
	case 1062:
		return false, ErrorReasonDuplicateKey, true
	case 1213:
		return true, ErrorReasonDeadlock, true
	case 1205:
		return true, ErrorReasonLockTimeout, true
	case 3024, 1317:
		return true, ErrorReasonTimeout, true
	case 1040, 1042, 1043, 2002, 2003, 2006, 2013:
		return true, ErrorReasonConnection, true
	case 1064:
		return false, ErrorReasonSyntax, true
	default:
		return false, ErrorReasonNonRetryable, true
	}
}

func classifyPostgreSQLError(err error) (retryable bool, reason string, ok bool) {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false, "", false
	}
	code := pqErr.SQLState()
	switch {
	case code == "23505":
		return false, ErrorReasonDuplicateKey, true
	case code == "40P01":
		return true, ErrorReasonDeadlock, true
	case code == "55P03":
		return true, ErrorReasonLockTimeout, true
	case code == "57014":
		return true, ErrorReasonTimeout, true
	case code == "42601":
		return false, ErrorReasonSyntax, true
	case strings.HasPrefix(code, "08"), code == "53300", code == "57P01", code == "57P02", code == "57P03":
		return true, ErrorReasonConnection, true
	default:
		return false, ErrorReasonNonRetryable, true
	}
}

func unwrapBatchCause(err error) error {
	var batchErr *BatchError
	if errors.As(err, &batchErr) && batchErr.Cause != nil {
		err = batchErr.Cause
	}
	var sqlErr *SQLError
	if errors.As(err, &sqlErr) && sqlErr.Cause != nil {
		err = sqlErr.Cause
	}
	return err
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
