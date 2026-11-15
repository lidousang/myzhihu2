// Package serializer contains request and response structures used at the
// HTTP boundary. These types are separate from GORM models so database-only
// fields, especially password hashes, are not returned directly to clients.
package serializer

// UserRegisterRequest is the JSON body accepted by the registration endpoint.
type UserRegisterRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Password    string `json:"password" binding:"required,min=8,max=72"`
	DisplayName string `json:"display_name" binding:"omitempty,max=64"`
}

// UserLoginRequest is the JSON body accepted by the login endpoint.
type UserLoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	// Login does not enforce password strength; only registration does. A short
	// wrong password must return 401, not a 400 validation error.
	Password string `json:"password" binding:"required,max=72"`
}

// DemoLoginRequest selects one of the two public demo accounts.
type DemoLoginRequest struct {
	Role string `json:"role" binding:"required,oneof=reader bigv"`
}

// MetaResponse exposes public deployment settings needed by the frontend.
type MetaResponse struct {
	DemoEnabled         bool `json:"demo_enabled"`
	RegistrationEnabled bool `json:"registration_enabled"`
	BigVThreshold       int  `json:"bigv_threshold"`
	ReaderUserID        uint `json:"reader_user_id,omitempty"`
	BigVUserID          uint `json:"bigv_user_id,omitempty"`
}

// UpdateUserRequest contains only the profile fields users may edit.
type UpdateUserRequest struct {
	// Pointers distinguish an omitted field from a field explicitly set to "".
	DisplayName *string `json:"display_name" binding:"omitempty,max=64"`
	Avatar      *string `json:"avatar" binding:"omitempty,max=500"`
	Bio         *string `json:"bio" binding:"omitempty,max=500"`
}

// UserResponse is the public representation of a user. It deliberately omits
// PasswordHash and internal persistence details.
type UserResponse struct {
	ID            uint   `json:"id"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Avatar        string `json:"avatar"`
	Bio           string `json:"bio"`
	FollowerCount int    `json:"follower_count"`
	IsBigV        bool   `json:"is_bigv"`
	CreatedAt     int64  `json:"created_at"`
}

// AuthResponse is returned after registration or login.
type AuthResponse struct {
	User        UserResponse `json:"user"`
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
}
