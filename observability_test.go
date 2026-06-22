package batchflow_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/rushairer/batchflow/v2"
)

type eventRecorder struct {
	events []batchflow.BatchEvent
}

func (r *eventRecorder) OnBatchEvent(ctx context.Context, event batchflow.BatchEvent) {
	r.events = append(r.events, event)
}

type previewProcessor struct{}

func (previewProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	return batchflow.Operations{"op"}, nil
}

func (previewProcessor) GenerateOperationPreview(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, batchflow.OperationPreview, error) {
	return batchflow.Operations{"op"}, batchflow.OperationPreview{
		Backend:     "http",
		Operation:   "post",
		Schema:      schema.Name(),
		InputItems:  len(data),
		OutputItems: 1,
		ArgCount:    1,
		Fingerprint: "abc123",
		Attributes:  map[string]any{"token": "secret-token", "route": "/v1/items"},
	}, nil
}

func (previewProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	return nil
}

func TestThrottledExecutor_ObserverReceivesCustomPreviewEvent(t *testing.T) {
	recorder := &eventRecorder{}
	exec := batchflow.NewThrottledBatchExecutor(previewProcessor{}).WithObserver(recorder)
	schema := batchflow.NewSchema("api_events", "id")

	if err := exec.ExecuteBatch(context.Background(), schema, []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Backend != "http" || event.Operation != "post" || event.Fingerprint != "abc123" || event.Status != "success" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestSlogObserver_RedactsAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	observer := batchflow.NewSlogObserver(logger, batchflow.NewRatioSampler(1), batchflow.DefaultRedactor(), 0)

	observer.OnBatchEvent(context.Background(), batchflow.BatchEvent{
		Stage:      batchflow.BatchStageExecute,
		Backend:    "http",
		Operation:  "post",
		Schema:     "api_events",
		Status:     "success",
		BatchSize:  1,
		Attributes: map[string]any{"token": "secret-token", "route": "/v1/items"},
	})

	out := buf.String()
	if strings.Contains(out, "secret-token") {
		t.Fatalf("log output leaked token: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") || !strings.Contains(out, "/v1/items") {
		t.Fatalf("log output missing redacted or safe attributes: %s", out)
	}
}

type failingPreviewProcessor struct{}

func (failingPreviewProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	return nil, errors.New("unused")
}

func (failingPreviewProcessor) GenerateOperationPreview(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, batchflow.OperationPreview, error) {
	return nil, batchflow.OperationPreview{
		Backend:     "http",
		Operation:   "post",
		Schema:      schema.Name(),
		InputItems:  len(data),
		Fingerprint: "fail123",
	}, errors.New("upstream unavailable")
}

func (failingPreviewProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	return nil
}

func TestThrottledExecutor_WrapsGenericBatchError(t *testing.T) {
	recorder := &eventRecorder{}
	exec := batchflow.NewThrottledBatchExecutor(failingPreviewProcessor{}).WithObserver(recorder)
	schema := batchflow.NewSchema("api_events", "id")

	err := exec.ExecuteBatch(context.Background(), schema, []map[string]any{{"id": 1}})
	if err == nil {
		t.Fatal("expected error")
	}
	var batchErr *batchflow.BatchError
	if !errors.As(err, &batchErr) {
		t.Fatalf("error type = %T, want *BatchError", err)
	}
	if batchErr.Stage != batchflow.BatchStageGenerate || batchErr.Backend != "http" || batchErr.Fingerprint != "fail123" {
		t.Fatalf("unexpected batch error: %#v", batchErr)
	}
	if len(recorder.events) == 0 || recorder.events[0].Stage != batchflow.BatchStageGenerate {
		t.Fatalf("missing generate failure event: %#v", recorder.events)
	}
}

func TestErrorAndSlowSampler(t *testing.T) {
	sampler := batchflow.NewErrorAndSlowSampler(10 * time.Millisecond)
	if !sampler.ShouldSample(batchflow.BatchEvent{Err: errors.New("boom")}) {
		t.Fatal("error event should be sampled")
	}
	if !sampler.ShouldSample(batchflow.BatchEvent{Duration: 20 * time.Millisecond}) {
		t.Fatal("slow event should be sampled")
	}
	if sampler.ShouldSample(batchflow.BatchEvent{Duration: time.Millisecond}) {
		t.Fatal("fast success event should not be sampled")
	}
}

func TestBatchErrorDoesNotPrintAttributeValues(t *testing.T) {
	err := &batchflow.BatchError{
		Stage:       batchflow.BatchStageExecute,
		Backend:     batchflow.BackendCustom,
		Schema:      "api_events",
		BatchSize:   1,
		Fingerprint: "abc123",
		Attributes: map[string]any{
			"token": "secret-token",
			"route": "/v1/items",
		},
		Cause: errors.New("upstream failed"),
	}
	out := err.Error()
	if strings.Contains(out, "secret-token") || strings.Contains(out, "/v1/items") {
		t.Fatalf("BatchError leaked attribute value: %s", out)
	}
	if !strings.Contains(out, "token") || !strings.Contains(out, "route") {
		t.Fatalf("BatchError should include attribute keys: %s", out)
	}
}
