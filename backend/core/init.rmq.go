package core

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"myzhihu2/util/global"

	amqp "github.com/rabbitmq/amqp091-go"
)

// InitRMQ 初始化 RabbitMQ 连接及消息路由结构。
func InitRMQ() *amqp.Connection {
	// 1. 从全局配置中读取 RabbitMQ 配置
	r := global.Conf.RabbitMq

	// 2. 如果未启用，则不进行初始化
	if !r.Enabled {
		global.RMQ = nil
		fmt.Println("RabbitMQ is disabled")
		return nil
	}

	// 3. 检查必要配置
	if r.Host == "" {
		panic("RabbitMQ 配置错误：host 不能为空")
	}

	if r.Port <= 0 || r.Port > 65535 {
		panic("RabbitMQ 配置错误：port 无效")
	}

	if r.Username == "" {
		panic("RabbitMQ 配置错误：username 不能为空")
	}

	if r.VHost == "" {
		panic("RabbitMQ 配置错误：vhost 不能为空")
	}

	if r.Exchange == "" {
		panic("RabbitMQ 配置错误：exchange 不能为空")
	}

	if r.ExchangeType == "" {
		panic("RabbitMQ 配置错误：exchange_type 不能为空")
	}

	if r.Queue == "" {
		panic("RabbitMQ 配置错误：queue 不能为空")
	}

	if r.RoutingKey == "" {
		panic("RabbitMQ 配置错误：routing_key 不能为空")
	}

	if r.FeedQueue == "" {
		panic("RabbitMQ 配置错误：feed_queue 不能为空")
	}

	if r.FeedRoutingKey == "" {
		panic("RabbitMQ 配置错误：feed_routing_key 不能为空")
	}

	// 4. 构造 AMQP 连接地址
	// 对用户名、密码和 VHost 中的特殊字符进行正确编码。
	hostPort := net.JoinHostPort(
		r.Host,
		strconv.Itoa(r.Port),
	)

	uri := fmt.Sprintf(
		"amqp://%s@%s/%s",
		url.UserPassword(r.Username, r.Password).String(),
		hostPort,
		url.PathEscape(r.VHost),
	)

	// 5. 建立 RabbitMQ Connection
	conn, err := amqp.DialConfig(uri, amqp.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
		Dial: func(network, addr string) (net.Conn, error) {
			dialer := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return dialer.Dial(network, addr)
		},
	})

	if err != nil {
		panic("RabbitMQ 连接失败: " + err.Error())
	}

	// 如果后续初始化失败，关闭刚创建的连接
	initialized := false
	defer func() {
		if !initialized {
			_ = conn.Close()
		}
	}()

	// 6. 创建 Channel
	ch, err := conn.Channel()
	if err != nil {
		panic("RabbitMQ Channel 创建失败: " + err.Error())
	}
	defer ch.Close()

	// 7. 声明 Exchange
	err = ch.ExchangeDeclare(
		r.Exchange,     // 交换机名称
		r.ExchangeType, // 交换机类型
		true,           // durable：交换机持久化
		false,          // autoDelete：不自动删除
		false,          // internal：不是内部专用交换机
		false,          // noWait：等待服务器确认
		nil,            // arguments：额外参数
	)
	if err != nil {
		panic("RabbitMQ Exchange 初始化失败: " + err.Error())
	}

	// 8. 声明并绑定行为队列和 Feed 队列。独立路由键可以避免
	// Feed Consumer 抢走未来的用户行为消息。
	queues := []struct {
		name       string
		routingKey string
	}{
		{name: r.Queue, routingKey: r.RoutingKey},
		{name: r.FeedQueue, routingKey: r.FeedRoutingKey},
	}
	for _, queue := range queues {
		if _, err = ch.QueueDeclare(queue.name, true, false, false, false, nil); err != nil {
			panic("RabbitMQ Queue 初始化失败: " + err.Error())
		}
		if err = ch.QueueBind(queue.name, queue.routingKey, r.Exchange, false, nil); err != nil {
			panic("RabbitMQ Queue 绑定失败: " + err.Error())
		}
	}

	// 10. 初始化成功，将连接保存到全局变量

	initialized = true

	fmt.Println("RabbitMQ connected successfully")

	return conn
}
