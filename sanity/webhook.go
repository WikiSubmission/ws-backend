package sanity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader is the header Sanity sends with GROQ-powered webhooks.
const SignatureHeader = "Sanity-Webhook-Signature"

// WebhookPayload is the projection ws-backend asks Sanity to deliver. Configure
// the webhook in Sanity Studio with the projection:
//
//	{ "_id": _id, "_type": _type, "operation": delta::operation() }
type WebhookPayload struct {
	ID        string `json:"_id"`
	Type      string `json:"_type"`
	Operation string `json:"operation"` // "create" | "update" | "delete"
}

// MaxSignatureAge bounds how old a webhook signature may be. Anything older
// is rejected as a replay candidate.
const MaxSignatureAge = 5 * time.Minute

// VerifyWebhook validates the Sanity-Webhook-Signature header against the raw
// body using the configured shared secret. Returns the parsed payload on
// success.
//
// Header format (Sanity GROQ webhooks): "t=<unix-millis>,v1=<base64url-hmac>".
// The HMAC is computed as HMAC-SHA256(secret, "<t>.<body>").
func (c *Client) VerifyWebhook(headerVal string, body []byte) (WebhookPayload, error) {
	if c.cfg.WebhookSecret == "" {
		return WebhookPayload{}, errors.New("sanity: SANITY_WEBHOOK_SECRET is not configured")
	}
	ts, sig, err := parseSignatureHeader(headerVal)
	if err != nil {
		return WebhookPayload{}, err
	}
	// Reject stale signatures. ts is unix-millis.
	delivered := time.UnixMilli(ts)
	if time.Since(delivered) > MaxSignatureAge {
		return WebhookPayload{}, errors.New("sanity: webhook signature too old")
	}

	mac := hmac.New(sha256.New, []byte(c.cfg.WebhookSecret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := mac.Sum(nil)

	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		// Fall back to standard base64 in case the encoder differs.
		got, err = base64.StdEncoding.DecodeString(sig)
		if err != nil {
			return WebhookPayload{}, errors.New("sanity: webhook signature is not valid base64")
		}
	}
	if !hmac.Equal(expected, got) {
		return WebhookPayload{}, errors.New("sanity: webhook signature mismatch")
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return WebhookPayload{}, errors.New("sanity: webhook body is not valid JSON")
	}
	return payload, nil
}

func parseSignatureHeader(h string) (int64, string, error) {
	if h == "" {
		return 0, "", errors.New("sanity: missing webhook signature header")
	}
	var ts int64
	var sig string
	for _, part := range strings.Split(h, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			parsed, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return 0, "", errors.New("sanity: webhook timestamp is not numeric")
			}
			ts = parsed
		case "v1":
			sig = kv[1]
		}
	}
	if ts == 0 || sig == "" {
		return 0, "", errors.New("sanity: webhook signature is missing required components")
	}
	return ts, sig, nil
}
