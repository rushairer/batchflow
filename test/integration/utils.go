package main

import (
	"fmt"
	"os"
	"time"
)

// 验证数据库中的实际记录数
func getActualRecordCount(db interface{}) (int64, error) {
	// 这个函数需要根据数据库类型进行类型断言
	// 在 sql_tests.go 中会有具体的 SQL 实现
	// 在 redis_tests.go 中会有具体的 Redis 实现
	return 0, fmt.Errorf("not implemented for this database type")
}

// calculateMemoryDiffMB 安全计算内存差值并转换为 MB 单位
//
// 更新历史：
// - 2025-10-03: 确认转换公式正确性，验证与 Grafana 显示单位一致性
//
// 功能说明：
//   - 计算两个内存快照之间的差值
//   - 将字节单位转换为 MB 单位
//   - 处理 GC 回收导致的负数情况
//
// 参数：
//   - after: 后续内存快照值（字节），来自 runtime.ReadMemStats()
//   - before: 初始内存快照值（字节），来自 runtime.ReadMemStats()
//
// 返回值：
//   - float64: 内存差值，单位 MB，计算公式：(after - before) / 1024 / 1024
//
// 使用场景：
//   - 测试前后内存使用量对比
//   - 内存压力测试中的内存增长监控
//   - 与 batchflow_memory_usage_mb 指标配合使用
func calculateMemoryDiffMB(after, before uint64) float64 {
	if after >= before {
		return float64(after-before) / 1024 / 1024
	}
	// 如果 after < before（GC回收了内存），返回0而不是负数
	return 0.0
}

// calculateDataIntegrity 计算数据完整性状态
func calculateDataIntegrity(submitted, actual int64) (rate float64, status string, rpsValid bool, rpsNote string) {
	if actual < 0 {
		return 0.0, "❓ 无法验证", false, "无法获取实际记录数，RPS无效"
	}

	if submitted == 0 {
		return 0.0, "❌ 无提交记录", false, "无提交记录，RPS无效"
	}

	rate = float64(actual) / float64(submitted) * 100.0

	if actual == submitted {
		return 100.0, "✅ 完全一致", true, "数据完整性100%，RPS有效"
	} else if actual > submitted {
		return rate, fmt.Sprintf("⚠️ 超出预期 (+%d条)", actual-submitted), false, "数据超出预期，RPS无效"
	} else {
		lossCount := submitted - actual
		lossRate := float64(lossCount) / float64(submitted) * 100.0
		return rate, fmt.Sprintf("❌ 数据丢失 (-%d条, %.1f%%)", lossCount, lossRate), false, fmt.Sprintf("数据丢失%.1f%%，RPS无效", lossRate)
	}
}

// getReportsDirectory 智能检测报告目录，兼容本地和Docker环境
func getReportsDirectory() string {
	// 检查是否在Docker环境中（通过检查/app目录是否存在且可写）
	if info, err := os.Stat("/app"); err == nil && info.IsDir() {
		// 尝试在/app目录创建测试文件来检查写权限
		testFile := "/app/.write_test"
		if file, err := os.Create(testFile); err == nil {
			file.Close()
			os.Remove(testFile)
			return "/app/reports" // Docker环境，使用/app/reports
		}
	}

	// 本地环境或Docker环境无写权限，使用相对路径
	return "reports"
}

// generateSummary 生成测试摘要
func generateSummary(results []TestResult, totalDuration time.Duration) TestSummary {
	summary := TestSummary{
		TotalTests:    len(results),
		TotalDuration: totalDuration.String(),
	}

	var totalRecords int64
	var validRPSCount int
	var totalValidRPS float64
	maxRPS := 0.0

	for _, result := range results {
		if result.Success {
			summary.PassedTests++
		} else {
			summary.FailedTests++
		}

		totalRecords += result.TotalRecords

		// 只统计有效的RPS
		if result.RPSValid {
			totalValidRPS += result.RecordsPerSecond
			validRPSCount++

			if result.RecordsPerSecond > maxRPS {
				maxRPS = result.RecordsPerSecond
			}
		}
	}

	summary.TotalRecords = totalRecords
	summary.MaxRPS = maxRPS
	if validRPSCount > 0 {
		summary.AverageRPS = totalValidRPS / float64(validRPSCount)
	} else {
		summary.AverageRPS = 0.0 // 没有有效的RPS数据
	}

	return summary
}
