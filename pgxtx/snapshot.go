// Package pgxtx provides transaction helpers for native pgx connections.
package pgxtx

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadSnapshot runs read in a read-only, repeatable-read transaction.
// The callback must finish all queries and must not commit or roll back tx.
// The result is returned only after a successful rollback. A callback error or
// panic keeps precedence over any cleanup failure. rollbackTimeout must be positive.
func ReadSnapshot[T any](ctx context.Context, pool *pgxpool.Pool, rollbackTimeout time.Duration, read func(pgx.Tx) (T, error)) (value T, err error) {
	var zero T
	if rollbackTimeout <= 0 {
		return zero, errors.New("pgxtx: rollback timeout must be positive")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanup); err == nil && rollbackErr != nil {
			value, err = zero, rollbackErr
		}
	}()
	value, err = read(tx)
	if err != nil {
		return zero, err
	}
	return value, nil
}
