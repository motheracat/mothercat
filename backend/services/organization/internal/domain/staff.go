package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidStaff       = errors.New("invalid_staff")
	ErrDuplicateStaffID   = errors.New("duplicate_staff_id")
	ErrDepartmentNotFound = errors.New("department_not_found")
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
	StaffID          string  `json:"staff_id"`
	DepartmentID     *string `json:"department_id"`
	FullName         string  `json:"full_name"`
	WorkEmail        *string `json:"work_email"`
	JobTitle         *string `json:"job_title"`
	EmploymentStatus string  `json:"employment_status"`
}

func (request *CreateStaffRequest) NormalizeAndValidate() error {
	request.StaffID = strings.TrimSpace(request.StaffID)
	request.FullName = strings.TrimSpace(request.FullName)
	if request.StaffID == "" || len(request.StaffID) > 80 || request.FullName == "" ||
		utf8.RuneCountInString(request.FullName) > 200 {
		return ErrInvalidStaff
	}
	for _, char := range request.StaffID {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') &&
			!(char >= '0' && char <= '9') && char != '-' && char != '_' && char != '.' {
			return ErrInvalidStaff
		}
	}
	if request.WorkEmail != nil {
		value := strings.TrimSpace(*request.WorkEmail)
		if value == "" {
			request.WorkEmail = nil
		} else if len(value) > 254 || strings.ContainsAny(value, "\r\n") || !strings.Contains(value, "@") {
			return ErrInvalidStaff
		} else {
			request.WorkEmail = &value
		}
	}
	if request.JobTitle != nil {
		value := strings.TrimSpace(*request.JobTitle)
		if value == "" {
			request.JobTitle = nil
		} else if utf8.RuneCountInString(value) > 150 {
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
