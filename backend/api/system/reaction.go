package system

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"myzhihu2/service"
)

// LikeContent defines the operation that records the current user's like.
func LikeContent(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	contentID, ok := routeID(c, "contentID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content id"})
		return
	}
	state, err := service.GetInteractionSrv().Like(c.Request.Context(), userID, contentID)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": state})
}

// UnlikeContent defines removal of the current user's like.
func UnlikeContent(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	contentID, ok := routeID(c, "contentID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content id"})
		return
	}
	state, err := service.GetInteractionSrv().Unlike(c.Request.Context(), userID, contentID)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": state})
}

// CollectContent defines the operation that records the current user's
// collection of a content item.
func CollectContent(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	contentID, ok := routeID(c, "contentID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content id"})
		return
	}
	state, err := service.GetInteractionSrv().Collect(c.Request.Context(), userID, contentID)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": state})
}

// UncollectContent defines removal of the current user's collection.
func UncollectContent(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	contentID, ok := routeID(c, "contentID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content id"})
		return
	}
	state, err := service.GetInteractionSrv().Uncollect(c.Request.Context(), userID, contentID)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": state})
}
