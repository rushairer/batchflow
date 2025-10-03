package batchflow

import (
	"context"
	"database/sql"
	"errors"

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

// SQLBatchProcessor SQL数据库批量处理器
// 实现 BatchProcessor 接口，专注于SQL数据库的核心处理逻辑
type SQLBatchProcessor struct {
	db     *sql.DB   // 数据库连接
	driver SQLDriver // SQL生成器（数据库特定）
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

func (bp *SQLBatchProcessor) GenerateOperations(ctx context.Context, schema SchemaInterface, data []map[string]any) (operations Operations, err error) {
	s, ok := schema.(*SQLSchema)
	if !ok {
		return nil, errors.New("schema is not a SQLSchema")
	}

	sql, args, innerErr := bp.driver.GenerateInsertSQL(ctx, s, data)
	if innerErr != nil {
		return nil, innerErr
	}
	operations = append(operations, sql)
	operations = append(operations, args...)
	return operations, nil
}

func (bp *SQLBatchProcessor) ExecuteOperations(ctx context.Context, operations Operations) error {
	if sql, ok := operations[0].(string); ok {
		args := operations[1:]
		_, err := bp.db.ExecContext(ctx, sql, args...)
		return err
	}
	return errors.New("invalid operation type")
}

// RedisBatchProcessor Redis批量处理器
// 实现 BatchProcessor 接口，专注于Redis的核心处理逻辑
type RedisBatchProcessor struct {
	client *redis.Client // Redis客户端连接
	driver RedisDriver   // Redis操作生成器
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

func (rp *RedisBatchProcessor) ExecuteOperations(ctx context.Context, operations Operations) error {
	// 使用Pipeline批量执行
	pipeline := rp.client.Pipeline()

	for _, operation := range operations {
		if cmd, ok := operation.(RedisCmd); ok {
			pipeline.Do(ctx, cmd...)
		}
	}

	// 执行Pipeline
	cmds, err := pipeline.Exec(ctx)
	if err != nil {
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
