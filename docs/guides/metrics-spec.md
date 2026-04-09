# BatchFlow Metrics 规格

这份文档定义当前仓库推荐的指标语义。以后调整埋点时，应该先改这里，再改实现和 dashboard。

## 指标分层

### 1. Submit / Queue

| 指标 | 类型 | 语义 |
|---|---|---|
| `enqueue_latency_seconds` | Histogram | `Submit` 调用到成功写入内部队列的耗时 |
| `pipeline_queue_length` | Gauge | 当前队列长度的近似值 |
| `submit_rejected_total` | Counter | `Submit` 被拒绝的次数，按原因分类 |

`submit_rejected_total` 常见 reason：

- `context_canceled`
- `context_deadline_exceeded`
- `batchflow_closed`
- `empty_request`
- `invalid_schema`
- `missing_column`
- `empty_schema_name`

### 2. Pipeline / Flush

| 指标 | 类型 | 语义 |
|---|---|---|
| `pipeline_dequeue_latency_seconds` | Histogram | 请求成功入队后，到 flush 开始处理前的等待时间 |
| `pipeline_process_duration_seconds` | Histogram | 整次 flush 的处理耗时 |
| `pipeline_flush_size` | Histogram | 整次 flush 收到的请求数 |
| `schema_groups_per_flush` | Histogram | 一次 flush 内拆出的 schema 组数 |
| `pipeline_dropped_total` | Counter | pipeline 级丢弃事件 |

说明：

- `pipeline_dequeue_latency_seconds` 当前由 BatchFlow 自采样，不依赖 go-pipeline 原生 hook。
- `pipeline_dropped_total` 当前主要对应错误通道写满。

### 3. Executor

| 指标 | 类型 | 语义 |
|---|---|---|
| `batch_assemble_duration_seconds` | Histogram | 单个 schema 组装成执行输入的耗时 |
| `execute_duration_seconds` | Histogram | 单个 schema 执行批的总耗时，包含重试与退避 |
| `batch_size` | Histogram | 单个 schema 执行批大小 |
| `inflight_batches` | Gauge | 当前执行中的批次数 |
| `executor_concurrency` | Gauge | 当前配置的执行并发上限，`0` 表示不限流 |
| `errors_total` | Counter | 执行器错误计数 |

`errors_total` 的 `error_type` 约定：

- `retry:<reason>`
- `final:<reason>`

常见 reason：

- `deadlock`
- `lock_timeout`
- `timeout`
- `connection`
- `io`
- `context`
- `non_retryable`

## 重要区分

### `batch_size` 和 `pipeline_flush_size` 不是同一个指标

- `batch_size`：最终交给一次 `ExecuteBatch` 的单个 schema 批大小。
- `pipeline_flush_size`：某次 flush 一共拉到了多少请求，可能之后被拆成多个 schema 组。

例子：

- 一次 flush 收到 100 条请求，其中 `users` 70 条，`orders` 30 条。
- 那么：
  - `pipeline_flush_size = 100`
  - `schema_groups_per_flush = 2`
  - `batch_size` 会记录两个样本：70 和 30

## 标签建议

- `database`：`mysql` / `postgres` / `sqlite` / `redis`
- `instance_id`：业务实例名，例如 `order_writer`
- `status`：`success` / `fail`
- `error_type`：`retry:*` / `final:*`
- `reason`：拒绝原因或丢弃原因

建议：

- `instance_id` 可以开。
- `table` 维度谨慎开启，避免高基数。
- 不要引入 `request_id`、时间戳等高基数标签。

## PromQL 示例

```promql
# Submit 被拒绝的速率
sum(rate(batchflow_submit_rejected_total[5m])) by (instance_id, reason)

# Flush 输入大小 P95
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_pipeline_flush_size_bucket[5m])))

# 一次 flush 的 schema 组复杂度
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_schema_groups_per_flush_bucket[5m])))

# 队列等待 P95
histogram_quantile(0.95, sum by (le, instance_id) (rate(batchflow_pipeline_dequeue_latency_seconds_bucket[5m])))

# 执行耗时 P95
histogram_quantile(0.95, sum by (le, instance_id, status) (rate(batchflow_execute_duration_seconds_bucket[5m])))

# 最终失败速率
sum(rate(batchflow_errors_total{error_type=~"final:.*"}[5m])) by (instance_id, error_type)
```

## 实现约定

- `ObserveBatchSize` 只用于 `batch_size`。
- `ObserveExecuteDuration` 不应该再顺手记录 `batch_size`，避免双计数。
- `PipelineMetricsReporter` 只承载 pipeline 级指标。
- `BatchFlowMetricsReporter` 只承载 BatchFlow 自身流程指标。
