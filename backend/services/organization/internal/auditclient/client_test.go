package auditclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
)

func TestRecordDoesNotFollowRedirect(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Store(true)
	}))
	defer target.Close()
	audit := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusTemporaryRedirect))
	defer audit.Close()

	client, err := New(audit.URL, strings.Repeat("x", 32), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Record(context.Background(), application.AuditEvent{
		EventKey: "event-1",
	})
	if err == nil {
		t.Fatal("Record() succeeded after Audit redirected the request")
	}
	if redirected.Load() {
		t.Fatal("Audit client followed a redirect")
	}
}
