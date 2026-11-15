package system

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"myzhihu2/serializer"
	"myzhihu2/service"
	"myzhihu2/util/global"
)

// DemoLogin issues a token for the reader or big V demo account.
func DemoLogin(c *gin.Context) {
	var req serializer.DemoLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be reader or bigv"})
		return
	}
	result, err := service.GetUserSrv().DemoLogin(c.Request.Context(), req.Role)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Meta exposes public deployment settings used by the login page and badges.
func Meta(c *gin.Context) {
	cfg := global.Conf
	meta := serializer.MetaResponse{
		DemoEnabled:         cfg.Demo.Enabled,
		RegistrationEnabled: !cfg.Demo.Enabled || cfg.Demo.RegistrationEnabled,
		BigVThreshold:       cfg.Feed.BigVThreshold,
	}
	if cfg.Demo.Enabled {
		meta.ReaderUserID, meta.BigVUserID = cfg.Demo.ReaderUserID, cfg.Demo.BigVUserID
	}
	c.JSON(http.StatusOK, gin.H{"data": meta})
}

// GetFanoutStats reports how a newly created post was delivered. It returns
// pending=true until the outbox relay and the consumer have processed it.
func GetFanoutStats(c *gin.Context) {
	contentID, ok := routeID(c, "contentID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content id"})
		return
	}
	stats, found, err := service.GetContentSrv().GetFanoutStats(c.Request.Context(), contentID)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	if !found {
		c.JSON(http.StatusOK, gin.H{"data": nil, "pending": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats, "pending": false})
}
