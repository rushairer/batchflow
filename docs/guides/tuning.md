# 调优最佳实践

本指南提供一套在不破坏现有 API 的前提下，可逐步启用的“指标细化 + 自适应策略”设计与落地步骤，并附带基准与压力脚本，帮助你建立可重复的调优流程与对比基线。

## 目标

- 零侵入：保留现有 `PipelineConfig` 与对外 API，默认关闭自适应功能
- 可观测：补足关键阶段指标，支撑调参与回归
- 可验证：提供脚本化的基准与压力方法，输出可对比的报告

## 配置结构（设计草案，向后兼容）

说明：以下为“文档设计”，建议后续按该结构在代码层实现。零值/空指针表示关闭，保持现有默认行为不变。

```go
// 新增（可选）自适应策略配置
type AdaptiveConfig struct {
    // 批量大小自适应（根据延迟、错误率、队列长度）
    Batch struct {
        Enabled        bool
        MinSize        int           // 默认: 0(=使用原 FlushSize 下限)
        MaxSize        int           // 默认: 0(=使用原 FlushSize 上限或推导上限)
        TargetP95lat   time.Duration // 目标 p95 批处理延迟窗口(如 50ms)
        MaxBatchAge    time.Duration // 单批等待的最大时间(如 100ms，避免低QPS时长等待)
        StepUpRatio    float64       // 扩批步进比例(如 1.25)
        StepDownRatio  float64       // 缩批步进比例(如 0.8)
    }

    // 并发度自适应（根据错误/超时/锁等待与 p95/p99）
    Concurrency struct {
        Enabled          bool
        Min              int   // 0 表示不低于默认并发(或=1)
        Max              int   // 0 表示不高于已有限流(若未设置则推导)
        ErrorBackoffBase time.Duration // 错误退避基值(如 50ms)
        Cooldown         time.Duration // 冷却期，防止抖动(如 1s)
        InitialRampUp    int           // 冷启动斜坡步进(如 1)
    }

    // 重试与幂等（统一策略）
    Retry struct {
        Enabled       bool
        MaxAttempts   int           // 如 3
        BackoffBase   time.Duration // 指数退避基值(如 20ms) + 抖动
        IdempotentKey string        // 可选的幂等键字段名（与业务唯一键配合）
    }
}

// 指标细化与导出开关
type MetricsConfig struct {
    Enabled bool
    // 直方图桶、Quantiles、命名空间前缀等可选项
}
```

与现有 `PipelineConfig` 的整合建议：
- 以“可选指针字段”方式扩展，不破坏现有字段
- 零值关闭：未提供 AdaptiveConfig / MetricsConfig 时，行为与现状一致
- 维持现有 `FlushSize` / `FlushInterval` 语义作为初始值与保护边界

## 指标细化（建议实现项）

为实现自适应与调优，需要至少以下阶段指标（Prometheus 建议）：
- 入队延迟：submit_enqueue_latency_seconds (histogram)
- 攒批时长：batch_assemble_duration_seconds (histogram)
- 批大小：batch_size (histogram or summary)
- 执行时长：batch_execute_duration_seconds (histogram)
- 重试次数：batch_retry_total (counter)
- 错误分类计数：batch_error_total{type="timeout|deadlock|duplicate|network|..."} (counter)
- 并发度：executor_concurrency (gauge)
- 队列长度：pipeline_queue_length (gauge)

注：直方图桶需统一，避免面板误读；具体桶按目标延迟与数据库特性设定（如 1ms~1s 对数桶）。

## 默认策略（建议值）

- 自适应关闭（默认）：确保升级不影响现网
- 开启推荐（灰度）：
  - Batch:
    - TargetP95lat=50ms, MaxBatchAge=100ms
    - StepUpRatio=1.25, StepDownRatio=0.8
    - Min/MaxSize 可按 DB/网络情况设定（如 [50, 2000]）
  - Concurrency:
    - 初始并发取现配置或 1，Max=8~32（按DB与机器规格）
    - ErrorBackoffBase=50ms, Cooldown=1s, InitialRampUp=1
  - Retry:
    - MaxAttempts=3, BackoffBase=20ms + 抖动
    - IdempotentKey 仅在业务具备幂等语义时开启

## 落地与验证步骤

1) 建立基线
- 使用 scripts/benchmark_matrix.sh 跑默认静态配置（Adaptive 关闭）
- 固定参数：BufferSize/FlushSize/FlushInterval
- 输出 bench 报告归档到 reports/benchmarks/YYYYMMDD-HHMMSS

2) 开启指标细化
- 调整 Grafana 面板，增加关键阶段指标曲线
- 校验数值是否合理（无 10000% 等异常）

3) 按数据库逐步开启自适应（灰度）
- Redis → MySQL → PostgreSQL（按你的环境优先级）
- 控制 Min/MaxSize 与 Max 并发，先小幅开启（如 MaxSize=当前2倍、Max并发=当前+2）

4) 压力验证
- 使用 scripts/stress_with_monitoring.sh 启动监控与集成测试/压测
- 观察 p95/p99、错误率、RPS 与资源指标是否趋稳

5) 回归与文档化
- 将有效配置整理为环境/部署建议，形成面向生产的模板

## 常见问题

- Q: 自适应会不会造成震荡？
  - A: 通过 Cooldown、步进比例与上下限边界可避免；必要时分阶段启用。
- Q: SQLite 表现不佳如何处理？
  - A: 针对单写入者限制，不建议高并发写；优先使用 MySQL/PG/Redis。
- Q: 幂等键如何设计？
  - A: 与业务唯一键（如主键/唯一索引）对齐，避免重试产生覆盖风险。