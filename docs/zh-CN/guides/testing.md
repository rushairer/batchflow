# 测试指南

本文档是 [Testing Guide](../../guides/testing.md) 的中文镜像摘要。

## 本地检查

```bash
go test ./...
go test -run '^$' ./...
make docs-check
```

## 推荐分层

### 契约测试

- `Submit` 取消语义。
- `Close()` 最终 flush。
- `Wait()` / `Done()` 生命周期。
- `Schema` 和 `Request.Validate()`。
- `SetInt`、`SetUint64`、`SetBool`、`SetNull` 等 typed setter。
- 错误分类和低基数 reason label。

### 执行器测试

- SQL driver 生成逻辑。
- PostgreSQL/MySQL update/replace 冲突列行为。
- Redis command 生成。
- `WithConcurrencyLimit(...)`。
- `WithRetryConfig(...)`。
- Metrics 回调阶段。

### 集成与压力测试

```bash
make docker-postgres-test
make docker-mysql-test
make docker-redis-test
```

生成汇总报告：

```bash
./scripts/run_stress_report.sh
```

默认压力测试范围是 PostgreSQL、MySQL、Redis。SQLite 高并发写入不是 gating 后端，需要趋势数据时使用 `--include-sqlite`。

## 新增功能最低要求

- 改公开 API：补单元测试和文档。
- 改 SQL 生成：补 driver 测试和目标数据库 Docker 测试。
- 改 metrics 语义：更新 `metrics-spec.md` 和 reporter 测试。
- 改 lifecycle：补 `Close`、`Wait` 或 `Done` 测试。
- 改示例：跑 `go test -run '^$' ./...`。
