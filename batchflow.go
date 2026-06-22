package batchflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
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
	pipeline        *gopipeline.StandardPipeline[*queuedRequest] // 异步批量处理管道
	executor        BatchExecutor                                // 批量执行器（数据库特定）
	metricsReporter MetricsReporter                              // 指标上报器（默认 Noop）
	closed          atomic.Bool                                  // 当创建时上下文被取消后置为 true，拒绝后续提交
	closeOnce       sync.Once
	done            chan struct{}

	runErrMu sync.RWMutex
	runErr   error
}

type queuedRequest struct {
	request    *Request
	enqueuedAt time.Time
}

// NewBatchFlow 创建 BatchFlow 实例
// 这是最底层的构造函数，接受任何实现了BatchExecutor接口的执行器
// 通常不直接使用，而是通过具体数据库的工厂方法创建
func NewBatchFlow(ctx context.Context, buffSize uint32, flushSize uint32, flushInterval time.Duration, executor BatchExecutor) *BatchFlow {
	return newBatchFlow(ctx, PipelineConfig{
		BufferSize:       buffSize,
		FlushSize:        flushSize,
		FlushInterval:    flushInterval,
		DrainOnCancel:    true,
		DrainGracePeriod: 2 * time.Second,
	}, executor)
}

func newBatchFlow(ctx context.Context, config PipelineConfig, executor BatchExecutor) *BatchFlow {
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
		done:            make(chan struct{}),
	}

	// 创建 flush 函数，使用批量执行器处理数据
	flushFunc := func(ctx context.Context, batchData []*queuedRequest) (err error) {
		// 管道级处理耗时（与执行器级 ObserveExecuteDuration 区分）
		processStart := time.Now()
		defer func() {
			// 仅当实现了 PipelineMetricsReporter 时上报；未实现则忽略
			if pmr, ok := batchFlow.metricsReporter.(PipelineMetricsReporter); ok && pmr != nil {
				status := "success"
				if err != nil {
					status = "fail"
				}
				pmr.ObserveProcessDuration(time.Since(processStart), status)
			}
		}()

		if pmr, ok := batchFlow.metricsReporter.(PipelineMetricsReporter); ok && pmr != nil {
			now := time.Now()
			for _, item := range batchData {
				if item == nil || item.enqueuedAt.IsZero() {
					continue
				}
				pmr.ObserveDequeueLatency(now.Sub(item.enqueuedAt))
			}
		}

		if bmr, ok := batchFlow.metricsReporter.(BatchFlowMetricsReporter); ok && bmr != nil {
			bmr.ObservePipelineFlushSize(len(batchData))
		}
		// 按schema分组处理
		schemaGroups := make(map[SchemaInterface][]*Request)
		for _, item := range batchData {
			if item == nil || item.request == nil {
				continue
			}
			request := item.request
			schema := request.Schema()
			schemaGroups[schema] = append(schemaGroups[schema], request)
		}
		if bmr, ok := batchFlow.metricsReporter.(BatchFlowMetricsReporter); ok && bmr != nil {
			bmr.ObserveSchemaGroupsPerFlush(len(schemaGroups))
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
		config.goPipelineConfig(),
		flushFunc,
	)

	batchFlow.pipeline = pipeline

	// 预留：挂接 go-pipeline v2.2.0 的 WithMetrics 到我们的 Reporter 扩展接口
	attachPipelineMetrics(pipeline, reporter)
	go func() {
		defer close(batchFlow.done)
		batchFlow.setRunErr(pipeline.AsyncPerform(ctx))
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
		b.reportSubmitRejected(reasonFromContextErr(err))
		return err
	}
	// 若 BatchFlow 所属生命周期已结束（创建时的 ctx 已取消），直接拒绝提交
	if b.closed.Load() {
		b.reportSubmitRejected("batchflow_closed")
		return context.Canceled
	}

	if request == nil {
		b.reportSubmitRejected("empty_request")
		return ErrEmptyRequest
	}

	schema := request.Schema()
	if schema == nil {
		b.reportSubmitRejected("invalid_schema")
		return ErrInvalidSchema
	}
	if schema.Columns() == nil || len(schema.Columns()) == 0 {
		b.reportSubmitRejected("missing_column")
		return ErrMissingColumn
	}
	if len(schema.Name()) == 0 {
		b.reportSubmitRejected("empty_schema_name")
		return ErrEmptySchemaName
	}

	dataChan := b.pipeline.DataChan()
	enqueueStart := time.Now()

	select {
	case dataChan <- &queuedRequest{request: request, enqueuedAt: time.Now()}:
		// 入队成功后记录入队耗时与队列长度
		// 注意：len(dataChan) 是近似观测，仅用于指标参考
		// 这里将耗时统计放在调用方路径内，默认 Noop 不引入开销
		b.metricsReporter.ObserveEnqueueLatency(time.Since(enqueueStart))
		b.metricsReporter.SetQueueLength(len(dataChan))
		return nil
	case <-ctx.Done():
		b.reportSubmitRejected(reasonFromContextErr(ctx.Err()))
		return ctx.Err()
	}
}

