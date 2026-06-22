package batchflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// BatchExecutor 批量执行器接口 - 所有数据库驱动的统一入口
type BatchExecutor interface {
	// ExecuteBatch 执行批量操作
	ExecuteBatch(ctx context.Context, schema SchemaInterface, data []map[string]any) error
}

/*
Metrics 相关接口设计说明

- BatchExecutor：仅负责“执行”职责，保持最小接口。
- MetricsCapable[T]（泛型）：提供“读 + 写”的度量能力，方法返回自类型 T，便于在具体类型或已实例化接口上进行链式配置。
  注意：泛型接口在运行时类型断言时必须使用具体的类型实参（如 MetricsCapable[*ThrottledBatchExecutor]）。
- 兼容性与运行时探测：
  在 BatchFlow 等仅持有 BatchExecutor 的位置，无法统一断言到 MetricsCapable[T]，
  因此使用一个非泛型只读探测接口（即仅调用 MetricsReporter()）来判断是否已有 Reporter；
  若返回 nil，则在本地使用 Noop 兜底，不强制写回（写回需要具体类型 T）。

这允许：
- 在需要链式的调用点：以具体类型或已实例化能力接口使用 Set/With 风格链式；
- 在框架内部通用路径：通过只读探测保证安全、零开销的观测兜底。
*/
// MetricsCapable 扩展接口：支持性能监控报告器（自类型泛型）
type MetricsCapable[T any] interface {
	// WithMetricsReporter 设置性能监控报告器（返回实现者类型以支持链式）
	WithMetricsReporter(MetricsReporter) T
	// MetricsReporter 返回当前性能监控报告器（可能为 nil）
	MetricsReporter() MetricsReporter
}

type ConcurrencyCapable[T any] interface {
	WithConcurrencyLimit(int) T
}

// ThrottledBatchExecutor SQL数据库通用批量执行器
// 实现 ThrottledBatchExecutor 接口，为SQL数据库提供统一的执行逻辑
// 架构：ThrottledBatchExecutor -> BatchProcessor -> SQLDriver -> Database
//
// 设计优势：
// - 代码复用：所有SQL数据库共享相同的执行逻辑和指标收集
// - 职责分离：执行控制与具体处理逻辑分离
// - 易于扩展：新增SQL数据库只需实现SQLDriver接口
type ThrottledBatchExecutor struct {
	processor       BatchProcessor  // 具体的批量处理逻辑
	metricsReporter MetricsReporter // 性能指标报告器
	observer        Observer
	semaphore       chan struct{} // 可选信号量，用于限制 ExecuteBatch 并发

	// 重试配置（默认关闭）
	retryEnabled     bool
	retryMaxAttempts int
	retryBackoffBase time.Duration
	retryMaxBackoff  time.Duration
	retryClassifier  func(error) (retryable bool, reason string)
}

var _ MetricsCapable[*ThrottledBatchExecutor] = (*ThrottledBatchExecutor)(nil)

var _ ConcurrencyCapable[*ThrottledBatchExecutor] = (*ThrottledBatchExecutor)(nil)

var _ BatchExecutor = (*ThrottledBatchExecutor)(nil)

// NewThrottledBatchExecutor 创建通用执行器（使用自定义BatchProcessor）
func NewThrottledBatchExecutor(processor BatchProcessor) *ThrottledBatchExecutor {
	return &ThrottledBatchExecutor{
		processor: processor,
	}
}

// NewThrottledBatchExecutorWithDriver 创建SQL数据库执行器（推荐方式）
// 内部使用 SQLBatchProcessor + SQLDriver 组合
func NewSQLThrottledBatchExecutorWithDriver(db *sql.DB, driver SQLDriver) *ThrottledBatchExecutor {
	return NewThrottledBatchExecutor(NewSQLBatchProcessor(db, driver))
}

