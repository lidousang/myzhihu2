package dao

import (
	"context"
	"errors"

	"myzhihu2/repository/model"
	"myzhihu2/util/global"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReactionDao stores current like and collection states.
type ReactionDao struct{ db *gorm.DB }

// NewReactionDao binds the shared database pool to a request context.
func NewReactionDao(ctx context.Context) (*ReactionDao, error) {
	if global.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &ReactionDao{db: global.DB.WithContext(ctx)}, nil
}

// Create records a reaction and ignores a duplicate request. The returned bool
// says whether a row was inserted, allowing the service to avoid duplicate
// behavior events for retries.
func (d *ReactionDao) Create(reaction *model.Reaction) (bool, error) {
	result := d.db.Clauses(clause.OnConflict{DoNothing: true}).Create(reaction)
	return result.RowsAffected > 0, result.Error
}

// Delete removes one current reaction state. Repeating the request is harmless.
func (d *ReactionDao) Delete(userID, contentID uint, kind string) error {
	return d.db.Where("user_id = ? AND content_id = ? AND kind = ?", userID, contentID, kind).
		Delete(&model.Reaction{}).Error
}

// Exists checks the current user's state for one content and reaction kind.
func (d *ReactionDao) Exists(userID, contentID uint, kind string) (bool, error) {
	var count int64
	err := d.db.Model(&model.Reaction{}).Where("user_id = ? AND content_id = ? AND kind = ?", userID, contentID, kind).Count(&count).Error
	return count > 0, err
}

// Count returns the aggregate count for a reaction kind on a content item.
func (d *ReactionDao) Count(contentID uint, kind string) (int64, error) {
	var count int64
	err := d.db.Model(&model.Reaction{}).Where("content_id = ? AND kind = ?", contentID, kind).Count(&count).Error
	return count, err
}

// ListByContentIDs fetches interaction rows for a Feed page in one query. The
// service aggregates counts and the current user's flags in memory to avoid an
// N+1 query per card.
func (d *ReactionDao) ListByContentIDs(contentIDs []uint) ([]model.Reaction, error) {
	if len(contentIDs) == 0 {
		return []model.Reaction{}, nil
	}
	var reactions []model.Reaction
	err := d.db.Where("content_id IN ? AND kind IN ?", contentIDs, []string{"like", "collect"}).Find(&reactions).Error
	return reactions, err
}

// IsNotFound is retained here only for consistency with DAO call sites.
func (d *ReactionDao) IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
