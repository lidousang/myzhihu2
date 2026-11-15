// Package feed contains the asynchronous consumers that maintain Feed Redis
// projections from RabbitMQ events.
package feed

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"myzhihu2/midware/rmq"
	"myzhihu2/repository/cache"
	"myzhihu2/repository/dao"
	"myzhihu2/repository/model"
	"myzhihu2/serializer"
	"myzhihu2/service"
	"myzhihu2/util/global"

	amqp "github.com/rabbitmq/amqp091-go"
)

type contentEvent struct {
	ContentID uint              `json:"content_id"`
	Kind      model.ContentKind `json:"kind"`
	AuthorID  uint              `json:"author_id"`
}

type followEvent struct {
	FollowerID  uint `json:"follower_id"`
	FollowingID uint `json:"following_id"`
}

type behaviorEvent struct {
	UserID    uint               `json:"user_id"`
	ContentID uint               `json:"content_id"`
	Kind      model.BehaviorKind `json:"kind"`
	Duration  int                `json:"duration"`
}

// Start starts independent consumers for Feed membership events and user
// behavior events. They use separate queues so neither workload steals the
// other's messages from a shared competing-consumer queue.
func Start(ctx context.Context) {
	if global.RMQ == nil || global.Conf == nil {
		log.Printf("feed consumers are disabled because RabbitMQ is not initialized")
		return
	}
	if global.Redis == nil {
		log.Printf("feed consumers are disabled because Redis is not initialized")
		return
	}
	go consumeQueue(ctx, global.Conf.RabbitMq.FeedQueue, handleFeedEvent)
	go consumeQueue(ctx, global.Conf.RabbitMq.Queue, handleBehaviorEvent)
}

type eventHandler func(context.Context, rmq.Event) error

// consumeQueue receives messages with manual acknowledgement. The broker
// considers work complete only after the handler has successfully updated its
// Redis projection.
func consumeQueue(ctx context.Context, queue string, handler eventHandler) {
	channel, err := global.RMQ.Channel()
	if err != nil {
		log.Printf("feed consumer cannot open channel for %s: %v", queue, err)
		return
	}
	if err := channel.Qos(20, 0, false); err != nil {
		_ = channel.Close()
		log.Printf("feed consumer cannot set QoS for %s: %v", queue, err)
		return
	}
	deliveries, err := channel.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		_ = channel.Close()
		log.Printf("feed consumer cannot consume %s: %v", queue, err)
		return
	}
	log.Printf("feed consumer started for queue %s", queue)
	defer channel.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case delivery, ok := <-deliveries:
			if !ok {
				return
			}
			processDelivery(ctx, delivery, handler)
		}
	}
}

func processDelivery(parent context.Context, delivery amqp.Delivery, handler eventHandler) {
	var event rmq.Event
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		log.Printf("discarding malformed RabbitMQ event: %v", err)
		_ = delivery.Nack(false, false)
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := handler(ctx, event); err != nil {
		log.Printf("RabbitMQ event %s failed and will be retried: %v", event.Type, err)
		_ = delivery.Nack(false, true)
		return
	}
	_ = delivery.Ack(false)
}

func handleFeedEvent(ctx context.Context, event rmq.Event) error {
	switch event.Type {
	case "content.created", "content.deleted":
		var payload contentEvent
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		if payload.ContentID == 0 || payload.AuthorID == 0 {
			return nil
		}
		if event.Type == "content.created" {
			return fanoutCreated(ctx, payload)
		}
		return fanoutDeleted(ctx, payload)
	case "content.updated":
		// Projections contain IDs only, so edits do not change membership.
		return nil
	case "user.followed", "user.unfollowed":
		var payload followEvent
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		if payload.FollowerID == 0 || payload.FollowingID == 0 {
			return nil
		}
		return handleFollowChange(ctx, payload, event.Type == "user.followed")
	default:
		return nil
	}
}

