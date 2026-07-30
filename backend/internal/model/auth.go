package model

// Claims 是本地轻量 token 中携带的登录态信息。
type Claims struct {
	UserID      int64    `json:"userId"`
	Username    string   `json:"username"`
	RoleCode    string   `json:"roleCode,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	AuthVersion int64    `json:"authVersion"`
	ExpiresAt   int64    `json:"expiresAt"`
}

// LoginRequest 是登录入参。
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// RegisterRequest 是注册入参。
type RegisterRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}
