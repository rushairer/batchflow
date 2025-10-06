# BatchFlow 故障排除手册

## 🔍 快速诊断

### 问题分类

| 问题类型 | 常见症状 | 快速检查 |
|---------|---------|----------|
| **连接问题** | 连接超时、拒绝连接 | `telnet host port` |
| **性能问题** | RPS低、延迟高 | 检查批次配置、连接池 |
| **数据问题** | 数据丢失、重复 | 检查冲突模式、事务 |
| **监控问题** | 指标异常、面板空白 | 检查指标端点、查询语句 |

### 诊断命令

```bash
# 快速健康检查
curl -f http://localhost:9090/metrics || echo "指标服务异常"
curl -f http://localhost:3000/api/health || echo "Grafana异常"

# 数据库连接测试
mysql -h localhost -u root -p -e "SELECT 1" 2>/dev/null && echo "MySQL连接正常"
psql -h localhost -U postgres -c "SELECT 1" 2>/dev/null && echo "PostgreSQL连接正常"
redis-cli ping 2>/dev/null && echo "Redis连接正常"

# 进程和端口检查
ps aux | grep -E "(batchflow|prometheus|grafana)"
netstat -tlnp | grep -E "(3000|9090|3306|5432|6379)"
```

## 🚨 常见问题解决

### 1. 连接和配置问题

#### MySQL 连接失败

**症状**：
```
Error: dial tcp 127.0.0.1:3306: connect: connection refused
```

**解决方案**：
```bash
# 检查 MySQL 服务状态
systemctl status mysql
# 或
brew services list | grep mysql

# 检查端口监听
netstat -tlnp | grep 3306

# 测试连接
mysql -h localhost -u root -p -e "SELECT VERSION()"

# 检查用户权限
mysql -u root -p -e "SHOW GRANTS FOR 'your_user'@'localhost'"
```

**配置修复**：
```go
// 正确的 MySQL DSN 格式
dsn := "username:password@tcp(localhost:3306)/database?parseTime=true&timeout=30s"

// 连接池配置
db.SetMaxOpenConns(100)
db.SetMaxIdleConns(50)
db.SetConnMaxLifetime(time.Hour)
```

#### PostgreSQL 连接问题

**症状**：
```
pq: password authentication failed for user "postgres"
```

**解决方案**：
```bash
# 检查 PostgreSQL 服务
systemctl status postgresql
# 或
brew services list | grep postgresql

# 重置密码
sudo -u postgres psql -c "ALTER USER postgres PASSWORD 'newpassword';"

# 检查 pg_hba.conf 配置
sudo cat /etc/postgresql/*/main/pg_hba.conf | grep -v "^#"
```

**配置修复**：
```go
// 正确的 PostgreSQL DSN
dsn := "postgres://username:password@localhost:5432/database?sslmode=disable&connect_timeout=30"

// 处理 SSL 问题
dsn := "postgres://username:password@localhost:5432/database?sslmode=require"
```

#### Redis 连接问题

**症状**：
```
dial tcp 127.0.0.1:6379: connect: connection refused
```

**解决方案**：
```bash
# 检查 Redis 服务
systemctl status redis
# 或
brew services list | grep redis

# 测试连接
redis-cli ping

# 检查配置
redis-cli CONFIG GET "*"
```

**配置修复**：
```go
// Redis 连接配置
rdb := redis.NewClient(&redis.Options{
    Addr:         "localhost:6379",
    Password:     "",
    DB:           0,
    DialTimeout:  30 * time.Second,
    ReadTimeout:  30 * time.Second,
    WriteTimeout: 30 * time.Second,
    PoolSize:     100,
    MinIdleConns: 10,
})

// 连接测试
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
_, err := rdb.Ping(ctx).Result()
if err != nil {
    log.Fatal("Redis connection failed:", err)
}
```

### PostgreSQL: "canceling statement due to user request"

**症状（日志）**：
```
ERROR:  canceling statement due to user request
STATEMENT:  INSERT INTO ...
```

