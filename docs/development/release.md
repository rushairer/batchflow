# Release Process

BatchFlow releases follow semantic versioning.

## Versioning

- Patch release: bug fixes, documentation fixes, test improvements, non-breaking diagnostics improvements.
- Minor release: new backward-compatible APIs, new metrics, new backend support, additive configuration.
- Major release: breaking public API, metrics, lifecycle, SQL semantics, or default retry behavior changes.

Pre-release versions use standard suffixes such as `v1.2.0-beta.1`.

## Release Checklist

Before tagging:

```bash
go test ./...
go test -race .
go vet ./...
go test -run '^$' ./...
make cover
make docs-check
make security
```

For SQL, Redis, retry, observability, or performance-sensitive changes, run the relevant Docker integration tests:

```bash
make docker-postgres-test
make docker-mysql-test
make docker-redis-test
```

SQLite high-concurrency stress results must be reviewed but may be non-gating when failures match documented SQLite write-concurrency limits.

## Changelog Rules

- Keep user-facing changes in [changelog.md](changelog.md).
- Move completed `Unreleased` entries into the release section before tagging.
- Include migration notes for breaking changes.
- Mention metrics label changes explicitly.
- Mention SQL write semantics changes explicitly.

## Tagging

1. Confirm `main` is green.
2. Confirm the changelog and release notes are ready.
3. Create and push the tag:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

The release workflow validates tests, coverage, integration tests, changelog, and release artifacts.

## Compatibility Review

Before every minor or major release, review:

- [API stability policy](api-stability.md)
- [Metrics spec](../guides/metrics-spec.md)
- [Error classification](../guides/error-classification.md)
- SQL update/replace behavior
- Observability redaction behavior
