# Changelog

## [Unreleased]
- 待补充

## 2025-10-13
- go-pipeline 升级对齐与指标接入：
  - 新增可选扩展接口 PipelineMetricsReporter（ObserveDequeueLatency/ObserveProcessDuration/IncDropped），不破坏原 MetricsReporter
  - 新增 gopipeline_metrics_adapter，实现 go-pipeline v2.2.0 MetricsHook 并注入 WithMetrics
  - 管道级处理耗时通过 flush defer 上报（携带 success/fail 状态），避免与 MetricsHook.Flush 重复度量
  - adapter.Flush 仅上报批大小，ErrorDropped → IncDropped("error_chan_full")
- 集成测试/Prometheus：
  - 新增 batchflow_pipeline_process_duration_seconds、batchflow_pipeline_dequeue_latency_seconds、batchflow_pipeline_dropped_total 指标导出
  - 执行耗时直方图修正按 test_name 维度聚合
- Grafana：
  - 合并并细化 “流水线阶段（核心阶段指标）” 分组（p50/p95/p99），统一秒/reqps单位，三列对齐
  - 修复“执行耗时(按测试名)”无曲线问题（聚合维度调整为 sum by (le, test_name)）
- 稳定性：
  - 记录 v2.2.0 ResetBatchData 并发别名问题与修复背景，建议配置与调优说明