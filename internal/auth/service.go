// Package auth 负责登录、JWT 签发和当前用户上下文。
package auth

import (
	"fmt"
	"strconv"
	"time"

	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/model"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ContextUserKey 是 Echo Context 中保存当前登录用户的键。
const ContextUserKey = "current_user"

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
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	AccountType string `json:"account_type"`
	jwt.RegisteredClaims
}

// Service 封装认证相关能力。
type Service struct {
	// Config 是 JWT 签发所需配置。
	Config *config.Config
	// DB 是用户、角色和权限查询所需数据库连接。
	DB *gorm.DB
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
	expiresAt := time.Now().Add(s.Config.JWT.ExpiresIn)
	claims := Claims{
		UserID:      user.ID,
		Username:    user.Username,
		AccountType: user.AccountType,
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
func CurrentUserResponse(current *CurrentUser) map[string]any {
	return map[string]any{
		"id":              current.ID,
		"username":        current.Username,
		"account_type":    current.AccountType,
		"name":            current.Name,
		"organization_id": current.OrganizationID,
		"department_id":   current.DepartmentID,
		"terminal_id":     current.TerminalID,
		"roles":           current.Roles,
		"permissions":     current.Permissions,
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
