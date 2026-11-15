package service

import (
	"context"
	"log"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"myzhihu2/repository/cache"
	"myzhihu2/repository/dao"
	"myzhihu2/repository/model"
	"myzhihu2/serializer"
	"myzhihu2/util/global"
)

var (
	// FeedSrvIns is the single stateless Feed service instance.
	FeedSrvIns  *FeedSrv
	feedSrvOnce sync.Once
)

// FeedSrv gathers following, interest and explore candidates and produces one
// ID-cursor page. The following stream is a push/pull hybrid:
//
//	following = my inbox (posts pushed by ordinary authors)
//	          ∪ outbox of every big V I follow (pulled while reading)
//
// Every source is filtered by the same "id < cursor" boundary and the union is
// ordered by descending ID, so pagination stays stable across sources.
type FeedSrv struct{}

// GetFeedSrv returns the Feed service singleton.
func GetFeedSrv() *FeedSrv {
	feedSrvOnce.Do(func() { FeedSrvIns = &FeedSrv{} })
	return FeedSrvIns
}

const (
	feedSourceFollowing = "following"
	feedSourceInterest  = "interest"
	feedSourceExplore   = "explore"
	feedViaPush         = "push"
	feedViaPull         = "pull"
	maxFeedLimit        = 50
)

// followingResult is the merged following stream for one page.
type followingResult struct {
	ids     []uint
	via     map[uint]string // push or pull ("" when served from MySQL fallback)
	pullBy  map[uint]uint   // content ID -> big V author whose outbox supplied it
	hasMore bool
}

