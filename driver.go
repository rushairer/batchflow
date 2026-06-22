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

func prepareSQLRowsAndArgs(ctx context.Context, schema *SQLSchema, data []map[string]any) ([]map[string]any, []any, error) {
	rows := deduplicateSQLRows(schema, data)
	columns := schema.Columns()
	args := make([]any, 0, len(rows)*len(columns))
	for _, row := range rows {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		for _, col := range columns {
			args = append(args, row[col])
		}
	}
	return rows, args, nil
}

func deduplicateSQLRows(schema *SQLSchema, data []map[string]any) []map[string]any {
	cfg := schema.operationConfig.withDefaults()
	switch cfg.ConflictStrategy {
	case ConflictIgnore, ConflictReplace, ConflictUpdate:
	default:
		return data
	}
	if !cfg.DeduplicateByConflictColumns {
		return data
	}
	conflictCols := sqlConflictColumns(schema)
	if len(conflictCols) == 0 || len(data) < 2 {
		return data
	}

	rows := make([]map[string]any, 0, len(data))
	seen := make(map[string]int, len(data))
	for _, row := range data {
		key := sqlConflictKey(row, conflictCols)
		if idx, ok := seen[key]; ok {
			switch cfg.ConflictStrategy {
			case ConflictIgnore:
				continue
			case ConflictReplace:
				rows[idx] = cloneSQLRow(row)
			case ConflictUpdate:
				for _, col := range schema.Columns() {
					if val, exists := row[col]; exists {
						rows[idx][col] = val
					}
				}
			}
			continue
		}
		seen[key] = len(rows)
		rows = append(rows, cloneSQLRow(row))
	}
	return rows
}

func cloneSQLRow(row map[string]any) map[string]any {
	cloned := make(map[string]any, len(row))
	for k, v := range row {
		cloned[k] = v
	}
	return cloned
}

func sqlConflictKey(row map[string]any, conflictCols []string) string {
	var b strings.Builder
	for _, col := range conflictCols {
		val, exists := row[col]
		fmt.Fprintf(&b, "%s=%t:%T:%#v\x00", col, exists, val, val)
	}
	return b.String()
}

func sqlConflictColumns(schema *SQLSchema) []string {
	if len(schema.operationConfig.ConflictColumns) > 0 {
		return append([]string(nil), schema.operationConfig.ConflictColumns...)
	}
	columns := schema.Columns()
	if len(columns) == 0 {
		return nil
	}
	return []string{columns[0]}
}

func sqlConflictColumnSet(schema *SQLSchema) map[string]struct{} {
	conflictCols := sqlConflictColumns(schema)
	set := make(map[string]struct{}, len(conflictCols))
	for _, col := range conflictCols {
		set[col] = struct{}{}
	}
	return set
}

func sqlUpdateColumns(schema *SQLSchema, replace bool) []string {
	conflictSet := sqlConflictColumnSet(schema)
	updateSet := make(map[string]struct{}, len(schema.operationConfig.UpdateColumns))
	if !replace && len(schema.operationConfig.UpdateColumns) > 0 {
		for _, col := range schema.operationConfig.UpdateColumns {
			updateSet[col] = struct{}{}
		}
	}

	columns := schema.Columns()
	out := make([]string, 0, len(columns))
	for _, col := range columns {
		if _, isConflict := conflictSet[col]; isConflict {
			continue
		}
		if !replace && len(updateSet) > 0 {
			if _, ok := updateSet[col]; !ok {
				continue
			}
		}
		out = append(out, col)
	}
	return out
}

func mysqlUpdatePairs(columns []string) []string {
	updatePairs := make([]string, len(columns))
	for i, col := range columns {
		updatePairs[i] = fmt.Sprintf("%s = VALUES(%s)", col, col)
	}
	return updatePairs
}

