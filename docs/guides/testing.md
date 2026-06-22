# BatchFlow 测试指南

## 本地快速验证

```bash
go test ./...
go test -run '^$' ./...   # 只做全仓编译检查
make docs-check
```

## CI 分层

### PR / main 快速 CI

`.github/workflows/ci.yml` 只运行必须稳定、可快速反馈的检查：

- `gofmt -s -l .`
- `git diff --check`
- `go vet ./...`
- `golangci-lint run`
- `scripts/check-doc-consistency.sh`
- `go test -run '^$' ./...`
- `go test ./...`
- `make cover`
- `go test -race .`

这个层级用于阻止格式、静态检查、文档一致性、编译、单元测试、覆盖率生成和核心并发语义回归。覆盖率报告作为趋势证据上传，不在 PR 阶段设置过高硬门槛。

### Nightly / manual 长耗时测试

`.github/workflows/nightly.yml` 承担 Docker 数据库、长时间压测和性能趋势类验证。MySQL、PostgreSQL、Redis 的 nightly 结果是 gating；失败应触发调查。SQLite stress 是 non-gating，因为高并发写入失败属于已文档化的 SQLite 架构限制，但报告仍会上传用于趋势分析。

### Release 验证

发布前应在快速 CI 通过后，再运行数据库集成测试、性能基准、安全扫描和文档发布检查。SQL 写入语义、观测事件、错误分类、重试策略这类跨模块改动必须至少覆盖单元测试和对应数据库的 Docker 真机测试。

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
- 改依赖、安全边界或日志字段：跑 `govulncheck ./...`，并确认不会暴露敏感数据。

## 为什么强调 docs-check

这个仓库之前的主要问题之一是“实现变了，文档和示例没跟上”。所以现在除了 `go test`，还建议把 `make docs-check` 当成常规验证的一部分。

## 集成测试入口

- [集成测试文档](integration-tests.md)
- `test/integration`
