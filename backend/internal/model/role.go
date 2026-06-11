package model

import "time"

// Role 表示系统角色。
type Role struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Code        string    `json:"code"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// RoleRequest 是角色新增和编辑入参。
type RoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