func NewRedisThrottledBatchExecutor(client *redis.Client) *ThrottledBatchExecutor {
	return NewThrottledBatchExecutor(NewRedisBatchProcessor(client, DefaultRedisPipelineDriver))
}

func NewRedisThrottledBatchExecutorWithDriver(client *redis.Client, driver RedisDriver) *ThrottledBatchExecutor {
	return NewThrottledBatchExecutor(NewRedisBatchProcessor(client, driver))
}

// RetryConfig 可选重试配置（零值关闭）
type RetryConfig struct {
	Enabled     bool
	MaxAttempts int           // 总尝试次数（含首轮），建议 2~3
	BackoffBase time.Duration // 退避基值（指数退避起点）
	MaxBackoff  time.Duration // 最大退避时长（上限）
	// 自定义错误分类（可选）；返回是否可重试与原因标签
	Classifier func(error) (retryable bool, reason string)
}

// WithRetryConfig 启用/配置重试（仅对 ThrottledBatchExecutor 可用）
func (e *ThrottledBatchExecutor) WithRetryConfig(cfg RetryConfig) *ThrottledBatchExecutor {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = 20 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 2 * time.Second
	}
	e.retryEnabled = cfg.Enabled
	e.retryMaxAttempts = cfg.MaxAttempts
	e.retryBackoffBase = cfg.BackoffBase
	e.retryMaxBackoff = cfg.MaxBackoff
	if cfg.Classifier != nil {
		e.retryClassifier = cfg.Classifier
	} else {
		e.retryClassifier = defaultRetryClassifier
	}
	return e
}

/*
默认重试分类器策略说明：
- 对调用方外层 ctx 的取消/超时（context.Canceled/context.DeadlineExceeded）判为不可重试（final:context）。
- 可通过 RetryConfig.Classifier 自定义策略，例如将“处理器内部超时（由处理器用 WithTimeoutCause 附带 cause）”判为可重试。
- 建议对可重试错误使用指数退避与抖动，避免热点重试风暴。
*/
func defaultRetryClassifier(err error) (bool, string) {
	if err == nil {
		return false, ""
	}
	// 非可重试：上下文取消/超时
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, "context"
	}
	// 朴素字符串分类（MySQL/PG/Redis 常见瞬态错误）
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "deadlock"):
		return true, "deadlock"
	case strings.Contains(s, "lock wait timeout"):
		return true, "lock_timeout"
	case strings.Contains(s, "timeout"):
		return true, "timeout"
	case strings.Contains(s, "connection") && (strings.Contains(s, "refused") || strings.Contains(s, "reset") || strings.Contains(s, "closed")):
		return true, "connection"
	case strings.Contains(s, "broken pipe") || strings.Contains(s, "eof"):
		return true, "io"
	default:
		return false, "non_retryable"
	}
}

