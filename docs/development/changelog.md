# BatchFlow 变更日志

## v1.0.0-alpha.0 (2025-10-03)

### 🎉 首次发布

这是 BatchFlow 项目的首个 Alpha 版本，提供完整的通用批量处理功能。

### ✨ 核心功能

#### 批量处理引擎
- 基于 `go-pipeline` 的高性能异步批量处理
- 智能 Schema 分组，自动聚合相同表的请求
- 指针传递优化，减少内存复制开销
- 并发安全的请求提交机制

#### 多数据库支持
- **MySQL**: 完整支持，包含所有冲突策略
- **PostgreSQL**: 完整支持，包含所有冲突策略  
- **SQLite**: 基础支持，适合轻量级场景
- **Redis**: 完整支持，优化的 Pipeline 批量执行

#### 冲突处理策略
- `ConflictIgnore`: 跳过冲突记录
- `ConflictReplace`: 覆盖冲突记录
- `ConflictUpdate`: 更新冲突记录

#### 架构设计
- 三层模块化架构: BatchFlow → Executor → Processor → Driver
- 统一的 `BatchExecutor` 接口
- 可选的并发限流支持 (`WithConcurrencyLimit`)
- 完整的指标监控接口 (`MetricsReporter`)

### 🚀 性能特性

#### 高性能设计
- **Submit 性能**: 7.5M ops/sec, 147.2 ns/op, 96 B/op
- **Request 创建**: 61M ops/sec, 18.95 ns/op, 48 B/op
- **SQL 生成**: 69M ops/sec, 17.02 ns/op, 48 B/op

#### 内存优化
- 指针传递减少内存复制
- 零拷贝数据传递
- 智能缓冲区管理
- 全局 Driver 实例共享

### 📊 监控与可观测性

#### MetricsReporter 接口
- 统一的指标上报接口
- 支持自定义监控系统集成
- 默认 NoopMetricsReporter (零开销)
- 完整的性能指标收集

#### Prometheus 集成
- 开箱即用的 Prometheus 指标
- Grafana 仪表板模板
- 实时性能监控
- 数据完整性监控

### 🧪 测试覆盖

#### 单元测试
- 代码覆盖率 75%+
- 完整的功能测试
- 边界条件测试
- 并发安全测试

#### 集成测试
- 多数据库集成测试
- Docker 容器化测试环境
- 自动化 CI/CD 流程
- 性能回归测试

### 📚 文档体系

#### 完整文档
- 详细的 API 参考文档
- 丰富的使用示例
- 架构设计说明
- 最佳实践指南

#### 用户指南
- 快速开始教程
- 配置参数说明
- 监控部署指南
- 故障排除手册

### 🔧 开发工具

#### 代码质量
- golangci-lint 静态检查
- go vet 代码分析
- gofumpt 代码格式化
- 完整的 Makefile 工具链

#### 测试工具
- Docker Compose 测试环境
- 自动化测试脚本
- 性能基准测试
- 集成测试报告

### 🎯 API 设计

#### 简洁易用
```go
// 创建 BatchFlow
batch := batchflow.NewMySQLBatchFlow(ctx, db, config)

// 定义 Schema
schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")

// 提交请求
request := batchflow.NewRequest(schema).SetInt64("id", 1).SetString("name", "John")
batch.Submit(ctx, request)
```

#### 扩展支持
```go
// 自定义驱动
customDriver := &MyCustomSQLDriver{}
batch := batchflow.NewMySQLBatchFlowWithDriver(ctx, db, config, customDriver)

// 自定义执行器
customExecutor := &MyExecutor{}
batch := batchflow.NewBatchFlow(ctx, bufferSize, flushSize, flushInterval, customExecutor)
```

### 🔄 提交语义

#### Context 取消优先
- Submit 在入队前优先检查 `ctx.Err()`
- 取消/超时立即返回，不进入批处理通道
- 确定性的取消行为，避免不确定性

### 🛠️ 框架集成

#### 数据库框架支持
- 原生 `sql.DB` 支持
- GORM 集成示例
- sqlx 集成示例
- 任意连接池框架兼容

#### 第三方扩展
- 自定义 SQLDriver 接口
- 自定义 BatchExecutor 实现
- 插件化架构设计
- 易于扩展新数据库类型

### 📋 已知限制

#### SQLite 限制
- 单写入者架构限制
- 大批次并发写入可能失败
- 适合轻量级单机场景

#### Alpha 版本说明
- API 可能有小幅调整
- 部分高级功能待完善
- 欢迎社区反馈和贡献

### 🔗 相关资源

- **项目仓库**: https://github.com/rushairer/batchflow
- **文档**: [docs/index.md](../index.md)
- **示例**: [docs/guides/examples.md](../guides/examples.md)
- **API 参考**: [docs/api/reference.md](../api/reference.md)

---

## 版本规划

### 下一版本: v1.0.0-beta.0 (计划 4-6 周后)
- API 稳定性优化
- 错误处理增强
- 监控功能完善
- 社区反馈集成

### 正式版本: v1.0.0 (计划 2-3 个月后)
- API 完全稳定
- 生产环境验证
- 企业级文档
- 长期支持承诺

---

**发布日期**: 2025-10-03  
**版本**: v1.0.0-alpha.0  
**状态**: ✅ 已发布