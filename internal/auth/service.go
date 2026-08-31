// Package auth 负责登录、JWT 签发和当前用户上下文。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	// ErrInvalidRefreshToken 表示 refresh token 不存在、过期或已被轮换/撤销。
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrRefreshAccountDisabled 表示 refresh token 所属账号已停用。
	ErrRefreshAccountDisabled = errors.New("refresh account disabled")
)

const (
	// ContextUserKey 是 Echo Context 中保存当前登录用户的键。
	ContextUserKey = "current_user"

	// InitialPasswordVersion 是新账号和新 JWT 的起始密码版本。
	InitialPasswordVersion = 1
	// MinPasswordLength 是密码允许的最小字符数。
	MinPasswordLength = 8
	// MaxPasswordBytes 是 bcrypt 支持的密码最大字节数。
	MaxPasswordBytes = 72
)

// CurrentUser 是从 JWT 和数据库中还原出来的当前登录身份。
//
// 部门终端账号会带上 DepartmentID 和 TerminalID，审计中据此记录部门与终端。
type CurrentUser struct {
	ID             uint
	Username       string
	AccountType    string
	Name           string
	OrganizationID uint
	DepartmentID   *uint
	TerminalID     *uint
	Permissions    []string
	Roles          []string
}

// Claims 是写入 JWT 的业务声明。
type Claims struct {
	UserID          uint   `json:"user_id"`
	Username        string `json:"username"`
	AccountType     string `json:"account_type"`
	PasswordVersion int    `json:"password_version"`
	jwt.RegisteredClaims
}

// Service 封装认证相关能力。
type Service struct {
	// Config 是 JWT 签发所需配置。
	Config *config.Config
	// DB 是用户、角色和权限查询所需数据库连接。
	DB *gorm.DB
}

// TokenPair 是登录或续期时返回给客户端的令牌集合。
type TokenPair struct {
	AccessToken      string
	ExpiresAt        time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// NewService 创建认证服务。
//
// 参数说明：
// - cfg：系统配置。
// - db：GORM 数据库连接。
func NewService(cfg *config.Config, db *gorm.DB) *Service {
	return &Service{Config: cfg, DB: db}
}

// IssueToken 为指定用户签发 JWT。
//
// 参数说明：
// - user：已通过密码校验且状态正常的用户模型。
//
// 返回说明：
// - string：JWT 字符串。
// - time.Time：过期时间。
// - error：签名失败时返回错误。
func (s *Service) IssueToken(user model.User) (string, time.Time, error) {
	if user.PasswordVersion < InitialPasswordVersion {
		return "", time.Time{}, fmt.Errorf("user password version must be at least %d", InitialPasswordVersion)
	}
	expiresAt := time.Now().Add(s.Config.JWT.ExpiresIn)
	claims := Claims{
		UserID:          user.ID,
		Username:        user.Username,
		AccountType:     user.AccountType,
		PasswordVersion: user.PasswordVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    s.Config.JWT.Issuer,
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.Config.JWT.Secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issue jwt token: %w", err)
	}
	return token, expiresAt, nil
}

// IssueTokenPair 为指定用户签发 access token 和新的 refresh token。
//
// 原始 refresh token 不会写入数据库，数据库只保存不可逆摘要。
func (s *Service) IssueTokenPair(user model.User) (TokenPair, error) {
	accessToken, expiresAt, err := s.IssueToken(user)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, refreshExpiresAt, err := s.createRefreshSession(user.ID, time.Now())
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:      accessToken,
		ExpiresAt:        expiresAt,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

// RotateRefreshToken 原子轮换 refresh token，并签发新的 access token。
//
// 旧 refresh token 成功使用后立即撤销；连续 30 天没有成功轮换时，旧会话自然过期。
func (s *Service) RotateRefreshToken(raw string) (model.User, TokenPair, error) {
	now := time.Now()
	hash := hashRefreshToken(raw)
	var user model.User
	var pair TokenPair

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var session model.RefreshSession
		if err := tx.Where("token_hash = ?", hash).First(&session).Error; err != nil {
			return ErrInvalidRefreshToken
		}
		if session.RevokedAt != nil || !now.Before(session.ExpiresAt) {
			return ErrInvalidRefreshToken
		}
		if err := tx.First(&user, session.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidRefreshToken
			}
			return fmt.Errorf("load refresh session user: %w", err)
		}
		if user.Status != model.StatusActive {
			return ErrRefreshAccountDisabled
		}

		accessToken, accessExpiresAt, err := s.IssueToken(user)
		if err != nil {
			return err
		}
		newRefreshToken, refreshExpiresAt, err := s.createRefreshSessionWithDB(tx, user.ID, now)
		if err != nil {
			return err
		}

		revokedAt := now
		result := tx.Model(&model.RefreshSession{}).
			Where("id = ? AND revoked_at IS NULL", session.ID).
			Updates(map[string]any{"revoked_at": revokedAt, "last_used_at": now})
		if result.Error != nil {
			return fmt.Errorf("revoke rotated refresh session: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrInvalidRefreshToken
		}

		pair = TokenPair{
			AccessToken:      accessToken,
			ExpiresAt:        accessExpiresAt,
			RefreshToken:     newRefreshToken,
			RefreshExpiresAt: refreshExpiresAt,
		}
		return nil
	})
	if err != nil {
		return model.User{}, TokenPair{}, err
	}
	return user, pair, nil
}

