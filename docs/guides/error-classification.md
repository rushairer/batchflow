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

For reusable backend integrations, register a classifier once during initialization:

```go
unregister := batchflow.RegisterErrorClassifier(batchflow.ErrorClassifierFunc(
	func(err error) (bool, string, bool) {
		if errors.Is(err, myBackendTemporaryError) {
			return true, batchflow.ErrorReasonTimeout, true
		}
		return false, "", false
	},
))
defer unregister()
```

Registered classifiers run after built-in structured MySQL/PostgreSQL recognition and before the string fallback. This keeps database driver codes stable while allowing custom Redis, HTTP, queue, or storage backends to participate in the same retry and metrics reason dictionary.

## Label Discipline

Reason labels must stay low-cardinality. Do not use raw error strings, SQL text, request IDs, user IDs, Redis keys, HTTP paths with IDs, or backend-specific detailed codes as Prometheus labels.

Use detailed values in redacted structured logs or error causes, not metrics labels.

## Database-Specific Structured Codes

BatchFlow classifies structured driver errors before falling back to normalized error text.

### PostgreSQL SQLSTATE

| SQLSTATE | Reason |
|---|---|
| `23505` | `duplicate_key` |
| `40P01` | `deadlock` |
| `55P03` | `lock_timeout` |
| `57014` | `timeout` |
| `42601` | `syntax` |
| class `08`, `53300`, `57P01`, `57P02`, `57P03` | `connection` |

### MySQL Error Numbers

| Number | Reason |
|---:|---|
| `1062` | `duplicate_key` |
| `1213` | `deadlock` |
| `1205` | `lock_timeout` |
| `3024`, `1317` | `timeout` |
| `1040`, `1042`, `1043`, `2002`, `2003`, `2006`, `2013` | `connection` |
| `1064` | `syntax` |

Unknown structured database codes fall back to `non_retryable`.

Adding new structured recognition must preserve the reason dictionary above unless a new low-cardinality reason is documented first.
