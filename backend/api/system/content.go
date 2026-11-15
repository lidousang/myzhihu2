package system

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"myzhihu2/serializer"
	"myzhihu2/service"
)

// CreateContent defines creation of a question or article. The request's kind
// selects the content type; the author ID must come from the authenticated user.
func CreateContent(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req serializer.CreateContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content request"})
		return
	}
	result, err := service.GetContentSrv().CreateContent(c.Request.Context(), userID, req)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

// CreateAnswer defines creation of an answer under the question in the URL.
func CreateAnswer(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	questionID, ok := routeID(c, "questionID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid question id"})
		return
	}
	var req serializer.CreateAnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid answer request"})
		return
	}
	result, err := service.GetContentSrv().CreateAnswer(c.Request.Context(), userID, questionID, req)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

// GetContent defines retrieval of a question, answer, article, or comment.
func GetContent(c *gin.Context) {
	contentID, ok := routeID(c, "contentID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content id"})
		return
	}
	result, err := service.GetContentSrv().GetContent(c.Request.Context(), contentID)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// UpdateContent defines editing of a content item by its author.
func UpdateContent(c *gin.Context) {
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
	var req serializer.UpdateContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content update"})
		return
	}
	result, err := service.GetContentSrv().UpdateContent(c.Request.Context(), userID, contentID, req)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// DeleteContent defines deletion of a content item by its author.
func DeleteContent(c *gin.Context) {
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
	if err := service.GetContentSrv().DeleteContent(c.Request.Context(), userID, contentID); err != nil {
		writeSystemError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListQuestionAnswers defines a paginated answer list for a question.
func ListQuestionAnswers(c *gin.Context) {
	questionID, ok := routeID(c, "questionID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid question id"})
		return
	}
	page, limit := pagination(c)
	result, err := service.GetContentSrv().ListQuestionAnswers(c.Request.Context(), questionID, page, limit)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ListUserContents defines a paginated list of a user's published contents.
func ListUserContents(c *gin.Context) {
	userID, ok := routeID(c, "userID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	page, limit := pagination(c)
	result, err := service.GetContentSrv().ListUserContents(c.Request.Context(), userID, c.Query("kind"), page, limit)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// ListComments defines a paginated list of comments attached to a content item.
func ListComments(c *gin.Context) {
	contentID, ok := routeID(c, "contentID")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content id"})
		return
	}
	page, limit := pagination(c)
	result, err := service.GetContentSrv().ListComments(c.Request.Context(), contentID, page, limit)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// CreateComment defines creation of a comment on the content item in the URL.
func CreateComment(c *gin.Context) {
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
	var req serializer.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment request"})
		return
	}
	result, err := service.GetContentSrv().CreateComment(c.Request.Context(), userID, contentID, req)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

// routeID parses a positive unsigned ID from a Gin path parameter.
func routeID(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	return uint(id), err == nil && id > 0
}

// pagination reads page/limit query parameters and supplies safe defaults.
func pagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}
