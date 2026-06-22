package batchflow_test

import (
	"context"
	"testing"

	"github.com/rushairer/batchflow"
)

func TestPublicAPIContract(t *testing.T) {
	var _ batchflow.BatchExecutor = batchflow.NewMockExecutor()
	var _ batchflow.Coalescer = batchflow.CoalescerFunc(nil)
	var _ batchflow.ErrorClassifier = batchflow.ErrorClassifierFunc(nil)

	_ = batchflow.Record{"id": 1}
	_ = batchflow.Batch{{"id": 1}}
	_ = batchflow.DefaultPipelineConfig()
	_ = batchflow.DefaultBatchFlowConfig(batchflow.NewMockExecutor())
	_ = batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "id")
	unregister := batchflow.RegisterErrorClassifier(batchflow.ErrorClassifierFunc(func(error) (bool, string, bool) {
		return false, "", false
	}))
	defer unregister()

	flow, err := batchflow.NewBatchFlowWithConfig(context.Background(), batchflow.BatchFlowConfig{
		Executor: batchflow.NewMockExecutor(),
	})
	if err != nil {
		t.Fatalf("NewBatchFlowWithConfig failed: %v", err)
	}
	if err := flow.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