// ExecuteBatch 执行批量操作
func (e *ThrottledBatchExecutor) ExecuteBatch(ctx context.Context, schema SchemaInterface, data []map[string]any) error {
	if len(data) == 0 {
		return nil
	}

	// 可选并发限流：当设置了信号量时，进入前需占用一个令牌
	if e.semaphore != nil {
		select {
		case e.semaphore <- struct{}{}:
			defer func() { <-e.semaphore }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	startTime := time.Now()
	status := "success"
	// 在途批次 +1（整个批次生命周期内有效）
	if e.metricsReporter != nil {
		e.metricsReporter.IncInflight()
		defer e.metricsReporter.DecInflight()
	}

	var err error
	attempts := 1
	if e.retryEnabled && e.retryMaxAttempts > 1 {
		attempts = e.retryMaxAttempts
	}

RETRY:
	for attempt := 1; attempt <= attempts; attempt++ {
		// 生成与执行（一次尝试）
		var operations Operations
		var preview OperationPreview
		var hasPreview bool
		attemptStart := time.Now()
		operations, preview, hasPreview, err = e.generateOperations(ctx, schema, data)
		if err != nil {
			err = batchErrorFromError(BatchStageGenerate, preview, len(data), err)
			e.reportOperationError(schema.Name(), BatchStageGenerate, err)
			e.observeBatchEvent(ctx, BatchEvent{
				Stage:       BatchStageGenerate,
				Status:      "fail",
				Attempt:     attempt,
				BatchSize:   len(data),
				Duration:    time.Since(attemptStart),
				Err:         err,
				Reason:      defaultOperationErrorReason(err),
				Backend:     preview.Backend,
				Operation:   preview.Operation,
				Schema:      schema.Name(),
				InputItems:  preview.InputItems,
				OutputItems: preview.OutputItems,
				ArgCount:    preview.ArgCount,
				Fingerprint: preview.Fingerprint,
				Attributes:  cloneAttributes(preview.Attributes),
			})
		}
		e.reportOperationGenerated(schema.Name(), operations, data, preview, hasPreview)
		if err == nil {
			err = e.processor.ExecuteOperations(ctx, operations)
			if err != nil {
				err = batchErrorFromError(BatchStageExecute, preview, len(data), err)
				e.reportOperationError(schema.Name(), BatchStageExecute, err)
				e.observeBatchEvent(ctx, BatchEvent{
					Stage:       BatchStageExecute,
					Status:      "fail",
					Attempt:     attempt,
					BatchSize:   len(data),
					Duration:    time.Since(attemptStart),
					Err:         err,
					Reason:      defaultOperationErrorReason(err),
					Backend:     preview.Backend,
					Operation:   preview.Operation,
					Schema:      schema.Name(),
					InputItems:  preview.InputItems,
					OutputItems: preview.OutputItems,
					ArgCount:    preview.ArgCount,
					Fingerprint: preview.Fingerprint,
					Attributes:  cloneAttributes(preview.Attributes),
				})
			}
		}

		if err == nil {
			status = "success"
			e.observeBatchEvent(ctx, BatchEvent{
				Stage:       BatchStageExecute,
				Status:      "success",
				Attempt:     attempt,
				BatchSize:   len(data),
				Duration:    time.Since(attemptStart),
				Backend:     preview.Backend,
				Operation:   preview.Operation,
				Schema:      schema.Name(),
				InputItems:  preview.InputItems,
				OutputItems: preview.OutputItems,
				ArgCount:    preview.ArgCount,
				Fingerprint: preview.Fingerprint,
				Attributes:  cloneAttributes(preview.Attributes),
			})
			break
		}

		// 错误分类与重试判定
		retryable, reason := false, "unknown"
		if e.retryClassifier != nil {
			retryable, reason = e.retryClassifier(err)
		}
		if !e.retryEnabled || attempt == attempts || !retryable {
			status = "fail"
			if e.metricsReporter != nil {
				e.metricsReporter.IncError(schema.Name(), "final:"+reason)
			}
			e.observeBatchEvent(ctx, BatchEvent{
				Stage:       BatchStageFinal,
				Status:      "fail",
				Attempt:     attempt,
				BatchSize:   len(data),
				Duration:    time.Since(startTime),
				Err:         err,
				Reason:      reason,
				Backend:     preview.Backend,
				Operation:   preview.Operation,
				Schema:      schema.Name(),
				InputItems:  preview.InputItems,
				OutputItems: preview.OutputItems,
				ArgCount:    preview.ArgCount,
				Fingerprint: preview.Fingerprint,
				Attributes:  cloneAttributes(preview.Attributes),
			})
			break
		}

		// 记录一次重试指标
		if e.metricsReporter != nil {
			e.metricsReporter.IncError(schema.Name(), "retry:"+reason)
		}
		e.observeBatchEvent(ctx, BatchEvent{
			Stage:       BatchStageRetry,
			Status:      "retry",
			Attempt:     attempt,
			BatchSize:   len(data),
			Duration:    time.Since(attemptStart),
			Err:         err,
			Reason:      reason,
			Backend:     preview.Backend,
			Operation:   preview.Operation,
			Schema:      schema.Name(),
			InputItems:  preview.InputItems,
			OutputItems: preview.OutputItems,
			ArgCount:    preview.ArgCount,
			Fingerprint: preview.Fingerprint,
			Attributes:  cloneAttributes(preview.Attributes),
		})

		// 指数退避 + 抖动
		backoff := e.retryBackoffBase
		for i := 1; i < attempt; i++ {
			backoff *= 2
			if backoff > e.retryMaxBackoff {
				backoff = e.retryMaxBackoff
				break
			}
		}
		// 抖动 ±20%
		jitter := time.Duration(int64(float64(backoff) * 0.2))
		sleep := backoff - jitter + time.Duration(randInt63n(int64(2*jitter+1)))
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			// 安全停止/清理定时器并终止整个重试流程
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			status = "fail"
			err = ctx.Err()
			break RETRY
		case <-timer.C:
		}
	}

	if e.metricsReporter != nil {
		e.metricsReporter.ObserveExecuteDuration(schema.Name(), len(data), time.Since(startTime), status)
	}
	return err
}

// WithMetricsReporter 设置指标报告器
func (e *ThrottledBatchExecutor) WithMetricsReporter(metricsReporter MetricsReporter) *ThrottledBatchExecutor {
	e.metricsReporter = metricsReporter
	// 注入 reporter 后，立即上报一次当前并发度（如已配置）
	if e.metricsReporter != nil {
		if e.semaphore != nil {
			e.metricsReporter.SetConcurrency(cap(e.semaphore))
		} else {
			e.metricsReporter.SetConcurrency(0)
		}
	}
	return e
}

// MetricsReporter 获取指标报告器
func (e *ThrottledBatchExecutor) MetricsReporter() MetricsReporter { return e.metricsReporter }

func (e *ThrottledBatchExecutor) WithObserver(observer Observer) *ThrottledBatchExecutor {
	e.observer = observer
	return e
}

func (e *ThrottledBatchExecutor) Observer() Observer { return e.observer }

func (e *ThrottledBatchExecutor) WithObservability(config ObservabilityConfig) *ThrottledBatchExecutor {
	e.observer = config.observer()
	return e
}

func (e *ThrottledBatchExecutor) generateOperations(ctx context.Context, schema SchemaInterface, data []map[string]any) (Operations, OperationPreview, bool, error) {
	if p, ok := e.processor.(OperationPreviewer); ok {
		ops, preview, err := p.GenerateOperationPreview(ctx, schema, data)
		if preview.Schema == "" {
			preview.Schema = schema.Name()
		}
		if preview.InputItems == 0 {
			preview.InputItems = len(data)
		}
		return ops, preview, true, err
	}
	ops, err := e.processor.GenerateOperations(ctx, schema, data)
	preview := OperationPreview{
		Backend:     BackendCustom,
		Operation:   OperationCustom,
		Schema:      schema.Name(),
		InputItems:  len(data),
		OutputItems: len(ops),
		ArgCount:    len(ops),
		Fingerprint: OperationFingerprint(BackendCustom, schema.Name(), fmt.Sprint(len(ops))),
	}
	return ops, preview, false, err
}

func (e *ThrottledBatchExecutor) reportOperationGenerated(table string, operations Operations, data []map[string]any, preview OperationPreview, hasPreview bool) {
	if reporter, ok := e.metricsReporter.(OperationMetricsReporter); ok {
		if hasPreview {
			reporter.ObserveOperationGenerated(preview)
		} else {
			reporter.ObserveOperationGenerated(OperationPreview{
				Backend:     BackendCustom,
				Operation:   OperationCustom,
				Schema:      table,
				InputItems:  len(data),
				OutputItems: len(operations),
				ArgCount:    len(operations),
				Fingerprint: OperationFingerprint(BackendCustom, table, fmt.Sprint(len(operations))),
			})
		}
	}
	reporter, ok := e.metricsReporter.(SQLMetricsReporter)
	if !ok || len(operations) == 0 {
		return
	}
	sqlText, ok := operations[0].(string)
	if !ok {
		return
	}
	if sqlPreview, ok := sqlPreviewFromOperation(preview); ok {
		reporter.ObserveSQLGenerated(table, sqlPreview.DedupStats.InputRows, sqlPreview.DedupStats.OutputRows, sqlPreview.ArgsCount)
		reporter.ObserveSQLDeduplicated(table, sqlPreview.ConflictStrategy, sqlPreview.DedupStats.DeduplicatedRows, sqlPreview.DedupStats.MergedRows)
		return
	}
	argsCount := len(sqlOperationArgs(operations))
	reporter.ObserveSQLGenerated(table, len(data), len(data), argsCount)
	_ = sqlText
}

func (e *ThrottledBatchExecutor) reportOperationError(table, stage string, err error) {
	if err == nil {
		return
	}
	reason := defaultOperationErrorReason(err)
	if reporter, ok := e.metricsReporter.(OperationMetricsReporter); ok {
		backend := ""
		var batchErr *BatchError
		if errors.As(err, &batchErr) {
			backend = batchErr.Backend
		}
		reporter.IncOperationError(table, backend, stage, reason)
	}
	if reporter, ok := e.metricsReporter.(SQLMetricsReporter); ok {
		sqlStage := SQLStage(stage)
		var sqlErr *SQLError
		if errors.As(err, &sqlErr) {
			sqlStage = sqlErr.Stage
			reason = defaultOperationErrorReason(sqlErr.Cause)
		}
		reporter.IncSQLError(table, sqlStage, reason)
	}
}

func (e *ThrottledBatchExecutor) observeBatchEvent(ctx context.Context, event BatchEvent) {
	if e.observer != nil {
		e.observer.OnBatchEvent(ctx, event)
	}
}

func defaultOperationErrorReason(err error) string {
	if err == nil {
		return "unknown"
	}
	var batchErr *BatchError
	if errors.As(err, &batchErr) && batchErr.Cause != nil {
		err = batchErr.Cause
	}
	var sqlErr *SQLError
	if errors.As(err, &sqlErr) && sqlErr.Cause != nil {
		err = sqlErr.Cause
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "duplicate"):
		return "duplicate_key"
	case strings.Contains(s, "deadlock"):
		return "deadlock"
	case strings.Contains(s, "timeout"):
		return "timeout"
	case strings.Contains(s, "connection"):
		return "connection"
	case strings.Contains(s, "syntax"):
		return "syntax"
	default:
		return "other"
	}
}

