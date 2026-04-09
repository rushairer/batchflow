# BatchFlow 使用示例

本文档只保留当前仓库已经验证过的推荐写法。

## MySQL

```go
db, err := sql.Open("mysql", dsn)
if err != nil {
	return err
}
defer db.Close()

flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
})
defer flow.Close()

schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictIgnoreOperationConfig,
	"id", "name", "email",
)

for i := 0; i < 1000; i++ {
	req := batchflow.NewRequest(schema).
		SetUint64("id", uint64(i)).
		SetString("name", fmt.Sprintf("user_%d", i)).
		SetString("email", fmt.Sprintf("user_%d@example.com", i))

	if err := flow.Submit(ctx, req); err != nil {
		return err
	}
}
```

## PostgreSQL

```go
db, err := sql.Open("postgres", dsn)
if err != nil {
	return err
}
defer db.Close()

flow := batchflow.NewPostgreSQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:    2000,
	FlushSize:     200,
	FlushInterval: 100 * time.Millisecond,
})
defer flow.Close()
```

## SQLite

```go
db, err := sql.Open("sqlite3", "./test.db")
if err != nil {
	return err
}
defer db.Close()

flow := batchflow.NewSQLiteBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     100,
	FlushInterval: 200 * time.Millisecond,
})
defer flow.Close()
```

## Redis

```go
rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
defer rdb.Close()

flow := batchflow.NewRedisBatchFlow(ctx, rdb, batchflow.PipelineConfig{
	BufferSize:    5000,
	FlushSize:     500,
	FlushInterval: 50 * time.Millisecond,
})
defer flow.Close()

schema := batchflow.NewSchema("cache", "cmd", "key", "ttl", "value")

req := batchflow.NewRequest(schema).
	SetString("cmd", "SETEX").
	SetString("key", "user:1").
	SetInt64("ttl", 3600).
	SetString("value", `{"name":"alice"}`)

if err := flow.Submit(ctx, req); err != nil {
	return err
}
```

## 自定义执行器

```go
type MyExecutor struct{}

func (e *MyExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
	// 自定义执行逻辑
	return nil
}

flow := batchflow.NewBatchFlow(ctx, 1000, 100, 100*time.Millisecond, &MyExecutor{})
defer flow.Close()
```

## 带重试和限流

```go
flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       5000,
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
```

## 指标接入

```go
import prommetrics "github.com/rushairer/batchflow/examples/metrics/prometheus"

metrics := prommetrics.NewMetrics(prommetrics.Options{
	Namespace:             "batchflow",
	IncludeInstanceID:     true,
	EnablePipelineMetrics: true,
})

if err := metrics.StartServer(2112); err != nil {
	return err
}
defer metrics.StopServer(context.Background())

reporter := prommetrics.NewReporter(metrics, "mysql", "order_writer")

flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:      5000,
	FlushSize:       200,
	FlushInterval:   100 * time.Millisecond,
	MetricsReporter: reporter,
})
defer flow.Close()
```

## 错误消费

```go
errs := flow.ErrorChan(64)
go func() {
	for err := range errs {
		log.Printf("async batch error: %v", err)
	}
}()
```

## 生命周期收尾

```go
if err := flow.Close(); err != nil {
	return err
}
```

## Wait / Done

```go
go func() {
	<-flow.Done()
	log.Println("batchflow stopped")
}()

if err := flow.Wait(); err != nil {
	return err
}
```

适合“提交方”和“关闭方”不在同一个 goroutine 的场景。

## Request 基础类型 setter

```go
req := batchflow.NewRequest(schema).
	SetInt("retry_count", 3).
	SetUint64("id", 42).
	SetUint8("shard", 7).
	SetBool("enabled", true)
```

说明：

- 业务代码推荐始终调用 `Close()`，不要只依赖 `FlushInterval` 自然触发。
- 如果你只是等待退出而不关闭输入，请用 `Wait()`。
- `Done()` 适合做退出通知，不负责触发收尾。
- 基础整数类型优先使用对应的 `SetInt...` / `SetUint...` 便捷方法；其他类型继续用 `Set(...)` 或现有 typed setter。
