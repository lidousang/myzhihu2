package jwtmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myzhihu2/conf"
	"myzhihu2/util/jwtutil"

	"github.com/gin-gonic/gin"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

func TestRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := testJWTConfig()

	validToken := signTestToken(t, config, jwtlib.SigningMethodHS256, 42, config.Issuer, time.Now().Add(time.Hour), true)
	expiredToken := signTestToken(t, config, jwtlib.SigningMethodHS256, 42, config.Issuer, time.Now().Add(-time.Hour), true)
	wrongIssuerToken := signTestToken(t, config, jwtlib.SigningMethodHS256, 42, "another-service", time.Now().Add(time.Hour), true)
	wrongAlgorithmToken := signTestToken(t, config, jwtlib.SigningMethodHS384, 42, config.Issuer, time.Now().Add(time.Hour), true)
	missingExpirationToken := signTestToken(t, config, jwtlib.SigningMethodHS256, 42, config.Issuer, time.Time{}, false)

	wrongSecretConfig := config
	wrongSecretConfig.Secret = "abcdef0123456789abcdef0123456789"
	wrongSignatureToken, err := jwtutil.Sign(wrongSecretConfig, 42)
	if err != nil {
		t.Fatalf("create wrong-signature token: %v", err)
	}

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantUserID uint
	}{
		{name: "missing header", wantStatus: http.StatusUnauthorized},
		{name: "missing bearer scheme", header: validToken, wantStatus: http.StatusUnauthorized},
		{name: "malformed bearer header", header: "Bearer", wantStatus: http.StatusUnauthorized},
		{name: "wrong signature", header: "Bearer " + wrongSignatureToken, wantStatus: http.StatusUnauthorized},
		{name: "expired", header: "Bearer " + expiredToken, wantStatus: http.StatusUnauthorized},
		{name: "wrong issuer", header: "Bearer " + wrongIssuerToken, wantStatus: http.StatusUnauthorized},
		{name: "wrong algorithm", header: "Bearer " + wrongAlgorithmToken, wantStatus: http.StatusUnauthorized},
		{name: "missing expiration", header: "Bearer " + missingExpirationToken, wantStatus: http.StatusUnauthorized},
		{name: "valid", header: "Bearer " + validToken, wantStatus: http.StatusOK, wantUserID: 42},
		{name: "case insensitive scheme", header: "bearer " + validToken, wantStatus: http.StatusOK, wantUserID: 42},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/protected", Required(config), func(c *gin.Context) {
				userID, ok := UserID(c)
				if !ok {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "missing user id"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"user_id": userID})
			})

			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantStatus == http.StatusOK && response.Body.String() != `{"user_id":42}` {
				t.Fatalf("body = %s, want user_id %d", response.Body.String(), test.wantUserID)
			}
			if test.wantStatus == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("unauthorized response is missing WWW-Authenticate: Bearer")
			}
		})
	}
}

func testJWTConfig() conf.JWT {
	return conf.JWT{
		Algorithm:   "HS256",
		Issuer:      "myzhihu2-test",
		Secret:      "0123456789abcdef0123456789abcdef",
		ExpireHours: 1,
	}
}

func signTestToken(t *testing.T, config conf.JWT, method jwtlib.SigningMethod, userID uint, issuer string, expiresAt time.Time, includeExpiration bool) string {
	t.Helper()
	claims := jwtutil.Claims{
		UserID: userID,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:   issuer,
			IssuedAt: jwtlib.NewNumericDate(time.Now()),
		},
	}
	if includeExpiration {
		claims.ExpiresAt = jwtlib.NewNumericDate(expiresAt)
	}

	token := jwtlib.NewWithClaims(method, claims)
	rawToken, err := token.SignedString([]byte(config.Secret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return rawToken
}
