# BatchFlow 配置说明

本文档描述业务侧真正会用到的配置项。集成测试和 Docker 环境配置请看 [集成测试文档](../guides/integration-tests.md)。

## PipelineConfig

```go
type PipelineConfig struct {
	BufferSize               uint32
	FlushSize                uint32
	FlushInterval            time.Duration
	MaxConcurrentFlushes     uint32
	DrainOnCancel            bool
	DrainGracePeriod         time.Duration
	FinalFlushOnCloseTimeout time.Duration
	Retry                    RetryConfig
	Timeout                  time.Duration
	MetricsReporter          MetricsReporter
	Observability            ObservabilityConfig
	ConcurrencyLimit         int
	Coalescer                Coalescer
}
```

`DefaultPipelineConfig()` 提供生产可用默认值。`NewBatchFlowWithConfig(ctx, BatchFlowConfig{...})` 会执行配置校验；旧构造函数仍保持兼容。

`Coalescer` 用于非 SQL 后端的批内同 key 合并，例如 Redis、HTTP、MongoDB 或自定义 API。SQL 默认仍使用 `SQLOperationConfig.ConflictColumns` 做 conflict-key 合并，避免丢失 SQL dry-run 里的 dedup 统计。

## SQLOperationConfig

SQL schema 的冲突写入行为由 `SQLOperationConfig` 控制：

```go
type SQLOperationConfig struct {
	ConflictStrategy ConflictStrategy
	ConflictColumns  []string
	UpdateColumns    []string
	DeduplicateByConflictColumns bool
}
```

推荐使用链式方法配置：

```go
config := batchflow.ConflictUpdateOperationConfig.
	WithConflictColumns("tenant_id", "user_id").
	WithUpdateColumns("name", "email")

schema := batchflow.NewSQLSchema(
	"users",
	config,
	"tenant_id", "user_id", "name", "email", "updated_at",
)
```

字段说明：

- `ConflictStrategy`：冲突处理策略，支持 `ConflictIgnore`、`ConflictUpdate`、`ConflictReplace`。
- `ConflictColumns`：冲突键，用于 PostgreSQL/SQLite `ON CONFLICT (...)`，也用于批内同键合并。未配置时兼容旧行为，使用 schema 第一列。
- `UpdateColumns`：仅对 `ConflictUpdate` 生效；未配置时更新所有非冲突列。
- `DeduplicateByConflictColumns`：默认开启。开启后同一批次内相同冲突键会先合并，避免 PostgreSQL 同一条 SQL 多次影响同一行。

数据库差异：

- PostgreSQL `ConflictReplace` 是 upsert 覆盖：`ON CONFLICT (...) DO UPDATE SET ...`，更新所有非冲突列。
- MySQL `ConflictReplace` 是原生 `REPLACE INTO`，语义上可能触发 delete+insert。
- MySQL `ConflictUpdate` 使用 `ON DUPLICATE KEY UPDATE`，配置 `ConflictColumns` 后不会更新这些冲突列。

## 字段说明

### BufferSize

- 含义：内部输入通道容量。
- 影响：越大越能吸收提交峰值，但也会延长尾部排队时间。
- 建议：先从 `FlushSize` 的 `2x ~ 10x` 开始。

### FlushSize

- 含义：达到多少条请求后立即触发 flush。
- 影响：越大吞吐通常越高，但单次执行延迟和内存占用也会升高。
- 建议：
  - OLTP 写入：`100 ~ 500`
  - 日志/批同步：`500 ~ 2000`

### FlushInterval

- 含义：即使没有满批，也会在该时间间隔触发 flush。
- 影响：越短越偏实时，越长越偏吞吐。
- 建议：
  - 实时写入：`50ms ~ 200ms`
  - 吞吐优先：`200ms ~ 1s`

### Timeout

- 含义：单次处理器执行超时。
- 作用范围：作用在 SQL/Redis processor 执行阶段。
- 建议：只有当你希望把慢执行快速失败并交给重试分类器时才开启。

### Retry

- 含义：执行器级重试配置。
- 建议默认值：

