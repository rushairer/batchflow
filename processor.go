package batchflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Operations []any

// BatchProcessor 批量处理器接口 - SQL数据库的核心处理逻辑
type BatchProcessor interface {
	// GenerateOperations 生成批量操作
	GenerateOperations(ctx context.Context, schema SchemaInterface, data []map[string]any) (operations Operations, err error)

	// ExecuteOperations 执行批量操作
	ExecuteOperations(ctx context.Context, operations Operations) error
}

// TimeOutCapable 扩展接口：支持超时设置（自类型泛型）
type TimeOutCapable[T any] interface {
	WithTimeout(time.Duration) T
}

// SQLBatchProcessor SQL数据库批量处理器
// 实现 BatchProcessor 接口，专注于SQL数据库的核心处理逻辑
type SQLBatchProcessor struct {
	db      *sql.DB   // 数据库连接
	driver  SQLDriver // SQL生成器（数据库特定）
	timeout time.Duration
}

var _ BatchProcessor = (*SQLBatchProcessor)(nil)

// NewSQLBatchProcessor 创建SQL批量处理器
// 参数：
// - db: 数据库连接（用户管理连接池）
// - driver: 数据库特定的SQL生成器
func NewSQLBatchProcessor(db *sql.DB, driver SQLDriver) *SQLBatchProcessor {
	return &SQLBatchProcessor{
		db:     db,
		driver: driver,
	}
}

func (bp *SQLBatchProcessor) WithTimeout(timeout time.Duration) *SQLBatchProcessor {
	bp.timeout = timeout
	return bp
}

func (bp *SQLBatchProcessor) GenerateSQLPreview(ctx context.Context, schema *SQLSchema, data []map[string]any) (SQLPreview, error) {
	return GenerateSQLPreview(ctx, bp.driver, schema, data)
}

func (bp *SQLBatchProcessor) GenerateOperationPreview(ctx context.Context, schema SchemaInterface, data []map[string]any) (Operations, OperationPreview, error) {
	s, ok := schema.(*SQLSchema)
	if !ok {
		err := &BatchError{Stage: BatchStageValidate, Backend: BackendSQL, Schema: schema.Name(), BatchSize: len(data), Cause: errors.New("schema is not a SQLSchema")}
		return nil, OperationPreview{Backend: BackendSQL, Schema: schema.Name(), InputItems: len(data)}, err
	}
	preview, err := bp.GenerateSQLPreview(ctx, s, data)
	if err != nil {
		return nil, preview.OperationPreview(), err
	}
	operations := make(Operations, 0, 1+len(preview.Args))
	operations = append(operations, preview.SQL)
	operations = append(operations, preview.Args...)
	return operations, preview.OperationPreview(), nil
}

func (bp *SQLBatchProcessor) GenerateOperations(ctx context.Context, schema SchemaInterface, data []map[string]any) (operations Operations, err error) {
	s, ok := schema.(*SQLSchema)
	if !ok {
		return nil, &SQLError{Stage: SQLStageValidate, Table: schema.Name(), BatchSize: len(data), Cause: errors.New("schema is not a SQLSchema")}
	}

	preview, innerErr := bp.GenerateSQLPreview(ctx, s, data)
	if innerErr != nil {
		return nil, innerErr
	}
	operations = append(operations, preview.SQL)
	operations = append(operations, preview.Args...)
	return operations, nil
}

/*
SQL 执行语义：
  - 在设置了 bp.timeout 时，使用 context.WithTimeoutCause 派生子 ctx（具体 cause 如 "execute batch timeout"）。
  - 当子 ctx 达到超时时，驱动通常返回 context.DeadlineExceeded；本处理器会读取 context.Cause(ctx) 并原样返回该 cause，
    以便上层执行器的重试分类器可以区分“处理器内部超时”，按需实施重试与退避。
  - 安全性：在执行前校验空 operations，避免越界；不持久化/返回子 ctx，defer cancel() 安全。
*/
func (bp *SQLBatchProcessor) ExecuteOperations(ctx context.Context, operations Operations) error {
	if bp.timeout > 0 {
		ctxTimeout, cancel := context.WithTimeoutCause(ctx, bp.timeout, errors.New("execute batch timeout"))
		defer cancel()

		ctx = ctxTimeout
	}

	if len(operations) < 1 {
		return &SQLError{Stage: SQLStageValidate, Cause: errors.New("empty operations")}
	}

	if preview, ok := operations[0].(SQLPreview); ok {
		_, err := bp.db.ExecContext(ctx, preview.SQL, preview.Args...)
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			if cause := context.Cause(ctx); cause != nil {
				err = cause
			}
		}
		if err != nil {
			return &SQLError{
				Stage:            SQLStageExecute,
				Table:            preview.Table,
				BatchSize:        preview.DedupStats.InputRows,
				ConflictStrategy: preview.ConflictStrategy,
				ConflictColumns:  preview.ConflictColumns,
				UpdateColumns:    preview.UpdateColumns,
				SQLFingerprint:   preview.Fingerprint,
				ArgsCount:        preview.ArgsCount,
				Cause:            err,
			}
		}
		return nil
	}

	if sql, ok := operations[0].(string); ok {
		args := sqlOperationArgs(operations)
		_, err := bp.db.ExecContext(ctx, sql, args...)
		// processor 会捕获超时异常, 可以出发重试
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
		}
		if err != nil {
			return &SQLError{
				Stage:          SQLStageExecute,
				SQLFingerprint: FingerprintSQL(sql),
				ArgsCount:      len(args),
				Cause:          err,
			}
		}
		return err
	}
	return &SQLError{Stage: SQLStageValidate, Cause: errors.New("invalid operation type")}
}

