package batchflow

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// blockingExecutor 阻塞 ExecuteBatch，配合 MaxConcurrentFlushes=1 使 flusher 确定性卡住，
// 从而让队列数据真实堆积，用于稳定观测 backpressure / memory limit 行为。
type blockingExecutor struct {
	releaseOnce sync.Once
	releaseCh   chan struct{}
	flushes     atomic.Int32
}

func newBlockingExecutor() *blockingExecutor {
	return &blockingExecutor{releaseCh: make(chan struct{})}
}

func (b *blockingExecutor) ExecuteBatch(_ context.Context, _ SchemaInterface, _ []map[string]any) error {
	b.flushes.Add(1)
	<-b.releaseCh
	return nil
}

func (b *blockingExecutor) release() {
	b.releaseOnce.Do(func() { close(b.releaseCh) })
}

// newBlockingEngine 构造一个 FlushSize=1、MaxConcurrentFlushes=1、执行器阻塞的引擎。
// flusher 消费 2 条后（1 条在飞 flush 占满信号量，1 条在等信号量）即确定性卡住，
// 之后发送的数据都会堆积在通道中。
func newBlockingEngine(t *testing.T, runtimeCfg RuntimeConfig) (*RuntimeEngine, *blockingExecutor) {
	t.Helper()
	block := newBlockingExecutor()
	cfg := Config{
		Pipeline: PipelineConfig{
			BufferSize:           1000,
			FlushSize:            1,
			FlushInterval:        time.Hour,
			MaxConcurrentFlushes: 1,
		},
		Runtime:  runtimeCfg,
		Executor: block,
	}
	e, err := NewRuntimeEngine(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewRuntimeEngine failed: %v", err)
	}
	t.Cleanup(func() {
		block.release()
		_ = e.Close()
	})
	return e, block
}

// newTestEngine 构造一个 flusher 正常运行、仅按 FlushInterval 自动 flush 的引擎，
// 适用于不依赖队列堆积的场景。
func newTestEngine(t *testing.T, runtimeCfg RuntimeConfig) *RuntimeEngine {
	t.Helper()
	cfg := Config{
		Pipeline: PipelineConfig{
			BufferSize:    1000,
			FlushSize:     1 << 20, // 足够大避免按 FlushSize 触发 flush，同时避免过大预分配（initBatchData 按 FlushSize 预分配）
			FlushInterval: time.Hour,
		},
		Runtime:  runtimeCfg,
		Executor: NewMockExecutor(),
	}
	e, err := NewRuntimeEngine(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewRuntimeEngine failed: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func newTestSQLRequest(table, col string) *Request {
	schema := NewSQLSchema(table, ConflictIgnoreOperationConfig, col)
	return NewRequest(schema).SetUint64(col, 1)
}

// fillQueue 向指定 shard 的通道直接发送 n 条请求。在 blocking 引擎上，
// flusher 最多消费 2 条即卡住，其余数据稳定堆积。
func fillQueue(e *RuntimeEngine, shard, n int, req *Request) {
	dataChan := e.shards[shard].pipeline.DataChan()
	for i := 0; i < n; i++ {
		dataChan <- &queuedRequest{request: req, enqueuedAt: time.Now()}
	}
}

func TestNewRuntimeEngine_NilExecutor(t *testing.T) {
	_, err := NewRuntimeEngine(context.Background(), Config{Executor: nil})
	if err == nil {
		t.Fatal("expected error for nil executor")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Field != "Executor" {
		t.Fatalf("expected field Executor, got %q", cfgErr.Field)
	}
}

func TestRuntimeEngine_Submit_ValidateErrors(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 1})

	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.Submit(cctx, newTestSQLRequest("t", "id")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if err := e.Submit(context.Background(), nil); !errors.Is(err, ErrEmptyRequest) {
		t.Fatalf("expected ErrEmptyRequest, got %v", err)
	}
	if err := e.Submit(context.Background(), &Request{}); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
}

func TestValidateRequestForSubmit(t *testing.T) {
	if err := validateRequestForSubmit(nil); !errors.Is(err, ErrEmptyRequest) {
		t.Fatalf("expected ErrEmptyRequest, got %v", err)
	}
	if err := validateRequestForSubmit(&Request{}); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}
	if err := validateRequestForSubmit(NewRequest(NewSQLSchema("t", ConflictIgnoreOperationConfig))); !errors.Is(err, ErrMissingColumn) {
		t.Fatalf("expected ErrMissingColumn, got %v", err)
	}
	if err := validateRequestForSubmit(NewRequest(NewSQLSchema("", ConflictIgnoreOperationConfig, "id"))); !errors.Is(err, ErrEmptySchemaName) {
		t.Fatalf("expected ErrEmptySchemaName, got %v", err)
	}
	if err := validateRequestForSubmit(newTestSQLRequest("t", "id")); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEstimatedQueuedBytes(t *testing.T) {
	var nilEngine *RuntimeEngine
	if got := nilEngine.estimatedQueuedBytes(MemoryLimitConfig{}); got != 0 {
		t.Fatalf("nil engine should return 0, got %d", got)
	}

	e := newTestEngine(t, RuntimeConfig{ShardCount: 2})
	if got := e.estimatedQueuedBytes(MemoryLimitConfig{AvgRequestBytes: 512}); got != 0 {
		t.Fatalf("empty queue should return 0, got %d", got)
	}

	blocking, _ := newBlockingEngine(t, RuntimeConfig{ShardCount: 1})
	req := newTestSQLRequest("t", "id")
	fillQueue(blocking, 0, 10, req)
	if got := blocking.estimatedQueuedBytes(MemoryLimitConfig{AvgRequestBytes: 512}); got < 8*512 {
		t.Fatalf("expected at least %d queued bytes, got %d", 8*512, got)
	}
}

func TestWaitMemoryLimit_Disabled(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 1})
	if err := e.waitMemoryLimit(context.Background()); err != nil {
		t.Fatalf("disabled memory limit should pass, got %v", err)
	}
}

