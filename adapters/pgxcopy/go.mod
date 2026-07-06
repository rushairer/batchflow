module github.com/rushairer/batchflow/adapters/pgxcopy

go 1.24

require (
	github.com/jackc/pgx/v5 v5.7.6
	github.com/rushairer/batchflow/v2 v2.0.0-rc.2
)

replace github.com/rushairer/batchflow/v2 => ../..
