package service

import (
	"context"
	"log"
	"time"

	"myzhihu2/midware/rmq"
	"myzhihu2/repository/dao"
	"myzhihu2/util/global"
)

// StartOutboxRelay periodically publishes committed Outbox rows. It is a
// deliberately indirect path: business services only insert rows, while this
// component owns Broker retry and Publisher Confirm handling.
func StartOutboxRelay(ctx context.Context) {
	if global.DB == nil || global.RMQ == nil {
		log.Printf("outbox relay disabled: database or RabbitMQ is unavailable")
		return
	}
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				relayOutboxBatch(ctx)
			}
		}
	}()
}

func relayOutboxBatch(ctx context.Context) {
	outbox, err := dao.NewOutboxDao(ctx)
	if err != nil {
		log.Printf("outbox dao unavailable: %v", err)
		return
	}
	events, err := outbox.ListPending(ctx, 20)
	if err != nil {
		log.Printf("outbox list failed: %v", err)
		return
	}
	for i := range events {
		event := &events[i]
		body, encodeErr := rmq.EncodeEvent(event.EventID, event.EventType, event.Payload)
		if encodeErr != nil {
			_ = outbox.ScheduleRetry(ctx, event, encodeErr)
			continue
		}
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		publishErr := rmq.PublishConfirmed(publishCtx, event.RoutingKey, body)
		cancel()
		if publishErr != nil {
			if err := outbox.ScheduleRetry(ctx, event, publishErr); err != nil {
				log.Printf("outbox retry update failed: %v", err)
			}
			continue
		}
		if err := outbox.MarkSent(ctx, event.ID); err != nil {
			// The message may already be accepted; leaving it pending causes a
			// duplicate on the next relay pass. Consumer idempotency is therefore
			// required even when Publisher Confirm succeeds.
			log.Printf("outbox mark sent failed for %s: %v", event.EventID, err)
		}
	}
}
