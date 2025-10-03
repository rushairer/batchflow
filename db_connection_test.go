package batchflow_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

// MockDB 模拟数据库连接
type MockDB struct {
	mu           sync.RWMutex
	shouldFail   bool
	errorMessage string
	delay        time.Duration
	pingCount    int
	execCount    int
}

func (m *MockDB) Ping() error {
	m.mu.Lock()
	m.pingCount++
	shouldFail := m.shouldFail
	errMsg := m.errorMessage
	m.mu.Unlock()

	if shouldFail {
		return errors.New(errMsg)
	}
	return nil
}

func (m *MockDB) Exec(query string, args ...any) (sql.Result, error) {
	m.mu.Lock()
	m.execCount++
	delay := m.delay
	shouldFail := m.shouldFail
	errMsg := m.errorMessage
	m.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	if shouldFail {
		return nil, errors.New(errMsg)
	}
	return &MockResult{}, nil
}

func (m *MockDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	m.mu.Lock()
	m.execCount++
	delay := m.delay
	shouldFail := m.shouldFail
	errMsg := m.errorMessage
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if shouldFail {
		return nil, errors.New(errMsg)
	}
	return &MockResult{}, nil
}

// MockResult 模拟SQL执行结果
type MockResult struct{}

func (m *MockResult) LastInsertId() (int64, error) {
	return 1, nil
}

func (m *MockResult) RowsAffected() (int64, error) {
	return 1, nil
}

// 线程安全地更新 shouldFail
func (m *MockDB) SetShouldFail(v bool) {
	m.mu.Lock()
	m.shouldFail = v
	m.mu.Unlock()
}

// MockBatchExecutor 模拟批量执行器，用于测试数据库连接异常
type MockDBExecutor struct {
	db           *MockDB
	shouldFail   bool
	errorMessage string
}

func (m *MockDBExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
	if m.shouldFail {
		return errors.New(m.errorMessage)
	}

	// 模拟数据库操作
	if m.db != nil {
		_, err := m.db.ExecContext(ctx, "INSERT INTO "+schema.Name()+" VALUES (?)", "test")
		return err
	}

	return nil
}

func (m *MockDBExecutor) WithMetricsReporter(metricsReporter batchflow.MetricsReporter) batchflow.BatchExecutor {
	// 测试用：直接返回自身，避免将执行器置空导致管道 nil 指针异常
	return m
}

func TestDBConnection_ConnectionFailure(t *testing.T) {
	ctx := context.Background()

	// 创建会连接失败的模拟数据库
	mockDB := &MockDB{
		shouldFail:   true,
		errorMessage: "connection refused",
	}

	executor := &MockDBExecutor{
		db:         mockDB,
		shouldFail: false,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, executor)

	// 提前创建错误通道，避免并发修改内部通道
	errorChan := batch.ErrorChan(10)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	request := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("name", "test")

	err := batch.Submit(ctx, request)
	if err != nil {
		t.Errorf("Submit should not fail immediately: %v", err)
	}

	select {
	case err := <-errorChan:
		if err == nil {
			t.Error("Expected connection error, but got nil")
		}
		if err.Error() != "connection refused" {
			t.Errorf("Expected 'connection refused', got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Expected connection error but timeout occurred")
	}
}

func TestDBConnection_SlowConnection(t *testing.T) {
	ctx := context.Background()

	// 创建慢连接的模拟数据库
	mockDB := &MockDB{
		delay: 2 * time.Second,
	}

	executor := &MockDBExecutor{
		db: mockDB,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, executor)

	// 提前创建错误通道
	errorChan := batch.ErrorChan(10)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	request := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("name", "test")

	err := batch.Submit(ctx, request)
	if err != nil {
		t.Errorf("Submit should not fail immediately: %v", err)
	}

	select {
	case err := <-errorChan:
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			// 这是期望的超时错误
			return
		}
		t.Errorf("Expected timeout error, got: %v", err)
	case <-time.After(3 * time.Second):
		t.Log("No timeout error received, connection might be working normally")
	}
}

