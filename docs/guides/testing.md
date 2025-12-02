# BatchFlow 测试指南

## 📋 测试体系概览

BatchFlow 采用多层次测试策略，确保代码质量和系统稳定性：

- **单元测试**：核心逻辑和组件测试
- **集成测试**：多数据库端到端测试  
- **性能测试**：压力测试和基准测试
- **监控测试**：指标收集和可视化验证

## 🧪 单元测试

### 运行单元测试

```bash
# 运行所有单元测试
go test -v ./...

# 运行特定包的测试
go test -v ./drivers/mysql
go test -v ./drivers/postgresql
go test -v ./drivers/sqlite
go test -v ./drivers/redis

# 运行测试覆盖率分析
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# 查看覆盖率报告
open coverage.html  # macOS
xdg-open coverage.html  # Linux
```

### 核心测试用例

#### BatchFlow 核心功能测试

```go
func TestBatchFlowBasicOperations(t *testing.T) {
    // 测试基本的批量插入功能
    ctx := context.Background()
    executor := mock.NewMockExecutor()
    batchFlow := batchflow.NewBatchFlow(ctx, 100, 10, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")
    
    // 提交测试数据
    for i := 0; i < 50; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("name", fmt.Sprintf("test_%d", i))
        
        err := batchFlow.Submit(ctx, request)
        assert.NoError(t, err)
    }
    
    // 验证执行结果
    assert.Equal(t, 50, executor.GetProcessedCount())
}
```

#### 数据库驱动测试

```go
func TestMySQLDriver(t *testing.T) {
    db := setupTestDB(t, "mysql")
    defer db.Close()
    
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    
    // 测试批量插入
    schema := batchflow.NewSQLSchema("test_users", batchflow.ConflictIgnoreOperationConfig, 
        "id", "name", "email")
    
    data := []map[string]any{
        {"id": 1, "name": "Alice", "email": "alice@example.com"},
        {"id": 2, "name": "Bob", "email": "bob@example.com"},
    }
    
    err := executor.ExecuteBatch(context.Background(), schema, data)
    assert.NoError(t, err)
    
    // 验证数据插入
    count := getRecordCount(t, db, "test_users")
    assert.Equal(t, 2, count)
}
```

### Mock 测试工具

```go
// test/mock/executor.go
type MockExecutor struct {
    processedBatches []BatchData
    shouldFail       bool
    mu               sync.Mutex
}

func (m *MockExecutor) ExecuteBatch(ctx context.Context, schema *Schema, data []map[string]any) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if m.shouldFail {
        return errors.New("mock execution failed")
    }
    
    m.processedBatches = append(m.processedBatches, BatchData{
        Schema: schema,
        Data:   data,
        Time:   time.Now(),
    })
    
    return nil
}

func (m *MockExecutor) GetProcessedCount() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    total := 0
    for _, batch := range m.processedBatches {
        total += len(batch.Data)
    }
    return total
}
```

### 并发安全与快照断言

- MockExecutor 现在对内部批次写入加锁，新增 SnapshotExecutedBatches() 以提供一次性拷贝快照，避免并发读写竞态。
- 在并发测试或异步断言中，推荐使用快照方法而非直接读取内部切片。

示例：
```go
batch, mock := batchflow.NewBatchFlowWithMock(ctx, batchflow.PipelineConfig{
    BufferSize: 100, FlushSize: 10, FlushInterval: 50*time.Millisecond,
})

// 并发提交若干请求...
// 等待一小段时间让批处理刷新（或使用更稳妥的同步手段）
time.Sleep(100 * time.Millisecond)

// 使用快照进行断言，避免竞态
batches := mock.SnapshotExecutedBatches()
total := 0
for _, b := range batches {
    total += len(b)
}
require.GreaterOrEqual(t, total, expectedMin)
```

## 🔗 集成测试

### 快速运行集成测试

