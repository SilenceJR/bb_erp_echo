package model

import "time"

// User 是系统登录账号。
//
// AccountType 控制责任归属：
// - personal：办公室、仓库管理员等责任到人的岗位。
// - department_terminal：车间公共电脑等只能责任到部门和终端的岗位。
type User struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	Username       string     `json:"username" gorm:"size:80;not null;uniqueIndex"`
	AccountType    string     `json:"account_type" gorm:"size:40;not null;index"`
	Name           string     `json:"name" gorm:"size:120;not null"`
	OrganizationID uint       `json:"organization_id" gorm:"not null;index"`
	DepartmentID   *uint      `json:"department_id" gorm:"index"`
	TerminalID     *uint      `json:"terminal_id" gorm:"index"`
	Status         string     `json:"status" gorm:"size:30;not null;default:active"`
	PasswordHash   string     `json:"-" gorm:"size:255;not null"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
