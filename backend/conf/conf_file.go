package conf

// 配置文件抽象
type Config struct {
	System System `yaml:"system"`
	//Log    Log    `yaml:"log"`
	DB       DB       `yaml:"db"`
	Redis    Redis    `yaml:"redis"`
	RabbitMq RabbitMQ `yaml:"rabbitmq"`
	// jwt: configuration for token generation and validation.
	JWT JWT `yaml:"jwt"`
	// Feed configures the push/pull hybrid and candidate pools.
	Feed Feed `yaml:"feed"`
	// Demo configures the public demonstration deployment.
	Demo Demo `yaml:"demo"`
}
