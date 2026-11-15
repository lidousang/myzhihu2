package serializer

// FeedItem wraps a content card with its origin stream and the current user's
// reaction state. Source is one of following, interest, or explore.
type FeedItem struct {
	Content ContentResponse `json:"content"`
	Source  string          `json:"source"`
	// Via is push or pull for following items: pushed into the reader's inbox
	// or pulled from a big V outbox while reading.
	Via string `json:"via,omitempty"`
	// FollowingAuthor reports whether the reader follows the content author.
	FollowingAuthor bool  `json:"following_author"`
	LikeCount       int64 `json:"like_count"`
	CollectionCount int64 `json:"collection_count"`
	Liked           bool  `json:"liked"`
	Collected       bool  `json:"collected"`
}

// FeedPage is one cursor-paginated page. NextCursor is the last returned
// content ID; clients send it as cursor to fetch IDs strictly below it.
type FeedPage struct {
	Items      []FeedItem `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
	HasMore    bool       `json:"has_more"`
}