// Close 停止接收新请求，触发最终 flush，并等待后台 pipeline 退出。
// 它是幂等的；首次调用会关闭内部数据通道，后续调用仅等待同一个退出结果。
func (b *BatchFlow) Close() error {
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		close(b.pipeline.DataChan())
	})
	return b.Wait()
}

// Wait 等待后台 pipeline 退出并返回最终运行结果。
func (b *BatchFlow) Wait() error {
	<-b.done
	return b.getRunErr()
}

// Done 返回一个在 BatchFlow 后台 goroutine 退出时关闭的信号。
func (b *BatchFlow) Done() <-chan struct{} {
	return b.done
}

// PipelineConfig 管道配置
type PipelineConfig struct {
	BufferSize               uint32
	FlushSize                uint32
	FlushInterval            time.Duration
	MaxConcurrentFlushes     uint32
	DrainOnCancel            bool
	DrainGracePeriod         time.Duration
	FinalFlushOnCloseTimeout time.Duration

	// 可选重试配置（零值=关闭，向后兼容）
	Retry RetryConfig

	// 可选超时配置（零值=关闭，向后兼容）
	Timeout time.Duration

	// 可选指标上报器（零值=关闭，向后兼容）
	MetricsReporter MetricsReporter

	// 可选通用观测配置（结构化日志、采样、脱敏、trace hook）
	Observability ObservabilityConfig

	// 可选并发限制（零值=无限制，向后兼容）
	ConcurrencyLimit int

	// 可选批内合并/去重策略。SQL 默认仍使用 SQLOperationConfig 的 conflict-key 合并。
	Coalescer Coalescer
}

// BatchFlowConfig is the v2 constructor config for a fully assembled BatchFlow.
type BatchFlowConfig struct {
	Pipeline PipelineConfig
	Executor BatchExecutor
}

type ConfigError struct {
	Field string
	Cause error
}

func (e *ConfigError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("invalid config %s: %v", e.Field, e.Cause)
}

func (e *ConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		BufferSize:       1000,
		FlushSize:        100,
		FlushInterval:    100 * time.Millisecond,
		DrainOnCancel:    true,
		DrainGracePeriod: 2 * time.Second,
	}
}

func DefaultBatchFlowConfig(executor BatchExecutor) BatchFlowConfig {
	return BatchFlowConfig{
		Pipeline: DefaultPipelineConfig(),
		Executor: executor,
	}
}

func (c PipelineConfig) withDefaults() PipelineConfig {
	defaults := DefaultPipelineConfig()
	if c.BufferSize == 0 {
		c.BufferSize = defaults.BufferSize
	}
	if c.FlushSize == 0 {
		c.FlushSize = defaults.FlushSize
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = defaults.FlushInterval
	}
	if c.DrainGracePeriod == 0 {
		c.DrainGracePeriod = defaults.DrainGracePeriod
	}
	return c
}

func (c PipelineConfig) Validate() error {
	if c.ConcurrencyLimit < 0 {
		return &ConfigError{Field: "ConcurrencyLimit", Cause: errors.New("must be >= 0")}
	}
	if c.Retry.MaxAttempts < 0 {
		return &ConfigError{Field: "Retry.MaxAttempts", Cause: errors.New("must be >= 0")}
	}
	if c.Retry.BackoffBase < 0 {
		return &ConfigError{Field: "Retry.BackoffBase", Cause: errors.New("must be >= 0")}
	}
	if c.Retry.MaxBackoff < 0 {
		return &ConfigError{Field: "Retry.MaxBackoff", Cause: errors.New("must be >= 0")}
	}
	if c.Timeout < 0 {
		return &ConfigError{Field: "Timeout", Cause: errors.New("must be >= 0")}
	}
	if c.FlushInterval < 0 {
		return &ConfigError{Field: "FlushInterval", Cause: errors.New("must be >= 0")}
	}
	if c.DrainGracePeriod < 0 {
		return &ConfigError{Field: "DrainGracePeriod", Cause: errors.New("must be >= 0")}
	}
	if c.FinalFlushOnCloseTimeout < 0 {
		return &ConfigError{Field: "FinalFlushOnCloseTimeout", Cause: errors.New("must be >= 0")}
	}
	return nil
}

