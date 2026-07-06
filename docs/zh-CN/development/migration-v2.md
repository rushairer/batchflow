# BatchFlow v2 迁移指南

BatchFlow v2.0.0-rc.2 将核心能力进一步收敛为后端无关的写入 runtime。

公开模块路径仍然是：

```bash
go get github.com/rushairer/batchflow/v2@v2.0.0-rc.2
```

Go import:

```go
import batchflow "github.com/rushairer/batchflow/v2"
```

因为 v2 还没有正式发布，公开 API 使用干净命名，不加 `V2` / `V3` 装饰。

## 构造函数迁移

新生产代码推荐：

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Runtime.ShardCount = 4

flow, err := batchflow.New(ctx, cfg)
if err != nil {
	return err
}
defer flow.Close()
```

`NewMySQLBatchFlow`、`NewPostgreSQLBatchFlow`、`NewSQLiteBatchFlow`、`NewRedisBatchFlow` 继续可用，适合简单迁移或快速测试。

## Runtime 控制项

RC2 新增 runtime 级控制：

- `RuntimeConfig.ShardCount`
- `RuntimeConfig.Routing`
- `RuntimeConfig.Backpressure`
- `RuntimeConfig.MemoryLimit`
- `RuntimeConfig.Adaptive`

生产迁移建议显式启用背压和内存保护。

## COPY FROM 迁移

PostgreSQL/Hologres append-only 高吞吐写入推荐 COPY 路径：

```go
copyExecutor := pgxcopy.NewExecutor(pool)

cfg := batchflow.DefaultConfig(copyExecutor)
cfg.Pipeline.BufferSize = 50000
cfg.Pipeline.FlushSize = 5000
cfg.Pipeline.FlushInterval = 20 * time.Millisecond
cfg.Runtime.ShardCount = 8

flow, err := batchflow.New(ctx, cfg)
```

COPY FROM 只支持 append-only。需要 upsert/update/replace 时继续使用 SQL executor。

## 批数据模型

推荐使用命名别名：

```go
type Record = map[string]any
type Batch = []Record
```

已有 `[]map[string]any` 实现仍可编译。

## 批内合并

非 SQL 后端使用通用 `Coalescer`：

```go
cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.Coalescer = batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "id")
```

SQL 后端继续使用 `SQLOperationConfig.WithConflictColumns(...)`，保留数据库特定语义和 SQL dry-run 去重统计。

## Request.Columns 行为

`Request.Columns()` 返回防御性副本。依赖修改返回 map 的旧代码，应改为在提交前使用 `Set(...)`、`SetNull(...)` 或类型化 setter。

## 错误分类

自定义后端应注册结构化分类器：

```go
unregister := batchflow.RegisterErrorClassifier(classifier)
defer unregister()
```

自定义分类器在内置 MySQL/PostgreSQL/Redis 结构化识别之后、字符串 fallback 之前运行。
