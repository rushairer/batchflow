package batchflow

import (
	"context"
	"errors"
	"strings"
	"sync"

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

// ErrorClassifier recognizes backend-specific errors and returns a low-cardinality reason.
type ErrorClassifier interface {
	Classify(error) (retryable bool, reason string, ok bool)
}

type ErrorClassifierFunc func(error) (retryable bool, reason string, ok bool)

func (f ErrorClassifierFunc) Classify(err error) (retryable bool, reason string, ok bool) {
	if f == nil {
		return false, "", false
	}
	return f(err)
}

var errorClassifiers struct {
	sync.RWMutex
	nextID uint64
	custom []registeredErrorClassifier
}

type registeredErrorClassifier struct {
	id         uint64
	classifier ErrorClassifier
}

// RegisterErrorClassifier adds a custom classifier after built-in structured classifiers
// and before the string fallback. The returned function unregisters it.
func RegisterErrorClassifier(classifier ErrorClassifier) func() {
	if classifier == nil {
		return func() {}
	}
	errorClassifiers.Lock()
	errorClassifiers.nextID++
	id := errorClassifiers.nextID
	errorClassifiers.custom = append(errorClassifiers.custom, registeredErrorClassifier{
		id:         id,
		classifier: classifier,
	})
	errorClassifiers.Unlock()
	return func() {
		errorClassifiers.Lock()
		defer errorClassifiers.Unlock()
		for i, candidate := range errorClassifiers.custom {
			if candidate.id == id {
				errorClassifiers.custom = append(errorClassifiers.custom[:i], errorClassifiers.custom[i+1:]...)
				return
			}
		}
	}
}

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
	for _, classifier := range snapshotCustomErrorClassifiers() {
		if retryable, reason, ok := classifier.Classify(err); ok {
			if reason == "" {
				reason = ErrorReasonNonRetryable
			}
			return retryable, reason
		}
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

func snapshotCustomErrorClassifiers() []ErrorClassifier {
	errorClassifiers.RLock()
	defer errorClassifiers.RUnlock()
	if len(errorClassifiers.custom) == 0 {
		return nil
	}
	out := make([]ErrorClassifier, len(errorClassifiers.custom))
	for i, entry := range errorClassifiers.custom {
		out[i] = entry.classifier
	}
	return out
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
