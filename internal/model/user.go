package model

import (
	"time"
)

// User 是系统登录账号。
//
// AccountType 控制责任归属：
// - personal：办公室、仓库管理员等责任到人的岗位。
// - department_terminal：车间公共电脑等只能责任到部门和终端的岗位。
type User struct {
	BaseModel
	Username        string     `json:"username" gorm:"size:80;not null;uniqueIndex"`
	AccountType     string     `json:"account_type" gorm:"size:40;not null;index"`
	Name            string     `json:"name" gorm:"size:120;not null"`
	OrganizationID  uint       `json:"organization_id" gorm:"not null;index"`
	DepartmentID    *uint      `json:"department_id" gorm:"index"`
	TerminalID      *uint      `json:"terminal_id" gorm:"index"`
	Status          string     `json:"status" gorm:"size:30;not null;default:active"`
	PasswordHash    string     `json:"-" gorm:"size:255;not null"`
	PasswordVersion int        `json:"-" gorm:"not null;default:1"`
	LastLoginAt     *time.Time `json:"last_login_at"`
}

// RefreshSession 保存登录会话的 refresh token 摘要。
//
// 原始 refresh token 只在登录或轮换响应中返回，数据库仅保存 SHA-256 摘要；
// 每次成功轮换都会撤销旧记录并创建新记录，避免长期复用同一个凭据。
type RefreshSession struct {
	BaseModel
	UserID     uint       `json:"user_id" gorm:"not null;index"`
	TokenHash  string     `json:"-" gorm:"size:64;not null;uniqueIndex"`
	ExpiresAt  time.Time  `json:"expires_at" gorm:"not null;index"`
	LastUsedAt time.Time  `json:"last_used_at" gorm:"not null"`
	RevokedAt  *time.Time `json:"revoked_at" gorm:"index"`
}
