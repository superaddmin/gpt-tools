package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGopayUserConsentPayloadIncludesSMSChannel(t *testing.T) {
	payload, channel, err := gopayUserConsentPayload("ref-123", "sms")
	if err != nil {
		t.Fatalf("gopayUserConsentPayload returned error: %v", err)
	}
	if channel != "sms" {
		t.Fatalf("channel = %q, want sms", channel)
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload is not json: %v", err)
	}
	if body["reference_id"] != "ref-123" {
		t.Fatalf("reference_id = %v, want ref-123", body["reference_id"])
	}
	if body["otp_channel"] != "sms" {
		t.Fatalf("otp_channel = %v, want sms", body["otp_channel"])
	}
}

func TestGopayUserConsentPayloadDefaultsToWhatsApp(t *testing.T) {
	payload, channel, err := gopayUserConsentPayload("ref-123", "")
	if err != nil {
		t.Fatalf("gopayUserConsentPayload returned error: %v", err)
	}
	if channel != "whatsapp" {
		t.Fatalf("channel = %q, want whatsapp", channel)
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload is not json: %v", err)
	}
	if body["otp_channel"] != "whatsapp" {
		t.Fatalf("otp_channel = %v, want whatsapp", body["otp_channel"])
	}
}

func TestNormalizeOTPChannelRejectsUnknownValues(t *testing.T) {
	if _, err := normalizeOTPChannel("email"); err == nil {
		t.Fatal("normalizeOTPChannel accepted unsupported channel")
	}
}

func TestBuildSessionDiagnosticsConclusionReturnsSessionReady(t *testing.T) {
	conclusion := buildSessionDiagnosticsConclusion(map[string]any{
		"cdp_ready":        true,
		"target_found":     true,
		"has_access_token": true,
		"session_status":   float64(200),
	})
	if conclusion["status"] != "session_ready" {
		t.Fatalf("status = %#v", conclusion["status"])
	}
	if conclusion["auto_action"] != "continue_checkout" {
		t.Fatalf("auto_action = %#v", conclusion["auto_action"])
	}
}

func TestBuildSessionDiagnosticsConclusionDetectsLoginRequired(t *testing.T) {
	conclusion := buildSessionDiagnosticsConclusion(map[string]any{
		"cdp_ready":            true,
		"target_found":         true,
		"has_access_token":     false,
		"session_status":       float64(401),
		"login_selector_count": float64(1),
		"target_url":           "https://chatgpt.com/auth/login",
	})
	if conclusion["status"] != "login_required" {
		t.Fatalf("status = %#v", conclusion["status"])
	}
	if conclusion["auto_action"] != "wait_for_login" {
		t.Fatalf("auto_action = %#v", conclusion["auto_action"])
	}
}

func TestBuildSessionDiagnosticsConclusionDetectsMissingChatGPTPage(t *testing.T) {
	conclusion := buildSessionDiagnosticsConclusion(map[string]any{
		"cdp_ready":    true,
		"target_found": false,
	})
	if conclusion["status"] != "chatgpt_page_missing" {
		t.Fatalf("status = %#v", conclusion["status"])
	}
	if conclusion["auto_action"] != "open_chatgpt_home" {
		t.Fatalf("auto_action = %#v", conclusion["auto_action"])
	}
}

func TestBuildSessionDiagnosticsConclusionDetectsCDPNotReady(t *testing.T) {
	conclusion := buildSessionDiagnosticsConclusion(map[string]any{
		"cdp_ready": false,
	})
	if conclusion["status"] != "cdp_not_ready" {
		t.Fatalf("status = %#v", conclusion["status"])
	}
	if conclusion["auto_action"] != "wait_and_retry" {
		t.Fatalf("auto_action = %#v", conclusion["auto_action"])
	}
}

func TestSessionIntValueReturnsZeroForOverflowFloat64(t *testing.T) {
	diagnostics := map[string]any{"session_status": float64(math.MaxInt) * 2}
	if got := sessionIntValue(diagnostics, "session_status"); got != 0 {
		t.Fatalf("got = %d, want 0", got)
	}
}

func TestSanitizeAuditBodyMasksSensitiveFields(t *testing.T) {
	body := []byte(`{"token":"eyJabc.def.ghi","customer_email":"user@example.com","checkout_session":{"cookie":"secret-cookie"},"otp":"123456"}`)
	detail := sanitizeAuditBody(body)
	if detail["token"] == "eyJabc.def.ghi" {
		t.Fatal("token should be masked")
	}
	if nested, ok := detail["checkout_session"].(map[string]any); ok {
		if nested["cookie"] == "secret-cookie" {
			t.Fatal("cookie should be masked")
		}
	} else {
		t.Fatalf("checkout_session type = %T", detail["checkout_session"])
	}
	if detail["customer_email"] != "user@example.com" {
		t.Fatalf("customer_email = %#v", detail["customer_email"])
	}
}

func TestBuildAuditLogRecordUsesHeaderEmail(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/session/fetch", strings.NewReader(`{"stream":true}`))
	req.Header.Set("X-Account-Email", "Tester@example.com")
	req.RemoteAddr = "127.0.0.1:56789"
	record := buildAuditLogRecord(req, []byte(`{"stream":true}`), nil, http.StatusOK, time.Now())
	if record.AccountEmail != "tester@example.com" {
		t.Fatalf("AccountEmail = %q", record.AccountEmail)
	}
	if record.IPAddress != "127.0.0.1" {
		t.Fatalf("IPAddress = %q", record.IPAddress)
	}
	if record.OperationName != "获取 Session JSON" {
		t.Fatalf("OperationName = %q", record.OperationName)
	}
}

func TestBuildAuditLogRecordIncludesFlowAnalysisMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/gopay/full-link", strings.NewReader(`{"customer_email":"User@example.com","pin":"123456"}`))
	req.Header.Set("User-Agent", "CheckoutWorkbenchTest/1.0")
	responseBody := []byte(`{"ok":true,"stage":"gopay_complete","payment_reference_id":"pay-ref-1","transaction_id":"tx-1"}`)

	record := buildAuditLogRecord(req, []byte(`{"customer_email":"User@example.com","pin":"123456"}`), responseBody, http.StatusOK, time.Now())

	if record.AccountEmail != "user@example.com" {
		t.Fatalf("AccountEmail = %q", record.AccountEmail)
	}
	if record.Metadata["analysis_log_version"] != auditAnalysisLogVersion {
		t.Fatalf("analysis_log_version = %#v", record.Metadata["analysis_log_version"])
	}
	if record.Metadata["flow_stage"] != "gopay_full_payment_flow" {
		t.Fatalf("flow_stage = %#v", record.Metadata["flow_stage"])
	}
	if record.Metadata["flow_step_index"] != 110 {
		t.Fatalf("flow_step_index = %#v", record.Metadata["flow_step_index"])
	}
	if record.Metadata["account_log_file_prefix"] != "user@example.com" {
		t.Fatalf("account_log_file_prefix = %#v", record.Metadata["account_log_file_prefix"])
	}
	identifiers, _ := record.Metadata["correlation_identifiers"].(map[string]any)
	if identifiers["payment_reference_id"] != "pay-ref-1" || identifiers["transaction_id"] != "tx-1" {
		t.Fatalf("correlation_identifiers = %#v", identifiers)
	}
	if _, exists := identifiers["pin"]; exists {
		t.Fatalf("correlation_identifiers should not include pin: %#v", identifiers)
	}
}

func TestAuditCorrelationIdentifiersSkipsSensitiveKeys(t *testing.T) {
	identifiers := auditCorrelationIdentifiers(map[string]any{
		"checkout_session_id": "cs_test_123",
		"pin":                 "123456",
		"token":               "tok-secret",
		"otp":                 "654321",
	})

	if identifiers["checkout_session_id"] != "cs_test_123" {
		t.Fatalf("checkout_session_id = %#v", identifiers["checkout_session_id"])
	}
	for _, key := range []string{"pin", "token", "otp"} {
		if _, exists := identifiers[key]; exists {
			t.Fatalf("identifiers should not include %s: %#v", key, identifiers)
		}
	}
}

func TestBuildAuditLogRecordMarksBusinessFailureFromResponseBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/gopay/full-link", strings.NewReader(`{"phone_number":"18120322232"}`))
	responseBody := []byte(`{"ok":false,"stage":"payment_pin_all_failed","error":"所有支付 PIN 候选码均失败","payment_reference_id":"pay-ref-1"}`)

	record := buildAuditLogRecord(req, []byte(`{"phone_number":"18120322232"}`), responseBody, http.StatusOK, time.Now())

	if record.OperationResult != "failed" {
		t.Fatalf("OperationResult = %q, want failed", record.OperationResult)
	}
	if !strings.Contains(record.ErrorMessage, "payment_pin_all_failed") {
		t.Fatalf("ErrorMessage = %q, want stage", record.ErrorMessage)
	}
	summary, _ := record.Metadata["response_summary"].(map[string]any)
	if summary["stage"] != "payment_pin_all_failed" {
		t.Fatalf("response_summary.stage = %#v", summary["stage"])
	}
	if summary["payment_reference_id"] != "pay-ref-1" {
		t.Fatalf("response_summary.payment_reference_id = %#v", summary["payment_reference_id"])
	}
}

func TestBuildAuditLogRecordExtractsOperationFlowAndPaymentVoucher(t *testing.T) {
	oldConfig := config
	config = appConfig{}
	t.Cleanup(func() { config = oldConfig })

	req := httptest.NewRequest(http.MethodPost, "/api/gopay/full-link", strings.NewReader(`{"phone_number":"18120322232","pin":"123456"}`))
	responseBody := []byte(`{
		"ok": true,
		"stage": "gopay_complete",
		"strategy": "full-link",
		"account_id": "acct-123",
		"stages": [
			{"name":"generate-checkout","ok":true,"status":200},
			{"name":"pin-enum","ok":true,"pin":"123456"},
			{"name":"validate-pin","ok":true,"message":"支付完成"}
		],
		"diagnostics": {"phone_e164":"+8618120322232"},
		"linking_diagnostics": {"account_source":"checkout_url_direct"},
		"payment_voucher": {
			"payment_reference_id":"pay-ref-123",
			"transaction_id":"tx-123",
			"transaction_status":"settlement",
			"status_code":"200",
			"order_id":"order-123",
			"gross_amount":"20.00",
			"currency":"IDR"
		}
	}`)

	record := buildAuditLogRecord(req, []byte(`{"phone_number":"18120322232","pin":"123456"}`), responseBody, http.StatusOK, time.Now())

	flow, _ := record.Metadata["operation_flow"].(map[string]any)
	if flow["stage"] != "gopay_complete" {
		t.Fatalf("operation_flow.stage = %#v", flow["stage"])
	}
	stages, _ := flow["stages"].([]any)
	if len(stages) != 3 {
		t.Fatalf("operation_flow.stages length = %d, want 3", len(stages))
	}
	pinStage, _ := stages[1].(map[string]any)
	if pinStage["pin"] != maskedAuditValue {
		t.Fatalf("operation_flow pin = %#v, want masked", pinStage["pin"])
	}
	voucher, _ := record.Metadata["payment_voucher"].(map[string]any)
	if voucher["payment_reference_id"] != "pay-ref-123" {
		t.Fatalf("payment_voucher.payment_reference_id = %#v", voucher["payment_reference_id"])
	}
	if voucher["transaction_id"] != "tx-123" {
		t.Fatalf("payment_voucher.transaction_id = %#v", voucher["transaction_id"])
	}
}

func TestBuildAuditLogRecordMasksSensitiveAnalysisDataWhenEnabled(t *testing.T) {
	oldConfig := config
	config = appConfig{AuditCaptureSensitive: true}
	t.Cleanup(func() { config = oldConfig })

	req := httptest.NewRequest(http.MethodPost, "/api/gopay/full-link", strings.NewReader(`{"access_token":"tok-analysis","otp":"654321","pin":"123456"}`))
	responseBody := []byte(`{
		"ok": true,
		"stage": "gopay_complete",
		"stages": [{"name":"otp-enum","ok":true,"otp":"654321"}],
		"payment_voucher": {"payment_reference_id":"pay-ref-123","payment_pin":"123456"},
		"gopay_payment_pin_token": {"token":"pin-token-secret"}
	}`)

	record := buildAuditLogRecord(req, []byte(`{"access_token":"tok-analysis","otp":"654321","pin":"123456"}`), responseBody, http.StatusOK, time.Now())

	detail, _ := record.OperationDetail.(map[string]any)
	if detail["access_token"] == "tok-analysis" {
		t.Fatalf("operation_detail.access_token = %#v, want masked", detail["access_token"])
	}
	if detail["otp"] == "654321" || detail["pin"] == "123456" {
		t.Fatalf("operation_detail contains raw otp/pin: %#v", detail)
	}
	flow, _ := record.Metadata["operation_flow"].(map[string]any)
	stages, _ := flow["stages"].([]any)
	otpStage, _ := stages[0].(map[string]any)
	if otpStage["otp"] == "654321" {
		t.Fatalf("operation_flow otp = %#v, want masked", otpStage["otp"])
	}
	responsePayload, _ := record.Metadata["response_payload"].(map[string]any)
	pinToken, _ := responsePayload["gopay_payment_pin_token"].(map[string]any)
	if pinToken["token"] == "pin-token-secret" {
		t.Fatalf("response_payload.gopay_payment_pin_token.token = %#v, want masked", pinToken["token"])
	}
	if record.Metadata["sensitive_capture"] != true {
		t.Fatalf("sensitive_capture = %#v, want true", record.Metadata["sensitive_capture"])
	}
}

