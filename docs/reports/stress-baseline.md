# Stress Test Baseline

This page defines how BatchFlow stress reports should be produced and interpreted.

## Default Command

```bash
./scripts/run_stress_report.sh
```

Default backends:

- MySQL
- PostgreSQL
- Redis

SQLite is optional and non-gating:

```bash
./scripts/run_stress_report.sh --include-sqlite
```

## Default Parameters

| Parameter | Default |
| --- | --- |
| `TEST_DURATION` | `120s` |
| `CONCURRENT_WORKERS` | `10` |
| `RECORDS_PER_WORKER` | `5000` |
| `BATCH_SIZE` | `500` |
| `BUFFER_SIZE` | `10000` |
| `FLUSH_INTERVAL` | `50ms` |

All values can be overridden through environment variables:

```bash
TEST_DURATION=300s CONCURRENT_WORKERS=20 ./scripts/run_stress_report.sh
```

## Output

The script writes generated artifacts under:

```text
reports/stress/<timestamp>/
```

That directory is intentionally ignored by Git. It contains:

- `stress-report.md`: consolidated environment, parameter, and backend summary.
- `raw/`: copied JSON/HTML reports from `test/reports`.

## Interpretation Rules

- MySQL, PostgreSQL, and Redis are gating stress backends.
- SQLite is non-gating for high-concurrency write stress.
- RPS is meaningful only when data integrity is `100%` and `RPS Valid` is `true`.
- Errors, data-integrity loss, or invalid RPS require inspecting the raw JSON/HTML report.
- Compare runs using the same commit, backend version, machine class, and parameter set.

## Release Baseline Checklist

- [ ] `go test ./...` passed.
- [ ] `go test -race .` passed.
- [ ] `make docs-check` passed.
- [ ] `./scripts/run_stress_report.sh` passed for MySQL, PostgreSQL, and Redis.
- [ ] The generated report records commit SHA, Go version, Docker version, OS, CPU, memory, and stress parameters.
- [ ] PostgreSQL/MySQL duplicate-key update/replace cases report 100% integrity.
