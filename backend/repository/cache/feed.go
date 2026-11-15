// Package cache wraps Redis structures used by the personalized Feed.
// MySQL remains authoritative; every key here is a rebuildable projection.
//
// Keys:
//
//	feed:user:{uid}          ZSET  following inbox (pushed content IDs, score = ID)
//	feed:ready:{uid}         STR   inbox built marker (distinguishes empty from missing)
//	feed:outbox:{aid}        ZSET  author outbox (read by followers of big V authors)
//	feed:outbox:ready:{aid}  STR   outbox built marker
//	feed:bigv                SET   authors currently in pull mode
//	feed:bigv:ready          STR   big V set built marker
//	feed:explore:pool        ZSET  recent explore candidates
//	feed:fanout:{cid}        STR   JSON fan-out statistics for the demo (24h TTL)
//	interest:user:{uid}      HASH  author:{id} / topic:{topic} weights
//	processed:event:{eid}    STR   consumer idempotency marker (24h TTL)
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"myzhihu2/serializer"
	"myzhihu2/util/global"

	redis "github.com/redis/go-redis/v9"
)

var ErrRedisUnavailable = errors.New("redis is not initialized")

// behaviorInterestScript atomically checks a message id and applies the two
// interest increments. HINCRBY alone is not idempotent: a redelivered like
// event would otherwise increase the weight twice.
var behaviorInterestScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 1 then
    return 0
