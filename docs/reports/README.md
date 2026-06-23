# Reports

This directory keeps current, reviewable performance and stress-test documentation.

Historical SQLite-only analysis documents were removed because they described an old troubleshooting phase and no longer represent the current BatchFlow performance contract. SQLite remains supported, but high-concurrency SQLite stress is documented as optional and non-gating in the current testing guide.

## Current Reports

- [Stress test baseline](stress-baseline.md): how to run and interpret reproducible stress reports.
- [2026-06-23 Chinese stress summary](STRESS_TEST_SUMMARY_2026-06-23.zh-CN.md): human-readable summary for the latest MySQL, PostgreSQL, and Redis Docker stress run.

## Generated Artifacts

Generated stress artifacts are written to:

```text
reports/stress/<timestamp>/
```

That directory is ignored by Git. Keep generated JSON/HTML/Markdown artifacts local unless a specific report is intentionally promoted into `docs/reports`.

## Rules

- Keep only reports that are still useful for release decisions or project onboarding.
- Prefer short summaries with clear parameters, environment, correctness result, and backend comparison.
- Link to generated raw reports only when they are intentionally preserved.
- Remove stale reports when their assumptions no longer match the current architecture or test harness.
