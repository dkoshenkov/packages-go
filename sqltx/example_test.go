package sqltx_test

import (
	"context"
	"database/sql"

	"github.com/dkoshenkov/packages-go/sqltx"
)

func ExampleDo() {
	var db *sql.DB // Obtain a configured database before calling this function.
	ctx := context.Background()
	err := sqltx.Do(ctx, db, nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE accounts SET active = $1 WHERE id = $2`, true, 42)
		return err
	})
	if err != nil {
		// Handle the transaction error.
		return
	}
}

func ExampleValue() {
	var db *sql.DB // Obtain a configured database before calling this function.
	ctx := context.Background()
	count, err := sqltx.Value(ctx, db, nil, func(tx *sql.Tx) (int, error) {
		var count int
		err := tx.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&count)
		return count, err
	})
	if err != nil {
		// Handle the transaction error.
		return
	}
	_ = count // Use the value after commit.
}
