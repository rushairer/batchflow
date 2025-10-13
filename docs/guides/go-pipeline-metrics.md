# BatchFlow 与 go-pipeline v2.2.0 指标集成指南

本指南说明 BatchFlow 项目如何对齐 go-pipeline v2.2.0 的指标能力，包括接口扩展、适配器接入、Grafana 看板、集成测试侧指标接入与常见问题。

1. 升级概述
- 已对齐 go-pipeline v2.2.0 的 MetricsHook（Flush/Error/ErrorDropped）
- 在本项目中采用“非破坏性扩展”策略：不修改原 MetricsReporter 接口，新增可选扩展接口 PipelineMetricsReporter 来承接管道级指标
- 提供适配器桥接 go-pipeline.WithMetrics 到我们的 Reporter

2. 接口与适配
- 新增扩展接口（可选实现）
  type PipelineMetricsReporter interface {
      ObserveDequeueLatency(d time.Duration)
      ObserveProcessDuration(d time.Duration, status string)
      IncDropped(reason string)
  }
- 适配器
  - 文件：gopipeline_metrics_adapter.go
  - 实现 go-pipeline 的 MetricsHook，并在构造后通过 pipeline.WithMetrics(adapter) 注入
  - 映射关系：
    - ErrorDropped() → IncDropped("error_chan_full")
    - Flush(items, duration)：当前仅上报批大小到 ObserveBatchSize(items)，避免与我们已有的“带状态的处理耗时”重复统计
    - 管道级处理耗时：在 flushFunc defer 中根据 err 上报 ObserveProcessDuration(duration, "success"/"fail")

3. 集成测试 Prometheus 指标
- Prometheus 实现文件：
  - test/integration/metrics_reporter.go：实现 PipelineMetricsReporter 的三个方法
  - test/integration/prometheus.go：注册新增指标
- 新增指标（建议命名）
  - batchflow_pipeline_process_duration_seconds_bucket{status}
  - batchflow_pipeline_dequeue_latency_seconds_bucket
  - batchflow_pipeline_dropped_total{reason}
- 注意：执行耗时直方图导出的标签按 test_name 上报，配合仪表盘使用

4. Grafana 看板
- 主看板位置：test/integration/grafana/provisioning/dashboards/batchflow-performance.json
- 新增/优化的“流水线阶段（核心阶段指标）”分组（单位为秒，除丢弃错误速率为 req/s）：
  - 入队延迟 (p50/p95/p99)
  - 攒批耗时 (p50/p95/p99)
  - 执行耗时 (p50/p95/p99)（按测试名）
  - 流水线处理耗时 (p50/p95/p99)（按状态）
  - 流水线出队等待 (p50/p95/p99)
  - 流水线丢弃错误速率 (by reason)
- 执行耗时面板聚合修正为 sum by (le, test_name)，避免多余分组导致曲线缺失
- 布局对齐：上述 6 个面板按 3 列 × 2 行整齐排布

5. v2.2.0 ResetBatchData 并发别名问题与说明
- 现象：v2.2.0 将 flush 后重置从 initBatchData() 改为 ResetBatchData(batchData)
- 在 async 模式下，如果 ResetBatchData 复用了底层内存，可能与在途的 flush goroutine 共享底层数组导致数据覆盖、竞态和异常
- 当前状态：你已手工更新 go-pipeline 修复该问题；本仓也通过在 flush defer 侧自有上报规避重复度量
- 可选兜底（如需）：切换为 SyncPerform 模式运行 pipeline，完全避免别名共享

6. 调优建议
- PipelineConfig.MaxConcurrentFlushes：建议默认设为 1 以维持稳定（可根据场景调优）
- BufferSize 与 FlushSize：保持 1×~4× 的量级关系，避免通道巨缓冲引发 flush 洪峰
- FlushInterval：避免过短；与满批触发叠加会放大并发
- 执行器并发：多 schema 场景建议设定小的 WithConcurrencyLimit（如 4~8），避免 DB 端压力

7. 常见问题
- 面板无曲线：确认对应直方图 bucket 是否产生样本；执行耗时面板需确保 test_name 标签存在且与仪表盘变量匹配
- 丢弃错误速率无数据：该指标代表错误通道满被丢弃的计数，正常情况下应接近 0；出现升高需要关注背压、错误消费与队列容量配置
- 出队等待无数据：需确认 ObserveDequeueLatency 的上报路径是否被触发（当前 pipeline MetricsHook 未直接提供 dequeue 事件，已在 Reporter 扩展中预留接口；如需要可在获取任务取用处度量）

8. 变更摘要
- 新增 PipelineMetricsReporter 接口（非破坏性）
- 适配 go-pipeline MetricsHook，并连接到我们的 Reporter
- 集成测试侧 Prometheus 实现与 Grafana 看板同步更新
- 修复并细化面板：执行耗时(按测试名)聚合、单位补齐、布局对齐

如需扩展更多指标或调整仪表盘样式，请在 PR 中注明偏好，我们将继续完善。