package notifications

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAccountID = "00000000-0000-4000-8000-000000000001"

type authenticatorStub struct {
	principal Principal
	err       error
}

func (a authenticatorStub) Authenticate(context.Context, string) (Principal, error) {
	return a.principal, a.err
}

type discardWriter struct{}

func (discardWriter) Write(data []byte) (int, error) { return len(data), nil }

func notificationTestHandler(auth Authenticator) http.Handler {
	return NewHandler(nil, auth, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
}

func notificationRequest(method, target, body string, authenticated bool) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Origin", "http://example.com")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
	}
	return request
}

func TestNotificationEndpointsRequireAuthentication(t *testing.T) {
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/notifications"},
		{http.MethodGet, "/api/v1/notifications/unread-count"},
		{http.MethodPost, "/api/v1/notifications/00000000-0000-4000-8000-000000000002/read"},
		{http.MethodPost, "/api/v1/notifications/read-all"},
		{http.MethodGet, "/api/v1/notifications/preferences"},
		{http.MethodPut, "/api/v1/notifications/preferences"},
	} {
		response := httptest.NewRecorder()
		notificationTestHandler(authenticatorStub{}).ServeHTTP(
			response, notificationRequest(route.method, route.path, `{"preferences":[]}`, false),
		)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status=%d, want %d", route.method, route.path, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestNotificationMutationsRequireSameOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
	request.Header.Set("Origin", "https://attacker.example")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	notificationTestHandler(authenticatorStub{principal: Principal{ID: testAccountID}}).
		ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d: %s", response.Code, http.StatusForbidden, response.Body)
	}
}

func TestParseListQuery(t *testing.T) {
	tests := []struct {
		query      string
		wantLimit  int
		wantOff    int
		wantUnread bool
		wantErr    bool
	}{
		{"", 50, 0, false, false},
		{"limit=10&offset=20&unread=true", 10, 20, true, false},
		{"unread=no", 0, 0, false, true},
		{"limit=101", 0, 0, false, true},
		{"offset=-1", 0, 0, false, true},
		{"unexpected=value", 0, 0, false, true},
		{"limit=1&limit=2", 0, 0, false, true},
	}
	for _, test := range tests {
		limit, offset, unread, err := parseListQuery(httptest.NewRequest(
			http.MethodGet, "/?"+test.query, nil,
		).URL.Query())
		if (err != nil) != test.wantErr {
			t.Errorf("parseListQuery(%q) error=%v, wantErr=%t", test.query, err, test.wantErr)
			continue
		}
		if !test.wantErr && (limit != test.wantLimit || offset != test.wantOff || unread != test.wantUnread) {
			t.Errorf("parseListQuery(%q)=(%d,%d,%t), want (%d,%d,%t)",
				test.query, limit, offset, unread, test.wantLimit, test.wantOff, test.wantUnread)
		}
	}
}

func TestReplacePreferencesRejectsUnknownOrDuplicateEventsBeforeDatabase(t *testing.T) {
	auth := authenticatorStub{principal: Principal{ID: testAccountID}}
	for _, body := range []string{
		`{"preferences":[{"event_type":"not allowed","enabled":true}]}`,
		`{"preferences":[{"event_type":"task.assigned","enabled":true},{"event_type":"task.assigned","enabled":false}]}`,
		`{"preferences":[],"unexpected":true}`,
	} {
		response := httptest.NewRecorder()
		notificationTestHandler(auth).ServeHTTP(response,
			notificationRequest(http.MethodPut, "/api/v1/notifications/preferences", body, true))
		if response.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d, want %d: %s", body, response.Code, http.StatusBadRequest, response.Body)
		}
	}
}

func TestNotificationPreferenceEventTypeValidation(t *testing.T) {
	for _, test := range []struct {
		value string
		valid bool
	}{
		{"task.assigned", true},
		{"task.comment_added", true},
		{"Task.Assigned", false},
		{"task assigned", false},
		{"", false},
	} {
		if validEventType(test.value) != test.valid {
			t.Errorf("validEventType(%q)=%t, want %t", test.value, validEventType(test.value), test.valid)
		}
	}
}
