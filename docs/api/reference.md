# BatchFlow API 参考手册

## 📖 概述

BatchFlow 提供了简洁而强大的API，支持多种数据源的高性能批量处理。本文档提供完整的API参考和最佳实践。

## 🏗️ 核心组件

### BatchFlow 主类

```go
type BatchFlow struct {
    // 内部字段（不直接访问）
}

// 创建BatchFlow实例
func NewBatchFlow(
    ctx context.Context,
    bufferSize int,
    batchSize int,
    flushInterval time.Duration,
    executor batchflow.BatchExecutor,
) *BatchFlow
```

**参数说明**：
- `ctx`: 上下文，用于控制生命周期
- `bufferSize`: 内存缓冲区大小（推荐：1000-10000）
- `batchSize`: 批次大小（推荐：100-1000）
- `flushInterval`: 刷新间隔（推荐：100ms-1s）
- `executor`: 批量执行器实现

### Submit 取消语义

- 当传入的 ctx 已被取消或超时，Submit 会在尝试入队之前立即返回 ctx.Err()（context.Canceled 或 context.DeadlineExceeded）
- 对提交通道的选择发生前即检查 ctx，避免“已入队但外部随后取消”的不确定性
- 调用方应在提交前管理好 context 生命周期，避免无效提交

最小示例：
```go
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
cancel() // 或自然超时

if err := batch.Submit(ctx, req); err != nil {
    // 立即返回 context.Canceled 或 context.DeadlineExceeded，不会入队
    log.Printf("submit cancelled: %v", err)
}
```

### Schema 定义

```go
type Schema struct {
    Name     string
    ConflictMode  ConflictMode
    Fields        []string
}
```

### 可选并发限流（WithConcurrencyLimit）

```go
// 直接在执行器上启用限流（示例：MySQL）
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
    WithConcurrencyLimit(8)

// 创建 BatchFlow
batch := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
```

说明：
- limit <= 0 不启用限流（默认行为）
- 限流在 ExecuteBatch 入口，避免攒批后同时触发高并发
- 指标上报与错误处理与不限流路径一致

// 创建基础Schema（用于Redis等不需要冲突策略的场景）
func NewSchema(tableName string, fields ...string) *Schema

// 创建SQL Schema（用于MySQL、PostgreSQL、SQLite等需要冲突策略的场景）
func NewSQLSchema(tableName string, conflictMode ConflictMode, fields ...string) *Schema
```

**冲突处理模式**：
```go
const (
    ConflictIgnore  ConflictMode = "IGNORE"   // 忽略冲突
    ConflictReplace ConflictMode = "REPLACE"  // 替换冲突
    ConflictUpdate  ConflictMode = "UPDATE"   // 更新冲突
)
```

### Request 构建

```go
type Request struct {
    schema *Schema
    data   map[string]any
}

// 创建请求
func NewRequest(schema *Schema) *Request

// 设置字段值
func (r *Request) SetString(field, value string) *Request
func (r *Request) SetInt64(field string, value int64) *Request
func (r *Request) SetFloat64(field string, value float64) *Request
func (r *Request) SetBool(field string, value bool) *Request
func (r *Request) SetTime(field string, value time.Time) *Request
func (r *Request) SetBytes(field string, value []byte) *Request
func (r *Request) SetAny(field string, value any) *Request
```

## 🔌 数据库驱动

### MySQL 驱动

```go
import "github.com/rushairer/batchflow/drivers/mysql"

// 创建MySQL执行器
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)

// 使用示例
db, _ := sql.Open("mysql", dsn)
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
```

### PostgreSQL 驱动

```go
import "github.com/rushairer/batchflow/drivers/postgresql"

// 创建PostgreSQL执行器
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)

// 使用示例
db, _ := sql.Open("postgres", dsn)
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)
batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
```

### SQLite 驱动

```go
import "github.com/rushairer/batchflow/drivers/sqlite"

// 创建SQLite执行器
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultSQLiteDriver)

// 使用示例
db, _ := sql.Open("sqlite3", dsn)
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultSQLiteDriver)
batchFlow := batchflow.NewBatchFlow(ctx, 1000, 100, 200*time.Millisecond, executor)
```

### Redis 驱动

```go
import "github.com/rushairer/batchflow/drivers/redis"

