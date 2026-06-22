package batchflow

import (
	"context"
	"errors"
	"fmt"
)

// SQLDedupStats describes client-side duplicate conflict-key handling before SQL generation.
type SQLDedupStats struct {
	InputRows        int
	OutputRows       int
	DeduplicatedRows int
	MergedRows       int
}

// SQLPreview is a dry-run view of the final SQL operation that would be executed.
//
// Args contains raw values and may include sensitive data. Prefer ArgsCount and
// Fingerprint for logs and metrics unless the caller explicitly needs full inspection.
type SQLPreview struct {
	Table            string
	SQL              string
	Args             []any
	ArgsCount        int
	Columns          []string
	ConflictStrategy ConflictStrategy
	ConflictColumns  []string
	UpdateColumns    []string
	DedupStats       SQLDedupStats
	Fingerprint      string
}

// SQLStage identifies where a SQL batch failed.
type SQLStage string

const (
	SQLStageValidate SQLStage = "validate"
	SQLStageGenerate SQLStage = "generate"
	SQLStageExecute  SQLStage = "execute"
)

// SQLError wraps SQL batch failures with safe diagnostic metadata.
type SQLError struct {
	Stage            SQLStage
	Table            string
	BatchSize        int
	ConflictStrategy ConflictStrategy
	ConflictColumns  []string
	UpdateColumns    []string
	SQLFingerprint   string
	ArgsCount        int
	Cause            error
}

func (e *SQLError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("sql %s failed: table=%s batch_size=%d conflict_strategy=%d conflict_columns=%v update_columns=%v fingerprint=%s args=%d: %v",
		e.Stage, e.Table, e.BatchSize, e.ConflictStrategy, e.ConflictColumns, e.UpdateColumns, e.SQLFingerprint, e.ArgsCount, e.Cause)
}

func (e *SQLError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// SQLMetricsReporter is an optional SQL-only detail extension kept for SQL diagnostics
// and backward-compatible Prometheus examples. Backend-neutral integrations should
// prefer OperationMetricsReporter. Implementations must keep labels low-cardinality;
// do not use raw SQL as a label.
type SQLMetricsReporter interface {
	ObserveSQLGenerated(table string, inputRows, outputRows, argsCount int)
	ObserveSQLDeduplicated(table string, strategy ConflictStrategy, deduplicatedRows, mergedRows int)
	IncSQLError(table string, stage SQLStage, reason string)
}

// GenerateSQLPreview builds the same final SQL and args as SQLBatchProcessor without executing it.
func GenerateSQLPreview(ctx context.Context, driver SQLDriver, schema *SQLSchema, data []map[string]any) (SQLPreview, error) {
	if driver == nil {
		return SQLPreview{}, &SQLError{Stage: SQLStageValidate, BatchSize: len(data), Cause: errors.New("nil SQL driver")}
	}
	if schema == nil {
		return SQLPreview{}, &SQLError{Stage: SQLStageValidate, BatchSize: len(data), Cause: errors.New("nil SQL schema")}
	}

	stats := analyzeSQLDedup(schema, data)
	sqlText, args, err := driver.GenerateInsertSQL(ctx, schema, data)
	preview := SQLPreview{
		Table:            schema.Name(),
		SQL:              sqlText,
		Args:             append([]any(nil), args...),
		ArgsCount:        len(args),
		Columns:          append([]string(nil), schema.Columns()...),
		ConflictStrategy: schema.operationConfig.withDefaults().ConflictStrategy,
		ConflictColumns:  sqlConflictColumns(schema),
		UpdateColumns:    sqlUpdateColumnsForPreview(schema),
		DedupStats:       stats,
		Fingerprint:      FingerprintSQL(sqlText),
	}
	if err != nil {
		return preview, &SQLError{
			Stage:            SQLStageGenerate,
			Table:            schema.Name(),
			BatchSize:        len(data),
			ConflictStrategy: preview.ConflictStrategy,
			ConflictColumns:  preview.ConflictColumns,
			UpdateColumns:    preview.UpdateColumns,
			SQLFingerprint:   preview.Fingerprint,
			ArgsCount:        preview.ArgsCount,
			Cause:            err,
		}
	}
	return preview, nil
}

func FingerprintSQL(sqlText string) string {
	return FingerprintText(sqlText)
}

func sqlUpdateColumnsForPreview(schema *SQLSchema) []string {
	cfg := schema.operationConfig.withDefaults()
	switch cfg.ConflictStrategy {
	case ConflictReplace:
		return sqlUpdateColumns(schema, true)
	case ConflictUpdate:
		return sqlUpdateColumns(schema, false)
	default:
		return nil
	}
}

func analyzeSQLDedup(schema *SQLSchema, data []map[string]any) SQLDedupStats {
	_, stats := deduplicateSQLRowsWithStats(schema, data)
	return stats
}

func (p SQLPreview) OperationPreview() OperationPreview {
	operation := OperationInsert
	switch p.ConflictStrategy {
	case ConflictIgnore, ConflictReplace, ConflictUpdate:
		operation = OperationUpsert
	}
	return OperationPreview{
		Backend:     BackendSQL,
		Operation:   operation,
		Schema:      p.Table,
		InputItems:  p.DedupStats.InputRows,
		OutputItems: p.DedupStats.OutputRows,
		ArgCount:    p.ArgsCount,
		Fingerprint: p.Fingerprint,
		Attributes: map[string]any{
			"conflict_strategy":       conflictStrategyName(p.ConflictStrategy),
			"conflict_columns_count":  len(p.ConflictColumns),
			"update_columns_count":    len(p.UpdateColumns),
			"deduplicated_rows":       p.DedupStats.DeduplicatedRows,
			"merged_rows":             p.DedupStats.MergedRows,
			"deduplicate_output_rows": p.DedupStats.OutputRows,
		},
	}
}

func conflictStrategyName(strategy ConflictStrategy) string {
	switch strategy {
	case ConflictIgnore:
		return "ignore"
	case ConflictReplace:
		return "replace"
	case ConflictUpdate:
		return "update"
	default:
		return "unknown"
	}
}
