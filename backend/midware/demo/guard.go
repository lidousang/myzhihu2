// Package demo restricts what the two shared public demo accounts can do.
// The restrictions are enforced on the server; the frontend only hides the
// corresponding buttons, which an attacker could bypass with curl.
package demo

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"myzhihu2/conf"
	jwtmiddleware "myzhihu2/midware/jwt"

	"github.com/gin-gonic/gin"
)

const (
	pathContents = "/api/v1/contents"
	pathContent  = "/api/v1/contents/:contentID"
	pathAnswers  = "/api/v1/questions/:questionID/answers"
	pathComments = "/api/v1/contents/:contentID/comments"
	pathMe       = "/api/v1/me"
)

// Guard returns middleware for the authenticated route group. Rules:
//   - both demo accounts: profile edits are refused (shared account defacement);
//   - reader: browse, like, collect and follow only; no posting or editing;
//   - big V: may post, answer and comment up to PostsPerHour, and may edit or
//     delete only content it created after seeding (ID > ProtectedMaxContentID).
//
// Non-demo users are not affected.
func Guard(cfg conf.Demo) gin.HandlerFunc {
	limiter := newWindowLimiter(cfg.PostsPerHour, time.Hour)
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}
		userID, ok := jwtmiddleware.UserID(c)
		if !ok || (userID != cfg.ReaderUserID && userID != cfg.BigVUserID) {
			c.Next()
			return
		}
		method, path := c.Request.Method, c.FullPath()
		creates := method == http.MethodPost && (path == pathContents || path == pathAnswers || path == pathComments)
		modifies := (method == http.MethodPut || method == http.MethodDelete) && path == pathContent

		switch {
		case method == http.MethodPut && path == pathMe:
			deny(c, http.StatusForbidden, "演示账号不能修改个人资料")
			return
		case userID == cfg.ReaderUserID && (creates || modifies):
			deny(c, http.StatusForbidden, "演示普通用户只能浏览、点赞、收藏和关注，不能发布或修改内容")
			return
		case modifies:
			id, err := strconv.ParseUint(c.Param("contentID"), 10, 64)
			if err != nil || uint(id) <= cfg.ProtectedMaxContentID {
				deny(c, http.StatusForbidden, "预置的演示内容不能修改或删除，只能操作你在演示中新发布的内容")
				return
			}
		case creates:
			if !limiter.Allow(userID, time.Now()) {
				deny(c, http.StatusTooManyRequests, fmt.Sprintf("发布过于频繁：演示账号每小时最多发布 %d 条", cfg.PostsPerHour))
				return
			}
		}
		c.Next()
	}
}

func deny(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}

// windowLimiter is a per-user sliding-window counter. It is in-process, which
// is sufficient for the single-instance demo; multiple replicas would need a
// shared counter such as a Redis ZSET or INCR with expiry.
type windowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[uint][]time.Time
}

func newWindowLimiter(limit int, window time.Duration) *windowLimiter {
	return &windowLimiter{limit: limit, window: window, hits: make(map[uint][]time.Time)}
}

// Allow records a hit at now and reports whether it is within the limit.
func (l *windowLimiter) Allow(userID uint, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	kept := l.hits[userID][:0]
	for _, t := range l.hits[userID] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[userID] = kept
		return false
	}
	l.hits[userID] = append(kept, now)
	return true
}
