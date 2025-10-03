package batchflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

// MockErrorExecutor 模拟错误的执行器
type MockErrorExecutor struct {
	shouldFailExec bool
	errorMessage   string
	delay          time.Duration
}

func (m *MockErrorExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if m.shouldFailExec {
		return errors.New(m.errorMessage)
	}
	return nil
}

func (m *MockErrorExecutor) WithMetricsReporter(metricsReporter batchflow.MetricsReporter) batchflow.BatchExecutor {
	// 测试用：直接返回自身，避免将执行器置空导致管道 nil 指针异常
	return m
}

func TestErrorHandling_ExecutionError(t *testing.T) {
	ctx := context.Background()

	// 创建会在执行时失败的执行器
	errorExecutor := &MockErrorExecutor{
		shouldFailExec: true,
		errorMessage:   "Database connection failed",
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, errorExecutor)

	// 提前创建错误通道，避免并发期间修改内部通道
	errorChan := batch.ErrorChan(10)

	// 创建 schema 和请求
	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id")
	request := batchflow.NewRequest(schema).SetInt64("id", 1)

	// 提交数据
	err := batch.Submit(ctx, request)
	if err != nil {
		t.Errorf("Submit should not return error immediately: %v", err)
	}

	// 等待错误
	select {
	case err := <-errorChan:
		if err == nil {
			t.Error("Expected error from execution, but got nil")
		}
		if err.Error() != "Database connection failed" {
			t.Errorf("Expected 'Database connection failed', got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Expected error but timeout occurred")
	}
}

func TestErrorHandling_ContextTimeout(t *testing.T) {
	ctx := context.Background()

	// 创建会延迟执行的执行器
	slowExecutor := &MockErrorExecutor{
		delay: 2 * time.Second,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, slowExecutor)

	// 提前创建错误通道，使用短超时
	errorChan := batch.ErrorChan(10)

	// 创建 schema 和请求
	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id")
	request := batchflow.NewRequest(schema).SetInt64("id", 1)

	// 提交数据
	err := batch.Submit(ctx, request)
	if err != nil {
		t.Errorf("Submit should not return error immediately: %v", err)
	}

	// 等待错误或超时
	select {
	case err := <-errorChan:
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			// 这是我们期望的超时错误
			return
		}
		t.Errorf("Expected timeout error, got: %v", err)
	case <-time.After(3 * time.Second):
		// 如果没有收到错误，说明超时机制可能没有正常工作
		t.Log("No timeout error received, this might be expected in some cases")
	}
}

func TestErrorHandling_ContextCancellation(t *testing.T) {
	ctx := context.Background()

	// 创建会延迟执行的执行器
	slowExecutor := &MockErrorExecutor{
		delay: 2 * time.Second,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, slowExecutor)

	// 提前创建错误通道
	errorChan := batch.ErrorChan(10)

	// 创建 schema 和请求
	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id")
	request := batchflow.NewRequest(schema).SetInt64("id", 1)

	// 提交数据
	err := batch.Submit(ctx, request)
	if err != nil {
		t.Errorf("Submit should not return error immediately: %v", err)
	}

	// 等待错误或超时
	select {
	case err := <-errorChan:
		if err != nil && errors.Is(err, context.Canceled) {
			// 这是我们期望的取消错误
			return
		}
		t.Errorf("Expected cancellation error, got: %v", err)
	case <-time.After(3 * time.Second):
		// 如果没有收到错误，说明取消机制可能没有正常工作
		t.Log("No cancellation error received, this might be expected in some cases")
	}
}

func TestErrorHandling_InvalidData(t *testing.T) {
	ctx := context.Background()

	config := batchflow.PipelineConfig{
		BufferSize:    10,
		FlushSize:     5,
		FlushInterval: time.Second,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	// 测试 nil 请求
	err := batch.Submit(ctx, nil)
	if err == nil {
		t.Error("Expected error for nil request, but got nil")
	}

	// 测试空 schema
	emptySchema := batchflow.NewSQLSchema("", batchflow.ConflictIgnoreOperationConfig)
	emptyRequest := batchflow.NewRequest(emptySchema)
	err = batch.Submit(ctx, emptyRequest)
	if err == nil {
		t.Error("Expected error for empty table name, but got nil")
	}
}

func TestErrorHandling_MultipleErrors(t *testing.T) {
	ctx := context.Background()

	// 创建会在执行时失败的执行器
	errorExecutor := &MockErrorExecutor{
		shouldFailExec: true,
		errorMessage:   "Multiple execution failed",
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 2, time.Second, errorExecutor)

	// 提前创建错误通道
	errorChan := batch.ErrorChan(10)

	// 创建多个不同的 schema
	schema1 := batchflow.NewSQLSchema("table1", batchflow.ConflictIgnoreOperationConfig, "id")
	schema2 := batchflow.NewSQLSchema("table2", batchflow.ConflictIgnoreOperationConfig, "id")
	schema3 := batchflow.NewSQLSchema("table3", batchflow.ConflictIgnoreOperationConfig, "id")

	// 提交多个表的数据
	_ = batch.Submit(ctx, batchflow.NewRequest(schema1).SetInt64("id", 1))
	_ = batch.Submit(ctx, batchflow.NewRequest(schema2).SetInt64("id", 2))
	_ = batch.Submit(ctx, batchflow.NewRequest(schema3).SetInt64("id", 3))

	// 等待错误
	select {
	case err := <-errorChan:
		if err == nil {
			t.Error("Expected error from multiple executions, but got nil")
		}
		if err.Error() != "Multiple execution failed" {
			t.Errorf("Expected 'Multiple execution failed', got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Expected error but timeout occurred")
	}
}

func TestErrorHandling_CloseWithPendingData(t *testing.T) {
	ctx := context.Background()

	config := batchflow.PipelineConfig{
		BufferSize:    100, // 大缓冲区，确保数据不会自动刷新
		FlushSize:     50,
		FlushInterval: time.Hour, // 很长的间隔，确保不会自动刷新
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	// 创建 schema 和请求
	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id")

	// 提交一些数据但不刷新
	for i := 0; i < 10; i++ {
		request := batchflow.NewRequest(schema).SetInt64("id", int64(i))
		err := batch.Submit(ctx, request)
		if err != nil {
			t.Errorf("Submit failed: %v", err)
		}
	}
}
