package batchflow_test

import (
	"testing"
	"time"

	"github.com/rushairer/batchflow/v2"
)

func TestAdaptiveTuner_Defaults(t *testing.T) {
	tuner := batchflow.NewAdaptiveTuner(batchflow.AdaptiveTuningConfig{})
	rec := tuner.Tune(batchflow.TuningSignal{})

	// 零信号 = 低延迟（0 < 默认 TargetLatency 50ms）→ flush/interval 回落并钳制在下限
	if rec.FlushSize != 100 {
		t.Fatalf("expected default min flush size 100, got %d", rec.FlushSize)
	}
	if rec.FlushInterval != 10*time.Millisecond {
		t.Fatalf("expected default min flush interval 10ms, got %v", rec.FlushInterval)
	}
	// 队列无压力 → HW 保持 ScaleUpQueueDepth 默认值
	if rec.BackpressureHW != 8000 {
		t.Fatalf("expected default scale-up depth 8000, got %d", rec.BackpressureHW)
	}
}

func TestAdaptiveTuner_HighLatency(t *testing.T) {
	tuner := batchflow.NewAdaptiveTuner(batchflow.AdaptiveTuningConfig{
		MinFlushSize:     100,
		MaxFlushSize:     1000,
		MinFlushInterval: 10 * time.Millisecond,
		MaxFlushInterval: 100 * time.Millisecond,
		TargetLatency:    50 * time.Millisecond,
	})
	rec := tuner.Tune(batchflow.TuningSignal{LatencyAvg: 100 * time.Millisecond})

	if rec.FlushSize != 200 {
		t.Fatalf("expected flush size 200 (doubled), got %d", rec.FlushSize)
	}
	if rec.FlushInterval != 20*time.Millisecond {
		t.Fatalf("expected flush interval 20ms (doubled), got %v", rec.FlushInterval)
	}
}

func TestAdaptiveTuner_LowLatency_ClampedToMin(t *testing.T) {
	tuner := batchflow.NewAdaptiveTuner(batchflow.AdaptiveTuningConfig{
		MinFlushSize:     100,
		MaxFlushSize:     1000,
		MinFlushInterval: 10 * time.Millisecond,
		MaxFlushInterval: 100 * time.Millisecond,
		TargetLatency:    50 * time.Millisecond,
	})
	// 连续低延迟信号：即使反复减半也不低于下限
	rec := tuner.Tune(batchflow.TuningSignal{LatencyAvg: time.Millisecond})
	rec = tuner.Tune(batchflow.TuningSignal{LatencyAvg: time.Millisecond})
	if rec.FlushSize != 100 {
		t.Fatalf("expected flush size clamped to 100, got %d", rec.FlushSize)
	}
	if rec.FlushInterval != 10*time.Millisecond {
		t.Fatalf("expected flush interval clamped to 10ms, got %v", rec.FlushInterval)
	}
}

func TestAdaptiveTuner_HighLatency_CappedAtMax(t *testing.T) {
	tuner := batchflow.NewAdaptiveTuner(batchflow.AdaptiveTuningConfig{
		MinFlushSize:     100,
		MaxFlushSize:     1000,
		MinFlushInterval: 10 * time.Millisecond,
		MaxFlushInterval: 100 * time.Millisecond,
		TargetLatency:    50 * time.Millisecond,
	})
	var rec batchflow.TuningRecommendation
	for i := 0; i < 10; i++ {
		rec = tuner.Tune(batchflow.TuningSignal{LatencyAvg: 500 * time.Millisecond})
	}
	if rec.FlushSize > 1000 {
		t.Fatalf("expected flush size capped at 1000, got %d", rec.FlushSize)
	}
	if rec.FlushInterval > 100*time.Millisecond {
		t.Fatalf("expected flush interval capped at 100ms, got %v", rec.FlushInterval)
	}
}

func TestAdaptiveTuner_QueuePressure(t *testing.T) {
	tuner := batchflow.NewAdaptiveTuner(batchflow.AdaptiveTuningConfig{
		ScaleUpQueueDepth: 500,
	})

	noPressure := tuner.Tune(batchflow.TuningSignal{QueueDepth: 100})
	if noPressure.BackpressureHW != 500 {
		t.Fatalf("expected HW 500 without pressure, got %d", noPressure.BackpressureHW)
	}

	withPressure := tuner.Tune(batchflow.TuningSignal{QueueDepth: 600})
	if withPressure.BackpressureHW != 1000 {
		t.Fatalf("expected HW 1000 (doubled) with pressure, got %d", withPressure.BackpressureHW)
	}
}

func TestAdaptiveTuner_ShardCountUnchanged(t *testing.T) {
	tuner := batchflow.NewAdaptiveTuner(batchflow.AdaptiveTuningConfig{})
	rec := tuner.Tune(batchflow.TuningSignal{LatencyAvg: 200 * time.Millisecond})
	if rec.ShardCount != 0 {
		t.Fatalf("expected ShardCount 0 (auto), got %d", rec.ShardCount)
	}
}
