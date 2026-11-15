// Package jwtmiddleware authenticates Bearer access tokens at the Gin boundary.
package jwtmiddleware

import (
	"net/http"
	"strings"

	"myzhihu2/conf"
	"myzhihu2/util/jwtutil"

	"github.com/gin-gonic/gin"
)

// ContextUserIDKey is the single context key used by middleware and handlers.
const ContextUserIDKey = "user_id"

// Required rejects requests without a valid Bearer access token. A successful
// validation makes the authenticated user ID available to downstream handlers.
func Required(config conf.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			unauthorized(c, "authentication required")
			return
		}

		claims, err := jwtutil.Parse(config, rawToken)
		if err != nil {
			unauthorized(c, "invalid or expired access token")
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Next()
	}
}

// UserID returns the authenticated identity installed by Required.
func UserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get(ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(uint)
	return userID, ok && userID > 0
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func unauthorized(c *gin.Context, message string) {
	c.Header("WWW-Authenticate", "Bearer")
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
}
