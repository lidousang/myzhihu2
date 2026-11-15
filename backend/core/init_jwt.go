package core

import (
	"errors"
	"strings"

	"myzhihu2/util/global"
)

// InitJWT validates jwt configuration before the HTTP server starts.
// The jwt runtime does not use a global token manager; middleware receives
// the validated configuration explicitly from the application configuration.
func InitJWT() error {
	if global.Conf == nil {
		return errors.New("jwt: application configuration is not initialized")
	}
	config := global.Conf.JWT
	if config.Algorithm != "HS256" {
		return errors.New("jwt: only HS256 is supported")
	}
	if strings.TrimSpace(config.Issuer) == "" {
		return errors.New("jwt: issuer is required")
	}
	if len(config.Secret) < 32 {
		return errors.New("jwt: secret must contain at least 32 characters")
	}
	if config.ExpireHours <= 0 {
		return errors.New("jwt: expire_hours must be greater than zero")
	}
	return nil
}
