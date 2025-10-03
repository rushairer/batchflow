package batchflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// SQLDriver 数据库特定的SQL生成器接口
type SQLDriver interface {
	GenerateInsertSQL(ctx context.Context, schema *SQLSchema, data []map[string]any) (sql string, args []any, err error)
}

var DefaultMySQLDriver = NewMySQLDriver()

type MySQLDriver struct {
	placeholders sync.Map // key: (colCount<<32)|batchSize  value: string
}

var _ SQLDriver = (*MySQLDriver)(nil)

func NewMySQLDriver() *MySQLDriver {
	return &MySQLDriver{}
}

// GenerateInsertSQL 生成MySQL批量插入SQL
func (d *MySQLDriver) GenerateInsertSQL(ctx context.Context, schema *SQLSchema, data []map[string]any) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, nil
	}

	columns := schema.Columns()
	if len(columns) == 0 {
		return "", nil, errors.New("no columns defined in schema")
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(data))

	// 构建参数数组
	args := make([]any, 0, len(data)*len(columns))
	for _, row := range data {
		// 忽略超时或取消的请求
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		for _, col := range columns {
			args = append(args, row[col])
		}
	}

	baseSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)

	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("INSERT IGNORE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictReplace:
		sql := fmt.Sprintf("REPLACE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictUpdate:
		updatePairs := make([]string, len(columns))
		for i, col := range columns {
			updatePairs[i] = fmt.Sprintf("%s = VALUES(%s)", col, col)
		}
		sql := fmt.Sprintf("%s ON DUPLICATE KEY UPDATE %s", baseSQL, strings.Join(updatePairs, ", "))
		return sql, args, nil
	default:
		return baseSQL, args, nil
	}
}

func (d *MySQLDriver) generatePlaceholders(columnCount, batchSize int) string {
	if columnCount <= 0 || batchSize <= 0 {
		return ""
	}
	key := (uint64(columnCount) << 32) | uint64(batchSize)
	if v, ok := d.placeholders.Load(key); ok {
		return v.(string)
	}
	singleRow := "(" + strings.Repeat("?, ", columnCount-1) + "?)"
	rows := make([]string, batchSize)
	for i := range rows {
		rows[i] = singleRow
	}
	out := strings.Join(rows, ", ")
	d.placeholders.Store(key, out)
	return out
}

var DefaultPostgreSQLDriver = NewPostgreSQLDriver()

type PostgreSQLDriver struct {
	placeholders sync.Map // key: (colCount<<32)|batchSize  value: string
}

var _ SQLDriver = (*PostgreSQLDriver)(nil)

func NewPostgreSQLDriver() *PostgreSQLDriver {
	return &PostgreSQLDriver{}
}

// GenerateInsertSQL 生成PostgreSQL批量插入SQL
func (d *PostgreSQLDriver) GenerateInsertSQL(ctx context.Context, schema *SQLSchema, data []map[string]any) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, nil
	}

	columns := schema.Columns()
	if len(columns) == 0 {
		return "", nil, errors.New("no columns defined in schema")
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(data))

	// 构建参数数组
	args := make([]any, 0, len(data)*len(columns))
	for _, row := range data {
		// 忽略超时或取消的请求
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		for _, col := range columns {
			args = append(args, row[col])
		}
	}

	baseSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)

	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := baseSQL + " ON CONFLICT DO NOTHING"
		return sql, args, nil
	case ConflictReplace, ConflictUpdate:
		updatePairs := make([]string, len(columns))
		for i, col := range columns {
			updatePairs[i] = fmt.Sprintf("%s = EXCLUDED.%s", col, col)
		}
		// 假设第一个列是主键
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s", baseSQL, columns[0], strings.Join(updatePairs, ", "))
		return sql, args, nil
	default:
		return baseSQL, args, nil
	}
}

func (d *PostgreSQLDriver) generatePlaceholders(columnCount, batchSize int) string {
	if columnCount <= 0 || batchSize <= 0 {
		return ""
	}
	key := (uint64(columnCount) << 32) | uint64(batchSize)
	if v, ok := d.placeholders.Load(key); ok {
		return v.(string)
	}
	rows := make([]string, batchSize)
	for i := 0; i < batchSize; i++ {
		ph := make([]string, columnCount)
		for j := 0; j < columnCount; j++ {
			ph[j] = fmt.Sprintf("$%d", i*columnCount+j+1)
		}
		rows[i] = "(" + strings.Join(ph, ", ") + ")"
	}
	out := strings.Join(rows, ", ")
	d.placeholders.Store(key, out)
	return out
}

var DefaultSQLiteDriver = NewSQLiteDriver()

type SQLiteDriver struct {
	placeholders sync.Map // key: (colCount<<32)|batchSize  value: string
}

var _ SQLDriver = (*SQLiteDriver)(nil)

func NewSQLiteDriver() *SQLiteDriver {
	return &SQLiteDriver{}
}

// GenerateInsertSQL 生成SQLite批量插入SQL
func (d *SQLiteDriver) GenerateInsertSQL(ctx context.Context, schema *SQLSchema, data []map[string]any) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, nil
	}

	columns := schema.Columns()
	if len(columns) == 0 {
		return "", nil, errors.New("no columns defined in schema")
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(data))

	// 构建参数数组
	args := make([]any, 0, len(data)*len(columns))
	for _, row := range data {
		// 忽略超时或取消的请求
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		for _, col := range columns {
			args = append(args, row[col])
		}
	}

	baseSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)

	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictReplace:
		sql := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictUpdate:
		updatePairs := make([]string, len(columns))
		for i, col := range columns {
			updatePairs[i] = fmt.Sprintf("%s = excluded.%s", col, col)
		}
		sql := fmt.Sprintf("%s ON CONFLICT DO UPDATE SET %s", baseSQL, strings.Join(updatePairs, ", "))
		return sql, args, nil
	default:
		return baseSQL, args, nil
	}
}

