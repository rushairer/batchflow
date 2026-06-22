# API Stability Policy

BatchFlow follows semantic versioning for public Go APIs, metrics contracts, and documented operational behavior.

## Stability Levels

| Level | Meaning | Change Policy |
|---|---|---|
| Stable | Recommended for production use | No breaking changes within a major version |
| Compatibility | Kept for existing users or old diagnostics paths | May be deprecated before removal in a future major version |
| Experimental | Useful but still being validated | May change in minor releases with release notes |
| Internal | Not part of the public contract | May change at any time |

## Stable Public Contract

- `BatchFlow`, `NewBatchFlow`, `Submit`, `Close`, `Wait`, `Done`, `ErrorChan`
- `PipelineConfig`
- `SchemaInterface`, `Schema`, `SQLSchema`, `NewSchema`, `NewSQLSchema`
- `Request` and typed setters
- SQL conflict configuration:
  - `SQLOperationConfig`
  - `ConflictStrategy`
  - `WithConflictColumns`
  - `WithUpdateColumns`
  - `WithDeduplicateByConflictColumns`
- Recommended constructors:
  - `NewMySQLBatchFlow`
  - `NewPostgreSQLBatchFlow`
  - `NewSQLiteBatchFlow`
  - `NewRedisBatchFlow`
- `BatchExecutor`
- `RetryConfig`
- `ClassifyError`, `RegisterErrorClassifier`, `ErrorClassifier`, and `ErrorReason*` constants
- `GenerateSQLPreview`, `SQLPreview`, `SQLError`, `SQLStage`

## Stable Metrics Contract

The metrics contract is defined in [Metrics 规格](../guides/metrics-spec.md). Metric names, label meanings, and low-cardinality reason values are treated as stable once documented there.

Adding a new metric or label is allowed in a minor release. Renaming or removing a metric or label requires a major release unless the field was explicitly experimental.

## Compatibility APIs

- `SQLMetricsReporter` is retained for SQL-specific diagnostics and compatibility with existing Prometheus integrations. New generic backends should prefer `OperationMetricsReporter`.
- `SQLBatchProcessor.ExecuteOperations` accepts `SQLPreview` as the first operation for compatibility with older diagnostics/tests. The normal generation path remains SQL string plus args.

## Experimental Extension Points

- `BatchProcessor`
- `OperationPreviewer`
- `Observer`, `Sampler`, `Redactor`, `ObservabilityConfig`
- `OperationMetricsReporter`

These APIs are intended for extension and are actively used, but they may receive additive changes as more non-SQL backends are validated. Breaking changes still require migration notes.

## Breaking Change Rules

A change is breaking if it:

- removes or renames an exported stable symbol
- changes stable method signatures
- changes SQL update/replace semantics without an opt-in
- changes documented lifecycle behavior
- renames or removes stable metrics or labels
- changes default retryability for an existing `ErrorReason*` without release notes

Breaking changes require:

- major version bump
- migration guide
- release notes with before/after examples
- tests covering the compatibility boundary where practical
