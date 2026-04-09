# BatchFlow 测试指南

## 本地快速验证

```bash
go test ./...
go test -run '^$' ./...   # 只做全仓编译检查
make docs-check
```

## 推荐测试分层

### 1. 契约测试

重点覆盖：

- `Submit` 的取消语义
- `Close()` 最终 flush 语义
- `Wait()` / `Done()` 生命周期语义
- 生命周期示例的可编译性（例如 `ExampleBatchFlow_Close`、`ExampleBatchFlow_Done`）
- `Schema` 校验和 `Request.Validate()`
- `NewRequest` typed setter 的列值保真
- 重试分类与错误标签

### 2. 执行器测试

重点覆盖：

- SQL Driver 生成逻辑
- Redis command 生成逻辑
- `WithConcurrencyLimit(...)`
- `WithRetryConfig(...)`
- Metrics 回调是否在正确阶段触发

### 3. 集成测试

重点覆盖：

- MySQL / PostgreSQL / SQLite / Redis 端到端写入
- 高吞吐压测
- Grafana / Prometheus 面板验证

## 新增功能时最低要求

- 改公开 API：补单元测试和文档。
- 改 metrics 语义：补 `metrics-spec.md` 和至少一个 reporter 测试。
- 改 lifecycle：补 `Close/Wait/Done` 相关测试。
- 改示例：跑 `go test -run '^$' ./...`，确保仓库可编译。

## 为什么强调 docs-check

这个仓库之前的主要问题之一是“实现变了，文档和示例没跟上”。所以现在除了 `go test`，还建议把 `make docs-check` 当成常规验证的一部分。

## 集成测试入口

- [集成测试文档](integration-tests.md)
- `test/integration`
