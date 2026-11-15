package conf

// Feed controls the push/pull hybrid and candidate pools used by FeedSrv.
type Feed struct {
	// BigVThreshold is the follower count at which an author switches from
	// push (fan-out on write) to pull (fan-out on read). The demo profile uses
	// a small value so a 1000-user dataset still contains big V accounts.
	BigVThreshold int `yaml:"bigv_threshold"`
	// InboxLimit caps each user's following inbox ZSET.
	InboxLimit int `yaml:"inbox_limit"`
	// OutboxLimit caps each author's outbox ZSET read by followers in pull mode.
	OutboxLimit int `yaml:"outbox_limit"`
	// FanoutBatch is the number of followers checked and written per pipeline.
	FanoutBatch int `yaml:"fanout_batch"`
	// ExploreWindowDays limits the explore pool to recent content.
	ExploreWindowDays int `yaml:"explore_window_days"`
	// ExplorePoolSize caps the explore candidate pool.
	ExplorePoolSize int `yaml:"explore_pool_size"`
	// ExploreRefreshSeconds is the explore pool rebuild interval.
	ExploreRefreshSeconds int `yaml:"explore_refresh_seconds"`
}

// BigVExitThreshold is the hysteresis lower bound: a big V is demoted only
// after falling below 80% of BigVThreshold, so authors near the boundary do
// not flip between push and pull on every follow/unfollow.
func (f Feed) BigVExitThreshold() int {
	return f.BigVThreshold * 4 / 5
}

// Demo configures the public demonstration deployment.
type Demo struct {
	// Enabled turns on demo login and demo-account restrictions.
	Enabled bool `yaml:"enabled"`
	// RegistrationEnabled keeps the registration page usable; in the public
	// demo it is false so the API refuses new accounts.
	RegistrationEnabled bool `yaml:"registration_enabled"`
	// ReaderUserID and BigVUserID are the only accounts issued by demo login.
	ReaderUserID uint `yaml:"reader_user_id"`
	BigVUserID   uint `yaml:"bigv_user_id"`
	// ProtectedMaxContentID protects seeded content: demo accounts cannot edit
	// or delete content whose ID is at or below this value.
	ProtectedMaxContentID uint `yaml:"protected_max_content_id"`
	// PostsPerHour limits content and comment creation by demo accounts.
	PostsPerHour int `yaml:"posts_per_hour"`
}

// ApplyDefaults fills unset values so older settings files keep working.
func (c *Config) ApplyDefaults() {
	f := &c.Feed
	if f.BigVThreshold <= 0 {
		f.BigVThreshold = 100
	}
	if f.InboxLimit <= 0 {
		f.InboxLimit = 1000
	}
	if f.OutboxLimit <= 0 {
		f.OutboxLimit = 1000
	}
	if f.FanoutBatch <= 0 {
		f.FanoutBatch = 500
	}
	if f.ExploreWindowDays <= 0 {
		f.ExploreWindowDays = 7
	}
	if f.ExplorePoolSize <= 0 {
		f.ExplorePoolSize = 500
	}
	if f.ExploreRefreshSeconds <= 0 {
		f.ExploreRefreshSeconds = 60
	}
	if c.Demo.PostsPerHour <= 0 {
		c.Demo.PostsPerHour = 10
	}
}