**成因**：
- 客户端在 ExecContext 使用的 ctx 被取消/超时（包括处理器内的 WithTimeout/WithTimeoutCause 到期），驱动向 PG 发送 CancelRequest，PG 侧记录该 ERROR。
- 或命中服务端的 statement_timeout，服务端主动取消语句并记录日志。

**确认方式**：
- 应用侧错误多为 context.DeadlineExceeded 或驱动返回的取消错误文本；
- 若处理器采用 WithTimeoutCause 并将 cause 返回（如 "execute batch timeout"），可据此区分“内部超时”。

**处理建议**：
- 校准超时：避免客户端/服务端双重过短的超时叠加，导致频繁取消与日志噪音。
- 重试策略：默认对超时不重试；若确需对短暂性内部超时重试，请自定义分类器并使用指数退避。
- 降噪：必要时调低 PG 的 log_min_error_statement 或调整 log_statement，减少语句回显。

### errors.Join 判断建议

- 使用 errors.Is 判断合并错误中是否“包含”某类哨兵错误。
- 使用 errors.As 提取具体错误类型实例并访问其字段。
- 如需定位多个匹配项，可遍历 err.(interface{ Unwrap() []error }).Unwrap() 返回的子错误逐个判断。

### 2. 性能问题

#### 低 RPS 问题

**症状**：
- RPS 远低于预期
- 批次执行耗时过长
- CPU 使用率低但性能差

**诊断步骤**：
```bash
# 检查系统资源
top -p $(pgrep batchflow)
iostat -x 1 5
netstat -i

# 检查数据库性能
# MySQL
mysql -e "SHOW PROCESSLIST; SHOW ENGINE INNODB STATUS\G"

# PostgreSQL  
psql -c "SELECT * FROM pg_stat_activity; SELECT * FROM pg_stat_database;"

# Redis
redis-cli --latency-history -i 1
```

**优化方案**：

1. **调整批次配置**：
```go
// 高性能配置
batchFlow := batchflow.NewBatchFlow(
    ctx,
    10000,                   // 大缓冲区
    500,                     // 大批次
    50*time.Millisecond,     // 快速刷新
    executor,
)
```

2. **数据库连接池优化**：
```go
// MySQL/PostgreSQL
db.SetMaxOpenConns(100)    // 增加最大连接数
db.SetMaxIdleConns(50)     // 增加空闲连接数
db.SetConnMaxLifetime(time.Hour)

// Redis
rdb := redis.NewClient(&redis.Options{
    PoolSize:     100,      // 连接池大小
    MinIdleConns: 20,       // 最小空闲连接
})
```

3. **使用事务批处理**：
```go
// MySQL 事务优化
tx, err := db.Begin()
if err != nil {
    return err
}
defer tx.Rollback()

// 执行批量操作...

return tx.Commit()
```

#### 内存使用过高

**症状**：
- 内存使用持续增长
- 出现 OOM 错误
- GC 频繁触发

**诊断工具**：
```bash
# Go 内存分析
go tool pprof http://localhost:6060/debug/pprof/heap
go tool pprof http://localhost:6060/debug/pprof/allocs

# 系统内存监控
free -h
vmstat 1 5
```

**解决方案**：

1. **减少缓冲区大小**：
```go
// 内存优化配置
batchFlow := batchflow.NewBatchFlow(
    ctx,
    1000,                    // 小缓冲区
    100,                     // 小批次
    200*time.Millisecond,    // 较慢刷新
    executor,
)
```

2. **分批处理大数据集**：
```go
const chunkSize = 10000
for offset := 0; offset < totalRecords; offset += chunkSize {
    // 处理当前批次
    processChunk(offset, chunkSize)
    
    // 强制 GC 和休息
    runtime.GC()
    time.Sleep(100 * time.Millisecond)
}
```

3. **启用内存监控**：
```go
// 添加内存监控
go func() {
    for {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)
        
        log.Printf("内存使用: Alloc=%d KB, Sys=%d KB, NumGC=%d",
            m.Alloc/1024, m.Sys/1024, m.NumGC)
        
        time.Sleep(30 * time.Second)
    }
}()
```

### 3. 数据完整性问题

#### 数据丢失

**症状**：
- 提交的记录数与数据库中的记录数不匹配
- 数据完整性率 < 100%

