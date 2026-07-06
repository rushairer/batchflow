package batchflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrCopyFromUnsupportedOperation = errors.New("copy from supports append-only SQL schemas only")

// CopyFromClient is the minimal adapter surface required by CopyFromExecutor.
// It intentionally avoids importing pgx in the root module. A pgxpool.Pool can
// be adapted with a small wrapper that calls pgx.CopyFromRows(rows).
type CopyFromClient interface {
	CopyFrom(ctx context.Context, table string, columns []string, rows [][]any) (int64, error)
}

// CopyFromExecutor is a zero-SQL-generation backend for append-only database
// ingestion. It is designed for Hologres/PostgreSQL COPY FROM fast paths while
// keeping the core module free from mandatory pgx dependencies.
type CopyFromExecutor struct {
	client  CopyFromClient
	timeout time.Duration
}

var _ BatchExecutor = (*CopyFromExecutor)(nil)

func NewCopyFromExecutor(client CopyFromClient) *CopyFromExecutor {
	return &CopyFromExecutor{client: client}
}

func (e *CopyFromExecutor) WithTimeout(timeout time.Duration) *CopyFromExecutor {
	e.timeout = timeout
	return e
}

func (e *CopyFromExecutor) ExecuteBatch(ctx context.Context, schema SchemaInterface, data []map[string]any) error {
	if e == nil || e.client == nil {
		return errors.New("copy from client must not be nil")
	}
	if len(data) == 0 {
		return nil
	}
	if err := validateCopyFromSchema(schema); err != nil {
		return err
	}
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	columns := schema.Columns()
	rows := buildCopyRows(ctx, columns, data)
	if err := ctx.Err(); err != nil {
		return err
	}

	written, err := e.client.CopyFrom(ctx, schema.Name(), columns, rows)
	if err != nil {
		return err
	}
	if written != int64(len(rows)) {
		return fmt.Errorf("copy from wrote %d rows, want %d", written, len(rows))
	}
	return nil
}

func validateCopyFromSchema(schema SchemaInterface) error {
	if schema == nil {
		return ErrInvalidSchema
	}
	if len(schema.Name()) == 0 {
		return ErrEmptySchemaName
	}
	if len(schema.Columns()) == 0 {
		return ErrMissingColumn
	}
	if sqlSchema, ok := schema.(*SQLSchema); ok {
		cfg := sqlSchema.operationConfig.withDefaults()
		if cfg.ConflictStrategy != ConflictIgnore || len(cfg.ConflictColumns) > 0 || len(cfg.UpdateColumns) > 0 {
			return ErrCopyFromUnsupportedOperation
		}
	}
	return nil
}

func buildCopyRows(ctx context.Context, columns []string, data []map[string]any) [][]any {
	rows := make([][]any, len(data))
	for i, row := range data {
		if i&1023 == 0 && ctx.Err() != nil {
			return rows[:i]
		}
		values := make([]any, len(columns))
		for j, col := range columns {
			values[j] = row[col]
		}
		rows[i] = values
	}
	return rows
}
