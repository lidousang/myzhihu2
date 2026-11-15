package main

import (
	"context"

	"myzhihu2/core"
	feedconsumer "myzhihu2/feed"
	"myzhihu2/route"
	"myzhihu2/service"
	"myzhihu2/util/flags"
	"myzhihu2/util/global"
)

func main() {
	flags.Parse()
	global.Conf = core.ReadConf()
	if err := core.InitJWT(); err != nil {
		panic(err)
	}
	global.DB = core.InitDB()
	// The migration command only needs MySQL; exit before starting optional
	// Redis/RabbitMQ dependencies when -db was supplied.
	flags.Run()
	global.Redis = core.InitRedis()
	global.RMQ = core.InitRMQ()
	service.StartOutboxRelay(context.Background())
	// Feed consumers update the Redis following projection and interest weights
	// asynchronously. If RabbitMQ is disabled, FeedSrv can still use MySQL.
	feedconsumer.Start(context.Background())
	// The explore pool replaces ORDER BY RAND(): recent content IDs are kept in
	// a Redis ZSET and sampled in Go.
	service.StartExploreRefresher(context.Background())
	router := route.NewRouter(global.Conf.JWT, global.Conf.Demo)
	if err := router.Run(global.Conf.System.Addr()); err != nil {
		panic(err)
	}
}
