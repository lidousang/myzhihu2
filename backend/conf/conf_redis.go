package conf

// RedisConfig Redis 配置
type Redis struct {
	// 是否启用 Redis
	Enabled bool `yaml:"enabled"`

	// Redis 服务地址
	Host string `yaml:"host"`

	// Redis 服务端口
	Port int `yaml:"port"`

	// Redis 连接密码；未设置密码时为空字符串
	Password string `yaml:"password"`

	// Redis 数据库编号，默认为 0
	DB int `yaml:"db"`

	// 连接池最大连接数
	PoolSize int `yaml:"pool_size"`

	// 连接池最小空闲连接数
	MinIdleConns int `yaml:"min_idle_conns"`

	// 建立连接的超时时间，例如 5s
	DialTimeout string `yaml:"dial_timeout"`

	// 读取数据的超时时间，例如 3s
	ReadTimeout string `yaml:"read_timeout"`

	// 写入数据的超时时间，例如 3s
	WriteTimeout string `yaml:"write_timeout"`
}
