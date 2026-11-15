package core

import (
	"os"
	"testing"

	"myzhihu2/conf"
	"myzhihu2/util/global"
)

// Integration test against a real MySQL. InitDB exits the process on failure
// (log.Fatal), so it only runs when explicitly enabled:
//
//	MYZHIHU2_DB_TEST=1 MYZHIHU2_DB_USER=root MYZHIHU2_DB_PASSWORD=root go test ./core/
func TestInitDBAppliesPoolLimits(t *testing.T) {
	if os.Getenv("MYZHIHU2_DB_TEST") != "1" {
		t.Skip("set MYZHIHU2_DB_TEST=1 (and MYZHIHU2_DB_USER/PASSWORD) to run against a real MySQL")
	}
	global.Conf = &conf.Config{DB: conf.DB{
		User: os.Getenv("MYZHIHU2_DB_USER"), Password: os.Getenv("MYZHIHU2_DB_PASSWORD"),
		Host: "127.0.0.1", Port: 3306, Name: "mysql",
		MaxIdleConns: 50, MaxOpenConns: 25, ConnMaxLifetime: "10m", // both above the fixed cap
	}}
	db := InitDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if got := sqlDB.Stats().MaxOpenConnections; got != dbMaxOpenConns {
		t.Fatalf("MaxOpenConnections=%d, want fixed %d (config value 25 must be ignored)", got, dbMaxOpenConns)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatal(err)
	}
}
