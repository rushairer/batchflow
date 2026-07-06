package batchflow

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

// Flow is the stable public runtime surface for the v2 module.
type Flow = RuntimeEngine

// RuntimeEngine owns sharding, routing, backpressure and memory protection.
type RuntimeEngine struct {
	cfg    RuntimeConfig
	shards []*BatchFlow

	rr     atomic.Uint64
	closed atomic.Bool
	done   chan struct{}
	once   sync.Once

	runErrMu sync.RWMutex
	runErr   error
}

// New creates the converged runtime flow.
func New(ctx context.Context, cfg Config) (*Flow, error) {
	return NewRuntimeEngine(ctx, cfg)
}

func NewRuntimeEngine(ctx context.Context, cfg Config) (*RuntimeEngine, error) {
	if cfg.Executor == nil {
		return nil, &ConfigError{Field: "Executor", Cause: errors.New("must not be nil")}
	}
	if err := cfg.Pipeline.Validate(); err != nil {
		return nil, err
	}

	pipelineCfg := cfg.Pipeline.withDefaults()
	runtimeCfg := cfg.Runtime.withDefaults()

	engine := &RuntimeEngine{
		cfg:    runtimeCfg,
		shards: make([]*BatchFlow, 0, runtimeCfg.ShardCount),
		done:   make(chan struct{}),
	}

	for i := uint32(0); i < runtimeCfg.ShardCount; i++ {
		engine.shards = append(engine.shards, newBatchFlow(ctx, pipelineCfg, cfg.Executor))
	}

	go func() {
		var wg sync.WaitGroup
		wg.Add(len(engine.shards))
		for _, shard := range engine.shards {
			go func(s *BatchFlow) {
				defer wg.Done()
				<-s.Done()
			}(shard)
		}
		wg.Wait()
		close(engine.done)
	}()

	go func() {
		<-ctx.Done()
		engine.closed.Store(true)
	}()

	return engine, nil
}

func (e *RuntimeEngine) Submit(ctx context.Context, req *Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil || len(e.shards) == 0 {
		return ErrInvalidSchema
	}
	if e.closed.Load() {
		return context.Canceled
	}
	if req == nil {
		return ErrEmptyRequest
	}
	if err := validateRequestForSubmit(req); err != nil {
		return err
	}
	if err := e.waitMemoryLimit(ctx); err != nil {
		return err
	}

	idx := e.pickShard(req)
	if err := e.waitBackpressure(ctx, idx); err != nil {
		return err
	}
	return e.shards[idx].Submit(ctx, req)
}

func (e *RuntimeEngine) Close() error {
	if e == nil {
		return nil
	}
	e.once.Do(func() {
		e.closed.Store(true)
		var wg sync.WaitGroup
		errCh := make(chan error, len(e.shards))
		for _, shard := range e.shards {
			wg.Add(1)
			go func(s *BatchFlow) {
				defer wg.Done()
				if err := s.Close(); err != nil {
					errCh <- err
				}
			}(shard)
		}
		wg.Wait()
		close(errCh)

		var errs []error
		for err := range errCh {
			errs = append(errs, err)
		}
		e.setRunErr(errors.Join(errs...))
	})
	return e.Wait()
}

func (e *RuntimeEngine) Wait() error {
	if e == nil {
		return nil
	}
	<-e.done
	return e.getRunErr()
}

func (e *RuntimeEngine) Done() <-chan struct{} {
	if e == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return e.done
}

func (e *RuntimeEngine) ErrorChan(size int) <-chan error {
	out := make(chan error, size)
	if e == nil || len(e.shards) == 0 {
		close(out)
		return out
	}
	go func() {
		var wg sync.WaitGroup
		for _, shard := range e.shards {
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
							case <-e.Done():
								return
							}
						}
					case <-e.Done():
						return
					}
				}
			}(shard.ErrorChan(size))
		}
		wg.Wait()
		close(out)
	}()
	return out
}

func (e *RuntimeEngine) pickShard(req *Request) int {
	n := len(e.shards)
	if n <= 1 {
		return 0
	}
	switch e.cfg.Routing {
	case ShardRoutingRoundRobin:
		return int(e.rr.Add(1)-1) % n
	case ShardRoutingLeastLoaded:
		return e.leastLoadedShard()
	case ShardRoutingHash:
		fallthrough
	default:
		key := e.requestKey(req)
		return int(key % uint64(n))
	}
}

func (e *RuntimeEngine) leastLoadedShard() int {
	best := 0
	bestDepth := e.queueDepth(0)
	for i := 1; i < len(e.shards); i++ {
		depth := e.queueDepth(i)
		if depth < bestDepth {
			best = i
			bestDepth = depth
		}
	}
	return best
}

func (e *RuntimeEngine) requestKey(req *Request) uint64 {
	if e.cfg.ShardKeyFunc != nil {
		return e.cfg.ShardKeyFunc(req)
	}
	h := fnv.New64a()
	schema := req.Schema()
	_, _ = h.Write([]byte(schema.Name()))

	if sqlSchema, ok := schema.(*SQLSchema); ok {
		for _, col := range sqlSchema.operationConfig.ConflictColumns {
			_, _ = h.Write([]byte("|"))
			_, _ = h.Write([]byte(col))
			_, _ = h.Write([]byte("="))
			_, _ = h.Write([]byte(fmt.Sprintf("%#v", req.columns[col])))
		}
	}
	return h.Sum64()
}

func (e *RuntimeEngine) waitBackpressure(ctx context.Context, shard int) error {
	bp := e.cfg.Backpressure.withDefaults()
	if !bp.Enabled || shard < 0 || shard >= len(e.shards) {
		return nil
	}
	high := bp.HighWatermark
	if high <= 0 {
		high = cap(e.shards[shard].pipeline.DataChan())
	}
	if high <= 0 || e.queueDepth(shard) < high {
		return nil
	}

	switch bp.Mode {
	case BackpressureReject:
		return ErrBackpressure
	case BackpressureTimeout:
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, bp.Timeout)
		defer cancel()
	}

	ticker := time.NewTicker(bp.CheckInterval)
	defer ticker.Stop()
	for {
		if e.queueDepth(shard) < high {
			return nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrBackpressure
			}
			return ctx.Err()
		case <-e.Done():
			return context.Canceled
		case <-ticker.C:
		}
	}
}

func (e *RuntimeEngine) queueDepth(shard int) int {
	if shard < 0 || shard >= len(e.shards) || e.shards[shard] == nil || e.shards[shard].pipeline == nil {
		return 0
	}
	return len(e.shards[shard].pipeline.DataChan())
}

func (e *RuntimeEngine) setRunErr(err error) {
	e.runErrMu.Lock()
	defer e.runErrMu.Unlock()
	e.runErr = err
}

func (e *RuntimeEngine) getRunErr() error {
	e.runErrMu.RLock()
	defer e.runErrMu.RUnlock()
	return e.runErr
}

func validateRequestForSubmit(request *Request) error {
	if request == nil {
		return ErrEmptyRequest
	}
	schema := request.Schema()
	if schema == nil {
		return ErrInvalidSchema
	}
	if schema.Columns() == nil || len(schema.Columns()) == 0 {
		return ErrMissingColumn
	}
	if len(schema.Name()) == 0 {
		return ErrEmptySchemaName
	}
	return nil
}