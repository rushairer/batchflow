# BatchFlow Documentation

BatchFlow documentation uses English as the canonical public documentation. Chinese mirrors are available under [docs/zh-CN](zh-CN/index.md).

Recommended reading path:

1. Start with the [README](../README.md) for installation, lifecycle, and the high-level model.
2. Use [API reference](api/reference.md) and [configuration](api/configuration.md) for public contracts.
3. Read the production, monitoring, testing, and architecture guides as needed.

## Users

- [README](../README.md)
- [API reference](api/reference.md)
- [Configuration](api/configuration.md)
- [Examples](guides/examples.md)
- [Production guide](guides/production.md)
- [Testing guide](guides/testing.md)
- [Troubleshooting](guides/troubleshooting.md)
- [Error classification](guides/error-classification.md)
- [Executor capabilities](guides/executor-capabilities.md)
- [Monitoring quickstart](guides/monitoring-quickstart.md)
- [Monitoring guide](guides/monitoring.md)
- [Metrics specification](guides/metrics-spec.md)
- [Custom MetricsReporter](guides/custom-metrics-reporter.md)

## Maintainers

- [Architecture](development/architecture.md)
- [API stability policy](development/api-stability.md)
- [v2 migration guide](development/migration-v2.md)
- [Contributing](development/contributing.md)
- [Integration tests](guides/integration-tests.md)
- [go-pipeline metrics integration](guides/go-pipeline-metrics.md)
- [Quality guide](development/quality.md)
- [Release process](development/release.md)
- [Changelog](development/changelog.md)

## Documentation Rules

- `README.md`, `docs/api/reference.md`, `docs/api/configuration.md`, `docs/guides/production.md`, and `docs/guides/metrics-spec.md` are the primary public contracts.
- Examples must compile against `github.com/rushairer/batchflow/v2`.
- SQL update/replace examples must use explicit `ConflictColumns`.
- Non-SQL duplicate-key behavior must use `PipelineConfig.Coalescer`.
- `docs/reports` should contain only current, reviewable reports or baseline instructions.
