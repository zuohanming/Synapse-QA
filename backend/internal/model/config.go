package model

import "time"

type Project struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProjectRequest struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Product struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"projectId"`
	ProjectName string    `json:"projectName"`
	Name        string    `json:"name"`
	UIType      string    `json:"uiType"`
	APIType     string    `json:"apiType"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ProductRequest struct {
	ProjectID int64  `json:"projectId"`
	Name      string `json:"name"`
	UIType    string `json:"uiType"`
	APIType   string `json:"apiType"`
}

type ProductModule struct {
	ID        int64     `json:"id"`
	ProductID int64     `json:"productId"`
	Name      string    `json:"name"`
	Level1    string    `json:"level1"`
	Level2    string    `json:"level2"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProductModuleRequest struct {
	ProductID int64  `json:"productId"`
	Name      string `json:"name"`
	Level1    string `json:"level1"`
	Level2    string `json:"level2"`
}

type TestObject struct {
	ID           int64     `json:"id"`
	ProductID    int64     `json:"productId"`
	ProductName  string    `json:"productName"`
	EnvName      string    `json:"envName"`
	Target       string    `json:"target"`
	DeployEnv    string    `json:"deployEnv"`
	AutoType     string    `json:"autoType"`
	Owner        string    `json:"owner"`
	QueryEnabled bool      `json:"queryEnabled"`
	WriteEnabled bool      `json:"writeEnabled"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type TestObjectRequest struct {
	ProductID    int64  `json:"productId"`
	EnvName      string `json:"envName"`
	Target       string `json:"target"`
	DeployEnv    string `json:"deployEnv"`
	AutoType     string `json:"autoType"`
	Owner        string `json:"owner"`
	QueryEnabled bool   `json:"queryEnabled"`
	WriteEnabled bool   `json:"writeEnabled"`
}

type ExecutorTokenConfig struct {
	MaskedToken string    `json:"maskedToken"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ExecutorTokenGenerateResult struct {
	Token       string    `json:"token"`
	MaskedToken string    `json:"maskedToken"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
