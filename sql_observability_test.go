package batchflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

func TestGenerateSQLPreview_DryRunIncludesFinalSQLAndDedupStats(t *testing.T) {
	cfg := batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("id", "tenant_id").
		WithUpdateColumns("name")
	schema := batchflow.NewSQLSchema("users", cfg, "id", "tenant_id", "name", "age")

	preview, err := batchflow.GenerateSQLPreview(context.Background(), batchflow.DefaultPostgreSQLDriver, schema, []map[string]any{
		{"id": 1, "tenant_id": 10, "name": "first"},
		{"id": 1, "tenant_id": 10, "age": 30},
	})
	if err != nil {
		t.Fatalf("GenerateSQLPreview error: %v", err)
	}

	if !strings.Contains(preview.SQL, "ON CONFLICT (id, tenant_id) DO UPDATE SET name = EXCLUDED.name") {
		t.Fatalf("preview SQL missing expected conflict/update clause: %s", preview.SQL)
	}
	if preview.ArgsCount != 4 {
		t.Fatalf("ArgsCount = %d, want 4", preview.ArgsCount)
	}
	if preview.DedupStats.InputRows != 2 || preview.DedupStats.OutputRows != 1 || preview.DedupStats.MergedRows != 1 {
		t.Fatalf("unexpected dedup stats: %#v", preview.DedupStats)
	}
	if preview.Fingerprint == "" {
		t.Fatal("Fingerprint is empty")
	}
}

func TestSQLErrorWrapsGenerationMetadata(t *testing.T) {
	cfg := batchflow.ConflictUpdateOperationConfig.WithConflictColumns("id").WithUpdateColumns("id")
	schema := batchflow.NewSQLSchema("users", cfg, "id")

	_, err := batchflow.GenerateSQLPreview(context.Background(), batchflow.DefaultPostgreSQLDriver, schema, []map[string]any{
		{"id": 1},
	})
	if err == nil {
		t.Fatal("expected generation error")
	}
	var sqlErr *batchflow.SQLError
	if !errors.As(err, &sqlErr) {
		t.Fatalf("error type = %T, want *SQLError", err)
	}
	if sqlErr.Stage != batchflow.SQLStageGenerate || sqlErr.Table != "users" || sqlErr.BatchSize != 1 {
		t.Fatalf("unexpected sql error metadata: %#v", sqlErr)
	}
}

type sqlMetricsRecorder struct {
	generated          int
	dedup              int
	errors             int
	operationGenerated int
	operationErrors    int
	lastStage          batchflow.SQLStage
	lastSQLReason      string
	lastOpReason       string
}

func (r *sqlMetricsRecorder) ObserveEnqueueLatency(d time.Duration) {}
func (r *sqlMetricsRecorder) ObserveBatchAssemble(d time.Duration)  {}
func (r *sqlMetricsRecorder) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
}
func (r *sqlMetricsRecorder) ObserveBatchSize(n int)     {}
func (r *sqlMetricsRecorder) IncError(table, typ string) {}
func (r *sqlMetricsRecorder) SetConcurrency(n int)       {}
func (r *sqlMetricsRecorder) SetQueueLength(n int)       {}
func (r *sqlMetricsRecorder) IncInflight()               {}
func (r *sqlMetricsRecorder) DecInflight()               {}
func (r *sqlMetricsRecorder) ObserveSQLGenerated(table string, inputRows, outputRows, argsCount int) {
	r.generated++
}

func (r *sqlMetricsRecorder) ObserveSQLDeduplicated(table string, strategy batchflow.ConflictStrategy, deduplicatedRows, mergedRows int) {
	r.dedup += deduplicatedRows + mergedRows
}

func (r *sqlMetricsRecorder) IncSQLError(table string, stage batchflow.SQLStage, reason string) {
	r.errors++
	r.lastStage = stage
	r.lastSQLReason = reason
}

func (r *sqlMetricsRecorder) ObserveOperationGenerated(preview batchflow.OperationPreview) {
	r.operationGenerated++
}

func (r *sqlMetricsRecorder) IncOperationError(schema string, backend string, stage string, reason string) {
	r.operationErrors++
	r.lastOpReason = reason
}

type sqlMetricsProcessor struct{}

func (sqlMetricsProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	return batchflow.Operations{"INSERT INTO users (id, name) VALUES (?, ?)", 1, "a"}, nil
}

func (sqlMetricsProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	return &batchflow.SQLError{
		Stage:          batchflow.SQLStageExecute,
		Table:          "users",
		BatchSize:      1,
		SQLFingerprint: batchflow.FingerprintSQL(ops[0].(string)),
		ArgsCount:      2,
		Cause:          errors.New("duplicate key"),
	}
}

func TestThrottledExecutorReportsSQLErrorMetrics(t *testing.T) {
	reporter := &sqlMetricsRecorder{}
	exec := batchflow.NewThrottledBatchExecutor(sqlMetricsProcessor{}).WithMetricsReporter(reporter)
	schema := batchflow.NewSQLSchema("users", batchflow.DefaultOperationConfig, "id", "name")

	err := exec.ExecuteBatch(context.Background(), schema, []map[string]any{{"id": 1, "name": "a"}})
	if err == nil {
		t.Fatal("expected execute error")
	}
	if reporter.errors != 1 || reporter.lastStage != batchflow.SQLStageExecute {
		t.Fatalf("unexpected sql error metrics: errors=%d stage=%s", reporter.errors, reporter.lastStage)
	}
	if reporter.lastSQLReason != batchflow.ErrorReasonDuplicateKey || reporter.lastOpReason != batchflow.ErrorReasonDuplicateKey {
		t.Fatalf("unexpected error reasons: sql=%q operation=%q", reporter.lastSQLReason, reporter.lastOpReason)
	}
}

type stringOperationProcessor struct{}

func (stringOperationProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	return batchflow.Operations{"POST /v1/events", "body"}, nil
}

func (stringOperationProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	return nil
}

func TestThrottledExecutor_DoesNotReportSQLMetricsForCustomStringOperation(t *testing.T) {
	reporter := &sqlMetricsRecorder{}
	exec := batchflow.NewThrottledBatchExecutor(stringOperationProcessor{}).WithMetricsReporter(reporter)
	schema := batchflow.NewSchema("api_events", "id")

	if err := exec.ExecuteBatch(context.Background(), schema, []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if reporter.operationGenerated != 1 {
		t.Fatalf("operation generated count = %d, want 1", reporter.operationGenerated)
	}
	if reporter.generated != 0 || reporter.dedup != 0 || reporter.errors != 0 {
		t.Fatalf("custom operation polluted SQL metrics: generated=%d dedup=%d errors=%d", reporter.generated, reporter.dedup, reporter.errors)
	}
}
