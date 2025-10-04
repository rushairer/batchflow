# BatchFlow 使用示例

## 🎯 快速开始示例

### 基础 MySQL 批量插入

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
    // 连接MySQL
    db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/testdb?parseTime=true")
    if err != nil {
        panic(err)
    }
    defer db.Close()
    
    // 创建BatchFlow
    ctx := context.Background()
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 定义表结构
    schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig,
        "id", "name", "email", "age", "created_at")
    
    // 批量插入10000条记录
    for i := 0; i < 10000; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("name", fmt.Sprintf("User_%d", i)).
            SetString("email", fmt.Sprintf("user_%d@example.com", i)).
            SetInt64("age", int64(20+i%50)).
            SetTime("created_at", time.Now())
        
        if err := batchFlow.Submit(ctx, request); err != nil {
            fmt.Printf("提交失败: %v\n", err)
        }
    }
    
    fmt.Println("批量插入完成!")
}
```

## 🗄️ 多数据库示例

### PostgreSQL 示例

```go
func postgresqlExample() {
    // 连接PostgreSQL
    db, err := sql.Open("postgres", "postgres://user:password@localhost/testdb?sslmode=disable")
    if err != nil {
        panic(err)
    }
    defer db.Close()
    
    // 创建执行器
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // PostgreSQL特定Schema
    schema := batchflow.NewSQLSchema("products", batchflow.ConflictUpdateOperationConfig,
        "id", "name", "price", "category", "updated_at")
    
    // 批量更新产品信息
    products := []Product{
        {ID: 1, Name: "Laptop", Price: 999.99, Category: "Electronics"},
        {ID: 2, Name: "Mouse", Price: 29.99, Category: "Electronics"},
        // ... 更多产品
    }
    
    for _, product := range products {
        request := batchflow.NewRequest(schema).
            SetInt64("id", product.ID).
            SetString("name", product.Name).
            SetFloat64("price", product.Price).
            SetString("category", product.Category).
            SetTime("updated_at", time.Now())
        
        batchFlow.Submit(ctx, request)
    }
}
```

### SQLite 示例

```go
func sqliteExample() {
    // 连接SQLite
    db, err := sql.Open("sqlite3", "./test.db")
    if err != nil {
        panic(err)
    }
    defer db.Close()
    
    // SQLite优化配置（较小的批次）
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultSQLiteDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 1000, 100, 200*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 创建日志表
    schema := batchflow.NewSQLSchema("logs", batchflow.ConflictIgnoreOperationConfig,
        "id", "level", "message", "timestamp")
    
    // 批量插入日志
    logLevels := []string{"INFO", "WARN", "ERROR", "DEBUG"}
    for i := 0; i < 5000; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("level", logLevels[i%len(logLevels)]).
            SetString("message", fmt.Sprintf("Log message %d", i)).
            SetTime("timestamp", time.Now())
        
        batchFlow.Submit(ctx, request)
    }
}
```

### Redis 示例

```go
func redisExample() {
    // 连接Redis
    rdb := redis.NewClient(&redis.Options{
        Addr:     "localhost:6379",
        Password: "",
        DB:       0,
    })
    defer rdb.Close()
    
    // 创建Redis执行器
    executor := batchflow.NewRedisThrottledBatchExecutor(rdb)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 500, 50*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // Redis命令Schema
    schema := batchflow.NewSchema("cache",
        "cmd", "key", "value", "ex_flag", "ttl")
    
    // 批量缓存用户会话
    for i := 0; i < 10000; i++ {
        sessionData := fmt.Sprintf(`{"user_id":%d,"login_time":"%s","ip":"192.168.1.%d"}`, 
            i, time.Now().Format(time.RFC3339), i%255)
        
        request := batchflow.NewRequest(schema).
            SetString("cmd", "SET").
            SetString("key", fmt.Sprintf("session:%d", i)).
            SetString("value", sessionData).
            SetString("ex_flag", "EX").
            SetInt64("ttl", 3600) // 1小时过期
        
        batchFlow.Submit(ctx, request)
    }
}
```

## 📊 监控集成示例

### Prometheus 监控集成

```go
func withPrometheusMonitoring() {
    // 创建Prometheus指标收集器
    prometheusMetrics := NewPrometheusMetrics()
    
    // 启动指标服务器
    go func() {
        if err := prometheusMetrics.StartServer(9090); err != nil {
            log.Printf("Prometheus server failed: %v", err)
        }
    }()
    defer prometheusMetrics.StopServer()
    
    // 创建带监控的执行器
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, "mysql", "user_batch")
    executor = executor.WithMetricsReporter(metricsReporter).(*batchflow.ThrottledBatchExecutor)
    
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 执行批量操作（自动收集指标）
    schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")
    
    for i := 0; i < 50000; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("name", fmt.Sprintf("User_%d", i)).
            SetString("email", fmt.Sprintf("user_%d@example.com", i))
        
        batchFlow.Submit(ctx, request)
        
        // 每1000条记录输出进度
        if i%1000 == 0 {
            fmt.Printf("已处理 %d 条记录\n", i)
        }
    }
    
    fmt.Println("批量操作完成，可访问 http://localhost:9090/metrics 查看指标")
}
```

### 自定义指标报告器

```go
type CustomMetricsReporter struct {
    totalRecords    int64
    successRecords  int64
    failedRecords   int64
    totalDuration   time.Duration
    logger          *log.Logger
}

