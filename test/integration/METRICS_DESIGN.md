# BatchFlow 性能指标设计文档

> 更新时间：2025-12-02  
> 版本：v2.0

## 概述

本文档说明 BatchFlow 性能指标体系的设计原则、标签维度、指标覆盖范围，以及如何支持多实例场景。

## 核心改进（v2.0）

### 1. 多实例支持

**问题**：v1.0 使用 `database` 和 `test_name` 标签，语义不清晰，无法区分同一项目中的多个 BatchFlow 实例。

**解决方案**：引入 `instance_id` 标签：

- **标签维度**：`database` + `instance_id`
  - `database`：数据库类型（mysql/postgres/sqlite/redis）
  - `instance_id`：BatchFlow 实例标识
    - 集成测试：使用测试名称（如 "高吞吐量测试"）
    - 生产环境：使用业务标识（如 "order_writer", "log_collector", "analytics_batch"）

**示例**：
```go
// 集成测试
reporter := NewPrometheusMetricsReporter(metrics, "mysql", "高吞吐量测试")

// 生产环境
reporter1 := NewPrometheusMetricsReporter(metrics, "mysql", "order_writer")
reporter2 := NewPrometheusMetricsReporter(metrics, "mysql", "log_collector")
```

### 2. 完整的指标覆盖

#### 2.1 核心流程指标（Histogram）

| 指标名称 | 说明 | 标签 | 覆盖阶段 |
|---------|------|------|---------|
| `batchflow_enqueue_latency_seconds` | 提交到入队延迟 | database, instance_id | Submit → 入队 |
| `batchflow_batch_assemble_duration_seconds` | 批次组装耗时 | database, instance_id | 攒批/组装 |
| `batchflow_execute_duration_seconds` | 批次执行耗时（含重试） | database, instance_id, status | 执行（success/fail） |
| `batchflow_batch_size` | 批次大小分布 | database, instance_id | 每批次 |

#### 2.2 状态观测指标（Gauge）

| 指标名称 | 说明 | 标签 |
|---------|------|------|
| `batchflow_executor_concurrency` | 执行器并发度 | database, instance_id |
| `batchflow_pipeline_queue_length` | 队列长度 | database, instance_id |
| `batchflow_inflight_batches` | 在途批次数 | database, instance_id |

#### 2.3 错误统计（Counter）

| 指标名称 | 说明 | 标签 | error_type 示例 |
|---------|------|------|----------------|
| `batchflow_errors_total` | 错误总数 | database, instance_id, error_type | retry:deadlock, final:context |

**错误类型约定**：
- `retry:*`：可重试错误（deadlock, lock_timeout, connection, io）
- `final:*`：最终失败（context, non_retryable）

#### 2.4 管道级指标（Pipeline Metrics，可选）

| 指标名称 | 说明 | 标签 |
|---------|------|------|
| `batchflow_pipeline_dequeue_latency_seconds` | 出队等待延迟 | database, instance_id |
| `batchflow_pipeline_process_duration_seconds` | 管道处理耗时 | database, instance_id, status |
| `batchflow_pipeline_dropped_total` | 丢弃事件计数 | database, instance_id, reason |

## 接口实现

### MetricsReporter 接口（核心）

```go
type MetricsReporter interface {
    // 阶段耗时
    ObserveEnqueueLatency(d time.Duration)
    ObserveBatchAssemble(d time.Duration)
    ObserveExecuteDuration(table string, n int, d time.Duration, status string)
    
    // 观测
    ObserveBatchSize(n int)
    IncError(table string, typ string)
    SetConcurrency(n int)
    SetQueueLength(n int)
    IncInflight()
    DecInflight()
}
```

### PipelineMetricsReporter 接口（可选扩展）

```go
type PipelineMetricsReporter interface {
    ObserveDequeueLatency(d time.Duration)
    ObserveProcessDuration(d time.Duration, status string)
    IncDropped(reason string)
}
```

## 使用示例

### 集成测试

```go
// 创建指标收集器
prometheusMetrics := NewPrometheusMetrics()
prometheusMetrics.StartServer(9090)

// 为每个测试用例创建独立的 Reporter
reporter := NewPrometheusMetricsReporter(prometheusMetrics, "mysql", "高吞吐量测试")

// 绑定到 BatchFlow
executor := batchflow.NewThrottledBatchExecutor(processor).
    WithMetricsReporter(reporter)
```

### 生产环境

