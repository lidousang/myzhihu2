package dao

import (
	"context"
	"errors"
	"time"

	"myzhihu2/repository/model"
	"myzhihu2/util/global"

	"gorm.io/gorm"
)

// OutboxDao persists event records and is also used by the background relay.
type OutboxDao struct{ db *gorm.DB }

func NewOutboxDao(ctx context.Context) (*OutboxDao, error) {
	if global.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &OutboxDao{db: global.DB.WithContext(ctx)}, nil
}

// CreateWithTx inserts the event using the business operation's transaction.
func (d *OutboxDao) CreateWithTx(tx *gorm.DB, event *model.OutboxEvent) error {
	return tx.Create(event).Error
}

// ListPending fetches a bounded batch of events whose retry time has arrived.
func (d *OutboxDao) ListPending(ctx context.Context, limit int) ([]model.OutboxEvent, error) {
	var events []model.OutboxEvent
	err := d.db.WithContext(ctx).Where("status = ? AND next_retry_at <= ?", "pending", time.Now()).
		Order("id ASC").Limit(limit).Find(&events).Error
	return events, err
}

// MarkSent records that RabbitMQ confirmed the publication.
func (d *OutboxDao) MarkSent(ctx context.Context, id uint) error {
	now := time.Now()
	return d.db.WithContext(ctx).Model(&model.OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]any{"status": "sent", "sent_at": now, "last_error": ""}).Error
}

// ScheduleRetry leaves the event pending and applies a small exponential
// backoff. It is deliberately simple for this single-process learning project.
func (d *OutboxDao) ScheduleRetry(ctx context.Context, event *model.OutboxEvent, cause error) error {
	event.RetryCount++
	delay := time.Second * time.Duration(1<<min(event.RetryCount, 6))
	return d.db.WithContext(ctx).Model(&model.OutboxEvent{}).Where("id = ?", event.ID).
		Updates(map[string]any{
			"retry_count":   event.RetryCount,
			"next_retry_at": time.Now().Add(delay),
			"last_error":    cause.Error(),
		}).Error
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