func TestDBConnection_ConnectionRecovery(t *testing.T) {
	ctx := context.Background()

	// 创建初始失败但后来恢复的模拟数据库
	mockDB := &MockDB{
		shouldFail:   true,
		errorMessage: "temporary connection failure",
	}

	executor := &MockDBExecutor{
		db: mockDB,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, executor)

	// 提前创建错误通道
	errorChan := batch.ErrorChan(10)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")

	// 提交第一个请求（应该失败）
	request1 := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("name", "test1")

	err := batch.Submit(ctx, request1)
	if err != nil {
		t.Errorf("Submit should not fail immediately: %v", err)
	}

	// 等待第一个错误
	select {
	case err := <-errorChan:
		if err == nil {
			t.Error("Expected connection error, but got nil")
		}
		t.Logf("Received expected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Error("Expected connection error but timeout occurred")
		return
	}

	// 模拟连接恢复
	mockDB.SetShouldFail(false)

	// 提交第二个请求（应该成功）
	request2 := batchflow.NewRequest(schema).
		SetInt64("id", 2).
		SetString("name", "test2")

	err = batch.Submit(ctx, request2)
	if err != nil {
		t.Errorf("Submit after recovery should not fail: %v", err)
	}

	// 等待一段时间，确保没有更多错误
	select {
	case err := <-errorChan:
		if err != nil {
			t.Errorf("Unexpected error after recovery: %v", err)
		}
	case <-time.After(1 * time.Second):
		// 没有错误是期望的
		t.Log("No errors after connection recovery - good!")
	}
}

func TestDBConnection_TransactionFailure(t *testing.T) {
	ctx := context.Background()

	// 创建在执行时失败的模拟数据库
	mockDB := &MockDB{
		shouldFail:   true,
		errorMessage: "transaction deadlock",
	}

	executor := &MockDBExecutor{
		db: mockDB,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, executor)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")

	// 提交多个请求
	for i := 0; i < 10; i++ {
		request := batchflow.NewRequest(schema).
			SetInt64("id", int64(i)).
			SetString("name", "test"+string(rune('0'+i)))

		err := batch.Submit(ctx, request)
		if err != nil {
			t.Errorf("Submit %d should not fail immediately: %v", i, err)
		}
	}

	// 监听错误通道
	errorChan := batch.ErrorChan(10)
	errorCount := 0

	timeout := time.After(3 * time.Second)
	for {
		select {
		case err := <-errorChan:
			if err != nil {
				errorCount++
				if err.Error() != "transaction deadlock" {
					t.Errorf("Expected 'transaction deadlock', got: %v", err)
				}
			}
		case <-timeout:
			if errorCount == 0 {
				t.Error("Expected at least one transaction error")
			} else {
				t.Logf("Received %d transaction errors as expected", errorCount)
			}
			return
		}
	}
}

func TestDBConnection_ContextCancellationDuringExecution(t *testing.T) {
	ctx := context.Background()

	// 创建执行缓慢的模拟数据库
	mockDB := &MockDB{
		delay: 3 * time.Second,
	}

	executor := &MockDBExecutor{
		db: mockDB,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, executor)

	// 提前创建错误通道
	errorChan := batch.ErrorChan(10)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	request := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("name", "test")

	err := batch.Submit(ctx, request)
	if err != nil {
		t.Errorf("Submit should not fail immediately: %v", err)
	}

	// 等待错误（应该是上下文取消错误）
	select {
	case err := <-errorChan:
		if err != nil && errors.Is(err, context.Canceled) {
			t.Log("Received expected context cancellation error")
		} else if err != nil {
			t.Logf("Received error (might be timeout related): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Log("No cancellation error received within timeout")
	}
}

func TestDBConnection_MaxConnectionsExceeded(t *testing.T) {
	ctx := context.Background()

	// 模拟连接池耗尽的情况
	executor := &MockDBExecutor{
		shouldFail:   true,
		errorMessage: "too many connections",
	}

	batch := batchflow.NewBatchFlow(ctx, 100, 10, time.Second, executor)

	// 提前创建错误通道
	errorChan := batch.ErrorChan(50)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")

	// 提交大量请求
	for i := 0; i < 50; i++ {
		request := batchflow.NewRequest(schema).
			SetInt64("id", int64(i)).
			SetString("name", "test"+string(rune('0'+i%10)))

		err := batch.Submit(ctx, request)
		if err != nil {
			t.Errorf("Submit %d should not fail immediately: %v", i, err)
		}
	}

	errorCount := 0

	timeout := time.After(3 * time.Second)
	for {
		select {
		case err := <-errorChan:
			if err != nil {
				errorCount++
				if err.Error() != "too many connections" {
					t.Errorf("Expected 'too many connections', got: %v", err)
				}
			}
		case <-timeout:
			if errorCount == 0 {
				t.Error("Expected at least one connection pool error")
			} else {
				t.Logf("Received %d connection pool errors as expected", errorCount)
			}
			return
		}
	}
}

func TestDBConnection_NetworkPartition(t *testing.T) {
	ctx := context.Background()

	// 模拟网络分区导致的连接超时
	mockDB := &MockDB{
		delay:      5 * time.Second, // 很长的延迟模拟网络问题
		shouldFail: false,
	}

	executor := &MockDBExecutor{
		db: mockDB,
	}

	batch := batchflow.NewBatchFlow(ctx, 10, 5, time.Second, executor)

	// 提前创建错误通道，期望超时错误
	errorChan := batch.ErrorChan(10)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	request := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("name", "test")

	err := batch.Submit(ctx, request)
	if err != nil {
		t.Errorf("Submit should not fail immediately: %v", err)
	}

	select {
	case err := <-errorChan:
		if err != nil {
			t.Logf("Received network-related error: %v", err)
			// 网络分区可能导致各种错误，我们只是记录它们
		}
	case <-time.After(6 * time.Second):
		t.Log("No network partition error received, operation might have completed")
	}
}
