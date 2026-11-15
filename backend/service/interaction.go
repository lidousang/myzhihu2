package service

import (
	"context"
	"errors"
	"sync"

	"myzhihu2/repository/dao"
	"myzhihu2/repository/model"
	"myzhihu2/serializer"
	"myzhihu2/util/global"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// InteractionSrvIns is the singleton for follows, reactions, and behavior reports.
	InteractionSrvIns  *InteractionSrv
	interactionSrvOnce sync.Once
)

// InteractionSrv contains business operations on relationships and user actions.
type InteractionSrv struct{}

// GetInteractionSrv returns the shared stateless interaction service.
func GetInteractionSrv() *InteractionSrv {
	interactionSrvOnce.Do(func() { InteractionSrvIns = &InteractionSrv{} })
	return InteractionSrvIns
}

// FollowUser verifies the target account, stores the relationship, and publishes
// an event so a Feed consumer can backfill the target user's recent content.
func (s *InteractionSrv) FollowUser(ctx context.Context, followerID, followingID uint) error {
	if followerID == 0 || followingID == 0 {
		return ErrInvalidInput
	}
	if followerID == followingID {
		return ErrSelfFollow
	}
	users, err := dao.NewUserDao(ctx)
	if err != nil {
		return err
	}
	if _, err := users.FindByID(followingID); dao.IsNotFound(err) {
		return ErrUserNotFound
	} else if err != nil {
		return err
	}
	if err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.UserFollow{FollowerID: followerID, FollowingID: followingID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		// The follower count decides push versus pull, so it changes in the same
		// transaction as the relationship it counts.
		if err := tx.Model(&model.User{}).Where("id = ?", followingID).
			UpdateColumn("follower_count", gorm.Expr("follower_count + 1")).Error; err != nil {
			return err
		}
		return createOutboxTx(ctx, tx, "user.followed", "feed.content", map[string]uint{"follower_id": followerID, "following_id": followingID})
	}); err != nil {
		return err
	}
	return nil
}

// UnfollowUser removes the relationship. The event allows a future Feed
// consumer to apply whatever simple cleanup policy is selected.
func (s *InteractionSrv) UnfollowUser(ctx context.Context, followerID, followingID uint) error {
	if followerID == 0 || followingID == 0 {
		return ErrInvalidInput
	}
	if followerID == followingID {
		return ErrSelfFollow
	}
	if err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("follower_id = ? AND following_id = ?", followerID, followingID).Delete(&model.UserFollow{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Model(&model.User{}).Where("id = ? AND follower_count > 0", followingID).
			UpdateColumn("follower_count", gorm.Expr("follower_count - 1")).Error; err != nil {
			return err
		}
		return createOutboxTx(ctx, tx, "user.unfollowed", "feed.content", map[string]uint{"follower_id": followerID, "following_id": followingID})
	}); err != nil {
		return err
	}
	return nil
}

// Like creates the user's current like state and records a like behavior only
// when the relation was newly inserted. The behavior event is then published
// for asynchronous interest-signal processing.
func (s *InteractionSrv) Like(ctx context.Context, userID, contentID uint) (*serializer.InteractionState, error) {
	return s.addReaction(ctx, userID, contentID, "like", model.BehaviorKindLike)
}

// Unlike removes the user's current like state. It does not erase historical
// behavior, because a previous like can still be a recommendation signal.
func (s *InteractionSrv) Unlike(ctx context.Context, userID, contentID uint) (*serializer.InteractionState, error) {
	return s.removeReaction(ctx, userID, contentID, "like")
}

// Collect creates the user's current collection state and records a behavior
// signal if it is a newly created relation.
func (s *InteractionSrv) Collect(ctx context.Context, userID, contentID uint) (*serializer.InteractionState, error) {
	return s.addReaction(ctx, userID, contentID, "collect", model.BehaviorKindCollect)
}

// Uncollect removes the current collection relation without deleting history.
func (s *InteractionSrv) Uncollect(ctx context.Context, userID, contentID uint) (*serializer.InteractionState, error) {
	return s.removeReaction(ctx, userID, contentID, "collect")
}