**诊断步骤**：
```sql
-- 检查实际插入的记录数
SELECT COUNT(*) FROM your_table WHERE created_at >= '2025-09-30 00:00:00';

-- 检查是否有重复数据
SELECT id, COUNT(*) FROM your_table GROUP BY id HAVING COUNT(*) > 1;

-- 检查约束违反
SHOW ENGINE INNODB STATUS; -- MySQL
-- 或查看 PostgreSQL 日志
```

**解决方案**：

1. **检查冲突处理模式**：
```go
// 确保使用正确的冲突模式
schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")
// 或
schema := batchflow.NewSQLSchema("users", batchflow.ConflictReplaceOperationConfig, "id", "name", "email")
```

2. **添加重试机制**：
```go
func submitWithRetry(batchFlow *batchflow.BatchFlow, request *batchflow.Request) error {
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        if err := batchFlow.Submit(ctx, request); err != nil {
            if i == maxRetries-1 {
                return fmt.Errorf("最终失败: %w", err)
            }
            time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
            continue
        }
        return nil
    }
    return nil
}
```

3. **启用详细日志**：
```go
// 添加详细的错误日志
type LoggingMetricsReporter struct {
    logger *log.Logger
}

func (r *LoggingMetricsReporter) ObserveExecuteDuration(tableName string, batchSize int, d time.Duration, status string) {
    if status != "success" {
        r.logger.Printf("批次执行失败: table=%s, size=%d, duration=%dms, status=%s",
            tableName, batchSize, duration, status)
    }
}
```

#### 数据重复

**症状**：
- 相同的记录被插入多次
- 唯一约束违反错误

**解决方案**：

1. **使用正确的冲突处理**：
```go
// 对于可能重复的数据，使用 IGNORE 模式
schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")

// 或使用 REPLACE 模式更新重复数据
schema := batchflow.NewSQLSchema("users", batchflow.ConflictReplaceOperationConfig, "id", "name", "email")
```

2. **添加唯一性检查**：
```sql
-- 在数据库层面添加唯一约束
ALTER TABLE users ADD UNIQUE KEY unique_email (email);
ALTER TABLE users ADD UNIQUE KEY unique_id (id);
```

3. **应用层去重**：
```go
type DeduplicatedBatchFlow struct {
    batchFlow *batchflow.BatchFlow
    seen     map[string]bool
    mu       sync.Mutex
}

func (d *DeduplicatedBatchFlow) Submit(ctx context.Context, request *batchflow.Request) error {
    d.mu.Lock()
    defer d.mu.Unlock()
    
    // 生成记录的唯一标识
    key := generateRecordKey(request)
    if d.seen[key] {
        return nil // 跳过重复记录
    }
    
    d.seen[key] = true
    return d.batchFlow.Submit(ctx, request)
}
```

### 4. 监控问题

#### Grafana 面板显示异常

**症状**：
- 数据完整性显示 10000%
- 面板显示 "No data"
- 指标值异常

**解决步骤**：

1. **检查 Prometheus 指标**：
```bash
# 检查指标是否存在
curl -s http://localhost:9090/api/v1/label/__name__/values | grep batchflow

# 检查具体指标值
curl -s "http://localhost:9090/api/v1/query?query=batchflow_data_integrity_rate"

# 检查指标范围
curl -s "http://localhost:9090/api/v1/query?query=batchflow_data_integrity_rate" | jq '.data.result[].value[1]'
```

2. **修正 Grafana 查询**：
```json
// 错误的查询（导致 10000%）
{
  "expr": "batchflow_data_integrity_rate * 10000"
}

// 正确的查询
{
  "expr": "batchflow_data_integrity_rate * 100"
}
```

3. **验证指标计算逻辑**：
```go
// 确保指标范围为 0-1
integrityRate := float64(actualRecords) / float64(submittedRecords)
pm.dataIntegrityRate.WithLabelValues(database, testName).Set(integrityRate)

// 而不是百分比值
// pm.dataIntegrityRate.WithLabelValues(database, testName).Set(integrityRate * 100) // 错误
```

#### Prometheus 指标缺失

