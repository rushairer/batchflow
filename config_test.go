package batchflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rushairer/batchflow/v2"
)

func TestPipelineConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  batchflow.PipelineConfig
	}{
		{name: "negative concurrency", cfg: batchflow.PipelineConfig{ConcurrencyLimit: -1}},
		{name: "negative timeout", cfg: batchflow.PipelineConfig{Timeout: -time.Millisecond}},
		{name: "negative retry attempts", cfg: batchflow.PipelineConfig{Retry: batchflow.RetryConfig{MaxAttempts: -1}}},
		{name: "negative flush interval", cfg: batchflow.PipelineConfig{FlushInterval: -time.Millisecond}},
		{name: "negative drain grace", cfg: batchflow.PipelineConfig{DrainGracePeriod: -time.Millisecond}},
		{name: "negative final flush timeout", cfg: batchflow.PipelineConfig{FinalFlushOnCloseTimeout: -time.Millisecond}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			var cfgErr *batchflow.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("Validate err=%v, want ConfigError", err)
			}
		})
	}
}

func TestNewBatchFlowWithConfigRejectsNilExecutor(t *testing.T) {
	_, err := batchflow.NewBatchFlowWithConfig(context.Background(), batchflow.DefaultBatchFlowConfig(nil))
	var cfgErr *batchflow.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err=%v, want ConfigError", err)
	}
}

func TestNewBatchFlowWithConfigAppliesDefaults(t *testing.T) {
	bf, err := batchflow.NewBatchFlowWithConfig(context.Background(), batchflow.BatchFlowConfig{
		Executor: batchflow.NewMockExecutor(),
	})
	if err != nil {
		t.Fatalf("NewBatchFlowWithConfig failed: %v", err)
	}
	if err := bf.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
