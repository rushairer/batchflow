# BatchFlow 文档索引

当前建议把文档分成三层阅读：

1. 先看 [README](../README.md)，确认推荐用法和生命周期。
2. 再看 [API 参考](api/reference.md) 与 [配置说明](api/configuration.md)，确认公开契约。
3. 接着根据需要进入监控、测试和架构文档。

## 面向使用者

- [README](../README.md)
- [API 参考](api/reference.md)
- [配置说明](api/configuration.md)
- [使用示例](guides/examples.md)
- [监控快速上手](guides/monitoring-quickstart.md)
- [监控指南](guides/monitoring.md)
- [Metrics 规格](guides/metrics-spec.md)
- [自定义 MetricsReporter](guides/custom-metrics-reporter.md)
- [错误分类](guides/error-classification.md)

## 面向维护者

- [架构设计](development/architecture.md)
- [API 稳定性策略](development/api-stability.md)
- [贡献指南](development/contributing.md)
- [测试指南](guides/testing.md)
- [集成测试](guides/integration-tests.md)
- [go-pipeline 指标接入说明](guides/go-pipeline-metrics.md)
- [变更记录](development/changelog.md)

## 当前文档约定

- `README`、`docs/api/reference.md`、`docs/guides/metrics-spec.md` 是一线契约文档。
- 示例代码以仓库内真实 API 为准，不再使用历史包路径或过期签名。
- `test/integration` 文档主要描述测试和仪表盘，不再承担业务接入文档角色。