```bash
# 运行所有数据库集成测试
make docker-all-tests

# 运行单个数据库测试
make docker-mysql-test
make docker-postgresql-test  
make docker-sqlite-test
make docker-redis-test

# 本地运行（需要预先启动数据库）
cd test/integration
go run . -db=mysql
go run . -db=postgresql
go run . -db=sqlite
go run . -db=redis
```

### 集成测试配置

```bash
# 环境变量配置
export TEST_DURATION=5m
export CONCURRENT_WORKERS=100
export RECORDS_PER_WORKER=20000
export BATCH_SIZE=200
export BUFFER_SIZE=5000
export PROMETHEUS_ENABLED=true
export PROMETHEUS_PORT=9090

# 数据库连接配置
export MYSQL_DSN="root:password@tcp(localhost:3306)/batchflow_test"
export POSTGRES_DSN="postgres://postgres:password@localhost:5432/batchflow_test?sslmode=disable"
export SQLITE_DSN="./test.db"
export REDIS_DSN="redis://localhost:6379/0"
```

### 集成测试场景

#### 1. 高吞吐量测试
- **目标**：验证系统在高负载下的稳定性
- **配置**：大缓冲区 + 中等批次 + 快速刷新
- **验证**：RPS > 100,000，数据完整性 = 100%

#### 2. 并发工作线程测试  
- **目标**：验证多线程并发安全性
- **配置**：100个并发工作线程
- **验证**：无数据竞争，数据完整性 = 100%

#### 3. 大批次测试
- **目标**：验证大批次处理能力
- **配置**：批次大小 5000，缓冲区 50000
- **验证**：内存使用稳定，无内存泄漏

#### 4. 内存压力测试
- **目标**：验证内存使用优化
- **配置**：小批次 + 大数据量
- **验证**：内存使用 < 500MB

#### 5. 长时间运行测试
- **目标**：验证长期稳定性
- **配置**：运行时间 > 10分钟
- **验证**：性能无衰减，无内存泄漏

### Redis 测试报告

#### Redis 集成测试结果

| 测试场景 | 配置 | 结果 | 性能指标 |
|---------|------|------|----------|
| **高吞吐量测试** | BufferSize: 5000<br>BatchSize: 500<br>Workers: 1 | ✅ 通过 | RPS: 180,000+<br>数据完整性: 100% |
| **并发工作线程测试** | Workers: 100<br>Records: 2,000,000 | ✅ 通过 | RPS: 250,000+<br>数据完整性: 100% |
| **大批次测试** | BatchSize: 5000<br>BufferSize: 50000 | ✅ 通过 | RPS: 200,000+<br>内存使用: < 200MB |
| **内存压力测试** | Records: 5,000,000<br>BatchSize: 100 | ✅ 通过 | 内存峰值: < 300MB<br>GC次数: 正常 |
| **长时间运行测试** | Duration: 10min<br>持续负载 | ✅ 通过 | 性能稳定<br>无内存泄漏 |

#### Redis 特性验证

```go
func TestRedisSpecificFeatures(t *testing.T) {
    rdb := setupRedisClient(t)
    defer rdb.Close()
    
    executor := batchflow.NewRedisThrottledBatchExecutor(rdb)
    batchFlow := batchflow.NewBatchFlow(ctx, 1000, 100, 50*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 测试TTL设置
    schema := batchflow.NewSchema("redis_test",
        "cmd", "key", "value", "ex_flag", "ttl")
    
    request := batchflow.NewRequest(schema).
        SetString("cmd", "SET").
        SetString("key", "test:ttl").
        SetString("value", "test_value").
        SetString("ex_flag", "EX").
        SetInt64("ttl", 60) // 60秒TTL
    
    err := batchFlow.Submit(ctx, request)
    assert.NoError(t, err)
    
    // 等待处理完成
    time.Sleep(100 * time.Millisecond)
    
    // 验证key存在且有TTL
    exists := rdb.Exists(ctx, "test:ttl").Val()
    assert.Equal(t, int64(1), exists)
    
    ttl := rdb.TTL(ctx, "test:ttl").Val()
    assert.True(t, ttl > 0 && ttl <= 60*time.Second)
}
```

