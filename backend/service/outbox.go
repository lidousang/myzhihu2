package service

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"myzhihu2/midware/rmq"
	"myzhihu2/repository/dao"
	"myzhihu2/repository/model"
)

// newOutboxEvent turns a business event into a pending database row. The
// caller must insert the returned row using the same transaction as the
// business change; this is the key Outbox guarantee.
func newOutboxEvent(eventType, routingKey string, payload any) (*model.OutboxEvent, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &model.OutboxEvent{
		EventID: rmq.NewEventID(), EventType: eventType, RoutingKey: routingKey,
		Payload: data, Status: "pending", NextRetryAt: time.Now(),
	}, nil
}

func createOutboxTx(ctx context.Context, tx *gorm.DB, eventType, routingKey string, payload any) error {
	event, err := newOutboxEvent(eventType, routingKey, payload)
	if err != nil {
		return err
	}
	outboxDAO, err := dao.NewOutboxDao(ctx)
	if err != nil {
		return err
	}
	return outboxDAO.CreateWithTx(tx.WithContext(ctx), event)
}