func sqlPreviewFromOperation(preview OperationPreview) (SQLPreview, bool) {
	if preview.Backend != BackendSQL {
		return SQLPreview{}, false
	}
	stats := SQLDedupStats{
		InputRows:        preview.InputItems,
		OutputRows:       preview.OutputItems,
		DeduplicatedRows: intAttribute(preview.Attributes, "deduplicated_rows"),
		MergedRows:       intAttribute(preview.Attributes, "merged_rows"),
	}
	return SQLPreview{
		Table:            preview.Schema,
		ArgsCount:        preview.ArgCount,
		ConflictStrategy: conflictStrategyFromName(fmt.Sprint(preview.Attributes["conflict_strategy"])),
		DedupStats:       stats,
		Fingerprint:      preview.Fingerprint,
	}, true
}

func conflictStrategyFromName(name string) ConflictStrategy {
	switch name {
	case "replace":
		return ConflictReplace
	case "update":
		return ConflictUpdate
	default:
		return ConflictIgnore
	}
}

func intAttribute(attrs map[string]any, key string) int {
	switch v := attrs[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// WithConcurrencyLimit 设置并发上限（limit <= 0 表示不启用限流）
func (e *ThrottledBatchExecutor) WithConcurrencyLimit(limit int) *ThrottledBatchExecutor {
	if limit > 0 {
		e.semaphore = make(chan struct{}, limit)
	} else {
		e.semaphore = nil
	}
	// 配置并发上限时，上报 Gauge（0 表示不限流）
	if e.metricsReporter != nil {
		if e.semaphore != nil {
			e.metricsReporter.SetConcurrency(cap(e.semaphore))
		} else {
			e.metricsReporter.SetConcurrency(0)
		}
	}
	return e
}

// Executor 模拟批量执行器（用于测试）
type MockExecutor struct {
	ExecutedBatches [][]map[string]any
	driver          SQLDriver
	mu              sync.RWMutex

	// 并发安全的统计聚合：按表名累计批次数、行数、参数数
	statsMu sync.Mutex
	stats   map[string]*mockStats
}

var _ BatchExecutor = (*MockExecutor)(nil)

// NewMockExecutor 创建模拟批量执行器（使用默认Driver）
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		ExecutedBatches: make([][]map[string]any, 0),
		driver:          DefaultMySQLDriver,
		stats:           make(map[string]*mockStats),
	}
}

