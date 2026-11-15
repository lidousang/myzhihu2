package model

import (
	"time"
)

// BehaviorKind identifies a historical user action that can contribute to
// the interest feed.
type BehaviorKind string

const (
	BehaviorKindView     BehaviorKind = "view"
	BehaviorKindReadTime BehaviorKind = "read_time"
	BehaviorKindLike     BehaviorKind = "like"
	BehaviorKindCollect  BehaviorKind = "collect"
)

// BehaviorEvent records a user action. Reactions represent current like and
// collection state; BehaviorEvent records the action history used as an
// interest signal.

type BehaviorEvent struct {
	ID        uint         `gorm:"primaryKey"`
	UserID    uint         `gorm:"not null;index:idx_behavior_user_created,priority:1;index:idx_behavior_user_kind_created,priority:1"`
	ContentID uint         `gorm:"not null;index:idx_behavior_content"`
	Kind      BehaviorKind `gorm:"size:16;not null;index:idx_behavior_user_kind_created,priority:2"`
	Duration  int          `gorm:"not null;default:0"`
	CreatedAt time.Time    `gorm:"index:idx_behavior_user_created,priority:2;index:idx_behavior_user_kind_created,priority:3"`

	User    User    `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Content Content `gorm:"foreignKey:ContentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (BehaviorEvent) TableName() string { return "behavior_events" }
