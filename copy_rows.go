package batchflow

import (
	"context"
	"sync"
)

const copyRowsMaxPooledValues = 1 << 20

type copyRowsBuffer struct {
	rows [][]any
	flat []any
}

var copyRowsPool = sync.Pool{
	New: func() any {
		return &copyRowsBuffer{}
	},
}

func acquireCopyRows(ctx context.Context, columns []string, data []map[string]any) (*copyRowsBuffer, error) {
	buf := copyRowsPool.Get().(*copyRowsBuffer)
	rowCount := len(data)
	colCount := len(columns)
	valueCount := rowCount * colCount

	if cap(buf.rows) < rowCount {
		buf.rows = make([][]any, rowCount)
	} else {
		buf.rows = buf.rows[:rowCount]
	}
	if cap(buf.flat) < valueCount {
		buf.flat = make([]any, valueCount)
	} else {
		buf.flat = buf.flat[:valueCount]
	}

	for i, row := range data {
		if i&1023 == 0 {
			if err := ctx.Err(); err != nil {
				releaseCopyRows(buf)
				return nil, err
			}
		}
		start := i * colCount
		values := buf.flat[start : start+colCount]
		for j, col := range columns {
			values[j] = row[col]
		}
		buf.rows[i] = values
	}
	return buf, nil
}

func releaseCopyRows(buf *copyRowsBuffer) {
	if buf == nil {
		return
	}
	for i := range buf.flat {
		buf.flat[i] = nil
	}
	for i := range buf.rows {
		buf.rows[i] = nil
	}
	if cap(buf.flat) > copyRowsMaxPooledValues {
		buf.flat = nil
	}
	copyRowsPool.Put(buf)
}
