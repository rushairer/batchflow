package batchflow

// Record is a single batch item keyed by field or column name.
type Record = map[string]any

// Batch is the backend-neutral batch payload passed through BatchFlow.
type Batch = []Record
