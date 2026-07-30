package model

import "time"

// User 表示系统用户列表和当前登录用户信息。
type User struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"displayName"`
	Email              string     `json:"email"`
	Status             string     `json:"status"`
	RoleName           string     `json:"roleName"`
	RoleCode           string     `json:"roleCode"`
	RoleID             *int64     `json:"roleId"`
	Permissions        []string   `json:"permissions"`
	MustChangePassword bool       `json:"mustChangePassword"`
	AuthVersion        int64      `json:"-"`
	MCPAPIKey          string     `json:"mcpApiKey"`
	LastLoginIP        string     `json:"lastLoginIp"`
	LastLoginAt        *time.Time `json:"lastLoginAt"`
	LockedUntil        *time.Time `json:"lockedUntil"`
	CreatedAt          time.Time  `json:"createdAt"`
}

type UserCreateRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	RoleID      int64  `json:"roleId"`
}

type UserCreateResult struct {
	ID                int64  `json:"id"`
	TemporaryPassword string `json:"temporaryPassword"`
}

// UserUpdateRequest 是用户编辑和启停用的入参。
type UserUpdateRequest struct {
	Status      *string `json:"status"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	RoleID      *int64  `json:"roleId"`
}
