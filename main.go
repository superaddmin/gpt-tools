package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type appConfig struct {
	CheckoutEndpoint             string   `json:"checkout_endpoint"`
	CheckoutApproveEndpoint      string   `json:"checkout_approve_endpoint"`
	CheckoutCookie               string   `json:"checkout_cookie"`
	CheckoutUserAgent            string   `json:"checkout_user_agent"`
	AuditCaptureSensitive        bool     `json:"audit_capture_sensitive"`
	LocalMockBaseURL             string   `json:"local_mock_base_url"`
	MidtransMockBaseURL          string   `json:"midtrans_mock_base_url"`
	GopayGWAMockBaseURL          string   `json:"gopay_gwa_mock_base_url"`
	GopayCustomerMockBaseURL     string   `json:"gopay_customer_mock_base_url"`
	MidtransLinkingAuthorization string   `json:"midtrans_linking_authorization"`
	MidtransLinkingCookie        string   `json:"midtrans_linking_cookie"`
	MidtransChargeCookie         string   `json:"midtrans_charge_cookie"`
	ProxyTestURLs                []string `json:"proxy_test_urls"`
}

var config appConfig

//go:embed web/*
var embeddedWebFiles embed.FS

//go:embed config.json
var embeddedConfigFile []byte

type checkoutRequest struct {
	Token           string          `json:"token"`
	EntryPoint      string          `json:"entry_point"`
	PlanName        string          `json:"plan_name"`
	BillingDetails  billingDetails  `json:"billing_details"`
	PromoCampaign   promoCampaign   `json:"promo_campaign"`
	CheckoutUIMode  string          `json:"checkout_ui_mode"`
	Proxy           proxySettings   `json:"proxy"`
	TaxRegion       taxRegion       `json:"tax_region"`
	CustomerEmail   string          `json:"customer_email"`
	CheckoutSession checkoutSession `json:"checkout_session"`
	GopayLink       gopayLink       `json:"gopay_link"`
}

type billingDetails struct {
	Country  string `json:"country"`
	Currency string `json:"currency"`
}

type promoCampaign struct {
	PromoCampaignID        string `json:"promo_campaign_id"`
	IsCouponFromQueryParam bool   `json:"is_coupon_from_query_param"`
}

type proxySettings struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type taxRegion struct {
	Country    string `json:"country"`
	Line1      string `json:"line1"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
	State      string `json:"state"`
}

type checkoutSession struct {
	Cookie    string `json:"cookie"`
	UserAgent string `json:"user_agent"`
}

type stripeClientContext struct {
	GUID              string
	MUID              string
	SID               string
	ClientSessionID   string
	ElementsSessionID string
}

type proxyTestRequest struct {
	Proxy     proxySettings `json:"proxy"`
	TargetURL string        `json:"target_url"`
}

type checkoutApproveRequest struct {
	CheckoutSessionID   string `json:"checkout_session_id"`
	ProcessorEntity     string `json:"processor_entity"`
	PaymentMethodID     string `json:"payment_method_id,omitempty"`
	SubmissionAttemptID string `json:"submission_attempt_id,omitempty"`
}

type gopayLink struct {
	Type        string `json:"type"`
	CountryCode string `json:"country_code"`
	PhoneNumber string `json:"phone_number"`
	OTPChannel  string `json:"otp_channel"`
	OTP         string `json:"otp"`
	PIN         string `json:"pin"`
}

type gopayAutoTriggerCheckRequest struct {
	Source      string `json:"source"`
	CheckoutURL string `json:"checkout_url"`
	PageText    string `json:"page_text"`
	Submitted   bool   `json:"submitted"`
}

type gopayAutoTriggerDecision struct {
	OK               bool           `json:"ok"`
	Stage            string         `json:"stage"`
	Ready            bool           `json:"ready"`
	AlreadyTriggered bool           `json:"already_triggered"`
	CheckoutKey      string         `json:"checkout_key,omitempty"`
	Reason           string         `json:"reason"`
	Source           string         `json:"source,omitempty"`
	Conditions       map[string]any `json:"conditions"`
}

type gopayOTPRequest struct {
	ReferenceID string `json:"reference_id"`
	OTPChannel  string `json:"otp_channel"`
	OTP         string `json:"otp"`
}

type gopayPINRequest struct {
	ReferenceID string `json:"reference_id"`
	ChallengeID string `json:"challenge_id"`
	GopayGUID   string `json:"gopay_guid"`
	PIN         string `json:"pin"`
}

type proxyTestAttempt struct {
	TargetURL string `json:"target_url"`
	Status    int    `json:"status,omitempty"`
	Body      string `json:"body,omitempty"`
	Error     string `json:"error,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type proxyTestResponse struct {
	OK        bool               `json:"ok"`
	TargetURL string             `json:"target_url"`
	Proxy     proxySummary       `json:"proxy"`
	Status    int                `json:"status"`
	Body      string             `json:"body"`
	ElapsedMS int64              `json:"elapsed_ms"`
	Attempts  []proxyTestAttempt `json:"attempts,omitempty"`
}

type checkoutResponse struct {
	Tag                       string         `json:"tag"`
	CheckoutSessionID         string         `json:"checkout_session_id"`
	PublishableKey            string         `json:"publishable_key"`
	ProcessorEntity           string         `json:"processor_entity"`
	CheckoutUIMode            string         `json:"checkout_ui_mode"`
	AutomaticTaxEnabled       bool           `json:"automatic_tax_enabled"`
	ScheduledDiscountPreview  any            `json:"scheduled_discount_preview"`
	ImmediateDiscountSettings any            `json:"immediate_discount_settings"`
	PromoCampaign             promoCampaign  `json:"promo_campaign"`
	PromoCreditGrant          any            `json:"promo_credit_grant"`
	PlanName                  string         `json:"plan_name"`
	RequiresManualApproval    bool           `json:"requires_manual_approval"`
	BillingDetails            billingDetails `json:"billing_details"`
	URL                       any            `json:"url"`
	ClientSecret              string         `json:"client_secret"`
	Status                    string         `json:"status"`
	PaymentStatus             string         `json:"payment_status"`
	CheckoutProvider          string         `json:"checkout_provider"`
	CustomerSessionSecret     any            `json:"customer_session_client_secret"`
	PaymentMethodTypes        any            `json:"payment_method_types"`
	CustomPaymentMethods      any            `json:"custom_payment_methods"`
	ConfirmReturnURL          any            `json:"confirm_return_url"`
	CheckoutState             any            `json:"checkout_state"`
	ReceivedTokenSummary      tokenSummary   `json:"received_token_summary"`
	Proxy                     proxySummary   `json:"proxy"`
	Mode                      string         `json:"mode"`
	Note                      string         `json:"note"`
	CreatedAt                 string         `json:"created_at"`
}

type tokenSummary struct {
	Present bool   `json:"present"`
	Prefix  string `json:"prefix,omitempty"`
	Suffix  string `json:"suffix,omitempty"`
	Length  int    `json:"length"`
}

type proxySummary struct {
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"`
	URL     string `json:"url,omitempty"`
}

type stripeInitResult struct {
	OK                bool   `json:"ok"`
	Skipped           bool   `json:"skipped,omitempty"`
	Error             string `json:"error,omitempty"`
	Stage             string `json:"stage,omitempty"`
	Method            string `json:"method,omitempty"`
	Endpoint          string `json:"endpoint,omitempty"`
	Status            int    `json:"status,omitempty"`
	ContentType       string `json:"content_type,omitempty"`
	Body              any    `json:"body,omitempty"`
	RawBody           string `json:"raw_body,omitempty"`
	CheckoutSessionID string `json:"checkout_session_id,omitempty"`
	PublishableKey    string `json:"publishable_key,omitempty"`
	ElapsedMS         int64  `json:"elapsed_ms,omitempty"`
}

type stripeInitBodyCheck struct {
	OK    bool
	Total int64
	Error string
}

type stripeConfirmCheck struct {
	OK    bool
	Error string
}

type stripeRedirectCheck struct {
	OK          bool
	Error       string
	Source      string
	RedirectURL string
}

type gopayRedirectResult struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	Status      int    `json:"status,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
	Location    string `json:"location,omitempty"`
	GUID        string `json:"guid,omitempty"`
}

type gopayLinkingResolution struct {
	LinkResult        map[string]any
	ReusedExisting    bool
	AccountResult     map[string]any
	ConflictReason    string
	LinkError         string
	LinkHTTPStatus    int
	LinkErrorMessage  []string
	TokenNotFound     bool
	AccountSource     string
	AccountOriginURL  string
	AccountSourceNote string
}

type gopayPaymentResolution struct {
	Charge             map[string]any
	PaymentReferenceID string
	PaymentValidate    map[string]any
	PaymentConfirm     map[string]any
	PaymentChallengeID string
	PaymentClientID    string
	PaymentPINToken    map[string]any
	PaymentProcess     map[string]any
	TransactionID      string
	MidtransStatus     map[string]any
	PinUsed            string
	PinTried           []string
}

type gopayFullLinkPaymentDeps struct {
	completePayment func(context.Context, string, string) (gopayPaymentResolution, error)
}

type stripeSnapAccountFlowDeps struct {
	init                func(context.Context, *http.Client, any) stripeInitResult
	update              func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult
	createPaymentMethod func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult
	confirm             func(context.Context, *http.Client, any, string, string, string, stripeClientContext) stripeInitResult
	approve             func(context.Context, *http.Client, string, checkoutSession, checkoutApproveRequest) (map[string]any, error)
	waitForRedirect     func(context.Context, int, time.Duration, func() stripeInitResult) (stripeInitResult, stripeRedirectCheck, []map[string]any)
	redirect            func(context.Context, *http.Client, string) gopayRedirectResult
}

type stripeSnapAccountFlowResult struct {
	AccountID        string
	AccountSource    string
	AccountOriginURL string
	Init             stripeInitResult
	InitCheck        stripeInitBodyCheck
	Update           stripeInitResult
	PaymentMethod    stripeInitResult
	Confirm          stripeInitResult
	Approval         map[string]any
	TermsInteraction map[string]any
	ConfirmCheck     stripeConfirmCheck
	Details          stripeInitResult
	RedirectCheck    stripeRedirectCheck
	Redirect         gopayRedirectResult
	Attempts         []map[string]any
}

type auditResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseBody bytes.Buffer
}

func (w *auditResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	if w.responseBody.Len() < auditMaxResponseBodyBytes {
		remaining := auditMaxResponseBodyBytes - w.responseBody.Len()
		if len(data) > remaining {
			w.responseBody.Write(data[:remaining])
		} else {
			w.responseBody.Write(data)
		}
	}
	return w.ResponseWriter.Write(data)
}

func (w *auditResponseWriter) Flush() {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *auditResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func newAuditLogger(baseDir string, archiveAfter time.Duration, maxFileSize int64, queueSize int) *auditLogger {
	logger := &auditLogger{
		baseDir:       baseDir,
		archiveAfter:  archiveAfter,
		maxFileSize:   maxFileSize,
		queue:         make(chan auditLogEnvelope, queueSize),
		fileCache:     map[string]*os.File{},
		fileOpenedAt:  map[string]time.Time{},
		fileCreatedAt: map[string]time.Time{},
		fileSizes:     map[string]int64{},
	}
	go logger.run()
	return logger
}

func (l *auditLogger) run() {
	for item := range l.queue {
		l.write(item)
	}
}

func (l *auditLogger) Log(record auditLogRecord) {
	if l == nil {
		return
	}
	timestamp, err := time.Parse(time.RFC3339Nano, record.OperationTime)
	if err != nil {
		timestamp = time.Now()
		record.OperationTime = timestamp.Format(time.RFC3339Nano)
	}
	envelope := auditLogEnvelope{
		Email:     normalizeAuditEmail(record.AccountEmail),
		Timestamp: timestamp,
		Record:    record,
	}
	select {
	case l.queue <- envelope:
	default:
		go l.write(envelope)
	}
}

func (l *auditLogger) write(item auditLogEnvelope) {
	if err := os.MkdirAll(l.baseDir, 0o755); err != nil {
		log.Printf("create audit log dir failed: %v", err)
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	filePath, fileHandle, err := l.fileFor(item)
	if err != nil {
		log.Printf("open audit log file failed: %v", err)
		return
	}

	payload, err := json.Marshal(item.Record)
	if err != nil {
		log.Printf("marshal audit log failed: %v", err)
		return
	}
	payload = append(payload, '\n')
	written, err := fileHandle.Write(payload)
	if err != nil {
		log.Printf("write audit log failed: %v", err)
		return
	}
	l.fileSizes[filePath] += int64(written)
	l.fileOpenedAt[filePath] = time.Now()
}

func (l *auditLogger) fileFor(item auditLogEnvelope) (string, *os.File, error) {
	targetPath := l.targetPath(item.Email, item.Timestamp)
	if fileHandle, ok := l.fileCache[targetPath]; ok && !l.shouldRotate(targetPath, item.Timestamp) {
		return targetPath, fileHandle, nil
	}
	if err := l.rotateOthers(item.Email, item.Timestamp); err != nil {
		return "", nil, err
	}
	if fileHandle, ok := l.fileCache[targetPath]; ok && !l.shouldRotate(targetPath, item.Timestamp) {
		return targetPath, fileHandle, nil
	}
	fileHandle, err := os.OpenFile(targetPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", nil, err
	}
	fileInfo, err := fileHandle.Stat()
	if err != nil {
		fileHandle.Close()
		return "", nil, err
	}
	l.fileCache[targetPath] = fileHandle
	l.fileOpenedAt[targetPath] = time.Now()
	l.fileCreatedAt[targetPath] = item.Timestamp
	l.fileSizes[targetPath] = fileInfo.Size()
	return targetPath, fileHandle, nil
}

func (l *auditLogger) rotateOthers(email string, now time.Time) error {
	prefix := sanitizeAuditFileName(email) + "_"
	for path, fileHandle := range l.fileCache {
		if !strings.Contains(filepath.Base(path), prefix) {
			continue
		}
		if l.shouldRotate(path, now) {
			if err := fileHandle.Close(); err != nil {
				return err
			}
			delete(l.fileCache, path)
			delete(l.fileOpenedAt, path)
			delete(l.fileCreatedAt, path)
			delete(l.fileSizes, path)
		}
	}
	return nil
}

func (l *auditLogger) shouldRotate(path string, now time.Time) bool {
	createdAt, ok := l.fileCreatedAt[path]
	if ok && l.archiveAfter > 0 && now.Sub(createdAt) >= l.archiveAfter {
		return true
	}
	if l.maxFileSize > 0 && l.fileSizes[path] >= l.maxFileSize {
		return true
	}
	return false
}

func (l *auditLogger) targetPath(email string, ts time.Time) string {
	name := fmt.Sprintf("%s_%s.log", sanitizeAuditFileName(email), ts.Format("20060102150405"))
	return filepath.Join(l.baseDir, name)
}

func normalizeAuditEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return "unknown_account"
	}
	return email
}

func sanitizeAuditFileName(email string) string {
	safe := normalizeAuditEmail(email)
	safe = auditFileNameSanitizer.Replace(safe)
	safe = strings.Trim(safe, "._")
	if safe == "" {
		return "unknown_account"
	}
	return safe
}

func auditOperationName(path string) string {
	if name := operationDisplayNames[path]; name != "" {
		return name
	}
	return path
}

func auditOperationType(path string) string {
	if kind := operationTypes[path]; kind != "" {
		return kind
	}
	return "request"
}

func extractClientIP(r *http.Request) string {
	forwardedFor := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if forwardedFor != "" {
		return forwardedFor
	}
	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func auditLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		bodyBytes, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if readErr != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		auditWriter := &auditResponseWriter{ResponseWriter: w, statusCode: http.StatusOK, responseBody: bytes.Buffer{}}
		startedAt := time.Now()
		next.ServeHTTP(auditWriter, r)

		record := buildAuditLogRecord(r, bodyBytes, auditWriter.responseBody.Bytes(), auditWriter.statusCode, startedAt)
		flowAuditLogger.Log(record)
	})
}

func buildAuditLogRecord(r *http.Request, body []byte, responseBody []byte, statusCode int, startedAt time.Time) auditLogRecord {
	captureSensitive := auditCaptureSensitive()
	detail := auditBodyMap(body, captureSensitive)
	email := extractAuditEmail(r, detail)
	responsePayload := auditBodyMap(responseBody, captureSensitive)
	responseSummary := auditResponseSummary(responsePayload)
	result := "success"
	businessFailed, businessError := auditBusinessFailure(responseSummary)
	if statusCode >= 400 || businessFailed {
		result = "failed"
	}
	metadata := map[string]any{
		"analysis_log_version":         auditAnalysisLogVersion,
		"duration_ms":                  time.Since(startedAt).Milliseconds(),
		"request_body_bytes":           len(body),
		"captured_response_body_bytes": len(responseBody),
		"account_log_file_prefix":      sanitizeAuditFileName(email),
		"flow_stage":                   auditFlowStage(r.URL.Path),
		"flow_step_index":              auditFlowStepIndex(r.URL.Path),
		"flow_route_role":              auditFlowRouteRole(r.URL.Path),
		"request_host":                 r.Host,
		"request_user_agent":           sanitizeAuditValue(r.UserAgent()),
	}
	if len(detail) > 0 {
		metadata["request_payload"] = detail
	}
	if len(responseSummary) > 0 {
		metadata["response_summary"] = responseSummary
	}
	if operationFlow := auditOperationFlow(responsePayload); len(operationFlow) > 0 {
		metadata["operation_flow"] = operationFlow
	}
	if paymentVoucher := auditPaymentVoucher(responsePayload); len(paymentVoucher) > 0 {
		metadata["payment_voucher"] = paymentVoucher
	}
	if identifiers := auditCorrelationIdentifiers(detail, responseSummary, responsePayload); len(identifiers) > 0 {
		metadata["correlation_identifiers"] = identifiers
	}
	if captureSensitive {
		metadata["sensitive_capture"] = true
		if len(responsePayload) > 0 {
			metadata["response_payload"] = responsePayload
		}
	}
	record := auditLogRecord{
		OperationTime:   startedAt.Format(time.RFC3339Nano),
		OperationType:   auditOperationType(r.URL.Path),
		OperationName:   auditOperationName(r.URL.Path),
		OperationDetail: detail,
		OperationResult: result,
		StatusCode:      statusCode,
		AccountEmail:    email,
		IPAddress:       extractClientIP(r),
		RequestPath:     r.URL.Path,
		RequestMethod:   r.Method,
		Summary:         fmt.Sprintf("%s %s => %d", r.Method, r.URL.Path, statusCode),
		Metadata:        metadata,
	}
	if statusCode >= 400 {
		record.ErrorMessage = http.StatusText(statusCode)
	} else if businessFailed {
		record.ErrorMessage = businessError
	}
	return record
}

func auditResponseSummary(parsed map[string]any) map[string]any {
	if len(parsed) == 0 {
		return nil
	}
	summary := make(map[string]any)
	for key, value := range parsed {
		if _, ok := auditResponseSummaryKeys[key]; !ok {
			continue
		}
		summary[key] = value
	}
	return summary
}

func auditBusinessFailure(summary map[string]any) (bool, string) {
	if len(summary) == 0 {
		return false, ""
	}
	if okValue, exists := summary["ok"]; exists {
		if okBool, ok := okValue.(bool); ok && !okBool {
			stage := stringifyJSONValue(summary["stage"])
			errText := stringifyJSONValue(summary["error"])
			return true, strings.TrimSpace(firstNonEmpty(stage, errText, "business response failed"))
		}
	}
	return false, ""
}

// auditFlowStage 将 API 路由归一化为支付链路分析阶段，方便按账号日志复盘全流程。
func auditFlowStage(path string) string {
	if stage := auditFlowStages[path]; stage != "" {
		return stage
	}
	return "other"
}

// auditFlowStepIndex 为支付链路阶段提供稳定排序索引，方便后续日志分析工具还原执行顺序。
func auditFlowStepIndex(path string) int {
	if index, ok := auditFlowStepIndexes[path]; ok {
		return index
	}
	return 900
}

// auditFlowRouteRole 说明当前接口在 checkout 到 GoPay 支付全流程中的职责边界。
func auditFlowRouteRole(path string) string {
	if role := auditFlowRouteRoles[path]; role != "" {
		return role
	}
	return "通用 API 请求记录"
}

// auditCorrelationIdentifiers 从已脱敏载荷中提取可关联的非密钥字段，用于跨接口追踪同一支付链路。
func auditCorrelationIdentifiers(sources ...map[string]any) map[string]any {
	identifiers := make(map[string]any)
	for _, source := range sources {
		copyAuditIdentifier(identifiers, source, "checkout_session_id")
		copyAuditIdentifier(identifiers, source, "checkout_key")
		copyAuditIdentifier(identifiers, source, "target_url")
		copyAuditIdentifier(identifiers, source, "account_id")
		copyAuditIdentifier(identifiers, source, "reference_id")
		copyAuditIdentifier(identifiers, source, "linking_reference_id")
		copyAuditIdentifier(identifiers, source, "payment_reference_id")
		copyAuditIdentifier(identifiers, source, "gopay_payment_reference_id")
		copyAuditIdentifier(identifiers, source, "transaction_id")
	}
	if len(identifiers) == 0 {
		return nil
	}
	return identifiers
}

// copyAuditIdentifier 将安全标识字段复制到关联索引中，同时跳过空值和已存在字段。
func copyAuditIdentifier(target map[string]any, source map[string]any, key string) {
	if len(source) == 0 {
		return
	}
	if _, exists := target[key]; exists {
		return
	}
	value, exists := source[key]
	if !exists || stringifyJSONValue(value) == "" {
		return
	}
	target[key] = value
}

func sanitizeAuditBody(body []byte) map[string]any {
	return auditBodyMap(body, false)
}

func auditBodyMap(body []byte, captureSensitive bool) map[string]any {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return map[string]any{"raw": sanitizeAuditValue(string(trimmed))}
	}
	sanitized := sanitizeAuditValue(parsed)
	if m, ok := sanitized.(map[string]any); ok {
		return m
	}
	return map[string]any{"value": sanitized}
}

func auditOperationFlow(response map[string]any) map[string]any {
	if len(response) == 0 {
		return nil
	}
	keys := []string{
		"ok",
		"stage",
		"ready",
		"reason",
		"strategy",
		"error",
		"code",
		"account_id",
		"account_source",
		"account_origin_url",
		"target_url",
		"country_code",
		"phone_number",
		"reference_id",
		"linking_reference_id",
		"payment_reference_id",
		"gopay_payment_reference_id",
		"transaction_id",
		"reused_existing",
		"already_triggered",
		"checkout_key",
		"conditions",
		"conflict_reason",
		"request",
		"diagnostics",
		"linking_diagnostics",
		"summary",
		"stages",
		"browser_result",
		"network_diagnostics",
		"network_debug_artifact",
		"network_debug_error",
		"aggressive_retry",
		"pin_stage",
		"pin_auto_filled",
		"pin_auto_submitted",
		"pin_input_strategy",
		"balance_amount",
		"balance_state",
		"hubungkan_auto_clicked",
		"pay_now_auto_clicked",
		"auto_action_paused",
		"auto_action_stage",
		"otp_manual_required",
		"has_otp_field",
		"has_pin_field",
		"cdp_url_host",
		"cdp_url_path",
	}
	flow := make(map[string]any)
	for _, key := range keys {
		if value, ok := response[key]; ok {
			flow[key] = value
		}
	}
	return flow
}

func auditPaymentVoucher(response map[string]any) map[string]any {
	if len(response) == 0 {
		return nil
	}
	if voucher, ok := response["payment_voucher"].(map[string]any); ok && len(voucher) > 0 {
		return voucher
	}
	voucher := make(map[string]any)
	copyVoucherValue := func(key string, value any) {
		if value != nil && stringifyJSONValue(value) != "" {
			voucher[key] = value
		}
	}
	copyVoucherValue("payment_reference_id", firstNonNil(response["payment_reference_id"], response["gopay_payment_reference_id"], response["reference_id"]))
	copyVoucherValue("gopay_payment_reference_id", response["gopay_payment_reference_id"])
	copyVoucherValue("transaction_id", response["transaction_id"])
	copyPaymentVoucherFields(voucher, response["gopay_charge"])
	copyPaymentVoucherFields(voucher, response["midtrans_status"])
	if len(voucher) == 0 {
		return nil
	}
	return voucher
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil && stringifyJSONValue(value) != "" {
			return value
		}
	}
	return nil
}

func copyPaymentVoucherFields(voucher map[string]any, source any) {
	fields, ok := source.(map[string]any)
	if !ok || len(fields) == 0 {
		return
	}
	for _, key := range paymentVoucherFieldKeys {
		if value, exists := fields[key]; exists && stringifyJSONValue(value) != "" {
			voucher[key] = value
		}
	}
}

func sanitizeAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		cleaned := make(map[string]any, len(typed))
		for _, key := range keys {
			if _, blocked := sensitiveJSONKeys[key]; blocked {
				cleaned[key] = maskSensitiveValue(typed[key])
				continue
			}
			cleaned[key] = sanitizeAuditValue(typed[key])
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(typed))
		for _, item := range typed {
			cleaned = append(cleaned, sanitizeAuditValue(item))
		}
		return cleaned
	case string:
		if looksSensitiveString(typed) {
			return maskSensitiveString(typed)
		}
		return typed
	default:
		return typed
	}
}

func maskSensitiveValue(value any) any {
	switch typed := value.(type) {
	case string:
		return maskSensitiveString(typed)
	default:
		return maskedAuditValue
	}
}

func maskSensitiveString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return maskedAuditValue
	}
	return value[:4] + maskedAuditValue + value[len(value)-4:]
}

func looksSensitiveString(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "eyJ") {
		return true
	}
	return len(trimmed) > 40 && strings.Count(trimmed, ".") >= 2
}

func auditCaptureSensitive() bool {
	return configBool("AUDIT_CAPTURE_SENSITIVE", config.AuditCaptureSensitive)
}

func extractAuditEmail(r *http.Request, detail map[string]any) string {
	if email := strings.TrimSpace(r.Header.Get("X-Account-Email")); email != "" {
		return normalizeAuditEmail(email)
	}
	if email := auditEmailFromMap(detail); email != "" {
		return email
	}
	return "unknown_account"
}

func auditEmailFromMap(detail map[string]any) string {
	if detail == nil {
		return ""
	}
	priorityKeys := []string{"customer_email", "email", "account_email", "user_email"}
	for _, key := range priorityKeys {
		if value, ok := detail[key].(string); ok && auditEmailPattern.MatchString(value) {
			return normalizeAuditEmail(value)
		}
	}
	for _, value := range detail {
		switch typed := value.(type) {
		case string:
			if auditEmailPattern.MatchString(typed) {
				return normalizeAuditEmail(auditEmailPattern.FindString(typed))
			}
		case map[string]any:
			if nested := auditEmailFromMap(typed); nested != "" {
				return nested
			}
		case []any:
			for _, item := range typed {
				if nestedMap, ok := item.(map[string]any); ok {
					if nested := auditEmailFromMap(nestedMap); nested != "" {
						return nested
					}
				}
				if text, ok := item.(string); ok && auditEmailPattern.MatchString(text) {
					return normalizeAuditEmail(auditEmailPattern.FindString(text))
				}
			}
		}
	}
	return ""
}

func withAuditLogging(next http.Handler) http.Handler {
	return auditLogMiddleware(next)
}

func main() {
	config = loadAppConfig()

	staticFiles, err := fs.Sub(embeddedWebFiles, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/checkout", handleCheckout)
	mux.HandleFunc("/api/checkout/start", handleCheckout)
	mux.HandleFunc("/api/incognito/open", handleIncognitoOpen)
	mux.HandleFunc("/api/session/fetch", handleSessionFetch)
	mux.HandleFunc("/api/gopay/force-link", handleGopayForceLink)
	mux.HandleFunc("/api/gopay/auto-link", handleGopayAutoLink)
	mux.HandleFunc("/api/gopay/cdp-otp", handleGopayCDPOTP)
	mux.HandleFunc("/api/gopay/smart-link", handleGopaySmartLink)
	mux.HandleFunc("/api/gopay/snap-probe", handleGopaySnapProbe)
	mux.HandleFunc("/api/gopay/monitor", handleGopayMonitor)
	mux.HandleFunc("/api/pricing/monitor", handlePricingMonitor)
	mux.HandleFunc("/api/gopay/full-link", handleGopayFullLink)
	mux.HandleFunc("/api/gopay/auto-trigger-check", handleGopayAutoTriggerCheck)
	mux.HandleFunc("/api/gopay/midtrans-linking-fill", handleGopayMidtransLinkingFill)
	mux.HandleFunc("/api/checkout/resolve-target", handleCheckoutResolveTarget)
	mux.HandleFunc("/api/checkout/auto-fill", handleCheckoutAutoFill)
	mux.HandleFunc("/api/perf/client", handleClientPerformanceLog)
	mux.Handle("/", staticAssetHandler(staticFiles))

	handler := securityHeaders(withAuditLogging(mux))

	server := &http.Server{
		Addr:              "127.0.0.1:18473",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("server started: http://localhost:18473")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func staticAssetHandler(staticFiles fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		cleanPath := path.Clean("/" + strings.TrimSpace(r.URL.Path))
		fileName := strings.TrimPrefix(cleanPath, "/")
		if fileName == "" || fileName == "." {
			fileName = "index.html"
		}
		info, err := fs.Stat(staticFiles, fileName)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(staticFiles, fileName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "static file read failed")
			return
		}
		ext := strings.ToLower(path.Ext(fileName))
		contentType := mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = http.DetectContentType(data)
		}
		w.Header().Set("Content-Type", contentType)
		if fileName == "index.html" || ext == ".html" {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		serveStaticBytes(w, r, data)
	})
}

func serveStaticBytes(w http.ResponseWriter, r *http.Request, data []byte) {
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		return
	}
	if len(data) >= 1024 && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.WriteHeader(http.StatusOK)
		gz := gzip.NewWriter(w)
		_, _ = gz.Write(data)
		_ = gz.Close()
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func acceptsGzip(value string) bool {
	for _, part := range strings.Split(value, ",") {
		token := strings.ToLower(strings.TrimSpace(strings.Split(part, ";")[0]))
		if token == "gzip" {
			return true
		}
	}
	return false
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func handleClientPerformanceLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	summary := map[string]any{
		"page_url":               safeNetworkDiagnosticURL(stringifyJSONValue(payload["page_url"])),
		"resource_count":         jsonArrayLength(payload["resources"]),
		"long_task_count":        jsonArrayLength(payload["long_tasks"]),
		"slow_interaction_count": jsonArrayLength(payload["slow_interactions"]),
		"error_count":            jsonArrayLength(payload["errors"]),
	}
	if nav, ok := payload["nav"].(map[string]any); ok {
		summary["navigation"] = sanitizeAuditValue(nav)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"stage":   "client_perf_recorded",
		"summary": summary,
	})
}

func jsonArrayLength(value any) int {
	if items, ok := value.([]any); ok {
		return len(items)
	}
	return 0
}

type incognitoOpenRequest struct {
	URL       string `json:"url"`
	NewWindow bool   `json:"new_window"`
}

const cdpDebuggingPort = 9223

func incognitoUserDataDir() string {
	return filepath.Join(os.TempDir(), "chatadd-chrome-incognito")
}

func buildChromeArgs(targetURL string, newWindow bool) []string {
	args := []string{"/c", "start", ""}
	baseArgs := []string{
		"chrome",
		"--incognito",
		"--remote-debugging-port=" + strconv.Itoa(cdpDebuggingPort),
		"--user-data-dir=" + incognitoUserDataDir(),
	}
	if newWindow {
		baseArgs = append(baseArgs, "--new-window")
	}
	baseArgs = append(baseArgs, targetURL)
	return append(args, baseArgs...)
}

func handleIncognitoOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req incognitoOpenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	parsed, err := url.Parse(req.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "url must be http or https")
		return
	}
	args := buildChromeArgs(req.URL, req.NewWindow)
	cmd := exec.Command("cmd", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		log.Printf("launch chrome incognito failed: %v", err)
		writeError(w, http.StatusInternalServerError, "无法启动 Chrome 无痕窗口: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "url": req.URL, "new_window": req.NewWindow,
	})
}

type cdpCommand struct {
	ID     int            `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int            `json:"id"`
	Result map[string]any `json:"result,omitempty"`
	Error  *cdpErrorBody  `json:"error,omitempty"`
}

type cdpErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	Title                string `json:"title,omitempty"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpNotReadyError struct{ message string }

func (e *cdpNotReadyError) Error() string { return e.message }

func isCDPReady(port int) bool {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func getCDPTargets(port int) ([]cdpTarget, error) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return nil, fmt.Errorf("无法连接 CDP: %w", err)
	}
	defer resp.Body.Close()
	var targets []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("解析 CDP 目标列表失败: %w", err)
	}
	return targets, nil
}

func findChatGPTTarget(port int) (*cdpTarget, error) {
	targets, err := getCDPTargets(port)
	if err != nil {
		return nil, err
	}
	for i := range targets {
		if targets[i].Type == "page" && strings.Contains(targets[i].URL, "chatgpt.com") {
			return &targets[i], nil
		}
	}
	return nil, &cdpNotReadyError{message: "未找到 chatgpt.com 页面"}
}

func findAnyTarget(port int, urlPattern string) (*cdpTarget, error) {
	targets, err := getCDPTargets(port)
	if err != nil {
		return nil, err
	}
	for i := range targets {
		t := &targets[i]
		if (t.Type == "page" || t.Type == "iframe") && strings.Contains(t.URL, urlPattern) {
			return t, nil
		}
	}
	return nil, &cdpNotReadyError{message: "未在 CDP 中找到匹配 " + urlPattern + " 的页面"}
}

func sendCDPCommandWithID(conn *websocket.Conn, id int64, method string, params map[string]any) (*cdpResponse, error) {
	cmd := cdpCommand{ID: int(id), Method: method, Params: params}
	if err := conn.WriteJSON(cmd); err != nil {
		return nil, fmt.Errorf("发送 CDP 命令失败: %w", err)
	}
	for {
		var resp cdpResponse
		if err := conn.ReadJSON(&resp); err != nil {
			return nil, fmt.Errorf("读取 CDP 响应失败: %w", err)
		}
		if resp.ID != int(id) {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("CDP 错误: %s (code %d)", resp.Error.Message, resp.Error.Code)
		}
		return &resp, nil
	}
}

func sendCDPCommand(conn *websocket.Conn, method string, params map[string]any) (*cdpResponse, error) {
	return sendCDPCommandWithID(conn, 1, method, params)
}

func extractResultString(resp *cdpResponse) (string, error) {
	resultObj, ok := resp.Result["result"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("CDP 返回结构异常: result 字段缺失")
	}
	val, ok := resultObj["value"]
	if !ok {
		return "", fmt.Errorf("CDP 返回结构异常: value 字段缺失")
	}
	switch v := val.(type) {
	case string:
		return v, nil
	default:
		return fmt.Sprint(v), nil
	}
}

var cdpCommandCounter int64 = 1

func executeCDPScript(conn *websocket.Conn, script string) (string, error) {
	id := atomic.AddInt64(&cdpCommandCounter, 1)
	resp, err := sendCDPCommandWithID(conn, id, "Runtime.evaluate", map[string]any{
		"expression": script, "awaitPromise": true, "returnByValue": true,
	})
	if err != nil {
		return "", err
	}
	return extractResultString(resp)
}

func fetchSessionJSONViaCDP() (string, error) {
	sessionJSON, _, err := fetchSessionJSONViaCDPWithDiagnostics(nil)
	return sessionJSON, err
}

func sessionBoolValue(diagnostics map[string]any, key string) bool {
	value, _ := diagnostics[key].(bool)
	return value
}

func sessionIntValue(diagnostics map[string]any, key string) int {
	switch value := diagnostics[key].(type) {
	case float64:
		if value > float64(math.MaxInt) || value < float64(math.MinInt) {
			return 0
		}
		return int(value)
	case int:
		return value
	case int64:
		if value > int64(math.MaxInt) || value < int64(math.MinInt) {
			return 0
		}
		return int(value)
	default:
		return 0
	}
}

func buildSessionDiagnosticsConclusion(diagnostics map[string]any) map[string]any {
	if diagnostics == nil {
		return map[string]any{
			"status":      "unknown",
			"message":     "尚未生成 Session 诊断信息",
			"auto_action": "open_chatgpt_home",
			"next_steps": []string{
				"打开 Chrome 无痕窗口",
				"访问 chatgpt.com 并完成登录",
				"再次点击获取 Session JSON",
			},
		}
	}

	if !sessionBoolValue(diagnostics, "cdp_ready") {
		return map[string]any{
			"status":      "cdp_not_ready",
			"message":     "Chrome CDP 尚未就绪，本地无痕窗口或远程调试端口不可用",
			"auto_action": "wait_and_retry",
			"next_steps": []string{
				"先点击打开无痕窗口",
				"确认 Chrome 已启动且远程调试端口 9223 可用",
				"等待页面打开后重新获取 Session JSON",
			},
		}
	}

	if !sessionBoolValue(diagnostics, "target_found") {
		return map[string]any{
			"status":      "chatgpt_page_missing",
			"message":     "未找到 chatgpt.com 页面，当前无痕窗口可能未打开到目标站点",
			"auto_action": "open_chatgpt_home",
			"next_steps": []string{
				"确认无痕窗口已打开 chatgpt.com",
				"若页面被跳转到其他站点，请切回 ChatGPT 首页或登录页",
				"重新执行获取 Session JSON",
			},
		}
	}

	hasToken := sessionBoolValue(diagnostics, "has_access_token")
	loginSelectorCount := sessionIntValue(diagnostics, "login_selector_count")
	sessionStatus := sessionIntValue(diagnostics, "session_status")
	targetURL := stringifyJSONValue(diagnostics["target_url"])
	preview := strings.ToLower(stringifyJSONValue(diagnostics["session_preview"]))
	bodyText := strings.ToLower(stringifyJSONValue(diagnostics["body_text"]))

	if hasToken {
		return map[string]any{
			"status":      "session_ready",
			"message":     "已检测到 accessToken，可以继续后续 checkout 自动化流程",
			"auto_action": "continue_checkout",
			"next_steps": []string{
				"直接生成支付链接",
				"如需排查支付链路，可保持当前监控面板开启",
			},
		}
	}

	if sessionStatus == http.StatusUnauthorized || loginSelectorCount > 0 || strings.Contains(targetURL, "/auth/login") || strings.Contains(bodyText, "log in") || strings.Contains(bodyText, "登录") {
		return map[string]any{
			"status":      "login_required",
			"message":     "当前页面仍处于未登录态，请先在无痕窗口中完成 ChatGPT 登录",
			"auto_action": "wait_for_login",
			"next_steps": []string{
				"在无痕窗口输入账号并完成登录",
				"确认页面进入聊天页或已登录首页",
				"再次点击获取 Session JSON",
			},
		}
	}

	if sessionStatus >= 500 {
		return map[string]any{
			"status":      "session_endpoint_error",
			"message":     "已访问 /api/auth/session，但上游返回 5xx，当前更像是会话接口临时异常",
			"auto_action": "retry_session_fetch",
			"next_steps": []string{
				"刷新无痕窗口页面后重试",
				"确认网络代理或浏览器环境未拦截请求",
				"如多次失败，查看监控面板中的 OpenAI 网络请求",
			},
		}
	}

	if sessionStatus >= 200 && sessionStatus < 300 && !hasToken {
		message := "已请求 /api/auth/session，但返回内容里没有 accessToken"
		if strings.Contains(preview, "error") || strings.Contains(preview, "unauthorized") {
			message = "会话接口已返回响应，但内容表现为异常或未登录态"
		}
		return map[string]any{
			"status":      "session_missing_token",
			"message":     message,
			"auto_action": "retry_session_fetch",
			"next_steps": []string{
				"确认当前账号已完整登录而不是停留在中间页",
				"检查是否命中了风控、验证页或灰度页面",
				"继续观察监控面板里的 session_preview 和 body_text",
			},
		}
	}

	return map[string]any{
		"status":      "session_unknown",
		"message":     "Session 获取流程已执行，但暂时无法自动判定具体卡点",
		"auto_action": "manual_review",
		"next_steps": []string{
			"查看监控面板中的 session_preview、body_text 和 target_url",
			"确认页面是否已完成登录并停留在 chatgpt.com",
			"必要时重新打开无痕窗口后重试",
		},
	}
}

func withSessionDiagnosticsConclusion(diagnostics map[string]any) map[string]any {
	if diagnostics == nil {
		diagnostics = map[string]any{}
	}
	diagnostics["conclusion"] = buildSessionDiagnosticsConclusion(diagnostics)
	return diagnostics
}

func fetchSessionJSONViaCDPWithDiagnostics(onEvent func(monitorEvent)) (string, map[string]any, error) {
	if !isCDPReady(cdpDebuggingPort) {
		return "", withSessionDiagnosticsConclusion(map[string]any{"cdp_ready": false}), &cdpNotReadyError{message: "CDP 未就绪"}
	}
	if onEvent != nil {
		onEvent(monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: "Log", Method: "session-cdp", Summary: "CDP 已就绪，开始查找 chatgpt.com 页面"})
	}
	start := time.Now()
	target, err := findChatGPTTarget(cdpDebuggingPort)
	if err != nil {
		return "", withSessionDiagnosticsConclusion(map[string]any{"cdp_ready": true, "target_found": false}), err
	}
	if onEvent != nil {
		onEvent(monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: "Page", Method: "session-target", Summary: truncateURL(target.URL, 160)})
	}
	conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		return "", withSessionDiagnosticsConclusion(map[string]any{"cdp_ready": true, "target_found": true, "target_url": target.URL}), fmt.Errorf("WebSocket 连接失败: %w", err)
	}
	defer conn.Close()
	_, _ = sendCDPCommand(conn, "Runtime.enable", nil)
	_, _ = sendCDPCommand(conn, "Network.enable", map[string]any{"maxTotalBufferSize": 20000000})

	snapshotRaw, snapshotErr := executeCDPScript(conn, `(async () => {
		const sessionResp = await fetch('/api/auth/session');
		const sessionText = await sessionResp.text();
		const loginSelectors = [
			document.querySelector('button[data-testid="login-button"]'),
			document.querySelector('a[href*="/auth/login"]'),
			document.querySelector('input[type="email"]'),
			document.querySelector('form[action*="login"]')
		].filter(Boolean).length;
		let parsed = null;
		try { parsed = JSON.parse(sessionText); } catch (_err) {}
		return JSON.stringify({
			url: location.href,
			title: document.title,
			ready_state: document.readyState,
			login_selector_count: loginSelectors,
			has_access_token: !!(parsed && parsed.accessToken),
			session_status: sessionResp.status,
			session_length: sessionText.length,
			session_preview: sessionText.slice(0, 240),
			cookies_enabled: navigator.cookieEnabled,
			body_text: (document.body?.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 300),
			session_text: sessionText
		});
	})()`)
	if snapshotErr != nil {
		return "", withSessionDiagnosticsConclusion(map[string]any{"cdp_ready": true, "target_found": true, "target_url": target.URL}), snapshotErr
	}

	var snapshot map[string]any
	if err := json.Unmarshal([]byte(snapshotRaw), &snapshot); err != nil {
		return snapshotRaw, withSessionDiagnosticsConclusion(map[string]any{"cdp_ready": true, "target_found": true, "target_url": target.URL, "parse_error": err.Error()}), nil
	}
	sessionJSON := stringifyJSONValue(snapshot["session_text"])
	diagnostics := map[string]any{
		"cdp_ready":            true,
		"target_found":         true,
		"target_url":           stringifyJSONValue(snapshot["url"]),
		"title":                stringifyJSONValue(snapshot["title"]),
		"ready_state":          stringifyJSONValue(snapshot["ready_state"]),
		"login_selector_count": snapshot["login_selector_count"],
		"has_access_token":     snapshot["has_access_token"],
		"session_status":       snapshot["session_status"],
		"session_length":       snapshot["session_length"],
		"session_preview":      stringifyJSONValue(snapshot["session_preview"]),
		"cookies_enabled":      snapshot["cookies_enabled"],
		"body_text":            stringifyJSONValue(snapshot["body_text"]),
		"elapsed_ms":           time.Since(start).Milliseconds(),
	}
	diagnostics["conclusion"] = buildSessionDiagnosticsConclusion(diagnostics)
	if onEvent != nil {
		onEvent(monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: "Network", Method: "session-fetch", Summary: fmt.Sprintf("/api/auth/session status=%v len=%v", snapshot["session_status"], snapshot["session_length"])})
		if hasToken, _ := snapshot["has_access_token"].(bool); hasToken {
			onEvent(monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: "Log", Method: "session-success", Summary: "已提取 accessToken"})
		} else {
			onEvent(monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: "Error", Method: "session-missing-token", Summary: "未在 /api/auth/session 中检测到 accessToken"})
		}
	}
	return sessionJSON, diagnostics, nil
}

func handleSessionFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req struct {
		Stream bool `json:"stream"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&req)

	if req.Stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		encoder := json.NewEncoder(w)
		_ = encoder.Encode(monitorStreamEnvelope{Type: "started", Targets: []string{"chatgpt.com:/api/auth/session"}})
		flusher.Flush()

		sessionJSON, diagnostics, err := fetchSessionJSONViaCDPWithDiagnostics(func(evt monitorEvent) {
			item := evt
			_ = encoder.Encode(monitorStreamEnvelope{Type: "event", Event: &item})
			flusher.Flush()
		})
		if err != nil {
			_ = encoder.Encode(monitorStreamEnvelope{Type: "error", Error: err.Error(), Data: diagnostics})
			flusher.Flush()
			return
		}
		if conclusion, ok := diagnostics["conclusion"].(map[string]any); ok {
			message := stringifyJSONValue(conclusion["message"])
			if message != "" {
				_ = encoder.Encode(monitorStreamEnvelope{Type: "event", Event: &monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: "Log", Method: "session-conclusion", Summary: message}})
				flusher.Flush()
			}
		}
		_ = encoder.Encode(monitorStreamEnvelope{Type: "data", Data: map[string]any{"json": sessionJSON, "diagnostics": diagnostics}})
		flusher.Flush()
		_ = encoder.Encode(monitorStreamEnvelope{Type: "done", Data: map[string]any{"json": sessionJSON, "diagnostics": diagnostics}})
		flusher.Flush()
		return
	}

	sessionJSON, diagnostics, err := fetchSessionJSONViaCDPWithDiagnostics(nil)
	if err != nil {
		var notReady *cdpNotReadyError
		if errors.As(err, &notReady) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error(), "code": "cdp_not_ready", "diagnostics": diagnostics})
			return
		}
		log.Printf("fetch session JSON failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "获取失败: " + err.Error(), "diagnostics": diagnostics})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "json": sessionJSON, "diagnostics": diagnostics})
}

type gopayForceLinkRequest struct {
	AccountID   string `json:"account_id"`
	CountryCode string `json:"country_code"`
	PhoneNumber string `json:"phone_number"`
}

func injectIntoGoPayIframeDirect(countryCode, phoneNumber string) (map[string]any, error) {
	return injectIntoGoPayIframeDirectWithFilter(countryCode, phoneNumber, "")
}

func injectIntoGoPayIframeDirectWithFilter(countryCode, phoneNumber, urlFilter string) (map[string]any, error) {
	targets, err := getCDPTargets(cdpDebuggingPort)
	if err != nil {
		return nil, fmt.Errorf("获取 CDP 目标失败: %w", err)
	}
	var iframeTarget *cdpTarget
	for i := len(targets) - 1; i >= 0; i-- {
		t := &targets[i]
		if (t.Type == "page" || t.Type == "iframe") && strings.Contains(t.URL, "merchants-gws-app.gopayapi.com") && strings.Contains(t.URL, "user-creation") {
			if urlFilter == "" || strings.Contains(t.URL, urlFilter) {
				iframeTarget = t
				if urlFilter != "" {
					break
				}
			}
		}
	}
	if iframeTarget == nil {
		return nil, fmt.Errorf("未找到 GoPay iframe")
	}
	conn, _, err := websocket.DefaultDialer.Dial(iframeTarget.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("GoPay iframe 连接失败: %w", err)
	}
	defer conn.Close()
	script := fmt.Sprintf(`
		(async () => {
			function setNV(el, v) { const s = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set; s.call(el, v); el.dispatchEvent(new Event('input', {bubbles: true})); el.dispatchEvent(new Event('change', {bubbles: true})); }
			let r = {page: window.location.href, title: document.title, found: {phone: false, country: false, button: false}};
			const all = document.querySelectorAll('input');
			r.totalInputs = all.length;
			r.inputInfo = Array.from(all).map(el => ({type: el.type, mode: el.inputMode, ph: (el.placeholder||'').slice(0,20), nm: el.name||''}));
			const pe = document.querySelector('input[type="tel"], input[type="number"], input[inputmode="numeric"]') || Array.from(all).find(el => (el.placeholder||'').includes('phone') || (el.name||'').includes('phone'));
			if (pe) { setNV(pe, '%s'); r.found.phone = true; r.phoneType = pe.type; }
			const cs = document.querySelector('select');
			if (cs) { r.selectOptionCount = cs.options.length; r.selectOptions = Array.from(cs.options).slice(0,20).map(o => ({val: o.value, txt: o.text}));
				const o = Array.from(cs.options).find(o => o.value === '86' || o.value === '%s' || o.text.includes('+86'));
				if (o) { cs.value = o.value; cs.dispatchEvent(new Event('change', {bubbles: true})); r.found.country = true; r.selectedOption = {val: o.value, txt: o.text}; } }
			await new Promise(rs => setTimeout(rs, 800));
			const bt = document.querySelectorAll('button'); r.buttonTexts = Array.from(bt).slice(0,5).map(b => b.textContent.trim().slice(0,30));
			const sb = document.querySelector('button[type="submit"]') || Array.from(bt).find(b => { const t = (b.textContent||'').toLowerCase(); return t.includes('continue')||t.includes('submit')||t.includes('lanjut')||t.includes('kirim')||t.includes('next')||t.includes('verify'); });
			if (sb && !sb.disabled) { r.found.button = true; r.buttonText = sb.textContent.trim(); sb.click(); r.clicked = true; }
			else if (sb) { r.found.button = true; r.buttonText = sb.textContent.trim(); r.buttonDisabled = sb.disabled; }
			return JSON.stringify(r);
		})()`, phoneNumber, countryCode)
	resultJSON, err := executeCDPScript(conn, script)
	if err != nil {
		return nil, fmt.Errorf("iframe 注入失败: %w", err)
	}
	var result map[string]any
	if json.Unmarshal([]byte(resultJSON), &result) != nil {
		result = map[string]any{"raw": resultJSON}
	}
	stateJSON, _ := executeCDPScript(conn, `(async () => { await new Promise(r => setTimeout(r, 3000)); const e = document.querySelector('[class*="error" i]'); const inps = document.querySelectorAll('input'); return JSON.stringify({title: document.title, url: window.location.href, bodySnippet: (document.body?.textContent||'').trim().slice(0,200), errorText: e?.textContent?.trim()||'', inputCount: inps.length}); })()`)
	var stateResult map[string]any
	if json.Unmarshal([]byte(stateJSON), &stateResult) == nil {
		result["after_submit"] = stateResult
	}
	return result, nil
}

type gopayAutoLinkRequest struct {
	AccountID   string `json:"account_id"`
	CountryCode string `json:"country_code"`
	PhoneNumber string `json:"phone_number"`
	OTPChannel  string `json:"otp_channel,omitempty"`
	OTP         string `json:"otp,omitempty"`
	PIN         string `json:"pin,omitempty"`
}

type gopaySmartLinkRequest struct {
	AccountID   string `json:"account_id"`
	CountryCode string `json:"country_code"`
	PhoneNumber string `json:"phone_number"`
	OTPChannel  string `json:"otp_channel,omitempty"`
	OTP         string `json:"otp,omitempty"`
	PIN         string `json:"pin,omitempty"`
}

type gopayMidtransLinkingFillRequest struct {
	TargetURL       string `json:"target_url"`
	CheckoutURL     string `json:"checkout_url,omitempty"`
	CountryCode     string `json:"country_code"`
	PhoneNumber     string `json:"phone_number"`
	DebugNetwork    bool   `json:"debug_network,omitempty"`
	AggressiveRetry bool   `json:"aggressive_retry,omitempty"`
}

type usAddress struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Line1     string `json:"line1"`
	City      string `json:"city"`
	State     string `json:"state"`
	ZipCode   string `json:"zip_code"`
	Country   string `json:"country"`
}

type monitorEvent struct {
	Timestamp int64  `json:"ts"`
	Domain    string `json:"domain"`
	Method    string `json:"method"`
	Summary   string `json:"summary"`
	Detail    any    `json:"detail,omitempty"`
}

type monitorSummaryStats struct {
	TotalEvents     int `json:"total_events"`
	NetworkCalls    int `json:"network_calls"`
	PageNavigations int `json:"page_navigations"`
	Errors          int `json:"errors"`
	ConsoleCalls    int `json:"console_calls"`
	OpenAIAPICalls  int `json:"openai_api_calls"`
	StripeAPICalls  int `json:"stripe_api_calls"`
	SnapAPICalls    int `json:"snap_api_calls"`
	GopayAPICalls   int `json:"gopay_api_calls"`
	DurationS       int `json:"duration_s"`
}

type monitorStreamEnvelope struct {
	Type    string               `json:"type"`
	Event   *monitorEvent        `json:"event,omitempty"`
	Summary *monitorSummaryStats `json:"summary,omitempty"`
	Targets []string             `json:"targets,omitempty"`
	Data    any                  `json:"data,omitempty"`
	Error   string               `json:"error,omitempty"`
}

type monitorTargetDescriptor struct {
	Label      string
	URLPattern string
}

type auditLogRecord struct {
	OperationTime   string         `json:"operation_time"`
	OperationType   string         `json:"operation_type"`
	OperationName   string         `json:"operation_name"`
	OperationDetail any            `json:"operation_detail,omitempty"`
	OperationResult string         `json:"operation_result"`
	StatusCode      int            `json:"status_code,omitempty"`
	AccountEmail    string         `json:"account_email"`
	IPAddress       string         `json:"ip_address"`
	RequestPath     string         `json:"request_path"`
	RequestMethod   string         `json:"request_method"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	Summary         string         `json:"summary,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type auditLogEnvelope struct {
	Email     string
	Timestamp time.Time
	Record    auditLogRecord
}

type auditLogger struct {
	baseDir       string
	archiveAfter  time.Duration
	maxFileSize   int64
	queue         chan auditLogEnvelope
	mu            sync.Mutex
	fileCache     map[string]*os.File
	fileOpenedAt  map[string]time.Time
	fileCreatedAt map[string]time.Time
	fileSizes     map[string]int64
}

func (l *auditLogger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	var closeErr error
	for path, fileHandle := range l.fileCache {
		if err := fileHandle.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(l.fileCache, path)
		delete(l.fileOpenedAt, path)
		delete(l.fileCreatedAt, path)
		delete(l.fileSizes, path)
	}
	return closeErr
}

const auditAnalysisLogVersion = "checkout-payment-flow-v1"

var auditFlowStages = map[string]string{
	"/api/health":                      "system_health",
	"/api/incognito/open":              "login_browser_prepare",
	"/api/session/fetch":               "session_capture",
	"/api/checkout":                    "checkout_create",
	"/api/checkout/start":              "checkout_create",
	"/api/checkout/resolve-target":     "checkout_target_resolve",
	"/api/checkout/auto-fill":          "checkout_page_fill",
	"/api/gopay/auto-trigger-check":    "gopay_auto_trigger_decision",
	"/api/gopay/midtrans-linking-fill": "gopay_midtrans_page_linking",
	"/api/gopay/full-link":             "gopay_full_payment_flow",
	"/api/gopay/force-link":            "gopay_linking",
	"/api/gopay/auto-link":             "gopay_linking",
	"/api/gopay/smart-link":            "gopay_linking",
	"/api/gopay/cdp-otp":               "gopay_otp_pin_browser_flow",
	"/api/gopay/snap-probe":            "gopay_snap_probe",
	"/api/gopay/monitor":               "payment_monitoring",
	"/api/pricing/monitor":             "pricing_monitoring",
}

var auditFlowStepIndexes = map[string]int{
	"/api/health":                      10,
	"/api/incognito/open":              20,
	"/api/session/fetch":               30,
	"/api/checkout":                    40,
	"/api/checkout/start":              40,
	"/api/checkout/resolve-target":     50,
	"/api/checkout/auto-fill":          60,
	"/api/gopay/auto-trigger-check":    70,
	"/api/gopay/midtrans-linking-fill": 80,
	"/api/gopay/force-link":            90,
	"/api/gopay/auto-link":             90,
	"/api/gopay/smart-link":            90,
	"/api/gopay/cdp-otp":               100,
	"/api/gopay/full-link":             110,
	"/api/gopay/snap-probe":            120,
	"/api/gopay/monitor":               130,
	"/api/pricing/monitor":             140,
}

var auditFlowRouteRoles = map[string]string{
	"/api/health":                      "确认本地 Go 服务可用，是全流程运行前的环境健康信号",
	"/api/incognito/open":              "打开或复用系统 Chrome 无痕窗口，为登录态、checkout 页面和 CDP 观测建立浏览器上下文",
	"/api/session/fetch":               "从浏览器上下文读取 ChatGPT Session JSON，用于后续 checkout token 获取或诊断",
	"/api/checkout":                    "基于账号 token、套餐、账单地区和税区信息生成 OpenAI/Stripe checkout 链接",
	"/api/checkout/start":              "基于账号 token、套餐、账单地区和税区信息生成 OpenAI/Stripe checkout 链接",
	"/api/checkout/resolve-target":     "从当前浏览器页面或输入链接中锁定真实 checkout 支付页目标",
	"/api/checkout/auto-fill":          "在 checkout 页面执行地址、支付方式和提交相关自动化动作",
	"/api/gopay/auto-trigger-check":    "判断页面是否已满足 GoPay 自动触发条件，避免重复或过早执行支付链路",
	"/api/gopay/midtrans-linking-fill": "在 Midtrans/GoPay 页面填充手机号并推进绑定入口",
	"/api/gopay/full-link":             "执行 GoPay 绑定、PIN 验证、支付处理和最终状态查询的后端全流程辅助",
	"/api/gopay/force-link":            "执行 GoPay 指定参数绑定请求",
	"/api/gopay/auto-link":             "执行 GoPay 自动绑定请求",
	"/api/gopay/smart-link":            "执行 GoPay 智能绑定请求并处理冲突复用场景",
	"/api/gopay/cdp-otp":               "通过 CDP 观测或操作 GoPay OTP/PIN 页面，但不记录明文 OTP/PIN",
	"/api/gopay/snap-probe":            "探测 Snap/Midtrans 页面结构与账号标识",
	"/api/gopay/monitor":               "采集支付相关页面、网络和控制台状态，用于异常定位和流畅性分析",
	"/api/pricing/monitor":             "采集定价页面状态，用于 checkout 前置页面诊断",
}

var flowAuditLogger = newAuditLogger(filepath.Join(".", "log"), 24*time.Hour, 8<<20, 512)

var gopayAutoTriggerMu sync.Mutex
var gopayAutoTriggeredCheckout = map[string]time.Time{}

var operationDisplayNames = map[string]string{
	"/api/health":                      "健康检查",
	"/api/checkout":                    "生成支付链接",
	"/api/checkout/start":              "生成支付链接",
	"/api/incognito/open":              "打开无痕窗口",
	"/api/session/fetch":               "获取 Session JSON",
	"/api/gopay/force-link":            "GoPay 强制绑定",
	"/api/gopay/auto-link":             "GoPay 自动绑定",
	"/api/gopay/cdp-otp":               "CDP OTP 获取",
	"/api/gopay/smart-link":            "GoPay 智能绑定",
	"/api/gopay/snap-probe":            "Snap 探测",
	"/api/gopay/monitor":               "流程监控",
	"/api/pricing/monitor":             "定价监控",
	"/api/gopay/full-link":             "GoPay 全流程绑定",
	"/api/gopay/auto-trigger-check":    "GoPay 自动触发检查",
	"/api/gopay/midtrans-linking-fill": "Midtrans GoPay 页面填充",
	"/api/checkout/resolve-target":     "Checkout 目标解析",
	"/api/checkout/auto-fill":          "Checkout 自动填充",
	"/api/perf/client":                 "客户端性能日志",
}

var operationTypes = map[string]string{
	"/api/health":                      "system_health",
	"/api/checkout":                    "checkout_create",
	"/api/checkout/start":              "checkout_create",
	"/api/incognito/open":              "login_open_window",
	"/api/session/fetch":               "session_query",
	"/api/gopay/force-link":            "gopay_link",
	"/api/gopay/auto-link":             "gopay_link",
	"/api/gopay/cdp-otp":               "otp_query",
	"/api/gopay/smart-link":            "gopay_link",
	"/api/gopay/snap-probe":            "data_query",
	"/api/gopay/monitor":               "monitor_trace",
	"/api/pricing/monitor":             "monitor_trace",
	"/api/gopay/full-link":             "gopay_full_flow",
	"/api/gopay/auto-trigger-check":    "gopay_auto_trigger",
	"/api/gopay/midtrans-linking-fill": "gopay_browser_linking",
	"/api/checkout/resolve-target":     "data_query",
	"/api/checkout/auto-fill":          "data_modify",
	"/api/perf/client":                 "performance_trace",
}

var sensitiveJSONKeys = map[string]struct{}{
	"token":                      {},
	"accessToken":                {},
	"access_token":               {},
	"cookie":                     {},
	"checkout_cookie":            {},
	"authorization":              {},
	"pin":                        {},
	"pin_token":                  {},
	"pin_used":                   {},
	"payment_pin":                {},
	"otp":                        {},
	"detected_otp":               {},
	"otp_used":                   {},
	"used_otp":                   {},
	"challenge_id":               {},
	"gopay_payment_challenge_id": {},
	"client_secret":              {},
	"session":                    {},
	"session_json":               {},
	"gopay_pin_token":            {},
	"gopay_payment_pin_token":    {},
	"payment_method_id":          {},
}

const maskedAuditValue = "***"

var paymentVoucherFieldKeys = []string{
	"status_code",
	"transaction_status",
	"fraud_status",
	"payment_type",
	"gross_amount",
	"currency",
	"merchant_id",
	"order_id",
	"settlement_time",
	"transaction_time",
}

var auditEmailPattern = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)

var auditFileNameSanitizer = strings.NewReplacer(
	"<", "_",
	">", "_",
	":", "_",
	"\"", "_",
	"/", "_",
	"\\", "_",
	"|", "_",
	"?", "_",
	"*", "_",
	" ", "_",
)

var auditSafeKeySanitizer = strings.NewReplacer("-", "_", ".", "_", " ", "_")

const auditMaxResponseBodyBytes = 512 << 10

var auditResponseSummaryKeys = map[string]struct{}{
	"ok":                         {},
	"stage":                      {},
	"ready":                      {},
	"reason":                     {},
	"strategy":                   {},
	"error":                      {},
	"code":                       {},
	"account_id":                 {},
	"target_url":                 {},
	"country_code":               {},
	"phone_number":               {},
	"reference_id":               {},
	"linking_reference_id":       {},
	"payment_reference_id":       {},
	"gopay_payment_reference_id": {},
	"payment_voucher":            {},
	"transaction_id":             {},
	"reused_existing":            {},
	"already_triggered":          {},
	"checkout_key":               {},
	"aggressive_retry":           {},
	"pin_stage":                  {},
	"pin_auto_filled":            {},
	"pin_auto_submitted":         {},
	"pin_input_strategy":         {},
	"balance_amount":             {},
	"balance_state":              {},
	"hubungkan_auto_clicked":     {},
	"pay_now_auto_clicked":       {},
	"auto_action_paused":         {},
	"auto_action_stage":          {},
	"otp_manual_required":        {},
	"has_otp_field":              {},
	"has_pin_field":              {},
	"cdp_url_host":               {},
	"cdp_url_path":               {},
}

var usStates = []string{"AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "FL", "GA", "HI", "ID", "IL", "IN", "IA", "KS", "KY", "LA", "ME", "MD", "MA", "MI", "MN", "MS", "MO", "MT", "NE", "NV", "NH", "NJ", "NM", "NY", "NC", "ND", "OH", "OK", "OR", "PA", "RI", "SC", "SD", "TN", "TX", "UT", "VT", "VA", "WA", "WV", "WI", "WY"}
var usStateNames = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas", "CA": "California",
	"CO": "Colorado", "CT": "Connecticut", "DE": "Delaware", "FL": "Florida", "GA": "Georgia",
	"HI": "Hawaii", "ID": "Idaho", "IL": "Illinois", "IN": "Indiana", "IA": "Iowa",
	"KS": "Kansas", "KY": "Kentucky", "LA": "Louisiana", "ME": "Maine", "MD": "Maryland",
	"MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota", "MS": "Mississippi", "MO": "Missouri",
	"MT": "Montana", "NE": "Nebraska", "NV": "Nevada", "NH": "New Hampshire", "NJ": "New Jersey",
	"NM": "New Mexico", "NY": "New York", "NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio",
	"OK": "Oklahoma", "OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island", "SC": "South Carolina",
	"SD": "South Dakota", "TN": "Tennessee", "TX": "Texas", "UT": "Utah", "VT": "Vermont",
	"VA": "Virginia", "WA": "Washington", "WV": "West Virginia", "WI": "Wisconsin", "WY": "Wyoming",
}
var usFirstNames = []string{"James", "John", "Robert", "Michael", "William", "David", "Richard", "Joseph", "Thomas", "Charles", "Mary", "Patricia", "Jennifer", "Linda", "Barbara", "Elizabeth", "Susan", "Jessica", "Sarah", "Karen", "Emma", "Olivia", "Ava", "Isabella", "Sophia", "Mia", "Charlotte", "Amelia", "Harper", "Evelyn"}
var usLastNames = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas", "Taylor", "Moore", "Jackson", "Martin", "Lee", "Perez", "Thompson", "White", "Harris", "Sanchez", "Clark", "Ramirez", "Lewis"}
var usStreets = []string{"Main St", "Oak Ave", "Elm St", "Maple Dr", "Cedar Ln", "Pine Rd", "Washington Blvd", "Park Ave", "Broadway", "Lake Dr", "Hill Rd", "River Rd", "Church St", "School St", "Mill Rd", "Valley View Dr", "Sunset Blvd"}
var usCities = []string{"New York", "Los Angeles", "Chicago", "Houston", "Phoenix", "Philadelphia", "San Antonio", "San Diego", "Dallas", "Austin", "Jacksonville", "Fort Worth", "Columbus", "Charlotte", "Indianapolis", "San Francisco", "Seattle", "Denver", "Nashville", "Portland", "Memphis", "Baltimore", "Milwaukee", "Albuquerque"}

func randomInt(n int) int {
	b := make([]byte, 8)
	rand.Read(b)
	return int(binary.BigEndian.Uint64(b) % uint64(n))
}

func generateUSAddress() usAddress {
	return usAddress{
		FirstName: usFirstNames[randomInt(len(usFirstNames))],
		LastName:  usLastNames[randomInt(len(usLastNames))],
		Line1:     fmt.Sprintf("%d %s", 100+randomInt(9900), usStreets[randomInt(len(usStreets))]),
		City:      usCities[randomInt(len(usCities))],
		State:     usStates[randomInt(len(usStates))],
		ZipCode:   fmt.Sprintf("%05d", 10000+randomInt(90000)),
		Country:   "US",
	}
}

func checkoutStateSelectValue(state string) string {
	state = strings.TrimSpace(state)
	if state == "" {
		return ""
	}
	if mapped, ok := usStateNames[strings.ToUpper(state)]; ok {
		return mapped
	}
	return state
}

func truncateURL(u string, n int) string {
	if len(u) <= n {
		return u
	}
	return u[:n] + "..."
}

func monitorTargets() []monitorTargetDescriptor {
	return []monitorTargetDescriptor{
		{Label: "OpenAI", URLPattern: "chatgpt.com"},
		{Label: "OpenAI", URLPattern: "pay.openai.com"},
		{Label: "Stripe", URLPattern: "checkout.stripe.com"},
		{Label: "Stripe", URLPattern: "pm-redirects.stripe.com"},
		{Label: "Snap", URLPattern: "midtrans.com"},
		{Label: "GoPay", URLPattern: "merchants-gws-app.gopayapi.com"},
	}
}

func classifyMonitorURL(rawURL string) string {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.Contains(u, "chatgpt.com") || strings.Contains(u, "pay.openai.com"):
		return "openai"
	case strings.Contains(u, "stripe.com"):
		return "stripe"
	case strings.Contains(u, "midtrans.com"):
		return "snap"
	case strings.Contains(u, "gopayapi.com"):
		return "gopay"
	default:
		return "other"
	}
}

func summarizeMonitorEvents(events []monitorEvent, durationS int) monitorSummaryStats {
	summary := monitorSummaryStats{DurationS: durationS, TotalEvents: len(events)}
	for _, e := range events {
		switch e.Domain {
		case "Network":
			summary.NetworkCalls++
			classification := classifyMonitorURL(e.Summary)
			switch classification {
			case "openai":
				summary.OpenAIAPICalls++
			case "stripe":
				summary.StripeAPICalls++
			case "snap":
				summary.SnapAPICalls++
			case "gopay":
				summary.GopayAPICalls++
			}
		case "Page":
			summary.PageNavigations++
		case "Error":
			summary.Errors++
		case "Console":
			summary.ConsoleCalls++
		}
	}
	return summary
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func handleLocalCheckoutApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()
	var req checkoutApproveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.CheckoutSessionID = strings.TrimSpace(req.CheckoutSessionID)
	req.ProcessorEntity = strings.TrimSpace(req.ProcessorEntity)
	result, err := approveCheckoutLocally(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func approveCheckoutLocally(req checkoutApproveRequest) (map[string]any, error) {
	req.CheckoutSessionID = strings.TrimSpace(req.CheckoutSessionID)
	req.ProcessorEntity = strings.TrimSpace(req.ProcessorEntity)
	req.PaymentMethodID = strings.TrimSpace(req.PaymentMethodID)
	req.SubmissionAttemptID = strings.TrimSpace(req.SubmissionAttemptID)
	if req.CheckoutSessionID == "" {
		return nil, errors.New("checkout_session_id is required")
	}
	if req.ProcessorEntity == "" {
		return nil, errors.New("processor_entity is required")
	}

	return map[string]any{
		"result":                "approved",
		"mode":                  "local",
		"checkout_session_id":   req.CheckoutSessionID,
		"processor_entity":      req.ProcessorEntity,
		"payment_method_id":     req.PaymentMethodID,
		"submission_attempt_id": req.SubmissionAttemptID,
		"approval_note":         "local approval result; external ChatGPT approval endpoint is not called",
	}, nil
}

func approveCheckoutViaLocalHTTP(ctx context.Context, req checkoutApproveRequest) (map[string]any, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:8080/backend-api/payments/checkout/approve", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Origin", "https://chatgpt.com")
	httpReq.Header.Set("Referer", "https://chatgpt.com")
	httpReq.Header.Set("User-Agent", "gopay2codex-local-approve/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := doHTTPRequestWithRetry(httpReq.Context(), client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("local approve request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func approveCheckoutViaExternalHTTP(ctx context.Context, client *http.Client, token string, session checkoutSession, req checkoutApproveRequest) (map[string]any, error) {
	req.CheckoutSessionID = strings.TrimSpace(req.CheckoutSessionID)
	req.ProcessorEntity = strings.TrimSpace(req.ProcessorEntity)
	req.PaymentMethodID = strings.TrimSpace(req.PaymentMethodID)
	req.SubmissionAttemptID = strings.TrimSpace(req.SubmissionAttemptID)
	token = strings.TrimSpace(token)
	if req.CheckoutSessionID == "" {
		return nil, errors.New("checkout_session_id is required")
	}
	if req.ProcessorEntity == "" {
		return nil, errors.New("processor_entity is required")
	}
	if token == "" {
		return nil, errors.New("token is required")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	endpoint := checkoutApproveEndpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setCheckoutBackendHeaders(httpReq, token, session)

	startedAt := time.Now()
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"ok":                    resp.StatusCode >= 200 && resp.StatusCode < 300,
		"stage":                 "checkout_approval_upstream",
		"method":                http.MethodPost,
		"endpoint":              endpoint,
		"status":                resp.StatusCode,
		"content_type":          resp.Header.Get("Content-Type"),
		"checkout_session_id":   req.CheckoutSessionID,
		"processor_entity":      req.ProcessorEntity,
		"payment_method_id":     req.PaymentMethodID,
		"submission_attempt_id": req.SubmissionAttemptID,
		"elapsed_ms":            time.Since(startedAt).Milliseconds(),
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			result["body"] = parsed
			if parsedMap, ok := parsed.(map[string]any); ok {
				if upstreamErr, exists := parsedMap["error"]; exists {
					result["original_error"] = upstreamErr
				}
			}
			return result, nil
		}
	}
	result["raw_body"] = string(body)
	return result, nil
}

func handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()
	var req checkoutRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	normalizeCheckoutRequest(&req)
	if err := validateCheckoutRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	upstreamURL := checkoutEndpoint()
	client, err := newHTTPClient(req.Proxy, shouldSkipTLSVerify(upstreamURL))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	upstreamPayload := map[string]any{
		"entry_point":      req.EntryPoint,
		"plan_name":        req.PlanName,
		"billing_details":  req.BillingDetails,
		"promo_campaign":   req.PromoCampaign,
		"checkout_ui_mode": req.CheckoutUIMode,
	}
	payloadBytes, err := json.Marshal(upstreamPayload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(payloadBytes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create request")
		return
	}
	setCheckoutBackendHeaders(upstreamReq, req.Token, req.CheckoutSession)

	resp, err := doHTTPRequestWithRetry(ctx, client, upstreamReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "checkout upstream request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to read checkout response")
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "application/json") {
		writeJSON(w, resp.StatusCode, checkoutUpstreamNonJSONPayload(resp.StatusCode, contentType, upstreamURL, body))
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		if _, err := w.Write(body); err != nil {
			log.Printf("write response failed: %v", err)
		}
		return
	}

	var checkoutData any
	if err := json.Unmarshal(body, &checkoutData); err != nil {
		writeJSON(w, resp.StatusCode, map[string]any{
			"checkout": map[string]any{
				"status":  resp.StatusCode,
				"rawBody": string(body),
			},
			"stripe_init": stripeInitResult{
				OK:      false,
				Skipped: true,
				Error:   "checkout response is not valid json",
			},
		})
		return
	}

	stripeInit := initStripePaymentPage(ctx, client, checkoutData)
	stripeInitCheck := stripeInitBodyCheck{}
	if stripeInit.OK {
		stripeInitCheck = checkStripeInitTotal(stripeInit.Body)
	}
	checkoutURL := ""
	if stripeInit.OK && stripeInitCheck.OK {
		checkoutURL = checkoutLongURL(stripeInit.Body)
	}
	if stripeInit.OK && !stripeInitCheck.OK {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok":                  false,
			"stage":               "checkout_trial_validation",
			"error":               firstNonEmpty(stripeInitCheck.Error, "checkout session is not an eligible 0-IDR trial"),
			"hint":                "当前 token/account 没有返回可直接支付的 0 元试用会话；如果浏览器里本应看到 0 元试用，请补充 chatgpt.com 会话 Cookie 后重试。",
			"checkout_session_id": firstNonEmpty(findStringField(checkoutData, "checkout_session_id", "id"), stripeInit.CheckoutSessionID),
			"checkout_provider":   findStringField(checkoutData, "checkout_provider"),
			"promo_campaign":      nestedMapFromAny(checkoutData, "promo_campaign"),
			"trial_total":         stripeInitCheck.Total,
			"stripe_init":         summarizeStripeResult(stripeInit),
			"stripe_init_body":    stripeInit.Body,
		})
		return
	}
	if checkoutURL == "" {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":                  false,
			"stage":               "checkout_link",
			"error":               "stripe init did not return a usable checkout url",
			"checkout_session_id": firstNonEmpty(findStringField(checkoutData, "checkout_session_id", "id"), stripeInit.CheckoutSessionID),
			"checkout_provider":   findStringField(checkoutData, "checkout_provider"),
			"url":                 findStringField(checkoutData, "url"),
			"stripe_hosted_url":   findStringField(checkoutData, "stripe_hosted_url"),
			"confirm_return_url":  findStringField(checkoutData, "confirm_return_url"),
			"raw_checkout":        checkoutData,
			"stripe_init":         summarizeStripeResult(stripeInit),
			"stripe_init_body":    stripeInit.Body,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"checkout_url":        checkoutURL,
		"url":                 checkoutURL,
		"checkout_session_id": firstNonEmpty(stripeInit.CheckoutSessionID, findStringField(checkoutData, "checkout_session_id", "id"), sessionIDFromCheckoutURL(checkoutURL)),
	})
	return
}

func handleGopayOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()
	var req gopayOTPRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.ReferenceID = strings.TrimSpace(req.ReferenceID)
	req.OTPChannel = strings.TrimSpace(req.OTPChannel)
	req.OTP = strings.TrimSpace(req.OTP)
	otpChannel, err := normalizeOTPChannel(req.OTPChannel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"stage": "gopay_validate_otp",
			"error": err.Error(),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	gopayOTPResult, err := validateGopayOTPViaLocalMock(ctx, req.ReferenceID, req.OTP)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":        false,
			"stage":     "gopay_validate_otp",
			"error":     err.Error(),
			"reference": req.ReferenceID,
		})
		return
	}
	if err := validateGopayOTPResult(gopayOTPResult); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":        false,
			"stage":     "gopay_validate_otp",
			"error":     err.Error(),
			"gopay_otp": gopayOTPResult,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"stage":        "gopay_validate_otp",
		"next_action":  "enter_pin",
		"reference_id": req.ReferenceID,
		"otp_channel":  otpChannel,
		"challenge_id": gopayOTPChallengeID(gopayOTPResult),
		"gopay_otp":    gopayOTPResult,
	})
}

func handleGopayPIN(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()
	var req gopayPINRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.ReferenceID = strings.TrimSpace(req.ReferenceID)
	req.ChallengeID = strings.TrimSpace(req.ChallengeID)
	req.GopayGUID = strings.TrimSpace(req.GopayGUID)
	req.PIN = strings.TrimSpace(req.PIN)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	gopayPINResult, err := requestGopayPINTokenViaLocalMock(ctx, req.ChallengeID, req.PIN)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"stage": "gopay_pin_token",
			"error": err.Error(),
		})
		return
	}

	gopayValidatePINResult, err := validateGopayPINViaLocalMock(ctx, req.ReferenceID, gopayPINToken(gopayPINResult))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"stage": "gopay_validate_pin",
			"error": err.Error(),
		})
		return
	}
	if err := validateGopayPINResult(gopayValidatePINResult); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":                   false,
			"stage":                "gopay_validate_pin",
			"error":                err.Error(),
			"gopay_validate_pin":   gopayValidatePINResult,
			"gopay_pin_token_body": gopayPINResult,
		})
		return
	}

	gopayCharge, err := chargeMidtransGopayViaLocalMock(ctx, req.GopayGUID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":           false,
			"stage":        "midtrans_gopay_charge",
			"error":        err.Error(),
			"gopay_charge": gopayCharge,
			"gopay_guid":   req.GopayGUID,
			"reference_id": req.ReferenceID,
		})
		return
	}

	verificationLink := stringifyJSONValue(gopayCharge["gopay_verification_link_url"])
	if verificationLink == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":                 false,
			"stage":              "gopay_payment_reference",
			"error":              "midtrans charge did not return gopay_verification_link_url",
			"status_code":        stringifyJSONValue(gopayCharge["status_code"]),
			"status_message":     stringifyJSONValue(gopayCharge["status_message"]),
			"transaction_status": stringifyJSONValue(gopayCharge["transaction_status"]),
			"fraud_status":       stringifyJSONValue(gopayCharge["fraud_status"]),
			"gopay_charge":       gopayCharge,
		})
		return
	}
	paymentReferenceID, err := referenceFromGopayVerificationLink(verificationLink)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":           false,
			"stage":        "gopay_payment_reference",
			"error":        err.Error(),
			"gopay_charge": gopayCharge,
		})
		return
	}
	gopayPaymentValidate, err := validateGopayPaymentViaLocalMock(ctx, paymentReferenceID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":                         false,
			"stage":                      "gopay_payment_validate",
			"error":                      err.Error(),
			"gopay_payment_reference_id": paymentReferenceID,
			"gopay_payment_validate":     gopayPaymentValidate,
		})
		return
	}

	gopayPaymentConfirm, err := confirmGopayPaymentViaLocalMock(ctx, paymentReferenceID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":                         false,
			"stage":                      "gopay_payment_confirm",
			"error":                      err.Error(),
			"gopay_payment_reference_id": paymentReferenceID,
			"gopay_payment_confirm":      gopayPaymentConfirm,
		})
		return
	}
	paymentChallengeID := gopayPaymentChallengeID(gopayPaymentConfirm)
	paymentClientID := gopayPaymentClientID(gopayPaymentConfirm)
	paymentPINTokenResult, err := requestGopayPaymentPINTokenViaLocalMock(ctx, paymentChallengeID, paymentClientID, req.PIN)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":                         false,
			"stage":                      "gopay_payment_pin_token",
			"error":                      err.Error(),
			"gopay_payment_challenge_id": paymentChallengeID,
			"gopay_payment_client_id":    paymentClientID,
		})
		return
	}
	gopayPaymentProcess, err := processGopayPaymentViaLocalMock(ctx, paymentReferenceID, gopayPINToken(paymentPINTokenResult))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":                         false,
			"stage":                      "gopay_payment_process",
			"error":                      err.Error(),
			"gopay_payment_reference_id": paymentReferenceID,
			"gopay_payment_process":      gopayPaymentProcess,
		})
		return
	}
	transactionID, err := transactionIDFromRedirectURL(findStringField(gopayPaymentProcess, "redirect_url"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":                    false,
			"stage":                 "midtrans_transaction_id",
			"error":                 err.Error(),
			"gopay_payment_process": gopayPaymentProcess,
		})
		return
	}
	midtransStatus, err := getMidtransTransactionStatusViaLocalMock(ctx, transactionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":              false,
			"stage":           "midtrans_transaction_status",
			"error":           err.Error(),
			"transaction_id":  transactionID,
			"midtrans_status": midtransStatus,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                         true,
		"stage":                      "gopay_complete",
		"reference_id":               req.ReferenceID,
		"gopay_guid":                 req.GopayGUID,
		"gopay_pin_token":            gopayPINResult,
		"gopay_validate_pin":         gopayValidatePINResult,
		"gopay_charge":               gopayCharge,
		"gopay_payment_reference_id": paymentReferenceID,
		"gopay_payment_validate":     gopayPaymentValidate,
		"gopay_payment_confirm":      gopayPaymentConfirm,
		"gopay_payment_challenge_id": paymentChallengeID,
		"gopay_payment_client_id":    paymentClientID,
		"gopay_payment_pin_token":    paymentPINTokenResult,
		"gopay_payment_process":      gopayPaymentProcess,
		"midtrans_status":            midtransStatus,
		"payment_voucher": buildGopayPaymentVoucher(gopayPaymentResolution{
			Charge:             gopayCharge,
			PaymentReferenceID: paymentReferenceID,
			TransactionID:      transactionID,
			MidtransStatus:     midtransStatus,
			PinUsed:            req.PIN,
		}),
	})
}

func initStripePaymentPage(ctx context.Context, client *http.Client, checkoutData any) stripeInitResult {
	sessionID := findStringField(checkoutData, "checkout_session_id", "checkoutSessionId", "id")
	publishableKey := findStringField(checkoutData, "publishable_key", "publishableKey", "key")
	if sessionID != "" && !strings.HasPrefix(sessionID, "cs_") {
		sessionID = ""
	}
	if publishableKey != "" && !strings.HasPrefix(publishableKey, "pk_") {
		publishableKey = ""
	}
	if sessionID == "" || publishableKey == "" {
		return stripeInitResult{
			OK:      false,
			Skipped: true,
			Error:   "checkout response missing checkout_session_id or publishable_key",
		}
	}

	form := stripeInitForm(publishableKey)
	initURL := "https://api.stripe.com/v1/payment_pages/" + url.PathEscape(sessionID) + "/init"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, strings.NewReader(form.Encode()))
	if err != nil {
		return stripeInitResult{
			OK:                false,
			Error:             "failed to create stripe init request: " + err.Error(),
			Stage:             "stripe_init",
			Method:            http.MethodPost,
			Endpoint:          initURL,
			CheckoutSessionID: sessionID,
			PublishableKey:    publishableKey,
		}
	}

	setStripeInitHeaders(httpReq)
	startedAt := time.Now()
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return stripeInitResult{
			OK:                false,
			Error:             "stripe init request failed: " + err.Error(),
			Stage:             "stripe_init",
			Method:            http.MethodPost,
			Endpoint:          initURL,
			CheckoutSessionID: sessionID,
			PublishableKey:    publishableKey,
			ElapsedMS:         time.Since(startedAt).Milliseconds(),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return stripeInitResult{
			OK:                false,
			Error:             "failed to read stripe init response: " + err.Error(),
			Stage:             "stripe_init",
			Method:            http.MethodPost,
			Endpoint:          initURL,
			Status:            resp.StatusCode,
			ContentType:       resp.Header.Get("Content-Type"),
			CheckoutSessionID: sessionID,
			PublishableKey:    publishableKey,
			ElapsedMS:         time.Since(startedAt).Milliseconds(),
		}
	}

	result := stripeInitResult{
		OK:                resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stage:             "stripe_init",
		Method:            http.MethodPost,
		Endpoint:          initURL,
		Status:            resp.StatusCode,
		ContentType:       resp.Header.Get("Content-Type"),
		CheckoutSessionID: sessionID,
		PublishableKey:    publishableKey,
		ElapsedMS:         time.Since(startedAt).Milliseconds(),
	}
	if strings.Contains(strings.ToLower(result.ContentType), "application/json") {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			result.Body = parsed
			return result
		}
	}

	result.RawBody = string(body)
	return result
}

func stripeInitForm(publishableKey string) url.Values {
	form := url.Values{}
	form.Set("browser_locale", "zh-CN")
	form.Set("browser_timezone", "Asia/Shanghai")
	form.Set("elements_session_client[client_betas][0]", "custom_checkout_server_updates_1")
	form.Set("elements_session_client[client_betas][1]", "custom_checkout_manual_approval_1")
	form.Set("elements_session_client[elements_init_source]", "custom_checkout")
	form.Set("elements_session_client[referrer_host]", "chatgpt.com")
	form.Set("elements_session_client[stripe_js_id]", randomUUID())
	form.Set("elements_session_client[locale]", "zh-CN")
	form.Set("elements_session_client[is_aggregation_expected]", "false")
	form.Set("elements_options_client[stripe_js_locale]", "auto")
	form.Set("elements_options_client[saved_payment_method][enable_save]", "auto")
	form.Set("elements_options_client[saved_payment_method][enable_redisplay]", "auto")
	form.Set("key", publishableKey)
	form.Set("_stripe_version", "2025-03-31.basil; checkout_server_update_beta=v1; checkout_manual_approval_preview=v1")
	return form
}

func updateStripePaymentPage(ctx context.Context, client *http.Client, initBody any, sessionID string, publishableKey string, requestedTaxRegion taxRegion, requestedCustomerEmail string, clientCtx stripeClientContext) stripeInitResult {
	if sessionID == "" || publishableKey == "" {
		return stripeInitResult{
			OK:      false,
			Skipped: true,
			Error:   "missing stripe session id or publishable key",
		}
	}

	form := stripeUpdateForm(initBody, publishableKey, requestedTaxRegion, requestedCustomerEmail, clientCtx)
	updateURL := "https://api.stripe.com/v1/payment_pages/" + url.PathEscape(sessionID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, updateURL, strings.NewReader(form.Encode()))
	if err != nil {
		return stripeInitResult{
			OK:                false,
			Error:             "failed to create stripe update request: " + err.Error(),
			Stage:             "stripe_update",
			Method:            http.MethodPost,
			Endpoint:          updateURL,
			CheckoutSessionID: sessionID,
			PublishableKey:    publishableKey,
		}
	}

	setStripeInitHeaders(httpReq)
	startedAt := time.Now()
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return stripeInitResult{
			OK:                false,
			Error:             "stripe update request failed: " + err.Error(),
			Stage:             "stripe_update",
			Method:            http.MethodPost,
			Endpoint:          updateURL,
			CheckoutSessionID: sessionID,
			PublishableKey:    publishableKey,
			ElapsedMS:         time.Since(startedAt).Milliseconds(),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return stripeInitResult{
			OK:                false,
			Error:             "failed to read stripe update response: " + err.Error(),
			Stage:             "stripe_update",
			Method:            http.MethodPost,
			Endpoint:          updateURL,
			Status:            resp.StatusCode,
			ContentType:       resp.Header.Get("Content-Type"),
			CheckoutSessionID: sessionID,
			PublishableKey:    publishableKey,
			ElapsedMS:         time.Since(startedAt).Milliseconds(),
		}
	}

	result := stripeInitResult{
		OK:                resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stage:             "stripe_update",
		Method:            http.MethodPost,
		Endpoint:          updateURL,
		Status:            resp.StatusCode,
		ContentType:       resp.Header.Get("Content-Type"),
		CheckoutSessionID: sessionID,
		PublishableKey:    publishableKey,
		ElapsedMS:         time.Since(startedAt).Milliseconds(),
	}
	if strings.Contains(strings.ToLower(result.ContentType), "application/json") {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			result.Body = parsed
			return result
		}
	}

	result.RawBody = string(body)
	return result
}

func stripeUpdateForm(initBody any, publishableKey string, requestedTaxRegion taxRegion, requestedCustomerEmail string, clientCtx stripeClientContext) url.Values {
	form := url.Values{}
	root, _ := initBody.(map[string]any)
	region := mergeTaxRegion(defaultTaxRegion(), taxRegionFromMap(nestedMap(root, "customer", "address")), requestedTaxRegion)

	setFormValue(form, "tax_region[country]", region.Country)
	setFormValue(form, "tax_region[line1]", region.Line1)
	setFormValue(form, "tax_region[city]", region.City)
	setFormValue(form, "tax_region[postal_code]", region.PostalCode)
	setFormValue(form, "tax_region[state]", region.State)

	form.Set("elements_session_client[client_betas][0]", "custom_checkout_server_updates_1")
	form.Set("elements_session_client[client_betas][1]", "custom_checkout_manual_approval_1")
	form.Set("elements_session_client[elements_init_source]", "custom_checkout")
	form.Set("elements_session_client[referrer_host]", "chatgpt.com")
	form.Set("elements_session_client[session_id]", clientCtx.ElementsSessionID)
	form.Set("elements_session_client[stripe_js_id]", clientCtx.ClientSessionID)
	form.Set("elements_session_client[locale]", stripeLocale(initBody))
	form.Set("elements_session_client[is_aggregation_expected]", "false")
	form.Set("elements_options_client[stripe_js_locale]", "auto")
	form.Set("elements_options_client[saved_payment_method][enable_save]", "auto")
	form.Set("elements_options_client[saved_payment_method][enable_redisplay]", "auto")
	form.Set("client_attribution_metadata[merchant_integration_additional_elements][0]", "payment")
	form.Set("client_attribution_metadata[merchant_integration_additional_elements][1]", "address")
	form.Set("key", publishableKey)
	form.Set("_stripe_version", "2025-03-31.basil; checkout_server_update_beta=v1; checkout_manual_approval_preview=v1")
	return form
}

