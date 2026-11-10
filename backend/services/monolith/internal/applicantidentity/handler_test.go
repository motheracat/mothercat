package applicantidentity

import (
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplicantRegistrationRejectsWeakPasswordBeforeDatabaseAccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	handler := NewHandler(db, nil, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/applicant/register",
		strings.NewReader(`{"email":"applicant@example.test","display_name":"Applicant","password":"weak"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d: %s", response.Code, http.StatusBadRequest, response.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplicantEmailVerificationRejectsInvalidToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE identity\.applicant_email_verification_tokens`).
		WithArgs(hashToken(strings.Repeat("a", 64))).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	handler := NewHandler(db, nil, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/applicant/verify-email",
		strings.NewReader(`{"token":"`+strings.Repeat("a", 64)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"invalid_verification_token"`) {
		t.Fatalf("status=%d body=%s, want invalid verification response", response.Code, response.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplicantEmailVerificationConsumesTokenAndVerifiesAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const accountID = "00000000-0000-4000-8000-000000000001"
	token := strings.Repeat("b", 64)
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE identity\.applicant_email_verification_tokens`).
		WithArgs(hashToken(token)).
		WillReturnRows(sqlmock.NewRows([]string{"applicant_account_id"}).AddRow(accountID))
	mock.ExpectExec(`UPDATE identity\.applicant_accounts`).
		WithArgs(accountID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	handler := NewHandler(db, nil, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/applicant/verify-email",
		strings.NewReader(`{"token":"`+token+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"verified"`) {
		t.Fatalf("status=%d body=%s, want verified response", response.Code, response.Body)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("response is missing request ID")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplicantLoginRequiresVerifiedAccountAndUsesSeparateCookie(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`SELECT id::text, email, display_name, password_hash`).
		WithArgs("applicant@example.test").
		WillReturnError(sql.ErrNoRows)

	handler := NewHandler(db, nil, slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/applicant/login",
		strings.NewReader(`{"email":"Applicant@Example.Test","password":"StrongPassword123!"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d: %s", response.Code, http.StatusUnauthorized, response.Body)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == applicantSessionCookie {
			t.Fatal("unverified applicant received a session cookie")
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	email, ok := normalizeEmail(" Applicant@Example.Test ")
	if !ok || email != "applicant@example.test" {
		t.Fatalf("normalizeEmail()=(%q,%t), want lower-case valid email", email, ok)
	}
	if _, ok := normalizeEmail("not-an-email"); ok {
		t.Fatal("normalizeEmail() accepted invalid email")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(value []byte) (int, error) { return len(value), nil }
