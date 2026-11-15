package model

import (
	"time"
)

// User is an account that can publish content and interact with other users'
// content.

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"size:64;not null;uniqueIndex"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	DisplayName  string `gorm:"size:64;not null;default:''"`
	Avatar       string `gorm:"size:500;not null;default:''"`
	Bio          string `gorm:"size:500;not null;default:''"`
	// FollowerCount is maintained in the follow/unfollow transaction and decides
	// whether the author's posts are pushed to followers or pulled at read time.
	FollowerCount int `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (User) TableName() string { return "users" }
