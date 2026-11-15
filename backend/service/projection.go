package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"myzhihu2/conf"
	"myzhihu2/repository/cache"
	"myzhihu2/repository/dao"
	"myzhihu2/util/global"

	"golang.org/x/sync/singleflight"
)

// Projection owns the rebuildable Redis projections used by the push/pull
// feed: user inboxes, author outboxes, the big V set and the explore pool.
// Both FeedSrv (read path) and the RabbitMQ consumers (write path) use it, so
// rebuild rules live in one place.
//
// Rebuild protocol (inbox and outbox):
//  1. take a MySQL snapshot of the newest IDs;
//  2. write them to a temporary key, RENAME it over the live key and set the
//     ready marker in one MULTI/EXEC;
//  3. run a catch-up query for IDs newer than the snapshot and ZADD them.
//
// Consumers only write to projections whose ready marker exists. A post whose
// event was skipped because the marker was missing was committed before the
// marker was set, so the catch-up query in step 3 (which runs after the
// marker) is guaranteed to see it. This closes the rebuild race in which a
// plain DEL+ZADD would erase a concurrent consumer write.
type Projection struct {
	group singleflight.Group
}

var (
	projectionIns  *Projection
	projectionOnce sync.Once
)

// GetProjection returns the shared projection manager.
func GetProjection() *Projection {
	projectionOnce.Do(func() { projectionIns = &Projection{} })
	return projectionIns
}

func feedConf() conf.Feed {
	if global.Conf == nil {
		c := &conf.Config{}
		c.ApplyDefaults()
		return c.Feed
	}
	return global.Conf.Feed
}

// ---------------------------------------------------------------- big V --

// EnsureBigVSet rebuilds the big V set from follower counts when Redis lost it.
func (p *Projection) EnsureBigVSet(ctx context.Context) error {
	fc := cache.NewFeedCache()
	ready, err := fc.BigVReady(ctx)
	if err != nil || ready {
		return err
	}
	_, err, _ = p.group.Do("bigv", func() (any, error) {
		if ready, err := fc.BigVReady(ctx); err != nil || ready {
			return nil, err
		}
		users, err := dao.NewUserDao(ctx)
		if err != nil {
			return nil, err
		}
		ids, err := users.ListIDsWithFollowersAtLeast(feedConf().BigVThreshold)
		if err != nil {
			return nil, err
		}
		log.Printf("feed projection: rebuilt big V set %v", ids)
		return nil, fc.ReplaceBigVSet(ctx, ids)
	})
	return err
}

// BigVs returns the authors currently in pull mode.
func (p *Projection) BigVs(ctx context.Context) ([]uint, error) {
	if err := p.EnsureBigVSet(ctx); err != nil {
		return nil, err
	}
	return cache.NewFeedCache().BigVMembers(ctx)
}

// IsBigV reports whether an author is in pull mode.
func (p *Projection) IsBigV(ctx context.Context, authorID uint) (bool, error) {
	if err := p.EnsureBigVSet(ctx); err != nil {
		return false, err
	}
	return cache.NewFeedCache().IsBigV(ctx, authorID)
}

// ReclassifyAuthor applies the hysteresis rule after a follower count change:
// promote at >= threshold, demote below 80% of it. It returns +1 for a
// promotion, -1 for a demotion and 0 when membership did not change.
func (p *Projection) ReclassifyAuthor(ctx context.Context, authorID uint, followerCount int) (int, error) {
	if err := p.EnsureBigVSet(ctx); err != nil {
		return 0, err
	}
	fc := cache.NewFeedCache()
	cfg := feedConf()
	switch {
	case followerCount >= cfg.BigVThreshold:
		changed, err := fc.SetBigV(ctx, authorID, true)
		if changed {
			return 1, err
		}
		return 0, err
	case followerCount < cfg.BigVExitThreshold():
		changed, err := fc.SetBigV(ctx, authorID, false)
		if changed {
			return -1, err
		}
		return 0, err
	}
	return 0, nil
}

// ---------------------------------------------------------------- inbox --

// EnsureInbox builds a user's push inbox if it is missing and reports whether
// a rebuild happened. Concurrent misses for the same user share one rebuild.
func (p *Projection) EnsureInbox(ctx context.Context, userID uint) (bool, error) {
	fc := cache.NewFeedCache()
	ready, err := fc.IsInitialized(ctx, userID)
	if err != nil || ready {
		return false, err
	}
	_, err, shared := p.group.Do(fmt.Sprintf("inbox:%d", userID), func() (any, error) {
		if ready, err := fc.IsInitialized(ctx, userID); err != nil || ready {
			return nil, err
		}
		return nil, p.rebuildInbox(ctx, userID)
	})
	return err == nil && !shared, err
}