// NewMockExecutorWithDriver 创建模拟批量执行器（使用自定义Driver）
func NewMockExecutorWithDriver(driver SQLDriver) *MockExecutor {
	if driver == nil {
		driver = DefaultMySQLDriver
	}
	return &MockExecutor{
		ExecutedBatches: make([][]map[string]any, 0),
		driver:          driver,
		stats:           make(map[string]*mockStats),
	}
}

type mockStats struct {
	Batches int64
	Rows    int64
	Args    int64
}

// addStats 并发安全地累计统计
func (e *MockExecutor) addStats(table string, rows, args int) {
	if table == "" {
		table = "_unknown_"
	}
	e.statsMu.Lock()
	s, ok := e.stats[table]
	if !ok {
		s = &mockStats{}
		e.stats[table] = s
	}
	s.Batches++
	s.Rows += int64(rows)
	s.Args += int64(args)
	e.statsMu.Unlock()
}

// SnapshotResults 返回只读快照（拷贝），用于测试收尾输出或断言
func (e *MockExecutor) SnapshotResults() map[string]map[string]int64 {
	out := make(map[string]map[string]int64)
	e.statsMu.Lock()
	for k, v := range e.stats {
		out[k] = map[string]int64{
			"batches": v.Batches,
			"rows":    v.Rows,
			"args":    v.Args,
		}
	}
	e.statsMu.Unlock()
	return out
}

