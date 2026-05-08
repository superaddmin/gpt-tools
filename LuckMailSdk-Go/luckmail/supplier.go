package luckmail

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

const supplierAPIPrefix = "/api/v1/openapi/supplier"

// SupplierAPI 供应商端 API 接口集合
//
// 所有方法通过 context 控制超时和取消。
//
// 示例:
//
//	ctx := context.Background()
//	profile, err := client.Supplier.GetProfile(ctx)
//	fmt.Println(profile.Username, profile.Balance)
type SupplierAPI struct {
	client *HTTPClient
}

// NewSupplierAPI 创建供应商端 API 实例
func NewSupplierAPI(client *HTTPClient) *SupplierAPI {
	return &SupplierAPI{client: client}
}

func (s *SupplierAPI) path(p string) string {
	return supplierAPIPrefix + p
}

// ===== 供应商信息 =====

// GetProfile 获取供应商个人信息
//
// 示例:
//
//	profile, err := client.Supplier.GetProfile(ctx)
//	fmt.Println(profile.Username, profile.Balance)
func (s *SupplierAPI) GetProfile(ctx context.Context) (*SupplierProfile, error) {
	data, err := s.client.Get(ctx, s.path("/profile"), nil)
	if err != nil {
		return nil, err
	}
	var profile SupplierProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse supplier profile: %w", err)
	}
	return &profile, nil
}

// ===== 邮箱管理 =====

// GetSupplierEmailsParams 获取邮箱列表参数
type GetSupplierEmailsParams struct {
	Page     int
	PageSize int
	Keyword  string
	// EmailType 邮箱类型：ms_graph / ms_imap / google_variant / self_built
	EmailType string
	// IsShortTerm 仅微软邮箱有效：0=长效 1=短效，传 -1 表示不过滤
	IsShortTerm int
	// Status 状态：1=正常 2=异常 4=禁用，传 0 表示不过滤
	Status int
}

// GetSupplierEmailsResult 获取邮箱列表结果
type GetSupplierEmailsResult struct {
	List     []SupplierEmailItem
	Total    int
	Page     int
	PageSize int
}

// GetEmails 获取邮箱列表（分页）
//
// 示例:
//
//	result, err := client.Supplier.GetEmails(ctx, &luckmail.GetSupplierEmailsParams{
//	    EmailType:   "ms_graph",
//	    IsShortTerm: 0,
//	})
//	fmt.Printf("长效 MS Graph 邮箱: %d 个\n", result.Total)
func (s *SupplierAPI) GetEmails(ctx context.Context, params *GetSupplierEmailsParams) (*GetSupplierEmailsResult, error) {
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
		if params.EmailType != "" {
			p["type"] = params.EmailType
		}
		if params.IsShortTerm >= 0 {
			p["is_short_term"] = strconv.Itoa(params.IsShortTerm)
		}
		if params.Status > 0 {
			p["status"] = strconv.Itoa(params.Status)
		}
	}

	data, err := s.client.Get(ctx, s.path("/emails"), p)
	if err != nil {
		return nil, err
	}

	var page struct {
		List     []SupplierEmailItem `json:"list"`
		Total    int                 `json:"total"`
		Page     int                 `json:"page"`
		PageSize int                 `json:"page_size"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("failed to parse supplier emails: %w", err)
	}
	return &GetSupplierEmailsResult{
		List:     page.List,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

// ImportSupplierEmailsRequest 导入邮箱请求（供应商端）
type ImportSupplierEmailsRequest struct {
	// Type 邮箱类型：microsoft / ms_graph / ms_imap / google_variant / self_built
	Type string `json:"type"`
	// IsShortTerm 仅微软邮箱有效：0=长效（默认）1=短效
	IsShortTerm int `json:"is_short_term"`
	// Emails 邮箱列表
	Emails []map[string]interface{} `json:"emails"`
}

// ImportEmails 批量导入邮箱到供应商资源池
//
// 示例:
//
//	result, err := client.Supplier.ImportEmails(ctx, &luckmail.ImportSupplierEmailsRequest{
//	    Type:        "ms_graph",
//	    IsShortTerm: 0,
//	    Emails: []map[string]interface{}{
//	        {
//	            "address":       "user1@outlook.com",
//	            "client_id":     "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
//	            "refresh_token": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
//	        },
//	    },
//	})
//	fmt.Printf("成功: %d, 重复: %d\n", result.Success, result.Duplicate)
func (s *SupplierAPI) ImportEmails(ctx context.Context, req *ImportSupplierEmailsRequest) (*ImportResult, error) {
	data, err := s.client.Post(ctx, s.path("/emails/import"), req)
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
// 示例:
//
//	content, err := client.Supplier.ExportEmails(ctx, "", "ms_graph", -1, 0)
//	if err == nil {
//	    os.WriteFile("emails.txt", content, 0644)
//	}
func (s *SupplierAPI) ExportEmails(ctx context.Context, keyword, emailType string, isShortTerm, status int) ([]byte, error) {
	params := map[string]string{}
	if keyword != "" {
		params["keyword"] = keyword
	}
	if emailType != "" {
		params["type"] = emailType
	}
	if isShortTerm >= 0 {
		params["is_short_term"] = strconv.Itoa(isShortTerm)
	}
	if status > 0 {
		params["status"] = strconv.Itoa(status)
	}
	return s.client.GetStream(ctx, s.path("/emails/export"), params)
}

// ===== 申述管理 =====

// GetAppealsParams 获取申述列表参数
type GetAppealsParams struct {
	Page     int
	PageSize int
	// Status 申述状态：1=待处理 2=已同意 3=待仲裁 4=已拒绝
	Status int
	// AppealType 申述类型过滤
	AppealType int
}

// GetAppealsResult 获取申述列表结果
type GetAppealsResult struct {
	List     []AppealItem
	Total    int
	Page     int
	PageSize int
}

// GetAppeals 获取申述列表（分页）
//
// 示例:
//
//	result, err := client.Supplier.GetAppeals(ctx, &luckmail.GetAppealsParams{Status: 1})
//	fmt.Printf("待处理申述: %d 个\n", result.Total)
func (s *SupplierAPI) GetAppeals(ctx context.Context, params *GetAppealsParams) (*GetAppealsResult, error) {
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
		if params.AppealType > 0 {
			p["type"] = strconv.Itoa(params.AppealType)
		}
	}

	data, err := s.client.Get(ctx, s.path("/appeals"), p)
	if err != nil {
		return nil, err
	}

	var page struct {
		List     []AppealItem `json:"list"`
		Total    int          `json:"total"`
		Page     int          `json:"page"`
		PageSize int          `json:"page_size"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("failed to parse appeals: %w", err)
	}
	return &GetAppealsResult{
		List:     page.List,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

// GetAppeal 获取申述详情
//
// 示例:
//
//	detail, err := client.Supplier.GetAppeal(ctx, "APL20240310001")
//	fmt.Println(detail.Reason, detail.Status)
func (s *SupplierAPI) GetAppeal(ctx context.Context, appealNo string) (*AppealDetail, error) {
	data, err := s.client.Get(ctx, s.path(fmt.Sprintf("/appeal/%s", appealNo)), nil)
	if err != nil {
		return nil, err
	}
	var detail AppealDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("failed to parse appeal detail: %w", err)
	}
	return &detail, nil
}

