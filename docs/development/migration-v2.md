# BatchFlow v2 Migration Guide

BatchFlow v2 moves the framework toward a backend-neutral batch processing core while keeping compatibility wrappers during the transition.

## Constructor Migration

Prefer the config-based constructor for new generic integrations:

```go
executor := batchflow.NewThrottledBatchExecutor(processor)

flow, err := batchflow.NewBatchFlowWithConfig(ctx, batchflow.BatchFlowConfig{
	Pipeline: batchflow.DefaultPipelineConfig(),
	Executor: executor,
})
if err != nil {
	return err
}
defer flow.Close()
```

Existing helpers such as `NewMySQLBatchFlow`, `NewPostgreSQLBatchFlow`, `NewSQLiteBatchFlow`, and `NewRedisBatchFlow` remain available.

## Batch Data Model

New code should use the named aliases:

```go
type Record = map[string]any
type Batch = []Record
```

Existing `[]map[string]any` implementations continue to compile because these are aliases.

## Coalescing

For non-SQL backends, configure batch-level key coalescing explicitly:

```go
config := batchflow.DefaultPipelineConfig()
config.Coalescer = batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "id")
```

SQL backends should continue to use `SQLOperationConfig.WithConflictColumns(...)`. SQL conflict strategies keep database-specific semantics and preserve SQL dry-run dedup statistics.

## Request Columns

`Request.Columns()` now returns a defensive copy. Code that intentionally mutated the returned map must switch to `Set(...)`, `SetNull(...)`, or typed setters before submission.

## Error Classification

Reusable custom backends should register structured classifiers instead of relying on string matching:

```go
unregister := batchflow.RegisterErrorClassifier(classifier)
defer unregister()
```

Custom classifiers run after built-in MySQL/PostgreSQL structured code recognition and before string fallback.
