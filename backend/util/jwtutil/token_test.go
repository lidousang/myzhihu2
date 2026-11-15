package jwtutil

import (
	"testing"

	"myzhihu2/conf"
)

func TestSignAndParse(t *testing.T) {
	config := conf.JWT{
		Algorithm:   "HS256",
		Issuer:      "myzhihu2-test",
		Secret:      "0123456789abcdef0123456789abcdef",
		ExpireHours: 1,
	}

	rawToken, err := Sign(config, 42)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	claims, err := Parse(config, rawToken)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("claims.UserID = %d, want 42", claims.UserID)
	}
	if claims.Issuer != config.Issuer {
		t.Fatalf("claims.Issuer = %q, want %q", claims.Issuer, config.Issuer)
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		t.Fatal("registered time claims were not populated")
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	config := conf.JWT{
		Algorithm:   "HS256",
		Issuer:      "myzhihu2-test",
		Secret:      "0123456789abcdef0123456789abcdef",
		ExpireHours: 1,
	}

	rawToken, err := Sign(config, 42)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	wrongConfig := config
	wrongConfig.Secret = "abcdef0123456789abcdef0123456789"
	if _, err := Parse(wrongConfig, rawToken); err == nil {
		t.Fatal("Parse() accepted a token signed with another secret")
	}
}
