package serializer

// CreateContentRequest creates a question or article in the unified contents
// table. Answers and comments use dedicated nested routes because each needs a
// parent content ID from the URL.
type CreateContentRequest struct {
	Kind  string `json:"kind" binding:"required,oneof=question article"`
	Topic string `json:"topic" binding:"required,oneof=tech food travel photo fitness"`
	Title string `json:"title" binding:"required,max=255"`
	// max counts characters; 20000 CJK characters stay below MySQL TEXT's 64 KB.
	Body string `json:"body" binding:"required,max=20000"`
}

// CreateAnswerRequest contains the text of an answer. The question ID is taken
// from the route rather than trusted from the JSON body.
type CreateAnswerRequest struct {
	Body string `json:"body" binding:"required,max=20000"`
}

// CreateCommentRequest contains the text of a comment. Its parent content ID is
// taken from the route.
type CreateCommentRequest struct {
	Body string `json:"body" binding:"required,max=2000"`
}

// UpdateContentRequest supports partial editing. Nil means "leave unchanged";
// an explicitly supplied empty string clears the field where allowed.
type UpdateContentRequest struct {
	Title *string `json:"title" binding:"omitempty,max=255"`
	Body  *string `json:"body" binding:"omitempty,max=20000"`
}

// ContentAuthor is the small public author representation embedded in content
// responses.
type ContentAuthor struct {
	ID          uint   `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
	// IsBigV marks authors whose posts are pulled at read time.
	IsBigV bool `json:"is_bigv"`
}

// ContentResponse is the API representation shared by questions, answers,
// articles, and comments.
type ContentResponse struct {
	ID        uint          `json:"id"`
	Author    ContentAuthor `json:"author"`
	Kind      string        `json:"kind"`
	Topic     string        `json:"topic,omitempty"`
	ParentID  *uint         `json:"parent_id,omitempty"`
	Title     string        `json:"title,omitempty"`
	Body      string        `json:"body"`
	CreatedAt int64         `json:"created_at"`
	UpdatedAt int64         `json:"updated_at"`
}

// ContentPage wraps a paginated content list and its total row count.
type ContentPage struct {
	Items []ContentResponse `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
}

// FanoutStats records how a content.created event was delivered. The compose
// page polls it so the demo can show push versus pull at a small scale.
type FanoutStats struct {
	ContentID     uint   `json:"content_id"`
	Mode          string `json:"mode"`           // push or pull
	Followers     int    `json:"followers"`      // total followers of the author
	ActiveTargets int    `json:"active_targets"` // followers that received an inbox write
	RedisWrites   int    `json:"redis_writes"`   // ZADD commands issued for this event
	DurationMs    int64  `json:"duration_ms"`
}
