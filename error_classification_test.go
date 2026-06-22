package batchflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rushairer/batchflow"
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
