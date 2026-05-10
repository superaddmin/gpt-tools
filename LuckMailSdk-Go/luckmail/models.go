package luckmail

import "encoding/json"

// UserInfo 用户信息
type UserInfo struct {
	ID              int    `json:"id"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	Balance         string `json:"balance"`
	Status          int    `json:"status"`
	APIEmailEnabled int    `json:"api_email_enabled"`
	APIEmailPrice   string `json:"api_email_price"`
}

// EmailItem 邮箱列表项
type EmailItem struct {
	ID           int    `json:"id"`
	Address      string `json:"address"`
	Type         string `json:"type"`
	Status       int    `json:"status"`
	Domain       string `json:"domain"`
	TotalUsed    int    `json:"total_used"`
	SuccessCount int    `json:"success_count"`
	FailCount    int    `json:"fail_count"`
}

// ProjectPrice 项目定价
type ProjectPrice struct {
	EmailType string `json:"email_type"`
	CodePrice string `json:"code_price"`
	BuyPrice  string `json:"buy_price"`
}

// ProjectItem 项目信息
type ProjectItem struct {
	ID             int            `json:"id"`
	Name           string         `json:"name"`
	Code           string         `json:"code"`
	EmailTypes     []string       `json:"email_types"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	WarrantyHours  int            `json:"warranty_hours"`
	DailyLimit     int            `json:"daily_limit"`
	Description    string         `json:"description"`
	Prices         []ProjectPrice `json:"prices"`
}

// OrderInfo 订单信息（创建后）
type OrderInfo struct {
	OrderNo        string `json:"order_no"`
	EmailAddress   string `json:"email_address"`
	Project        string `json:"project"`
	Price          string `json:"price"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ExpiredAt      string `json:"expired_at"`
}

// OrderCode 订单验证码查询结果
// Status: pending / success / timeout / cancelled
type OrderCode struct {
	OrderNo          string `json:"order_no"`
	Status           string `json:"status"`
	VerificationCode string `json:"verification_code,omitempty"`
	MailFrom         string `json:"mail_from,omitempty"`
	MailSubject      string `json:"mail_subject,omitempty"`
	MailBodyHTML     string `json:"mail_body_html,omitempty"`
}

// PurchaseItem 已购邮箱
type PurchaseItem struct {
	ID            int    `json:"id"`
	EmailAddress  string `json:"email_address"`
	Token         string `json:"token"`
	ProjectName   string `json:"project_name"`
	Price         string `json:"price"`
	Status        int    `json:"status"`
	TagID         int    `json:"tag_id"`
	TagName       string `json:"tag_name"`
	UserDisabled  int    `json:"user_disabled"`
	WarrantyHours int    `json:"warranty_hours"`
	WarrantyUntil string `json:"warranty_until,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// TagItem 邮箱标签
type TagItem struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Remark        string `json:"remark"`
	LimitType     int    `json:"limit_type"` // 0=不下发 1=可下发
	PurchaseCount int    `json:"purchase_count"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// CreateTagResult 创建标签结果
type CreateTagResult struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Remark    string `json:"remark"`
	LimitType int    `json:"limit_type"`
	CreatedAt string `json:"created_at,omitempty"`
}

// TokenCode Token 查询验证码结果
type TokenCode struct {
	EmailAddress     string          `json:"email_address"`
	Project          string          `json:"project"`
	HasNewMail       bool            `json:"has_new_mail"`
	VerificationCode string          `json:"verification_code,omitempty"`
	Mail             json.RawMessage `json:"mail,omitempty"`
}

// TokenMailItem Token 邮件列表项
// TokenAliveResult Token 测活结果
type TokenAliveResult struct {
	EmailAddress string `json:"email_address"`
	Project      string `json:"project"`
	Alive        bool   `json:"alive"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	MailCount    int    `json:"mail_count"`
}

