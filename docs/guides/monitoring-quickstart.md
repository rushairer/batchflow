# 监控快速上手（Prometheus + Grafana）

适用对象：想要“开箱即用”查看 BatchFlow 性能指标（入队延迟、攒批耗时、执行耗时、批大小、队列长度、执行并发、在途批次）的用户。

## 一、启动 Prometheus 指标端点

示例（集成测试同款思路）：
```go
pm := integration.NewPrometheusMetrics()
go pm.StartServer(9090)
defer pm.StopServer()
```
确认浏览器打开 http://localhost:9090/metrics 能看到 batchflow_* 指标。

## 二、把 Reporter 注入执行器（务必在 NewBatchFlow 之前）

```go
exec := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, driver)
reporter := integration.NewPrometheusMetricsReporter(pm, "postgres", "user_batch") // database/test_name 标签
exec = exec.WithMetricsReporter(reporter)

bs := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, exec)
defer bs.Close()
```

提示：
- 默认使用 NoopMetricsReporter（零开销）。只有在你注入 Reporter 后，库内埋点才会真正上报。
- 一定要“先 WithMetricsReporter，再 NewBatchFlow”。NewBatchFlow 会尊重已注入的 Reporter；若未设置则仅在内部使用本地 Noop 兜底，不写回执行器。

## 三、导入 Grafana 面板

- 面板 JSON 已在仓库：test/integration/grafana/provisioning/dashboards/batchflow-performance.json
- 你可以：
  - 在现有 Grafana 中导入该 JSON
  - 或复用集成测试的 Grafana 配置启动，自动加载该面板

常见可视图表（中文标题）：
- 入队延迟（p50/p95）
- 攒批耗时（p50/p95）
- 执行耗时（p50/p95）
- 批大小分布
- 队列长度、执行并发度、在途批次数

## 常见问题排查

- 指标没有数据：
  - 是否在 NewBatchFlow 之前注入了 Reporter？
  - Prometheus /metrics 是否可访问？Grafana 数据源是否指向该 Prometheus？
  - 面板变量 database/test_name 是否包含当前值（例如 postgres）？
- 执行并发度为 0：
  - 表示未限流（不限流场景并发度 Gauge 为 0）。
  - 如需非 0 值，调用 WithConcurrencyLimit(8) 等。
  - 也可关注“在途批次数”图表，反映实时压力。