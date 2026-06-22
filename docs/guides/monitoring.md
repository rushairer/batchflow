# BatchFlow 监控指南

这份文档描述业务侧应该怎样理解和使用 BatchFlow 指标。指标语义的 source of truth 是 [Metrics 规格](metrics-spec.md)。

## 推荐接入方式

优先使用仓库内的 Prometheus 示例：

```go
import prommetrics "github.com/rushairer/batchflow/examples/metrics/prometheus"
```

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

## 指标分层

### Submit / Queue

- `enqueue_latency_seconds`
- `pipeline_queue_length`
- `submit_rejected_total`

### Pipeline / Flush

- `pipeline_dequeue_latency_seconds`
- `pipeline_process_duration_seconds`
- `pipeline_flush_size`
- `schema_groups_per_flush`
- `pipeline_dropped_total`

### Executor

- `batch_assemble_duration_seconds`
- `execute_duration_seconds`
- `batch_size`
- `executor_concurrency`
- `inflight_batches`
- `errors_total`

### Operation Diagnostics

- `operation_errors_total`
- `operation_generated_items`
- `operation_generated_args`

### SQL Diagnostics

- `sql_errors_total`
- `sql_generated_rows`
- `sql_generated_args`
- `sql_deduplicated_rows_total`

## 关键语义

### `batch_size`

表示单个 schema 执行批大小，也就是一次 `ExecuteBatch(...)` 收到的数据量。

### `pipeline_flush_size`

表示一次 pipeline flush 收到的总请求数。它可能会在内部被拆成多个 schema 组。

### `schema_groups_per_flush`

表示一次 flush 内被拆成多少个 schema 组。这个值越高，说明单次 flush 的数据异质性越高。

### `submit_rejected_total`

表示 `Submit(...)` 在进入内部队列前被拒绝的次数，常见原因：

- `context_canceled`
- `context_deadline_exceeded`
- `batchflow_closed`
- `empty_request`
- `invalid_schema`
- `missing_column`
- `empty_schema_name`

### `operation_errors_total`

表示 backend-neutral operation 生成或执行错误，适用于 SQL、Redis、自定义 processor：

- `backend`：`sql`、`redis`、`custom` 或自定义 backend。
- `stage`：`validate`、`generate`、`execute`、`retry`、`final`。
- `reason`：低基数错误分类，如 `timeout`、`connection`、`duplicate_key`、`other`。
- `table`：仅在 `Options.IncludeTable=true` 时启用；对非 SQL 场景表示 schema 名。

### `operation_generated_items`

记录 operation 生成前后的 item 数：

- `kind="input"`：进入当前 batch 的原始数据量。
- `kind="output"`：实际生成的后端操作数量。
- `backend` / `operation`：用于区分 SQL upsert、Redis command、自定义 HTTP post 等。

这些指标是通用诊断入口。SQL 专用指标保留，用于查看 conflict key 去重等 SQL 细节。

### `sql_errors_total`

表示 SQL 生成或执行阶段的错误，标签包含：

- `stage`：`validate`、`generate`、`execute`
- `reason`：低基数错误分类，如 `duplicate_key`、`deadlock`、`timeout`、`connection`、`syntax`、`other`
- `table`：仅在 `Options.IncludeTable=true` 时启用

不要把原始 SQL 或参数值作为 Prometheus label。需要定位具体 SQL 时使用日志中的 `SQLPreview.Fingerprint` 或 `SQLError.SQLFingerprint`。

### `sql_generated_rows`

记录 SQL 生成前后的行数：

- `kind="input"`：调用 `ExecuteBatch` 时收到的原始行数。
- `kind="output"`：批内冲突键去重/合并后实际进入 SQL 的行数。

如果 PostgreSQL 曾出现 `ON CONFLICT DO UPDATE command cannot affect row a second time`，应重点检查 `input` 与 `output` 的差值，以及 `sql_deduplicated_rows_total`。

### `sql_deduplicated_rows_total`

记录批内同 conflict key 的处理：

- `strategy="ignore"`：重复 key 保留第一条。
- `strategy="replace"`：重复 key 保留最后一条。
- `strategy="update"`：重复 key 合并字段。
- `kind="deduplicated"` 或 `kind="merged"` 表示移除或合并的行数。

## 推荐告警

### 队列和背压

```promql
sum(rate(batchflow_submit_rejected_total{reason="batchflow_closed"}[5m])) by (instance_id)
```

```promql
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_pipeline_dequeue_latency_seconds_bucket[5m])))
```

### 执行质量

```promql
sum(rate(batchflow_errors_total{error_type=~"final:.*"}[5m])) by (instance_id, error_type)
```

```promql
histogram_quantile(0.99, sum by (le, instance_id, status) (rate(batchflow_execute_duration_seconds_bucket[5m])))
```

```promql
sum(rate(batchflow_operation_errors_total[5m])) by (instance_id, backend, stage, reason)
```

```promql
histogram_quantile(0.95, sum by (le, instance_id, backend, operation, kind) (rate(batchflow_operation_generated_items_bucket[5m])))
```

```promql
sum(rate(batchflow_sql_errors_total[5m])) by (instance_id, stage, reason)
```

```promql
sum(rate(batchflow_sql_deduplicated_rows_total[5m])) by (instance_id, strategy, kind)
```

### flush 结构复杂度

```promql
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_schema_groups_per_flush_bucket[5m])))
```

## 仪表盘建议

建议最少做 3 组图：

### 入口压力

- `enqueue_latency_seconds`
- `pipeline_queue_length`
- `submit_rejected_total`

### flush 形态

- `pipeline_flush_size`
- `schema_groups_per_flush`
- `pipeline_dequeue_latency_seconds`

### 执行结果

- `execute_duration_seconds`
- `batch_size`
- `errors_total`
- `operation_errors_total`
- `operation_generated_items`
- `sql_errors_total`
- `sql_deduplicated_rows_total`
- `inflight_batches`

## 常见误区

- 不要把 `batch_size` 当吞吐总量指标；它是单次 schema 执行批大小分布。
- 不要把 `pipeline_dequeue_latency_seconds` 理解成“网络延迟”；它反映的是内部排队等待。
- 不要把 `Close()` 省略掉，否则最后一批数据和对应指标都可能没被完整 flush。
- 不要把原始 SQL、Redis key、HTTP body、用户标识作为 Prometheus label；需要定位时使用 fingerprint 和结构化日志。

继续阅读：

- [监控快速上手](monitoring-quickstart.md)
- [Metrics 规格](metrics-spec.md)
- [自定义 MetricsReporter](custom-metrics-reporter.md)
