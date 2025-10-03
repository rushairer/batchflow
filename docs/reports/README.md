# SQLite 测试工具集

本目录包含了专门用于 SQLite 数据库测试和分析的工具集合。

## 目录结构

```
test/sqlite/
├── README.md                    # 本文件
├── tools/                       # SQLite 测试工具
│   ├── benchmark/              # 性能基准测试
│   │   └── main.go
│   ├── clear-test/             # 清理方式测试
│   │   └── main.go
│   ├── config-analysis/        # 配置分析工具
│   │   └── main.go
│   └── path-compatibility/     # 路径兼容性测试
│       └── main.go
└── data/                       # 测试数据目录（自动创建）
```

## 工具说明

### 1. 配置分析工具 (config-analysis)

分析和对比不同的 SQLite 配置方案，提供性能优化建议。

```bash
cd test/sqlite/tools/config-analysis
go run main.go
```

**功能**：
- 对比原始配置与优化配置
- 显示基准测试验证结果
- 提供最终配置建议

### 2. 性能基准测试 (benchmark)

全面的 SQLite 性能基准测试，测试不同并发和批次配置下的性能表现。

```bash
cd test/sqlite/tools/benchmark
go run main.go
```

**功能**：
- 多种测试场景（单线程、低并发、中等并发）
- 实时性能监控（RPS、耗时、数据一致性）
- 自动生成性能报告和建议

### 3. 清理方式测试 (clear-test)

测试和对比不同的 SQLite 表清理方式的性能。

```bash
cd test/sqlite/tools/clear-test
go run main.go
```

**功能**：
- 对比 DELETE+VACUUM 与重建表两种清理方式
- 性能测试和时间对比
- 验证清理效果

### 4. 路径兼容性测试 (path-compatibility)

测试在不同环境（本地、Docker）下的路径兼容性。

```bash
cd test/sqlite/tools/path-compatibility
go run main.go
```

**功能**：
- 自动检测运行环境
- 测试目录创建和文件写入权限
- 提供环境适配建议

## 数据文件

所有工具生成的测试数据库文件都会保存在 `test/sqlite/data/` 目录下：

- `benchmark.db` - 基准测试数据库
- `benchmark_test.db` - 清理测试数据库
- `clean_test.db` - 其他测试数据库

## 使用建议

1. **首次使用**：先运行配置分析工具了解推荐配置
2. **性能测试**：使用基准测试工具验证实际性能
3. **问题排查**：使用清理测试和路径兼容性工具排查具体问题

## 注意事项

- 所有工具都使用独立的 `main` 包，避免包名冲突
- 数据库文件路径已调整为相对于工具目录的路径
- 建议在项目根目录或工具目录下运行命令
- 测试数据会自动清理，不会影响生产数据

## 相关文档和配置

### 文档
- **[SQLITE_OPTIMIZATION.md](SQLITE_OPTIMIZATION.md)** - SQLite 性能优化总结和解决方案
- **[PERFORMANCE_ANALYSIS.md](PERFORMANCE_ANALYSIS.md)** - SQLite 性能分析报告和问题诊断  
- **[TEST_REPORT_ANALYSIS.md](TEST_REPORT_ANALYSIS.md)** - 测试报告详细分析（包含测试参数）
- **[tools/README.md](tools/README.md)** - 测试工具集详细说明

### 配置文件
- **[../../.env.sqlite.test](../../.env.sqlite.test)** - SQLite 专用测试配置  
- **[../../docker-compose.sqlite.yml](../../docker-compose.sqlite.yml)** - SQLite Docker 配置
- **[../../CONFIG.md](../../CONFIG.md)** - 统一配置说明文档

### 项目文档
- **[../../README.md](../../README.md)** - 项目主文档
- **[../../QUALITY_ASSESSMENT.md](../../QUALITY_ASSESSMENT.md)** - 项目质量评估报告
- **[../../README-INTEGRATION-TESTS.md](../../README-INTEGRATION-TESTS.md)** - 集成测试文档

## 配置选择建议

- **问题排查**: 参考 [TEST_REPORT_ANALYSIS.md](TEST_REPORT_ANALYSIS.md) 查看详细测试分析
- **性能优化**: 参考 [PERFORMANCE_ANALYSIS.md](PERFORMANCE_ANALYSIS.md) 了解优化方案
- **历史问题**: 参考 [SQLITE_OPTIMIZATION.md](SQLITE_OPTIMIZATION.md) 了解解决过程
- **工具使用**: 参考 [tools/README.md](tools/README.md) 了解各工具详细用法