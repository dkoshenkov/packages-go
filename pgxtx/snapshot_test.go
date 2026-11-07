package pgxtx_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dkoshenkov/packages-go/pgxtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSnapshotRequiresPositiveRollbackTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		_, err := pgxtx.ReadSnapshot(t.Context(), nil, timeout, func(pgx.Tx) (int, error) {
			t.Fatal("callback ran")
			return 1, nil
		})
		if err == nil {
			t.Fatal("accepted invalid timeout", timeout)
		}
	}
}

func TestPostgresSnapshot(t *testing.T) {
	dsn := os.Getenv("PGXTX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PGXTX_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	writer, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := writer.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	table := pgx.Identifier{fmt.Sprintf("pgxtx_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := writer.Exec(ctx, "CREATE TABLE "+table+" (value int)"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := writer.Exec(ctx, "DROP TABLE "+table); err != nil {
			t.Error(err)
		}
	}()
	if _, err := writer.Exec(ctx, "INSERT INTO "+table+" VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	got, err := pgxtx.ReadSnapshot(ctx, pool, time.Second, func(tx pgx.Tx) (int, error) {
		var first, second int
		if err := tx.QueryRow(ctx, "SELECT value FROM "+table).Scan(&first); err != nil {
			return 0, err
		}
		if _, err := writer.Exec(ctx, "UPDATE "+table+" SET value=2"); err != nil {
			return 0, err
		}
		if err := tx.QueryRow(ctx, "SELECT value FROM "+table).Scan(&second); err != nil {
			return 0, err
		}
		if first != 1 || first != second {
			return 0, fmt.Errorf("inconsistent snapshot: %d then %d", first, second)
		}
		return second, nil
	})
	if got != 1 || err != nil {
		t.Fatal(got, err)
	}
	got, err = pgxtx.ReadSnapshot(ctx, pool, time.Second, func(tx pgx.Tx) (int, error) {
		_, err := tx.Exec(ctx, "UPDATE "+table+" SET value=3")
		return 99, err
	})
	var pgerr *pgconn.PgError
	if got != 0 || !errors.As(err, &pgerr) || pgerr.Code != "25006" {
		t.Fatal("expected read-only SQLSTATE 25006", got, err)
	}

	pid := func() int {
		t.Helper()
		var pid int
		if err := pool.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
			t.Fatal(err)
		}
		return pid
	}
	before := pid()
	canceled, stop := context.WithCancel(ctx)
	got, err = pgxtx.ReadSnapshot(canceled, pool, time.Second, func(pgx.Tx) (int, error) {
		stop()
		return 12, context.Canceled
	})
	if got != 0 || !errors.Is(err, context.Canceled) || before != pid() {
		t.Fatal("canceled read did not release its connection", got, err)
	}
	got, err = pgxtx.ReadSnapshot(canceled, pool, time.Nanosecond, func(pgx.Tx) (int, error) {
		t.Fatal("callback after begin failure")
		return 5, nil
	})
	if got != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal("begin", got, err)
	}

	if recovered := func() (p any) {
		defer func() { p = recover() }()
		if _, err := pgxtx.ReadSnapshot(ctx, pool, time.Second, func(pgx.Tx) (int, error) { panic("snapshot panic") }); err != nil {
			t.Fatal(err)
		}
		return nil
	}(); recovered != "snapshot panic" || before != pid() {
		t.Fatal("panic did not preserve panic and connection", recovered)
	}

	original := errors.New("domain error")
	for _, callbackErr := range []error{nil, original} {
		got, err = pgxtx.ReadSnapshot(ctx, pool, time.Second, func(tx pgx.Tx) (int, error) {
			// Close the server connection while the snapshot is open; rollback must fail.
			if err := tx.Conn().Close(ctx); err != nil {
				return 0, err
			}
			return 99, callbackErr
		})
		if got != 0 || err == nil || (callbackErr != nil && !errors.Is(err, callbackErr)) {
			t.Fatal("failed rollback", got, err)
		}
		_ = pid() // MaxConns=1: the unusable connection must be released/replaced.
	}
}

type rollbackDeadline struct {
	expires  time.Time
	canceled bool
}

func (p *rollbackDeadline) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if d.SQL == "rollback" {
		p.expires, _ = ctx.Deadline()
		p.canceled = ctx.Err() != nil
	}
	return ctx
}

func (*rollbackDeadline) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestPostgresSnapshotCleanupDeadline(t *testing.T) {
	dsn := os.Getenv("PGXTX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PGXTX_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	observation := new(rollbackDeadline)
	cfg.ConnConfig.Tracer = observation
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	const budget = 250 * time.Millisecond
	var returned time.Time
	_, err = pgxtx.ReadSnapshot(ctx, pool, budget, func(pgx.Tx) (int, error) {
		cancel()
		returned = time.Now()
		return 0, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// A real ROLLBACK must carry a fresh deadline even when the caller has canceled.
	if observation.canceled || observation.expires.Before(returned.Add(budget)) || observation.expires.After(time.Now().Add(budget)) {
		t.Fatalf("rollback used incorrect context: canceled=%v deadline=%v", observation.canceled, observation.expires)
	}
}
