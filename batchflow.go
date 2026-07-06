package batchflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"

	redisV9 "github.com/redis/go-redis/v9"
	gopipeline "github.com/rushairer/go-pipeline/v2"
)

// BatchFlow supports optional sharded execution for high throughput workloads.
// If ShardCount > 1, requests are routed across independent pipelines.
type BatchFlow struct {
	pipeline        *gopipeline.StandardPipeline[*queuedRequest]
	executor        BatchExecutor
	metricsReporter MetricsReporter

	closed    atomic.Bool
	closeOnce sync.Once
	done      chan struct{}

	runErrMu sync.RWMutex
	runErr   error

	// sharding
	shards        []*BatchFlow
	shardKeyFunc  func(*Request) uint64
	shardSeq      atomic.Uint64
}

type queuedRequest struct {
	request    *Request
	enqueuedAt time.Time
}

// PipelineConfig governs batch behavior.
type PipelineConfig struct {
	BufferSize               uint32
	FlushSize                uint32
	FlushInterval            time.Duration
	MaxConcurrentFlushes     uint32
	DrainOnCancel            bool
	DrainGracePeriod         time.Duration
	FinalFlushOnCloseTimeout time.Duration

	Retry       RetryConfig
	Timeout     time.Duration
	MetricsReporter MetricsReporter
	Observability   ObservabilityConfig
	ConcurrencyLimit int
	Coalescer        Coalescer

	// sharding controls (production-grade routing)
	ShardCount   uint32
	ShardKeyFunc  func(*Request) uint64
}

func NewBatchFlow(ctx context.Context, buffSize uint32, flushSize uint32, flushInterval time.Duration, executor BatchExecutor) *BatchFlow {
	return newBatchFlow(ctx, PipelineConfig{
		BufferSize:    buffSize,
		FlushSize:     flushSize,
		FlushInterval: flushInterval,
		DrainOnCancel: true,
		DrainGracePeriod: 2 * time.Second,
	}, executor)
}

func newBatchFlow(ctx context.Context, config PipelineConfig, executor BatchExecutor) *BatchFlow {
	config = config.withDefaults()

	reporter := metricsReporterFromExecutor(executor)

	bf := &BatchFlow{
		executor:        executor,
		metricsReporter: reporter,
		done:            make(chan struct{}),
		shardKeyFunc:    config.ShardKeyFunc,
	}

	// sharding mode
	if config.ShardCount > 1 {
		bf.shards = make([]*BatchFlow, 0, config.ShardCount)
		for i := uint32(0); i < config.ShardCount; i++ {
			bf.shards = append(bf.shards, newBatchFlow(ctx, PipelineConfig{
				BufferSize:       config.BufferSize,
				FlushSize:        config.FlushSize,
				FlushInterval:    config.FlushInterval,
				DrainOnCancel:    config.DrainOnCancel,
				DrainGracePeriod: config.DrainGracePeriod,
				Retry:            config.Retry,
				Timeout:          config.Timeout,
				MetricsReporter:  config.MetricsReporter,
				Observability:    config.Observability,
			}, executor))
		}

		go func() {
			var wg sync.WaitGroup
			wg.Add(len(bf.shards))
			for _, s := range bf.shards {
				go func(sh *BatchFlow) {
					defer wg.Done()
					<-sh.Done()
				}(s)
			}
			wg.Wait()
			close(bf.done)
		}()

		go func() {
			<-ctx.Done()
			bf.closed.Store(true)
		}()

		return bf
	}

	flushFunc := func(ctx context.Context, batchData []*queuedRequest) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		groups := make(map[SchemaInterface][]*Request)
		for _, item := range batchData {
			if item == nil || item.request == nil {
				continue
			}
			schema := item.request.Schema()
			groups[schema] = append(groups[schema], item.request)
		}

		for schema, requests := range groups {
			data := make([]map[string]any, len(requests))
			for i, r := range requests {
				row := map[string]any{}
				vals := r.GetOrderedValues()
				cols := schema.Columns()
				for j, c := range cols {
					if j < len(vals) {
						row[c] = vals[j]
					}
				}
				data[i] = row
			}

			if err := executor.ExecuteBatch(ctx, schema, data); err != nil {
				return err
			}
		}
		return nil
	}

	bf.pipeline = gopipeline.NewStandardPipeline(config.goPipelineConfig(), flushFunc)

	go func() {
		defer close(bf.done)
		bf.setRunErr(bf.pipeline.AsyncPerform(ctx))
	}()

	return bf
}