**症状**：
- `/metrics` 端点返回空或错误
- Prometheus 无法抓取指标

**解决方案**：

1. **检查指标服务器**：
```go
// 确保正确启动指标服务器
prometheusMetrics := NewPrometheusMetrics()
go func() {
    if err := prometheusMetrics.StartServer(9090); err != nil {
        log.Printf("指标服务器启动失败: %v", err)
    }
}()
```

2. **检查防火墙和网络**：
```bash
# 检查端口监听
netstat -tlnp | grep 9090

# 测试本地访问
curl -f http://localhost:9090/metrics

# 检查防火墙
sudo ufw status
sudo iptables -L
```

3. **验证指标注册**：
```go
// 确保指标正确注册
func (pm *PrometheusMetrics) RegisterMetrics() {
    prometheus.MustRegister(
        pm.recordsProcessed,
        pm.currentRPS,
        pm.dataIntegrityRate,
        pm.batchExecutionDuration,
    )
}
```

## 🛠️ 调试工具

### 1. 日志配置

```go
// 启用详细日志
import "log/slog"

logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

// 在关键位置添加日志
func (bs *BatchFlow) Submit(ctx context.Context, request *Request) error {
    logger.Debug("提交请求", 
        "table", request.schema.Name,
        "fields", len(request.data))
    
    // ... 处理逻辑
    
    logger.Debug("请求处理完成",
        "table", request.schema.Name,
        "success", err == nil)
    
    return err
}
```

### 2. 性能分析

```go
// 启用 pprof
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()

// 使用方法：
// go tool pprof http://localhost:6060/debug/pprof/profile
// go tool pprof http://localhost:6060/debug/pprof/heap
```

### 3. 指标验证工具

```go
// 指标验证器
type MetricsValidator struct {
    expectedRecords int64
    actualRecords   int64
}

func (mv *MetricsValidator) Validate() error {
    if mv.actualRecords != mv.expectedRecords {
        return fmt.Errorf("记录数不匹配: 期望=%d, 实际=%d", 
            mv.expectedRecords, mv.actualRecords)
    }
    return nil
}

// 使用示例
validator := &MetricsValidator{expectedRecords: 10000}
// ... 执行批量操作
validator.actualRecords = getActualRecordCount()
if err := validator.Validate(); err != nil {
    log.Printf("验证失败: %v", err)
}
```

## 📋 故障排查清单

### 启动前检查

- [ ] 数据库服务正常运行
- [ ] 网络连接正常
- [ ] 配置文件正确
- [ ] 权限设置正确
- [ ] 端口未被占用

### 运行时监控

- [ ] CPU 使用率正常 (< 80%)
- [ ] 内存使用稳定
- [ ] 网络延迟正常 (< 10ms)
- [ ] 数据库连接池健康
- [ ] 错误率低 (< 1%)

### 数据验证

- [ ] 记录数匹配
- [ ] 数据完整性 = 100%
- [ ] 无重复数据
- [ ] 约束满足
- [ ] 事务一致性

### 监控验证

- [ ] 指标端点可访问
- [ ] Prometheus 正常抓取
- [ ] Grafana 面板显示正常
- [ ] 告警规则生效
- [ ] 日志记录完整

## 📞 获取帮助

### 社区支持

- **GitHub Issues**: [项目地址]/issues
- **文档**: [DOCUMENTATION_INDEX.md](DOCUMENTATION_INDEX.md)
- **示例**: [EXAMPLES.md](EXAMPLES.md)

### 报告问题

提交问题时请包含：

1. **环境信息**：
   - 操作系统和版本
   - Go 版本
   - 数据库版本
   - BatchFlow 版本

2. **问题描述**：
   - 具体症状
   - 错误信息
   - 重现步骤

3. **配置信息**：
   - 数据库连接配置
   - BatchFlow 参数配置
   - 监控配置

4. **日志和指标**：
   - 应用日志
   - 数据库日志
   - Prometheus 指标快照

---

💡 **故障排查建议**：
1. 从简单问题开始排查（连接、配置）
2. 使用分层诊断方法（网络→数据库→应用→监控）
3. 保留详细的日志和指标数据
4. 建立故障排查的标准流程