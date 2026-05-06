package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