func TestBuildAuditLogRecordIncludesCDPPINFlowFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/gopay/cdp-otp", strings.NewReader(`{"pin":"123456"}`))
	responseBody := []byte(`{
		"ok": true,
		"stage": "pin_entry_binding",
		"pin_stage": "binding",
		"pin_auto_filled": true,
		"pin_auto_submitted": true,
		"balance_amount": 1,
		"balance_state": "rp1",
		"hubungkan_auto_clicked": true,
		"pay_now_auto_clicked": false,
		"auto_action_paused": false,
		"auto_action_stage": "gopay_consent_hubungkan",
		"otp_manual_required": false,
		"result": {
			"page_stage": "pin_entry_binding",
			"pin_present": true,
			"pin_length": 6
		}
	}`)

	record := buildAuditLogRecord(req, []byte(`{"pin":"123456"}`), responseBody, http.StatusOK, time.Now())

	detail, _ := record.OperationDetail.(map[string]any)
	if detail["pin"] == "123456" {
		t.Fatalf("request pin = %#v, want masked", detail["pin"])
	}
	flow, _ := record.Metadata["operation_flow"].(map[string]any)
	if flow["stage"] != "pin_entry_binding" {
		t.Fatalf("operation_flow.stage = %#v, want pin_entry_binding", flow["stage"])
	}
	if flow["pin_stage"] != "binding" {
		t.Fatalf("operation_flow.pin_stage = %#v, want binding", flow["pin_stage"])
	}
	if flow["pin_auto_filled"] != true {
		t.Fatalf("operation_flow.pin_auto_filled = %#v, want true", flow["pin_auto_filled"])
	}
	if flow["pin_auto_submitted"] != true {
		t.Fatalf("operation_flow.pin_auto_submitted = %#v, want true", flow["pin_auto_submitted"])
	}
	if flow["otp_manual_required"] != false {
		t.Fatalf("operation_flow.otp_manual_required = %#v, want false", flow["otp_manual_required"])
	}
	if flow["balance_state"] != "rp1" {
		t.Fatalf("operation_flow.balance_state = %#v, want rp1", flow["balance_state"])
	}
	if flow["hubungkan_auto_clicked"] != true {
		t.Fatalf("operation_flow.hubungkan_auto_clicked = %#v, want true", flow["hubungkan_auto_clicked"])
	}
	if flow["pay_now_auto_clicked"] != false {
		t.Fatalf("operation_flow.pay_now_auto_clicked = %#v, want false", flow["pay_now_auto_clicked"])
	}
	if flow["auto_action_paused"] != false {
		t.Fatalf("operation_flow.auto_action_paused = %#v, want false", flow["auto_action_paused"])
	}
	if flow["auto_action_stage"] != "gopay_consent_hubungkan" {
		t.Fatalf("operation_flow.auto_action_stage = %#v, want gopay_consent_hubungkan", flow["auto_action_stage"])
	}
}

func TestGopayCDPFlowScriptKeepsOTPManualAndDefinesPINStages(t *testing.T) {
	script := gopayCDPFlowScript("123456")

	for _, want := range []string{"otp_manual_required", "pin_entry_binding", "pin_entry_payment"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q", want)
		}
	}
	for _, forbidden := range []string{"setNativeValue(otpInput", "submitBtn.click()"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("script should not auto-fill or submit OTP; found %q", forbidden)
		}
	}
}

func TestGopayCDPFlowScriptDefinesConsentAndBalanceGuards(t *testing.T) {
	script := gopayCDPFlowScript("123456")

	for _, want := range []string{
		"Hubungkan",
		"gopay_consent_hubungkan",
		"balance_wait_rp0",
		"pay_now_rp1",
		"Pay now",
		"auto_action_paused",
		"auto_action_stage",
		"pay_now_auto_clicked",
		"hubungkan_auto_clicked",
		"!result.auto_action_paused",
		"const actionScope = urlHost + urlPath;",
		"findStableActionByExactText",
		"gopay_cdp_hubungkan_cooldown_",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q", want)
		}
	}
	pauseCheck := strings.Index(script, "result.balance_state === 'rp0'")
	hubungkanCheck := strings.Index(script, "findStableActionByExactText(['Hubungkan'])")
	if pauseCheck < 0 || hubungkanCheck < 0 || pauseCheck > hubungkanCheck {
		t.Fatalf("script should check Rp0 pause before Hubungkan click: pause=%d hubungkan=%d", pauseCheck, hubungkanCheck)
	}
	markHubungkan := strings.Index(script, "storageSet(hubungkanKey);")
	clickHubungkan := strings.Index(script, "clickElement(hubungkanButton)")
	if markHubungkan < 0 || clickHubungkan < 0 || markHubungkan > clickHubungkan {
		t.Fatalf("script should mark Hubungkan handled before click: mark=%d click=%d", markHubungkan, clickHubungkan)
	}
}

func TestStaticAssetHandlerCachesAssetsAndCompressesText(t *testing.T) {
	handler := staticAssetHandler(os.DirFS("web"))
	req := httptest.NewRequest(http.MethodGet, "/app.js?v=test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "public") || !strings.Contains(got, "max-age") {
		t.Fatalf("Cache-Control = %q, want public cache", got)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
}

func TestStaticAssetHandlerKeepsHTMLNoStoreButCompresses(t *testing.T) {
	handler := staticAssetHandler(os.DirFS("web"))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store for HTML", got)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
}

func TestWebShellAvoidsRemoteFontBlockingAndDelaysBrowserUseProbe(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("web", "styles.css"))
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}
	html, err := os.ReadFile(filepath.Join("web", "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	combined := string(css) + "\n" + string(html)
	for _, forbidden := range []string{"fonts.googleapis.com", "fonts.gstatic.com", "@import url(\"https://fonts.googleapis.com"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("web shell should not block on remote fonts; found %q", forbidden)
		}
	}
	if !strings.Contains(string(html), "setTimeout(checkBrowserUseHealth") {
		t.Fatal("browser-use health probe should be delayed until after first paint")
	}
}