func createStripePaymentMethod(ctx context.Context, client *http.Client, pageBody any, sessionID string, publishableKey string, requestedTaxRegion taxRegion, requestedCustomerEmail string, clientCtx stripeClientContext) stripeInitResult {
	if err := requireStripeTestMode(sessionID, publishableKey); err != nil {
		return stripeInitResult{OK: false, Error: err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	form := stripePaymentMethodForm(pageBody, sessionID, publishableKey, requestedTaxRegion, requestedCustomerEmail, clientCtx)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/payment_methods", strings.NewReader(form.Encode()))
	if err != nil {
		return stripeInitResult{OK: false, Error: "failed to create payment method request: " + err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	setStripeInitHeaders(httpReq)
	return doStripeFormRequest(client, httpReq, sessionID, publishableKey, "payment method")
}

func confirmStripePaymentPage(ctx context.Context, client *http.Client, pageBody any, sessionID string, publishableKey string, paymentMethodID string, clientCtx stripeClientContext) stripeInitResult {
	if err := requireStripeTestMode(sessionID, publishableKey); err != nil {
		return stripeInitResult{OK: false, Error: err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}
	if paymentMethodID == "" {
		return stripeInitResult{OK: false, Error: "missing payment method id", CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	form := stripeConfirmForm(pageBody, sessionID, publishableKey, paymentMethodID, clientCtx)
	confirmURL := "https://api.stripe.com/v1/payment_pages/" + url.PathEscape(sessionID) + "/confirm"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, confirmURL, strings.NewReader(form.Encode()))
	if err != nil {
		return stripeInitResult{OK: false, Error: "failed to create stripe confirm request: " + err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	setStripeInitHeaders(httpReq)
	return doStripeFormRequest(client, httpReq, sessionID, publishableKey, "confirm")
}

func getStripePaymentPageDetails(ctx context.Context, client *http.Client, sessionID string, publishableKey string, clientCtx stripeClientContext, pageBody any) stripeInitResult {
	if err := requireStripeTestMode(sessionID, publishableKey); err != nil {
		return stripeInitResult{OK: false, Error: err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	query := stripePaymentPageDetailsQuery(publishableKey, clientCtx, pageBody)
	detailsURL := "https://api.stripe.com/v1/payment_pages/" + url.PathEscape(sessionID) + "?" + query.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, detailsURL, nil)
	if err != nil {
		return stripeInitResult{OK: false, Error: "failed to create stripe payment details request: " + err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	setStripeInitHeaders(httpReq)
	return doStripeFormRequest(client, httpReq, sessionID, publishableKey, "payment details")
}

func stripePaymentMethodForm(pageBody any, sessionID string, publishableKey string, requestedTaxRegion taxRegion, requestedCustomerEmail string, clientCtx stripeClientContext) url.Values {
	form := url.Values{}
	root, _ := pageBody.(map[string]any)
	region := mergeTaxRegion(defaultTaxRegion(), taxRegionFromMap(nestedMap(root, "customer", "address")), requestedTaxRegion)

	form.Set("billing_details[name]", firstNonEmpty(findStringField(pageBody, "name"), "Test User"))
	form.Set("billing_details[email]", firstNonEmpty(requestedCustomerEmail, findStringField(pageBody, "customer_email", "email"), "test@example.com"))
	setFormValue(form, "billing_details[address][country]", region.Country)
	setFormValue(form, "billing_details[address][line1]", region.Line1)
	setFormValue(form, "billing_details[address][city]", region.City)
	setFormValue(form, "billing_details[address][postal_code]", region.PostalCode)
	setFormValue(form, "billing_details[address][state]", region.State)
	form.Set("type", "gopay")
	form.Set("payment_user_agent", "stripe.js/332636417d; stripe-js-v3/332636417d; payment-element; deferred-intent")
	form.Set("referrer", "https://chatgpt.com")
	form.Set("time_on_page", "1000")
	form.Set("client_attribution_metadata[client_session_id]", clientCtx.ClientSessionID)
	form.Set("client_attribution_metadata[checkout_session_id]", sessionID)
	form.Set("client_attribution_metadata[merchant_integration_source]", "elements")
	form.Set("client_attribution_metadata[merchant_integration_subtype]", "payment-element")
	form.Set("client_attribution_metadata[merchant_integration_version]", "2021")
	form.Set("client_attribution_metadata[payment_intent_creation_flow]", "deferred")
	form.Set("client_attribution_metadata[payment_method_selection_flow]", "automatic")
	form.Set("client_attribution_metadata[elements_session_id]", clientCtx.ElementsSessionID)
	form.Set("client_attribution_metadata[elements_session_config_id]", firstNonEmpty(findStringField(pageBody, "config_id"), randomUUID()))
	form.Set("client_attribution_metadata[checkout_config_id]", firstNonEmpty(findStringField(pageBody, "config_id"), randomUUID()))
	form.Set("client_attribution_metadata[merchant_integration_additional_elements][0]", "payment")
	form.Set("client_attribution_metadata[merchant_integration_additional_elements][1]", "address")
	form.Set("guid", clientCtx.GUID)
	form.Set("muid", clientCtx.MUID)
	form.Set("sid", clientCtx.SID)
	form.Set("key", publishableKey)
	form.Set("_stripe_version", "2025-03-31.basil; checkout_server_update_beta=v1; checkout_manual_approval_preview=v1")
	return form
}

func stripePaymentPageDetailsQuery(publishableKey string, clientCtx stripeClientContext, pageBody any) url.Values {
	query := url.Values{}
	query.Set("elements_session_client[client_betas][0]", "custom_checkout_server_updates_1")
	query.Set("elements_session_client[client_betas][1]", "custom_checkout_manual_approval_1")
	query.Set("elements_session_client[elements_init_source]", "custom_checkout")
	query.Set("elements_session_client[referrer_host]", "chatgpt.com")
	query.Set("elements_session_client[session_id]", clientCtx.ElementsSessionID)
	query.Set("elements_session_client[stripe_js_id]", clientCtx.ClientSessionID)
	query.Set("elements_session_client[locale]", stripeLocale(pageBody))
	query.Set("elements_session_client[is_aggregation_expected]", "false")
	query.Set("elements_options_client[stripe_js_locale]", "auto")
	query.Set("elements_options_client[saved_payment_method][enable_save]", "auto")
	query.Set("elements_options_client[saved_payment_method][enable_redisplay]", "auto")
	query.Set("key", publishableKey)
	query.Set("_stripe_version", "2025-03-31.basil; checkout_server_update_beta=v1; checkout_manual_approval_preview=v1")
	return query
}

func stripeConfirmForm(pageBody any, sessionID string, publishableKey string, paymentMethodID string, clientCtx stripeClientContext) url.Values {
	form := url.Values{}
	form.Set("guid", clientCtx.GUID)
	form.Set("muid", clientCtx.MUID)
	form.Set("sid", clientCtx.SID)
	form.Set("payment_method", paymentMethodID)
	form.Set("init_checksum", findStringField(pageBody, "init_checksum"))
	form.Set("version", "332636417d")
	form.Set("expected_amount", "0")
	form.Set("expected_payment_method_type", "gopay")
	form.Set("return_url", firstNonEmpty(findStringField(pageBody, "stripe_hosted_url"), findStringField(pageBody, "return_url"), "https://chatgpt.com/checkout/verify?stripe_session_id="+url.QueryEscape(sessionID)))
	form.Set("elements_session_client[client_betas][0]", "custom_checkout_server_updates_1")
	form.Set("elements_session_client[client_betas][1]", "custom_checkout_manual_approval_1")
	form.Set("elements_session_client[elements_init_source]", "custom_checkout")
	form.Set("elements_session_client[referrer_host]", "chatgpt.com")
	form.Set("elements_session_client[session_id]", clientCtx.ElementsSessionID)
	form.Set("elements_session_client[stripe_js_id]", clientCtx.ClientSessionID)
	form.Set("elements_session_client[locale]", stripeLocale(pageBody))
	form.Set("elements_session_client[is_aggregation_expected]", "false")
	form.Set("elements_options_client[stripe_js_locale]", "auto")
	form.Set("elements_options_client[saved_payment_method][enable_save]", "auto")
	form.Set("elements_options_client[saved_payment_method][enable_redisplay]", "auto")
	form.Set("client_attribution_metadata[client_session_id]", clientCtx.ClientSessionID)
	form.Set("client_attribution_metadata[checkout_session_id]", sessionID)
	form.Set("client_attribution_metadata[merchant_integration_source]", "checkout")
	form.Set("client_attribution_metadata[merchant_integration_version]", "custom")
	form.Set("client_attribution_metadata[merchant_integration_subtype]", "payment-element")
	form.Set("client_attribution_metadata[merchant_integration_additional_elements][0]", "payment")
	form.Set("client_attribution_metadata[merchant_integration_additional_elements][1]", "address")
	form.Set("client_attribution_metadata[payment_intent_creation_flow]", "deferred")
	form.Set("client_attribution_metadata[payment_method_selection_flow]", "automatic")
	form.Set("client_attribution_metadata[elements_session_id]", clientCtx.ElementsSessionID)
	form.Set("client_attribution_metadata[elements_session_config_id]", firstNonEmpty(findStringField(pageBody, "config_id"), randomUUID()))
	form.Set("client_attribution_metadata[checkout_config_id]", firstNonEmpty(findStringField(pageBody, "config_id"), randomUUID()))
	form.Set("key", publishableKey)
	form.Set("_stripe_version", "2025-03-31.basil; checkout_server_update_beta=v1; checkout_manual_approval_preview=v1")
	return form
}

func doStripeFormRequest(client *http.Client, httpReq *http.Request, sessionID string, publishableKey string, label string) stripeInitResult {
	startedAt := time.Now()
	stage := "stripe_" + strings.ReplaceAll(label, " ", "_")
	endpoint := ""
	method := ""
	if httpReq != nil {
		method = httpReq.Method
		if httpReq.URL != nil {
			endpoint = httpReq.URL.String()
		}
	}
	resp, err := doHTTPRequestWithRetry(httpReq.Context(), client, httpReq)
	if err != nil {
		return stripeInitResult{OK: false, Error: "stripe " + label + " request failed: " + err.Error(), Stage: stage, Method: method, Endpoint: endpoint, CheckoutSessionID: sessionID, PublishableKey: publishableKey, ElapsedMS: time.Since(startedAt).Milliseconds()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return stripeInitResult{OK: false, Error: "failed to read stripe " + label + " response: " + err.Error(), Stage: stage, Method: method, Endpoint: endpoint, Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), CheckoutSessionID: sessionID, PublishableKey: publishableKey, ElapsedMS: time.Since(startedAt).Milliseconds()}
	}

	result := stripeInitResult{
		OK:                resp.StatusCode >= 200 && resp.StatusCode < 300,
		Stage:             stage,
		Method:            method,
		Endpoint:          endpoint,
		Status:            resp.StatusCode,
		ContentType:       resp.Header.Get("Content-Type"),
		CheckoutSessionID: sessionID,
		PublishableKey:    publishableKey,
		ElapsedMS:         time.Since(startedAt).Milliseconds(),
	}
	if strings.Contains(strings.ToLower(result.ContentType), "application/json") {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			result.Body = parsed
			return result
		}
	}
	result.RawBody = string(body)
	return result
}

func getGopayRedirect(ctx context.Context, client *http.Client, redirectURL string) gopayRedirectResult {
	redirectURL = strings.TrimSpace(redirectURL)
	if !strings.HasPrefix(redirectURL, "https://pm-redirects.stripe.com/") {
		return gopayRedirectResult{
			OK:          false,
			Error:       "redirect url is not a pm-redirects.stripe.com url",
			RedirectURL: redirectURL,
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, redirectURL, nil)
	if err != nil {
		return gopayRedirectResult{OK: false, Error: "failed to create gopay redirect request: " + err.Error(), RedirectURL: redirectURL}
	}
	setGopayRedirectHeaders(httpReq)

	redirectClient := &http.Client{
		Transport: client.Transport,
		Timeout:   client.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, redirectClient, httpReq)
	if err != nil {
		return gopayRedirectResult{OK: false, Error: "gopay redirect request failed: " + err.Error(), RedirectURL: redirectURL}
	}
	defer resp.Body.Close()

	location := strings.TrimSpace(resp.Header.Get("Location"))
	result := gopayRedirectResult{
		Status:      resp.StatusCode,
		RedirectURL: redirectURL,
		Location:    location,
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("gopay redirect expected 3xx status, got %d", resp.StatusCode)
		return result
	}
	if !strings.HasPrefix(location, "https://app.midtrans.com/snap/v4/redirection/") {
		result.Error = "gopay redirect location is not a Midtrans snap redirection url"
		return result
	}

	guid, err := lastURLPathSegment(location)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.OK = true
	result.GUID = guid
	return result
}

func gopayRequestDiagnostics(countryCode string, phoneNumber string, otpChannel string) map[string]any {
	countryCode = strings.TrimSpace(countryCode)
	phoneNumber = strings.TrimSpace(phoneNumber)
	otpChannel = strings.TrimSpace(otpChannel)
	normalizedChannel, channelErr := normalizeOTPChannel(otpChannel)
	digitsOnly := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phoneNumber)
	return map[string]any{
		"country_code":           countryCode,
		"phone_number":           phoneNumber,
		"otp_channel":            otpChannel,
		"normalized_otp_channel": normalizedChannel,
		"otp_channel_error": func() string {
			if channelErr != nil {
				return channelErr.Error()
			}
			return ""
		}(),
		"country_has_plus":         strings.HasPrefix(countryCode, "+"),
		"country_digits_only":      strings.TrimPrefix(countryCode, "+"),
		"phone_digits_only":        digitsOnly,
		"phone_has_plus":           strings.HasPrefix(phoneNumber, "+"),
		"phone_has_country_prefix": strings.HasPrefix(digitsOnly, strings.TrimPrefix(countryCode, "+")) && strings.TrimPrefix(countryCode, "+") != "",
	}
}

func gopayLinkingDiagnostics(resolution gopayLinkingResolution) map[string]any {
	return map[string]any{
		"reused_existing":     resolution.ReusedExisting,
		"conflict_reason":     resolution.ConflictReason,
		"link_http_status":    resolution.LinkHTTPStatus,
		"link_error":          resolution.LinkError,
		"link_error_messages": resolution.LinkErrorMessage,
		"token_not_found":     resolution.TokenNotFound,
		"account_source":      resolution.AccountSource,
		"account_origin_url":  resolution.AccountOriginURL,
		"account_source_note": resolution.AccountSourceNote,
		"account_status":      stringifyJSONValue(resolution.AccountResult["account_status"]),
		"reference_id":        stringifyJSONValue(resolution.LinkResult["reference_id"]),
	}
}

func gopayTokenNotFound(result map[string]any, reqErr error) bool {
	candidates := gopayLinkingErrorMessages(result)
	if reqErr != nil {
		candidates = append(candidates, reqErr.Error())
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), "token not found") {
			return true
		}
	}
	return false
}

func validateGopayReferenceViaLocalMock(ctx context.Context, referenceID string) (map[string]any, error) {
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return nil, errors.New("gopay reference_id is required")
	}

	payload, err := json.Marshal(map[string]any{
		"reference_id": referenceID,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/validate-reference", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayReferenceHeaders(httpReq)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("local gopay reference validation failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	result["reference_id"] = referenceID
	return result, nil
}

func createGopayLinkingViaLocalMock(ctx context.Context, accountID string, link gopayLink) (map[string]any, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, errors.New("gopay account id is required")
	}
	countryCode := strings.TrimSpace(link.CountryCode)
	phoneNumber := strings.TrimSpace(link.PhoneNumber)
	if countryCode == "" {
		return nil, errors.New("gopay_link.country_code is required")
	}
	if phoneNumber == "" {
		return nil, errors.New("gopay_link.phone_number is required")
	}

	payload, err := json.Marshal(map[string]any{
		"type":         firstNonEmpty(strings.TrimSpace(link.Type), "gopay"),
		"country_code": countryCode,
		"phone_number": phoneNumber,
	})
	if err != nil {
		return nil, err
	}

	linkingURL := "https://app.midtrans.com/snap/v3/accounts/" + url.PathEscape(accountID) + "/linking"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, linkingURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayLinkingHeaders(httpReq, accountID)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"stage":      "gopay_midtrans_linking",
		"method":     http.MethodPost,
		"endpoint":   linkingURL,
		"status":     resp.StatusCode,
		"account_id": accountID,
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err == nil {
			for key, value := range parsed {
				result[key] = value
			}
		}
	} else {
		result["raw_body"] = string(body)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("local gopay linking failed with status %d: %s", resp.StatusCode, string(body))
	}

	referenceID, err := referenceFromActivationLink(stringifyJSONValue(result["activation_link_url"]))
	if err != nil {
		return result, err
	}
	result["reference_id"] = referenceID
	return result, nil
}

func gopayLinkingErrorMessages(result map[string]any) []string {
	if len(result) == 0 {
		return nil
	}
	raw, ok := result["error_messages"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(stringifyJSONValue(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text != "" {
			return []string{text}
		}
	}
	return nil
}

func gopayLinkingAlreadyLinked(result map[string]any, reqErr error) bool {
	candidates := gopayLinkingErrorMessages(result)
	if reqErr != nil {
		candidates = append(candidates, reqErr.Error())
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), "account already linked") {
			return true
		}
	}
	return false
}

func validateReusableGopayAccount(result map[string]any) error {
	if stringifyJSONValue(result["account_status"]) != "ENABLED" {
		return errors.New("gopay reusable account_status is not ENABLED")
	}
	return nil
}

func createOrReuseGopayLinkingViaLocalMock(ctx context.Context, accountID string, link gopayLink) (gopayLinkingResolution, error) {
	linkResult, linkErr := createGopayLinkingViaLocalMock(ctx, accountID, link)
	resolution := gopayLinkingResolution{LinkResult: linkResult}
	if linkErr != nil {
		resolution.LinkError = linkErr.Error()
		if status, ok := jsonNumberToInt(linkResult["status"]); ok {
			resolution.LinkHTTPStatus = int(status)
		}
		resolution.LinkErrorMessage = gopayLinkingErrorMessages(linkResult)
		resolution.TokenNotFound = gopayTokenNotFound(linkResult, linkErr)
	}
	if linkErr == nil {
		return resolution, nil
	}
	if !gopayLinkingAlreadyLinked(linkResult, linkErr) {
		return resolution, linkErr
	}

	accountResult, accountErr := getGopayAccountDetailsViaLocalMock(ctx, accountID)
	resolution.AccountResult = accountResult
	resolution.ReusedExisting = true
	resolution.ConflictReason = "account already linked"
	if accountErr != nil {
		return resolution, fmt.Errorf("gopay linking conflict detected but account lookup failed: %w", accountErr)
	}
	if err := validateReusableGopayAccount(accountResult); err != nil {
		return resolution, fmt.Errorf("gopay linking conflict detected but current account cannot be reused: %w", err)
	}
	return resolution, nil
}

func gopayReuseStages(accountResult map[string]any) []map[string]any {
	status := firstNonEmpty(stringifyJSONValue(accountResult["account_status"]), "ENABLED")
	return []map[string]any{
		{"name": "force-link-api", "ok": true, "message": "Midtrans 返回 account already linked，改为复用已绑定账号"},
		{"name": "validate-reference", "ok": true, "message": "已绑定账号，跳过 reference 校验"},
		{"name": "user-consent", "ok": true, "message": "已绑定账号，跳过用户授权"},
		{"name": "otp-enum", "ok": true, "message": "已绑定账号，跳过 OTP 验证"},
		{"name": "pin-enum", "ok": true, "message": "已绑定账号，跳过 PIN 验证"},
		{"name": "validate-pin", "ok": true, "message": "复用成功，当前账号状态 " + status},
	}
}

func gopayPaymentPINCandidates(preferred string) []string {
	defaults := []string{"123456", "111111", "000000", "654321", "145236"}
	seen := map[string]bool{}
	candidates := make([]string, 0, len(defaults)+1)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		candidates = append(candidates, value)
	}
	add(preferred)
	for _, value := range defaults {
		add(value)
	}
	return candidates
}

func completeGopayPaymentViaLocalMock(ctx context.Context, gopayGUID string, preferredPIN string) (gopayPaymentResolution, error) {
	result := gopayPaymentResolution{}

	gopayCharge, err := chargeMidtransGopayViaLocalMock(ctx, gopayGUID)
	result.Charge = gopayCharge
	if err != nil {
		return result, err
	}

	verificationLink := stringifyJSONValue(gopayCharge["gopay_verification_link_url"])
	if verificationLink == "" {
		return result, errors.New("midtrans charge did not return gopay_verification_link_url")
	}

	paymentReferenceID, err := referenceFromGopayVerificationLink(verificationLink)
	if err != nil {
		return result, err
	}
	result.PaymentReferenceID = paymentReferenceID

	gopayPaymentValidate, err := validateGopayPaymentViaLocalMock(ctx, paymentReferenceID)
	result.PaymentValidate = gopayPaymentValidate
	if err != nil {
		return result, err
	}

	gopayPaymentConfirm, err := confirmGopayPaymentViaLocalMock(ctx, paymentReferenceID)
	result.PaymentConfirm = gopayPaymentConfirm
	if err != nil {
		return result, err
	}

	result.PaymentChallengeID = gopayPaymentChallengeID(gopayPaymentConfirm)
	result.PaymentClientID = gopayPaymentClientID(gopayPaymentConfirm)
	for _, candidate := range gopayPaymentPINCandidates(preferredPIN) {
		result.PinTried = append(result.PinTried, candidate)
		paymentPINTokenResult, pinErr := requestGopayPaymentPINTokenViaLocalMock(ctx, result.PaymentChallengeID, result.PaymentClientID, candidate)
		if pinErr != nil {
			continue
		}
		pinToken := gopayPINToken(paymentPINTokenResult)
		if pinToken == "" {
			continue
		}
		result.PaymentPINToken = paymentPINTokenResult
		result.PinUsed = candidate

		gopayPaymentProcess, processErr := processGopayPaymentViaLocalMock(ctx, paymentReferenceID, pinToken)
		result.PaymentProcess = gopayPaymentProcess
		if processErr != nil {
			return result, processErr
		}

		transactionID, txErr := transactionIDFromRedirectURL(findStringField(gopayPaymentProcess, "redirect_url"))
		if txErr != nil {
			return result, txErr
		}
		result.TransactionID = transactionID

		midtransStatus, statusErr := getMidtransTransactionStatusViaLocalMock(ctx, transactionID)
		result.MidtransStatus = midtransStatus
		if statusErr != nil {
			return result, statusErr
		}
		return result, nil
	}

	return result, errors.New("所有支付 PIN 候选码均失败")
}

func gopayReuseStagesWithPayment(accountResult map[string]any, payment gopayPaymentResolution) []map[string]any {
	status := firstNonEmpty(stringifyJSONValue(accountResult["account_status"]), "ENABLED")
	pinLabel := firstNonEmpty(payment.PinUsed, "已校验")
	return []map[string]any{
		{"name": "force-link-api", "ok": true, "message": "Midtrans 返回 account already linked，改为复用已绑定账号"},
		{"name": "validate-reference", "ok": true, "message": "已绑定账号，跳过 reference 校验"},
		{"name": "user-consent", "ok": true, "message": "已绑定账号，直接进入支付阶段"},
		{"name": "otp-enum", "ok": true, "message": "已绑定账号，跳过 OTP 验证"},
		{"name": "pin-enum", "ok": true, "message": "支付 PIN 通过 (" + pinLabel + ")"},
		{"name": "validate-pin", "ok": true, "message": "支付完成，当前账号状态 " + status},
	}
}

func gopayReuseStagesWithPaymentFailure(accountResult map[string]any, payment gopayPaymentResolution, err error) []map[string]any {
	stages := gopayReuseStages(accountResult)
	for _, stage := range stages {
		name := stringifyJSONValue(stage["name"])
		switch name {
		case "user-consent":
			stage["message"] = "已绑定账号，直接进入支付阶段"
		case "pin-enum":
			if payment.PinUsed != "" {
				stage["ok"] = true
				stage["message"] = "支付 PIN 通过 (" + payment.PinUsed + ")"
			} else {
				stage["ok"] = false
				stage["tried"] = payment.PinTried
				stage["error"] = err.Error()
				delete(stage, "message")
			}
		case "validate-pin":
			stage["ok"] = false
			stage["message"] = "支付阶段失败: " + err.Error()
		}
	}
	return stages
}

func defaultGopayFullLinkPaymentDeps() gopayFullLinkPaymentDeps {
	return gopayFullLinkPaymentDeps{completePayment: completeGopayPaymentViaLocalMock}
}

func appendGopayFullLinkStage(response map[string]any, name string, data map[string]any) {
	data["name"] = name
	stages, _ := response["stages"].([]map[string]any)
	response["stages"] = append(stages, data)
}

func attachGopayPaymentFields(response map[string]any, payment gopayPaymentResolution) {
	response["gopay_charge"] = payment.Charge
	response["gopay_payment_reference_id"] = payment.PaymentReferenceID
	response["gopay_payment_validate"] = payment.PaymentValidate
	response["gopay_payment_confirm"] = payment.PaymentConfirm
	response["gopay_payment_challenge_id"] = payment.PaymentChallengeID
	response["gopay_payment_client_id"] = payment.PaymentClientID
	response["gopay_payment_pin_token"] = payment.PaymentPINToken
	response["gopay_payment_process"] = payment.PaymentProcess
	response["midtrans_status"] = payment.MidtransStatus
	if voucher := buildGopayPaymentVoucher(payment); len(voucher) > 0 {
		response["payment_voucher"] = voucher
	}
}

func buildGopayPaymentVoucher(payment gopayPaymentResolution) map[string]any {
	voucher := make(map[string]any)
	if payment.PaymentReferenceID != "" {
		voucher["payment_reference_id"] = payment.PaymentReferenceID
		voucher["gopay_payment_reference_id"] = payment.PaymentReferenceID
	}
	if payment.TransactionID != "" {
		voucher["transaction_id"] = payment.TransactionID
	}
	if payment.PinUsed != "" {
		voucher["payment_pin"] = payment.PinUsed
	}
	copyPaymentVoucherFields(voucher, payment.Charge)
	copyPaymentVoucherFields(voucher, payment.MidtransStatus)
	if len(voucher) == 0 {
		return nil
	}
	return voucher
}

func completeGopayFullLinkPayment(ctx context.Context, response map[string]any, accountID, countryCode, phoneNumber, preferredPIN string, resolution gopayLinkingResolution, deps gopayFullLinkPaymentDeps) {
	if deps.completePayment == nil {
		deps = defaultGopayFullLinkPaymentDeps()
	}
	paymentResult, paymentErr := deps.completePayment(ctx, accountID, preferredPIN)
	attachGopayPaymentFields(response, paymentResult)
	response["account_id"] = accountID

	if paymentErr != nil {
		response["ok"] = false
		response["stage"] = "gopay_payment_failed"
		if paymentResult.PinUsed == "" && len(paymentResult.PinTried) > 0 {
			response["stage"] = "payment_pin_all_failed"
		}
		response["error"] = paymentErr.Error()
		appendGopayFullLinkStage(response, "pin-enum", map[string]any{
			"ok":    paymentResult.PinUsed != "",
			"pin":   paymentResult.PinUsed,
			"tried": paymentResult.PinTried,
			"error": paymentErr.Error(),
		})
		appendGopayFullLinkStage(response, "validate-pin", map[string]any{
			"ok":      false,
			"message": "支付阶段失败: " + paymentErr.Error(),
		})
		return
	}

	linkingReferenceID := stringifyJSONValue(response["reference_id"])
	if linkingReferenceID != "" {
		response["linking_reference_id"] = linkingReferenceID
	}
	response["ok"] = true
	response["stage"] = "gopay_complete"
	response["reference_id"] = paymentResult.PaymentReferenceID
	appendGopayFullLinkStage(response, "pin-enum", map[string]any{
		"ok":      true,
		"pin":     firstNonEmpty(paymentResult.PinUsed, preferredPIN),
		"message": "支付 PIN 通过 (" + firstNonEmpty(paymentResult.PinUsed, preferredPIN, "已校验") + ")",
	})
	appendGopayFullLinkStage(response, "validate-pin", map[string]any{
		"ok":      true,
		"message": "支付完成",
	})

	summary := map[string]any{
		"account_id":           accountID,
		"phone_number":         phoneNumber,
		"country_code":         countryCode,
		"linking_reference_id": linkingReferenceID,
		"reference_id":         paymentResult.PaymentReferenceID,
		"payment_reference_id": paymentResult.PaymentReferenceID,
		"transaction_id":       paymentResult.TransactionID,
		"payment_pin":          paymentResult.PinUsed,
		"gopay_linked":         true,
	}
	if resolution.AccountResult != nil {
		summary["account_status"] = stringifyJSONValue(resolution.AccountResult["account_status"])
	}
	response["summary"] = summary
}

func requestGopayUserConsentViaLocalMock(ctx context.Context, referenceID string, otpChannel string) (map[string]any, error) {
	payload, normalizedChannel, err := gopayUserConsentPayload(referenceID, otpChannel)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/user-consent", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayReferenceHeaders(httpReq)
	httpReq.Header.Set("X-User-Locale", "id_ID")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("local gopay user consent failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	result["reference_id"] = strings.TrimSpace(referenceID)
	result["otp_channel"] = normalizedChannel
	return result, nil
}

func gopayUserConsentPayload(referenceID string, otpChannel string) ([]byte, string, error) {
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return nil, "", errors.New("reference_id is required for gopay user consent")
	}
	normalizedChannel, err := normalizeOTPChannel(otpChannel)
	if err != nil {
		return nil, "", err
	}

	payload, err := json.Marshal(map[string]any{
		"reference_id": referenceID,
		"otp_channel":  normalizedChannel,
	})
	if err != nil {
		return nil, "", err
	}
	return payload, normalizedChannel, nil
}

func normalizeOTPChannel(otpChannel string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(otpChannel)) {
	case "":
		return "whatsapp", nil
	case "sms":
		return "sms", nil
	case "wa", "whatsapp", "whats_app", "whats-app":
		return "whatsapp", nil
	default:
		return "", fmt.Errorf("unsupported gopay otp_channel %q", otpChannel)
	}
}

func validateGopayReferenceResult(result map[string]any) error {
	success, ok := result["success"].(bool)
	if !ok || !success {
		return errors.New("gopay reference success is not true")
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		return errors.New("gopay reference missing data")
	}
	if stringifyJSONValue(data["next_action"]) != "linking-user-consent" {
		return errors.New("gopay reference next_action is not linking-user-consent")
	}
	return nil
}

func validateGopayUserConsentResult(result map[string]any) error {
	success, ok := result["success"].(bool)
	if !ok || !success {
		return errors.New("gopay user consent success is not true")
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		return errors.New("gopay user consent missing data")
	}
	if stringifyJSONValue(data["next_action"]) != "linking-validate-otp" {
		return errors.New("gopay user consent next_action is not linking-validate-otp")
	}
	return nil
}

func validateGopayOTPViaLocalMock(ctx context.Context, referenceID string, otp string) (map[string]any, error) {
	referenceID = strings.TrimSpace(referenceID)
	otp = strings.TrimSpace(otp)
	if referenceID == "" {
		return nil, errors.New("reference_id is required for gopay otp validation")
	}
	if otp == "" {
		return nil, errors.New("gopay_link.otp is required")
	}

	payload, err := json.Marshal(map[string]any{
		"reference_id": referenceID,
		"otp":          otp,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/validate-otp", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayReferenceHeaders(httpReq)
	httpReq.Header.Set("X-User-Locale", "id_ID")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	result["reference_id"] = referenceID
	result["http_status"] = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("local gopay otp validation failed with status %d", resp.StatusCode)
	}
	return result, nil
}

func validateGopayOTPResult(result map[string]any) error {
	success, ok := result["success"].(bool)
	if !ok || !success {
		return errors.New("gopay otp success is not true")
	}
	if gopayOTPChallengeID(result) == "" {
		return errors.New("gopay otp missing challenge_id")
	}
	return nil
}

func gopayOTPChallengeID(result map[string]any) string {
	value := nestedMap(result, "data", "challenge", "action", "value")
	return stringifyJSONValue(value["challenge_id"])
}

func gopayPaymentChallengeID(result map[string]any) string {
	value := nestedMap(result, "data", "challenge", "action", "value")
	return stringifyJSONValue(value["challenge_id"])
}

func gopayPaymentClientID(result map[string]any) string {
	value := nestedMap(result, "data", "challenge", "action", "value")
	return stringifyJSONValue(value["client_id"])
}

func requestGopayPINTokenViaLocalMock(ctx context.Context, challengeID string, pin string) (map[string]any, error) {
	challengeID = strings.TrimSpace(challengeID)
	pin = strings.TrimSpace(pin)
	if challengeID == "" {
		return nil, errors.New("gopay otp challenge_id is required")
	}
	if pin == "" {
		return nil, errors.New("gopay_link.pin is required")
	}

	payload, err := json.Marshal(map[string]any{
		"challenge_id": challengeID,
		"client_id":    "51b5f09a-3813-11ee-be56-0242ac120002-MGUPA",
		"pin":          pin,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://customer.gopayapi.com/api/v1/users/pin/tokens/nb", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayPINHeaders(httpReq)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("local gopay pin token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	result["http_status"] = resp.StatusCode
	return result, nil
}

func requestGopayPaymentPINTokenViaLocalMock(ctx context.Context, challengeID string, clientID string, pin string) (map[string]any, error) {
	challengeID = strings.TrimSpace(challengeID)
	clientID = strings.TrimSpace(clientID)
	pin = strings.TrimSpace(pin)
	if challengeID == "" {
		return nil, errors.New("gopay payment challenge_id is required")
	}
	if clientID == "" {
		return nil, errors.New("gopay payment client_id is required")
	}
	if pin == "" {
		return nil, errors.New("gopay_link.pin is required")
	}

	payload, err := json.Marshal(map[string]any{
		"pin":          pin,
		"challenge_id": challengeID,
		"client_id":    clientID,
	})
	if err != nil {
		return nil, err
	}

	tokenURL := gopayCustomerMockURL("/api/v1/users/pin/tokens/nb")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayPaymentPINHeaders(httpReq)

	client := localMockHTTPClient()
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result, err := readJSONResponseMap(resp, "gopay_payment_pin_token")
	if err != nil {
		return result, err
	}
	if success, ok := result["success"].(bool); !ok || !success {
		return result, errors.New("gopay payment pin token success is not true")
	}
	if gopayPINToken(result) == "" {
		return result, errors.New("gopay payment pin token missing token")
	}
	result["challenge_id"] = challengeID
	result["client_id"] = clientID
	return result, nil
}

func setLocalGopayPINHeaders(r *http.Request) {
	r.Header.Set("Accept", "application/json, text/plain, */*")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://pin-web-client.gopayapi.com")
	r.Header.Set("Priority", "u=1, i")
	r.Header.Set("Referer", "https://pin-web-client.gopayapi.com/")
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-site")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
	r.Header.Set("X-Appversion", "1.0.0")
	r.Header.Set("X-Correlation-ID", randomUUID())
	r.Header.Set("X-Is-Mobile", "false")
	r.Header.Set("X-Platform", "Windows 10")
	r.Header.Set("X-Request-ID", randomUUID())
	r.Header.Set("X-User-Locale", "id")
}

func setLocalGopayPaymentPINHeaders(r *http.Request) {
	r.Header.Set("Accept", "application/json, text/plain, */*")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://merchants-gws-app.gopayapi.com")
	r.Header.Set("Priority", "u=1, i")
	r.Header.Set("Referer", "https://merchants-gws-app.gopayapi.com/")
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-site")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
	r.Header.Set("X-Request-ID", randomUUID())
}

func gopayPINToken(result map[string]any) string {
	if token := stringifyJSONValue(result["token"]); token != "" {
		return token
	}
	if token := stringifyJSONValue(result["pin_token"]); token != "" {
		return token
	}
	if token := findStringField(result, "token", "pin_token"); token != "" {
		return token
	}
	return ""
}

func validateGopayPINViaLocalMock(ctx context.Context, referenceID string, token string) (map[string]any, error) {
	referenceID = strings.TrimSpace(referenceID)
	token = strings.TrimSpace(token)
	if referenceID == "" {
		return nil, errors.New("reference_id is required for gopay validate-pin")
	}
	if token == "" {
		return nil, errors.New("gopay pin token is required")
	}

	payload, err := json.Marshal(map[string]any{
		"reference_id": referenceID,
		"token":        token,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://gwa.gopayapi.com/v1/linking/validate-pin", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayReferenceHeaders(httpReq)
	httpReq.Header.Set("X-User-Locale", "id_ID")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	result["reference_id"] = referenceID
	result["http_status"] = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("local gopay validate-pin failed with status %d", resp.StatusCode)
	}
	return result, nil
}

func validateGopayPINResult(result map[string]any) error {
	success, ok := result["success"].(bool)
	if !ok || !success {
		return errors.New("gopay validate-pin success is not true")
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		return errors.New("gopay validate-pin missing data")
	}
	if stringifyJSONValue(data["next_action"]) != "linking-success" {
		return errors.New("gopay validate-pin next_action is not linking-success")
	}
	return nil
}

func chargeMidtransGopayViaLocalMock(ctx context.Context, transactionID string) (map[string]any, error) {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return nil, errors.New("gopay transaction id is required")
	}

	payload, err := json.Marshal(map[string]any{
		"payment_type":  "gopay",
		"tokenization":  "true",
		"promo_details": nil,
	})
	if err != nil {
		return nil, err
	}

	chargeURL := midtransMockURL("/snap/v2/transactions/" + url.PathEscape(transactionID) + "/charge")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chargeURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayChargeHeaders(httpReq, transactionID)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"stage":          "midtrans_gopay_charge",
		"method":         http.MethodPost,
		"endpoint":       chargeURL,
		"status":         resp.StatusCode,
		"transaction_id": transactionID,
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err == nil {
			for key, value := range parsed {
				result[key] = value
			}
		}
	} else {
		result["raw_body"] = string(body)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("local midtrans gopay charge failed with status %d: %s", resp.StatusCode, string(body))
	}
	return result, nil
}

func validateGopayPaymentViaLocalMock(ctx context.Context, referenceID string) (map[string]any, error) {
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return nil, errors.New("gopay payment reference_id is required")
	}

	validateURL := gopayGWAMockURL("/v1/payment/validate") + "?reference_id=" + url.QueryEscape(referenceID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, validateURL, nil)
	if err != nil {
		return nil, err
	}
	setLocalGopayReferenceHeaders(httpReq)
	httpReq.Header.Del("Content-Type")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"stage":        "gopay_payment_validate",
		"method":       http.MethodGet,
		"endpoint":     validateURL,
		"status":       resp.StatusCode,
		"reference_id": referenceID,
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err == nil {
			for key, value := range parsed {
				result[key] = value
			}
		}
	} else {
		result["raw_body"] = string(body)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("local gopay payment validate failed with status %d: %s", resp.StatusCode, string(body))
	}
	if success, ok := result["success"].(bool); !ok || !success {
		return result, errors.New("gopay payment validate success is not true")
	}
	return result, nil
}

func confirmGopayPaymentViaLocalMock(ctx context.Context, referenceID string) (map[string]any, error) {
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return nil, errors.New("gopay payment reference_id is required")
	}

	payload, err := json.Marshal(map[string]any{
		"payment_instructions": []any{},
	})
	if err != nil {
		return nil, err
	}

	confirmURL := gopayGWAMockURL("/v1/payment/confirm") + "?reference_id=" + url.QueryEscape(referenceID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, confirmURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayReferenceHeaders(httpReq)

	client := localMockHTTPClient()
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result, err := readJSONResponseMap(resp, "gopay_payment_confirm")
	if err != nil {
		return result, err
	}
	if success, ok := result["success"].(bool); !ok || !success {
		return result, errors.New("gopay payment confirm success is not true")
	}
	if gopayPaymentChallengeID(result) == "" {
		return result, errors.New("gopay payment confirm missing challenge_id")
	}
	return result, nil
}

func processGopayPaymentViaLocalMock(ctx context.Context, referenceID string, pinToken string) (map[string]any, error) {
	referenceID = strings.TrimSpace(referenceID)
	pinToken = strings.TrimSpace(pinToken)
	if referenceID == "" {
		return nil, errors.New("gopay payment reference_id is required")
	}
	if pinToken == "" {
		return nil, errors.New("gopay payment pin_token is required")
	}

	payload, err := json.Marshal(map[string]any{
		"challenge": map[string]any{
			"type": "GOPAY_PIN_CHALLENGE",
			"value": map[string]any{
				"pin_token": pinToken,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	processURL := gopayGWAMockURL("/v1/payment/process") + "?reference_id=" + url.QueryEscape(referenceID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, processURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setLocalGopayReferenceHeaders(httpReq)

	client := localMockHTTPClient()
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result, err := readJSONResponseMap(resp, "gopay_payment_process")
	if err != nil {
		return result, err
	}
	if success, ok := result["success"].(bool); !ok || !success {
		return result, errors.New("gopay payment process success is not true")
	}
	data, _ := result["data"].(map[string]any)
	if stringifyJSONValue(data["next_action"]) != "payment-success" {
		return result, errors.New("gopay payment process next_action is not payment-success")
	}
	return result, nil
}

func getMidtransTransactionStatusViaLocalMock(ctx context.Context, transactionID string) (map[string]any, error) {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" {
		return nil, errors.New("midtrans transaction id is required")
	}

	statusURL := midtransMockURL("/snap/v1/transactions/" + url.PathEscape(transactionID) + "/status")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	setLocalGopayChargeHeaders(httpReq, transactionID)
	httpReq.Header.Del("Content-Type")

	client := localMockHTTPClient()
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	result, err := readJSONResponseMap(resp, "midtrans_transaction_status")
	if err != nil {
		return result, err
	}
	return result, nil
}

func getGopayAccountDetailsViaLocalMock(ctx context.Context, accountID string) (map[string]any, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, errors.New("gopay account id is required")
	}

	accountURL := "https://app.midtrans.com/snap/v3/accounts/" + url.PathEscape(accountID) + "/gopay"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, accountURL, nil)
	if err != nil {
		return nil, err
	}
	setLocalGopayAccountHeaders(httpReq, accountID)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		},
	}
	resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("local gopay account details failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	result["account_id"] = accountID
	result["http_status"] = resp.StatusCode
	return result, nil
}

func setLocalGopayAccountHeaders(r *http.Request, accountID string) {
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Cookie", "preferredPayment-G761482587=gopay; locale=en")
	r.Header.Set("Referer", "https://app.midtrans.com/snap/v4/redirection/"+accountID)
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
	r.Header.Set("X-Source", "snap")
	r.Header.Set("X-Source-App-Type", "redirection")
	r.Header.Set("X-Source-Version", "2.3.0")
}

func setLocalGopayLinkingHeaders(r *http.Request, accountID string) {
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Authorization", configString("MIDTRANS_LINKING_AUTHORIZATION", config.MidtransLinkingAuthorization, "Basic TWlkLWNsaWVudC0zVFg4blVhLWZfUmdOcmt5Og=="))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Cookie", configString("MIDTRANS_LINKING_COOKIE", config.MidtransLinkingCookie, "preferredPayment-G761482587=gopay; locale=en"))
	r.Header.Set("Origin", "https://app.midtrans.com")
	r.Header.Set("Referer", "https://app.midtrans.com/snap/v4/redirection/"+accountID)
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
}

func setLocalGopayChargeHeaders(r *http.Request, transactionID string) {
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Cookie", configString("MIDTRANS_CHARGE_COOKIE", config.MidtransChargeCookie, configString("MIDTRANS_LINKING_COOKIE", config.MidtransLinkingCookie, "preferredPayment-G761482587=gopay; locale=en")))
	r.Header.Set("Origin", "https://app.midtrans.com")
	r.Header.Set("Priority", "u=1, i")
	r.Header.Set("Referer", "https://app.midtrans.com/snap/v4/redirection/"+transactionID)
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
	r.Header.Set("X-Source", "snap")
	r.Header.Set("X-Source-App-Type", "redirection")
	r.Header.Set("X-Source-Version", "2.3.0")
}

func validateGopayAccountDetails(result map[string]any) error {
	if stringifyJSONValue(result["account_status"]) != "ENABLED" {
		return errors.New("gopay account_status is not ENABLED")
	}
	balance, ok := jsonNumberToFloat(result["balance"])
	if !ok {
		return errors.New("gopay account balance is invalid")
	}
	if balance <= 1 {
		return errors.New("gopay account balance is not greater than 1")
	}
	return nil
}

func setLocalGopayReferenceHeaders(r *http.Request) {
	r.Header.Set("Accept", "application/json, text/plain, */*")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://merchants-gws-app.gopayapi.com")
	r.Header.Set("Priority", "u=1, i")
	r.Header.Set("Referer", "https://merchants-gws-app.gopayapi.com/")
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-site")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
}

func setGopayRedirectHeaders(r *http.Request) {
	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Priority", "u=0, i")
	r.Header.Set("Referer", "https://chatgpt.com/")
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("Upgrade-Insecure-Requests", "1")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
}

func lastURLPathSegment(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[len(parts)-1]) == "" {
		return "", errors.New("redirect location missing guid path segment")
	}
	return parts[len(parts)-1], nil
}

func referenceFromActivationLink(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("gopay activation_link_url is empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	referenceID := strings.TrimSpace(parsed.Query().Get("reference"))
	if referenceID == "" {
		return "", errors.New("gopay activation_link_url missing reference")
	}
	return referenceID, nil
}

func referenceFromGopayVerificationLink(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("gopay_verification_link_url is empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	referenceID := strings.TrimSpace(parsed.Query().Get("reference"))
	if referenceID == "" {
		return "", errors.New("gopay_verification_link_url missing reference")
	}
	return referenceID, nil
}

func transactionIDFromRedirectURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("payment redirect_url is empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[len(parts)-1]) == "" {
		return "", errors.New("payment redirect_url missing transaction id")
	}
	return strings.TrimSpace(parts[len(parts)-1]), nil
}

func localMockURL(path string) string {
	return baseURLWithPath(configString("LOCAL_MOCK_BASE_URL", config.LocalMockBaseURL, "http://localhost:8282"), path)
}

func midtransMockURL(path string) string {
	return baseURLWithPath(configString("MIDTRANS_MOCK_BASE_URL", config.MidtransMockBaseURL, configString("LOCAL_MOCK_BASE_URL", config.LocalMockBaseURL, "http://localhost:8282")), path)
}

func gopayGWAMockURL(path string) string {
	return baseURLWithPath(configString("GOPAY_GWA_MOCK_BASE_URL", config.GopayGWAMockBaseURL, configString("LOCAL_MOCK_BASE_URL", config.LocalMockBaseURL, "http://localhost:8282")), path)
}

func gopayCustomerMockURL(path string) string {
	return baseURLWithPath(configString("GOPAY_CUSTOMER_MOCK_BASE_URL", config.GopayCustomerMockBaseURL, configString("LOCAL_MOCK_BASE_URL", config.LocalMockBaseURL, "http://localhost:8282")), path)
}

func baseURLWithPath(base string, path string) string {
	base = strings.TrimRight(firstNonEmpty(base, "http://localhost:8282"), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func localMockHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func readJSONResponseMap(resp *http.Response, stage string) (map[string]any, error) {
	result := map[string]any{
		"stage":        stage,
		"status":       resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return result, err
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err == nil {
			for key, value := range parsed {
				result[key] = value
			}
		}
	} else {
		result["raw_body"] = string(body)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("%s failed with status %d: %s", stage, resp.StatusCode, string(body))
	}
	return result, nil
}

func requireStripeTestMode(sessionID string, publishableKey string) error {
	return nil
}

func setStripeInitHeaders(r *http.Request) {
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://js.stripe.com")
	r.Header.Set("Priority", "u=1, i")
	r.Header.Set("Referer", "https://js.stripe.com/")
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-site")
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0")
}

func findStringField(value any, names ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, name := range names {
			if raw, ok := typed[name]; ok {
				if found := stringifyJSONValue(raw); found != "" {
					return found
				}
			}
		}
		for _, raw := range typed {
			if found := findStringField(raw, names...); found != "" {
				return found
			}
		}
	case []any:
		for _, raw := range typed {
			if found := findStringField(raw, names...); found != "" {
				return found
			}
		}
	}
	return ""
}

func stringifyJSONValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func stripeOriginalError(value any) any {
	root, _ := value.(map[string]any)
	if root == nil {
		return nil
	}
	if stripeErr, ok := root["error"]; ok {
		return stripeErr
	}
	if submissionAttempt := nestedMap(root, "submission_attempt"); submissionAttempt != nil {
		if stripeErr, ok := submissionAttempt["error"]; ok {
			return stripeErr
		}
	}
	return nil
}

func stripeOriginalErrorCode(value any) string {
	original := stripeOriginalError(value)
	if original == nil {
		return ""
	}
	return findStringField(original, "code", "type")
}

func stripeOriginalErrorMessage(value any) string {
	original := stripeOriginalError(value)
	if original == nil {
		return ""
	}
	if message := findStringField(original, "message", "error", "detail"); message != "" {
		return message
	}
	if text := stringifyJSONValue(original); text != "" {
		return text
	}
	body, err := json.Marshal(original)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func nestedMap(root map[string]any, keys ...string) map[string]any {
	current := root
	for _, key := range keys {
		if current == nil {
			return nil
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func nestedMapFromAny(value any, keys ...string) map[string]any {
	root, _ := value.(map[string]any)
	return nestedMap(root, keys...)
}

func setFormFromMap(form url.Values, formKey string, values map[string]any, jsonKey string) {
	if values == nil {
		return
	}
	value := stringifyJSONValue(values[jsonKey])
	if value != "" {
		form.Set(formKey, value)
	}
}

func setFormValue(form url.Values, key string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		form.Set(key, value)
	}
}

func defaultTaxRegion() taxRegion {
	return taxRegion{
		Country:    "US",
		Line1:      "1208 Oakdale Street",
		City:       "Jonesboro",
		PostalCode: "72401",
		State:      "AR",
	}
}

func taxRegionFromMap(values map[string]any) taxRegion {
	if values == nil {
		return taxRegion{}
	}
	return taxRegion{
		Country:    strings.ToUpper(stringifyJSONValue(values["country"])),
		Line1:      stringifyJSONValue(values["line1"]),
		City:       stringifyJSONValue(values["city"]),
		PostalCode: stringifyJSONValue(values["postal_code"]),
		State:      strings.ToUpper(stringifyJSONValue(values["state"])),
	}
}

func mergeTaxRegion(base taxRegion, overlays ...taxRegion) taxRegion {
	merged := base
	for _, overlay := range overlays {
		if overlay.Country != "" {
			merged.Country = overlay.Country
		}
		if overlay.Line1 != "" {
			merged.Line1 = overlay.Line1
		}
		if overlay.City != "" {
			merged.City = overlay.City
		}
		if overlay.PostalCode != "" {
			merged.PostalCode = overlay.PostalCode
		}
		if overlay.State != "" {
			merged.State = overlay.State
		}
	}
	return merged
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func newStripeClientContext(initBody any) stripeClientContext {
	return stripeClientContext{
		GUID:              randomUUID(),
		MUID:              randomUUID(),
		SID:               randomUUID(),
		ClientSessionID:   randomUUID(),
		ElementsSessionID: stripeElementsSessionID(initBody),
	}
}

func stripeElementsSessionID(initBody any) string {
	if sessionID := findStringField(initBody, "elements_session_id", "elementsSessionId"); sessionID != "" {
		return sessionID
	}
	return "elements_session_" + randomHex(6)
}

func stripeLocale(initBody any) string {
	if locale := findStringField(initBody, "locale"); locale != "" {
		return locale
	}
	return "zh"
}

func checkStripeInitTotal(body any) stripeInitBodyCheck {
	root, ok := body.(map[string]any)
	if !ok {
		return stripeInitBodyCheck{Error: "stripe init response is not a json object"}
	}

	totalSummary, ok := root["total_summary"].(map[string]any)
	if !ok {
		return stripeInitBodyCheck{Error: "stripe init response missing total_summary"}
	}

	total, ok := jsonNumberToInt(totalSummary["total"])
	if !ok {
		return stripeInitBodyCheck{Error: "stripe init response missing total_summary.total"}
	}
	if total != 0 {
		return stripeInitBodyCheck{Error: fmt.Sprintf("当前会话应付金额为 %d，不是 0 元试用会话", total), Total: total}
	}

	return stripeInitBodyCheck{
		OK:    true,
		Total: total,
	}
}

func checkStripeConfirm(body any, expectedSessionID string) stripeConfirmCheck {
	root, ok := body.(map[string]any)
	if !ok {
		return stripeConfirmCheck{Error: "stripe confirm response is not a json object"}
	}
	if object := stringifyJSONValue(root["object"]); object != "checkout.session" {
		return stripeConfirmCheck{Error: "stripe confirm object is not checkout.session"}
	}
	if sessionID := stringifyJSONValue(root["session_id"]); sessionID != expectedSessionID {
		return stripeConfirmCheck{Error: "stripe confirm session_id does not match checkout session"}
	}

	totalSummary, ok := root["total_summary"].(map[string]any)
	if !ok {
		return stripeConfirmCheck{Error: "stripe confirm response missing total_summary"}
	}
	total, ok := jsonNumberToInt(totalSummary["total"])
	if !ok {
		return stripeConfirmCheck{Error: "stripe confirm response missing total_summary.total"}
	}
	if total != 0 {
		return stripeConfirmCheck{Error: "金额不为0"}
	}

	submissionAttempt, ok := root["submission_attempt"].(map[string]any)
	if !ok {
		return stripeConfirmCheck{Error: "stripe confirm response missing submission_attempt"}
	}
	if submissionAttempt["error"] != nil {
		return stripeConfirmCheck{Error: "stripe confirm submission_attempt has error"}
	}
	if state := stringifyJSONValue(submissionAttempt["state"]); state != "requires_approval" {
		return stripeConfirmCheck{Error: "stripe confirm submission_attempt state is not requires_approval"}
	}

	return stripeConfirmCheck{OK: true}
}

func checkStripeRedirectNextAction(body any) stripeRedirectCheck {
	root, ok := body.(map[string]any)
	if !ok {
		return stripeRedirectCheck{Error: "stripe payment details response is not a json object"}
	}

	source := "setup_intent.next_action"
	nextAction := nestedMap(root, "setup_intent", "next_action")
	if nextAction == nil {
		source = "payment_intent.next_action"
		nextAction = nestedMap(root, "payment_intent", "next_action")
	}
	if nextAction == nil {
		if nestedMap(root, "setup_intent") == nil && nestedMap(root, "payment_intent") == nil {
			if state := stringifyJSONValue(nestedMap(root, "submission_attempt")["state"]); state == "requires_approval" {
				return stripeRedirectCheck{Error: "stripe payment details has no setup_intent/payment_intent because checkout submission is still requires_approval"}
			}
			return stripeRedirectCheck{Error: "stripe payment details has no setup_intent/payment_intent"}
		}
		return stripeRedirectCheck{Error: "stripe payment details missing setup_intent.next_action and payment_intent.next_action"}
	}
	if actionType := stringifyJSONValue(nextAction["type"]); actionType != "redirect_to_url" {
		return stripeRedirectCheck{Error: "stripe " + source + " type is not redirect_to_url"}
	}

	redirect, _ := nextAction["redirect_to_url"].(map[string]any)
	if redirect == nil {
		return stripeRedirectCheck{Error: "stripe " + source + " missing redirect_to_url"}
	}
	redirectURL := stringifyJSONValue(redirect["url"])
	if !strings.HasPrefix(redirectURL, "https://pm-redirects.stripe.com/") {
		return stripeRedirectCheck{Error: "stripe " + source + " redirect url is not a pm-redirects.stripe.com url"}
	}

	return stripeRedirectCheck{
		OK:          true,
		Source:      source,
		RedirectURL: redirectURL,
	}
}

func waitForStripeRedirectNextAction(ctx context.Context, maxAttempts int, delay time.Duration, fetch func() stripeInitResult) (stripeInitResult, stripeRedirectCheck, []map[string]any) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastDetails stripeInitResult
	var lastCheck stripeRedirectCheck
	attempts := make([]map[string]any, 0, maxAttempts)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastDetails = fetch()
		summary := map[string]any{
			"attempt": attempt,
			"ok":      lastDetails.OK,
			"stage":   lastDetails.Stage,
			"status":  lastDetails.Status,
		}
		if !lastDetails.OK {
			summary["error"] = lastDetails.Error
			attempts = append(attempts, summary)
			return lastDetails, stripeRedirectCheck{Error: firstNonEmpty(lastDetails.Error, "stripe payment details request failed")}, attempts
		}

		lastCheck = checkStripeRedirectNextAction(lastDetails.Body)
		summary["redirect_ready"] = lastCheck.OK
		if !lastCheck.OK {
			summary["redirect_error"] = lastCheck.Error
		}
		if status := findStringField(lastDetails.Body, "payment_status", "status"); status != "" {
			summary["payment_status"] = status
		}
		attempts = append(attempts, summary)
		if lastCheck.OK {
			return lastDetails, lastCheck, attempts
		}
		if attempt < maxAttempts && delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return lastDetails, stripeRedirectCheck{Error: ctx.Err().Error()}, attempts
			case <-timer.C:
			}
		}
	}

	if lastCheck.Error == "" {
		lastCheck.Error = "stripe redirect next_action was not available"
	}
	lastCheck.Error = fmt.Sprintf("stripe redirect next_action was not available after %d attempts: %s", len(attempts), lastCheck.Error)
	return lastDetails, lastCheck, attempts
}

func jsonNumberToInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func jsonNumberToFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func handleProxyTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()
	var req proxyTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Proxy.Type = strings.ToLower(strings.TrimSpace(req.Proxy.Type))
	req.Proxy.URL = strings.TrimSpace(req.Proxy.URL)
	req.TargetURL = strings.TrimSpace(req.TargetURL)
	if err := validateProxy(req.Proxy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	targets, err := proxyTestTargets(req.TargetURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var attempts []proxyTestAttempt
	for _, targetURL := range targets {
		startedAt := time.Now()

		client, err := newHTTPClient(req.Proxy, shouldSkipTLSVerify(targetURL))
		if err != nil {
			attempts = append(attempts, proxyTestAttempt{
				TargetURL: targetURL,
				Error:     err.Error(),
				ElapsedMS: time.Since(startedAt).Milliseconds(),
			})
			continue
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			cancel()
			attempts = append(attempts, proxyTestAttempt{
				TargetURL: targetURL,
				Error:     "failed to create request",
				ElapsedMS: time.Since(startedAt).Milliseconds(),
			})
			continue
		}
		httpReq.Header.Set("User-Agent", "gopay2codex-proxy-test/1.0")

		resp, err := doHTTPRequestWithRetry(ctx, client, httpReq)
		if err != nil {
			cancel()
			attempts = append(attempts, proxyTestAttempt{
				TargetURL: targetURL,
				Error:     err.Error(),
				ElapsedMS: time.Since(startedAt).Milliseconds(),
			})
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		cancel()
		if readErr != nil {
			attempts = append(attempts, proxyTestAttempt{
				TargetURL: targetURL,
				Status:    resp.StatusCode,
				Error:     "failed to read response body",
				ElapsedMS: time.Since(startedAt).Milliseconds(),
			})
			continue
		}

		elapsed := time.Since(startedAt).Milliseconds()
		attempt := proxyTestAttempt{
			TargetURL: targetURL,
			Status:    resp.StatusCode,
			Body:      string(body),
			ElapsedMS: elapsed,
		}
		attempts = append(attempts, attempt)

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			writeJSON(w, http.StatusOK, proxyTestResponse{
				OK:        true,
				TargetURL: targetURL,
				Proxy:     summarizeProxy(req.Proxy),
				Status:    resp.StatusCode,
				Body:      string(body),
				ElapsedMS: elapsed,
				Attempts:  attempts,
			})
			return
		}
	}

	errorMessage := "proxy test failed on all targets"
	if len(attempts) > 0 && attempts[len(attempts)-1].Error != "" {
		errorMessage = "proxy test failed: " + attempts[len(attempts)-1].Error
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{
		"error":    errorMessage,
		"status":   http.StatusBadGateway,
		"proxy":    summarizeProxy(req.Proxy),
		"attempts": attempts,
	})
}

func normalizeCheckoutRequest(req *checkoutRequest) {
	req.Token = extractAccessToken(req.Token)
	req.EntryPoint = strings.TrimSpace(req.EntryPoint)
	req.PlanName = strings.TrimSpace(req.PlanName)
	req.BillingDetails.Country = strings.ToUpper(strings.TrimSpace(req.BillingDetails.Country))
	req.BillingDetails.Currency = strings.ToUpper(strings.TrimSpace(req.BillingDetails.Currency))
	req.PromoCampaign.PromoCampaignID = strings.TrimSpace(req.PromoCampaign.PromoCampaignID)
	req.CheckoutUIMode = strings.TrimSpace(req.CheckoutUIMode)
	if req.CheckoutUIMode == "" || strings.EqualFold(req.CheckoutUIMode, "custom") {
		req.CheckoutUIMode = "hosted"
	}
	req.Proxy.Type = strings.ToLower(strings.TrimSpace(req.Proxy.Type))
	req.Proxy.URL = strings.TrimSpace(req.Proxy.URL)
	req.TaxRegion.Country = strings.ToUpper(strings.TrimSpace(req.TaxRegion.Country))
	req.TaxRegion.Line1 = strings.TrimSpace(req.TaxRegion.Line1)
	req.TaxRegion.City = strings.TrimSpace(req.TaxRegion.City)
	req.TaxRegion.PostalCode = strings.TrimSpace(req.TaxRegion.PostalCode)
	req.TaxRegion.State = strings.ToUpper(strings.TrimSpace(req.TaxRegion.State))
	req.CustomerEmail = strings.TrimSpace(req.CustomerEmail)
	req.CheckoutSession.Cookie = strings.TrimSpace(req.CheckoutSession.Cookie)
	req.CheckoutSession.UserAgent = strings.TrimSpace(req.CheckoutSession.UserAgent)
}

func extractAccessToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "eyJ") {
		return raw
	}

	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if token := findStringField(payload, "accessToken", "access_token"); token != "" {
			return strings.TrimSpace(token)
		}
	}

	for _, key := range []string{`"accessToken":"`, `"accessToken": "`, `"access_token":"`, `"access_token": "`} {
		if start := strings.Index(raw, key); start >= 0 {
			valueStart := start + len(key)
			if end := strings.Index(raw[valueStart:], `"`); end >= 0 {
				return strings.TrimSpace(raw[valueStart : valueStart+end])
			}
		}
	}

	return raw
}

func validateCheckoutRequest(req checkoutRequest) error {
	if req.Token == "" {
		return errors.New("token is required")
	}
	if req.PlanName == "" {
		return errors.New("plan_name is required")
	}
	if req.BillingDetails.Country == "" {
		return errors.New("billing_details.country is required")
	}
	if req.BillingDetails.Currency == "" {
		return errors.New("billing_details.currency is required")
	}
	if req.CheckoutUIMode == "" {
		return errors.New("checkout_ui_mode is required")
	}
	if err := validateProxy(req.Proxy); err != nil {
		return err
	}
	return nil
}

func validateProxy(proxy proxySettings) error {
	if proxy.Type == "" {
		return nil
	}

	switch proxy.Type {
	case "direct":
		return nil
	case "http", "socks5":
		if proxy.URL == "" {
			return errors.New("proxy.url is required when proxy type is http or socks5")
		}
	default:
		return errors.New("proxy.type must be direct, http, or socks5")
	}

	parsed, err := url.Parse(proxy.URL)
	if err != nil || parsed.Host == "" {
		return errors.New("proxy.url must be a valid URL")
	}

	if proxy.Type == "http" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("http proxy url must start with http:// or https://")
	}
	if proxy.Type == "socks5" && parsed.Scheme != "socks5" {
		return errors.New("socks5 proxy url must start with socks5://")
	}

	return nil
}

func summarizeToken(token string) tokenSummary {
	token = strings.TrimSpace(token)
	summary := tokenSummary{Present: token != "", Length: len(token)}
	if len(token) <= 12 {
		return summary
	}

	summary.Prefix = token[:6]
	summary.Suffix = token[len(token)-4:]
	return summary
}

func summarizeProxy(proxy proxySettings) proxySummary {
	if proxy.Type == "" || proxy.Type == "direct" {
		return proxySummary{Enabled: false, Type: "direct"}
	}

	return proxySummary{
		Enabled: true,
		Type:    proxy.Type,
		URL:     maskProxyURL(proxy.URL),
	}
}

func maskProxyURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if parsed.User != nil {
		parsed.User = url.UserPassword("***", "***")
	}
	return parsed.String()
}

func newHTTPClient(proxy proxySettings, insecureTLS bool) (*http.Client, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if insecureTLS {
		tlsConfig.InsecureSkipVerify = true
	}

	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 12 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 12 * time.Second,
		TLSClientConfig:     tlsConfig,
		ForceAttemptHTTP2:   false,
	}

	switch proxy.Type {
	case "", "direct":
		transport.Proxy = nil
	case "http":
		proxyURL, err := url.Parse(proxy.URL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5":
		proxyURL, err := url.Parse(proxy.URL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
			return dialSOCKS5(ctx, proxyURL, network, address)
		}
	default:
		return nil, errors.New("unsupported proxy type")
	}

	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
	}, nil
}

func doHTTPRequestWithRetry(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 && req.Body != nil {
			if req.GetBody == nil {
				return nil, lastErr
			}
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = body
		}

		resp, err := client.Do(req)
		if err == nil && !shouldRetryHTTPStatus(resp.StatusCode) {
			return resp, nil
		}
		if attempt == maxAttempts {
			if err == nil && resp != nil {
				return resp, nil
			}
			return nil, err
		}
		if err == nil && resp != nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			lastErr = fmt.Errorf("retryable http status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		timer := time.NewTimer(time.Duration(attempt*350) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
}

func shouldRetryHTTPStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func loadAppConfig() appConfig {
	path := strings.TrimSpace(os.Getenv("APP_CONFIG"))
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Printf("read config failed: %v", err)
			} else {
				log.Printf("config file not found: %s", path)
			}
			return appConfig{}
		}
		return parseAppConfig(body, path)
	}

	return parseAppConfig(embeddedConfigFile, "embedded config.json")
}