// fanoutCreated delivers a new post with the push/pull hybrid:
//
//	every author   -> ZADD feed:outbox:{author}          (one write)
//	ordinary author-> ZADD feed:user:{f} for each ACTIVE follower f, in
//	                  pipelines of FanoutBatch, each inbox trimmed to InboxLimit
//	big V author   -> nothing else; followers pull the outbox when reading
//
// Only followers with an existing inbox (feed:ready) are "active". Inactive
// followers get a full rebuild from MySQL when they next open the feed, so
// pushing to them would be wasted work.
func fanoutCreated(ctx context.Context, ev contentEvent) error {
	started := time.Now()
	cfg := global.Conf.Feed
	proj := service.GetProjection()
	fc := cache.NewFeedCache()
	users, err := dao.NewUserDao(ctx)
	if err != nil {
		return err
	}
	follows, err := dao.NewUserFollowDao(ctx)
	if err != nil {
		return err
	}
	followers, err := users.FollowerCount(ev.AuthorID)
	if err != nil {
		return err
	}
	bigV, err := proj.IsBigV(ctx, ev.AuthorID)
	if err != nil {
		return err
	}
	stats := serializer.FanoutStats{ContentID: ev.ContentID, Followers: followers}

	if bigV {
		// Pull mode: make sure the outbox exists (a rebuild already includes
		// this post) and add the post. Followers merge it while reading.
		if err := proj.EnsureOutbox(ctx, ev.AuthorID); err != nil {
			return err
		}
		if err := fc.AddOutbox(ctx, ev.AuthorID, []uint{ev.ContentID}, cfg.OutboxLimit); err != nil {
			return err
		}
		stats.Mode, stats.RedisWrites = "pull", 1
	} else {
		// Keep an existing outbox current so a later promotion to big V starts
		// from fresh data; a missing outbox is rebuilt on demand.
		if ready, err := fc.OutboxReady(ctx, ev.AuthorID); err != nil {
			return err
		} else if ready {
			if err := fc.AddOutbox(ctx, ev.AuthorID, []uint{ev.ContentID}, cfg.OutboxLimit); err != nil {
				return err
			}
			stats.RedisWrites++
		}
		err := follows.EachFollowerBatch(ev.AuthorID, cfg.FanoutBatch, func(ids []uint) error {
			active, err := fc.FilterReady(ctx, ids)
			if err != nil {
				return err
			}
			n, err := fc.PushInboxBatch(ctx, active, []uint{ev.ContentID}, cfg.InboxLimit)
			stats.ActiveTargets += len(active)
			stats.RedisWrites += n
			return err
		})
		if err != nil {
			return err
		}
		stats.Mode = "push"
	}

	if exists, err := fc.ExplorePoolExists(ctx); err == nil && exists {
		_ = fc.AddExplore(ctx, ev.ContentID, cfg.ExplorePoolSize)
	}
	stats.DurationMs = time.Since(started).Milliseconds()
	if err := fc.SetFanoutStats(ctx, stats); err != nil {
		log.Printf("fanout stats for content %d not stored: %v", ev.ContentID, err)
	}
	log.Printf("fanout content=%d author=%d mode=%s followers=%d active=%d writes=%d took=%dms",
		ev.ContentID, ev.AuthorID, stats.Mode, stats.Followers, stats.ActiveTargets, stats.RedisWrites, stats.DurationMs)
	return nil
}

// fanoutDeleted removes a deleted post from the outbox, the explore pool and,
// for ordinary authors, from active followers' inboxes. A big V post that was
// pushed before the promotion is cleaned lazily by the read path.
func fanoutDeleted(ctx context.Context, ev contentEvent) error {
	fc := cache.NewFeedCache()
	if err := fc.RemoveOutboxIDs(ctx, ev.AuthorID, []uint{ev.ContentID}); err != nil {
		return err
	}
	_ = fc.RemoveExplore(ctx, ev.ContentID)
	bigV, err := service.GetProjection().IsBigV(ctx, ev.AuthorID)
	if err != nil || bigV {
		return err
	}
	follows, err := dao.NewUserFollowDao(ctx)
	if err != nil {
		return err
	}
	return follows.EachFollowerBatch(ev.AuthorID, global.Conf.Feed.FanoutBatch, func(ids []uint) error {
		active, err := fc.FilterReady(ctx, ids)
		if err != nil {
			return err
		}
		return fc.RemoveFromInboxes(ctx, active, ev.ContentID)
	})
}

