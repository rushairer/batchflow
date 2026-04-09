# BatchFlow API 参考

本文档只描述当前仓库已经实现并建议公开使用的 API。

## 核心类型

### BatchFlow

```go
type BatchFlow struct { /* unexported */ }
```

主要方法：

```go
func NewBatchFlow(
	ctx context.Context,
	bufferSize uint32,
	flushSize uint32,
	flushInterval time.Duration,
	executor batchflow.BatchExecutor,
) *BatchFlow

func (b *BatchFlow) Submit(ctx context.Context, request *Request) error
func (b *BatchFlow) ErrorChan(size int) <-chan error
func (b *BatchFlow) Close() error
func (b *BatchFlow) Wait() error
func (b *BatchFlow) Done() <-chan struct{}
```

语义：

- `Submit` 只负责入队，不保证立即执行。
- `ErrorChan` 返回异步执行错误通道；首次调用决定缓冲大小。
- `Close` 幂等。首次调用会关闭输入并等待最终 flush 结束。
- `Wait` 只等待后台退出，不主动关闭输入。
- `Done` 在后台 pipeline 退出时关闭。

典型模式：

```go
if err := flow.Close(); err != nil {
	return err
}
```

```go
go func() {
	<-flow.Done()
	log.Println("batchflow stopped")
}()

if err := flow.Wait(); err != nil {
	return err
}
```

### PipelineConfig

```go
type PipelineConfig struct {
	BufferSize       uint32
	FlushSize        uint32
	FlushInterval    time.Duration
	Retry            RetryConfig
	Timeout          time.Duration
	MetricsReporter  MetricsReporter
	ConcurrencyLimit int
}
```

### RetryConfig

```go
type RetryConfig struct {
	Enabled     bool
	MaxAttempts int
	BackoffBase time.Duration
	MaxBackoff  time.Duration
	Classifier  func(error) (retryable bool, reason string)
}
```

说明：

- `MaxAttempts` 是总尝试次数，包含第一次执行。
- 默认分类器会把 `context.Canceled` / `context.DeadlineExceeded` 视为不可重试。
- `ObserveExecuteDuration` 会包含重试和退避时间。

## 推荐构造函数

```go
func NewMySQLBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow
func NewPostgreSQLBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow
func NewSQLiteBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow
func NewRedisBatchFlow(ctx context.Context, db *redis.Client, config PipelineConfig) *BatchFlow
```

扩展入口：

```go
type BatchExecutor interface {
	ExecuteBatch(ctx context.Context, schema SchemaInterface, data []map[string]any) error
}
```

当你实现自定义执行器时，可以直接传给 `NewBatchFlow(...)`。

## Schema

```go
type SchemaInterface interface {
	Name() string
	Columns() []string
}

type Schema struct { /* unexported */ }
type SQLSchema struct { /* unexported */ }
```

构造函数：

```go
func NewSchema(name string, columns ...string) *Schema
func NewSQLSchema(name string, operationConfig SQLOperationConfig, columns ...string) *SQLSchema
```

SQL 冲突策略：

```go
type ConflictStrategy uint8

const (
	ConflictIgnore ConflictStrategy = iota
	ConflictReplace
	ConflictUpdate
)
```

对应配置值：

```go
var DefaultOperationConfig SQLOperationConfig
var ConflictIgnoreOperationConfig SQLOperationConfig
var ConflictReplaceOperationConfig SQLOperationConfig
var ConflictUpdateOperationConfig SQLOperationConfig
```

## Request

```go
type Request struct { /* unexported */ }
```

构造与常用方法：

```go
func NewRequest(schema SchemaInterface) *Request
func (r *Request) Schema() SchemaInterface
func (r *Request) Columns() map[string]any
func (r *Request) Validate() error

func (r *Request) SetInt(name string, value int) *Request
func (r *Request) SetInt8(name string, value int8) *Request
func (r *Request) SetInt16(name string, value int16) *Request
func (r *Request) SetInt32(name string, value int32) *Request
func (r *Request) SetInt64(name string, value int64) *Request
func (r *Request) SetUint(name string, value uint) *Request
func (r *Request) SetUint8(name string, value uint8) *Request
func (r *Request) SetUint16(name string, value uint16) *Request
func (r *Request) SetUint32(name string, value uint32) *Request
func (r *Request) SetUint64(name string, value uint64) *Request
func (r *Request) SetFloat32(name string, value float32) *Request
func (r *Request) SetFloat64(name string, value float64) *Request
func (r *Request) SetString(name string, value string) *Request
func (r *Request) SetBool(name string, value bool) *Request
func (r *Request) SetTime(name string, value time.Time) *Request
func (r *Request) SetBytes(name string, value []byte) *Request
func (r *Request) SetNull(name string) *Request
func (r *Request) Set(name string, value any) *Request
```

注意：

- 当前公开通用 setter 是 `Set(...)`，不是 `SetAny(...)`。
- 基础整数类型优先使用对应的 `SetInt...` / `SetUint...` 便捷方法，减少调用侧手动转换。
- `Validate()` 会验证 schema 声明的列是否全部赋值。

## 执行器

推荐执行器构造：

```go
func NewSQLThrottledBatchExecutorWithDriver(db *sql.DB, driver SQLDriver) *ThrottledBatchExecutor
func NewRedisThrottledBatchExecutor(client *redis.Client) *ThrottledBatchExecutor
func NewRedisThrottledBatchExecutorWithDriver(client *redis.Client, driver RedisDriver) *ThrottledBatchExecutor
```

可选能力：

```go
func (e *ThrottledBatchExecutor) WithRetryConfig(cfg RetryConfig) *ThrottledBatchExecutor
func (e *ThrottledBatchExecutor) WithConcurrencyLimit(limit int) *ThrottledBatchExecutor
func (e *ThrottledBatchExecutor) WithMetricsReporter(reporter MetricsReporter) *ThrottledBatchExecutor
func (e *ThrottledBatchExecutor) MetricsReporter() MetricsReporter
```

## Metrics 接口

### MetricsReporter

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

约定：

- `ObserveBatchSize` 表示单个 schema 执行批的大小。
- `ObserveExecuteDuration` 只表示执行耗时，不应该再重复记录 batch size。

### PipelineMetricsReporter

```go
type PipelineMetricsReporter interface {
	ObserveDequeueLatency(d time.Duration)
	ObserveProcessDuration(d time.Duration, status string)
	IncDropped(reason string)
}
```

约定：

- `ObserveDequeueLatency` 当前由 BatchFlow 自己采样，不依赖 go-pipeline 原生 hook。
- `IncDropped` 当前用于错误通道写满导致的丢弃。

### BatchFlowMetricsReporter

```go
type BatchFlowMetricsReporter interface {
	IncSubmitRejected(reason string)
	ObservePipelineFlushSize(n int)
	ObserveSchemaGroupsPerFlush(n int)
}
```

适用场景：

- 需要观察被拒绝的 `Submit` 次数和原因。
- 需要区分“整次 flush 输入大小”和“单个 schema 执行批大小”。
- 需要了解一次 flush 的拆组复杂度。

## 示例

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)

flow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
defer flow.Close()

schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
req := batchflow.NewRequest(schema).
	SetUint64("id", 1).
	SetString("name", "alice")

if err := flow.Submit(ctx, req); err != nil {
	return err
}
```

推荐继续阅读：

- [配置说明](configuration.md)
- [使用示例](../guides/examples.md)
- [Metrics 规格](../guides/metrics-spec.md)
