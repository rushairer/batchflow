package batchflow

import "time"

// TuningSignal is runtime feedback from engine.
type TuningSignal struct {
	LatencyAvg    time.Duration
	QueueDepth    int
	ErrorRate     float64
	ThroughputRPS float64
}

// TuningRecommendation is output of adaptive tuner.
type TuningRecommendation struct {
	FlushSize      uint32
	FlushInterval  time.Duration
	BackpressureHW int
	ShardCount     uint32
}

// AdaptiveTuner is a stateless policy engine.
type AdaptiveTuner struct {
	cfg AdaptiveTuningConfig
}

func NewAdaptiveTuner(cfg AdaptiveTuningConfig) *AdaptiveTuner {
	return &AdaptiveTuner{cfg: cfg.withDefaults()}
}

func (t *AdaptiveTuner) Tune(s TuningSignal) TuningRecommendation {
	cfg := t.cfg

	flush := cfg.MinFlushSize
	interval := cfg.MinFlushInterval

	// latency-based tuning
	if s.LatencyAvg > cfg.TargetLatency {
		flush = minU32(cfg.MaxFlushSize, flush*2)
		interval = minDur(cfg.MaxFlushInterval, interval*2)
	} else {
		flush = maxU32(cfg.MinFlushSize, flush/2)
		interval = maxDur(cfg.MinFlushInterval, interval/2)
	}

	// queue pressure tuning
	hw := cfg.ScaleUpQueueDepth
	if s.QueueDepth > cfg.ScaleUpQueueDepth {
		hw = cfg.ScaleUpQueueDepth * 2
	}

	return TuningRecommendation{
		FlushSize:      flush,
		FlushInterval:  interval,
		BackpressureHW: hw,
		ShardCount:     0,
	}
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func maxU32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDur(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
