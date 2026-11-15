// Package system contains HTTP handlers for ordinary backend features.
package system

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	jwtmiddleware "myzhihu2/midware/jwt"
	"myzhihu2/serializer"
	"myzhihu2/service"
)

// UserRegister defines the registration endpoint. It will bind credentials,
// create a user, and return the public user data and access token.
func UserRegister(c *gin.Context) {
	var req serializer.UserRegisterRequest
	// ShouldBindJSON validates both the JSON shape and the binding tags on the
	// request DTO before the service is called.
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid registration request"})
		return
	}

	// The API layer handles HTTP concerns. UserSrv performs business rules,
	// password hashing, persistence through DAO, and token creation.
	result, err := service.GetUserSrv().Register(c.Request.Context(), req)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

// UserLogin defines the login endpoint. It will verify credentials and return
// an access token for subsequent authenticated requests.
func UserLogin(c *gin.Context) {
	var req serializer.UserLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login request"})
		return
	}

	result, err := service.GetUserSrv().Login(c.Request.Context(), req)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetMe defines the endpoint that returns the authenticated user's profile.
func GetMe(c *gin.Context) {
	// JWT middleware validates the token and stores its subject under the Gin
	// context key "user_id". Never accept this ID from query or JSON.
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	result, err := service.GetUserSrv().GetByID(c.Request.Context(), userID)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// UpdateMe defines the endpoint that updates the authenticated user's profile.
func UpdateMe(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var req serializer.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile request"})
		return
	}
	result, err := service.GetUserSrv().UpdateProfile(c.Request.Context(), userID, req)
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetUser defines the public endpoint that returns a user's profile by ID.
func GetUser(c *gin.Context) {
	// Public profile lookup uses a URL path parameter, not an identity claim.
	userID, err := strconv.ParseUint(c.Param("userID"), 10, 64)
	if err != nil || userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	result, err := service.GetUserSrv().GetByID(c.Request.Context(), uint(userID))
	if err != nil {
		writeSystemError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// currentUserID reads the identity injected by the JWT middleware.
// Keeping this contract in one helper makes protected handlers consistent.
func currentUserID(c *gin.Context) (uint, bool) {
	return jwtmiddleware.UserID(c)
}

// writeSystemError translates domain errors into HTTP status codes. Internal
// database or signing errors are logged but their details are not sent to users.
func writeSystemError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidKind), errors.Is(err, service.ErrInvalidParent):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrContentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrSelfFollow):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrRegistrationClosed):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrDemoDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		log.Printf("system API operation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
