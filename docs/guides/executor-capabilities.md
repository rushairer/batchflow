# 执行器能力与度量接口（重构后）

目标：
- BatchExecutor 仅承载执行职责；
- 能力按需拆分：度量、并发限流等；
- 在需要链式的地方提供自类型泛型接口，避免污染最小接口。

核心接口
```go
type BatchExecutor interface {
    ExecuteBatch(ctx context.Context, schema *Schema, data []map[string]any) error
}

// 自类型泛型接口：返回实现者类型 T，便于链式
type MetricsCapable[T any] interface {
    WithMetricsReporter(MetricsReporter) T
    MetricsReporter() MetricsReporter
}
type ConcurrencyCapable[T any] interface {
    WithConcurrencyLimit(int) T
}
```

为什么 batchflow 内部仍使用只读探测
- Go 的泛型接口在类型断言时必须带具体类型实参。
- 在 batchflow.go（仅持有 BatchExecutor）无法统一断言 MetricsCapable[T]。
- 方案：使用只读探测（MetricsReporter()）判断是否已有 Reporter；若为 nil，使用本地 Noop 兜底，不强制写回。

使用方式示例
1) 具体类型链式
```go
exec := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
    WithConcurrencyLimit(8).
    WithMetricsReporter(promReporter)
_ = exec.ExecuteBatch(ctx, schema, rows)
```

2) 已实例化能力接口链式
```go
var mexec batchflow.MetricsCapable[*batchflow.ThrottledBatchExecutor] = exec
mexec.WithMetricsReporter(promReporter).
      WithConcurrencyLimit(4).
      ExecuteBatch(ctx, schema, rows)
```

3) 仅依赖最小接口（非链式，分行更清晰）
```go
var bexec batchflow.BatchExecutor = exec
// 在仅持有 BatchExecutor 时，按需通过类型断言到能力接口再配置：
if mexec, ok := exec.(batchflow.MetricsCapable[*batchflow.ThrottledBatchExecutor]); ok {
    mexec.WithMetricsReporter(promReporter)
}
_ = bexec.ExecuteBatch(ctx, schema, rows)
```

NoopMetricsReporter 仍有必要
- 兜底：当未设置/不支持时，保证指标调用安全（零开销）。
- 消除判空分支：避免在热点路径写 if reporter != nil。

迁移提示
- 文档与示例从 MetricsProvider 迁移为：运行时只读探测（MetricsReporter()）+ 本地 Noop 兜底，不强制写回。
- 若需要强制写回，请在构造期（NewXxx）或在具体类型上下文中配置（具备 T 的信息时）。