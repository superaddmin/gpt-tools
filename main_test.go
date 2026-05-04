package main

import (
	"context"
	"encoding/json"
	"net/http"
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
