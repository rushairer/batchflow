# BatchFlow 文档索引

英文文档是当前对外主契约，中文文档作为镜像和补充说明。

推荐阅读路径：

1. 先看 [README.zh-CN](../../README.zh-CN.md)，确认安装、生命周期和核心模型。
2. 再看 [API 参考](../api/reference.md) 与 [配置说明](api/configuration.md)，确认公开契约。
3. 根据需要阅读生产、监控、测试和架构文档。

## 使用者文档

- [README.zh-CN](../../README.zh-CN.md)
- [API 参考](../api/reference.md)
- [配置说明](api/configuration.md)
- [使用示例](guides/examples.md)
- [生产指南](guides/production.md)
- [测试指南](guides/testing.md)
- [错误分类](../guides/error-classification.md)
- [监控快速上手](../guides/monitoring-quickstart.md)
- [Metrics 规格](../guides/metrics-spec.md)

## 维护者文档

- [架构设计](../development/architecture.md)
- [API 稳定性策略](../development/api-stability.md)
- [v2 迁移指南](development/migration-v2.md)
- [集成测试](../guides/integration-tests.md)
- [质量指南](../development/quality.md)

## 文档约定

- 英文 `README.md`、`docs/api/reference.md`、`docs/api/configuration.md`、`docs/guides/production.md`、`docs/guides/metrics-spec.md` 是主契约。
- 示例必须使用 `github.com/rushairer/batchflow/v2`。
- SQL update/replace 示例必须显式声明 `ConflictColumns`。
- 非 SQL 同 key 合并使用 `PipelineConfig.Coalescer`。
