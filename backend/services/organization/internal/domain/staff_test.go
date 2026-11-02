package domain

import (
	"encoding/json"
	"testing"
)

func TestCreateStaffRequestNormalizeAndValidate(t *testing.T) {
	email := "  staff@example.test "
	title := "  Site Engineer "
	department := "1c9491b8-21ce-4c66-a025-006bb346b810"
	request := CreateStaffRequest{
		StaffID: " 23847 ", FullName: " Ada Okafor ",
		WorkEmail: &email, JobTitle: &title, DepartmentID: &department,
	}

	if err := request.NormalizeAndValidate(); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if request.StaffID != "23847" || request.FullName != "Ada Okafor" ||
		*request.WorkEmail != "staff@example.test" || *request.JobTitle != "Site Engineer" {
		t.Fatalf("normalized request = %+v", request)
	}
}

func TestCreateStaffRequestRejectsInvalidIdentifiersAndLengths(t *testing.T) {
	tests := []CreateStaffRequest{
		{StaffID: "ID:23847", FullName: "Ada Okafor"},
		{StaffID: "123", FullName: "Ada Okafor"},
		{StaffID: "123456", FullName: "Ada Okafor"},
		{StaffID: "23A47", FullName: "Ada Okafor"},
		{StaffID: "23847", FullName: ""},
		{StaffID: "23847", FullName: "Ada", DepartmentID: stringPointer("not-a-uuid")},
	}
	for _, request := range tests {
		if err := request.NormalizeAndValidate(); err == nil {
			t.Errorf("NormalizeAndValidate(%+v) succeeded; want error", request)
		}
	}
}

func TestCreateStaffRequestAcceptsFourOrFiveDigitNumbers(t *testing.T) {
	for _, staffID := range []string{"2384", "23847"} {
		request := CreateStaffRequest{StaffID: staffID, FullName: "Ada Okafor"}
		if err := request.NormalizeAndValidate(); err != nil {
			t.Errorf("NormalizeAndValidate(%q) error = %v", staffID, err)
		}
	}
}

func stringPointer(value string) *string { return &value }

func TestUpdateStaffRequestDistinguishesOmittedAndNullFields(t *testing.T) {
	var request UpdateStaffRequest
	if err := json.Unmarshal([]byte(`{"work_email":null,"job_title":" Lead Engineer "}`), &request); err != nil {
		t.Fatal(err)
	}
	if !request.WorkEmail.Set || request.WorkEmail.Value != nil ||
		!request.JobTitle.Set || request.JobTitle.Value == nil {
		t.Fatalf("decoded optional fields = %+v", request)
	}
	if err := request.NormalizeAndValidate(); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if *request.JobTitle.Value != "Lead Engineer" {
		t.Fatalf("job title = %q", *request.JobTitle.Value)
	}
}

func TestUpdateStaffRequestRejectsEmptyOrImmutableFields(t *testing.T) {
	tests := []string{
		`{}`,
		`{"full_name":null}`,
		`{"full_name":" "}`,
		`{"department_id":"not-a-uuid"}`,
	}
	for _, value := range tests {
		var request UpdateStaffRequest
		if err := json.Unmarshal([]byte(value), &request); err != nil {
			t.Fatalf("decode %s: %v", value, err)
		}
		if err := request.NormalizeAndValidate(); err == nil {
			t.Errorf("NormalizeAndValidate(%s) succeeded; want error", value)
		}
	}
}

func TestDepartmentRequestAllowsClearingDescriptionButRequiresNameOnCreate(t *testing.T) {
	var create DepartmentRequest
	if err := json.Unmarshal([]byte(`{"name":" Engineering "}`), &create); err != nil {
		t.Fatal(err)
	}
	if err := create.NormalizeAndValidate(true); err != nil {
		t.Fatalf("valid create request: %v", err)
	}
	if create.Name.Value == nil || *create.Name.Value != "Engineering" {
		t.Fatalf("normalized department name = %+v", create.Name)
	}
	var update DepartmentRequest
	if err := json.Unmarshal([]byte(`{"description":null}`), &update); err != nil {
		t.Fatal(err)
	}
	if err := update.NormalizeAndValidate(false); err != nil {
		t.Fatalf("clearing description should be valid: %v", err)
	}
}

func TestStaffValidationRejectsControlCharacters(t *testing.T) {
	request := CreateStaffRequest{StaffID: "23847", FullName: "Ada\nOkafor"}
	if err := request.NormalizeAndValidate(); err == nil {
		t.Fatal("expected control character in staff name to be rejected")
	}
}