func sqlOperationArgs(operations Operations) []any {
	return operations[1:]
}

// RedisBatchProcessor Redis批量处理器
// 实现 BatchProcessor 接口，专注于Redis的核心处理逻辑
type RedisBatchProcessor struct {
	client  *redis.Client // Redis客户端连接
	driver  RedisDriver   // Redis操作生成器
	timeout time.Duration
}

var _ BatchProcessor = (*RedisBatchProcessor)(nil)

// NewRedisBatchProcessor 创建Redis批量处理器
// 参数：
// - client: Redis客户端连接
// - driver: Redis操作生成器
func NewRedisBatchProcessor(client *redis.Client, driver RedisDriver) *RedisBatchProcessor {
	return &RedisBatchProcessor{
		client: client,
		driver: driver,
	}
}

func (rp *RedisBatchProcessor) WithTimeout(timeout time.Duration) *RedisBatchProcessor {
	rp.timeout = timeout
	return rp
}

func (rp *RedisBatchProcessor) GenerateOperationPreview(ctx context.Context, schema SchemaInterface, data []map[string]any) (Operations, OperationPreview, error) {
	operations, err := rp.GenerateOperations(ctx, schema, data)
	preview := OperationPreview{
		Backend:     BackendRedis,
		Operation:   OperationCommand,
		Schema:      schema.Name(),
		InputItems:  len(data),
		OutputItems: len(operations),
		ArgCount:    redisOperationArgCount(operations),
		Fingerprint: redisOperationsFingerprint(schema, operations),
		Attributes: map[string]any{
			"commands": len(operations),
			"columns":  len(schema.Columns()),
		},
	}
	if err != nil {
		return nil, preview, &BatchError{
			Stage:       BatchStageGenerate,
			Backend:     BackendRedis,
			Schema:      schema.Name(),
			BatchSize:   len(data),
			Fingerprint: preview.Fingerprint,
			Attributes:  cloneAttributes(preview.Attributes),
			Cause:       err,
		}
	}
	return operations, preview, nil
}

// GenerateOperations 执行批量操作
func (rp *RedisBatchProcessor) GenerateOperations(ctx context.Context, schema SchemaInterface, data []map[string]any) (operations Operations, err error) {
	s, ok := schema.(*Schema)
	if !ok {
		return nil, errors.New("schema is not a Schema")
	}

	cmds, innerErr := rp.driver.GenerateCmds(ctx, s, data)
	if innerErr != nil {
		return nil, innerErr
	}

	for _, cmd := range cmds {
		operations = append(operations, cmd)
	}
	return operations, nil
}

/*
Redis 执行与快速退出：
- 在设置了 rp.timeout 时，使用 context.WithTimeoutCause 限定执行时限。
- 大批量 operations 时，在循环内检查 ctx（可每次或每 N 次）以快速响应取消/超时，避免无谓迭代开销。
- Pipeline 在本函数内构建并执行，不跨越函数生命周期，defer cancel() 安全。
*/
func (rp *RedisBatchProcessor) ExecuteOperations(ctx context.Context, operations Operations) error {
	if rp.timeout > 0 {
		ctxTimeout, cancel := context.WithTimeoutCause(ctx, rp.timeout, errors.New("execute batch timeout"))
		defer cancel()

		ctx = ctxTimeout
	}

	// 使用Pipeline批量执行
	pipeline := rp.client.Pipeline()

	for _, operation := range operations {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cmd, ok := operation.(RedisCmd); ok {
			pipeline.Do(ctx, cmd...)
		}
	}

	// 执行Pipeline
	cmds, err := pipeline.Exec(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
		}
		return err
	}

	// 检查每个命令的执行结果
	for _, cmd := range cmds {
		if cmd.Err() != nil {
			err = errors.Join(err, cmd.Err())
		}
	}

	return err
}

func redisOperationArgCount(operations Operations) int {
	count := 0
	for _, operation := range operations {
		if cmd, ok := operation.(RedisCmd); ok {
			count += len(cmd)
		}
	}
	return count
}

func redisOperationsFingerprint(schema SchemaInterface, operations Operations) string {
	parts := []string{BackendRedis, schema.Name()}
	for _, operation := range operations {
		if cmd, ok := operation.(RedisCmd); ok && len(cmd) > 0 {
			parts = append(parts, fmt.Sprint(cmd[0]))
		}
	}
	return OperationFingerprint(parts...)
}
