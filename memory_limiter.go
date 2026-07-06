package batchflow

import (
	"context"
	"errors"
	"time"
)

var ErrMemoryLimitExceeded = errors.New("batchflow memory limit exceeded")

// MemoryLimitConfig guards the runtime against unbounded queued-memory growth.
// It is intentionally estimation-based: the hot path should not walk every
// queued request or allocate just to measure memory.
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

func (e *RuntimeEngine) waitMemoryLimit(ctx context.Context) error {
	limit := e.cfg.MemoryLimit.withDefaults()
	if !limit.Enabled || limit.MaxQueueBytes <= 0 {
		return nil
	}
	if e.estimatedQueuedBytes(limit) < limit.MaxQueueBytes {
		return nil
	}

	switch limit.Mode {
	case BackpressureReject:
		return ErrMemoryLimitExceeded
	case BackpressureTimeout:
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit.Timeout)
		defer cancel()
	}

	ticker := time.NewTicker(limit.CheckInterval)
	defer ticker.Stop()
	for {
		if e.estimatedQueuedBytes(limit) < limit.MaxQueueBytes {
			return nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrMemoryLimitExceeded
			}
			return ctx.Err()
		case <-e.Done():
			return context.Canceled
		case <-ticker.C:
		}
	}
}

func (e *RuntimeEngine) estimatedQueuedBytes(limit MemoryLimitConfig) int64 {
	if e == nil {
		return 0
	}
	var queued int64
	for i := range e.shards {
		queued += int64(e.queueDepth(i))
	}
	return queued * limit.AvgRequestBytes
}
