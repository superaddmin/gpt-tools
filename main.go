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
	"math/big"
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

	"github.com/TryHarder-L/luckmail/luckmail"
	"github.com/gorilla/websocket"
)

type appConfig struct {
	CheckoutEndpoint             string   `json:"checkout_endpoint"`
	CheckoutApproveEndpoint      string   `json:"checkout_approve_endpoint"`
	CheckoutCookie               string   `json:"checkout_cookie"`
	CheckoutUserAgent            string   `json:"checkout_user_agent"`
	AuditCaptureSensitive        bool     `json:"audit_capture_sensitive"`
	LuckMailAPIKey               string   `json:"luckmail_api_key"`
	LuckMailBaseURL              string   `json:"luckmail_base_url"`
	LuckMailDefaultProjectCode   string   `json:"luckmail_default_project_code"`
	LuckMailDefaultEmailType     string   `json:"luckmail_default_email_type"`
	LuckMailDefaultDomain        string   `json:"luckmail_default_domain"`
	LuckMailTimeoutS             int      `json:"luckmail_timeout_s"`
	LuckMailIntervalS            int      `json:"luckmail_interval_s"`
	CodeViewPublicBaseURL        string   `json:"code_view_public_base_url"`
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
	PaymentMethod   string          `json:"payment_method"`
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

type luckMailCreateAndWaitRequest struct {
	ProjectCode    string `json:"project_code"`
	EmailType      string `json:"email_type"`
	Domain         string `json:"domain"`
	SpecifiedEmail string `json:"specified_email"`
	VariantMode    string `json:"variant_mode"`
	TimeoutS       int    `json:"timeout_s"`
	IntervalS      int    `json:"interval_s"`
}

type luckMailCreateAndWaitResponse struct {
	OK               bool   `json:"ok"`
	Stage            string `json:"stage"`
	Status           string `json:"status"`
	OrderNo          string `json:"order_no,omitempty"`
	EmailAddress     string `json:"email_address,omitempty"`
	VerificationCode string `json:"verification_code,omitempty"`
	MailFrom         string `json:"mail_from,omitempty"`
	MailSubject      string `json:"mail_subject,omitempty"`
	MailBodyHTML     string `json:"mail_body_html,omitempty"`
	ProjectCode      string `json:"project_code"`
	EmailType        string `json:"email_type,omitempty"`
	Domain           string `json:"domain,omitempty"`
	TimeoutS         int    `json:"timeout_s"`
	IntervalS        int    `json:"interval_s"`
	ElapsedMS        int64  `json:"elapsed_ms"`
	Error            string `json:"error,omitempty"`
}

type luckMailTokenRequest struct {
	LuckMailToken string `json:"luckmail_token"`
	TimeoutS      int    `json:"timeout_s"`
	IntervalS     int    `json:"interval_s"`
	SinceUnixMS   int64  `json:"since_unix_ms"`
}

type luckMailTokenCodeResponse struct {
	OK               bool   `json:"ok"`
	Stage            string `json:"stage"`
	EmailAddress     string `json:"email_address,omitempty"`
	Project          string `json:"project,omitempty"`
	HasNewMail       bool   `json:"has_new_mail"`
	VerificationCode string `json:"verification_code,omitempty"`
	Mail             any    `json:"mail,omitempty"`
	TimeoutS         int    `json:"timeout_s"`
	IntervalS        int    `json:"interval_s"`
	SinceUnixMS      int64  `json:"since_unix_ms,omitempty"`
	ElapsedMS        int64  `json:"elapsed_ms"`
	Error            string `json:"error,omitempty"`
}

type luckMailTokenMailItemResponse struct {
	MessageID  string `json:"message_id"`
	From       string `json:"from"`
	Subject    string `json:"subject"`
	ReceivedAt string `json:"received_at"`
}

type luckMailTokenMailsResponse struct {
	OK            bool                            `json:"ok"`
	Stage         string                          `json:"stage"`
	EmailAddress  string                          `json:"email_address,omitempty"`
	Project       string                          `json:"project,omitempty"`
	WarrantyUntil string                          `json:"warranty_until,omitempty"`
	Mails         []luckMailTokenMailItemResponse `json:"mails"`
	ElapsedMS     int64                           `json:"elapsed_ms"`
	Error         string                          `json:"error,omitempty"`
}

type luckMailPurchaseListRequest struct {
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
	Keyword      string `json:"keyword"`
	UserDisabled *int   `json:"user_disabled,omitempty"`
}

type luckMailPurchaseItemResponse struct {
	ID            int    `json:"id"`
	EmailAddress  string `json:"email_address"`
	LuckMailToken string `json:"luckmail_token,omitempty"`
	ProjectName   string `json:"project_name,omitempty"`
	Status        int    `json:"status"`
	TagName       string `json:"tag_name,omitempty"`
	UserDisabled  int    `json:"user_disabled"`
	WarrantyUntil string `json:"warranty_until,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

type luckMailPurchasesResponse struct {
	OK        bool                           `json:"ok"`
	Stage     string                         `json:"stage"`
	List      []luckMailPurchaseItemResponse `json:"list"`
	Total     int                            `json:"total"`
	Page      int                            `json:"page"`
	PageSize  int                            `json:"page_size"`
	Selected  *luckMailPurchaseItemResponse  `json:"selected,omitempty"`
	ElapsedMS int64                          `json:"elapsed_ms"`
	Error     string                         `json:"error,omitempty"`
}

type gptPlusRecordRequest struct {
	EmailAddress  string `json:"email_address"`
	LuckMailToken string `json:"luckmail_token"`
	PaymentStage  string `json:"payment_stage,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
	CheckoutURL   string `json:"checkout_url,omitempty"`
}

type gptPlusRecordFile struct {
	OK            bool   `json:"ok"`
	Stage         string `json:"stage"`
	SavedAt       string `json:"saved_at"`
	EmailAddress  string `json:"email_address"`
	LuckMailToken string `json:"luckmail_token"`
	QueryURL      string `json:"query_url"`
	CodeViewURL   string `json:"code_view_url"`
	PaymentStage  string `json:"payment_stage,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
	CheckoutURL   string `json:"checkout_url,omitempty"`
}

type gptPlusRecordResponse struct {
	OK           bool   `json:"ok"`
	Stage        string `json:"stage"`
	EmailAddress string `json:"email_address,omitempty"`
	QueryURL     string `json:"query_url,omitempty"`
	CodeViewURL  string `json:"code_view_url,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
	Error        string `json:"error,omitempty"`
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

type paypalCheckoutFlowResult struct {
	OK            bool             `json:"ok"`
	Error         string           `json:"error,omitempty"`
	URL           string           `json:"url,omitempty"`
	Update        stripeInitResult `json:"update,omitempty"`
	PaymentMethod stripeInitResult `json:"payment_method,omitempty"`
	Confirm       stripeInitResult `json:"confirm,omitempty"`
	Details       stripeInitResult `json:"details,omitempty"`
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
	mux.HandleFunc("/api/incognito/diagnostics", handleIncognitoDiagnostics)
	mux.HandleFunc("/api/login/click", handleLoginClick)
	mux.HandleFunc("/api/login/email-fill", handleLoginEmailFill)
	mux.HandleFunc("/api/login/code-fill", handleLoginCodeFill)
	mux.HandleFunc("/api/session/fetch", handleSessionFetch)
	mux.HandleFunc("/api/gopay/force-link", handleGopayForceLink)
	mux.HandleFunc("/api/gopay/auto-link", handleGopayAutoLink)
	mux.HandleFunc("/api/gopay/cdp-otp", handleGopayCDPOTP)
	mux.HandleFunc("/api/gopay/smart-link", handleGopaySmartLink)
	mux.HandleFunc("/api/gopay/snap-probe", handleGopaySnapProbe)
	mux.HandleFunc("/api/gopay/monitor", handleGopayMonitor)
	mux.HandleFunc("/api/pricing/monitor", handlePricingMonitor)
	mux.HandleFunc("/api/luckmail/create-and-wait", handleLuckMailCreateAndWait)
	mux.HandleFunc("/api/luckmail/token-code", handleLuckMailTokenCode)
	mux.HandleFunc("/api/luckmail/token-mails", handleLuckMailTokenMails)
	mux.HandleFunc("/api/luckmail/purchases", handleLuckMailPurchases)
	mux.HandleFunc("/api/gptpls/record", handleGPTPlusRecord)
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
		if fileName == "mail-code" {
			fileName = "mail-code.html"
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

type loginEmailFillRequest struct {
	Email    string `json:"email"`
	TimeoutS int    `json:"timeout_s"`
}

type loginCodeFillRequest struct {
	Code     string `json:"code"`
	TimeoutS int    `json:"timeout_s"`
}

type loginClickRequest struct {
	TimeoutS int `json:"timeout_s"`
}

type loginClickResult struct {
	OK                   bool             `json:"ok"`
	Stage                string           `json:"stage"`
	TargetURL            string           `json:"target_url,omitempty"`
	TargetTitle          string           `json:"target_title,omitempty"`
	ClickedLogin         bool             `json:"clicked_login"`
	LoginButtonText      string           `json:"login_button_text,omitempty"`
	EmailModeSwitched    bool             `json:"email_mode_switched"`
	EmailModeButtonText  string           `json:"email_mode_button_text,omitempty"`
	EmailInputReady      bool             `json:"email_input_ready"`
	LoginSurfaceReady    bool             `json:"login_surface_ready"`
	CurrentURL           string           `json:"current_url,omitempty"`
	PageReadyState       string           `json:"page_ready_state,omitempty"`
	NotInteractiveReason string           `json:"not_interactive_reason,omitempty"`
	RootCause            string           `json:"root_cause,omitempty"`
	DecisionStrategy     string           `json:"decision_strategy,omitempty"`
	ContextRebuilt       bool             `json:"context_rebuilt"`
	PageMonitor          map[string]any   `json:"page_monitor,omitempty"`
	InteractiveWaitMS    int64            `json:"interactive_wait_ms,omitempty"`
	WaitExtendedMS       int64            `json:"wait_extended_ms,omitempty"`
	ElapsedMS            int64            `json:"elapsed_ms"`
	Attempts             []map[string]any `json:"attempts,omitempty"`
	Error                string           `json:"error,omitempty"`
}

type loginEmailFillResult struct {
	OK                  bool             `json:"ok"`
	Stage               string           `json:"stage"`
	TargetURL           string           `json:"target_url,omitempty"`
	TargetTitle         string           `json:"target_title,omitempty"`
	ClickedLogin        bool             `json:"clicked_login"`
	LoginButtonText     string           `json:"login_button_text,omitempty"`
	EmailModeSwitched   bool             `json:"email_mode_switched"`
	EmailModeButtonText string           `json:"email_mode_button_text,omitempty"`
	LoginSurfaceReady   bool             `json:"login_surface_ready"`
	EmailFilled         bool             `json:"email_filled"`
	EmailInputLabel     string           `json:"email_input_label,omitempty"`
	ClickedContinue     bool             `json:"clicked_continue"`
	ContinueButtonText  string           `json:"continue_button_text,omitempty"`
	VerificationReady   bool             `json:"verification_ready"`
	VerificationHint    string           `json:"verification_hint,omitempty"`
	OperationTimedOut   bool             `json:"operation_timed_out"`
	PageError           string           `json:"page_error,omitempty"`
	CurrentURL          string           `json:"current_url,omitempty"`
	WaitExtendedMS      int64            `json:"wait_extended_ms,omitempty"`
	ElapsedMS           int64            `json:"elapsed_ms"`
	Attempts            []map[string]any `json:"attempts,omitempty"`
	Error               string           `json:"error,omitempty"`
}

type loginCodeFillResult struct {
	OK               bool             `json:"ok"`
	Stage            string           `json:"stage"`
	TargetURL        string           `json:"target_url,omitempty"`
	TargetTitle      string           `json:"target_title,omitempty"`
	CodeFilled       bool             `json:"code_filled"`
	CodeInputLabel   string           `json:"code_input_label,omitempty"`
	CodeLength       int              `json:"code_length,omitempty"`
	CodeSubmitted    bool             `json:"code_submitted"`
	CodeRejected     bool             `json:"code_rejected"`
	SubmitButtonText string           `json:"submit_button_text,omitempty"`
	ProfileFilled    bool             `json:"profile_filled"`
	ProfileName      string           `json:"profile_name,omitempty"`
	ProfileAge       int              `json:"profile_age,omitempty"`
	ProfileButton    string           `json:"profile_button_text,omitempty"`
	LoginCompleted   bool             `json:"login_completed"`
	CompletionHint   string           `json:"completion_hint,omitempty"`
	CurrentURL       string           `json:"current_url,omitempty"`
	ElapsedMS        int64            `json:"elapsed_ms"`
	Attempts         []map[string]any `json:"attempts,omitempty"`
	Error            string           `json:"error,omitempty"`
}

const cdpDebuggingPort = 9223

type japaneseProfile struct {
	Name string
	Age  int
}

var usedJapaneseProfileNames = struct {
	mu    sync.Mutex
	names map[string]struct{}
}{names: map[string]struct{}{}}

var japaneseProfileSurnames = []string{
	"佐藤", "鈴木", "高橋", "田中", "伊藤", "渡辺", "山本", "中村", "小林", "加藤",
	"吉田", "山田", "佐々木", "山口", "松本", "井上", "木村", "林", "清水", "斎藤",
	"山崎", "森", "池田", "橋本", "阿部", "石川", "前田", "藤田", "岡田", "後藤",
	"長谷川", "村上", "近藤", "石井", "坂本", "遠藤", "青木", "藤井", "西村", "福田",
	"太田", "三浦", "藤原", "岡本", "松田", "中川", "中島", "原田", "小川", "竹内",
}

var japaneseProfileGivenNames = []string{
	"陽葵", "凛", "結衣", "芽依", "葵", "紬", "澪", "美月", "莉子", "心春",
	"咲良", "杏", "結菜", "彩葉", "琴音", "花音", "七海", "美咲", "愛莉", "優奈",
	"蓮", "湊", "悠真", "陽翔", "樹", "蒼", "朝陽", "律", "大和", "奏太",
	"颯真", "悠人", "陽向", "新", "伊織", "拓海", "翔太", "健太", "直樹", "隼人",
	"春樹", "悠斗", "海斗", "亮太", "大輝", "優斗", "晴翔", "一真", "瑛太", "怜",
}

func randomIntBelow(limit int) int {
	if limit <= 0 {
		return 0
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err == nil {
		return int(value.Int64())
	}
	return int(time.Now().UnixNano() % int64(limit))
}

func nextRandomJapaneseProfile() japaneseProfile {
	total := len(japaneseProfileSurnames) * len(japaneseProfileGivenNames)
	age := 18 + randomIntBelow(31)

	usedJapaneseProfileNames.mu.Lock()
	defer usedJapaneseProfileNames.mu.Unlock()
	for attempts := 0; attempts < total*2; attempts++ {
		name := japaneseProfileSurnames[randomIntBelow(len(japaneseProfileSurnames))] + " " + japaneseProfileGivenNames[randomIntBelow(len(japaneseProfileGivenNames))]
		if _, exists := usedJapaneseProfileNames.names[name]; !exists {
			usedJapaneseProfileNames.names[name] = struct{}{}
			return japaneseProfile{Name: name, Age: age}
		}
	}
	for _, surname := range japaneseProfileSurnames {
		for _, given := range japaneseProfileGivenNames {
			name := surname + " " + given
			if _, exists := usedJapaneseProfileNames.names[name]; !exists {
				usedJapaneseProfileNames.names[name] = struct{}{}
				return japaneseProfile{Name: name, Age: age}
			}
		}
	}
	name := fmt.Sprintf("%s %s %d", japaneseProfileSurnames[randomIntBelow(len(japaneseProfileSurnames))], japaneseProfileGivenNames[randomIntBelow(len(japaneseProfileGivenNames))], time.Now().UnixNano())
	usedJapaneseProfileNames.names[name] = struct{}{}
	return japaneseProfile{Name: name, Age: age}
}

type managedIncognitoState struct {
	mu               sync.Mutex
	openedURL        string
	targetIDs        map[string]time.Time
	openedAt         time.Time
	browserContextID string
}

var latestManagedIncognito = managedIncognitoState{targetIDs: map[string]time.Time{}}
var managedChromeLaunchMu sync.Mutex

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

func launchManagedIncognitoChrome(targetURL string, newWindow bool) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		targetURL = "https://chatgpt.com/"
	}
	cmd := exec.Command("cmd", buildChromeArgs(targetURL, newWindow)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	rememberManagedIncognitoOpen(targetURL)
	return nil
}

func ensureManagedCDPReady(ctx context.Context, targetURL string) (bool, error) {
	if isCDPReady(cdpDebuggingPort) {
		return false, nil
	}
	managedChromeLaunchMu.Lock()
	defer managedChromeLaunchMu.Unlock()
	if isCDPReady(cdpDebuggingPort) {
		return false, nil
	}
	if err := launchManagedIncognitoChrome(targetURL, true); err != nil {
		return false, err
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		default:
		}
		if isCDPReady(cdpDebuggingPort) {
			return true, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return true, errors.New("Chrome 已重新启动，但 CDP 端口 9223 未在限定时间内就绪")
}

func currentManagedBrowserContextID() string {
	latestManagedIncognito.mu.Lock()
	defer latestManagedIncognito.mu.Unlock()
	return strings.TrimSpace(latestManagedIncognito.browserContextID)
}

func rememberManagedIncognitoContextOpen(openedURL, browserContextID, targetID string) {
	latestManagedIncognito.mu.Lock()
	latestManagedIncognito.openedURL = strings.TrimSpace(openedURL)
	latestManagedIncognito.openedAt = time.Now()
	latestManagedIncognito.browserContextID = strings.TrimSpace(browserContextID)
	latestManagedIncognito.targetIDs = map[string]time.Time{}
	if targetID = strings.TrimSpace(targetID); targetID != "" {
		latestManagedIncognito.targetIDs[targetID] = time.Now()
	}
	latestManagedIncognito.mu.Unlock()
}

func dialBrowserCDP(ctx context.Context) (*websocket.Conn, error) {
	version, err := fetchCDPVersion(cdpDebuggingPort)
	if err != nil {
		return nil, err
	}
	wsURL := strings.TrimSpace(stringifyJSONValue(version["webSocketDebuggerUrl"]))
	if wsURL == "" {
		return nil, errors.New("CDP browser websocket url 为空")
	}
	dialer := websocket.Dialer{HandshakeTimeout: 2500 * time.Millisecond}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func findTargetByID(port int, targetID string) (*cdpTarget, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil, errors.New("target id is required")
	}
	targets, err := getCDPTargets(port)
	if err != nil {
		return nil, err
	}
	for i := range targets {
		if strings.TrimSpace(targets[i].ID) == targetID {
			return &targets[i], nil
		}
	}
	return nil, errors.New("未找到指定 target: " + targetID)
}

func waitForManagedTargetNavigation(ctx context.Context, targetID string, timeout time.Duration) (*cdpTarget, error) {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		target, err := findTargetByID(cdpDebuggingPort, targetID)
		if err == nil && target != nil {
			urlText := strings.TrimSpace(target.URL)
			if target.WebSocketDebuggerURL != "" && isManagedLoginCandidateURL(urlText) {
				return target, nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil, errors.New("新浏览器上下文页面未在限定时间内完成导航")
}

func rebuildManagedLoginBrowserContext(ctx context.Context, currentTargetID, targetURL string) (map[string]any, error) {
	if strings.TrimSpace(targetURL) == "" {
		targetURL = "https://chatgpt.com/"
	}
	if _, err := ensureManagedCDPReady(ctx, targetURL); err != nil {
		return nil, err
	}
	conn, err := dialBrowserCDP(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	result := map[string]any{
		"attempted":           true,
		"target_url":          targetURL,
		"current_target_id":   strings.TrimSpace(currentTargetID),
		"previous_context_id": currentManagedBrowserContextID(),
	}
	if previousContextID := currentManagedBrowserContextID(); previousContextID != "" {
		if _, disposeErr := sendCDPCommand(conn, "Target.disposeBrowserContext", map[string]any{"browserContextId": previousContextID}); disposeErr != nil {
			result["dispose_previous_context_error"] = disposeErr.Error()
		} else {
			result["disposed_previous_context"] = true
		}
	}
	createContextResp, err := sendCDPCommand(conn, "Target.createBrowserContext", map[string]any{})
	if err != nil {
		return nil, err
	}
	browserContextID := strings.TrimSpace(stringifyJSONValue(createContextResp.Result["browserContextId"]))
	if browserContextID == "" {
		return nil, errors.New("CDP 未返回 browserContextId")
	}
	result["browser_context_id"] = browserContextID

	createTargetResp, err := sendCDPCommand(conn, "Target.createTarget", map[string]any{
		"url":              targetURL,
		"browserContextId": browserContextID,
		"newWindow":        true,
	})
	if err != nil {
		result["create_target_new_window_error"] = err.Error()
		createTargetResp, err = sendCDPCommand(conn, "Target.createTarget", map[string]any{
			"url":              targetURL,
			"browserContextId": browserContextID,
		})
		if err != nil {
			return nil, err
		}
		result["create_target_fallback_used"] = true
	}
	targetID := strings.TrimSpace(stringifyJSONValue(createTargetResp.Result["targetId"]))
	if targetID == "" {
		return nil, errors.New("CDP 未返回新 targetId")
	}
	result["new_target_id"] = targetID
	rememberManagedIncognitoContextOpen(targetURL, browserContextID, targetID)

	if currentTargetID = strings.TrimSpace(currentTargetID); currentTargetID != "" {
		if _, closeErr := sendCDPCommand(conn, "Target.closeTarget", map[string]any{"targetId": currentTargetID}); closeErr != nil {
			result["close_previous_target_error"] = closeErr.Error()
		} else {
			result["closed_previous_target"] = true
		}
	}

	target, waitErr := waitForManagedTargetNavigation(ctx, targetID, 12*time.Second)
	if waitErr != nil {
		result["target_wait_error"] = waitErr.Error()
		return result, nil
	}
	result["navigated_target_url"] = target.URL
	result["navigated_target_title"] = target.Title

	probe, probeErr := probeLoginTarget(ctx, target)
	if probeErr != nil {
		result["probe_error"] = probeErr.Error()
		return result, nil
	}
	rootCause, strategy, _ := classifyLoginHomepageState(probe)
	result["page_probe"] = probe
	result["root_cause"] = rootCause
	result["decision_strategy"] = strategy
	result["succeeded"] = rootCause != "already_logged_in_session" && rootCause != "already_logged_in_chat_context"
	return result, nil
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
	if err := launchManagedIncognitoChrome(req.URL, req.NewWindow); err != nil {
		log.Printf("launch chrome incognito failed: %v", err)
		writeError(w, http.StatusInternalServerError, "无法启动 Chrome 无痕窗口: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "url": req.URL, "new_window": req.NewWindow,
	})
}

func handleIncognitoDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, buildIncognitoDiagnostics(r.Context()))
}

func buildIncognitoDiagnostics(ctx context.Context) map[string]any {
	startedAt := time.Now()
	state := managedIncognitoSnapshot()
	result := map[string]any{
		"ok":                         true,
		"stage":                      "incognito_diagnostics",
		"checked_at":                 startedAt.Format(time.RFC3339),
		"cdp_port":                   cdpDebuggingPort,
		"user_data_dir":              incognitoUserDataDir(),
		"managed_opened_url":         state["opened_url"],
		"managed_opened_at":          state["opened_at"],
		"managed_age_seconds":        state["age_seconds"],
		"managed_target_ids":         state["target_ids"],
		"managed_browser_context_id": state["browser_context_id"],
	}

	versionStarted := time.Now()
	version, versionErr := fetchCDPVersion(cdpDebuggingPort)
	result["cdp_version_latency_ms"] = time.Since(versionStarted).Milliseconds()
	if versionErr != nil {
		result["cdp_ready"] = false
		result["conclusion"] = incognitoDiagnosticsConclusion(false, 0, nil, versionErr)
		result["elapsed_ms"] = time.Since(startedAt).Milliseconds()
		return result
	}
	result["cdp_ready"] = true
	result["browser"] = version["Browser"]
	result["protocol_version"] = version["Protocol-Version"]
	result["websocket_debugger_url"] = safeNetworkDiagnosticURL(stringifyJSONValue(version["webSocketDebuggerUrl"]))

	targetsStarted := time.Now()
	targets, targetsErr := getCDPTargets(cdpDebuggingPort)
	result["targets_latency_ms"] = time.Since(targetsStarted).Milliseconds()
	if targetsErr != nil {
		result["target_error"] = targetsErr.Error()
		result["conclusion"] = incognitoDiagnosticsConclusion(true, 0, nil, targetsErr)
		result["elapsed_ms"] = time.Since(startedAt).Milliseconds()
		return result
	}

	sanitizedTargets := make([]map[string]any, 0, len(targets))
	targetTypeCounts := map[string]int{}
	for _, target := range targets {
		targetTypeCounts[target.Type]++
		if len(sanitizedTargets) >= 40 {
			continue
		}
		sanitizedTargets = append(sanitizedTargets, map[string]any{
			"id":            target.ID,
			"type":          target.Type,
			"url":           safeNetworkDiagnosticURL(target.URL),
			"title":         safeIncognitoDiagnosticText(target.Title),
			"managed_score": managedIncognitoTargetScore(target),
		})
	}
	result["target_count"] = len(targets)
	result["target_type_counts"] = targetTypeCounts
	result["targets"] = sanitizedTargets

	selected := selectIncognitoDiagnosticTarget(targets)
	if selected == nil {
		result["conclusion"] = incognitoDiagnosticsConclusion(true, len(targets), nil, nil)
		result["elapsed_ms"] = time.Since(startedAt).Milliseconds()
		return result
	}
	result["selected_target"] = map[string]any{
		"id":            selected.ID,
		"type":          selected.Type,
		"url":           safeNetworkDiagnosticURL(selected.URL),
		"title":         safeIncognitoDiagnosticText(selected.Title),
		"managed_score": managedIncognitoTargetScore(*selected),
	}

	probe, probeErr := probeIncognitoTarget(ctx, *selected)
	if probeErr != nil {
		result["probe_error"] = probeErr.Error()
	} else {
		result["page_probe"] = probe
	}
	result["conclusion"] = incognitoDiagnosticsConclusion(true, len(targets), probe, probeErr)
	result["elapsed_ms"] = time.Since(startedAt).Milliseconds()
	return result
}

func managedIncognitoSnapshot() map[string]any {
	latestManagedIncognito.mu.Lock()
	defer latestManagedIncognito.mu.Unlock()
	targetIDs := make([]string, 0, len(latestManagedIncognito.targetIDs))
	for targetID := range latestManagedIncognito.targetIDs {
		targetIDs = append(targetIDs, targetID)
	}
	sort.Strings(targetIDs)
	openedAt := ""
	ageSeconds := int64(0)
	if !latestManagedIncognito.openedAt.IsZero() {
		openedAt = latestManagedIncognito.openedAt.Format(time.RFC3339)
		ageSeconds = int64(time.Since(latestManagedIncognito.openedAt).Seconds())
	}
	return map[string]any{
		"opened_url":         safeNetworkDiagnosticURL(latestManagedIncognito.openedURL),
		"opened_at":          openedAt,
		"age_seconds":        ageSeconds,
		"target_ids":         targetIDs,
		"browser_context_id": strings.TrimSpace(latestManagedIncognito.browserContextID),
	}
}

func fetchCDPVersion(port int) (map[string]any, error) {
	client := &http.Client{Timeout: 2500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return nil, fmt.Errorf("无法连接 CDP version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CDP version 状态异常: %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 CDP version 失败: %w", err)
	}
	return payload, nil
}

func selectIncognitoDiagnosticTarget(targets []cdpTarget) *cdpTarget {
	bestScore := -1
	var best *cdpTarget
	for i := range targets {
		target := targets[i]
		if target.Type != "page" {
			continue
		}
		urlText := strings.ToLower(strings.TrimSpace(target.URL))
		if urlText == "" || strings.HasPrefix(urlText, "devtools://") || strings.Contains(urlText, "localhost:18473") || strings.Contains(urlText, "127.0.0.1:18473") {
			continue
		}
		score := managedIncognitoTargetScore(target)
		if strings.Contains(urlText, "chatgpt.com") || strings.Contains(urlText, "auth.openai.com") {
			score += 40
		}
		if strings.Contains(urlText, "pay.openai.com") || strings.Contains(urlText, "checkout") {
			score += 25
		}
		if score > bestScore {
			bestScore = score
			best = &targets[i]
		}
	}
	return best
}

func probeIncognitoTarget(ctx context.Context, target cdpTarget) (map[string]any, error) {
	probeStarted := time.Now()
	dialer := websocket.Dialer{HandshakeTimeout: 2500 * time.Millisecond}
	conn, _, err := dialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("连接目标页 CDP 失败: %w", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(4 * time.Second)); err != nil {
		return nil, err
	}
	if _, err := sendCDPCommand(conn, "Runtime.enable", nil); err != nil {
		return nil, err
	}
	raw, err := executeCDPScript(conn, loginTargetProbeScript())
	if err != nil {
		return nil, err
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, fmt.Errorf("解析页面性能探针失败: %w", err)
	}
	rootCause, strategy, _ := classifyLoginHomepageState(probe)
	probe["root_cause"] = rootCause
	probe["decision_strategy"] = strategy
	probe["probe_latency_ms"] = time.Since(probeStarted).Milliseconds()
	probe["url"] = safeNetworkDiagnosticURL(stringifyJSONValue(probe["url"]))
	probe["title"] = safeIncognitoDiagnosticText(stringifyJSONValue(probe["title"]))
	return probe, nil
}

func safeIncognitoDiagnosticText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.Contains(text, "?") || strings.Contains(text, "#") || strings.Contains(text, "://") {
		return safeNetworkDiagnosticURL(text)
	}
	return sanitizeNetworkDiagnosticText(text)
}

func incognitoDiagnosticsConclusion(cdpReady bool, targetCount int, probe map[string]any, probeErr error) map[string]any {
	if !cdpReady {
		return map[string]any{
			"status":     "cdp_not_ready",
			"message":    "9223 CDP 端口未就绪；卡顿可能来自 Chrome 未启动、端口被占用、或无痕实例已退出。",
			"next_steps": []string{"点击打开无痕窗口重新拉起 Chrome", "检查 netstat 中 9223 是否 LISTENING", "必要时清理临时 Chrome profile 后重试"},
		}
	}
	if targetCount == 0 {
		return map[string]any{
			"status":     "target_missing",
			"message":    "CDP 已就绪，但没有发现可诊断页面；卡顿可能发生在 Chrome 启动或页面尚未打开阶段。",
			"next_steps": []string{"先打开 ChatGPT 无痕窗口", "等待页面加载后刷新诊断"},
		}
	}
	if probeErr != nil {
		return map[string]any{
			"status":     "probe_failed",
			"message":    "已找到页面，但页面性能探针执行失败；可能是页面跳转、目标关闭或 CDP 正在切换。",
			"next_steps": []string{"等待 2 秒后刷新诊断", "如果持续失败，重新打开无痕窗口"},
		}
	}
	readyState := strings.ToLower(stringifyJSONValue(probe["ready_state"]))
	rootCause := stringifyJSONValue(probe["root_cause"])
	notInteractiveReason := stringifyJSONValue(probe["not_interactive_reason"])
	routeKind := stringifyJSONValue(probe["route_kind"])
	if routeKind == "checkout" || rootCause == "checkout_page_active" {
		return map[string]any{"status": "checkout_page_active", "message": "当前无痕页停留在 checkout/支付页，这不是可点击登录按钮的首页上下文。", "next_steps": []string{"先回到 https://chatgpt.com/ 首页", "再执行登录按钮识别", "避免在 checkout 页反复等待或点击登录"}}
	}
	if rootCause == "already_logged_in_session" || rootCause == "already_logged_in_chat_context" {
		return map[string]any{"status": "already_logged_in_session", "message": "当前无痕页已经处于登录后的 ChatGPT 工作区，无需再寻找登录按钮。", "next_steps": []string{"直接执行 Session 读取或后续已登录流程", "若确实要重新登录，可先退出当前账号再返回首页"}}
	}
	if nav, ok := probe["nav"].(map[string]any); ok {
		duration, _ := numberFromAny(nav["duration_ms"])
		responseStart, _ := numberFromAny(nav["response_start_ms"])
		if duration >= 12000 {
			return map[string]any{"status": "page_load_slow", "message": "页面加载耗时偏高，主要怀疑网络、代理、DNS 或 OpenAI 登录页风控资源加载慢。", "next_steps": []string{"检查代理节点对 chatgpt.com/auth.openai.com 是否稳定", "观察慢资源列表", "尝试清理临时 Chrome profile 后重开"}}
		}
		if responseStart >= 4000 {
			return map[string]any{"status": "network_latency_high", "message": "页面首包响应较慢，更像网络/代理链路导致卡顿。", "next_steps": []string{"切换更稳定代理节点", "检查系统代理是否对调试 Chrome 生效", "手动打开 chatgpt.com 对比速度"}}
		}
	}
	if readyState != "complete" {
		return map[string]any{"status": "page_loading", "message": "页面仍未 complete，当前卡顿可能是资源还在加载或登录页跳转中。", "next_steps": []string{"等待页面稳定后刷新诊断", "如果长期停留 loading，优先检查网络/代理"}}
	}
	if rootCause == "page_navigation_pending" || notInteractiveReason == "about_blank" {
		return map[string]any{"status": "navigation_pending", "message": "页面目标已创建，但浏览器仍停留在 about:blank 或导航尚未真正落地，此时不适合点击登录按钮。", "next_steps": []string{"继续等待页面真正进入 chatgpt.com DOM", "若长时间停留 about:blank，检查代理或重开无痕窗口"}}
	}
	if rootCause == "network_delay_or_hydration_pending" || rootCause == "network_delay_during_load" {
		return map[string]any{"status": "network_or_hydration_pending", "message": "首页仍在等待网络资源或前端 hydration；此时登录按钮可能已显示，但点击事件还未完全挂载。", "next_steps": []string{"继续等待而不是反复点击", "观察 slow_resources 是否持续增长", "必要时更换代理节点"}}
	}
	return map[string]any{"status": "healthy", "message": "CDP 和页面探针正常；如果仍感到卡顿，多半来自外部页面资源、代理链路或本机 Chrome 负载。", "next_steps": []string{"对比慢资源列表", "观察任务管理器 CPU/内存", "必要时减少同时打开的 Chrome 页面"}}
}

func rememberManagedIncognitoOpen(openedURL string) {
	latestManagedIncognito.mu.Lock()
	latestManagedIncognito.openedURL = strings.TrimSpace(openedURL)
	latestManagedIncognito.openedAt = time.Now()
	latestManagedIncognito.browserContextID = ""
	latestManagedIncognito.targetIDs = map[string]time.Time{}
	latestManagedIncognito.mu.Unlock()

	go func(expectedURL string) {
		time.Sleep(1800 * time.Millisecond)
		targets, err := getCDPTargets(cdpDebuggingPort)
		if err != nil {
			return
		}
		latestManagedIncognito.mu.Lock()
		defer latestManagedIncognito.mu.Unlock()
		if latestManagedIncognito.openedURL != expectedURL {
			return
		}
		for _, target := range targets {
			if isManagedLoginCandidateURL(target.URL) && target.ID != "" {
				latestManagedIncognito.targetIDs[target.ID] = time.Now()
			}
		}
	}(strings.TrimSpace(openedURL))
}

func managedIncognitoTargetScore(target cdpTarget) int {
	latestManagedIncognito.mu.Lock()
	defer latestManagedIncognito.mu.Unlock()
	if latestManagedIncognito.openedURL == "" {
		return 0
	}
	if _, ok := latestManagedIncognito.targetIDs[target.ID]; ok && target.ID != "" {
		return 30
	}
	openedURL := strings.ToLower(latestManagedIncognito.openedURL)
	currentURL := strings.ToLower(strings.TrimSpace(target.URL))
	if openedURL != "" && currentURL != "" && currentURL == openedURL {
		return 20
	}
	openedHost := hostnameOnly(openedURL)
	currentHost := hostnameOnly(currentURL)
	if openedHost != "" && currentHost != "" && openedHost == currentHost {
		return 10
	}
	return 0
}

func hostnameOnly(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func isManagedLoginCandidateURL(rawURL string) bool {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	if u == "" || strings.HasPrefix(u, "devtools://") || strings.Contains(u, "localhost:18473") || strings.Contains(u, "127.0.0.1:18473") || u == "about:blank" {
		return false
	}
	return strings.Contains(u, "chatgpt.com") || strings.Contains(u, "auth.openai.com") || strings.Contains(u, "openai.com") || strings.Contains(u, "pay.openai.com")
}

func handleLoginClick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req loginClickRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	timeoutS := req.TimeoutS
	if timeoutS <= 0 {
		timeoutS = 35
	}
	if timeoutS > 120 {
		timeoutS = 120
	}
	result := automateLoginClick(r.Context(), time.Duration(timeoutS)*time.Second)
	writeJSON(w, http.StatusOK, result)
}

func automateLoginClick(ctx context.Context, timeout time.Duration) loginClickResult {
	startedAt := time.Now()
	result := loginClickResult{Stage: "login_click_started", Attempts: []map[string]any{}}
	sawInteractivePage := false
	sawPageNotInteractive := false
	directNavigationAttempted := false
	networkWaitExtended := false
	homepageRecoveryAttempts := 0
	freshContextRebuildAttempts := 0
	relaunched, err := ensureManagedCDPReady(ctx, "https://chatgpt.com/")
	if err != nil {
		result.Stage = "cdp_relaunch_failed"
		result.Error = "Chrome CDP 未就绪，自动重新拉起无痕窗口失败: " + err.Error()
		result.ElapsedMS = time.Since(startedAt).Milliseconds()
		return result
	}
	if relaunched {
		result.Attempts = appendLoginAttempt(result.Attempts, map[string]any{"stage": "cdp_relaunched", "url": "https://chatgpt.com/"})
	}
	softDeadline := startedAt.Add(timeout)
	hardTimeout := timeout
	if hardTimeout < 120*time.Second {
		hardTimeout = 120 * time.Second
	}
	hardDeadline := startedAt.Add(hardTimeout)
	for time.Now().Before(hardDeadline) {
		select {
		case <-ctx.Done():
			result.Stage = "login_click_cancelled"
			result.Error = ctx.Err().Error()
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		default:
		}

		target, err := waitForInteractiveLoginTarget(ctx, 2*time.Second)
		if err != nil {
			time.Sleep(700 * time.Millisecond)
			continue
		}
		result.TargetURL = target.URL
		result.TargetTitle = target.Title
		conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
		if err != nil {
			time.Sleep(700 * time.Millisecond)
			continue
		}
		_, _ = sendCDPCommand(conn, "Runtime.enable", nil)
		readiness, readinessErr := runLoginPageReadinessProbeStep(conn)
		if readinessErr != nil {
			_ = conn.Close()
			if isTransientCDPPageError(readinessErr) {
				time.Sleep(700 * time.Millisecond)
				continue
			}
			result.Stage = "login_page_probe_failed"
			result.Error = readinessErr.Error()
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		if readiness != nil {
			if currentURL := stringifyJSONValue(readiness["url"]); currentURL != "" {
				result.CurrentURL = currentURL
			}
			if readyState := stringifyJSONValue(readiness["ready_state"]); readyState != "" {
				result.PageReadyState = readyState
			}
			if reason := stringifyJSONValue(readiness["not_interactive_reason"]); reason != "" {
				result.NotInteractiveReason = reason
			}
			result.PageMonitor = readiness
			rootCause, strategy, waitable := classifyLoginHomepageState(readiness)
			result.RootCause = rootCause
			result.DecisionStrategy = strategy
			readiness["root_cause"] = rootCause
			readiness["decision_strategy"] = strategy
			if shouldRecoverHomepageFromLoginCause(rootCause) && homepageRecoveryAttempts < 2 {
				homepageRecoveryAttempts++
				if recovery, recoveryErr := runLoginHomepageRecoveryStep(conn); recoveryErr != nil {
					readiness["homepage_recovery_error"] = recoveryErr.Error()
				} else {
					readiness["homepage_recovery"] = recovery
					result.PageMonitor = recovery
				}
				readiness["stage"] = "homepage_recovery"
				result.Attempts = appendLoginAttempt(result.Attempts, readiness)
				_ = conn.Close()
				time.Sleep(900 * time.Millisecond)
				continue
			}
			if rootCause == "already_logged_in_session" || rootCause == "already_logged_in_chat_context" {
				readiness["stage"] = "already_logged_in"
				if freshContextRebuildAttempts < 2 {
					freshContextRebuildAttempts++
					rebuild, rebuildErr := rebuildManagedLoginBrowserContext(ctx, target.ID, "https://chatgpt.com/")
					if rebuildErr != nil {
						readiness["context_rebuild_error"] = rebuildErr.Error()
						result.Attempts = appendLoginAttempt(result.Attempts, readiness)
						result.Stage = "login_context_rebuild_failed"
						result.Error = "识别到当前上下文已经登录，但重建全新无登录态浏览器上下文失败: " + rebuildErr.Error()
						result.ElapsedMS = time.Since(startedAt).Milliseconds()
						_ = conn.Close()
						return result
					}
					readiness["fresh_context_rebuild"] = rebuild
					readiness["stage"] = "fresh_context_rebuild"
					result.ContextRebuilt = true
					result.RootCause = "fresh_browser_context_rebuilt"
					result.DecisionStrategy = "retry_login_click_on_fresh_context"
					result.PageMonitor = rebuild
					result.Attempts = appendLoginAttempt(result.Attempts, readiness)
					_ = conn.Close()
					time.Sleep(1200 * time.Millisecond)
					continue
				}
				result.Attempts = appendLoginAttempt(result.Attempts, readiness)
				result.OK = true
				result.Stage = "already_logged_in"
				result.Error = ""
				result.ElapsedMS = time.Since(startedAt).Milliseconds()
				_ = conn.Close()
				return result
			}
			if interactive, _ := readiness["interactive_ready"].(bool); !interactive {
				sawPageNotInteractive = true
				readiness["stage"] = "page_not_interactive"
				result.Attempts = appendLoginAttempt(result.Attempts, readiness)
				if time.Now().After(softDeadline) {
					if waitable {
						networkWaitExtended = true
						result.WaitExtendedMS = time.Since(softDeadline).Milliseconds()
						_ = conn.Close()
						time.Sleep(700 * time.Millisecond)
						continue
					}
					_ = conn.Close()
					break
				}
				_ = conn.Close()
				time.Sleep(700 * time.Millisecond)
				continue
			}
			sawInteractivePage = true
			if result.InteractiveWaitMS == 0 {
				result.InteractiveWaitMS = time.Since(startedAt).Milliseconds()
			}
		}
		action, err := runLoginClickStep(conn)
		if err == nil {
			if result.PageReadyState != "" {
				action["page_ready_state"] = result.PageReadyState
			}
			if result.NotInteractiveReason != "" {
				action["page_last_not_interactive_reason"] = result.NotInteractiveReason
			}
			if result.RootCause != "" {
				action["page_root_cause"] = result.RootCause
			}
			if result.DecisionStrategy != "" {
				action["page_decision_strategy"] = result.DecisionStrategy
			}
			if clicked, _ := action["clicked_login"].(bool); clicked {
				emailReady, _ := action["email_input_ready"].(bool)
				surfaceReady, _ := action["login_surface_ready"].(bool)
				if !emailReady && !surfaceReady {
					if cookieDismiss, dismissErr := runLoginCookieConsentDismissStep(conn); dismissErr == nil {
						action["cookie_consent_probe"] = cookieDismiss
						if dismissed, _ := cookieDismiss["cookie_consent_dismissed"].(bool); dismissed {
							action["cookie_consent_dismissed_before_trusted_click"] = true
							time.Sleep(600 * time.Millisecond)
						}
					} else {
						action["cookie_consent_probe_error"] = dismissErr.Error()
					}
					if clickErr := dispatchCDPMouseClickFromAction(conn, action); clickErr != nil {
						action["trusted_login_click_error"] = clickErr.Error()
					} else {
						action["trusted_login_click_dispatched"] = true
						time.Sleep(900 * time.Millisecond)
						if probe, probeErr := runLoginWindowProbeStep(conn); probeErr != nil {
							action["post_trusted_click_probe_error"] = probeErr.Error()
						} else {
							action["post_trusted_click_probe"] = probe
							if ready, _ := probe["email_input_ready"].(bool); ready {
								action["email_input_ready"] = true
							}
							if ready, _ := probe["login_surface_ready"].(bool); ready {
								action["login_surface_ready"] = true
							}
							if currentURL := stringifyJSONValue(probe["url"]); currentURL != "" {
								action["url"] = currentURL
							}
							if title := stringifyJSONValue(probe["title"]); title != "" {
								action["title"] = title
							}
						}
					}
					if shouldUseDirectLoginNavigation(action) {
						directNavigationAttempted = true
						result.RootCause = "homepage_click_handler_unresponsive"
						result.DecisionStrategy = "direct_login_navigation"
						if directNav, directErr := runLoginDirectNavigationStep(conn); directErr != nil {
							action["login_direct_navigation_error"] = directErr.Error()
						} else {
							action["login_direct_navigation"] = directNav
							result.PageMonitor = directNav
							if ready, _ := directNav["email_input_ready"].(bool); ready {
								action["email_input_ready"] = true
							}
							if ready, _ := directNav["login_surface_ready"].(bool); ready {
								action["login_surface_ready"] = true
							}
							if currentURL := stringifyJSONValue(directNav["url"]); currentURL != "" {
								action["url"] = currentURL
							}
							if title := stringifyJSONValue(directNav["title"]); title != "" {
								action["title"] = title
							}
						}
					}
				}
			}
		}
		_ = conn.Close()
		if err != nil {
			if isTransientCDPPageError(err) {
				result.Stage = "login_click_target_switching"
				result.Error = "页面正在跳转或目标页切换，已自动重试"
				result.ElapsedMS = time.Since(startedAt).Milliseconds()
				time.Sleep(900 * time.Millisecond)
				continue
			}
			result.Stage = "login_click_cdp_failed"
			result.Error = err.Error()
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		result.Attempts = appendLoginAttempt(result.Attempts, action)
		if clicked, _ := action["clicked_login"].(bool); clicked {
			result.ClickedLogin = true
		}
		if ready, _ := action["email_input_ready"].(bool); ready {
			result.EmailInputReady = true
		}
		if ready, _ := action["login_surface_ready"].(bool); ready {
			result.LoginSurfaceReady = true
		}
		if text := stringifyJSONValue(action["login_button_text"]); text != "" {
			result.LoginButtonText = text
		}
		if switched, _ := action["email_mode_switched"].(bool); switched {
			result.EmailModeSwitched = true
		}
		if text := stringifyJSONValue(action["email_mode_button_text"]); text != "" {
			result.EmailModeButtonText = text
		}
		if currentURL := stringifyJSONValue(action["url"]); currentURL != "" {
			result.CurrentURL = currentURL
		}
		if result.EmailInputReady || result.EmailModeSwitched || result.LoginSurfaceReady {
			if dismissed, _ := action["cookie_consent_dismissed_before_trusted_click"].(bool); dismissed {
				result.RootCause = "cookie_banner_interference"
				result.DecisionStrategy = "dismiss_cookie_then_click"
			}
			if result.RootCause == "" && result.LoginSurfaceReady && !result.ClickedLogin {
				result.RootCause = "already_on_login_surface"
				result.DecisionStrategy = "skip_homepage_click_and_continue_login"
			}
			result.OK = true
			result.Stage = "login_window_ready"
			result.Error = ""
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		if time.Now().After(softDeadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !sawInteractivePage && sawPageNotInteractive {
		result.Stage = "login_page_not_interactive"
		if networkWaitExtended {
			result.Error = "ChatGPT 首页持续处于网络加载或前端初始化阶段；系统已按网络慢策略延长等待，但仍未进入可点击状态: " + describeLoginNotInteractiveReason(result.NotInteractiveReason)
		} else {
			result.Error = "ChatGPT 页面长时间没有进入可点击状态；当前证据显示页面仍处于加载或前端初始化阶段: " + describeLoginNotInteractiveReason(result.NotInteractiveReason)
		}
	} else if result.RootCause == "checkout_page_active" || result.RootCause == "non_homepage_chatgpt_route" {
		result.Stage = "login_wrong_page_context"
		result.Error = "当前无痕页并不在可点登录的 ChatGPT 首页，而是在其他路由中；系统已尝试拉回首页。请查看 page_monitor.route_kind 和 homepage_recovery 诊断。"
	} else if result.ClickedLogin {
		result.Stage = "login_window_not_ready_after_click"
		if directNavigationAttempted {
			result.Error = "已识别并点击登录入口，但首页登录交互长时间无响应；系统已自动尝试直达登录页，仍未检测到登录或注册邮箱窗口。根因更接近页面登录脚本或网络初始化过慢，而不是按钮识别失败。"
		} else {
			result.Error = "已点击右上角登录入口，但在限定时间内未检测到登录或注册邮箱窗口；可能是页面跳转慢、弹窗被网络请求卡住，或页面停留在手机登录模式"
		}
	} else {
		result.Stage = "login_button_not_found"
		if sawPageNotInteractive {
			result.Error = "未识别到可操作的登录按钮；目标页在超时前始终没有进入稳定可点击状态: " + describeLoginNotInteractiveReason(result.NotInteractiveReason)
		} else {
			result.Error = "未识别到右上角登录按钮；请确认无痕窗口停留在未登录的 ChatGPT 首页，或查看输出里的 visible_buttons / scored_buttons 诊断"
		}
	}
	if networkWaitExtended && result.WaitExtendedMS == 0 {
		result.WaitExtendedMS = time.Since(softDeadline).Milliseconds()
	}
	result.ElapsedMS = time.Since(startedAt).Milliseconds()
	return result
}

func handleLoginEmailFill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req loginEmailFillRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "email is invalid")
		return
	}
	timeoutS := req.TimeoutS
	if timeoutS <= 0 {
		timeoutS = 90
	}
	if timeoutS > 300 {
		timeoutS = 300
	}
	result := automateLoginEmailFill(r.Context(), email, time.Duration(timeoutS)*time.Second)
	status := http.StatusOK
	if !result.OK {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func automateLoginEmailFill(ctx context.Context, email string, timeout time.Duration) loginEmailFillResult {
	startedAt := time.Now()
	result := loginEmailFillResult{Stage: "login_email_fill_started", Attempts: []map[string]any{}}
	sawLoginSurface := false
	extendedWaitActive := false
	relaunched, err := ensureManagedCDPReady(ctx, "https://chatgpt.com/")
	if err != nil {
		result.Stage = "cdp_relaunch_failed"
		result.Error = "Chrome CDP 未就绪，自动重新拉起无痕窗口失败: " + err.Error()
		result.ElapsedMS = time.Since(startedAt).Milliseconds()
		return result
	}
	if relaunched {
		result.Attempts = append(result.Attempts, map[string]any{"stage": "cdp_relaunched", "url": "https://chatgpt.com/"})
	}
	softDeadline := startedAt.Add(timeout)
	hardTimeout := timeout
	if hardTimeout < 180*time.Second {
		hardTimeout = 180 * time.Second
	}
	hardDeadline := startedAt.Add(hardTimeout)
	for time.Now().Before(hardDeadline) {
		select {
		case <-ctx.Done():
			result.Stage = "login_email_fill_cancelled"
			result.Error = ctx.Err().Error()
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		default:
		}

		target, err := waitForInteractiveLoginTarget(ctx, 2*time.Second)
		if err != nil {
			result.CurrentURL = stringifyJSONValue(result.CurrentURL)
			time.Sleep(700 * time.Millisecond)
			continue
		}
		result.TargetURL = target.URL
		result.TargetTitle = target.Title
		conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
		if err != nil {
			time.Sleep(700 * time.Millisecond)
			continue
		}
		_, _ = sendCDPCommand(conn, "Runtime.enable", nil)
		action, err := runLoginEmailFillStep(conn, email)
		_ = conn.Close()
		if err != nil {
			if isTransientCDPPageError(err) {
				result.Stage = "login_email_target_switching"
				result.Error = "页面正在跳转或目标页切换，已自动重试"
				result.ElapsedMS = time.Since(startedAt).Milliseconds()
				time.Sleep(900 * time.Millisecond)
				continue
			}
			result.Stage = "login_email_fill_cdp_failed"
			result.Error = err.Error()
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		result.Attempts = appendLoginAttempt(result.Attempts, action)
		if clicked, _ := action["clicked_login"].(bool); clicked {
			result.ClickedLogin = true
		}
		if text := stringifyJSONValue(action["login_button_text"]); text != "" {
			result.LoginButtonText = text
		}
		if switched, _ := action["email_mode_switched"].(bool); switched {
			result.EmailModeSwitched = true
		}
		if text := stringifyJSONValue(action["email_mode_button_text"]); text != "" {
			result.EmailModeButtonText = text
		}
		if ready, _ := action["login_surface_ready"].(bool); ready {
			result.LoginSurfaceReady = true
			sawLoginSurface = true
		}
		if clicked, _ := action["clicked_continue"].(bool); clicked {
			result.ClickedContinue = true
		}
		if text := stringifyJSONValue(action["continue_button_text"]); text != "" {
			result.ContinueButtonText = text
		}
		if ready, _ := action["verification_ready"].(bool); ready {
			result.OK = true
			result.Stage = "login_verification_ready"
			result.VerificationReady = true
			result.VerificationHint = stringifyJSONValue(action["verification_hint"])
			result.CurrentURL = stringifyJSONValue(action["url"])
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		if timedOut, _ := action["operation_timed_out"].(bool); timedOut {
			result.OperationTimedOut = true
			result.PageError = stringifyJSONValue(action["page_error"])
			result.CurrentURL = stringifyJSONValue(action["url"])
			if result.ClickedContinue && time.Now().Before(hardDeadline) {
				result.Stage = "login_email_transient_timeout_cleared"
				result.Error = ""
				time.Sleep(1200 * time.Millisecond)
				continue
			}
			result.OK = false
			result.Stage = "login_email_operation_timed_out"
			if result.PageError != "" {
				result.Error = result.PageError
			} else {
				result.Error = "页面提示 Operation timed out，邮箱提交请求超时"
			}
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		if filled, _ := action["email_filled"].(bool); filled {
			result.OK = true
			if result.VerificationReady {
				result.Stage = "login_verification_ready"
			} else if result.ClickedContinue {
				result.Stage = "login_email_submitted"
			} else {
				result.Stage = "login_email_filled"
			}
			result.EmailFilled = true
			result.EmailInputLabel = stringifyJSONValue(action["email_input_label"])
			result.CurrentURL = stringifyJSONValue(action["url"])
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		if currentURL := stringifyJSONValue(action["url"]); currentURL != "" {
			result.CurrentURL = currentURL
		}
		if time.Now().After(softDeadline) {
			if sawLoginSurface {
				extendedWaitActive = true
				result.WaitExtendedMS = time.Since(softDeadline).Milliseconds()
				time.Sleep(900 * time.Millisecond)
				continue
			}
			break
		}
		time.Sleep(900 * time.Millisecond)
	}
	if sawLoginSurface {
		result.Stage = "login_email_input_not_ready_after_surface"
		if extendedWaitActive && result.WaitExtendedMS == 0 {
			result.WaitExtendedMS = time.Since(softDeadline).Milliseconds()
		}
		result.Error = "已检测到登录或注册窗口，但邮箱输入框在延长等待后仍未出现；系统已经持续等待登录框加载完成"
	} else {
		result.Stage = "login_email_input_not_found"
		result.Error = "已点击登录入口，但在限定时间内未找到可填写的电子邮件地址输入框"
	}
	result.ElapsedMS = time.Since(startedAt).Milliseconds()
	return result
}

func handleLoginCodeFill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req loginCodeFillRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}
	if len(code) < 4 || len(code) > 12 {
		writeError(w, http.StatusBadRequest, "code length is invalid")
		return
	}
	timeoutS := req.TimeoutS
	if timeoutS <= 0 {
		timeoutS = 45
	}
	if timeoutS > 180 {
		timeoutS = 180
	}
	result := automateLoginCodeFill(r.Context(), code, time.Duration(timeoutS)*time.Second)
	writeJSON(w, http.StatusOK, result)
}

func automateLoginCodeFill(ctx context.Context, code string, timeout time.Duration) loginCodeFillResult {
	startedAt := time.Now()
	result := loginCodeFillResult{Stage: "login_code_fill_started", CodeLength: len(code), Attempts: []map[string]any{}}
	profile := nextRandomJapaneseProfile()
	relaunched, err := ensureManagedCDPReady(ctx, "https://chatgpt.com/")
	if err != nil {
		result.Stage = "cdp_relaunch_failed"
		result.Error = "Chrome CDP 未就绪，自动重新拉起无痕窗口失败: " + err.Error()
		result.ElapsedMS = time.Since(startedAt).Milliseconds()
		return result
	}
	if relaunched {
		result.Attempts = append(result.Attempts, map[string]any{"stage": "cdp_relaunched", "url": "https://chatgpt.com/"})
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			result.Stage = "login_code_fill_cancelled"
			result.Error = ctx.Err().Error()
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		default:
		}

		target, err := waitForInteractiveLoginTarget(ctx, 2*time.Second)
		if err != nil {
			time.Sleep(700 * time.Millisecond)
			continue
		}
		result.TargetURL = target.URL
		result.TargetTitle = target.Title
		conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
		if err != nil {
			time.Sleep(700 * time.Millisecond)
			continue
		}
		_, _ = sendCDPCommand(conn, "Runtime.enable", nil)
		action, err := runLoginCodeFillStep(conn, code, profile)
		_ = conn.Close()
		if err != nil {
			if isTransientCDPPageError(err) {
				result.Stage = "login_code_target_switching"
				result.Error = "页面正在跳转或目标页切换，已自动重试"
				result.ElapsedMS = time.Since(startedAt).Milliseconds()
				time.Sleep(900 * time.Millisecond)
				continue
			}
			result.Stage = "login_code_fill_cdp_failed"
			result.Error = err.Error()
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		result.Attempts = append(result.Attempts, action)
		if filled, _ := action["code_filled"].(bool); filled {
			result.CodeFilled = true
			result.OK = true
			result.Stage = "login_code_filled"
		}
		if submitted, _ := action["code_submitted"].(bool); submitted {
			result.CodeSubmitted = true
			result.Stage = "login_code_submitted"
		}
		if rejected, _ := action["code_rejected"].(bool); rejected {
			result.CodeRejected = true
			result.OK = false
			result.Stage = "login_code_rejected"
			if result.Error == "" {
				result.Error = "验证码提交后页面提示验证码错误或无效"
			}
		}
		if filled, _ := action["profile_filled"].(bool); filled {
			result.ProfileFilled = true
			result.OK = true
			result.ProfileName = stringifyJSONValue(action["profile_name"])
			if age, ok := numberFromAny(action["profile_age"]); ok {
				result.ProfileAge = int(age)
			}
			result.ProfileButton = stringifyJSONValue(action["profile_button_text"])
			if result.Stage == "login_code_submitted" || result.Stage == "login_code_filled" || result.Stage == "login_code_fill_started" {
				result.Stage = "login_profile_filled"
			}
		}
		if completed, _ := action["login_completed"].(bool); completed {
			result.LoginCompleted = true
			result.OK = true
			result.Stage = "login_completed"
		}
		result.CodeInputLabel = stringifyJSONValue(action["code_input_label"])
		result.SubmitButtonText = stringifyJSONValue(action["submit_button_text"])
		result.CompletionHint = stringifyJSONValue(action["completion_hint"])
		result.CurrentURL = stringifyJSONValue(action["url"])
		if result.CodeFilled || result.ProfileFilled || result.LoginCompleted || result.CodeRejected {
			result.ElapsedMS = time.Since(startedAt).Milliseconds()
			return result
		}
		time.Sleep(900 * time.Millisecond)
	}
	result.Stage = "login_code_input_not_found"
	result.Error = "已等待验证码页面，但在限定时间内未找到可填写的验证码输入框"
	result.ElapsedMS = time.Since(startedAt).Milliseconds()
	return result
}

func waitForInteractiveLoginTarget(ctx context.Context, timeout time.Duration) (*cdpTarget, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		targets, err := getCDPTargets(cdpDebuggingPort)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		candidates := make([]cdpTarget, 0, len(targets))
		for _, target := range targets {
			if target.Type != "page" || target.WebSocketDebuggerURL == "" || !isManagedLoginCandidateURL(target.URL) {
				continue
			}
			candidates = append(candidates, target)
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			left := managedIncognitoTargetScore(candidates[i])
			right := managedIncognitoTargetScore(candidates[j])
			if left != right {
				return left > right
			}
			return i > j
		})
		var fallback *cdpTarget
		var fallbackInteractive *cdpTarget
		for i := range candidates {
			target := &candidates[i]
			if fallback == nil {
				copyTarget := *target
				fallback = &copyTarget
			}
			probe, err := probeLoginTarget(ctx, target)
			if err != nil {
				lastErr = err
				continue
			}
			interactiveReady, _ := probe["interactive_ready"].(bool)
			if interactiveReady && fallbackInteractive == nil {
				copyTarget := *target
				fallbackInteractive = &copyTarget
			}
			if hasEmail, _ := probe["has_email_input"].(bool); hasEmail {
				return target, nil
			}
			if hasEmailMode, _ := probe["has_email_mode_button"].(bool); hasEmailMode {
				return target, nil
			}
			if isAuth, _ := probe["is_auth_page"].(bool); isAuth && interactiveReady {
				return target, nil
			}
			if isLoginSurface, _ := probe["is_login_surface"].(bool); isLoginSurface && interactiveReady {
				return target, nil
			}
			if hasLogin, _ := probe["has_login_button"].(bool); hasLogin && interactiveReady && managedIncognitoTargetScore(*target) > 0 {
				return target, nil
			}
		}
		if fallbackInteractive != nil {
			return fallbackInteractive, nil
		}
		if fallback != nil {
			return fallback, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("未找到可监控的 OpenAI/ChatGPT 登录页面")
}

func loginTargetProbeScript() string {
	return `(async () => {
		const visible = (el) => {
			if (!el) return false;
			const rect = el.getBoundingClientRect();
			const style = window.getComputedStyle(el);
			return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
		};
		const textOf = (el) => ((el && (el.innerText || el.textContent || el.getAttribute('aria-label') || el.value || '')) || '').replace(/\s+/g, ' ').trim();
		const authSurfaceTextReady = (text) => {
			const hay = (text || '').toLowerCase();
			const hasEntryTitle = hay.includes('log in or sign up') || hay.includes('login or sign up') || hay.includes('登录或注册');
			const hasEmailCue = hay.includes('email address') || hay.includes('电子邮件地址') || hay.includes('邮箱地址');
			const hasProviderCue = hay.includes('continue with google') || hay.includes('continue with apple') || hay.includes('continue with phone') || hay.includes('phone number') || hay.includes('使用 google') || hay.includes('使用 apple') || hay.includes('使用电话号码') || hay.includes('电话号码继续');
			const hasContinueCue = hay.includes('continue') || hay.includes('继续');
			return (hasEntryTitle && (hasEmailCue || hasProviderCue || hasContinueCue)) || (hasEmailCue && hasContinueCue);
		};
		const hasLoginShell = () => {
			const frames = Array.from(document.querySelectorAll('iframe')).filter(visible).some((el) => {
				const hay = [el.src, el.title, el.name, el.id, el.className, el.getAttribute('aria-label')].filter(Boolean).join(' ').toLowerCase();
				return hay.includes('auth.openai.com') || hay.includes('/auth/') || hay.includes('login') || hay.includes('signin') || hay.includes('sign-in') || hay.includes('登录') || hay.includes('邮箱');
			});
			const shells = Array.from(document.querySelectorAll('dialog, [role="dialog"], [aria-modal="true"], [data-testid*="modal"], [data-testid*="auth"], [data-testid*="login"], [data-radix-portal], [popover]')).filter(visible).some((el) => {
				return authSurfaceTextReady(textOf(el));
			});
			return frames || shells;
		};
		const bodyText = document.body ? textOf(document.body) : '';
		const lowerBody = bodyText.toLowerCase();
		const pathName = location.pathname || '/';
		let routeKind = 'unknown';
		if (location.href === 'about:blank') routeKind = 'blank';
		else if (location.hostname.includes('pay.openai.com') || pathName.includes('/checkout/') || /付款方式|支付|订阅|checkout|subscribe/i.test(bodyText)) routeKind = 'checkout';
		else if (location.hostname.includes('auth.openai.com') || location.href.toLowerCase().includes('/auth/login')) routeKind = 'auth';
		else if (location.hostname.includes('chatgpt.com') && (pathName === '/' || pathName === '')) routeKind = 'homepage';
		else if (location.hostname.includes('chatgpt.com') && (pathName.startsWith('/c/') || pathName.startsWith('/g/'))) routeKind = 'conversation';
		else if (location.hostname.includes('chatgpt.com')) routeKind = 'chatgpt_other';
		const isLoginSurface = () => {
			return hasLoginShell() || location.hostname.includes('auth.openai.com') || location.href.toLowerCase().includes('/auth/login') || authSurfaceTextReady(lowerBody);
		};
		const inputs = Array.from(document.querySelectorAll('input, textarea')).filter(visible);
		const hasEmailInput = inputs.some((el) => {
			const label = [el.type, el.name, el.autocomplete, el.placeholder, el.getAttribute('aria-label')].filter(Boolean).join(' ').toLowerCase();
			return el.type === 'email' || label.includes('email') || label.includes('e-mail') || label.includes('电子邮件') || label.includes('邮箱');
		});
		const buttons = Array.from(document.querySelectorAll('button, a, [role="button"], input[type="button"], input[type="submit"]')).filter(visible);
		const loggedInShell = !!document.querySelector('[data-testid="accounts-profile-button"], [data-testid="create-new-chat-button"]') || lowerBody.includes('历史聊天记录') || lowerBody.includes('新聊天') || lowerBody.includes('搜索聊天') || lowerBody.includes('projects') || lowerBody.includes('codex');
		const visibleButtons = buttons.slice(0, 12).map((el) => {
			const rect = el.getBoundingClientRect();
			return {
				text: textOf(el).slice(0, 80),
				tag: el.tagName,
				href: el.getAttribute('href') || '',
				aria: el.getAttribute('aria-label') || '',
				testid: el.getAttribute('data-testid') || '',
				rect: { top: Math.round(rect.top), left: Math.round(rect.left), width: Math.round(rect.width), height: Math.round(rect.height) }
			};
		});
		const loginButtonCandidates = buttons.map((el) => {
			const rect = el.getBoundingClientRect();
			const text = textOf(el).toLowerCase();
			const href = (el.getAttribute('href') || '').toLowerCase();
			const id = (el.id || '').toLowerCase();
			const cls = (el.className || '').toString().toLowerCase();
			const aria = (el.getAttribute('aria-label') || '').toLowerCase();
			const testid = (el.getAttribute('data-testid') || '').toLowerCase();
			const title = (el.getAttribute('title') || '').toLowerCase();
			const hay = [text, href, id, cls, aria, testid, title].join(' ');
			let score = 0;
			if (text === 'log in' || text === 'login' || text === 'sign in' || text === '登录' || text === '登入') score += 100;
			if (hay.includes('log in') || hay.includes('login') || hay.includes('sign in') || hay.includes('signin') || hay.includes('auth/login') || hay.includes('登录') || hay.includes('登入')) score += 70;
			if (href.includes('/auth/login') || href.includes('login')) score += 60;
			if (id.includes('login') || cls.includes('login') || aria.includes('login') || testid.includes('login')) score += 40;
			if (rect.top >= 0 && rect.top <= Math.max(180, window.innerHeight * 0.3)) score += 20;
			if (rect.left >= window.innerWidth * 0.5) score += 20;
			if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 120;
			return { score, text: textOf(el).slice(0, 80), href: el.getAttribute('href') || '', rect: { top: Math.round(rect.top), left: Math.round(rect.left), width: Math.round(rect.width), height: Math.round(rect.height) } };
		}).filter((item) => item.score >= 60).sort((a, b) => b.score - a.score).slice(0, 6);
		const hasEmailModeButton = buttons.some((el) => {
			const text = textOf(el).toLowerCase();
			const hay = [text, el.getAttribute('aria-label'), el.id, el.getAttribute('name'), el.className, el.value, el.getAttribute('href')].filter(Boolean).join(' ').toLowerCase();
			return hay.includes('continue with email') || hay.includes('use email') || hay.includes('email instead') || hay.includes('sign in with email') || hay.includes('login with email') || hay.includes('log in with email') || hay.includes('电子邮箱') || hay.includes('邮箱继续') || hay.includes('用邮箱') || hay.includes('通过邮箱');
		});
		const hasLoginButton = buttons.some((el) => {
			const text = textOf(el).toLowerCase();
			const href = (el.getAttribute('href') || '').toLowerCase();
			return text.includes('login') || text.includes('log in') || text.includes('sign in') || text.includes('登录') || text.includes('登入') || href.includes('/auth/login');
		});
		const readyState = (document.readyState || '').toLowerCase();
		const looksBootstrap = bodyText.length > 0 && bodyText.length < 320 && (/document\.documentelement|localstorage|matchmedia|theme/i.test(bodyText) || lowerBody.startsWith('!function(){try{var d=document.documentelement'));
		let interactiveReady = false;
		let notInteractiveReason = '';
		if (location.href === 'about:blank') {
			notInteractiveReason = 'about_blank';
		} else if (!document.body) {
			notInteractiveReason = 'body_missing';
		} else if (readyState !== 'interactive' && readyState !== 'complete') {
			notInteractiveReason = 'document_loading';
		} else if (looksBootstrap) {
			notInteractiveReason = 'bootstrap_placeholder';
		} else if (!isLoginSurface() && !hasLoginButton && !hasEmailModeButton && !hasEmailInput && buttons.length === 0 && inputs.length === 0) {
			notInteractiveReason = 'no_interactive_controls';
		} else if (!isLoginSurface() && !hasLoginButton && !hasEmailModeButton && !hasEmailInput && buttons.length === 0) {
			notInteractiveReason = 'buttons_not_ready';
		} else {
			interactiveReady = true;
		}
		const nav = performance.getEntriesByType('navigation')[0];
		const resources = performance.getEntriesByType('resource');
		const slowResources = resources.filter((r) => r.duration > 1000).sort((a, b) => b.duration - a.duration).slice(0, 8).map((r) => {
			try {
				const u = new URL(r.name);
				return { host: u.host, path: u.pathname.slice(0, 140), type: r.initiatorType, duration_ms: Math.round(r.duration), transfer_size: r.transferSize || 0 };
			} catch (_) {
				return { host: '', path: String(r.name).slice(0, 140), type: r.initiatorType, duration_ms: Math.round(r.duration), transfer_size: r.transferSize || 0 };
			}
		});
		const resourceTotals = resources.reduce((acc, r) => {
			acc.count += 1;
			acc.duration_ms += Math.round(r.duration || 0);
			acc.transfer_size += r.transferSize || 0;
			if ((r.transferSize || 0) === 0) acc.zero_transfer += 1;
			return acc;
		}, { count: 0, duration_ms: 0, transfer_size: 0, zero_transfer: 0 });
		const cookieBannerPresent = /cookie|cookies|cookie 政策|我们使用 cookie|隐私|privacy/i.test(bodyText);
		return JSON.stringify({
			url: location.href,
			title: document.title,
			pathname: pathName,
			route_kind: routeKind,
			ready_state: document.readyState,
			visibility_state: document.visibilityState,
			body_text_length: bodyText.length,
			body_text_sample: bodyText.slice(0, 500),
			is_auth_page: location.hostname.includes('auth.openai.com'),
			is_login_surface: isLoginSurface(),
			has_email_input: hasEmailInput,
			has_email_mode_button: hasEmailModeButton,
			has_login_button: hasLoginButton,
			logged_in_shell: loggedInShell,
			input_count: inputs.length,
			button_count: buttons.length,
			visible_input_count: inputs.length,
			visible_button_count: buttons.length,
			visible_buttons: visibleButtons,
			login_button_candidates: loginButtonCandidates,
			interactive_ready: interactiveReady,
			not_interactive_reason: notInteractiveReason,
			looks_like_bootstrap: looksBootstrap,
			cookie_banner_present: cookieBannerPresent,
			nav: nav ? {
				duration_ms: Math.round(nav.duration || 0),
				dom_content_loaded_ms: Math.round(nav.domContentLoadedEventEnd || 0),
				load_event_ms: Math.round(nav.loadEventEnd || 0),
				response_start_ms: Math.round(nav.responseStart || 0),
				response_end_ms: Math.round(nav.responseEnd || 0),
				transfer_size: nav.transferSize || 0
			} : null,
			resource_totals: resourceTotals,
			slow_resources: slowResources,
			viewport: { width: window.innerWidth, height: window.innerHeight }
		});
	})()`
}

func probeLoginTarget(ctx context.Context, target *cdpTarget) (map[string]any, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_, _ = sendCDPCommand(conn, "Runtime.enable", nil)
	raw, err := executeCDPScript(conn, loginTargetProbeScript())
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return result, nil
}

func runLoginPageReadinessProbeStep(conn *websocket.Conn) (map[string]any, error) {
	raw, err := executeCDPScript(conn, loginTargetProbeScript())
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return result, nil
}

func compactLoginAttemptData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	compact := map[string]any{}
	copyKeys := []string{
		"stage",
		"url",
		"title",
		"pathname",
		"route_kind",
		"ready_state",
		"interactive_ready",
		"not_interactive_reason",
		"root_cause",
		"decision_strategy",
		"clicked_login",
		"email_input_ready",
		"login_surface_ready",
		"email_mode_switched",
		"login_button_text",
		"email_mode_button_text",
		"cookie_banner_present",
		"body_text_length",
		"visible_button_count",
		"visible_input_count",
		"all_clickable_count",
	}
	for _, key := range copyKeys {
		if value, ok := data[key]; ok {
			compact[key] = value
		}
	}
	if candidates, ok := data["login_button_candidates"].([]any); ok && len(candidates) > 0 {
		if len(candidates) > 3 {
			compact["login_button_candidates"] = candidates[:3]
		} else {
			compact["login_button_candidates"] = candidates
		}
	}
	if scored, ok := data["scored_buttons"].([]any); ok && len(scored) > 0 {
		if len(scored) > 3 {
			compact["scored_buttons"] = scored[:3]
		} else {
			compact["scored_buttons"] = scored
		}
	}
	if buttons, ok := data["visible_buttons"].([]any); ok && len(buttons) > 0 {
		if len(buttons) > 4 {
			compact["visible_buttons"] = buttons[:4]
		} else {
			compact["visible_buttons"] = buttons
		}
	}
	if slowResources, ok := data["slow_resources"].([]any); ok && len(slowResources) > 0 {
		if len(slowResources) > 4 {
			compact["slow_resources"] = slowResources[:4]
		} else {
			compact["slow_resources"] = slowResources
		}
	}
	if bodySample := stringifyJSONValue(data["body_text_sample"]); bodySample != "" {
		if len(bodySample) > 180 {
			bodySample = bodySample[:180]
		}
		compact["body_text_sample"] = bodySample
	}
	if recovery, ok := data["homepage_recovery"].(map[string]any); ok {
		compact["homepage_recovery"] = recovery
	}
	if rebuild, ok := data["fresh_context_rebuild"].(map[string]any); ok {
		compact["fresh_context_rebuild"] = rebuild
	}
	if directNav, ok := data["login_direct_navigation"].(map[string]any); ok {
		compact["login_direct_navigation"] = directNav
	}
	return compact
}

func sameCompactLoginAttempt(left, right map[string]any) bool {
	if left == nil || right == nil {
		return false
	}
	keys := []string{"stage", "url", "route_kind", "root_cause", "decision_strategy", "not_interactive_reason", "ready_state", "clicked_login", "email_input_ready", "login_surface_ready"}
	for _, key := range keys {
		if stringifyJSONValue(left[key]) != stringifyJSONValue(right[key]) {
			return false
		}
	}
	return true
}

func appendLoginAttempt(attempts []map[string]any, data map[string]any) []map[string]any {
	compact := compactLoginAttemptData(data)
	if compact == nil {
		return attempts
	}
	if len(attempts) > 0 && sameCompactLoginAttempt(attempts[len(attempts)-1], compact) {
		last := attempts[len(attempts)-1]
		repeatCount := 1
		if raw, ok := last["repeat_count"]; ok {
			if count, okCount := numberFromAny(raw); okCount {
				repeatCount = int(count)
			}
		}
		last["repeat_count"] = repeatCount + 1
		return attempts
	}
	compact["repeat_count"] = 1
	return append(attempts, compact)
}

func shouldRecoverHomepageFromLoginCause(rootCause string) bool {
	switch strings.TrimSpace(rootCause) {
	case "checkout_page_active", "non_homepage_chatgpt_route":
		return true
	default:
		return false
	}
}

func runLoginHomepageRecoveryStep(conn *websocket.Conn) (map[string]any, error) {
	result := map[string]any{
		"attempted":  true,
		"target_url": "https://chatgpt.com/",
	}
	_, _ = sendCDPCommand(conn, "Page.enable", nil)
	if _, err := sendCDPCommand(conn, "Page.navigate", map[string]any{"url": "https://chatgpt.com/"}); err != nil {
		return nil, err
	}
	time.Sleep(1500 * time.Millisecond)
	probe, err := runLoginPageReadinessProbeStep(conn)
	if err != nil {
		result["probe_error"] = err.Error()
		return result, nil
	}
	rootCause, strategy, _ := classifyLoginHomepageState(probe)
	result["probe"] = probe
	result["root_cause"] = rootCause
	result["decision_strategy"] = strategy
	result["url"] = stringifyJSONValue(probe["url"])
	result["title"] = stringifyJSONValue(probe["title"])
	result["succeeded"] = stringifyJSONValue(probe["route_kind"]) == "homepage" || stringifyJSONValue(probe["route_kind"]) == "auth"
	return result, nil
}

func classifyLoginHomepageState(probe map[string]any) (string, string, bool) {
	if probe == nil {
		return "probe_missing", "retry_page_monitor", true
	}
	hasEmailInput, _ := probe["has_email_input"].(bool)
	hasEmailModeButton, _ := probe["has_email_mode_button"].(bool)
	hasLoginButton, _ := probe["has_login_button"].(bool)
	routeKind := stringifyJSONValue(probe["route_kind"])
	switch routeKind {
	case "checkout":
		return "checkout_page_active", "return_homepage_before_login_click", false
	case "conversation":
		return "already_logged_in_chat_context", "skip_login_and_continue_logged_in_flow", false
	case "chatgpt_other":
		return "non_homepage_chatgpt_route", "return_homepage_before_login_click", false
	}
	if hasEmailInput {
		return "already_on_login_surface", "skip_homepage_click_and_continue_login", false
	}
	if ready, _ := probe["is_login_surface"].(bool); ready {
		return "already_on_login_surface", "continue_login_surface_detection", false
	}
	if hasLoginButton {
		return "homepage_login_button_ready", "click_login_button", false
	}
	if hasEmailModeButton {
		return "email_mode_button_ready", "switch_to_email_mode", false
	}
	if loggedIn, _ := probe["logged_in_shell"].(bool); loggedIn && !hasLoginButton && !hasEmailModeButton && !hasEmailInput {
		return "already_logged_in_session", "skip_login_and_continue_session_fetch", false
	}
	if banner, _ := probe["cookie_banner_present"].(bool); banner {
		if hasLoginButton {
			return "cookie_banner_interference", "dismiss_cookie_then_click", false
		}
	}
	notInteractiveReason := stringifyJSONValue(probe["not_interactive_reason"])
	navSlow := false
	if nav, ok := probe["nav"].(map[string]any); ok {
		duration, _ := numberFromAny(nav["duration_ms"])
		responseStart, _ := numberFromAny(nav["response_start_ms"])
		if duration >= 12000 || responseStart >= 4000 {
			navSlow = true
		}
	}
	slowCount := 0
	if slowResources, ok := probe["slow_resources"].([]any); ok {
		slowCount = len(slowResources)
	}
	if notInteractiveReason != "" {
		switch notInteractiveReason {
		case "about_blank":
			return "page_navigation_pending", "wait_for_target_navigation", true
		case "body_missing", "document_loading":
			if navSlow || slowCount > 0 {
				return "network_delay_during_load", "continue_wait_for_network", true
			}
			return "document_still_loading", "continue_wait_for_dom", true
		case "bootstrap_placeholder", "buttons_not_ready", "no_interactive_controls":
			if navSlow || slowCount > 0 {
				return "network_delay_or_hydration_pending", "continue_wait_for_network_and_hydration", true
			}
			return "frontend_hydration_pending", "continue_wait_for_hydration", true
		default:
			return "page_not_interactive", "continue_wait_for_interactive_controls", true
		}
	}
	if navSlow || slowCount > 0 {
		return "network_delay_or_hydration_pending", "continue_wait_for_network_and_hydration", true
	}
	return "homepage_state_unknown", "collect_monitor_and_retry", false
}

func runLoginClickStep(conn *websocket.Conn) (map[string]any, error) {
	script := `(async () => {
		const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
		const visible = (el) => {
			if (!el) return false;
			const rect = el.getBoundingClientRect();
			const style = window.getComputedStyle(el);
			return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
		};
		const textOf = (el) => ((el && (el.innerText || el.textContent || el.getAttribute('aria-label') || el.value || '')) || '').replace(/\s+/g, ' ').trim();
		const authSurfaceTextReady = (text) => {
			const hay = (text || '').toLowerCase();
			const hasEntryTitle = hay.includes('log in or sign up') || hay.includes('login or sign up') || hay.includes('登录或注册');
			const hasEmailCue = hay.includes('email address') || hay.includes('电子邮件地址') || hay.includes('邮箱地址');
			const hasProviderCue = hay.includes('continue with google') || hay.includes('continue with apple') || hay.includes('continue with phone') || hay.includes('phone number') || hay.includes('使用 google') || hay.includes('使用 apple') || hay.includes('使用电话号码') || hay.includes('电话号码继续');
			const hasContinueCue = hay.includes('continue') || hay.includes('继续');
			return (hasEntryTitle && (hasEmailCue || hasProviderCue || hasContinueCue)) || (hasEmailCue && hasContinueCue);
		};
		const hasLoginShell = () => {
			const frames = Array.from(document.querySelectorAll('iframe')).filter(visible).some((el) => {
				const hay = [el.src, el.title, el.name, el.id, el.className, el.getAttribute('aria-label')].filter(Boolean).join(' ').toLowerCase();
				return hay.includes('auth.openai.com') || hay.includes('/auth/') || hay.includes('login') || hay.includes('signin') || hay.includes('sign-in') || hay.includes('登录') || hay.includes('邮箱');
			});
			const shells = Array.from(document.querySelectorAll('dialog, [role="dialog"], [aria-modal="true"], [data-testid*="modal"], [data-testid*="auth"], [data-testid*="login"], [data-radix-portal], [popover]')).filter(visible).some((el) => {
				return authSurfaceTextReady(textOf(el));
			});
			return frames || shells;
		};
		const isLoginSurface = () => {
			const body = document.body ? textOf(document.body).toLowerCase() : '';
			return hasLoginShell() || location.hostname.includes('auth.openai.com') || location.href.toLowerCase().includes('/auth/login') || authSurfaceTextReady(body);
		};
		const waitForEmailInput = async (tries, delay) => {
			for (let i = 0; i < tries; i += 1) {
				const emailInput = findEmailInput();
				if (emailInput && isLoginSurface()) return emailInput;
				await sleep(delay);
			}
			return null;
		};
		const clickElement = (el) => {
			try { el.scrollIntoView({ block: 'center', inline: 'center' }); } catch (_) {}
			const rect = el.getBoundingClientRect();
			const x = rect.left + rect.width / 2;
			const y = rect.top + rect.height / 2;
			for (const type of ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click']) {
				el.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, view: window, clientX: x, clientY: y }));
			}
		};
		const clickables = () => Array.from(document.querySelectorAll('button, a, [role="button"], input[type="button"], input[type="submit"]')).filter(visible);
		const findEmailModeButton = () => {
			const items = clickables().map((el) => {
				const text = textOf(el).toLowerCase();
				const hay = [text, el.getAttribute('aria-label'), el.id, el.getAttribute('name'), el.className, el.value, el.getAttribute('href')].filter(Boolean).join(' ').toLowerCase();
				let score = 0;
				if (text === 'continue with email' || text === 'use email' || text === 'use email instead' || text === 'sign in with email' || text === '使用电子邮箱继续' || text === '使用邮箱继续' || text === '用邮箱继续' || text === '通过邮箱继续') score += 120;
				if (hay.includes('continue with email') || hay.includes('use email') || hay.includes('email instead') || hay.includes('sign in with email') || hay.includes('login with email') || hay.includes('log in with email') || hay.includes('电子邮箱') || hay.includes('邮箱继续') || hay.includes('用邮箱') || hay.includes('通过邮箱')) score += 90;
				if (hay.includes('phone') || hay.includes('sms') || hay.includes('mobile') || hay.includes('手机号') || hay.includes('手机号码')) score -= 80;
				if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 100;
				return { el, score, text: textOf(el).slice(0, 80) || el.value || el.getAttribute('aria-label') || 'use email' };
			}).filter((item) => item.score > 0).sort((a, b) => b.score - a.score);
			return items[0] || null;
		};
		const findEmailInput = () => Array.from(document.querySelectorAll('input, textarea')).filter(visible).find((el) => {
			const id = el.id || '';
			const label = id ? (document.querySelector('label[for="' + CSS.escape(id) + '"]')?.innerText || '') : '';
			const hay = [el.type, el.name, el.autocomplete, el.placeholder, el.getAttribute('aria-label'), label].filter(Boolean).join(' ').toLowerCase();
			return el.type === 'email' || hay.includes('email') || hay.includes('e-mail') || hay.includes('电子邮件') || hay.includes('邮箱') || hay.includes('mail');
		});
		const result = { url: location.href, title: document.title, clicked_login: false, email_mode_switched: false, email_input_ready: false };
		if (findEmailInput() && isLoginSurface()) {
			result.email_input_ready = true;
			return JSON.stringify(result);
		}
		const emailModeButton = findEmailModeButton();
		if (emailModeButton) {
			result.email_mode_button_text = emailModeButton.text;
			clickElement(emailModeButton.el);
			result.email_mode_switched = true;
			for (let i = 0; i < 28; i += 1) {
				await sleep(250);
				const emailInput = findEmailInput();
				if (emailInput && isLoginSurface()) {
					result.email_input_ready = true;
					break;
				}
			}
			result.url = location.href;
			result.title = document.title;
			return JSON.stringify(result);
		}
		const clickablesList = clickables();
		const scored = clickablesList.map((el) => {
			const rect = el.getBoundingClientRect();
			const text = textOf(el).toLowerCase();
			const href = (el.getAttribute('href') || '').toLowerCase();
			const id = (el.id || '').toLowerCase();
			const cls = (el.className || '').toString().toLowerCase();
			const aria = (el.getAttribute('aria-label') || '').toLowerCase();
			const testid = (el.getAttribute('data-testid') || '').toLowerCase();
			const title = (el.getAttribute('title') || '').toLowerCase();
			const hay = [text, href, id, cls, aria, testid, title].join(' ');
			let score = 0;
			if (text === 'log in' || text === 'login' || text === 'sign in' || text === '登录' || text === '登入') score += 100;
			if (hay.includes('log in') || hay.includes('login') || hay.includes('sign in') || hay.includes('signin') || hay.includes('auth/login') || hay.includes('登录') || hay.includes('登入')) score += 70;
			if (href.includes('/auth/login') || href.includes('login')) score += 60;
			if (id.includes('login') || cls.includes('login') || aria.includes('login') || testid.includes('login')) score += 40;
			if (rect.top >= 0 && rect.top <= Math.max(180, window.innerHeight * 0.3)) score += 20;
			if (rect.left >= window.innerWidth * 0.5) score += 20;
			if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 120;
			return { el, score, text: textOf(el).slice(0, 80) || el.getAttribute('aria-label') || el.getAttribute('data-testid') || el.getAttribute('href') || 'login', rect: { top: rect.top, left: rect.left, width: rect.width, height: rect.height } };
		}).filter((item) => item.score >= 80).sort((a, b) => b.score - a.score);
		const loginButton = scored[0]?.el || null;
		result.scored_buttons = scored.slice(0, 8).map((item) => ({ score: item.score, text: item.text, rect: item.rect }));
		if (loginButton) {
			const startURL = location.href;
			const loginHref = loginButton.getAttribute('href') || '';
			result.login_button_text = scored[0].text;
			result.login_button_href = loginHref;
			result.login_button_rect = scored[0].rect;
			clickElement(loginButton);
			try { loginButton.click(); } catch (_) {}
			result.clicked_login = true;
			let popupReadyAfterClick = false;
			for (let i = 0; i < 12; i += 1) {
				await sleep(250);
				const emailInput = findEmailInput();
				if (emailInput && isLoginSurface()) {
					result.email_input_ready = true;
					popupReadyAfterClick = true;
					break;
				}
				const nextEmailModeButton = findEmailModeButton();
				if (nextEmailModeButton) {
					result.email_mode_button_text = nextEmailModeButton.text;
					clickElement(nextEmailModeButton.el);
					result.email_mode_switched = true;
					await sleep(500);
				}
				if (isLoginSurface()) {
					result.login_surface_ready = true;
					popupReadyAfterClick = true;
					break;
				}
			}
			if (!popupReadyAfterClick) {
				result.login_popup_not_ready_after_3s = true;
				const shellAfter3s = hasLoginShell();
				const urlChangedAfterClick = location.href !== startURL;
				if (shellAfter3s || urlChangedAfterClick || location.hostname.includes('auth.openai.com') || location.href.toLowerCase().includes('/auth/')) {
					result.login_surface_ready = true;
					result.login_reload_skipped_popup_detected = true;
					result.url = location.href;
					result.title = document.title;
					return JSON.stringify(result);
				}
				result.login_trusted_click_required = true;
				result.login_direct_navigation_suggested = true;
				result.url = location.href;
				result.title = document.title;
				return JSON.stringify(result);
			}
			for (let i = 0; i < 28; i += 1) {
				await sleep(250);
				if (i === 4 && loginHref && !isLoginSurface()) {
					try {
						const nextURL = new URL(loginHref, location.href).href;
						if (nextURL.toLowerCase().includes('login') || nextURL.toLowerCase().includes('/auth/')) {
							location.href = nextURL;
							result.login_href_fallback = true;
						}
					} catch (_) {}
				}
				const emailInput = findEmailInput();
				if (emailInput && isLoginSurface()) {
					result.email_input_ready = true;
					break;
				}
				const nextEmailModeButton = findEmailModeButton();
				if (nextEmailModeButton) {
					result.email_mode_button_text = nextEmailModeButton.text;
					clickElement(nextEmailModeButton.el);
					result.email_mode_switched = true;
					await sleep(500);
				}
				if (isLoginSurface()) {
					result.login_surface_ready = true;
				}
			}
			result.url = location.href;
			result.title = document.title;
			return JSON.stringify(result);
		}
		if (isLoginSurface()) {
			const delayedEmailInput = await waitForEmailInput(24, 250);
			result.login_surface_ready = true;
			result.email_input_ready = !!delayedEmailInput;
			result.url = location.href;
			result.title = document.title;
			return JSON.stringify(result);
		}
		result.visible_buttons = clickablesList.slice(0, 20).map((el) => {
			const rect = el.getBoundingClientRect();
			return {
				text: textOf(el).slice(0, 80),
				tag: el.tagName,
				href: el.getAttribute('href') || '',
				aria: el.getAttribute('aria-label') || '',
				testid: el.getAttribute('data-testid') || '',
				rect: { top: rect.top, left: rect.left, width: rect.width, height: rect.height }
			};
		});
		result.visible_inputs = Array.from(document.querySelectorAll('input, textarea')).filter(visible).length;
		result.all_clickable_count = clickablesList.length;
		result.body_text_sample = (document.body ? textOf(document.body).slice(0, 500) : '');
		result.url = location.href;
		result.title = document.title;
		return JSON.stringify(result);
	})()`
	raw, err := executeCDPScript(conn, script)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return result, nil
}

func runLoginWindowProbeStep(conn *websocket.Conn) (map[string]any, error) {
	script := `(async () => {
		const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
		const visible = (el) => {
			if (!el) return false;
			const rect = el.getBoundingClientRect();
			const style = window.getComputedStyle(el);
			return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
		};
		const textOf = (el) => ((el && (el.innerText || el.textContent || el.getAttribute('aria-label') || el.value || '')) || '').replace(/\s+/g, ' ').trim();
		const authSurfaceTextReady = (text) => {
			const hay = (text || '').toLowerCase();
			const hasEntryTitle = hay.includes('log in or sign up') || hay.includes('login or sign up') || hay.includes('登录或注册');
			const hasEmailCue = hay.includes('email address') || hay.includes('电子邮件地址') || hay.includes('邮箱地址');
			const hasProviderCue = hay.includes('continue with google') || hay.includes('continue with apple') || hay.includes('continue with phone') || hay.includes('phone number') || hay.includes('使用 google') || hay.includes('使用 apple') || hay.includes('使用电话号码') || hay.includes('电话号码继续');
			const hasContinueCue = hay.includes('continue') || hay.includes('继续');
			return (hasEntryTitle && (hasEmailCue || hasProviderCue || hasContinueCue)) || (hasEmailCue && hasContinueCue);
		};
		const hasLoginShell = () => {
			const frames = Array.from(document.querySelectorAll('iframe')).filter(visible).some((el) => {
				const hay = [el.src, el.title, el.name, el.id, el.className, el.getAttribute('aria-label')].filter(Boolean).join(' ').toLowerCase();
				return hay.includes('auth.openai.com') || hay.includes('/auth/') || hay.includes('login') || hay.includes('signin') || hay.includes('sign-in') || hay.includes('登录') || hay.includes('邮箱');
			});
			const shells = Array.from(document.querySelectorAll('dialog, [role="dialog"], [aria-modal="true"], [data-testid*="modal"], [data-testid*="auth"], [data-testid*="login"], [data-radix-portal], [popover]')).filter(visible).some((el) => {
				return authSurfaceTextReady(textOf(el));
			});
			return frames || shells;
		};
		const isLoginSurface = () => {
			const body = document.body ? textOf(document.body).toLowerCase() : '';
			return hasLoginShell() || location.hostname.includes('auth.openai.com') || location.href.toLowerCase().includes('/auth/login') || authSurfaceTextReady(body);
		};
		const findEmailInput = () => Array.from(document.querySelectorAll('input, textarea')).filter(visible).find((el) => {
			const id = el.id || '';
			const label = id ? (document.querySelector('label[for="' + CSS.escape(id) + '"]')?.innerText || '') : '';
			const hay = [el.type, el.name, el.autocomplete, el.placeholder, el.getAttribute('aria-label'), label].filter(Boolean).join(' ').toLowerCase();
			return el.type === 'email' || hay.includes('email') || hay.includes('e-mail') || hay.includes('电子邮件') || hay.includes('邮箱') || hay.includes('mail');
		});
		const result = { url: location.href, title: document.title, email_input_ready: false, login_surface_ready: false };
		for (let i = 0; i < 16; i += 1) {
			const emailInput = findEmailInput();
			const surfaceReady = isLoginSurface();
			if (emailInput && surfaceReady) {
				result.email_input_ready = true;
				result.login_surface_ready = true;
				result.email_input_label = [emailInput.type, emailInput.name, emailInput.placeholder, emailInput.getAttribute('aria-label')].filter(Boolean).join(' ');
				break;
			}
			if (surfaceReady) {
				result.login_surface_ready = true;
				break;
			}
			await sleep(250);
		}
		result.url = location.href;
		result.title = document.title;
		result.body_text_sample = (document.body ? textOf(document.body).slice(0, 500) : '');
		result.visible_inputs = Array.from(document.querySelectorAll('input, textarea')).filter(visible).map((el) => ({ type: el.type || '', name: el.name || '', placeholder: el.placeholder || '', aria: el.getAttribute('aria-label') || '' })).slice(0, 12);
		return JSON.stringify(result);
	})()`
	raw, err := executeCDPScript(conn, script)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return result, nil
}

func shouldUseDirectLoginNavigation(action map[string]any) bool {
	if action == nil {
		return false
	}
	if ready, _ := action["email_input_ready"].(bool); ready {
		return false
	}
	if ready, _ := action["login_surface_ready"].(bool); ready {
		return false
	}
	if suggested, _ := action["login_direct_navigation_suggested"].(bool); suggested {
		return true
	}
	if required, _ := action["login_trusted_click_required"].(bool); required {
		return true
	}
	return false
}

func describeLoginNotInteractiveReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "about_blank":
		return "当前目标页仍是 about:blank"
	case "body_missing":
		return "页面 body 尚未建立"
	case "document_loading":
		return "document.readyState 仍在 loading"
	case "bootstrap_placeholder":
		return "页面还停留在前端 bootstrap/theme 初始化脚本阶段"
	case "no_interactive_controls":
		return "页面尚未渲染出任何可操作控件"
	case "buttons_not_ready":
		return "页面主体已出现，但按钮控件仍未渲染完成"
	case "":
		return "未捕获到明确原因"
	default:
		return reason
	}
}

func runLoginDirectNavigationStep(conn *websocket.Conn) (map[string]any, error) {
	result := map[string]any{
		"attempted": true,
		"attempts":  []map[string]any{},
	}
	candidates := []string{
		"https://chatgpt.com/auth/login",
		"https://auth.openai.com/log-in",
	}
	_, _ = sendCDPCommand(conn, "Page.enable", nil)
	attempts := make([]map[string]any, 0, len(candidates))
	for _, targetURL := range candidates {
		attempt := map[string]any{"url": targetURL}
		if _, err := sendCDPCommand(conn, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
			attempt["navigate_error"] = err.Error()
			attempts = append(attempts, attempt)
			continue
		}
		attempt["navigate_dispatched"] = true
		time.Sleep(1200 * time.Millisecond)
		probe, err := runLoginWindowProbeStep(conn)
		if err != nil {
			attempt["probe_error"] = err.Error()
			attempts = append(attempts, attempt)
			if isTransientCDPPageError(err) {
				time.Sleep(600 * time.Millisecond)
			}
			continue
		}
		attempt["probe"] = probe
		attempts = append(attempts, attempt)
		if ready, _ := probe["email_input_ready"].(bool); ready {
			result["email_input_ready"] = true
		}
		if ready, _ := probe["login_surface_ready"].(bool); ready {
			result["login_surface_ready"] = true
		}
		if currentURL := stringifyJSONValue(probe["url"]); currentURL != "" {
			result["url"] = currentURL
		}
		if title := stringifyJSONValue(probe["title"]); title != "" {
			result["title"] = title
		}
		if ready, _ := probe["email_input_ready"].(bool); ready {
			result["attempts"] = attempts
			result["succeeded"] = true
			return result, nil
		}
		if ready, _ := probe["login_surface_ready"].(bool); ready {
			result["attempts"] = attempts
			result["succeeded"] = true
			return result, nil
		}
	}
	result["attempts"] = attempts
	result["succeeded"] = false
	return result, nil
}

func runLoginCookieConsentDismissStep(conn *websocket.Conn) (map[string]any, error) {
	script := `(async () => {
		const visible = (el) => {
			if (!el) return false;
			const rect = el.getBoundingClientRect();
			const style = window.getComputedStyle(el);
			return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
		};
		const textOf = (el) => ((el && (el.innerText || el.textContent || el.getAttribute('aria-label') || el.value || '')) || '').replace(/\s+/g, ' ').trim();
		const clickElement = (el) => {
			try { el.scrollIntoView({ block: 'center', inline: 'center' }); } catch (_) {}
			const rect = el.getBoundingClientRect();
			const x = rect.left + rect.width / 2;
			const y = rect.top + rect.height / 2;
			for (const type of ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click']) {
				el.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, view: window, clientX: x, clientY: y }));
			}
			try { el.click(); } catch (_) {}
		};
		const bodyText = document.body ? textOf(document.body) : '';
		const hasCookieBanner = /cookie|cookies|cookie 政策|我们使用 cookie|隐私|privacy/i.test(bodyText);
		const buttons = Array.from(document.querySelectorAll('button, a, [role="button"], input[type="button"], input[type="submit"]')).filter(visible).map((el) => {
			const text = textOf(el);
			const hay = [text, el.getAttribute('aria-label'), el.id, el.getAttribute('name'), el.className, el.value].filter(Boolean).join(' ').toLowerCase();
			let score = 0;
			if (text === '拒绝非必需' || text === '全部接受' || text === '接受全部' || text === '同意' || text === '接受') score += 120;
			if (hay.includes('reject non-essential') || hay.includes('reject optional') || hay.includes('reject all') || hay.includes('拒绝非必需') || hay.includes('拒绝')) score += 110;
			if (hay.includes('accept all') || hay.includes('allow all') || hay.includes('agree') || hay.includes('全部接受') || hay.includes('接受全部') || hay.includes('同意') || hay.includes('接受')) score += 90;
			if (hay.includes('cookie')) score += 25;
			if (hay.includes('manage') || hay.includes('preferences') || hay.includes('管理') || hay.includes('设置')) score -= 30;
			if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 120;
			return { el, score, text: text.slice(0, 80) || el.value || el.getAttribute('aria-label') || '' };
		}).filter((item) => item.score >= 80).sort((a, b) => b.score - a.score);
		const result = { url: location.href, title: document.title, cookie_banner_present: hasCookieBanner, cookie_consent_dismissed: false };
		if (buttons.length) {
			result.cookie_button_text = buttons[0].text;
			clickElement(buttons[0].el);
			result.cookie_consent_dismissed = true;
		}
		return JSON.stringify(result);
	})()`
	raw, err := executeCDPScript(conn, script)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return result, nil
}

func runLoginEmailFillStep(conn *websocket.Conn, email string) (map[string]any, error) {
	emailJSON, _ := json.Marshal(email)
	script := fmt.Sprintf(`(async () => {
		const email = %s;
		const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
		const visible = (el) => {
			if (!el) return false;
			const rect = el.getBoundingClientRect();
			const style = window.getComputedStyle(el);
			return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
		};
		const textOf = (el) => ((el && (el.innerText || el.textContent || el.getAttribute('aria-label') || el.value || '')) || '').replace(/\s+/g, ' ').trim();
		const authSurfaceTextReady = (text) => {
			const hay = (text || '').toLowerCase();
			const hasEntryTitle = hay.includes('log in or sign up') || hay.includes('login or sign up') || hay.includes('登录或注册');
			const hasEmailCue = hay.includes('email address') || hay.includes('电子邮件地址') || hay.includes('邮箱地址');
			const hasProviderCue = hay.includes('continue with google') || hay.includes('continue with apple') || hay.includes('continue with phone') || hay.includes('phone number') || hay.includes('使用 google') || hay.includes('使用 apple') || hay.includes('使用电话号码') || hay.includes('电话号码继续');
			const hasContinueCue = hay.includes('continue') || hay.includes('继续');
			return (hasEntryTitle && (hasEmailCue || hasProviderCue || hasContinueCue)) || (hasEmailCue && hasContinueCue);
		};
		const hasLoginShell = () => {
			const frames = Array.from(document.querySelectorAll('iframe')).filter(visible).some((el) => {
				const hay = [el.src, el.title, el.name, el.id, el.className, el.getAttribute('aria-label')].filter(Boolean).join(' ').toLowerCase();
				return hay.includes('auth.openai.com') || hay.includes('/auth/') || hay.includes('login') || hay.includes('signin') || hay.includes('sign-in') || hay.includes('登录') || hay.includes('邮箱');
			});
			const shells = Array.from(document.querySelectorAll('dialog, [role="dialog"], [aria-modal="true"], [data-testid*="modal"], [data-testid*="auth"], [data-testid*="login"], [data-radix-portal], [popover]')).filter(visible).some((el) => {
				return authSurfaceTextReady(textOf(el));
			});
			return frames || shells;
		};
		const isLoginSurface = () => {
			const body = document.body ? textOf(document.body).toLowerCase() : '';
			return hasLoginShell() || location.hostname.includes('auth.openai.com') || location.href.toLowerCase().includes('/auth/login') || authSurfaceTextReady(body);
		};
		const setValue = (el, value) => {
			const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
			const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
			if (setter) setter.call(el, value); else el.value = value;
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
		};
		const inputCandidates = () => Array.from(document.querySelectorAll('input, textarea')).filter(visible).map((el) => {
			const id = el.id || '';
			const label = id ? (document.querySelector('label[for="' + CSS.escape(id) + '"]')?.innerText || '') : '';
			return { el, label: [label, el.getAttribute('aria-label'), el.getAttribute('placeholder'), el.getAttribute('name'), el.getAttribute('autocomplete'), el.type].filter(Boolean).join(' ') };
		});
		const findEmailInput = () => {
			const candidates = inputCandidates();
			return candidates.find((item) => {
				const hay = item.label.toLowerCase();
				return item.el.type === 'email' || hay.includes('email') || hay.includes('e-mail') || hay.includes('电子邮件') || hay.includes('邮箱') || hay.includes('mail');
			}) || candidates.find((item) => ['text', 'search', ''].includes((item.el.type || '').toLowerCase()));
		};
		const clickables = () => Array.from(document.querySelectorAll('button, a, [role="button"], input[type="button"], input[type="submit"]')).filter(visible);
		const findEmailModeButton = () => {
			const items = clickables().map((el) => {
				const text = textOf(el).toLowerCase();
				const hay = [text, el.getAttribute('aria-label'), el.id, el.getAttribute('name'), el.className, el.value, el.getAttribute('href')].filter(Boolean).join(' ').toLowerCase();
				let score = 0;
				if (text === 'continue with email' || text === 'use email' || text === 'use email instead' || text === 'sign in with email' || text === '使用电子邮箱继续' || text === '使用邮箱继续' || text === '用邮箱继续' || text === '通过邮箱继续') score += 120;
				if (hay.includes('continue with email') || hay.includes('use email') || hay.includes('email instead') || hay.includes('sign in with email') || hay.includes('login with email') || hay.includes('log in with email') || hay.includes('电子邮箱') || hay.includes('邮箱继续') || hay.includes('用邮箱') || hay.includes('通过邮箱')) score += 90;
				if (hay.includes('phone') || hay.includes('sms') || hay.includes('mobile') || hay.includes('手机号') || hay.includes('手机号码')) score -= 80;
				if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 100;
				return { el, score, text: textOf(el).slice(0, 80) || el.value || el.getAttribute('aria-label') || 'use email' };
			}).filter((item) => item.score > 0).sort((a, b) => b.score - a.score);
			return items[0] || null;
		};
		const findContinueButton = () => {
			const items = clickables().map((el) => {
				const text = textOf(el).toLowerCase();
				const aria = (el.getAttribute('aria-label') || '').toLowerCase();
				const id = (el.id || '').toLowerCase();
				const name = (el.getAttribute('name') || '').toLowerCase();
				const cls = (el.className || '').toString().toLowerCase();
				const value = (el.value || '').toLowerCase();
				const hay = [text, aria, id, name, cls, value].join(' ');
				let score = 0;
				if (text === 'continue' || text === 'next' || text === '继续' || text === '下一步') score += 100;
				if (hay.includes('continue') || hay.includes('next') || hay.includes('继续') || hay.includes('下一步')) score += 70;
				if (el.tagName === 'BUTTON' || (el.getAttribute('role') || '').toLowerCase() === 'button') score += 10;
				if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 100;
				return { el, score, text: textOf(el).slice(0, 80) || el.value || el.getAttribute('aria-label') || 'continue' };
			}).filter((item) => item.score > 0).sort((a, b) => b.score - a.score);
			return items[0] || null;
		};
		const verificationProbe = () => {
			const inputs = inputCandidates();
			const visibleText = document.body ? textOf(document.body).toLowerCase() : '';
			const otpInput = inputs.find((item) => {
				const hay = [item.label, item.el.inputMode, item.el.pattern, item.el.maxLength, item.el.type].filter(Boolean).join(' ').toLowerCase();
				return hay.includes('code') || hay.includes('verification') || hay.includes('otp') || hay.includes('验证码') || hay.includes('verify') || item.el.inputMode === 'numeric' || Number(item.el.maxLength) >= 4 && Number(item.el.maxLength) <= 8;
			});
			const textReady = visibleText.includes('verification code') || visibleText.includes('enter code') || visibleText.includes('check your email') || visibleText.includes('验证码') || visibleText.includes('验证') || location.href.includes('verification');
			return { ready: !!(otpInput || textReady), hint: otpInput ? (otpInput.label || otpInput.el.type || 'code input') : (textReady ? 'verification text detected' : '') };
		};
		const pageErrorProbe = () => {
			const nodes = Array.from(document.querySelectorAll('[role="alert"], [aria-live], .error, .text-error, [data-testid*="error"], [data-testid*="toast"]')).filter(visible);
			const messages = nodes.map((el) => textOf(el)).filter(Boolean).slice(0, 40);
			const bodyText = document.body ? textOf(document.body).slice(0, 3000) : '';
			if (bodyText.toLowerCase().includes('operation timed out')) messages.push('Operation timed out');
			const message = messages.find((text) => text.toLowerCase().includes('operation timed out') || text.includes('操作超时') || text.includes('请求超时')) || '';
			return { timed_out: Boolean(message), message };
		};
		const clearTransientPageError = () => {
			const buttons = clickables().filter((el) => {
				const text = textOf(el).toLowerCase();
				const label = [text, el.getAttribute('aria-label'), el.getAttribute('title')].filter(Boolean).join(' ').toLowerCase();
				return label.includes('close') || label.includes('dismiss') || label.includes('retry') || label.includes('try again') || label.includes('关闭') || label.includes('重试') || label === '×' || label === 'x';
			});
			for (const button of buttons.slice(0, 3)) {
				try { clickElement(button); } catch (_err) {}
			}
			return buttons.length > 0;
		};
		const result = { url: location.href, title: document.title, clicked_login: false, email_mode_switched: false, email_filled: false, clicked_continue: false, verification_ready: false, operation_timed_out: false };
		const existingVerification = verificationProbe();
		if (existingVerification.ready) {
			result.verification_ready = true;
			result.verification_hint = existingVerification.hint;
			return JSON.stringify(result);
		}
		let emailInput = findEmailInput();
		if (!emailInput) {
			const emailModeButton = findEmailModeButton();
			if (emailModeButton) {
				result.email_mode_button_text = emailModeButton.text;
				emailModeButton.el.click();
				result.email_mode_switched = true;
				for (let i = 0; i < 36; i += 1) {
					await sleep(250);
					emailInput = findEmailInput();
					if (emailInput) break;
				}
			}
		}
		if (emailInput) {
			emailInput.el.focus();
			setValue(emailInput.el, email);
			await sleep(500);
			result.email_filled = true;
			result.email_input_label = emailInput.label || emailInput.el.type || 'input';
			let continueButton = null;
			for (let i = 0; i < 20; i += 1) {
				continueButton = findContinueButton();
				if (continueButton) break;
				await sleep(250);
			}
			if (continueButton) {
				result.continue_button_text = continueButton.text;
				continueButton.el.click();
				result.clicked_continue = true;
				for (let i = 0; i < 80; i += 1) {
					await sleep(300);
					const probe = verificationProbe();
					if (probe.ready) {
						result.verification_ready = true;
						result.verification_hint = probe.hint;
						break;
					}
					const pageError = pageErrorProbe();
					if (pageError.timed_out) {
						result.operation_timed_out = true;
						result.page_error = pageError.message;
						result.transient_error_cleared = clearTransientPageError();
						await sleep(500);
					}
				}
			}
			result.url = location.href;
			result.title = document.title;
			return JSON.stringify(result);
		}
		if (isLoginSurface()) {
			result.login_surface_ready = true;
			result.visible_inputs = inputCandidates().length;
			result.visible_buttons = clickables().slice(0, 8).map((el) => textOf(el).slice(0, 60)).filter(Boolean);
			result.url = location.href;
			result.title = document.title;
			return JSON.stringify(result);
		}
		const clickable = clickables();
		const loginButton = clickable.find((el) => {
			const t = textOf(el).toLowerCase();
			const href = (el.getAttribute('href') || '').toLowerCase();
			const id = (el.id || '').toLowerCase();
			const cls = (el.className || '').toString().toLowerCase();
			return t === 'log in' || t === 'login' || t === 'sign in' || t.includes('log in') || t.includes('login') || t.includes('sign in') || t.includes('登录') || t.includes('登入') || href.includes('/auth/login') || id.includes('login') || cls.includes('login');
		});
		if (loginButton) {
			result.login_button_text = textOf(loginButton).slice(0, 80);
			loginButton.click();
			result.clicked_login = true;
			await sleep(800);
			result.url = location.href;
			return JSON.stringify(result);
		}
		result.visible_inputs = inputCandidates().length;
		result.visible_buttons = clickable.slice(0, 8).map((el) => textOf(el).slice(0, 60)).filter(Boolean);
		return JSON.stringify(result);
	})()`, string(emailJSON))
	raw, err := executeCDPScript(conn, script)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return result, nil
}

func runLoginCodeFillStep(conn *websocket.Conn, code string, profile japaneseProfile) (map[string]any, error) {
	codeJSON, _ := json.Marshal(code)
	profileNameJSON, _ := json.Marshal(profile.Name)
	script := fmt.Sprintf(`(async () => {
		const code = %s;
		const profileName = %s;
		const profileAge = %d;
		const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
		const visible = (el) => {
			if (!el) return false;
			const rect = el.getBoundingClientRect();
			const style = window.getComputedStyle(el);
			return rect.width > 0 && rect.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
		};
		const textOf = (el) => ((el && (el.innerText || el.textContent || el.getAttribute('aria-label') || el.value || '')) || '').replace(/\s+/g, ' ').trim();
		const setValue = (el, value) => {
			const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
			const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
			if (setter) setter.call(el, value); else el.value = value;
			el.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: value }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
		};
		const clickElement = (el) => {
			try { el.scrollIntoView({ block: 'center', inline: 'center' }); } catch (_) {}
			const rect = el.getBoundingClientRect();
			const x = rect.left + rect.width / 2;
			const y = rect.top + rect.height / 2;
			for (const type of ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click']) {
				el.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, view: window, clientX: x, clientY: y }));
			}
		};
		const inputCandidates = () => Array.from(document.querySelectorAll('input, textarea')).filter(visible).map((el) => {
			const id = el.id || '';
			const label = id ? (document.querySelector('label[for="' + CSS.escape(id) + '"]')?.innerText || '') : '';
			const wrapperText = textOf(el.closest('label, [role="group"], form, section, div') || '').slice(0, 240);
			return { el, label: [label, el.getAttribute('aria-label'), el.getAttribute('placeholder'), el.getAttribute('name'), el.getAttribute('autocomplete'), el.inputMode, el.pattern, el.type, wrapperText].filter(Boolean).join(' ') };
		});
		const clickables = () => Array.from(document.querySelectorAll('button, a, [role="button"], input[type="button"], input[type="submit"]')).filter(visible);
		const profileSurfaceProbe = () => {
			const bodyText = document.body ? textOf(document.body) : '';
			const lower = bodyText.toLowerCase();
			const surface = lower.includes('how old are you') || bodyText.includes('你的年龄是多少') || bodyText.includes('全名') && bodyText.includes('年龄') || bodyText.includes('完成帐户创建') || bodyText.includes('完成账户创建');
			if (!surface) return { ready: false };
			const candidates = inputCandidates().filter((item) => {
				const type = (item.el.type || '').toLowerCase();
				return !['hidden', 'file', 'checkbox', 'radio', 'submit', 'button'].includes(type) && !item.el.disabled && !item.el.readOnly;
			});
			const scoreName = (item) => {
				const hay = item.label.toLowerCase();
				let score = 0;
				if (hay.includes('full name') || hay.includes('display name') || hay.includes('your name') || hay.includes('全名') || hay.includes('姓名') || hay.includes('名字') || hay.includes('氏名') || hay.includes('名前')) score += 100;
				if (hay.includes('age') || hay.includes('年龄') || hay.includes('年齢') || hay.includes('验证码') || hay.includes('verification') || hay.includes('code') || hay.includes('otp')) score -= 120;
				return score;
			};
			const scoreAge = (item) => {
				const hay = item.label.toLowerCase();
				let score = 0;
				if (hay.includes('age') || hay.includes('年龄') || hay.includes('年齢')) score += 120;
				if (item.el.type === 'number' || item.el.inputMode === 'numeric' || item.el.inputMode === 'decimal') score += 40;
				if (hay.includes('name') || hay.includes('全名') || hay.includes('姓名') || hay.includes('名字') || hay.includes('氏名') || hay.includes('名前') || hay.includes('验证码') || hay.includes('verification') || hay.includes('code') || hay.includes('otp')) score -= 120;
				return score;
			};
			const rankedName = candidates.map((item) => ({ item, score: scoreName(item) })).filter((entry) => entry.score > 0).sort((a, b) => b.score - a.score);
			const rankedAge = candidates.map((item) => ({ item, score: scoreAge(item) })).filter((entry) => entry.score > 0).sort((a, b) => b.score - a.score);
			let nameInput = rankedName[0]?.item || null;
			let ageInput = rankedAge[0]?.item || null;
			if ((!nameInput || !ageInput) && candidates.length >= 2) {
				nameInput = nameInput || candidates[0];
				ageInput = ageInput || candidates.find((item) => item.el !== nameInput && (item.el.type === 'number' || item.el.inputMode === 'numeric')) || candidates.find((item) => item.el !== nameInput) || null;
			}
			return { ready: !!(nameInput && ageInput), surface, nameInput, ageInput, input_count: candidates.length };
		};
		const findProfileSubmitButton = () => {
			const items = clickables().map((el) => {
				const text = textOf(el).toLowerCase();
				const hay = [text, el.getAttribute('aria-label'), el.id, el.getAttribute('name'), el.className, el.value].filter(Boolean).join(' ').toLowerCase();
				let score = 0;
				if (text === 'complete account creation' || text === '完成帐户创建' || text === '完成账户创建') score += 140;
				if (hay.includes('complete account') || hay.includes('create account') || hay.includes('finish') || hay.includes('continue') || hay.includes('完成帐户创建') || hay.includes('完成账户创建') || hay.includes('完成') || hay.includes('创建') || hay.includes('继续')) score += 90;
				if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 120;
				return { el, score, text: textOf(el).slice(0, 80) || el.value || el.getAttribute('aria-label') || 'complete' };
			}).filter((item) => item.score > 0).sort((a, b) => b.score - a.score);
			return items[0] || null;
		};
		const fillProfileIfNeeded = async () => {
			const probe = profileSurfaceProbe();
			if (!probe.ready) return { profile_ready: probe.surface || false, profile_filled: false, profile_input_count: probe.input_count || 0 };
			probe.nameInput.el.focus();
			setValue(probe.nameInput.el, profileName);
			await sleep(120);
			probe.ageInput.el.focus();
			setValue(probe.ageInput.el, String(profileAge));
			await sleep(650);
			const submitButton = findProfileSubmitButton();
			const result = { profile_ready: true, profile_filled: true, profile_name: profileName, profile_age: profileAge, profile_input_count: probe.input_count || 0 };
			if (submitButton) {
				result.profile_button_text = submitButton.text;
				clickElement(submitButton.el);
				try { submitButton.el.click(); } catch (_) {}
				result.profile_submitted = true;
			}
			return result;
		};
		const findCodeInput = () => {
			const candidates = inputCandidates();
			const scored = candidates.map((item) => {
				const hay = item.label.toLowerCase();
				const maxLength = Number(item.el.maxLength || 0);
				let score = 0;
				if (hay.includes('code') || hay.includes('verification') || hay.includes('otp') || hay.includes('verify') || hay.includes('验证码') || hay.includes('验证')) score += 90;
				if (item.el.inputMode === 'numeric' || item.el.type === 'tel' || item.el.type === 'number') score += 50;
				if (maxLength >= 4 && maxLength <= 12) score += 40;
				if (item.el.autocomplete === 'one-time-code') score += 100;
				if (item.el.disabled || item.el.readOnly) score -= 120;
				return { item, score };
			}).filter((entry) => entry.score > 0).sort((a, b) => b.score - a.score);
			return scored[0]?.item || candidates.find((item) => ['text', 'tel', 'number', ''].includes((item.el.type || '').toLowerCase())) || null;
		};
		const findSubmitButton = () => {
			const items = clickables().map((el) => {
				const text = textOf(el).toLowerCase();
				const hay = [text, el.getAttribute('aria-label'), el.id, el.getAttribute('name'), el.className, el.value].filter(Boolean).join(' ').toLowerCase();
				let score = 0;
				if (text === 'continue' || text === 'verify' || text === 'submit' || text === 'next' || text === '继续' || text === '验证' || text === '提交' || text === '下一步') score += 100;
				if (hay.includes('continue') || hay.includes('verify') || hay.includes('submit') || hay.includes('next') || hay.includes('继续') || hay.includes('验证') || hay.includes('提交') || hay.includes('下一步')) score += 70;
				if (el.tagName === 'BUTTON' || (el.getAttribute('role') || '').toLowerCase() === 'button') score += 10;
				if (el.disabled || el.getAttribute('aria-disabled') === 'true') score -= 100;
				return { el, score, text: textOf(el).slice(0, 80) || el.value || el.getAttribute('aria-label') || 'submit' };
			}).filter((item) => item.score > 0).sort((a, b) => b.score - a.score);
			return items[0] || null;
		};
		const completionProbe = () => {
			const profileProbe = profileSurfaceProbe();
			if (profileProbe.ready || profileProbe.surface) return { completed: false, hint: 'profile setup form detected' };
			const bodyText = document.body ? textOf(document.body).toLowerCase() : '';
			const href = location.href.toLowerCase();
			const composer = Array.from(document.querySelectorAll('#prompt-textarea, [data-testid*="composer"], textarea, [role="textbox"]')).some(visible);
			const checkoutReady = href.includes('chatgpt.com/checkout/openai_llc/') || href.includes('pay.openai.com/c/pay/') || href.includes('checkout.stripe.com/c/pay/');
			const chatReady = href.includes('chatgpt.com') && !href.includes('auth.openai.com') && !href.includes('/auth/') && !href.includes('verification') && (composer || href.includes('/c/') || href.includes('pricing') || bodyText.includes('chatgpt'));
			const completed = checkoutReady || chatReady;
			const hint = checkoutReady ? 'checkout page detected after login' : (completed && composer ? 'chatgpt composer detected' : (completed ? 'chatgpt logged-in page detected' : ''));
			return { completed, hint };
		};
		const rejectionProbe = () => {
			const errorSelectors = '[role="alert"], [aria-invalid="true"], .error, .text-error, [data-testid*="error"]';
			const alerts = Array.from(document.querySelectorAll(errorSelectors)).filter(visible);
			const messages = alerts.map((el) => textOf(el)).filter(Boolean).slice(0, 40);
			const patterns = ['invalid code', 'incorrect code', 'wrong code', 'expired code', 'code is invalid', 'verification code is invalid', '验证码错误', '验证码无效', '验证码不正确', '验证码已过期', '代码无效'];
			const message = messages.find((text) => patterns.some((pattern) => text.toLowerCase().includes(pattern)));
			return { rejected: Boolean(message), message: message || '' };
		};
		const result = { url: location.href, title: document.title, code_filled: false, code_submitted: false, code_rejected: false, profile_filled: false, login_completed: false };
		const completionBeforeCode = completionProbe();
		if (completionBeforeCode.completed) {
			result.login_completed = true;
			result.completion_hint = completionBeforeCode.hint;
			result.url = location.href;
			result.title = document.title;
			return JSON.stringify(result);
		}
		const profileBeforeCode = await fillProfileIfNeeded();
		if (profileBeforeCode.profile_filled) {
			Object.assign(result, profileBeforeCode);
			for (let i = 0; i < 30; i += 1) {
				await sleep(500);
				const probe = completionProbe();
				if (probe.completed) {
					result.login_completed = true;
					result.completion_hint = probe.hint;
					break;
				}
			}
			result.url = location.href;
			result.title = document.title;
			return JSON.stringify(result);
		}
		const codeInput = findCodeInput();
		if (!codeInput) {
			result.visible_inputs = inputCandidates().map((item) => item.label.slice(0, 80)).filter(Boolean).slice(0, 8);
			return JSON.stringify(result);
		}
		codeInput.el.focus();
		setValue(codeInput.el, code);
		await sleep(180);
		result.code_filled = true;
		result.code_input_label = codeInput.label || codeInput.el.type || 'code input';
		const submitButton = findSubmitButton();
		if (submitButton) {
			result.submit_button_text = submitButton.text;
			submitButton.el.click();
			result.code_submitted = true;
		} else {
			codeInput.el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', bubbles: true }));
			codeInput.el.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', code: 'Enter', bubbles: true }));
			result.code_submitted = true;
			result.submit_button_text = 'Enter';
		}
		for (let i = 0; i < 60; i += 1) {
			await sleep(500);
			const profileResult = await fillProfileIfNeeded();
			if (profileResult.profile_filled) {
				Object.assign(result, profileResult);
				await sleep(900);
				continue;
			}
			const probe = completionProbe();
			if (probe.completed) {
				result.login_completed = true;
				result.completion_hint = probe.hint;
				break;
			}
			const rejection = rejectionProbe();
			if (rejection.rejected) {
				result.code_rejected = true;
				result.completion_hint = rejection.message;
				break;
			}
		}
		result.url = location.href;
		result.title = document.title;
		return JSON.stringify(result);
	})()`, string(codeJSON), string(profileNameJSON), profile.Age)
	raw, err := executeCDPScript(conn, script)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{"raw": raw}, nil
	}
	return result, nil
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

func isGopayCDPTargetURL(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "pin-web-client.gopayapi.com") ||
		strings.Contains(lower, "merchants-gws-app.gopayapi.com") ||
		strings.Contains(lower, "midtrans.com")
}

type gopayCDPTargetScope struct {
	TargetID    string
	TargetURL   string
	AccountID   string
	CheckoutURL string
}

func normalizeGopayCDPTargetScope(scope gopayCDPTargetScope) gopayCDPTargetScope {
	scope.TargetID = strings.TrimSpace(scope.TargetID)
	scope.TargetURL = strings.TrimSpace(scope.TargetURL)
	scope.AccountID = strings.TrimSpace(scope.AccountID)
	scope.CheckoutURL = strings.TrimSpace(scope.CheckoutURL)
	if scope.AccountID == "" && scope.TargetURL != "" {
		scope.AccountID = midtransRedirectionAccountID(scope.TargetURL)
	}
	return scope
}

func gopayCDPTargetScore(target cdpTarget) int {
	lower := strings.ToLower(strings.TrimSpace(target.URL))
	if lower == "" {
		return -1
	}
	score := 0
	if target.Type == "page" {
		score += 30
	} else if target.Type == "iframe" {
		score += 20
	}
	switch {
	case strings.Contains(lower, "/payment/validate-pin"):
		score += 600
	case strings.Contains(lower, "/payment/pin"):
		score += 560
	case strings.Contains(lower, "/auth/pin/verify"):
		score += 500
	case strings.Contains(lower, "/linking/otp"):
		score += 420
	case strings.Contains(lower, "/payment/details"):
		score += 340
	case strings.Contains(lower, "/snap/v4/redirection/"):
		score += 260
	case strings.Contains(lower, "/linking/success"):
		score += 120
	}
	if strings.Contains(lower, "pin-web-client.gopayapi.com") {
		score += 120
	}
	if strings.Contains(lower, "merchants-gws-app.gopayapi.com") {
		score += 90
	}
	if strings.Contains(lower, "midtrans.com") {
		score += 60
	}
	if strings.Contains(lower, "payment") {
		score += 40
	}
	if strings.Contains(lower, "pin") {
		score += 35
	}
	if strings.Contains(lower, "otp") {
		score += 25
	}
	if strings.Contains(lower, "success") {
		score -= 30
	}
	return score
}

func gopayCDPTargetScopeScore(target cdpTarget, scope gopayCDPTargetScope) int {
	scope = normalizeGopayCDPTargetScope(scope)
	lower := strings.ToLower(strings.TrimSpace(target.URL))
	score := 0
	if scope.TargetID != "" && target.ID == scope.TargetID {
		score += 1400
	}
	if scope.TargetURL != "" && strings.EqualFold(strings.TrimSpace(target.URL), scope.TargetURL) {
		score += 1000
	}
	if scope.AccountID != "" {
		targetAccountID := midtransRedirectionAccountID(target.URL)
		switch {
		case targetAccountID == scope.AccountID:
			score += 900
		case targetAccountID != "":
			score -= 500
		case strings.Contains(lower, strings.ToLower(scope.AccountID)):
			score += 600
		}
	}
	if scope.CheckoutURL != "" {
		if checkoutKey := checkoutAutoTriggerKey(scope.CheckoutURL); checkoutKey != "" && strings.Contains(lower, strings.ToLower(checkoutKey)) {
			score += 200
		}
	}
	return score
}

func findBestGopayCDPTarget(port int, scope gopayCDPTargetScope) (*cdpTarget, error) {
	targets, err := getCDPTargets(port)
	if err != nil {
		return nil, err
	}
	scope = normalizeGopayCDPTargetScope(scope)
	bestScore := -1
	var best *cdpTarget
	for i := range targets {
		target := &targets[i]
		exactScopedTarget := scope.TargetID != "" && target.ID == scope.TargetID
		if target.WebSocketDebuggerURL == "" || (!exactScopedTarget && !isGopayCDPTargetURL(target.URL)) {
			continue
		}
		score := gopayCDPTargetScore(*target) + gopayCDPTargetScopeScore(*target, scope)
		if score > bestScore {
			bestScore = score
			copyTarget := *target
			best = &copyTarget
		}
	}
	if best == nil {
		return nil, &cdpNotReadyError{message: "未在 CDP 中找到 GoPay OTP/PIN 页面"}
	}
	return best, nil
}

func dispatchCDPMouseClickFromAction(conn *websocket.Conn, action map[string]any) error {
	rawRect, ok := action["login_button_rect"].(map[string]any)
	if !ok {
		return errors.New("login button rect missing")
	}
	left, okLeft := numberFromAny(rawRect["left"])
	top, okTop := numberFromAny(rawRect["top"])
	width, okWidth := numberFromAny(rawRect["width"])
	height, okHeight := numberFromAny(rawRect["height"])
	if !okLeft || !okTop || !okWidth || !okHeight || width <= 0 || height <= 0 {
		return errors.New("login button rect invalid")
	}
	x := left + width/2
	y := top + height/2
	idMove := atomic.AddInt64(&cdpCommandCounter, 1)
	if _, err := sendCDPCommandWithID(conn, idMove, "Input.dispatchMouseEvent", map[string]any{
		"type":        "mouseMoved",
		"x":           x,
		"y":           y,
		"button":      "none",
		"buttons":     0,
		"pointerType": "mouse",
	}); err != nil {
		return err
	}
	idDown := atomic.AddInt64(&cdpCommandCounter, 1)
	if _, err := sendCDPCommandWithID(conn, idDown, "Input.dispatchMouseEvent", map[string]any{
		"type":        "mousePressed",
		"x":           x,
		"y":           y,
		"button":      "left",
		"buttons":     1,
		"clickCount":  1,
		"pointerType": "mouse",
	}); err != nil {
		return err
	}
	idUp := atomic.AddInt64(&cdpCommandCounter, 1)
	_, err := sendCDPCommandWithID(conn, idUp, "Input.dispatchMouseEvent", map[string]any{
		"type":        "mouseReleased",
		"x":           x,
		"y":           y,
		"button":      "left",
		"buttons":     0,
		"clickCount":  1,
		"pointerType": "mouse",
	})
	return err
}

func numberFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
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

func isTransientCDPPageError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	transientSignals := []string{
		"inspected target navigated or closed",
		"target closed",
		"websocket: close",
		"close 1006",
		"use of closed network connection",
		"connection reset",
		"unexpected eof",
		"broken pipe",
		"no such execution context",
		"execution context was destroyed",
		"cannot find context with specified id",
	}
	for _, signal := range transientSignals {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

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
	if relaunched, err := ensureManagedCDPReady(context.Background(), "https://chatgpt.com/"); err != nil {
		return "", withSessionDiagnosticsConclusion(map[string]any{"cdp_ready": false, "cdp_relaunch_failed": true}), &cdpNotReadyError{message: "CDP 未就绪，自动重新拉起无痕窗口失败: " + err.Error()}
	} else if relaunched && onEvent != nil {
		onEvent(monitorEvent{Timestamp: time.Now().UnixMilli(), Domain: "Log", Method: "session-cdp-relaunch", Summary: "CDP 未就绪，已自动重新拉起 Chrome 无痕窗口"})
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
	"/api/login/email-fill":            "login_email_prepare",
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
	"/api/luckmail/create-and-wait":    "luckmail_email_code_flow",
	"/api/luckmail/token-code":         "luckmail_purchased_email_code_flow",
	"/api/luckmail/token-mails":        "luckmail_purchased_email_mail_list",
}

var auditFlowStepIndexes = map[string]int{
	"/api/health":                      10,
	"/api/incognito/open":              20,
	"/api/login/click":                 23,
	"/api/login/email-fill":            25,
	"/api/login/code-fill":             27,
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
	"/api/luckmail/create-and-wait":    150,
	"/api/luckmail/token-code":         151,
	"/api/luckmail/token-mails":        152,
}

var auditFlowRouteRoles = map[string]string{
	"/api/health":                      "确认本地 Go 服务可用，是全流程运行前的环境健康信号",
	"/api/incognito/open":              "打开或复用系统 Chrome 无痕窗口，为登录态、checkout 页面和 CDP 观测建立浏览器上下文",
	"/api/login/email-fill":            "监控无痕窗口页面，点击登录入口、填写已购邮箱地址、点击继续并等待验证码页面",
	"/api/login/code-fill":             "监控无痕窗口验证码页面，自动填入 LuckMail 邮箱验证码并尝试提交",
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
	"/api/luckmail/create-and-wait":    "创建 LuckMail 邮箱接码订单并等待邮件验证码返回，后续可用于人工填入验证码",
	"/api/luckmail/token-code":         "通过 LuckMail 已购邮箱 Token 等待或查询最新邮件验证码",
	"/api/luckmail/token-mails":        "通过 LuckMail 已购邮箱 Token 查询邮件列表摘要，用于确认邮件是否到达",
}

var flowAuditLogger = newAuditLogger(filepath.Join(".", "log"), 24*time.Hour, 8<<20, 512)

var gopayAutoTriggerMu sync.Mutex
var gopayAutoTriggeredCheckout = map[string]time.Time{}

var operationDisplayNames = map[string]string{
	"/api/health":                      "健康检查",
	"/api/checkout":                    "生成支付链接",
	"/api/checkout/start":              "生成支付链接",
	"/api/incognito/open":              "打开无痕窗口",
	"/api/login/click":                 "登录按钮自动点击",
	"/api/login/email-fill":            "登录页填写邮箱",
	"/api/login/code-fill":             "登录验证码自动填入",
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
	"/api/luckmail/create-and-wait":    "LuckMail 邮箱接码",
	"/api/luckmail/token-code":         "LuckMail 已购邮箱验证码",
	"/api/luckmail/token-mails":        "LuckMail 已购邮箱邮件列表",
}

var operationTypes = map[string]string{
	"/api/health":                      "system_health",
	"/api/checkout":                    "checkout_create",
	"/api/checkout/start":              "checkout_create",
	"/api/incognito/open":              "login_open_window",
	"/api/login/click":                 "login_click",
	"/api/login/email-fill":            "login_email_fill",
	"/api/login/code-fill":             "login_code_fill",
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
	"/api/luckmail/create-and-wait":    "email_code_query",
	"/api/luckmail/token-code":         "email_code_query",
	"/api/luckmail/token-mails":        "email_mail_query",
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
	"verification_code":          {},
	"mail_body_html":             {},
	"html_body":                  {},
	"body_html":                  {},
	"body_text":                  {},
	"luckmail_token":             {},
	"luckmail_api_key":           {},
	"api_key":                    {},
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
	"target_title":               {},
	"clicked_login":              {},
	"login_button_text":          {},
	"email_mode_switched":        {},
	"email_mode_button_text":     {},
	"email_input_ready":          {},
	"email_filled":               {},
	"email_input_label":          {},
	"clicked_continue":           {},
	"continue_button_text":       {},
	"verification_ready":         {},
	"verification_hint":          {},
	"code_filled":                {},
	"code_input_label":           {},
	"code_length":                {},
	"code_submitted":             {},
	"submit_button_text":         {},
	"login_completed":            {},
	"completion_hint":            {},
	"current_url":                {},
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
	"order_no":                   {},
	"email_address":              {},
	"mail_from":                  {},
	"mail_subject":               {},
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
	paypalFlow := paypalCheckoutFlowResult{}
	if req.PaymentMethod == "paypal" {
		paypalFlow = createPayPalCheckoutRedirect(ctx, client, checkoutData, stripeInit.Body, req)
		checkoutURL = paypalFlow.URL
	} else {
		checkoutURL = checkoutPaymentURL(req.PaymentMethod, checkoutData, stripeInit.Body)
	}
	if req.PaymentMethod == "paypal" && checkoutURL == "" {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":                  false,
			"stage":               "paypal_link",
			"error":               firstNonEmpty(paypalFlow.Error, "checkout response did not include a valid PayPal payment url"),
			"checkout_session_id": firstNonEmpty(findStringField(checkoutData, "checkout_session_id", "id"), stripeInit.CheckoutSessionID),
			"payment_method":      req.PaymentMethod,
			"checkout_provider":   findStringField(checkoutData, "checkout_provider"),
			"raw_checkout":        checkoutData,
			"stripe_init":         summarizeStripeResult(stripeInit),
			"stripe_init_body":    stripeInit.Body,
			"paypal_flow":         paypalFlow,
		})
		return
	}
	if req.PaymentMethod != "paypal" && stripeInit.OK && !stripeInitCheck.OK {
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok":                  false,
			"stage":               "checkout_amount_validation",
			"error":               firstNonEmpty(stripeInitCheck.Error, "checkout session amount is invalid"),
			"hint":                "当前 checkout 会话金额解析失败或金额无效；不同套餐允许不同应付金额，但必须能从 Stripe 会话中解析出有效 total。",
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
		"payment_method":      req.PaymentMethod,
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
	return createStripePaymentMethodForType(ctx, client, pageBody, sessionID, publishableKey, requestedTaxRegion, requestedCustomerEmail, clientCtx, "gopay")
}

func createStripePaymentMethodForType(ctx context.Context, client *http.Client, pageBody any, sessionID string, publishableKey string, requestedTaxRegion taxRegion, requestedCustomerEmail string, clientCtx stripeClientContext, paymentMethodType string) stripeInitResult {
	if err := requireStripeTestMode(sessionID, publishableKey); err != nil {
		return stripeInitResult{OK: false, Error: err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	form := stripePaymentMethodFormForType(pageBody, sessionID, publishableKey, requestedTaxRegion, requestedCustomerEmail, clientCtx, paymentMethodType)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/payment_methods", strings.NewReader(form.Encode()))
	if err != nil {
		return stripeInitResult{OK: false, Error: "failed to create payment method request: " + err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	setStripeInitHeaders(httpReq)
	return doStripeFormRequest(client, httpReq, sessionID, publishableKey, "payment method")
}

func confirmStripePaymentPage(ctx context.Context, client *http.Client, pageBody any, sessionID string, publishableKey string, paymentMethodID string, clientCtx stripeClientContext) stripeInitResult {
	return confirmStripePaymentPageForType(ctx, client, pageBody, sessionID, publishableKey, paymentMethodID, clientCtx, "gopay")
}

func confirmStripePaymentPageForType(ctx context.Context, client *http.Client, pageBody any, sessionID string, publishableKey string, paymentMethodID string, clientCtx stripeClientContext, paymentMethodType string) stripeInitResult {
	if err := requireStripeTestMode(sessionID, publishableKey); err != nil {
		return stripeInitResult{OK: false, Error: err.Error(), CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}
	if paymentMethodID == "" {
		return stripeInitResult{OK: false, Error: "missing payment method id", CheckoutSessionID: sessionID, PublishableKey: publishableKey}
	}

	form := stripeConfirmFormForType(pageBody, sessionID, publishableKey, paymentMethodID, clientCtx, paymentMethodType)
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

func normalizeStripePaymentMethodType(paymentMethodType string) string {
	paymentMethodType = strings.ToLower(strings.TrimSpace(paymentMethodType))
	switch paymentMethodType {
	case "paypal", "pay_pal":
		return "paypal"
	case "gopay", "go_pay", "popay", "":
		return "gopay"
	default:
		return paymentMethodType
	}
}

func stripePaymentMethodForm(pageBody any, sessionID string, publishableKey string, requestedTaxRegion taxRegion, requestedCustomerEmail string, clientCtx stripeClientContext) url.Values {
	return stripePaymentMethodFormForType(pageBody, sessionID, publishableKey, requestedTaxRegion, requestedCustomerEmail, clientCtx, "gopay")
}

func stripePaymentMethodFormForType(pageBody any, sessionID string, publishableKey string, requestedTaxRegion taxRegion, requestedCustomerEmail string, clientCtx stripeClientContext, paymentMethodType string) url.Values {
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
	form.Set("type", normalizeStripePaymentMethodType(paymentMethodType))
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
	return stripeConfirmFormForType(pageBody, sessionID, publishableKey, paymentMethodID, clientCtx, "gopay")
}

func stripeExpectedAmount(pageBody any) string {
	root, _ := pageBody.(map[string]any)
	if root == nil {
		return "0"
	}
	totalSummary, _ := root["total_summary"].(map[string]any)
	if totalSummary == nil {
		return "0"
	}
	if total, ok := jsonNumberToInt(totalSummary["total"]); ok && total >= 0 {
		return strconv.FormatInt(total, 10)
	}
	return "0"
}

func stripeConfirmFormForType(pageBody any, sessionID string, publishableKey string, paymentMethodID string, clientCtx stripeClientContext, paymentMethodType string) url.Values {
	form := url.Values{}
	form.Set("guid", clientCtx.GUID)
	form.Set("muid", clientCtx.MUID)
	form.Set("sid", clientCtx.SID)
	form.Set("payment_method", paymentMethodID)
	form.Set("init_checksum", findStringField(pageBody, "init_checksum"))
	form.Set("version", "332636417d")
	form.Set("expected_amount", stripeExpectedAmount(pageBody))
	form.Set("expected_payment_method_type", normalizeStripePaymentMethodType(paymentMethodType))
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

func gopayLinkingPINCandidates(preferred string) []string {
	return gopayPaymentPINCandidates(preferred)
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
	if total < 0 {
		return stripeInitBodyCheck{Error: fmt.Sprintf("当前会话应付金额为 %d，金额无效", total), Total: total}
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
	if total < 0 {
		return stripeConfirmCheck{Error: "金额无效"}
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
	req.PaymentMethod = normalizePaymentMethod(req.PaymentMethod)
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

func normalizePaymentMethod(method string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	switch method {
	case "", "popay", "gopay", "go_pay":
		return "gopay"
	case "paypal", "pay_pal":
		return "paypal"
	default:
		return ""
	}
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
	if req.PaymentMethod == "" {
		return errors.New("payment_method is required")
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

	if body, err := os.ReadFile("config.local.json"); err == nil {
		return parseAppConfig(body, "config.local.json")
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("read config.local.json failed: %v", err)
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

func configInt(envName string, configured int, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	if configured > 0 {
		return configured
	}
	return fallback
}

func luckMailAPIKey() string {
	return configString("LUCKMAIL_API_KEY", config.LuckMailAPIKey, "")
}

func luckMailBaseURL() string {
	return configString("LUCKMAIL_BASE_URL", config.LuckMailBaseURL, luckmail.DefaultBaseURL)
}

func luckMailDefaultProjectCode() string {
	return configString("LUCKMAIL_DEFAULT_PROJECT_CODE", config.LuckMailDefaultProjectCode, "openai")
}

func luckMailDefaultEmailType() string {
	return configString("LUCKMAIL_DEFAULT_EMAIL_TYPE", config.LuckMailDefaultEmailType, "ms_graph")
}

func luckMailDefaultDomain() string {
	return configString("LUCKMAIL_DEFAULT_DOMAIN", config.LuckMailDefaultDomain, "")
}

func luckMailTimeoutSeconds() int {
	return configInt("LUCKMAIL_TIMEOUT_S", config.LuckMailTimeoutS, 300)
}

func luckMailIntervalSeconds() int {
	return configInt("LUCKMAIL_INTERVAL_S", config.LuckMailIntervalS, 3)
}

func codeViewPublicBaseURL() string {
	baseURL := strings.TrimSpace(configString("CODE_VIEW_PUBLIC_BASE_URL", config.CodeViewPublicBaseURL, "http://127.0.0.1:18473"))
	return strings.TrimRight(baseURL, "/")
}

func luckMailCodeViewURL(token string) string {
	return codeViewPublicBaseURL() + "/mail-code?token=" + url.QueryEscape(token)
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

func checkoutPaymentURL(paymentMethod string, checkoutData any, stripeInitBody any) string {
	if paymentMethod == "paypal" {
		return paypalPaymentURL(checkoutData, stripeInitBody)
	}
	return checkoutLongURL(stripeInitBody)
}

func paypalPaymentURL(values ...any) string {
	if officialURL := paypalOfficialPaymentURL(values...); officialURL != "" {
		return officialURL
	}
	for _, value := range values {
		if hostedURL := checkoutLongURL(value); hostedURL != "" {
			return hostedURL
		}
	}
	return ""
}

func createPayPalCheckoutRedirect(ctx context.Context, client *http.Client, checkoutData any, stripeInitBody any, req checkoutRequest) paypalCheckoutFlowResult {
	if checkoutURL := paypalPaymentURL(checkoutData, stripeInitBody); checkoutURL != "" {
		return paypalCheckoutFlowResult{OK: true, URL: checkoutURL}
	}

	sessionID := firstNonEmpty(findStringField(checkoutData, "checkout_session_id", "checkoutSessionId", "id"), findStringField(stripeInitBody, "checkout_session_id", "checkoutSessionId", "id"))
	publishableKey := firstNonEmpty(findStringField(checkoutData, "publishable_key", "publishableKey", "key"), findStringField(stripeInitBody, "publishable_key", "publishableKey", "key"))
	if sessionID == "" || publishableKey == "" {
		return paypalCheckoutFlowResult{Error: "checkout response missing checkout_session_id or publishable_key"}
	}

	clientCtx := newStripeClientContext(stripeInitBody)
	requestedTaxRegion := mergeTaxRegion(defaultTaxRegion(), req.TaxRegion)
	pageBody := stripeInitBody
	result := paypalCheckoutFlowResult{}
	result.Update = updateStripePaymentPage(ctx, client, stripeInitBody, sessionID, publishableKey, requestedTaxRegion, req.CustomerEmail, clientCtx)
	if result.Update.OK && result.Update.Body != nil {
		pageBody = result.Update.Body
	} else if !result.Update.OK {
		result.Error = firstNonEmpty(result.Update.Error, "failed to update Stripe payment page for PayPal")
		return result
	}

	result.PaymentMethod = createStripePaymentMethodForType(ctx, client, pageBody, sessionID, publishableKey, requestedTaxRegion, req.CustomerEmail, clientCtx, "paypal")
	if !result.PaymentMethod.OK {
		result.Error = firstNonEmpty(result.PaymentMethod.Error, "failed to create PayPal payment method")
		return result
	}
	paymentMethodID := findStringField(result.PaymentMethod.Body, "id", "payment_method")
	if paymentMethodID == "" {
		result.Error = "stripe PayPal payment method response missing id"
		return result
	}

	result.Confirm = confirmStripePaymentPageForType(ctx, client, pageBody, sessionID, publishableKey, paymentMethodID, clientCtx, "paypal")
	if !result.Confirm.OK {
		result.Error = firstNonEmpty(result.Confirm.Error, "failed to confirm Stripe payment page for PayPal")
		return result
	}
	if checkoutURL := paypalPaymentURL(result.Confirm.Body); checkoutURL != "" {
		result.OK = true
		result.URL = checkoutURL
		return result
	}

	result.Details = getStripePaymentPageDetails(ctx, client, sessionID, publishableKey, clientCtx, pageBody)
	if result.Details.OK {
		if checkoutURL := paypalPaymentURL(result.Details.Body); checkoutURL != "" {
			result.OK = true
			result.URL = checkoutURL
			return result
		}
	}
	result.Error = "checkout confirm response did not include a valid PayPal payment url"
	return result
}

func paypalOfficialPaymentURL(values ...any) string {
	for _, value := range values {
		if candidate := findPayPalURL(value); candidate != "" {
			return candidate
		}
	}
	return ""
}

func findPayPalURL(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, raw := range typed {
			if candidate := paypalURLFromString(stringifyJSONValue(raw)); candidate != "" {
				return candidate
			}
		}
		for _, raw := range typed {
			if candidate := findPayPalURL(raw); candidate != "" {
				return candidate
			}
		}
	case []any:
		for _, raw := range typed {
			if candidate := findPayPalURL(raw); candidate != "" {
				return candidate
			}
		}
	case string:
		return paypalURLFromString(typed)
	}
	return ""
}

func paypalURLFromString(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if parsed.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "paypal.com" || strings.HasSuffix(host, ".paypal.com") {
		return parsed.String()
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

func resolveLatestCheckoutRuntimeTarget(targets []cdpTarget) (*cdpTarget, string) {
	for i := len(targets) - 1; i >= 0; i-- {
		target := &targets[i]
		if target.Type != "page" {
			continue
		}
		urlText := strings.TrimSpace(target.URL)
		if urlText == "" {
			continue
		}
		if normalized := normalizeManagedCheckoutURL(urlText); normalized != "" {
			return target, normalized
		}
		if midtransRedirectionAccountID(urlText) != "" || isGopayCDPTargetURL(urlText) {
			return target, urlText
		}
	}
	return nil, ""
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

func handleGPTPlusRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req gptPlusRecordRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	email := strings.TrimSpace(req.EmailAddress)
	token := strings.TrimSpace(req.LuckMailToken)
	if email == "" {
		writeJSON(w, http.StatusOK, gptPlusRecordResponse{OK: false, Stage: "gptpls_record_failed", Error: "email_address is required"})
		return
	}
	if token == "" {
		writeJSON(w, http.StatusOK, gptPlusRecordResponse{OK: false, Stage: "gptpls_record_failed", Error: "luckmail_token is required"})
		return
	}
	if !strings.Contains(email, "@") {
		writeJSON(w, http.StatusOK, gptPlusRecordResponse{OK: false, Stage: "gptpls_record_failed", Error: "email_address is invalid"})
		return
	}
	now := time.Now()
	queryURL := "https://mails.luckyous.com/api/v1/email/query/" + url.PathEscape(token)
	codeViewURL := luckMailCodeViewURL(token)
	record := gptPlusRecordFile{
		OK:            true,
		Stage:         "gptpls_payment_success_recorded",
		SavedAt:       now.Format(time.RFC3339),
		EmailAddress:  email,
		LuckMailToken: token,
		QueryURL:      queryURL,
		CodeViewURL:   codeViewURL,
		PaymentStage:  strings.TrimSpace(req.PaymentStage),
		PaymentMethod: strings.TrimSpace(req.PaymentMethod),
		CheckoutURL:   strings.TrimSpace(req.CheckoutURL),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		writeJSON(w, http.StatusOK, gptPlusRecordResponse{OK: false, Stage: "gptpls_record_failed", Error: err.Error()})
		return
	}
	dir := filepath.Join(".", "gptpls")
	if err := os.MkdirAll(dir, 0700); err != nil {
		writeJSON(w, http.StatusOK, gptPlusRecordResponse{OK: false, Stage: "gptpls_record_failed", Error: err.Error()})
		return
	}
	fileName := fmt.Sprintf("%s_%s.json", sanitizeAuditFileName(email), now.Format("20060102_150405"))
	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, append(data, '\n'), 0600); err != nil {
		writeJSON(w, http.StatusOK, gptPlusRecordResponse{OK: false, Stage: "gptpls_record_failed", Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, gptPlusRecordResponse{
		OK:           true,
		Stage:        "gptpls_payment_success_recorded",
		EmailAddress: email,
		QueryURL:     queryURL,
		CodeViewURL:  codeViewURL,
		FilePath:     filePath,
	})
}

func handleLuckMailCreateAndWait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req luckMailCreateAndWaitRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	apiKey := luckMailAPIKey()
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "LUCKMAIL_API_KEY is required")
		return
	}

	projectCode := firstNonEmpty(req.ProjectCode, luckMailDefaultProjectCode())
	if projectCode == "" {
		writeError(w, http.StatusBadRequest, "project_code is required")
		return
	}
	emailType := firstNonEmpty(req.EmailType, luckMailDefaultEmailType())
	domain := firstNonEmpty(req.Domain, luckMailDefaultDomain())
	timeoutS := req.TimeoutS
	if timeoutS <= 0 {
		timeoutS = luckMailTimeoutSeconds()
	}
	if timeoutS <= 0 {
		timeoutS = 300
	}
	if timeoutS > 900 {
		timeoutS = 900
	}
	intervalS := req.IntervalS
	if intervalS <= 0 {
		intervalS = luckMailIntervalSeconds()
	}
	if intervalS <= 0 {
		intervalS = 3
	}
	if intervalS > 30 {
		intervalS = 30
	}

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutS+10)*time.Second)
	defer cancel()

	client := luckmail.New(apiKey, luckmail.WithBaseURL(luckMailBaseURL()))
	options := &luckmail.OrderOptions{
		EmailType:      emailType,
		Domain:         domain,
		SpecifiedEmail: req.SpecifiedEmail,
		VariantMode:    req.VariantMode,
		Timeout:        time.Duration(timeoutS) * time.Second,
		Interval:       time.Duration(intervalS) * time.Second,
	}
	order, err := client.User.CreateOrder(ctx, projectCode, options)
	if err != nil {
		writeJSON(w, http.StatusOK, luckMailCreateAndWaitResponse{
			OK:          false,
			Stage:       "luckmail_create_order_failed",
			Status:      "error",
			ProjectCode: projectCode,
			EmailType:   emailType,
			Domain:      domain,
			TimeoutS:    timeoutS,
			IntervalS:   intervalS,
			ElapsedMS:   time.Since(startedAt).Milliseconds(),
			Error:       err.Error(),
		})
		return
	}
	result, err := client.User.WaitForCode(ctx, order.OrderNo, options.Timeout, options.Interval, nil)
	elapsedMS := time.Since(startedAt).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, luckMailCreateAndWaitResponse{
			OK:           false,
			Stage:        "luckmail_create_and_wait_failed",
			Status:       "error",
			OrderNo:      order.OrderNo,
			EmailAddress: order.EmailAddress,
			ProjectCode:  projectCode,
			EmailType:    emailType,
			Domain:       domain,
			TimeoutS:     timeoutS,
			IntervalS:    intervalS,
			ElapsedMS:    elapsedMS,
			Error:        err.Error(),
		})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusOK, luckMailCreateAndWaitResponse{
			OK:           false,
			Stage:        "luckmail_empty_result",
			Status:       "empty",
			OrderNo:      order.OrderNo,
			EmailAddress: order.EmailAddress,
			ProjectCode:  projectCode,
			EmailType:    emailType,
			Domain:       domain,
			TimeoutS:     timeoutS,
			IntervalS:    intervalS,
			ElapsedMS:    elapsedMS,
			Error:        "empty luckmail response",
		})
		return
	}

	ok := strings.EqualFold(result.Status, "success") && strings.TrimSpace(result.VerificationCode) != ""
	stage := "luckmail_wait_finished"
	if ok {
		stage = "luckmail_code_received"
	}
	orderNo := firstNonEmpty(result.OrderNo, order.OrderNo)
	writeJSON(w, http.StatusOK, luckMailCreateAndWaitResponse{
		OK:               ok,
		Stage:            stage,
		Status:           result.Status,
		OrderNo:          orderNo,
		EmailAddress:     order.EmailAddress,
		VerificationCode: result.VerificationCode,
		MailFrom:         result.MailFrom,
		MailSubject:      result.MailSubject,
		MailBodyHTML:     result.MailBodyHTML,
		ProjectCode:      projectCode,
		EmailType:        emailType,
		Domain:           domain,
		TimeoutS:         timeoutS,
		IntervalS:        intervalS,
		ElapsedMS:        elapsedMS,
	})
}

func handleLuckMailTokenCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req luckMailTokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	apiKey := luckMailAPIKey()
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "LUCKMAIL_API_KEY is required")
		return
	}
	token := strings.TrimSpace(req.LuckMailToken)
	if token == "" {
		writeError(w, http.StatusBadRequest, "luckmail_token is required")
		return
	}

	timeoutS := req.TimeoutS
	if timeoutS <= 0 {
		timeoutS = luckMailTimeoutSeconds()
	}
	if timeoutS <= 0 {
		timeoutS = 300
	}
	if timeoutS > 900 {
		timeoutS = 900
	}
	intervalS := req.IntervalS
	if intervalS <= 0 {
		intervalS = luckMailIntervalSeconds()
	}
	if intervalS <= 0 {
		intervalS = 3
	}
	if intervalS > 30 {
		intervalS = 30
	}

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutS+10)*time.Second)
	defer cancel()
	client := luckmail.New(apiKey, luckmail.WithBaseURL(luckMailBaseURL()))
	result, err := waitForFreshLuckMailTokenCode(ctx, client, token, time.Duration(timeoutS)*time.Second, time.Duration(intervalS)*time.Second, req.SinceUnixMS)
	result = enrichLuckMailTokenCodeWithFreshMailCode(ctx, client, token, result, req.SinceUnixMS)
	elapsedMS := time.Since(startedAt).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, luckMailTokenCodeResponse{
			OK:          false,
			Stage:       "luckmail_token_code_failed",
			TimeoutS:    timeoutS,
			IntervalS:   intervalS,
			SinceUnixMS: req.SinceUnixMS,
			ElapsedMS:   elapsedMS,
			Error:       err.Error(),
		})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusOK, luckMailTokenCodeResponse{
			OK:          false,
			Stage:       "luckmail_token_code_empty",
			TimeoutS:    timeoutS,
			IntervalS:   intervalS,
			SinceUnixMS: req.SinceUnixMS,
			ElapsedMS:   elapsedMS,
			Error:       "empty luckmail token response",
		})
		return
	}

	ok := result.HasNewMail && strings.TrimSpace(result.VerificationCode) != ""
	stage := "luckmail_token_wait_finished"
	if ok {
		stage = "luckmail_token_code_received"
	}
	writeJSON(w, http.StatusOK, luckMailTokenCodeResponse{
		OK:               ok,
		Stage:            stage,
		EmailAddress:     result.EmailAddress,
		Project:          result.Project,
		HasNewMail:       result.HasNewMail,
		VerificationCode: result.VerificationCode,
		TimeoutS:         timeoutS,
		IntervalS:        intervalS,
		SinceUnixMS:      req.SinceUnixMS,
		ElapsedMS:        elapsedMS,
	})
}

func waitForFreshLuckMailTokenCode(ctx context.Context, client *luckmail.Client, token string, timeout time.Duration, interval time.Duration, sinceUnixMS int64) (*luckmail.TokenCode, error) {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}
	if sinceUnixMS <= 0 {
		return client.User.WaitForTokenCode(ctx, token, timeout, interval, nil)
	}
	deadline := time.Now().Add(timeout)
	var lastResult *luckmail.TokenCode
	for {
		result, err := client.User.GetTokenCode(ctx, token)
		if err != nil {
			return nil, err
		}
		lastResult = result
		if tokenCodeIsFresh(ctx, client, token, result, sinceUnixMS) {
			return result, nil
		}
		if time.Now().After(deadline) {
			return freshTimeoutTokenCode(lastResult), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func tokenCodeIsFresh(ctx context.Context, client *luckmail.Client, token string, result *luckmail.TokenCode, sinceUnixMS int64) bool {
	if result == nil || !result.HasNewMail || strings.TrimSpace(result.VerificationCode) == "" {
		return false
	}
	if sinceUnixMS <= 0 {
		return true
	}
	if len(result.Mail) > 0 {
		mailTime, ok := luckMailRawMessageTime(result.Mail)
		if ok {
			return mailTime.UnixMilli() >= sinceUnixMS-1500
		}
	}
	return tokenCodeMatchesFreshMail(ctx, client, token, result.VerificationCode, sinceUnixMS)
}

func freshTimeoutTokenCode(result *luckmail.TokenCode) *luckmail.TokenCode {
	if result == nil {
		return nil
	}
	return &luckmail.TokenCode{
		EmailAddress: result.EmailAddress,
		Project:      result.Project,
		HasNewMail:   false,
	}
}

func tokenCodeMatchesFreshMail(ctx context.Context, client *luckmail.Client, token string, code string, sinceUnixMS int64) bool {
	mails, err := client.User.GetTokenMails(ctx, token)
	if err != nil || mails == nil {
		return false
	}
	code = strings.TrimSpace(code)
	for _, mail := range mails.Mails {
		mailTime, ok := parseLuckMailTimeValue(mail.ReceivedAt)
		if ok && mailTime.UnixMilli() < sinceUnixMS-1500 {
			continue
		}
		if !ok {
			continue
		}
		if strings.Contains(mail.Subject, code) || strings.Contains(mail.Body, code) || strings.Contains(mail.HTMLBody, code) {
			return true
		}
		if strings.TrimSpace(mail.MessageID) == "" {
			continue
		}
		detail, err := client.User.GetTokenMailDetail(ctx, token, mail.MessageID)
		if err != nil || detail == nil {
			continue
		}
		if strings.TrimSpace(detail.VerificationCode) == code || strings.Contains(detail.BodyText, code) || strings.Contains(detail.BodyHTML, code) || strings.Contains(detail.Subject, code) {
			return true
		}
	}
	return false
}

func enrichLuckMailTokenCodeWithFreshMailCode(ctx context.Context, client *luckmail.Client, token string, result *luckmail.TokenCode, sinceUnixMS int64) *luckmail.TokenCode {
	if result == nil || strings.TrimSpace(result.VerificationCode) != "" {
		return result
	}
	if len(result.Mail) > 0 {
		if sinceUnixMS <= 0 || luckMailRawMessageIsFresh(result.Mail, sinceUnixMS) {
			if code := extractLuckMailVerificationCodeFromRaw(result.Mail); code != "" {
				copyResult := *result
				copyResult.HasNewMail = true
				copyResult.VerificationCode = code
				return &copyResult
			}
		}
	}
	mails, err := client.User.GetTokenMails(ctx, token)
	if err != nil || mails == nil {
		return result
	}
	for _, mail := range mails.Mails {
		mailTime, ok := parseLuckMailTimeValue(mail.ReceivedAt)
		if sinceUnixMS > 0 && (!ok || mailTime.UnixMilli() < sinceUnixMS-1500) {
			continue
		}
		if code := extractLuckMailVerificationCode(mail.Subject, mail.Body, mail.HTMLBody); code != "" {
			copyResult := *result
			copyResult.EmailAddress = firstNonEmpty(copyResult.EmailAddress, mails.EmailAddress)
			copyResult.Project = firstNonEmpty(copyResult.Project, mails.Project)
			copyResult.HasNewMail = true
			copyResult.VerificationCode = code
			return &copyResult
		}
		if strings.TrimSpace(mail.MessageID) == "" {
			continue
		}
		detail, err := client.User.GetTokenMailDetail(ctx, token, mail.MessageID)
		if err != nil || detail == nil {
			continue
		}
		if code := firstNonEmpty(strings.TrimSpace(detail.VerificationCode), extractLuckMailVerificationCode(detail.Subject, detail.BodyText, detail.BodyHTML)); code != "" {
			copyResult := *result
			copyResult.EmailAddress = firstNonEmpty(copyResult.EmailAddress, mails.EmailAddress)
			copyResult.Project = firstNonEmpty(copyResult.Project, mails.Project)
			copyResult.HasNewMail = true
			copyResult.VerificationCode = code
			return &copyResult
		}
	}
	return result
}

func luckMailRawMessageIsFresh(raw json.RawMessage, sinceUnixMS int64) bool {
	if sinceUnixMS <= 0 {
		return true
	}
	mailTime, ok := luckMailRawMessageTime(raw)
	return ok && mailTime.UnixMilli() >= sinceUnixMS-1500
}

func extractLuckMailVerificationCodeFromRaw(raw json.RawMessage) string {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return extractLuckMailVerificationCodeFromAny(payload)
}

func extractLuckMailVerificationCodeFromAny(value any) string {
	switch item := value.(type) {
	case string:
		return extractLuckMailVerificationCode(item)
	case map[string]any:
		priority := []string{"verification_code", "verificationCode", "code", "subject", "body", "body_text", "bodyText", "body_html", "bodyHTML", "html_body", "htmlBody", "text"}
		for _, key := range priority {
			if code := extractLuckMailVerificationCodeFromAny(item[key]); code != "" {
				return code
			}
		}
		for _, nested := range item {
			if code := extractLuckMailVerificationCodeFromAny(nested); code != "" {
				return code
			}
		}
	case []any:
		for _, nested := range item {
			if code := extractLuckMailVerificationCodeFromAny(nested); code != "" {
				return code
			}
		}
	case float64:
		if item >= 100000 && item <= 999999 && math.Trunc(item) == item {
			return strconv.FormatInt(int64(item), 10)
		}
	case int:
		if item >= 100000 && item <= 999999 {
			return strconv.Itoa(item)
		}
	case int64:
		if item >= 100000 && item <= 999999 {
			return strconv.FormatInt(item, 10)
		}
	}
	return ""
}

func extractLuckMailVerificationCode(parts ...string) string {
	for _, part := range parts {
		text := htmlTextToPlain(part)
		if text == "" {
			continue
		}
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`(?i)(?:code|verification|verify|otp|验证码|验证代码|安全代码)[^0-9]{0,40}([0-9]{6})`),
			regexp.MustCompile(`\b([0-9]{6})\b`),
		}
		for _, pattern := range patterns {
			matches := pattern.FindStringSubmatch(text)
			if len(matches) > 1 {
				return matches[1]
			}
		}
	}
	return ""
}

func htmlTextToPlain(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	text = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(text, " ")
	text = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`).Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

func luckMailRawMessageTime(raw json.RawMessage) (time.Time, bool) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return time.Time{}, false
	}
	return findLuckMailTimeValue(payload)
}

func findLuckMailTimeValue(value any) (time.Time, bool) {
	switch item := value.(type) {
	case map[string]any:
		for _, key := range []string{"received_at", "receivedAt", "date", "sent_at", "sentAt", "created_at", "createdAt", "time", "timestamp"} {
			if parsed, ok := parseLuckMailTimeValue(item[key]); ok {
				return parsed, true
			}
		}
		for _, nested := range item {
			if parsed, ok := findLuckMailTimeValue(nested); ok {
				return parsed, true
			}
		}
	case []any:
		for _, nested := range item {
			if parsed, ok := findLuckMailTimeValue(nested); ok {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func parseLuckMailTimeValue(value any) (time.Time, bool) {
	switch item := value.(type) {
	case string:
		text := strings.TrimSpace(item)
		if text == "" {
			return time.Time{}, false
		}
		layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006/01/02 15:04:05"}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed, true
			}
			if parsed, err := time.ParseInLocation(layout, text, time.Local); err == nil {
				return parsed, true
			}
		}
		if number, err := strconv.ParseInt(text, 10, 64); err == nil {
			return unixTimeFromLuckMailNumber(number)
		}
	case float64:
		return unixTimeFromLuckMailNumber(int64(item))
	case int64:
		return unixTimeFromLuckMailNumber(item)
	case int:
		return unixTimeFromLuckMailNumber(int64(item))
	}
	return time.Time{}, false
}

func unixTimeFromLuckMailNumber(value int64) (time.Time, bool) {
	if value <= 0 {
		return time.Time{}, false
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value), true
	}
	if value > 1_000_000_000 {
		return time.Unix(value, 0), true
	}
	return time.Time{}, false
}

func handleLuckMailTokenMails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req luckMailTokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	apiKey := luckMailAPIKey()
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "LUCKMAIL_API_KEY is required")
		return
	}
	token := strings.TrimSpace(req.LuckMailToken)
	if token == "" {
		writeError(w, http.StatusBadRequest, "luckmail_token is required")
		return
	}

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	client := luckmail.New(apiKey, luckmail.WithBaseURL(luckMailBaseURL()))
	result, err := client.User.GetTokenMails(ctx, token)
	elapsedMS := time.Since(startedAt).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, luckMailTokenMailsResponse{
			OK:        false,
			Stage:     "luckmail_token_mails_failed",
			Mails:     []luckMailTokenMailItemResponse{},
			ElapsedMS: elapsedMS,
			Error:     err.Error(),
		})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusOK, luckMailTokenMailsResponse{
			OK:        false,
			Stage:     "luckmail_token_mails_empty",
			Mails:     []luckMailTokenMailItemResponse{},
			ElapsedMS: elapsedMS,
			Error:     "empty luckmail token mails response",
		})
		return
	}

	mails := make([]luckMailTokenMailItemResponse, 0, len(result.Mails))
	for _, item := range result.Mails {
		mails = append(mails, luckMailTokenMailItemResponse{
			MessageID:  item.MessageID,
			From:       item.From,
			Subject:    item.Subject,
			ReceivedAt: item.ReceivedAt,
		})
	}
	writeJSON(w, http.StatusOK, luckMailTokenMailsResponse{
		OK:            true,
		Stage:         "luckmail_token_mails_loaded",
		EmailAddress:  result.EmailAddress,
		Project:       result.Project,
		WarrantyUntil: result.WarrantyUntil,
		Mails:         mails,
		ElapsedMS:     elapsedMS,
	})
}

func handleLuckMailPurchases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer r.Body.Close()
	var req luckMailPurchaseListRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	apiKey := luckMailAPIKey()
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "LUCKMAIL_API_KEY is required")
		return
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	params := &luckmail.GetPurchasesParams{
		Page:     page,
		PageSize: pageSize,
		Keyword:  strings.TrimSpace(req.Keyword),
	}
	if req.UserDisabled != nil {
		params.HasUserDisabled = true
		params.UserDisabled = *req.UserDisabled
	} else {
		params.HasUserDisabled = true
		params.UserDisabled = 0
	}
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	client := luckmail.New(apiKey, luckmail.WithBaseURL(luckMailBaseURL()))
	result, err := client.User.GetPurchases(ctx, params)
	elapsedMS := time.Since(startedAt).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, luckMailPurchasesResponse{
			OK:        false,
			Stage:     "luckmail_purchases_failed",
			List:      []luckMailPurchaseItemResponse{},
			Page:      page,
			PageSize:  pageSize,
			ElapsedMS: elapsedMS,
			Error:     err.Error(),
		})
		return
	}
	if result == nil {
		writeJSON(w, http.StatusOK, luckMailPurchasesResponse{
			OK:        false,
			Stage:     "luckmail_purchases_empty",
			List:      []luckMailPurchaseItemResponse{},
			Page:      page,
			PageSize:  pageSize,
			ElapsedMS: elapsedMS,
			Error:     "empty luckmail purchases response",
		})
		return
	}
	items := make([]luckMailPurchaseItemResponse, 0, len(result.List))
	for _, item := range result.List {
		items = append(items, luckMailPurchaseItemResponse{
			ID:            item.ID,
			EmailAddress:  item.EmailAddress,
			LuckMailToken: item.Token,
			ProjectName:   item.ProjectName,
			Status:        item.Status,
			TagName:       item.TagName,
			UserDisabled:  item.UserDisabled,
			WarrantyUntil: item.WarrantyUntil,
			CreatedAt:     item.CreatedAt,
		})
	}
	var selected *luckMailPurchaseItemResponse
	for i := range items {
		if strings.TrimSpace(items[i].EmailAddress) != "" && strings.TrimSpace(items[i].LuckMailToken) != "" && items[i].UserDisabled == 0 {
			selected = &items[i]
			break
		}
	}
	writeJSON(w, http.StatusOK, luckMailPurchasesResponse{
		OK:        true,
		Stage:     "luckmail_purchases_loaded",
		List:      items,
		Total:     result.Total,
		Page:      result.Page,
		PageSize:  result.PageSize,
		Selected:  selected,
		ElapsedMS: elapsedMS,
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
			RetryStillLinking:      false,
			UnboundedUntilNextStep: true,
		}
	}
	return midtransLinkingRetryProfile{
		ButtonWaitCycles:       8,
		PostClickWaitCycles:    18,
		PostClickWaitMs:        500,
		RecoveryWaitMs:         900,
		RetryStillLinking:      false,
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
		const loadingShellReloadKey = 'gopay_midtrans_loading_reload_' + result.account_id;
		const scheduleLoadingShellReload = (minIntervalMs = 30000) => {
			const cooldownMs = Number.isFinite(Number(minIntervalMs)) ? Number(minIntervalMs) : 30000;
			try {
				const last = Number(window.sessionStorage?.getItem(loadingShellReloadKey) || 0);
				if (Number.isFinite(last) && last > 0 && Date.now() - last < cooldownMs) return false;
				window.sessionStorage?.setItem(loadingShellReloadKey, String(Date.now()));
				window.setTimeout(() => window.location.reload(), 1200);
				return true;
			} catch (_) {
				window.setTimeout(() => window.location.reload(), 1200);
				return true;
			}
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
		const visibleInputs = () => Array.from(document.querySelectorAll('input')).filter((el) => visible(el) && el.type !== 'hidden');
		const visibleButtons = () => Array.from(document.querySelectorAll('button')).filter(visible);
		const snapshotPage = () => ({
			url: window.location.href,
			title: document.title,
			page_text_snippet: pageText().slice(0, 1000),
			input_count: visibleInputs().length,
			buttons: visibleButtons().map((button) => ({ text: textOf(button).slice(0, 80), disabled: !!button.disabled })).slice(0, 8),
		});
		const looksLikeBlankLoadingShell = () => {
			const href = window.location.href.toLowerCase();
			if (!href.includes('gopay-tokenization/linking')) return false;
			return pageText().length === 0 && visibleInputs().length === 0 && visibleButtons().length === 0;
		};
		const headerText = (headers) => {
			try { return JSON.stringify(headers || {}).toLowerCase(); } catch (_) { return String(headers || '').toLowerCase(); }
		};
		const hasRateLimit = () => networkDiagnostics.entries.some((entry) =>
			Number(entry.status) === 429 ||
			headerText(entry.response_headers).includes('x-envoy-ratelimited') ||
			headerText(entry.response_headers).includes('ratelimited')
		);
		const markCooldownState = (stage, retryAfterMs, options = {}) => {
			const waitMs = Number.isFinite(Number(retryAfterMs)) ? Number(retryAfterMs) : 30000;
			Object.assign(result, snapshotPage(), {
				stage,
				retry_after_ms: waitMs,
				cooldown_ms: waitMs,
				loading_shell: stage === 'midtrans_linking_blank_shell' || stage === 'midtrans_linking_loading_stuck',
				rate_limited: stage === 'midtrans_linking_rate_limited',
			});
			if (options.reload) {
				result.loading_shell_reload_scheduled = scheduleLoadingShellReload(waitMs);
			}
			return JSON.stringify(result);
		};
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
		for (let i = 0; i < 12 && looksLikeBlankLoadingShell(); i++) {
			await wait(500);
		}
		if (looksLikeBlankLoadingShell()) {
			return markCooldownState('midtrans_linking_blank_shell', 30000, { reload: true });
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
			if (looksLikeBlankLoadingShell()) {
				return markCooldownState('midtrans_linking_blank_shell', 30000, { reload: true });
			}
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
		const findTechnicalErrorBackButton = () => {
			const candidates = uniqueElements(Array.from(document.querySelectorAll('button, [role="button"], a, input[type="button"], input[type="submit"], div, span'))
				.filter(visible)
				.map((el) => el.closest?.('button, [role="button"], a, input[type="button"], input[type="submit"]') || el)
				.filter(visible));
			const scored = candidates.map((el) => {
				const text = textOf(el).trim();
				const normalized = text.toLowerCase();
				let score = -1;
				if (/^(back|kembali|try again|coba lagi)$/i.test(text)) score = 100;
				else if (/\b(back|kembali|try again|coba lagi)\b/i.test(text) && text.length <= 80) score = 70;
				if (score >= 0 && (el.tagName === 'BUTTON' || (el.getAttribute('role') || '').toLowerCase() === 'button')) score += 10;
				if (score >= 0 && normalized.includes('link and pay')) score -= 80;
				return { el, text, score };
			}).filter((item) => item.score >= 0).sort((a, b) => b.score - a.score || a.text.length - b.text.length);
			result.technical_error_back_candidates = scored.map((item) => ({ text: item.text.slice(0, 80), score: item.score })).slice(0, 6);
			return scored[0]?.el || null;
		};
		const clickBackFromTechnicalError = async () => {
			let backButton = null;
			for (let i = 0; i < 8 && !backButton; i++) {
				backButton = findTechnicalErrorBackButton();
				if (!backButton) await wait(250);
			}
			result.technical_error_recovery_attempts = (result.technical_error_recovery_attempts || 0) + 1;
			result.technical_error_back_button = backButton ? textOf(backButton).slice(0, 80) : '';
			if (!backButton) return false;
			clickElement(backButton);
			result.technical_error_back_clicked = true;
			await wait(recoveryWaitMs);
			for (let i = 0; i < 10; i++) {
				if (looksLikeLinkingPage() && !hasTechnicalError()) return true;
				await wait(500);
			}
			return looksLikeLinkingPage() && !hasTechnicalError();
		};
		const isSubmitActionButton = (button) => {
			const text = textOf(button).toLowerCase();
			return text.includes('link and pay') || text.includes('hubungkan') || text.includes('bayar') || text.includes('continue') || text.includes('lanjut') || text.includes('pay');
		};
		const isLoadingActionButton = (button) => {
			if (!button || isSubmitActionButton(button)) return false;
			const text = textOf(button);
			if (text) return false;
			return !!button.querySelector?.('.centerload, .load-dot, [class*="load" i], [class*="spinner" i]');
		};
		const findSubmitActionButton = () => Array.from(document.querySelectorAll('button')).filter(visible).find(isSubmitActionButton);
		const hasLoadingActionButton = () => Array.from(document.querySelectorAll('button')).filter(visible).some(isLoadingActionButton);

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
			let buttons = Array.from(document.querySelectorAll('button')).filter(visible);
			result.buttons = buttons.map((button) => ({ text: textOf(button).slice(0, 80), disabled: !!button.disabled })).slice(0, 8);
			let submitButton = buttons.find(isSubmitActionButton);
			if (!submitButton && hasLoadingActionButton()) {
				result.loading_action_button_detected = true;
				for (let i = 0; i < buttonWaitCycles * 2; i++) {
					await wait(500);
					if (hasNextStep() || hasTechnicalError()) break;
					submitButton = findSubmitActionButton();
					if (submitButton || !hasLoadingActionButton()) break;
				}
				buttons = Array.from(document.querySelectorAll('button')).filter(visible);
				result.buttons_after_loading_wait = buttons.map((button) => ({ text: textOf(button).slice(0, 80), disabled: !!button.disabled })).slice(0, 8);
			}
			if (!submitButton) {
				if (looksLikeBlankLoadingShell()) {
					result.stage = 'midtrans_linking_blank_shell';
					result.retry_after_ms = 30000;
					result.cooldown_ms = 30000;
					result.loading_shell_reload_scheduled = scheduleLoadingShellReload(30000);
					recordClickAttempt({ attempt, action: 'blank_loading_shell', reload_scheduled: result.loading_shell_reload_scheduled, url: window.location.href });
					break;
				}
				if (hasLoadingActionButton()) {
					result.stage = 'midtrans_linking_loading_stuck';
					result.retry_after_ms = 30000;
					result.cooldown_ms = 30000;
					result.loading_shell_reload_scheduled = scheduleLoadingShellReload(30000);
					recordClickAttempt({ attempt, action: 'loading_button', reload_scheduled: result.loading_shell_reload_scheduled, url: window.location.href, snippet: pageText().slice(0, 240) });
					break;
				}
				result.stage = 'midtrans_linking_button_missing';
				recordClickAttempt({ attempt, action: 'missing_button', url: window.location.href, snippet: pageText().slice(0, 240) });
				break;
			}
			result.found.button = true;
			result.button_text = textOf(submitButton).slice(0, 100);
			result.button_disabled = !!submitButton.disabled;
			if (submitButton.disabled) {
				for (let i = 0; i < buttonWaitCycles && submitButton.disabled; i++) {
					await wait(500);
					const refreshedButtons = Array.from(document.querySelectorAll('button')).filter(visible);
					submitButton = refreshedButtons.find(isSubmitActionButton) || submitButton;
				}
				result.button_disabled_after_wait = !!submitButton.disabled;
			}
			if (submitButton.disabled) {
				result.stage = 'midtrans_linking_button_disabled';
				recordClickAttempt({ attempt, action: 'button_disabled' });
				break;
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
			if (hasRateLimit()) {
				result.stage = 'midtrans_linking_rate_limited';
				result.retry_after_ms = 90000;
				result.cooldown_ms = 90000;
				recordClickAttempt({ attempt, action: 'rate_limited', retry_after_ms: result.retry_after_ms });
				break;
			}
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
			has_rate_limit: hasRateLimit(),
			entry_count: networkDiagnostics.entries.length,
			dropped_entries: networkDiagnostics.dropped_entries,
		};
		if (hasRateLimit()) {
			result.stage = 'midtrans_linking_rate_limited';
			result.retry_after_ms = result.retry_after_ms || 90000;
			result.cooldown_ms = result.cooldown_ms || 90000;
			result.rate_limited = true;
		} else if (hasTechnicalError()) {
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
		"cdp_target_id":    target.ID,
		"cdp_target_url":   target.URL,
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
	pinCands := gopayLinkingPINCandidates(req.PIN)
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
	pinCands := gopayLinkingPINCandidates(req.PIN)
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
		OTP         string `json:"otp,omitempty"`
		PIN         string `json:"pin,omitempty"`
		TargetID    string `json:"target_id,omitempty"`
		TargetURL   string `json:"target_url,omitempty"`
		AccountID   string `json:"account_id,omitempty"`
		CheckoutURL string `json:"checkout_url,omitempty"`
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

	scope := normalizeGopayCDPTargetScope(gopayCDPTargetScope{
		TargetID:    req.TargetID,
		TargetURL:   req.TargetURL,
		AccountID:   req.AccountID,
		CheckoutURL: req.CheckoutURL,
	})
	target, err := findBestGopayCDPTarget(cdpDebuggingPort, scope)
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
		"ok":                      true,
		"stage":                   firstNonEmpty(stringifyJSONValue(result["page_stage"]), "page_observed"),
		"selected_target_id":      target.ID,
		"selected_target_url":     target.URL,
		"selected_target_type":    target.Type,
		"selected_target_score":   gopayCDPTargetScore(*target) + gopayCDPTargetScopeScore(*target, scope),
		"target_scope":            map[string]any{"target_id": scope.TargetID, "target_url": scope.TargetURL, "account_id": scope.AccountID, "checkout_url": scope.CheckoutURL},
		"payment_completed":       boolMapValue(result, "payment_completed"),
		"payment_complete_reason": stringifyJSONValue(result["payment_complete_reason"]),
		"pin_stage":               stringifyJSONValue(result["pin_stage"]),
		"pin_auto_filled":         boolMapValue(result, "pin_auto_filled"),
		"pin_auto_submitted":      boolMapValue(result, "pin_auto_submitted"),
		"pin_input_strategy":      stringifyJSONValue(result["pin_input_strategy"]),
		"balance_amount":          result["balance_amount"],
		"balance_state":           stringifyJSONValue(result["balance_state"]),
		"hubungkan_auto_clicked":  boolMapValue(result, "hubungkan_auto_clicked"),
		"pay_now_auto_clicked":    boolMapValue(result, "pay_now_auto_clicked"),
		"auto_action_paused":      boolMapValue(result, "auto_action_paused"),
		"auto_action_stage":       stringifyJSONValue(result["auto_action_stage"]),
		"otp_manual_required":     boolMapValue(result, "otp_manual_required"),
		"has_otp_field":           boolMapValue(result, "has_otp_field"),
		"has_pin_field":           boolMapValue(result, "has_pin_field"),
		"cdp_url_host":            stringifyJSONValue(result["cdp_url_host"]),
		"cdp_url_path":            stringifyJSONValue(result["cdp_url_path"]),
		"result":                  result,
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
			payment_completed: false,
			payment_complete_reason: '',
			already_handled: false,
		};
		const visible = (el) => !!el && !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
		const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
		const normalizeText = (value) => String(value || '').replace(/\s+/g, ' ').trim();
		const collectElements = (selector, root = document, seen = new Set()) => {
			const out = [];
			const visit = (node) => {
				if (!node || seen.has(node)) return;
				seen.add(node);
				try {
					if (node.querySelectorAll) {
						node.querySelectorAll(selector).forEach((el) => out.push(el));
						node.querySelectorAll('*').forEach((el) => {
							if (el.shadowRoot) visit(el.shadowRoot);
						});
					}
				} catch (_err) {}
			};
			visit(root);
			return Array.from(new Set(out));
		};
		const hashText = (value) => {
			let hash = 5381;
			for (const ch of String(value || '')) {
				hash = ((hash << 5) + hash) ^ ch.charCodeAt(0);
				hash >>>= 0;
			}
			return hash.toString(36);
		};
		const pageText = normalizeText((document.body && document.body.innerText) || document.documentElement.innerText || '');
		const lowerText = pageText.toLowerCase();
		result.page_text_snippet = pageText.slice(0, 1000);
		const successTextPatterns = [
			'payment successful',
			'payment success',
			'payment completed',
			'payment complete',
			'transaction successful',
			'transaction success',
			'transaction completed',
			'purchase successful',
			'subscription active',
			'subscription activated',
			'terima kasih',
			'pembayaran berhasil',
			'transaksi berhasil',
			'berhasil dibayar',
			'berhasil bayar',
			'支付成功',
			'支付完成',
			'订阅成功',
			'已订阅'
		];
		const successURLPattern = /(?:success|complete|completed|finish|thank|receipt)/i.test(currentURL);
		const failureTextPattern = /(?:payment failed|transaction failed|pembayaran gagal|transaksi gagal|失败|错误|declined|cancelled|canceled)/i.test(lowerText);
		if (!failureTextPattern && (successURLPattern || successTextPatterns.some((pattern) => lowerText.includes(pattern)))) {
			result.payment_completed = true;
			result.payment_complete_reason = successURLPattern ? 'url_or_text_success' : 'text_success';
			result.page_stage = 'gopay_complete';
			return JSON.stringify(result);
		}
		const inputElements = () => collectElements('input, textarea').filter((el) => visible(el) && el.type !== 'hidden');
		const actionElements = () => collectElements('button, [role="button"], input[type="button"], input[type="submit"], a').filter(visible);
		const allInputs = inputElements();
		const allActionElements = actionElements();
		const metaText = (el) => normalizeText([
			el?.type,
			el?.inputMode,
			el?.name,
			el?.id,
			el?.placeholder,
			el?.autocomplete,
			el?.getAttribute?.('aria-label'),
			el?.getAttribute?.('data-testid'),
			el?.getAttribute?.('data-test'),
			Array.from(el?.labels || []).map((label) => label.innerText || label.textContent || '').join(' '),
			el?.closest?.('label')?.innerText
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
			if (!el) return;
			try { el.focus(); } catch (_err) {}
			try { el.dispatchEvent(new FocusEvent('focus', { bubbles: true })); } catch (_err) {}
			try { el.dispatchEvent(new InputEvent('beforeinput', { bubbles: true, cancelable: true, inputType: 'insertReplacementText', data: String(value || '') })); } catch (_err) {}
			const proto = el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLElement.prototype;
			const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
			if (setter) {
				setter.call(el, value);
			} else {
				el.value = value;
			}
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new KeyboardEvent('keyup', { key: String(value || '').slice(-1) || '0', bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));
		};
		const typeDigit = (el, digit) => {
			if (!el) return;
			try { el.focus(); } catch (_err) {}
			try { el.dispatchEvent(new KeyboardEvent('keydown', { key: digit, code: 'Digit' + digit, bubbles: true, cancelable: true })); } catch (_err) {}
			try { el.dispatchEvent(new KeyboardEvent('keypress', { key: digit, code: 'Digit' + digit, bubbles: true, cancelable: true })); } catch (_err) {}
			try { el.dispatchEvent(new InputEvent('beforeinput', { bubbles: true, cancelable: true, inputType: 'insertText', data: digit })); } catch (_err) {}
			setNativeValue(el, digit);
			try { el.dispatchEvent(new KeyboardEvent('keyup', { key: digit, code: 'Digit' + digit, bubbles: true })); } catch (_err) {}
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
		const pinPageBySignal = urlHost.includes('pin-web-client.gopayapi.com') || lowerText.includes('pin kamu') || lowerText.includes('masukkan pin') || lowerText.includes('masukin pin') || lowerText.includes('enter your pin');
		const pinKeyword = (meta) => meta.includes('pin') || meta.includes('passcode') || meta.includes('security code') || meta.includes('kode keamanan') || meta.includes('sandi');
		const otpKeyword = (meta) => meta.includes('otp') || meta.includes('kode') || meta.includes('verification code');
		const pinInputs = allInputs.filter((el) => {
			const meta = metaText(el);
			if (pinKeyword(meta)) return true;
			if (otpKeyword(meta)) return false;
			if (el.type === 'password') return true;
			if ((el.inputMode || '').toLowerCase() === 'numeric' && Number(el.maxLength || 0) === 1) return true;
			if ((el.inputMode || '').toLowerCase() === 'numeric' && [4, 6, 8].includes(Number(el.maxLength || 0))) return true;
			if (pinPageBySignal && /^(tel|number|text|password)?$/i.test(el.type || '') && /numeric|decimal|tel/i.test(el.inputMode || '')) return true;
			return false;
		});
		result.has_pin_field = pinInputs.length > 0;
		result.pin_input_candidates = pinInputs.map((el) => ({ type: el.type, inputMode: el.inputMode, maxLength: el.maxLength, name: el.name, id: el.id, autocomplete: el.autocomplete })).slice(0, 8);

		const otpPage = urlPath.includes('/linking/otp') || lowerText.includes('otp') || lowerText.includes('verification code');
		const pinPage = pinPageBySignal;
		const paymentPath = urlPath.includes('/payment/validate-pin') || urlPath.includes('/payment/pin');
		const bindingPath = urlPath.includes('/auth/pin/verify') || urlPath.includes('/linking/');
		const paymentContext = paymentPath || lowerText.includes('payment') || lowerText.includes('bayar') || lowerText.includes('pembayaran') || lowerText.includes('total') || lowerText.includes('subscribe') || lowerText.includes('subscription');
		const bindingContext = bindingPath || lowerText.includes('link') || lowerText.includes('hubungkan') || lowerText.includes('authorize') || lowerText.includes('otorisasi');

		if (otpPage) {
			result.page_stage = 'otp_entry';
			result.otp_manual_required = true;
		} else if (pinPage) {
			if (paymentPath) {
				result.pin_stage = 'payment';
			} else if (bindingPath) {
				result.pin_stage = 'binding';
			} else {
				result.pin_stage = paymentContext && !bindingContext ? 'payment' : 'binding';
			}
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

		const pinPageSignature = hashText(currentURL + '|' + result.page_stage + '|' + pageText.slice(0, 320));
		result.pin_page_signature = pinPageSignature;
		const handledKey = 'gopay_cdp_handled_' + result.page_stage + '_' + actionScope + '_' + pinPageSignature;
		result.already_handled = storageGet(handledKey);

		const singlePinInput = pinInputs.find((el) => {
			const maxLength = Number(el.maxLength || 0);
			return maxLength !== 1 && (maxLength >= 6 || maxLength === 0 || el.type === 'password' || (pinPage && pinInputs.length === 1));
		});
		const splitPinInputs = pinInputs.filter((el) => Number(el.maxLength || 0) === 1).slice(0, 6);
		const digitButtons = () => actionElements().filter((button) => /^[0-9]$/.test(normalizeText(elementLabel(button))));
		const submitPIN = async (activeInput) => {
			await wait(300);
			const actionButton = actionElements().find((button) => {
				const text = normalizeText(elementLabel(button)).toLowerCase();
				return text.includes('verify') || text.includes('continue') || text.includes('lanjut') || text.includes('lanjutkan') || text.includes('confirm') || text.includes('konfirmasi') || text.includes('pay') || text.includes('bayar') || text.includes('submit') || text.includes('selesai') || text.includes('oke') || text === 'ok';
			});
			if (actionButton && !isDisabled(actionButton)) {
				clickElement(actionButton);
				return true;
			}
			if (singlePinInput && typeof singlePinInput.form?.requestSubmit === 'function') {
				try { singlePinInput.form.requestSubmit(); return true; } catch (_err) {}
			}
			if (activeInput) {
				try {
					activeInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', bubbles: true }));
					activeInput.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', code: 'Enter', bubbles: true }));
					activeInput.dispatchEvent(new Event('change', { bubbles: true }));
					return true;
				} catch (_err) {}
			}
			return false;
		};
		if (!result.auto_action_paused && result.page_stage.startsWith('pin_entry_') && preferredPin && preferredPin.length === 6 && !result.already_handled) {
			if (singlePinInput) {
				setNativeValue(singlePinInput, preferredPin);
				result.pin_auto_filled = true;
				result.pin_input_strategy = 'single_input';
			} else if (splitPinInputs.length >= 6) {
				for (const [index, digit] of preferredPin.split('').slice(0, 6).entries()) {
					typeDigit(splitPinInputs[index], digit);
					await wait(45);
				}
				result.pin_auto_filled = true;
				result.pin_input_strategy = 'split_inputs';
			} else {
				const keypad = digitButtons();
				if (keypad.length >= 10 || preferredPin.split('').every((digit) => keypad.some((button) => normalizeText(elementLabel(button)) === digit))) {
					for (const digit of preferredPin.split('').slice(0, 6)) {
						const button = digitButtons().find((item) => normalizeText(elementLabel(item)) === digit);
						if (!button || !clickElement(button)) {
							result.pin_keypad_missing_digit = digit;
							break;
						}
						await wait(80);
					}
					if (!result.pin_keypad_missing_digit) {
						result.pin_auto_filled = true;
						result.pin_input_strategy = 'virtual_keypad';
					}
				}
			}
			if (result.pin_auto_filled) {
				const activeInput = splitPinInputs.length >= 6 ? splitPinInputs[splitPinInputs.length - 1] : singlePinInput;
				result.pin_auto_submitted = await submitPIN(activeInput);
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
	pinCands := gopayLinkingPINCandidates(req.PIN)
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
	if !isStripeFrame && checkoutProbeShowsGopayChoice(probe) {
		return "activate_gopay"
	}
	if hasFillableCheckoutInputs(probe) {
		return "fill"
	}
	return "manual"
}

func checkoutShouldReturnToPageTargetAfterActivation(activated bool, targetType string, beforeProbe map[string]any, afterProbe map[string]any) bool {
	return activated &&
		strings.EqualFold(strings.TrimSpace(targetType), "iframe") &&
		hasFillableCheckoutInputs(beforeProbe) &&
		!hasFillableCheckoutInputs(afterProbe)
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
		if fallbackTarget, fallbackURL := resolveLatestCheckoutRuntimeTarget(targets); fallbackTarget != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":             true,
				"opened_url":     req.OpenedURL,
				"current_url":    fallbackURL,
				"fallback":       true,
				"fallback_error": err.Error(),
				"candidate_urls": checkoutResolveCandidateURLs(targets),
				"target": map[string]any{
					"id":   fallbackTarget.ID,
					"type": fallbackTarget.Type,
					"url":  fallbackTarget.URL,
				},
			})
			return
		}
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
		"address":                            addr,
		"expected_url":                       req.ExpectedURL,
		"current_url":                        canonicalURL,
		"safe_checkout_assist":               true,
		"manual_confirmation_required":       true,
		"requires_manual_terms_confirmation": true,
		"requires_manual_subscription_click": true,
		"terms_confirmation_auto_clicked":    false,
		"subscription_submit_auto_clicked":   false,
		"gopay_selected":                     false,
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
	initialProbe := probe
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
		response["gopay_selected"] = boolMapValue(clickMap, "clicked")
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
		if checkoutShouldReturnToPageTargetAfterActivation(boolMapValue(clickMap, "clicked"), target.Type, initialProbe, probe) {
			response["blank_iframe_fallback"] = true
			target = pageTarget
			canonicalFillURL = canonicalURL
			response["fill_target"] = map[string]any{
				"id":   target.ID,
				"type": target.Type,
				"url":  target.URL,
			}
			conn.Close()
			conn, _, err = websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "CDP failed: "+err.Error())
				return
			}
			defer conn.Close()
			sendCDPCommand(conn, "Runtime.enable", nil)
			isStripeFrame = false
			probeR, _ = executeCDPScript(conn, probeScript)
			probe = map[string]any{}
			if probeR != "" {
				json.Unmarshal([]byte(probeR), &probe)
			}
			response["probe_after_blank_iframe_fallback"] = probe
		}
		if boolMapValue(clickMap, "clicked") && hasFillableCheckoutInputs(probe) {
			action = "fill"
		} else {
			action = checkoutAutoFillAction(probe, isStripeFrame)
		}
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
	fillR, _ := executeCDPScript(conn, fmt.Sprintf(`(async () => {
		function snv(el,v){
			const s=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set;
			if(s){s.call(el,v)}else{el.value=v}
			el.dispatchEvent(new Event('input',{bubbles:true}));
			el.dispatchEvent(new Event('change',{bubbles:true}));
			el.dispatchEvent(new KeyboardEvent('keyup',{bubbles:true,key:String(v||'').slice(-1)||'1'}));
		}
		function so(sel,v){
			const vals=Array.isArray(v)?v:[v];
			const o=Array.from(sel.options).find(o=>vals.some(val=>o.value===val||o.text===val||o.text.includes(val)));
			if(o){sel.value=o.value;sel.dispatchEvent(new Event('change',{bubbles:true}));return true}
			return false
		}
		function norm(value){return String(value||'').replace(/\s+/g,' ').trim()}
		function mf(el,ks){
			const s=(el.name+'|'+el.id+'|'+el.autocomplete+'|'+(el.placeholder||'')+'|'+(el.getAttribute('aria-label')||'')).toLowerCase();
			return ks.some(k=>s.includes(k))
		}
		const a={fn:%q,ln:%q,l1:%q,c:%q,s:%q,sn:%q,z:%q};
		const ins=Array.from(document.querySelectorAll('input')).filter(el=>el.type!=='hidden'&&el.type!=='checkbox'&&el.type!=='radio');
		const sls=Array.from(document.querySelectorAll('select'));
		let f={};
		const fi=ins.find(el=>mf(el,['first','given','fname','firstname','first_name','vorname']));
		if(fi){snv(fi,a.fn);f.first_name=true}else{
			const ni=ins.find(el=>mf(el,['fullname','full_name','name']));
			if(ni){snv(ni,a.fn+' '+a.ln);f.full_name=true}
		}
		const li=ins.find(el=>mf(el,['last','family','lname','lastname','surname','nachname']));
		if(li){snv(li,a.ln);f.last_name=true}
		const ai=ins.find(el=>mf(el,['address-line1','address1','address','street','addr1','line1']));
		if(ai){snv(ai,a.l1);f.address=true}
		const ci=ins.find(el=>mf(el,['city','town','locality','address-level2']));
		if(ci){snv(ci,a.c);f.city=true}
		const zi=ins.find(el=>mf(el,['zip','postal','postcode','postal_code','zip_code']));
		if(zi){snv(zi,a.z);f.zip=true}
		const cs=sls.find(el=>mf(el,['country']));
		if(cs){f.country=so(cs,'US'); await new Promise(r=>setTimeout(r,700))}
		const si=ins.find(el=>mf(el,['state','region','province','administrative','address-level1']));
		const ss=sls.find(el=>mf(el,['state','region','province','administrative','address-level1']));
		if(ss){f.state=so(ss,[a.s,a.sn]); if(!f.state){await new Promise(r=>setTimeout(r,300)); f.state=so(ss,[a.s,a.sn])}}else if(si){snv(si,a.s);f.state=true}
		await new Promise(r=>setTimeout(r,600));
		const required=['address','city','state','zip'];
		if(cs) required.push('country');
		const validation={
			ok:false,
			required_fields:required,
			missing_required:required.filter(k=>!f[k]),
			filled_fields:f,
			manual_terms_required:Array.from(document.querySelectorAll('input[type="checkbox"]')).some(el=>norm(el.closest('label')?.innerText||el.parentElement?.innerText||el.getAttribute('aria-label')).length>0),
			manual_subscription_required:Array.from(document.querySelectorAll('button')).some(btn=>/subscribe|订阅/i.test(norm(btn.textContent))),
		};
		validation.ok=validation.missing_required.length===0;
		return JSON.stringify({url:window.location.href,filled:f,inputCount:ins.length,validation:validation});
	})()`, addr.FirstName, addr.LastName, addr.Line1, addr.City, addr.State, checkoutStateSelectValue(addr.State), addr.ZipCode))
	var fillMap map[string]any
	if fillR != "" {
		json.Unmarshal([]byte(fillR), &fillMap)
	}
	response["filled"] = fillMap
	validationOK := true
	if fillMap != nil {
		if validation, _ := fillMap["validation"].(map[string]any); validation != nil {
			response["address_validation"] = validation
			validationOK = boolMapValue(validation, "ok")
		}
	}
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
	if !validationOK {
		response["ok"] = false
		response["error"] = "地址字段已写入，但必填地址校验未通过。"
		writeJSON(w, http.StatusOK, response)
		return
	}
	response["ok"] = true
	response["submitted"] = false
	writeJSON(w, http.StatusOK, response)
}
