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
	// 其他操作相关配置...
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
	return &SQLSchema{
		Schema:          NewSchema(name, columns...),
		operationConfig: operationConfig,
	}
}

func (s *SQLSchema) OperationConfig() any {
	return s.operationConfig
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
