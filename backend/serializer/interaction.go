package serializer

// RecordBehaviorRequest is used to report reading behavior. Like and
// collection events are recorded by their corresponding interaction APIs.
type RecordBehaviorRequest struct {
	ContentID uint   `json:"content_id" binding:"required,gt=0"`
	Kind      string `json:"kind" binding:"required,oneof=view read_time"`
	Duration  int    `json:"duration" binding:"gte=0,lte=86400"`
}

// InteractionState reports the current user's state and aggregate counts for a
// content item. Counts are computed from reactions because the model has no
// denormalized counter columns.
type InteractionState struct {
	ContentID       uint  `json:"content_id"`
	LikeCount       int64 `json:"like_count"`
	CollectionCount int64 `json:"collection_count"`
	Liked           bool  `json:"liked"`
	Collected       bool  `json:"collected"`
}
