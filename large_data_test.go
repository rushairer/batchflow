package batchflow_test

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

func TestLargeData_MillionRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large data test in short mode")
	}

	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    10000,
		FlushSize:     1000,
		FlushInterval: 500 * time.Millisecond,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("large_table", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email", "created_at")

	const totalRecords = 1000000 // 100万条记录
	startTime := time.Now()

	// 记录内存使用情况
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	t.Logf("Starting to submit %d records...", totalRecords)

	for i := 0; i < totalRecords; i++ {
		request := batchflow.NewRequest(schema).
			SetInt64("id", int64(i)).
			SetString("name", fmt.Sprintf("User_%d", i)).
			SetString("email", fmt.Sprintf("user_%d@example.com", i)).
			SetTime("created_at", time.Now())

		err := batch.Submit(ctx, request)
		if err != nil {
			t.Errorf("Failed to submit record %d: %v", i, err)
			return
		}

		// 每10万条记录报告一次进度（减少输出频率）
		if (i+1)%200000 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(i+1) / elapsed.Seconds()
			t.Logf("Submitted %d records, rate: %.2f records/sec", i+1, rate)
		}
	}

	submitDuration := time.Since(startTime)
	t.Logf("Submission completed in %v", submitDuration)
	t.Logf("Average submission rate: %.2f records/sec", float64(totalRecords)/submitDuration.Seconds())

	// 等待所有数据处理完成
	t.Log("Waiting for processing to complete...")
	time.Sleep(10 * time.Second)

	// 记录处理完成后的内存使用情况
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)

	totalDuration := time.Since(startTime)
	t.Logf("Total processing time: %v", totalDuration)
	t.Logf("Overall throughput: %.2f records/sec", float64(totalRecords)/totalDuration.Seconds())
	t.Logf("Memory usage - Before: %d KB, After: %d KB, Diff: %d KB",
		m1.Alloc/1024, m2.Alloc/1024, (m2.Alloc-m1.Alloc)/1024)
}

func TestLargeData_WideTable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping wide table test in short mode")
	}

	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    1000,
		FlushSize:     100,
		FlushInterval: time.Second,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	// 创建有很多列的表（500列）
	const numColumns = 500
	columns := make([]string, numColumns)
	for i := 0; i < numColumns; i++ {
		columns[i] = fmt.Sprintf("col_%d", i)
	}

	schema := batchflow.NewSQLSchema("wide_table", batchflow.ConflictIgnoreOperationConfig, columns...)

	const numRecords = 10000
	startTime := time.Now()

	t.Logf("Starting to submit %d records with %d columns each...", numRecords, numColumns)

	for i := 0; i < numRecords; i++ {
		request := batchflow.NewRequest(schema)

		// 为每一列设置值
		for j, col := range columns {
			switch j % 4 {
			case 0:
				request.SetInt64(col, int64(i*numColumns+j))
			case 1:
				request.SetString(col, fmt.Sprintf("value_%d_%d", i, j))
			case 2:
				request.SetFloat64(col, float64(i*j)/100.0)
			case 3:
				request.SetBool(col, (i+j)%2 == 0)
			}
		}

		err := batch.Submit(ctx, request)
		if err != nil {
			t.Errorf("Failed to submit wide record %d: %v", i, err)
			return
		}

		if (i+1)%1000 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(i+1) / elapsed.Seconds()
			t.Logf("Submitted %d wide records, rate: %.2f records/sec", i+1, rate)
		}
	}

	duration := time.Since(startTime)
	t.Logf("Wide table test completed in %v", duration)
	t.Logf("Average rate: %.2f records/sec", float64(numRecords)/duration.Seconds())

	// 等待处理完成
	time.Sleep(5 * time.Second)
}