// 创建Redis执行器
executor := batchflow.NewRedisThrottledBatchExecutor(rdb)

// 使用示例
rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
executor := batchflow.NewRedisThrottledBatchExecutor(rdb)
batchFlow := batchflow.NewBatchFlow(ctx, 5000, 500, 50*time.Millisecond, executor)
```

## 📊 指标监控

### MetricsReporter 接口

```go
type MetricsReporter interface {
    // 阶段耗时
    ObserveEnqueueLatency(d time.Duration)                                      // Submit -> 入队
    ObserveBatchAssemble(d time.Duration)                                       // 攒批/组装
    ObserveExecuteDuration(table string, n int, d time.Duration, status string) // 执行

    // 其他观测
    ObserveBatchSize(n int)
    IncError(table string, kind string)
    SetConcurrency(n int)
    SetQueueLength(n int)

    // 执行期负载（不限流场景也有值）
    IncInflight()
    DecInflight()
}
```

- NoopMetricsReporter：默认空实现，零依赖、零开销。未注入 Reporter 时，库内埋点不会产生任何副作用；当注入后立即开始上报。
- 注入时机：务必在 NewBatchFlow 之前对执行器调用 WithMetricsReporter，NewBatchFlow 会尊重已注入的 Reporter。
  - 若执行器未设置 Reporter：BatchFlow 仅在内部使用本地 NoopMetricsReporter 进行自有观测，不写回执行器；
  - 若执行器已设置 Reporter：BatchFlow 复用该 Reporter，绝不覆盖。

语义、单位与标签建议
- 时间：秒（Prometheus 推荐）。将 time.Duration 转换为秒上报。
- 批大小：整数（n）。
- Gauge：并发度（0 表示不限流）、队列长度、在途批次。
- 错误：Counter，kind 建议采用 retry:<reason> 或 final:<reason>。
- 标签说明：
  - database：数据库类型（mysql/postgres/sqlite/redis）
  - instance_id：实例标识，用于区分多个 BatchFlow 实例
    - 集成测试：使用测试名称（如 "高吞吐量测试"）
    - 生产环境：使用业务标识（如 "order_writer", "log_collector"）
  - table：表名（可选，仅在需要按表维度统计时使用）
  - status：执行状态（success/fail）

进一步阅读
- 监控快速上手：docs/guides/monitoring-quickstart.md
- 自定义 Reporter：docs/guides/custom-metrics-reporter.md

### WithMetricsReporter 最佳实践

#### 1. 基本用法

```go
// 创建指标报告器
metricsReporter := NewCustomMetricsReporter()

// 为执行器添加指标监控
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
executor = executor.WithMetricsReporter(metricsReporter)

// 创建BatchFlow实例
batchFlow := batchflow.NewBatchFlow(ctx, bufferSize, batchSize, flushInterval, executor)
```

#### 2. Prometheus 集成示例

```go
// 创建Prometheus指标报告器
prometheusMetrics := NewPrometheusMetrics()
metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, "mysql", "order_writer")

// 应用到执行器
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
if prometheusMetrics != nil {
    executor = executor.WithMetricsReporter(metricsReporter)
}

batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
```

#### 3. 自定义指标报告器

```go
type CustomMetricsReporter struct {
    logger *log.Logger
    stats  *Stats
}

func (r *CustomMetricsReporter) ObserveExecuteDuration(tableName string, batchSize int, d time.Duration, status string) {
    r.logger.Printf("Batch executed: table=%s, size=%d, duration=%dms, status=%s", 
        tableName, batchSize, duration, status)
    
    r.stats.RecordBatch(batchSize, duration, status == "success")
}

// 使用自定义报告器
metricsReporter := &CustomMetricsReporter{
    logger: log.New(os.Stdout, "[METRICS] ", log.LstdFlags),
    stats:  NewStats(),
}

