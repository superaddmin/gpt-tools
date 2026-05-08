// Package luckmail 提供 LuckMail 平台 API 的 Go SDK 封装
//
// 支持用户端和供应商端两套 API，所有请求均通过统一的 HTTP 客户端处理。
package luckmail

import "fmt"

// LuckMailError 是 SDK 所有异常的基础类型
type LuckMailError struct {
	Message string
}

func (e *LuckMailError) Error() string {
	return e.Message
}

// AuthError 鉴权失败异常
type AuthError struct {
	LuckMailError
}

// NewAuthError 创建鉴权失败异常
func NewAuthError(message string) *AuthError {
	if message == "" {
		message = "Authentication failed"
	}
	return &AuthError{LuckMailError{Message: message}}
}

// APIError API 调用异常
type APIError struct {
	Code    int
	Message string
	Data    interface{}
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API Error [%d]: %s", e.Code, e.Message)
}

// NewAPIError 创建 API 调用异常
func NewAPIError(code int, message string, data interface{}) *APIError {
	return &APIError{Code: code, Message: message, Data: data}
}

// NetworkError 网络请求异常
type NetworkError struct {
	LuckMailError
}

// NewNetworkError 创建网络请求异常
func NewNetworkError(message string) *NetworkError {
	if message == "" {
		message = "Network error occurred"
	}
	return &NetworkError{LuckMailError{Message: message}}
}

// TimeoutError 超时异常
type TimeoutError struct {
	LuckMailError
}

// NewTimeoutError 创建超时异常
func NewTimeoutError(message string) *TimeoutError {
	if message == "" {
		message = "Request timed out"
	}
	return &TimeoutError{LuckMailError{Message: message}}
}
