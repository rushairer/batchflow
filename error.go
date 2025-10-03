package batchflow

import "errors"

var (
	// ErrEmptyRequest 空请求错误
	ErrEmptyRequest = errors.New("empty request")

	// ErrContextCanceled 上下文被取消错误
	ErrContextCanceled = errors.New("context canceled")

	// ErrInvalidSchema 无效的 schema 错误
	ErrInvalidSchema = errors.New("invalid schema")

	// ErrMissingColumn 缺少列错误
	ErrMissingColumn = errors.New("missing required column")

	// ErrInvalidColumnType 无效的列类型错误
	ErrInvalidColumnType = errors.New("invalid column type")

	// ErrEmptyBatch 空批次错误
	ErrEmptyBatch = errors.New("empty batch")

	// ErrEmptySchemaName 空表名错误
	ErrEmptySchemaName = errors.New("empty schema name")
)
