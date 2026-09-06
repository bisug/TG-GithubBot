package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github-webhook/internal/config"
	"github-webhook/internal/utils"
)

// newAppWebhookTestServer builds a WebhookServer with an encryption key and
// app webhook secret suitable for tests. Returns the server, an encrypted
// chat-ID token, and the encryption key.
func newAppWebhookTestServer(t *testing.T) (*WebhookServer, string, string) {
	t.Helper()
	const key = "0123456789abcdef0123456789abcdef"
	cfg := &config.Config{
		EncryptionKey:          key,
		GitHubWebhookSecret:    "repo-secret",
		GitHubAppWebhookSecret: "app-secret",
	}
	token, err := utils.Encrypt("424242", key)
	if err != nil {
		t.Fatalf("encrypt chat id: %v", err)
	}
	s := NewWebhookServer(cfg, nil, nil, nil, nil)
	return s, token, key
}

// signBody computes the X-Hub-Signature-256 header value for a payload.
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestAppWebhookAcceptsValidDelivery(t *testing.T) {
	s, token, _ := newAppWebhookTestServer(t)

	payload, _ := json.Marshal(map[string]any{
		"action": "created",
		"installation": map[string]any{
			"id":      123,
			"account": map[string]any{"login": "octocat"},
		},
		"sender": map[string]any{"login": "octocat"},
	})

	req := httptest.NewRequest(http.MethodPost, "/app-webhook/"+token, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "installation")
	req.Header.Set("X-GitHub-Delivery", "app-delivery-1")
	req.Header.Set("X-Hub-Signature-256", signBody("app-secret", payload))

	rec := httptest.NewRecorder()
	s.AppHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body %q)", rec.Code, rec.Body.String())
	}
}

func TestAppWebhookRejectsRepoSecretSignature(t *testing.T) {
	s, token, _ := newAppWebhookTestServer(t)

	payload := []byte(`{"action":"created"}`)
	req := httptest.NewRequest(http.MethodPost, "/app-webhook/"+token, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "installation")
	req.Header.Set("X-GitHub-Delivery", "app-delivery-2")
	// Signed with the REPO secret: must be rejected because the app endpoint
	// verifies with GITHUB_APP_WEBHOOK_SECRET.
	req.Header.Set("X-Hub-Signature-256", signBody("repo-secret", payload))

	rec := httptest.NewRecorder()
	s.AppHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong secret, got %d", rec.Code)
	}
}

func TestAppWebhookRejectsInvalidToken(t *testing.T) {
	s, _, _ := newAppWebhookTestServer(t)

	payload := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/app-webhook/not-a-valid-token", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "installation")
	req.Header.Set("X-Hub-Signature-256", signBody("app-secret", payload))

	rec := httptest.NewRecorder()
	s.AppHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", rec.Code)
	}
}

func TestRepoWebhookStillUsesRepoSecret(t *testing.T) {
	s, token, _ := newAppWebhookTestServer(t)

	payload := []byte(`{"zen":"Design for failure."}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/"+token, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "ping")
	req.Header.Set("X-GitHub-Delivery", "repo-delivery-1")
	req.Header.Set("X-Hub-Signature-256", signBody("repo-secret", payload))

	rec := httptest.NewRecorder()
	s.Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body %q)", rec.Code, rec.Body.String())
	}
}