// RebuildInbox forces a rebuild; the seeder uses it to warm "online" users.
func (p *Projection) RebuildInbox(ctx context.Context, userID uint) error {
	_, err, _ := p.group.Do(fmt.Sprintf("inbox:%d", userID), func() (any, error) {
		return nil, p.rebuildInbox(ctx, userID)
	})
	return err
}

func (p *Projection) rebuildInbox(ctx context.Context, userID uint) error {
	bigVs, err := p.BigVs(ctx)
	if err != nil {
		return err
	}
	contents, err := dao.NewContentDao(ctx)
	if err != nil {
		return err
	}
	limit := feedConf().InboxLimit
	ids, err := contents.ListInboxIDs(userID, bigVs, 0, limit)
	if err != nil {
		return err
	}
	fc := cache.NewFeedCache()
	if err := fc.ReplaceFollowingIDs(ctx, userID, ids); err != nil {
		return err
	}
	var snapshotMax uint
	if len(ids) > 0 {
		snapshotMax = ids[0]
	}
	extra, err := contents.ListInboxIDs(userID, bigVs, snapshotMax, limit)
	if err != nil {
		return err
	}
	if snapshotMax == 0 {
		extra = subtract(extra, ids)
	}
	return fc.AddFollowingIDs(ctx, userID, extra, limit)
}

// --------------------------------------------------------------- outbox --

// EnsureOutbox builds an author's outbox if it is missing.
func (p *Projection) EnsureOutbox(ctx context.Context, authorID uint) error {
	fc := cache.NewFeedCache()
	ready, err := fc.OutboxReady(ctx, authorID)
	if err != nil || ready {
		return err
	}
	_, err, _ = p.group.Do(fmt.Sprintf("outbox:%d", authorID), func() (any, error) {
		if ready, err := fc.OutboxReady(ctx, authorID); err != nil || ready {
			return nil, err
		}
		contents, err := dao.NewContentDao(ctx)
		if err != nil {
			return nil, err
		}
		limit := feedConf().OutboxLimit
		ids, err := contents.ListRecentIDsByAuthor(authorID, 0, limit)
		if err != nil {
			return nil, err
		}
		if err := fc.ReplaceOutbox(ctx, authorID, ids); err != nil {
			return nil, err
		}
		var snapshotMax uint
		if len(ids) > 0 {
			snapshotMax = ids[0]
		}
		extra, err := contents.ListRecentIDsByAuthor(authorID, snapshotMax, limit)
		if err != nil {
			return nil, err
		}
		if snapshotMax == 0 {
			extra = subtract(extra, ids)
		}
		return nil, fc.AddOutbox(ctx, authorID, extra, limit)
	})
	return err
}

// -------------------------------------------------------------- explore --

// RefreshExplorePool recomputes the explore candidates: eligible content from
// the last ExploreWindowDays, newest first. If the window is nearly empty
// (for example the demo data was not reset for a while) it falls back to the
// newest content regardless of age.
func (p *Projection) RefreshExplorePool(ctx context.Context) error {
	_, err, _ := p.group.Do("explore", func() (any, error) {
		contents, err := dao.NewContentDao(ctx)
		if err != nil {
			return nil, err
		}
		cfg := feedConf()
		since := time.Now().AddDate(0, 0, -cfg.ExploreWindowDays)
		ids, err := contents.ListRecentIDs(0, since, cfg.ExplorePoolSize)
		if err != nil {
			return nil, err
		}
		if len(ids) < 50 {
			if ids, err = contents.ListRecentIDs(0, time.Time{}, cfg.ExplorePoolSize); err != nil {
				return nil, err
			}
		}
		return nil, cache.NewFeedCache().ReplaceExplorePool(ctx, ids)
	})
	return err
}

// EnsureExplorePool builds the explore pool on first use.
func (p *Projection) EnsureExplorePool(ctx context.Context) error {
	exists, err := cache.NewFeedCache().ExplorePoolExists(ctx)
	if err != nil || exists {
		return err
	}
	return p.RefreshExplorePool(ctx)
}

// StartExploreRefresher periodically rebuilds the explore pool so content
// ages out of the window. It is a no-op without Redis.
func StartExploreRefresher(ctx context.Context) {
	if global.Redis == nil || global.DB == nil {
		return
	}
	interval := time.Duration(feedConf().ExploreRefreshSeconds) * time.Second
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := GetProjection().RefreshExplorePool(ctx); err != nil {
				log.Printf("explore pool refresh failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// subtract returns the elements of a that are not in b.
func subtract(a, b []uint) []uint {
	if len(a) == 0 || len(b) == 0 {
		return a
	}
	skip := make(map[uint]struct{}, len(b))
	for _, id := range b {
		skip[id] = struct{}{}
	}
	out := a[:0:0]
	for _, id := range a {
		if _, ok := skip[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}