func postgresUpdatePairs(columns []string) []string {
	updatePairs := make([]string, len(columns))
	for i, col := range columns {
		updatePairs[i] = fmt.Sprintf("%s = EXCLUDED.%s", col, col)
	}
	return updatePairs
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
	rows, args, err := prepareSQLRowsAndArgs(ctx, schema, data)
	if err != nil {
		return "", nil, err
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(rows))

	baseSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)

	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("INSERT IGNORE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictReplace:
		sql := fmt.Sprintf("REPLACE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictUpdate:
		updateColumns := sqlUpdateColumns(schema, false)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict update")
		}
		sql := fmt.Sprintf("%s ON DUPLICATE KEY UPDATE %s", baseSQL, strings.Join(mysqlUpdatePairs(updateColumns), ", "))
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
	rows, args, err := prepareSQLRowsAndArgs(ctx, schema, data)
	if err != nil {
		return "", nil, err
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(rows))

	baseSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)

	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO NOTHING", baseSQL, strings.Join(sqlConflictColumns(schema), ", "))
		return sql, args, nil
	case ConflictReplace:
		updateColumns := sqlUpdateColumns(schema, true)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict replace")
		}
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s", baseSQL, strings.Join(sqlConflictColumns(schema), ", "), strings.Join(postgresUpdatePairs(updateColumns), ", "))
		return sql, args, nil
	case ConflictUpdate:
		updateColumns := sqlUpdateColumns(schema, false)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict update")
		}
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s", baseSQL, strings.Join(sqlConflictColumns(schema), ", "), strings.Join(postgresUpdatePairs(updateColumns), ", "))
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
	rows, args, err := prepareSQLRowsAndArgs(ctx, schema, data)
	if err != nil {
		return "", nil, err
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(rows))

	baseSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)

	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictReplace:
		sql := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES %s", schema.Name(), columnsStr, placeholders)
		return sql, args, nil
	case ConflictUpdate:
		updateColumns := sqlUpdateColumns(schema, false)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict update")
		}
		updatePairs := make([]string, len(updateColumns))
		for i, col := range updateColumns {
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
	rows, args, err := prepareSQLRowsAndArgs(ctx, schema, data)
	if err != nil {
		return "", nil, err
	}

	columnsStr := strings.Join(columns, ", ")
	placeholders := d.generatePlaceholders(len(columns), len(rows))

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
		updateColumns := sqlUpdateColumns(schema, false)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict update")
		}
		sql := fmt.Sprintf("%s ON DUPLICATE KEY UPDATE %s", baseSQL, strings.Join(mysqlUpdatePairs(updateColumns), ", "))
		return sql, args, nil
	default:
		return baseSQL, args, nil
	}
}

func (d *MockDriver) generatePostgreSQLSQL(schema *SQLSchema, baseSQL, _, _ string, args []any) (string, []any, error) {
	switch schema.operationConfig.ConflictStrategy {
	case ConflictIgnore:
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO NOTHING", baseSQL, strings.Join(sqlConflictColumns(schema), ", "))
		return sql, args, nil
	case ConflictReplace:
		updateColumns := sqlUpdateColumns(schema, true)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict replace")
		}
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s", baseSQL, strings.Join(sqlConflictColumns(schema), ", "), strings.Join(postgresUpdatePairs(updateColumns), ", "))
		return sql, args, nil
	case ConflictUpdate:
		updateColumns := sqlUpdateColumns(schema, false)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict update")
		}
		sql := fmt.Sprintf("%s ON CONFLICT (%s) DO UPDATE SET %s", baseSQL, strings.Join(sqlConflictColumns(schema), ", "), strings.Join(postgresUpdatePairs(updateColumns), ", "))
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
		updateColumns := sqlUpdateColumns(schema, false)
		if len(updateColumns) == 0 {
			return "", nil, errors.New("no update columns defined for conflict update")
		}
		updatePairs := make([]string, len(updateColumns))
		for i, col := range updateColumns {
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
