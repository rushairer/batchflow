# Prometheus 指标（开箱即用示例）

> 版本：v2.0  
> 更新时间：2025-12-02

本示例提供一套与 BatchFlow 对齐的 Prometheus 指标与 Reporter，支持多实例隔离和管道级指标，做到"拿来即用、按需裁剪"。

- 指标实现：`prometheus_metrics.go`
- Reporter 实现：`prometheus_reporter.go`
- 对应仪表板：`test/integration/grafana/provisioning/dashboards/batchflow-performance.json`

## 功能与特性

### 核心特性（v2.0）

- ✅ **多实例支持**：通过 `instance_id` 标签隔离多个 BatchFlow 实例
- ✅ **完整指标覆盖**：包含入队、组装、执行、错误、状态等全流程指标
- ✅ **管道级指标**：可选的 `PipelineMetricsReporter` 接口支持
- ✅ **重试感知**：error_type 使用 `retry:*`/`final:*` 前缀，便于面板聚合
- ✅ **灵活配置**：可选 instance_id/table 维度、自定义直方图桶、常量标签

### 指标分类

#### 1. Histogram（分布统计）
- `enqueue_latency_seconds`：入队延迟
- `batch_assemble_duration_seconds`：攒批耗时
- `execute_duration_seconds`：执行耗时（含重试/退避，按 status 区分）
- `batch_size`：批大小分布
- `pipeline_dequeue_latency_seconds`：出队延迟（可选）
- `pipeline_process_duration_seconds`：管道处理耗时（可选）

#### 2. Gauge（状态观测）
- `executor_concurrency`：执行器并发度
- `pipeline_queue_length`：队列长度
- `inflight_batches`：在途批次数

#### 3. Counter（累计计数）
- `errors_total`：错误总数（按 error_type）
- `pipeline_dropped_total`：丢弃事件数（可选）

## 快速开始

### 基础用法

```go
import (
    "context"
    "log"
    batchflow "github.com/rushairer/batchflow"
    pm "github.com/rushairer/batchflow/examples/metrics/prometheus"
)

func main() {
    // 1) 创建指标（使用默认配置）
    m := pm.NewMetrics(pm.Options{
        Namespace:         "batchflow",
        IncludeInstanceID: true,  // 推荐：支持多实例
        EnablePipelineMetrics: true, // 可选：启用管道级指标
    })

    // 2) 启动 /metrics 端点
    if err := m.StartServer(2112); err != nil {
        log.Fatal(err)
    }
    defer m.StopServer(context.Background())

    // 3) 为不同业务创建独立 Reporter
    orderReporter := pm.NewReporter(m, "mysql", "order_writer")
    logReporter := pm.NewReporter(m, "postgres", "log_collector")

    // 4) 绑定到 BatchFlow
    orderFlow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
        MetricsReporter: orderReporter,
        // ...
    })

    _ = orderFlow // 使用...
}
```

### 高级配置

```go
m := pm.NewMetrics(pm.Options{
    // 指标命名
    Namespace:   "myapp",
    Subsystem:   "batch",
    ConstLabels: map[string]string{
        "env":    "prod",
        "region": "us-west",
    },

    // 标签维度
    IncludeInstanceID: true,  // 推荐开启
    IncludeTable:      false, // 谨慎开启（高基数）

    // 管道级指标
    EnablePipelineMetrics: true,

    // 自定义直方图桶
    EnqueueBuckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms ~ 16s
    AssembleBuckets:  prometheus.ExponentialBuckets(0.001, 2, 15),
    ExecuteBuckets:   prometheus.ExponentialBuckets(0.001, 2, 18), // 1ms ~ 131s
    BatchSizeBuckets: prometheus.ExponentialBuckets(10, 2, 10),    // 10 ~ 5120
})
```

## 配置说明（Options）

### 指标命名

