# BatchFlow 发布评估

## 当前结论

可以发新版，但建议先按 `v1.1.0` 作为下一个版本号完成一次标准发布收尾，再打 tag。

当前最新 tag：

- `v1.0.3`

推荐下一个版本：

- `v1.1.0`

## 为什么是 `v1.1.0`

按照 semver 的判断，这次变更更适合次版本，不适合补丁版本，也还不到主版本：

### 不是 `v1.0.4`

因为这次不是单纯修 bug，还新增了公开能力：

- `BatchFlow.Close()`
- `BatchFlow.Wait()`
- `BatchFlow.Done()`
- `BatchFlowMetricsReporter`
- 新的官方 metrics 契约和官方示例
- 新的 `docs-check` 文档一致性校验

这属于向后兼容的新功能，应该提升 minor 版本。

### 不是 `v2.0.0`

因为当前没有发现必须按“公开 Go API 破坏性变更”处理的内容：

- 现有构造函数还在
- 现有执行器接口还在
- 现有核心 metrics 接口还在
- 现有模块路径没有变

虽然 metrics 语义比以前更严格了，`batch_size` 的统计口径也修正了，但这更像“修正错误语义 + 明确契约”，还不足以单独推到 major。

## 发布内容摘要

### API / 生命周期

- `BatchFlow` 现在有完整生命周期：
  - `Close()`
  - `Wait()`
  - `Done()`
- `Close()` 会停止接收新请求、触发最终 flush 并等待后台退出。

### Metrics

- 新增 `BatchFlowMetricsReporter`。
- `ObserveDequeueLatency` 从“文档预留”变成真实采样。
- 新增指标语义：
  - `submit_rejected_total`
  - `pipeline_flush_size`
  - `schema_groups_per_flush`
- 修正 `batch_size` 语义，只表示单个 schema 执行批大小。

### 文档

- README、API 参考、配置说明、监控文档、示例文档全部切到当前实现口径。
- 新增 `docs/guides/metrics-spec.md` 作为指标语义 source of truth。

### 工程防线

- 新增 `make docs-check`
- 新增 `scripts/check-doc-consistency.sh`

## 发布前检查

### 已完成

- [x] `go test ./...`
- [x] `go test -run '^$' ./...`
- [x] `make docs-check`
- [x] `make lint`
- [x] `make release-check`
- [x] 已在打 tag 前再次执行 `make release-check`
- [x] changelog 已记录本轮 API / metrics / docs 收敛变更
- [x] release notes 草案已生成：`docs/development/release-notes-v1.1.0.md`

### 建议在打 tag 前再做一次

- [ ] 如需对外展示监控面板，对官方 dashboard 再做一次人工 smoke check
- [x] 审查 `docs/development/changelog.md` 中 Unreleased 是否只保留本次准备发布的内容

## 建议的发布动作

1. 确认工作区只包含本次要发布的变更。
2. 运行：
   - `make docs-check`
   - `go test ./...`
   - `make lint`
3. 将 changelog 中本次内容从 `Unreleased` 收口到正式版本段。
4. 创建 tag：`v1.1.0`
5. 生成 release notes。

## 建议的 release notes 标题

`v1.1.0: 生命周期补齐、metrics 契约收敛、文档一致性重构`