executor = executor.WithMetricsReporter(metricsReporter)
```

#### 4. 多数据库监控模式

```go
func setupExecutorWithMetrics(dbType string, db interface{}, prometheusMetrics *PrometheusMetrics, instanceID string) batchflow.BatchExecutor {
    var executor batchflow.BatchExecutor
    
    switch dbType {
    case "mysql":
        executor = batchflow.NewSQLThrottledBatchExecutorWithDriver(db.(*sql.DB), batchflow.DefaultMySQLDriver)
    case "postgres":
        executor = batchflow.NewSQLThrottledBatchExecutorWithDriver(db.(*sql.DB), batchflow.DefaultPostgreSQLDriver)
    case "sqlite3":
        executor = batchflow.NewSQLThrottledBatchExecutorWithDriver(db.(*sql.DB), batchflow.DefaultSQLiteDriver)
    case "redis":
        executor = batchflow.NewRedisThrottledBatchExecutor(db.(*redis.Client))
    }
    
    // 统一添加指标监控
    if prometheusMetrics != nil {
        metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, dbType, instanceID)
        executor = executor.WithMetricsReporter(metricsReporter)
    }
    
    return executor
}
```

## 🚀 完整使用示例

### 基础批量插入

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "time"
    
    "github.com/rushairer/batchflow"
    "github.com/rushairer/batchflow/drivers"
    "github.com/rushairer/batchflow/drivers/mysql"
    _ "github.com/go-sql-driver/mysql"
)

func main() {
    // 1. 连接数据库
    db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/testdb")
    if err != nil {
        panic(err)
    }
    defer db.Close()
    
    // 2. 创建执行器
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    
    // 3. 创建BatchFlow实例
    ctx := context.Background()
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 4. 定义Schema
    schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig,
        "id", "name", "email", "created_at")
    
    // 5. 批量提交数据
    for i := 0; i < 10000; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("name", fmt.Sprintf("User_%d", i)).
            SetString("email", fmt.Sprintf("user_%d@example.com", i)).
            SetTime("created_at", time.Now())
        
        if err := batchFlow.Submit(ctx, request); err != nil {
            fmt.Printf("Submit error: %v\n", err)
        }
    }
    
    fmt.Println("Batch insert completed!")
}
```

### 高级配置示例

```go
func advancedBatchInsert() {
    // 高性能配置
    config := BatchConfig{
        BufferSize:    10000,  // 大缓冲区
        BatchSize:     500,    // 中等批次
        FlushInterval: 50 * time.Millisecond, // 快速刷新
    }
    
    // 创建带监控的执行器
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    
    // 添加Prometheus监控
    if prometheusEnabled {
        metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, "mysql", "high_performance")
        executor = executor.WithMetricsReporter(metricsReporter)
    }
    
    batchFlow := batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)
    
    // 使用事务控制
    tx, _ := db.Begin()
    defer tx.Rollback()
    
    // 批量操作...
    
    tx.Commit()
}
```

### Redis 批量操作示例

```go
func redisBatchExample() {
    // 连接Redis
    rdb := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
    
    // 创建Redis执行器
    executor := batchflow.NewRedisThrottledBatchExecutor(rdb)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 500, 50*time.Millisecond, executor)
    
    // Redis Schema（使用命令格式）
    schema := batchflow.NewSchema("redis_cache",
        "cmd", "key", "value", "ex_flag", "ttl")
    
    // 批量SET操作
    for i := 0; i < 1000; i++ {
        request := batchflow.NewRequest(schema).
            SetString("cmd", "SET").
            SetString("key", fmt.Sprintf("user:%d", i)).
            SetString("value", fmt.Sprintf(`{"id":%d,"name":"User_%d"}`, i, i)).
            SetString("ex_flag", "EX").
            SetInt64("ttl", 3600) // 1小时TTL
        
        batchFlow.Submit(ctx, request)
    }
}
```

## ⚙️ 配置参数指南

### 性能调优参数

| 参数 | 推荐值 | 说明 |
|------|--------|------|
| **BufferSize** | 1000-10000 | 内存缓冲区大小，影响内存使用 |
| **BatchSize** | 100-1000 | 单次批处理大小，影响网络效率 |
| **FlushInterval** | 50ms-1s | 刷新间隔，影响延迟 |

