package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// clearSQLiteTableByRecreate SQLite专用的重建表清理方式
func clearSQLiteTableByRecreate(db *sql.DB) error {
	// 1. 删除表
	if _, err := db.Exec("DROP TABLE IF EXISTS integration_test"); err != nil {
		return fmt.Errorf("failed to drop table: %v", err)
	}

	// 2. 重新创建表
	createSQL := `
	CREATE TABLE integration_test (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		data TEXT,
		value REAL,
		is_active INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	if _, err := db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// 3. 重新创建索引
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_name ON integration_test(name)",
		"CREATE INDEX IF NOT EXISTS idx_email ON integration_test(email)",
	}

	for _, indexSQL := range indexes {
		if _, err := db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	log.Printf("  ✅ SQLite table recreated successfully")
	return nil
}

// 旧的清理方式
func clearTableOldWay(db *sql.DB) error {
	_, err := db.Exec("DELETE FROM integration_test; VACUUM;")
	return err
}

func main() {
	// 创建 SQLite 数据库
	db, err := sql.Open("sqlite3", "../../data/benchmark_test.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	// 创建表
	createSQL := `
	CREATE TABLE IF NOT EXISTS integration_test (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		data TEXT,
		value REAL,
		is_active INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	if _, err := db.Exec(createSQL); err != nil {
		log.Fatal("Failed to create table:", err)
	}

	// 插入测试数据
	fmt.Println("🔄 插入测试数据...")
	for i := 0; i < 10000; i++ {
		_, err := db.Exec(`
			INSERT INTO integration_test (name, email, data, value, is_active) 
			VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("User_%d", i),
			fmt.Sprintf("user_%d@example.com", i),
			fmt.Sprintf("Data_%d", i),
			float64(i)/100.0,
			i%2 == 0,
		)
		if err != nil {
			log.Printf("Failed to insert record %d: %v", i, err)
		}
	}

	// 检查记录数
	var count int
	db.QueryRow("SELECT COUNT(*) FROM integration_test").Scan(&count)
	fmt.Printf("📊 插入了 %d 条记录\n", count)

	// 测试旧方式清理
	fmt.Println("\n🧪 测试旧方式清理 (DELETE + VACUUM)...")
	start := time.Now()
	if err := clearTableOldWay(db); err != nil {
		log.Printf("❌ 旧方式清理失败: %v", err)
	} else {
		duration := time.Since(start)
		db.QueryRow("SELECT COUNT(*) FROM integration_test").Scan(&count)
		fmt.Printf("✅ 旧方式清理完成，耗时: %v，剩余记录: %d\n", duration, count)
	}

	// 重新插入数据
	fmt.Println("\n🔄 重新插入测试数据...")
	for i := 0; i < 10000; i++ {
		db.Exec(`
			INSERT INTO integration_test (name, email, data, value, is_active) 
			VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("User_%d", i),
			fmt.Sprintf("user_%d@example.com", i),
			fmt.Sprintf("Data_%d", i),
			float64(i)/100.0,
			i%2 == 0,
		)
	}

	// 测试新方式清理
	fmt.Println("\n🧪 测试新方式清理 (重建表)...")
	start = time.Now()
	if err := clearSQLiteTableByRecreate(db); err != nil {
		log.Printf("❌ 新方式清理失败: %v", err)
	} else {
		duration := time.Since(start)
		db.QueryRow("SELECT COUNT(*) FROM integration_test").Scan(&count)
		fmt.Printf("✅ 新方式清理完成，耗时: %v，剩余记录: %d\n", duration, count)
	}

	fmt.Println("\n🎉 测试完成！新方式应该更快且不会出现锁定问题。")
}
