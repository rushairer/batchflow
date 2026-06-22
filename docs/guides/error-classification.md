# Error Classification

BatchFlow uses `ClassifyError(err)` to produce low-cardinality reason labels for retry decisions, metrics, and structured logs.

## Reason Dictionary

| Reason | Default Retryable | Typical Source |
|---|---:|---|
| `context_canceled` | No | Caller or parent context canceled |
| `context_deadline` | No | Caller or parent context deadline exceeded |
| `duplicate_key` | No | Unique/primary-key violation |
| `deadlock` | Yes | Database transaction deadlock |
| `lock_timeout` | Yes | Database lock wait timeout |
| `timeout` | Yes | Transient network or backend timeout |
| `connection` | Yes | Refused, reset, closed, unavailable, or exhausted connection |
| `io` | Yes | Broken pipe or EOF |
| `syntax` | No | SQL or command syntax error |
| `non_retryable` | No | Known non-transient error without a more specific reason |
| `unknown` | No | Nil or unclassified error path |

## Usage

```go
retryable, reason := batchflow.ClassifyError(err)
```

The default `RetryConfig` classifier delegates to `ClassifyError`. If you provide a custom classifier, return the same reason strings whenever possible:

```go
Retry: batchflow.RetryConfig{
	Enabled:     true,
	MaxAttempts: 3,
	Classifier: func(err error) (bool, string) {
		if errors.Is(err, myBackendTemporaryError) {
			return true, batchflow.ErrorReasonTimeout
		}
		return batchflow.ClassifyError(err)
	},
}
```

## Label Discipline

Reason labels must stay low-cardinality. Do not use raw error strings, SQL text, request IDs, user IDs, Redis keys, HTTP paths with IDs, or backend-specific detailed codes as Prometheus labels.

Use detailed values in redacted structured logs or error causes, not metrics labels.

## Database-Specific Future Work

String matching is currently used for common cross-driver errors. Future improvements should add structured database-code classification where drivers expose it, for example:

- PostgreSQL SQLSTATE
- MySQL error numbers
- Redis typed errors

Adding structured recognition must preserve the reason dictionary above unless a new low-cardinality reason is documented first.
