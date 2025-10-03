package batchflow_test

import (
	"testing"

	"github.com/rushairer/batchflow"
)

func TestNewSchema_Basic(t *testing.T) {
	s := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")
	if s.Name() != "users" {
		t.Fatalf("schema name expected users, got %s", s.Name())
	}
	columns := s.Columns()
	if len(columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(columns))
	}
	if columns[0] != "id" || columns[1] != "name" || columns[2] != "email" {
		t.Fatalf("columns order unexpected: %#v", columns)
	}
}