```go
Retry: batchflow.RetryConfig{
	Enabled:     true,
	MaxAttempts: 3,
	BackoffBase: 20 * time.Millisecond,
	MaxBackoff:  500 * time.Millisecond,
}
```

说明：

- `MaxAttempts` 包含第一次执行。
- 默认分类器把 `context.Canceled` / `context.DeadlineExceeded` 视为不可重试。
- 默认 reason 来自 `batchflow.ClassifyError(err)`；自定义 `Classifier` 建议返回同一套低基数字典。
- 如果你要区分内部超时和外部取消，请自定义 `Classifier`。

### MetricsReporter

- 含义：可选指标上报器。
- 使用方式：在 `PipelineConfig` 中直接传入。
- 推荐：优先使用 `examples/metrics/prometheus` 中的官方示例实现。
- 通用场景：官方 Prometheus reporter 实现了 `OperationMetricsReporter`，会额外上报 SQL、Redis、自定义 processor 的 operation 生成数量、参数数量和错误。
- SQL 场景：官方 Prometheus reporter 仍实现 `SQLMetricsReporter`，保留 SQL 专用指标兼容。

### Observability

`ObservabilityConfig` 用于配置结构化日志、采样和脱敏：

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

flow := batchflow.NewRedisBatchFlow(ctx, redisClient, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	MetricsReporter:  reporter,
	Observability: batchflow.ObservabilityConfig{
		Logger:             logger,
		Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
		Redactor:           batchflow.DefaultRedactor(),
		SlowBatchThreshold: 500 * time.Millisecond,
	},
})
```

默认建议：

- 错误事件全量记录。
- 成功事件按低比例采样，或只记录慢批次。
- 不记录原始 row、SQL args、Redis key、HTTP body。
- 自定义 processor 实现 `OperationPreviewer`，把 backend、operation、fingerprint 和安全 attributes 提供给框架。

### SQL Dry Run

`GenerateSQLPreview` 用于在执行前检查最终 SQL：

```go
preview, err := batchflow.GenerateSQLPreview(ctx, batchflow.DefaultPostgreSQLDriver, schema, rows)
if err != nil {
	var sqlErr *batchflow.SQLError
	if errors.As(err, &sqlErr) {
		log.Printf("sql generate failed: stage=%s table=%s conflict=%v update=%v args=%d cause=%v",
			sqlErr.Stage, sqlErr.Table, sqlErr.ConflictColumns, sqlErr.UpdateColumns, sqlErr.ArgsCount, sqlErr.Cause)
	}
	return err
}

log.Printf("sql dry-run: table=%s fingerprint=%s args=%d input=%d output=%d dedup=%d merged=%d sql=%s",
	preview.Table,
	preview.Fingerprint,
	preview.ArgsCount,
	preview.DedupStats.InputRows,
	preview.DedupStats.OutputRows,
	preview.DedupStats.DeduplicatedRows,
	preview.DedupStats.MergedRows,
	preview.SQL,
)
```

注意：`preview.Args` 包含原始参数值，可能有 PII 或业务敏感数据。生产日志默认不要打印 `Args`。

### ConcurrencyLimit

- 含义：限制 `ExecuteBatch` 的并发度。
- 作用点：执行器入口，不是 Submit 阶段。
- 语义：`<= 0` 表示不限流。
- 建议：数据库较脆弱或多 schema 并发场景，优先从 `4 ~ 8` 开始。

## 调优组合建议

### 低延迟优先

```go
batchflow.PipelineConfig{
	BufferSize:       500,
	FlushSize:        100,
	FlushInterval:    50 * time.Millisecond,
	ConcurrencyLimit: 4,
}
```

### 吞吐优先

```go
batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        500,
	FlushInterval:    200 * time.Millisecond,
	ConcurrencyLimit: 8,
}
```

### 有重试和指标

```go
batchflow.PipelineConfig{
	BufferSize:       2000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
	MetricsReporter:  reporter,
	Retry: batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	},
}
```

## 生命周期配置建议

- 业务代码结束前一定要调用 `Close()`，保证最后一批数据被 flush。
- 如果你只想等后台退出而不关闭输入，用 `Wait()`。
- 不要把 `FlushInterval` 当成唯一的收尾机制；收尾应该由 `Close()` 驱动。