// RevokeRefreshToken 撤销一个 refresh token；令牌不存在时保持幂等成功。
func (s *Service) RevokeRefreshToken(raw string) error {
	if raw == "" {
		return nil
	}
	now := time.Now()
	return s.DB.Model(&model.RefreshSession{}).
		Where("token_hash = ? AND revoked_at IS NULL", hashRefreshToken(raw)).
		Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error
}

// RevokeRefreshTokensForUser 撤销指定用户的全部 refresh token。
func (s *Service) RevokeRefreshTokensForUser(db *gorm.DB, userID uint, now time.Time) error {
	return RevokeRefreshTokensForUser(db, userID, now)
}

// RevokeRefreshTokensForUser 在调用方事务中撤销指定用户的全部 refresh token。
//
// 该函数不依赖认证服务配置，便于用户管理等跨模块事务在更新密码版本时
// 一并撤销会话，确保旧 refresh token 不能重新签发新版本 JWT。
func RevokeRefreshTokensForUser(db *gorm.DB, userID uint, now time.Time) error {
	return db.Model(&model.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error
}

func (s *Service) createRefreshSession(userID uint, now time.Time) (string, time.Time, error) {
	return s.createRefreshSessionWithDB(s.DB, userID, now)
}

func (s *Service) createRefreshSessionWithDB(db *gorm.DB, userID uint, now time.Time) (string, time.Time, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(bytes)
	expiresAt := now.Add(s.Config.JWT.RefreshExpiresIn)
	session := model.RefreshSession{
		UserID:     userID,
		TokenHash:  hashRefreshToken(raw),
		ExpiresAt:  expiresAt,
		LastUsedAt: now,
	}
	if err := db.Create(&session).Error; err != nil {
		return "", time.Time{}, fmt.Errorf("store refresh session: %w", err)
	}
	return raw, expiresAt, nil
}

func hashRefreshToken(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", digest[:])
}

// CurrentUserFromModel 根据用户模型组装请求上下文中的 CurrentUser。
//
// 参数说明：
// - user：数据库中的用户模型。
//
// 返回说明：包含角色编码和权限编码的当前用户快照。
func (s *Service) CurrentUserFromModel(user model.User) (*CurrentUser, error) {
	roles, err := s.RoleCodesForUser(user.ID)
	if err != nil {
		return nil, err
	}
	permissions, err := s.PermissionCodesForUser(user.ID)
	if err != nil {
		return nil, err
	}
	return &CurrentUser{
		ID:             user.ID,
		Username:       user.Username,
		AccountType:    user.AccountType,
		Name:           user.Name,
		OrganizationID: user.OrganizationID,
		DepartmentID:   user.DepartmentID,
		TerminalID:     user.TerminalID,
		Permissions:    permissions,
		Roles:          roles,
	}, nil
}

// RoleCodesForUser 查询用户拥有的角色编码。
//
// 参数说明：
// - userID：用户 ID。
func (s *Service) RoleCodesForUser(userID uint) ([]string, error) {
	var roles []string
	err := s.DB.Table("user_roles").
		Select("roles.code").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ?", userID).
		Scan(&roles).Error
	return roles, err
}

// PermissionCodesForUser 查询用户拥有的权限编码。
//
// 参数说明：
// - userID：用户 ID。
func (s *Service) PermissionCodesForUser(userID uint) ([]string, error) {
	var codes []string
	err := s.DB.Table("user_roles").
		Select("DISTINCT permissions.code").
		Joins("JOIN role_permissions ON role_permissions.role_id = user_roles.role_id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("user_roles.user_id = ?", userID).
		Order("permissions.code").
		Scan(&codes).Error
	return codes, err
}

// GetCurrentUser 从 Echo Context 中读取当前登录用户。
//
// 参数说明：
// - c：Echo 请求上下文。
func GetCurrentUser(c *echo.Context) *CurrentUser {
	value := c.Get(ContextUserKey)
	current, _ := value.(*CurrentUser)
	return current
}

// CurrentUserResponse 将 CurrentUser 转成 API 响应。
//
// 参数说明：
// - current：当前登录用户快照。
func CurrentUserResponse(current *CurrentUser) CurrentUserDTO {
	return CurrentUserDTO{
		ID:             current.ID,
		Username:       current.Username,
		AccountType:    current.AccountType,
		Name:           current.Name,
		OrganizationID: current.OrganizationID,
		DepartmentID:   current.DepartmentID,
		TerminalID:     current.TerminalID,
		Roles:          current.Roles,
		Permissions:    current.Permissions,
	}
}

// HashPassword 使用 bcrypt 生成密码哈希。
//
// 参数说明：
// - password：明文密码，只允许在请求处理短暂存在。
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(bytes), nil
}

// ValidatePassword 校验密码字符数和 bcrypt 的字节长度限制。
//
// bcrypt 的长度限制按字节计算，而 API 的最小长度按 UTF-8 字符计算；两者
// 分开校验，避免多字节密码绕过 bcrypt 的最大长度限制或被截断。
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("password must contain at least %d characters", MinPasswordLength)
	}
	if len([]byte(password)) > MaxPasswordBytes {
		return fmt.Errorf("password must not exceed %d bytes", MaxPasswordBytes)
	}
	return nil
}
