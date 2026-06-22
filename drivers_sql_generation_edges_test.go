package batchflow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rushairer/batchflow/v2"
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

func TestPostgreSQLConflictColumnsAndUpdateColumns(t *testing.T) {
	cfg := batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("id", "tenant_id").
		WithUpdateColumns("name")
	schema := batchflow.NewSQLSchema("users", cfg, "id", "tenant_id", "name", "email")

	sql, args, err := batchflow.DefaultPostgreSQLDriver.GenerateInsertSQL(context.Background(), schema, []map[string]any{
		{"id": 1, "tenant_id": 10, "name": "alice", "email": "a@example.com"},
	})
	if err != nil {
		t.Fatalf("generate sql failed: %v", err)
	}
	if !strings.Contains(sql, "ON CONFLICT (id, tenant_id) DO UPDATE SET name = EXCLUDED.name") {
		t.Fatalf("unexpected postgres conflict/update sql: %s", sql)
	}
	if strings.Contains(sql, "id = EXCLUDED.id") || strings.Contains(sql, "tenant_id = EXCLUDED.tenant_id") || strings.Contains(sql, "email = EXCLUDED.email") {
		t.Fatalf("postgres update should only include configured non-conflict columns: %s", sql)
	}
	if len(args) != 4 {
		t.Fatalf("args len = %d, want 4: %#v", len(args), args)
	}
}

func TestPostgreSQLReplaceUpdatesAllNonConflictColumns(t *testing.T) {
	cfg := batchflow.ConflictReplaceOperationConfig.WithConflictColumns("id")
	schema := batchflow.NewSQLSchema("users", cfg, "id", "name", "email")

	sql, _, err := batchflow.DefaultPostgreSQLDriver.GenerateInsertSQL(context.Background(), schema, []map[string]any{
		{"id": 1, "name": "alice", "email": "a@example.com"},
	})
	if err != nil {
		t.Fatalf("generate sql failed: %v", err)
	}
	if !strings.Contains(sql, "ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email") {
		t.Fatalf("unexpected postgres replace sql: %s", sql)
	}
	if strings.Contains(sql, "id = EXCLUDED.id") {
		t.Fatalf("postgres replace should not update conflict columns: %s", sql)
	}
}

func TestSQLDeduplicateByConflictColumns(t *testing.T) {
	tests := []struct {
		name     string
		cfg      batchflow.SQLOperationConfig
		wantArgs []any
	}{
		{
			name: "ignore keeps first row",
			cfg:  batchflow.ConflictIgnoreOperationConfig.WithConflictColumns("id"),
			wantArgs: []any{
				1, "first", "first@example.com",
				2, "other", "other@example.com",
			},
		},
		{
			name: "replace keeps last row",
			cfg:  batchflow.ConflictReplaceOperationConfig.WithConflictColumns("id"),
			wantArgs: []any{
				1, "second", nil,
				2, "other", "other@example.com",
			},
		},
		{
			name: "update merges later present columns",
			cfg:  batchflow.ConflictUpdateOperationConfig.WithConflictColumns("id"),
			wantArgs: []any{
				1, "second", "first@example.com",
				2, "other", "other@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := batchflow.NewSQLSchema("users", tt.cfg, "id", "name", "email")
			_, args, err := batchflow.DefaultPostgreSQLDriver.GenerateInsertSQL(context.Background(), schema, []map[string]any{
				{"id": 1, "name": "first", "email": "first@example.com"},
				{"id": 1, "name": "second"},
				{"id": 2, "name": "other", "email": "other@example.com"},
			})
			if err != nil {
				t.Fatalf("generate sql failed: %v", err)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args len = %d, want %d: %#v", len(args), len(tt.wantArgs), args)
			}
			for i := range tt.wantArgs {
				if args[i] != tt.wantArgs[i] {
					t.Fatalf("args[%d] = %#v, want %#v; args=%#v", i, args[i], tt.wantArgs[i], args)
				}
			}
		})
	}
}

func TestMySQLConflictUpdateSkipsConflictColumns(t *testing.T) {
	cfg := batchflow.ConflictUpdateOperationConfig.WithConflictColumns("id")
	schema := batchflow.NewSQLSchema("users", cfg, "id", "name", "email")

	sql, _, err := batchflow.DefaultMySQLDriver.GenerateInsertSQL(context.Background(), schema, []map[string]any{
		{"id": 1, "name": "alice", "email": "a@example.com"},
	})
	if err != nil {
		t.Fatalf("generate sql failed: %v", err)
	}
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email)") {
		t.Fatalf("unexpected mysql update sql: %s", sql)
	}
	if strings.Contains(sql, "id = VALUES(id)") {
		t.Fatalf("mysql update should not update conflict columns: %s", sql)
	}
}

func TestSQLConflictColumnsFallbackToFirstColumn(t *testing.T) {
	schema := batchflow.NewSQLSchema("users", batchflow.ConflictUpdateOperationConfig, "id", "name")

	sql, _, err := batchflow.DefaultPostgreSQLDriver.GenerateInsertSQL(context.Background(), schema, []map[string]any{
		{"id": 1, "name": "alice"},
	})
	if err != nil {
		t.Fatalf("generate sql failed: %v", err)
	}
	if !strings.Contains(sql, "ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name") {
		t.Fatalf("expected first column conflict fallback, got: %s", sql)
	}
}
