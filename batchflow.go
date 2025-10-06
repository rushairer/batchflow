package batchflow

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	redisV9 "github.com/redis/go-redis/v9"
	gopipeline "github.com/rushairer/go-pipeline/v2"
)

// BatchFlow 批量处理管道
// 核心组件，整合 go-pipeline 和 BatchExecutor，提供统一的批量处理接口
//
// 架构层次：
// Application -> BatchFlow -> gopipeline -> BatchExecutor -> Database
//
/*
支持的 BatchExecutor 实现：
- SQL 数据库：ThrottledBatchExecutor + SQLBatchProcessor + SQLDriver
- NoSQL 数据库：ThrottledBatchExecutor + RedisBatchProcessor + RedisDriver（直接生成/执行命令）
- 测试环境：MockExecutor（直接实现 BatchExecutor）
可选能力：
- WithConcurrencyLimit：通过信号量限制 ExecuteBatch 并发，避免攒批后同时冲击数据库（limit <= 0 等价于不限流）
*/
type BatchFlow struct {
	pipeline        *gopipeline.StandardPipeline[*Request] // 异步批量处理管道
	executor        BatchExecutor                          // 批量执行器（数据库特定）
	metricsReporter MetricsReporter                        // 指标上报器（默认 Noop）
	closed          atomic.Bool                            // 当创建时上下文被取消后置为 true，拒绝后续提交
}

// NewBatchFlow 创建 BatchFlow 实例
// 这是最底层的构造函数，接受任何实现了BatchExecutor接口的执行器
// 通常不直接使用，而是通过具体数据库的工厂方法创建
func NewBatchFlow(ctx context.Context, buffSize uint32, flushSize uint32, flushInterval time.Duration, executor BatchExecutor) *BatchFlow {
	// 确保 BatchFlow 始终拥有可用 reporter，但不误覆盖自定义执行器的已有配置
	var reporter MetricsReporter
	// 说明：
	// - 由于 Go 对泛型接口的类型断言需要具体类型实参，无法在此处（仅持有 BatchExecutor）统一断言 MetricsCapable[T]。
	// - 因此采用非泛型的只读探测接口 MetricsProvider 进行安全探测；若为 nil，则在本地使用 Noop 兜底，不强制写回。
	if mp, ok := executor.(interface{ MetricsReporter() MetricsReporter }); ok {
		if r := mp.MetricsReporter(); r != nil {
			reporter = r
		} else {
			reporter = NewNoopMetricsReporter()
			// 不强制写回：写回需要具体的 T（MetricsCapable[T]），此处无法统一处理
		}
	} else {
		reporter = NewNoopMetricsReporter()
	}

	batchFlow := &BatchFlow{
		executor:        executor,
		metricsReporter: reporter,
	}

	// 创建 flush 函数，使用批量执行器处理数据
	flushFunc := func(ctx context.Context, batchData []*Request) error {
		// 按schema分组处理
		schemaGroups := make(map[SchemaInterface][]*Request)
		for _, request := range batchData {
			schema := request.Schema()
			schemaGroups[schema] = append(schemaGroups[schema], request)
		}

		// 处理每个schema组
		for schema, requests := range schemaGroups {
			assembleStart := time.Now()
			// 在开始耗时操作前快速检查
			if err := ctx.Err(); err != nil {
				return err
			}

			// 转换为数据格式
			data := make([]map[string]any, len(requests))
			for i, request := range requests {
				// 如果单个schema的数据量很大，可以定期检查
				if len(requests) > 10000 && i%1000 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				rowData := make(map[string]any)
				values := request.GetOrderedValues()
				columns := schema.Columns()

				for j, col := range columns {
					if j < len(values) {
						rowData[col] = values[j]
					}
				}
				data[i] = rowData
			}

			// 组装完成指标（批大小 + 组装耗时）
			batchFlow.metricsReporter.ObserveBatchSize(len(requests))
			batchFlow.metricsReporter.ObserveBatchAssemble(time.Since(assembleStart))

			// 执行批量操作
			if err := batchFlow.executor.ExecuteBatch(ctx, schema, data); err != nil {
				return err
			}
		}
		return nil
	}

	pipeline := gopipeline.NewStandardPipeline(
		gopipeline.PipelineConfig{
			BufferSize:    buffSize,
			FlushSize:     flushSize,
			FlushInterval: flushInterval,
		},
		flushFunc,
	)

	batchFlow.pipeline = pipeline
	go func() {
		_ = pipeline.AsyncPerform(ctx)
	}()
	// 标记管道生命周期：创建时 ctx 一旦取消，后续 Submit 均应拒绝
	go func() {
		<-ctx.Done()
		batchFlow.closed.Store(true)
	}()

	return batchFlow
}

// ErrorChan 获取错误通道
func (b *BatchFlow) ErrorChan(size int) <-chan error {
	return b.pipeline.ErrorChan(size)
}