type TokenMailItem struct {
	MessageID  string `json:"message_id"`
	From       string `json:"from"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	HTMLBody   string `json:"html_body"`
	ReceivedAt string `json:"received_at"`
}

// TokenMailList Token 邮件列表结果
type TokenMailList struct {
	EmailAddress  string          `json:"email_address"`
	Project       string          `json:"project"`
	WarrantyUntil string          `json:"warranty_until"`
	Mails         []TokenMailItem `json:"mails"`
}

// TokenMailDetail Token 邮件详情结果
type TokenMailDetail struct {
	MessageID        string `json:"message_id"`
	From             string `json:"from"`
	To               string `json:"to"`
	Subject          string `json:"subject"`
	BodyText         string `json:"body_text"`
	BodyHTML         string `json:"body_html"`
	ReceivedAt       string `json:"received_at"`
	VerificationCode string `json:"verification_code"`
}

// AppealInfo 申述信息
type AppealInfo struct {
	AppealNo    string `json:"appeal_no"`
	AppealType  int    `json:"appeal_type"`
	Reason      string `json:"reason"`
	Description string `json:"description"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// PageResult 分页结果
type PageResult struct {
	List     []json.RawMessage `json:"list"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// ImportResult 导入邮箱结果
type ImportResult struct {
	Success   int `json:"success"`
	Duplicate int `json:"duplicate"`
	Failed    int `json:"failed"`
}

// PurchaseResult 购买邮箱结果
type PurchaseResult struct {
	Purchases    []PurchaseItem `json:"purchases"`
	TotalCost    string         `json:"total_cost"`
	BalanceAfter string         `json:"balance_after"`
}

// CreateAppealResult 创建申述结果
type CreateAppealResult struct {
	AppealNo string `json:"appeal_no"`
}

// BatchReplyResult 批量处理申述结果
type BatchReplyResult struct {
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// EmailTypeItem 邮箱类型信息
type EmailTypeItem struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ===== 供应商模型 =====

// SupplierProfile 供应商个人信息
type SupplierProfile struct {
	ID                 int    `json:"id"`
	Username           string `json:"username"`
	Email              string `json:"email"`
	Balance            string `json:"balance"`
	FrozenBalance      string `json:"frozen_balance"`
	CodeCommissionRate string `json:"code_commission_rate"`
	BuyCommissionRate  string `json:"buy_commission_rate"`
	Status             int    `json:"status"`
}

// SupplierEmailItem 供应商邮箱列表项
type SupplierEmailItem struct {
	ID           int    `json:"id"`
	Address      string `json:"address"`
	Type         string `json:"type"`
	Status       int    `json:"status"`
	Domain       string `json:"domain"`
	TotalUsed    int    `json:"total_used"`
	SuccessCount int    `json:"success_count"`
	FailCount    int    `json:"fail_count"`
	IsShortTerm  int    `json:"is_short_term"`
}

// AppealItem 申述列表项（供应商端）
type AppealItem struct {
	ID        int    `json:"id"`
	AppealNo  string `json:"appeal_no"`
	OrderNo   string `json:"order_no"`
	Reason    string `json:"reason"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}

// AppealDetail 申述详情
type AppealDetail struct {
	AppealNo      string `json:"appeal_no"`
	OrderNo       string `json:"order_no"`
	Reason        string `json:"reason"`
	Status        int    `json:"status"`
	SupplierReply string `json:"supplier_reply,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// DashboardSummary 供应商数据看板
type DashboardSummary struct {
	TotalEmails      int                    `json:"total_emails"`
	ActiveEmails     int                    `json:"active_emails"`
	TotalAssigned    int                    `json:"total_assigned"`
	TotalSuccess     int                    `json:"total_success"`
	SuccessRate      float64                `json:"success_rate"`
	TotalCommission  string                 `json:"total_commission"`
	AvailableBalance string                 `json:"available_balance"`
	TodayAssigned    int                    `json:"today_assigned"`
	TodaySuccess     int                    `json:"today_success"`
	TodayCommission  string                 `json:"today_commission"`
	EmailCategory    map[string]interface{} `json:"email_category"`
}
