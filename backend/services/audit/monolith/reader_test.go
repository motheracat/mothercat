package monolith

import (
	"reflect"
	"testing"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/audit/internal/domain"
)

func TestEventWhereBuildsParameterizedFilters(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	filter := domain.EventFilter{
		ActorID: "actor' OR true --", Action: "staff.updated",
		ResourceType: "staff_member", ResourceID: "staff-1",
		From: &from, To: &to,
	}

	where, args := eventWhere(filter)
	wantWhere := " WHERE actor_id = $1 AND action = $2 AND resource_type = $3 AND resource_id = $4 AND occurred_at >= $5 AND occurred_at <= $6"
	wantArgs := []any{"actor' OR true --", "staff.updated", "staff_member", "staff-1", from, to}
	if where != wantWhere {
		t.Fatalf("where = %q, want %q", where, wantWhere)
	}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestEventWhereWithoutFiltersIsEmpty(t *testing.T) {
	where, args := eventWhere(domain.EventFilter{})
	if where != "" || len(args) != 0 {
		t.Fatalf("where=%q args=%#v, want empty filter", where, args)
	}
}