func (b *BatchFlow) Submit(ctx context.Context, request *Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.closed.Load() {
		return context.Canceled
	}
	if request == nil {
		return ErrEmptyRequest
	}

	// shard routing
	if len(b.shards) > 0 {
		idx := b.pickShard(request)
		return b.shards[idx].Submit(ctx, request)
	}

	dataChan := b.pipeline.DataChan()
	select {
	case dataChan <- &queuedRequest{request: request, enqueuedAt: time.Now()}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *BatchFlow) pickShard(req *Request) int {
	n := len(b.shards)
	if n == 0 {
		return 0
	}
	if b.shardKeyFunc != nil {
		return int(b.shardKeyFunc(req) % uint64(n))
	}

	h := fnv.New64a()
	h.Write([]byte(req.Schema().Name()))
	return int(h.Sum64() % uint64(n))
}

func (b *BatchFlow) ErrorChan(size int) <-chan error {
	if len(b.shards) == 0 {
		return b.pipeline.ErrorChan(size)
	}
	out := make(chan error, size)
	go func() {
		var wg sync.WaitGroup
		for _, s := range b.shards {
			wg.Add(1)
			go func(ch <-chan error) {
				defer wg.Done()
				for {
					select {
					case err, ok := <-ch:
						if !ok {
							return
						}
						if err != nil {
							select {
							case out <- err:
							case <-b.Done():
								return
							}
					case <-b.Done():
						return
					}
				}
			}(s.ErrorChan(size))
		}
		wg.Wait()
		close(out)
	}()
	return out
}

func (b *BatchFlow) Close() error {
	b.closeOnce.Do(func() {
		b.closed.Store(true)

		if len(b.shards) > 0 {
			var errs []error
			for _, s := range b.shards {
				errs = append(errs, s.Close())
			}
			b.setRunErr(errors.Join(errs...))
		} else {
			close(b.pipeline.DataChan())
		}
	})
	return b.Wait()
}

func (b *BatchFlow) Wait() error {
	<-b.done
	return b.getRunErr()
}

func (b *BatchFlow) Done() <-chan struct{} {
	return b.done
}

func (b *BatchFlow) setRunErr(err error) {
	b.runErrMu.Lock()
	defer b.runErrMu.Unlock()
	b.runErr = err
}

func (b *BatchFlow) getRunErr() error {
	b.runErrMu.RLock()
	defer b.runErrMu.RUnlock()
	return b.runErr
}

func metricsReporterFromExecutor(executor BatchExecutor) MetricsReporter {
	if mp, ok := executor.(interface{ MetricsReporter() MetricsReporter }); ok {
		if r := mp.MetricsReporter(); r != nil {
			return r
		}
	}
	return NewNoopMetricsReporter()
}

func (c PipelineConfig) withDefaults() PipelineConfig {
	if c.BufferSize == 0 {
		c.BufferSize = 1000
	}
	if c.FlushSize == 0 {
		c.FlushSize = 100
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = 100 * time.Millisecond
	}
	return c
}

func (c PipelineConfig) goPipelineConfig() gopipeline.PipelineConfig {
	c = c.withDefaults()
	return gopipeline.PipelineConfig{
		BufferSize:            c.BufferSize,
		FlushSize:             c.FlushSize,
		FlushInterval:         c.FlushInterval,
		MaxConcurrentFlushes:  c.MaxConcurrentFlushes,
		DrainOnCancel:         c.DrainOnCancel,
		DrainGracePeriod:      c.DrainGracePeriod,
		FinalFlushOnCloseTimeout: c.FinalFlushOnCloseTimeout,
	}
}
