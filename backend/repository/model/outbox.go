package model

import "time"

// OutboxEvent is committed in the same MySQL transaction as its business
// change. A background relay publishes pending rows to RabbitMQ.
type OutboxEvent struct {
	ID          uint      `gorm:"primaryKey"`
	EventID     string    `gorm:"size:64;not null;uniqueIndex"`
	EventType   string    `gorm:"size:64;not null;index:idx_outbox_pending,priority:1"`
	RoutingKey  string    `gorm:"size:128;not null"`
	Payload     []byte    `gorm:"type:json;not null"`
	Status      string    `gorm:"size:16;not null;default:pending;index:idx_outbox_pending,priority:2"`
	RetryCount  int       `gorm:"not null;default:0"`
	NextRetryAt time.Time `gorm:"index:idx_outbox_pending,priority:3"`
	LastError   string    `gorm:"type:text"`
	CreatedAt   time.Time
	SentAt      *time.Time
}

func (OutboxEvent) TableName() string { return "outbox_events" }
