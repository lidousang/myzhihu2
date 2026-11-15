package model

import (
	"time"

	"gorm.io/gorm"
)

// ContentKind identifies the type of a row in the contents table.
type ContentKind string

const (
	ContentKindQuestion ContentKind = "question"
	ContentKindAnswer   ContentKind = "answer"
	ContentKindArticle  ContentKind = "article"
	ContentKindComment  ContentKind = "comment"
)

// Topics are the subject areas used by the interest stream. Answers and
// comments inherit the topic of their parent content.
const (
	TopicTech    = "tech"
	TopicFood    = "food"
	TopicTravel  = "travel"
	TopicPhoto   = "photo"
	TopicFitness = "fitness"
)

// Topics lists every valid topic in display order.
var Topics = []string{TopicTech, TopicFood, TopicTravel, TopicPhoto, TopicFitness}

// ValidTopic reports whether topic is one of the supported topics.
func ValidTopic(topic string) bool {
	for _, t := range Topics {
		if t == topic {
			return true
		}
	}
	return false
}

// Content stores questions, answers, articles, and comments in one table.
// ParentID is nil for questions and articles, points to a question for answers,
// and points to the commented content for comments. The service layer must
// validate these kind/parent combinations.
type Content struct {
	ID        uint        `gorm:"primaryKey;index:idx_contents_topic_id,priority:2"`
	AuthorID  uint        `gorm:"not null;index:idx_contents_author_kind_created,priority:1"`
	Kind      ContentKind `gorm:"size:16;not null;index:idx_contents_kind_created,priority:1;index:idx_contents_author_kind_created,priority:2;index:idx_contents_parent_kind_created,priority:2"`
	ParentID  *uint       `gorm:"index:idx_contents_parent_kind_created,priority:1"`
	Topic     string      `gorm:"size:16;not null;default:'';index:idx_contents_topic_id,priority:1"`
	Title     string      `gorm:"size:255;not null;default:''"`
	Body      string      `gorm:"type:text;not null"`
	CreatedAt time.Time   `gorm:"index:idx_contents_kind_created,priority:2;index:idx_contents_author_kind_created,priority:3;index:idx_contents_parent_kind_created,priority:3"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	Author User     `gorm:"foreignKey:AuthorID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Parent *Content `gorm:"foreignKey:ParentID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (Content) TableName() string { return "contents" }
