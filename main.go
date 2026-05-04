package main

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type appConfig struct {
	CheckoutEndpoint             string   `json:"checkout_endpoint"`
	CheckoutApproveEndpoint      string   `json:"checkout_approve_endpoint"`
	CheckoutCookie               string   `json:"checkout_cookie"`
	CheckoutUserAgent            string   `json:"checkout_user_agent"`
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
	mux.Handle("/", noCache(http.FileServer(http.FS(staticFiles))))

	server := &http.Server{
		Addr:              "127.0.0.1:18473",
		Handler:           securityHeaders(mux),
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
	if token := findStringField(result, "token"); token != "" {
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
	return map[string]any{
		"ok":           result.OK,
		"stage":        result.Stage,
		"status":       result.Status,
		"content_type": result.ContentType,
		"elapsed_ms":   result.ElapsedMS,
		"error":        result.Error,
	}
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
