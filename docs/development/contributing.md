# Contributing to BatchFlow

Thank you for your interest in contributing to BatchFlow! This document provides guidelines and information for contributors.

## 🚀 Getting Started

### Prerequisites
- Go 1.20 or later
- Docker and Docker Compose (for integration tests)
- Git

### Development Setup
1. **Fork and Clone**
   ```bash
   git clone https://github.com/rushairer/batchflow.git
   cd batchflow
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Verify Setup**
   ```bash
   make test-unit
   make lint
   ```

## 📋 Development Workflow

### 1. Create a Branch
```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/issue-number
```

### 2. Make Changes
- Write clean, well-documented code
- Follow Go best practices and project conventions
- Add tests for new functionality
- Update documentation as needed

### 3. Test Your Changes
```bash
# Run unit tests
make test-unit

# Run linting
make lint

# Run integration tests (optional but recommended)
make docker-sqlite-test
make docker-mysql-test
make docker-postgres-test
make docker-redis-test
```

### 4. Commit Changes
```bash
git add .
git commit -m "feat: add new feature description"
# or
git commit -m "fix: resolve issue description"
```

**Commit Message Format:**
- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation changes
- `test:` - Test additions or modifications
- `refactor:` - Code refactoring
- `perf:` - Performance improvements
- `chore:` - Maintenance tasks

### 5. Push and Create PR
```bash
git push origin your-branch-name
```
Then create a Pull Request on GitHub.

## 🧪 Testing Guidelines

### Unit Tests
- Write tests for all new functions and methods
- Aim for at least 80% code coverage
- Use table-driven tests where appropriate
- Mock external dependencies

**Example:**
```go
func TestBatchFlow_Submit(t *testing.T) {
    tests := []struct {
        name    string
        request *Request
        wantErr bool
    }{
        {
            name:    "valid request",
            request: NewRequest(schema).SetString("name", "test"),
            wantErr: false,
        },
        // Add more test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Tests
- Test real database interactions
- Verify performance characteristics
- Test error handling and edge cases
- Use Docker containers for consistent environments

### Performance Tests
- Add benchmarks for performance-critical code
- Monitor memory allocations
- Test with realistic data volumes

**Example:**
```go
func BenchmarkBatchFlow_Submit(b *testing.B) {
    batch, _ := NewBatchFlowWithMock(ctx, config)
    request := NewRequest(schema).SetString("name", "test")
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        batch.Submit(ctx, request)
    }
}
```

## 📝 Code Style Guidelines

### Go Code Style
- Follow standard Go formatting (`go fmt`)
- Use meaningful variable and function names
- Write clear, concise comments
- Keep functions small and focused
- Handle errors appropriately

### Documentation
- Add GoDoc comments for public functions and types
- Update README.md for significant changes
- Include code examples in documentation
- Document configuration options and their effects

### Error Handling
```go
// Good: Specific error types
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation error in field %s: %s", e.Field, e.Message)
}

// Good: Contextual error wrapping
if err := validateRequest(req); err != nil {
    return fmt.Errorf("failed to validate request: %w", err)
}
```

## 🏗️ Architecture Guidelines

*基于重构后的架构设计 - 版本 v1.3.0*

### 架构概览
BatchFlow 采用灵活的分层架构，通过统一的 `BatchExecutor` 接口支持不同类型的数据源：

- **SQL数据库**: 使用 `ThrottledBatchExecutor` + `BatchProcessor` + `SQLDriver`
- **NoSQL数据库**: 直接实现 `BatchExecutor` 接口
- **消息推送/API调用**: 直接实现 `BatchExecutor` 接口，支持各种自定义批量任务
- **测试环境**: 使用 `MockExecutor` 直接实现

### 添加新的SQL数据源支持

1. **实现SQLDriver接口**:
   ```go
   // drivers/newdb/driver.go
   type NewDBDriver struct{}
   
   func (d *NewDBDriver) GenerateInsertSQL(schema batchflow.SchemaInterface, data []map[string]any) (string, []any, error) {
       // 生成数据库特定的SQL语句
       // 处理冲突策略：ConflictIgnore, ConflictReplace, ConflictUpdate
       return sql, args, nil
   }
   ```

2. **创建执行器工厂**:
   ```go
   // drivers/newdb/executor.go
   func NewBatchExecutor(db *sql.DB) *batchflow.ThrottledBatchExecutor {
       return batchflow.NewSQLThrottledBatchExecutorWithDriver(db, &NewDBDriver{})
   }
   
   func NewBatchExecutorWithDriver(db *sql.DB, driver batchflow.SQLDriver) *batchflow.ThrottledBatchExecutor {
       return batchflow.NewSQLThrottledBatchExecutorWithDriver(db, driver)
   }
   ```

3. **添加BatchFlow工厂方法**:
   ```go
   // batchflow.go
   func NewNewDBBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow {
       executor := newdb.NewBatchExecutor(db)
       return NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, executor)
   }
   ```

### 添加新的NoSQL数据源支持

1. **直接实现BatchExecutor接口**:
   ```go
   // drivers/newnosql/executor.go
   type Executor struct {
       client          *NewNoSQLClient
   }
   
   func (e *Executor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
       // 直接实现数据库特定的批量操作
       // 无需经过BatchProcessor层
       return nil
   }
   
   
   ```

2. **创建工厂方法**:
   ```go
   func NewBatchExecutor(client *NewNoSQLClient) *Executor {
       return &Executor{client: client}
   }
   ```

3. **添加BatchFlow工厂方法**:
   ```go
   func NewNewNoSQLBatchFlow(ctx context.Context, client *NewNoSQLClient, config PipelineConfig) *BatchFlow {
       executor := newnosql.NewBatchExecutor(client)
       return NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, executor)
   }
   ```

### 测试新的数据源驱动

1. **单元测试**:
   ```go
   func TestNewDBDriver_GenerateInsertSQL(t *testing.T) {
       driver := &NewDBDriver{}
       schema := &batchflow.Schema{
           Name: "test_table",
           Columns:   []string{"id", "name"},
           ConflictStrategy: batchflow.ConflictIgnore,
       }
       data := []map[string]any{
           {"id": 1, "name": "test"},
       }
       
       sql, args, err := driver.GenerateInsertSQL(schema, data)
       assert.NoError(t, err)
       assert.Contains(t, sql, "INSERT")
       assert.Len(t, args, 2)
   }
   ```

2. **集成测试**:
   ```go
   func TestNewDBBatchFlow_Integration(t *testing.T) {
       db := setupTestDB(t) // 设置测试数据库
       defer db.Close()
       
       config := PipelineConfig{
           BufferSize:    100,
           FlushSize:     10,
           FlushInterval: time.Second,
       }
       batch := NewNewDBBatchFlow(ctx, db, config)
       
       // 测试批量插入
       schema := NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")
       request := NewRequest(schema).SetInt64("id", 1).SetString("name", "test")
       
       err := batch.Submit(ctx, request)
       assert.NoError(t, err)
       
       // 验证数据插入
       // ...
   }
   ```

### 架构最佳实践

1. **选择合适的实现方式**:
   - SQL数据库：使用 ThrottledBatchExecutor 架构，复用通用逻辑
   - NoSQL数据库：直接实现BatchExecutor，避免不必要的抽象

2. **性能优化**:
   - 使用数据库特定的批量操作API
   - 避免在热路径中进行内存分配
   - 利用数据库的Pipeline或Batch特性

3. **错误处理**:
   - 提供清晰的错误信息
   - 区分临时错误和永久错误
   - 支持错误重试机制

4. **指标收集**:
   - 实现MetricsReporter接口
   - 记录执行时间、批次大小、成功/失败状态
   - 提供数据库特定的指标

### Performance Considerations
- Use pointer receivers for methods
- Minimize memory allocations in hot paths
- Consider using sync.Pool for frequently allocated objects
- Profile code to identify bottlenecks

## 🐛 Bug Reports and Feature Requests

### Reporting Bugs
1. Check existing issues first
2. Use the bug report template
3. Provide minimal reproduction case
4. Include environment details
5. Add relevant logs and error messages

### Requesting Features
1. Use the feature request template
2. Explain the use case and problem
3. Propose a solution
4. Consider backwards compatibility
5. Discuss API design implications

## 🔄 Review Process

### Pull Request Requirements
- [ ] All tests pass
- [ ] Code coverage maintained or improved
- [ ] Documentation updated
- [ ] No linting errors
- [ ] Backwards compatibility preserved (unless breaking change is justified)

### Review Criteria
- **Functionality**: Does the code work as intended?
- **Performance**: Are there any performance regressions?
- **Security**: Are there any security implications?
- **Maintainability**: Is the code easy to understand and maintain?
- **Testing**: Are there adequate tests?
- **Documentation**: Is the documentation clear and complete?

## 📊 CI/CD Pipeline

### Automated Checks
- Code formatting (`go fmt`)
- Linting (`golangci-lint`)
- Documentation consistency (`scripts/check-doc-consistency.sh`)
- Unit tests (`go test ./...`)
- Core race tests (`go test -race .`)
- Security scanning (`govulncheck`)

Docker database integration tests and performance benchmarks run in nightly/manual workflows, not in the normal PR fast path.

### Manual Testing
- Test with different Go versions
- Verify on different operating systems
- Test with various database versions
- Performance testing under load

## 🎯 Project Priorities

### Current Focus Areas
1. **Performance Optimization**: Improving throughput and reducing latency
2. **Error Handling**: Better error messages and recovery mechanisms
3. **Documentation**: Comprehensive guides and examples
4. **Testing**: Increasing test coverage and reliability

### Future Roadmap
- Additional database support (TiDB, ClickHouse)
- Monitoring and metrics integration
- Connection pool optimization
- Advanced batching strategies

## 🤝 Community Guidelines

### Code of Conduct
- Be respectful and inclusive
- Provide constructive feedback
- Help newcomers get started
- Focus on technical merit
- Maintain professional communication

### Getting Help
- Check existing documentation first
- Search closed issues for similar problems
- Ask questions in GitHub Discussions
- Provide context and examples when asking for help

## 📚 Resources

### Documentation
- [README.md](../../README.md) - Project overview and basic usage
- [API reference](../api/reference.md) - Public API contract
- [Configuration](../api/configuration.md) - Configuration options
- [Testing guide](../guides/testing.md) - Test tiers and commands
- [Integration tests](../guides/integration-tests.md) - Docker database validation
- [Metrics spec](../guides/metrics-spec.md) - Metrics contract

### Development Tools
- [golangci-lint](https://golangci-lint.run/) - Go linting
- [Docker](https://www.docker.com/) - Containerization
- [Make](https://www.gnu.org/software/make/) - Build automation

### Learning Resources
- [Effective Go](https://golang.org/doc/effective_go.html)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Go Testing](https://golang.org/pkg/testing/)

## 📞 Contact

- **Issues**: [GitHub Issues](https://github.com/rushairer/batchflow/issues)
- **Discussions**: [GitHub Discussions](https://github.com/rushairer/batchflow/discussions)
- **Security**: Follow [SECURITY.md](../../SECURITY.md)

---

Thank you for contributing to BatchFlow! Your efforts help make this project better for everyone. 🙏
