# 生产指南

本文档是 [Production Guide](../../guides/production.md) 的中文镜像摘要。

## 基线配置

```go
flow := batchflow.NewPostgreSQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	Timeout:          2 * time.Second,
	ConcurrencyLimit: 8,
	Retry: batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	},
	MetricsReporter: reporter,
	Observability: batchflow.ObservabilityConfig{
		Logger:             logger,
		Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
		Redactor:           batchflow.DefaultRedactor(),
		SlowBatchThreshold: 500 * time.Millisecond,
	},
})
```

## 数据库连接池

BatchFlow 不管理 `*sql.DB` 连接池。应用侧需要设置：

```go
db.SetMaxOpenConns(64)
db.SetMaxIdleConns(32)
db.SetConnMaxLifetime(time.Hour)
db.SetConnMaxIdleTime(10 * time.Minute)
```

`ConcurrencyLimit` 应低于数据库连接池和后端真实写入能力。

## SQL 上线检查

- update/replace 必须显式配置 `ConflictColumns`。
- 上线前用 `GenerateSQLPreview` 检查最终 SQL、冲突列、更新列和批内去重统计。
- 不在生产日志中输出 `preview.Args`。

## 非 SQL / DIY 后端

HTTP、文档库、消息队列、自定义 API 推荐：

1. 实现 `BatchProcessor`。
2. 实现 `OperationPreviewer` 输出安全诊断信息。
3. 使用 `NewThrottledBatchExecutor` 复用重试、限流、指标和观测能力。
4. 如需同 key 合并，配置 `PipelineConfig.Coalescer`。

## 上线清单

- [ ] shutdown 时调用 `Close()`。
- [ ] 消费或明确忽略 `ErrorChan`。
- [ ] 配置 `MetricsReporter`。
- [ ] `ObservabilityConfig` 已脱敏敏感字段。
- [ ] SQL update/replace 已显式配置 `ConflictColumns`。
- [ ] PostgreSQL/MySQL 写入路径已用 `GenerateSQLPreview` 检查。
- [ ] 重试策略使用低基数 reason label。
- [ ] 目标后端 Docker 集成/压力测试已通过。
