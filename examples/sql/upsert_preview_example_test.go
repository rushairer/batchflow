package sql_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/rushairer/batchflow/v2"
)

func ExampleGenerateSQLPreview_postgreSQLUpdate() {
	schema := batchflow.NewSQLSchema(
		"user_profiles",
		batchflow.ConflictUpdateOperationConfig.
			WithConflictColumns("tenant_id", "user_id").
			WithUpdateColumns("display_name", "avatar_url"),
		"tenant_id", "user_id", "display_name", "avatar_url",
	)

	preview, err := batchflow.GenerateSQLPreview(context.Background(), batchflow.DefaultPostgreSQLDriver, schema, []map[string]any{
		{"tenant_id": 100, "user_id": 42, "display_name": "alice"},
		{"tenant_id": 100, "user_id": 42, "avatar_url": "https://example.test/avatar.png"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(strings.Contains(preview.SQL, "ON CONFLICT (tenant_id, user_id) DO UPDATE"))
	fmt.Println(preview.ArgsCount)
	fmt.Println(preview.DedupStats.InputRows, preview.DedupStats.OutputRows, preview.DedupStats.MergedRows)
	fmt.Println(preview.ConflictColumns)
	fmt.Println(preview.UpdateColumns)
	// Output:
	// true
	// 4
	// 2 1 1
	// [tenant_id user_id]
	// [display_name avatar_url]
}

func ExampleGenerateSQLPreview_mySQLUpdate() {
	schema := batchflow.NewSQLSchema(
		"users",
		batchflow.ConflictUpdateOperationConfig.
			WithConflictColumns("id").
			WithUpdateColumns("name", "email"),
		"id", "name", "email",
	)

	preview, err := batchflow.GenerateSQLPreview(context.Background(), batchflow.DefaultMySQLDriver, schema, []map[string]any{
		{"id": 7, "name": "alice", "email": "alice@example.test"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(strings.Contains(preview.SQL, "ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email)"))
	fmt.Println(strings.Contains(preview.SQL, "id = VALUES(id)"))
	fmt.Println(preview.ArgsCount)
	// Output:
	// true
	// false
	// 3
}