func (c PipelineConfig) goPipelineConfig() gopipeline.PipelineConfig {
	c = c.withDefaults()
	return gopipeline.PipelineConfig{
		BufferSize:               c.BufferSize,
		FlushSize:                c.FlushSize,
		FlushInterval:            c.FlushInterval,
		MaxConcurrentFlushes:     c.MaxConcurrentFlushes,
		DrainOnCancel:            c.DrainOnCancel,
		DrainGracePeriod:         c.DrainGracePeriod,
		FinalFlushOnCloseTimeout: c.FinalFlushOnCloseTimeout,
	}
}

func NewBatchFlowWithConfig(ctx context.Context, config BatchFlowConfig) (*BatchFlow, error) {
	if config.Executor == nil {
		return nil, &ConfigError{Field: "Executor", Cause: errors.New("must not be nil")}
	}
	if err := config.Pipeline.Validate(); err != nil {
		return nil, err
	}
	return newBatchFlow(ctx, config.Pipeline.withDefaults(), config.Executor), nil
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
	if observer := config.Observability.observer(); observer != nil {
		executor.WithObserver(observer)
	}
	if config.ConcurrencyLimit > 0 {
		executor.WithConcurrencyLimit(config.ConcurrencyLimit)
	}
	if config.Coalescer != nil {
		executor.WithCoalescer(config.Coalescer)
	}
	return newBatchFlow(ctx, config, executor)
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
	if observer := config.Observability.observer(); observer != nil {
		executor.WithObserver(observer)
	}
	if config.ConcurrencyLimit > 0 {
		executor.WithConcurrencyLimit(config.ConcurrencyLimit)
	}
	if config.Coalescer != nil {
		executor.WithCoalescer(config.Coalescer)
	}
	return newBatchFlow(ctx, config, executor)
}

// NewBatchFlowWithMock 使用模拟执行器创建 BatchFlow 实例（用于测试）
// 内部架构：BatchFlow -> MockExecutor（直接实现BatchExecutor，无真实数据库操作）
// 适用于单元测试，不依赖真实数据库连接
func NewBatchFlowWithMock(ctx context.Context, config PipelineConfig) (*BatchFlow, *MockExecutor) {
	mockExecutor := NewMockExecutor()
	batchFlow := newBatchFlow(ctx, config, mockExecutor)
	return batchFlow, mockExecutor
}

// NewBatchFlowWithMockDriver 使用模拟执行器创建 BatchFlow 实例（测试特定SQLDriver）
// 内部架构：BatchFlow -> MockExecutor（模拟ThrottledBatchExecutor行为，测试SQLDriver逻辑）
// 适用于测试自定义SQLDriver的SQL生成逻辑
func NewBatchFlowWithMockDriver(ctx context.Context, config PipelineConfig, sqlDriver SQLDriver) (*BatchFlow, *MockExecutor) {
	mockExecutor := NewMockExecutorWithDriver(sqlDriver)
	batchFlow := newBatchFlow(ctx, config, mockExecutor)
	return batchFlow, mockExecutor
}

func (b *BatchFlow) setRunErr(err error) {
	// Close() 驱动的正常收尾返回 nil；构造上下文取消则保留原始错误，供 Wait/调用方判断。
	b.runErrMu.Lock()
	defer b.runErrMu.Unlock()
	b.runErr = err
}

func (b *BatchFlow) getRunErr() error {
	b.runErrMu.RLock()
	defer b.runErrMu.RUnlock()
	return b.runErr
}

func (b *BatchFlow) reportSubmitRejected(reason string) {
	if reason == "" {
		return
	}
	bmr, ok := b.metricsReporter.(BatchFlowMetricsReporter)
	if ok && bmr != nil {
		bmr.IncSubmitRejected(reason)
	}
}

func reasonFromContextErr(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	default:
		return "context_error"
	}
}
