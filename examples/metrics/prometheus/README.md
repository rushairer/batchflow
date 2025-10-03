# Prometheus 指标（开箱即用示例）

本示例提供一套与 BatchFlow 对齐的 Prometheus 指标与 Reporter，支持常用可配置项，做到“拿来即用、按需裁剪”。

- 指标实现：examples/metrics/prometheus/prometheus_metrics.go
- Reporter 实现：examples/metrics/prometheus/prometheus_reporter.go
- 对应仪表板：test/integration/grafana/provisioning/dashboards/batchflow-performance.json（单一 Dashboard）

## 功能与特性
- 含重试相关指标：error_type 使用 "retry:*"/"final:*" 前缀，便于面板聚合
- 直方图：入队延迟、攒批耗时、执行耗时（覆盖重试/退避）、批大小
- 仪表：并发度、队列长度、在途批次
- 可配置项：命名空间/子系统、常量标签、是否启用 test_name/table 维度、Buckets

## 快速开始

```go
import (
    "context"
    "log"
    bsql "github.com/rushairer/batchflow"
    pm "github.com/rushairer/batchflow/examples/metrics/prometheus"
)

func main() {
    // 1) 创建指标
    m := pm.NewMetrics(pm.Options{
        Namespace:      "batchflow",
        IncludeTestName: true, // 可选
        IncludeTable:    false,
        ConstLabels:     map[string]string{"env":"dev"},
    })

    // 2) 启动 /metrics
    if err := m.StartServer(2112); err != nil {
        log.Fatal(err)
    }
    defer m.StopServer(context.Background())

    // 3) 创建 Reporter 并绑定到 BatchFlow/Executor
    reporter := pm.NewReporter(m, "mysql", "batch_insert")

    exec := bsql.NewSQLThrottledBatchExecutorWithDriver(nil, bsql.DefaultMySQLDriver).
        WithMetricsReporter(reporter) // executor 指标

    batch := bsql.NewBatchFlow(exec) // BatchFlow 内部会使用 reporter 进行观测

    _ = batch // 按需提交任务...
}
```

## 配置说明（Options）
- Namespace/Subsystem：Prometheus 命名空间/子系统
- ConstLabels：追加到所有指标的常量标签（如 env/region/tenant）
- IncludeTestName：是否启用 test_name 维度（集成/压测推荐开启）
- IncludeTable：是否启用 table 维度（注意基数）
- 各直方图 Buckets：Enqueue/Assemble/Execute/BatchSize

## 与仪表板的配合
- 已提供单一 Dashboard：test/integration/grafana/provisioning/dashboards/batchflow-performance.json
- 面板依赖的关键指标与标签：
  - errors_total{error_type=~"retry:.*|final:.*"}
  - enqueue_latency_seconds, batch_assemble_duration_seconds, execute_duration_seconds, batch_size
  - executor_concurrency, pipeline_queue_length, inflight_batches
  - 维度：database（必）、test_name（若开启）

## 异常与性能
- 所有 Reporter 方法在 m=nil 时直接返回，避免空指针
- 指标写入为加锁/原子开销，谨慎开启高基数标签（如 table）
- 直方图 Buckets 越多、写入越频繁，资源消耗越大；生产中按 P95/P99 目标合理配置