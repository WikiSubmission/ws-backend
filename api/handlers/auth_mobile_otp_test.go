package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// recordingMailer captures the last message so a test can assert on delivery.
type recordingMailer struct {
	to, subject, body string
	sends             int
	err               error
}

func (m *recordingMailer) Send(_ context.Context, to, subject, body string) error {
	if m.err != nil {
		return m.err
	}
	m.to, m.subject, m.body = to, subject, body
	m.sends++
	return nil
}

func newOTPHandler(t *testing.T, store nativeAuthStore, mailer *recordingMailer) *NativeAuthHandler {
	t.Helper()
	h := newTestHandler(t, store, nil, nil)
	otp, err := auth.NewOTPManager(mobileTestSecret, time.Minute)
	if err != nil {
		t.Fatalf("NewOTPManager: %v", err)
	}
	h.OTP = otp
	if mailer != nil {
		h.Mailer = mailer
	}
	return h
}

func doOTPRoute(t *testing.T, h *NativeAuthHandler, route string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	switch route {
	case "request":
		r.POST("/api/v1/auth/mobile/otp/request", h.OtpRequest)
	case "verify":
		r.POST("/api/v1/auth/mobile/otp/verify", h.OtpVerify)
	default:
		t.Fatalf("unknown route %q", route)
	}
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mobile/otp/"+route, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeInto(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
}

var codeRe = regexp.MustCompile(`\b(\d{6})\b`)

func extractCode(t *testing.T, body string) string {
	t.Helper()
	m := codeRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no 6-digit code in body: %q", body)
	}
	return m[1]
}

func TestOTPRequestAndVerifyFlow(t *testing.T) {
	store := &fakeStore{}
	mailer := &recordingMailer{}
	h := newOTPHandler(t, store, mailer)

	w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "User@Example.com"})
	if w.Code != http.StatusOK {
		t.Fatalf("request status = %d, body = %s", w.Code, w.Body.String())
	}
	var reqResp mobileOTPRequestResponse
	decodeInto(t, w, &reqResp)
	if reqResp.OTPToken == "" {
		t.Fatal("expected an otp_token")
	}
	if mailer.sends != 1 || mailer.to != "user@example.com" {
		t.Fatalf("unexpected mail: %+v", mailer)
	}

	code := extractCode(t, mailer.body)

	w2 := doOTPRoute(t, h, "verify", mobileOTPVerifyBody{OTPToken: reqResp.OTPToken, Code: code})
	if w2.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body = %s", w2.Code, w2.Body.String())
	}
	resp := decodeAuth(t, w2)
	if resp.User.AuthID != "email:user@example.com" || resp.User.Email != "user@example.com" {
		t.Fatalf("unexpected user: %+v", resp.User)
	}

	v, _ := auth.NewAuthJSVerifier(mobileTestSecret)
	if _, _, _, err := v.VerifyToken(resp.AccessToken); err != nil {
		t.Fatalf("minted access token invalid: %v", err)
	}
}

func TestOTPVerifyWrongCode(t *testing.T) {
	mailer := &recordingMailer{}
	h := newOTPHandler(t, &fakeStore{}, mailer)
	w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "a@b.co"})
	var reqResp mobileOTPRequestResponse
	decodeInto(t, w, &reqResp)

	w2 := doOTPRoute(t, h, "verify", mobileOTPVerifyBody{OTPToken: reqResp.OTPToken, Code: "000000"})
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("wrong code status = %d, want 401", w2.Code)
	}
}

func TestOTPRequestInvalidEmail(t *testing.T) {
	h := newOTPHandler(t, &fakeStore{}, &recordingMailer{})
	w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "not-an-email"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestOTPRequestRateLimitPerEmail(t *testing.T) {
	h := newOTPHandler(t, &fakeStore{}, &recordingMailer{})
	for i := 0; i < otpPerEmailLimit; i++ {
		w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "rl@b.co"})
		if w.Code != http.StatusOK {
			t.Fatalf("send %d status = %d, want 200", i, w.Code)
		}
	}
	w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "rl@b.co"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("over-limit status = %d, want 429", w.Code)
	}
}

func TestOTPDisabledWithoutManager(t *testing.T) {
	h := newTestHandler(t, &fakeStore{}, nil, nil) // no OTP manager
	w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "a@b.co"})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestOTPRequestNoMailerNoDevLog(t *testing.T) {
	h := newOTPHandler(t, &fakeStore{}, nil) // OTP configured, Mailer nil, OTPDevLog false
	w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "a@b.co"})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestOTPVerifyInactiveUser(t *testing.T) {
	store := &fakeStore{template: &db.UserRow{ID: 5, Role: "member", IsActive: false}}
	mailer := &recordingMailer{}
	h := newOTPHandler(t, store, mailer)
	w := doOTPRoute(t, h, "request", mobileOTPRequestBody{Email: "a@b.co"})
	var reqResp mobileOTPRequestResponse
	decodeInto(t, w, &reqResp)
	code := extractCode(t, mailer.body)

	w2 := doOTPRoute(t, h, "verify", mobileOTPVerifyBody{OTPToken: reqResp.OTPToken, Code: code})
	if w2.Code != http.StatusForbidden {
		t.Fatalf("inactive verify status = %d, want 403", w2.Code)
	}
}
