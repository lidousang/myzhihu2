package route

import (
	"github.com/gin-gonic/gin"

	feedapi "myzhihu2/api/feed"
	systemapi "myzhihu2/api/system"
	"myzhihu2/conf"
	"myzhihu2/midware/demo"
	jwtmiddleware "myzhihu2/midware/jwt"
)

// NewRouter creates the Gin engine and registers public and authenticated APIs.
func NewRouter(jwtConfig conf.JWT, demoConfig conf.Demo) *gin.Engine {
	router := gin.Default()
	v1 := router.Group("/api/v1")

	// Public routes: registration, login, and publicly readable resources.
	{
		v1.POST("/auth/register", systemapi.UserRegister)
		v1.POST("/auth/login", systemapi.UserLogin)
		v1.POST("/auth/demo-login", systemapi.DemoLogin)
		v1.GET("/meta", systemapi.Meta)

		v1.GET("/users/:userID", systemapi.GetUser)
		v1.GET("/users/:userID/contents", systemapi.ListUserContents)
		v1.GET("/contents/:contentID", systemapi.GetContent)
		v1.GET("/questions/:questionID/answers", systemapi.ListQuestionAnswers)
		v1.GET("/contents/:contentID/comments", systemapi.ListComments)
	}

	// Authenticated routes validate a Bearer token before their handlers run.
	auth := v1.Group("")
	auth.Use(jwtmiddleware.Required(jwtConfig), demo.Guard(demoConfig))
	{
		auth.GET("/me", systemapi.GetMe)
		auth.PUT("/me", systemapi.UpdateMe)

		auth.POST("/contents", systemapi.CreateContent)
		auth.POST("/questions/:questionID/answers", systemapi.CreateAnswer)
		auth.PUT("/contents/:contentID", systemapi.UpdateContent)
		auth.DELETE("/contents/:contentID", systemapi.DeleteContent)
		auth.GET("/contents/:contentID/fanout", systemapi.GetFanoutStats)
		auth.POST("/contents/:contentID/comments", systemapi.CreateComment)

		auth.POST("/contents/:contentID/like", systemapi.LikeContent)
		auth.DELETE("/contents/:contentID/like", systemapi.UnlikeContent)
		auth.POST("/contents/:contentID/collection", systemapi.CollectContent)
		auth.DELETE("/contents/:contentID/collection", systemapi.UncollectContent)
		auth.POST("/users/:userID/follow", systemapi.FollowUser)
		auth.DELETE("/users/:userID/follow", systemapi.UnfollowUser)
		auth.POST("/behaviors", systemapi.RecordBehavior)

		// The home feed is personalized, so it belongs behind JWT authentication.
		auth.GET("/feed", feedapi.GetFeed)
	}

	return router
}
