# BatchFlow v1.1.1 Release Notes

发布日期：2026-04-09

## 亮点

- 补齐 `Close()` / `Wait()` / `Done()` 的可编译示例与行为测试
- `NewRequest` 新增更多基础整数 typed setter，减少调用侧手动类型转换
- README、API 参考、示例指南与 `docs-check` 同步到最新公开 API

## 新增

### 生命周期示例

- `ExampleBatchFlow_Close`
- `ExampleBatchFlow_Done`

### Request typed setter

- `SetInt`
- `SetInt8`
- `SetInt16`
- `SetUint`
- `SetUint8`
- `SetUint16`
- `SetUint32`
- `SetUint64`

## 变更

### 测试

- 新增 `Wait()` / `Done()` 契约测试，覆盖：
  - `Close()` 后退出信号关闭
  - 重复 `Close()` 幂等
  - 父上下文取消时 `Wait()` 返回值
- 新增 typed setter 列值保真测试，确认基础整数类型不会被隐式改写

### 文档

- README 增加生命周期常见用法与 request setter 说明
- `docs/api/reference.md` 同步公开 setter 列表与 `Wait()` / `Done()` 模式
- `docs/guides/examples.md` 增加 `Wait / Done` 与 typed setter 示例
- `scripts/check-doc-consistency.sh` 新增 `Done()`、`SetInt`、`SetUint64` 契约检查

## 兼容性

这是一个向后兼容的 patch 版本更新。

- 现有构造函数保持不变
- 现有执行器与 metrics 接口保持不变
- 现有调用方式继续可用；只是补充更多 setter 与示例

## 验证

- `go test ./...` 通过
- `go test -run '^$' ./...` 通过
- `make docs-check` 通过
- `make release-check` 通过
