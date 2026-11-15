// Package service contains business rules grouped by domain. UserSrv is a
// stateless singleton: request-specific values are passed into each method.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"myzhihu2/repository/dao"
	"myzhihu2/repository/model"
	"myzhihu2/serializer"
	"myzhihu2/util/global"
	"myzhihu2/util/jwtutil"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var (
	// UserSrvIns is the shared user service instance.
	UserSrvIns  *UserSrv
	userSrvOnce sync.Once

	// User-service errors are translated to HTTP status codes by the API layer.
	ErrInvalidInput       = errors.New("invalid user input")
	ErrUsernameTaken      = errors.New("username is already registered")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserNotFound       = errors.New("user not found")
)

// UserSrv groups account registration, login, and profile operations.
type UserSrv struct{}

// GetUserSrv returns the singleton UserSrv, initializing it exactly once.
func GetUserSrv() *UserSrv {
	userSrvOnce.Do(func() {
		UserSrvIns = &UserSrv{}
	})
	return UserSrvIns
}

// Register validates registration data, hashes the password, stores the user,
// and issues a JWT. Database access itself is delegated to UserDao.
func (s *UserSrv) Register(ctx context.Context, req serializer.UserRegisterRequest) (*serializer.AuthResponse, error) {
	if global.Conf != nil && global.Conf.Demo.Enabled && !global.Conf.Demo.RegistrationEnabled {
		return nil, ErrRegistrationClosed
	}
	username := strings.TrimSpace(req.Username)
	displayName := strings.TrimSpace(req.DisplayName)
	if len(username) < 3 || len(username) > 64 || len(req.Password) < 8 || len(req.Password) > 72 {
		return nil, ErrInvalidInput
	}
	if displayName == "" {
		displayName = username
	}

	userDao, err := dao.NewUserDao(ctx)
	if err != nil {
		return nil, err
	}

	// Check the unique username first to return a clear business error. The
	// database unique index remains the final protection against concurrent
	// registration requests racing between this query and the insert.
	_, err = userDao.FindByUsername(username)
	if err == nil {
		return nil, ErrUsernameTaken
	}
	if !dao.IsNotFound(err) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
	}
	if err := userDao.Create(user); err != nil {
		// Two concurrent registrations can both pass the lookup above; the
		// unique index rejects the second one with MySQL error 1062.
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}

	return s.authResponse(user)
}

// DemoLogin issues a token for one of the two configured demo accounts. The
// accounts have no usable password, so this endpoint is the only way in.
func (s *UserSrv) DemoLogin(ctx context.Context, role string) (*serializer.AuthResponse, error) {
	if global.Conf == nil || !global.Conf.Demo.Enabled {
		return nil, ErrDemoDisabled
	}
	var userID uint
	switch role {
	case "reader":
		userID = global.Conf.Demo.ReaderUserID
	case "bigv":
		userID = global.Conf.Demo.BigVUserID
	default:
		return nil, ErrInvalidInput
	}
	userDao, err := dao.NewUserDao(ctx)
	if err != nil {
		return nil, err
	}
	user, err := userDao.FindByID(userID)
	if dao.IsNotFound(err) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.authResponse(user)
}

// Login finds a user by username, verifies the password hash, and issues a JWT.
// The same error is returned for an unknown username and a wrong password so
// the endpoint does not reveal which usernames are registered.
func (s *UserSrv) Login(ctx context.Context, req serializer.UserLoginRequest) (*serializer.AuthResponse, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return nil, ErrInvalidInput
	}

	userDao, err := dao.NewUserDao(ctx)
	if err != nil {
		return nil, err
	}
	user, err := userDao.FindByUsername(username)
	if dao.IsNotFound(err) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.authResponse(user)
}

// GetByID returns a public user profile. The model is converted to a response
// DTO so PasswordHash and database-specific fields are never exposed.
func (s *UserSrv) GetByID(ctx context.Context, id uint) (*serializer.UserResponse, error) {
	userDao, err := dao.NewUserDao(ctx)
	if err != nil {
		return nil, err
	}
	user, err := userDao.FindByID(id)
	if dao.IsNotFound(err) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	response := buildUserResponse(user)
	return &response, nil
}

// UpdateProfile applies the editable profile fields for the authenticated user.
// The caller supplies the user ID from verified JWT context, never from JSON.
func (s *UserSrv) UpdateProfile(ctx context.Context, userID uint, req serializer.UpdateUserRequest) (*serializer.UserResponse, error) {
	if userID == 0 {
		return nil, ErrInvalidInput
	}
	// Build a whitelist of editable columns. A nil pointer means the client did
	// not send that field, while a pointer to an empty string clears the field.
	fields := make(map[string]any)
	if req.DisplayName != nil {
		if len(*req.DisplayName) > 64 {
			return nil, ErrInvalidInput
		}
		fields["display_name"] = strings.TrimSpace(*req.DisplayName)
	}
	if req.Avatar != nil {
		if len(*req.Avatar) > 500 {
			return nil, ErrInvalidInput
		}
		fields["avatar"] = strings.TrimSpace(*req.Avatar)
	}
	if req.Bio != nil {
		if len(*req.Bio) > 500 {
			return nil, ErrInvalidInput
		}
		fields["bio"] = strings.TrimSpace(*req.Bio)
	}
	userDao, err := dao.NewUserDao(ctx)
	if err != nil {
		return nil, err
	}
	if err := userDao.UpdateProfile(userID, fields); dao.IsNotFound(err) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	user, err := userDao.FindByID(userID)
	if err != nil {
		return nil, err
	}
	response := buildUserResponse(user)
	return &response, nil
}

// authResponse creates the common login/registration response and signs a JWT.
func (s *UserSrv) authResponse(user *model.User) (*serializer.AuthResponse, error) {
	if global.Conf == nil {
		return nil, errors.New("application configuration is not initialized")
	}
	signedToken, err := jwtutil.Sign(global.Conf.JWT, user.ID)
	if err != nil {
		return nil, err
	}
	return &serializer.AuthResponse{
		User:        buildUserResponse(user),
		AccessToken: signedToken,
		TokenType:   "Bearer",
	}, nil
}

// buildUserResponse centralizes safe conversion from persistence model to API DTO.
func buildUserResponse(user *model.User) serializer.UserResponse {
	return serializer.UserResponse{
		ID:            user.ID,
		Username:      user.Username,
		DisplayName:   user.DisplayName,
		Avatar:        user.Avatar,
		Bio:           user.Bio,
		FollowerCount: user.FollowerCount,
		IsBigV:        user.FollowerCount >= feedConf().BigVThreshold,
		CreatedAt:     user.CreatedAt.Unix(),
	}
}