func TestLargeData_LargeStrings(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large strings test in short mode")
	}

	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    100,
		FlushSize:     10,
		FlushInterval: time.Second,
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("large_strings_table", batchflow.ConflictIgnoreOperationConfig, "id", "small_text", "medium_text", "large_text")

	const numRecords = 1000
	startTime := time.Now()

	// 创建不同大小的字符串（减小尺寸避免日志输出过多）
	smallText := strings.Repeat("A", 256)   // 256B
	mediumText := strings.Repeat("B", 1024) // 1KB
	largeText := strings.Repeat("C", 4096)  // 4KB

	t.Logf("Starting to submit %d records with large strings...", numRecords)

	for i := 0; i < numRecords; i++ {
		request := batchflow.NewRequest(schema).
			SetInt64("id", int64(i)).
			SetString("small_text", smallText).
			SetString("medium_text", mediumText).
			SetString("large_text", largeText)

		err := batch.Submit(ctx, request)
		if err != nil {
			t.Errorf("Failed to submit large string record %d: %v", i, err)
			return
		}

		if (i+1)%500 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(i+1) / elapsed.Seconds()
			t.Logf("Submitted %d large string records, rate: %.2f records/sec", i+1, rate)
		}
	}

	duration := time.Since(startTime)
	t.Logf("Large strings test completed in %v", duration)
	t.Logf("Average rate: %.2f records/sec", float64(numRecords)/duration.Seconds())

	// 计算总数据量
	totalDataSize := int64(numRecords) * int64(len(smallText)+len(mediumText)+len(largeText))
	t.Logf("Total data processed: %.2f MB", float64(totalDataSize)/(1024*1024))
	t.Logf("Data throughput: %.2f MB/sec", float64(totalDataSize)/(1024*1024)/duration.Seconds())

	// 等待处理完成
	time.Sleep(5 * time.Second)
}

func TestLargeData_MemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}

	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    50000,           // 大缓冲区
		FlushSize:     10000,           // 大批次
		FlushInterval: 5 * time.Second, // 长间隔
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("memory_pressure_table", batchflow.ConflictIgnoreOperationConfig, "id", "data", "timestamp")

	const numRecords = 100000
	const dataSize = 1024 // 1KB per record (减小数据量)
	largeData := strings.Repeat("X", dataSize)

	// 监控内存使用
	var initialMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMem)

	startTime := time.Now()
	t.Logf("Starting memory pressure test with %d records of %d bytes each...", numRecords, dataSize)

	for i := 0; i < numRecords; i++ {
		request := batchflow.NewRequest(schema).
			SetInt64("id", int64(i)).
			SetString("data", largeData).
			SetTime("timestamp", time.Now())

		err := batch.Submit(ctx, request)
		if err != nil {
			t.Errorf("Failed to submit memory pressure record %d: %v", i, err)
			return
		}

		// 每2万条记录检查内存使用（减少输出频率）
		if (i+1)%20000 == 0 {
			var currentMem runtime.MemStats
			runtime.ReadMemStats(&currentMem)

			elapsed := time.Since(startTime)
			rate := float64(i+1) / elapsed.Seconds()
			memUsed := (currentMem.Alloc - initialMem.Alloc) / (1024 * 1024) // MB

			t.Logf("Submitted %d records, rate: %.2f records/sec, memory used: %d MB",
				i+1, rate, memUsed)
		}
	}

	submitDuration := time.Since(startTime)

	// 最终内存检查
	var finalMem runtime.MemStats
	runtime.ReadMemStats(&finalMem)

	t.Logf("Memory pressure test submission completed in %v", submitDuration)
	t.Logf("Peak memory usage: %d MB", (finalMem.Alloc-initialMem.Alloc)/(1024*1024))
	t.Logf("Expected data size: %d MB", (numRecords*dataSize)/(1024*1024))

	// 等待处理完成并检查内存释放
	t.Log("Waiting for processing to complete and memory to be released...")
	time.Sleep(10 * time.Second)

	runtime.GC()
	var afterGCMem runtime.MemStats
	runtime.ReadMemStats(&afterGCMem)

	t.Logf("Memory after GC: %d MB", (afterGCMem.Alloc-initialMem.Alloc)/(1024*1024))
}

