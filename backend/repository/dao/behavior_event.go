package dao

import (
	"context"
	"errors"

	"myzhihu2/repository/model"
	"myzhihu2/util/global"

	"gorm.io/gorm"
)

// BehaviorEventDao stores historical actions used to build interest signals.
type BehaviorEventDao struct{ db *gorm.DB }

// NewBehaviorEventDao binds the shared DB pool to the current request context.
func NewBehaviorEventDao(ctx context.Context) (*BehaviorEventDao, error) {
	if global.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &BehaviorEventDao{db: global.DB.WithContext(ctx)}, nil
}

// Create appends one historical behavior row; it does not change a Reaction's
// current state.
func (d *BehaviorEventDao) Create(event *model.BehaviorEvent) error {
	return d.db.Create(event).Error
}

// ListRecentByUser reads a bounded set of recent actions for simple interest
// candidate generation.
func (d *BehaviorEventDao) ListRecentByUser(userID uint, limit int) ([]model.BehaviorEvent, error) {
	var events []model.BehaviorEvent
	err := d.db.Where("user_id = ?", userID).Order("created_at DESC, id DESC").Limit(limit).Find(&events).Error
	return events, err
}

// ListRecentWithContent preloads the referenced content so FeedSrv can derive
// author and kind interests when Redis interest signals are missing.
func (d *BehaviorEventDao) ListRecentWithContent(userID uint, limit int) ([]model.BehaviorEvent, error) {
	var events []model.BehaviorEvent
	err := d.db.Preload("Content").Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").Limit(limit).Find(&events).Error
	return events, err
}
