// Package luckmail 提供 LuckMail 平台 API 的 Go SDK 封装
//
// # 快速开始
//
//	client := luckmail.New("your_api_key")
//
//	// 查询余额
//	ctx := context.Background()
//	balance, err := client.User.GetBalance(ctx)
//
//	// 接码（一站式）
//	result, err := client.User.CreateAndWait(ctx, "twitter", nil)
//	if err == nil && result.Status == "success" {
//	    fmt.Println("验证码:", result.VerificationCode)
//	}
package luckmail

import (
	"context"
	"time"
)

// DefaultBaseURL 默认 API 基础 URL
const DefaultBaseURL = "https://mails.luckyous.com"

// Version SDK 版本号
const Version = "1.0.3"

// Client LuckMail SDK 主客户端
//
// 提供用户端（User）和供应商端（Supplier）两套 API 访问入口。
//
// 示例（用户端）:
//
//	client := luckmail.New("your_api_key")
//	ctx := context.Background()
//
//	// 查询余额
//	balance, err := client.User.GetBalance(ctx)
//	fmt.Println("余额:", balance)
//
//	// 接码（一站式方法）
//	result, err := client.User.CreateAndWait(ctx, "twitter", nil)
//	if err == nil {
//	    fmt.Println("验证码:", result.VerificationCode)
//	}
//
// 示例（供应商端）:
//
//	// 查看数据看板
//	summary, err := client.Supplier.GetDashboard(ctx)
//	fmt.Println("今日接码:", summary.TodayAssigned)
//
//	// 处理申述
//	err = client.Supplier.ReplyAppeal(ctx, "APL001", 1, "同意退款")
type Client struct {
	http     *HTTPClient
	User     *UserAPI
	Supplier *SupplierAPI
}

// ClientOption 客户端选项
type ClientOption func(*Client)

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.http.Timeout = timeout
		c.http.httpClient.Timeout = timeout
	}
}

// WithBaseURL 自定义 API 基础 URL（默认 https://mails.luckyous.com）
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.http.BaseURL = baseURL
	}
}

// New 创建 LuckMail 客户端（API Key 模式）
//
// 参数:
//   - apiKey: API Key（在平台「个人设置」页面生成）
//   - opts: 可选配置项（WithTimeout、WithBaseURL）
//
// 示例:
//
//	// 基础用法（使用默认地址 https://mails.luckyous.com）
//	client := luckmail.New("your_api_key")
//
//	// 自定义超时
//	client := luckmail.New("your_api_key",
//	    luckmail.WithTimeout(60 * time.Second),
//	)
//
//	// 自定义 API 地址
//	client := luckmail.New("your_api_key",
//	    luckmail.WithBaseURL("https://your-domain.com"),
//	)
func New(apiKey string, opts ...ClientOption) *Client {
	httpClient := NewHTTPClient(DefaultBaseURL, apiKey, defaultTimeout)
	c := &Client{
		http: httpClient,
	}
	c.User = NewUserAPI(httpClient)
	c.Supplier = NewSupplierAPI(httpClient)

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CreateAndWait 创建接码订单并等待验证码（一站式方法）
//
// 自动创建订单并轮询等待验证码，是最简便的接码方式。
//
// 参数:
//   - ctx: 上下文
//   - projectCode: 项目编码，如 "twitter", "facebook"
//   - opts: 可选参数（OrderOptions）
//
// 示例:
//
//	result, err := client.CreateAndWait(ctx, "twitter", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if result.Status == "success" {
//	    fmt.Println("✅ 验证码:", result.VerificationCode)
//	    fmt.Println("📧 来自:", result.MailFrom)
//	} else {
//	    fmt.Println("❌ 接码失败:", result.Status)
//	}
func (c *Client) CreateAndWait(ctx context.Context, projectCode string, opts *OrderOptions) (*OrderCode, error) {
	return c.User.CreateAndWait(ctx, projectCode, opts)
}