### 数据库特定建议

#### MySQL
- BufferSize: 5000-10000
- BatchSize: 200-500
- FlushInterval: 100ms

#### PostgreSQL  
- BufferSize: 5000-10000
- BatchSize: 200-500
- FlushInterval: 100ms

#### SQLite
- BufferSize: 1000-2000
- BatchSize: 50-200
- FlushInterval: 200ms

#### Redis
- BufferSize: 5000-20000
- BatchSize: 500-2000
- FlushInterval: 50ms

## 🔍 错误处理

### 常见错误类型

```go
// 连接错误
if err := db.Ping(); err != nil {
    log.Fatal("Database connection failed:", err)
}

// 提交错误
if err := batchFlow.Submit(ctx, request); err != nil {
    log.Printf("Submit failed: %v", err)
    // 实现重试逻辑
}

// 批处理错误
// 通过MetricsReporter监控失败率
```

### 最佳实践

1. **连接池配置**
```go
db.SetMaxOpenConns(100)
db.SetMaxIdleConns(50)
db.SetConnMaxLifetime(time.Hour)
```

2. **上下文控制**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

3. **优雅关闭**
```go
defer batchFlow.Close() // 确保所有数据都被刷新
```

## ⏱️ 超时与重试策略

- 默认重试分类器：context.Canceled 与 context.DeadlineExceeded 均被判定为不可重试（final:context）。
- 处理器行为：
  - SQL/Redis 处理器在 ExecuteOperations 内部使用 context.WithTimeoutCause 包裹子 ctx。
  - 当子 ctx 达到超时，驱动返回 context.DeadlineExceeded；处理器会读取 context.Cause(ctx)，并将其作为返回错误（如 "execute batch timeout"），便于上层分类与观测。
- 如需对“处理器内部超时”进行重试：请自定义 RetryConfig.Classifier，将带有该 cause 的错误判定为可重试，并合理设置退避。
- 注意区分：
  - 外层 ctx 取消/超时（调用方主动取消或上游时限到达）：不应重试。
  - 处理器内部短暂性超时（锁等待、瞬时抖动）：可按策略重试并配合指数退避。

示例（自定义分类器片段）：
```go
exec.WithRetryConfig(batchflow.RetryConfig{
    Enabled:     true,
    MaxAttempts: 3,
    BackoffBase: 20*time.Millisecond,
    Classifier: func(err error) (bool, string) {
        if errors.Is(err, context.Canceled) {
            return false, "canceled"
        }
        if errors.Is(err, context.DeadlineExceeded) {
            // 默认不可重试；若使用处理器 cause，可按需放开
            return false, "deadline"
        }
        if strings.Contains(strings.ToLower(err.Error()), "execute batch timeout") {
            return true, "processor_timeout"
        }
        return defaultRetryClassifier(err)
    },
})
```

## 🧰 处理器实现要点（与失败快速退出）

- SQL 执行：
  - 增加空 operations 校验（len(operations) < 1 返回 "empty operations"）。
- Redis 执行：
  - 大批量时在循环内检测 ctx（例如每次或每 N 次迭代检查 ctx.Err()），在取消/超时后快速返回，避免在超大 operations 上浪费迭代成本。

建议：若 operations 极大，可按“每 32/64 次迭代”检查一次 ctx，降低分支开销且保持取消响应性。

## 🧪 errors.Join 的判断建议

- 存在性判断用 errors.Is：判断是否包含某类哨兵错误（如 context.DeadlineExceeded、io.EOF）。
- 提取具体类型用 errors.As：解包第一个匹配的具体类型（如 *net.OpError）。
- errors.Join 支持多错误链路，Is/As 会遍历所有子错误进行匹配。

## 📚 相关文档

- [EXAMPLES.md](EXAMPLES.md) - 更多使用示例
- [MONITORING_GUIDE.md](MONITORING_GUIDE.md) - 监控配置
- [PERFORMANCE_TUNING.md](PERFORMANCE_TUNING.md) - 性能优化
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - 问题排查

---

💡 **提示**：API设计遵循链式调用模式，支持流畅的代码编写。建议结合具体使用场景选择合适的配置参数。