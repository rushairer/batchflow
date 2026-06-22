package batchflow_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mysqlerr "github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/rushairer/batchflow/v2"
)

func TestClassifyErrorReasons(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
		reason    string
	}{
		{
			name:      "context canceled",
			err:       context.Canceled,
			retryable: false,
			reason:    batchflow.ErrorReasonContextCanceled,
		},
		{
			name:      "context deadline",
			err:       context.DeadlineExceeded,
			retryable: false,
			reason:    batchflow.ErrorReasonContextDeadline,
		},
		{
			name:      "duplicate key",
			err:       errors.New("ERROR: duplicate key value violates unique constraint"),
			retryable: false,
			reason:    batchflow.ErrorReasonDuplicateKey,
		},
		{
			name:      "deadlock",
			err:       errors.New("deadlock detected"),
			retryable: true,
			reason:    batchflow.ErrorReasonDeadlock,
		},
		{
			name:      "lock timeout",
			err:       errors.New("Lock wait timeout exceeded; try restarting transaction"),
			retryable: true,
			reason:    batchflow.ErrorReasonLockTimeout,
		},
		{
			name:      "timeout",
			err:       errors.New("i/o timeout"),
			retryable: true,
			reason:    batchflow.ErrorReasonTimeout,
		},
		{
			name:      "connection",
			err:       errors.New("connection reset by peer"),
			retryable: true,
			reason:    batchflow.ErrorReasonConnection,
		},
		{
			name:      "io",
			err:       errors.New("unexpected EOF"),
			retryable: true,
			reason:    batchflow.ErrorReasonIO,
		},
		{
			name:      "syntax",
			err:       errors.New("syntax error near VALUES"),
			retryable: false,
			reason:    batchflow.ErrorReasonSyntax,
		},
		{
			name:      "other",
			err:       errors.New("validation failed"),
			retryable: false,
			reason:    batchflow.ErrorReasonNonRetryable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable, reason := batchflow.ClassifyError(tt.err)
			if retryable != tt.retryable || reason != tt.reason {
				t.Fatalf("ClassifyError() = (%v, %q), want (%v, %q)", retryable, reason, tt.retryable, tt.reason)
			}
		})
	}
}

func TestClassifyErrorUnwrapsBatchAndSQLError(t *testing.T) {
	err := &batchflow.BatchError{
		Stage:   batchflow.BatchStageExecute,
		Backend: batchflow.BackendSQL,
		Cause: &batchflow.SQLError{
			Stage: batchflow.SQLStageExecute,
			Cause: errors.New("deadlock detected"),
		},
	}

	retryable, reason := batchflow.ClassifyError(err)
	if !retryable || reason != batchflow.ErrorReasonDeadlock {
		t.Fatalf("ClassifyError() = (%v, %q), want retryable deadlock", retryable, reason)
	}
}

func TestClassifyErrorMySQLDriverCodes(t *testing.T) {
	tests := []struct {
		name      string
		number    uint16
		retryable bool
		reason    string
	}{
		{name: "duplicate", number: 1062, retryable: false, reason: batchflow.ErrorReasonDuplicateKey},
		{name: "deadlock", number: 1213, retryable: true, reason: batchflow.ErrorReasonDeadlock},
		{name: "lock timeout", number: 1205, retryable: true, reason: batchflow.ErrorReasonLockTimeout},
		{name: "query timeout", number: 3024, retryable: true, reason: batchflow.ErrorReasonTimeout},
		{name: "too many connections", number: 1040, retryable: true, reason: batchflow.ErrorReasonConnection},
		{name: "server gone", number: 2006, retryable: true, reason: batchflow.ErrorReasonConnection},
		{name: "lost connection", number: 2013, retryable: true, reason: batchflow.ErrorReasonConnection},
		{name: "syntax", number: 1064, retryable: false, reason: batchflow.ErrorReasonSyntax},
		{name: "other", number: 1146, retryable: false, reason: batchflow.ErrorReasonNonRetryable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped mysql error: %w", &mysqlerr.MySQLError{
				Number:  tt.number,
				Message: "driver error",
			})
			retryable, reason := batchflow.ClassifyError(err)
			if retryable != tt.retryable || reason != tt.reason {
				t.Fatalf("ClassifyError() = (%v, %q), want (%v, %q)", retryable, reason, tt.retryable, tt.reason)
			}
		})
	}
}

