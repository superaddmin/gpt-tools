package luckmail

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const userAPIPrefix = "/api/v1/openapi"

// OrderOptions 创建订单可选参数
type OrderOptions struct {
	// EmailType 邮箱类型：ms_graph / ms_imap / self_built / google_variant
	EmailType string
	// Domain 指定域名，如 "outlook.com"
	Domain string
	// SpecifiedEmail 指定邮箱地址
	SpecifiedEmail string
	// VariantMode 谷歌变种模式（仅 EmailType=google_variant 时有效）: dot=点号变种 / plus=+号变种 / mixed=混合变种 / all=随机选择
	VariantMode string
	// Timeout 等待验证码的最大时间（仅用于 CreateAndWait），默认 300s
	Timeout time.Duration
	// Interval 轮询间隔（仅用于 CreateAndWait），默认 3s
	Interval time.Duration
	// OnPoll 每次轮询的回调（可选）
	OnPoll func(*OrderCode)
}

// UserAPI 用户端 API 接口集合
//
// 所有方法通过 context 控制超时和取消。
//
// 示例:
//
//	ctx := context.Background()
//	info, err := client.User.GetUserInfo(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(info.Username, info.Balance)
type UserAPI struct {
	client *HTTPClient
}

// NewUserAPI 创建用户端 API 实例
func NewUserAPI(client *HTTPClient) *UserAPI {
	return &UserAPI{client: client}
}

func (u *UserAPI) path(p string) string {
	return userAPIPrefix + p
}

// ===== 用户信息 =====

// GetUserInfo 获取用户信息及余额
//
// 示例:
//
//	info, err := client.User.GetUserInfo(ctx)
//	fmt.Println(info.Username, info.Balance)
func (u *UserAPI) GetUserInfo(ctx context.Context) (*UserInfo, error) {
	data, err := u.client.Get(ctx, u.path("/user/info"), nil)
	if err != nil {
		return nil, err
	}
	var info UserInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}
	return &info, nil
}

// GetBalance 查询余额
//
// 返回余额字符串，如 "150.0000"
//
// 示例:
//
//	balance, err := client.User.GetBalance(ctx)
//	fmt.Println("余额:", balance)
func (u *UserAPI) GetBalance(ctx context.Context) (string, error) {
	data, err := u.client.Get(ctx, u.path("/balance"), nil)
	if err != nil {
		return "", err
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse balance: %w", err)
	}
	if balance, ok := result["balance"]; ok {
		return balance, nil
	}
	return "0.0000", nil
}

// ===== 邮箱类型 =====

// GetEmailTypes 获取支持的邮箱类型列表
//
// 示例:
//
//	types, err := client.User.GetEmailTypes(ctx)
//	for _, t := range types {
//	    fmt.Println(t.Type, t.Name)
//	}
func (u *UserAPI) GetEmailTypes(ctx context.Context) ([]EmailTypeItem, error) {
	data, err := u.client.Get(ctx, u.path("/email-types"), nil)
	if err != nil {
		return nil, err
	}
	var items []EmailTypeItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse email types: %w", err)
	}
	return items, nil
}

// ===== 我的邮箱管理 =====

// GetEmailsParams 获取邮箱列表参数
type GetEmailsParams struct {
	Page     int
	PageSize int
	Keyword  string
	// Status 状态过滤：1=正常 2=异常 4=禁用
	Status int
}

// GetEmailsResult 获取邮箱列表结果
type GetEmailsResult struct {
	List     []EmailItem
	Total    int
	Page     int
	PageSize int
}

// GetEmails 获取我的邮箱列表（分页）
//
// 示例:
//
//	result, err := client.User.GetEmails(ctx, &luckmail.GetEmailsParams{
//	    Page: 1, Keyword: "outlook",
//	})
//	for _, email := range result.List {
//	    fmt.Println(email.Address, email.Status)
//	}
func (u *UserAPI) GetEmails(ctx context.Context, params *GetEmailsParams) (*GetEmailsResult, error) {
	p := map[string]string{
		"page":      "1",
		"page_size": "20",
	}
	if params != nil {
		if params.Page > 0 {
			p["page"] = strconv.Itoa(params.Page)
		}
		if params.PageSize > 0 {
			p["page_size"] = strconv.Itoa(params.PageSize)
		}
		if params.Keyword != "" {
			p["keyword"] = params.Keyword
		}
		if params.Status > 0 {
			p["status"] = strconv.Itoa(params.Status)
		}
	}

	data, err := u.client.Get(ctx, u.path("/emails"), p)
	if err != nil {
		return nil, err
	}
	return parseEmailsResult(data)
}

