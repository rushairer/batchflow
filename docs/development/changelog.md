# Changelog

## [Unreleased]

### Enterprise readiness
- Added open-source governance and security documentation.
- Added security scanning and dependency update automation.
- Strengthened CI, coverage, release, and nightly test gates.
- Documented API stability, release policy, and error classification contracts.

## [v1.1.1] - 2026-04-09

### 生命周期示例 / Request setter / 文档一致性补强
- 生命周期：
  - 新增可编译示例 `ExampleBatchFlow_Close` 与 `ExampleBatchFlow_Done`
  - 补充 `Wait()` / `Done()` 契约测试：覆盖 `Close()` 后退出信号、重复关闭幂等、父上下文取消返回值
- Request：
  - 新增基础整数 setter：`SetInt`、`SetInt8`、`SetInt16`、`SetUint`、`SetUint8`、`SetUint16`、`SetUint32`、`SetUint64`
  - 新增对应测试，确认列值类型不会被隐式改写
- 文档：
  - README、API 参考、示例指南同步新增 setter 与生命周期样例
  - `docs-check` 新增对 `Done()`、`SetInt`、`SetUint64` 的契约检查

## [v1.1.0] - 2026-04-09

### API / Metrics / Docs 契约收敛
- 生命周期：
  - `BatchFlow` 新增 `Close()`、`Wait()`、`Done()`，补齐公开生命周期契约
  - `Close()` 负责停止接收输入、触发最终 flush 并等待后台退出
- 指标语义：
  - 新增可选扩展接口 `BatchFlowMetricsReporter`
  - 新增真实队列等待采样：`ObserveDequeueLatency` 不再只是预留接口
  - 新增 `submit_rejected_total`、`pipeline_flush_size`、`schema_groups_per_flush`
  - 明确 `batch_size` 仅表示单个 schema 执行批大小，不再与整次 flush 输入量混用
  - reporter 示例修正：`ObserveExecuteDuration` 不再顺手重复记录 `batch_size`
- 文档：
  - README、API 参考、配置说明、示例、监控文档重写为当前实现口径
  - 新增 `docs/guides/metrics-spec.md` 作为指标语义 source of truth
  - 新增 `make docs-check` 与 `scripts/check-doc-consistency.sh`，阻止关键文档继续引用过期 API/指标

### 2025-12-02 - 指标体系升级
- 标签体系重构：
  - 引入 `instance_id` 标签替代 `test_name`，支持多实例隔离
  - 明确 `database` 语义：仅表示数据库类型（mysql/postgres/sqlite/redis）
  - execute_duration 添加 `status` 维度区分成功/失败
- 指标完整性补充：
  - 实现完整的 PipelineMetricsReporter 接口（ObserveDequeueLatency、ObserveProcessDuration、IncDropped）
  - 所有核心指标添加 instance_id 支持
- 示例更新：
  - examples/metrics/prometheus 添加 Options.IncludeInstanceID
  - 提供完整的 PipelineMetricsReporter 实现
  - 动态标签检测（hasLabel 函数）
- 文档同步：
  - 更新 docs/guides 下所有监控相关文档
  - 标签说明统一为 instance_id
  - 添加管道级指标说明

## 2025-10-13
- go-pipeline 升级对齐与指标接入：
  - 新增可选扩展接口 PipelineMetricsReporter（ObserveDequeueLatency/ObserveProcessDuration/IncDropped），不破坏原 MetricsReporter
  - 新增 gopipeline_metrics_adapter，实现 go-pipeline v2.2.0 MetricsHook 并注入 WithMetrics
  - 管道级处理耗时通过 flush defer 上报（携带 success/fail 状态），避免与 MetricsHook.Flush 重复度量
  - adapter.Flush 仅上报批大小，ErrorDropped → IncDropped("error_chan_full")
- 集成测试/Prometheus：
  - 新增 batchflow_pipeline_process_duration_seconds、batchflow_pipeline_dequeue_latency_seconds、batchflow_pipeline_dropped_total 指标导出
  - 执行耗时直方图修正按 instance_id 维度聚合
- Grafana：
  - 合并并细化 "流水线阶段（核心阶段指标）" 分组（p50/p95/p99），统一秒/reqps单位，三列对齐
  - 修复"执行耗时(按实例)"无曲线问题（聚合维度调整为 sum by (le, instance_id)）
- 稳定性：
  - 记录 v2.2.0 ResetBatchData 并发别名问题与修复背景，建议配置与调优说明
