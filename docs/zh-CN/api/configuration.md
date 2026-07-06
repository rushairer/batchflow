# 配置说明

本文档是 [Configuration](../../api/configuration.md) 的中文镜像摘要。英文文档是主契约。

## 推荐入口

RC2 推荐使用干净 runtime API：

```go
cfg := batchflow.DefaultConfig(executor)
flow, err := batchflow.New(ctx, cfg)
```

版本只体现在 module path `github.com/rushairer/batchflow/v2` 和 tag 中，不进入 API 名。

## Config

```go
type Config struct {
	Pipeline PipelineConfig
	Runtime  RuntimeConfig
	Executor BatchExecutor
}
```

- `Pipeline`：队列、flush、重试、指标、观测、执行器并发。
- `Runtime`：分片、路由、背压、内存保护、自适应调参策略。
- `Executor`：SQL、Redis、COPY FROM 或自定义批写入后端。

## PipelineConfig

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
	Coalescer        Coalescer
}
```

`Coalescer` 用于 Redis、HTTP、MongoDB、队列等非 SQL 后端的批内同 key 合并。SQL 后端使用 `SQLOperationConfig.ConflictColumns` 执行 conflict-key 合并，并在 SQL dry-run 中输出统计。

## RuntimeConfig

```go
type RuntimeConfig struct {
	ShardCount   uint32
	Routing      ShardRoutingPolicy
	ShardKeyFunc ShardKeyFunc

	Backpressure BackpressureConfig
	MemoryLimit  MemoryLimitConfig
	Adaptive     AdaptiveTuningConfig
}
```

建议起点：

- 低延迟在线服务：`ShardCount=2`
- 普通 SQL batch：`ShardCount=4`
- COPY FROM / Hologres：`ShardCount=8`

## BackpressureConfig

```go
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
	Enabled:       true,
	Mode:          batchflow.BackpressureTimeout,
	HighWatermark: 8000,
	Timeout:       500 * time.Millisecond,
}
```

- `BackpressureTimeout`：生产默认推荐。
- `BackpressureReject`：适合在线 API，上游可以快速重试。
- `BackpressureBlock`：适合离线任务，但可能掩盖下游饱和。

## MemoryLimitConfig

```go
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
	Enabled:         true,
	MaxQueueBytes:   512 << 20,
	AvgRequestBytes: 512,
	Mode:            batchflow.BackpressureTimeout,
	Timeout:         500 * time.Millisecond,
}
```

推荐公式：

```text
MaxQueueBytes = BufferSize * ShardCount * AvgRequestBytes * 1.5
```

宽行数据可把 `AvgRequestBytes` 调到 `1024` 或 `2048`。

## AdaptiveTuningConfig

RC2 的 `AdaptiveTuner` 是策略对象，只输出推荐值，不自动热改运行时配置。

```go
cfg.Runtime.Adaptive = batchflow.AdaptiveTuningConfig{
	Enabled:             true,
	MinFlushSize:        100,
	MaxFlushSize:        5000,
	MinFlushInterval:    10 * time.Millisecond,
	MaxFlushInterval:    200 * time.Millisecond,
	ScaleUpQueueDepth:   8000,
	ScaleDownQueueDepth: 1000,
	TargetLatency:       50 * time.Millisecond,
}
```

## SQLOperationConfig

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

规则：

- `ConflictColumns`：冲突键。未配置时兼容旧行为，默认使用 schema 第一列。
- `UpdateColumns`：仅 `ConflictUpdate` 生效；未配置时更新所有非冲突列。
- `DeduplicateByConflictColumns`：默认开启，避免同一批次重复冲突键导致 PostgreSQL 一条 upsert 多次影响同一行。
- PostgreSQL `ConflictReplace` 是 upsert 覆盖，不模拟 MySQL delete+insert。
- MySQL `ConflictReplace` 保持原生 `REPLACE INTO`。

## SQL Dry Run

```go
preview, err := batchflow.GenerateSQLPreview(ctx, batchflow.DefaultPostgreSQLDriver, schema, rows)
if err != nil {
	return err
}

log.Printf("fingerprint=%s args=%d input=%d output=%d dedup=%d",
	preview.Fingerprint,
	preview.ArgsCount,
	preview.DedupStats.InputRows,
	preview.DedupStats.OutputRows,
	preview.DedupStats.DeduplicatedRows,
)
```

不要在生产日志中输出 `preview.Args`。

## 观测性

`ObservabilityConfig` 支持结构化日志、采样和脱敏：

```go
cfg.Pipeline.Observability = batchflow.ObservabilityConfig{
	Logger:             logger,
	Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
	Redactor:           batchflow.DefaultRedactor(),
	SlowBatchThreshold: 500 * time.Millisecond,
}
```

推荐策略：

- 错误事件全量记录。
- 成功事件采样或只记录慢批次。
- 不记录原始 row、SQL args、Redis key、HTTP body、邮箱、手机号、token。
- 自定义 processor 实现 `OperationPreviewer`，提供安全 fingerprint 和低基数字段。