// ReplyAppeal 处理申述（回复）
//
// 参数:
//   - appealNo: 申述单号
//   - result: 处理结果：1=同意退款 2=拒绝申述 3=申请仲裁
//   - reply: 回复内容说明
//
// 示例:
//
//	// 同意退款
//	err := client.Supplier.ReplyAppeal(ctx, "APL20240310001", 1, "邮箱确有问题，同意退款")
//
//	// 拒绝申述
//	err := client.Supplier.ReplyAppeal(ctx, "APL20240310001", 2, "邮箱状态正常，拒绝申述")
func (s *SupplierAPI) ReplyAppeal(ctx context.Context, appealNo string, result int, reply string) error {
	body := map[string]interface{}{
		"result": result,
		"reply":  reply,
	}
	_, err := s.client.Post(ctx, s.path(fmt.Sprintf("/appeal/%s/reply", appealNo)), body)
	return err
}

// BatchReplyAppealsRequest 批量处理申述请求
type BatchReplyAppealsRequest struct {
	// AppealNos 申述单号列表（最多 100 条）
	AppealNos []string `json:"appeal_nos"`
	// Result 处理结果：1=同意退款 2=拒绝申述 3=申请仲裁
	Result int `json:"result"`
	// Reply 回复内容说明
	Reply string `json:"reply"`
}

// BatchReplyAppeals 批量处理申述
//
// 示例:
//
//	result, err := client.Supplier.BatchReplyAppeals(ctx, &luckmail.BatchReplyAppealsRequest{
//	    AppealNos: []string{"APL001", "APL002", "APL003"},
//	    Result:    2,
//	    Reply:     "经验证邮箱正常，拒绝申述",
//	})
//	fmt.Printf("成功处理: %d\n", result.Success)
func (s *SupplierAPI) BatchReplyAppeals(ctx context.Context, req *BatchReplyAppealsRequest) (*BatchReplyResult, error) {
	data, err := s.client.Post(ctx, s.path("/appeals/batch-reply"), req)
	if err != nil {
		return nil, err
	}
	var result BatchReplyResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse batch reply result: %w", err)
	}
	return &result, nil
}

// ===== 数据看板 =====

// GetDashboard 获取数据看板总览
//
// 示例:
//
//	summary, err := client.Supplier.GetDashboard(ctx)
//	fmt.Printf("总邮箱: %d\n", summary.TotalEmails)
//	fmt.Printf("今日佣金: %s\n", summary.TodayCommission)
//	fmt.Printf("成功率: %.2f%%\n", summary.SuccessRate)
func (s *SupplierAPI) GetDashboard(ctx context.Context) (*DashboardSummary, error) {
	data, err := s.client.Get(ctx, s.path("/dashboard/summary"), nil)
	if err != nil {
		return nil, err
	}
	var summary DashboardSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard summary: %w", err)
	}
	return &summary, nil
}
