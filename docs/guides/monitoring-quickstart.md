# 监控快速上手

推荐直接使用仓库里的官方 Prometheus 示例：

```go
import prommetrics "github.com/rushairer/batchflow/examples/metrics/prometheus"
```

## 1. 启动指标端点

```go
metrics := prommetrics.NewMetrics(prommetrics.Options{
	Namespace:             "batchflow",
	IncludeInstanceID:     true,
	EnablePipelineMetrics: true,
})

if err := metrics.StartServer(2112); err != nil {
	return err
}
defer metrics.StopServer(context.Background())
```

## 2. 把 reporter 注入 BatchFlow

```go
reporter := prommetrics.NewReporter(metrics, "mysql", "order_writer")

flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
	MetricsReporter:  reporter,
})
defer flow.Close()
```

## 3. 访问 `/metrics`

- 地址：`http://localhost:2112/metrics`
- 建议确认可以看到：
  - `batchflow_enqueue_latency_seconds`
  - `batchflow_pipeline_dequeue_latency_seconds`
  - `batchflow_execute_duration_seconds`
  - `batchflow_batch_size`
  - `batchflow_pipeline_flush_size`
  - `batchflow_submit_rejected_total`

## 4. 推荐先看的图表

- `enqueue_latency_seconds` P95
- `pipeline_dequeue_latency_seconds` P95
- `execute_duration_seconds` P95 / P99
- `batch_size` P50 / P95
- `pipeline_flush_size` P50 / P95
- `schema_groups_per_flush` P95
- `submit_rejected_total` rate

## 5. PromQL 示例

```promql
# Submit 被拒绝的速率
sum(rate(batchflow_submit_rejected_total[5m])) by (instance_id, reason)

# 队列等待 P95
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_pipeline_dequeue_latency_seconds_bucket[5m])))

# 执行耗时 P95
histogram_quantile(0.95, sum by (le, instance_id, status) (rate(batchflow_execute_duration_seconds_bucket[5m])))

# Flush 输入大小 P95
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_pipeline_flush_size_bucket[5m])))

# 每次 flush 的 schema 组数量 P95
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_schema_groups_per_flush_bucket[5m])))

# 最终失败速率
sum(rate(batchflow_errors_total{error_type=~"final:.*"}[5m])) by (instance_id, error_type)
```

## 注意事项

- `ObserveBatchSize` 和 `pipeline_flush_size` 不是一回事。
- `MetricsReporter` 最好在构造 `BatchFlow` 之前通过 `PipelineConfig.MetricsReporter` 传入。
- `Close()` 负责最终 flush；不建议只依赖 `FlushInterval` 作为收尾机制。

继续阅读：

- [监控指南](monitoring.md)
- [Metrics 规格](metrics-spec.md)
- [自定义 MetricsReporter](custom-metrics-reporter.md)