#### Redis 性能基准

```go
func BenchmarkRedisBatchInsert(b *testing.B) {
    rdb := setupRedisClient(b)
    defer rdb.Close()
    
    executor := batchflow.NewRedisThrottledBatchExecutor(rdb)
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 500, 50*time.Millisecond, executor)
    defer batchFlow.Close()
    
    schema := batchflow.NewSchema("benchmark",
        "cmd", "key", "value")
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        request := batchflow.NewRequest(schema).
            SetString("cmd", "SET").
            SetString("key", fmt.Sprintf("bench:%d", i)).
            SetString("value", fmt.Sprintf("value_%d", i))
        
        batchFlow.Submit(ctx, request)
    }
}
```

## 📊 性能测试

### 基准测试

```bash
# 运行基准测试
go test -bench=. -benchmem ./...

# 运行特定基准测试
go test -bench=BenchmarkBatchInsert -benchmem

# 生成性能分析报告
go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof
go tool pprof cpu.prof
go tool pprof mem.prof
```

### 压力测试

```go
func TestStressTest(t *testing.T) {
    if testing.Short() {
        t.Skip("跳过压力测试")
    }
    
    db := setupTestDB(t, "mysql")
    defer db.Close()
    
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    batchFlow := batchflow.NewBatchFlow(ctx, 10000, 500, 50*time.Millisecond, executor)
    defer batchFlow.Close()
    
    schema := batchflow.NewSQLSchema("stress_test", batchflow.ConflictIgnoreOperationConfig, "id", "data")
    
    const totalRecords = 1000000
    startTime := time.Now()
    
    for i := 0; i < totalRecords; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("data", strings.Repeat("x", 100)) // 100字节数据
        
        err := batchFlow.Submit(ctx, request)
        assert.NoError(t, err)
    }
    
    duration := time.Since(startTime)
    rps := float64(totalRecords) / duration.Seconds()
    
    t.Logf("压力测试完成: %d 记录, 耗时: %v, RPS: %.0f", totalRecords, duration, rps)
    
    // 验证性能要求
    assert.True(t, rps > 50000, "RPS应该大于50,000")
}
```

## 📈 监控测试

### Prometheus 指标验证

```go
func TestPrometheusMetrics(t *testing.T) {
    prometheusMetrics := NewPrometheusMetrics()
    err := prometheusMetrics.StartServer(9091)
    assert.NoError(t, err)
    defer prometheusMetrics.StopServer()
    
    // 创建带监控的执行器
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, "mysql", "test_instance")
    executor = executor.WithMetricsReporter(metricsReporter).(*batchflow.ThrottledBatchExecutor)
    
    batchFlow := batchflow.NewBatchFlow(ctx, 1000, 100, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 执行一些操作
    schema := batchflow.NewSQLSchema("metrics_test", batchflow.ConflictIgnoreOperationConfig, "id", "data")
    for i := 0; i < 500; i++ {
        request := batchflow.NewRequest(schema).
            SetInt64("id", int64(i)).
            SetString("data", fmt.Sprintf("data_%d", i))
        
        batchFlow.Submit(ctx, request)
    }
    
    // 等待指标更新
    time.Sleep(200 * time.Millisecond)
    
    // 验证指标端点
    resp, err := http.Get("http://localhost:9091/metrics")
    assert.NoError(t, err)
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    assert.NoError(t, err)
    
    metrics := string(body)
    assert.Contains(t, metrics, "batchflow_records_processed_total")
    assert.Contains(t, metrics, "batchflow_records_rate_total")
    assert.Contains(t, metrics, "batchflow_data_integrity_rate")
    assert.Contains(t, metrics, "batchflow_current_rps")
}
```

