package model

import "errors"

var (
	ErrUnauthorized = errors.New("未授权")
	ErrNotFound     = errors.New("资源不存在")
	ErrConflict     = errors.New("状态冲突")
	ErrValidation   = errors.New("请求参数无效")
)

// DomainError 保留安全的中文提示，并让 Controller 按稳定类别映射 HTTP 状态。
type DomainError struct {
	Kind    error
	Message string
}

func (e *DomainError) Error() string { return e.Message }
func (e *DomainError) Unwrap() error { return e.Kind }

func NewDomainError(kind error, message string) error {
	return &DomainError{Kind: kind, Message: message}
}
