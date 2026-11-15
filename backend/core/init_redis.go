package core

import (
	"context"
	"fmt"
	"time"

	"myzhihu2/util/global"

	"github.com/redis/go-redis/v9"
)

func InitRedis() *redis.Client {
	// 如果未启用，则直接返回 nil
	r := global.Conf.Redis
	if !r.Enabled {
		return nil
	}

	// 解析超时时间字符串
	dialTimeout, _ := time.ParseDuration(r.DialTimeout)
	readTimeout, _ := time.ParseDuration(r.ReadTimeout)
	writeTimeout, _ := time.ParseDuration(r.WriteTimeout)

	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", r.Host, r.Port), // 对应配置中的 host 和 port[cite: 5, 7]
		Password:     r.Password,                           // 对应 password[cite: 5, 7]
		DB:           r.DB,                                 // 对应 db 编号[cite: 5, 7]
		PoolSize:     r.PoolSize,                           // 对应 pool_size[cite: 5, 7]
		MinIdleConns: r.MinIdleConns,                       // 对应 min_idle_conns[cite: 5, 7]
		DialTimeout:  dialTimeout,                          // 对应 dial_timeout[cite: 5, 7]
		ReadTimeout:  readTimeout,                          // 对应 read_timeout[cite: 5, 7]
		WriteTimeout: writeTimeout,                         // 对应 write_timeout[cite: 5, 7]
	})

	// 测试连通性
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		panic("Redis 连接失败: " + err.Error())
	}

	fmt.Println("Redis connected")
	return rdb
}
