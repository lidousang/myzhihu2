package service

import "errors"

// Shared domain errors let API handlers map business failures to HTTP status
// codes without interpreting database-driver error strings.
var (
	ErrContentNotFound = errors.New("content not found")
	ErrForbidden       = errors.New("operation is not allowed for this user")
	ErrInvalidKind     = errors.New("invalid content kind")
	ErrInvalidParent   = errors.New("invalid parent content")
	ErrSelfFollow      = errors.New("a user cannot follow themselves")
	ErrInvalidCursor   = errors.New("invalid feed cursor")
	// ErrRegistrationClosed is returned by the public demo, where new accounts
	// are refused at the API so the restriction cannot be bypassed via curl.
	ErrRegistrationClosed = errors.New("演示环境已关闭注册，请返回登录页使用演示账号体验")
	ErrDemoDisabled       = errors.New("demo login is disabled")
)
