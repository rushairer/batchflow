# BatchFlow 集成测试配置说明

## 🎯 统一配置原则

**统一配置源**: `.env.test` 文件 + 各数据库的 docker-compose 文件

- **`.env.test`**: 统一的测试参数配置，所有数据库共享
- **`docker-compose.{database}.yml`**: 仅包含数据库特定的连接参数

## 🏗️ 架构配置说明

### BatchExecutor 实现方式
- **SQL数据库** (MySQL/PostgreSQL/SQLite): 使用 `ThrottledBatchExecutor` (+ 可选并发限流 `WithConcurrencyLimit`) + `BatchProcessor` + `SQLDriver`
- **NoSQL数据库** (Redis): 直接实现 `BatchExecutor` 接口
- **测试环境**: 使用 `MockExecutor` 直接实现 `BatchExecutor` 接口

## 📋 配置文件结构

```
batchflow/
├── .env.test                    # 🎯 统一测试参数 (所有数据库共享)
├── docker-compose.mysql.yml     # MySQL连接配置
├── docker-compose.postgres.yml  # PostgreSQL连接配置
└── docker-compose.sqlite.yml    # SQLite连接配置
```

## ⚙️ 统一测试参数 (`.env.test`)

```bash
# 测试时长配置
TEST_DURATION=1800s              # 30分钟压力测试

# 并发配置
CONCURRENT_WORKERS=10            # 10个并发工作者
RECORDS_PER_WORKER=2000          # 每个工作者2000条记录

# 批处理配置
BATCH_SIZE=200                   # 批次大小200条
BUFFER_SIZE=5000                 # 缓冲区5000条
FLUSH_INTERVAL=100ms             # 刷新间隔100毫秒
```

## 🔗 数据库连接配置

### MySQL (`docker-compose.mysql.yml`)
```yaml
env_file:
  - .env.test                    # 引用统一配置
environment:
  - MYSQL_DSN=testuser:testpass123@tcp(mysql:3306)/batchflow_test?parseTime=true&multiStatements=true
```

### PostgreSQL (`docker-compose.postgres.yml`)
```yaml
env_file:
  - .env.test                    # 引用统一配置
environment:
  - POSTGRES_DSN=postgres://testuser:testpass123@postgres:5432/batchflow_test?sslmode=disable
```

### SQLite (`docker-compose.sqlite.yml`)
```yaml
env_file:
  - .env.test                    # 引用统一配置
environment:
  - SQLITE_DSN=/app/data/test.db
```

## 🔧 修改配置

### 修改测试参数 (影响所有数据库)
编辑 `.env.test` 文件：

```bash
# 示例：修改为1小时测试，20个并发
TEST_DURATION=3600s
CONCURRENT_WORKERS=20
RECORDS_PER_WORKER=5000
BATCH_SIZE=500
BUFFER_SIZE=10000
FLUSH_INTERVAL=50ms
```

### 修改数据库连接 (仅影响特定数据库)
编辑对应的 docker-compose 文件中的 `MYSQL_DSN`、`POSTGRES_DSN` 或 `SQLITE_DSN`。

## ✅ 统一配置的优势

1. **DRY原则**: 测试参数只定义一次，避免重复
2. **一致性保证**: 所有数据库自动使用相同的测试参数
3. **易于维护**: 修改测试配置只需编辑 `.env.test` 文件
4. **公平对比**: 确保所有数据库在完全相同的条件下测试
5. **避免配置漂移**: 不会出现某个数据库配置不同步的问题

## 📊 配置参数说明

### 性能相关参数
- `CONCURRENT_WORKERS`: 并发工作线程数，影响CPU和内存使用
- `RECORDS_PER_WORKER`: 每个工作者处理的记录数，影响总测试数据量
- `BATCH_SIZE`: 批量插入的记录数，影响内存使用和数据库性能
- `BUFFER_SIZE`: 内存缓冲区大小，影响内存使用
- `FLUSH_INTERVAL`: 数据刷新间隔，影响实时性和性能

### 时间相关参数
- `TEST_DURATION`: 测试持续时间，支持格式：`30s`, `5m`, `1h`, `1800s`

## 🚀 测试命令

```bash
# 所有数据库现在使用相同的配置参数
make docker-mysql-test      # MySQL 30分钟压力测试
make docker-postgres-test   # PostgreSQL 30分钟压力测试  
make docker-sqlite-test     # SQLite 30分钟压力测试
make docker-all-tests       # 运行所有数据库测试
```

## 📈 性能调优建议

### 高性能配置（需要充足内存）
```bash
TEST_DURATION=3600s          # 1小时
CONCURRENT_WORKERS=20        # 20个并发
RECORDS_PER_WORKER=5000      # 每个5000条
BATCH_SIZE=500              # 大批次
BUFFER_SIZE=10000           # 大缓冲区
```

### 当前稳定配置
```bash
TEST_DURATION=1800s          # 30分钟
CONCURRENT_WORKERS=10        # 10个并发
RECORDS_PER_WORKER=2000      # 每个2000条
BATCH_SIZE=200              # 中等批次
BUFFER_SIZE=5000            # 中等缓冲区
```

### 保守配置（内存受限环境）
```bash
TEST_DURATION=600s           # 10分钟
CONCURRENT_WORKERS=5         # 5个并发
RECORDS_PER_WORKER=1000      # 每个1000条
BATCH_SIZE=100              # 小批次
BUFFER_SIZE=2000            # 小缓冲区
```

## ⚠️ 重要提醒

1. **修改 `.env.test` 后需要重新启动测试容器**
2. **所有数据库会自动使用相同的测试参数**
3. **如果测试失败，首先检查内存使用情况**
4. **建议的配置范围**：
   - CONCURRENT_WORKERS: 5-20
   - RECORDS_PER_WORKER: 1000-5000
   - BATCH_SIZE: 100-500
   - 总记录数建议不超过100,000条