package mail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendRecoveryUsesBrevoAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/smtp/email" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("api-key") != "test-api-key" {
			t.Errorf("api-key header was not set")
		}
		var payload struct {
			Sender struct {
				Name  string `json:"name"`
				Email string `json:"email"`
			} `json:"sender"`
			To []struct {
				Email string `json:"email"`
			} `json:"to"`
			Subject     string `json:"subject"`
			TextContent string `json:"textContent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if payload.Sender.Name != "Mothercat" || payload.Sender.Email != "noreply@example.com" {
			t.Errorf("unexpected sender: %+v", payload.Sender)
		}
		if len(payload.To) != 1 || payload.To[0].Email != "user@example.com" {
			t.Errorf("unexpected recipients: %+v", payload.To)
		}
		if payload.Subject != "Reset your password" ||
			!strings.Contains(payload.TextContent, "https://app.example.com/reset-password?token=secret%2Btoken") {
			t.Errorf("unexpected email content: %+v", payload)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	sender, err := NewBrevo(BrevoConfig{
		APIKey: "test-api-key", From: "noreply@example.com", FromName: "Mothercat",
		PublicBaseURL: "https://app.example.com/", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	sender.endpoint = server.URL + "/v3/smtp/email"
	if err := sender.SendRecovery(context.Background(), "user@example.com", "password_reset", "secret+token"); err != nil {
		t.Fatalf("SendRecovery() error = %v", err)
	}
}

func TestSendRecoveryRejectsBrevoFailuresWithoutResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("sensitive provider response"))
	}))
	defer server.Close()

	sender, err := NewBrevo(BrevoConfig{
		APIKey: "test-api-key", From: "noreply@example.com",
		PublicBaseURL: "https://app.example.com", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	sender.endpoint = server.URL
	err = sender.SendRecovery(context.Background(), "user@example.com", "activation", "token")
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("SendRecovery() error = %v, want HTTP status error", err)
	}
	if strings.Contains(err.Error(), "sensitive provider response") {
		t.Fatalf("SendRecovery() leaked provider response: %v", err)
	}
}

func TestNewBrevoRequiresValidConfiguration(t *testing.T) {
	_, err := NewBrevo(BrevoConfig{
		APIKey: "key", From: "noreply@example.com", PublicBaseURL: "http://app.example.com",
		Timeout: time.Second,
	})
	if err == nil {
		t.Fatal("NewBrevo() accepted a non-HTTPS public URL")
	}
}
