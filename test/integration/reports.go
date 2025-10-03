package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func saveReport(report *TestReport) {
	// 智能检测报告目录 - 兼容本地和Docker环境
	reportsDir := getReportsDirectory()
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		log.Printf("❌ 创建报告目录失败：%v", err)
		return
	}

	// 生成文件名
	timestamp := report.Timestamp.Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s/integration_test_report_%s.json", reportsDir, timestamp)

	// 保存 JSON 报告
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Printf("❌ 序列化报告失败：%v", err)
		return
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		log.Printf("❌ 保存报告失败：%v", err)
		return
	}

	log.Printf("📊 测试报告已保存到：%s", filename)

	// 生成 HTML 报告
	generateHTMLReport(report, timestamp, reportsDir)
}

func generateHTMLReport(report *TestReport, timestamp string, reportsDir string) {
	htmlContent := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>BatchFlow 集成测试报告</title>
    <style>
        body { font-family: "Microsoft YaHei", "SimHei", Arial, sans-serif; margin: 20px; }
        .header { background: #f4f4f4; padding: 20px; border-radius: 5px; }
        .summary { background: #e8f5e8; padding: 15px; margin: 20px 0; border-radius: 5px; }
        .failed { background: #ffe8e8; }
        .result { margin: 10px 0; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }
        .success { border-left: 5px solid #4CAF50; }
        .error { border-left: 5px solid #f44336; }
        table { width: 100%%; border-collapse: collapse; margin: 10px 0; }
        th, td { padding: 8px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f2f2f2; }
        .metric { display: inline-block; margin: 10px; padding: 10px; background: #f9f9f9; border-radius: 3px; }
    </style>
</head>
<body>
    <div class="header">
        <h1>🚀 BatchFlow 集成测试报告</h1>
        <p><strong>测试时间:</strong> %s</p>
        <p><strong>测试环境:</strong> %s</p>
        <p><strong>Go 版本:</strong> %s</p>
    </div>

    <div class="summary %s">
        <h2>📊 测试摘要</h2>
        <div class="metric"><strong>总测试数:</strong> %d</div>
        <div class="metric"><strong>通过:</strong> %d</div>
        <div class="metric"><strong>失败:</strong> %d</div>
        <div class="metric"><strong>总记录数:</strong> %d</div>
        <div class="metric"><strong>平均 RPS:</strong> %.2f</div>
        <div class="metric"><strong>最大 RPS:</strong> %.2f</div>
        <div class="metric"><strong>总耗时:</strong> %s</div>
    </div>

    <h2>📋 测试结果</h2>
`,
		report.Timestamp.Format(time.RFC3339), // 使用 RFC3339 格式显示时区
		report.Environment,
		report.GoVersion,
		func() string {
			if report.Summary.FailedTests > 0 {
				return "failed"
			}
			return ""
		}(),
		report.Summary.TotalTests,
		report.Summary.PassedTests,
		report.Summary.FailedTests,
		report.Summary.TotalRecords,
		report.Summary.AverageRPS,
		report.Summary.MaxRPS,
		report.Summary.TotalDuration,
	)

	for _, result := range report.Results {
		status := "success"
		statusIcon := "✅"
		if !result.Success {
			status = "error"
			statusIcon = "❌"
		}

		// 使用新的数据完整性状态
		consistencyStatus := result.DataIntegrityStatus

		actualRecordsDisplay := "N/A"
		if result.ActualRecords >= 0 {
			actualRecordsDisplay = fmt.Sprintf("%d", result.ActualRecords)
		}

		// RPS显示逻辑
		rpsDisplay := ""
		if result.RPSValid {
			rpsDisplay = fmt.Sprintf("%.2f", result.RecordsPerSecond)
		} else {
			rpsDisplay = fmt.Sprintf("<s>%.2f</s> (无效)", result.RecordsPerSecond)
		}

		htmlContent += fmt.Sprintf(`
    <div class="result %s">
        <h3>%s %s - %s</h3>
        <table>
            <tr><th>指标</th><th>数值</th></tr>
            <tr><td>测试耗时</td><td>%s</td></tr>
            <tr><td>提交记录数</td><td>%d</td></tr>
            <tr><td>数据库实际记录数</td><td>%s</td></tr>
            <tr><td>数据完整性</td><td>%s (%.1f%%)</td></tr>
            <tr><td>每秒记录数 (RPS)</td><td>%s</td></tr>
            <tr><td>RPS有效性</td><td>%s</td></tr>
            <tr><td>并发工作者数</td><td>%d</td></tr>
            <tr><td>批次大小</td><td>%d</td></tr>
            <tr><td>缓冲区大小</td><td>%d</td></tr>
            <tr><td>刷新间隔</td><td>%s</td></tr>
            <tr><td>内存分配 (MB)</td><td>%.2f</td></tr>
            <tr><td>总内存分配 (MB)</td><td>%.2f</td></tr>
            <tr><td>系统内存 (MB)</td><td>%.2f</td></tr>
            <tr><td>GC 运行次数</td><td>%d</td></tr>
            <tr><td>错误数量</td><td>%d</td></tr>
        </table>
`,
			status,
			statusIcon,
			result.Database,
			result.TestName,
			result.Duration.String(),
			result.TotalRecords,
			actualRecordsDisplay,
			consistencyStatus,
			result.DataIntegrityRate,
			rpsDisplay,
			result.RPSNote,
			result.ConcurrentWorkers,
			result.TestParameters.BatchSize,
			result.TestParameters.BufferSize,
			result.TestParameters.FlushInterval.String(),
			result.MemoryUsage.AllocMB,
			result.MemoryUsage.TotalAllocMB,
			result.MemoryUsage.SysMB,
			result.MemoryUsage.NumGC,
			len(result.Errors),
		)

		if len(result.Errors) > 0 {
			htmlContent += "<h4>错误信息:</h4><ul>"
			for i, err := range result.Errors {
				if i >= 10 { // 只显示前10个错误
					htmlContent += fmt.Sprintf("<li>... 还有 %d 个错误</li>", len(result.Errors)-10)
					break
				}
				htmlContent += fmt.Sprintf("<li>%s</li>", err)
			}
			htmlContent += "</ul>"
		}

		htmlContent += "</div>"
	}

	htmlContent += `
</body>
</html>`

	htmlFilename := fmt.Sprintf("%s/integration_test_report_%s.html", reportsDir, timestamp)
	if err := os.WriteFile(htmlFilename, []byte(htmlContent), 0o644); err != nil {
		log.Printf("❌ 保存 HTML 报告失败：%v", err)
		return
	}

	log.Printf("📊 HTML 报告已保存到：%s", htmlFilename)
}

func printSummary(report *TestReport) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🚀 BatchFlow 集成测试总结")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("📅 测试时间: %s\n", report.Timestamp.Format(time.RFC3339)) // 使用 RFC3339 显示时区
	fmt.Printf("🌍 测试环境: %s\n", report.Environment)
	fmt.Printf("🔧 Go 版本: %s\n", report.GoVersion)

	// 显示时区信息
	if name, offset := report.Timestamp.Zone(); true {
		fmt.Printf("🕒 时区: %s (偏移 %+02d:%02d)\n", name, offset/3600, (offset%3600)/60)
	}

	fmt.Println("\n📊 总体结果:")
	fmt.Printf("   总测试数: %d\n", report.Summary.TotalTests)
	fmt.Printf("   ✅ 通过: %d\n", report.Summary.PassedTests)
	fmt.Printf("   ❌ 失败: %d\n", report.Summary.FailedTests)
	fmt.Printf("   📈 总记录数: %d\n", report.Summary.TotalRecords)
	fmt.Printf("   ⚡ 平均 RPS: %.2f\n", report.Summary.AverageRPS)
	fmt.Printf("   🚀 最大 RPS: %.2f\n", report.Summary.MaxRPS)
	fmt.Printf("   ⏱️  总耗时: %s\n", report.Summary.TotalDuration)

	fmt.Println("\n📋 详细结果:")
	for _, result := range report.Results {
		status := "✅"
		if !result.Success {
			status = "❌"
		}

		// 使用新的数据完整性信息
		consistencyInfo := fmt.Sprintf(" | %s (%.1f%%)", result.DataIntegrityStatus, result.DataIntegrityRate)

		// RPS显示
		rpsInfo := ""
		if result.RPSValid {
			rpsInfo = fmt.Sprintf("RPS: %.2f", result.RecordsPerSecond)
		} else {
			rpsInfo = fmt.Sprintf("RPS: ~~%.2f~~ (无效)", result.RecordsPerSecond)
		}

		fmt.Printf("   %s %s - %s\n", status, result.Database, result.TestName)
		fmt.Printf("      耗时: %s | 提交: %d | %s | 工作者: %d | 错误: %d%s\n",
			result.Duration.String(),
			result.TotalRecords,
			rpsInfo,
			result.ConcurrentWorkers,
			len(result.Errors),
			consistencyInfo,
		)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))

	if report.Summary.FailedTests > 0 {
		fmt.Println("❌ 部分测试失败 - 请查看详细报告获取更多信息")
	} else {
		fmt.Println("🎉 所有测试通过 - BatchFlow 运行状态优秀！")
	}

	fmt.Println(strings.Repeat("=", 80))
}
