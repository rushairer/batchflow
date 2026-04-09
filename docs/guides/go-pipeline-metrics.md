# BatchFlow 与 go-pipeline 指标接入说明

## 当前事实

`go-pipeline v2.2.4` 暴露的 `MetricsHook` 只有 3 个事件：

- `Flush(items, duration)`
- `Error(err)`
- `ErrorDropped()`

它没有原生的 dequeue 事件。

## BatchFlow 的接入策略

BatchFlow 当前把指标来源分成两部分：

### 1. 直接桥接 go-pipeline hook

文件：[gopipeline_metrics_adapter.go](/Users/aben/Git/go/2025/batchflow/gopipeline_metrics_adapter.go)

当前桥接内容：

- `ErrorDropped()` → `IncDropped("error_chan_full")`

当前不在 adapter 中直接上报：

- `Flush(items, duration)`：不再映射到 `ObserveBatchSize`，避免把整次 flush 输入量与单 schema 执行批大小混成一个指标。
- `Error(err)`：错误计数与执行耗时已经由 BatchFlow / Executor 自己上报，adapter 不再重复上报。

### 2. BatchFlow 自采样

BatchFlow 自己补充了上游没有提供但又需要的指标：

- `ObserveDequeueLatency`：在请求成功入队时记录时间戳，在 flush 开始时计算等待时间。
- `ObserveProcessDuration`：在整个 flush 函数外层记录处理耗时。
- `ObservePipelineFlushSize`：记录整次 flush 收到的请求数。
- `ObserveSchemaGroupsPerFlush`：记录整次 flush 内拆出来的 schema 组数。

## 为什么这样做

如果只依赖上游 `Flush(items, duration)`：

- 可以知道一次 flush 结束了。
- 但不知道这次 flush 后面拆成了多少个 schema 组。
- 也不知道请求在队列里具体等了多久。
- 更不能把“整次 flush 输入量”和“单次执行批大小”区分开。

所以 BatchFlow 现在的策略是：

- 上游 hook 只用来拿它真正有的事件。
- 业务真正关心的批处理语义，由 BatchFlow 自己补齐。

## 当前约束

- `ObserveDequeueLatency` 是 BatchFlow 自采样出来的，不是 go-pipeline 原生指标。
- `pipeline_dropped_total` 当前主要描述错误通道饱和。
- 如果未来 go-pipeline 新增 dequeue 或 trigger 级事件，BatchFlow 应优先切到上游原生事件，再更新 [Metrics 规格](metrics-spec.md)。
