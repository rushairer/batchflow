package batchflow_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

func TestConcurrency_MultipleGoroutinesSubmit(t *testing.T) {
	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    1000,
		FlushSize:     100,
		FlushInterval: time.Second,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("test_table", batchflow.ConflictIgnoreOperationConfig, "id", "value")

	// 启动多个 goroutine 并发提交数据
	const numGoroutines = 10
	const requestsPerGoroutine = 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				request := batchflow.NewRequest(schema).
					SetInt64("id", int64(goroutineID*requestsPerGoroutine+j)).
					SetString("value", "goroutine_"+string(rune('0'+goroutineID)))

				err := batch.Submit(ctx, request)
				if err != nil {
					t.Errorf("Goroutine %d failed to submit request %d: %v", goroutineID, j, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// 等待所有数据处理完成
	time.Sleep(2 * time.Second)
}

func TestConcurrency_MultipleSchemas(t *testing.T) {
	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    500,
		FlushSize:     50,
		FlushInterval: time.Second,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	// 创建多个不同的 schema
	schemas := []*batchflow.SQLSchema{
		batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email"),
		batchflow.NewSQLSchema("products", batchflow.ConflictUpdateOperationConfig, "id", "name", "price"),
		batchflow.NewSQLSchema("orders", batchflow.ConflictReplaceOperationConfig, "id", "user_id", "product_id", "quantity"),
	}

	const numGoroutines = 6
	const requestsPerGoroutine = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			schema := schemas[goroutineID%len(schemas)]

			for j := 0; j < requestsPerGoroutine; j++ {
				var request *batchflow.Request

				switch schema.Name() {
				case "users":
					request = batchflow.NewRequest(schema).
						SetInt64("id", int64(goroutineID*requestsPerGoroutine+j)).
						SetString("name", "User"+string(rune('0'+goroutineID))).
						SetString("email", "user"+string(rune('0'+goroutineID))+"@example.com")
				case "products":
					request = batchflow.NewRequest(schema).
						SetInt64("id", int64(goroutineID*requestsPerGoroutine+j)).
						SetString("name", "Product"+string(rune('0'+goroutineID))).
						SetFloat64("price", float64(j*10+goroutineID))
				case "orders":
					request = batchflow.NewRequest(schema).
						SetInt64("id", int64(goroutineID*requestsPerGoroutine+j)).
						SetInt64("user_id", int64(goroutineID)).
						SetInt64("product_id", int64(j)).
						SetInt64("quantity", int64(j%10+1))
				}

				err := batch.Submit(ctx, request)
				if err != nil {
					t.Errorf("Goroutine %d failed to submit %s request %d: %v", goroutineID, schema.Name(), j, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// 等待所有数据处理完成
	time.Sleep(2 * time.Second)
}

func TestConcurrency_HighFrequencySubmission(t *testing.T) {
	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    2000,
		FlushSize:     200,
		FlushInterval: 100 * time.Millisecond, // 更频繁的刷新
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("high_freq_table", batchflow.ConflictIgnoreOperationConfig, "id", "timestamp", "data")

	const numGoroutines = 20
	const requestsPerGoroutine = 200
	var wg sync.WaitGroup
	var errorCount int32

	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				request := batchflow.NewRequest(schema).
					SetInt64("id", int64(goroutineID*requestsPerGoroutine+j)).
					SetTime("timestamp", time.Now()).
					SetString("data", "data_"+string(rune('0'+goroutineID))+"_"+string(rune('0'+j%10)))

				err := batch.Submit(ctx, request)
				if err != nil {
					// 使用原子操作计数错误
					atomic.AddInt32(&errorCount, 1)
					t.Errorf("High frequency submission failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	t.Logf("High frequency test completed in %v", duration)
	t.Logf("Total requests: %d", numGoroutines*requestsPerGoroutine)
	t.Logf("Requests per second: %.2f", float64(numGoroutines*requestsPerGoroutine)/duration.Seconds())
	t.Logf("Error count: %d", errorCount)

	// 等待所有数据处理完成
	time.Sleep(2 * time.Second)
}

func TestConcurrency_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := batchflow.PipelineConfig{
		BufferSize:    100,
		FlushSize:     50,
		FlushInterval: time.Second,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("cancel_test", batchflow.ConflictIgnoreOperationConfig, "id", "data")

	const numGoroutines = 5
	var wg sync.WaitGroup
	var submittedCount int32
	var errorCount int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ { // 大量请求
				select {
				case <-ctx.Done():
					return // 上下文被取消，退出
				default:
					request := batchflow.NewRequest(schema).
						SetInt64("id", int64(goroutineID*1000+j)).
						SetString("data", "data_"+string(rune('0'+goroutineID)))

					err := batch.Submit(ctx, request)
					if err != nil {
						atomic.AddInt32(&errorCount, 1)
					} else {
						atomic.AddInt32(&submittedCount, 1)
					}
				}
			}
		}(i)
	}

	// 在短时间后取消上下文
	go func() {
		time.Sleep(1 * time.Millisecond)
		cancel()
	}()

	wg.Wait()

	t.Logf("Submitted requests before cancellation: %d", atomic.LoadInt32(&submittedCount))
	t.Logf("Errors after cancellation: %d", atomic.LoadInt32(&errorCount))

	// 验证取消后不能再提交
	request := batchflow.NewRequest(schema).SetInt64("id", 99999).SetString("data", "after_cancel")
	err := batch.Submit(ctx, request)
	if err == nil {
		t.Error("Expected error when submitting after context cancellation")
	}
}

func TestConcurrency_MixedOperations(t *testing.T) {
	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    500,
		FlushSize:     100,
		FlushInterval: 500 * time.Millisecond,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("mixed_ops", batchflow.ConflictIgnoreOperationConfig, "id", "operation", "timestamp")

	var wg sync.WaitGroup
	const numOperations = 1000

	// 提前创建错误通道，避免并发期间修改内部通道指针
	errorChan := batch.ErrorChan(100)

	// 提交操作的 goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			request := batchflow.NewRequest(schema).
				SetInt64("id", int64(i)).
				SetString("operation", "submit").
				SetTime("timestamp", time.Now())

			err := batch.Submit(ctx, request)
			if err != nil {
				t.Errorf("Submit operation failed: %v", err)
			}

			// 随机延迟
			if i%100 == 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// 监听错误的 goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		errorCount := 0

		timeout := time.After(5 * time.Second)
		for {
			select {
			case err := <-errorChan:
				if err != nil {
					errorCount++
					t.Logf("Received error: %v", err)
				}
			case <-timeout:
				t.Logf("Error monitoring completed, total errors: %d", errorCount)
				return
			}
		}
	}()

	wg.Wait()
}

func TestConcurrency_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    5000,
		FlushSize:     500,
		FlushInterval: 200 * time.Millisecond,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("stress_test", batchflow.ConflictIgnoreOperationConfig, "id", "thread_id", "data", "timestamp")

	const numGoroutines = 50
	const requestsPerGoroutine = 1000
	var wg sync.WaitGroup
	var totalErrors int32

	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			localErrors := 0

			for j := 0; j < requestsPerGoroutine; j++ {
				request := batchflow.NewRequest(schema).
					SetInt64("id", int64(goroutineID*requestsPerGoroutine+j)).
					SetInt64("thread_id", int64(goroutineID)).
					SetString("data", "stress_data_"+string(rune('A'+goroutineID%26))).
					SetTime("timestamp", time.Now())

				err := batch.Submit(ctx, request)
				if err != nil {
					localErrors++
				}

				// 偶尔暂停以模拟真实负载
				if j%500 == 0 {
					time.Sleep(time.Microsecond * 100)
				}
			}

			atomic.AddInt32(&totalErrors, int32(localErrors))
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	totalRequests := numGoroutines * requestsPerGoroutine
	t.Logf("Stress test completed:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Requests per second: %.2f", float64(totalRequests)/duration.Seconds())
	t.Logf("  Total errors: %d", totalErrors)
	t.Logf("  Error rate: %.2f%%", float64(totalErrors)/float64(totalRequests)*100)

	// 等待所有数据处理完成
	time.Sleep(3 * time.Second)

	if totalErrors > int32(totalRequests/100) { // 错误率不应超过1%
		t.Errorf("Error rate too high: %d errors out of %d requests", totalErrors, totalRequests)
	}
}
