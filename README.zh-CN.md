# BatchFlow

[![Release](https://img.shields.io/github/v/release/rushairer/batchflow?display_name=tag&include_prereleases&sort=semver)](https://github.com/rushairer/batchflow/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/rushairer/batchflow/v2.svg)](https://pkg.go.dev/github.com/rushairer/batchflow/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/rushairer/batchflow/v2)](https://goreportcard.com/report/github.com/rushairer/batchflow/v2)
[![License](https://img.shields.io/github/license/rushairer/batchflow)](https://github.com/rushairer/batchflow/blob/main/LICENSE)

BatchFlow 是一个基于 [go-pipeline](https://github.com/rushairer/go-pipeline) 的 Go 通用批处理框架。它为 SQL 数据库、Redis 和自定义批量写入后端提供统一入口：攒批、异步 flush、重试、并发限制、指标和安全诊断。

英文主文档见 [README.md](README.md)。

## 特性

- 统一 API：MySQL、PostgreSQL、SQLite、Redis 和自定义 `BatchExecutor` 都通过同一套模型接入。
- 异步批处理：按 `FlushSize` / `FlushInterval` 触发 flush。
- SQL upsert：支持显式冲突键、更新列、批内重复 key 合并。
- 非 SQL 批内合并：通过通用 `Coalescer` 支持 Redis、HTTP、MongoDB、队列等自定义数据流。
- 可配置重试、超时、并发限制、结构化错误分类和观测 hooks。
- 提供 Prometheus 示例、SQL dry-run、Docker 集成测试和压力测试报告脚本。
- 生命周期完整：`Close()`、`Wait()`、`Done()`。

## 安装

```bash
go get github.com/rushairer/batchflow/v2
```

## 快速开始

```go
flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       1000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
	Retry: batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	},
})
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

## 非 SQL 和自定义数据流

Redis、HTTP、MongoDB、消息队列等非 SQL 后端可以用 `PipelineConfig.Coalescer` 做批内同 key 合并：

```go
flow := batchflow.NewRedisBatchFlow(ctx, redisClient, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     100,
	FlushInterval: 100 * time.Millisecond,
	Coalescer:     batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "key"),
})
defer flow.Close()
```

自定义后端建议实现 `BatchProcessor`，可选实现 `OperationPreviewer`，再通过 `NewThrottledBatchExecutor` 复用重试、限流、指标和结构化日志能力。

## 文档

- [文档索引](docs/zh-CN/index.md)
- [API 参考](docs/api/reference.md)
- [配置说明](docs/zh-CN/api/configuration.md)
- [使用示例](docs/zh-CN/guides/examples.md)
- [生产指南](docs/zh-CN/guides/production.md)
- [测试指南](docs/zh-CN/guides/testing.md)
- [v2 迁移指南](docs/zh-CN/development/migration-v2.md)

## 开发验证

```bash
make fmt
make test
make lint
make docs-check
```

Docker 压力测试：

```bash
make docker-postgres-test
make docker-mysql-test
make docker-redis-test
```

生成压力测试报告：

```bash
./scripts/run_stress_report.sh
```
