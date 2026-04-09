# 自定义 MetricsReporter

如果你不想直接使用官方 Prometheus 示例，可以自己实现指标接口。

## 需要实现哪些接口

### 必选：MetricsReporter

```go
type MetricsReporter interface {
	ObserveEnqueueLatency(d time.Duration)
	ObserveBatchAssemble(d time.Duration)
	ObserveExecuteDuration(table string, n int, d time.Duration, status string)

	ObserveBatchSize(n int)
	IncError(table string, typ string)
	SetConcurrency(n int)
	SetQueueLength(n int)
	IncInflight()
	DecInflight()
}
```

语义约定：

- `ObserveBatchSize` 表示单个 schema 执行批大小。
- `ObserveExecuteDuration` 不应该再重复记录 batch size。

### 可选：PipelineMetricsReporter

```go
type PipelineMetricsReporter interface {
	ObserveDequeueLatency(d time.Duration)
	ObserveProcessDuration(d time.Duration, status string)
	IncDropped(reason string)
}
```

### 可选：BatchFlowMetricsReporter

```go
type BatchFlowMetricsReporter interface {
	IncSubmitRejected(reason string)
	ObservePipelineFlushSize(n int)
	ObserveSchemaGroupsPerFlush(n int)
}
```

## 最小示例

```go
type MyReporter struct{}

func (*MyReporter) ObserveEnqueueLatency(time.Duration) {}
func (*MyReporter) ObserveBatchAssemble(time.Duration)  {}
func (*MyReporter) ObserveExecuteDuration(string, int, time.Duration, string) {}
func (*MyReporter) ObserveBatchSize(int)               {}
func (*MyReporter) IncError(string, string)            {}
func (*MyReporter) SetConcurrency(int)                 {}
func (*MyReporter) SetQueueLength(int)                 {}
func (*MyReporter) IncInflight()                       {}
func (*MyReporter) DecInflight()                       {}
```

## 完整示例

```go
type MyReporter struct{}

func (*MyReporter) ObserveEnqueueLatency(time.Duration) {}
func (*MyReporter) ObserveBatchAssemble(time.Duration)  {}
func (*MyReporter) ObserveExecuteDuration(string, int, time.Duration, string) {}
func (*MyReporter) ObserveBatchSize(int)               {}
func (*MyReporter) IncError(string, string)            {}
func (*MyReporter) SetConcurrency(int)                 {}
func (*MyReporter) SetQueueLength(int)                 {}
func (*MyReporter) IncInflight()                       {}
func (*MyReporter) DecInflight()                       {}

func (*MyReporter) ObserveDequeueLatency(time.Duration)         {}
func (*MyReporter) ObserveProcessDuration(time.Duration, string) {}
func (*MyReporter) IncDropped(string)                           {}

func (*MyReporter) IncSubmitRejected(string)          {}
func (*MyReporter) ObservePipelineFlushSize(int)      {}
func (*MyReporter) ObserveSchemaGroupsPerFlush(int)   {}
```

## 注入方式

推荐通过 `PipelineConfig` 传入：

```go
flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:      5000,
	FlushSize:       200,
	FlushInterval:   100 * time.Millisecond,
	MetricsReporter: &MyReporter{},
})
defer flow.Close()
```

如果你直接构造执行器，也可以：

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
	WithMetricsReporter(&MyReporter{})

flow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
defer flow.Close()
```

## 建议

- 没有明确需求时，只实现 `MetricsReporter` 即可。
- 只有在你确实需要区分 flush 输入和 schema 执行批大小时，才实现 `BatchFlowMetricsReporter`。
- 如果你已经使用 Prometheus，优先复用 `examples/metrics/prometheus`，不要重复造轮子。