// GetFeed constructs a page from the three candidate streams.
func (s *FeedSrv) GetFeed(ctx context.Context, userID, beforeID uint, limit int) (*serializer.FeedPage, error) {
	if userID == 0 {
		return nil, ErrInvalidInput
	}
	if limit < 1 {
		limit = 20
	}
	if limit > maxFeedLimit {
		limit = maxFeedLimit
	}
	contentDAO, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	followDAO, err := dao.NewUserFollowDao(ctx)
	if err != nil {
		return nil, err
	}
	reactionDAO, err := dao.NewReactionDao(ctx)
	if err != nil {
		return nil, err
	}

	// One indexed query (primary key prefix follower_id) gives the authors the
	// reader follows. It identifies followed big Vs and lets the interest and
	// explore streams exclude already-followed authors.
	followedAuthors, err := followDAO.ListFollowingIDs(userID)
	if err != nil {
		return nil, err
	}
	followed := make(map[uint]struct{}, len(followedAuthors)+1)
	for _, id := range followedAuthors {
		followed[id] = struct{}{}
	}
	excluded := append(append([]uint{}, followedAuthors...), userID)

	// Over-fetch each source so deleted IDs and cross-source duplicates rarely
	// make the visible page short. Each source remains bounded.
	candidateLimit := limit*3 + 1
	following, err := s.followingCandidates(ctx, contentDAO, userID, followedAuthors, beforeID, candidateLimit)
	if err != nil {
		return nil, err
	}
	authorIDs, topics, err := s.interestProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	interestIDs, err := contentDAO.ListInterestIDsBefore(beforeID, authorIDs, topics, excluded, candidateLimit)
	if err != nil {
		return nil, err
	}
	exploreIDs, exploreHasMore, err := s.exploreCandidates(ctx, contentDAO, beforeID, candidateLimit)
	if err != nil {
		return nil, err
	}

	// The first source claiming an ID supplies its label: following, then
	// interest, then explore. Final order is the global ID order of the cursor.
	source := make(map[uint]string, len(following.ids)+len(interestIDs)+len(exploreIDs))
	claim := func(ids []uint, label string) {
		for _, id := range ids {
			if _, ok := source[id]; !ok {
				source[id] = label
			}
		}
	}
	claim(following.ids, feedSourceFollowing)
	claim(interestIDs, feedSourceInterest)
	claim(exploreIDs, feedSourceExplore)
	orderedIDs := make([]uint, 0, len(source))
	for id := range source {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Slice(orderedIDs, func(i, j int) bool { return orderedIDs[i] > orderedIDs[j] })

	// Fetch live details in one SQL query; the soft-delete scope drops deleted
	// rows. Projections may briefly keep a deleted ID, so remove those lazily.
	contents, err := contentDAO.ListByIDs(orderedIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[uint]*model.Content, len(contents))
	for i := range contents {
		byID[contents[i].ID] = &contents[i]
	}
	s.removeStale(ctx, userID, following, byID)

	validIDs := make([]uint, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		content := byID[id]
		if content == nil {
			continue
		}
		// Explore samples the whole pool; skip the reader's own posts and posts
		// by followed authors that were not part of this page's following set.
		if source[id] == feedSourceExplore {
			if _, isFollowed := followed[content.AuthorID]; isFollowed || content.AuthorID == userID {
				continue
			}
		}
		validIDs = append(validIDs, id)
	}
	if len(validIDs) > limit {
		validIDs = validIDs[:limit]
	}

	reactionRows, err := reactionDAO.ListByContentIDs(validIDs)
	if err != nil {
		return nil, err
	}
	states := aggregateReactionStates(reactionRows, userID)

	page := &serializer.FeedPage{Items: make([]serializer.FeedItem, 0, len(validIDs))}
	for _, id := range validIDs {
		content := byID[id]
		state := states[id]
		item := serializer.FeedItem{
			Content:         buildContentResponse(content),
			Source:          source[id],
			LikeCount:       state.LikeCount,
			CollectionCount: state.CollectionCount,
			Liked:           state.Liked,
			Collected:       state.Collected,
		}
		_, item.FollowingAuthor = followed[content.AuthorID]
		if item.Source == feedSourceFollowing {
			item.Via = following.via[id]
		}
		page.Items = append(page.Items, item)
	}

	page.HasMore = len(orderedIDs) > limit || following.hasMore ||
		len(interestIDs) == candidateLimit || exploreHasMore
	if len(page.Items) > 0 {
		page.NextCursor = strconv.FormatUint(uint64(page.Items[len(page.Items)-1].Content.ID), 10)
	} else if len(orderedIDs) > 0 {
		// A batch of only stale or filtered IDs still advances the cursor so the
		// client does not request the same batch forever.
		page.NextCursor = strconv.FormatUint(uint64(orderedIDs[len(orderedIDs)-1]), 10)
	}
	return page, nil
}

// followingCandidates merges the reader's push inbox with the outboxes of the
// big V authors the reader follows. Without Redis or RabbitMQ the projections
// cannot be kept current, so MySQL serves the whole following stream.
func (s *FeedSrv) followingCandidates(ctx context.Context, contentDAO *dao.ContentDao, userID uint, followedAuthors []uint, beforeID uint, limit int) (followingResult, error) {
	if global.Redis == nil || global.RMQ == nil {
		return s.followingFromMySQL(contentDAO, userID, beforeID, limit)
	}
	result, err := s.followingFromRedis(ctx, userID, followedAuthors, beforeID, limit)
	if err != nil {
		log.Printf("feed projection unavailable, using MySQL: %v", err)
		return s.followingFromMySQL(contentDAO, userID, beforeID, limit)
	}
	return result, nil
}

func (s *FeedSrv) followingFromMySQL(contentDAO *dao.ContentDao, userID, beforeID uint, limit int) (followingResult, error) {
	ids, err := contentDAO.ListFollowingIDsBefore(userID, beforeID, limit)
	return followingResult{ids: ids, via: map[uint]string{}, pullBy: map[uint]uint{}, hasMore: len(ids) == limit}, err
}

func (s *FeedSrv) followingFromRedis(ctx context.Context, userID uint, followedAuthors []uint, beforeID uint, limit int) (followingResult, error) {
	proj := GetProjection()
	fc := cache.NewFeedCache()

	bigVs, err := proj.BigVs(ctx)
	if err != nil {
		return followingResult{}, err
	}
	followedBigVs := intersect(followedAuthors, bigVs)

	if _, err := proj.EnsureInbox(ctx, userID); err != nil {
		return followingResult{}, err
	}
	inbox, err := fc.ListFollowingBefore(ctx, userID, beforeID, limit)
	if err != nil {
		return followingResult{}, err
	}
	for _, aid := range followedBigVs {
		if err := proj.EnsureOutbox(ctx, aid); err != nil {
			return followingResult{}, err
		}
	}
	outboxes, err := fc.ListOutboxesBefore(ctx, followedBigVs, beforeID, limit)
	if err != nil {
		return followingResult{}, err
	}

	result := followingResult{via: make(map[uint]string), pullBy: make(map[uint]uint)}
	lists := make([][]uint, 0, len(followedBigVs)+1)
	lists = append(lists, inbox)
	result.hasMore = len(inbox) == limit
	for _, id := range inbox {
		result.via[id] = feedViaPush
	}
	for _, aid := range followedBigVs {
		ids := outboxes[aid]
		lists = append(lists, ids)
		result.hasMore = result.hasMore || len(ids) == limit
		for _, id := range ids {
			// A post pushed before its author became a big V can be in both the
			// inbox and the outbox; keep the inbox label.
			if _, pushed := result.via[id]; !pushed {
				result.via[id] = feedViaPull
				result.pullBy[id] = aid
			}
		}
	}
	result.ids = mergeDesc(lists, limit)
	return result, nil
}

// exploreCandidates samples the explore pool: half of the candidates are the
// newest pool members below the cursor, the other half a random sample of the
// following 4x window ("recent first, plus discovery"). This replaces the
// previous ORDER BY RAND() full-table sort.
func (s *FeedSrv) exploreCandidates(ctx context.Context, contentDAO *dao.ContentDao, beforeID uint, limit int) ([]uint, bool, error) {
	window := limit * 4
	var ids []uint
	var err error
	if global.Redis != nil {
		if err = GetProjection().EnsureExplorePool(ctx); err == nil {
			ids, err = cache.NewFeedCache().ListExploreBefore(ctx, beforeID, window)
		}
		if err != nil {
			log.Printf("explore pool unavailable, using MySQL: %v", err)
		}
	}
	if global.Redis == nil || err != nil {
		since := time.Now().AddDate(0, 0, -feedConf().ExploreWindowDays)
		if ids, err = contentDAO.ListRecentIDs(beforeID, since, window); err != nil {
			return nil, false, err
		}
	}
	hasMore := len(ids) == window
	newest := min((limit+1)/2, len(ids))
	rest := ids[newest:]
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, hasMore, nil
}

// removeStale deletes IDs of soft-deleted content from the projection that
// supplied them: the reader's inbox for pushed IDs, a big V outbox for pulled.
func (s *FeedSrv) removeStale(ctx context.Context, userID uint, following followingResult, live map[uint]*model.Content) {
	if global.Redis == nil {
		return
	}
	fc := cache.NewFeedCache()
	var staleInbox []uint
	staleOutbox := make(map[uint][]uint)
	for _, id := range following.ids {
		if live[id] != nil {
			continue
		}
		switch following.via[id] {
		case feedViaPush:
			staleInbox = append(staleInbox, id)
		case feedViaPull:
			staleOutbox[following.pullBy[id]] = append(staleOutbox[following.pullBy[id]], id)
		}
	}
	if len(staleInbox) > 0 {
		_ = fc.RemoveFollowingIDs(ctx, userID, staleInbox)
	}
	for aid, ids := range staleOutbox {
		_ = fc.RemoveOutboxIDs(ctx, aid, ids)
	}
}

// interestProfile reads the reader's author/topic weights from Redis, falling
// back to recent MySQL behavior rows when Redis is empty or unavailable.
func (s *FeedSrv) interestProfile(ctx context.Context, userID uint) ([]uint, []string, error) {
	if global.Redis != nil && global.RMQ != nil {
		weights, err := cache.NewFeedCache().InterestSignals(ctx, userID)
		if err == nil && len(weights) > 0 {
			authors, topics := topInterestSignals(weights)
			return authors, topics, nil
		}
		if err != nil {
			log.Printf("interest cache read failed, using MySQL behavior history: %v", err)
		}
	}
	behaviorDAO, err := dao.NewBehaviorEventDao(ctx)
	if err != nil {
		return nil, nil, err
	}
	events, err := behaviorDAO.ListRecentWithContent(userID, 100)
	if err != nil {
		return nil, nil, err
	}
	weights := make(map[string]int64)
	for _, event := range events {
		if event.Content.ID == 0 {
			continue
		}
		weight := BehaviorWeight(event.Kind, event.Duration)
		weights["author:"+strconv.FormatUint(uint64(event.Content.AuthorID), 10)] += weight
		if field := TopicField(&event.Content); field != "" {
			weights[field] += weight
		}
	}
	authors, topics := topInterestSignals(weights)
	return authors, topics, nil
}

// BehaviorWeight maps a behavior to an interest weight. It is shared by the
// MySQL fallback and the behavior consumer so both compute identical weights.
func BehaviorWeight(kind model.BehaviorKind, duration int) int64 {
	switch kind {
	case model.BehaviorKindLike:
		return 3
	case model.BehaviorKindCollect:
		return 4
	case model.BehaviorKindReadTime:
		weight := int64(1 + duration/60)
		if weight > 5 {
			return 5
		}
		return weight
	default:
		return 1
	}
}

// TopicField is the interest hash field for a content item's topic.
func TopicField(content *model.Content) string {
	if content.Topic == "" {
		return ""
	}
	return "topic:" + content.Topic
}

type interestSignal struct {
	field  string
	weight int64
}

// topInterestSignals returns the 10 strongest authors and 3 strongest topics.
func topInterestSignals(weights map[string]int64) ([]uint, []string) {
	authors := make([]interestSignal, 0)
	topics := make([]interestSignal, 0)
	for field, weight := range weights {
		if strings.HasPrefix(field, "author:") {
			authors = append(authors, interestSignal{field: strings.TrimPrefix(field, "author:"), weight: weight})
		} else if topic := strings.TrimPrefix(field, "topic:"); topic != field && model.ValidTopic(topic) {
			topics = append(topics, interestSignal{field: topic, weight: weight})
		}
	}
	byWeight := func(list []interestSignal) {
		sort.Slice(list, func(i, j int) bool {
			if list[i].weight == list[j].weight {
				return list[i].field < list[j].field
			}
			return list[i].weight > list[j].weight
		})
	}
	byWeight(authors)
	byWeight(topics)
	if len(authors) > 10 {
		authors = authors[:10]
	}
	// A topic joins the interest stream only if its weight reaches 30% of the
	// strongest topic, so one incidental collect or view does not dilute it.
	if len(topics) > 0 {
		top := topics[0].weight
		kept := topics[:0]
		for _, t := range topics {
			if t.weight*10 >= top*3 {
				kept = append(kept, t)
			}
		}
		topics = kept
	}
	if len(topics) > 3 {
		topics = topics[:3]
	}
	authorIDs := make([]uint, 0, len(authors))
	for _, signal := range authors {
		if id, err := strconv.ParseUint(signal.field, 10, 64); err == nil && id > 0 {
			authorIDs = append(authorIDs, uint(id))
		}
	}
	topicNames := make([]string, 0, len(topics))
	for _, signal := range topics {
		topicNames = append(topicNames, signal.field)
	}
	return authorIDs, topicNames
}

// mergeDesc merges several newest-first ID lists into one deduplicated
// newest-first list of at most limit IDs (k-way merge).
func mergeDesc(lists [][]uint, limit int) []uint {
	pos := make([]int, len(lists))
	out := make([]uint, 0, limit)
	var last uint
	for len(out) < limit {
		best := -1
		for i, list := range lists {
			if pos[i] < len(list) && (best == -1 || list[pos[i]] > lists[best][pos[best]]) {
				best = i
			}
		}
		if best == -1 {
			break
		}
		id := lists[best][pos[best]]
		pos[best]++
		if len(out) > 0 && id == last {
			continue
		}
		out = append(out, id)
		last = id
	}
	return out
}

// intersect returns the elements of a that also appear in b, preserving a's order.
func intersect(a, b []uint) []uint {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	in := make(map[uint]struct{}, len(b))
	for _, id := range b {
		in[id] = struct{}{}
	}
	out := make([]uint, 0)
	for _, id := range a {
		if _, ok := in[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

type aggregatedReaction struct {
	LikeCount       int64
	CollectionCount int64
	Liked           bool
	Collected       bool
}

func aggregateReactionStates(rows []model.Reaction, userID uint) map[uint]aggregatedReaction {
	states := make(map[uint]aggregatedReaction)
	for _, reaction := range rows {
		id := reaction.ContentID
		state := states[id]
		switch reaction.Kind {
		case "like":
			state.LikeCount++
			if reaction.UserID == userID {
				state.Liked = true
			}
		case "collect":
			state.CollectionCount++
			if reaction.UserID == userID {
				state.Collected = true
			}
		}
		states[id] = state
	}
	return states
}
