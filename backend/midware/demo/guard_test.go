package demo

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myzhihu2/conf"
	jwtmiddleware "myzhihu2/midware/jwt"

	"github.com/gin-gonic/gin"
)

func newRouter(cfg conf.Demo, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(func(c *gin.Context) { c.Set(jwtmiddleware.ContextUserIDKey, userID) }, Guard(cfg))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	auth.POST("/contents", ok)
	auth.PUT("/contents/:contentID", ok)
	auth.DELETE("/contents/:contentID", ok)
	auth.POST("/contents/:contentID/like", ok)
	auth.POST("/users/:userID/follow", ok)
	auth.PUT("/me", ok)
	return r
}

func do(r *gin.Engine, method, path string) int {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w.Code
}

func TestGuard(t *testing.T) {
	cfg := conf.Demo{Enabled: true, ReaderUserID: 10, BigVUserID: 1, ProtectedMaxContentID: 29999, PostsPerHour: 2}
	reader, bigV, other := newRouter(cfg, 10), newRouter(cfg, 1), newRouter(cfg, 500)
	cases := []struct {
		name   string
		r      *gin.Engine
		method string
		path   string
		want   int
	}{
		{"reader likes", reader, http.MethodPost, "/api/v1/contents/5/like", 200},
		{"reader follows", reader, http.MethodPost, "/api/v1/users/3/follow", 200},
		{"reader posts", reader, http.MethodPost, "/api/v1/contents", 403},
		{"reader edits profile", reader, http.MethodPut, "/api/v1/me", 403},
		{"big V edits profile", bigV, http.MethodPut, "/api/v1/me", 403},
		{"big V deletes seeded", bigV, http.MethodDelete, "/api/v1/contents/10005", 403},
		{"big V edits own new post", bigV, http.MethodPut, "/api/v1/contents/30001", 200},
		{"other user unrestricted", other, http.MethodDelete, "/api/v1/contents/10005", 200},
	}
	for _, c := range cases {
		if got := do(c.r, c.method, c.path); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
	for i, want := range []int{200, 200, 429} {
		if got := do(bigV, http.MethodPost, "/api/v1/contents"); got != want {
			t.Errorf("big V post #%d: got %d, want %d", i+1, got, want)
		}
	}
}

func TestGuardDisabled(t *testing.T) {
	r := newRouter(conf.Demo{Enabled: false, ReaderUserID: 10}, 10)
	if got := do(r, http.MethodPost, "/api/v1/contents"); got != 200 {
		t.Fatalf("disabled guard blocked request: %d", got)
	}
}

func TestWindowLimiterSlides(t *testing.T) {
	l := newWindowLimiter(1, time.Hour)
	t0 := time.Now()
	if !l.Allow(1, t0) || l.Allow(1, t0.Add(time.Minute)) {
		t.Fatal("limit of 1 per hour not enforced")
	}
	if !l.Allow(1, t0.Add(61*time.Minute)) {
		t.Fatal("window did not slide")
	}
}
