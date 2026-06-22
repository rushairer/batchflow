# BatchFlow API 参考

本文档只描述当前仓库已经实现并建议公开使用的 API。

API 稳定性说明见 [API Stability Policy](../development/api-stability.md)。

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
	Observability    ObservabilityConfig
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
- 默认错误分类由 `ClassifyError(err)` 提供，reason 使用低基数字典，例如 `deadlock`、`lock_timeout`、`timeout`、`connection`、`io`、`duplicate_key`、`syntax`、`non_retryable`。
- `ObserveExecuteDuration` 会包含重试和退避时间。

错误分类扩展接口：

```go
type ErrorClassifier interface {
	Classify(error) (retryable bool, reason string, ok bool)
}

type ErrorClassifierFunc func(error) (retryable bool, reason string, ok bool)

func RegisterErrorClassifier(classifier ErrorClassifier) func()
func ClassifyError(err error) (retryable bool, reason string)
```

自定义 classifier 会在内置 MySQL/PostgreSQL 结构化错误识别之后、字符串 fallback 之前执行。

错误原因完整列表见 [Error Classification](../guides/error-classification.md)。

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

SQL 操作配置：

```go
type SQLOperationConfig struct {
	ConflictStrategy ConflictStrategy
	ConflictColumns  []string
	UpdateColumns    []string
	DeduplicateByConflictColumns bool
}

func (c SQLOperationConfig) WithConflictColumns(cols ...string) SQLOperationConfig
func (c SQLOperationConfig) WithUpdateColumns(cols ...string) SQLOperationConfig
func (c SQLOperationConfig) WithDeduplicateByConflictColumns(enabled bool) SQLOperationConfig
```

- `ConflictColumns` 用于 PostgreSQL/SQLite 的 `ON CONFLICT (...)` 目标，也用于批内同键合并；为空时兼容旧行为，使用 schema 第一列。
- `UpdateColumns` 仅限制 `ConflictUpdate` 更新列；为空时更新所有非冲突列。
- `DeduplicateByConflictColumns` 默认开启，避免 PostgreSQL 同一批次重复冲突键导致一次 upsert 影响同一行多次。
- PostgreSQL 的 `ConflictReplace` 是 upsert 覆盖语义：冲突时更新所有非冲突列，不模拟 MySQL `REPLACE INTO` 的 delete+insert 语义。

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
- `Columns()` 返回当前列数据的副本；修改返回值不会影响 request 内部状态。
- 基础整数类型优先使用对应的 `SetInt...` / `SetUint...` 便捷方法，减少调用侧手动转换。
- `Validate()` 会验证 schema 声明的列是否全部赋值。

## Batch 与 Coalescer

通用批数据类型：

```go
type Record = map[string]any
type Batch = []Record
```

批内合并接口：

```go
type CoalesceStrategy string

const (
	CoalesceDisabled           CoalesceStrategy = "disabled"
	CoalesceKeepFirst          CoalesceStrategy = "keep_first"
	CoalesceKeepLast           CoalesceStrategy = "keep_last"
	CoalesceMergePresentFields CoalesceStrategy = "merge_present_fields"
)

type Coalescer interface {
	Coalesce(ctx context.Context, schema SchemaInterface, batch Batch) (CoalesceResult, error)
}

func NewKeyCoalescer(strategy CoalesceStrategy, keyColumns ...string) *KeyCoalescer
```

`PipelineConfig.Coalescer` 可用于 Redis、HTTP、MongoDB 等非 SQL 后端的同批次同 key 合并。SQL 默认仍由 `SQLOperationConfig` 的 `ConflictColumns` 控制，保留 `ConflictIgnore` / `ConflictUpdate` / `ConflictReplace` 的数据库语义和 dry-run 统计。

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
func (e *ThrottledBatchExecutor) WithCoalescer(coalescer Coalescer) *ThrottledBatchExecutor
func (e *ThrottledBatchExecutor) MetricsReporter() MetricsReporter
func (e *ThrottledBatchExecutor) WithObserver(observer Observer) *ThrottledBatchExecutor
func (e *ThrottledBatchExecutor) WithObservability(config ObservabilityConfig) *ThrottledBatchExecutor
```

## 通用 Dry Run 与错误诊断

Backend-neutral 预览接口：

```go
type OperationPreview struct {
	Backend     string
	Operation   string
	Schema      string
	InputItems  int
	OutputItems int
	ArgCount    int
	Fingerprint string
	Attributes  map[string]any
}

