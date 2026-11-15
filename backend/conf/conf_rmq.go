package conf

// RabbitMQConfig RabbitMQ 消息队列配置
type RabbitMQ struct {
	// 是否启用 RabbitMQ
	Enabled bool `yaml:"enabled"`

	// RabbitMQ 服务地址
	Host string `yaml:"host"`

	// RabbitMQ 服务端口，默认 AMQP 端口为 5672
	Port int `yaml:"port"`

	// RabbitMQ 登录用户名
	Username string `yaml:"username"`

	// RabbitMQ 登录密码
	Password string `yaml:"password"`

	// RabbitMQ 虚拟主机，用于隔离不同的消息队列资源
	VHost string `yaml:"vhost"`

	// 交换机名称，生产者通常向交换机发送消息
	Exchange string `yaml:"exchange"`

	// 交换机类型，例如 direct、topic、fanout、headers
	ExchangeType string `yaml:"exchange_type"`

	// 队列名称，消费者从队列中获取消息
	Queue string `yaml:"queue"`

	// 路由键，用于交换机根据绑定规则将消息路由到队列
	RoutingKey string `yaml:"routing_key"`

	// Feed 专用队列和路由键，避免 Feed Consumer 抢走用户行为消息。
	FeedQueue      string `yaml:"feed_queue"`
	FeedRoutingKey string `yaml:"feed_routing_key"`
}
