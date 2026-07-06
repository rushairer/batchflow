package batchflow

type SchemaInterface interface {
	Name() string
	Columns() []string
}

// ConflictStrategy 冲突处理策略
type ConflictStrategy uint8

const (
	ConflictIgnore ConflictStrategy = iota
	ConflictReplace
	ConflictUpdate
)

// 操作配置
type SQLOperationConfig struct {
	ConflictStrategy ConflictStrategy
	// ConflictColumns defines the conflict target used by upsert-style writes.
	// When empty, PostgreSQL/Hologres ConflictIgnore falls back to plain INSERT.
	ConflictColumns []string
	// UpdateColumns limits columns updated by ConflictUpdate. When empty,
	// drivers update all non-conflict columns.
	UpdateColumns []string
	// DeduplicateByConflictColumns controls client-side merge of duplicate
	// conflict keys before generating SQL. The default is false for maximum
	// append-write throughput; enable it explicitly when upsert batches may
	// contain duplicate conflict keys.
	DeduplicateByConflictColumns bool
	deduplicateConfigured        bool
}

// Schema 表结构定义
type Schema struct {
	name    string
	columns []string
}

// NewSchema 创建新的Schema实例
func NewSchema(
	name string,
	columns ...string,
) *Schema {
	return &Schema{
		name:    name,
		columns: columns,
	}
}

func (s *Schema) Name() string {
	return s.name
}

func (s *Schema) Columns() []string {
	return s.columns
}

type SQLSchema struct {
	*Schema
	operationConfig SQLOperationConfig
}

func NewSQLSchema(name string, operationConfig SQLOperationConfig, columns ...string) *SQLSchema {
	operationConfig = operationConfig.withDefaults()
	return &SQLSchema{
		Schema:          NewSchema(name, columns...),
		operationConfig: operationConfig,
	}
}

func (s *SQLSchema) OperationConfig() any {
	return s.operationConfig
}

func (c SQLOperationConfig) withDefaults() SQLOperationConfig {
	if !c.deduplicateConfigured {
		c.DeduplicateByConflictColumns = false
	}
	return c
}

func (c SQLOperationConfig) WithConflictColumns(cols ...string) SQLOperationConfig {
	c.ConflictColumns = append([]string(nil), cols...)
	return c.withDefaults()
}

func (c SQLOperationConfig) WithUpdateColumns(cols ...string) SQLOperationConfig {
	c.UpdateColumns = append([]string(nil), cols...)
	return c.withDefaults()
}

func (c SQLOperationConfig) WithDeduplicateByConflictColumns(enabled bool) SQLOperationConfig {
	c.DeduplicateByConflictColumns = enabled
	c.deduplicateConfigured = true
	return c
}

var DefaultOperationConfig = SQLOperationConfig{
	ConflictStrategy: ConflictIgnore,
}

var ConflictIgnoreOperationConfig = SQLOperationConfig{
	ConflictStrategy: ConflictIgnore,
}

var ConflictReplaceOperationConfig = SQLOperationConfig{
	ConflictStrategy: ConflictReplace,
}

var ConflictUpdateOperationConfig = SQLOperationConfig{
	ConflictStrategy: ConflictUpdate,
}
