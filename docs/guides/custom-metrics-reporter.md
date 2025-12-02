# 自定义 MetricsReporter 指南

适用对象：需要将 BatchFlow 指标接入自有监控体系（埋点、日志、PushGateway、SaaS 平台等）。

## 接口说明

BatchFlow 提供两个指标接口：

### 1. MetricsReporter（核心接口）

接口位于 metrics_reporter.go：
- ObserveEnqueueLatency(d)        提交到入队的耗时
- ObserveBatchAssemble(d)         攒批/组装耗时
- ObserveExecuteDuration(table, n, d, status) 执行耗时
- ObserveBatchSize(n)             批大小
- IncError(table, kind)           错误与重试计数（示例：retry:timeout、final:non_retryable）
- SetConcurrency(n)               执行并发度（0 表示不限流）
- SetQueueLength(n)               队列长度
- IncInflight()/DecInflight()     在途批次数（进入/退出执行区间）

### 2. PipelineMetricsReporter（扩展接口，可选）

用于 go-pipeline 集成的管道级指标：
- ObserveDequeueLatency(d)        管道出队延迟
- ObserveProcessDuration(d, status) 管道处理耗时（按状态）
- IncDropped(reason)              管道丢弃计数（按原因）

实现时可按需选择方法；推荐至少实现：
- ObserveExecuteDuration、ObserveBatchSize
- 以及 IncInflight/DecInflight（获得实时负载视角）

## 最小实现示例

```go
type MyReporter struct{}

func (*MyReporter) ObserveEnqueueLatency(d time.Duration) {}
func (*MyReporter) ObserveBatchAssemble(d time.Duration) {}
func (*MyReporter) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
    // 上报到你们自建监控
}
func (*MyReporter) ObserveBatchSize(n int) {}
func (*MyReporter) IncError(table, kind string) {}
func (*MyReporter) SetConcurrency(n int) {}
func (*MyReporter) SetQueueLength(n int) {}
func (*MyReporter) IncInflight() {}
func (*MyReporter) DecInflight() {}
```

注入方式（务必在 NewBatchFlow 之前）：
```go
exec := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, driver).
    WithMetricsReporter(&MyReporter{})
bs := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, exec)
defer bs.Close()
```

## 语义与单位建议

- 时间类：单位秒（Prometheus 推荐）。内部 time.Duration 可换算为秒数。
- 批大小：整数（n）。
- 并发度、队列长度、在途批次：Gauge。
- 错误：Counter，kind 维度可区分 retry:reason 与 final:reason。
- 标签建议：
  - database：数据库类型（mysql/postgres/sqlite/redis）
  - instance_id：实例标识，用于区分多个 BatchFlow 实例
    - 集成测试：使用测试名称（如 "高吞吐量测试"）
    - 生产环境：使用业务标识（如 "order_writer", "log_collector"）
  - table：表名（可选，仅在需要按表维度统计时使用）
  - status：执行状态（success/fail），用于区分成功/失败耗时分布
  - reason：丢弃原因（如 "error_chan_full"，仅管道级指标）

## Noop 实现与渐进启用

- NoopMetricsReporter 是空实现，默认零开销，便于在未准备好监控系统前先集成 BatchFlow。
- 准备就绪后随时切换到你自己的 Reporter，不需改动业务调用逻辑。