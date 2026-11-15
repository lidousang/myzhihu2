package dao

import (
	"context"
	"errors"

	"myzhihu2/repository/model"
	"myzhihu2/util/global"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserFollowDao stores and queries user-to-user follow relationships.
type UserFollowDao struct{ db *gorm.DB }

// NewUserFollowDao binds the global DB connection to the supplied request context.
func NewUserFollowDao(ctx context.Context) (*UserFollowDao, error) {
	if global.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &UserFollowDao{db: global.DB.WithContext(ctx)}, nil
}

// Create inserts the relationship. OnConflict DoNothing makes repeated follow
// requests idempotent while the composite primary key remains the DB guarantee.
func (d *UserFollowDao) Create(follow *model.UserFollow) (bool, error) {
	result := d.db.Clauses(clause.OnConflict{DoNothing: true}).Create(follow)
	return result.RowsAffected > 0, result.Error
}

// Delete removes a follow relationship. Removing a missing relationship is
// treated as success, which makes unfollow safe to retry.
func (d *UserFollowDao) Delete(followerID, followingID uint) (bool, error) {
	result := d.db.Where("follower_id = ? AND following_id = ?", followerID, followingID).
		Delete(&model.UserFollow{})
	return result.RowsAffected > 0, result.Error
}

// Exists reports whether followerID currently follows followingID.
func (d *UserFollowDao) Exists(followerID, followingID uint) (bool, error) {
	var count int64
	err := d.db.Model(&model.UserFollow{}).Where("follower_id = ? AND following_id = ?", followerID, followingID).Count(&count).Error
	return count > 0, err
}

// ListFollowerIDs returns user IDs that follow the specified author. The Feed
// publisher/consumer uses this lookup when fanning out a new content reference.
func (d *UserFollowDao) ListFollowerIDs(authorID uint) ([]uint, error) {
	var ids []uint
	err := d.db.Model(&model.UserFollow{}).Where("following_id = ?", authorID).
		Pluck("follower_id", &ids).Error
	return ids, err
}

// ListFollowingIDs returns the authors followed by a user, used to collect
// candidates for the following feed.
func (d *UserFollowDao) ListFollowingIDs(userID uint) ([]uint, error) {
	var ids []uint
	err := d.db.Model(&model.UserFollow{}).Where("follower_id = ?", userID).
		Pluck("following_id", &ids).Error
	return ids, err
}

// EachFollowerBatch walks an author's followers with a keyset cursor on
// follower_id, so even a very large follower list is read in bounded batches.
func (d *UserFollowDao) EachFollowerBatch(authorID uint, batch int, fn func([]uint) error) error {
	var after uint
	for {
		var ids []uint
		err := d.db.Model(&model.UserFollow{}).
			Where("following_id = ? AND follower_id > ?", authorID, after).
			Order("follower_id ASC").Limit(batch).Pluck("follower_id", &ids).Error
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := fn(ids); err != nil {
			return err
		}
		if len(ids) < batch {
			return nil
		}
		after = ids[len(ids)-1]
	}
}

// FollowingAmong returns which of authorIDs are followed by userID.
func (d *UserFollowDao) FollowingAmong(userID uint, authorIDs []uint) ([]uint, error) {
	if len(authorIDs) == 0 {
		return []uint{}, nil
	}
	var ids []uint
	err := d.db.Model(&model.UserFollow{}).
		Where("follower_id = ? AND following_id IN ?", userID, authorIDs).
		Pluck("following_id", &ids).Error
	return ids, err
}