// ExecuteBatch 模拟执行批量操作
func (e *MockExecutor) ExecuteBatch(ctx context.Context, schema SchemaInterface, data []map[string]any) error {
	s, ok := schema.(*SQLSchema)
	if !ok {
		return errors.New("schema is not a SQLSchema")
	}

	e.mu.Lock()
	e.ExecutedBatches = append(e.ExecutedBatches, data)
	e.mu.Unlock()

	// 生成SQL信息（不输出大参数）
	_, args, err := e.driver.GenerateInsertSQL(ctx, s, data)
	if err != nil {
		return err
	}

	// 统计聚合（避免每批次打印噪音日志）
	e.addStats(schema.Name(), len(data), len(args))

	return nil
}

// SnapshotExecutedBatches 返回一次性快照，避免并发读写竞态
func (e *MockExecutor) SnapshotExecutedBatches() [][]map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([][]map[string]any, len(e.ExecutedBatches))
	copy(out, e.ExecutedBatches)
	return out
}

// randInt63n 返回 [0,n) 的随机数；避免额外依赖，用 time.Now 纳秒抖动
func randInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	// LCG 简易随机（不要求强随机，仅用于退避抖动）
	seed := time.Now().UnixNano()
	seed = (seed*6364136223846793005 + 1) & 0x7fffffffffffffff
	return int64(seed % n)
}
