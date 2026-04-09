# BatchFlow

[![Release](https://img.shields.io/github/v/release/rushairer/batchflow?display_name=tag&include_prereleases&sort=semver)](https://github.com/rushairer/batchflow/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/rushairer/batchflow.svg)](https://pkg.go.dev/github.com/rushairer/batchflow)
[![Go Report Card](https://goreportcard.com/badge/github.com/rushairer/batchflow)](https://goreportcard.com/report/github.com/rushairer/batchflow)
[![License](https://img.shields.io/github/license/rushairer/batchflow)](https://github.com/rushairer/batchflow/blob/main/LICENSE)

一个通用的 Go 批处理框架，基于 [go-pipeline](https://github.com/rushairer/go-pipeline) 构建，统一封装了批量攒批、异步 flush、执行器重试、可选并发限流和指标接入。

## 特性

- 统一 API：MySQL、PostgreSQL、SQLite、Redis 都通过 `BatchFlow + PipelineConfig` 使用。
- 异步批处理：`Submit` 只负责入队，后台自动按 `FlushSize` / `FlushInterval` flush。
- 扩展执行器：可以直接传入自定义 `BatchExecutor`。
- 可选重试和限流：通过 `RetryConfig` 和 `ConcurrencyLimit` 调整执行行为。
- 可观测性：支持 `MetricsReporter`、`PipelineMetricsReporter` 和 BatchFlow 自身的可选流程指标。
- 生命周期完整：支持 `Close()`、`Wait()` 和 `Done()`。

## 安装

```bash
go get github.com/rushairer/batchflow
```

## 快速开始

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/rushairer/batchflow"
)

func main() {
	ctx := context.Background()

	db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/testdb?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
		BufferSize:       1000,
		FlushSize:        200,
		FlushInterval:    100 * time.Millisecond,
		Timeout:          500 * time.Millisecond,
		ConcurrencyLimit: 8,
		Retry: batchflow.RetryConfig{
			Enabled:     true,
			MaxAttempts: 3,
			BackoffBase: 20 * time.Millisecond,
			MaxBackoff:  500 * time.Millisecond,
		},
	})
	defer func() {
		if err := flow.Close(); err != nil {
			log.Printf("batchflow close: %v", err)
		}
	}()

	schema := batchflow.NewSQLSchema(
		"users",
		batchflow.ConflictIgnoreOperationConfig,
		"id", "name", "email",
	)

req := batchflow.NewRequest(schema).
	SetUint64("id", 1).
	SetString("name", "alice").
	SetString("email", "alice@example.com")

	if err := flow.Submit(ctx, req); err != nil {
		log.Fatal(err)
	}

	errs := flow.ErrorChan(32)
	go func() {
		for err := range errs {
			log.Printf("batchflow async error: %v", err)
		}
	}()
}
```

## 核心语义

### 生命周期

- `NewXxxBatchFlow(...)` 创建后会立即启动后台 pipeline。
- `Submit(ctx, req)` 负责入队；如果 `ctx` 已取消或 `BatchFlow` 已关闭，会立即返回错误。
- `Close()` 会停止接收新请求、关闭内部数据通道、触发最终 flush，并等待后台退出。
- `Wait()` 只等待退出，不主动关闭输入。
- `Done()` 返回后台退出时关闭的只读通道。

常见用法：

```go
if err := flow.Close(); err != nil {
	return err
}
```

```go
go func() {
	<-flow.Done()
	log.Println("batchflow stopped")
}()

if err := flow.Wait(); err != nil {
	return err
}
```

### Submit 与取消

- `Submit` 会优先检查调用方 `ctx.Err()`。
- 若 `ctx` 已取消或超时，不会把请求放入内部队列。
- 若 `BatchFlow` 自身生命周期已经结束，后续 `Submit` 会被拒绝。

### 批处理语义

- pipeline 先按 `FlushSize` / `FlushInterval` 聚合请求。
- 一次 flush 内部会再按 `SchemaInterface` 分组。
- 每个 schema 组都会调用一次 `BatchExecutor.ExecuteBatch(...)`。
- `ObserveBatchSize(n)` 的语义是“单个 schema 执行批大小”，不是“整次 flush 输入大小”。

### Request 赋值

- `NewRequest(schema)` 返回可链式构建的请求对象。
- 常用整数 setter 现在覆盖 `SetInt`、`SetInt8`、`SetInt16`、`SetInt32`、`SetInt64`、`SetUint`、`SetUint8`、`SetUint16`、`SetUint32`、`SetUint64`。
- 其他基础类型继续使用 `SetFloat32`、`SetFloat64`、`SetString`、`SetBool`、`SetTime`、`SetBytes`、`SetNull`。
- 遇到未封装类型时，使用 `Set(name, value)`。

## 推荐入口

- 业务侧优先使用：
  - `NewMySQLBatchFlow`
  - `NewPostgreSQLBatchFlow`
  - `NewSQLiteBatchFlow`
  - `NewRedisBatchFlow`
- 需要自定义执行逻辑时使用：
  - `NewBatchFlow(ctx, bufferSize, flushSize, flushInterval, executor)`

## 指标与监控

BatchFlow 的指标分三层：

- `MetricsReporter`：执行器和核心阶段指标。
- `PipelineMetricsReporter`：队列等待、管道处理耗时、错误通道丢弃。
- `BatchFlowMetricsReporter`：Submit 被拒绝次数、整次 flush 输入大小、每次 flush 的 schema 组数量。

官方可直接复用的 Prometheus 示例位于：

```go
import prommetrics "github.com/rushairer/batchflow/examples/metrics/prometheus"
```

最小示例：

```go
pm := prommetrics.NewMetrics(prommetrics.Options{
	Namespace:             "batchflow",
	IncludeInstanceID:     true,
	EnablePipelineMetrics: true,
})

if err := pm.StartServer(2112); err != nil {
	log.Fatal(err)
}
defer pm.StopServer(context.Background())

reporter := prommetrics.NewReporter(pm, "mysql", "order_writer")

flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       1000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	MetricsReporter:  reporter,
	ConcurrencyLimit: 8,
})
defer flow.Close()
```

主要指标口径：

- `enqueue_latency_seconds`：Submit 到成功入队的耗时。
- `pipeline_dequeue_latency_seconds`：请求成功入队后，到 flush 开始处理前的等待时间。
- `batch_assemble_duration_seconds`：单个 schema 组装成执行输入的耗时。
- `execute_duration_seconds`：单个 schema 执行批的总耗时。
- `batch_size`：单个 schema 执行批大小。
- `pipeline_flush_size`：整次 flush 收到的请求数。
- `schema_groups_per_flush`：整次 flush 内拆出的 schema 组数量。
- `submit_rejected_total`：Submit 被拒绝的次数和原因。

## 文档

- [文档索引](docs/index.md)
- [API 参考](docs/api/reference.md)
- [配置说明](docs/api/configuration.md)
- [使用示例](docs/guides/examples.md)
- [监控快速上手](docs/guides/monitoring-quickstart.md)
- [监控指南](docs/guides/monitoring.md)
- [Metrics 规格](docs/guides/metrics-spec.md)
- [自定义 MetricsReporter](docs/guides/custom-metrics-reporter.md)
- [go-pipeline 指标接入说明](docs/guides/go-pipeline-metrics.md)

## 开发

```bash
make fmt
make test
make lint
make docs-check
```

## 当前推荐理解

- README、`docs/api/reference.md` 和 `docs/guides/metrics-spec.md` 是当前对外契约的主入口。
- `test/integration` 下的代码主要服务于集成测试和仪表盘验证，不作为业务侧首选接入示例。
