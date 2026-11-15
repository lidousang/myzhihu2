package seed

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"myzhihu2/repository/model"
	"myzhihu2/service"

	"gorm.io/gorm"
)

// Check is one acceptance check from myzhihu2_demo_data.md section 9.
type Check struct {
	Name   string
	OK     bool
	Warn   bool // informational: depends on random explore sampling
	Detail string
}

// Verify runs the acceptance checks against the loaded database. The first
// page check calls FeedSrv directly, exactly like GET /api/v1/feed.
func Verify(ctx context.Context, db *gorm.DB, ds *Dataset, p Profile, threshold int, protectedMax uint) []Check {
	db = db.WithContext(ctx)
	var checks []Check
	add := func(name string, ok bool, detail string, args ...any) {
		checks = append(checks, Check{Name: name, OK: ok, Detail: fmt.Sprintf(detail, args...)})
	}

	var users int64
	db.Model(&model.User{}).Count(&users)
	expectedUsers := 4 + 30 + p.Ordinary
	add("user count", users == int64(expectedUsers), "%d (expected %d)", users, expectedUsers)

	var bigVs []uint
	db.Model(&model.User{}).Where("follower_count >= ?", threshold).Order("id").Pluck("id", &bigVs)
	add("big V set", slices.Equal(bigVs, []uint{BigVTechID, BigVFoodID, BigVTravelID}),
		"follower_count >= %d: %v (expected [1 2 3])", threshold, bigVs)

	var techFollowers int64
	db.Model(&model.UserFollow{}).Where("following_id = ?", BigVTechID).Count(&techFollowers)
	r := p.BigVFollowers[0]
	add("demo big V followers", int(techFollowers) >= r.Min && int(techFollowers) <= r.Max,
		"%d (expected %d-%d)", techFollowers, r.Min, r.Max)

	var readerFollowing []uint
	db.Model(&model.UserFollow{}).Where("follower_id = ?", ReaderID).Order("following_id").Pluck("following_id", &readerFollowing)
	want := slices.Clone(readerFollows)
	slices.Sort(want)
	add("reader follows", slices.Equal(readerFollowing, want), "%v", readerFollowing)

	var readerPosts int64
	db.Model(&model.Content{}).Where("author_id = ?", ReaderID).Count(&readerPosts)
	add("reader has no posts", readerPosts == 0, "%d", readerPosts)

	type topicCount struct {
		Topic string
		N     int
	}
	var perTopic []topicCount
	db.Model(&model.Content{}).Select("topic, COUNT(*) AS n").
		Where("id >= ? AND id < ?", ShowcaseStartID, CommentStartID).Group("topic").Order("topic").Scan(&perTopic)
	parts := make([]string, 0, len(perTopic))
	for _, t := range perTopic {
		parts = append(parts, fmt.Sprintf("%s=%d", t.Topic, t.N))
	}
	add("showcase per topic", len(perTopic) == len(model.Topics), "%s", strings.Join(parts, " "))

	var maxSeeded uint
	db.Model(&model.Content{}).Select("COALESCE(MAX(id), 0)").Scan(&maxSeeded)
	add("seeded content is protected", maxSeeded <= protectedMax && protectedMax < DemoContentStartID,
		"max seeded id %d <= protected_max_content_id %d < first demo id %d", maxSeeded, protectedMax, DemoContentStartID)

	var usable int64
	db.Model(&model.User{}).Where("password_hash <> ?", UnusablePasswordHash).Count(&usable)
	add("no usable passwords", usable == 0, "%d accounts with a password hash", usable)

	page, err := service.GetFeedSrv().GetFeed(ctx, ReaderID, 0, 10)
	if err != nil {
		add("reader first page", false, "feed error: %v", err)
		return checks
	}
	sources, vias := map[string]int{}, map[string]int{}
	photoInterest := false
	for _, item := range page.Items {
		sources[item.Source]++
		if item.Via != "" {
			vias[item.Via]++
		}
		if item.Source == "interest" && item.Content.Topic == model.TopicPhoto {
			photoInterest = true
		}
	}
	add("first page has following + interest", sources["following"] > 0 && sources["interest"] > 0, "sources=%v via=%v", sources, vias)
	add("interest surfaces photo", photoInterest, "an unfollowed photo post is labelled interest")
	checks = append(checks, Check{Name: "first page has 3 sources", OK: len(sources) >= 3, Warn: true,
		Detail: fmt.Sprintf("%d sources (explore is randomly sampled)", len(sources))})
	return checks
}
