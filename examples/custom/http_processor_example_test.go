package custom_test

import (
	"context"
	"fmt"

	"github.com/rushairer/batchflow"
)

type httpRequest struct {
	method string
	path   string
	body   map[string]any
}

type httpBatchProcessor struct{}

func (httpBatchProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	ops := make(batchflow.Operations, 0, len(data))
	for _, row := range data {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ops = append(ops, httpRequest{
			method: "POST",
			path:   "/v1/events",
			body:   row,
		})
	}
	return ops, nil
}

func (p httpBatchProcessor) GenerateOperationPreview(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, batchflow.OperationPreview, error) {
	ops, err := p.GenerateOperations(ctx, schema, data)
	preview := batchflow.OperationPreview{
		Backend:     "http",
		Operation:   "post",
		Schema:      schema.Name(),
		InputItems:  len(data),
		OutputItems: len(ops),
		ArgCount:    len(ops),
		Fingerprint: batchflow.OperationFingerprint("http", "post", schema.Name(), fmt.Sprint(len(ops))),
		Attributes: map[string]any{
			"route":        "/v1/events",
			"column_count": len(schema.Columns()),
		},
	}
	return ops, preview, err
}

func (httpBatchProcessor) ExecuteOperations(ctx context.Context, operations batchflow.Operations) error {
	for _, op := range operations {
		if err := ctx.Err(); err != nil {
			return err
		}
		req := op.(httpRequest)
		_ = req.method
		_ = req.path
		_ = req.body
	}
	return nil
}

func Example_httpBatchProcessor() {
	exec := batchflow.NewThrottledBatchExecutor(httpBatchProcessor{})
	schema := batchflow.NewSchema("events", "id", "payload")

	err := exec.ExecuteBatch(context.Background(), schema, []map[string]any{
		{"id": 1, "payload": "created"},
		{"id": 2, "payload": "updated"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("sent http batch")
	// Output:
	// sent http batch
}