// handleFollowChange reclassifies the followed author (hysteresis) and keeps
// the follower's inbox consistent:
//   - follow an ordinary author: backfill the author's recent posts into the
//     follower's inbox (only if the inbox exists; otherwise a rebuild covers it);
//   - follow a big V: nothing, posts are pulled while reading;
//   - unfollow anyone: remove the author's posts from the follower's inbox
//     (posts pushed before a promotion may still be there);
//   - demotion of a big V: push its recent outbox to active followers, because
//     followers stop pulling it.
func handleFollowChange(ctx context.Context, ev followEvent, followed bool) error {
	cfg := global.Conf.Feed
	proj := service.GetProjection()
	fc := cache.NewFeedCache()
	users, err := dao.NewUserDao(ctx)
	if err != nil {
		return err
	}
	contents, err := dao.NewContentDao(ctx)
	if err != nil {
		return err
	}
	count, err := users.FollowerCount(ev.FollowingID)
	if err != nil {
		return err
	}
	change, err := proj.ReclassifyAuthor(ctx, ev.FollowingID, count)
	if err != nil {
		return err
	}
	if change != 0 {
		log.Printf("author %d %s (followers=%d)", ev.FollowingID, map[int]string{1: "promoted to big V", -1: "demoted to push mode"}[change], count)
	}
	if change == -1 {
		if err := pushRecentToActiveFollowers(ctx, ev.FollowingID); err != nil {
			return err
		}
	}

	ready, err := fc.IsInitialized(ctx, ev.FollowerID)
	if err != nil || !ready {
		return err
	}
	if !followed {
		ids, err := contents.ListIDsByAuthor(ev.FollowingID)
		if err != nil {
			return err
		}
		return fc.RemoveFollowingIDs(ctx, ev.FollowerID, ids)
	}
	bigV, err := proj.IsBigV(ctx, ev.FollowingID)
	if err != nil || bigV {
		return err
	}
	recent, err := contents.ListRecentIDsByAuthor(ev.FollowingID, 0, 100)
	if err != nil {
		return err
	}
	return fc.AddFollowingIDs(ctx, ev.FollowerID, recent, cfg.InboxLimit)
}

func pushRecentToActiveFollowers(ctx context.Context, authorID uint) error {
	cfg := global.Conf.Feed
	fc := cache.NewFeedCache()
	contents, err := dao.NewContentDao(ctx)
	if err != nil {
		return err
	}
	follows, err := dao.NewUserFollowDao(ctx)
	if err != nil {
		return err
	}
	recent, err := contents.ListRecentIDsByAuthor(authorID, 0, 100)
	if err != nil || len(recent) == 0 {
		return err
	}
	return follows.EachFollowerBatch(authorID, cfg.FanoutBatch, func(ids []uint) error {
		active, err := fc.FilterReady(ctx, ids)
		if err != nil {
			return err
		}
		_, err = fc.PushInboxBatch(ctx, active, recent, cfg.InboxLimit)
		return err
	})
}

func handleBehaviorEvent(ctx context.Context, event rmq.Event) error {
	if event.Type != "user.behavior" {
		return nil
	}
	var payload behaviorEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	if payload.UserID == 0 || payload.ContentID == 0 {
		return nil
	}
	switch payload.Kind {
	case model.BehaviorKindLike, model.BehaviorKindCollect, model.BehaviorKindReadTime, model.BehaviorKindView:
	default:
		return errors.New("unknown behavior kind")
	}
	contentDAO, err := dao.NewContentDao(ctx)
	if err != nil {
		return err
	}
	content, err := contentDAO.FindByID(payload.ContentID)
	if dao.IsNotFound(err) {
		// A deleted item no longer contributes to new interest signals.
		return nil
	}
	if err != nil {
		return err
	}
	topicField := service.TopicField(content)
	if topicField == "" {
		topicField = "kind:" + string(content.Kind)
	}
	if event.EventID == "" {
		// Messages without an event_id cannot be deduplicated safely. Ack them
		// once after logging instead of requeueing a poison message forever.
		log.Printf("acknowledging legacy behavior event without event_id")
		return nil
	}
	weight := service.BehaviorWeight(payload.Kind, payload.Duration)
	_, err = cache.NewFeedCache().AddBehaviorInterestOnce(ctx, event.EventID, payload.UserID, content.AuthorID, topicField, weight)
	return err
}
