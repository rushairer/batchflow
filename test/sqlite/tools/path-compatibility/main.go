package main

import (
	"fmt"
	"os"
)

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

func main() {
	fmt.Println("🔍 测试路径兼容性...")

	reportsDir := getReportsDirectory()
	fmt.Printf("📁 检测到的报告目录: %s\n", reportsDir)

	// 测试创建目录
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		fmt.Printf("❌ 无法创建目录 %s: %v\n", reportsDir, err)
	} else {
		fmt.Printf("✅ 成功创建目录: %s\n", reportsDir)

		// 测试写入文件
		testFile := fmt.Sprintf("%s/test_file.txt", reportsDir)
		if file, err := os.Create(testFile); err != nil {
			fmt.Printf("❌ 无法创建测试文件: %v\n", err)
		} else {
			file.WriteString("测试内容")
			file.Close()
			fmt.Printf("✅ 成功创建测试文件: %s\n", testFile)

			// 清理测试文件
			os.Remove(testFile)
			fmt.Printf("🧹 已清理测试文件\n")
		}
	}

	// 环境检测
	fmt.Println("\n🌍 环境信息:")
	if _, err := os.Stat("/app"); err == nil {
		fmt.Println("   - 检测到 /app 目录存在")
		if file, err := os.Create("/app/.write_test"); err == nil {
			file.Close()
			os.Remove("/app/.write_test")
			fmt.Println("   - /app 目录可写 → Docker 环境")
		} else {
			fmt.Printf("   - /app 目录不可写: %v → 受限环境\n", err)
		}
	} else {
		fmt.Println("   - /app 目录不存在 → 本地环境")
	}

	fmt.Println("\n🎯 结论:")
	if reportsDir == "/app/reports" {
		fmt.Println("   使用 Docker 路径: /app/reports")
	} else {
		fmt.Println("   使用本地路径: reports/")
	}
}
