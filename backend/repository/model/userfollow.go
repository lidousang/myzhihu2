package model

import (
	"time"
)

// UserFollow records that FollowerID follows FollowingID. The pair is the
// composite primary key, so the same follow relationship cannot be inserted
// twice.
type UserFollow struct {
	FollowerID  uint `gorm:"primaryKey;autoIncrement:false"`
	FollowingID uint `gorm:"primaryKey;autoIncrement:false;index:idx_user_follows_following,priority:1"`
	CreatedAt   time.Time

	Follower  User `gorm:"foreignKey:FollowerID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Following User `gorm:"foreignKey:FollowingID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (UserFollow) TableName() string { return "user_follows" }