func TestClassifyErrorPostgreSQLDriverCodes(t *testing.T) {
	tests := []struct {
		name      string
		code      pq.ErrorCode
		retryable bool
		reason    string
	}{
		{name: "duplicate", code: "23505", retryable: false, reason: batchflow.ErrorReasonDuplicateKey},
		{name: "deadlock", code: "40P01", retryable: true, reason: batchflow.ErrorReasonDeadlock},
		{name: "lock unavailable", code: "55P03", retryable: true, reason: batchflow.ErrorReasonLockTimeout},
		{name: "query canceled", code: "57014", retryable: true, reason: batchflow.ErrorReasonTimeout},
		{name: "connection class", code: "08006", retryable: true, reason: batchflow.ErrorReasonConnection},
		{name: "too many connections", code: "53300", retryable: true, reason: batchflow.ErrorReasonConnection},
		{name: "cannot connect now", code: "57P03", retryable: true, reason: batchflow.ErrorReasonConnection},
		{name: "syntax", code: "42601", retryable: false, reason: batchflow.ErrorReasonSyntax},
		{name: "other", code: "42P01", retryable: false, reason: batchflow.ErrorReasonNonRetryable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped postgres error: %w", &pq.Error{
				Code:    tt.code,
				Message: "driver error",
			})
			retryable, reason := batchflow.ClassifyError(err)
			if retryable != tt.retryable || reason != tt.reason {
				t.Fatalf("ClassifyError() = (%v, %q), want (%v, %q)", retryable, reason, tt.retryable, tt.reason)
			}
		})
	}
}

func TestClassifyErrorRedisErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
		reason    string
	}{
		{name: "nil", err: redis.Nil, retryable: false, reason: batchflow.ErrorReasonNonRetryable},
		{name: "tx failed", err: redis.TxFailedErr, retryable: true, reason: batchflow.ErrorReasonDeadlock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable, reason := batchflow.ClassifyError(fmt.Errorf("wrapped redis error: %w", tt.err))
			if retryable != tt.retryable || reason != tt.reason {
				t.Fatalf("ClassifyError() = (%v, %q), want (%v, %q)", retryable, reason, tt.retryable, tt.reason)
			}
		})
	}
}

func TestClassifyErrorUnwrapsStructuredSQLErrorCause(t *testing.T) {
	err := &batchflow.BatchError{
		Stage:   batchflow.BatchStageExecute,
		Backend: batchflow.BackendSQL,
		Cause: &batchflow.SQLError{
			Stage: batchflow.SQLStageExecute,
			Cause: &pq.Error{
				Code:    "23505",
				Message: "unique violation",
			},
		},
	}

	retryable, reason := batchflow.ClassifyError(err)
	if retryable || reason != batchflow.ErrorReasonDuplicateKey {
		t.Fatalf("ClassifyError() = (%v, %q), want duplicate_key", retryable, reason)
	}
}

func TestRegisterErrorClassifier(t *testing.T) {
	sentinel := errors.New("backend throttled")
	unregister := batchflow.RegisterErrorClassifier(batchflow.ErrorClassifierFunc(func(err error) (bool, string, bool) {
		if errors.Is(err, sentinel) {
			return true, batchflow.ErrorReasonTimeout, true
		}
		return false, "", false
	}))

	retryable, reason := batchflow.ClassifyError(fmt.Errorf("wrapped: %w", sentinel))
	if !retryable || reason != batchflow.ErrorReasonTimeout {
		t.Fatalf("ClassifyError() = (%v, %q), want timeout", retryable, reason)
	}

	unregister()
	retryable, reason = batchflow.ClassifyError(fmt.Errorf("wrapped: %w", sentinel))
	if retryable || reason != batchflow.ErrorReasonNonRetryable {
		t.Fatalf("after unregister ClassifyError() = (%v, %q), want non_retryable", retryable, reason)
	}
}

func TestRegisterErrorClassifierEmptyReasonFallsBackToNonRetryable(t *testing.T) {
	sentinel := errors.New("classified with empty reason")
	unregister := batchflow.RegisterErrorClassifier(batchflow.ErrorClassifierFunc(func(err error) (bool, string, bool) {
		return errors.Is(err, sentinel), "", errors.Is(err, sentinel)
	}))
	defer unregister()

	retryable, reason := batchflow.ClassifyError(sentinel)
	if !retryable || reason != batchflow.ErrorReasonNonRetryable {
		t.Fatalf("ClassifyError() = (%v, %q), want retryable non_retryable", retryable, reason)
	}
}
