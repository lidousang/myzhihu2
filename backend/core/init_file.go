package core

import (
	"fmt"
	"os"

	"myzhihu2/conf"
	"myzhihu2/util/flags"

	"gopkg.in/yaml.v2"
)

// after reading configfile FIRST, the related global
// structure is assigned, inner strcuture for midware
// init is also assigned, then init other connection

func ReadConf() *conf.Config {
	byteData, err := os.ReadFile(flags.FlagOption.File)
	if err != nil {
		panic(err)
	}

	c := new(conf.Config)
	// 重利用err, 使用'='而非':='
	// define a type, unmarshal from binary to struct
	err = yaml.Unmarshal(byteData, c)
	if err != nil {
		panic(fmt.Sprintf("error reading file from init_config.go, the error is %s ", err))
	}

	c.ApplyDefaults()
	return c

}
