package batchflow

import (
	"context"
	"errors"
	"strings"
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
