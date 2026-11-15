package conf

type DB struct {
	User     string `yaml:"user"`
	Password string `yaml:"passport"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"db_name"`
	Debug    bool   `yaml:"debug"`
	Source   string `yaml:"source"` //可扩展其它数据库

	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime string `yaml:"conn_max_lifetime"`
}
