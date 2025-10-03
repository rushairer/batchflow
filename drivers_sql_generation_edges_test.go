package batchflow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rushairer/batchflow"
)

func TestSQLGeneration_Edges(t *testing.T) {
	type tc struct {
		name   string
		driver batchflow.SQLDriver
		schema batchflow.SchemaInterface
		data   []map[string]any
		check  func(sql string, args []any)
	}
	usersSchema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")

	tests := []tc{
		{
			name:   "MySQL_empty_data_no_panic",
			driver: batchflow.DefaultMySQLDriver,
			schema: usersSchema,
			data:   []map[string]any{},
			check: func(sql string, args []any) {
				if sql == "" && len(args) == 0 {
					// 允许空，关键是不 panic，函数外层应处理空批
					// 添加有效语句避免 staticcheck SA9003 空分支告警
					t.Log("empty batch allowed")
					_ = sql
					_ = args
				}
			},
		},
		{
			name:   "Postgres_placeholders_and_order",
			driver: batchflow.DefaultPostgreSQLDriver,
			schema: usersSchema,
			data: []map[string]any{
				{"id": 1, "name": "a"},
				{"id": 2, "name": "b"},
			},
			check: func(sql string, args []any) {
				if !strings.Contains(sql, "VALUES ($1, $2), ($3, $4)") {
					t.Fatalf("unexpected pg placeholders: %s", sql)
				}
				if len(args) != 4 || args[0] != 1 || args[1] != "a" || args[2] != 2 || args[3] != "b" {
					t.Fatalf("unexpected args: %#v", args)
				}
			},
		},
		{
			name:   "SQLite_special_chars_escape",
			driver: batchflow.DefaultSQLiteDriver,
			schema: usersSchema,
			data: []map[string]any{
				{"id": 1, "name": "O'Reilly"},
			},
			check: func(sql string, args []any) {
				// 使用参数占位，SQL 不直接含原始字符串，args 含原值
				if !strings.Contains(sql, "INSERT OR IGNORE INTO users (id, name) VALUES (?, ?)") {
					t.Fatalf("unexpected sqlite insert: %s", sql)
				}
				if len(args) != 2 || args[1] != "O'Reilly" {
					t.Fatalf("unexpected args: %#v", args)
				}
			},
		},
		{
			name:   "MySQL_column_order_and_case",
			driver: batchflow.DefaultMySQLDriver,
			schema: batchflow.NewSQLSchema("Users", batchflow.ConflictIgnoreOperationConfig, "ID", "Name"),
			data: []map[string]any{
				{"ID": 10, "Name": "X"},
			},
			check: func(sql string, args []any) {
				// 列顺序应与 Schema 一致
				if !strings.Contains(sql, "INSERT IGNORE INTO Users (ID, Name) VALUES (?, ?)") {
					t.Fatalf("unexpected mysql sql: %s", sql)
				}
				if len(args) != 2 || args[0] != 10 || args[1] != "X" {
					t.Fatalf("unexpected args: %#v", args)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := tt.driver.GenerateInsertSQL(context.Background(), tt.schema.(*batchflow.SQLSchema), tt.data)
			if err != nil && len(tt.data) > 0 { // 空数据允许外层处理，这里只断言非空批应无错
				t.Fatalf("generate sql failed: %v", err)
			}
			if tt.check != nil {
				tt.check(sql, args)
			}
		})
	}
}
