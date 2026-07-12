package handlers

import (
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Email-OTP mobile sign-in. A mobile client cannot run the web's Next.js
// send-otp route (cookies + next-auth), so the same passwordless flow is served
// here against ws-backend directly:
//
//	POST /auth/mobile/otp/request { email }        -> { otp_token }  (+ emails a 6-digit code)
//	POST /auth/mobile/otp/verify  { otp_token, code } -> the standard mobileAuthResponse
//
// The pending code lives only as an HMAC inside the signed otp_token handed back
// to the client (mirroring the web cookie), so the flow needs no new DB table.

const (
	otpEmailSubject = "Your WikiSubmission sign-in code"
	// otpRateWindow / limits mirror the web send-otp route.
	otpRateWindow    = 10 * time.Minute
	otpPerIPLimit    = 5
	otpPerEmailLimit = 3
)

type mobileOTPRequestBody struct {
	Email string `json:"email"`
}

type mobileOTPRequestResponse struct {
	OTPToken string `json:"otp_token"`
}

type mobileOTPVerifyBody struct {
	OTPToken string `json:"otp_token"`
	Code     string `json:"code"`
}

// OtpRequest generates a code, emails it, and returns a pending otp_token.
// POST /api/v1/auth/mobile/otp/request
func (h *NativeAuthHandler) OtpRequest(c *gin.Context) {
	if h.OTP == nil || !h.OTP.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email sign-in not configured"})
		return
	}

	var req mobileOTPRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
		return
	}
	emailAddr := strings.ToLower(addr.Address)

	if h.ipLimiter().exceeded(clientIP(c), otpPerIPLimit, otpRateWindow, h.now()) ||
		h.emailLimiter().exceeded(emailAddr, otpPerEmailLimit, otpRateWindow, h.now()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
		return
	}

	code, err := h.OTP.GenerateCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	token, err := h.OTP.IssueToken(emailAddr, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if h.Mailer != nil {
		body := "Your WikiSubmission sign-in code is: " + code +
			"\n\nThis code expires in 10 minutes."
		if err := h.Mailer.Send(c.Request.Context(), emailAddr, otpEmailSubject, body); err != nil {
			_ = c.Error(err) // surfaced via the request error logger middleware
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send code"})
			return
		}
	} else if h.OTPDevLog {
		// Dev-only: log the code without the email so logs never hold an
		// exploitable email+code pair, matching the web dev path.
		slog.Warn("email OTP dev mode: code generated", "code", code)
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email delivery not configured"})
		return
	}

	c.JSON(http.StatusOK, mobileOTPRequestResponse{OTPToken: token})
}

// OtpVerify checks the code against the pending token and returns a session.
// POST /api/v1/auth/mobile/otp/verify
func (h *NativeAuthHandler) OtpVerify(c *gin.Context) {
	if h.OTP == nil || !h.OTP.Configured() || h.Minter == nil || !h.Minter.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "email sign-in not configured"})
		return
	}

	var req mobileOTPVerifyBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.OTPToken) == "" || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "otp_token and code are required"})
		return
	}

	emailAddr, err := h.OTP.Verify(strings.TrimSpace(req.OTPToken), strings.TrimSpace(req.Code))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired code"})
		return
	}

	authID := "email:" + emailAddr
	user, err := h.Store.UpsertUser(c.Request.Context(), authID, emailAddr, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is not active"})
		return
	}

	h.respondWithSession(c, user)
}

func (h *NativeAuthHandler) ipLimiter() *rateLimiter {
	if h.otpIPLimiter == nil {
		h.otpIPLimiter = newRateLimiter()
	}
	return h.otpIPLimiter
}

func (h *NativeAuthHandler) emailLimiter() *rateLimiter {
	if h.otpEmailLimiter == nil {
		h.otpEmailLimiter = newRateLimiter()
	}
	return h.otpEmailLimiter
}

// clientIP returns gin's resolved client IP. gin only honours X-Forwarded-For
// when the peer is a configured trusted proxy (see SetTrustedProxies in
// server.go), so an untrusted client cannot spoof the header to evade the
// per-IP OTP rate limit. Reading the raw header directly, as this used to,
// let any caller forge an arbitrary source IP per request.
func clientIP(c *gin.Context) string {
	return c.ClientIP()
}

// rateLimiter is a small fixed-window in-memory limiter, matching the web
// send-otp route's semantics. Note: like the web version, this is per-instance;
// a multi-instance deployment would need a shared store (e.g. Redis).
type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]*windowEntry
}

type windowEntry struct {
	count int
	reset time.Time
}

func newRateLimiter() *rateLimiter {
	r := &rateLimiter{entries: make(map[string]*windowEntry)}
	go r.cleanup()
	return r
}

// cleanup evicts expired windows every 10 minutes so the entry map cannot grow
// without bound (each distinct key — client IP or email — otherwise lingers
// forever after its window closes).
func (r *rateLimiter) cleanup() {
	t := time.NewTicker(10 * time.Minute)
	for range t.C {
		r.mu.Lock()
		now := time.Now()
		for k, e := range r.entries {
			if now.After(e.reset) {
				delete(r.entries, k)
			}
		}
		r.mu.Unlock()
	}
}

// exceeded records a hit for key and reports whether it is over max within the
// window. The first call in a new window resets the counter.
func (r *rateLimiter) exceeded(key string, max int, window time.Duration, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[key]
	if !ok || now.After(e.reset) {
		r.entries[key] = &windowEntry{count: 1, reset: now.Add(window)}
		return false
	}
	if e.count >= max {
		return true
	}
	e.count++
	return false
}