func parseAppConfig(body []byte, source string) appConfig {
	var cfg appConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		log.Printf("parse config failed: %v", err)
		return appConfig{}
	}
	log.Printf("loaded config: %s", source)
	return cfg
}

func configString(envName string, configured string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value
	}
	return firstNonEmpty(configured, fallback)
}

func configBool(envName string, configured bool) bool {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
		switch strings.ToLower(value) {
		case "1", "yes", "y", "on", "enabled":
			return true
		case "0", "no", "n", "off", "disabled":
			return false
		}
	}
	return configured
}

func configStringList(envName string, configured []string) []string {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if item := strings.TrimSpace(part); item != "" {
				out = append(out, item)
			}
		}
		return out
	}
	return configured
}

func checkoutEndpoint() string {
	configured := configString("CHECKOUT_ENDPOINT", config.CheckoutEndpoint, "")
	if configured == "" {
		return "https://chatgpt.com/backend-api/payments/checkout"
	}
	return configured
}

func checkoutApproveEndpoint() string {
	configured := configString("CHECKOUT_APPROVE_ENDPOINT", config.CheckoutApproveEndpoint, "")
	if configured != "" {
		return configured
	}
	return strings.TrimRight(checkoutEndpoint(), "/") + "/approve"
}

func setCheckoutBackendHeaders(r *http.Request, token string, session checkoutSession) {
	r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://chatgpt.com")
	r.Header.Set("Referer", "https://chatgpt.com/")
	r.Header.Set("Sec-CH-UA", `"Microsoft Edge";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	r.Header.Set("Sec-CH-UA-Mobile", "?0")
	r.Header.Set("Sec-CH-UA-Platform", `"Windows"`)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	r.Header.Set("Sec-Fetch-Mode", "cors")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("User-Agent", checkoutUserAgent(session))
	if cookie := checkoutCookie(session); cookie != "" {
		r.Header.Set("Cookie", cookie)
	}
}

func checkoutCookie(session checkoutSession) string {
	return strings.TrimSpace(firstNonEmpty(session.Cookie, configString("CHECKOUT_COOKIE", config.CheckoutCookie, "")))
}

func checkoutUserAgent(session checkoutSession) string {
	return firstNonEmpty(
		strings.TrimSpace(session.UserAgent),
		configString("CHECKOUT_USER_AGENT", config.CheckoutUserAgent, ""),
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36 Edg/147.0.0.0",
	)
}

func checkoutUpstreamNonJSONPayload(status int, contentType string, endpoint string, body []byte) map[string]any {
	bodyText := string(body)
	lowerBody := strings.ToLower(bodyText)
	requiresCookie := strings.Contains(lowerBody, "cookie")
	errorMessage := "checkout upstream returned non-json response"
	if status == http.StatusForbidden {
		errorMessage = "checkout upstream returned 403 before GoPay; ChatGPT session cookie or browser context is likely required"
	}
	payload := map[string]any{
		"ok":           false,
		"stage":        "checkout_upstream",
		"method":       http.MethodPost,
		"endpoint":     endpoint,
		"status":       status,
		"content_type": contentType,
		"error":        errorMessage,
		"body_excerpt": trimForDisplay(bodyText, 600),
	}
	if status == http.StatusUnauthorized && (strings.Contains(lowerBody, "token_invalidated") || strings.Contains(lowerBody, "authentication token has been invalidated")) {
		payload["token_invalidated"] = true
		payload["error"] = "checkout upstream rejected the access token because it has been invalidated"
		payload["hint"] = "请重新获取有效的 access token 或重新登录 ChatGPT 后再试。"
	}
	if requiresCookie {
		payload["requires_cookie"] = true
		payload["hint"] = "在“ChatGPT 会话”里填入当前 chatgpt.com 会话 Cookie 后再初始化；本工具不会自动读取浏览器 Cookie。"
	}
	return payload
}

func checkoutProcessorEntity(checkoutData any) string {
	return firstNonEmpty(findStringField(checkoutData, "processor_entity"), "openai_llc")
}

func checkoutLongURL(checkoutData any) string {
	if payURL := openAIHostedCheckoutURL(checkoutData); payURL != "" {
		return payURL
	}
	for _, candidate := range []string{
		findStringField(checkoutData, "url"),
		findStringField(checkoutData, "checkout_url"),
		findStringField(checkoutData, "stripe_hosted_url"),
		findStringField(checkoutData, "confirm_return_url"),
	} {
		if isSupportedCheckoutURL(candidate) {
			return candidate
		}
	}
	return ""
}

func extractSnapAccountIDViaStripeFlow(ctx context.Context, client *http.Client, checkoutData any, token string, session checkoutSession) stripeSnapAccountFlowResult {
	return extractSnapAccountIDViaStripeFlowWithDeps(ctx, client, checkoutData, token, session, stripeSnapAccountFlowDeps{
		init:                initStripePaymentPage,
		update:              updateStripePaymentPage,
		createPaymentMethod: createStripePaymentMethod,
		confirm:             confirmStripePaymentPage,
		approve:             approveCheckoutViaExternalHTTP,
		waitForRedirect:     waitForStripeRedirectNextAction,
		redirect:            getGopayRedirect,
	})
}

func extractSnapAccountIDViaStripeFlowWithDeps(ctx context.Context, client *http.Client, checkoutData any, token string, session checkoutSession, deps stripeSnapAccountFlowDeps) stripeSnapAccountFlowResult {
	result := stripeSnapAccountFlowResult{}
	expectedCheckoutURL := checkoutLongURL(checkoutData)
	sessionID := findStringField(checkoutData, "checkout_session_id", "checkoutSessionId", "id")
	publishableKey := findStringField(checkoutData, "publishable_key", "publishableKey", "key")
	if sessionID == "" || publishableKey == "" {
		result.Init = stripeInitResult{OK: false, Skipped: true, Error: "checkout response missing checkout_session_id or publishable_key"}
		return result
	}

	result.Init = deps.init(ctx, client, checkoutData)
	if !result.Init.OK {
		return result
	}
	result.InitCheck = checkStripeInitTotal(result.Init.Body)
	if !result.InitCheck.OK {
		return result
	}

	clientCtx := newStripeClientContext(result.Init.Body)
	result.Update = deps.update(ctx, client, result.Init.Body, sessionID, publishableKey, defaultTaxRegion(), "", clientCtx)
	pageBody := result.Init.Body
	if result.Update.OK && result.Update.Body != nil {
		pageBody = result.Update.Body
	}
	if !result.Update.OK {
		return result
	}

	result.PaymentMethod = deps.createPaymentMethod(ctx, client, pageBody, sessionID, publishableKey, defaultTaxRegion(), "", clientCtx)
	if !result.PaymentMethod.OK {
		return result
	}
	paymentMethodID := findStringField(result.PaymentMethod.Body, "id", "payment_method")
	if paymentMethodID == "" {
		result.PaymentMethod.OK = false
		result.PaymentMethod.Error = "stripe payment method response missing id"
		return result
	}

	result.Confirm = deps.confirm(ctx, client, pageBody, sessionID, publishableKey, paymentMethodID, clientCtx)
	if !result.Confirm.OK {
		if strings.Contains(strings.ToLower(stripeOriginalErrorMessage(result.Confirm.Body)), "terms of service") && expectedCheckoutURL != "" {
			interaction, interactionErr := acceptCheckoutTermsViaCDP(ctx, expectedCheckoutURL)
			if interactionErr == nil {
				result.TermsInteraction = interaction
				result.Confirm = deps.confirm(ctx, client, pageBody, sessionID, publishableKey, paymentMethodID, clientCtx)
			}
		}
		if !result.Confirm.OK {
			return result
		}
	}
	result.ConfirmCheck = checkStripeConfirm(result.Confirm.Body, sessionID)
	if !result.ConfirmCheck.OK {
		result.Confirm.OK = false
		result.Confirm.Error = result.ConfirmCheck.Error
		return result
	}

	approvalReq := checkoutApproveRequest{
		CheckoutSessionID:   sessionID,
		ProcessorEntity:     checkoutProcessorEntity(checkoutData),
		PaymentMethodID:     paymentMethodID,
		SubmissionAttemptID: checkoutSubmissionAttemptID(result.Confirm.Body),
	}
	if deps.approve != nil {
		result.Approval, _ = deps.approve(ctx, client, token, session, approvalReq)
	}

	fetchDetails := func() stripeInitResult {
		return getStripePaymentPageDetails(ctx, client, sessionID, publishableKey, clientCtx, pageBody)
	}
	result.Details, result.RedirectCheck, result.Attempts = deps.waitForRedirect(ctx, 4, 1500*time.Millisecond, fetchDetails)
	if !result.RedirectCheck.OK {
		return result
	}

	result.Redirect = deps.redirect(ctx, client, result.RedirectCheck.RedirectURL)
	if !result.Redirect.OK {
		return result
	}
	result.AccountID = strings.TrimSpace(result.Redirect.GUID)
	if result.AccountID != "" {
		result.AccountSource = "stripe_redirect_guid"
		result.AccountOriginURL = result.Redirect.Location
	}
	return result
}

func openAIHostedCheckoutURL(checkoutData any) string {
	rawURL := firstNonEmpty(
		findStringField(checkoutData, "url"),
		findStringField(checkoutData, "checkout_url"),
		findStringField(checkoutData, "stripe_hosted_url"),
	)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	if !strings.EqualFold(parsed.Host, "checkout.stripe.com") {
		if strings.EqualFold(parsed.Host, "pay.openai.com") && strings.HasPrefix(parsed.Path, "/c/pay/") {
			return parsed.String()
		}
		return ""
	}
	if !strings.HasPrefix(parsed.Path, "/c/pay/") {
		return ""
	}
	if parsed.Fragment == "" {
		return ""
	}

	parsed.Host = "pay.openai.com"
	return parsed.String()
}

func sessionIDFromCheckoutURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if strings.HasPrefix(last, "cs_") {
		return last
	}
	return ""
}

func isSupportedCheckoutURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" {
		return false
	}

	host := strings.ToLower(parsed.Host)
	if host != "pay.openai.com" && host != "checkout.stripe.com" {
		return false
	}

	return strings.HasPrefix(parsed.Path, "/c/pay/") || strings.HasPrefix(parsed.Path, "/pay/")
}

func isManagedCheckoutPageURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	if isSupportedCheckoutURL(rawURL) {
		return true
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" && strings.EqualFold(parsed.Host, "chatgpt.com") && strings.HasPrefix(parsed.Path, "/checkout/")
}

func normalizeManagedCheckoutURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if !isManagedCheckoutPageURL(parsed.String()) {
		return ""
	}
	return parsed.String()
}

func checkoutOwnerURLFromTargetURL(targetURL string) string {
	if normalized := normalizeManagedCheckoutURL(targetURL); normalized != "" {
		return normalized
	}
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return ""
	}
	tryCandidate := func(candidate string) string {
		if normalized := normalizeManagedCheckoutURL(candidate); normalized != "" {
			return normalized
		}
		return ""
	}
	for _, key := range []string{"url", "referrer"} {
		if value := tryCandidate(parsed.Query().Get(key)); value != "" {
			return value
		}
	}
	if fragment := strings.TrimSpace(parsed.Fragment); fragment != "" {
		if values, err := url.ParseQuery(fragment); err == nil {
			for _, key := range []string{"url", "referrer"} {
				if value := tryCandidate(values.Get(key)); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func checkoutTargetMatchesExpected(targetURL string, expectedURL string) bool {
	expected := normalizeManagedCheckoutURL(expectedURL)
	if expected == "" {
		return false
	}
	if normalized := normalizeManagedCheckoutURL(targetURL); normalized == expected {
		return true
	}
	if owner := checkoutOwnerURLFromTargetURL(targetURL); owner == expected {
		return true
	}
	expectedSessionID := sessionIDFromCheckoutURL(expected)
	if expectedSessionID == "" {
		return false
	}
	for _, candidate := range []string{normalizeManagedCheckoutURL(targetURL), checkoutOwnerURLFromTargetURL(targetURL)} {
		if candidate != "" && sessionIDFromCheckoutURL(candidate) == expectedSessionID {
			return true
		}
	}
	return false
}

func resolveCheckoutPageTarget(targets []cdpTarget, expectedURL string) (*cdpTarget, string, error) {
	expected := normalizeManagedCheckoutURL(expectedURL)
	if expected == "" {
		return nil, "", errors.New("expected_url is required")
	}
	for i := len(targets) - 1; i >= 0; i-- {
		target := &targets[i]
		if target.Type != "page" {
			continue
		}
		if normalized := normalizeManagedCheckoutURL(target.URL); normalized == expected {
			return target, normalized, nil
		}
	}
	for i := len(targets) - 1; i >= 0; i-- {
		target := &targets[i]
		if target.Type != "page" {
			continue
		}
		if checkoutTargetMatchesExpected(target.URL, expected) {
			return target, normalizeManagedCheckoutURL(target.URL), nil
		}
	}
	return nil, "", errors.New("未找到本工具最近一次打开的支付链接页面，请重新打开支付链接后再试。")
}

func checkoutResolveCandidateURLs(targets []cdpTarget) []string {
	urls := make([]string, 0, min(len(targets), 10))
	seen := map[string]struct{}{}
	for i := len(targets) - 1; i >= 0 && len(urls) < 10; i-- {
		target := targets[i]
		if target.Type != "page" {
			continue
		}
		urlText := strings.TrimSpace(target.URL)
		if urlText == "" {
			continue
		}
		if _, ok := seen[urlText]; ok {
			continue
		}
		seen[urlText] = struct{}{}
		urls = append(urls, urlText)
	}
	return urls
}

func resolveCheckoutFillTarget(targets []cdpTarget, expectedURL string) (*cdpTarget, string, error) {
	pageTarget, canonicalURL, err := resolveCheckoutPageTarget(targets, expectedURL)
	if err != nil {
		return nil, "", err
	}
	for i := len(targets) - 1; i >= 0; i-- {
		target := &targets[i]
		if target.Type != "page" && target.Type != "iframe" {
			continue
		}
		urlText := strings.TrimSpace(target.URL)
		if !(strings.Contains(urlText, "elements-inner-payment") || strings.Contains(urlText, "stripe.com/v3") || strings.Contains(urlText, "m.stripe.network")) {
			continue
		}
		if checkoutTargetMatchesExpected(urlText, canonicalURL) {
			return target, canonicalURL, nil
		}
	}
	return pageTarget, canonicalURL, nil
}

var acceptCheckoutTermsViaCDP = func(ctx context.Context, expectedURL string) (map[string]any, error) {
	if !isCDPReady(cdpDebuggingPort) {
		return nil, &cdpNotReadyError{message: "CDP not ready"}
	}
	targets, err := getCDPTargets(cdpDebuggingPort)
	if err != nil {
		return nil, err
	}
	candidateTargets := make([]cdpTarget, 0)
	for i := len(targets) - 1; i >= 0; i-- {
		target := targets[i]
		if target.Type != "page" && target.Type != "iframe" {
			continue
		}
		urlText := strings.TrimSpace(target.URL)
		if checkoutTargetMatchesExpected(urlText, expectedURL) || strings.Contains(urlText, "elements-inner-payment") || strings.Contains(urlText, "stripe.com/v3") || strings.Contains(urlText, "m.stripe.network") {
			candidateTargets = append(candidateTargets, target)
		}
	}
	if len(candidateTargets) == 0 {
		return nil, errors.New("未找到与当前 checkout 对应的 page/iframe target")
	}
	response := map[string]any{
		"ok":           false,
		"expected_url": expectedURL,
		"candidates":   []map[string]any{},
	}
	appendCandidate := func(item map[string]any) {
		list, _ := response["candidates"].([]map[string]any)
		response["candidates"] = append(list, item)
	}
	for _, target := range candidateTargets {
		candidate := map[string]any{
			"id":    target.ID,
			"type":  target.Type,
			"url":   target.URL,
			"title": target.Title,
		}
		conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
		if err != nil {
			candidate["error"] = err.Error()
			appendCandidate(candidate)
			continue
		}
		_, _ = sendCDPCommand(conn, "Runtime.enable", nil)
		resultJSON, scriptErr := executeCDPScript(conn, `(async () => {
			const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));
			const norm = (text) => String(text || '').replace(/\s+/g, ' ').trim();
			const lower = (text) => norm(text).toLowerCase();
			const checkboxNodes = Array.from(document.querySelectorAll('input[type="checkbox"]'));
			const checkboxInfo = checkboxNodes.map((el, index) => ({
				index,
				checked: !!el.checked,
				text: norm(el.closest('label')?.innerText || el.parentElement?.innerText || el.getAttribute('aria-label') || ''),
				name: el.name || '',
				id: el.id || ''
			}));
			const buttons = Array.from(document.querySelectorAll('button')).map((btn, index) => ({
				index,
				text: norm(btn.textContent),
				disabled: !!btn.disabled,
			}));
			const pickCheckbox = () => checkboxNodes.find((el) => {
				const text = lower(el.closest('label')?.innerText || el.parentElement?.innerText || '') + ' ' + lower(el.getAttribute('aria-label')) + ' ' + lower(el.name) + ' ' + lower(el.id);
				return text.includes('terms') || text.includes('service') || text.includes('agree') || text.includes('merchant');
			});
			const submitButton = Array.from(document.querySelectorAll('button')).find((btn) => {
				const text = lower(btn.textContent);
				return text.includes('subscribe') || text.includes('pay') || text.includes('continue') || text.includes('confirm') || text.includes('submit');
			});
			const result = {
				title: document.title,
				url: window.location.href,
				page_snippet: norm(document.body?.innerText || '').slice(0, 1600),
				checkboxes: checkboxInfo,
				buttons,
				has_terms_text: lower(document.body?.innerText || '').includes('terms of service') || lower(document.body?.innerText || '').includes('agree to the terms'),
				clicked_checkbox: false,
				clicked_submit: false,
			};
			const checkbox = pickCheckbox();
			if (checkbox && !checkbox.checked) {
				checkbox.click();
				result.clicked_checkbox = true;
				await sleep(300);
			}
			if (checkbox) {
				result.checkbox_checked = !!checkbox.checked;
			}
			if (submitButton && !submitButton.disabled && (!checkbox || checkbox.checked)) {
				submitButton.click();
				result.clicked_submit = true;
				await sleep(1200);
			}
			result.after_url = window.location.href;
			result.after_snippet = norm(document.body?.innerText || '').slice(0, 1600);
			return JSON.stringify(result);
		})()`)
		conn.Close()
		if scriptErr != nil {
			candidate["error"] = scriptErr.Error()
			appendCandidate(candidate)
			continue
		}
		if resultJSON != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(resultJSON), &parsed); err == nil {
				candidate["interaction"] = parsed
				if clicked, _ := parsed["clicked_checkbox"].(bool); clicked {
					response["ok"] = true
					response["target"] = map[string]any{"id": target.ID, "type": target.Type, "url": target.URL, "title": target.Title}
					response["interaction"] = parsed
				}
			} else {
				candidate["raw"] = resultJSON
			}
		}
		appendCandidate(candidate)
	}
	return response, nil
}

func summarizeCheckoutApproval(approval map[string]any) map[string]any {
	if approval == nil {
		return nil
	}
	summary := map[string]any{}
	for _, key := range []string{
		"ok",
		"stage",
		"status",
		"content_type",
		"checkout_session_id",
		"processor_entity",
		"payment_method_id",
		"submission_attempt_id",
		"elapsed_ms",
		"original_error",
	} {
		if value, ok := approval[key]; ok {
			summary[key] = value
		}
	}
	return summary
}

func checkoutSubmissionAttemptID(confirmBody any) string {
	return findStringField(nestedMapFromAny(confirmBody, "submission_attempt"), "id")
}

func summarizeStripeResult(result stripeInitResult) map[string]any {
	summary := map[string]any{
		"ok":           result.OK,
		"stage":        result.Stage,
		"status":       result.Status,
		"content_type": result.ContentType,
		"elapsed_ms":   result.ElapsedMS,
		"error":        result.Error,
	}
	if originalError := stripeOriginalError(result.Body); originalError != nil {
		summary["original_error"] = originalError
	}
	if !result.OK {
		if result.Body != nil {
			summary["body"] = result.Body
		} else if trimmed := trimForDisplay(result.RawBody, 800); trimmed != "" {
			summary["raw_body"] = trimmed
		}
	}
	return summary
}

func trimForDisplay(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "...(truncated)"
}

func proxyTestTargets(customTarget string) ([]string, error) {
	customTarget = strings.TrimSpace(customTarget)
	if customTarget != "" {
		if err := validateProxyTargetURL(customTarget); err != nil {
			return nil, err
		}
		return []string{customTarget}, nil
	}

	configuredTargets := configStringList("PROXY_TEST_URLS", config.ProxyTestURLs)
	if len(configuredTargets) > 0 {
		targets := make([]string, 0, len(configuredTargets))
		for _, item := range configuredTargets {
			target := strings.TrimSpace(item)
			if target == "" {
				continue
			}
			if err := validateProxyTargetURL(target); err != nil {
				return nil, fmt.Errorf("invalid proxy test target %q: %w", target, err)
			}
			targets = append(targets, target)
		}
		if len(targets) > 0 {
			return targets, nil
		}
	}

	return []string{
		"https://api.ipify.org?format=json",
		"https://api64.ipify.org?format=json",
		"https://httpbin.org/ip",
		"http://httpbin.org/ip",
	}, nil
}

func validateProxyTargetURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return errors.New("target_url must be a valid URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return errors.New("target_url scheme must be http or https")
	}
}

func shouldSkipTLSVerify(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func dialSOCKS5(ctx context.Context, proxyURL *url.URL, network string, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("socks5 only supports tcp")
	}

	conn, err := (&net.Dialer{Timeout: 12 * time.Second}).DialContext(ctx, "tcp", proxyURL.Host)
	if err != nil {
		return nil, err
	}

	if err := socks5Handshake(conn, proxyURL, address); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func socks5Handshake(conn net.Conn, proxyURL *url.URL, targetAddress string) error {
	if err := conn.SetDeadline(time.Now().Add(12 * time.Second)); err != nil {
		return err
	}
	defer conn.SetDeadline(time.Time{})

	methods := []byte{0x00}
	username := ""
	password := ""
	if proxyURL.User != nil {
		username = proxyURL.User.Username()
		password, _ = proxyURL.User.Password()
		methods = append(methods, 0x02)
	}

	if _, err := conn.Write([]byte{0x05, byte(len(methods))}); err != nil {
		return err
	}
	if _, err := conn.Write(methods); err != nil {
		return err
	}

	methodReply := make([]byte, 2)
	if _, err := io.ReadFull(conn, methodReply); err != nil {
		return err
	}
	if methodReply[0] != 0x05 {
		return errors.New("invalid socks5 version")
	}
	if methodReply[1] == 0xff {
		return errors.New("socks5 proxy has no acceptable auth method")
	}
	if methodReply[1] == 0x02 {
		if err := socks5UsernamePasswordAuth(conn, username, password); err != nil {
			return err
		}
	} else if methodReply[1] != 0x00 {
		return errors.New("unsupported socks5 auth method")
	}

	host, portString, err := net.SplitHostPort(targetAddress)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portString)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("invalid target port")
	}

	request := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			request = append(request, 0x01)
			request = append(request, ip4...)
		} else {
			request = append(request, 0x04)
			request = append(request, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return errors.New("target host is too long")
		}
		request = append(request, 0x03, byte(len(host)))
		request = append(request, []byte(host)...)
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	request = append(request, portBytes...)

	if _, err := conn.Write(request); err != nil {
		return err
	}

	replyHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, replyHeader); err != nil {
		return err
	}
	if replyHeader[0] != 0x05 {
		return errors.New("invalid socks5 connect reply")
	}
	if replyHeader[1] != 0x00 {
		return fmt.Errorf("socks5 connect failed with code 0x%02x", replyHeader[1])
	}

	return discardSOCKS5BindAddress(conn, replyHeader[3])
}

func socks5UsernamePasswordAuth(conn net.Conn, username string, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return errors.New("socks5 username or password is too long")
	}

	payload := []byte{0x01, byte(len(username))}
	payload = append(payload, []byte(username)...)
	payload = append(payload, byte(len(password)))
	payload = append(payload, []byte(password)...)
	if _, err := conn.Write(payload); err != nil {
		return err
	}

	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[0] != 0x01 || reply[1] != 0x00 {
		return errors.New("socks5 username/password auth failed")
	}

	return nil
}

func discardSOCKS5BindAddress(conn net.Conn, addressType byte) error {
	switch addressType {
	case 0x01:
		_, err := io.CopyN(io.Discard, conn, 4+2)
		return err
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return err
		}
		_, err := io.CopyN(io.Discard, conn, int64(length[0])+2)
		return err
	case 0x04:
		_, err := io.CopyN(io.Discard, conn, 16+2)
		return err
	default:
		return errors.New("unsupported socks5 bind address type")
	}
}

func randomHex(byteCount int) string {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func randomUUID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(buf)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write response failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error":  message,
		"status": status,
	})
}

func handleGopayAutoTriggerCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req gopayAutoTriggerCheckRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	decision := markGopayAutoTriggerReady(req)
	writeJSON(w, http.StatusOK, decision)
}

func markGopayAutoTriggerReady(req gopayAutoTriggerCheckRequest) gopayAutoTriggerDecision {
	gopayAutoTriggerMu.Lock()
	defer gopayAutoTriggerMu.Unlock()

	checkoutKey := checkoutAutoTriggerKey(req.CheckoutURL)
	_, alreadyTriggered := gopayAutoTriggeredCheckout[checkoutKey]
	return evaluateGopayAutoTrigger(req, auditCaptureSensitive(), alreadyTriggered)
}

func claimGopayAutoTrigger(rawURL string) bool {
	checkoutKey := checkoutAutoTriggerKey(rawURL)
	if checkoutKey == "" {
		return false
	}
	gopayAutoTriggerMu.Lock()
	defer gopayAutoTriggerMu.Unlock()
	if _, exists := gopayAutoTriggeredCheckout[checkoutKey]; exists {
		return false
	}
	gopayAutoTriggeredCheckout[checkoutKey] = time.Now()
	return true
}

func evaluateGopayAutoTrigger(req gopayAutoTriggerCheckRequest, captureSensitive bool, alreadyTriggered bool) gopayAutoTriggerDecision {
	req.Source = strings.TrimSpace(req.Source)
	req.CheckoutURL = strings.TrimSpace(req.CheckoutURL)
	checkoutKey := checkoutAutoTriggerKey(req.CheckoutURL)
	pageText := strings.TrimSpace(req.PageText)
	checkoutURLOK := checkoutKey != ""
	gopayDetected := strings.Contains(strings.ToLower(pageText), "gopay")
	todayDueZero := paymentPageHasZeroDue(pageText)

	conditions := map[string]any{
		"audit_capture_sensitive": captureSensitive,
		"checkout_url_ok":         checkoutURLOK,
		"gopay_detected":          gopayDetected,
		"today_due_zero":          todayDueZero,
		"submitted":               req.Submitted,
		"already_triggered":       alreadyTriggered,
	}
	decision := gopayAutoTriggerDecision{
		OK:               true,
		CheckoutKey:      checkoutKey,
		Source:           req.Source,
		AlreadyTriggered: alreadyTriggered,
		Conditions:       conditions,
	}

	switch {
	case !captureSensitive:
		decision.Reason = "sensitive_capture_disabled"
	case !checkoutURLOK:
		decision.Reason = "not_checkout_payment_page"
	case alreadyTriggered:
		decision.Reason = "checkout_already_triggered"
	case !gopayDetected:
		decision.Reason = "gopay_not_detected"
	case !todayDueZero:
		decision.Reason = "today_due_not_zero"
	case !req.Submitted:
		decision.Reason = "waiting_for_checkout_submit"
	default:
		decision.Ready = true
		decision.Reason = "ready"
	}
	if decision.Ready {
		decision.Stage = "auto_trigger_ready"
	} else if decision.AlreadyTriggered {
		decision.Stage = "auto_trigger_skipped"
	} else {
		decision.Stage = "auto_trigger_waiting"
	}
	return decision
}

func checkoutAutoTriggerKey(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return ""
	}
	if !strings.EqualFold(parsed.Hostname(), "pay.openai.com") {
		return ""
	}
	if !strings.HasPrefix(parsed.Path, "/c/pay/cs_") {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.Scheme + "://" + parsed.Host + parsed.Path
}

func midtransLinkingAccountID(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return ""
	}
	if !strings.Contains(strings.ToLower(parsed.Hostname()), "midtrans.com") {
		return ""
	}
	if !strings.Contains(strings.ToLower(parsed.Fragment), "gopay-tokenization/linking") {
		return ""
	}
	return extractSnapAccountID(rawURL)
}

func midtransRedirectionAccountID(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return ""
	}
	if !strings.Contains(strings.ToLower(parsed.Hostname()), "midtrans.com") {
		return ""
	}
	return extractSnapAccountID(rawURL)
}

func midtransLinkingFillOutcome(result map[string]any) (bool, string) {
	if result == nil {
		return false, "midtrans_linking_fill_incomplete"
	}
	if resultStage := strings.TrimSpace(stringifyJSONValue(result["stage"])); resultStage != "" && resultStage != "midtrans_linking_filled" && resultStage != "midtrans_linking_submitted" {
		return false, resultStage
	}
	pageText := strings.Join([]string{
		stringifyJSONValue(result["page_text_snippet"]),
		stringifyJSONValue(result["url"]),
		stringifyJSONValue(result["after_url"]),
	}, " ")
	if midtransTechnicalErrorInText(pageText) {
		return false, "midtrans_linking_technical_error"
	}
	found, _ := result["found"].(map[string]any)
	countryOK := boolMapValue(found, "country") || boolJSONValue(result["country_verified"])
	phoneOK := boolMapValue(found, "phone")
	buttonOK := boolMapValue(found, "button")
	requestedCountry := strings.TrimPrefix(strings.TrimSpace(stringifyJSONValue(result["country_code"])), "+")
	if requestedCountry != "" {
		pageCountry := phoneLineCountryCode(pageText)
		if pageCountry != "" && pageCountry != requestedCountry {
			countryOK = false
		}
	}
	if !countryOK {
		return false, "midtrans_country_code_not_selected"
	}
	if !phoneOK || !buttonOK {
		return false, "midtrans_linking_fill_incomplete"
	}
	if clicked, _ := result["clicked"].(bool); clicked {
		return true, "midtrans_linking_submitted"
	}
	return true, "midtrans_linking_filled"
}

type midtransLinkingRetryProfile struct {
	ButtonWaitCycles       int
	PostClickWaitCycles    int
	PostClickWaitMs        int
	RecoveryWaitMs         int
	RetryStillLinking      bool
	UnboundedUntilNextStep bool
}

func midtransLinkingRetryConfig(aggressive bool) midtransLinkingRetryProfile {
	if aggressive {
		return midtransLinkingRetryProfile{
			ButtonWaitCycles:       18,
			PostClickWaitCycles:    24,
			PostClickWaitMs:        900,
			RecoveryWaitMs:         1400,
			RetryStillLinking:      true,
			UnboundedUntilNextStep: true,
		}
	}
	return midtransLinkingRetryProfile{
		ButtonWaitCycles:       8,
		PostClickWaitCycles:    18,
		PostClickWaitMs:        500,
		RecoveryWaitMs:         900,
		RetryStillLinking:      true,
		UnboundedUntilNextStep: true,
	}
}

func midtransTechnicalErrorInText(text string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "technical error") ||
		strings.Contains(normalized, "please try again") ||
		strings.Contains(normalized, "we're working on it") ||
		strings.Contains(normalized, "we’re working on it")
}

const midtransNetworkDiagnosticsMaxEntries = 24
const midtransNetworkDiagnosticsTextLimit = 1200
const midtransNetworkDebugDir = "artifacts/network-debug"

var networkDiagnosticBearerPattern = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._\-+/=]{8,}`)
var networkDiagnosticJSONSecretPattern = regexp.MustCompile(`(?i)("(?:access_token|token|authorization|cookie|client_secret|session|pin|otp)"\s*:\s*")[^"]*(")`)

