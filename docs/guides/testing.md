# Testing Guide

## Local Checks

```bash
go test ./...
go test -run '^$' ./...
make docs-check
```

Use compile-only checks after documentation or example changes, then run the full unit suite before committing.

## CI Layers

### Pull Request and Main CI

`.github/workflows/ci.yml` runs fast, stable checks:

- `gofmt -s -l .`
- `git diff --check`
- `go vet ./...`
- `golangci-lint run`
- `scripts/check-doc-consistency.sh`
- `go test -run '^$' ./...`
- `go test ./...`
- `make cover`
- `go test -race .`

This layer catches formatting, static analysis, documentation drift, compile failures, unit regressions, coverage generation issues, and core concurrency regressions.

### Nightly and Manual Long-Running Tests

`.github/workflows/nightly.yml` covers Docker databases, long-running stress tests, and performance trend checks. MySQL, PostgreSQL, and Redis results are gating. SQLite stress is non-gating because high-concurrency write failures are a documented SQLite limitation, but the report is still useful for trend analysis.

### Release Validation

Before a release:

- Run fast CI locally or verify the latest CI run.
- Run Docker integration/stress tests for target backends.
- Review SQL update/replace behavior with `GenerateSQLPreview`.
- Run security scanning when dependencies or execution boundaries changed.
- Check generated reports for throughput, integrity, error rate, memory, and GC regressions.

## Recommended Test Layers

### Contract Tests

Cover:

- `Submit` cancellation semantics.
- `Close()` final flush behavior.
- `Wait()` and `Done()` lifecycle behavior.
- `Schema` validation and `Request.Validate()`.
- Typed setters such as `SetInt`, `SetUint64`, `SetBool`, and `SetNull`.
- Retry classification and low-cardinality error labels.

### Executor Tests

Cover:

- SQL driver generation.
- PostgreSQL/MySQL update and replace conflict-column behavior.
- Redis command generation.
- `WithConcurrencyLimit(...)`.
- `WithRetryConfig(...)`.
- Metrics callback stages.

### Integration Tests

Cover:

- MySQL, PostgreSQL, SQLite, and Redis end-to-end writes.
- Duplicate-key batches for PostgreSQL/MySQL update/replace.
- Throughput stress and data integrity.
- Prometheus/Grafana dashboard behavior when monitoring is enabled.

## Minimum Bar for New Changes

- Public API change: update unit tests and documentation.
- SQL generation change: add SQL driver tests plus at least one Docker test for the affected backend.
- Metrics semantic change: update `metrics-spec.md` and reporter tests.
- Lifecycle change: update `Close`, `Wait`, or `Done` tests.
- Example change: run `go test -run '^$' ./...`.
- Dependency, security, or logging boundary change: run `govulncheck ./...` when available and confirm sensitive data is not logged.

## Documentation Consistency

Run:

```bash
make docs-check
```

The check blocks stale import paths, old metric names, missing lifecycle contracts, missing v2 installation commands, and missing references to critical public APIs such as `GenerateSQLPreview`, `RegisterErrorClassifier`, `ConflictColumns`, and `Coalescer`.

## Docker Stress Tests

Backend-specific targets:

```bash
make docker-postgres-test
make docker-mysql-test
make docker-redis-test
```

Generate a consolidated local report:

```bash
./scripts/run_stress_report.sh
```

Default stress scope is PostgreSQL, MySQL, and Redis. Use `--include-sqlite` only when you explicitly want SQLite trend data; SQLite is not a gating high-concurrency backend.

## Integration Test Entrypoints

- [Integration test guide](integration-tests.md)
- `test/integration`
