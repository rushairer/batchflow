# BatchFlow 发布评估

## 当前结论

可以发新版；本轮按 `v1.1.1` 做发布收尾。

当前最新 tag：

- `v1.1.0`

推荐下一个版本：

- `v1.1.1`

## 为什么是 `v1.1.1`

这轮工作的重点是补齐示例、测试和文档一致性，同时顺手扩展 `Request` 的常用 typed setter，整体按一次低风险 API 可用性补强处理。

### 本轮内容

- 生命周期：
  - 新增可编译示例 `ExampleBatchFlow_Close`
  - 新增可编译示例 `ExampleBatchFlow_Done`
  - 测试覆盖 `Close()` 后 `Done()` 关闭、`Wait()` 返回值、重复 `Close()` 幂等
- Request：
  - 新增 `SetInt`、`SetInt8`、`SetInt16`
  - 新增 `SetUint`、`SetUint8`、`SetUint16`、`SetUint32`、`SetUint64`
- 文档：
  - README、API 参考、示例指南同步到新 setter 和生命周期样例
  - `docs-check` 增加 `Done()` / `SetInt` / `SetUint64` 契约校验

## 发布前检查

### 已完成

- [x] `go test ./...`
- [x] `go test -run '^$' ./...`
- [x] `make docs-check`
- [x] `make lint`
- [x] `make release-check`
- [x] 已在打 tag 前再次执行 `make release-check`
- [x] changelog 已记录本轮 setter / lifecycle / docs 补强变更
- [x] release notes 草案已生成：`docs/development/release-notes-v1.1.1.md`

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
4. 创建 tag：`v1.1.1`
5. 生成 release notes。

## 建议的 release notes 标题

`v1.1.1: 生命周期示例补强、Request typed setter 扩展、文档一致性校对`