func TestLargeData_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high throughput test in short mode")
	}

	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    20000,
		FlushSize:     2000,
		FlushInterval: 100 * time.Millisecond, // 非常频繁的刷新
	}

	batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

	schema := batchflow.NewSQLSchema("high_throughput_table", batchflow.ConflictIgnoreOperationConfig, "id", "value", "timestamp")

	const numRecords = 500000             // 50万条记录
	const testDuration = 30 * time.Second // 30秒测试

	startTime := time.Now()
	recordCount := 0

	t.Logf("Starting high throughput test for %v...", testDuration)

	// 使用定时器控制测试时长
	timer := time.NewTimer(testDuration)
	defer timer.Stop()

MainLoop:
	for {
		select {
		case <-timer.C:
			// 测试时间到
			break MainLoop
		default:
			// 继续提交数据
			request := batchflow.NewRequest(schema).
				SetInt64("id", int64(recordCount)).
				SetString("value", fmt.Sprintf("value_%d", recordCount)).
				SetTime("timestamp", time.Now())

			err := batch.Submit(ctx, request)
			if err != nil {
				t.Errorf("Failed to submit high throughput record %d: %v", recordCount, err)
				return
			}

			recordCount++

			// 每20万条记录报告一次（减少输出频率）
			if recordCount%200000 == 0 {
				elapsed := time.Since(startTime)
				rate := float64(recordCount) / elapsed.Seconds()
				t.Logf("Submitted %d records in %v, current rate: %.2f records/sec",
					recordCount, elapsed, rate)
			}

			// 如果达到最大记录数，也退出
			if recordCount >= numRecords {
				break MainLoop
			}
		}
	}

	actualDuration := time.Since(startTime)
	finalRate := float64(recordCount) / actualDuration.Seconds()

	t.Logf("High throughput test completed:")
	t.Logf("  Duration: %v", actualDuration)
	t.Logf("  Records submitted: %d", recordCount)
	t.Logf("  Average throughput: %.2f records/sec", finalRate)
	t.Logf("  Peak throughput target: %.2f records/sec", float64(numRecords)/testDuration.Seconds())

	// 等待处理完成
	time.Sleep(5 * time.Second)
}

func TestLargeData_BatchSizeOptimization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping batch size optimization test in short mode")
	}

	ctx := context.Background()
	schema := batchflow.NewSQLSchema("batch_optimization_table", batchflow.ConflictIgnoreOperationConfig, "id", "data")

	// 测试不同的批次大小
	batchSizes := []uint32{10, 50, 100, 500, 1000, 5000}
	const recordsPerTest = 50000

	results := make(map[uint32]time.Duration)

	for _, batchSize := range batchSizes {
		t.Logf("Testing batch size: %d", batchSize)

		config := batchflow.PipelineConfig{
			BufferSize:    batchSize * 10, // 缓冲区是批次大小的10倍
			FlushSize:     batchSize,
			FlushInterval: time.Second,
		}

		batch, _ := batchflow.NewBatchFlowWithMock(ctx, config)

		startTime := time.Now()

		for i := 0; i < recordsPerTest; i++ {
			request := batchflow.NewRequest(schema).
				SetInt64("id", int64(i)).
				SetString("data", fmt.Sprintf("data_%d", i))

			err := batch.Submit(ctx, request)
			if err != nil {
				t.Errorf("Failed to submit record %d with batch size %d: %v", i, batchSize, err)
				continue
			}
		}

		// 等待处理完成
		time.Sleep(3 * time.Second)

		duration := time.Since(startTime)
		results[batchSize] = duration

		rate := float64(recordsPerTest) / duration.Seconds()
		t.Logf("Batch size %d: %v duration, %.2f records/sec", batchSize, duration, rate)
	}

	// 找出最优批次大小
	var bestBatchSize uint32
	bestDuration := time.Hour // 初始化为很大的值

	t.Log("\nBatch size optimization results:")
	for batchSize, duration := range results {
		rate := float64(recordsPerTest) / duration.Seconds()
		t.Logf("  Batch size %d: %v (%.2f records/sec)", batchSize, duration, rate)

		if duration < bestDuration {
			bestDuration = duration
			bestBatchSize = batchSize
		}
	}

	t.Logf("\nOptimal batch size: %d (%.2f records/sec)",
		bestBatchSize, float64(recordsPerTest)/bestDuration.Seconds())
}
