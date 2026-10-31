package sqltx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

func TestValueCommitAndOptions(t *testing.T) {
	state := &driverState{}
	db := openTestDB(t, state)
	options := &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true}
	called := false
	value, err := Value(context.Background(), db, options, func(tx *sql.Tx) (string, error) {
		called = tx != nil
		return "secret", nil
	})
	if err != nil || value != "secret" || !called {
		t.Fatalf("value=%q called=%v err=%v", value, called, err)
	}
	if state.begins != 1 || state.commits != 1 || state.rollbacks != 0 {
		t.Fatalf("unexpected transaction counts: %+v", state)
	}
	if state.options.Isolation != driver.IsolationLevel(sql.LevelSerializable) || !state.options.ReadOnly {
		t.Fatalf("options: %+v", state.options)
	}
}

func TestValueBeginFailureAndCancellation(t *testing.T) {
	beginErr := errors.New("begin failed")
	for _, tc := range []struct {
		name     string
		beginErr error
		canceled bool
		want     error
	}{
		{"begin", beginErr, false, beginErr},
		{"canceled", nil, true, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &driverState{beginErr: tc.beginErr}
			db := openTestDB(t, state)
			ctx := context.Background()
			if tc.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			called := false
			value, err := Value(ctx, db, nil, func(*sql.Tx) (string, error) {
				called = true
				return "secret", nil
			})
			if !errors.Is(err, tc.want) || value != "" || called {
				t.Fatalf("value=%q called=%v err=%v", value, called, err)
			}
			if state.commits != 0 || state.rollbacks != 0 {
				t.Fatalf("transaction completed after begin failure: %+v", state)
			}
		})
	}
}

func TestValueCallbackErrorAndRollbackError(t *testing.T) {
	callbackErr := errors.New("callback failed")
	rollbackErr := errors.New("rollback failed")
	for _, tc := range []struct {
		name        string
		rollbackErr error
	}{{"rollback succeeds", nil}, {"rollback fails", rollbackErr}} {
		t.Run(tc.name, func(t *testing.T) {
			state := &driverState{rollbackErr: tc.rollbackErr}
			db := openTestDB(t, state)
			value, err := Value(context.Background(), db, nil, func(*sql.Tx) (string, error) { return "secret", callbackErr })
			if value != "" || !errors.Is(err, callbackErr) || (tc.rollbackErr != nil && !errors.Is(err, tc.rollbackErr)) {
				t.Fatalf("value=%q err=%v", value, err)
			}
			if state.begins != 1 || state.commits != 0 || state.rollbacks != 1 {
				t.Fatalf("counts: %+v", state)
			}
		})
	}
}

func TestValueCommitFailureReturnsZero(t *testing.T) {
	commitErr := errors.New("commit failed")
	state := &driverState{commitErr: commitErr}
	db := openTestDB(t, state)
	value, err := Value(context.Background(), db, nil, func(*sql.Tx) (string, error) { return "secret", nil })
	if value != "" || !errors.Is(err, commitErr) {
		t.Fatalf("value=%q err=%v", value, err)
	}
	// database/sql marks a transaction done after Commit, even when Commit fails.
	if state.commits != 1 || state.rollbacks != 0 {
		t.Fatalf("counts: %+v", state)
	}
}

func TestValuePanicRollsBackAndPropagates(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rollbackErr error
	}{
		{"rollback succeeds", nil},
		{"rollback fails", errors.New("rollback failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &driverState{rollbackErr: tc.rollbackErr}
			db := openTestDB(t, state)
			marker := new(int)
			func() {
				defer func() {
					if recovered := recover(); recovered != marker {
						t.Errorf("panic = %v", recovered)
					}
				}()
				value, err := Value(context.Background(), db, nil, func(*sql.Tx) (int, error) { panic(marker) })
				t.Errorf("callback did not panic: value=%d err=%v", value, err)
			}()
			if state.commits != 0 || state.rollbacks != 1 {
				t.Fatalf("counts: %+v", state)
			}
		})
	}
}

