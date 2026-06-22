# Contributing to BatchFlow

Thanks for contributing. This top-level guide points to the maintained development docs and defines the minimum bar for changes.

## Start Here

- Architecture: [docs/development/architecture.md](docs/development/architecture.md)
- Development guide: [docs/development/contributing.md](docs/development/contributing.md)
- Testing guide: [docs/guides/testing.md](docs/guides/testing.md)
- Metrics contract: [docs/guides/metrics-spec.md](docs/guides/metrics-spec.md)
- Security reports: [SECURITY.md](SECURITY.md)

## Local Checks

Run the fast checks before opening a PR:

```bash
go test ./...
go test -race .
go vet ./...
scripts/check-doc-consistency.sh
```

For SQL, Redis, or performance-sensitive changes, also run the relevant Docker integration test from the Makefile.

## Pull Request Expectations

- Keep changes scoped.
- Update tests for behavior changes.
- Update docs for public API, metrics, SQL behavior, or operational semantics.
- Keep Prometheus labels low-cardinality.
- Do not log raw payloads, SQL args, Redis keys, HTTP bodies, credentials, tokens, or user identifiers.

## Compatibility

Public APIs should remain backward compatible within a major version. Breaking changes require explicit migration notes and release notes.