type OperationPreviewer interface {
	GenerateOperationPreview(ctx context.Context, schema SchemaInterface, data []map[string]any) (Operations, OperationPreview, error)
}
```

自定义 processor 实现 `OperationPreviewer` 后，框架会自动把 preview 用于结构化日志和指标。`Attributes` 只放低基数字段或计数，不要放原始 payload、SQL 参数、Redis key、HTTP body。

通用错误：

```go
type BatchError struct {
	Stage       string
	Backend     string
	Schema      string
	BatchSize   int
	Fingerprint string
	Attributes  map[string]any
	Cause       error
}
```

执行失败时可用 `errors.As(err, *BatchError)` 提取 backend、stage、schema、fingerprint 和安全 attributes。

Observer / slog：

```go
type BatchEvent struct { /* stage, backend, schema, status, duration, fingerprint, err */ }

type Observer interface {
	OnBatchEvent(ctx context.Context, event BatchEvent)
}

type ObservabilityConfig struct {
	Observer           Observer
	Logger             *slog.Logger
	Sampler            Sampler
	Redactor           Redactor
	SlowBatchThreshold time.Duration
}
```

`NewSlogObserver` 提供结构化日志，默认错误全量采样、慢批次采样，并按字段名脱敏 `password`、`token`、`secret`、`email`、`phone` 等常见敏感字段。

## SQL Dry Run 与错误诊断

最终 SQL 预览：

```go
type SQLPreview struct {
	Table            string
	SQL              string
	Args             []any
	ArgsCount        int
	Columns          []string
	ConflictStrategy ConflictStrategy
	ConflictColumns  []string
	UpdateColumns    []string
	DedupStats       SQLDedupStats
	Fingerprint      string
}

type SQLDedupStats struct {
	InputRows        int
	OutputRows       int
	DeduplicatedRows int
	MergedRows       int
}

func GenerateSQLPreview(ctx context.Context, driver SQLDriver, schema *SQLSchema, data []map[string]any) (SQLPreview, error)
func FingerprintSQL(sqlText string) string
```

`GenerateSQLPreview` 不访问数据库，返回的 SQL 与真实执行路径使用同一套 driver 生成逻辑。`Args` 可能包含敏感数据，生产日志建议只记录 `ArgsCount`、`Fingerprint`、冲突列、更新列和去重统计。

`SQLBatchProcessor.ExecuteOperations` 兼容以 `SQLPreview` 作为首个 operation 的直接执行形式；常规生成路径仍返回 SQL 字符串和参数列表。

SQL 结构化错误：

```go
type SQLStage string

const (
	SQLStageValidate SQLStage = "validate"
	SQLStageGenerate SQLStage = "generate"
	SQLStageExecute  SQLStage = "execute"
)

type SQLError struct {
	Stage            SQLStage
	Table            string
	BatchSize        int
	ConflictStrategy ConflictStrategy
	ConflictColumns  []string
	UpdateColumns    []string
	SQLFingerprint   string
	ArgsCount        int
	Cause            error
}
```

执行失败时可用 `errors.As(err, *SQLError)` 提取阶段、表名、冲突键、SQL 指纹和参数数量。默认错误文本不包含参数值。

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
- `ObserveExecuteDuration` 的第一个参数历史命名为 `table`；非 SQL 后端按 schema 名传入。

### SQLMetricsReporter

```go
type SQLMetricsReporter interface {
	ObserveSQLGenerated(table string, inputRows, outputRows, argsCount int)
	ObserveSQLDeduplicated(table string, strategy ConflictStrategy, deduplicatedRows, mergedRows int)
	IncSQLError(table string, stage SQLStage, reason string)
}
```

这是 SQL-only 细节扩展接口，保留用于 SQL 诊断和兼容官方 Prometheus 示例。通用后端优先实现 `OperationMetricsReporter`。

### OperationMetricsReporter

```go
type OperationMetricsReporter interface {
	ObserveOperationGenerated(preview OperationPreview)
	IncOperationError(schema string, backend string, stage string, reason string)
}
```

这是推荐的 backend-neutral 指标扩展，适用于 SQL、Redis 和自定义 processor。官方 Prometheus 示例已经实现该接口。

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