func TestWaitMemoryLimit_Reject(t *testing.T) {
	e, _ := newBlockingEngine(t, RuntimeConfig{
		ShardCount: 1,
		MemoryLimit: MemoryLimitConfig{
			Enabled:         true,
			MaxQueueBytes:   512,
			AvgRequestBytes: 512,
			Mode:            BackpressureReject,
		},
	})
	fillQueue(e, 0, 10, newTestSQLRequest("t", "id"))
	if err := e.waitMemoryLimit(context.Background()); !errors.Is(err, ErrMemoryLimitExceeded) {
		t.Fatalf("expected ErrMemoryLimitExceeded, got %v", err)
	}
}

func TestWaitMemoryLimit_Timeout(t *testing.T) {
	e, _ := newBlockingEngine(t, RuntimeConfig{
		ShardCount: 1,
		MemoryLimit: MemoryLimitConfig{
			Enabled:         true,
			MaxQueueBytes:   512,
			AvgRequestBytes: 512,
			Mode:            BackpressureTimeout,
			CheckInterval:   5 * time.Millisecond,
			Timeout:         20 * time.Millisecond,
		},
	})
	fillQueue(e, 0, 10, newTestSQLRequest("t", "id"))
	start := time.Now()
	err := e.waitMemoryLimit(context.Background())
	if !errors.Is(err, ErrMemoryLimitExceeded) {
		t.Fatalf("expected ErrMemoryLimitExceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("expected timeout wait, returned too fast: %v", elapsed)
	}
}

func TestWaitMemoryLimit_Recovery(t *testing.T) {
	e, block := newBlockingEngine(t, RuntimeConfig{
		ShardCount: 1,
		MemoryLimit: MemoryLimitConfig{
			Enabled:         true,
			MaxQueueBytes:   512,
			AvgRequestBytes: 512,
			Mode:            BackpressureTimeout,
			CheckInterval:   5 * time.Millisecond,
			Timeout:         time.Second,
		},
	})
	fillQueue(e, 0, 10, newTestSQLRequest("t", "id"))
	go func() {
		time.Sleep(20 * time.Millisecond)
		block.release() // 放行 flusher，队列被消费清空
	}()
	if err := e.waitMemoryLimit(context.Background()); err != nil {
		t.Fatalf("expected recovery after flush, got %v", err)
	}
}

func TestPickShard_RoundRobin(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 3, Routing: ShardRoutingRoundRobin})
	req := newTestSQLRequest("t", "id")
	want := []int{0, 1, 2, 0}
	for i, w := range want {
		if got := e.pickShard(req); got != w {
			t.Fatalf("round %d: expected shard %d, got %d", i, w, got)
		}
	}
}

func TestPickShard_LeastLoaded(t *testing.T) {
	e, _ := newBlockingEngine(t, RuntimeConfig{ShardCount: 3, Routing: ShardRoutingLeastLoaded})
	req := newTestSQLRequest("t", "id")
	// flusher 在 shard 上消费 2 条后卡住；各 shard 通道中剩余堆积：
	// shard0: 10 条发送 → 至少剩 8 条；shard1: 5 条 → 至少剩 3 条；shard2: 0 条
	fillQueue(e, 0, 10, req)
	fillQueue(e, 1, 5, req)
	if got := e.pickShard(req); got != 2 {
		t.Fatalf("expected least-loaded shard 2, got %d", got)
	}
}

func TestPickShard_HashStable(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 3, Routing: ShardRoutingHash})
	req := newTestSQLRequest("t", "id")
	first := e.pickShard(req)
	for i := 0; i < 10; i++ {
		if got := e.pickShard(req); got != first {
			t.Fatalf("hash routing should be stable, got %d want %d", got, first)
		}
	}
}

func TestRequestKey_CustomFunc(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{
		ShardCount:   2,
		ShardKeyFunc: func(*Request) uint64 { return 42 },
	})
	if got := e.requestKey(newTestSQLRequest("t", "id")); got != 42 {
		t.Fatalf("expected custom key 42, got %d", got)
	}
}