var networkDiagnosticBodyFieldKeys = map[string]struct{}{
	"code":                       {},
	"description":                {},
	"error":                      {},
	"error_code":                 {},
	"fraud_status":               {},
	"gopay_payment_reference_id": {},
	"message":                    {},
	"order_id":                   {},
	"payment_reference_id":       {},
	"payment_type":               {},
	"reason":                     {},
	"reference_id":               {},
	"status":                     {},
	"status_code":                {},
	"transaction_id":             {},
	"transaction_status":         {},
}

func sanitizeMidtransNetworkDiagnostics(result map[string]any) {
	if result == nil {
		return
	}
	diagnostics, ok := result["network_diagnostics"]
	if !ok {
		return
	}
	result["network_diagnostics"] = sanitizeNetworkDiagnosticValue(diagnostics, "")
}

func sanitizeNetworkDiagnosticValue(value any, key string) any {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if isNetworkDiagnosticHeaderKey(lowerKey) {
		return summarizeNetworkDiagnosticHeaders(value)
	}
	if isNetworkDiagnosticCredentialKey(lowerKey) {
		return networkDiagnosticCredentialSummary(value)
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for nestedKey := range typed {
			keys = append(keys, nestedKey)
		}
		sort.Strings(keys)
		cleaned := make(map[string]any, len(typed))
		for _, nestedKey := range keys {
			cleaned[nestedKey] = sanitizeNetworkDiagnosticValue(typed[nestedKey], nestedKey)
		}
		return cleaned
	case []any:
		limit := len(typed)
		if lowerKey == "entries" && limit > midtransNetworkDiagnosticsMaxEntries {
			limit = midtransNetworkDiagnosticsMaxEntries
		}
		cleaned := make([]any, 0, limit)
		for i := 0; i < limit; i++ {
			cleaned = append(cleaned, sanitizeNetworkDiagnosticValue(typed[i], ""))
		}
		return cleaned
	case string:
		if lowerKey == "url" || strings.HasSuffix(lowerKey, "_url") {
			return safeNetworkDiagnosticURL(typed)
		}
		if isNetworkDiagnosticResponseTextKey(lowerKey) {
			return summarizeNetworkDiagnosticResponseText(typed)
		}
		return sanitizeNetworkDiagnosticText(typed)
	default:
		return typed
	}
}

func isNetworkDiagnosticHeaderKey(key string) bool {
	switch key {
	case "headers", "request_headers", "request_headers_raw", "response_headers", "response_headers_raw":
		return true
	default:
		return strings.HasSuffix(key, "_headers")
	}
}

func isNetworkDiagnosticResponseTextKey(key string) bool {
	switch key {
	case "response_text", "response_text_snippet", "response_body", "response_body_raw", "body", "body_text":
		return true
	default:
		return false
	}
}

func isNetworkDiagnosticCredentialKey(key string) bool {
	normalized := strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch key {
	case "authorization", "cookie", "set-cookie", "set_cookie":
		return true
	}
	switch normalized {
	case "access_token", "authorization", "challenge_id", "client_secret", "cookie", "gopay_payment_challenge_id", "gopay_payment_pin_token", "gopay_pin_token", "otp", "payment_token", "pin", "refresh_token", "session", "session_json", "token":
		return true
	}
	_, ok := sensitiveJSONKeys[normalized]
	return ok
}

func safeNetworkDiagnosticURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Fragment = safeNetworkDiagnosticFragment(parsed.Fragment)
		parsed.RawQuery = redactRawQueryValues(parsed.RawQuery)
		return parsed.String()
	}
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		return trimmed[:idx+1] + redactRawQueryValues(trimmed[idx+1:])
	}
	return sanitizeNetworkDiagnosticText(trimmed)
}

func safeNetworkDiagnosticFragment(fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return ""
	}
	path, rawQuery, found := strings.Cut(fragment, "?")
	if !found {
		return fragment
	}
	return path + "?" + redactRawQueryValues(rawQuery)
}

func redactRawQueryValues(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	parts := strings.Split(rawQuery, "&")
	for i, part := range parts {
		if part == "" {
			continue
		}
		name, _, found := strings.Cut(part, "=")
		if found {
			parts[i] = name + "=" + maskedAuditValue
		}
	}
	return strings.Join(parts, "&")
}

func summarizeNetworkDiagnosticHeaders(value any) map[string]any {
	headers := flattenNetworkDiagnosticHeaders(value)
	cleaned := make(map[string]any, len(headers))
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cleaned[key] = networkDiagnosticHeaderSummary(headers[key])
	}
	return cleaned
}

func flattenNetworkDiagnosticHeaders(value any) map[string]string {
	headers := make(map[string]string)
	add := func(name string, headerValue any) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			return
		}
		headers[name] = stringifyJSONValue(headerValue)
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			add(key, item)
		}
	case map[string]string:
		for key, item := range typed {
			add(key, item)
		}
	case string:
		lines := strings.Split(strings.ReplaceAll(typed, "\r\n", "\n"), "\n")
		for _, line := range lines {
			name, headerValue, ok := strings.Cut(line, ":")
			if ok {
				add(name, strings.TrimSpace(headerValue))
			}
		}
	case []any:
		for _, item := range typed {
			switch header := item.(type) {
			case map[string]any:
				name := firstNonEmpty(stringifyJSONValue(header["name"]), stringifyJSONValue(header["key"]))
				add(name, firstNonNil(header["value"], header["values"]))
			case []any:
				if len(header) >= 2 {
					add(stringifyJSONValue(header[0]), header[1])
				}
			}
		}
	default:
		if text := stringifyJSONValue(value); text != "" {
			add("value", text)
		}
	}
	return headers
}

func networkDiagnosticHeaderSummary(value string) map[string]any {
	summary := networkDiagnosticFingerprint(value)
	summary["prefix4"] = firstNRunes(value, 4)
	summary["suffix4"] = lastNRunes(value, 4)
	return summary
}

func networkDiagnosticCredentialSummary(value any) map[string]any {
	text := stringifyJSONValue(value)
	summary := networkDiagnosticFingerprint(text)
	summary["tail4"] = lastNRunes(text, 4)
	return summary
}

func networkDiagnosticFingerprint(value string) map[string]any {
	sum := sha256.Sum256([]byte(value))
	return map[string]any{
		"present": true,
		"length":  len(value),
		"sha256":  hex.EncodeToString(sum[:]),
	}
}

func firstNRunes(value string, n int) string {
	runes := []rune(value)
	if len(runes) <= n {
		return value
	}
	return string(runes[:n])
}

func lastNRunes(value string, n int) string {
	runes := []rune(value)
	if len(runes) <= n {
		return value
	}
	return string(runes[len(runes)-n:])
}

func summarizeNetworkDiagnosticResponseText(text string) map[string]any {
	trimmed := strings.TrimSpace(text)
	summary := map[string]any{
		"present": len(trimmed) > 0,
		"length":  len(trimmed),
	}
	if trimmed == "" {
		return summary
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		fields := map[string]any{}
		credentials := map[string]any{}
		collectNetworkDiagnosticResponseFields(parsed, fields, credentials, 0)
		summary["format"] = "json"
		if len(fields) > 0 {
			summary["fields"] = fields
		}
		if len(credentials) > 0 {
			summary["credentials"] = credentials
		}
		return summary
	}
	summary["format"] = "text"
	summary["text"] = sanitizeNetworkDiagnosticText(trimmed)
	return summary
}

func collectNetworkDiagnosticResponseFields(value any, fields map[string]any, credentials map[string]any, depth int) {
	if depth > 5 {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			item := typed[key]
			normalized := strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(strings.ToLower(strings.TrimSpace(key)))
			if isNetworkDiagnosticCredentialKey(normalized) {
				credentials[key] = networkDiagnosticCredentialSummary(item)
				continue
			}
			if _, ok := networkDiagnosticBodyFieldKeys[normalized]; ok {
				fields[key] = sanitizeNetworkDiagnosticFieldValue(item)
				continue
			}
			collectNetworkDiagnosticResponseFields(item, fields, credentials, depth+1)
		}
	case []any:
		limit := len(typed)
		if limit > 12 {
			limit = 12
		}
		for i := 0; i < limit; i++ {
			collectNetworkDiagnosticResponseFields(typed[i], fields, credentials, depth+1)
		}
	}
}

func sanitizeNetworkDiagnosticFieldValue(value any) any {
	switch typed := value.(type) {
	case string:
		return sanitizeNetworkDiagnosticText(typed)
	case float64, bool, nil:
		return typed
	default:
		return sanitizeNetworkDiagnosticText(stringifyJSONValue(typed))
	}
}

func sanitizeNetworkDiagnosticText(text string) string {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return ""
	}
	cleaned = networkDiagnosticJSONSecretPattern.ReplaceAllString(cleaned, `$1`+maskedAuditValue+`$2`)
	cleaned = networkDiagnosticBearerPattern.ReplaceAllString(cleaned, "Bearer "+maskedAuditValue)
	if len(cleaned) > midtransNetworkDiagnosticsTextLimit {
		cleaned = cleaned[:midtransNetworkDiagnosticsTextLimit] + "..."
	}
	return cleaned
}

func writeMidtransNetworkDebugArtifact(baseDir string, accountID string, req gopayMidtransLinkingFillRequest, result map[string]any) (map[string]any, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = midtransNetworkDebugDir
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, err
	}
	fileName := fmt.Sprintf("%s_%s.json", sanitizeAuditFileName(firstNonEmpty(accountID, "unknown_account")), time.Now().Format("20060102_150405_000"))
	path := filepath.Join(baseDir, fileName)
	payload := map[string]any{
		"created_at":          time.Now().Format(time.RFC3339Nano),
		"account_id":          accountID,
		"target_url":          safeNetworkDiagnosticURL(req.TargetURL),
		"checkout_url":        safeNetworkDiagnosticURL(req.CheckoutURL),
		"network_diagnostics": nil,
		"browser_result":      sanitizeNetworkDiagnosticValue(result, ""),
	}
	if result != nil {
		payload["network_diagnostics"] = result["network_diagnostics"]
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return map[string]any{
		"path":       path,
		"size_bytes": len(data),
	}, nil
}

func boolMapValue(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	return boolJSONValue(values[key])
}

func boolJSONValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func phoneLineCountryCode(text string) string {
	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)phone\s*number[^+]{0,80}\+([0-9]{1,4})`),
		regexp.MustCompile(`(?i)nomor[^+]{0,120}\+([0-9]{1,4})`),
		regexp.MustCompile(`(?i)otp[^+]{0,120}\+([0-9]{1,4})`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(normalized)
		if len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

func paymentPageHasZeroDue(text string) bool {
	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" {
		return false
	}
	labels := []string{
		"今日应付合计",
		"今日应付金额",
		"今日应付",
		"today's total",
		"today total",
		"today's due",
		"today due",
		"due today",
	}
	lower := strings.ToLower(normalized)
	amountPattern := regexp.MustCompile(`(?i)(?:IDR|JPY|JP¥|USD|US\$|Rp|¥|\$)?\s*([0-9][0-9.,]*)`)
	for _, label := range labels {
		idx := strings.Index(lower, strings.ToLower(label))
		if idx < 0 {
			continue
		}
		end := idx + 120
		if end > len(normalized) {
			end = len(normalized)
		}
		window := normalized[idx:end]
		match := amountPattern.FindStringSubmatch(window)
		if len(match) < 2 {
			return false
		}
		amountText := strings.NewReplacer(",", "", "，", "").Replace(match[1])
		amount, err := strconv.ParseFloat(amountText, 64)
		return err == nil && amount == 0
	}
	return false
}

func handleGopayMidtransLinkingFill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req gopayMidtransLinkingFillRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.TargetURL = strings.TrimSpace(req.TargetURL)
	req.CheckoutURL = strings.TrimSpace(req.CheckoutURL)
	req.CountryCode = strings.TrimSpace(req.CountryCode)
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.PhoneNumber == "" {
		req.PhoneNumber = "18120322232"
	}
	accountID := midtransRedirectionAccountID(req.TargetURL)
	if accountID == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"stage":      "midtrans_redirection_page_not_detected",
			"error":      "target_url is not a Midtrans redirection page",
			"target_url": req.TargetURL,
		})
		return
	}
	if !isCDPReady(cdpDebuggingPort) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "stage": "cdp_not_ready", "error": "CDP not ready"})
		return
	}

	targets, err := getCDPTargets(cdpDebuggingPort)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "stage": "cdp_targets_failed", "error": err.Error()})
		return
	}
	var target *cdpTarget
	for i := len(targets) - 1; i >= 0; i-- {
		candidate := &targets[i]
		if candidate.Type != "page" {
			continue
		}
		if midtransRedirectionAccountID(candidate.URL) == accountID {
			target = candidate
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             false,
			"stage":          "midtrans_linking_target_not_found",
			"account_id":     accountID,
			"target_url":     req.TargetURL,
			"candidate_urls": checkoutResolveCandidateURLs(targets),
		})
		return
	}

	conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CDP failed: "+err.Error())
		return
	}
	defer conn.Close()
	go func() {
		<-r.Context().Done()
		_ = conn.Close()
	}()
	retryProfile := midtransLinkingRetryConfig(req.AggressiveRetry)
	countryJSON := strconv.Quote(strings.TrimPrefix(req.CountryCode, "+"))
	phoneJSON := strconv.Quote(req.PhoneNumber)
	resultJSON, execErr := executeCDPScript(conn, fmt.Sprintf(`(async () => {
		const countryCode = %s;
		const phoneNumber = %s;
		const aggressiveRetry = %t;
		const retryStillLinking = %t;
		const postClickWaitCycles = %d;
		const postClickWaitMs = %d;
		const recoveryWaitMs = %d;
		const buttonWaitCycles = %d;
		const unboundedUntilNextStep = %t;
		const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
		const visible = (el) => !!el && !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
		const textOf = (el) => ((el?.innerText || el?.textContent || '') + ' ' + (el?.getAttribute?.('aria-label') || '') + ' ' + (el?.getAttribute?.('title') || '')).replace(/\s+/g, ' ').trim();
		const setNativeValue = (el, value) => {
			const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
			const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
			if (setter) setter.call(el, value); else el.value = value;
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
			el.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true, key: '1' }));
		};
		const result = {
			url: window.location.href,
			title: document.title,
			account_id: %q,
			country_code: countryCode,
			phone_number: phoneNumber,
			found: { country: false, phone: false, button: false },
		};
		const networkDiagnostics = {
			capture_method: 'fetch_xhr_wrapper',
			started_at: new Date().toISOString(),
			entries: [],
			dropped_entries: 0,
		};
		result.network_diagnostics = networkDiagnostics;
		const shouldCaptureNetworkURL = (raw) => {
			const text = String(raw || '').toLowerCase();
			return text.includes('midtrans') || text.includes('gopay') || text.includes('snap') || text.includes('tokenization') || text.includes('merchants-gws');
		};
		const safeNetworkURL = (raw) => {
			try {
				const parsed = new URL(String(raw || ''), window.location.href);
				const names = [];
				parsed.searchParams.forEach((_, key) => {
					if (!names.includes(key)) names.push(key);
				});
				const query = names.map((key) => encodeURIComponent(key) + '=***').join('&');
				let hash = parsed.hash || '';
				if (hash.includes('?')) {
					const parts = hash.split('?');
					const hashRoute = parts.shift();
					const hashQuery = new URLSearchParams(parts.join('?'));
					const hashNames = [];
					hashQuery.forEach((_, key) => {
						if (!hashNames.includes(key)) hashNames.push(key);
					});
					hash = hashRoute + '?' + hashNames.map((key) => encodeURIComponent(key) + '=***').join('&');
				}
				return parsed.origin + parsed.pathname + (query ? '?' + query : '') + hash;
			} catch (_) {
				return String(raw || '').split('#')[0].slice(0, 220);
			}
		};
		const headerEntriesFrom = (headers) => {
			const out = {};
			if (!headers) return out;
			try {
				if (typeof Headers !== 'undefined' && headers instanceof Headers) {
					headers.forEach((value, key) => { out[String(key).toLowerCase()] = String(value); });
					return out;
				}
				if (Array.isArray(headers)) {
					headers.forEach((item) => {
						if (Array.isArray(item) && item.length >= 2) out[String(item[0]).toLowerCase()] = String(item[1]);
					});
					return out;
				}
				if (typeof headers === 'object') {
					Object.keys(headers).forEach((key) => { out[String(key).toLowerCase()] = String(headers[key]); });
				}
			} catch (_) {}
			return out;
		};
		const mergeHeaderEntries = (...items) => {
			const out = {};
			items.forEach((item) => {
				const entries = headerEntriesFrom(item);
				Object.keys(entries).forEach((key) => { out[key] = entries[key]; });
			});
			return out;
		};
		const normalizeNetworkText = (text) => String(text || '')
			.replace(/\s+/g, ' ')
			.trim()
			.slice(0, 1200);
		const addNetworkEntry = (entry) => {
			if (!entry || !shouldCaptureNetworkURL(entry.url)) return;
			if (networkDiagnostics.entries.length >= 24) {
				networkDiagnostics.dropped_entries += 1;
				return;
			}
			const cleaned = { ...entry, url: safeNetworkURL(entry.url) };
			if (cleaned.response_text_snippet) cleaned.response_text_snippet = normalizeNetworkText(cleaned.response_text_snippet);
			if (cleaned.error) cleaned.error = normalizeNetworkText(cleaned.error);
			networkDiagnostics.entries.push(cleaned);
		};
		const installNetworkDiagnostics = () => {
			window.__gopayNetworkDiagnostics = networkDiagnostics;
			window.__gopayRecordNetworkEntry = addNetworkEntry;
			if (window.__gopayNetworkDiagnosticsInstalled) return;
			window.__gopayNetworkDiagnosticsInstalled = true;
			const originalFetch = window.fetch;
			if (typeof originalFetch === 'function') {
				window.fetch = async function(input, init) {
					const rawURL = typeof input === 'string' ? input : (input && input.url) || String(input || '');
					const method = String((init && init.method) || (input && input.method) || 'GET').toUpperCase();
					const requestHeaders = mergeHeaderEntries(input && input.headers, init && init.headers);
					const started = performance.now();
					try {
						const response = await originalFetch.apply(this, arguments);
						const entry = {
							kind: 'fetch',
							method,
							url: rawURL,
							request_headers: requestHeaders,
							response_headers: headerEntriesFrom(response.headers),
							status: response.status,
							ok: response.ok,
							status_text: response.statusText || '',
							elapsed_ms: Math.round(performance.now() - started),
						};
						if (shouldCaptureNetworkURL(rawURL)) {
							try {
								const text = await Promise.race([response.clone().text(), wait(900).then(() => '')]);
								if (text) entry.response_text_snippet = text;
							} catch (error) {
								entry.response_read_error = String(error && error.message || error || '').slice(0, 180);
							}
						}
						window.__gopayRecordNetworkEntry?.(entry);
						return response;
					} catch (error) {
						window.__gopayRecordNetworkEntry?.({
							kind: 'fetch',
							method,
							url: rawURL,
							error: String(error && error.message || error || ''),
							elapsed_ms: Math.round(performance.now() - started),
						});
						throw error;
					}
				};
			}
			const originalOpen = XMLHttpRequest.prototype.open;
			const originalSetRequestHeader = XMLHttpRequest.prototype.setRequestHeader;
			const originalSend = XMLHttpRequest.prototype.send;
			XMLHttpRequest.prototype.open = function(method, url) {
				this.__gopayNetworkInfo = { method: String(method || 'GET').toUpperCase(), url: String(url || ''), request_headers: {} };
				return originalOpen.apply(this, arguments);
			};
			XMLHttpRequest.prototype.setRequestHeader = function(name, value) {
				if (!this.__gopayNetworkInfo) this.__gopayNetworkInfo = { method: 'GET', url: '', request_headers: {} };
				this.__gopayNetworkInfo.request_headers[String(name || '').toLowerCase()] = String(value || '');
				return originalSetRequestHeader.apply(this, arguments);
			};
			XMLHttpRequest.prototype.send = function() {
				const info = this.__gopayNetworkInfo || {};
				const started = performance.now();
				this.addEventListener('loadend', () => {
					const entry = {
						kind: 'xhr',
						method: info.method || 'GET',
						url: info.url || this.responseURL || '',
						request_headers: info.request_headers || {},
						response_headers: this.getAllResponseHeaders?.() || '',
						status: this.status,
						ok: this.status >= 200 && this.status < 400,
						status_text: this.statusText || '',
						elapsed_ms: Math.round(performance.now() - started),
					};
					try {
						if (shouldCaptureNetworkURL(entry.url) && typeof this.responseText === 'string') {
							entry.response_text_snippet = this.responseText.slice(0, 1200);
						}
					} catch (_) {}
					window.__gopayRecordNetworkEntry?.(entry);
				});
				this.addEventListener('error', () => {
					window.__gopayRecordNetworkEntry?.({
						kind: 'xhr',
						method: info.method || 'GET',
						url: info.url || this.responseURL || '',
						error: 'xhr error',
						elapsed_ms: Math.round(performance.now() - started),
					});
				});
				return originalSend.apply(this, arguments);
			};
		};
		installNetworkDiagnostics();
		const pageText = () => ((document.body?.innerText || document.body?.textContent || '')).replace(/\s+/g, ' ').trim();
		const snapshotPage = () => ({
			url: window.location.href,
			title: document.title,
			page_text_snippet: pageText().slice(0, 1000),
			input_count: Array.from(document.querySelectorAll('input')).filter((el) => visible(el) && el.type !== 'hidden').length,
			buttons: Array.from(document.querySelectorAll('button')).filter(visible).map((button) => ({ text: textOf(button).slice(0, 80), disabled: !!button.disabled })).slice(0, 8),
		});
		const looksLikeLinkingPage = () => {
			const href = window.location.href.toLowerCase();
			const text = pageText().toLowerCase();
			if (href.includes('gopay-tokenization/linking')) return true;
			if (text.includes('you have completed payment') || text.includes('checkout session has timed out') || text.includes('您已经完成付款') || text.includes('本结账会话已超时')) return false;
			return text.includes('gopay') && (text.includes('link and pay') || text.includes('phone number') || text.includes('nomor') || text.includes('hubungkan'));
		};
		for (let i = 0; i < 16 && !looksLikeLinkingPage(); i++) {
			await wait(500);
		}
		if (!looksLikeLinkingPage()) {
			Object.assign(result, snapshotPage(), { stage: 'midtrans_not_linking_current_state' });
			return JSON.stringify(result);
		}
		const phoneCountryCode = () => {
			const text = pageText();
			const patterns = [
				/phone\s*number[^+]{0,80}\+([0-9]{1,4})/i,
				/nomor[^+]{0,120}\+([0-9]{1,4})/i,
				/otp[^+]{0,120}\+([0-9]{1,4})/i,
			];
			for (const pattern of patterns) {
				const match = text.match(pattern);
				if (match) return match[1];
			}
			return '';
		};
		const countryVerified = () => phoneCountryCode() === countryCode;
		const clickElement = (el) => {
			el.scrollIntoView?.({ block: 'center', inline: 'center' });
			el.dispatchEvent(new MouseEvent('mouseover', { bubbles: true }));
			el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
			el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
			el.click();
		};
		const uniqueElements = (items) => Array.from(new Set(items.filter(Boolean)));
		const countryTargetPattern = new RegExp('(\\+' + countryCode + '\\b|china|tiongkok|中国)', 'i');
		const currentCountryPattern = /\+?\s*62\b|indonesia|🇮🇩/i;
		const optionTextOK = (el) => {
			const text = textOf(el);
			return text.length > 0 && text.length <= 180 && countryTargetPattern.test(text);
		};
		const tryPickCountryOption = async () => {
			const searchInputs = Array.from(document.querySelectorAll('input')).filter((el) => visible(el) && el.type !== 'hidden' && !/tel|number/i.test(el.type));
			const searchInput = searchInputs.find((el) => /search|country|negara|cari|kode|code/i.test((el.name || '') + ' ' + (el.id || '') + ' ' + (el.placeholder || '') + ' ' + (el.getAttribute('aria-label') || ''))) || (searchInputs.length === 1 ? searchInputs[0] : null);
			if (searchInput) {
				searchInput.focus();
				setNativeValue(searchInput, countryCode);
				result.country_search_used = true;
				await wait(600);
			}
			const options = uniqueElements(Array.from(document.querySelectorAll('button, [role="option"], [role="menuitem"], li, [tabindex], div, span'))
				.filter(visible)
				.map((el) => el.closest?.('button, [role="option"], [role="menuitem"], li, [tabindex]') || el))
				.filter(visible)
				.filter(optionTextOK)
				.sort((a, b) => {
					const at = textOf(a);
					const bt = textOf(b);
					const as = (/\+86/.test(at) ? 0 : 10) + (/china/i.test(at) ? 0 : 2) + at.length;
					const bs = (/\+86/.test(bt) ? 0 : 10) + (/china/i.test(bt) ? 0 : 2) + bt.length;
					return as - bs;
				});
			result.country_option_candidates = options.map((el) => textOf(el).slice(0, 120)).slice(0, 6);
			for (const option of options.slice(0, 4)) {
				clickElement(option);
				result.selected_country = textOf(option).slice(0, 120);
				await wait(900);
				document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
				await wait(250);
				if (countryVerified()) {
					result.found.country = true;
					result.country_via = result.country_search_used ? 'dropdown-search' : 'dropdown';
					return true;
				}
			}
			return false;
		};

		const selects = Array.from(document.querySelectorAll('select')).filter(visible);
		for (const select of selects) {
			const option = Array.from(select.options).find((o) => {
				const text = (o.text || '').toLowerCase();
				const value = String(o.value || '').toLowerCase();
				return value === countryCode || text.includes('+' + countryCode) || text.includes('china') || text.includes('tiongkok') || text.includes('中国');
			});
			if (option) {
				select.value = option.value;
				select.dispatchEvent(new Event('change', { bubbles: true }));
				result.country_via = 'select';
				result.selected_country = { value: option.value, text: option.text };
				await wait(600);
				if (countryVerified()) {
					result.found.country = true;
				}
				break;
			}
		}

		if (!result.found.country) {
			const countryOpeners = uniqueElements(Array.from(document.querySelectorAll('button, [role="button"], [aria-haspopup], [aria-expanded], [tabindex], div, span'))
				.filter(visible)
				.map((el) => el.closest?.('button, [role="button"], [aria-haspopup], [aria-expanded], [tabindex]') || el)
				.filter(visible)
				.filter((el) => {
					const text = textOf(el);
					return text.length > 0 && text.length <= 180 && currentCountryPattern.test(text);
				})
				.sort((a, b) => textOf(a).length - textOf(b).length));
			result.country_opener_candidates = countryOpeners.map((el) => textOf(el).slice(0, 120)).slice(0, 6);
			for (const opener of countryOpeners.slice(0, 4)) {
				clickElement(opener);
				result.country_dropdown_clicked = textOf(opener).slice(0, 120);
				await wait(700);
				if (await tryPickCountryOption()) break;
			}
		}
		result.country_verified = countryVerified();
		result.detected_phone_country_code = phoneCountryCode();
		if (!result.country_verified) {
			Object.assign(result, snapshotPage(), { stage: 'midtrans_country_code_not_selected' });
			return JSON.stringify(result);
		}

		const inputs = Array.from(document.querySelectorAll('input')).filter((el) => visible(el) && el.type !== 'hidden' && el.type !== 'checkbox' && el.type !== 'radio');
		result.input_count = inputs.length;
		result.inputs = inputs.map((el) => ({ type: el.type, inputMode: el.inputMode, name: el.name, id: el.id, placeholder: el.placeholder })).slice(0, 8);
		const phoneInput = inputs.find((el) => /tel|number/i.test(el.type) || /numeric|tel/i.test(el.inputMode) || /phone|mobile|nomor|telepon/i.test((el.name || '') + ' ' + (el.id || '') + ' ' + (el.placeholder || ''))) || inputs[inputs.length - 1];
		if (phoneInput) {
			phoneInput.focus();
			setNativeValue(phoneInput, phoneNumber);
			result.found.phone = true;
			result.phone_input = { type: phoneInput.type, inputMode: phoneInput.inputMode, name: phoneInput.name, id: phoneInput.id, placeholder: phoneInput.placeholder };
			await wait(900);
		}
		result.country_verified_after_phone = countryVerified();
		result.detected_phone_country_code_after_phone = phoneCountryCode();
		if (!result.country_verified_after_phone) {
			Object.assign(result, snapshotPage(), { stage: 'midtrans_country_code_not_selected' });
			return JSON.stringify(result);
		}
		const hasTechnicalError = () => {
			const text = pageText().toLowerCase();
			return text.includes('technical error') || text.includes('please try again') || text.includes("we're working on it") || text.includes('we’re working on it');
		};
		const hasNextStep = () => {
			const href = window.location.href.toLowerCase();
			const text = pageText().toLowerCase();
			return href.includes('merchants-gws-app.gopayapi.com') ||
				href.includes('pin-web-client.gopayapi.com') ||
				text.includes('otp dikirim') ||
				text.includes('masukkin otp') ||
				text.includes('masukkan otp') ||
				text.includes('buat akun gopay') ||
				text.includes('create gopay account') ||
				text.includes('pin kamu');
		};
		const waitForPostClickState = async () => {
			for (let i = 0; i < postClickWaitCycles; i++) {
				await wait(postClickWaitMs);
				if (hasNextStep()) return 'next_step';
				if (hasTechnicalError()) return 'technical_error';
				if (!window.location.href.toLowerCase().includes('midtrans.com')) return 'navigated';
			}
			return hasTechnicalError() ? 'technical_error' : 'still_linking';
		};
		const clickBackFromTechnicalError = async () => {
			const buttons = Array.from(document.querySelectorAll('button')).filter(visible);
			const backButton = buttons.find((button) => /back|kembali|try again/i.test(textOf(button)));
			result.technical_error_recovery_attempts = (result.technical_error_recovery_attempts || 0) + 1;
			result.technical_error_back_button = backButton ? textOf(backButton).slice(0, 80) : '';
			if (!backButton) return false;
			clickElement(backButton);
			await wait(recoveryWaitMs);
			for (let i = 0; i < 10; i++) {
				if (looksLikeLinkingPage() && !hasTechnicalError()) return true;
				await wait(500);
			}
			return looksLikeLinkingPage() && !hasTechnicalError();
		};

		result.click_attempts = [];
		result.technical_error_back_loop = true;
		result.unbounded_until_next_step = unboundedUntilNextStep;
		result.total_click_attempts = 0;
		const recordClickAttempt = (entry) => {
			result.total_click_attempts += 1;
			result.last_click_attempt = entry;
			if (result.click_attempts.length < 80) {
				result.click_attempts.push(entry);
			} else {
				result.click_attempts_truncated = true;
			}
		};
		let attempt = 0;
		while (true) {
			if (hasNextStep()) break;
			if (!looksLikeLinkingPage() && !hasTechnicalError()) {
				recordClickAttempt({ attempt: attempt + 1, action: 'not_linking_state', url: window.location.href, snippet: pageText().slice(0, 240) });
				break;
			}
			attempt += 1;
			if (hasTechnicalError()) {
				const recovered = await clickBackFromTechnicalError();
				recordClickAttempt({ attempt, action: 'recover_technical_error', recovered });
				if (!recovered) {
					await wait(recoveryWaitMs);
					continue;
				}
				await wait(700);
				if (hasNextStep()) break;
				if (hasTechnicalError()) {
					await wait(recoveryWaitMs);
					continue;
				}
			}
			const buttons = Array.from(document.querySelectorAll('button')).filter(visible);
			result.buttons = buttons.map((button) => ({ text: textOf(button).slice(0, 80), disabled: !!button.disabled })).slice(0, 8);
			let submitButton = buttons.find((button) => {
				const text = textOf(button).toLowerCase();
				return text.includes('link and pay') || text.includes('hubungkan') || text.includes('bayar') || text.includes('continue') || text.includes('lanjut') || text.includes('pay');
			});
			if (!submitButton) {
				recordClickAttempt({ attempt, action: 'missing_button', url: window.location.href, snippet: pageText().slice(0, 240) });
				await wait(postClickWaitMs);
				continue;
			}
			result.found.button = true;
			result.button_text = textOf(submitButton).slice(0, 100);
			result.button_disabled = !!submitButton.disabled;
			if (submitButton.disabled) {
				for (let i = 0; i < buttonWaitCycles && submitButton.disabled; i++) {
					await wait(500);
					const refreshedButtons = Array.from(document.querySelectorAll('button')).filter(visible);
					submitButton = refreshedButtons.find((button) => {
						const text = textOf(button).toLowerCase();
						return text.includes('link and pay') || text.includes('hubungkan') || text.includes('bayar') || text.includes('continue') || text.includes('lanjut') || text.includes('pay');
					}) || submitButton;
				}
				result.button_disabled_after_wait = !!submitButton.disabled;
			}
			if (submitButton.disabled) {
				recordClickAttempt({ attempt, action: 'button_disabled' });
				await wait(postClickWaitMs);
				continue;
			}
			if (!countryVerified()) {
				Object.assign(result, snapshotPage(), { stage: 'midtrans_country_code_not_selected' });
				return JSON.stringify(result);
			}
			clickElement(submitButton);
			result.clicked = true;
			result.clicked_attempts = (result.clicked_attempts || 0) + 1;
			const postClickState = await waitForPostClickState();
			recordClickAttempt({ attempt, action: 'click_link_and_pay', state: postClickState, url: window.location.href, snippet: pageText().slice(0, 240) });
			if (postClickState === 'next_step' || postClickState === 'navigated') break;
			if (postClickState === 'technical_error') continue;
			if (postClickState === 'still_linking' && retryStillLinking) {
				await wait(postClickWaitMs);
				continue;
			}
			break;
		}
		result.after_url = window.location.href;
		result.page_text_snippet = ((document.body?.innerText || document.body?.textContent || '')).replace(/\s+/g, ' ').trim().slice(0, 1000);
		result.aggressive_retry = aggressiveRetry;
		result.retry_still_linking = retryStillLinking;
		networkDiagnostics.ended_at = new Date().toISOString();
		networkDiagnostics.final_state = {
			has_technical_error: hasTechnicalError(),
			has_next_step: hasNextStep(),
			entry_count: networkDiagnostics.entries.length,
			dropped_entries: networkDiagnostics.dropped_entries,
		};
		if (hasTechnicalError()) {
			result.stage = 'midtrans_linking_technical_error';
		}
		return JSON.stringify(result);
	})()`, countryJSON, phoneJSON, req.AggressiveRetry, retryProfile.RetryStillLinking, retryProfile.PostClickWaitCycles, retryProfile.PostClickWaitMs, retryProfile.RecoveryWaitMs, retryProfile.ButtonWaitCycles, retryProfile.UnboundedUntilNextStep, accountID))
	if execErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "stage": "midtrans_linking_fill_failed", "account_id": accountID, "error": execErr.Error()})
		return
	}
	var result map[string]any
	if resultJSON != "" {
		_ = json.Unmarshal([]byte(resultJSON), &result)
	}
	sanitizeMidtransNetworkDiagnostics(result)
	var debugArtifact map[string]any
	var debugErr error
	if req.DebugNetwork {
		debugArtifact, debugErr = writeMidtransNetworkDebugArtifact(midtransNetworkDebugDir, accountID, req, result)
	}
	ok, stage := midtransLinkingFillOutcome(result)
	claimed := false
	if ok && req.CheckoutURL != "" {
		claimed = claimGopayAutoTrigger(req.CheckoutURL)
	}
	response := map[string]any{
		"ok":               ok,
		"stage":            stage,
		"account_id":       accountID,
		"target_url":       req.TargetURL,
		"checkout_url":     req.CheckoutURL,
		"country_code":     req.CountryCode,
		"phone_number":     req.PhoneNumber,
		"aggressive_retry": req.AggressiveRetry,
		"trigger_claimed":  claimed,
		"browser_result":   result,
	}
	if result != nil {
		if diagnostics, ok := result["network_diagnostics"]; ok {
			response["network_diagnostics"] = diagnostics
		}
	}
	if debugArtifact != nil {
		response["network_debug_artifact"] = debugArtifact
	}
	if debugErr != nil {
		response["network_debug_error"] = debugErr.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

func handleGopayForceLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req gopayForceLinkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.CountryCode = strings.TrimSpace(req.CountryCode)
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.PhoneNumber == "" {
		writeError(w, http.StatusBadRequest, "phone_number required")
		return
	}
	response := map[string]any{
		"request":     map[string]any{"account_id": req.AccountID, "country_code": req.CountryCode, "phone_number": req.PhoneNumber},
		"diagnostics": gopayRequestDiagnostics(req.CountryCode, req.PhoneNumber, ""),
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resolution := gopayLinkingResolution{AccountSource: "manual_input", AccountSourceNote: "account_id provided by request"}
	apiResolution, apiErr := createOrReuseGopayLinkingViaLocalMock(ctx, req.AccountID, gopayLink{Type: "gopay", CountryCode: req.CountryCode, PhoneNumber: req.PhoneNumber})
	resolution = apiResolution
	resolution.AccountSource = firstNonEmpty(resolution.AccountSource, "manual_input")
	resolution.AccountSourceNote = firstNonEmpty(resolution.AccountSourceNote, "account_id provided by request")
	response["linking_diagnostics"] = gopayLinkingDiagnostics(resolution)
	if apiErr != nil {
		response["api"] = map[string]any{"ok": false, "error": apiErr.Error()}
	} else {
		response["api"] = map[string]any{
			"ok":              true,
			"result":          resolution.LinkResult,
			"reused_existing": resolution.ReusedExisting,
			"conflict_reason": resolution.ConflictReason,
			"account":         resolution.AccountResult,
		}
		response["reused_existing"] = resolution.ReusedExisting
		response["reference_id"] = stringifyJSONValue(resolution.LinkResult["reference_id"])
	}
	writeJSON(w, http.StatusOK, response)
}

func handleGopayAutoLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req gopayAutoLinkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.CountryCode = strings.TrimSpace(req.CountryCode)
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	req.OTPChannel = strings.TrimSpace(req.OTPChannel)
	req.PIN = strings.TrimSpace(req.PIN)
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.PhoneNumber == "" {
		writeError(w, http.StatusBadRequest, "phone_number required")
		return
	}
	if req.OTPChannel == "" {
		req.OTPChannel = "whatsapp"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	response := map[string]any{
		"request":     map[string]any{"account_id": req.AccountID, "country_code": req.CountryCode, "phone_number": req.PhoneNumber, "otp_channel": req.OTPChannel},
		"diagnostics": gopayRequestDiagnostics(req.CountryCode, req.PhoneNumber, req.OTPChannel),
	}
	resolution, linkErr := createOrReuseGopayLinkingViaLocalMock(ctx, req.AccountID, gopayLink{Type: "gopay", CountryCode: req.CountryCode, PhoneNumber: req.PhoneNumber})
	response["linking_diagnostics"] = gopayLinkingDiagnostics(resolution)
	if linkErr != nil {
		response["stage"] = "linking"
		response["ok"] = false
		response["error"] = linkErr.Error()
		writeJSON(w, http.StatusOK, response)
		return
	}
	if resolution.ReusedExisting {
		response["stage"] = "linking_success"
		response["ok"] = true
		response["reused_existing"] = true
		response["account"] = resolution.AccountResult
		response["summary"] = map[string]any{
			"reference_id":    "",
			"gopay_linked":    true,
			"reused_existing": true,
			"account_id":      req.AccountID,
			"country_code":    req.CountryCode,
			"phone_number":    req.PhoneNumber,
			"account_status":  stringifyJSONValue(resolution.AccountResult["account_status"]),
			"conflict_reason": resolution.ConflictReason,
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	linkResult := resolution.LinkResult
	referenceID := stringifyJSONValue(linkResult["reference_id"])
	response["stage"] = "linking"
	response["reference_id"] = referenceID
	refResult, refErr := validateGopayReferenceViaLocalMock(ctx, referenceID)
	if refErr != nil || validateGopayReferenceResult(refResult) != nil {
		response["stage"] = "validate_reference"
		response["ok"] = false
		response["error"] = "reference validation failed"
		writeJSON(w, http.StatusOK, response)
		return
	}
	consentResult, consentErr := requestGopayUserConsentViaLocalMock(ctx, referenceID, req.OTPChannel)
	if consentErr != nil || validateGopayUserConsentResult(consentResult) != nil {
		response["stage"] = "user_consent"
		response["ok"] = false
		response["error"] = "consent failed"
		writeJSON(w, http.StatusOK, response)
		return
	}
	response["stage"] = "user_consent"
	sandboxOTPs := []string{"111111", "123456", "000000", "654321", "888888", "999999", "222222", "333333"}
	candidates := make([]string, 0, len(sandboxOTPs)+1)
	if req.OTP != "" {
		candidates = append(candidates, req.OTP)
	}
	candidates = append(candidates, sandboxOTPs...)
	var otpResult map[string]any
	var validOTP string
	otpOK := false
	for _, ot := range candidates {
		otpCtx, oc := context.WithTimeout(ctx, 8*time.Second)
		r, er := validateGopayOTPViaLocalMock(otpCtx, referenceID, ot)
		oc()
		if er == nil && validateGopayOTPResult(r) == nil {
			otpResult = r
			validOTP = ot
			otpOK = true
			break
		}
	}
	if !otpOK {
		response["stage"] = "validate_otp"
		response["ok"] = false
		response["error"] = "所有 OTP 候选码均失败"
		writeJSON(w, http.StatusOK, response)
		return
	}
	response["stage"] = "validate_otp"
	response["otp_used"] = validOTP
	challengeID := gopayOTPChallengeID(otpResult)
	sandboxPINs := []string{"123456", "111111", "000000", "654321", "145236"}
	pinCands := make([]string, 0, len(sandboxPINs)+1)
	if req.PIN != "" {
		pinCands = append(pinCands, req.PIN)
	}
	pinCands = append(pinCands, sandboxPINs...)
	var validPIN string
	pinOK := false
	pinTok := ""
	for _, p := range pinCands {
		pinCtx, pc := context.WithTimeout(ctx, 8*time.Second)
		r, er := requestGopayPINTokenViaLocalMock(pinCtx, challengeID, p)
		pc()
		if er == nil && r["success"] == true {
			pinTok = gopayPINToken(r)
			if pinTok != "" {
				validPIN = p
				pinOK = true
				break
			}
		}
	}
	if !pinOK {
		response["stage"] = "pin_token"
		response["ok"] = false
		response["error"] = "所有 PIN 候选码均失败"
		writeJSON(w, http.StatusOK, response)
		return
	}
	response["stage"] = "pin_token"
	response["pin_used"] = validPIN
	pinValidate, pvErr := validateGopayPINViaLocalMock(ctx, referenceID, pinTok)
	if pvErr != nil || validateGopayPINResult(pinValidate) != nil {
		response["stage"] = "validate_pin"
		response["ok"] = false
		response["error"] = "PIN validation failed"
		writeJSON(w, http.StatusOK, response)
		return
	}
	response["stage"] = "linking_success"
	response["ok"] = true
	response["summary"] = map[string]any{"otp": validOTP, "pin": validPIN, "reference_id": referenceID, "gopay_linked": true}
	writeJSON(w, http.StatusOK, response)
}

func handleGopaySmartLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req gopaySmartLinkRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.CountryCode = strings.TrimSpace(req.CountryCode)
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	req.OTPChannel = strings.TrimSpace(req.OTPChannel)
	req.PIN = strings.TrimSpace(req.PIN)
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "account_id required")
		return
	}
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.PhoneNumber == "" {
		writeError(w, http.StatusBadRequest, "phone_number required")
		return
	}
	if req.OTPChannel == "" {
		req.OTPChannel = "whatsapp"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	response := map[string]any{
		"ok":          false,
		"strategy":    "dual-channel",
		"request":     map[string]any{"account_id": req.AccountID, "country_code": req.CountryCode, "phone_number": req.PhoneNumber, "otp_channel": req.OTPChannel},
		"diagnostics": gopayRequestDiagnostics(req.CountryCode, req.PhoneNumber, req.OTPChannel),
		"stages":      []map[string]any{},
	}
	addStage := func(name string, data map[string]any) {
		data["name"] = name
		ss, _ := response["stages"].([]map[string]any)
		response["stages"] = append(ss, data)
	}
	resolution, linkErr := createOrReuseGopayLinkingViaLocalMock(ctx, req.AccountID, gopayLink{Type: "gopay", CountryCode: req.CountryCode, PhoneNumber: req.PhoneNumber})
	response["linking_diagnostics"] = gopayLinkingDiagnostics(resolution)
	if linkErr != nil {
		response["stage"] = "force_link_failed"
		response["error"] = linkErr.Error()
		addStage("force-link-api", map[string]any{"ok": false, "error": linkErr.Error()})
		writeJSON(w, http.StatusOK, response)
		return
	}
	if resolution.ReusedExisting {
		paymentResult, paymentErr := completeGopayPaymentViaLocalMock(ctx, req.AccountID, req.PIN)
		if paymentErr != nil {
			response["ok"] = false
			response["stage"] = "gopay_payment_failed"
			if paymentResult.PinUsed == "" && len(paymentResult.PinTried) > 0 {
				response["stage"] = "payment_pin_all_failed"
			}
			response["error"] = paymentErr.Error()
			response["reused_existing"] = true
			response["conflict_reason"] = resolution.ConflictReason
			response["account"] = resolution.AccountResult
			response["stages"] = gopayReuseStagesWithPaymentFailure(resolution.AccountResult, paymentResult, paymentErr)
			response["gopay_charge"] = paymentResult.Charge
			response["gopay_payment_reference_id"] = paymentResult.PaymentReferenceID
			response["gopay_payment_validate"] = paymentResult.PaymentValidate
			response["gopay_payment_confirm"] = paymentResult.PaymentConfirm
			response["gopay_payment_challenge_id"] = paymentResult.PaymentChallengeID
			response["gopay_payment_client_id"] = paymentResult.PaymentClientID
			response["gopay_payment_pin_token"] = paymentResult.PaymentPINToken
			response["gopay_payment_process"] = paymentResult.PaymentProcess
			response["midtrans_status"] = paymentResult.MidtransStatus
			response["payment_voucher"] = buildGopayPaymentVoucher(paymentResult)
			writeJSON(w, http.StatusOK, response)
			return
		}
		response["ok"] = true
		response["stage"] = "gopay_complete"
		response["reused_existing"] = true
		response["conflict_reason"] = resolution.ConflictReason
		response["account"] = resolution.AccountResult
		response["reference_id"] = paymentResult.PaymentReferenceID
		response["stages"] = gopayReuseStagesWithPayment(resolution.AccountResult, paymentResult)
		response["gopay_charge"] = paymentResult.Charge
		response["gopay_payment_reference_id"] = paymentResult.PaymentReferenceID
		response["gopay_payment_validate"] = paymentResult.PaymentValidate
		response["gopay_payment_confirm"] = paymentResult.PaymentConfirm
		response["gopay_payment_challenge_id"] = paymentResult.PaymentChallengeID
		response["gopay_payment_client_id"] = paymentResult.PaymentClientID
		response["gopay_payment_pin_token"] = paymentResult.PaymentPINToken
		response["gopay_payment_process"] = paymentResult.PaymentProcess
		response["midtrans_status"] = paymentResult.MidtransStatus
		response["payment_voucher"] = buildGopayPaymentVoucher(paymentResult)
		response["summary"] = map[string]any{
			"reference_id":         paymentResult.PaymentReferenceID,
			"payment_reference_id": paymentResult.PaymentReferenceID,
			"transaction_id":       paymentResult.TransactionID,
			"payment_pin":          paymentResult.PinUsed,
			"gopay_linked":         true,
			"reused_existing":      true,
			"account_id":           req.AccountID,
			"country_code":         req.CountryCode,
			"phone_number":         req.PhoneNumber,
			"account_status":       stringifyJSONValue(resolution.AccountResult["account_status"]),
			"conflict_reason":      resolution.ConflictReason,
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	linkResult := resolution.LinkResult
	referenceID := stringifyJSONValue(linkResult["reference_id"])
	response["reference_id"] = referenceID
	addStage("force-link-api", map[string]any{"ok": true, "reference_id": referenceID})
	refResult, refErr := validateGopayReferenceViaLocalMock(ctx, referenceID)
	if refErr != nil || validateGopayReferenceResult(refResult) != nil {
		response["stage"] = "validate_reference_failed"
		response["error"] = "ref validation failed"
		addStage("validate-reference", map[string]any{"ok": false})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("validate-reference", map[string]any{"ok": true})
	consentResult, consentErr := requestGopayUserConsentViaLocalMock(ctx, referenceID, req.OTPChannel)
	if consentErr != nil || validateGopayUserConsentResult(consentResult) != nil {
		response["stage"] = "user_consent_failed"
		response["error"] = "consent failed"
		addStage("user-consent", map[string]any{"ok": false})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("user-consent", map[string]any{"ok": true})
	sandboxOTPs := []string{"111111", "123456", "000000", "654321", "888888", "999999", "222222", "333333"}
	otpCands := make([]string, 0, len(sandboxOTPs)+1)
	if req.OTP != "" {
		otpCands = append(otpCands, req.OTP)
	}
	otpCands = append(otpCands, sandboxOTPs...)
	var otpR map[string]any
	var vOTP string
	otpOk := false
	otpTried := make([]string, 0, len(otpCands))
	for _, o := range otpCands {
		otpCtx, oc := context.WithTimeout(ctx, 8*time.Second)
		r, er := validateGopayOTPViaLocalMock(otpCtx, referenceID, o)
		oc()
		otpTried = append(otpTried, o)
		if er == nil && validateGopayOTPResult(r) == nil {
			otpR = r
			vOTP = o
			otpOk = true
			break
		}
	}
	if !otpOk {
		response["stage"] = "otp_all_failed"
		response["error"] = "所有 OTP 候选码均失败"
		addStage("otp-enum", map[string]any{"ok": false, "tried": otpTried})
		writeJSON(w, http.StatusOK, response)
		return
	}
	challengeID := gopayOTPChallengeID(otpR)
	addStage("otp-enum", map[string]any{"ok": true, "otp": vOTP, "challenge_id": challengeID})
	sandboxPINs := []string{"123456", "111111", "000000", "654321", "145236"}
	pinCands := make([]string, 0, len(sandboxPINs)+1)
	if req.PIN != "" {
		pinCands = append(pinCands, req.PIN)
	}
	pinCands = append(pinCands, sandboxPINs...)
	var vPIN string
	pinOk := false
	pinT := ""
	pinTried := make([]string, 0, len(pinCands))
	for _, p := range pinCands {
		pinCtx, pc := context.WithTimeout(ctx, 8*time.Second)
		r, er := requestGopayPINTokenViaLocalMock(pinCtx, challengeID, p)
		pc()
		pinTried = append(pinTried, p)
		if er == nil && r["success"] == true {
			pinT = gopayPINToken(r)
			if pinT != "" {
				vPIN = p
				pinOk = true
				break
			}
		}
	}
	if !pinOk {
		response["stage"] = "pin_all_failed"
		response["error"] = "所有 PIN 候选码均失败"
		addStage("pin-enum", map[string]any{"ok": false, "tried": pinTried})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("pin-enum", map[string]any{"ok": true, "pin": vPIN})
	pvR, pvErr := validateGopayPINViaLocalMock(ctx, referenceID, pinT)
	if pvErr != nil || validateGopayPINResult(pvR) != nil {
		response["stage"] = "validate_pin_failed"
		response["error"] = "PIN validation failed"
		addStage("validate-pin", map[string]any{"ok": false})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("validate-pin", map[string]any{"ok": true, "next_action": "linking-success"})
	response["ok"] = true
	response["stage"] = "linking_success"
	response["summary"] = map[string]any{"otp": vOTP, "pin": vPIN, "reference_id": referenceID, "gopay_linked": true}
	writeJSON(w, http.StatusOK, response)
}

func handleGopayCDPOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req struct {
		OTP string `json:"otp,omitempty"`
		PIN string `json:"pin,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !isCDPReady(cdpDebuggingPort) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "CDP not ready"})
		return
	}

	findOTPInText := func(text string) string {
		text = strings.TrimSpace(text)
		if text == "" {
			return ""
		}
		matches := regexp.MustCompile(`\b(\d{6})\b`).FindStringSubmatch(text)
		if len(matches) > 1 {
			return matches[1]
		}
		return ""
	}

	var target *cdpTarget
	var err error
	targetPatterns := []string{
		"pin-web-client.gopayapi.com",
		"merchants-gws-app.gopayapi.com",
		"midtrans.com",
	}
	for _, pattern := range targetPatterns {
		target, err = findAnyTarget(cdpDebuggingPort, pattern)
		if err == nil {
			break
		}
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CDP failed")
		return
	}
	defer conn.Close()

	resultJSON, _ := executeCDPScript(conn, gopayCDPFlowScript(strings.TrimSpace(req.PIN)))
	var result map[string]any
	if resultJSON != "" {
		json.Unmarshal([]byte(resultJSON), &result)
	} else {
		result = map[string]any{"error": "empty result"}
	}
	if detected := findOTPInText(stringifyJSONValue(result["page_text_snippet"])); detected != "" {
		if stringifyJSONValue(result["detected_otp"]) == "" {
			result["detected_otp"] = detected
		}
	}
	response := map[string]any{
		"ok":                     true,
		"stage":                  firstNonEmpty(stringifyJSONValue(result["page_stage"]), "page_observed"),
		"pin_stage":              stringifyJSONValue(result["pin_stage"]),
		"pin_auto_filled":        boolMapValue(result, "pin_auto_filled"),
		"pin_auto_submitted":     boolMapValue(result, "pin_auto_submitted"),
		"pin_input_strategy":     stringifyJSONValue(result["pin_input_strategy"]),
		"balance_amount":         result["balance_amount"],
		"balance_state":          stringifyJSONValue(result["balance_state"]),
		"hubungkan_auto_clicked": boolMapValue(result, "hubungkan_auto_clicked"),
		"pay_now_auto_clicked":   boolMapValue(result, "pay_now_auto_clicked"),
		"auto_action_paused":     boolMapValue(result, "auto_action_paused"),
		"auto_action_stage":      stringifyJSONValue(result["auto_action_stage"]),
		"otp_manual_required":    boolMapValue(result, "otp_manual_required"),
		"has_otp_field":          boolMapValue(result, "has_otp_field"),
		"has_pin_field":          boolMapValue(result, "has_pin_field"),
		"cdp_url_host":           stringifyJSONValue(result["cdp_url_host"]),
		"cdp_url_path":           stringifyJSONValue(result["cdp_url_path"]),
		"result":                 result,
	}
	writeJSON(w, http.StatusOK, response)
}

