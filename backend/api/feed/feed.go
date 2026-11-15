// Package feed contains HTTP handlers for feed retrieval.
package feed

import (
	"errors"
	"net/http"
	"strconv"

	jwtmiddleware "myzhihu2/midware/jwt"
	"myzhihu2/service"

	"github.com/gin-gonic/gin"
)

// GetFeed defines the personalized home-feed endpoint. FeedSrv will collect
// following, interest, and exploration candidates and combine them into one page.
func GetFeed(c *gin.Context) {
	// Authentication is enforced by the route group. Reading the identity here
	// completes the JWT boundary for the future personalized FeedSrv call.
	userID, ok := jwtmiddleware.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	// The cursor is the last content ID returned by the preceding page. The
	// service applies an exclusive "id < cursor" condition in Redis and MySQL.
	var cursor uint64
	if rawCursor := c.Query("cursor"); rawCursor != "" {
		parsed, err := strconv.ParseUint(rawCursor, 10, 64)
		if err != nil || parsed == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
			return
		}
		cursor = parsed
	}

	limit := 20
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = parsed
	}
	if limit > 50 {
		limit = 50
	}

	result, err := service.GetFeedSrv().GetFeed(c.Request.Context(), userID, uint(cursor), limit)
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) || errors.Is(err, service.ErrInvalidCursor) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "feed request failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