func TestRequestKey_DefaultStable(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 2})
	a := newTestSQLRequest("t", "id")
	b := newTestSQLRequest("t", "id")
	if k1, k2 := e.requestKey(a), e.requestKey(b); k1 != k2 {
		t.Fatalf("same request should produce same key, got %d vs %d", k1, k2)
	}
}

func TestQueueDepth_OutOfRange(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 1})
	if got := e.queueDepth(-1); got != 0 {
		t.Fatalf("expected 0 for negative shard, got %d", got)
	}
	if got := e.queueDepth(5); got != 0 {
		t.Fatalf("expected 0 for out-of-range shard, got %d", got)
	}
}

func TestWaitBackpressure_Disabled(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 1})
	if err := e.waitBackpressure(context.Background(), 0); err != nil {
		t.Fatalf("disabled backpressure should pass, got %v", err)
	}
	if err := e.waitBackpressure(context.Background(), -1); err != nil {
		t.Fatalf("out-of-range shard should pass when disabled, got %v", err)
	}
}

func TestWaitBackpressure_Reject(t *testing.T) {
	e, _ := newBlockingEngine(t, RuntimeConfig{
		ShardCount: 1,
		Backpressure: BackpressureConfig{
			Enabled:       true,
			HighWatermark: 1,
			Mode:          BackpressureReject,
		},
	})
	fillQueue(e, 0, 10, newTestSQLRequest("t", "id"))
	if err := e.waitBackpressure(context.Background(), 0); !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected ErrBackpressure, got %v", err)
	}
}

func TestWaitBackpressure_Timeout(t *testing.T) {
	e, _ := newBlockingEngine(t, RuntimeConfig{
		ShardCount: 1,
		Backpressure: BackpressureConfig{
			Enabled:       true,
			HighWatermark: 1,
			Mode:          BackpressureTimeout,
			CheckInterval: 5 * time.Millisecond,
			Timeout:       20 * time.Millisecond,
		},
	})
	fillQueue(e, 0, 10, newTestSQLRequest("t", "id"))
	start := time.Now()
	err := e.waitBackpressure(context.Background(), 0)
	if !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected ErrBackpressure, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("expected timeout wait, returned too fast: %v", elapsed)
	}
}

func TestWaitBackpressure_Recovery(t *testing.T) {
	e, block := newBlockingEngine(t, RuntimeConfig{
		ShardCount: 1,
		Backpressure: BackpressureConfig{
			Enabled:       true,
			HighWatermark: 1,
			Mode:          BackpressureTimeout,
			CheckInterval: 5 * time.Millisecond,
			Timeout:       time.Second,
		},
	})
	fillQueue(e, 0, 10, newTestSQLRequest("t", "id"))
	go func() {
		time.Sleep(20 * time.Millisecond)
		block.release() // 放行 flusher，队列被消费清空
	}()
	if err := e.waitBackpressure(context.Background(), 0); err != nil {
		t.Fatalf("expected recovery after flush, got %v", err)
	}
}

func TestRuntimeEngine_Submit_BackpressureEndToEnd(t *testing.T) {
	// 端到端验证：Submit 走完整个链路，队列堆积后最终被拒绝。
	// flusher 消费是异步的，因此不断提交直到出现 ErrBackpressure 为止。
	e, _ := newBlockingEngine(t, RuntimeConfig{
		ShardCount: 1,
		Backpressure: BackpressureConfig{
			Enabled:       true,
			HighWatermark: 1,
			Mode:          BackpressureReject,
		},
	})
	ctx := context.Background()
	req := newTestSQLRequest("t", "id")
	rejected := false
	for i := 0; i < 2000; i++ {
		err := e.Submit(ctx, req)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrBackpressure) {
			rejected = true
			break
		}
		t.Fatalf("unexpected error: %v", err)
	}
	if !rejected {
		t.Fatal("expected backpressure rejection after queue fill")
	}
}

func TestRuntimeEngine_CloseIdempotent(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 2})
	if err := e.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("second Close should be idempotent, got %v", err)
	}
}

func TestRuntimeEngine_DoneOnClose(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 1})
	select {
	case <-e.Done():
		t.Fatal("Done should not be closed before Close")
	default:
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	select {
	case <-e.Done():
	default:
		t.Fatal("Done should be closed after Close")
	}
}

func TestRuntimeEngine_Done_NilEngine(t *testing.T) {
	var e *RuntimeEngine
	select {
	case <-e.Done():
	default:
		t.Fatal("nil engine Done should be closed immediately")
	}
}

func TestRuntimeEngine_ErrorChan_ClosesOnClose(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 1})
	ch := e.ErrorChan(16)
	if err := e.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	for range ch {
		t.Fatal("unexpected error on channel")
	}
}

func TestRuntimeEngine_Submit_AfterClose(t *testing.T) {
	e := newTestEngine(t, RuntimeConfig{ShardCount: 1})
	if err := e.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := e.Submit(context.Background(), newTestSQLRequest("t", "id")); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled after close, got %v", err)
	}
}