### Grafana 面板测试

```bash
# 启动完整监控栈
docker-compose -f docker-compose.integration.yml up -d

# 运行集成测试生成数据
cd test/integration && go run .

# 验证Grafana面板
curl -u admin:admin http://localhost:3000/api/dashboards/uid/batchflow-performance

# 检查数据源连接
curl -u admin:admin http://localhost:3000/api/datasources/proxy/1/api/v1/query?query=up
```

## 🔧 测试工具和辅助函数

### 测试数据库设置

```go
func setupTestDB(t *testing.T, dbType string) *sql.DB {
    var dsn string
    var driver string
    
    switch dbType {
    case "mysql":
        driver = "mysql"
        dsn = os.Getenv("MYSQL_TEST_DSN")
        if dsn == "" {
            dsn = "root:password@tcp(localhost:3306)/batchflow_test"
        }
    case "postgres":
        driver = "postgres"
        dsn = os.Getenv("POSTGRES_TEST_DSN")
        if dsn == "" {
            dsn = "postgres://postgres:password@localhost:5432/batchflow_test?sslmode=disable"
        }
    case "sqlite":
        driver = "sqlite3"
        dsn = ":memory:"
    }
    
    db, err := sql.Open(driver, dsn)
    require.NoError(t, err)
    
    err = db.Ping()
    require.NoError(t, err)
    
    // 创建测试表
    createTestTables(t, db, dbType)
    
    return db
}

func createTestTables(t *testing.T, db *sql.DB, dbType string) {
    var createSQL string
    
    switch dbType {
    case "mysql":
        createSQL = `
        CREATE TABLE IF NOT EXISTS test_users (
            id BIGINT PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            email VARCHAR(255) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`
    case "postgres":
        createSQL = `
        CREATE TABLE IF NOT EXISTS test_users (
            id BIGINT PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            email VARCHAR(255) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`
    case "sqlite":
        createSQL = `
        CREATE TABLE IF NOT EXISTS test_users (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            email TEXT NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )`
    }
    
    _, err := db.Exec(createSQL)
    require.NoError(t, err)
}
```

### 性能断言工具

```go
type PerformanceAssertion struct {
    t         *testing.T
    startTime time.Time
    records   int64
}

func NewPerformanceAssertion(t *testing.T) *PerformanceAssertion {
    return &PerformanceAssertion{
        t:         t,
        startTime: time.Now(),
    }
}

func (pa *PerformanceAssertion) RecordProcessed(count int64) {
    pa.records += count
}

func (pa *PerformanceAssertion) AssertMinRPS(minRPS float64) {
    duration := time.Since(pa.startTime)
    actualRPS := float64(pa.records) / duration.Seconds()
    
    assert.True(pa.t, actualRPS >= minRPS, 
        "RPS too low: expected >= %.0f, got %.0f", minRPS, actualRPS)
}

func (pa *PerformanceAssertion) AssertMaxDuration(maxDuration time.Duration) {
    duration := time.Since(pa.startTime)
    assert.True(pa.t, duration <= maxDuration,
        "Duration too long: expected <= %v, got %v", maxDuration, duration)
}
```

## 🚀 CI/CD 集成

### GitHub Actions 配置

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      
      - name: Run unit tests
        run: |
          go test -v -race -coverprofile=coverage.out ./...
          go tool cover -html=coverage.out -o coverage.html
      
      - name: Upload coverage
        uses: actions/upload-artifact@v3
        with:
          name: coverage-report
          path: coverage.html

  integration-tests:
    runs-on: ubuntu-latest
    services:
      mysql:
        image: mysql:8.0
        env:
          MYSQL_ROOT_PASSWORD: password
          MYSQL_DATABASE: batchflow_test
        ports:
          - 3306:3306
      
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: password
          POSTGRES_DB: batchflow_test
        ports:
          - 5432:5432
      
      redis:
        image: redis:7
        ports:
          - 6379:6379
    
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      
      - name: Run integration tests
        run: |
          cd test/integration
          go run . -db=mysql
          go run . -db=postgresql  
          go run . -db=redis
```

### 测试报告生成

```bash
# 生成测试报告
go test -v -json ./... > test-results.json

# 转换为HTML报告
go-junit-report < test-results.json > test-results.xml

# 生成覆盖率徽章
gocov convert coverage.out | gocov-xml > coverage.xml
```

## 🔧 RPS 指标修复说明

### 问题描述
原来的 `batchflow_records_per_second` 指标使用了错误的 Histogram 类型，导致：
- `batchflow_records_per_second_bucket` 查询结果都是1
- `rate(batchflow_records_per_second_bucket[5m])` 查询结果都是0

### 修复方案
1. **删除有问题的 Histogram**：移除了 `recordsPerSecond` 字段
2. **新增专用 RPS Counter**：`batchflow_records_rate_total` 用于 `rate()` 计算
3. **保留瞬时 RPS Gauge**：`batchflow_current_rps` 显示当前值

### 正确的 RPS 查询方法

```promql
# 当前 RPS（瞬时值）
batchflow_current_rps

# 平均 RPS（过去5分钟，推荐）
rate(batchflow_records_rate_total[5m])

# 峰值 RPS（过去1小时）
max_over_time(batchflow_current_rps[1h])
```

### 响应时间指标修复

#### 问题描述
Grafana 面板中查询的分位数与 Prometheus 配置不匹配：
- 面板查询：`quantile="0.95"`
- 实际配置：只有 `0.5`, `0.9`, `0.99` 分位数

#### 修复方案
更新 Grafana 面板查询，使用实际可用的分位数：

```promql
# 可用的响应时间分位数查询
batchflow_response_time_seconds{quantile="0.99"}  # 99th percentile
batchflow_response_time_seconds{quantile="0.9"}   # 90th percentile  
batchflow_response_time_seconds{quantile="0.5"}   # 50th percentile (median)
```

### 测试验证

```go
// 验证修复后的指标存在
assert.Contains(t, metrics, "batchflow_records_rate_total")
assert.Contains(t, metrics, "batchflow_current_rps")
assert.Contains(t, metrics, "batchflow_tests_run_total")
assert.Contains(t, metrics, "batchflow_records_processed_total")

// 验证响应时间分位数指标
assert.Contains(t, metrics, "batchflow_response_time_seconds")

// 验证测试结果统计指标（修复后直接显示累计值）
assert.Contains(t, metrics, "batchflow_tests_run_total{result=\"success\"}")
assert.Contains(t, metrics, "batchflow_tests_run_total{result=\"failure\"}")

// 验证内存使用指标（已经是 MB 单位，无需转换）
assert.Contains(t, metrics, "batchflow_memory_usage_mb{type=\"alloc\"}")
assert.Contains(t, metrics, "batchflow_memory_usage_mb{type=\"sys\"}")

// 验证正确的查询方法
// RPS 计算：rate(batchflow_records_rate_total[5m])
// 处理记录数：batchflow_records_processed_total{status="processed"}
// 响应时间：batchflow_response_time_seconds{quantile="0.99|0.9|0.5"}
```

## 📚 相关文档

- [API_REFERENCE.md](API_REFERENCE.md) - API详细参考
- [EXAMPLES.md](EXAMPLES.md) - 使用示例
- [MONITORING_GUIDE.md](MONITORING_GUIDE.md) - 监控配置
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - 问题排查

---

💡 **测试最佳实践**：
1. 单元测试覆盖率保持在80%以上
2. 集成测试验证所有数据库驱动
3. 使用正确的指标类型进行监控测试
4. 定期验证 Prometheus 查询的准确性
3. 性能测试建立基准线和回归检测
4. 监控测试确保指标准确性