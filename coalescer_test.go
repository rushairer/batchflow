package batchflow_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/rushairer/batchflow/v2"
)

func TestKeyCoalescerStrategies(t *testing.T) {
	schema := batchflow.NewSchema("users", "id", "name", "email")
	input := batchflow.Batch{
		{"id": 1, "name": "first", "email": "first@example.com"},
		{"id": 1, "name": "second"},
		{"id": 2, "name": "other", "email": "other@example.com"},
	}

	tests := []struct {
		name     string
		strategy batchflow.CoalesceStrategy
		want     batchflow.Batch
		dedup    int
		merged   int
	}{
		{
			name:     "keep first",
			strategy: batchflow.CoalesceKeepFirst,
			want: batchflow.Batch{
				{"id": 1, "name": "first", "email": "first@example.com"},
				{"id": 2, "name": "other", "email": "other@example.com"},
			},
			dedup: 1,
		},
		{
			name:     "keep last",
			strategy: batchflow.CoalesceKeepLast,
			want: batchflow.Batch{
				{"id": 1, "name": "second"},
				{"id": 2, "name": "other", "email": "other@example.com"},
			},
			dedup: 1,
		},
		{
			name:     "merge present fields",
			strategy: batchflow.CoalesceMergePresentFields,
			want: batchflow.Batch{
				{"id": 1, "name": "second", "email": "first@example.com"},
				{"id": 2, "name": "other", "email": "other@example.com"},
			},
			merged: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := batchflow.NewKeyCoalescer(tt.strategy, "id").Coalesce(context.Background(), schema, input)
			if err != nil {
				t.Fatalf("coalesce failed: %v", err)
			}
			if !reflect.DeepEqual(result.Batch, tt.want) {
				t.Fatalf("batch=%#v, want %#v", result.Batch, tt.want)
			}
			if result.InputItems != 3 || result.OutputItems != 2 {
				t.Fatalf("stats input/output=%d/%d, want 3/2", result.InputItems, result.OutputItems)
			}
			if result.DeduplicatedItems != tt.dedup || result.MergedItems != tt.merged {
				t.Fatalf("stats dedup/merged=%d/%d, want %d/%d", result.DeduplicatedItems, result.MergedItems, tt.dedup, tt.merged)
			}
		})
	}
}

func TestExecutorCoalescerAppliesBeforeProcessor(t *testing.T) {
	processor := &captureProcessor{}
	executor := batchflow.NewThrottledBatchExecutor(processor).
		WithCoalescer(batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "id"))

	schema := batchflow.NewSchema("users", "id", "name")
	err := executor.ExecuteBatch(context.Background(), schema, batchflow.Batch{
		{"id": 1, "name": "first"},
		{"id": 1, "name": "last"},
		{"id": 2, "name": "other"},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch failed: %v", err)
	}
	want := batchflow.Batch{
		{"id": 1, "name": "last"},
		{"id": 2, "name": "other"},
	}
	if !reflect.DeepEqual(processor.generated, want) {
		t.Fatalf("processor batch=%#v, want %#v", processor.generated, want)
	}
}

type captureProcessor struct {
	generated batchflow.Batch
}

func (p *captureProcessor) GenerateOperations(_ context.Context, _ batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	p.generated = append(batchflow.Batch(nil), data...)
	return batchflow.Operations{"ok"}, nil
}

func (p *captureProcessor) ExecuteOperations(context.Context, batchflow.Operations) error {
	return nil
}