func TestDoCommitAndFailure(t *testing.T) {
	callbackErr := errors.New("callback failed")
	rollbackErr := errors.New("rollback failed")
	for _, tc := range []struct {
		name                     string
		callbackErr, rollbackErr error
		commits, rollbacks       int
	}{
		{"success", nil, nil, 1, 0},
		{"callback error", callbackErr, nil, 0, 1},
		{"rollback error", callbackErr, rollbackErr, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &driverState{rollbackErr: tc.rollbackErr}
			db := openTestDB(t, state)
			err := Do(context.Background(), db, nil, func(*sql.Tx) error { return tc.callbackErr })
			if !errors.Is(err, tc.callbackErr) || (tc.rollbackErr != nil && !errors.Is(err, tc.rollbackErr)) {
				t.Fatalf("err=%v", err)
			}
			if state.commits != tc.commits || state.rollbacks != tc.rollbacks {
				t.Fatalf("counts: %+v", state)
			}
		})
	}
}

func TestValueCanceledDuringCallback(t *testing.T) {
	state := &driverState{rollbackDone: make(chan struct{})}
	db := openTestDB(t, state)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	value, err := Value(ctx, db, nil, func(*sql.Tx) (string, error) {
		cancel()
		return "secret", nil
	})
	if value != "" || (!errors.Is(err, context.Canceled) && !errors.Is(err, sql.ErrTxDone)) {
		t.Fatalf("value=%q err=%v", value, err)
	}
	select {
	case <-state.rollbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("rollback did not complete")
	}
}

func TestDoBeginCommitAndPanic(t *testing.T) {
	beginErr := errors.New("begin failed")
	commitErr := errors.New("commit failed")
	for _, tc := range []struct {
		name                string
		beginErr, commitErr error
		want                error
	}{
		{"begin", beginErr, nil, beginErr},
		{"commit", nil, commitErr, commitErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &driverState{beginErr: tc.beginErr, commitErr: tc.commitErr}
			db := openTestDB(t, state)
			called := false
			err := Do(context.Background(), db, nil, func(*sql.Tx) error {
				called = true
				return nil
			})
			if !errors.Is(err, tc.want) || called != (tc.beginErr == nil) {
				t.Fatalf("called=%v err=%v", called, err)
			}
			if state.rollbacks != 0 {
				t.Fatalf("unexpected rollback: %+v", state)
			}
		})
	}
	state := &driverState{}
	db := openTestDB(t, state)
	marker := new(int)
	func() {
		defer func() {
			if recovered := recover(); recovered != marker {
				t.Errorf("panic = %v", recovered)
			}
		}()
		err := Do(context.Background(), db, nil, func(*sql.Tx) error { panic(marker) })
		t.Errorf("callback did not panic: %v", err)
	}()
	if state.commits != 0 || state.rollbacks != 1 {
		t.Fatalf("counts: %+v", state)
	}
}

type driverState struct {
	beginErr, commitErr, rollbackErr error
	begins, commits, rollbacks       int
	options                          driver.TxOptions
	rollbackDone                     chan struct{}
}

type testConnector struct{ state *driverState }

type testDriver struct{}

type testConn struct{ state *driverState }

type testTx struct{ state *driverState }

func openTestDB(t *testing.T, state *driverState) *sql.DB {
	t.Helper()
	db := sql.OpenDB(testConnector{state})
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func (testConnector) Driver() driver.Driver                          { return testDriver{} }
func (c testConnector) Connect(context.Context) (driver.Conn, error) { return testConn(c), nil }
func (testDriver) Open(string) (driver.Conn, error)                  { return nil, errors.New("unexpected Open") }
func (testConn) Prepare(string) (driver.Stmt, error)                 { return nil, errors.New("unexpected Prepare") }
func (testConn) Close() error                                        { return nil }
func (testConn) Begin() (driver.Tx, error)                           { return nil, errors.New("unexpected Begin") }
func (c testConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.state.begins++
	c.state.options = opts
	if c.state.beginErr != nil {
		return nil, c.state.beginErr
	}
	return testTx(c), nil
}

func (tx testTx) Commit() error {
	tx.state.commits++
	return tx.state.commitErr
}

func (tx testTx) Rollback() error {
	tx.state.rollbacks++
	if tx.state.rollbackDone != nil {
		close(tx.state.rollbackDone)
	}
	return tx.state.rollbackErr
}
