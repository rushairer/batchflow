# BatchFlow

[![Release](https://img.shields.io/github/v/release/rushairer/batchflow?display_name=tag&include_prereleases&sort=semver)](https://github.com/rushairer/batchflow/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/rushairer/batchflow/v2.svg)](https://pkg.go.dev/github.com/rushairer/batchflow/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/rushairer/batchflow/v2)](https://goreportcard.com/report/github.com/rushairer/batchflow/v2)
[![License](https://img.shields.io/github/license/rushairer/batchflow)](https://github.com/rushairer/batchflow/blob/main/LICENSE)

BatchFlow 是一个基于 [go-pipeline](https://github.com/rushairer/go-pipeline) 的 Go 写入运行时。它为 SQL、Redis、PostgreSQL/Hologres COPY FROM 以及自定义批量后端提供统一模型：入队、异步攒批、后端执行、重试/并发控制、生产保护和安全诊断。

英文主文档见 [README.md](README.md)。

## RC2 API 口径

公开模块路径仍然是：

```text
github.com/rushairer/batchflow/v2
```

因为 v2 还没有正式发布，所以公开 API 不加 `V2` / `V3` 装饰。推荐入口是：

```go
cfg := batchflow.DefaultConfig(executor)
flow, err := batchflow.New(ctx, cfg)
```

版本只体现在 module path 和 tag 里，不进入类型名和方法名。

## 特性

- 干净运行时 API：`Config`、`RuntimeConfig`、`Flow`、`DefaultConfig`、`New`。
- 统一执行器模型：SQL、Redis、COPY FROM、自定义 `BatchExecutor` 都能接入。
- Runtime 分片：支持 hash、round-robin、least-loaded。
- Runtime 背压：支持 block、reject、timeout。
- 估算型队列内存限制，避免 OOM。
- 自适应调参策略引擎，输出 flush size / interval / backpressure 推荐值。
- SQL upsert：显式冲突键、更新列、批内重复 key 合并。
- PostgreSQL/Hologres COPY FROM fast path：`CopyFromExecutor` + 可选 `adapters/pgxcopy`。
- 可配置重试、超时、并发限制、结构化错误分类、指标和安全诊断。
- 生命周期完整：`Close()`、`Wait()`、`Done()`。

## 安装

```bash
go get github.com/rushairer/batchflow/v2@v2.0.0-rc.2
```

## 快速开始：SQL runtime

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
	WithConcurrencyLimit(8).
	WithRetryConfig(batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	})

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Runtime.ShardCount = 4
cfg.Runtime.Routing = batchflow.ShardRoutingHash
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
	Enabled:       true,
	Mode:          batchflow.BackpressureTimeout,
	HighWatermark: 8000,
	Timeout:       500 * time.Millisecond,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
	Enabled:         true,
	MaxQueueBytes:   512 << 20,
	AvgRequestBytes: 512,
	Mode:            batchflow.BackpressureTimeout,
	Timeout:         500 * time.Millisecond,
}

flow, err := batchflow.New(ctx, cfg)
if err != nil {
	return err
}
defer flow.Close()

schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("id").
		WithUpdateColumns("name", "email"),
	"id", "name", "email",
)

req := batchflow.NewRequest(schema).
	SetUint64("id", 1).
	SetString("name", "alice").
	SetString("email", "alice@example.com")

if err := flow.Submit(ctx, req); err != nil {
	return err
}
```

## COPY FROM fast path

PostgreSQL/Hologres append-only 写入推荐使用 COPY 路径。根模块只依赖 `CopyFromClient` 最小接口，pgx 实现放在可选 adapter 中：

```bash
go get github.com/rushairer/batchflow/adapters/pgxcopy@v2.0.0-rc.2
```

```go
copyExecutor := pgxcopy.NewExecutor(pool)

cfg := batchflow.DefaultConfig(copyExecutor)
cfg.Pipeline.BufferSize = 50000
cfg.Pipeline.FlushSize = 5000
cfg.Pipeline.FlushInterval = 20 * time.Millisecond
cfg.Runtime.ShardCount = 8
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
	Enabled:       true,
	Mode:          batchflow.BackpressureTimeout,
	HighWatermark: 40000,
	Timeout:       time.Second,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
	Enabled:         true,
	MaxQueueBytes:   1 << 30,
	AvgRequestBytes: 512,
	Mode:            batchflow.BackpressureTimeout,
	Timeout:         time.Second,
}

flow, err := batchflow.New(ctx, cfg)
```

COPY FROM 只支持 append-only 语义。需要 upsert/update/replace 时，请使用 SQL executor 路径。

## SQL Update / Replace

推荐显式声明冲突键：

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("tenant_id", "user_id").
		WithUpdateColumns("name", "email"),
	"tenant_id", "user_id", "name", "email", "updated_at",
)
```

规则：

- 未配置 `ConflictColumns` 时，为兼容旧版本默认使用 schema 第一列；新代码不要依赖这个兜底。
- `ConflictUpdate` 默认更新非冲突列；配置 `UpdateColumns` 后只更新指定列。
- PostgreSQL `ConflictReplace` 是 upsert 覆盖：`ON CONFLICT (...) DO UPDATE SET ...`。
- MySQL `ConflictReplace` 保持原生 `REPLACE INTO` 语义。
- 同批次相同冲突键会先在客户端合并，避免 PostgreSQL 一条 upsert 多次影响同一行。

生产上线前建议用 `GenerateSQLPreview` 检查最终 SQL：

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

不要在生产日志中输出 `preview.Args`，除非你确认参数没有敏感信息。

## Legacy convenience constructors

旧的便捷构造器仍可用于简单迁移或快速测试：

```go
flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     200,
	FlushInterval: 100 * time.Millisecond,
})
```

新生产代码推荐使用 `DefaultConfig(executor)` + `New(ctx, cfg)`，因为这样可以显式配置 runtime 分片、内存保护和背压。

## 文档

- [文档索引](docs/zh-CN/index.md)
- [API 参考](docs/api/reference.md)
- [配置说明](docs/zh-CN/api/configuration.md)
- [使用示例](docs/zh-CN/guides/examples.md)
- [生产指南](docs/zh-CN/guides/production.md)
- [生产调优指南](docs/v2-production-tuning.md)
- [RC2 Release Notes](docs/releases/v2.0.0-rc.2.md)
- [测试指南](docs/zh-CN/guides/testing.md)
- [v2 迁移指南](docs/zh-CN/development/migration-v2.md)

## 开发验证

```bash
make fmt
make test
make lint
make docs-check
```

发布验证：

```bash
go test ./...
go test ./... -race
go test ./benchmark -bench=. -benchmem
cd adapters/pgxcopy && go test ./...
```