```go
// 创建指标收集器（全局单例）
var globalMetrics = NewPrometheusMetrics()

// 订单写入实例
orderReporter := NewPrometheusMetricsReporter(globalMetrics, "mysql", "order_writer")
orderFlow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
    MetricsReporter: orderReporter,
    // ...
})

// 日志收集实例
logReporter := NewPrometheusMetricsReporter(globalMetrics, "postgres", "log_collector")
logFlow := batchflow.NewPostgreSQLBatchFlow(ctx, db, batchflow.PipelineConfig{
    MetricsReporter: logReporter,
    // ...
})
```

## Grafana 查询示例

### 多实例聚合

```promql
# 所有实例的总吞吐量
sum(rate(batchflow_batch_size_sum[5m])) by (database)

# 按实例查看吞吐量
sum(rate(batchflow_batch_size_sum[5m])) by (database, instance_id)
```

### 执行耗时（按状态）

```promql
# 成功执行的 P99 延迟
histogram_quantile(0.99, 
  sum(rate(batchflow_execute_duration_seconds_bucket{status="success"}[5m])) 
  by (database, instance_id, le)
)

# 失败执行的 P99 延迟
histogram_quantile(0.99, 
  sum(rate(batchflow_execute_duration_seconds_bucket{status="fail"}[5m])) 
  by (database, instance_id, le)
)
```

### 错误监控

```promql
# 重试错误率
sum(rate(batchflow_errors_total{error_type=~"retry:.*"}[5m])) by (database, instance_id, error_type)

# 最终失败率
sum(rate(batchflow_errors_total{error_type=~"final:.*"}[5m])) by (database, instance_id, error_type)
```

## 迁移指南（v1.0 → v2.0）

### 代码修改

**之前**：
```go
reporter := NewPrometheusMetricsReporter(metrics, "mysql", "test1")
```

**之后**（无需修改）：
```go
// 集成测试：保持不变，testName 现在作为 instance_id
reporter := NewPrometheusMetricsReporter(metrics, "mysql", "test1")

// 生产环境：使用有意义的业务标识
reporter := NewPrometheusMetricsReporter(metrics, "mysql", "order_writer")
```

### Prometheus 指标标签

**之前**：
- `batchflow_execute_duration_seconds{database="mysql", test_name="test1"}`

**之后**：
- `batchflow_execute_duration_seconds{database="mysql", instance_id="test1"}`

### Grafana Dashboard

需要更新查询变量：
- `test_name` → `instance_id`
- 添加 `status` 维度过滤（success/fail）

## 最佳实践

### 1. instance_id 命名规范

- **集成测试**：使用测试名称（如 "高吞吐量测试"）
- **生产环境**：使用业务标识（如 "order_writer", "user_profile_sync"）
- **避免**：使用随机ID或时间戳（会导致基数爆炸）

### 2. 标签基数控制

- ✅ **推荐**：database, instance_id（低基数）
- ⚠️ **谨慎**：table（中等基数，建议在 instance_id 中区分）
- ❌ **避免**：request_id, timestamp（高基数）

### 3. 多实例隔离

```go
// 为不同业务创建独立 Reporter
orderReporter := NewPrometheusMetricsReporter(metrics, "mysql", "order_writer")
logReporter := NewPrometheusMetricsReporter(metrics, "postgres", "log_collector")

// 同一数据库的不同表也可以独立监控
userReporter := NewPrometheusMetricsReporter(metrics, "mysql", "user_sync")
productReporter := NewPrometheusMetricsReporter(metrics, "mysql", "product_sync")
```

## 性能考虑

1. **指标写入开销**：所有指标方法在 nil 检查后直接返回，无锁等待
2. **标签基数**：建议 instance_id 控制在 10-50 个以内
3. **直方图桶数**：默认 18 个桶，覆盖 0.5ms ~ 131s
4. **内存占用**：每个实例约 10KB（18个桶 × 4个直方图 × 平均开销）

## 常见问题

### Q: 如何区分同一数据库的多个表？

**A**: 使用不同的 instance_id：
```go
ordersReporter := NewPrometheusMetricsReporter(metrics, "mysql", "orders_writer")
usersReporter := NewPrometheusMetricsReporter(metrics, "mysql", "users_writer")
```

### Q: 是否支持动态创建实例？

**A**: 支持，但需注意标签基数控制。建议在应用启动时预先创建所有实例。

### Q: 如何在 Grafana 中过滤特定实例？

**A**: 使用变量：
```
Label: instance_id
Query: label_values(batchflow_execute_duration_seconds, instance_id)
```

## 参考资源

- [Prometheus 命名最佳实践](https://prometheus.io/docs/practices/naming/)
- [Grafana Dashboard 设计指南](https://grafana.com/docs/grafana/latest/dashboards/)
- [BatchFlow 核心库文档](../../README.md)
