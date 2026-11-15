package flags

import (
	"flag"
	"os"
)

// 为main解析参数
// use case: go run main.go -db ...， 用于数据库迁移
type Options struct {
	File    string // settings file
	DB      bool
	Version bool
	Mode    string
}

// for external calling
var FlagOption = new(Options)

func Parse() {
	flag.StringVar(&FlagOption.File, "f", "settings/settings.yaml", "reading configuration file")
	flag.BoolVar(&FlagOption.DB, "db", false, "database migration")
	flag.BoolVar(&FlagOption.Version, "v", false, "database version")
	flag.StringVar(&FlagOption.Mode, "m", "debug", "mode of project")
	flag.Parse()
}

func Run() {
	if FlagOption.DB {
		DBMigrate()
		// 停止了运行！！！
		os.Exit(0)
	}
}
