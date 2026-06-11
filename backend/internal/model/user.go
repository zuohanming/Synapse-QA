package model

import "time"

// User 表示系统用户列表和当前登录用户信息。
type User struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Email       string     `json:"email"`
	Status      string     `json:"status"`
	RoleName    string     `json:"roleName"`
	MCPAPIKey   string     `json:"mcpApiKey"`
	LastLoginIP string     `json:"lastLoginIp"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// UserUpdateRequest 是用户编辑和启停用的入参。
type UserUpdateRequest struct {
	Status      *string `json:"status"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	RoleID      *int64  `json:"roleId"`
}
