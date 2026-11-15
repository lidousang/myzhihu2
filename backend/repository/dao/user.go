// Package dao contains MySQL operations. It does not make HTTP responses or
// decide business policy; those responsibilities belong to api and service.
package dao

import (
	"context"
	"errors"

	"myzhihu2/repository/model"
	"myzhihu2/util/global"

	"gorm.io/gorm"
)

// UserDao is a request-scoped database access object. Its DB handle points to
// the shared connection pool, but carries the request's context for cancellation
// and deadlines.
type UserDao struct {
	db *gorm.DB
}

// NewUserDao builds a DAO around the database connection initialized by main.
// The constructor does not open a new database connection for each request.
func NewUserDao(ctx context.Context) (*UserDao, error) {
	if global.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &UserDao{db: global.DB.WithContext(ctx)}, nil
}

// Create inserts a user row. PasswordHash must already be hashed by the service.
func (d *UserDao) Create(user *model.User) error {
	return d.db.Create(user).Error
}

// FindByID returns one user by its primary key.
func (d *UserDao) FindByID(id uint) (*model.User, error) {
	var user model.User
	if err := d.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByUsername returns one user by the unique login name. A missing user
// returns gorm.ErrRecordNotFound, which the service interprets for registration
// and login behavior.
func (d *UserDao) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := d.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateProfile updates only the fields included in the map. The service
// constructs this map from the profile request, so callers cannot update IDs,
// usernames, or password hashes through this method.
func (d *UserDao) UpdateProfile(id uint, fields map[string]any) error {
	result := d.db.Model(&model.User{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// Updates returns zero for an unchanged row as well as a missing row.
		// Check existence to distinguish those cases for the service/API.
		var count int64
		if err := d.db.Model(&model.User{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

// FollowerCount returns the denormalized follower count of one user.
func (d *UserDao) FollowerCount(id uint) (int, error) {
	var count int
	err := d.db.Model(&model.User{}).Where("id = ?", id).Pluck("follower_count", &count).Error
	return count, err
}

// ListIDsWithFollowersAtLeast returns users whose follower count reaches the
// big V threshold; it seeds the Redis big V set after a cache loss.
func (d *UserDao) ListIDsWithFollowersAtLeast(threshold int) ([]uint, error) {
	var ids []uint
	err := d.db.Model(&model.User{}).Where("follower_count >= ?", threshold).Order("id").Pluck("id", &ids).Error
	return ids, err
}

// IsNotFound is a small helper for services that should not depend on a
// database driver's concrete error type.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