end
redis.call("HINCRBY", KEYS[1], ARGV[1], ARGV[2])
redis.call("HINCRBY", KEYS[1], ARGV[3], ARGV[2])
redis.call("SET", KEYS[2], "1", "EX", ARGV[4])
return 1
`)

// FeedCache is a small repository for the Redis keys used by FeedSrv and its
// RabbitMQ consumers. It uses the shared client initialized by core.InitRedis.
type FeedCache struct {
	client *redis.Client
}

// NewFeedCache creates a wrapper around the application's shared Redis client.
func NewFeedCache() *FeedCache {
	return &FeedCache{client: global.Redis}
}

// Available reports whether Redis is configured.
func (c *FeedCache) Available() bool { return c.client != nil }

func feedKey(userID uint) string          { return fmt.Sprintf("feed:user:%d", userID) }
func readyKey(userID uint) string         { return fmt.Sprintf("feed:ready:%d", userID) }
func outboxKey(authorID uint) string      { return fmt.Sprintf("feed:outbox:%d", authorID) }
func outboxReadyKey(authorID uint) string { return fmt.Sprintf("feed:outbox:ready:%d", authorID) }
func interestKey(userID uint) string      { return fmt.Sprintf("interest:user:%d", userID) }
func fanoutKey(contentID uint) string     { return fmt.Sprintf("feed:fanout:%d", contentID) }

const (
	bigVKey         = "feed:bigv"
	bigVReadyKey    = "feed:bigv:ready"
	explorePoolKey  = "feed:explore:pool"
	fanoutStatsTTL  = 24 * time.Hour
	processedMarker = 86400 // seconds
)

func idMember(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func parseIDs(values []string) []uint {
	ids := make([]uint, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseUint(value, 10, 64)
		if err == nil && id > 0 {
			ids = append(ids, uint(id))
		}
	}
	return ids
}

func zMembers(ids []uint) []redis.Z {
	items := make([]redis.Z, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			items = append(items, redis.Z{Score: float64(id), Member: idMember(id)})
		}
	}
	return items
}

// beforeRange builds an exclusive "score < beforeID" range. Score equals the
// content ID, so Redis and MySQL share the same cursor boundary.
func beforeRange(beforeID uint, limit int) *redis.ZRangeBy {
	max := "+inf"
	if beforeID > 0 {
		max = "(" + idMember(beforeID)
	}
	return &redis.ZRangeBy{Max: max, Min: "-inf", Offset: 0, Count: int64(limit)}
}

func (c *FeedCache) exists(ctx context.Context, key string) (bool, error) {
	if c.client == nil {
		return false, ErrRedisUnavailable
	}
	n, err := c.client.Exists(ctx, key).Result()
	return n == 1, err
}

// replaceZSet atomically swaps a projection: the new members are written to a
// temporary key and RENAMEd over the live key, then the ready marker is set.
// Readers never observe a half-built set. Writes made by consumers between the
// caller's MySQL snapshot and this swap are recovered by the caller's catch-up
// query, which runs after the ready marker exists.
func (c *FeedCache) replaceZSet(ctx context.Context, key, ready string, ids []uint) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	items := zMembers(ids)
	pipe := c.client.TxPipeline()
	if len(items) == 0 {
		pipe.Del(ctx, key)
	} else {
		tmp := fmt.Sprintf("%s:tmp:%d", key, time.Now().UnixNano())
		pipe.ZAdd(ctx, tmp, items...)
		pipe.Rename(ctx, tmp, key)
	}
	if ready != "" {
		pipe.Set(ctx, ready, "1", 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ---------------------------------------------------------------- inbox --

// IsInitialized checks the inbox ready marker. A ZSET cannot represent an
// empty set, so the marker distinguishes "built and empty" from "not built".
func (c *FeedCache) IsInitialized(ctx context.Context, userID uint) (bool, error) {
	return c.exists(ctx, readyKey(userID))
}

// ListFollowingBefore reads inbox content IDs newest-first below the cursor.
func (c *FeedCache) ListFollowingBefore(ctx context.Context, userID, beforeID uint, limit int) ([]uint, error) {
	if c.client == nil {
		return nil, ErrRedisUnavailable
	}
	values, err := c.client.ZRevRangeByScore(ctx, feedKey(userID), beforeRange(beforeID, limit)).Result()
	return parseIDs(values), err
}

// ReplaceFollowingIDs rebuilds one user's inbox and sets its ready marker.
func (c *FeedCache) ReplaceFollowingIDs(ctx context.Context, userID uint, contentIDs []uint) error {
	return c.replaceZSet(ctx, feedKey(userID), readyKey(userID), contentIDs)
}

// AddFollowingIDs adds IDs to an inbox and trims it. ZSET members are unique,
// so redelivered events do not duplicate entries.
func (c *FeedCache) AddFollowingIDs(ctx context.Context, userID uint, contentIDs []uint, limit int) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	items := zMembers(contentIDs)
	if len(items) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, feedKey(userID), items...)
	if limit > 0 {
		pipe.ZRemRangeByRank(ctx, feedKey(userID), 0, int64(-limit-1))
	}
	_, err := pipe.Exec(ctx)
	return err
}

// RemoveFollowingIDs removes stale entries from one inbox.
func (c *FeedCache) RemoveFollowingIDs(ctx context.Context, userID uint, contentIDs []uint) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	if len(contentIDs) == 0 {
		return nil
	}
	members := make([]any, 0, len(contentIDs))
	for _, id := range contentIDs {
		members = append(members, idMember(id))
	}
	return c.client.ZRem(ctx, feedKey(userID), members...).Err()
}

// FilterReady returns the users that already have an inbox. Only these
// "active" followers receive pushed content; the others get a full rebuild
// from MySQL when they next open the feed.
func (c *FeedCache) FilterReady(ctx context.Context, userIDs []uint) ([]uint, error) {
	if c.client == nil {
		return nil, ErrRedisUnavailable
	}
	if len(userIDs) == 0 {
		return nil, nil
	}
	pipe := c.client.Pipeline()
	cmds := make([]*redis.IntCmd, len(userIDs))
	for i, id := range userIDs {
		cmds[i] = pipe.Exists(ctx, readyKey(id))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	active := make([]uint, 0, len(userIDs))
	for i, cmd := range cmds {
		if cmd.Val() == 1 {
			active = append(active, userIDs[i])
		}
	}
	return active, nil
}

// PushInboxBatch writes one content ID to many inboxes in a single pipeline:
// ZADD plus ZREMRANGEBYRANK per user. It returns the number of ZADD commands.
func (c *FeedCache) PushInboxBatch(ctx context.Context, userIDs []uint, contentIDs []uint, limit int) (int, error) {
	if c.client == nil {
		return 0, ErrRedisUnavailable
	}
	items := zMembers(contentIDs)
	if len(userIDs) == 0 || len(items) == 0 {
		return 0, nil
	}
	pipe := c.client.Pipeline()
	for _, uid := range userIDs {
		pipe.ZAdd(ctx, feedKey(uid), items...)
		if limit > 0 {
			pipe.ZRemRangeByRank(ctx, feedKey(uid), 0, int64(-limit-1))
		}
	}
	_, err := pipe.Exec(ctx)
	return len(userIDs), err
}

// RemoveFromInboxes removes one content ID from many inboxes in one pipeline.
func (c *FeedCache) RemoveFromInboxes(ctx context.Context, userIDs []uint, contentID uint) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	if len(userIDs) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	for _, uid := range userIDs {
		pipe.ZRem(ctx, feedKey(uid), idMember(contentID))
	}
	_, err := pipe.Exec(ctx)
	return err
}

// --------------------------------------------------------------- outbox --

// OutboxReady reports whether an author's outbox has been built.
func (c *FeedCache) OutboxReady(ctx context.Context, authorID uint) (bool, error) {
	return c.exists(ctx, outboxReadyKey(authorID))
}

// ReplaceOutbox rebuilds one author's outbox and sets its ready marker.
func (c *FeedCache) ReplaceOutbox(ctx context.Context, authorID uint, contentIDs []uint) error {
	return c.replaceZSet(ctx, outboxKey(authorID), outboxReadyKey(authorID), contentIDs)
}

// AddOutbox adds content IDs to an author's outbox and trims it.
func (c *FeedCache) AddOutbox(ctx context.Context, authorID uint, contentIDs []uint, limit int) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	items := zMembers(contentIDs)
	if len(items) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, outboxKey(authorID), items...)
	if limit > 0 {
		pipe.ZRemRangeByRank(ctx, outboxKey(authorID), 0, int64(-limit-1))
	}
	_, err := pipe.Exec(ctx)
	return err
}

// RemoveOutboxIDs removes content IDs from an author's outbox.
func (c *FeedCache) RemoveOutboxIDs(ctx context.Context, authorID uint, contentIDs []uint) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	if len(contentIDs) == 0 {
		return nil
	}
	members := make([]any, 0, len(contentIDs))
	for _, id := range contentIDs {
		members = append(members, idMember(id))
	}
	return c.client.ZRem(ctx, outboxKey(authorID), members...).Err()
}

// ListOutboxesBefore reads several author outboxes below the cursor in one
// pipeline. The result maps author ID to its newest-first content IDs.
func (c *FeedCache) ListOutboxesBefore(ctx context.Context, authorIDs []uint, beforeID uint, limit int) (map[uint][]uint, error) {
	if c.client == nil {
		return nil, ErrRedisUnavailable
	}
	result := make(map[uint][]uint, len(authorIDs))
	if len(authorIDs) == 0 {
		return result, nil
	}
	pipe := c.client.Pipeline()
	cmds := make([]*redis.StringSliceCmd, len(authorIDs))
	for i, aid := range authorIDs {
		cmds[i] = pipe.ZRevRangeByScore(ctx, outboxKey(aid), beforeRange(beforeID, limit))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	for i, aid := range authorIDs {
		result[aid] = parseIDs(cmds[i].Val())
	}
	return result, nil
}

// ---------------------------------------------------------------- big V --

// BigVReady reports whether the big V set has been built.
func (c *FeedCache) BigVReady(ctx context.Context) (bool, error) {
	return c.exists(ctx, bigVReadyKey)
}

// ReplaceBigVSet rebuilds the big V set from MySQL follower counts.
func (c *FeedCache) ReplaceBigVSet(ctx context.Context, authorIDs []uint) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	pipe := c.client.TxPipeline()
	pipe.Del(ctx, bigVKey)
	if len(authorIDs) > 0 {
		members := make([]any, 0, len(authorIDs))
		for _, id := range authorIDs {
			members = append(members, idMember(id))
		}
		pipe.SAdd(ctx, bigVKey, members...)
	}
	pipe.Set(ctx, bigVReadyKey, "1", 0)
	_, err := pipe.Exec(ctx)
	return err
}

// BigVMembers returns every author currently in pull mode.
func (c *FeedCache) BigVMembers(ctx context.Context) ([]uint, error) {
	if c.client == nil {
		return nil, ErrRedisUnavailable
	}
	values, err := c.client.SMembers(ctx, bigVKey).Result()
	return parseIDs(values), err
}

// IsBigV reports whether an author is in pull mode.
func (c *FeedCache) IsBigV(ctx context.Context, authorID uint) (bool, error) {
	if c.client == nil {
		return false, ErrRedisUnavailable
	}
	return c.client.SIsMember(ctx, bigVKey, idMember(authorID)).Result()
}

// SetBigV adds (true) or removes (false) an author from the big V set and
// reports whether membership changed.
func (c *FeedCache) SetBigV(ctx context.Context, authorID uint, bigV bool) (bool, error) {
	if c.client == nil {
		return false, ErrRedisUnavailable
	}
	var n int64
	var err error
	if bigV {
		n, err = c.client.SAdd(ctx, bigVKey, idMember(authorID)).Result()
	} else {
		n, err = c.client.SRem(ctx, bigVKey, idMember(authorID)).Result()
	}
	return n == 1, err
}

// -------------------------------------------------------------- explore --

// ExplorePoolExists reports whether the explore pool key exists.
func (c *FeedCache) ExplorePoolExists(ctx context.Context) (bool, error) {
	return c.exists(ctx, explorePoolKey)
}

// ReplaceExplorePool swaps in a freshly computed explore candidate pool.
func (c *FeedCache) ReplaceExplorePool(ctx context.Context, contentIDs []uint) error {
	return c.replaceZSet(ctx, explorePoolKey, "", contentIDs)
}

// AddExplore adds new content to the explore pool and trims it.
func (c *FeedCache) AddExplore(ctx context.Context, contentID uint, limit int) error {
	return c.addTrim(ctx, explorePoolKey, contentID, limit)
}

// RemoveExplore removes deleted content from the explore pool.
func (c *FeedCache) RemoveExplore(ctx context.Context, contentID uint) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	return c.client.ZRem(ctx, explorePoolKey, idMember(contentID)).Err()
}

// ListExploreBefore returns the newest pool members below the cursor.
func (c *FeedCache) ListExploreBefore(ctx context.Context, beforeID uint, limit int) ([]uint, error) {
	if c.client == nil {
		return nil, ErrRedisUnavailable
	}
	values, err := c.client.ZRevRangeByScore(ctx, explorePoolKey, beforeRange(beforeID, limit)).Result()
	return parseIDs(values), err
}

func (c *FeedCache) addTrim(ctx context.Context, key string, id uint, limit int) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(id), Member: idMember(id)})
	if limit > 0 {
		pipe.ZRemRangeByRank(ctx, key, 0, int64(-limit-1))
	}
	_, err := pipe.Exec(ctx)
	return err
}

// --------------------------------------------------------- fanout stats --

// SetFanoutStats stores the delivery statistics of one content.created event.
func (c *FeedCache) SetFanoutStats(ctx context.Context, stats serializer.FanoutStats) error {
	if c.client == nil {
		return ErrRedisUnavailable
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, fanoutKey(stats.ContentID), data, fanoutStatsTTL).Err()
}

// GetFanoutStats returns stored statistics; ok is false until the consumer
// has processed the event.
func (c *FeedCache) GetFanoutStats(ctx context.Context, contentID uint) (serializer.FanoutStats, bool, error) {
	var stats serializer.FanoutStats
	if c.client == nil {
		return stats, false, ErrRedisUnavailable
	}
	data, err := c.client.Get(ctx, fanoutKey(contentID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return stats, false, nil
	}
	if err != nil {
		return stats, false, err
	}
	err = json.Unmarshal(data, &stats)
	return stats, err == nil, err
}

// ------------------------------------------------------------- interest --

// InterestSignals reads author/topic weights maintained from behavior events.
// Field names use author:<id> and topic:<topic>.
func (c *FeedCache) InterestSignals(ctx context.Context, userID uint) (map[string]int64, error) {
	if c.client == nil {
		return nil, ErrRedisUnavailable
	}
	values, err := c.client.HGetAll(ctx, interestKey(userID)).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(values))
	for field, raw := range values {
		weight, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr == nil && weight > 0 {
			result[field] = weight
		}
	}
	return result, nil
}

// AddBehaviorInterestOnce applies an interest event only once for eventID.
// Lua keeps the dedupe marker and both increments atomic inside Redis.
func (c *FeedCache) AddBehaviorInterestOnce(ctx context.Context, eventID string, userID, authorID uint, topicField string, amount int64) (bool, error) {
	if c.client == nil {
		return false, ErrRedisUnavailable
	}
	if eventID == "" || userID == 0 || authorID == 0 || topicField == "" || amount <= 0 {
		return false, nil
	}
	result, err := behaviorInterestScript.Run(ctx, c.client, []string{
		interestKey(userID),
		"processed:event:" + eventID,
	}, "author:"+idMember(authorID), amount, topicField, processedMarker).Int()
	return result == 1, err
}