func NewCustomMetricsReporter() *CustomMetricsReporter {
    return &CustomMetricsReporter{
        logger: log.New(os.Stdout, "[METRICS] ", log.LstdFlags),
    }
}

func (r *CustomMetricsReporter) ObserveExecuteDuration(tableName string, batchSize int, d time.Duration, status string) {
    r.totalRecords += int64(batchSize)
    r.totalDuration += d
    
    if status == "success" {
        r.successRecords += int64(batchSize)
    } else {
        r.failedRecords += int64(batchSize)
    }
    
    // 每处理10个批次输出一次统计
    if r.totalRecords%1000 == 0 {
        avgDuration := r.totalDuration.Milliseconds() / (r.totalRecords / int64(batchSize))
        successRate := float64(r.successRecords) / float64(r.totalRecords) * 100
        
        r.logger.Printf("统计: 总记录=%d, 成功率=%.2f%%, 平均批次耗时=%dms", 
            r.totalRecords, successRate, avgDuration)
    }
}

func withCustomMetrics() {
    // 使用自定义指标报告器
    metricsReporter := NewCustomMetricsReporter()
    
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    executor = executor.WithMetricsReporter(metricsReporter)
    
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 执行批量操作...
}
```

## 🔧 高级配置示例

### 高性能配置

```go
func highPerformanceConfig() {
    // 数据库连接池优化
    db.SetMaxOpenConns(100)    // 最大连接数
    db.SetMaxIdleConns(50)     // 最大空闲连接
    db.SetConnMaxLifetime(time.Hour) // 连接最大生存时间
    
    // 高性能BatchFlow配置
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(
        ctx,
        10000,                    // 大缓冲区
        500,                      // 大批次
        50*time.Millisecond,      // 快速刷新
        executor,
    )
    defer batchFlow.Close()
    
    // 使用事务提升性能
    tx, err := db.Begin()
    if err != nil {
        panic(err)
    }
    defer tx.Rollback()
    
    // 执行批量操作...
    
    // 提交事务
    if err := tx.Commit(); err != nil {
        panic(err)
    }
}
```

### 内存优化配置

```go
func memoryOptimizedConfig() {
    // 内存优化配置
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(
        ctx,
        1000,                     // 小缓冲区
        100,                      // 小批次
        200*time.Millisecond,     // 较慢刷新
        executor,
    )
    defer batchFlow.Close()
    
    // 分批处理大数据集
    const totalRecords = 1000000
    const chunkSize = 10000
    
    for offset := 0; offset < totalRecords; offset += chunkSize {
        end := offset + chunkSize
        if end > totalRecords {
            end = totalRecords
        }
        
        // 处理当前批次
        for i := offset; i < end; i++ {
            request := batchflow.NewRequest(schema).
                SetInt64("id", int64(i)).
                SetString("data", fmt.Sprintf("data_%d", i))
            
            batchFlow.Submit(ctx, request)
        }
        
        // 强制刷新并等待
        runtime.GC()
        time.Sleep(100 * time.Millisecond)
        
        fmt.Printf("已处理 %d/%d 记录\n", end, totalRecords)
    }
}
```

## 🔄 并发处理示例

### 多协程并发插入

```go
func concurrentInsert() {
    const numWorkers = 10
    const recordsPerWorker = 10000
    
    // 创建共享的BatchFlow实例
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 10000, 500, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    schema := batchflow.NewSQLSchema("concurrent_test", batchflow.ConflictIgnoreOperationConfig,
        "id", "worker_id", "data", "created_at")
    
    var wg sync.WaitGroup
    
    // 启动多个工作协程
    for workerID := 0; workerID < numWorkers; workerID++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            
            baseID := id * recordsPerWorker
            for i := 0; i < recordsPerWorker; i++ {
                request := batchflow.NewRequest(schema).
                    SetInt64("id", int64(baseID+i)).
                    SetInt64("worker_id", int64(id)).
                    SetString("data", fmt.Sprintf("worker_%d_data_%d", id, i)).
                    SetTime("created_at", time.Now())
                
                if err := batchFlow.Submit(ctx, request); err != nil {
                    log.Printf("Worker %d submit error: %v", id, err)
                }
            }
            
            fmt.Printf("Worker %d 完成\n", id)
        }(workerID)
    }
    
    wg.Wait()
    fmt.Println("所有工作协程完成")
}
```

## 🛠️ 实用工具函数

### 批量数据生成器

```go
type DataGenerator struct {
    faker *gofakeit.Faker
}