func (s *InteractionSrv) addReaction(ctx context.Context, userID, contentID uint, reactionKind string, behaviorKind model.BehaviorKind) (*serializer.InteractionState, error) {
	if userID == 0 || contentID == 0 {
		return nil, ErrInvalidInput
	}
	contentDAO, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	exists, err := contentDAO.ExistsPublicContent(contentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrContentNotFound
	}
	if err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reaction := &model.Reaction{UserID: userID, ContentID: contentID, Kind: reactionKind}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(reaction)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		behavior := &model.BehaviorEvent{UserID: userID, ContentID: contentID, Kind: behaviorKind}
		if err := tx.Create(behavior).Error; err != nil {
			return err
		}
		return createOutboxTx(ctx, tx, "user.behavior", "user.behavior", map[string]any{
			"user_id": behavior.UserID, "content_id": behavior.ContentID, "kind": behavior.Kind, "duration": behavior.Duration,
		})
	}); err != nil {
		return nil, err
	}
	return s.GetReactionState(ctx, userID, contentID)
}

func (s *InteractionSrv) removeReaction(ctx context.Context, userID, contentID uint, kind string) (*serializer.InteractionState, error) {
	if userID == 0 || contentID == 0 {
		return nil, ErrInvalidInput
	}
	contentDAO, err := dao.NewContentDao(ctx)
	if err != nil {
		return nil, err
	}
	exists, err := contentDAO.ExistsPublicContent(contentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrContentNotFound
	}
	if err := global.DB.WithContext(ctx).Where("user_id = ? AND content_id = ? AND kind = ?", userID, contentID, kind).Delete(&model.Reaction{}).Error; err != nil {
		return nil, err
	}
	return s.GetReactionState(ctx, userID, contentID)
}

// GetReactionState returns counts and the optional current user's like/collect
// flags. A userID of zero is used for anonymous public reads.
func (s *InteractionSrv) GetReactionState(ctx context.Context, userID, contentID uint) (*serializer.InteractionState, error) {
	reactions, err := dao.NewReactionDao(ctx)
	if err != nil {
		return nil, err
	}
	likes, err := reactions.Count(contentID, "like")
	if err != nil {
		return nil, err
	}
	collections, err := reactions.Count(contentID, "collect")
	if err != nil {
		return nil, err
	}
	state := &serializer.InteractionState{ContentID: contentID, LikeCount: likes, CollectionCount: collections}
	if userID > 0 {
		state.Liked, err = reactions.Exists(userID, contentID, "like")
		if err != nil {
			return nil, err
		}
		state.Collected, err = reactions.Exists(userID, contentID, "collect")
		if err != nil {
			return nil, err
		}
	}
	return state, nil
}

// RecordBehavior persists a view or reading-duration event and publishes it to
// the behavior queue for asynchronous interest-feed processing.
func (s *InteractionSrv) RecordBehavior(ctx context.Context, userID uint, req serializer.RecordBehaviorRequest) error {
	if userID == 0 || req.ContentID == 0 || req.Duration < 0 || req.Duration > 86400 {
		return ErrInvalidInput
	}
	var kind model.BehaviorKind
	switch req.Kind {
	case string(model.BehaviorKindView):
		kind = model.BehaviorKindView
	case string(model.BehaviorKindReadTime):
		kind = model.BehaviorKindReadTime
	default:
		return ErrInvalidInput
	}
	contentDAO, err := dao.NewContentDao(ctx)
	if err != nil {
		return err
	}
	exists, err := contentDAO.ExistsPublicContent(req.ContentID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrContentNotFound
	}
	return global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		event := &model.BehaviorEvent{UserID: userID, ContentID: req.ContentID, Kind: kind, Duration: req.Duration}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		return createOutboxTx(ctx, tx, "user.behavior", "user.behavior", map[string]any{
			"user_id": event.UserID, "content_id": event.ContentID, "kind": event.Kind, "duration": event.Duration,
		})
	})
}

// IsDomainError can be used by API code to recognize service-level errors.
func IsDomainError(err error) bool {
	return errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrContentNotFound) || errors.Is(err, ErrForbidden)
}
