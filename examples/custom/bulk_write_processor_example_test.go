package custom_test

import (
	"context"
	"fmt"
	"strconv"

	"github.com/rushairer/batchflow"
)

type bulkWriteOperation struct {
	collection string
	document   map[string]any
}

type documentBulkProcessor struct{}

func (documentBulkProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	ops := make(batchflow.Operations, 0, len(data))
	for _, row := range data {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ops = append(ops, bulkWriteOperation{
			collection: schema.Name(),
			document:   row,
		})
	}
	return ops, nil
}

func (p documentBulkProcessor) GenerateOperationPreview(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, batchflow.OperationPreview, error) {
	ops, err := p.GenerateOperations(ctx, schema, data)
	preview := batchflow.OperationPreview{
		Backend:     "document",
		Operation:   "bulk_write",
		Schema:      schema.Name(),
		InputItems:  len(data),
		OutputItems: len(ops),
		ArgCount:    len(ops),
		Fingerprint: batchflow.OperationFingerprint("document", "bulk_write", schema.Name(), strconv.Itoa(len(ops))),
		Attributes: map[string]any{
			"collection":   schema.Name(),
			"column_count": len(schema.Columns()),
		},
	}
	return ops, preview, err
}

func (documentBulkProcessor) ExecuteOperations(ctx context.Context, operations batchflow.Operations) error {
	for _, op := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		write := op.(bulkWriteOperation)
		_ = write.collection
		_ = write.document
	}
	return nil
}

func Example_documentBulkProcessor() {
	exec := batchflow.NewThrottledBatchExecutor(documentBulkProcessor{}).
		WithConcurrencyLimit(4)
	schema := batchflow.NewSchema("profiles", "user_id", "name")

	err := exec.ExecuteBatch(context.Background(), schema, []map[string]any{
		{"user_id": 1, "name": "alice"},
		{"user_id": 2, "name": "bob"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("wrote document batch")
	// Output:
	// wrote document batch
}
