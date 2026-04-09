# BatchFlow v1.1.0 Release Notes

发布日期：2026-04-09

## 亮点

- 补齐 `BatchFlow` 生命周期接口：`Close()`、`Wait()`、`Done()`
- 收敛 metrics 契约，新增真实的队列等待采样与 BatchFlow 自身流程指标
- 重写 README、API、监控与示例文档，统一到当前实现口径
- 增加 `docs-check`，避免文档再次漂移到过期 API/指标名

## 新增

### 生命周期

- `BatchFlow.Close()`
- `BatchFlow.Wait()`
- `BatchFlow.Done()`

### Metrics

- `BatchFlowMetricsReporter`
- `submit_rejected_total`
- `pipeline_flush_size`
- `schema_groups_per_flush`

## 变更

### Metrics 语义

- `ObserveDequeueLatency` 现在由 BatchFlow 真实采样，不再只是文档预留接口。
- `batch_size` 的语义固定为“单个 schema 执行批大小”。
- `ObserveExecuteDuration` 不再重复顺手记录 `batch_size`，避免双计数。

### 文档

- README 改成当前推荐用法：`NewXxxBatchFlow + PipelineConfig + Submit + Close`
- `examples/metrics/prometheus` 现在是官方推荐的监控接入方式
- 新增 `docs/guides/metrics-spec.md` 作为指标语义 source of truth

## 兼容性

这是一个向后兼容的 minor 版本更新。

- 现有核心构造函数仍然保留
- 现有执行器接口仍然保留
- 模块路径没有变化

## 验证

- `go test ./...` 通过
- `go test -run '^$' ./...` 通过
- `make docs-check` 通过

## 发布备注

本次版本已完成 `make release-check`，包含文档一致性检查、lint 与测试，满足打 tag 发布条件。