func (d *SQLiteDriver) generatePlaceholders(columnCount, batchSize int) string {
	if columnCount <= 0 || batchSize <= 0 {
		return ""
	}
	key := (uint64(columnCount) << 32) | uint64(batchSize)
	if v, ok := d.placeholders.Load(key); ok {
		return v.(string)
	}
	singleRow := "(" + strings.Repeat("?, ", columnCount-1) + "?)"
	rows := make([]string, batchSize)
	for i := range rows {
		rows[i] = singleRow
	}
	out := strings.Join(rows, ", ")
	d.placeholders.Store(key, out)
	return out
}

type MockDriver struct {
	databaseType string
}

var _ SQLDriver = (*MockDriver)(nil)

func NewMockDriver(databaseType string) *MockDriver {
	return &MockDriver{databaseType: databaseType}
}

// GenerateInsertSQL 生成模拟SQL（默认MySQL语法）
func (d *MockDriver) GenerateInsertSQL(ctx context.Context, schema *SQLSchema, data []map[string]any) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, nil
	}

	columns := schema.Columns()
	if len(columns) == 0 {
		return "", nil, errors.New("no columns defined in schema")
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(data))

	// 构建参数数组
	args := make([]any, 0, len(data)*len(columns))
	for _, row := range data {
		// 忽略超时或取消的请求
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		for _, col := range columns {
			args = append(args, row[col])
		}
	}

	baseSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)

	// 根据数据库类型生成不同的SQL
	switch d.databaseType {
	case "mysql":
		return d.generateMySQLSQL(schema, baseSQL, columnsStr, placeholders, args)
	case "postgresql":
		return d.generatePostgreSQLSQL(schema, baseSQL, columnsStr, placeholders, args)
	case "sqlite":
		return d.generateSQLiteSQL(schema, baseSQL, columnsStr, placeholders, args)
	default:
		return baseSQL, args, nil
	}
}

func (d *MockDriver) generateMySQLSQL(schema *SQLSchema, baseSQL, columnsStr, placeholders string, args []any) (string, []any, error) {
	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("INSERT IGNORE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictReplace:
		sql := fmt.Sprintf("REPLACE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictUpdate:
		columns := schema.Columns()
		updatePairs := make([]string, len(columns))
		for i, col := range columns {
			updatePairs[i] = fmt.Sprintf("%s = VALUES(%s)", col, col)
		}
		sql := fmt.Sprintf("%s ON DUPLICATE KEY UPDATE %s", baseSQL, strings.Join(updatePairs, ", "))
		return sql, args, nil
	default:
		return baseSQL, args, nil
	}
}

func (d *MockDriver) generatePostgreSQLSQL(schema *SQLSchema, baseSQL, _, _ string, args []any) (string, []any, error) {
	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := baseSQL + " ON CONFLICT DO NOTHING"
		return sql, args, nil
	case ConflictReplace, ConflictUpdate:
		columns := schema.Columns()
		updatePairs := make([]string, len(columns))
		for i, col := range columns {
			updatePairs[i] = fmt.Sprintf("%s = EXCLUDED.%s", col, col)
		}
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s", baseSQL, columns[0], strings.Join(updatePairs, ", "))
		return sql, args, nil
	default:
		return baseSQL, args, nil
	}
}

func (d *MockDriver) generateSQLiteSQL(schema *SQLSchema, baseSQL, columnsStr, placeholders string, args []any) (string, []any, error) {
	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictReplace:
		sql := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictUpdate:
		columns := schema.Columns()
		updatePairs := make([]string, len(columns))
		for i, col := range columns {
			updatePairs[i] = fmt.Sprintf("%s = excluded.%s", col, col)
		}
		sql := fmt.Sprintf("%s ON CONFLICT DO UPDATE SET %s", baseSQL, strings.Join(updatePairs, ", "))
		return sql, args, nil
	default:
		return baseSQL, args, nil
	}
}

func (d *MockDriver) generatePlaceholders(columnCount, batchSize int) string {
	singleRow := "(" + strings.Repeat("?, ", columnCount-1) + "?)"
	rows := make([]string, batchSize)
	for i := range rows {
		rows[i] = singleRow
	}
	return strings.Join(rows, ", ")
}

type RedisCmd []any

type RedisDriver interface {
	GenerateCmds(ctx context.Context, schema SchemaInterface, data []map[string]any) ([]RedisCmd, error)
}

var DefaultRedisPipelineDriver = NewRedisPipelineDriver()

type RedisPipelineDriver struct{}

var _ RedisDriver = (*RedisPipelineDriver)(nil)

func NewRedisPipelineDriver() *RedisPipelineDriver {
	return &RedisPipelineDriver{}
}

func (d *RedisPipelineDriver) GenerateCmds(ctx context.Context, schema SchemaInterface, data []map[string]any) ([]RedisCmd, error) {
	columns := schema.Columns()

	if len(columns) < 2 {
		return nil, errors.New("redis schema must have at least 2 columns: cmd and key")
	}

	batchCmd := make([]RedisCmd, len(data))
	for i, row := range data {
		// 忽略超时或取消的请求
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		batchCmd[i] = make(RedisCmd, len(columns))
		for j, col := range columns {
			batchCmd[i][j] = row[col]
		}
	}
	return batchCmd, nil
}
