# 配置说明

本文档是 [Configuration](../../api/configuration.md) 的中文镜像摘要。英文文档是主契约。

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

`DefaultPipelineConfig()` 提供生产可用默认值。`Coalescer` 用于 Redis、HTTP、MongoDB、队列等非 SQL 后端的批内同 key 合并。SQL 后端使用 `SQLOperationConfig.ConflictColumns` 执行 conflict-key 合并，并在 SQL dry-run 中输出统计。

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
Observability: batchflow.ObservabilityConfig{
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
