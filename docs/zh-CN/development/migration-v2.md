# BatchFlow v2 迁移指南

BatchFlow v2 将核心能力进一步抽象为后端无关的批处理框架，同时保留 MySQL、PostgreSQL、SQLite、Redis 的兼容构造函数。

## 模块路径

```bash
go get github.com/rushairer/batchflow/v2
```

Go import:

```go
import "github.com/rushairer/batchflow/v2"
```

## 构造函数迁移

新自定义后端推荐使用配置化构造函数：

```go
executor := batchflow.NewThrottledBatchExecutor(processor)

flow, err := batchflow.NewBatchFlowWithConfig(ctx, batchflow.BatchFlowConfig{
	Pipeline: batchflow.DefaultPipelineConfig(),
	Executor: executor,
})
if err != nil {
	return err
}
defer flow.Close()
```

`NewMySQLBatchFlow`、`NewPostgreSQLBatchFlow`、`NewSQLiteBatchFlow`、`NewRedisBatchFlow` 继续可用。

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
config := batchflow.DefaultPipelineConfig()
config.Coalescer = batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "id")
```

SQL 后端继续使用 `SQLOperationConfig.WithConflictColumns(...)`，保留数据库特定语义和 SQL dry-run 去重统计。

## 错误分类

自定义后端应注册结构化分类器：

```go
unregister := batchflow.RegisterErrorClassifier(classifier)
defer unregister()
```

自定义分类器在内置 MySQL/PostgreSQL/Redis 结构化识别之后、字符串 fallback 之前运行。
