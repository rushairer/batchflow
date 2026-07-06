package pgxcopy

import (
	"context"
	"strings"

	batchflow "github.com/rushairer/batchflow/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolAdapter adapts pgxpool.Pool to batchflow.CopyFromClient.
type PoolAdapter struct {
	pool *pgxpool.Pool
}

var _ batchflow.CopyFromClient = (*PoolAdapter)(nil)

func NewPoolAdapter(pool *pgxpool.Pool) *PoolAdapter {
	return &PoolAdapter{pool: pool}
}

func NewExecutor(pool *pgxpool.Pool) *batchflow.CopyFromExecutor {
	return batchflow.NewCopyFromExecutor(NewPoolAdapter(pool))
}

func (a *PoolAdapter) CopyFrom(ctx context.Context, table string, columns []string, rows [][]any) (int64, error) {
	conn, err := a.pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()

	return conn.CopyFrom(ctx, parseIdentifier(table), columns, pgx.CopyFromRows(rows))
}

func parseIdentifier(name string) pgx.Identifier {
	parts := strings.Split(name, ".")
	identifier := make(pgx.Identifier, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		if part != "" {
			identifier = append(identifier, part)
		}
	}
	if len(identifier) == 0 {
		return pgx.Identifier{name}
	}
	return identifier
}
