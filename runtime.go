package batchflow

import "time"

// ShardRoutingPolicy controls how a sharded BatchFlow chooses a target shard.
type ShardRoutingPolicy uint8

const (
	// ShardRoutingHash keeps rows with the same ShardKeyFunc value on the same shard.
	ShardRoutingHash ShardRoutingPolicy = iota
	// ShardRoutingRoundRobin spreads writes evenly without key affinity.
	ShardRoutingRoundRobin
	// ShardRoutingLeastLoaded chooses the shard with the shortest current queue.
	ShardRoutingLeastLoaded
)

// ShardKeyFunc returns a stable routing key for a request.
type ShardKeyFunc func(*Request) uint64

// BackpressureMode controls what Submit does when a target queue is above the high watermark.
type BackpressureMode uint8

const (
	// BackpressureBlock waits until the target shard falls below the high watermark.
	BackpressureBlock BackpressureMode = iota
	// BackpressureReject fails Submit immediately with ErrBackpressure.
	BackpressureReject
	// BackpressureTimeout waits up to Timeout, then fails with ErrBackpressure.
	BackpressureTimeout
)

// BackpressureConfig protects the process from unbounded queue growth under downstream pressure.
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

// RuntimeConfig contains the v3-style runtime concerns that are intentionally
// separated from database driver and executor configuration.
type RuntimeConfig struct {
	ShardCount  uint32
	Routing     ShardRoutingPolicy
	ShardKeyFunc ShardKeyFunc
	Backpressure BackpressureConfig
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{ShardCount: 1, Routing: ShardRoutingHash}
}

func (c RuntimeConfig) withDefaults() RuntimeConfig {
	if c.ShardCount == 0 {
		c.ShardCount = 1
	}
	c.Backpressure = c.Backpressure.withDefaults()
	return c
}

// V3Config is a stable, explicit constructor surface for the converged runtime.
// It is additive and keeps the v2 module path/API compatible during migration.
type V3Config struct {
	Pipeline PipelineConfig
	Runtime  RuntimeConfig
	Executor BatchExecutor
}

func DefaultV3Config(executor BatchExecutor) V3Config {
	return V3Config{
		Pipeline: DefaultPipelineConfig(),
		Runtime:  DefaultRuntimeConfig(),
		Executor: executor,
	}
}
