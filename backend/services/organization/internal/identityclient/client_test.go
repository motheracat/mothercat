package identityclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
)

func TestAuthenticateValidatesSessionAndReturnsPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "mothercat_session=session-value" {
			t.Errorf("Cookie = %q", got)
		}
		if got := r.Header.Get("X-Request-ID"); got != "0123456789abcdef0123456789abcdef" {
			t.Errorf("X-Request-ID = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"1c9491b8-21ce-4c66-a025-006bb346b810","staff_id":"ENG-1","display_name":"Staff","email":"staff@example.test","role":"SuperAdmin","roles":["SuperAdmin"],"permissions":["staff.create"]}}`))
	}))
	defer server.Close()
	client, err := New(server.URL, 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := application.ContextWithRequestID(context.Background(), "0123456789abcdef0123456789abcdef")

	principal, err := client.Authenticate(ctx, "session-value")

	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.ID != "1c9491b8-21ce-4c66-a025-006bb346b810" ||
		len(principal.Permissions) != 1 || principal.Permissions[0] != "staff.create" {
		t.Fatalf("principal = %+v", principal)
	}
}

func TestAuthenticateMapsUnauthorizedToUnauthenticated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	client, err := New(server.URL, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Authenticate(context.Background(), "session-value")

	if err != application.ErrUnauthenticated {
		t.Fatalf("Authenticate() error = %v, want unauthenticated", err)
	}
}

func TestNewRejectsNonLoopbackHTTPURL(t *testing.T) {
	if _, err := New("http://identity.example.test", 0); err == nil {
		t.Fatal("New() accepted non-loopback HTTP")
	}
}

func TestAuthenticateRejectsCookieHeaderInjection(t *testing.T) {
	client, err := New("http://127.0.0.1:8080", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Authenticate(context.Background(), "token\r\nAuthorization: Bearer "+strings.Repeat("x", 32)); err == nil {
		t.Fatal("Authenticate() accepted a cookie containing CR/LF")
	}
}
