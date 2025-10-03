# BatchFlow
[![Release](https://img.shields.io/github/v/release/rushairer/batchflow?display_name=tag&include_prereleases&sort=semver)](https://github.com/rushairer/batchflow/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/rushairer/batchflow.svg)](https://pkg.go.dev/github.com/rushairer/batchflow)
[![Go Report Card](https://goreportcard.com/badge/github.com/rushairer/batchflow)](https://goreportcard.com/report/github.com/rushairer/batchflow)
[![License](https://img.shields.io/github/license/rushairer/batchflow)](https://github.com/rushairer/batchflow/blob/main/LICENSE)
![Coverage](https://img.shields.io/badge/coverage-75%25-brightgreen)
![GitHub Stars](https://img.shields.io/github/stars/rushairer/batchflow?style=social)

一个高性能的 Go 批量 SQL 处理库，基于 `go-pipeline` 实现，支持多种数据库类型和冲突处理策略。


## 🏗️ 架构设计

延伸阅读
- [架构设计详解](docs/development/architecture.md)

### 核心组件
```mermaid
flowchart TB

  %% 子图1：系统级数据流
  subgraph A0[系统级数据流]
    A1[Application] --> A2["BatchFlow<br/>(MySQL/PG/SQLite/Redis)"] --> A3["gopipeline<br/>(异步批量处理)"]

    A2 --> A4["BatchExecutor<br/>(统一执行接口)"]
    A3 --> A5["Flush Function<br/>(批量刷新逻辑)"]

    A4 --> A6[数据库驱动层]
    A5 --> A7["Schema Grouping<br/>(按表分组聚合)"]

    A6 --> A8["SQL数据库<br/>(MySQL/PG/SQLite)"]
    A6 --> A9[Redis数据库]

    A8 --> A10["Database<br/>(SQL连接池)"]
    A9 --> A11["Redis Client<br/>(Redis连接)"]
  end

  %% 子图2：组件分层与驱动路径
  subgraph B0[组件分层与驱动路径]
    B1["BatchExecutor"] --> B2["ThrottledBatchExecutor<br/>(可选限流执行器)"]
    B2 --> B3["BatchProcessor + Driver<br/>(操作生成和执行)"]
    B3 --> B4["Database Connection"]

    %% 数据库类型分支
    B4 --> B5[SQL数据库]
    B4 --> B6[NoSQL数据库]

    B5 --> B51[MySQL]
    B5 --> B52[PostgreSQL]
    B5 --> B53[SQLite]

    B6 --> B61[Redis]
  end

  %% 视图之间的对应关系（虚线）
  A4 -. 同一执行器 .-> B1
  A6 -. 连接/驱动 .-> B4
```

### 设计原则
- **一个BatchFlow绑定一个数据库类型** - 避免混合数据库的复杂性
- **Schema专注表结构定义** - 职责单一，可复用性强
- **BatchExecutor统一接口** - 所有数据库驱动的统一入口
- **模块化设计** - 清晰的组件分工，便于维护和扩展
- **轻量级设计** - 不涉及连接池管理，支持任何数据库框架

## 🚀 功能特性

### 核心功能
- **批量处理**：使用 `gopipeline.StandardPipeline` 进行高效的批量数据处理
- **多数据库支持**：支持 MySQL、PostgreSQL、SQLite，易于扩展
- **冲突处理策略**：支持跳过、覆盖、更新三种冲突处理方式
- **类型安全**：提供类型化的列操作方法
- **智能聚合**：按 schema 指针自动聚合相同表的请求

### 设计亮点
- **指针传递优化**：使用指针传递减少内存复制，提高性能
- **并发安全**：支持并发提交请求，自动按 schema 分组处理
- **灵活配置**：支持自定义缓冲区大小、刷新大小和刷新间隔
- **混合API设计**：默认方式简单易用，自定义方式支持第三方扩展
- **框架无关**：支持原生 `sql.DB`、GORM、sqlx 等任何数据库框架

延伸阅读
- [API 参考](docs/api/reference.md)
- [配置指南](docs/api/configuration.md)
- [使用示例](docs/guides/examples.md)

## 🚀 快速开始

### 安装

```bash
go get github.com/rushairer/batchflow
```

### 基本使用

```go
package main

import (
    "context"
    "database/sql"
    "log"
    "time"
    "github.com/rushairer/batchflow"

    _ "github.com/go-sql-driver/mysql"
)

func main() {
    ctx := context.Background()
    
    // 1. 创建数据库连接（用户自己管理连接池）
    db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/testdb")
    if err != nil {
        panic(err)
    }
    defer db.Close()
    
    // 2. 创建MySQL BatchFlow实例
    // 内部架构：ThrottledBatchExecutor -> SQLBatchProcessor -> MySQLDriver
    config := batchflow.PipelineConfig{
        BufferSize:    1000,        // 缓冲区大小
        FlushSize:     100,         // 批量刷新大小
        FlushInterval: 5 * time.Second, // 刷新间隔
    }
    batch := batchflow.NewMySQLBatchFlow(ctx, db, config)

    // 3. 定义 schema（表结构定义，与数据库类型解耦）
    userSchema := batchflow.NewSchema(
        "users",                    // 表名
        batchflow.ConflictIgnoreOperationConfig,     // 冲突策略
        "id", "name", "email",      // 列名
    )

    // 4. 创建并提交请求
    request := batchflow.NewRequest(userSchema).
        SetInt64("id", 1).
        SetString("name", "John").
        SetString("email", "john@example.com")

    if err := batch.Submit(ctx, request); err != nil {
        panic(err)
    }
    
    // 5. 监听错误
    go func() {
        errorChan := batch.ErrorChan(10)
        for err := range errorChan {
            log.Printf("Batch processing error: %v", err)
        }
    }()
}
```

注意：
- Submit 会在尝试入队前优先检查 ctx.Err()（取消/超时将立即返回，不会进入内部批处理通道）。请在提交前妥善管理 context 生命周期，避免无效提交。

延伸阅读
- [使用示例](docs/guides/examples.md)
- [配置指南](docs/api/configuration.md)

### Redis 使用示例

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/redis/go-redis/v9"
    "github.com/rushairer/batchflow"

)

func main() {
    ctx := context.Background()
    
    // 1. 创建Redis连接
    rdb := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
    defer rdb.Close()
    
    // 2. 创建Redis BatchFlow实例
    // 内部架构：ThrottledBatchExecutor -> RedisBatchProcessor -> RedisDriver
    config := batchflow.PipelineConfig{
        BufferSize:    1000,
        FlushSize:     100,
        FlushInterval: 5 * time.Second,
    }
    batch := batchflow.NewRedisBatchFlow(ctx, rdb, config)

    // 3. 定义 Redis schema（使用 SETEX 命令格式）
    cacheSchema := batchflow.NewSchema(
        "cache",                    // 逻辑表名
        batchflow.ConflictReplace,    // Redis默认覆盖
        "cmd", "key", "ttl", "value", // SETEX 命令参数顺序
    )

    // 4. 提交Redis数据（SETEX 命令）
    request := batchflow.NewRequest(cacheSchema).
        SetString("cmd", "SETEX").
        SetString("key", "user:1").
        SetInt64("ttl", 3600).      // TTL in seconds
        SetString("value", `{"name":"John Doe","email":"john@example.com"}`)

    if err := batch.Submit(ctx, request); err != nil {
        panic(err)
    }
    
    // 5. 监听错误
    go func() {
        errorChan := batch.ErrorChan(10)
        for err := range errorChan {
            log.Printf("Redis batch processing error: %v", err)
        }
    }()
}
```

### 测试使用

延伸阅读
- [测试指南](docs/guides/testing.md)
- [集成测试指南](docs/guides/integration-tests.md)

```go
func TestBatchFlow(t *testing.T) {
    ctx := context.Background()
    
    // 使用模拟执行器进行测试
    // 内部使用 MockExecutor 直接实现 BatchExecutor 接口
    config := batchflow.PipelineConfig{
        BufferSize:    100,
        FlushSize:     10,
        FlushInterval: time.Second,
    }
    batch, mockExecutor := batchflow.NewBatchFlowWithMock(ctx, config)
    
    // 定义测试schema
    testSchema := batchflow.NewSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")
    
    // 提交测试数据
    request := batchflow.NewRequest(testSchema).
        SetInt64("id", 1).
        SetString("name", "test")
    
    err := batch.Submit(ctx, request)
    assert.NoError(t, err)
    
    // 验证模拟执行器的调用
    time.Sleep(100 * time.Millisecond) // 等待批量处理
    assert.True(t, mockExecutor.WasCalled())
    
    // 获取执行的数据
    executedData := mockExecutor.GetExecutedData()
    assert.Len(t, executedData, 1)
}
```

## 📡 监控与指标（MetricsReporter）

- 功能：统一上报入队延迟、攒批耗时、执行耗时、批大小、错误计数、执行并发、队列长度、在途批次等关键阶段与状态
- 使用场景：
  - 开箱即用观测（Prometheus + Grafana）
  - 接入自有监控体系（实现自定义 Reporter）
- 配置要点：
  - 默认 NoopMetricsReporter（零开销，未注入时不产生任何观测）
  - 务必在 NewBatchFlow 之前对执行器注入 Reporter（WithMetricsReporter）
  - NewBatchFlow 会尊重已注入的 Reporter，不会覆盖为 Noop

最小示例（Prometheus 快速上手）
```go
pm := integration.NewPrometheusMetrics()
go pm.StartServer(9090)
defer pm.StopServer()

exec := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, driver)
reporter := integration.NewPrometheusMetricsReporter(pm, "postgres", "user_batch")
exec = exec.WithMetricsReporter(reporter)

bs := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, exec)
defer bs.Close()
```

延伸阅读
- [监控快速上手（Prometheus + Grafana）](docs/guides/monitoring-quickstart.md)
- [自定义 MetricsReporter 指南](docs/guides/custom-metrics-reporter.md)
- [API 参考（MetricsReporter 小节）](docs/api/reference.md)

#### Prometheus + Grafana 快速监控

BatchFlow 支持 Prometheus 指标收集和 Grafana 可视化，让你能够实时监控性能曲线变化。

- 快速启动监控
```bash
# 使用 Make 命令（推荐）
make monitoring                           # 启动监控环境
make test-integration-with-monitoring     # 启动监控后运行测试
```

- 访问监控界面
  - Grafana 仪表板: http://localhost:3000 (admin/admin)
  - Prometheus 控制台: http://localhost:9091
  - BatchFlow 指标: http://localhost:9090/metrics

- 监控指标
  - 性能指标: RPS、响应时间、批处理时间
  - 资源指标: 内存使用、并发工作线程、活跃连接
  - 质量指标: 数据完整性率、错误率

详细使用说明请参考：[监控指南](docs/guides/monitoring.md)

#### Retry 指标

- 指标设计
  - 每次判定为“可重试”都会上报一次：IncError(schema.Name, "retry:"+reason)
  - 最终失败（达到最大次数或不可重试）会上报：IncError(schema.Name, "final:"+reason)
  - 执行耗时统计（包含重试与退避）：ObserveExecuteDuration(schema.Name, len(data), duration, status)
- 常见原因标签（reason）
  - deadlock、lock_timeout、timeout、connection、io、context、non_retryable
- PromQL 示例
  - 各表重试速率：`sum(rate(batchflow_errors_total{type=~"retry:.*"}[5m])) by (table,type)`
  - 最终失败速率：`sum(rate(batchflow_errors_total{type=~"final:.*"}[5m])) by (table,type)`
  - 重试占比（近5分钟）：`sum(rate(batchflow_errors_total{type=~"retry:.*"}[5m])) / sum(rate(batchflow_errors_total{type=~"(retry:|final:).*"}[5m]))`
- 实践建议
  - 观察“retry:*”与“final:*”的比值，若“final:*”持续升高需关注数据库稳定性与退避配置
  - ObserveExecuteDuration 已包含重试与退避时间，P95/99 将反映重试导致的尾部放大

#### 运行时只读探测：MetricsReporter()

提示：无需实现专门的接口类型，只需让执行器类型暴露 MetricsReporter() 方法即可；框架会在构造期进行只读探测并在必要时内部使用 Noop 兜底，不会写回执行器。

- 功能定位：为执行器（BatchExecutor 实现者）提供一种“可选能力”以暴露其当前 MetricsReporter；NewBatchFlow 会基于该能力安全决定是否注入默认的 NoopMetricsReporter，从而避免误覆盖自定义执行器的监控实现。
- 适用场景：
  - 你实现了自定义执行器，希望显式告知框架“我已有/没有 MetricsReporter”；
  - 希望 NewBatchFlow 能在“reporter 为空时自动注入 Noop”，“不为空时完全复用现有 reporter”，而不对未知执行器做强行覆盖。

接口说明
- 可选只读探测：若执行器类型暴露 MetricsReporter() 方法，NewBatchFlow 将在构造期读取该方法返回值。
  - 返回非 nil：复用现有 Reporter，不做覆盖
  - 返回 nil：在 BatchFlow 内部使用本地 NoopMetricsReporter 进行自有观测，不写回执行器
- 建议：在具体类型（如 ThrottledBatchExecutor）上进行 reporter 注入（WithMetricsReporter），并在 NewBatchFlow 之前完成。

参数与返回值
- 返回值
  - MetricsReporter：当前执行器使用的指标上报器实例；当执行器尚未配置 reporter 时，应返回 nil。
- 约定
  - 返回 nil 表示“未设置任何 reporter”，框架可注入 NewNoopMetricsReporter 作为默认实现；
  - 返回非 nil 表示“已自行设置 reporter”，框架将尊重现有实现，绝不覆盖。

典型用法示例

1) 执行器实现 MetricsProvider
```go
type MyExecutor struct {
    reporter batchflow.MetricsReporter
}
func (e *MyExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
    // ...
    return nil
}
func (e *MyExecutor) WithMetricsReporter(r batchflow.MetricsReporter) batchflow.BatchExecutor {
    e.reporter = r
    return e
}
// 实现可选能力
func (e *MyExecutor) MetricsReporter() batchflow.MetricsReporter { return e.reporter }

// 组合到 BatchFlow
exec := &MyExecutor{}
// 若未设置 reporter，NewBatchFlow 将自动注入 Noop（不覆盖已有实现）
bs := batchflow.NewBatchFlow(ctx, cfg.BufferSize, cfg.FlushSize, cfg.FlushInterval, exec)
```

2) 显式注入自定义 Reporter（推荐）
```go
exec := &MyExecutor{}
prom := integration.NewPrometheusMetricsReporter(pm, "mysql", "user_batch")
exec = exec.WithMetricsReporter(prom)
// NewBatchFlow 发现 MetricsProvider 返回非 nil，将复用 prom，不会覆盖
bs := batchflow.NewBatchFlow(ctx, cfg.BufferSize, cfg.FlushSize, cfg.FlushInterval, exec)
```

与 NewBatchFlow 的交互逻辑（简述）
- 若执行器实现了 MetricsProvider：
  - MetricsReporter() 返回非 nil：直接复用该 reporter；
  - 返回 nil：BatchFlow 在内部使用本地 NoopMetricsReporter 兜底，不写回执行器。
- 若执行器未实现 MetricsProvider：
  - 不会强制覆盖执行器内部状态；NewBatchFlow 仅在内部使用本地 Noop 进行自有观测，保持外部行为稳定。

异常处理机制
- ctx 取消与超时：执行器自身应优先处理上下文取消；框架不会因 reporter 缺失而改变取消语义。
- reporter 为空：
  - 对实现 MetricsProvider 的执行器：返回 nil 即可，框架会自动注入 Noop；
  - 对未实现 MetricsProvider 的执行器：框架不会修改执行器，只在 BatchFlow 内部使用本地 Noop，避免 panic 或 nil 调用。
- 执行器忽略 WithMetricsReporter：
  - 若执行器实现 MetricsProvider 且始终返回 nil，将被注入 Noop；若未实现 MetricsProvider，则由执行器自行负责内部调用安全（框架内部仍使用本地 Noop，避免外泄）。

性能考量
- 零额外开销：仅一次类型断言与空值判断；NoopMetricsReporter 方法为空实现，分支预测友好，基本为零成本。
- 无锁设计：建议执行器缓存 reporter 指针，不在热点路径加锁；NewBatchFlow 注入在构造期完成，不进入热路径。
- 重试与限流：引入 MetricsProvider 不改变 ExecuteBatch 的重试/限流与指标上报路径；在 ctx.Done() 场景下仍能保证统一上报。

最佳实践
- 在构造期（NewXxx 或 NewBatchFlow 之前）通过 WithMetricsReporter 显式注入自定义 reporter；
- 若短期无监控接入需求，无需实现 MetricsProvider，框架会在内部使用本地 Noop 保持行为稳定；
- 实现 MetricsProvider 时，确保返回值与 WithMetricsReporter 设置同步，避免出现“返回 nil 但内部已使用非空 reporter”的不一致状态。

## 📋 详细功能

延伸阅读
- [API 参考](docs/api/reference.md)
- [使用示例](docs/guides/examples.md)

### API 设计模式

#### 默认方式（推荐）
```go
// SQL数据库
mysqlBatch := batchflow.NewMySQLBatchFlow(ctx, db, config)
postgresBatch := batchflow.NewPostgreSQLBatchFlow(ctx, db, config)
sqliteBatch := batchflow.NewSQLiteBatchFlow(ctx, db, config)

// NoSQL数据库
redisBatch := batchflow.NewRedisBatchFlow(ctx, redisClient, config)

// 测试
batch, mockExecutor := batchflow.NewBatchFlowWithMock(ctx, config)
```

#### 自定义方式（扩展支持）
```go
// SQL数据库：支持自定义SQLDriver
customSQLDriver := &MyCustomSQLDriver{}
mysqlBatch := batchflow.NewMySQLBatchFlowWithDriver(ctx, db, config, customSQLDriver)

// Redis数据库：支持自定义RedisDriver
customRedisDriver := &MyCustomRedisDriver{}
redisBatch := batchflow.NewRedisBatchFlowWithDriver(ctx, redisClient, config, customRedisDriver)

// 测试：使用特定Driver的Mock
batch, mockExecutor := batchflow.NewBatchFlowWithMockDriver(ctx, config, customSQLDriver)

// 完全自定义：实现自己的BatchExecutor
type MyExecutor struct {
    // 自定义字段
}

func (e *MyExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
    // 自定义实现
    return nil
}

func (e *MyExecutor) WithMetricsReporter(reporter batchflow.MetricsReporter) batchflow.BatchExecutor {
    // 设置指标报告器
    return e
}

customExecutor := &MyExecutor{}
batch := batchflow.NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, customExecutor)
```

### 冲突处理策略

```go
type ConflictStrategy int

const (
    ConflictIgnore  ConflictStrategy = iota // 跳过冲突
    ConflictReplace                         // 覆盖冲突
    ConflictUpdate                          // 更新冲突
)
```

### Schema 设计
```go
// Schema专注于表结构定义，与数据库类型解耦
userSchema := batchflow.NewSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")
productSchema := batchflow.NewSchema("products", batchflow.ConflictUpdate, "id", "name", "price")

// 同一个Schema可以在不同数据库类型间复用
```

### 生成的 SQL 示例

#### MySQL
- **ConflictIgnore**: `INSERT IGNORE INTO users (id, name) VALUES (?, ?)`
- **ConflictReplace**: `REPLACE INTO users (id, name) VALUES (?, ?)`
- **ConflictUpdate**: `INSERT INTO users (id, name) VALUES (?, ?) ON DUPLICATE KEY UPDATE name = VALUES(name)`

#### PostgreSQL
- **ConflictIgnore**: `INSERT INTO users (id, name) VALUES (?, ?) ON CONFLICT DO NOTHING`
- **ConflictUpdate**: `INSERT INTO users (id, name) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`

#### SQLite
- **ConflictIgnore**: `INSERT OR IGNORE INTO users (id, name) VALUES (?, ?)`
- **ConflictReplace**: `INSERT OR REPLACE INTO users (id, name) VALUES (?, ?)`
- **ConflictUpdate**: `INSERT INTO users (id, name) VALUES (?, ?) ON CONFLICT DO UPDATE SET name = excluded.name`

### 类型化的列操作

```go
request := batchflow.NewRequest(schema).
    SetInt32("age", 30).
    SetInt64("id", 12345).
    SetFloat64("salary", 75000.50).
    SetString("name", "John Doe").
    SetBool("is_active", true).
    SetTime("created_at", time.Now()).
    SetBytes("data", []byte("binary data")).
    SetNull("optional_field")
```

### 获取类型化的值

```go
if name, err := request.GetString("name"); err == nil {
    fmt.Printf("Name: %s", name)
}

if age, err := request.GetInt32("age"); err == nil {
    fmt.Printf("Age: %d", age)
}
```

## 高级用法

### 多数据库支持

```go
import (
    "database/sql"
    "github.com/redis/go-redis/v9"
    _ "github.com/go-sql-driver/mysql"
    _ "github.com/lib/pq"
    _ "github.com/mattn/go-sqlite3"
)

func main() {
    ctx := context.Background()
    config := batchflow.PipelineConfig{
        BufferSize:    1000,
        FlushSize:     100,
        FlushInterval: 5 * time.Second,
    }
    
    // SQL数据库
    
    // MySQL
    mysqlDB, _ := sql.Open("mysql", "user:password@tcp(localhost:3306)/testdb")
    mysqlBatch := batchflow.NewMySQLBatchFlow(ctx, mysqlDB, config)
    
    // PostgreSQL
    postgresDB, _ := sql.Open("postgres", "postgres://user:password@localhost/testdb?sslmode=disable")
    postgresBatch := batchflow.NewPostgreSQLBatchFlow(ctx, postgresDB, config)
    
    // SQLite
    sqliteDB, _ := sql.Open("sqlite3", "./test.db")
    sqliteBatch := batchflow.NewSQLiteBatchFlow(ctx, sqliteDB, config)
    
    // NoSQL数据库
    
    // Redis
    redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    redisBatch := batchflow.NewRedisBatchFlow(ctx, redisClient, config)
    
    // 定义通用schema（可在不同数据库间复用）
    userSchema := batchflow.NewSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
    productSchema := batchflow.NewSchema("products", batchflow.ConflictUpdate, "id", "name", "price")
    
    // Redis专用schema（SETEX命令格式）
    cacheSchema := batchflow.NewSchema("cache", batchflow.ConflictReplace, "cmd", "key", "ttl", "value")
    
    // 每个BatchFlow处理对应数据库的多个表
    
    // MySQL处理用户和产品表
    mysqlBatch.Submit(ctx, batchflow.NewRequest(userSchema).SetInt64("id", 1).SetString("name", "User1"))
    mysqlBatch.Submit(ctx, batchflow.NewRequest(productSchema).SetInt64("id", 1).SetString("name", "Product1").SetFloat64("price", 99.99))
    
    // PostgreSQL处理相同的schema
    postgresBatch.Submit(ctx, batchflow.NewRequest(userSchema).SetInt64("id", 2).SetString("name", "User2"))
    
    // Redis处理缓存数据（使用SETEX命令）
    redisBatch.Submit(ctx, batchflow.NewRequest(cacheSchema).
        SetString("cmd", "SETEX").
        SetString("key", "user:1").
        SetInt64("ttl", 3600).
        SetString("value", `{"name":"User1","active":true}`))
}
```

### 第三方扩展示例

#### 扩展SQL数据库支持（如TiDB）
```go
// 实现SQLDriver接口
type TiDBDriver struct{}

func (d *TiDBDriver) GenerateInsertSQL(schema batchflow.SchemaInterface, data []map[string]any) (string, []any, error) {
    // TiDB特定的批量插入优化
    // 可以使用TiDB的特殊语法或优化
    return sql, args, nil
}

 // 使用自定义Driver，内部仍沿用三层架构（ThrottledBatchExecutor → SQLBatchProcessor → SQLDriver）
tidbDriver := &TiDBDriver{}
batch := batchflow.NewMySQLBatchFlowWithDriver(ctx, tidbDB, config, tidbDriver)
```

#### 扩展NoSQL数据库支持（如MongoDB）
```go
// 直接实现BatchExecutor接口
type MongoExecutor struct {
    client          *mongo.Client
    metricsReporter batchflow.MetricsReporter
}

func NewMongoBatchExecutor(client *mongo.Client) *MongoExecutor {
    return &MongoExecutor{client: client}
}

func (e *MongoExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
    if len(data) == 0 {
        return nil
    }
    
    // MongoDB特定的批量插入逻辑
    collection := e.client.Database("mydb").Collection(schema.Name)
    
    // 转换数据格式
    docs := make([]interface{}, len(data))
    for i, row := range data {
        docs[i] = row
    }
    
    // 执行批量插入
    _, err := collection.InsertMany(ctx, docs)
    return err
}

func (e *MongoExecutor) WithMetricsReporter(reporter batchflow.MetricsReporter) batchflow.BatchExecutor {
    e.metricsReporter = reporter
    return e
}

// 创建MongoDB BatchFlow
func NewMongoBatchFlow(ctx context.Context, client *mongo.Client, config batchflow.PipelineConfig) *batchflow.BatchFlow {
    executor := NewMongoBatchExecutor(client)
    return batchflow.NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, executor)
}

// 使用
mongoClient, _ := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
mongoBatch := NewMongoBatchFlow(ctx, mongoClient, config)
```

### 可选并发限流示例

```go
// 高级用法：自行构造可限流的执行器，再创建 BatchFlow
db, _ := sql.Open("mysql", dsn)
// 构造 SQL 执行器，并限制同时执行的批次数为 8
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
    WithConcurrencyLimit(8)

// 创建 BatchFlow（管道配置）
cfg := batchflow.PipelineConfig{BufferSize: 5000, FlushSize: 200, FlushInterval: 100 * time.Millisecond}
batch := batchflow.NewBatchFlow(ctx, cfg.BufferSize, cfg.FlushSize, cfg.FlushInterval, executor)
```

说明：
- limit <= 0 时不启用限流，行为等价于默认
- 限流发生在 ExecuteBatch 入口，避免攒批后过度并发压垮数据库
- 指标上报与错误处理逻辑保持一致

### 框架集成示例

延伸阅读
- [使用示例](docs/guides/examples.md)
- [API 参考](docs/api/reference.md)

```go
// 与GORM集成
gormDB, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{})
sqlDB, _ := gormDB.DB()
batch := batchflow.NewMySQLBatchFlow(ctx, sqlDB, config)

// 与sqlx集成
sqlxDB, _ := sqlx.Connect("mysql", dsn)
batch := batchflow.NewMySQLBatchFlow(ctx, sqlxDB.DB, config)
```

## ⚡ 性能优化

### 内存效率
- **指针传递**：使用 `StandardPipeline[*Request]` 而非值传递，减少内存复制
- **智能聚合**：按 schema 指针自动聚合相同表的请求，减少数据库操作次数
- **全局Driver共享**：SQLDriver实例全局共享，避免重复创建
- **零拷贝设计**：Request数据直接传递，无额外序列化开销

### 并发处理
- **多goroutine安全**：支持多 goroutine 并发提交请求
- **自动分组**：按 schema 指针聚合，确保相同表的请求批量处理
- **异步处理**：基于 go-pipeline 的异步处理，不阻塞主线程
- **背压控制**：缓冲区满时自动背压，防止内存溢出

### 数据库优化
- **批量插入**：自动生成优化的批量INSERT语句
- **事务保证**：每个批次使用单个事务，保证数据一致性
- **连接复用**：用户自己管理连接池，支持连接复用
- **SQL优化**：针对不同数据库生成最优的SQL语法

延伸阅读
- [调优最佳实践](docs/guides/tuning.md)
- [性能分析报告](docs/reports/PERFORMANCE_ANALYSIS.md)
- [SQLite 优化建议](docs/reports/SQLITE_OPTIMIZATION.md)


## 📊 质量评估

延伸阅读
- [质量评估方法](docs/development/quality.md)
- [测试报告分析](docs/reports/TEST_REPORT_ANALYSIS.md)

基于最新集成测试报告的项目质量状态评估：

### 测试通过率
| 数据库 | 测试数量 | 通过 | 失败 | 通过率 | BatchFlow 状态 |
|--------|----------|------|------|--------|---------------|
| **SQLite** | 5 | 4 | 1 | 80% | ✅ 正常（失败为 SQLite 架构限制） |
| **MySQL** | 5 | 5 | 0 | 100% | ✅ 优秀 |
| **PostgreSQL** | 5 | 5 | 0 | 100% | ✅ 优秀 |
| **Redis** | 5 | 5 | 0 | 100% | ✅ 优秀（三层架构重构完成） |
| **总计** | 20 | 19 | 1 | 95% | ✅ 优秀 |

### 性能指标
| 数据库 | 平均 RPS | 最大 RPS | 数据完整性 | BatchFlow 性能评级 |
|--------|----------|----------|------------|------------------|
| **SQLite** | 105,246 | 199,071 | 80% 测试通过 | ✅ 符合 SQLite 预期 |
| **MySQL** | 144,879 | 168,472 | 100% 测试通过 | ✅ 优秀 |
| **PostgreSQL** | 152,586 | 191,037 | 100% 测试通过 | ✅ 优秀 |
| **Redis** | 180,000+ | 250,000+ | 100% 测试通过 | ✅ 优秀（三层架构优化） |

### 技术说明
🔵 **SQLite 架构限制**（非项目缺陷）：SQLite 是单写入者数据库，大批次并发写入失败属于数据库引擎固有限制  
🟢 **BatchFlow 功能完整**：所有核心功能正常，错误检测机制完善  
🟢 **代码质量优秀**：在 MySQL/PostgreSQL/Redis 上表现优异，证明实现正确  

### 发布状态
**当前状态**：✅ **可以发布**  
**项目质量**：BatchFlow 核心功能完整，所有数据库驱动稳定可用  
**SQLite 说明**：测试失败源于 SQLite 单写入者架构限制，非项目问题  
**使用建议**：
- 高并发场景推荐 MySQL/PostgreSQL/Redis
- 轻量级场景可用 SQLite
- 缓存场景推荐 Redis（性能优异）

## 📚 文档导航

BatchFlow 提供完整的文档体系，按使用场景分类：

### 🚀 快速开始

- [📖 文档索引](docs/index.md) - 完整的文档导航和索引
- [💡 使用示例](docs/guides/examples.md) - 丰富的代码示例和最佳实践

### 📖 API 文档
- [🚀 API 参考](docs/api/reference.md) - 完整的 API 文档和使用指南
- [⚙️ 配置指南](docs/api/configuration.md) - 详细的配置参数说明

### 📖 用户指南
- [🧪 测试指南](docs/guides/testing.md) - 完整的测试文档和 Redis 测试报告
- [📊 监控指南](docs/guides/monitoring.md) - Prometheus + Grafana 监控系统
- [⚙️ 执行器能力与度量接口（重构后）](docs/guides/executor-capabilities.md) - 自类型泛型能力接口 + 只读探测 + Noop 兜底
- [🔧 故障排除](docs/guides/troubleshooting.md) - 完整的问题诊断和解决方案
- [🔗 集成测试](docs/guides/integration-tests.md) - 集成测试详细说明
- [🛠 调优最佳实践](docs/guides/tuning.md) - 指标细化 + 自适应策略 + 基准/压力流程

### 🔧 开发文档
- [🏗️ 架构设计](docs/development/architecture.md) - 系统架构和设计理念
- [🤝 贡献指南](docs/development/contributing.md) - 如何参与项目开发
- [📋 发布清单](docs/development/release.md) - 版本发布流程
- [📈 质量评估](docs/development/quality.md) - 代码质量分析报告
- [🐛 修复记录](docs/development/changelog.md) - 重要修复和改进记录

### 📊 测试报告
- [📈 性能分析](docs/reports/PERFORMANCE_ANALYSIS.md) - SQLite 性能测试分析
- [🔧 优化建议](docs/reports/SQLITE_OPTIMIZATION.md) - SQLite 优化策略
- [📋 测试报告](docs/reports/TEST_REPORT_ANALYSIS.md) - 详细测试结果分析
- [🛠️ 工具说明](docs/reports/sqlite-tools.md) - SQLite 测试工具文档

## 🐛 重要修复记录

延伸阅读
- [修复与改进记录](docs/development/changelog.md)

### 数据完整性监控指标修复 (2025-09-30)
- **问题**：Grafana 监控面板显示数据完整性为 10000% 而非正常的 100%
- **原因**：Prometheus 指标范围定义不一致（0-1 vs 0-100）
- **修复**：统一指标范围为 0-1，修复初始化和记录逻辑
- **影响**：✅ 监控面板现在正确显示数据完整性百分比
- **详情**：[修复日志](docs/development/changelog.md)

## 📋 测试

延伸阅读
- [测试指南](docs/guides/testing.md)
- [集成测试指南](docs/guides/integration-tests.md)

### 单元测试
```bash
# 运行所有单元测试
go test -v

# 运行测试覆盖率分析
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 集成测试
```bash
# 运行所有数据库集成测试
make docker-all-tests

# 运行单个数据库测试
make docker-mysql-test      # MySQL 测试
make docker-postgres-test   # PostgreSQL 测试
make docker-sqlite-test     # SQLite 测试
make docker-redis-test      # Redis 测试
```

> 提示：性能观测请参见上文“📡 监控与指标（MetricsReporter）”章节的「Prometheus + Grafana 快速监控」小节。

### 测试覆盖范围
- ✅ 基本批量处理功能
- ✅ Schema 分组逻辑
- ✅ SQL 生成正确性
- ✅ Redis 操作生成正确性
- ✅ 不同数据库类型和冲突策略
- ✅ 错误处理和边界条件
- ✅ 并发安全性测试
- ✅ 大数据量压力测试
- ✅ 数据库连接异常处理
- ✅ Redis Pipeline 批量执行

*详细测试文档：[集成测试指南](docs/guides/integration-tests.md)*

## 🏗️ 文件结构

```
batchflow/
├── README.md
├── go.mod
├── go.sum
├── Makefile
├── .golangci.yml
├── .env.test
├── .env.sqlite.test
├── docker-compose.integration.yml
├── Dockerfile.integration
├── Dockerfile.sqlite.integration
├── batchflow.go              # 主入口与管道工厂
├── driver.go                # 驱动接口与实现入口（SQL/Redis等）
├── executor.go              # 执行器（含可选并发限流：WithConcurrencyLimit）
├── processor.go             # 处理器（SQL/Redis等各自批处理实现）
├── metrics_reporter.go      # 指标上报接口与默认实现
├── schema.go                # Schema 定义
├── request.go               # Request 定义
├── error.go                 # 错误定义
├── batchflow_test.go
├── benchmark_test.go
├── boundary_test.go
├── concurrency_test.go
├── db_connection_test.go
├── error_handling_test.go
├── large_data_test.go
├── docs/
│   ├── index.md
│   ├── api/
│   │   ├── reference.md
│   │   └── configuration.md
│   ├── guides/
│   │   ├── examples.md
│   │   ├── testing.md
│   │   ├── monitoring.md
│   │   ├── monitoring-quickstart.md
│   │   ├── custom-metrics-reporter.md
│   │   ├── tuning.md
│   │   ├── troubleshooting.md
│   │   └── integration-tests.md
│   ├── development/
│   │   ├── architecture.md
│   │   ├── contributing.md
│   │   ├── changelog.md
│   │   ├── quality.md
│   │   └── release.md
│   └── reports/
│       ├── PERFORMANCE_ANALYSIS.md
│       ├── SQLITE_OPTIMIZATION.md
│       ├── TEST_REPORT_ANALYSIS.md
│       └── sqlite-tools.md
├── scripts/

└── test/
    ├── integration/
    │   ├── config.go
    │   ├── main.go
    │   ├── metrics_reporter.go
    │   ├── prometheus.go
    │   ├── prometheus.yml
    │   ├── redis_tests.go
    │   ├── reports.go
    │   ├── run-single-db-test.sh
    │   ├── sql_tests.go
    │   ├── types.go
    │   ├── utils.go
    │   └── grafana/
    │       └── provisioning/...
    ├── sql/
    │   ├── mysql/init.sql
    │   ├── postgres/init.sql
    │   └── sqlite/init.sql
    └── sqlite/
        └── tools/...
```




## 🤝 贡献

延伸阅读
- [贡献指南](docs/development/contributing.md)
- [发布流程](docs/development/release.md)

欢迎提交 Issue 和 Pull Request！

### 开发流程
1. Fork 项目
2. 创建功能分支
3. 运行完整测试：`make ci`
4. 提交 Pull Request

### 测试要求
- 所有单元测试必须通过
- 集成测试通过率 ≥ 90%
- 代码覆盖率 ≥ 60%
- 通过 golangci-lint 检查




## 📄 许可证

MIT License