| 字段 | 说明 | 默认值 | 示例 |
|------|------|--------|------|
| `Namespace` | Prometheus 命名空间 | "" | "batchflow" |
| `Subsystem` | Prometheus 子系统 | "" | "mysql" |
| `ConstLabels` | 常量标签（所有指标） | nil | `{"env":"prod"}` |

### 标签维度

| 字段 | 说明 | 推荐 | 基数影响 |
|------|------|------|---------|
| `IncludeInstanceID` | 启用 instance_id 标签 | ✅ 是 | 低（10-50） |
| `IncludeTable` | 启用 table 标签 | ⚠️ 谨慎 | 中（取决于表数量） |

### 直方图桶配置

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `EnqueueBuckets` | 入队延迟桶 | 0.5ms ~ 131s（18个桶） |
| `AssembleBuckets` | 组装耗时桶 | 0.5ms ~ 131s（18个桶） |
| `ExecuteBuckets` | 执行耗时桶 | 0.5ms ~ 131s（18个桶） |
| `BatchSizeBuckets` | 批大小桶 | 1 ~ 4096（12个桶） |

### 管道级指标

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `EnablePipelineMetrics` | 启用 PipelineMetricsReporter | false |

## 多实例使用示例

### 场景1：同一数据库的多个业务

```go
// 创建全局指标收集器
var globalMetrics = pm.NewMetrics(pm.Options{
    Namespace:         "myapp",
    IncludeInstanceID: true,
})

// 订单写入
orderReporter := pm.NewReporter(globalMetrics, "mysql", "order_writer")
orderFlow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
    MetricsReporter: orderReporter,
})

// 用户同步
userReporter := pm.NewReporter(globalMetrics, "mysql", "user_sync")
userFlow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
    MetricsReporter: userReporter,
})

// Prometheus 查询
// sum(rate(myapp_batch_size_sum{instance_id="order_writer"}[5m]))
// sum(rate(myapp_batch_size_sum{instance_id="user_sync"}[5m]))
```

### 场景2：多数据库混合

```go
// MySQL 订单
mysqlReporter := pm.NewReporter(globalMetrics, "mysql", "order_writer")

// PostgreSQL 日志
pgReporter := pm.NewReporter(globalMetrics, "postgres", "log_collector")

// Redis 缓存更新
redisReporter := pm.NewReporter(globalMetrics, "redis", "cache_updater")

// Grafana 按数据库聚合
// sum(rate(myapp_batch_size_sum[5m])) by (database)

// Grafana 按实例查看
// sum(rate(myapp_batch_size_sum[5m])) by (database, instance_id)
```

## Grafana 仪表板

### 关键查询模板

#### 1. 执行耗时（P99，按状态）

**成功执行**：
```promql
histogram_quantile(0.99,
  sum(rate(batchflow_execute_duration_seconds_bucket{
    database="mysql",
    instance_id="order_writer",
    status="success"
  }[5m])) by (le)
)
```

**失败执行**：
```promql
histogram_quantile(0.99,
  sum(rate(batchflow_execute_duration_seconds_bucket{
    database="mysql",
    instance_id="order_writer",
    status="fail"
  }[5m])) by (le)
)
```

#### 2. 错误率（按类型）

**重试错误率**：
```promql
sum(rate(batchflow_errors_total{
  error_type=~"retry:.*"
}[5m])) by (database, instance_id, error_type)
```

**最终失败率**：
```promql
sum(rate(batchflow_errors_total{
  error_type=~"final:.*"
}[5m])) by (database, instance_id, error_type)
```

#### 3. 吞吐量

```promql
# 每秒处理记录数
sum(rate(batchflow_batch_size_sum[5m])) by (database, instance_id)

# 平均批大小
sum(rate(batchflow_batch_size_sum[5m])) by (database, instance_id)
/
sum(rate(batchflow_batch_size_count[5m])) by (database, instance_id)
```

## 接口实现检查表

### MetricsReporter（核心接口）

