package monolith

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestStaffProvisioningFilterMatchesStoredMongoFieldNames(t *testing.T) {
	got := staffProvisioningFilter("staff-uuid", "account-uuid")
	want := bson.M{
		"id": "staff-uuid", "employmentstatus": "pending",
		"$or": bson.A{
			bson.M{"accountid": bson.M{"$exists": false}},
			bson.M{"accountid": nil},
			bson.M{"accountid": "account-uuid"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staffProvisioningFilter() = %#v, want %#v", got, want)
	}
}
