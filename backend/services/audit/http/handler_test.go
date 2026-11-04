package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/audit/internal/domain"
)

type authenticatorStub struct {
	principal Principal
	err       error
}

func (a authenticatorStub) Authenticate(context.Context, string) (Principal, error) {
	return a.principal, a.err
}

type readerStub struct {
	events    []domain.EventRecord
	total     int
	event     domain.EventRecord
	err       error
	listCalls int
	getCalls  int
	filter    domain.EventFilter
	eventID   string
}

func (r *readerStub) List(_ context.Context, filter domain.EventFilter) ([]domain.EventRecord, int, error) {
	r.listCalls++
	r.filter = filter
	return r.events, r.total, r.err
}

func (r *readerStub) Get(_ context.Context, id string) (domain.EventRecord, error) {
	r.getCalls++
	r.eventID = id
	return r.event, r.err
}

type testWriter struct{}

func (testWriter) Write(data []byte) (int, error) { return len(data), nil }

func newTestHandler(reader *readerStub, permissions ...string) http.Handler {
	return NewHandler(
		reader,
		authenticatorStub{principal: Principal{Permissions: permissions}},
		slog.New(slog.NewTextHandler(testWriter{}, nil)),
	)
}

func request(method, target string, authenticated bool) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	if authenticated {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
	}
	return r
}

func TestListAuditEventsRequiresAuditReadPermission(t *testing.T) {
	reader := &readerStub{}
	response := httptest.NewRecorder()
	newTestHandler(reader).ServeHTTP(response, request(http.MethodGet, "/api/v1/audit-events", true))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if reader.listCalls != 0 {
		t.Fatal("audit store was read without audit.read permission")
	}
}

func TestListAuditEventsAppliesFiltersAndPagination(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	event := domain.EventRecord{
		ID: "00000000-0000-4000-8000-000000000001", ActorType: "user", ActorID: "actor-1",
		Action: "staff.updated", ResourceType: "staff_member", ResourceID: "staff-1",
		OccurredAt: from, Details: json.RawMessage(`{"field":"job_title"}`),
	}
	reader := &readerStub{events: []domain.EventRecord{event}, total: 17}
	response := httptest.NewRecorder()
	target := "/api/v1/audit-events?actor_id=actor-1&action=staff.updated&resource_type=staff_member&resource_id=staff-1" +
		"&from=2026-11-01T00%3A00%3A00Z&to=2026-11-02T00%3A00%3A00Z&limit=10&offset=20"
	newTestHandler(reader, "audit.read").ServeHTTP(response, request(http.MethodGet, target, true))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	if reader.listCalls != 1 || reader.filter.ActorID != "actor-1" ||
		reader.filter.Action != "staff.updated" || reader.filter.ResourceType != "staff_member" ||
		reader.filter.ResourceID != "staff-1" || reader.filter.Limit != 10 || reader.filter.Offset != 20 {
		t.Fatalf("unexpected store filter: %#v", reader.filter)
	}
	if reader.filter.From == nil || !reader.filter.From.Equal(from) ||
		reader.filter.To == nil || !reader.filter.To.Equal(to) {
		t.Fatalf("unexpected time range: from=%v to=%v", reader.filter.From, reader.filter.To)
	}
	if !strings.Contains(response.Body.String(), `"total":17`) ||
		!strings.Contains(response.Body.String(), `"action":"staff.updated"`) {
		t.Fatalf("unexpected response: %s", response.Body)
	}
}

func TestListAuditEventsRejectsInvalidQuery(t *testing.T) {
	for _, target := range []string{
		"/api/v1/audit-events?limit=101",
		"/api/v1/audit-events?offset=-1",
		"/api/v1/audit-events?actor_id=one&actor_id=two",
		"/api/v1/audit-events?unknown=value",
		"/api/v1/audit-events?from=not-a-time",
		"/api/v1/audit-events?from=2026-11-02T00%3A00%3A00Z&to=2026-11-01T00%3A00%3A00Z",
	} {
		t.Run(target, func(t *testing.T) {
			reader := &readerStub{}
			response := httptest.NewRecorder()
			newTestHandler(reader, "audit.read").ServeHTTP(
				response, request(http.MethodGet, target, true),
			)
			if response.Code != http.StatusBadRequest || reader.listCalls != 0 {
				t.Fatalf("status=%d list calls=%d body=%s",
					response.Code, reader.listCalls, response.Body)
			}
		})
	}
}

func TestGetAuditEventRequiresValidIDAndMapsNotFound(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"

	t.Run("returns event", func(t *testing.T) {
		reader := &readerStub{event: domain.EventRecord{ID: id, Action: "staff.updated"}}
		response := httptest.NewRecorder()
		newTestHandler(reader, "audit.read").ServeHTTP(
			response, request(http.MethodGet, "/api/v1/audit-events/"+id, true),
		)
		if response.Code != http.StatusOK || reader.getCalls != 1 || reader.eventID != id {
			t.Fatalf("status=%d get calls=%d id=%q body=%s",
				response.Code, reader.getCalls, reader.eventID, response.Body)
		}
	})

	t.Run("rejects malformed ID", func(t *testing.T) {
		reader := &readerStub{}
		response := httptest.NewRecorder()
		newTestHandler(reader, "audit.read").ServeHTTP(
			response, request(http.MethodGet, "/api/v1/audit-events/not-a-uuid", true),
		)
		if response.Code != http.StatusBadRequest || reader.getCalls != 0 {
			t.Fatalf("status=%d get calls=%d body=%s",
				response.Code, reader.getCalls, response.Body)
		}
	})

	t.Run("returns 404 for missing event", func(t *testing.T) {
		reader := &readerStub{err: domain.ErrNotFound}
		response := httptest.NewRecorder()
		newTestHandler(reader, "audit.read").ServeHTTP(
			response, request(http.MethodGet, "/api/v1/audit-events/"+id, true),
		)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusNotFound, response.Body)
		}
	})
}

func TestAuditEndpointsRequireSession(t *testing.T) {
	for _, target := range []string{
		"/api/v1/audit-events",
		"/api/v1/audit-events/00000000-0000-4000-8000-000000000001",
	} {
		reader := &readerStub{}
		response := httptest.NewRecorder()
		newTestHandler(reader, "audit.read").ServeHTTP(
			response, request(http.MethodGet, target, false),
		)
		if response.Code != http.StatusUnauthorized || reader.listCalls != 0 || reader.getCalls != 0 {
			t.Fatalf("%s: status=%d list calls=%d get calls=%d",
				target, response.Code, reader.listCalls, reader.getCalls)
		}
	}
}

func TestGetAuditEventMapsStoreFailureToInternalError(t *testing.T) {
	reader := &readerStub{err: errors.New("database unavailable")}
	response := httptest.NewRecorder()
	newTestHandler(reader, "audit.read").ServeHTTP(
		response,
		request(http.MethodGet, "/api/v1/audit-events/00000000-0000-4000-8000-000000000001", true),
	)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusInternalServerError, response.Body)
	}
}
