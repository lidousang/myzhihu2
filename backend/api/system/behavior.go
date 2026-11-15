package system

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"myzhihu2/serializer"
	"myzhihu2/service"
)

// RecordBehavior defines the endpoint for reporting a view or reading-duration
// event that can be used as an interest-feed signal.
func RecordBehavior(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req serializer.RecordBehaviorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid behavior request"})
		return
	}
	if err := service.GetInteractionSrv().RecordBehavior(c.Request.Context(), userID, req); err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"accepted": true}})
}