func gopayCDPFlowScript(preferredPIN string) string {
	return fmt.Sprintf(`(async () => {
		const preferredPin = %s;
		const currentURL = window.location.href || '';
		const parsedURL = (() => { try { return new URL(currentURL); } catch (_err) { return null; } })();
		const urlHost = (parsedURL?.host || '').toLowerCase();
		const urlPath = (parsedURL?.pathname || '').toLowerCase();
		const actionScope = urlHost + urlPath;
		const result = {
			url: currentURL,
			cdp_url_host: urlHost,
			cdp_url_path: urlPath,
			page_stage: 'page_observed',
			pin_stage: '',
			has_otp_field: false,
			has_pin_field: false,
			pin_auto_filled: false,
			pin_auto_submitted: false,
			pin_input_strategy: '',
			balance_amount: null,
			balance_state: '',
			hubungkan_auto_clicked: false,
			pay_now_auto_clicked: false,
			auto_action_paused: false,
			auto_action_stage: '',
			otp_manual_required: false,
			detected_otp: '',
			page_text_snippet: '',
			already_handled: false,
		};
		const visible = (el) => !!el && !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
		const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
		const normalizeText = (value) => String(value || '').replace(/\s+/g, ' ').trim();
		const pageText = normalizeText((document.body && document.body.innerText) || document.documentElement.innerText || '');
		const lowerText = pageText.toLowerCase();
		result.page_text_snippet = pageText.slice(0, 1000);
		const allInputs = Array.from(document.querySelectorAll('input')).filter((el) => visible(el) && el.type !== 'hidden');
		const allButtons = Array.from(document.querySelectorAll('button')).filter(visible);
		const allActionElements = Array.from(document.querySelectorAll('button, [role="button"]')).filter(visible);
		const metaText = (el) => normalizeText([
			el?.type,
			el?.inputMode,
			el?.name,
			el?.id,
			el?.placeholder,
			el?.autocomplete,
			el?.getAttribute?.('aria-label'),
			el?.getAttribute?.('data-testid'),
			el?.getAttribute?.('data-test')
		].join(' ')).toLowerCase();
		const isDisabled = (el) => !!el && (
			!!el.disabled ||
			el.getAttribute?.('aria-disabled') === 'true' ||
			!!el.closest?.('[aria-disabled="true"]')
		);
		const elementLabel = (el) => {
			for (const value of [
				el?.innerText,
				el?.textContent,
				el?.value,
				el?.getAttribute?.('aria-label'),
				el?.getAttribute?.('title')
			]) {
				const text = normalizeText(value);
				if (text) return text;
			}
			return '';
		};
		const setNativeValue = (el, value) => {
			const proto = el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLElement.prototype;
			const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
			if (setter) {
				setter.call(el, value);
			} else {
				el.value = value;
			}
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
		};
		const clickElement = (el) => {
			if (!el || isDisabled(el)) return false;
			el.focus();
			el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
			el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
			el.dispatchEvent(new MouseEvent('click', { bubbles: true }));
			return true;
		};
		const storageGet = (key) => {
			try { return window.sessionStorage?.getItem(key) === '1'; } catch (_err) { return false; }
		};
		const storageSet = (key) => {
			try { window.sessionStorage?.setItem(key, '1'); } catch (_err) {}
		};
		const storageSetNow = (key) => {
			try { window.sessionStorage?.setItem(key, String(Date.now())); } catch (_err) {}
		};
		const storageFresh = (key, ttlMs) => {
			try {
				const value = Number(window.sessionStorage?.getItem(key) || 0);
				return Number.isFinite(value) && value > 0 && Date.now() - value < ttlMs;
			} catch (_err) {
				return false;
			}
		};
		const findOtp = (text) => {
			const match = String(text || '').match(/\b(\d{6})\b/);
			return match ? match[1] : '';
		};
		const findActionByExactText = (labels) => {
			const wanted = labels.map((label) => normalizeText(label).toLowerCase());
			return allActionElements.find((el) => !isDisabled(el) && wanted.includes(elementLabel(el).toLowerCase()));
		};
		const findStableActionByExactText = async (labels) => {
			const first = findActionByExactText(labels);
			if (!first) return null;
			await wait(350);
			return findActionByExactText(labels);
		};
		const parseIDRAmount = (text) => {
			const match = String(text || '').match(/\bRp\s*([0-9][0-9.,]*)\b/i);
			if (!match) return null;
			const digits = match[1].replace(/[^\d]/g, '');
			if (!digits) return null;
			const amount = Number(digits);
			return Number.isFinite(amount) ? amount : null;
		};
		const findBalanceAmount = () => {
			const labeledBalance = pageText.match(/(?:balance|saldo|available|gopay)[\s\S]{0,80}\bRp\s*[0-9][0-9.,]*\b/i) ||
				pageText.match(/\bRp\s*[0-9][0-9.,]*\b[\s\S]{0,80}(?:balance|saldo|available|gopay)/i);
			if (labeledBalance) {
				const amount = parseIDRAmount(labeledBalance[0]);
				if (amount !== null) return amount;
			}
			const pageMatches = Array.from(pageText.matchAll(/\bRp\s*([0-9][0-9.,]*)\b/gi));
			if (pageMatches.length === 1) {
				return parseIDRAmount(pageMatches[0][0]);
			}
			return null;
		};
		const otpInput = allInputs.find((el) => {
			const meta = metaText(el);
			return meta.includes('otp') || meta.includes('kode') || meta.includes('verification code') || meta.includes('one time');
		});
		result.has_otp_field = !!otpInput;
		result.detected_otp = findOtp(pageText) || findOtp(allInputs.map((el) => el.value || '').join(' '));
		const pinKeyword = (meta) => meta.includes('pin') || meta.includes('passcode') || meta.includes('security code');
		const otpKeyword = (meta) => meta.includes('otp') || meta.includes('kode') || meta.includes('verification code');
		const pinInputs = allInputs.filter((el) => {
			const meta = metaText(el);
			if (otpKeyword(meta)) return false;
			if (pinKeyword(meta)) return true;
			if (el.type === 'password') return true;
			if ((el.inputMode || '').toLowerCase() === 'numeric' && Number(el.maxLength || 0) === 1) return true;
			return false;
		});
		result.has_pin_field = pinInputs.length > 0;

		const otpPage = urlPath.includes('/linking/otp') || lowerText.includes('otp') || lowerText.includes('verification code');
		const pinPage = urlHost.includes('pin-web-client.gopayapi.com') || lowerText.includes('pin kamu') || lowerText.includes('masukkan pin') || lowerText.includes('enter your pin');
		const paymentContext = lowerText.includes('payment') || lowerText.includes('bayar') || lowerText.includes('pembayaran') || lowerText.includes('total') || lowerText.includes('subscribe') || lowerText.includes('subscription');
		const bindingContext = lowerText.includes('link') || lowerText.includes('hubungkan') || lowerText.includes('authorize') || lowerText.includes('otorisasi') || lowerText.includes('account') || lowerText.includes('akun');

		if (otpPage) {
			result.page_stage = 'otp_entry';
			result.otp_manual_required = true;
		} else if (pinPage) {
			result.pin_stage = paymentContext && !bindingContext ? 'payment' : 'binding';
			result.page_stage = result.pin_stage === 'payment' ? 'pin_entry_payment' : 'pin_entry_binding';
		}

		const isGoPayActionPage = urlHost.includes('gopayapi.com') ||
			(urlHost.includes('midtrans.com') && (lowerText.includes('gopay') || lowerText.includes('go pay'))) ||
			(lowerText.includes('openai llc') && (lowerText.includes('gopay') || lowerText.includes('go pay')));
		if (isGoPayActionPage) {
			const balanceAmount = findBalanceAmount();
			if (balanceAmount !== null) {
				result.balance_amount = balanceAmount;
				result.balance_state = balanceAmount === 0 ? 'rp0' : (balanceAmount === 1 ? 'rp1' : 'other');
			}
		}

		if (isGoPayActionPage && result.balance_state === 'rp0') {
			result.page_stage = 'balance_wait_rp0';
			result.auto_action_paused = true;
			result.auto_action_stage = 'balance_wait_rp0';
		} else if (isGoPayActionPage) {
			const hubungkanButton = await findStableActionByExactText(['Hubungkan']);
			const hubungkanKey = 'gopay_cdp_hubungkan_' + actionScope;
			const hubungkanCooldownKey = 'gopay_cdp_hubungkan_cooldown_' + actionScope;
			if (hubungkanButton && !storageGet(hubungkanKey) && !storageFresh(hubungkanCooldownKey, 10000)) {
				storageSet(hubungkanKey);
				storageSetNow(hubungkanCooldownKey);
				clickElement(hubungkanButton);
				result.page_stage = 'gopay_consent_hubungkan';
				result.hubungkan_auto_clicked = true;
				result.auto_action_stage = 'gopay_consent_hubungkan';
			} else if (result.balance_state === 'rp1') {
				const payNowButton = await findStableActionByExactText(['Pay now']);
				const payNowKey = 'gopay_cdp_pay_now_rp1_' + actionScope;
				const payNowCooldownKey = 'gopay_cdp_pay_now_cooldown_' + actionScope;
				if (payNowButton && !storageGet(payNowKey) && !storageFresh(payNowCooldownKey, 10000)) {
					storageSet(payNowKey);
					storageSetNow(payNowCooldownKey);
					clickElement(payNowButton);
					result.page_stage = 'pay_now_rp1';
					result.pay_now_auto_clicked = true;
					result.auto_action_stage = 'pay_now_rp1';
				} else {
					result.page_stage = 'balance_rp1_observed';
					result.auto_action_stage = 'balance_rp1_observed';
				}
			}
		}

		const handledKey = 'gopay_cdp_handled_' + result.page_stage + '_' + actionScope;
		result.already_handled = storageGet(handledKey);

		const singlePinInput = pinInputs.find((el) => Number(el.maxLength || 0) >= 6 || Number(el.maxLength || 0) === 0 || el.type === 'password');
		const splitPinInputs = pinInputs.filter((el) => Number(el.maxLength || 0) === 1).slice(0, 6);
		if (!result.auto_action_paused && result.page_stage.startsWith('pin_entry_') && preferredPin && preferredPin.length === 6 && !result.already_handled) {
			if (singlePinInput) {
				setNativeValue(singlePinInput, preferredPin);
				result.pin_auto_filled = true;
				result.pin_input_strategy = 'single_input';
			} else if (splitPinInputs.length >= 6) {
				preferredPin.split('').slice(0, 6).forEach((digit, index) => {
					setNativeValue(splitPinInputs[index], digit);
				});
				result.pin_auto_filled = true;
				result.pin_input_strategy = 'split_inputs';
			}
			if (result.pin_auto_filled) {
				const actionButton = allButtons.find((button) => {
					const text = normalizeText(button.textContent).toLowerCase();
					return text.includes('verify') || text.includes('continue') || text.includes('lanjut') || text.includes('confirm') || text.includes('pay') || text.includes('bayar') || text.includes('submit');
				});
				if (actionButton && !actionButton.disabled) {
					clickElement(actionButton);
					result.pin_auto_submitted = true;
				}
				if (result.pin_auto_submitted) {
					storageSet(handledKey);
				}
			}
		}

		return JSON.stringify(result);
	})()`, strconv.Quote(strings.TrimSpace(preferredPIN)))
}

func handleGopaySnapProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req struct{ SnapToken, CountryCode, PhoneNumber, OTP string }
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&req)
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.PhoneNumber == "" {
		req.PhoneNumber = "18120322232"
	}
	if !isCDPReady(cdpDebuggingPort) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "CDP not ready"})
		return
	}
	snapTarget, _ := findAnyTarget(cdpDebuggingPort, "midtrans.com")
	if snapTarget == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "no snap page"})
		return
	}
	snapConn, _, err := websocket.DefaultDialer.Dial(snapTarget.WebSocketDebuggerURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer snapConn.Close()
	response := map[string]any{"ok": true, "probe_steps": []map[string]any{}}
	addStep := func(n string, d map[string]any) {
		d["name"] = n
		ss, _ := response["probe_steps"].([]map[string]any)
		response["probe_steps"] = append(ss, d)
	}
	probeResult, _ := executeCDPScript(snapConn, `(async () => { const ifs=document.querySelectorAll('iframe'); const btns=Array.from(document.querySelectorAll('button')).map(b=>b.textContent.trim().slice(0,40)); return JSON.stringify({title:document.title,url:window.location.href,iframeCount:ifs.length,iframeSources:Array.from(ifs).map(i=>i.src.slice(0,120)),buttonCount:btns.length,buttons:btns.slice(0,8),pageText:(document.body?.textContent||'').trim().slice(0,500)}); })()`)
	var p map[string]any
	if probeResult != "" {
		json.Unmarshal([]byte(probeResult), &p)
	}
	addStep("probe-page", p)
	executeCDPScript(snapConn, `(async () => { const btns=document.querySelectorAll('button'); const t=Array.from(btns).find(b=>{const tx=(b.textContent||'').toLowerCase(); return tx.includes('link')||tx.includes('hubung')||tx.includes('gopay')||tx.includes('continue')||tx.includes('lanjut')||tx.includes('agree')}); if(t&&!t.disabled){t.click();return'clicked'} return'none'; })()`)
	time.Sleep(3 * time.Second)
	iframeProbe, _ := executeCDPScript(snapConn, `(async () => { const ifs=document.querySelectorAll('iframe'); return JSON.stringify({iframeCount:ifs.length,sources:Array.from(ifs).map(i=>i.src)}); })()`)
	var ip map[string]any
	if iframeProbe != "" {
		json.Unmarshal([]byte(iframeProbe), &ip)
	}
	addStep("after-click", ip)
	var iframeRef string
	if ss, ok := ip["sources"].([]any); ok {
		for _, s := range ss {
			if str, ok := s.(string); ok && strings.Contains(str, "merchants-gws-app") && strings.Contains(str, "reference=") {
				parts := strings.Split(str, "reference=")
				if len(parts) > 1 {
					iframeRef = strings.Split(parts[1], "&")[0]
				}
				break
			}
		}
	}
	var iframeResult map[string]any
	var iframeErr error
	if iframeRef != "" {
		iframeResult, iframeErr = injectIntoGoPayIframeDirectWithFilter(req.CountryCode, req.PhoneNumber, iframeRef)
	} else {
		iframeResult, iframeErr = injectIntoGoPayIframeDirect(req.CountryCode, req.PhoneNumber)
	}
	if iframeErr != nil {
		addStep("iframe-inject", map[string]any{"ok": false, "error": iframeErr.Error()})
	} else {
		addStep("iframe-inject", map[string]any{"ok": true, "result": iframeResult})
		if a, ok := iframeResult["after_submit"].(map[string]any); ok {
			response["otp_page_reached"] = true
			response["otp_info"] = a
		}
	}
	if response["otp_page_reached"] == true {
		ot := req.OTP
		if ot == "" {
			ot = "111111"
		}
		iframeTarget, _ := findAnyTarget(cdpDebuggingPort, "merchants-gws-app.gopayapi.com")
		if iframeTarget != nil {
			ifc, _, ie := websocket.DefaultDialer.Dial(iframeTarget.WebSocketDebuggerURL, nil)
			if ie == nil {
				defer ifc.Close()
				otpR, _ := executeCDPScript(ifc, fmt.Sprintf(`(async () => { const inputs=document.querySelectorAll('input'); const oi=Array.from(inputs).find(el=>el.type==='number'||el.inputMode==='numeric'); if(!oi) return JSON.stringify({ok:false,error:'no input'}); const st=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set; st.call(oi,'%s'); oi.dispatchEvent(new Event('input',{bubbles:true})); oi.dispatchEvent(new Event('change',{bubbles:true})); await new Promise(r=>setTimeout(r,500)); const btns=document.querySelectorAll('button'); const sb=Array.from(btns).find(b=>{const t=(b.textContent||'').toLowerCase(); return t.includes('verify')||t.includes('kirim')||t.includes('lanjut');}); if(sb&&!sb.disabled){sb.click();await new Promise(r=>setTimeout(r,2000))} return JSON.stringify({ok:true,otpTried:'%s',clicked:!!sb}); })()`, ot, ot))
				var om map[string]any
				if otpR != "" {
					json.Unmarshal([]byte(otpR), &om)
				}
				addStep("otp-inject", map[string]any{"ok": true, "otp": ot, "result": om})
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func handleGopayMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req struct {
		DurationS int  `json:"duration_s"`
		Stream    bool `json:"stream"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<17)).Decode(&req)
	if req.DurationS <= 0 || req.DurationS > 120 {
		req.DurationS = 30
	}
	if !isCDPReady(cdpDebuggingPort) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "CDP not ready"})
		return
	}

	targets := monitorTargets()
	activeTargets := make([]string, 0, len(targets))
	for _, descriptor := range targets {
		activeTargets = append(activeTargets, descriptor.Label+":"+descriptor.URLPattern)
	}

	if req.Stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		encoder := json.NewEncoder(w)
		_ = encoder.Encode(monitorStreamEnvelope{Type: "started", Targets: activeTargets})
		flusher.Flush()

		events, summary, runErr := runGopayMonitorSession(r.Context(), req.DurationS, func(evt monitorEvent) {
			item := evt
			_ = encoder.Encode(monitorStreamEnvelope{Type: "event", Event: &item})
			flusher.Flush()
		})
		if runErr != nil {
			_ = encoder.Encode(monitorStreamEnvelope{Type: "error", Error: runErr.Error()})
			flusher.Flush()
			return
		}
		_ = events
		_ = encoder.Encode(monitorStreamEnvelope{Type: "summary", Summary: &summary})
		flusher.Flush()
		_ = encoder.Encode(monitorStreamEnvelope{Type: "done", Summary: &summary})
		flusher.Flush()
		return
	}

	events, summary, err := runGopayMonitorSession(r.Context(), req.DurationS, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"targets": activeTargets,
		"events":  events,
		"summary": summary,
	})
}

func runGopayMonitorSession(ctx context.Context, durationS int, onEvent func(monitorEvent)) ([]monitorEvent, monitorSummaryStats, error) {
	type monitorConn struct {
		Label string
		Conn  *websocket.Conn
	}

	targets := monitorTargets()
	connections := make([]monitorConn, 0, len(targets))
	for _, descriptor := range targets {
		target, err := findAnyTarget(cdpDebuggingPort, descriptor.URLPattern)
		if err != nil || target == nil {
			continue
		}
		conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
		if err != nil {
			continue
		}
		connections = append(connections, monitorConn{Label: descriptor.Label, Conn: conn})
	}
	if len(connections) == 0 {
		return nil, monitorSummaryStats{}, errors.New("未找到可监控的 OpenAI / Stripe / Midtrans / GoPay 页面")
	}
	defer func() {
		for _, item := range connections {
			item.Conn.Close()
		}
	}()

	events := make([]monitorEvent, 0, 256)
	var mu sync.Mutex
	addEvent := func(domain, method, summary string) {
		evt := monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: domain, Method: method, Summary: summary}
		mu.Lock()
		events = append(events, evt)
		mu.Unlock()
		if onEvent != nil {
			onEvent(evt)
		}
	}

	for _, item := range connections {
		_, _ = sendCDPCommand(item.Conn, "Network.enable", map[string]any{"maxTotalBufferSize": 20000000})
		_, _ = sendCDPCommand(item.Conn, "Page.enable", nil)
		_, _ = sendCDPCommand(item.Conn, "Runtime.enable", nil)
		_, _ = sendCDPCommand(item.Conn, "Log.enable", nil)
		_, _ = sendCDPCommand(item.Conn, "Console.enable", nil)
	}

	readEvents := func(conn *websocket.Conn, label string, done <-chan struct{}) {
		if conn == nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Duration(durationS+5) * time.Second))
		for {
			select {
			case <-done:
				return
			default:
			}
			_, msg, er := conn.ReadMessage()
			if er != nil {
				return
			}
			var raw map[string]any
			if json.Unmarshal(msg, &raw) != nil {
				continue
			}
			method, _ := raw["method"].(string)
			if method == "" {
				continue
			}
			params, _ := raw["params"].(map[string]any)
			switch {
			case strings.HasPrefix(method, "Network.requestWillBeSent"):
				if rq, ok := params["request"].(map[string]any); ok {
					url := stringifyJSONValue(rq["url"])
					httpMethod := stringifyJSONValue(rq["method"])
					if url != "" && !strings.Contains(url, "favicon") {
						addEvent("Network", label+"-req", httpMethod+" "+truncateURL(url, 160))
					}
				}
			case strings.HasPrefix(method, "Network.responseReceived"):
				if rsp, ok := params["response"].(map[string]any); ok {
					url := stringifyJSONValue(rsp["url"])
					status, _ := toFloat(rsp["status"])
					if url != "" {
						addEvent("Network", label+"-resp", fmt.Sprintf("%.0f %s", status, truncateURL(url, 120)))
					}
				}
			case strings.HasPrefix(method, "Page.frameNavigated"):
				if fr, ok := params["frame"].(map[string]any); ok {
					addEvent("Page", label+"-nav", truncateURL(stringifyJSONValue(fr["url"]), 160))
				}
			case strings.HasPrefix(method, "Runtime.exceptionThrown"):
				if exc, ok := params["exceptionDetails"].(map[string]any); ok {
					text := stringifyJSONValue(exc["text"])
					if text != "" {
						addEvent("Error", label+"-exc", truncateURL(text, 200))
					}
				}
			case strings.HasPrefix(method, "Runtime.consoleAPICalled"):
				typeName := stringifyJSONValue(params["type"])
				addEvent("Console", label+"-console", firstNonEmpty(typeName, "console"))
			case strings.HasPrefix(method, "Log.entryAdded"):
				if entry, ok := params["entry"].(map[string]any); ok {
					level := stringifyJSONValue(entry["level"])
					text := stringifyJSONValue(entry["text"])
					addEvent("Console", label+"-log", strings.TrimSpace(firstNonEmpty(level, "log")+" "+truncateURL(text, 180)))
				}
			}
		}
	}

	ctx2, cancel2 := context.WithTimeout(ctx, time.Duration(durationS+15)*time.Second)
	defer cancel2()
	done := ctx2.Done()
	var wg sync.WaitGroup
	for _, item := range connections {
		wg.Add(1)
		go func(label string, conn *websocket.Conn) {
			defer wg.Done()
			readEvents(conn, label, done)
		}(item.Label, item.Conn)
	}

	select {
	case <-time.After(time.Duration(durationS) * time.Second):
	case <-ctx.Done():
	}
	cancel2()
	wg.Wait()

	mu.Lock()
	deferredEvents := append([]monitorEvent(nil), events...)
	mu.Unlock()
	summary := summarizeMonitorEvents(deferredEvents, durationS)
	return deferredEvents, summary, nil
}

func handlePricingMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req struct {
		DurationS  int      `json:"duration_s"`
		URLFilters []string `json:"url_filters"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.DurationS <= 0 || req.DurationS > 180 {
		req.DurationS = 12
	}
	if !isCDPReady(cdpDebuggingPort) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "CDP not ready"})
		return
	}

	target, err := findChatGPTTarget(cdpDebuggingPort)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "连接 CDP 失败: "+err.Error())
		return
	}
	defer conn.Close()

	filters := req.URLFilters
	if len(filters) == 0 {
		filters = []string{"pricing", "subscription", "checkout", "payments", "stripe", "offer", "promo", "trial", "eligible", "eligibility", "experiment", "feature", "plan"}
	}
	lowerFilters := make([]string, 0, len(filters))
	for _, f := range filters {
		f = strings.ToLower(strings.TrimSpace(f))
		if f != "" {
			lowerFilters = append(lowerFilters, f)
		}
	}

	time.Sleep(time.Duration(req.DurationS) * time.Second)

	script := fmt.Sprintf(`(async () => {
		const filters = %s;
		const match = (value) => {
			const text = String(value || '').toLowerCase();
			return filters.length === 0 || filters.some(f => text.includes(f));
		};
		const resources = performance.getEntriesByType('resource').map(r => ({
			name: r.name,
			initiatorType: r.initiatorType || '',
			transferSize: r.transferSize || 0,
			duration: Math.round(r.duration || 0)
		})).filter(r => match(r.name)).slice(0, 200);
		const nav = performance.getEntriesByType('navigation').map(r => ({
			name: r.name,
			type: r.type || '',
			duration: Math.round(r.duration || 0)
		}));
		const cards = Array.from(document.querySelectorAll('button, a, div, section')).map(el => ({
			text: (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 220),
			tag: el.tagName,
			role: el.getAttribute('role') || '',
			aria: el.getAttribute('aria-label') || '',
			dataTestid: el.getAttribute('data-testid') || ''
		})).filter(x => x.text && /(plus|pro|go|free|免费版|升级至|¥0|0元|trial|free trial|3000|16800|1400)/i.test(x.text)).slice(0, 80);
		const html = document.documentElement ? document.documentElement.outerHTML : '';
		const promoHints = [];
		['trial','promo','eligib','discount','coupon','free','plus-1-month-free','pricing'].forEach(k => {
			const idx = html.toLowerCase().indexOf(k);
			if (idx >= 0) {
				promoHints.push({keyword:k, snippet: html.slice(Math.max(0, idx - 180), Math.min(html.length, idx + 420))});
			}
		});
		const storageDump = {
			local: Object.keys(localStorage).slice(0, 80).map(k => ({key:k, value:String(localStorage.getItem(k) || '').slice(0, 300)})).filter(x => match(x.key + ' ' + x.value)),
			session: Object.keys(sessionStorage).slice(0, 80).map(k => ({key:k, value:String(sessionStorage.getItem(k) || '').slice(0, 300)})).filter(x => match(x.key + ' ' + x.value)),
		};
		return JSON.stringify({
			title: document.title,
			url: location.href,
			cards,
			resources,
			navigation: nav,
			promo_hints: promoHints.slice(0, 40),
			storage: storageDump,
			body_text: (document.body?.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 5000)
		});
	})()`, mustJSON(filters))

	raw, err := executeCDPScript(conn, script)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pricing inspect failed: "+err.Error())
		return
	}
	var snapshot map[string]any
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &snapshot)
	}
	resources, _ := snapshot["resources"].([]any)
	cards, _ := snapshot["cards"].([]any)
	promoHints, _ := snapshot["promo_hints"].([]any)
	storage, _ := snapshot["storage"].(map[string]any)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"summary": map[string]any{
			"duration_s":       req.DurationS,
			"resource_count":   len(resources),
			"card_count":       len(cards),
			"promo_hint_count": len(promoHints),
			"filters":          lowerFilters,
		},
		"snapshot": snapshot,
		"storage":  storage,
	})
}

func mustJSON(value any) string {
	b, _ := json.Marshal(value)
	return string(b)
}

func buildHostedCheckoutPayload() map[string]any {
	return map[string]any{
		"entry_point":      "all_plans_pricing_modal",
		"plan_name":        "chatgptplusplan",
		"checkout_ui_mode": "hosted",
		"billing_details":  map[string]string{"country": "ID", "currency": "IDR"},
		"promo_campaign":   map[string]any{"promo_campaign_id": "plus-1-month-free", "is_coupon_from_query_param": true},
	}
}

func requestHostedCheckout(ctx context.Context, client *http.Client, accessToken string) (map[string]any, int, error) {
	payloadBytes, err := json.Marshal(buildHostedCheckoutPayload())
	if err != nil {
		return nil, 0, err
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, checkoutEndpoint(), bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, 0, err
	}
	setCheckoutBackendHeaders(upstreamReq, accessToken, checkoutSession{})
	resp, err := doHTTPRequestWithRetry(ctx, client, upstreamReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("checkout API returned %d: %s", resp.StatusCode, trimForDisplay(string(body), 500))
	}
	var checkoutData map[string]any
	if err := json.Unmarshal(body, &checkoutData); err != nil {
		return nil, resp.StatusCode, err
	}
	return checkoutData, resp.StatusCode, nil
}

func shouldRetryCheckoutFlow(flow stripeSnapAccountFlowResult) bool {
	if !strings.EqualFold(stripeOriginalErrorCode(flow.Init.Body), "checkout_not_active_session") {
		return false
	}
	if flow.AccountID != "" {
		return false
	}
	return true
}

func buildExtractAccountFailureStage(flow stripeSnapAccountFlowResult, checkoutData map[string]any) map[string]any {
	stage := map[string]any{
		"ok":                    false,
		"checkout_data_keys":    mapKeys(checkoutData),
		"account_source":        flow.AccountSource,
		"account_origin_url":    flow.AccountOriginURL,
		"stripe_init":           summarizeStripeResult(flow.Init),
		"stripe_update":         summarizeStripeResult(flow.Update),
		"stripe_payment_method": summarizeStripeResult(flow.PaymentMethod),
		"stripe_confirm":        summarizeStripeResult(flow.Confirm),
		"terms_interaction":     flow.TermsInteraction,
		"checkout_approval":     summarizeCheckoutApproval(flow.Approval),
		"redirect_check": map[string]any{
			"ok":    flow.RedirectCheck.OK,
			"error": flow.RedirectCheck.Error,
		},
		"redirect": map[string]any{
			"ok":       flow.Redirect.OK,
			"error":    flow.Redirect.Error,
			"location": flow.Redirect.Location,
		},
		"attempts": flow.Attempts,
	}
	if flow.InitCheck.Error != "" {
		stage["stripe_init_total_error"] = flow.InitCheck.Error
	}
	return stage
}

func extractAccountIDWithFreshCheckoutRetry(ctx context.Context, client *http.Client, accessToken string, checkoutData map[string]any) (string, string, string, stripeSnapAccountFlowResult, map[string]any, bool, error) {
	flow := extractSnapAccountIDViaStripeFlow(ctx, client, checkoutData, accessToken, checkoutSession{})
	if flow.AccountID != "" {
		return flow.AccountID, flow.AccountSource, flow.AccountOriginURL, flow, checkoutData, false, nil
	}
	if !shouldRetryCheckoutFlow(flow) {
		fallback, source, origin := extractSnapAccountIDFromBodyWithSource(checkoutData)
		if fallback != "" {
			return fallback, source, origin, flow, checkoutData, false, nil
		}
		return "", "", "", flow, checkoutData, false, nil
	}
	refreshedCheckoutData, _, err := requestHostedCheckout(ctx, client, accessToken)
	if err != nil {
		return "", "", "", flow, checkoutData, true, err
	}
	refreshedFlow := extractSnapAccountIDViaStripeFlow(ctx, client, refreshedCheckoutData, accessToken, checkoutSession{})
	if refreshedFlow.AccountID != "" {
		return refreshedFlow.AccountID, refreshedFlow.AccountSource, refreshedFlow.AccountOriginURL, refreshedFlow, refreshedCheckoutData, true, nil
	}
	if fallback, source, origin := extractSnapAccountIDFromBodyWithSource(refreshedCheckoutData); fallback != "" {
		return fallback, source, origin, refreshedFlow, refreshedCheckoutData, true, nil
	}
	return "", "", "", refreshedFlow, refreshedCheckoutData, true, nil
}

func handleGopayFullLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req struct {
		AccessToken string `json:"access_token"`
		CountryCode string `json:"country_code"`
		PhoneNumber string `json:"phone_number"`
		OTPChannel  string `json:"otp_channel,omitempty"`
		OTP         string `json:"otp,omitempty"`
		PIN         string `json:"pin,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<19)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.AccessToken = strings.TrimSpace(req.AccessToken)
	req.CountryCode = strings.TrimSpace(req.CountryCode)
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	req.OTPChannel = strings.TrimSpace(req.OTPChannel)
	req.PIN = strings.TrimSpace(req.PIN)
	if req.AccessToken == "" {
		writeError(w, http.StatusBadRequest, "access_token required")
		return
	}
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.PhoneNumber == "" {
		writeError(w, http.StatusBadRequest, "phone_number required")
		return
	}
	if req.OTPChannel == "" {
		req.OTPChannel = "whatsapp"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	response := map[string]any{
		"ok":          false,
		"strategy":    "full-link",
		"diagnostics": gopayRequestDiagnostics(req.CountryCode, req.PhoneNumber, req.OTPChannel),
		"stages":      []map[string]any{},
	}
	addStage := func(name string, data map[string]any) {
		data["name"] = name
		ss, _ := response["stages"].([]map[string]any)
		response["stages"] = append(ss, data)
	}

	// ========================
	// Stage 0: 生成全新支付链接
	// ========================
	client, _ := newHTTPClient(proxySettings{}, false)
	checkoutData, checkoutStatus, err := requestHostedCheckout(ctx, client, req.AccessToken)
	if err != nil {
		response["stage"] = "checkout_api_failed"
		response["error"] = err.Error()
		addStage("generate-checkout", map[string]any{"ok": false, "error": err.Error()})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("generate-checkout", map[string]any{"ok": true, "status": checkoutStatus})

	// ========================
	// Stage 0.5: 从 checkout 响应中提取 account_id
	// ========================
	var accountID string
	var accountSource string
	var accountOriginURL string
	if urlVal := stringifyJSONValue(checkoutData["url"]); urlVal != "" {
		accountID = extractSnapAccountID(urlVal)
		if accountID != "" {
			accountSource = "checkout_url_direct"
			accountOriginURL = urlVal
		}
	}
	if accountID == "" {
		var flow stripeSnapAccountFlowResult
		var usedCheckoutData map[string]any
		var retriedFreshCheckout bool
		var extractErr error
		accountID, accountSource, accountOriginURL, flow, usedCheckoutData, retriedFreshCheckout, extractErr = extractAccountIDWithFreshCheckoutRetry(ctx, client, req.AccessToken, checkoutData)
		checkoutData = usedCheckoutData
		if extractErr != nil {
			response["stage"] = "checkout_retry_failed"
			response["error"] = extractErr.Error()
			addStage("extract-account", map[string]any{
				"ok":                     false,
				"error":                  extractErr.Error(),
				"retried_fresh_checkout": retriedFreshCheckout,
			})
			writeJSON(w, http.StatusOK, response)
			return
		}
		if accountID == "" {
			extractStage := buildExtractAccountFailureStage(flow, checkoutData)
			extractStage["retried_fresh_checkout"] = retriedFreshCheckout
			addStage("extract-account", extractStage)
			response["stage"] = "no_account_id"
			response["error"] = "无法从 checkout 响应中提取 Snap account_id"
			writeJSON(w, http.StatusOK, response)
			return
		}
	}
	response["account_id"] = accountID
	response["account_source"] = accountSource
	response["account_origin_url"] = accountOriginURL
	response["linking_diagnostics"] = gopayLinkingDiagnostics(gopayLinkingResolution{
		AccountSource:    accountSource,
		AccountOriginURL: accountOriginURL,
		AccountSourceNote: func() string {
			if accountSource == "checkout_url_direct" {
				return "account_id extracted directly from checkout url"
			}
			if accountSource == "stripe_redirect_guid" {
				return "account_id extracted from stripe redirect to Midtrans"
			}
			if accountSource == "checkout_body_fallback" {
				return "account_id extracted from checkout response body fallback"
			}
			return ""
		}(),
	})
	addStage("extract-account", map[string]any{"ok": true, "account_id": accountID, "account_source": accountSource, "account_origin_url": accountOriginURL})

	// ========================
	// Stage 1-6: GoPay 绑定管道（使用全新 account_id）
	// ========================
	resolution, linkErr := createOrReuseGopayLinkingViaLocalMock(ctx, accountID, gopayLink{Type: "gopay", CountryCode: req.CountryCode, PhoneNumber: req.PhoneNumber})
	response["linking_diagnostics"] = gopayLinkingDiagnostics(resolution)
	if linkErr != nil {
		response["stage"] = "force_link_failed"
		response["error"] = linkErr.Error()
		addStage("force-link-api", map[string]any{"ok": false, "error": linkErr.Error()})
		writeJSON(w, http.StatusOK, response)
		return
	}
	if resolution.ReusedExisting {
		paymentResult, paymentErr := completeGopayPaymentViaLocalMock(ctx, accountID, req.PIN)
		if paymentErr != nil {
			response["ok"] = false
			response["stage"] = "gopay_payment_failed"
			if paymentResult.PinUsed == "" && len(paymentResult.PinTried) > 0 {
				response["stage"] = "payment_pin_all_failed"
			}
			response["error"] = paymentErr.Error()
			response["reused_existing"] = true
			response["account_id"] = accountID
			response["conflict_reason"] = resolution.ConflictReason
			response["account"] = resolution.AccountResult
			response["stages"] = gopayReuseStagesWithPaymentFailure(resolution.AccountResult, paymentResult, paymentErr)
			response["gopay_charge"] = paymentResult.Charge
			response["gopay_payment_reference_id"] = paymentResult.PaymentReferenceID
			response["gopay_payment_validate"] = paymentResult.PaymentValidate
			response["gopay_payment_confirm"] = paymentResult.PaymentConfirm
			response["gopay_payment_challenge_id"] = paymentResult.PaymentChallengeID
			response["gopay_payment_client_id"] = paymentResult.PaymentClientID
			response["gopay_payment_pin_token"] = paymentResult.PaymentPINToken
			response["gopay_payment_process"] = paymentResult.PaymentProcess
			response["midtrans_status"] = paymentResult.MidtransStatus
			response["payment_voucher"] = buildGopayPaymentVoucher(paymentResult)
			writeJSON(w, http.StatusOK, response)
			return
		}
		response["ok"] = true
		response["stage"] = "gopay_complete"
		response["reused_existing"] = true
		response["account_id"] = accountID
		response["conflict_reason"] = resolution.ConflictReason
		response["account"] = resolution.AccountResult
		response["reference_id"] = paymentResult.PaymentReferenceID
		response["stages"] = gopayReuseStagesWithPayment(resolution.AccountResult, paymentResult)
		response["gopay_charge"] = paymentResult.Charge
		response["gopay_payment_reference_id"] = paymentResult.PaymentReferenceID
		response["gopay_payment_validate"] = paymentResult.PaymentValidate
		response["gopay_payment_confirm"] = paymentResult.PaymentConfirm
		response["gopay_payment_challenge_id"] = paymentResult.PaymentChallengeID
		response["gopay_payment_client_id"] = paymentResult.PaymentClientID
		response["gopay_payment_pin_token"] = paymentResult.PaymentPINToken
		response["gopay_payment_process"] = paymentResult.PaymentProcess
		response["midtrans_status"] = paymentResult.MidtransStatus
		response["payment_voucher"] = buildGopayPaymentVoucher(paymentResult)
		response["summary"] = map[string]any{
			"account_id":           accountID,
			"phone_number":         req.PhoneNumber,
			"country_code":         req.CountryCode,
			"reference_id":         paymentResult.PaymentReferenceID,
			"payment_reference_id": paymentResult.PaymentReferenceID,
			"transaction_id":       paymentResult.TransactionID,
			"payment_pin":          paymentResult.PinUsed,
			"gopay_linked":         true,
			"reused_existing":      true,
			"account_status":       stringifyJSONValue(resolution.AccountResult["account_status"]),
			"conflict_reason":      resolution.ConflictReason,
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	linkResult := resolution.LinkResult
	referenceID := stringifyJSONValue(linkResult["reference_id"])
	response["reference_id"] = referenceID
	addStage("force-link-api", map[string]any{"ok": true, "reference_id": referenceID})
	refResult, refErr := validateGopayReferenceViaLocalMock(ctx, referenceID)
	if refErr != nil || validateGopayReferenceResult(refResult) != nil {
		response["stage"] = "validate_reference_failed"
		response["error"] = "ref validation failed"
		addStage("validate-reference", map[string]any{"ok": false})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("validate-reference", map[string]any{"ok": true})
	consentResult, consentErr := requestGopayUserConsentViaLocalMock(ctx, referenceID, req.OTPChannel)
	if consentErr != nil || validateGopayUserConsentResult(consentResult) != nil {
		response["stage"] = "user_consent_failed"
		response["error"] = "consent failed"
		addStage("user-consent", map[string]any{"ok": false})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("user-consent", map[string]any{"ok": true})
	sandboxOTPs := []string{"111111", "123456", "000000", "654321", "888888", "999999", "222222", "333333"}
	otpCands := make([]string, 0, len(sandboxOTPs)+1)
	if req.OTP != "" {
		otpCands = append(otpCands, req.OTP)
	}
	otpCands = append(otpCands, sandboxOTPs...)
	var otpR map[string]any
	var vOTP string
	otpOk := false
	otpTried := make([]string, 0, len(otpCands))
	for _, o := range otpCands {
		otpCtx, oc := context.WithTimeout(ctx, 8*time.Second)
		r, er := validateGopayOTPViaLocalMock(otpCtx, referenceID, o)
		oc()
		otpTried = append(otpTried, o)
		if er == nil && validateGopayOTPResult(r) == nil {
			otpR = r
			vOTP = o
			otpOk = true
			break
		}
	}
	if !otpOk {
		response["stage"] = "otp_all_failed"
		response["error"] = "所有 OTP 候选码均失败"
		addStage("otp-enum", map[string]any{"ok": false, "tried": otpTried})
		writeJSON(w, http.StatusOK, response)
		return
	}
	challengeID := gopayOTPChallengeID(otpR)
	addStage("otp-enum", map[string]any{"ok": true, "otp": vOTP, "challenge_id": challengeID})
	sandboxPINs := []string{"123456", "111111", "000000", "654321", "145236"}
	pinCands := make([]string, 0, len(sandboxPINs)+1)
	pinCands = append(pinCands, sandboxPINs...)
	var vPIN string
	pinOk := false
	pinT := ""
	pinTried := make([]string, 0, len(pinCands))
	for _, p := range pinCands {
		pinCtx, pc := context.WithTimeout(ctx, 8*time.Second)
		r, er := requestGopayPINTokenViaLocalMock(pinCtx, challengeID, p)
		pc()
		pinTried = append(pinTried, p)
		if er == nil && r["success"] == true {
			pinT = gopayPINToken(r)
			if pinT != "" {
				vPIN = p
				pinOk = true
				break
			}
		}
	}
	if !pinOk {
		response["stage"] = "pin_all_failed"
		response["error"] = "所有 PIN 候选码均失败"
		addStage("pin-enum", map[string]any{"ok": false, "tried": pinTried})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("pin-enum", map[string]any{"ok": true, "pin": vPIN})
	pvR, pvErr := validateGopayPINViaLocalMock(ctx, referenceID, pinT)
	if pvErr != nil || validateGopayPINResult(pvR) != nil {
		response["stage"] = "validate_pin_failed"
		response["error"] = "PIN validation failed"
		addStage("validate-pin", map[string]any{"ok": false})
		writeJSON(w, http.StatusOK, response)
		return
	}
	addStage("validate-pin", map[string]any{"ok": true, "next_action": "linking-success"})
	completeGopayFullLinkPayment(ctx, response, accountID, req.CountryCode, req.PhoneNumber, firstNonEmpty(req.PIN, vPIN), resolution, defaultGopayFullLinkPaymentDeps())
	if summary, ok := response["summary"].(map[string]any); ok {
		summary["otp"] = vOTP
		summary["pin"] = vPIN
	}
	writeJSON(w, http.StatusOK, response)
}

func extractSnapAccountID(urlStr string) string {
	if urlStr == "" {
		return ""
	}
	const prefix = "app.midtrans.com/snap/v4/redirection/"
	idx := strings.Index(urlStr, prefix)
	if idx < 0 {
		return ""
	}
	return extractSnapAccountIDWithSource(urlStr)
}

func extractSnapAccountIDWithSource(urlStr string) string {
	if urlStr == "" {
		return ""
	}
	const prefix = "app.midtrans.com/snap/v4/redirection/"
	idx := strings.Index(urlStr, prefix)
	if idx < 0 {
		return ""
	}
	rest := urlStr[idx+len(prefix):]
	if hashIdx := strings.Index(rest, "#"); hashIdx >= 0 {
		rest = rest[:hashIdx]
	}
	if qIdx := strings.Index(rest, "?"); qIdx >= 0 {
		rest = rest[:qIdx]
	}
	return strings.TrimSpace(rest)
}

func extractSnapAccountIDFromBody(body any) string {
	accountID, _, _ := extractSnapAccountIDFromBodyWithSource(body)
	return accountID
}

func extractSnapAccountIDFromBodyWithSource(body any) (string, string, string) {
	if body == nil {
		return "", "", ""
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", "", ""
	}
	s := string(b)
	const prefix = "snap/v4/redirection/"
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return "", "", ""
	}
	rest := s[idx+len(prefix):]
	end := strings.IndexAny(rest, `"#?\`)
	if end < 0 {
		end = len(rest)
	}
	if end > 50 {
		end = 50
	}
	accountID := strings.TrimSpace(rest[:end])
	snippetStart := max(0, idx-120)
	snippetEnd := min(len(s), idx+220)
	return accountID, "checkout_body_fallback", s[snippetStart:snippetEnd]
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func hasFillableCheckoutInputs(probe map[string]any) bool {
	if probe == nil {
		return false
	}
	switch typed := probe["inputCount"].(type) {
	case float64:
		return typed > 0
	case int:
		return typed > 0
	case int64:
		return typed > 0
	default:
		return false
	}
}

func checkoutProbeText(probe map[string]any) string {
	if probe == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(stringifyJSONValue(probe["text"])))
}

func checkoutProbeShowsGopayChoice(probe map[string]any) bool {
	text := checkoutProbeText(probe)
	return strings.Contains(text, "gopay") && (strings.Contains(text, "银行卡") || strings.Contains(text, "bank card") || strings.Contains(text, "payment method") || strings.Contains(text, "支付方式"))
}

func checkoutAutoFillAction(probe map[string]any, isStripeFrame bool) string {
	if hasFillableCheckoutInputs(probe) {
		return "fill"
	}
	if !isStripeFrame && checkoutProbeShowsGopayChoice(probe) {
		return "activate_gopay"
	}
	return "manual"
}

func handleCheckoutResolveTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isCDPReady(cdpDebuggingPort) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "CDP not ready"})
		return
	}
	defer r.Body.Close()
	var req struct {
		OpenedURL string `json:"opened_url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.OpenedURL = strings.TrimSpace(req.OpenedURL)
	if req.OpenedURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "opened_url is required"})
		return
	}
	targets, err := getCDPTargets(cdpDebuggingPort)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	target, currentURL, err := resolveCheckoutPageTarget(targets, req.OpenedURL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             false,
			"error":          err.Error(),
			"opened_url":     req.OpenedURL,
			"candidate_urls": checkoutResolveCandidateURLs(targets),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"opened_url":  req.OpenedURL,
		"current_url": currentURL,
		"target": map[string]any{
			"id":   target.ID,
			"type": target.Type,
			"url":  target.URL,
		},
	})
}

func handleCheckoutAutoFill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isCDPReady(cdpDebuggingPort) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "CDP not ready"})
		return
	}
	defer r.Body.Close()
	var req struct {
		ExpectedURL string `json:"expected_url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.ExpectedURL = strings.TrimSpace(req.ExpectedURL)
	if req.ExpectedURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "expected_url is required"})
		return
	}
	targets, err := getCDPTargets(cdpDebuggingPort)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	pageTarget, canonicalURL, err := resolveCheckoutPageTarget(targets, req.ExpectedURL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "expected_url": req.ExpectedURL})
		return
	}
	addr := generateUSAddress()
	response := map[string]any{
		"address":      addr,
		"expected_url": req.ExpectedURL,
		"current_url":  canonicalURL,
		"page_target": map[string]any{
			"id":   pageTarget.ID,
			"type": pageTarget.Type,
			"url":  pageTarget.URL,
		},
	}
	target := pageTarget
	canonicalFillURL := canonicalURL
	response["fill_target"] = map[string]any{
		"id":   target.ID,
		"type": target.Type,
		"url":  target.URL,
	}
	conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CDP failed: "+err.Error())
		return
	}
	defer conn.Close()
	sendCDPCommand(conn, "Runtime.enable", nil)
	isStripeFrame := strings.Contains(target.URL, "elements-inner-payment") || strings.Contains(target.URL, "stripe.com/v3")
	probeScript := `(async () => { const ins=document.querySelectorAll('input'); const sels=document.querySelectorAll('select'); return JSON.stringify({url:window.location.href,title:document.title,text:(document.body?.innerText||document.body?.textContent||'').replace(/\s+/g,' ').trim().slice(0,2000),inputCount:ins.length,inputs:Array.from(ins).map(el=>({type:el.type,name:el.name,id:el.id,ph:(el.placeholder||'').slice(0,30),auto:el.autocomplete,required:el.required})),selectCount:sels.length,selects:Array.from(sels).map(el=>({name:el.name,id:el.id,ops:Array.from(el.options).slice(0,10).map(o=>o.value)}))}); })()`
	probeR, _ := executeCDPScript(conn, probeScript)
	var probe map[string]any
	if probeR != "" {
		json.Unmarshal([]byte(probeR), &probe)
	}
	response["probe"] = probe
	action := checkoutAutoFillAction(probe, isStripeFrame)
	if action == "activate_gopay" {
		clickR, _ := executeCDPScript(conn, `(async () => {
			const textOf = (el) => ((el?.textContent || '') + ' ' + (el?.getAttribute?.('aria-label') || '')).toLowerCase();
			const explicitButton = document.querySelector('[data-testid="gopay-accordion-item-button"]');
			if (explicitButton) { explicitButton.click(); await new Promise(r=>setTimeout(r,1200)); return JSON.stringify({clicked:true, via:'explicit-button'}); }
			const header = document.querySelector('[data-testid="gopay-accordion-item"] .AccordionItemHeader');
			if (header) { header.click(); await new Promise(r=>setTimeout(r,1200)); return JSON.stringify({clicked:true, via:'header'}); }
			const radio = Array.from(document.querySelectorAll('input[type="radio"], [role="radio"]')).find(el => textOf(el.closest('label,div,section') || el).includes('gopay'));
			if (radio) { radio.click(); await new Promise(r=>setTimeout(r,1200)); return JSON.stringify({clicked:true, via:'radio'}); }
			const label = Array.from(document.querySelectorAll('label, button, div, section, span')).find(el => textOf(el).includes('gopay'));
			if (label) { label.click(); await new Promise(r=>setTimeout(r,1200)); return JSON.stringify({clicked:true, via:'label'}); }
			return JSON.stringify({clicked:false});
		})()`)
		var clickMap map[string]any
		if clickR != "" {
			json.Unmarshal([]byte(clickR), &clickMap)
		}
		response["gopay_activation"] = clickMap
		updatedTargets, refreshErr := getCDPTargets(cdpDebuggingPort)
		if refreshErr == nil {
			if refreshedTarget, _, reselectErr := resolveCheckoutFillTarget(updatedTargets, req.ExpectedURL); reselectErr == nil {
				target = refreshedTarget
				response["fill_target"] = map[string]any{
					"id":   target.ID,
					"type": target.Type,
					"url":  target.URL,
				}
				if target.ID != pageTarget.ID {
					conn.Close()
					conn, _, err = websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "CDP failed: "+err.Error())
						return
					}
					defer conn.Close()
					sendCDPCommand(conn, "Runtime.enable", nil)
					isStripeFrame = strings.Contains(target.URL, "elements-inner-payment") || strings.Contains(target.URL, "stripe.com/v3")
				}
			}
		}
		probeR, _ = executeCDPScript(conn, probeScript)
		probe = map[string]any{}
		if probeR != "" {
			json.Unmarshal([]byte(probeR), &probe)
		}
		response["probe_after_gopay"] = probe
		action = checkoutAutoFillAction(probe, isStripeFrame)
	}
	if action != "fill" {
		updatedTargets, refreshErr := getCDPTargets(cdpDebuggingPort)
		if refreshErr == nil {
			if refreshedTarget, refreshedCanonicalURL, reselectErr := resolveCheckoutFillTarget(updatedTargets, req.ExpectedURL); reselectErr == nil {
				target = refreshedTarget
				canonicalFillURL = refreshedCanonicalURL
				response["fill_target"] = map[string]any{
					"id":   target.ID,
					"type": target.Type,
					"url":  target.URL,
				}
				if canonicalFillURL != canonicalURL {
					response["ok"] = false
					response["error"] = "当前支付页与本工具最近一次打开的支付链接不一致，已拒绝自动填写。"
					writeJSON(w, http.StatusOK, response)
					return
				}
				if target.ID != pageTarget.ID {
					conn.Close()
					conn, _, err = websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "CDP failed: "+err.Error())
						return
					}
					defer conn.Close()
					sendCDPCommand(conn, "Runtime.enable", nil)
					isStripeFrame = strings.Contains(target.URL, "elements-inner-payment") || strings.Contains(target.URL, "stripe.com/v3")
					probeR, _ = executeCDPScript(conn, probeScript)
					probe = map[string]any{}
					if probeR != "" {
						json.Unmarshal([]byte(probeR), &probe)
					}
					response["probe_fallback"] = probe
					action = checkoutAutoFillAction(probe, isStripeFrame)
				}
			}
		}
	}
	if action != "fill" {
		if !isStripeFrame {
			response["hint"] = "表单在 Stripe iframe 中，或当前还未切到 GoPay 表单。请先确认页面已展开 GoPay 地址区后重试。"
		}
		response["ok"] = false
		response["error"] = "未在目标支付页检测到可填写的地址表单。"
		writeJSON(w, http.StatusOK, response)
		return
	}
	fillR, _ := executeCDPScript(conn, fmt.Sprintf(`(async () => { function snv(el,v){const s=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set;s.call(el,v);el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));} function so(sel,v){const o=Array.from(sel.options).find(o=>o.value===v||o.text===v||o.text.includes(v));if(o){sel.value=o.value;sel.dispatchEvent(new Event('change',{bubbles:true}));return true}return false} function mf(el,ks){const s=(el.name+'|'+el.id+'|'+el.autocomplete+'|'+(el.placeholder||'')+'|'+(el.getAttribute('aria-label')||'')).toLowerCase();return ks.some(k=>s.includes(k))} const a={fn:%q,ln:%q,l1:%q,c:%q,s:%q,z:%q};const ins=document.querySelectorAll('input');const sls=document.querySelectorAll('select');let f={}; const fi=Array.from(ins).find(el=>mf(el,['first','given','fname','firstName','first_name','vorname']));if(fi){snv(fi,a.fn);f.first_name=true}else{const ni=Array.from(ins).find(el=>mf(el,['fullname','full_name','name']));if(ni){snv(ni,a.fn+' '+a.ln);f.full_name=true}} const li=Array.from(ins).find(el=>mf(el,['last','family','lname','lastName','surname','nachname']));if(li){snv(li,a.ln);f.last_name=true} const ai=Array.from(ins).find(el=>mf(el,['address-line1','address1','address','street','addr1','line1']));if(ai){snv(ai,a.l1);f.address=true} const ci=Array.from(ins).find(el=>mf(el,['city','town','locality','address-level2']));if(ci){snv(ci,a.c);f.city=true} const zi=Array.from(ins).find(el=>mf(el,['zip','postal','postcode','postal_code','zip_code']));if(zi){snv(zi,a.z);f.zip=true} const cs=Array.from(sls).find(el=>mf(el,['country']));if(cs){f.country=so(cs,'US'); await new Promise(r=>setTimeout(r,700));} const si=Array.from(ins).find(el=>mf(el,['state','region','province','address-level1']));const ss=Array.from(sls).find(el=>mf(el,['state','region','province']));if(ss){f.state=so(ss,a.s); if(!f.state){ await new Promise(r=>setTimeout(r,300)); f.state=so(ss,a.s); }}else if(si){snv(si,a.s);f.state=true} await new Promise(r=>setTimeout(r,600)); return JSON.stringify({url:window.location.href,filled:f,inputCount:ins.length}); })()`, addr.FirstName, addr.LastName, addr.Line1, addr.City, addr.State, addr.ZipCode))
	var fillMap map[string]any
	if fillR != "" {
		json.Unmarshal([]byte(fillR), &fillMap)
	}
	response["filled"] = fillMap
	anyF := false
	if fillMap != nil {
		if fm, _ := fillMap["filled"].(map[string]any); fm != nil {
			for _, v := range fm {
				if b, ok := v.(bool); ok && b {
					anyF = true
					break
				}
			}
		}
	}
	if !anyF {
		response["ok"] = false
		response["error"] = "地址表单已检测到，但没有任何字段被成功写入。"
		writeJSON(w, http.StatusOK, response)
		return
	}
	response["ok"] = true
	response["submitted"] = false
	writeJSON(w, http.StatusOK, response)
}
