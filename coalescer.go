package batchflow

import (
	"context"
	"fmt"
	"strings"
)

// CoalesceStrategy controls how duplicate keys are handled inside one batch.
type CoalesceStrategy string

const (
	CoalesceDisabled           CoalesceStrategy = "disabled"
	CoalesceKeepFirst          CoalesceStrategy = "keep_first"
	CoalesceKeepLast           CoalesceStrategy = "keep_last"
	CoalesceMergePresentFields CoalesceStrategy = "merge_present_fields"
)

// CoalesceResult contains the coalesced batch and low-cardinality statistics.
type CoalesceResult struct {
	Batch             Batch
	InputItems        int
	OutputItems       int
	DeduplicatedItems int
	MergedItems       int
}

// Coalescer merges or removes duplicate items before operation generation.
type Coalescer interface {
	Coalesce(ctx context.Context, schema SchemaInterface, batch Batch) (CoalesceResult, error)
}

type CoalescerFunc func(context.Context, SchemaInterface, Batch) (CoalesceResult, error)

func (f CoalescerFunc) Coalesce(ctx context.Context, schema SchemaInterface, batch Batch) (CoalesceResult, error) {
	if f == nil {
		return NewCoalesceResult(batch), nil
	}
	return f(ctx, schema, batch)
}

// KeyFunc returns a stable duplicate key for a record.
type KeyFunc func(schema SchemaInterface, record Record) (key string, ok bool)

// KeyCoalescer is a general key-based coalescer for custom backends.
type KeyCoalescer struct {
	Strategy   CoalesceStrategy
	KeyColumns []string
	KeyFunc    KeyFunc
}

// NewKeyCoalescer creates a key-based coalescer using schema field names.
func NewKeyCoalescer(strategy CoalesceStrategy, keyColumns ...string) *KeyCoalescer {
	return &KeyCoalescer{
		Strategy:   strategy,
		KeyColumns: append([]string(nil), keyColumns...),
	}
}

// WithKeyFunc overrides column-based key construction.
func (c *KeyCoalescer) WithKeyFunc(fn KeyFunc) *KeyCoalescer {
	c.KeyFunc = fn
	return c
}

func (c *KeyCoalescer) Coalesce(ctx context.Context, schema SchemaInterface, batch Batch) (CoalesceResult, error) {
	result := NewCoalesceResult(batch)
	if c == nil || c.Strategy == CoalesceDisabled || len(batch) < 2 {
		return result, nil
	}

	rows := make(Batch, 0, len(batch))
	seen := make(map[string]int, len(batch))
	for _, row := range batch {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		key, ok := c.key(schema, row)
		if !ok {
			rows = append(rows, cloneRecord(row))
			continue
		}
		if idx, exists := seen[key]; exists {
			switch c.Strategy {
			case CoalesceKeepFirst:
				result.DeduplicatedItems++
				continue
			case CoalesceKeepLast:
				rows[idx] = cloneRecord(row)
				result.DeduplicatedItems++
			case CoalesceMergePresentFields:
				for _, col := range schema.Columns() {
					if val, exists := row[col]; exists {
						rows[idx][col] = val
					}
				}
				result.MergedItems++
			default:
				return result, fmt.Errorf("unsupported coalesce strategy: %s", c.Strategy)
			}
			continue
		}
		seen[key] = len(rows)
		rows = append(rows, cloneRecord(row))
	}
	result.Batch = rows
	result.OutputItems = len(rows)
	return result, nil
}

func (c *KeyCoalescer) key(schema SchemaInterface, row Record) (string, bool) {
	if c.KeyFunc != nil {
		return c.KeyFunc(schema, row)
	}
	columns := c.KeyColumns
	if len(columns) == 0 && schema != nil {
		columns = schema.Columns()
	}
	if len(columns) == 0 {
		return "", false
	}
	return recordKey(row, columns), true
}

func NewCoalesceResult(batch Batch) CoalesceResult {
	return CoalesceResult{
		Batch:       batch,
		InputItems:  len(batch),
		OutputItems: len(batch),
	}
}

func cloneRecord(row Record) Record {
	cloned := make(Record, len(row))
	for k, v := range row {
		cloned[k] = v
	}
	return cloned
}

func recordKey(row Record, columns []string) string {
	var b strings.Builder
	for _, col := range columns {
		val, exists := row[col]
		fmt.Fprintf(&b, "%s=%t:%T:%#v\x00", col, exists, val, val)
	}
	return b.String()
}
