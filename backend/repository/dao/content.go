package dao

import (
	"context"
	"errors"
	"time"

	"myzhihu2/repository/model"
	"myzhihu2/util/global"

	"gorm.io/gorm"
)

// ContentDao contains queries for the unified contents table.
type ContentDao struct{ db *gorm.DB }

// NewContentDao binds the shared DB pool to the current request context.
func NewContentDao(ctx context.Context) (*ContentDao, error) {
	if global.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &ContentDao{db: global.DB.WithContext(ctx)}, nil
}

// Create inserts a question, answer, article, or comment model after the
// ContentSrv has checked the kind/parent rules.
func (d *ContentDao) Create(content *model.Content) error {
	return d.db.Create(content).Error
}

// FindByID loads a non-deleted content item and its author for serialization.
func (d *ContentDao) FindByID(id uint) (*model.Content, error) {
	var content model.Content
	err := d.db.Preload("Author").First(&content, id).Error
	if err != nil {
		return nil, err
	}
	return &content, nil
}

// Update changes only whitelisted fields supplied by ContentSrv.
func (d *ContentDao) Update(id uint, fields map[string]any) error {
	result := d.db.Model(&model.Content{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := d.db.Model(&model.Content{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

// Delete soft-deletes a content row by setting deleted_at.
func (d *ContentDao) Delete(id uint) error {
	result := d.db.Delete(&model.Content{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListByParent returns one page of children, such as answers or comments.
func (d *ContentDao) ListByParent(parentID uint, kind model.ContentKind, page, limit int) ([]model.Content, int64, error) {
	query := d.db.Model(&model.Content{}).Where("parent_id = ? AND kind = ?", parentID, kind)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.Content
	err := query.Preload("Author").Order("created_at DESC, id DESC").
		Offset((page - 1) * limit).Limit(limit).Find(&items).Error
	return items, total, err
}

// ListByAuthor returns a user's public content, optionally restricted to one
// content kind. Comments are excluded because they are not profile posts.
func (d *ContentDao) ListByAuthor(authorID uint, kind model.ContentKind, page, limit int) ([]model.Content, int64, error) {
	query := d.db.Model(&model.Content{}).Where("author_id = ? AND kind IN ?", authorID,
		[]model.ContentKind{model.ContentKindQuestion, model.ContentKindAnswer, model.ContentKindArticle})
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.Content
	err := query.Preload("Author").Order("created_at DESC, id DESC").
		Offset((page - 1) * limit).Limit(limit).Find(&items).Error
	return items, total, err
}

// ListInboxIDs returns the content IDs that belong in a user's push inbox:
// posts by followed authors, excluding big V authors (they are pulled at read
// time). afterID > 0 restricts the result to IDs newer than a rebuild snapshot,
// which is how a rebuild catches up with posts committed during the rebuild.
func (d *ContentDao) ListInboxIDs(userID uint, excludeAuthors []uint, afterID uint, limit int) ([]uint, error) {
	query := d.db.Model(&model.Content{}).
		Joins("JOIN user_follows ON user_follows.following_id = contents.author_id").
		Where("user_follows.follower_id = ?", userID).
		Where("contents.kind IN ?", feedContentKinds())
	if len(excludeAuthors) > 0 {
		query = query.Where("contents.author_id NOT IN ?", excludeAuthors)
	}
	if afterID > 0 {
		query = query.Where("contents.id > ?", afterID)
	}
	var ids []uint
	err := query.Order("contents.id DESC").Limit(limit).Pluck("contents.id", &ids).Error
	return ids, err
}

// ListFollowingIDsBefore is the MySQL fallback for a page of the following
// stream when Redis or RabbitMQ is unavailable. It covers push and pull authors
// alike and uses an ID cursor instead of OFFSET.
func (d *ContentDao) ListFollowingIDsBefore(userID, beforeID uint, limit int) ([]uint, error) {
	query := d.db.Model(&model.Content{}).
		Joins("JOIN user_follows ON user_follows.following_id = contents.author_id").
		Where("user_follows.follower_id = ?", userID).
		Where("contents.kind IN ?", feedContentKinds())
	if beforeID > 0 {
		query = query.Where("contents.id < ?", beforeID)
	}
	var ids []uint
	err := query.Order("contents.id DESC").Limit(limit).Pluck("contents.id", &ids).Error
	return ids, err
}

// ListInterestIDsBefore fetches candidate IDs by the reader's strongest author
// and topic signals. Authors the reader already follows (and the reader) are
// excluded, so this stream surfaces content the reader has not subscribed to.
func (d *ContentDao) ListInterestIDsBefore(beforeID uint, authorIDs []uint, topics []string, excludeAuthors []uint, limit int) ([]uint, error) {
	if len(authorIDs) == 0 && len(topics) == 0 {
		return []uint{}, nil
	}
	query := d.db.Model(&model.Content{}).Where("kind IN ?", feedContentKinds())
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	switch {
	case len(authorIDs) > 0 && len(topics) > 0:
		query = query.Where("(author_id IN ? OR topic IN ?)", authorIDs, topics)
	case len(authorIDs) > 0:
		query = query.Where("author_id IN ?", authorIDs)
	default:
		query = query.Where("topic IN ?", topics)
	}
	if len(excludeAuthors) > 0 {
		query = query.Where("author_id NOT IN ?", excludeAuthors)
	}
	var ids []uint
	err := query.Order("id DESC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// ListRecentIDs returns the newest eligible content IDs below the cursor that
// were created after since. It replaces ORDER BY RAND(): the explore pool is
// built from this index-friendly query and sampled in Go.
func (d *ContentDao) ListRecentIDs(beforeID uint, since time.Time, limit int) ([]uint, error) {
	query := d.db.Model(&model.Content{}).Where("kind IN ?", feedContentKinds())
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	if !since.IsZero() {
		query = query.Where("created_at >= ?", since)
	}
	var ids []uint
	err := query.Order("id DESC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// ListByIDs loads live content details in one query. SQL IN does not preserve
// the requested order, so FeedSrv restores ordering after this call.
func (d *ContentDao) ListByIDs(ids []uint) ([]model.Content, error) {
	if len(ids) == 0 {
		return []model.Content{}, nil
	}
	var items []model.Content
	err := d.db.Where("id IN ?", ids).Preload("Author").Find(&items).Error
	return items, err
}

// ListRecentIDsByAuthor returns an author's newest eligible content IDs; with
// afterID > 0 only IDs newer than a snapshot are returned (rebuild catch-up).
func (d *ContentDao) ListRecentIDsByAuthor(authorID, afterID uint, limit int) ([]uint, error) {
	query := d.db.Model(&model.Content{}).Where("author_id = ? AND kind IN ?", authorID, feedContentKinds())
	if afterID > 0 {
		query = query.Where("id > ?", afterID)
	}
	var ids []uint
	err := query.Order("id DESC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// ListIDsByAuthor returns eligible content IDs so a consumer can remove an
// unfollowed author's entries from one user's inbox.
func (d *ContentDao) ListIDsByAuthor(authorID uint) ([]uint, error) {
	var ids []uint
	err := d.db.Model(&model.Content{}).Where("author_id = ? AND kind IN ?", authorID, feedContentKinds()).
		Pluck("id", &ids).Error
	return ids, err
}

func feedContentKinds() []model.ContentKind {
	return []model.ContentKind{model.ContentKindQuestion, model.ContentKindAnswer, model.ContentKindArticle}
}

// ExistsPublicContent checks that a target exists, is not soft-deleted, and is
// one of the content kinds that can be publicly viewed or interacted with.
func (d *ContentDao) ExistsPublicContent(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&model.Content{}).Where("id = ? AND kind <> ?", id, model.ContentKindComment).Count(&count).Error
	return count > 0, err
}
