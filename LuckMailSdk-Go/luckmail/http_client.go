package luckmail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// HTTPClient LuckMail HTTP 客户端
//
// 提供统一的请求接口，使用 API Key 鉴权。
//
// 参数说明：
//   - BaseURL: API 基础 URL，如 https://your-domain.com
//   - APIKey: API Key（必填）
//   - Timeout: 请求超时时间，默认 30s
type HTTPClient struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration

	httpClient *http.Client
}

// NewHTTPClient 创建 HTTP 客户端
func NewHTTPClient(baseURL, apiKey string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &HTTPClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// buildHeaders 构建请求头（含鉴权信息）
func (c *HTTPClient) buildHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	if c.APIKey != "" {
		headers["X-API-Key"] = c.APIKey
	}

	return headers
}

// buildURL 构建完整 URL
func (c *HTTPClient) buildURL(path string, params map[string]string) string {
	fullURL := c.BaseURL + path
	if len(params) > 0 {
		query := url.Values{}
		for k, v := range params {
			if v != "" {
				query.Set(k, v)
			}
		}
		if encoded := query.Encode(); encoded != "" {
			fullURL = fullURL + "?" + encoded
		}
	}
	return fullURL
}

// apiResponse API 响应结构
type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// parseResponse 解析响应数据
func (c *HTTPClient) parseResponse(statusCode int, body []byte) (json.RawMessage, error) {
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		// 非 JSON 响应，直接返回原始内容（如文件流）
		return body, nil
	}

	if resp.Code != 0 {
		if statusCode == 401 || resp.Code == 401 {
			return nil, NewAuthError(resp.Message)
		}
		return nil, NewAPIError(resp.Code, resp.Message, nil)
	}

	return resp.Data, nil
}

// Request 发送 HTTP 请求
func (c *HTTPClient) Request(ctx context.Context, method, path string, params map[string]string, body interface{}) (json.RawMessage, error) {
	fullURL := c.buildURL(path, params)

	var reqBody io.Reader
	if body != nil && (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, NewNetworkError(fmt.Sprintf("failed to create request: %v", err))
	}

	// 设置请求头
	for k, v := range c.buildHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, NewTimeoutError(fmt.Sprintf("request timeout: %s", path))
		}
		return nil, NewNetworkError(fmt.Sprintf("network error: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewNetworkError(fmt.Sprintf("failed to read response: %v", err))
	}

	return c.parseResponse(resp.StatusCode, respBody)
}

// Get 发送 GET 请求
func (c *HTTPClient) Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	return c.Request(ctx, http.MethodGet, path, params, nil)
}

// Post 发送 POST 请求
func (c *HTTPClient) Post(ctx context.Context, path string, body interface{}) (json.RawMessage, error) {
	return c.Request(ctx, http.MethodPost, path, nil, body)
}

// Put 发送 PUT 请求
func (c *HTTPClient) Put(ctx context.Context, path string, body interface{}) (json.RawMessage, error) {
	return c.Request(ctx, http.MethodPut, path, nil, body)
}

// Delete 发送 DELETE 请求
func (c *HTTPClient) Delete(ctx context.Context, path string) (json.RawMessage, error) {
	return c.Request(ctx, http.MethodDelete, path, nil, nil)
}

// GetStream 获取流式响应（文件下载等），返回原始字节
func (c *HTTPClient) GetStream(ctx context.Context, path string, params map[string]string) ([]byte, error) {
	fullURL := c.buildURL(path, params)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, NewNetworkError(fmt.Sprintf("failed to create request: %v", err))
	}

	// 流式请求不设置 Accept: application/json
	headers := c.buildHeaders()
	delete(headers, "Accept")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, NewTimeoutError(fmt.Sprintf("request timeout: %s", path))
		}
		return nil, NewNetworkError(fmt.Sprintf("network error: %v", err))
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
