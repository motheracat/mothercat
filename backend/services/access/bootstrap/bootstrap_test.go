package bootstrap

import (
	"reflect"
	"testing"
)

func TestInitialPermissionKeysMatchMilestoneOneAdministrationScope(t *testing.T) {
	want := []string{
		"staff.read",
		"staff.create",
		"staff.update",
		"staff.deactivate",
		"staff.import",
		"departments.manage",
		"roles.read",
		"roles.manage",
		"user_roles.assign",
		"audit.read",
		"settings.manage",
	}
	if got := InitialPermissionKeys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("InitialPermissionKeys() = %v, want %v", got, want)
	}
}

func TestInitialPermissionKeysReturnsIndependentCopy(t *testing.T) {
	keys := InitialPermissionKeys()
	keys[0] = "changed"
	if InitialPermissionKeys()[0] == "changed" {
		t.Fatal("InitialPermissionKeys exposed its internal slice")
	}
}