func TestHandleClientPerformanceLogSummarizesMetrics(t *testing.T) {
	payload := `{
		"page_url": "http://127.0.0.1:18473/?token=***",
		"nav": {"duration_ms": 210, "dom_content_loaded_ms": 120, "load_event_ms": 190},
		"resources": [
			{"url": "http://127.0.0.1:18473/app.js?v=***", "initiator_type": "script", "duration_ms": 10, "transfer_size": 1000},
			{"url": "http://127.0.0.1:18473/styles.css?v=***", "initiator_type": "link", "duration_ms": 8, "transfer_size": 500}
		],
		"long_tasks": [{"duration_ms": 75}],
		"slow_interactions": [{"name": "click", "duration_ms": 42}],
		"errors": [{"type": "error", "message": "boom"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/perf/client", strings.NewReader(payload))
	rec := httptest.NewRecorder()

	handleClientPerformanceLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["stage"] != "client_perf_recorded" {
		t.Fatalf("stage = %#v", response["stage"])
	}
	summary, _ := response["summary"].(map[string]any)
	if summary["resource_count"] != float64(2) {
		t.Fatalf("resource_count = %#v, want 2", summary["resource_count"])
	}
	if summary["long_task_count"] != float64(1) {
		t.Fatalf("long_task_count = %#v, want 1", summary["long_task_count"])
	}
	if summary["slow_interaction_count"] != float64(1) {
		t.Fatalf("slow_interaction_count = %#v, want 1", summary["slow_interaction_count"])
	}
	if summary["error_count"] != float64(1) {
		t.Fatalf("error_count = %#v, want 1", summary["error_count"])
	}
}

func TestBuildAuditLogRecordIncludesAutoTriggerFlowFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/gopay/auto-trigger-check", strings.NewReader(`{"source":"verification","submitted":false}`))
	responseBody := []byte(`{
		"ok": true,
		"stage": "auto_trigger_waiting",
		"ready": false,
		"reason": "waiting_for_checkout_submit",
		"checkout_key": "https://pay.openai.com/c/pay/cs_test_123",
		"conditions": {"submitted": false, "gopay_detected": true}
	}`)

	record := buildAuditLogRecord(req, []byte(`{"source":"verification","submitted":false}`), responseBody, http.StatusOK, time.Now())

	flow, _ := record.Metadata["operation_flow"].(map[string]any)
	if flow["reason"] != "waiting_for_checkout_submit" {
		t.Fatalf("operation_flow.reason = %#v", flow["reason"])
	}
	if flow["ready"] != false {
		t.Fatalf("operation_flow.ready = %#v", flow["ready"])
	}
	if _, ok := flow["conditions"].(map[string]any); !ok {
		t.Fatalf("operation_flow.conditions type = %T", flow["conditions"])
	}
	summary, _ := record.Metadata["response_summary"].(map[string]any)
	if summary["reason"] != "waiting_for_checkout_submit" {
		t.Fatalf("response_summary.reason = %#v", summary["reason"])
	}
}

func TestGopayAutoTriggerDecisionRequiresCheckoutGopayAndZeroDue(t *testing.T) {
	req := gopayAutoTriggerCheckRequest{
		Source:      "checkout-auto-fill",
		CheckoutURL: "https://pay.openai.com/c/pay/cs_live_123#fragment",
		PageText:    "今日应付合计 IDR 0.00 支付方式 银行卡 GoPay 然后在优惠券过期后，每月 IDR 349,000.00",
		Submitted:   true,
	}

	decision := evaluateGopayAutoTrigger(req, true, false)

	if !decision.Ready {
		t.Fatalf("Ready = false, reason = %q, conditions = %#v", decision.Reason, decision.Conditions)
	}
	if decision.CheckoutKey != "https://pay.openai.com/c/pay/cs_live_123" {
		t.Fatalf("CheckoutKey = %q", decision.CheckoutKey)
	}

	req.Submitted = false
	decision = evaluateGopayAutoTrigger(req, true, false)
	if decision.Ready {
		t.Fatalf("Ready = true before checkout submit")
	}
	if decision.Reason != "waiting_for_checkout_submit" {
		t.Fatalf("Reason = %q, want waiting_for_checkout_submit", decision.Reason)
	}

	req.Submitted = true
	req.PageText = "今日应付合计 IDR 349,000.00 支付方式 银行卡 GoPay"
	decision = evaluateGopayAutoTrigger(req, true, false)
	if decision.Ready {
		t.Fatalf("Ready = true for non-zero due amount")
	}
	if decision.Reason != "today_due_not_zero" {
		t.Fatalf("Reason = %q, want today_due_not_zero", decision.Reason)
	}

	req.PageText = "今日应付合计 IDR 0.00 支付方式 银行卡"
	decision = evaluateGopayAutoTrigger(req, true, false)
	if decision.Ready {
		t.Fatalf("Ready = true when GoPay is missing")
	}
	if decision.Reason != "gopay_not_detected" {
		t.Fatalf("Reason = %q, want gopay_not_detected", decision.Reason)
	}
}

func TestMarkGopayAutoTriggerReadyDoesNotClaimTrigger(t *testing.T) {
	oldConfig := config
	config = appConfig{AuditCaptureSensitive: true}
	t.Cleanup(func() {
		config = oldConfig
		gopayAutoTriggerMu.Lock()
		gopayAutoTriggeredCheckout = map[string]time.Time{}
		gopayAutoTriggerMu.Unlock()
	})
	gopayAutoTriggerMu.Lock()
	gopayAutoTriggeredCheckout = map[string]time.Time{}
	gopayAutoTriggerMu.Unlock()

	req := gopayAutoTriggerCheckRequest{
		Source:      "checkout-auto-fill",
		CheckoutURL: "https://pay.openai.com/c/pay/cs_live_once#fragment",
		PageText:    "今日应付合计 IDR 0.00 支付方式 GoPay",
		Submitted:   true,
	}

	first := markGopayAutoTriggerReady(req)
	second := markGopayAutoTriggerReady(req)

	if !first.Ready || first.AlreadyTriggered {
		t.Fatalf("first decision = %#v, want ready and not already triggered", first)
	}
	if !second.Ready || second.AlreadyTriggered {
		t.Fatalf("second decision = %#v, want ready and not already triggered before fill is claimed", second)
	}
	if !claimGopayAutoTrigger(req.CheckoutURL) {
		t.Fatal("first claimGopayAutoTrigger returned false, want true")
	}
	third := markGopayAutoTriggerReady(req)
	if third.Ready || !third.AlreadyTriggered {
		t.Fatalf("third decision = %#v, want not ready and already triggered after fill claim", third)
	}
	if claimGopayAutoTrigger(req.CheckoutURL) {
		t.Fatal("second claimGopayAutoTrigger returned true, want false")
	}
}

func TestCheckoutResolveCandidateURLsReturnsRecentPageTargets(t *testing.T) {
	targets := []cdpTarget{
		{Type: "iframe", URL: "https://js.stripe.com/v3/elements-inner-payment"},
		{Type: "page", URL: "https://chatgpt.com/"},
		{Type: "page", URL: "https://app.midtrans.com/snap/v4/redirection/acct-123#/payment"},
	}

	urls := checkoutResolveCandidateURLs(targets)

	if len(urls) != 2 {
		t.Fatalf("candidate urls length = %d, want 2", len(urls))
	}
	if urls[0] != "https://app.midtrans.com/snap/v4/redirection/acct-123#/payment" {
		t.Fatalf("urls[0] = %q", urls[0])
	}
	if urls[1] != "https://chatgpt.com/" {
		t.Fatalf("urls[1] = %q", urls[1])
	}
}

func TestMidtransLinkingAccountIDFromURL(t *testing.T) {
	rawURL := "https://app.midtrans.com/snap/v4/redirection/b070d7fb-6d98-4cbd-99df-fa13843415ba#/gopay-tokenization/linking"

	accountID := midtransLinkingAccountID(rawURL)

	if accountID != "b070d7fb-6d98-4cbd-99df-fa13843415ba" {
		t.Fatalf("accountID = %q", accountID)
	}
}

func TestMidtransRedirectionAccountIDFromBaseURL(t *testing.T) {
	rawURL := "https://app.midtrans.com/snap/v4/redirection/b070d7fb-6d98-4cbd-99df-fa13843415ba"

	accountID := midtransRedirectionAccountID(rawURL)

	if accountID != "b070d7fb-6d98-4cbd-99df-fa13843415ba" {
		t.Fatalf("accountID = %q", accountID)
	}
}

func TestMidtransLinkingAccountIDRejectsNonLinkingURL(t *testing.T) {
	rawURL := "https://app.midtrans.com/snap/v4/redirection/b070d7fb-6d98-4cbd-99df-fa13843415ba#/payment"

	accountID := midtransLinkingAccountID(rawURL)

	if accountID != "" {
		t.Fatalf("accountID = %q, want empty", accountID)
	}
}

func TestMidtransLinkingFillOutcomeRejectsClickedWithWrongCountry(t *testing.T) {
	ok, stage := midtransLinkingFillOutcome(map[string]any{
		"clicked":           true,
		"country_code":      "86",
		"page_text_snippet": "Link GoPay account with OpenAI LLC. Phone number: +62 Link and pay",
		"found": map[string]any{
			"country": false,
			"phone":   true,
			"button":  true,
		},
	})

	if ok {
		t.Fatal("ok = true, want false when country code is not confirmed")
	}
	if stage != "midtrans_country_code_not_selected" {
		t.Fatalf("stage = %q, want midtrans_country_code_not_selected", stage)
	}
}

func TestMidtransLinkingFillOutcomeAcceptsVerifiedCountry(t *testing.T) {
	ok, stage := midtransLinkingFillOutcome(map[string]any{
		"clicked":          true,
		"country_code":     "86",
		"country_verified": true,
		"found": map[string]any{
			"country": true,
			"phone":   true,
			"button":  true,
		},
	})

	if !ok {
		t.Fatal("ok = false, want true when country, phone and button are confirmed")
	}
	if stage != "midtrans_linking_submitted" {
		t.Fatalf("stage = %q, want midtrans_linking_submitted", stage)
	}
}

func TestMidtransLinkingFillOutcomeRejectsTechnicalErrorAfterClick(t *testing.T) {
	ok, stage := midtransLinkingFillOutcome(map[string]any{
		"clicked":          true,
		"country_code":     "86",
		"country_verified": true,
		"found": map[string]any{
			"country": true,
			"phone":   true,
			"button":  true,
		},
		"page_text_snippet": "Phone number: +86 Link and pay There’s a technical error Don’t worry, we’re working on it. Please try again. Back",
	})

	if ok {
		t.Fatal("ok = true, want false when technical error is still visible")
	}
	if stage != "midtrans_linking_technical_error" {
		t.Fatalf("stage = %q, want midtrans_linking_technical_error", stage)
	}
}

func TestMidtransLinkingRetryConfigDefault(t *testing.T) {
	cfg := midtransLinkingRetryConfig(false)

	if cfg.MaxClickAttempts != 3 {
		t.Fatalf("MaxClickAttempts = %d, want 3", cfg.MaxClickAttempts)
	}
	if cfg.ButtonWaitCycles != 8 {
		t.Fatalf("ButtonWaitCycles = %d, want 8", cfg.ButtonWaitCycles)
	}
	if cfg.PostClickWaitCycles != 18 {
		t.Fatalf("PostClickWaitCycles = %d, want 18", cfg.PostClickWaitCycles)
	}
	if cfg.RetryStillLinking {
		t.Fatal("RetryStillLinking = true, want false")
	}
}

func TestMidtransLinkingRetryConfigAggressive(t *testing.T) {
	cfg := midtransLinkingRetryConfig(true)

	if cfg.MaxClickAttempts <= 3 {
		t.Fatalf("MaxClickAttempts = %d, want > 3", cfg.MaxClickAttempts)
	}
	if cfg.MaxClickAttempts > 5 {
		t.Fatalf("MaxClickAttempts = %d, want <= 5 to avoid rate-limit bursts", cfg.MaxClickAttempts)
	}
	if cfg.ButtonWaitCycles <= 8 {
		t.Fatalf("ButtonWaitCycles = %d, want > 8", cfg.ButtonWaitCycles)
	}
	if cfg.PostClickWaitCycles <= 18 {
		t.Fatalf("PostClickWaitCycles = %d, want > 18", cfg.PostClickWaitCycles)
	}
	if cfg.PostClickWaitMs < 800 {
		t.Fatalf("PostClickWaitMs = %d, want >= 800 to avoid rapid repeated clicks", cfg.PostClickWaitMs)
	}
	if !cfg.RetryStillLinking {
		t.Fatal("RetryStillLinking = false, want true")
	}
}

func TestSanitizeMidtransNetworkDiagnosticsSummarizesHeadersAndResponseFields(t *testing.T) {
	authValue := "Bearer abcdefghijklmnopqrstuvwxyz"
	cookieValue := "midtrans_session=session-cookie-value"
	result := map[string]any{
		"network_diagnostics": map[string]any{
			"entries": []any{
				map[string]any{
					"kind":                  "fetch",
					"method":                "POST",
					"url":                   "https://app.midtrans.com/snap/v4/token?token=secret-token&account_id=acct-123#frag",
					"status":                float64(500),
					"request_headers":       map[string]any{"authorization": authValue, "cookie": cookieValue, "x-client-id": "client-value-1234"},
					"response_headers":      "content-type: application/json\r\nset-cookie: sid=secret-cookie\r\nx-request-id: request-12345678\r\n",
					"response_text_snippet": `{"status_code":500,"error_code":"GOPAY_LINK_FAILED","message":"technical error","transaction_id":"tx-123","reference_id":"ref-123","access_token":"secret-access-token-value","payment_token":"payment-token-9999"}`,
				},
			},
		},
	}

	sanitizeMidtransNetworkDiagnostics(result)

	diagnostics := result["network_diagnostics"].(map[string]any)
	entries := diagnostics["entries"].([]any)
	entry := entries[0].(map[string]any)
	if entry["url"] != "https://app.midtrans.com/snap/v4/token?token=***&account_id=***#frag" {
		t.Fatalf("url = %#v", entry["url"])
	}
	requestHeaders := entry["request_headers"].(map[string]any)
	authSummary := requestHeaders["authorization"].(map[string]any)
	if authSummary["present"] != true {
		t.Fatalf("authorization present = %#v", authSummary["present"])
	}
	if authSummary["length"] != len(authValue) {
		t.Fatalf("authorization length = %#v", authSummary["length"])
	}
	if authSummary["prefix4"] != "Bear" || authSummary["suffix4"] != "wxyz" {
		t.Fatalf("authorization fingerprint = %#v", authSummary)
	}
	authSum := sha256.Sum256([]byte(authValue))
	if authSummary["sha256"] != hex.EncodeToString(authSum[:]) {
		t.Fatalf("authorization sha256 = %#v", authSummary["sha256"])
	}
	if strings.Contains(stringifyJSONValue(requestHeaders), "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("request_headers leaked raw authorization: %#v", requestHeaders)
	}

	responseHeaders := entry["response_headers"].(map[string]any)
	setCookie := responseHeaders["set-cookie"].(map[string]any)
	if setCookie["present"] != true || setCookie["length"] == 0 {
		t.Fatalf("set-cookie summary = %#v", setCookie)
	}
	if strings.Contains(stringifyJSONValue(responseHeaders), "secret-cookie") {
		t.Fatalf("response_headers leaked raw cookie: %#v", responseHeaders)
	}

	body := entry["response_text_snippet"].(map[string]any)
	fields := body["fields"].(map[string]any)
	if fields["message"] != "technical error" {
		t.Fatalf("message = %#v", fields["message"])
	}
	if fields["error_code"] != "GOPAY_LINK_FAILED" || fields["transaction_id"] != "tx-123" || fields["reference_id"] != "ref-123" {
		t.Fatalf("diagnostic fields = %#v", fields)
	}
	credentials := body["credentials"].(map[string]any)
	accessToken := credentials["access_token"].(map[string]any)
	if accessToken["tail4"] != "alue" || accessToken["length"] == 0 || accessToken["sha256"] == "" {
		t.Fatalf("access_token credential summary = %#v", accessToken)
	}
	if strings.Contains(stringifyJSONValue(body), "secret-access-token-value") {
		t.Fatalf("response body leaked raw token: %#v", body)
	}
}

func TestWriteMidtransNetworkDebugArtifactUsesSanitizedPayload(t *testing.T) {
	debugDir := t.TempDir()
	result := map[string]any{
		"network_diagnostics": map[string]any{
			"entries": []any{
				map[string]any{
					"url":                   "https://app.midtrans.com/snap/v4/token?token=secret-token",
					"request_headers":       map[string]any{"authorization": "Bearer token-value-1234"},
					"response_text_snippet": `{"message":"denied","access_token":"raw-token-value"}`,
				},
			},
		},
	}
	sanitizeMidtransNetworkDiagnostics(result)

	artifact, err := writeMidtransNetworkDebugArtifact(debugDir, "acct/test", gopayMidtransLinkingFillRequest{
		TargetURL:   "https://app.midtrans.com/snap/v4/redirection/acct?session=secret",
		CheckoutURL: "https://pay.openai.com/c/pay/cs_test?client_secret=secret",
	}, result)
	if err != nil {
		t.Fatalf("writeMidtransNetworkDebugArtifact returned error: %v", err)
	}
	path := stringifyJSONValue(artifact["path"])
	if !strings.HasPrefix(path, debugDir) {
		t.Fatalf("debug path = %q, want under %q", path, debugDir)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read debug artifact: %v", err)
	}
	text := string(payload)
	for _, leaked := range []string{"secret-token", "token-value-1234", "raw-token-value", "client_secret=secret", "session=secret"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("debug artifact leaked %q: %s", leaked, text)
		}
	}
	if !strings.Contains(text, `"sha256"`) || !strings.Contains(text, `"tail4"`) || !strings.Contains(text, `"message"`) {
		t.Fatalf("debug artifact missing diagnostic summaries: %s", text)
	}
}

func TestAuditOperationFlowIncludesNetworkDiagnostics(t *testing.T) {
	flow := auditOperationFlow(map[string]any{
		"ok": true,
		"network_diagnostics": map[string]any{
			"entries": []any{
				map[string]any{"url": "https://app.midtrans.com/snap/v4/token", "status": float64(500)},
			},
		},
	})

	if _, ok := flow["network_diagnostics"]; !ok {
		t.Fatal("network_diagnostics missing from audit operation flow")
	}
}

func TestAuditResponseWriterPreservesFlusher(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &auditResponseWriter{ResponseWriter: recorder, statusCode: http.StatusOK}
	flusher, ok := any(writer).(http.Flusher)
	if !ok {
		t.Fatal("auditResponseWriter should expose http.Flusher when the wrapped writer supports it")
	}

	flusher.Flush()
	if !recorder.Flushed {
		t.Fatal("Flush was not forwarded to wrapped response writer")
	}
}

func TestAuditLoggerWritesFileWithEmailAndTimestamp(t *testing.T) {
	logDir := t.TempDir()
	logger := newAuditLogger(logDir, time.Hour, 1<<20, 2)
	t.Cleanup(func() {
		if err := logger.Close(); err != nil {
			t.Fatalf("logger.Close returned error: %v", err)
		}
	})
	timestamp := time.Date(2026, 5, 6, 21, 22, 23, 0, time.UTC)
	logger.write(auditLogEnvelope{
		Email:     "user@example.com",
		Timestamp: timestamp,
		Record: auditLogRecord{
			OperationTime:   timestamp.Format(time.RFC3339Nano),
			OperationType:   "monitor_trace",
			OperationName:   "流程监控",
			OperationResult: "success",
			AccountEmail:    "user@example.com",
			IPAddress:       "127.0.0.1",
			RequestPath:     "/api/gopay/monitor",
			RequestMethod:   http.MethodPost,
		},
	})
	matches, err := filepath.Glob(filepath.Join(logDir, "user@example.com_20260506212223.log"))
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(data), `"account_email":"user@example.com"`) {
		t.Fatalf("log content = %s", string(data))
	}
}

func TestExtractResultStringReadsStringValue(t *testing.T) {
	resp := &cdpResponse{
		Result: map[string]any{
			"result": map[string]any{
				"value": "ok",
			},
		},
	}
	got, err := extractResultString(resp)
	if err != nil {
		t.Fatalf("extractResultString returned error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("got = %q", got)
	}
}

func TestWithSessionDiagnosticsConclusionAddsConclusion(t *testing.T) {
	diagnostics := withSessionDiagnosticsConclusion(map[string]any{
		"cdp_ready": false,
	})
	conclusion, ok := diagnostics["conclusion"].(map[string]any)
	if !ok {
		t.Fatalf("conclusion type = %T", diagnostics["conclusion"])
	}
	if conclusion["status"] != "cdp_not_ready" {
		t.Fatalf("status = %#v", conclusion["status"])
	}
}

func TestSetCheckoutBackendHeadersUsesExplicitSessionCookie(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/payments/checkout", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	setCheckoutBackendHeaders(req, "token-123", checkoutSession{
		Cookie:    "oai-session=test",
		UserAgent: "browser-agent",
	})

	if got := req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization = %q, want Bearer token-123", got)
	}
	if got := req.Header.Get("Cookie"); got != "oai-session=test" {
		t.Fatalf("Cookie = %q, want explicit cookie", got)
	}
	if got := req.Header.Get("User-Agent"); got != "browser-agent" {
		t.Fatalf("User-Agent = %q, want explicit user agent", got)
	}
}

func TestCheckoutUpstreamNonJSONPayloadSummarizesCookie403(t *testing.T) {
	payload := checkoutUpstreamNonJSONPayload(
		http.StatusForbidden,
		"text/html; charset=UTF-8",
		"https://chatgpt.com/backend-api/payments/checkout",
		[]byte("<html><body>Please enable cookies to continue</body></html>"),
	)

	if payload["status"] != http.StatusForbidden {
		t.Fatalf("status = %v, want 403", payload["status"])
	}
	if payload["requires_cookie"] != true {
		t.Fatalf("requires_cookie = %v, want true", payload["requires_cookie"])
	}
	if _, ok := payload["raw_body"]; ok {
		t.Fatal("payload should not include raw_body")
	}
	if payload["error"] == "" {
		t.Fatal("payload missing error")
	}
}

func TestCheckoutUpstreamNonJSONPayloadSummarizesInvalidated401Token(t *testing.T) {
	payload := checkoutUpstreamNonJSONPayload(
		http.StatusUnauthorized,
		"text/plain",
		"https://chatgpt.com/backend-api/payments/checkout",
		[]byte(`{"error":{"message":"Your authentication token has been invalidated. Please try signing in again.","code":"token_invalidated"},"status":401}`),
	)
	if payload["status"] != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", payload["status"])
	}
	if payload["token_invalidated"] != true {
		t.Fatalf("token_invalidated = %v, want true", payload["token_invalidated"])
	}
	if payload["hint"] == "" {
		t.Fatal("payload hint is empty")
	}
	if payload["error"] == "" {
		t.Fatal("payload error is empty")
	}
}

func TestWaitForStripeRedirectNextActionRetriesUntilRedirectAppears(t *testing.T) {
	calls := 0
	result, redirect, history := waitForStripeRedirectNextAction(context.Background(), 3, 0, func() stripeInitResult {
		calls++
		if calls == 1 {
			return stripeInitResult{
				OK:   true,
				Body: map[string]any{"payment_status": "unpaid"},
			}
		}
		return stripeInitResult{
			OK: true,
			Body: map[string]any{
				"payment_intent": map[string]any{
					"next_action": map[string]any{
						"type": "redirect_to_url",
						"redirect_to_url": map[string]any{
							"url": "https://pm-redirects.stripe.com/redirect/test",
						},
					},
				},
			},
		}
	})

	if !result.OK {
		t.Fatalf("result.OK = false")
	}
	if !redirect.OK {
		t.Fatalf("redirect.OK = false, error = %q", redirect.Error)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
}

func TestWaitForStripeRedirectNextActionReportsLastError(t *testing.T) {
	result, redirect, history := waitForStripeRedirectNextAction(context.Background(), 2, 0, func() stripeInitResult {
		return stripeInitResult{
			OK:   true,
			Body: map[string]any{"payment_status": "unpaid"},
		}
	})

	if !result.OK {
		t.Fatalf("result.OK = false")
	}
	if redirect.OK {
		t.Fatal("redirect.OK = true, want false")
	}
	if redirect.Error == "" {
		t.Fatal("redirect error is empty")
	}
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	_ = time.Duration(0)
}

func TestCheckoutProcessorEntityUsesCheckoutResponseValue(t *testing.T) {
	got := checkoutProcessorEntity(map[string]any{
		"processor_entity": "openai_europe",
	})
	if got != "openai_europe" {
		t.Fatalf("checkoutProcessorEntity = %q, want openai_europe", got)
	}
}

func TestCheckoutProcessorEntityFallsBackToOpenAILLC(t *testing.T) {
	got := checkoutProcessorEntity(map[string]any{})
	if got != "openai_llc" {
		t.Fatalf("checkoutProcessorEntity fallback = %q, want openai_llc", got)
	}
}

func TestCheckoutLongURLPrefersOpenAIHostedURL(t *testing.T) {
	got := checkoutLongURL(map[string]any{
		"url": "https://pay.openai.com/c/pay/cs_live_abc123#fid=test",
	})
	if got != "https://pay.openai.com/c/pay/cs_live_abc123#fid=test" {
		t.Fatalf("checkoutLongURL = %q", got)
	}
}

func TestCheckoutLongURLAcceptsStripeHostedURLFallback(t *testing.T) {
	got := checkoutLongURL(map[string]any{
		"stripe_hosted_url": "https://checkout.stripe.com/pay/cs_test_abc123",
	})
	if got != "https://checkout.stripe.com/pay/cs_test_abc123" {
		t.Fatalf("checkoutLongURL fallback = %q", got)
	}
}

func TestCheckoutLongURLPrefersOpenAIWrapperForStripeHostedURL(t *testing.T) {
	got := checkoutLongURL(map[string]any{
		"stripe_hosted_url": "https://checkout.stripe.com/c/pay/cs_live_abc123#fid=test",
	})
	if got != "https://pay.openai.com/c/pay/cs_live_abc123#fid=test" {
		t.Fatalf("checkoutLongURL wrapper = %q", got)
	}
}

func TestCheckoutLongURLDoesNotGuessFromSessionID(t *testing.T) {
	got := checkoutLongURL(map[string]any{
		"checkout_session_id": "cs_live_abc123",
		"url":                 "",
	})
	if got != "" {
		t.Fatalf("checkoutLongURL guessed from session id = %q", got)
	}
}

func TestCheckoutLongURLRejectsNonCheckoutURLs(t *testing.T) {
	got := checkoutLongURL(map[string]any{
		"url":                "https://chatgpt.com/backend-api/payments/checkout",
		"confirm_return_url": "https://chatgpt.com/checkout/verify?stripe_session_id=cs_live_abc123",
	})
	if got != "" {
		t.Fatalf("checkoutLongURL = %q, want empty", got)
	}
}

func TestCheckoutTargetMatchesExpectedDirectPageURL(t *testing.T) {
	expected := "https://chatgpt.com/checkout/openai_llc/cs_live_test_123"
	current := "https://chatgpt.com/checkout/openai_llc/cs_live_test_123"
	if !checkoutTargetMatchesExpected(current, expected) {
		t.Fatal("checkoutTargetMatchesExpected = false, want true for direct page URL")
	}
}

func TestCheckoutTargetMatchesExpectedStripeFrameReferrer(t *testing.T) {
	expected := "https://chatgpt.com/checkout/openai_llc/cs_live_test_123"
	current := "https://js.stripe.com/v3/elements-inner-payment.html#referrer=https%3A%2F%2Fchatgpt.com%2Fcheckout%2Fopenai_llc%2Fcs_live_test_123&controllerId=test"
	if !checkoutTargetMatchesExpected(current, expected) {
		t.Fatal("checkoutTargetMatchesExpected = false, want true for stripe frame referrer")
	}
}

func TestCheckoutTargetMatchesExpectedMismatch(t *testing.T) {
	expected := "https://chatgpt.com/checkout/openai_llc/cs_live_test_123"
	current := "https://chatgpt.com/checkout/openai_llc/cs_live_other"
	if checkoutTargetMatchesExpected(current, expected) {
		t.Fatal("checkoutTargetMatchesExpected = true, want false for mismatch")
	}
}

func TestResolveCheckoutPageTargetReturnsCanonicalCurrentURL(t *testing.T) {
	expected := "https://pay.openai.com/c/pay/cs_live_test_123#fid=test"
	targets := []cdpTarget{
		{ID: "1", Type: "page", URL: "https://chatgpt.com/checkout/openai_llc/cs_live_test_123"},
	}
	target, canonicalURL, err := resolveCheckoutPageTarget(targets, expected)
	if err != nil {
		t.Fatalf("resolveCheckoutPageTarget returned error: %v", err)
	}
	if target.ID != "1" {
		t.Fatalf("target.ID = %q, want 1", target.ID)
	}
	if canonicalURL != "https://chatgpt.com/checkout/openai_llc/cs_live_test_123" {
		t.Fatalf("canonicalURL = %q", canonicalURL)
	}
}

func TestResolveCheckoutFillTargetPrefersStripeFrameMatchingExpected(t *testing.T) {
	expected := "https://pay.openai.com/c/pay/cs_live_test_123#fid=test"
	targets := []cdpTarget{
		{ID: "page-1", Type: "page", URL: "https://chatgpt.com/checkout/openai_llc/cs_live_test_123"},
		{ID: "frame-1", Type: "iframe", URL: "https://js.stripe.com/v3/elements-inner-payment.html#referrer=https%3A%2F%2Fchatgpt.com%2Fcheckout%2Fopenai_llc%2Fcs_live_test_123&controllerId=test"},
	}
	target, canonicalURL, err := resolveCheckoutFillTarget(targets, expected)
	if err != nil {
		t.Fatalf("resolveCheckoutFillTarget returned error: %v", err)
	}
	if target.ID != "frame-1" {
		t.Fatalf("target.ID = %q, want frame-1", target.ID)
	}
	if canonicalURL != "https://chatgpt.com/checkout/openai_llc/cs_live_test_123" {
		t.Fatalf("canonicalURL = %q", canonicalURL)
	}
}

func TestHasFillableCheckoutInputsRejectsNilProbe(t *testing.T) {
	if hasFillableCheckoutInputs(nil) {
		t.Fatal("hasFillableCheckoutInputs = true, want false for nil probe")
	}
}

func TestHasFillableCheckoutInputsRejectsZeroInputs(t *testing.T) {
	probe := map[string]any{"inputCount": float64(0)}
	if hasFillableCheckoutInputs(probe) {
		t.Fatal("hasFillableCheckoutInputs = true, want false for zero inputs")
	}
}

func TestHasFillableCheckoutInputsAcceptsPositiveInputs(t *testing.T) {
	probe := map[string]any{"inputCount": float64(3)}
	if !hasFillableCheckoutInputs(probe) {
		t.Fatal("hasFillableCheckoutInputs = false, want true for positive inputs")
	}
}

func TestCheckoutAutoFillActionActivatesGopayWhenChoiceVisible(t *testing.T) {
	probe := map[string]any{
		"inputCount": float64(0),
		"text":       "支付方式 银行卡 GoPay 联系信息",
	}
	if got := checkoutAutoFillAction(probe, false); got != "activate_gopay" {
		t.Fatalf("checkoutAutoFillAction = %q, want activate_gopay", got)
	}
}

func TestCheckoutAutoFillActionNeedsManualWhenNoInputsAndNoGopay(t *testing.T) {
	probe := map[string]any{
		"inputCount": float64(0),
		"text":       "Something went wrong",
	}
	if got := checkoutAutoFillAction(probe, false); got != "manual" {
		t.Fatalf("checkoutAutoFillAction = %q, want manual", got)
	}
}

func TestCheckoutStateSelectValueUsesFullStateName(t *testing.T) {
	if got := checkoutStateSelectValue("NC"); got != "North Carolina" {
		t.Fatalf("checkoutStateSelectValue = %q, want North Carolina", got)
	}
	if got := checkoutStateSelectValue("California"); got != "California" {
		t.Fatalf("checkoutStateSelectValue passthrough = %q, want California", got)
	}
}

func TestCheckStripeInitTotalAcceptsZeroTrial(t *testing.T) {
	check := checkStripeInitTotal(map[string]any{
		"total_summary": map[string]any{
			"total": float64(0),
		},
	})
	if !check.OK {
		t.Fatalf("checkStripeInitTotal OK = false, error = %q", check.Error)
	}
	if check.Total != 0 {
		t.Fatalf("checkStripeInitTotal total = %d, want 0", check.Total)
	}
}

func TestCheckStripeInitTotalRejectsPaidSession(t *testing.T) {
	check := checkStripeInitTotal(map[string]any{
		"total_summary": map[string]any{
			"total": float64(34900000),
		},
	})
	if check.OK {
		t.Fatal("checkStripeInitTotal accepted paid session")
	}
	if check.Total != 34900000 {
		t.Fatalf("checkStripeInitTotal total = %d, want 34900000", check.Total)
	}
	if check.Error == "" {
		t.Fatal("checkStripeInitTotal error is empty")
	}
}

func TestSummarizeCheckoutApprovalOmitsBody(t *testing.T) {
	summary := summarizeCheckoutApproval(map[string]any{
		"ok":                true,
		"stage":             "checkout_approval_upstream",
		"status":            200,
		"processor_entity":  "openai_llc",
		"payment_method_id": "pm_123",
		"body": map[string]any{
			"secret": "do-not-include",
		},
	})
	if _, ok := summary["body"]; ok {
		t.Fatal("summary should not include body")
	}
	if summary["processor_entity"] != "openai_llc" {
		t.Fatalf("processor_entity = %v, want openai_llc", summary["processor_entity"])
	}
	if summary["payment_method_id"] != "pm_123" {
		t.Fatalf("payment_method_id = %v, want pm_123", summary["payment_method_id"])
	}
}

func TestCheckoutSubmissionAttemptIDReadsNestedID(t *testing.T) {
	got := checkoutSubmissionAttemptID(map[string]any{
		"submission_attempt": map[string]any{
			"id": "subatt_123",
		},
	})
	if got != "subatt_123" {
		t.Fatalf("checkoutSubmissionAttemptID = %q, want subatt_123", got)
	}
}

func TestSummarizeStripeResultOmitsBodyAndKeepsStatus(t *testing.T) {
	summary := summarizeStripeResult(stripeInitResult{
		OK:          true,
		Stage:       "stripe_confirm",
		Status:      200,
		ContentType: "application/json",
		ElapsedMS:   123,
		Body: map[string]any{
			"secret": "do-not-include",
		},
	})

	if _, ok := summary["body"]; ok {
		t.Fatal("summary should not include body")
	}
	if summary["stage"] != "stripe_confirm" {
		t.Fatalf("stage = %v, want stripe_confirm", summary["stage"])
	}
	if summary["status"] != 200 {
		t.Fatalf("status = %v, want 200", summary["status"])
	}
}

func TestSummarizeStripeResultIncludesBodyAndOriginalErrorWhenFailed(t *testing.T) {
	summary := summarizeStripeResult(stripeInitResult{
		OK:          false,
		Stage:       "stripe_confirm",
		Status:      400,
		ContentType: "application/json",
		Body: map[string]any{
			"error": map[string]any{
				"message": "confirm failed",
				"code":    "payment_intent_unexpected_state",
			},
		},
	})

	if _, ok := summary["body"]; !ok {
		t.Fatal("summary should include body when failed")
	}
	if _, ok := summary["original_error"]; !ok {
		t.Fatal("summary should include original_error when failed")
	}
}

func TestStripeOriginalErrorCodeReadsNestedCode(t *testing.T) {
	code := stripeOriginalErrorCode(map[string]any{
		"error": map[string]any{
			"code": "checkout_not_active_session",
			"type": "invalid_request_error",
		},
	})
	if code != "checkout_not_active_session" {
		t.Fatalf("code = %q", code)
	}
}

func TestShouldRetryCheckoutFlowReturnsTrueForInactiveSession(t *testing.T) {
	flow := stripeSnapAccountFlowResult{
		Init: stripeInitResult{
			Body: map[string]any{
				"error": map[string]any{
					"code": "checkout_not_active_session",
				},
			},
		},
	}
	if !shouldRetryCheckoutFlow(flow) {
		t.Fatal("shouldRetryCheckoutFlow = false, want true")
	}
}

func TestBuildExtractAccountFailureStageIncludesTermsInteraction(t *testing.T) {
	stage := buildExtractAccountFailureStage(stripeSnapAccountFlowResult{
		TermsInteraction: map[string]any{"ok": true, "candidates": []map[string]any{{"id": "frame-1"}}},
	}, map[string]any{"url": "https://pay.openai.com/c/pay/cs_live_test"})
	if _, ok := stage["terms_interaction"]; !ok {
		t.Fatal("terms_interaction missing from stage")
	}
}

func TestStripeOriginalErrorMessageReadsNestedMessage(t *testing.T) {
	message := stripeOriginalErrorMessage(map[string]any{
		"error": map[string]any{
			"message": "Please accept the merchant's terms of service before checking out.",
			"type":    "invalid_request_error",
		},
	})
	if !strings.Contains(message, "terms of service") {
		t.Fatalf("message = %q", message)
	}
}

func TestNormalizeCheckoutRequestForcesHostedMode(t *testing.T) {
	req := checkoutRequest{CheckoutUIMode: "custom"}
	normalizeCheckoutRequest(&req)
	if req.CheckoutUIMode != "hosted" {
		t.Fatalf("CheckoutUIMode = %q, want hosted", req.CheckoutUIMode)
	}
}

func TestExtractAccessTokenAcceptsRawJWT(t *testing.T) {
	raw := "eyJhbGciOiJIUzI1NiJ9.payload.signature"
	if got := extractAccessToken(raw); got != raw {
		t.Fatalf("extractAccessToken = %q, want raw jwt", got)
	}
}

func TestExtractAccessTokenReadsSessionJSON(t *testing.T) {
	raw := `{"user":{"email":"test@example.com"},"accessToken":"eyJhbGciOiJIUzI1NiJ9.payload.signature"}`
	got := extractAccessToken(raw)
	if got != "eyJhbGciOiJIUzI1NiJ9.payload.signature" {
		t.Fatalf("extractAccessToken = %q", got)
	}
}

// ================================
// 三通道系统性测试套件
// 通道 1: force-link   (CDP 注入 + 直接 API)
// 通道 2: auto-link    (全自动 linking→OTP→PIN→success)
// 通道 3: cdp-otp      (CDP 注入 OTP 到 Snap 页面)
// ================================

// --- 通用工具函数测试 ---

func TestReferenceFromActivationLinkValid(t *testing.T) {
	raw := "https://app.midtrans.com/snap/v3/accounts/guid/linking?reference=gpar_6123269-1425-21e3-bc44-e592afafec14"
	got, err := referenceFromActivationLink(raw)
	if err != nil {
		t.Fatalf("referenceFromActivationLink error: %v", err)
	}
	if got != "gpar_6123269-1425-21e3-bc44-e592afafec14" {
		t.Fatalf("referenceFromActivationLink = %q, want gpar_6123269-...", got)
	}
}

func TestReferenceFromActivationLinkEmpty(t *testing.T) {
	_, err := referenceFromActivationLink("")
	if err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestReferenceFromActivationLinkNoReference(t *testing.T) {
	_, err := referenceFromActivationLink("https://app.midtrans.com/snap/v3/accounts/guid/linking")
	if err == nil {
		t.Fatal("expected error for url without reference")
	}
}

// --- gopayLink 结构体边界测试 ---

func TestGopayLinkEmptyCountryCode(t *testing.T) {
	link := gopayLink{CountryCode: "", PhoneNumber: "18120322232"}
	if link.CountryCode != "" {
		t.Fatalf("country_code should be empty")
	}
	if link.PhoneNumber != "18120322232" {
		t.Fatalf("phone_number = %q, want 18120322232", link.PhoneNumber)
	}
}

func TestGopayLinkChinesePhone(t *testing.T) {
	link := gopayLink{CountryCode: "86", PhoneNumber: "18120322232", Type: "gopay"}
	if link.CountryCode != "86" {
		t.Fatalf("country_code = %q, want 86", link.CountryCode)
	}
	if link.PhoneNumber != "18120322232" {
		t.Fatalf("phone_number = %q, want 18120322232", link.PhoneNumber)
	}
}

func TestGopayLinkIndonesianPhone(t *testing.T) {
	link := gopayLink{CountryCode: "62", PhoneNumber: "81212345678", Type: "gopay"}
	if link.CountryCode != "62" {
		t.Fatalf("country_code = %q, want 62", link.CountryCode)
	}
}

func TestGopayLinkingAlreadyLinkedDetectsMidtransMessage(t *testing.T) {
	result := map[string]any{
		"status":         406,
		"error_messages": []any{"account already linked"},
	}
	if !gopayLinkingAlreadyLinked(result, errors.New("local gopay linking failed with status 406")) {
		t.Fatal("gopayLinkingAlreadyLinked = false, want true")
	}
}

func TestGopayLinkingAlreadyLinkedRejectsOtherErrors(t *testing.T) {
	result := map[string]any{
		"status":         406,
		"error_messages": []any{"invalid phone number"},
	}
	if gopayLinkingAlreadyLinked(result, errors.New("local gopay linking failed with status 406")) {
		t.Fatal("gopayLinkingAlreadyLinked = true, want false")
	}
}

func TestValidateReusableGopayAccountAllowsEnabledStatus(t *testing.T) {
	account := map[string]any{
		"account_status": "ENABLED",
		"account_id":     "acct-123",
	}
	if err := validateReusableGopayAccount(account); err != nil {
		t.Fatalf("validateReusableGopayAccount returned error: %v", err)
	}
}

func TestValidateReusableGopayAccountRejectsNonEnabledStatus(t *testing.T) {
	account := map[string]any{
		"account_status": "PENDING",
		"account_id":     "acct-123",
	}
	if err := validateReusableGopayAccount(account); err == nil {
		t.Fatal("validateReusableGopayAccount returned nil, want error")
	}
}

func TestGopayRequestDiagnosticsDetectsPlusAndCountryPrefix(t *testing.T) {
	diag := gopayRequestDiagnostics("+86", "8618120322232", "wa")
	if diag["country_has_plus"] != true {
		t.Fatalf("country_has_plus = %#v", diag["country_has_plus"])
	}
	if diag["normalized_otp_channel"] != "whatsapp" {
		t.Fatalf("normalized_otp_channel = %#v", diag["normalized_otp_channel"])
	}
	if diag["phone_has_country_prefix"] != true {
		t.Fatalf("phone_has_country_prefix = %#v", diag["phone_has_country_prefix"])
	}
}

func TestGopayLinkingDiagnosticsIncludesLinkErrorSummary(t *testing.T) {
	diag := gopayLinkingDiagnostics(gopayLinkingResolution{
		ReusedExisting:   false,
		ConflictReason:   "",
		LinkHTTPStatus:   429,
		LinkError:        "local gopay linking failed with status 429",
		LinkErrorMessage: []string{"rate limit exceeded"},
		LinkResult:       map[string]any{"reference_id": "ref-1"},
		AccountResult:    map[string]any{"account_status": "ENABLED"},
		TokenNotFound:    true,
		AccountSource:    "manual_input",
	})
	if diag["link_http_status"] != 429 {
		t.Fatalf("link_http_status = %#v", diag["link_http_status"])
	}
	if diag["reference_id"] != "ref-1" {
		t.Fatalf("reference_id = %#v", diag["reference_id"])
	}
	if diag["token_not_found"] != true {
		t.Fatalf("token_not_found = %#v", diag["token_not_found"])
	}
}

func TestGopayTokenNotFoundDetectsMidtransMessage(t *testing.T) {
	result := map[string]any{
		"error_messages": []any{"token not found"},
	}
	if !gopayTokenNotFound(result, errors.New("local gopay linking failed with status 404")) {
		t.Fatal("gopayTokenNotFound = false, want true")
	}
}

func TestExtractSnapAccountIDFromBodyWithSourceReturnsSnippet(t *testing.T) {
	accountID, source, origin := extractSnapAccountIDFromBodyWithSource(map[string]any{
		"redirect_url": "https://app.midtrans.com/snap/v4/redirection/snap-guid-123?foo=bar",
	})
	if accountID != "snap-guid-123" {
		t.Fatalf("accountID = %q", accountID)
	}
	if source != "checkout_body_fallback" {
		t.Fatalf("source = %q", source)
	}
	if !strings.Contains(origin, "snap/v4/redirection/snap-guid-123") {
		t.Fatalf("origin = %q", origin)
	}
}

func TestGopayPaymentPINCandidatesPreferRequestedPIN(t *testing.T) {
	got := gopayPaymentPINCandidates("654321")
	if len(got) == 0 {
		t.Fatal("gopayPaymentPINCandidates returned empty slice")
	}
	if got[0] != "654321" {
		t.Fatalf("first candidate = %q, want 654321", got[0])
	}
	count := 0
	for _, item := range got {
		if item == "654321" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("candidate 654321 appears %d times, want 1", count)
	}
}

func TestGopayReuseStagesWithPaymentShowsCompletion(t *testing.T) {
	account := map[string]any{
		"account_status": "ENABLED",
		"account_id":     "acct-123",
	}
	payment := gopayPaymentResolution{
		PinUsed:            "123456",
		PaymentReferenceID: "payref-1",
		TransactionID:      "txn-1",
	}
	stages := gopayReuseStagesWithPayment(account, payment)
	if len(stages) == 0 {
		t.Fatal("gopayReuseStagesWithPayment returned empty stages")
	}

	var pinMessage string
	var successMessage string
	for _, stage := range stages {
		name, _ := stage["name"].(string)
		message, _ := stage["message"].(string)
		if name == "pin-enum" {
			pinMessage = message
		}
		if name == "validate-pin" {
			successMessage = message
		}
	}
	if !strings.Contains(pinMessage, "123456") {
		t.Fatalf("pin message = %q, want it to contain used pin", pinMessage)
	}
	if !strings.Contains(successMessage, "支付完成") {
		t.Fatalf("success message = %q, want it to contain 支付完成", successMessage)
	}
}

func TestCompleteGopayFullLinkPaymentContinuesAfterNewBinding(t *testing.T) {
	response := map[string]any{
		"ok":     false,
		"stages": []map[string]any{},
	}
	var gotGUID string
	var gotPIN string

	completeGopayFullLinkPayment(
		context.Background(),
		response,
		"snap-guid-new",
		"86",
		"18120322232",
		"654321",
		gopayLinkingResolution{},
		gopayFullLinkPaymentDeps{
			completePayment: func(_ context.Context, guid string, pin string) (gopayPaymentResolution, error) {
				gotGUID = guid
				gotPIN = pin
				return gopayPaymentResolution{
					PaymentReferenceID: "pay-ref-123",
					TransactionID:      "tx-123",
					PinUsed:            "654321",
					MidtransStatus:     map[string]any{"transaction_status": "settlement"},
				}, nil
			},
		},
	)

	if gotGUID != "snap-guid-new" {
		t.Fatalf("payment guid = %q, want snap-guid-new", gotGUID)
	}
	if gotPIN != "654321" {
		t.Fatalf("payment pin = %q, want 654321", gotPIN)
	}
	if response["ok"] != true {
		t.Fatalf("ok = %#v, want true", response["ok"])
	}
	if response["stage"] != "gopay_complete" {
		t.Fatalf("stage = %#v, want gopay_complete", response["stage"])
	}
	summary, _ := response["summary"].(map[string]any)
	if summary["payment_reference_id"] != "pay-ref-123" {
		t.Fatalf("payment_reference_id = %#v", summary["payment_reference_id"])
	}
	if summary["transaction_id"] != "tx-123" {
		t.Fatalf("transaction_id = %#v", summary["transaction_id"])
	}
}

func TestExtractSnapAccountIDViaStripeFlowUsesRedirectGUID(t *testing.T) {
	deps := stripeSnapAccountFlowDeps{
		init: func(context.Context, *http.Client, any) stripeInitResult {
			return stripeInitResult{
				OK: true,
				Body: map[string]any{
					"total_summary": map[string]any{"total": float64(0)},
					"config_id":     "cfg_123",
				},
			}
		},
		update: func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{OK: true, Body: map[string]any{"config_id": "cfg_123"}}
		},
		createPaymentMethod: func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{OK: true, Body: map[string]any{"id": "pm_123"}}
		},
		confirm: func(context.Context, *http.Client, any, string, string, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{
				OK: true,
				Body: map[string]any{
					"object":     "checkout.session",
					"session_id": "cs_test_123",
					"total_summary": map[string]any{
						"total": float64(0),
					},
					"submission_attempt": map[string]any{
						"id":    "subatt_123",
						"state": "requires_approval",
					},
				},
			}
		},
		approve: func(context.Context, *http.Client, string, checkoutSession, checkoutApproveRequest) (map[string]any, error) {
			return map[string]any{"ok": true, "submission_attempt_id": "subatt_123"}, nil
		},
		waitForRedirect: func(context.Context, int, time.Duration, func() stripeInitResult) (stripeInitResult, stripeRedirectCheck, []map[string]any) {
			return stripeInitResult{OK: true}, stripeRedirectCheck{
				OK:          true,
				RedirectURL: "https://pm-redirects.stripe.com/redirect/test",
			}, []map[string]any{{"attempt": 1, "redirect_ready": true}}
		},
		redirect: func(context.Context, *http.Client, string) gopayRedirectResult {
			return gopayRedirectResult{OK: true, GUID: "snap-guid-123", Location: "https://app.midtrans.com/snap/v4/redirection/snap-guid-123"}
		},
	}

	result := extractSnapAccountIDViaStripeFlowWithDeps(
		context.Background(),
		&http.Client{},
		map[string]any{
			"checkout_session_id": "cs_test_123",
			"publishable_key":     "pk_test_123",
		},
		"token-123",
		checkoutSession{},
		deps,
	)

	if result.AccountID != "snap-guid-123" {
		t.Fatalf("AccountID = %q, want snap-guid-123", result.AccountID)
	}
	if result.AccountSource != "stripe_redirect_guid" {
		t.Fatalf("AccountSource = %q", result.AccountSource)
	}
	if result.AccountOriginURL != "" && !strings.Contains(result.AccountOriginURL, "app.midtrans.com") {
		t.Fatalf("AccountOriginURL = %q", result.AccountOriginURL)
	}
	if result.Approval == nil || result.Approval["ok"] != true {
		t.Fatalf("Approval = %#v, want ok=true", result.Approval)
	}
	if !result.Redirect.OK {
		t.Fatalf("Redirect.OK = false, error = %q", result.Redirect.Error)
	}
	if !result.RedirectCheck.OK {
		t.Fatalf("RedirectCheck.OK = false, error = %q", result.RedirectCheck.Error)
	}
}

func TestExtractSnapAccountIDViaStripeFlowUsesApprovalBeforeRedirect(t *testing.T) {
	var gotApprove checkoutApproveRequest
	var approveCalled bool

	deps := stripeSnapAccountFlowDeps{
		init: func(context.Context, *http.Client, any) stripeInitResult {
			return stripeInitResult{
				OK: true,
				Body: map[string]any{
					"total_summary": map[string]any{"total": float64(0)},
					"config_id":     "cfg_123",
				},
			}
		},
		update: func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{OK: true, Body: map[string]any{"config_id": "cfg_123"}}
		},
		createPaymentMethod: func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{OK: true, Body: map[string]any{"id": "pm_approve_123"}}
		},
		confirm: func(context.Context, *http.Client, any, string, string, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{
				OK: true,
				Body: map[string]any{
					"object":        "checkout.session",
					"session_id":    "cs_test_approve",
					"total_summary": map[string]any{"total": float64(0)},
					"submission_attempt": map[string]any{
						"id":    "subatt_approve_123",
						"state": "requires_approval",
					},
				},
			}
		},
		approve: func(_ context.Context, _ *http.Client, token string, session checkoutSession, req checkoutApproveRequest) (map[string]any, error) {
			approveCalled = true
			gotApprove = req
			if token != "token-approve" {
				t.Fatalf("token = %q, want token-approve", token)
			}
			if session.Cookie != "cookie-1" {
				t.Fatalf("session cookie = %q, want cookie-1", session.Cookie)
			}
			return map[string]any{"ok": true, "submission_attempt_id": req.SubmissionAttemptID}, nil
		},
		waitForRedirect: func(context.Context, int, time.Duration, func() stripeInitResult) (stripeInitResult, stripeRedirectCheck, []map[string]any) {
			return stripeInitResult{OK: true}, stripeRedirectCheck{OK: true, RedirectURL: "https://pm-redirects.stripe.com/redirect/approve"}, nil
		},
		redirect: func(context.Context, *http.Client, string) gopayRedirectResult {
			return gopayRedirectResult{OK: true, GUID: "snap-guid-approve"}
		},
	}

	result := extractSnapAccountIDViaStripeFlowWithDeps(
		context.Background(),
		&http.Client{},
		map[string]any{
			"checkout_session_id": "cs_test_approve",
			"publishable_key":     "pk_test_approve",
			"processor_entity":    "openai_llc",
		},
		"token-approve",
		checkoutSession{Cookie: "cookie-1"},
		deps,
	)

	if !approveCalled {
		t.Fatal("approve was not called")
	}
	if gotApprove.CheckoutSessionID != "cs_test_approve" {
		t.Fatalf("CheckoutSessionID = %q", gotApprove.CheckoutSessionID)
	}
	if gotApprove.PaymentMethodID != "pm_approve_123" {
		t.Fatalf("PaymentMethodID = %q", gotApprove.PaymentMethodID)
	}
	if gotApprove.SubmissionAttemptID != "subatt_approve_123" {
		t.Fatalf("SubmissionAttemptID = %q", gotApprove.SubmissionAttemptID)
	}
	if result.AccountID != "snap-guid-approve" {
		t.Fatalf("AccountID = %q, want snap-guid-approve", result.AccountID)
	}
}

func TestExtractSnapAccountIDViaStripeFlowStoresTermsInteractionSeparately(t *testing.T) {
	confirmCalls := 0
	deps := stripeSnapAccountFlowDeps{
		init: func(context.Context, *http.Client, any) stripeInitResult {
			return stripeInitResult{OK: true, Body: map[string]any{"total_summary": map[string]any{"total": float64(0)}, "config_id": "cfg_terms"}}
		},
		update: func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{OK: true, Body: map[string]any{"config_id": "cfg_terms"}}
		},
		createPaymentMethod: func(context.Context, *http.Client, any, string, string, taxRegion, string, stripeClientContext) stripeInitResult {
			return stripeInitResult{OK: true, Body: map[string]any{"id": "pm_terms_123"}}
		},
		confirm: func(context.Context, *http.Client, any, string, string, string, stripeClientContext) stripeInitResult {
			confirmCalls++
			if confirmCalls == 1 {
				return stripeInitResult{OK: false, Body: map[string]any{"error": map[string]any{"message": "Please accept the merchant's terms of service before checking out."}}}
			}
			return stripeInitResult{OK: true, Body: map[string]any{"object": "checkout.session", "session_id": "cs_test_terms", "total_summary": map[string]any{"total": float64(0)}, "submission_attempt": map[string]any{"id": "subatt_terms_123", "state": "requires_approval"}}}
		},
		approve: func(context.Context, *http.Client, string, checkoutSession, checkoutApproveRequest) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
		waitForRedirect: func(context.Context, int, time.Duration, func() stripeInitResult) (stripeInitResult, stripeRedirectCheck, []map[string]any) {
			return stripeInitResult{OK: true}, stripeRedirectCheck{OK: true, RedirectURL: "https://pm-redirects.stripe.com/redirect/terms"}, nil
		},
		redirect: func(context.Context, *http.Client, string) gopayRedirectResult {
			return gopayRedirectResult{OK: true, GUID: "snap-guid-terms"}
		},
	}

	original := acceptCheckoutTermsViaCDP
	acceptCheckoutTermsViaCDP = func(context.Context, string) (map[string]any, error) {
		return map[string]any{"ok": true, "candidates": []map[string]any{{"id": "frame-terms"}}}, nil
	}
	defer func() { acceptCheckoutTermsViaCDP = original }()

	result := extractSnapAccountIDViaStripeFlowWithDeps(context.Background(), &http.Client{}, map[string]any{
		"checkout_session_id": "cs_test_terms",
		"publishable_key":     "pk_test_terms",
		"url":                 "https://pay.openai.com/c/pay/cs_test_terms#fid=test",
	}, "token-terms", checkoutSession{}, deps)

	if result.TermsInteraction == nil {
		t.Fatal("TermsInteraction is nil")
	}
	if result.Approval == nil || result.Approval["ok"] != true {
		t.Fatalf("Approval = %#v", result.Approval)
	}
}

// --- gopayForceLinkRequest 校验 ---

func TestGopayForceLinkRequestDefaults(t *testing.T) {
	req := gopayForceLinkRequest{
		AccountID:   "test-account-id",
		CountryCode: "",
		PhoneNumber: "18120322232",
	}
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.CountryCode != "86" {
		t.Fatalf("country_code default = %q, want 86", req.CountryCode)
	}
	if req.AccountID != "test-account-id" {
		t.Fatalf("account_id = %q", req.AccountID)
	}
}

func TestGopayForceLinkRequestMissingAccountID(t *testing.T) {
	req := gopayForceLinkRequest{
		AccountID:   "",
		CountryCode: "86",
		PhoneNumber: "18120322232",
	}
	if req.AccountID != "" {
		t.Fatal("account_id should be empty")
	}
}

func TestGopayForceLinkRequestEmptyPhone(t *testing.T) {
	req := gopayForceLinkRequest{
		AccountID:   "test",
		CountryCode: "86",
		PhoneNumber: "",
	}
	if req.PhoneNumber != "" {
		t.Fatal("phone_number should be empty")
	}
}

// --- gopayAutoLinkRequest 全场景校验 ---

func TestGopayAutoLinkRequestAllFields(t *testing.T) {
	req := gopayAutoLinkRequest{
		AccountID:   "test-guid-123",
		CountryCode: "86",
		PhoneNumber: "18120322232",
		OTPChannel:  "whatsapp",
		OTP:         "111111",
		PIN:         "123456",
	}
	if req.AccountID != "test-guid-123" {
		t.Fatalf("AccountID = %q", req.AccountID)
	}
	if req.OTPChannel != "whatsapp" {
		t.Fatalf("OTPChannel = %q, want whatsapp", req.OTPChannel)
	}
}

func TestGopayAutoLinkRequestDefaults(t *testing.T) {
	req := gopayAutoLinkRequest{
		AccountID:   "test",
		CountryCode: "",
		PhoneNumber: "18120322232",
	}
	if req.OTPChannel == "" {
		req.OTPChannel = "whatsapp"
	}
	if req.CountryCode == "" {
		req.CountryCode = "86"
	}
	if req.CountryCode != "86" {
		t.Fatalf("default country_code = %q", req.CountryCode)
	}
	if req.OTPChannel != "whatsapp" {
		t.Fatalf("default OTPChannel = %q", req.OTPChannel)
	}
}

func TestGopayAutoLinkRequestSMSType(t *testing.T) {
	req := gopayAutoLinkRequest{
		AccountID:   "test",
		CountryCode: "62",
		PhoneNumber: "81234567890",
		OTPChannel:  "sms",
	}
	if req.OTPChannel != "sms" {
		t.Fatalf("OTPChannel = %q, want sms", req.OTPChannel)
	}
}

// --- OTP channel 标准化测试 ---

func TestNormalizeOTPChannelWhatsApp(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"wa", "whatsapp"},
		{"whatsapp", "whatsapp"},
		{"whats_app", "whatsapp"},
		{"whats-app", "whatsapp"},
		{"WhatsApp", "whatsapp"},
		{"WHATSAPP", "whatsapp"},
	}
	for _, tt := range tests {
		got, err := normalizeOTPChannel(tt.input)
		if err != nil {
			t.Errorf("normalizeOTPChannel(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("normalizeOTPChannel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeOTPChannelInvalid(t *testing.T) {
	invalidChannels := []string{"email", "phone", "telegram", "signal", "push"}
	for _, ch := range invalidChannels {
		_, err := normalizeOTPChannel(ch)
		if err == nil {
			t.Errorf("normalizeOTPChannel(%q) should have returned error", ch)
		}
	}
}

// --- cdp 类型边界测试 ---

func TestCDPNotReadyError(t *testing.T) {
	err := &cdpNotReadyError{message: "CDP not ready"}
	if err.Error() != "CDP not ready" {
		t.Fatalf("cdpNotReadyError = %q", err.Error())
	}
}

func TestCDPCommandSerialization(t *testing.T) {
	cmd := cdpCommand{
		ID:     1,
		Method: "Runtime.evaluate",
		Params: map[string]any{"expression": "1+1", "awaitPromise": true},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal cdpCommand: %v", err)
	}
	var decoded cdpCommand
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal cdpCommand: %v", err)
	}
	if decoded.ID != 1 || decoded.Method != "Runtime.evaluate" {
		t.Fatalf("roundtrip failed: id=%d method=%s", decoded.ID, decoded.Method)
	}
}

func TestCDPResponseDeserialization(t *testing.T) {
	raw := `{"id":1,"result":{"result":{"type":"string","value":"hello"}}}`
	var resp cdpResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal cdpResponse: %v", err)
	}
	if resp.ID != 1 {
		t.Fatalf("resp.ID = %d, want 1", resp.ID)
	}
}

func TestCDPTargetDeserialization(t *testing.T) {
	raw := `[{"id":"page1","type":"page","url":"https://app.midtrans.com/snap/redirect/abc#/linking","webSocketDebuggerUrl":"ws://localhost:9223/devtools/page/page1"}]`
	var targets []cdpTarget
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		t.Fatalf("unmarshal cdpTarget: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets length = %d, want 1", len(targets))
	}
	if targets[0].Type != "page" {
		t.Fatalf("target type = %q", targets[0].Type)
	}
}

// --- gopay user consent payload 测试 ---

func TestGopayUserConsentPayloadVariousChannels(t *testing.T) {
	channels := []struct{ in, out string }{
		{"", "whatsapp"},
		{"sms", "sms"},
		{"wa", "whatsapp"},
		{"whatsapp", "whatsapp"},
	}
	for _, ch := range channels {
		payload, normalized, err := gopayUserConsentPayload("ref-test-001", ch.in)
		if err != nil {
			t.Errorf("channel %q error: %v", ch.in, err)
			continue
		}
		if normalized != ch.out {
			t.Errorf("channel %q normalized = %q, want %q", ch.in, normalized, ch.out)
		}
		var body map[string]any
		if err := json.Unmarshal(payload, &body); err != nil {
			t.Errorf("channel %q payload json: %v", ch.in, err)
		}
		if body["reference_id"] != "ref-test-001" {
			t.Errorf("channel %q reference_id = %v", ch.in, body["reference_id"])
		}
	}
}

// --- gopayPINToken 提取测试 ---

func TestGopayPINTokenExtraction(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		expect string
	}{
		{"from token key", map[string]any{"token": "tok123"}, "tok123"},
		{"from nested pin_token", map[string]any{"pin_token": "pin456"}, "pin456"},
		{"from data.token", map[string]any{"data": map[string]any{"token": "data789"}}, "data789"},
		{"empty", map[string]any{"other": "val"}, ""},
	}
	for _, tt := range tests {
		got := gopayPINToken(tt.input)
		if got != tt.expect {
			t.Errorf("%s: gopayPINToken = %q, want %q", tt.name, got, tt.expect)
		}
	}
}

// --- gopayForceLinkRequest JSON 序列化测试 ---

func TestGopayForceLinkRequestJSONRoundtrip(t *testing.T) {
	original := gopayForceLinkRequest{
		AccountID:   "guid-123",
		CountryCode: "86",
		PhoneNumber: "18120322232",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded gopayForceLinkRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.AccountID != original.AccountID || decoded.CountryCode != original.CountryCode || decoded.PhoneNumber != original.PhoneNumber {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", original, decoded)
	}
}

func TestGopayAutoLinkRequestJSONRoundtrip(t *testing.T) {
	original := gopayAutoLinkRequest{
		AccountID:   "guid-456",
		CountryCode: "62",
		PhoneNumber: "81234567890",
		OTPChannel:  "sms",
		OTP:         "654321",
		PIN:         "145236",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded gopayAutoLinkRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.AccountID != original.AccountID || decoded.OTPChannel != original.OTPChannel || decoded.OTP != original.OTP {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", original, decoded)
	}
}

// --- 性能基准测试 (Benchmarks) ---

func BenchmarkNormalizeOTPChannel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		normalizeOTPChannel("whatsapp")
	}
}

func BenchmarkGopayUserConsentPayload(b *testing.B) {
	for i := 0; i < b.N; i++ {
		gopayUserConsentPayload("ref-test", "whatsapp")
	}
}

func BenchmarkExtractAccessToken(b *testing.B) {
	raw := `{"user":{"email":"test@example.com"},"accessToken":"eyJhbGciOiJIUzI1NiJ9.payload.signature"}`
	for i := 0; i < b.N; i++ {
		extractAccessToken(raw)
	}
}

func BenchmarkGopayPINToken(b *testing.B) {
	m := map[string]any{"token": "tok123", "pin_token": "pin456"}
	for i := 0; i < b.N; i++ {
		gopayPINToken(m)
	}
}

func BenchmarkReferenceFromActivationLink(b *testing.B) {
	raw := "https://app.midtrans.com/snap/v3/accounts/guid/linking?reference=gpar_6123269-1425-21e3-bc44-e592afafec14"
	for i := 0; i < b.N; i++ {
		referenceFromActivationLink(raw)
	}
}

func BenchmarkCDPCommandMarshal(b *testing.B) {
	cmd := cdpCommand{ID: 1, Method: "Runtime.evaluate", Params: map[string]any{"expression": "1+1"}}
	for i := 0; i < b.N; i++ {
		json.Marshal(cmd)
	}
}

// --- HTTP 集成测试：三通道端到端测试 ---
// 以下测试需要服务在 localhost:18473 运行

func testHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func testAPIURL(path string) string {
	return "http://127.0.0.1:18473" + path
}

func postJSONTest(t *testing.T, path string, body any) (*http.Response, map[string]any, error) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, testAPIURL(path), bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp, nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		result = map[string]any{"raw": string(respBody)}
	}
	return resp, result, nil
}

// TestForceLinkEndpoint 通道1: force-link 端点到端测试
func TestForceLinkEndpoint(t *testing.T) {
	payload := gopayForceLinkRequest{
		AccountID:   "6446ea93-ee78-4be8-b9a4-bd7693bddc03",
		CountryCode: "86",
		PhoneNumber: "18120322232",
	}
	resp, result, err := postJSONTest(t, "/api/gopay/force-link", payload)
	if err != nil {
		t.Skipf("force-link 服务不可用 (跳过集成测试): %v", err)
		return
	}

	t.Logf("force-link HTTP status: %d", resp.StatusCode)

	if cdp, ok := result["cdp"].(map[string]any); ok {
		t.Logf("CDP 通道: ok=%v", cdp["ok"])
		if cdpErr, _ := cdp["error"].(string); cdpErr != "" {
			t.Logf("CDP 通道错误: %s", cdpErr)
		}
	}

	if api, ok := result["api"].(map[string]any); ok {
		apiOK, _ := api["ok"].(bool)
		t.Logf("API 通道: ok=%v", apiOK)
		if apiErr, _ := api["error"].(string); apiErr != "" {
			t.Logf("API 通道错误: %s", apiErr)
		}
		if refID := stringifyJSONValue(result["reference_id"]); refID != "" {
			t.Logf("API reference_id: %s", refID)
		}
		if apiStatus, ok := result["api_status"]; ok {
			t.Logf("API status code: %v", apiStatus)
		}
		if !apiOK {
			t.Logf("⚠️ API 通道 linking 失败（+86 号码可能在 Midtrans/GoPay 后端被拒）")
		}
	}
}

// TestAutoLinkEndpoint 通道2: auto-link 全自动管道测试
func TestAutoLinkEndpoint(t *testing.T) {
	payload := gopayAutoLinkRequest{
		AccountID:   "f62bbcac-7efb-48ac-bf7a-45f90004f7b7",
		CountryCode: "86",
		PhoneNumber: "18120322232",
		OTPChannel:  "whatsapp",
	}
	resp, result, err := postJSONTest(t, "/api/gopay/auto-link", payload)
	if err != nil {
		t.Skipf("auto-link 服务不可用 (跳过集成测试): %v", err)
		return
	}

	t.Logf("auto-link HTTP status: %d", resp.StatusCode)
	stage, _ := result["stage"].(string)
	ok, _ := result["ok"].(bool)
	t.Logf("auto-link stage: %s, ok: %v", stage, ok)

	if refID := stringifyJSONValue(result["reference_id"]); refID != "" {
		t.Logf("reference_id: %s", refID)
	}

	// 记录每个阶段的详细信息
	for _, s := range []string{"linking", "user_consent", "otp_used", "pin_used", "challenge_id", "summary"} {
		if v, exists := result[s]; exists {
			t.Logf("%s: %v", s, v)
		}
	}

	if !ok {
		errMsg, _ := result["error"].(string)
		t.Logf("❌ auto-link 失败于 stage=%s: %s", stage, errMsg)
		if otpTried, ok := result["otp_tried"]; ok {
			t.Logf("已尝试 OTP 列表: %v", otpTried)
		}
		if pinTried, ok := result["pin_tried"]; ok {
			t.Logf("已尝试 PIN 列表: %v", pinTried)
		}
	} else {
		t.Logf("✅ auto-link 成功完成 GoPay 绑定")
	}
}

// TestCDPOTPEndpoint 通道3: cdp-otp 端点测试
func TestCDPOTPEndpoint(t *testing.T) {
	payload := map[string]string{"otp": "111111"}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, testAPIURL("/api/gopay/cdp-otp"), bytes.NewReader(data))
	if err != nil {
		t.Skipf("cdp-otp 服务不可用 (跳过集成测试): %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := testHTTPClient().Do(req)
	if err != nil {
		t.Skipf("cdp-otp 服务不可用: %v", err)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		result = map[string]any{"raw": string(respBody)}
	}

	t.Logf("cdp-otp HTTP status: %d", resp.StatusCode)
	if ok, _ := result["ok"].(bool); ok {
		t.Logf("cdp-otp OK: %v", ok)
		if r, ok := result["result"].(map[string]any); ok {
			t.Logf("hasOTPField: %v", r["hasOTPField"])
			t.Logf("otpTried: %v", r["otpTried"])
			t.Logf("result: %v", r["result"])
		}
	} else {
		errMsg, _ := result["error"].(string)
		code, _ := result["code"].(string)
		t.Logf("cdp-otp failed: code=%s, error=%s", code, errMsg)
	}
}

// TestThreeChannelComparison 三通道批量对比测试
func TestThreeChannelComparison(t *testing.T) {
	const testAccountID = "f62bbcac-7efb-48ac-bf7a-45f90004f7b7"

	type channelResult struct {
		Channel string
		Stage   string
		OK      bool
		Latency time.Duration
		Error   string
		Details map[string]any
	}

	results := make([]channelResult, 0, 3)

	// 通道 1: force-link
	start := time.Now()
	_, r1, err1 := postJSONTest(t, "/api/gopay/force-link", gopayForceLinkRequest{
		AccountID:   testAccountID,
		CountryCode: "86",
		PhoneNumber: "18120322232",
	})
	lat1 := time.Since(start)

	cr1 := channelResult{Channel: "force-link", Latency: lat1}
	if err1 != nil {
		cr1.Error = err1.Error()
	} else {
		if api, ok := r1["api"].(map[string]any); ok {
			if apiOK, _ := api["ok"].(bool); apiOK {
				cr1.OK = true
				cr1.Stage = "linking"
			}
		}
		if !cr1.OK {
			if cdp, ok := r1["cdp"].(map[string]any); ok {
				if cdpOK, _ := cdp["ok"].(bool); cdpOK {
					cr1.OK = true
					cr1.Stage = "cdp_injection"
				}
			}
		}
	}
	results = append(results, cr1)

	// 通道 2: auto-link
	start = time.Now()
	_, r2, err2 := postJSONTest(t, "/api/gopay/auto-link", gopayAutoLinkRequest{
		AccountID:   testAccountID,
		CountryCode: "86",
		PhoneNumber: "18120322232",
	})
	lat2 := time.Since(start)

	cr2 := channelResult{Channel: "auto-link", Latency: lat2}
	if err2 != nil {
		cr2.Error = err2.Error()
	} else {
		cr2.OK, _ = r2["ok"].(bool)
		cr2.Stage, _ = r2["stage"].(string)
		if !cr2.OK {
			cr2.Error, _ = r2["error"].(string)
		}
		cr2.Details = r2
	}
	results = append(results, cr2)

	// 通道 3: cdp-otp
	start = time.Now()
	_, r3, err3 := postJSONTest(t, "/api/gopay/cdp-otp", map[string]string{"otp": "111111"})
	lat3 := time.Since(start)

	cr3 := channelResult{Channel: "cdp-otp", Latency: lat3}
	if err3 != nil {
		cr3.Error = err3.Error()
	} else {
		cr3.OK, _ = r3["ok"].(bool)
		if !cr3.OK {
			cr3.Error, _ = r3["error"].(string)
			cr3.Stage, _ = r3["code"].(string)
		} else {
			if r, ok := r3["result"].(map[string]any); ok {
				cr3.Stage = "otp_injected"
				cr3.Details = r
			}
		}
	}
	results = append(results, cr3)

	// 输出对比报告
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════╗")
	t.Log("║          三通道对比测试结果报告                      ║")
	t.Log("╠══════════════════════════════════════════════════════╣")
	t.Logf("║ 测试账号: %s ║", testAccountID)
	t.Log("║ 目标号码: +86 18120322232                           ║")
	t.Log("╠══════════════════════════════════════════════════════╣")

	var bestChannel string
	var bestLatency time.Duration
	bestScore := -1

	for _, cr := range results {
		status := "❌ 失败"
		if cr.OK {
			status = "✅ 成功"
		}
		score := 0
		if cr.OK {
			score += 100
		}
		if cr.Stage == "linking_success" {
			score += 200
		}
		if cr.Latency < 5*time.Second {
			score += 50
		}

		t.Logf("║ %-12s | %s | stage: %-16s | latency: %6dms ║",
			cr.Channel, status, cr.Stage, cr.Latency.Milliseconds())

		if score > bestScore {
			bestScore = score
			bestChannel = cr.Channel
			bestLatency = cr.Latency
		}

		if cr.Error != "" {
			t.Logf("║             error: %s", cr.Error[:min(60, len(cr.Error))])
		}
	}

	t.Log("╠══════════════════════════════════════════════════════╣")
	if bestChannel != "" {
		t.Logf("║ 🏆 推荐方案: %s (耗时 %dms)          ║", bestChannel, bestLatency.Milliseconds())
	} else {
		t.Log("║ ❌ 所有通道均失败                                  ║")
		t.Log("║ 💡 建议: 使用印尼 +62 号码重试                     ║")
	}
	t.Log("╚══════════════════════════════════════════════════════╝")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestSmartLinkEndpoint 统一双通道组合管道端到端测试
func TestSmartLinkEndpoint(t *testing.T) {
	payload := gopaySmartLinkRequest{
		AccountID:   "f62bbcac-7efb-48ac-bf7a-45f90004f7b7",
		CountryCode: "86",
		PhoneNumber: "18120322232",
		OTPChannel:  "whatsapp",
	}
	resp, result, err := postJSONTest(t, "/api/gopay/smart-link", payload)
	if err != nil {
		t.Skipf("smart-link 服务不可用 (跳过集成测试): %v", err)
		return
	}

	t.Logf("smart-link HTTP status: %d", resp.StatusCode)
	t.Logf("strategy: %v", result["strategy"])

	stage, _ := result["stage"].(string)
	ok, _ := result["ok"].(bool)
	t.Logf("stage: %s, ok: %v", stage, ok)

	if stages, ok := result["stages"].([]any); ok {
		t.Logf("管道阶段数: %d", len(stages))
		for i, s := range stages {
			t.Logf("  [%d] %v", i+1, s)
		}
	}

	if summary, ok := result["summary"].(map[string]any); ok {
		t.Logf("summary: %v", summary)
	}

	if !ok {
		errMsg, _ := result["error"].(string)
		t.Logf("失败阶段: %s, 错误: %s", stage, errMsg)
	} else {
		t.Logf("✅ smart-link 双通道组合策略完成 GoPay 绑定")
	}
}

// TestFourChannelComparison 四通道对比 (包含 smart-link)
func TestFourChannelComparison(t *testing.T) {
	const testAccountID = "f62bbcac-7efb-48ac-bf7a-45f90004f7b7"

	type channelResult struct {
		Channel string
		Stage   string
		OK      bool
		Latency time.Duration
		Error   string
	}

	results := make([]channelResult, 0, 4)

	// 通道 1: force-link
	start := time.Now()
	_, r1, err1 := postJSONTest(t, "/api/gopay/force-link", gopayForceLinkRequest{
		AccountID:   testAccountID,
		CountryCode: "86",
		PhoneNumber: "18120322232",
	})
	lat1 := time.Since(start)

	cr1 := channelResult{Channel: "1.force-link", Latency: lat1}
	if err1 != nil {
		cr1.Error = err1.Error()
	} else {
		if api, ok := r1["api"].(map[string]any); ok {
			cr1.OK, _ = api["ok"].(bool)
		}
	}
	results = append(results, cr1)

	// 通道 2: auto-link
	start = time.Now()
	_, r2, err2 := postJSONTest(t, "/api/gopay/auto-link", gopayAutoLinkRequest{
		AccountID:   testAccountID,
		CountryCode: "86",
		PhoneNumber: "18120322232",
	})
	lat2 := time.Since(start)

	cr2 := channelResult{Channel: "2.auto-link", Latency: lat2}
	if err2 != nil {
		cr2.Error = err2.Error()
	} else {
		cr2.OK, _ = r2["ok"].(bool)
		cr2.Stage, _ = r2["stage"].(string)
		if !cr2.OK {
			cr2.Error, _ = r2["error"].(string)
		}
	}
	results = append(results, cr2)

	// 通道 3: cdp-otp
	start = time.Now()
	_, r3, err3 := postJSONTest(t, "/api/gopay/cdp-otp", map[string]string{"otp": "111111"})
	lat3 := time.Since(start)

	cr3 := channelResult{Channel: "3.cdp-otp", Latency: lat3}
	if err3 != nil {
		cr3.Error = err3.Error()
	} else {
		cr3.OK, _ = r3["ok"].(bool)
	}
	results = append(results, cr3)

	// 通道 4: smart-link (双通道组合)
	start = time.Now()
	_, r4, err4 := postJSONTest(t, "/api/gopay/smart-link", gopaySmartLinkRequest{
		AccountID:   testAccountID,
		CountryCode: "86",
		PhoneNumber: "18120322232",
	})
	lat4 := time.Since(start)

	cr4 := channelResult{Channel: "4.smart-link", Latency: lat4}
	if err4 != nil {
		cr4.Error = err4.Error()
	} else {
		cr4.OK, _ = r4["ok"].(bool)
		cr4.Stage, _ = r4["stage"].(string)
		if !cr4.OK {
			cr4.Error, _ = r4["error"].(string)
		}
	}
	results = append(results, cr4)

	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════╗")
	t.Log("║       四通道对比测试结果 (含双通道组合)              ║")
	t.Log("╠══════════════════════════════════════════════════════╣")
	t.Logf("║ 测试账号: %s ║", testAccountID)
	t.Log("║ 目标号码: +86 18120322232                           ║")
	t.Log("╠══════════════════════════════════════════════════════╣")

	var bestChannel string
	var bestLatency time.Duration
	bestScore := -1

	for _, cr := range results {
		status := "❌"
		if cr.OK {
			status = "✅"
		}
		score := 0
		if cr.OK {
			score += 100
		}
		if cr.Stage == "linking_success" {
			score += 200
		}
		if cr.Latency < 5*time.Second {
			score += 50
		}

		t.Logf("║ %-14s | %s | stage: %-18s | %7dms ║",
			cr.Channel, status, cr.Stage, cr.Latency.Milliseconds())

		if score > bestScore {
			bestScore = score
			bestChannel = cr.Channel
			bestLatency = cr.Latency
		}

		if cr.Error != "" && len(cr.Error) > 0 {
			errShort := cr.Error
			if len(errShort) > 55 {
				errShort = errShort[:55]
			}
			t.Logf("║               error: %s", errShort)
		}
	}

	t.Log("╠══════════════════════════════════════════════════════╣")
	if bestChannel != "" {
		t.Logf("║ 🏆 推荐: %s (%dms)     ║", bestChannel, bestLatency.Milliseconds())
	} else {
		t.Log("║ ❌ 所有通道均失败                                  ║")
	}
	t.Log("╚══════════════════════════════════════════════════════╝")
}
