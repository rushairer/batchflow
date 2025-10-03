package batchflow_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rushairer/batchflow"
)

func TestMockDriver_GenerateInsertSQL_Variants(t *testing.T) {
	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	data := []map[string]any{
		{"id": 1, "name": "a"},
		{"id": 2, "name": "b"},
	}

	tests := []struct {
		name   string
		driver *batchflow.MockDriver
		conf   batchflow.ConflictStrategy
		wantIn string
	}{
		{"mysql_base", batchflow.NewMockDriver("mysql"), batchflow.ConflictStrategy(255), "INSERT INTO users (id, name) VALUES"},
		{"mysql_ignore", batchflow.NewMockDriver("mysql"), batchflow.ConflictIgnore, "INSERT IGNORE INTO users"},
		{"mysql_replace", batchflow.NewMockDriver("mysql"), batchflow.ConflictReplace, "REPLACE INTO users"},
		{"mysql_update", batchflow.NewMockDriver("mysql"), batchflow.ConflictUpdate, "ON DUPLICATE KEY UPDATE"},
		{"pg_none", batchflow.NewMockDriver("postgresql"), batchflow.ConflictIgnore, "INSERT INTO users (id, name) VALUES"},
		{"pg_ignore", batchflow.NewMockDriver("postgresql"), batchflow.ConflictIgnore, "ON CONFLICT DO NOTHING"},
		{"pg_update", batchflow.NewMockDriver("postgresql"), batchflow.ConflictUpdate, "ON CONFLICT (id) DO UPDATE SET"},
		{"sqlite_base", batchflow.NewMockDriver("sqlite"), batchflow.ConflictStrategy(255), "INSERT INTO users (id, name) VALUES"},
		{"sqlite_ignore", batchflow.NewMockDriver("sqlite"), batchflow.ConflictIgnore, "INSERT OR IGNORE INTO users"},
		{"sqlite_replace", batchflow.NewMockDriver("sqlite"), batchflow.ConflictReplace, "INSERT OR REPLACE INTO users"},
		{"sqlite_update", batchflow.NewMockDriver("sqlite"), batchflow.ConflictUpdate, "ON CONFLICT DO UPDATE SET"},
		{"default_none", batchflow.NewMockDriver("unknown"), batchflow.ConflictIgnore, "INSERT INTO users (id, name) VALUES"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := batchflow.NewSQLSchema("users", batchflow.SQLOperationConfig{ConflictStrategy: tt.conf}, "id", "name")
			sql, args, err := tt.driver.GenerateInsertSQL(context.Background(), s, data)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !stringsContains(sql, tt.wantIn) {
				t.Fatalf("sql %q does not contain %q", sql, tt.wantIn)
			}
			if len(args) != 4 {
				t.Fatalf("args len = %d, want 4", len(args))
			}
		})
	}

	t.Run("empty_data", func(t *testing.T) {
		sql, args, err := batchflow.NewMockDriver("mysql").GenerateInsertSQL(context.Background(), schema, nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if sql != "" || args != nil {
			t.Fatalf("expect empty sql and nil args, got %q %#v", sql, args)
		}
	})

	t.Run("ctx_cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := batchflow.NewMockDriver("mysql").GenerateInsertSQL(ctx, schema, data)
		if err == nil {
			t.Fatalf("expected context error")
		}
		// reflect to ensure it's a context error (not strict type)
		if !isContextError(err) {
			t.Fatalf("expected context-related error, got %v", err)
		}
	})
}

func stringsContains(s, sub string) bool { return strings.Contains(s, sub) }
func isContextError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline")
}
