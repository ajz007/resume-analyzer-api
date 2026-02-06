package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nopDriver struct{}

func (d nopDriver) Open(name string) (driver.Conn, error) {
	return nopConn{}, nil
}

type nopConn struct{}

func (nopConn) Prepare(query string) (driver.Stmt, error) { return nopStmt{}, nil }
func (nopConn) Close() error                              { return nil }
func (nopConn) Begin() (driver.Tx, error)                 { return nopTx{}, nil }
func (nopConn) Ping(ctx context.Context) error            { return nil }

type nopStmt struct{}

func (nopStmt) Close() error                                   { return nil }
func (nopStmt) NumInput() int                                  { return -1 }
func (nopStmt) Exec(args []driver.Value) (driver.Result, error) { return nopResult{}, nil }
func (nopStmt) Query(args []driver.Value) (driver.Rows, error)  { return nopRows{}, nil }

type nopTx struct{}

func (nopTx) Commit() error   { return nil }
func (nopTx) Rollback() error { return nil }

type nopResult struct{}

func (nopResult) LastInsertId() (int64, error) { return 0, nil }
func (nopResult) RowsAffected() (int64, error) { return 0, nil }

type nopRows struct{}

func (nopRows) Columns() []string              { return []string{} }
func (nopRows) Close() error                   { return nil }
func (nopRows) Next(dest []driver.Value) error { return driver.ErrBadConn }

var registerTestDriverOnce sync.Once

func ensureTestDriverRegistered() {
	registerTestDriverOnce.Do(func() {
		sql.Register("dbtest", nopDriver{})
	})
}

func withTestDriver(t *testing.T) func() {
	t.Helper()
	ensureTestDriverRegistered()
	prev := openDB
	openDB = func(name, dsn string) (*sql.DB, error) {
		return sql.Open("dbtest", dsn)
	}
	return func() {
		openDB = prev
	}
}

func TestGetSingletonReturnsSamePointer(t *testing.T) {
	restore := withTestDriver(t)
	defer restore()

	singletonMu.Lock()
	singletonDB = nil
	singletonInFly = false
	singletonMu.Unlock()

	db1, err := GetSingleton(context.Background(), "ignored", DefaultLambdaOptions())
	if err != nil {
		t.Fatalf("GetSingleton first: %v", err)
	}
	db2, err := GetSingleton(context.Background(), "ignored", DefaultLambdaOptions())
	if err != nil {
		t.Fatalf("GetSingleton second: %v", err)
	}
	if db1 != db2 {
		t.Fatalf("expected singleton pointers to match")
	}
}

func TestOptionsFromEnvAppliesOverrides(t *testing.T) {
	restore := withTestDriver(t)
	defer restore()

	t.Setenv("DB_MAX_OPEN_CONNS", "7")
	t.Setenv("DB_MAX_IDLE_CONNS", "3")
	t.Setenv("DB_CONN_MAX_LIFETIME", "20m")
	t.Setenv("DB_CONN_MAX_IDLE_TIME", "45s")
	t.Setenv("DB_PING_TIMEOUT", "1s")

	opts := OptionsFromEnv(DefaultServerOptions())
	db, err := Connect(context.Background(), "ignored", opts)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != 7 {
		t.Fatalf("expected MaxOpenConnections=7, got %d", stats.MaxOpenConnections)
	}
	if opts.MaxIdleConns != 3 {
		t.Fatalf("expected MaxIdleConns=3, got %d", opts.MaxIdleConns)
	}
	if opts.ConnMaxLifetime != 20*time.Minute {
		t.Fatalf("expected ConnMaxLifetime=20m, got %s", opts.ConnMaxLifetime)
	}
	if opts.ConnMaxIdleTime != 45*time.Second {
		t.Fatalf("expected ConnMaxIdleTime=45s, got %s", opts.ConnMaxIdleTime)
	}
	if opts.PingTimeout != time.Second {
		t.Fatalf("expected PingTimeout=1s, got %s", opts.PingTimeout)
	}
}

func TestGetSingletonRetriesAfterFailure(t *testing.T) {
	var calls int32
	prev := openDB
	openDB = func(name, dsn string) (*sql.DB, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return nil, driver.ErrBadConn
		}
		ensureTestDriverRegistered()
		return sql.Open("dbtest", dsn)
	}
	defer func() {
		openDB = prev
	}()
	ensureTestDriverRegistered()

	singletonMu.Lock()
	singletonDB = nil
	singletonInFly = false
	singletonMu.Unlock()

	_, err := GetSingleton(context.Background(), "ignored", DefaultLambdaOptions())
	if err == nil {
		t.Fatalf("expected first call to fail")
	}
	db2, err := GetSingleton(context.Background(), "ignored", DefaultLambdaOptions())
	if err != nil {
		t.Fatalf("expected second call to succeed: %v", err)
	}
	if db2 == nil {
		t.Fatalf("expected db after retry")
	}
}
