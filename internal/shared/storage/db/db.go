package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // register pgx as database/sql driver
)

// Options controls database pool and connectivity behavior.
type Options struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

var (
	openDB         = sql.Open
	singletonMu    sync.Mutex
	singletonCond  = sync.NewCond(&singletonMu)
	singletonDB    *sql.DB
	singletonInFly bool
)

// IsLambdaRuntime reports whether the current process is running in AWS Lambda.
func IsLambdaRuntime() bool {
	return strings.TrimSpace(os.Getenv("AWS_LAMBDA_FUNCTION_NAME")) != ""
}

// DefaultLambdaOptions returns conservative defaults for Lambda concurrency.
func DefaultLambdaOptions() Options {
	return Options{
		MaxOpenConns:    2,
		MaxIdleConns:    1,
		ConnMaxIdleTime: 30 * time.Second,
		ConnMaxLifetime: 15 * time.Minute,
		PingTimeout:     3 * time.Second,
	}
}

// DefaultServerOptions returns defaults for long-running server processes.
func DefaultServerOptions() Options {
	return Options{
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxIdleTime: 2 * time.Minute,
		ConnMaxLifetime: time.Hour,
		PingTimeout:     5 * time.Second,
	}
}

// DefaultMigrateOptions returns defaults for short-lived CLI migrations.
func DefaultMigrateOptions() Options {
	return Options{
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxIdleTime: 2 * time.Minute,
		ConnMaxLifetime: time.Hour,
		PingTimeout:     5 * time.Second,
	}
}

// OptionsFromEnv overrides defaults with DB_* env vars if present.
func OptionsFromEnv(defaults Options) Options {
	opts := defaults
	if v, ok := readEnvInt("DB_MAX_OPEN_CONNS"); ok {
		opts.MaxOpenConns = v
	}
	if v, ok := readEnvInt("DB_MAX_IDLE_CONNS"); ok {
		opts.MaxIdleConns = v
	}
	if v, ok := readEnvDuration("DB_CONN_MAX_LIFETIME"); ok {
		opts.ConnMaxLifetime = v
	}
	if v, ok := readEnvDuration("DB_CONN_MAX_IDLE_TIME"); ok {
		opts.ConnMaxIdleTime = v
	}
	if v, ok := readEnvDuration("DB_PING_TIMEOUT"); ok {
		opts.PingTimeout = v
	}
	return opts
}

// Connect opens a *sql.DB using the provided DATABASE_URL and verifies connectivity.
// The returned *sql.DB should be shared and re-used by callers.
func Connect(ctx context.Context, databaseURL string, opts Options) (*sql.DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}

	db, err := openDB("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	applyOptions(db, opts)

	pingTimeout := opts.PingTimeout
	if pingTimeout <= 0 {
		pingTimeout = 5 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	logPoolStats(db, "db init")
	return db, nil
}

// GetSingleton returns a process-wide *sql.DB, initializing it once per execution environment.
// If initialization fails, a later call will retry until successful.
func GetSingleton(ctx context.Context, databaseURL string, opts Options) (*sql.DB, error) {
	singletonMu.Lock()
	if singletonDB != nil {
		singletonMu.Unlock()
		log.Printf("db singleton reuse")
		return singletonDB, nil
	}
	if singletonInFly {
		for singletonInFly && singletonDB == nil {
			singletonCond.Wait()
		}
		if singletonDB != nil {
			singletonMu.Unlock()
			log.Printf("db singleton reuse")
			return singletonDB, nil
		}
	}
	singletonInFly = true
	singletonMu.Unlock()

	db, err := Connect(ctx, databaseURL, opts)

	singletonMu.Lock()
	if err == nil {
		singletonDB = db
	}
	singletonInFly = false
	singletonCond.Broadcast()
	singletonMu.Unlock()

	if err == nil {
		log.Printf("db singleton cold-start init")
	}
	return singletonDB, err
}

func applyOptions(db *sql.DB, opts Options) {
	if opts.MaxOpenConns <= 0 {
		opts.MaxOpenConns = 10
	}
	if opts.MaxIdleConns <= 0 {
		opts.MaxIdleConns = 5
	}
	if opts.ConnMaxLifetime <= 0 {
		opts.ConnMaxLifetime = time.Hour
	}
	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxLifetime(opts.ConnMaxLifetime)
	if opts.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(opts.ConnMaxIdleTime)
	}
}

func logPoolStats(db *sql.DB, label string) {
	stats := db.Stats()
	log.Printf("%s: open=%d in_use=%d idle=%d wait=%d max_open=%d",
		label,
		stats.OpenConnections,
		stats.InUse,
		stats.Idle,
		stats.WaitCount,
		stats.MaxOpenConnections,
	)
}

func readEnvInt(key string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("db env %s invalid int: %v", key, err)
		return 0, false
	}
	return val, true
}

func readEnvDuration(key string) (time.Duration, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	val, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("db env %s invalid duration: %v", key, err)
		return 0, false
	}
	return val, true
}
