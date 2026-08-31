package auth

import (
	"testing"
	"time"

	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestIssueTokenIncludesPasswordVersion 验证新签发的 JWT 携带账号密码版本。
func TestIssueTokenIncludesPasswordVersion(t *testing.T) {
	service := NewService(&config.Config{JWT: config.JWTConfig{
		Secret:    "test-secret",
		ExpiresIn: time.Hour,
		Issuer:    "test-issuer",
	}}, nil)

	tokenText, _, err := service.IssueToken(model.User{ID: 7, PasswordVersion: 4})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	token, err := jwt.ParseWithClaims(tokenText, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte("test-secret"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("parse token: valid=%v err=%v", token.Valid, err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		t.Fatalf("claims type = %T", token.Claims)
	}
	if claims.PasswordVersion != 4 {
		t.Fatalf("password_version = %d, want 4", claims.PasswordVersion)
	}

}

func TestIssueTokenRequiresInitialPasswordVersion(t *testing.T) {
	service := NewService(&config.Config{JWT: config.JWTConfig{
		Secret:    "test-secret",
		ExpiresIn: time.Hour,
		Issuer:    "test-issuer",
	}}, nil)
	if _, _, err := service.IssueToken(model.User{ID: 8}); err == nil {
		t.Fatal("issue token accepted an account without an explicit password version")
	}
}

// TestIssueTokenPairStoresOnlyRefreshHash 验证 refresh token 只以摘要形式持久化。
func TestIssueTokenPairStoresOnlyRefreshHash(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&model.RefreshSession{}); err != nil {
		t.Fatalf("migrate refresh sessions: %v", err)
	}

	service := NewService(&config.Config{JWT: config.JWTConfig{
		Secret:           "test-secret",
		ExpiresIn:        time.Hour,
		RefreshExpiresIn: 24 * time.Hour,
		Issuer:           "test-issuer",
	}}, db)
	pair, err := service.IssueTokenPair(model.User{ID: 7, PasswordVersion: 1})
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}
	var session model.RefreshSession
	if err := db.Where("user_id = ?", 7).First(&session).Error; err != nil {
		t.Fatalf("find refresh session: %v", err)
	}
	if session.TokenHash == pair.RefreshToken || len(session.TokenHash) != 64 {
		t.Fatalf("refresh token was not stored as a SHA-256 hex hash: %q", session.TokenHash)
	}
	if !session.ExpiresAt.After(session.LastUsedAt) {
		t.Fatalf("refresh session expiry = %v, last used = %v", session.ExpiresAt, session.LastUsedAt)
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "seven characters", password: "1234567"},
		{name: "eight characters", password: "12345678", valid: true},
		{name: "seventy two bytes", password: "123456789012345678901234567890123456789012345678901234567890123456789012", valid: true},
		{name: "seventy three bytes", password: "1234567890123456789012345678901234567890123456789012345678901234567890123"},
		{name: "eight multibyte characters", password: "密码密码密码密码", valid: true},
		{name: "multibyte password over byte limit", password: "密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码密码", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidatePassword(tt.password) == nil; got != tt.valid {
				t.Fatalf("ValidatePassword valid = %v, want %v", got, tt.valid)
			}
		})
	}
}