// Submit 提交请求到批量处理管道
func (b *BatchFlow) Submit(ctx context.Context, request *Request) error {
	// 优先尊重取消，避免 select 在多就绪时随机选择发送路径
	if err := ctx.Err(); err != nil {
		return err
	}
	// 若 BatchFlow 所属生命周期已结束（创建时的 ctx 已取消），直接拒绝提交
	if b.closed.Load() {
		return context.Canceled
	}

	if request == nil {
		return ErrEmptyRequest
	}

	schema := request.Schema()
	if schema == nil {
		return ErrInvalidSchema
	}
	if schema.Columns() == nil || len(schema.Columns()) == 0 {
		return ErrMissingColumn
	}
	if len(schema.Name()) == 0 {
		return ErrEmptySchemaName
	}

	dataChan := b.pipeline.DataChan()
	enqueueStart := time.Now()

	select {
	case dataChan <- request:
		// 入队成功后记录入队耗时与队列长度
		// 注意：len(dataChan) 是近似观测，仅用于指标参考
		// 这里将耗时统计放在调用方路径内，默认 Noop 不引入开销
		b.metricsReporter.ObserveEnqueueLatency(time.Since(enqueueStart))
		b.metricsReporter.SetQueueLength(len(dataChan))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PipelineConfig 管道配置
type PipelineConfig struct {
	BufferSize    uint32
	FlushSize     uint32
	FlushInterval time.Duration

	// 可选重试配置（零值=关闭，向后兼容）
	Retry RetryConfig

	// 可选超时配置（零值=关闭，向后兼容）
	Timeout time.Duration

	// 可选指标上报器（零值=关闭，向后兼容）
	MetricsReporter MetricsReporter

	// 可选并发限制（零值=无限制，向后兼容）
	ConcurrencyLimit int
}

// NewSQLBatchFlow 创建SQL BatchFlow实例（使用自定义Driver）
func NewSQLBatchFlowWithDriver(ctx context.Context, db *sql.DB, config PipelineConfig, driver SQLDriver) *BatchFlow {
	processor := NewSQLBatchProcessor(db, driver)
	if config.Timeout > 0 {
		processor.WithTimeout(config.Timeout)
	}
	executor := NewThrottledBatchExecutor(processor)
	if config.Retry.Enabled {
		executor.WithRetryConfig(config.Retry)
	}
	if config.MetricsReporter != nil {
		executor.WithMetricsReporter(config.MetricsReporter)
	}
	if config.ConcurrencyLimit > 0 {
		executor.WithConcurrencyLimit(config.ConcurrencyLimit)
	}
	return NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, executor)
}

// NewMySQLBatchFlow 创建MySQL BatchFlow实例（使用默认Driver）
/*
内部架构：BatchFlow -> ThrottledBatchExecutor -> SQLBatchProcessor -> MySQLDriver -> MySQL
*/
// 这是推荐的使用方式，使用MySQL优化的默认配置
func NewMySQLBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow {
	return NewSQLBatchFlowWithDriver(ctx, db, config, DefaultMySQLDriver)
}

// NewPostgreSQLBatchFlow 创建PostgreSQL BatchFlow实例（使用默认Driver）
func NewPostgreSQLBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow {
	return NewSQLBatchFlowWithDriver(ctx, db, config, DefaultPostgreSQLDriver)
}

// NewSQLiteBatchFlow 创建SQLite BatchFlow实例（使用默认Driver）
func NewSQLiteBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow {
	return NewSQLBatchFlowWithDriver(ctx, db, config, DefaultSQLiteDriver)
}

// NewRedisBatchFlow 创建Redis BatchFlow实例
/*
内部架构（NoSQL）：BatchFlow -> ThrottledBatchExecutor -> RedisBatchProcessor -> RedisDriver -> Redis
说明：NoSQL 路径不使用 SQL 抽象层，直接生成并执行 Redis 命令；仍可启用 WithConcurrencyLimit 控制批次并发。
*/
func NewRedisBatchFlow(ctx context.Context, db *redisV9.Client, config PipelineConfig) *BatchFlow {
	return NewRedisBatchFlowWithDriver(ctx, db, config, DefaultRedisPipelineDriver)
}

func NewRedisBatchFlowWithDriver(ctx context.Context, db *redisV9.Client, config PipelineConfig, driver RedisDriver) *BatchFlow {
	processor := NewRedisBatchProcessor(db, driver)
	if config.Timeout > 0 {
		processor.WithTimeout(config.Timeout)
	}
	executor := NewThrottledBatchExecutor(processor)
	if config.Retry.Enabled {
		executor.WithRetryConfig(config.Retry)
	}
	if config.MetricsReporter != nil {
		executor.WithMetricsReporter(config.MetricsReporter)
	}
	if config.ConcurrencyLimit > 0 {
		executor.WithConcurrencyLimit(config.ConcurrencyLimit)
	}
	return NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, executor)
}

// NewBatchFlowWithMock 使用模拟执行器创建 BatchFlow 实例（用于测试）
// 内部架构：BatchFlow -> MockExecutor（直接实现BatchExecutor，无真实数据库操作）
// 适用于单元测试，不依赖真实数据库连接
func NewBatchFlowWithMock(ctx context.Context, config PipelineConfig) (*BatchFlow, *MockExecutor) {
	mockExecutor := NewMockExecutor()
	batchFlow := NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, mockExecutor)
	return batchFlow, mockExecutor
}

// NewBatchFlowWithMockDriver 使用模拟执行器创建 BatchFlow 实例（测试特定SQLDriver）
// 内部架构：BatchFlow -> MockExecutor（模拟ThrottledBatchExecutor行为，测试SQLDriver逻辑）
// 适用于测试自定义SQLDriver的SQL生成逻辑
func NewBatchFlowWithMockDriver(ctx context.Context, config PipelineConfig, sqlDriver SQLDriver) (*BatchFlow, *MockExecutor) {
	mockExecutor := NewMockExecutorWithDriver(sqlDriver)
	batchFlow := NewBatchFlow(ctx, config.BufferSize, config.FlushSize, config.FlushInterval, mockExecutor)
	return batchFlow, mockExecutor
}
