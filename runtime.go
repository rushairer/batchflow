package batchflow

import "time"

// ShardRoutingPolicy controls how a sharded BatchFlow chooses a target shard.
type ShardRoutingPolicy uint8

const (
	ShardRoutingHash ShardRoutingPolicy = iota
	ShardRoutingRoundRobin
	ShardRoutingLeastLoaded
)

// ShardKeyFunc returns routing key for a request.
type ShardKeyFunc func(*Request) uint64

// BackpressureMode controls queue pressure behavior.
type BackpressureMode uint8

const (
	BackpressureBlock BackpressureMode = iota
	BackpressureReject
	BackpressureTimeout
)

// BackpressureConfig controls queue pressure.
type BackpressureConfig struct {
	Enabled       bool
	Mode          BackpressureMode
	HighWatermark int
	CheckInterval time.Duration
	Timeout       time.Duration
}

func (c BackpressureConfig) withDefaults() BackpressureConfig {
	if c.CheckInterval <= 0 {
		c.CheckInterval = time.Millisecond
	}
	if c.Timeout <= 0 {
		c.Timeout = time.Second
	}
	return c
}

// MemoryLimitConfig provides OOM protection for runtime queue.
type MemoryLimitConfig struct {
	Enabled         bool
	MaxQueueBytes   int64
	AvgRequestBytes int64
	Mode            BackpressureMode
	CheckInterval   time.Duration
	Timeout         time.Duration
}

func (c MemoryLimitConfig) withDefaults() MemoryLimitConfig {
	if c.AvgRequestBytes <= 0 {
		c.AvgRequestBytes = 512
	}
	if c.CheckInterval <= 0 {
		c.CheckInterval = time.Millisecond
	}
	if c.Timeout <= 0 {
		c.Timeout = time.Second
	}
	return c
}

// AdaptiveTuningConfig controls runtime auto tuning behavior.
type AdaptiveTuningConfig struct {
	Enabled bool

	MinFlushSize uint32
	MaxFlushSize uint32

	MinFlushInterval time.Duration
	MaxFlushInterval time.Duration

	ScaleUpQueueDepth   int
	ScaleDownQueueDepth int

	TargetLatency time.Duration
}

func (c AdaptiveTuningConfig) withDefaults() AdaptiveTuningConfig {
	if c.MinFlushSize == 0 {
		c.MinFlushSize = 100
	}
	if c.MaxFlushSize == 0 {
		c.MaxFlushSize = 5000
	}
	if c.MinFlushInterval == 0 {
		c.MinFlushInterval = 10 * time.Millisecond
	}
	if c.MaxFlushInterval == 0 {
		c.MaxFlushInterval = 200 * time.Millisecond
	}
	if c.ScaleUpQueueDepth == 0 {
		c.ScaleUpQueueDepth = 8000
	}
	if c.ScaleDownQueueDepth == 0 {
		c.ScaleDownQueueDepth = 1000
	}
	if c.TargetLatency == 0 {
		c.TargetLatency = 50 * time.Millisecond
	}
	return c
}

// RuntimeConfig contains runtime controls separate from pipeline and executor settings.
type RuntimeConfig struct {
	ShardCount   uint32
	Routing      ShardRoutingPolicy
	ShardKeyFunc ShardKeyFunc

	Backpressure BackpressureConfig
	MemoryLimit  MemoryLimitConfig
	Adaptive     AdaptiveTuningConfig
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		ShardCount: 1,
		Routing:    ShardRoutingHash,
	}
}

func (c RuntimeConfig) withDefaults() RuntimeConfig {
	if c.ShardCount == 0 {
		c.ShardCount = 1
	}
	c.Backpressure = c.Backpressure.withDefaults()
	c.MemoryLimit = c.MemoryLimit.withDefaults()
	c.Adaptive = c.Adaptive.withDefaults()
	return c
}

// Config is the stable public constructor config for the v2 module.
type Config struct {
	Pipeline PipelineConfig
	Runtime  RuntimeConfig
	Executor BatchExecutor
}

func DefaultConfig(executor BatchExecutor) Config {
	return Config{
		Pipeline: DefaultPipelineConfig(),
		Runtime:  DefaultRuntimeConfig(),
		Executor: executor,
	}
}
