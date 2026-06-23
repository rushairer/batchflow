# Quality and Release Readiness

This page describes the current quality baseline for BatchFlow v2.

## Current Assessment

BatchFlow is ready for a v2 release candidate and can be promoted to a formal v2 release after the release workflow completes successfully on GitHub Actions.

The current local validation and Docker stress results support release readiness:

- Core unit tests pass.
- Core race tests pass.
- Static checks pass.
- Security scanning must pass with a patched Go 1.24+ toolchain.
- Documentation consistency checks pass.
- MySQL, PostgreSQL, and Redis Docker stress tests pass with 100% data integrity.
- PostgreSQL/MySQL update and replace duplicate-key semantics are covered by unit tests and real database stress tests.

## Latest Local Validation

The latest local release-readiness pass included:

```bash
go test ./...
go vet ./...
make lint
make docs-check
go test -race .
```

All commands passed.

## Latest Docker Stress Result

The latest MySQL, PostgreSQL, and Redis Docker stress run is summarized in:

- [Stress test summary, Chinese](../reports/STRESS_TEST_SUMMARY_2026-06-23.zh-CN.md)
- [Stress test baseline](../reports/stress-baseline.md)

Summary:

| Backend | Result | Data integrity | Notes |
| --- | --- | ---: | --- |
| MySQL | Passed | 100% | Includes duplicate-key upsert semantics |
| PostgreSQL | Passed | 100% | Includes duplicate-key upsert semantics |
| Redis | Passed | 100% | Covers non-SQL batch execution path |

SQLite remains supported, but high-concurrency SQLite stress is optional and non-gating because SQLite uses a single-writer architecture.

## Release Gates

Before tagging a release, the following gates should pass:

- `go test ./...`
- `go test -race .`
- `go vet ./...`
- `go test -run '^$' ./...`
- `make cover`
- `make docs-check`
- `make security` or `govulncheck ./...`
- Docker stress tests for MySQL, PostgreSQL, and Redis

## Quality Risks

Known residual risks:

- The latest stress report is a short local run, not a long-duration production capacity benchmark.
- Full release confidence still depends on GitHub Actions completing successfully in the target repository.
- `BatchProcessor`, `OperationPreviewer`, and observability extension points are documented as experimental extension APIs in the API stability policy.
- SQLite should not be presented as a high-concurrency write backend.

## Release Recommendation

Recommended sequence:

1. Run the local release gates.
2. Push `main` and confirm CI plus security workflows pass.
3. Run or trigger the release workflow with the intended version.
4. If GitHub Actions passes, tag the release.

For a conservative rollout, publish `v2.0.0-rc.1` first. If downstream validation is clean, promote to `v2.0.0`.
