package model

import (
	"time"
)

// Reaction records a user's current like or collection state for a content.
// The composite primary key allows one like and one collection per user/content
// pair while preventing duplicates of either kind.
type Reaction struct {
	UserID    uint   `gorm:"primaryKey;autoIncrement:false"`
	ContentID uint   `gorm:"primaryKey;autoIncrement:false;index:idx_reactions_content_kind,priority:1"`
	Kind      string `gorm:"primaryKey;size:16;index:idx_reactions_content_kind,priority:2"`
	CreatedAt time.Time

	User    User    `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Content Content `gorm:"foreignKey:ContentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (Reaction) TableName() string { return "reactions" }