func parseEmailsResult(data json.RawMessage) (*GetEmailsResult, error) {
	var page struct {
		List     []EmailItem `json:"list"`
		Total    int         `json:"total"`
		Page     int         `json:"page"`
		PageSize int         `json:"page_size"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("failed to parse emails: %w", err)
	}
	return &GetEmailsResult{
		List:     page.List,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

// ImportEmailsRequest 导入邮箱请求
type ImportEmailsRequest struct {
	// Type 邮箱类型：ms_graph / ms_imap / google_variant / self_built
	Type   string                   `json:"type"`
	Emails []map[string]interface{} `json:"emails"`
}

// ImportEmails 导入邮箱到私有邮箱池
//
// 示例:
//
//	result, err := client.User.ImportEmails(ctx, &luckmail.ImportEmailsRequest{
//	    Type: "ms_graph",
//	    Emails: []map[string]interface{}{
//	        {
//	            "address":       "user@outlook.com",
//	            "client_id":     "xxx",
//	            "refresh_token": "xxx",
//	        },
//	    },
//	})
//	fmt.Printf("成功: %d, 重复: %d, 失败: %d\n", result.Success, result.Duplicate, result.Failed)
func (u *UserAPI) ImportEmails(ctx context.Context, req *ImportEmailsRequest) (*ImportResult, error) {
	data, err := u.client.Post(ctx, u.path("/emails/import"), req)
	if err != nil {
		return nil, err
	}
	var result ImportResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse import result: %w", err)
	}
	return &result, nil
}

// ExportEmails 导出邮箱（txt 文件流）
//
// 返回 txt 文件内容，每行格式：address----password 或 address----client_id----refresh_token
//
// 示例:
//
//	content, err := client.User.ExportEmails(ctx, "", 0)
//	if err == nil {
//	    os.WriteFile("emails.txt", content, 0644)
//	}
func (u *UserAPI) ExportEmails(ctx context.Context, keyword string, status int) ([]byte, error) {
	params := map[string]string{}
	if keyword != "" {
		params["keyword"] = keyword
	}
	if status > 0 {
		params["status"] = strconv.Itoa(status)
	}
	return u.client.GetStream(ctx, u.path("/emails/export"), params)
}

// ===== 项目列表 =====

// GetProjectsResult 获取项目列表结果
type GetProjectsResult struct {
	List     []ProjectItem
	Total    int
	Page     int
	PageSize int
}

// GetProjects 获取项目列表
//
// 示例:
//
//	result, err := client.User.GetProjects(ctx, 1, 50)
//	for _, p := range result.List {
//	    fmt.Println(p.Name, p.Code)
//	}
func (u *UserAPI) GetProjects(ctx context.Context, page, pageSize int) (*GetProjectsResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	params := map[string]string{
		"page":      strconv.Itoa(page),
		"page_size": strconv.Itoa(pageSize),
	}
	data, err := u.client.Get(ctx, u.path("/projects"), params)
	if err != nil {
		return nil, err
	}

	var page2 struct {
		List     []ProjectItem `json:"list"`
		Total    int           `json:"total"`
		Page     int           `json:"page"`
		PageSize int           `json:"page_size"`
	}
	if err := json.Unmarshal(data, &page2); err != nil {
		return nil, fmt.Errorf("failed to parse projects: %w", err)
	}
	return &GetProjectsResult{
		List:     page2.List,
		Total:    page2.Total,
		Page:     page2.Page,
		PageSize: page2.PageSize,
	}, nil
}

// ===== 接码订单 =====

// CreateOrder 创建接码订单
//
// 参数:
//   - projectCode: 项目编码，如 "twitter", "facebook"
//   - opts: 可选参数（EmailType、Domain、SpecifiedEmail）
//
// 示例:
//
//	order, err := client.User.CreateOrder(ctx, "twitter", &luckmail.OrderOptions{
//	    EmailType: "ms_graph",
//	})
//	fmt.Println("订单号:", order.OrderNo)
//	fmt.Println("邮箱:", order.EmailAddress)
func (u *UserAPI) CreateOrder(ctx context.Context, projectCode string, opts *OrderOptions) (*OrderInfo, error) {
	body := map[string]interface{}{
		"project_code": projectCode,
	}
	if opts != nil {
		if opts.EmailType != "" {
			body["email_type"] = opts.EmailType
		}
		if opts.Domain != "" {
			body["domain"] = opts.Domain
		}
		if opts.SpecifiedEmail != "" {
			body["specified_email"] = opts.SpecifiedEmail
		}
		if opts.VariantMode != "" {
			body["variant_mode"] = opts.VariantMode
		}
	}

	data, err := u.client.Post(ctx, u.path("/order/create"), body)
	if err != nil {
		return nil, err
	}
	var order OrderInfo
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("failed to parse order info: %w", err)
	}
	return &order, nil
}

// GetOrderCode 查询验证码（单次查询）
//
// 示例:
//
//	code, err := client.User.GetOrderCode(ctx, order.OrderNo)
//	if err == nil && code.Status == "success" {
//	    fmt.Println("验证码:", code.VerificationCode)
//	}
func (u *UserAPI) GetOrderCode(ctx context.Context, orderNo string) (*OrderCode, error) {
	data, err := u.client.Get(ctx, u.path(fmt.Sprintf("/order/%s/code", orderNo)), nil)
	if err != nil {
		return nil, err
	}
	var code OrderCode
	if err := json.Unmarshal(data, &code); err != nil {
		return nil, fmt.Errorf("failed to parse order code: %w", err)
	}
	return &code, nil
}

// CancelOrder 取消订单
//
// 示例:
//
//	err := client.User.CancelOrder(ctx, order.OrderNo)
func (u *UserAPI) CancelOrder(ctx context.Context, orderNo string) error {
	_, err := u.client.Post(ctx, u.path(fmt.Sprintf("/order/%s/cancel", orderNo)), map[string]interface{}{})
	return err
}

// GetOrdersParams 获取订单列表参数
type GetOrdersParams struct {
	Page     int
	PageSize int
	// Status 状态：1=待接码 2=已完成 3=已超时 4=已取消 5=已退款
	Status    int
	ProjectID int
}

// GetOrdersResult 获取订单列表结果
type GetOrdersResult struct {
	List     []map[string]interface{}
	Total    int
	Page     int
	PageSize int
}

// GetOrders 获取订单列表（分页）
//
// 示例:
//
//	result, err := client.User.GetOrders(ctx, &luckmail.GetOrdersParams{Status: 2})
//	fmt.Printf("共 %d 条已完成订单\n", result.Total)
func (u *UserAPI) GetOrders(ctx context.Context, params *GetOrdersParams) (*GetOrdersResult, error) {
	p := map[string]string{
		"page":      "1",
		"page_size": "20",
	}
	if params != nil {
		if params.Page > 0 {
			p["page"] = strconv.Itoa(params.Page)
		}
		if params.PageSize > 0 {
			p["page_size"] = strconv.Itoa(params.PageSize)
		}
		if params.Status > 0 {
			p["status"] = strconv.Itoa(params.Status)
		}
		if params.ProjectID > 0 {
			p["project_id"] = strconv.Itoa(params.ProjectID)
		}
	}

	data, err := u.client.Get(ctx, u.path("/orders"), p)
	if err != nil {
		return nil, err
	}

	var page struct {
		List     []map[string]interface{} `json:"list"`
		Total    int                      `json:"total"`
		Page     int                      `json:"page"`
		PageSize int                      `json:"page_size"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("failed to parse orders: %w", err)
	}
	return &GetOrdersResult{
		List:     page.List,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

// ===== 接码轮询（高级方法）=====

// WaitForCode 等待接码（带自动轮询）
//
// 会自动每隔 interval 秒查询一次，直到收到验证码或超时。
//
// 参数:
//   - orderNo: 订单编号
//   - timeout: 最大等待时间，默认 300s（传 0 使用默认值）
//   - interval: 轮询间隔，默认 3s（传 0 使用默认值）
//   - onPoll: 每次轮询的回调（可为 nil）
//
// 示例:
//
//	order, err := client.User.CreateOrder(ctx, "twitter", nil)
//	result, err := client.User.WaitForCode(ctx, order.OrderNo, 0, 0, nil)
//	if result.Status == "success" {
//	    fmt.Println("✅ 验证码:", result.VerificationCode)
//	} else {
//	    fmt.Println("❌ 接码失败:", result.Status)
//	}
func (u *UserAPI) WaitForCode(ctx context.Context, orderNo string, timeout, interval time.Duration, onPoll func(*OrderCode)) (*OrderCode, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for {
		result, err := u.GetOrderCode(ctx, orderNo)
		if err != nil {
			return nil, err
		}

		if onPoll != nil {
			onPoll(result)
		}

		if result.Status == "success" || result.Status == "timeout" || result.Status == "cancelled" {
			return result, nil
		}

		if time.Now().After(deadline) {
			return result, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// CreateAndWait 创建接码订单并等待验证码（一站式方法）
//
// 自动创建订单并轮询等待验证码，是最简便的接码方式。
//
// 示例:
//
//	result, err := client.User.CreateAndWait(ctx, "twitter", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if result.Status == "success" {
//	    fmt.Println("✅ 验证码:", result.VerificationCode)
//	    fmt.Println("📧 来自:", result.MailFrom)
//	} else {
//	    fmt.Println("❌ 接码失败:", result.Status)
//	}
//
//	// 带选项
//	result, err := client.User.CreateAndWait(ctx, "twitter", &luckmail.OrderOptions{
//	    EmailType: "ms_graph",
//	    Timeout:   5 * time.Minute,
//	    OnPoll: func(code *luckmail.OrderCode) {
//	        fmt.Println("轮询中... 状态:", code.Status)
//	    },
//	})
func (u *UserAPI) CreateAndWait(ctx context.Context, projectCode string, opts *OrderOptions) (*OrderCode, error) {
	order, err := u.CreateOrder(ctx, projectCode, opts)
	if err != nil {
		return nil, err
	}

	var timeout, interval time.Duration
	var onPoll func(*OrderCode)
	if opts != nil {
		timeout = opts.Timeout
		interval = opts.Interval
		onPoll = opts.OnPoll
	}

	return u.WaitForCode(ctx, order.OrderNo, timeout, interval, onPoll)
}

// ===== 购买邮箱 =====

// PurchaseEmailsRequest 购买邮箱请求
type PurchaseEmailsRequest struct {
	// ProjectCode 项目编码
	ProjectCode string `json:"project_code"`
	// Quantity 购买数量（1-10000）
	Quantity int `json:"quantity"`
	// EmailType 邮箱类型（可选）
	EmailType string `json:"email_type,omitempty"`
	// Domain 指定域名（可选）
	Domain string `json:"domain,omitempty"`
	// VariantMode 谷歌变种模式（仅 EmailType=google_variant 时有效）: dot=点号变种 / plus=+号变种 / mixed=混合变种 / all=随机选择
	VariantMode string `json:"variant_mode,omitempty"`
}

// PurchaseEmails 购买邮箱
//
// 示例:
//
//	result, err := client.User.PurchaseEmails(ctx, &luckmail.PurchaseEmailsRequest{
//	    ProjectCode: "twitter",
//	    Quantity:    5,
//	    EmailType:   "ms_graph",
//	})
//	for _, item := range result.Purchases {
//	    fmt.Println(item.EmailAddress, item.Token)
//	}
func (u *UserAPI) PurchaseEmails(ctx context.Context, req *PurchaseEmailsRequest) (*PurchaseResult, error) {
	data, err := u.client.Post(ctx, u.path("/email/purchase"), req)
	if err != nil {
		return nil, err
	}
	var result PurchaseResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse purchase result: %w", err)
	}
	return &result, nil
}

// GetPurchasesParams 获取已购邮箱列表参数
type GetPurchasesParams struct {
	Page     int
	PageSize int
	// ProjectID 按项目 ID 筛选
	ProjectID int
	// TagID 按标签 ID 筛选
	TagID int
	// Keyword 邮箱地址关键词搜索
	Keyword string
	// UserDisabled 禁用状态：-1=全部 0=正常 1=已禁用（不填表示全部）
	UserDisabled int
	// HasUserDisabled 是否传递 user_disabled 参数（避免与默认值 0 混淆）
	HasUserDisabled bool
}

// GetPurchasesResult 获取已购邮箱列表结果
type GetPurchasesResult struct {
	List     []PurchaseItem
	Total    int
	Page     int
	PageSize int
}

// GetPurchases 获取已购邮箱列表
//
// 示例:
//
//	result, err := client.User.GetPurchases(ctx, nil)
//	for _, item := range result.List {
//	    fmt.Println(item.EmailAddress, item.Token)
//	}
func (u *UserAPI) GetPurchases(ctx context.Context, params *GetPurchasesParams) (*GetPurchasesResult, error) {
	p := map[string]string{
		"page":      "1",
		"page_size": "20",
	}
	if params != nil {
		if params.Page > 0 {
			p["page"] = strconv.Itoa(params.Page)
		}
		if params.PageSize > 0 {
			p["page_size"] = strconv.Itoa(params.PageSize)
		}
		if params.ProjectID > 0 {
			p["project_id"] = strconv.Itoa(params.ProjectID)
		}
		if params.TagID > 0 {
			p["tag_id"] = strconv.Itoa(params.TagID)
		}
		if params.Keyword != "" {
			p["keyword"] = params.Keyword
		}
		if params.HasUserDisabled {
			p["user_disabled"] = strconv.Itoa(params.UserDisabled)
		}
	}

	data, err := u.client.Get(ctx, u.path("/email/purchases"), p)
	if err != nil {
		return nil, err
	}

	var page struct {
		List     []PurchaseItem `json:"list"`
		Total    int            `json:"total"`
		Page     int            `json:"page"`
		PageSize int            `json:"page_size"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("failed to parse purchases: %w", err)
	}
	return &GetPurchasesResult{
		List:     page.List,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

// GetTokenCode 通过 Token 获取最新验证码（已购邮箱）
//
// 示例:
//
//	result, err := client.User.GetTokenCode(ctx, "tok_abc123def456")
//	if err == nil && result.HasNewMail {
//	    fmt.Println("验证码:", result.VerificationCode)
//	}
func (u *UserAPI) GetTokenCode(ctx context.Context, token string) (*TokenCode, error) {
	data, err := u.client.Get(ctx, u.path(fmt.Sprintf("/email/token/%s/code", token)), nil)
	if err != nil {
		return nil, err
	}
	var code TokenCode
	if err := json.Unmarshal(data, &code); err != nil {
		return nil, fmt.Errorf("failed to parse token code: %w", err)
	}
	return &code, nil
}

// WaitForTokenCode 等待 Token 邮箱的验证码（带自动轮询）
//
// 示例:
//
//	result, err := client.User.WaitForTokenCode(ctx, "tok_abc123", 120*time.Second, 0, nil)
//	if err == nil && result.HasNewMail {
//	    fmt.Println("✅ 验证码:", result.VerificationCode)
//	}
//
// CheckTokenAlive 通过 Token 测试已购邮箱是否可以正常获取邮件列表
func (u *UserAPI) CheckTokenAlive(ctx context.Context, token string) (*TokenAliveResult, error) {
	data, err := u.client.Get(ctx, u.path(fmt.Sprintf("/email/token/%s/alive", token)), nil)
	if err != nil {
		return nil, err
	}
	var result TokenAliveResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse token alive result: %w", err)
	}
	return &result, nil
}

func (u *UserAPI) WaitForTokenCode(ctx context.Context, token string, timeout, interval time.Duration, onPoll func(*TokenCode)) (*TokenCode, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for {
		result, err := u.GetTokenCode(ctx, token)
		if err != nil {
			return nil, err
		}

		if onPoll != nil {
			onPoll(result)
		}

		if result.HasNewMail {
			return result, nil
		}

		if time.Now().After(deadline) {
			return result, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// ===== 已购邮箱邮件列表和详情 =====

// GetTokenMails 通过 Token 获取已购邮箱的邮件列表
//
// 返回邮件列表，包含邮箱地址、项目名称、保修截止时间和邮件列表。
//
// 示例:
//
//	result, err := client.User.GetTokenMails(ctx, "tok_abc123def456")
//	if err == nil {
//	    fmt.Printf("邮箱: %s, 项目: %s\n", result.EmailAddress, result.Project)
//	    for _, mail := range result.Mails {
//	        fmt.Printf("  [%s] %s: %s\n", mail.ReceivedAt, mail.From, mail.Subject)
//	    }
//	}
func (u *UserAPI) GetTokenMails(ctx context.Context, token string) (*TokenMailList, error) {
	data, err := u.client.Get(ctx, u.path(fmt.Sprintf("/email/token/%s/mails", token)), nil)
	if err != nil {
		return nil, err
	}
	var result TokenMailList
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse token mail list: %w", err)
	}
	if result.Mails == nil {
		result.Mails = []TokenMailItem{}
	}
	return &result, nil
}

// GetTokenMailDetail 通过 Token 获取已购邮箱的邮件详情
//
// 参数:
//   - token: 已购邮箱的 token
//   - messageID: 邮件 ID（从 GetTokenMails 返回的列表中获取）
//
// 示例:
//
//	detail, err := client.User.GetTokenMailDetail(ctx, "tok_abc123", "AAMkAGI2...")
//	if err == nil {
//	    fmt.Printf("主题: %s\n", detail.Subject)
//	    fmt.Printf("正文: %s\n", detail.BodyText)
//	    if detail.VerificationCode != "" {
//	        fmt.Printf("验证码: %s\n", detail.VerificationCode)
//	    }
//	}
func (u *UserAPI) GetTokenMailDetail(ctx context.Context, token, messageID string) (*TokenMailDetail, error) {
	data, err := u.client.Get(ctx, u.path(fmt.Sprintf("/email/token/%s/mails/%s", token, messageID)), nil)
	if err != nil {
		return nil, err
	}
	var detail TokenMailDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("failed to parse token mail detail: %w", err)
	}
	return &detail, nil
}

// ===== 申述 =====

// CreateAppealRequest 提交申述请求
type CreateAppealRequest struct {
	// AppealType 申述类型：1=接码订单 2=购买邮箱
	AppealType int `json:"appeal_type"`
	// Reason 申述原因：no_code / wrong_code / email_invalid
	Reason string `json:"reason"`
	// Description 详细描述
	Description string `json:"description"`
	// OrderID 接码订单 ID（AppealType=1 时必填）
	OrderID int `json:"order_id,omitempty"`
	// PurchaseID 购买记录 ID（AppealType=2 时必填）
	PurchaseID int `json:"purchase_id,omitempty"`
	// EvidenceURLs 证据截图 URL 列表（可选）
	EvidenceURLs []string `json:"evidence_urls,omitempty"`
}

// CreateAppeal 提交申述
//
// 示例:
//
//	result, err := client.User.CreateAppeal(ctx, &luckmail.CreateAppealRequest{
//	    AppealType:  1,
//	    OrderID:     123,
//	    Reason:      "no_code",
//	    Description: "等待 5 分钟未收到验证码",
//	})
//	fmt.Println("申述单号:", result.AppealNo)
func (u *UserAPI) CreateAppeal(ctx context.Context, req *CreateAppealRequest) (*CreateAppealResult, error) {
	data, err := u.client.Post(ctx, u.path("/appeal/create"), req)
	if err != nil {
		return nil, err
	}
	var result CreateAppealResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse appeal result: %w", err)
	}
	return &result, nil
}

// ===== 已购邮箱禁用管理 =====

// SetPurchaseDisabledRequest 设置已购邮箱禁用状态请求
type SetPurchaseDisabledRequest struct {
	// Disabled 禁用状态：0=启用 1=禁用
	Disabled int `json:"disabled"`
}

// SetPurchaseDisabled 设置已购邮箱禁用状态
//
// 示例:
//
//	err := client.User.SetPurchaseDisabled(ctx, 1, 1) // 禁用
//	err := client.User.SetPurchaseDisabled(ctx, 1, 0) // 启用
func (u *UserAPI) SetPurchaseDisabled(ctx context.Context, purchaseID int, disabled int) error {
	req := SetPurchaseDisabledRequest{Disabled: disabled}
	_, err := u.client.Put(ctx, u.path(fmt.Sprintf("/email/purchases/%d/disabled", purchaseID)), req)
	return err
}

// BatchSetPurchaseDisabledRequest 批量设置已购邮箱禁用状态请求
type BatchSetPurchaseDisabledRequest struct {
	// IDs 已购邮箱 ID 列表
	IDs []int `json:"ids"`
	// Disabled 禁用状态：0=启用 1=禁用
	Disabled int `json:"disabled"`
}

// BatchSetPurchaseDisabled 批量设置已购邮箱禁用状态
//
// 示例:
//
//	err := client.User.BatchSetPurchaseDisabled(ctx, []int{1, 2, 3}, 1) // 批量禁用
func (u *UserAPI) BatchSetPurchaseDisabled(ctx context.Context, ids []int, disabled int) error {
	req := BatchSetPurchaseDisabledRequest{IDs: ids, Disabled: disabled}
	_, err := u.client.Post(ctx, u.path("/email/purchases/batch-disabled"), req)
	return err
}

// ===== 已购邮箱标签管理 =====

// SetPurchaseTagRequest 设置已购邮箱标签请求
type SetPurchaseTagRequest struct {
	// TagID 标签 ID（与 TagName 二选一，传 0 表示移除标签）
	TagID *int `json:"tag_id,omitempty"`
	// TagName 标签名称（与 TagID 二选一）
	TagName string `json:"tag_name,omitempty"`
}

// SetPurchaseTag 设置已购邮箱标签
//
// 示例:
//
//	tagID := 1
//	err := client.User.SetPurchaseTag(ctx, 1, &luckmail.SetPurchaseTagRequest{TagID: &tagID})
//	err := client.User.SetPurchaseTag(ctx, 1, &luckmail.SetPurchaseTagRequest{TagName: "主力号"})
func (u *UserAPI) SetPurchaseTag(ctx context.Context, purchaseID int, req *SetPurchaseTagRequest) error {
	_, err := u.client.Put(ctx, u.path(fmt.Sprintf("/email/purchases/%d/tag", purchaseID)), req)
	return err
}

// BatchSetPurchaseTagRequest 批量设置已购邮箱标签请求
type BatchSetPurchaseTagRequest struct {
	// IDs 已购邮箱 ID 列表
	IDs []int `json:"ids"`
	// TagID 标签 ID（与 TagName 二选一，传 0 表示移除标签）
	TagID *int `json:"tag_id,omitempty"`
	// TagName 标签名称（与 TagID 二选一）
	TagName string `json:"tag_name,omitempty"`
}

// BatchSetPurchaseTag 批量设置已购邮箱标签
//
// 示例:
//
//	err := client.User.BatchSetPurchaseTag(ctx, &luckmail.BatchSetPurchaseTagRequest{
//	    IDs:     []int{1, 2, 3},
//	    TagName: "主力号",
//	})
func (u *UserAPI) BatchSetPurchaseTag(ctx context.Context, req *BatchSetPurchaseTagRequest) error {
	_, err := u.client.Post(ctx, u.path("/email/purchases/batch-tag"), req)
	return err
}

// APIGetPurchasesRequest 按标签获取已购邮箱请求
type APIGetPurchasesRequest struct {
	// Count 获取数量（1-100）
	Count int `json:"count"`
	// TagID 按标签 ID 筛选（与 TagName 二选一）
	TagID *int `json:"tag_id,omitempty"`
	// TagName 按标签名称筛选（与 TagID 二选一）
	TagName string `json:"tag_name,omitempty"`
	// MarkTagID 获取后将邮箱标记为此标签 ID（与 MarkTagName 二选一）
	MarkTagID *int `json:"mark_tag_id,omitempty"`
	// MarkTagName 获取后将邮箱标记为此标签名称（与 MarkTagID 二选一）
	MarkTagName string `json:"mark_tag_name,omitempty"`
}

// APIGetPurchases 按标签获取已购邮箱（API 下发）
//
// 仅返回未禁用且标签 limit_type=1（可下发）的邮箱。
//
// 示例:
//
//	items, err := client.User.APIGetPurchases(ctx, &luckmail.APIGetPurchasesRequest{
//	    Count:       5,
//	    TagName:     "主力号",
//	    MarkTagName: "已使用",
//	})
//	for _, item := range items {
//	    fmt.Println(item.EmailAddress, item.Token)
//	}
func (u *UserAPI) APIGetPurchases(ctx context.Context, req *APIGetPurchasesRequest) ([]PurchaseItem, error) {
	data, err := u.client.Post(ctx, u.path("/email/purchases/api-get"), req)
	if err != nil {
		return nil, err
	}
	var items []PurchaseItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse purchases: %w", err)
	}
	return items, nil
}

// ===== 标签管理 =====

// CreateTagRequest 创建标签请求
type CreateTagRequest struct {
	// Name 标签名称（用户下唯一）
	Name string `json:"name"`
	// LimitType 限制类型：0=不下发 1=可下发
	LimitType int `json:"limit_type"`
	// Remark 备注说明（可选）
	Remark string `json:"remark,omitempty"`
}

// CreateTag 创建邮箱标签
//
// 示例:
//
//	tag, err := client.User.CreateTag(ctx, &luckmail.CreateTagRequest{
//	    Name:      "主力号",
//	    LimitType: 1,
//	    Remark:    "主力邮箱池，可下发",
//	})
//	fmt.Printf("标签 ID: %d, 名称: %s\n", tag.ID, tag.Name)
func (u *UserAPI) CreateTag(ctx context.Context, req *CreateTagRequest) (*TagItem, error) {
	data, err := u.client.Post(ctx, u.path("/email/tags"), req)
	if err != nil {
		return nil, err
	}
	var tag TagItem
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, fmt.Errorf("failed to parse tag: %w", err)
	}
	return &tag, nil
}

// GetTags 获取所有标签列表
//
// 示例:
//
//	tags, err := client.User.GetTags(ctx)
//	for _, tag := range tags {
//	    fmt.Println(tag.ID, tag.Name, tag.LimitType, tag.PurchaseCount)
//	}
func (u *UserAPI) GetTags(ctx context.Context) ([]TagItem, error) {
	data, err := u.client.Get(ctx, u.path("/email/tags"), nil)
	if err != nil {
		return nil, err
	}
	var tags []TagItem
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, fmt.Errorf("failed to parse tags: %w", err)
	}
	return tags, nil
}

// UpdateTagRequest 更新标签请求
type UpdateTagRequest struct {
	// LimitType 限制类型：0=不下发 1=可下发
	LimitType int `json:"limit_type"`
	// Name 新的标签名称（可选）
	Name string `json:"name,omitempty"`
	// Remark 备注说明（可选）
	Remark string `json:"remark,omitempty"`
}

// UpdateTag 更新标签
//
// tagIDOrName 可以是标签 ID（数字字符串）或标签名称。
//
// 示例:
//
//	err := client.User.UpdateTag(ctx, "1", &luckmail.UpdateTagRequest{LimitType: 1, Name: "备用号"})
//	err := client.User.UpdateTag(ctx, "主力号", &luckmail.UpdateTagRequest{LimitType: 0})
func (u *UserAPI) UpdateTag(ctx context.Context, tagIDOrName string, req *UpdateTagRequest) error {
	_, err := u.client.Put(ctx, u.path(fmt.Sprintf("/email/tags/%s", tagIDOrName)), req)
	return err
}

// DeleteTag 删除标签
//
// 删除后，该标签下的已购邮箱将变为无标签状态。
// tagIDOrName 可以是标签 ID（数字字符串）或标签名称。
//
// 示例:
//
//	err := client.User.DeleteTag(ctx, "1")
//	err := client.User.DeleteTag(ctx, "已使用")
func (u *UserAPI) DeleteTag(ctx context.Context, tagIDOrName string) error {
	_, err := u.client.Delete(ctx, u.path(fmt.Sprintf("/email/tags/%s", tagIDOrName)))
	return err
}
