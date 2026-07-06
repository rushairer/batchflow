# 生产指南

本文档是 [Production Guide](../../guides/production.md) 的中文镜像摘要。

## 基线配置

新生产代码推荐使用 clean runtime API：

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver).
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
cfg.Pipeline.Timeout = 2 * time.Second
cfg.Pipeline.MetricsReporter = reporter
cfg.Pipeline.Observability = batchflow.ObservabilityConfig{
	Logger:             logger,
	Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
	Redactor:           batchflow.DefaultRedactor(),
	SlowBatchThreshold: 500 * time.Millisecond,
}

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
```

旧便捷构造器仍可用于简单迁移，但新生产代码建议显式配置 runtime 控制层。

## 数据库连接池

BatchFlow 不管理 `*sql.DB` 连接池。应用侧需要设置：

```go
db.SetMaxOpenConns(64)
db.SetMaxIdleConns(32)
db.SetConnMaxLifetime(time.Hour)
db.SetConnMaxIdleTime(10 * time.Minute)
```

执行器并发和 runtime 分片数量都应低于数据库连接池和后端真实写入能力。

## COPY FROM / Hologres

append-only PostgreSQL/Hologres 写入推荐 COPY path：

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
```

COPY FROM 只支持 append-only。需要 upsert/update/replace 时使用 SQL executor。

## 背压和内存保护

生产环境建议同时启用：

- `BackpressureConfig`：保护每个 shard 的队列深度。
- `MemoryLimitConfig`：保护全局估算队列内存。

推荐：

- 在线 API：`BackpressureReject`，让上游快速重试。
- Worker/服务写入：`BackpressureTimeout`，短暂下游抖动可恢复。
- 离线任务：可使用 `BackpressureBlock`，但要接受阻塞。

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

- [ ] 新生产代码使用 `DefaultConfig(executor)` + `New(ctx, cfg)`。
- [ ] shutdown 时调用 `Close()`。
- [ ] 消费或明确忽略 `ErrorChan`。
- [ ] 配置 `MetricsReporter`。
- [ ] `ObservabilityConfig` 已脱敏敏感字段。
- [ ] 已配置 runtime backpressure。
- [ ] 已配置 runtime memory limit。
- [ ] SQL update/replace 已显式配置 `ConflictColumns`。
- [ ] PostgreSQL/MySQL 写入路径已用 `GenerateSQLPreview` 检查。
- [ ] 重试策略使用低基数 reason label。
- [ ] 目标后端 Docker 集成/压力测试已通过。
- [ ] 如使用 COPY FROM，已验证 `adapters/pgxcopy`。