- [x] `ObserveEnqueueLatency(d time.Duration)`
- [x] `ObserveBatchAssemble(d time.Duration)`
- [x] `ObserveExecuteDuration(table string, n int, d time.Duration, status string)`
- [x] `ObserveBatchSize(n int)`
- [x] `IncError(table string, typ string)`
- [x] `SetConcurrency(n int)`
- [x] `SetQueueLength(n int)`
- [x] `IncInflight()`
- [x] `DecInflight()`

### PipelineMetricsReporter（可选扩展）

- [x] `ObserveDequeueLatency(d time.Duration)`
- [x] `ObserveProcessDuration(d time.Duration, status string)`
- [x] `IncDropped(reason string)`

## 性能考虑

### 内存占用

| 配置 | 每实例内存 | 说明 |
|------|-----------|------|
| 默认（18桶 × 4指标） | ~10KB | 推荐 |
| 启用管道级指标 | ~15KB | 额外3个指标 |
| 启用 table 标签（100表） | ~1MB | ⚠️ 高基数 |

### 标签基数

- ✅ **低基数**（推荐）：database + instance_id（< 50 组合）
- ⚠️ **中基数**（谨慎）：+ table（< 1000 组合）
- ❌ **高基数**（禁止）：request_id, timestamp

## 常见问题

### Q1: 如何区分同一数据库的多个表？

**A**: 使用不同的 `instance_id`：
```go
ordersReporter := pm.NewReporter(m, "mysql", "orders_writer")
usersReporter := pm.NewReporter(m, "mysql", "users_writer")
```

### Q2: 是否需要启用 table 标签？

**A**: 不推荐。理由：
1. 会导致基数爆炸（每个表都是独立时间序列）
2. 可以通过 `instance_id` 区分不同业务
3. 如果必须，建议使用常量标签：
```go
ordersMetrics := pm.NewMetrics(pm.Options{
    ConstLabels: map[string]string{"table": "orders"},
})
```

### Q3: 如何监控管道级性能？

**A**: 启用 `EnablePipelineMetrics`：
```go
m := pm.NewMetrics(pm.Options{
    EnablePipelineMetrics: true,
})
```

然后使用 `pipeline_dequeue_latency_seconds` 和 `pipeline_process_duration_seconds` 指标。

### Q4: 如何在生产环境使用？

**A**: 推荐配置：
```go
m := pm.NewMetrics(pm.Options{
    Namespace:   "myapp",
    ConstLabels: map[string]string{
        "env": "prod",
        "region": os.Getenv("REGION"),
    },
    IncludeInstanceID: true,
    IncludeTable:      false, // 避免高基数
    EnablePipelineMetrics: false, // 按需启用
})

// 启动独立的 metrics 端口
m.StartServer(9090)
```

## 迁移指南（v1.0 → v2.0）

### 代码修改

**之前（v1.0）**：
```go
reporter := pm.NewReporter(m, "mysql", "test1")
```

**之后（v2.0，无需修改）**：
```go
// 集成测试：保持不变
reporter := pm.NewReporter(m, "mysql", "test1")

// 生产环境：使用业务标识
reporter := pm.NewReporter(m, "mysql", "order_writer")
```

### Options 修改

**之前（v1.0）**：
```go
pm.Options{
    IncludeTestName: true,
}
```

**之后（v2.0）**：
```go
pm.Options{
    IncludeInstanceID: true, // 语义更清晰
}
```

### 指标标签变更

| v1.0 | v2.0 | 说明 |
|------|------|------|
| `test_name` | `instance_id` | 支持多实例 |
| - | `status` | 新增：success/fail |

## 参考资源

- [指标设计文档](../../test/integration/METRICS_DESIGN.md)
- [Grafana Dashboard 更新指南](../../test/integration/grafana/DASHBOARD_UPDATE_GUIDE.md)
- [BatchFlow 核心库文档](../../README.md)
- [Prometheus 命名最佳实践](https://prometheus.io/docs/practices/naming/)