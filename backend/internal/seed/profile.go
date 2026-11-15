// Package seed builds the deterministic demo dataset described in
// interview/myzhihu2_demo_data.md. Generation is pure (no database access) so
// it can be unit-tested for determinism; Load writes the result to MySQL.
package seed

// Fixed user IDs. They are referenced by settings.yaml (demo.*_user_id).
const (
	BigVTechID   uint = 1
	BigVFoodID   uint = 2
	BigVTravelID uint = 3
	ReaderID     uint = 10
	MidStartID   uint = 100 // 30 mid-tier authors: 6 per topic, in topic order
	OrdinaryID   uint = 1000

	FillerStartID   uint = 1     // template content, oldest
	ShowcaseStartID uint = 10001 // reviewed content shown on the first screens
	CommentStartID  uint = 20001 // comments are never feed candidates
	// DemoContentStartID is the first ID given to content created during the
	// demo. Load sets AUTO_INCREMENT to it, and demo.protected_max_content_id
	// (29999) protects every seeded row below it from demo-account edits.
	DemoContentStartID uint = 30000

	// UnusablePasswordHash is not a valid bcrypt hash, so bcrypt comparison
	// always fails: seeded accounts have no password at all. The two demo
	// accounts are reachable only through POST /auth/demo-login.
	UnusablePasswordHash = "!"
)

// Range is an inclusive integer range.
type Range struct{ Min, Max int }

// Profile sets the scale of a generated dataset.
type Profile struct {
	Name            string
	Ordinary        int      // ordinary users starting at OrdinaryID
	BigVFollowers   [3]Range // target followers for users 1 (tech), 2 (food), 3 (travel)
	MidFollowers    Range    // target followers per mid-tier author
	OrdinaryFollow  Range    // ordinary users followed by each ordinary user
	PosterRatio     float64  // share of ordinary users who wrote filler posts
	Filler          int      // template posts
	LikesPerUser    Range
	CollectsPerUser Range
	HotLikes        int // minimum likes on the pinned big V article
	Warm            int // ordinary users whose inbox is pre-built ("online" followers)
}

// Profiles available through cmd/seed -profile.
var Profiles = map[string]Profile{
	"demo": {
		Name:            "demo",
		Ordinary:        970,
		BigVFollowers:   [3]Range{{280, 320}, {180, 220}, {140, 180}},
		MidFollowers:    Range{20, 80},
		OrdinaryFollow:  Range{0, 3},
		PosterRatio:     0.15,
		Filler:          2600,
		LikesPerUser:    Range{5, 25},
		CollectsPerUser: Range{0, 5},
		HotLikes:        200,
		Warm:            60,
	},
	// stress is used on a temporary machine for load tests (bigv_threshold 5000).
	"stress": {
		Name:            "stress",
		Ordinary:        59970,
		BigVFollowers:   [3]Range{{50000, 50000}, {5000, 6000}, {5000, 6000}},
		MidFollowers:    Range{200, 800},
		OrdinaryFollow:  Range{0, 5},
		PosterRatio:     0.2,
		Filler:          200000,
		LikesPerUser:    Range{5, 15},
		CollectsPerUser: Range{0, 3},
		HotLikes:        20000,
		Warm:            1000,
	},
}
