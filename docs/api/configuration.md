# BatchFlow 配置说明

本文档描述业务侧真正会用到的配置项。集成测试和 Docker 环境配置请看 [集成测试文档](../guides/integration-tests.md)。

## PipelineConfig

```go
type PipelineConfig struct {
	BufferSize       uint32
	FlushSize        uint32
	FlushInterval    time.Duration
	Retry            RetryConfig
	Timeout          time.Duration
	MetricsReporter  MetricsReporter
	ConcurrencyLimit int
}
```

## 字段说明

### BufferSize

- 含义：内部输入通道容量。
- 影响：越大越能吸收提交峰值，但也会延长尾部排队时间。
- 建议：先从 `FlushSize` 的 `2x ~ 10x` 开始。

### FlushSize

- 含义：达到多少条请求后立即触发 flush。
- 影响：越大吞吐通常越高，但单次执行延迟和内存占用也会升高。
- 建议：
  - OLTP 写入：`100 ~ 500`
  - 日志/批同步：`500 ~ 2000`

### FlushInterval

- 含义：即使没有满批，也会在该时间间隔触发 flush。
- 影响：越短越偏实时，越长越偏吞吐。
- 建议：
  - 实时写入：`50ms ~ 200ms`
  - 吞吐优先：`200ms ~ 1s`

### Timeout

- 含义：单次处理器执行超时。
- 作用范围：作用在 SQL/Redis processor 执行阶段。
- 建议：只有当你希望把慢执行快速失败并交给重试分类器时才开启。

### Retry

- 含义：执行器级重试配置。
- 建议默认值：

```go
Retry: batchflow.RetryConfig{
	Enabled:     true,
	MaxAttempts: 3,
	BackoffBase: 20 * time.Millisecond,
	MaxBackoff:  500 * time.Millisecond,
}
```

说明：

- `MaxAttempts` 包含第一次执行。
- 默认分类器把 `context.Canceled` / `context.DeadlineExceeded` 视为不可重试。
- 如果你要区分内部超时和外部取消，请自定义 `Classifier`。

### MetricsReporter

- 含义：可选指标上报器。
- 使用方式：在 `PipelineConfig` 中直接传入。
- 推荐：优先使用 `examples/metrics/prometheus` 中的官方示例实现。

### ConcurrencyLimit

- 含义：限制 `ExecuteBatch` 的并发度。
- 作用点：执行器入口，不是 Submit 阶段。
- 语义：`<= 0` 表示不限流。
- 建议：数据库较脆弱或多 schema 并发场景，优先从 `4 ~ 8` 开始。

## 调优组合建议

### 低延迟优先

```go
batchflow.PipelineConfig{
	BufferSize:       500,
	FlushSize:        100,
	FlushInterval:    50 * time.Millisecond,
	ConcurrencyLimit: 4,
}
```

### 吞吐优先

```go
batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        500,
	FlushInterval:    200 * time.Millisecond,
	ConcurrencyLimit: 8,
}
```

### 有重试和指标

```go
batchflow.PipelineConfig{
	BufferSize:       2000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
	MetricsReporter:  reporter,
	Retry: batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	},
}
```

## 生命周期配置建议

- 业务代码结束前一定要调用 `Close()`，保证最后一批数据被 flush。
- 如果你只想等后台退出而不关闭输入，用 `Wait()`。
- 不要把 `FlushInterval` 当成唯一的收尾机制；收尾应该由 `Close()` 驱动。
