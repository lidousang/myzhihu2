package global

import (
	"myzhihu2/conf"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var Conf *conf.Config
var DB *gorm.DB
var Redis *redis.Client
var RMQ *amqp.Connection
