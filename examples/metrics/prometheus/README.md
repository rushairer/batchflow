# Prometheus 指标示例

这个目录提供了 BatchFlow 官方推荐的 Prometheus 接入方式。

## 提供的能力

- `MetricsReporter`
- `PipelineMetricsReporter`
- `BatchFlowMetricsReporter`

## 覆盖的指标

### Counter

- `errors_total`
- `submit_rejected_total`
- `pipeline_dropped_total`

### Histogram

- `enqueue_latency_seconds`
- `batch_assemble_duration_seconds`
- `execute_duration_seconds`
- `batch_size`
- `pipeline_dequeue_latency_seconds`
- `pipeline_process_duration_seconds`
- `pipeline_flush_size`
- `schema_groups_per_flush`

### Gauge

- `executor_concurrency`
- `pipeline_queue_length`
- `inflight_batches`

## 最小示例

```go
metrics := prometheusmetrics.NewMetrics(prometheusmetrics.Options{
	Namespace:             "batchflow",
	IncludeInstanceID:     true,
	EnablePipelineMetrics: true,
})

if err := metrics.StartServer(2112); err != nil {
	log.Fatal(err)
}
defer metrics.StopServer(context.Background())

reporter := prometheusmetrics.NewReporter(metrics, "mysql", "order_writer")

flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:      5000,
	FlushSize:       200,
	FlushInterval:   100 * time.Millisecond,
	MetricsReporter: reporter,
})
defer flow.Close()
```

## 语义约定

- `batch_size`：单个 schema 执行批大小。
- `pipeline_flush_size`：整次 flush 收到的请求数。
- `schema_groups_per_flush`：整次 flush 拆出的 schema 组数量。
- `submit_rejected_total`：`Submit` 被拒绝的次数和原因。

## 推荐标签

- `database`
- `instance_id`
- `status`
- `error_type`
- `reason`

更多语义说明见：

- [Metrics 规格](../../../docs/guides/metrics-spec.md)
- [监控快速上手](../../../docs/guides/monitoring-quickstart.md)
