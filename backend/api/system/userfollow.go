package system

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"myzhihu2/service"
)

// FollowUser defines the operation that makes the authenticated user follow
// the user identified by the URL.
func FollowUser(c *gin.Context) {
	followerID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	followingID, ok := routeID(c, "userID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := service.GetInteractionSrv().FollowUser(c.Request.Context(), followerID, followingID); err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"following": true}})
}

// UnfollowUser defines removal of the authenticated user's follow relationship.
func UnfollowUser(c *gin.Context) {
	followerID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	followingID, ok := routeID(c, "userID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := service.GetInteractionSrv().UnfollowUser(c.Request.Context(), followerID, followingID); err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"following": false}})
}
