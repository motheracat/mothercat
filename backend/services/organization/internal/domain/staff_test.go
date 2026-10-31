package domain

import "testing"

func TestCreateStaffRequestNormalizeAndValidate(t *testing.T) {
	email := "  staff@example.test "
	title := "  Site Engineer "
	department := "1c9491b8-21ce-4c66-a025-006bb346b810"
	request := CreateStaffRequest{
		StaffID: " ENG-104 ", FullName: " Ada Okafor ",
		WorkEmail: &email, JobTitle: &title, DepartmentID: &department,
	}

	if err := request.NormalizeAndValidate(); err != nil {
		t.Fatalf("NormalizeAndValidate() error = %v", err)
	}
	if request.StaffID != "ENG-104" || request.FullName != "Ada Okafor" ||
		*request.WorkEmail != "staff@example.test" || *request.JobTitle != "Site Engineer" {
		t.Fatalf("normalized request = %+v", request)
	}
}

func TestCreateStaffRequestRejectsInvalidIdentifiersAndLengths(t *testing.T) {
	tests := []CreateStaffRequest{
		{StaffID: "ENG 104", FullName: "Ada Okafor"},
		{StaffID: "ENG-104", FullName: ""},
		{StaffID: "ENG-104", FullName: "Ada", DepartmentID: stringPointer("not-a-uuid")},
	}
	for _, request := range tests {
		if err := request.NormalizeAndValidate(); err == nil {
			t.Errorf("NormalizeAndValidate(%+v) succeeded; want error", request)
		}
	}
}

func stringPointer(value string) *string { return &value }
