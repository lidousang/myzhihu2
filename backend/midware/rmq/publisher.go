// Package rmq contains the small RabbitMQ publishing boundary used by services.
package rmq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"myzhihu2/util/global"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Event is the common event envelope. Type describes the business event while
// the routing key selects the RabbitMQ queue.
type Event struct {
	EventID   string          `json:"event_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// NewEventID creates a stable identifier used by Outbox and consumers for
// tracing and idempotency. It is deliberately dependency-free for this project.
func NewEventID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// EncodeEvent creates the wire envelope after an Outbox row has been selected.
func EncodeEvent(eventID, eventType string, payload json.RawMessage) ([]byte, error) {
	return json.Marshal(Event{
		EventID: eventID, Type: eventType, Payload: payload, CreatedAt: time.Now().UTC(),
	})
}

// PublishBestEffort sends a persistent event after its MySQL operation has
// succeeded. Broker failure is logged but does not roll back the user action;
// an Outbox can be added later if delivery must be guaranteed.
func PublishBestEffort(ctx context.Context, routingKey, eventType string, payload any) {
	if global.RMQ == nil || global.Conf == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("rabbitmq event %s encode failed: %v", eventType, err)
		return
	}
	body, err := json.Marshal(Event{EventID: NewEventID(), Type: eventType, Payload: data, CreatedAt: time.Now().UTC()})
	if err != nil {
		log.Printf("rabbitmq event %s envelope failed: %v", eventType, err)
		return
	}
	channel, err := global.RMQ.Channel()
	if err != nil {
		log.Printf("rabbitmq event %s channel failed: %v", eventType, err)
		return
	}
	defer channel.Close()
	if err := channel.PublishWithContext(ctx, global.Conf.RabbitMq.Exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}); err != nil {
		log.Printf("rabbitmq event %s publish failed: %v", eventType, err)
	}
}

// PublishConfirmed publishes an already-encoded Outbox envelope and waits for
// RabbitMQ's publisher confirmation. A nil error means the broker accepted the
// message, not that a consumer has finished processing it.
func PublishConfirmed(ctx context.Context, routingKey string, body []byte) error {
	if global.RMQ == nil || global.Conf == nil {
		return fmt.Errorf("rabbitmq is not initialized")
	}
	channel, err := global.RMQ.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	if err := channel.Confirm(false); err != nil {
		return err
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	if err := channel.PublishWithContext(ctx, global.Conf.RabbitMq.Exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}); err != nil {
		return err
	}
	select {
	case confirmation := <-confirmations:
		if !confirmation.Ack {
			return fmt.Errorf("rabbitmq publisher nack")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
