package core

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"myzhihu2/util/global"
)

const (
	// dbMaxOpenConns caps the pool. It is fixed at 10 for the 2-core/2 GB demo
	// server; db.max_open_conns in settings.yaml is intentionally not used.
	dbMaxOpenConns = 10
	// In Docker Compose the backend can start before MySQL accepts connections
	// (the first start initialises the data directory and may take 30s+).
	dbConnectAttempts = 10
	dbRetryInterval   = 3 * time.Second
	dbPingTimeout     = 5 * time.Second
)

// InitDB opens the MySQL pool, waits for the server to become reachable and
// applies the pool limits. It exits with a clear message (never the password)
// if MySQL is still unreachable after all attempts.
func InitDB() *gorm.DB {
	d := global.Conf.DB
	// timeout bounds the TCP dial; readTimeout/writeTimeout stop a hung MySQL
	// from blocking a request forever.
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=5s&readTimeout=30s&writeTimeout=30s",
		d.User, d.Password, d.Host, d.Port, d.Name)
	target := fmt.Sprintf("%s@%s:%d/%s", d.User, d.Host, d.Port, d.Name) // safe to log

	logLevel := logger.Warn // warnings and slow queries only
	if d.Debug {
		logLevel = logger.Info // every SQL statement
	}
	gormConfig := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logLevel),
	}

	// Pool settings: max open connections are fixed; idle connections and
	// lifetime still come from settings.yaml, with safe defaults.
	maxIdle := d.MaxIdleConns
	if maxIdle <= 0 || maxIdle > dbMaxOpenConns {
		maxIdle = dbMaxOpenConns
	}
	lifetime := 30 * time.Minute
	if d.ConnMaxLifetime != "" {
		parsed, err := time.ParseDuration(d.ConnMaxLifetime)
		if err != nil {
			log.Fatalf("mysql: invalid db.conn_max_lifetime %q: %v", d.ConnMaxLifetime, err)
		}
		lifetime = parsed
	}

	var lastErr error
	for attempt := 1; attempt <= dbConnectAttempts; attempt++ {
		db, err := gorm.Open(mysql.Open(dsn), gormConfig)
		var sqlDB *sql.DB
		if err == nil {
			sqlDB, err = db.DB()
		}
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), dbPingTimeout)
			err = sqlDB.PingContext(ctx)
			cancel()
			if err != nil {
				_ = sqlDB.Close() // do not leak the failed pool between attempts
			}
		}
		if err == nil {
			sqlDB.SetMaxOpenConns(dbMaxOpenConns)
			sqlDB.SetMaxIdleConns(maxIdle)
			// Recycle connections before MySQL's wait_timeout or a proxy drops them.
			sqlDB.SetConnMaxLifetime(lifetime)
			sqlDB.SetConnMaxIdleTime(5 * time.Minute)
			log.Printf("mysql: connected to %s (attempt %d/%d, max open conns %d)", target, attempt, dbConnectAttempts, dbMaxOpenConns)
			return db
		}
		lastErr = err
		log.Printf("mysql: %s not ready (attempt %d/%d): %v", target, attempt, dbConnectAttempts, err)
		if attempt < dbConnectAttempts {
			time.Sleep(dbRetryInterval)
		}
	}
	log.Fatalf("mysql: cannot connect to %s after %d attempts: %v", target, dbConnectAttempts, lastErr)
	return nil
}
