# Changelog

## [Unreleased]

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