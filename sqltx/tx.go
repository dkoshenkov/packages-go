package sqltx

import (
	"context"
	"database/sql"
	"errors"
)

// Do runs fn in a transaction and commits it when fn succeeds.
// A nil opts uses the database driver's default transaction options.
// The callback must not commit or roll back the transaction itself.
func Do(ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(*sql.Tx) error) error {
	_, err := Value(ctx, db, opts, func(tx *sql.Tx) (struct{}, error) {
		return struct{}{}, fn(tx)
	})
	return err
}

// Value runs fn in a transaction and returns its value only after a successful commit.
// On error, it returns the zero value of T. A nil opts uses the database
// driver's default transaction options. The callback must not commit or roll
// back the transaction itself.
func Value[T any](ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(*sql.Tx) (T, error)) (value T, err error) {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return value, err
	}

	committed := false
	defer func() {
		if !committed {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && err != nil {
				err = errors.Join(err, rollbackErr)
			}
		}
	}()

	result, err := fn(tx)
	if err != nil {
		return value, err
	}
	if err = tx.Commit(); err != nil {
		return value, err
	}
	committed = true
	return result, nil
}
