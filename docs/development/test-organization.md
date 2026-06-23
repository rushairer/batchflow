# Test Organization

BatchFlow keeps core package contract tests near the package and moves heavier or specialized tests into `test/` subpackages.

## Root Package Tests

Root-level `*_test.go` files should cover the public `batchflow` package contract:

- API stability and constructors
- lifecycle behavior
- request and schema behavior
- SQL generation and dry-run behavior
- retry and error classification
- metrics and observability callbacks
- executor behavior
- core concurrency behavior

These tests use `package batchflow_test` intentionally so they exercise exported APIs from the user's point of view.

## Subpackage Tests

Specialized tests should live outside the root package:

- `test/benchmark`: Go benchmarks and microbenchmark helpers.
- `test/boundary`: boundary-value tests that are useful but not part of the first-screen package contract.
- `test/stress`: large-data, memory-pressure, and long-running local stress tests.
- `test/integration`: Docker-backed integration and stress harness.
- `test/sqlite`: SQLite-specific tools and non-gating experiments.

## Coverage Rules

`make cover` measures coverage for the production packages and may execute focused `test/*` packages that are useful for coverage.

Default coverage excludes:

- examples
- Docker integration harness
- long-running stress packages
- SQLite tool binaries

Long-running or environment-dependent tests should remain runnable through explicit commands rather than default PR checks.

Large-data stress tests require an explicit opt-in:

```bash
BATCHFLOW_STRESS_TESTS=1 go test ./test/stress -run LargeData
```