func NewDataGenerator() *DataGenerator {
    return &DataGenerator{
        faker: gofakeit.New(0),
    }
}

func (g *DataGenerator) GenerateUsers(count int) []User {
    users := make([]User, count)
    for i := 0; i < count; i++ {
        users[i] = User{
            ID:       int64(i + 1),
            Name:     g.faker.Name(),
            Email:    g.faker.Email(),
            Phone:    g.faker.Phone(),
            Address:  g.faker.Address().Address,
            Birthday: g.faker.Date(),
        }
    }
    return users
}

func batchInsertUsers() {
    generator := NewDataGenerator()
    users := generator.GenerateUsers(100000)
    
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig,
        "id", "name", "email", "phone", "address", "birthday")
    
    for _, user := range users {
        request := batchflow.NewRequest(schema).
            SetInt64("id", user.ID).
            SetString("name", user.Name).
            SetString("email", user.Email).
            SetString("phone", user.Phone).
            SetString("address", user.Address).
            SetTime("birthday", user.Birthday)
        
        batchFlow.Submit(ctx, request)
    }
}
```

### 进度监控工具

```go
type ProgressMonitor struct {
    total     int64
    processed int64
    startTime time.Time
    mu        sync.Mutex
}

func NewProgressMonitor(total int64) *ProgressMonitor {
    return &ProgressMonitor{
        total:     total,
        startTime: time.Now(),
    }
}

func (p *ProgressMonitor) Update(count int64) {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    p.processed += count
    
    if p.processed%1000 == 0 || p.processed == p.total {
        elapsed := time.Since(p.startTime)
        rate := float64(p.processed) / elapsed.Seconds()
        percentage := float64(p.processed) / float64(p.total) * 100
        
        fmt.Printf("进度: %d/%d (%.1f%%), 速率: %.0f records/sec, 耗时: %v\n",
            p.processed, p.total, percentage, rate, elapsed.Truncate(time.Second))
    }
}

func withProgressMonitoring() {
    const totalRecords = 100000
    monitor := NewProgressMonitor(totalRecords)
    
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    schema := batchflow.NewSQLSchema("progress_test", batchflow.ConflictIgnoreOperationConfig, "id", "data")
    
    for i := 0; i < totalRecords; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("data", fmt.Sprintf("data_%d", i))
        
        batchFlow.Submit(ctx, request)
        monitor.Update(1)
    }
}
```

## 🔍 错误处理示例

### 重试机制

```go
func withRetryMechanism() {
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    schema := batchflow.NewSQLSchema("retry_test", batchflow.ConflictIgnoreOperationConfig, "id", "data")
    
    for i := 0; i < 10000; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("data", fmt.Sprintf("data_%d", i))
        
        // 重试机制
        maxRetries := 3
        for retry := 0; retry < maxRetries; retry++ {
            if err := batchFlow.Submit(ctx, request); err != nil {
                if retry == maxRetries-1 {
                    log.Printf("最终失败 (重试%d次): %v", maxRetries, err)
                } else {
                    log.Printf("重试 %d/%d: %v", retry+1, maxRetries, err)
                    time.Sleep(time.Duration(retry+1) * 100 * time.Millisecond)
                }
            } else {
                break // 成功，跳出重试循环
            }
        }
    }
}
```

### 错误收集和分析

```go
type ErrorCollector struct {
    errors []BatchError
    mu     sync.Mutex
}

type BatchError struct {
    Timestamp time.Time
    RecordID  int64
    Error     error
    Retries   int
}

func (ec *ErrorCollector) RecordError(recordID int64, err error, retries int) {
    ec.mu.Lock()
    defer ec.mu.Unlock()
    
    ec.errors = append(ec.errors, BatchError{
        Timestamp: time.Now(),
        RecordID:  recordID,
        Error:     err,
        Retries:   retries,
    })
}

func (ec *ErrorCollector) GetSummary() {
    ec.mu.Lock()
    defer ec.mu.Unlock()
    
    if len(ec.errors) == 0 {
        fmt.Println("没有错误记录")
        return
    }
    
    fmt.Printf("总错误数: %d\n", len(ec.errors))
    
    // 按错误类型分组
    errorTypes := make(map[string]int)
    for _, e := range ec.errors {
        errorTypes[e.Error.Error()]++
    }
    
    fmt.Println("错误类型分布:")
    for errType, count := range errorTypes {
        fmt.Printf("  %s: %d次\n", errType, count)
    }
}
```

## 📚 相关文档

- [API_REFERENCE.md](API_REFERENCE.md) - 完整API参考
- [MONITORING_GUIDE.md](MONITORING_GUIDE.md) - 监控配置
- [PERFORMANCE_TUNING.md](PERFORMANCE_TUNING.md) - 性能优化
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - 问题排查

---

💡 **提示**：这些示例涵盖了BatchFlow的主要使用场景。建议根据实际需求选择合适的配置和模式。