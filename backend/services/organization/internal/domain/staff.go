package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidStaff            = errors.New("invalid_staff")
	ErrDuplicateStaffID        = errors.New("duplicate_staff_id")
	ErrDepartmentNotFound      = errors.New("department_not_found")
	ErrStaffNotFound           = errors.New("staff_not_found")
	ErrStaffAlreadyProvisioned = errors.New("staff_already_provisioned")
	ErrDepartmentConflict      = errors.New("department_conflict")
	ErrImportNotFound          = errors.New("staff_import_not_found")
)

type CreateStaffRequest struct {
	StaffID      string  `json:"staff_id"`
	FullName     string  `json:"full_name"`
	WorkEmail    *string `json:"work_email"`
	JobTitle     *string `json:"job_title"`
	DepartmentID *string `json:"department_id"`
}

type Staff struct {
	ID               string  `json:"id"`
	AccountID        *string `json:"account_id,omitempty"`
	StaffID          string  `json:"staff_id"`
	DepartmentID     *string `json:"department_id"`
	FullName         string  `json:"full_name"`
	ProfileImageURL  *string `json:"profile_image_url"`
	WorkEmail        *string `json:"work_email"`
	JobTitle         *string `json:"job_title"`
	EmploymentStatus string  `json:"employment_status"`
}

type OptionalString struct {
	Set   bool
	Value *string
}

func (value *OptionalString) UnmarshalJSON(data []byte) error {
	value.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil
		return nil
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = &decoded
	return nil
}

type UpdateStaffRequest struct {
	FullName     OptionalString `json:"full_name"`
	WorkEmail    OptionalString `json:"work_email"`
	JobTitle     OptionalString `json:"job_title"`
	DepartmentID OptionalString `json:"department_id"`
}

func (request UpdateStaffRequest) HasChanges() bool {
	return request.FullName.Set || request.WorkEmail.Set ||
		request.JobTitle.Set || request.DepartmentID.Set
}

func (request *UpdateStaffRequest) NormalizeAndValidate() error {
	if !request.HasChanges() {
		return ErrInvalidStaff
	}
	if request.FullName.Set {
		if request.FullName.Value == nil {
			return ErrInvalidStaff
		}
		value := strings.TrimSpace(*request.FullName.Value)
		if value == "" || utf8.RuneCountInString(value) > 200 || hasControlCharacters(value) {
			return ErrInvalidStaff
		}
		request.FullName.Value = &value
	}
	if request.WorkEmail.Set {
		if request.WorkEmail.Value != nil {
			value := strings.TrimSpace(*request.WorkEmail.Value)
			if value == "" {
				request.WorkEmail.Value = nil
			} else if len(value) > 254 || hasControlCharacters(value) || !strings.Contains(value, "@") {
				return ErrInvalidStaff
			} else {
				request.WorkEmail.Value = &value
			}
		}
	}
	if request.JobTitle.Set && request.JobTitle.Value != nil {
		value := strings.TrimSpace(*request.JobTitle.Value)
		if value == "" {
			request.JobTitle.Value = nil
		} else if utf8.RuneCountInString(value) > 150 || hasControlCharacters(value) {
			return ErrInvalidStaff
		} else {
			request.JobTitle.Value = &value
		}
	}
	if request.DepartmentID.Set && request.DepartmentID.Value != nil {
		value := strings.TrimSpace(*request.DepartmentID.Value)
		if value == "" {
			request.DepartmentID.Value = nil
		} else if !isUUID(value) {
			return ErrInvalidStaff
		} else {
			request.DepartmentID.Value = &value
		}
	}
	return nil
}

type Department struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type DepartmentRequest struct {
	Name        OptionalString `json:"name"`
	Description OptionalString `json:"description"`
}

func (request *DepartmentRequest) NormalizeAndValidate(requireName bool) error {
	if requireName && (!request.Name.Set || request.Name.Value == nil) {
		return ErrInvalidStaff
	}
	if request.Name.Set {
		if request.Name.Value == nil {
			return ErrInvalidStaff
		}
		value := strings.TrimSpace(*request.Name.Value)
		if value == "" || utf8.RuneCountInString(value) > 160 || hasControlCharacters(value) {
			return ErrInvalidStaff
		}
		request.Name.Value = &value
	}
	if request.Description.Set && request.Description.Value != nil {
		value := strings.TrimSpace(*request.Description.Value)
		if utf8.RuneCountInString(value) > 2000 || hasControlCharacters(value) {
			return ErrInvalidStaff
		}
		if value == "" {
			request.Description.Value = nil
		} else {
			request.Description.Value = &value
		}
	}
	return nil
}

type StaffListFilter struct {
	Limit            int
	Offset           int
	Search           string
	DepartmentID     string
	EmploymentStatus string
}

type ImportStaffRow struct {
	RowNumber int
	Staff     CreateStaffRequest
}

type StaffImportIssue struct {
	Row     int    `json:"row"`
	StaffID string `json:"staff_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type StaffImportResult struct {
	TotalRows       int                `json:"total_rows"`
	ImportedRows    int                `json:"imported_rows"`
	ErrorCount      int                `json:"error_count"`
	ErrorsTruncated bool               `json:"errors_truncated,omitempty"`
	Errors          []StaffImportIssue `json:"errors,omitempty"`
}

type StaffImportBatch struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Result      StaffImportResult `json:"result"`
}

func (request *CreateStaffRequest) NormalizeAndValidate() error {
	request.StaffID = strings.TrimSpace(request.StaffID)
	request.FullName = strings.TrimSpace(request.FullName)
	if !isStaffNumber(request.StaffID) || request.FullName == "" ||
		utf8.RuneCountInString(request.FullName) > 200 || hasControlCharacters(request.FullName) {
		return ErrInvalidStaff
	}
	if request.WorkEmail != nil {
		value := strings.TrimSpace(*request.WorkEmail)
		if value == "" {
			request.WorkEmail = nil
		} else if len(value) > 254 || hasControlCharacters(value) || !strings.Contains(value, "@") {
			return ErrInvalidStaff
		} else {
			request.WorkEmail = &value
		}
	}
	if request.JobTitle != nil {
		value := strings.TrimSpace(*request.JobTitle)
		if value == "" {
			request.JobTitle = nil
		} else if utf8.RuneCountInString(value) > 150 || hasControlCharacters(value) {
			return ErrInvalidStaff
		} else {
			request.JobTitle = &value
		}
	}
	if request.DepartmentID != nil {
		value := strings.TrimSpace(*request.DepartmentID)
		if value == "" {
			request.DepartmentID = nil
		} else if !isUUID(value) {
			return ErrInvalidStaff
		} else {
			request.DepartmentID = &value
		}
	}
	return nil
}

func isStaffNumber(value string) bool {
	if len(value) != 4 && len(value) != 5 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func hasControlCharacters(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') &&
			!(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func IsUUID(value string) bool {
	return isUUID(value)
}
