package batchflow_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rushairer/batchflow"
)

// We will not use real *sql.DB; instead, rely on NewBatchFlowWithMockDriver for SQL path coverage.

func TestConstructors_SQL_WithCustomDriver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，避免后台 goroutine 长时间运行

	cfg := batchflow.PipelineConfig{BufferSize: 8, FlushSize: 4, FlushInterval: 1 * time.Millisecond}
	mockDriver := batchflow.NewMockDriver("mysql")

	// 使用 nil *sql.DB 会在内部路径中仅用于持有，不应被解引用（我们不触发执行）
	var nilDB *sql.DB

	// 直接走顶层封装的 WithDriver 构造，验证不会 panic
	_ = batchflow.NewSQLBatchFlowWithDriver(ctx, nilDB, cfg, mockDriver)
}

func TestConstructors_Redis_WithCustomDriver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := batchflow.PipelineConfig{BufferSize: 8, FlushSize: 4, FlushInterval: 1 * time.Millisecond}
	cli := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}) // 不会真正连接
	_ = batchflow.NewRedisBatchFlowWithDriver(ctx, cli, cfg, batchflow.NewRedisPipelineDriver())
}
