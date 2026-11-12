package systemmonitor

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNewSecurityServiceRequiresBase64EncodedAES256Key(t *testing.T) {
	db, _, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := NewSecurityService(db, "not-a-key"); !errors.Is(err, ErrSecurityUnavailable) {
		t.Fatalf("NewSecurityService() error = %v, want security unavailable", err)
	}
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if _, err := NewSecurityService(db, key); err != nil {
		t.Fatalf("NewSecurityService() valid key error = %v", err)
	}
}

func TestTOTPUsesStandardSixDigitTimeBasedCode(t *testing.T) {
	secret := base32NoPadding("12345678901234567890")
	step, valid := verifyTOTP(secret, "287082", time.Unix(59, 0))
	if !valid || step != 1 {
		t.Fatalf("verifyTOTP() = (%d, %v), want step 1 and valid", step, valid)
	}
	if _, valid := verifyTOTP(secret, "287083", time.Unix(59, 0)); valid {
		t.Fatal("verifyTOTP() accepted an invalid code")
	}
}

func TestSecurityServiceEncryptsMFASecretsWithAuthenticatedEncryption(t *testing.T) {
	db, _, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("base32 MFA secret")
	ciphertext, err := service.encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if string(ciphertext) == string(plaintext) {
		t.Fatal("encrypted MFA secret equals plaintext")
	}
	decrypted, err := service.decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted secret = %q, want %q", decrypted, plaintext)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff
	if _, err := service.decrypt(ciphertext); err == nil {
		t.Fatal("decrypt() accepted modified ciphertext")
	}
}

func TestVerifyMFAConsumesCodeOnceAndWritesNoCodeToStorage(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	secret := base32NoPadding("12345678901234567890")
	encrypted, err := service.encrypt([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, step := generateTOTP(secret, now)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT secret_ciphertext, last_used_step.*FOR UPDATE`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"secret_ciphertext", "last_used_step"}).
			AddRow(encrypted, step-1))
	mock.ExpectQuery(`(?s)SELECT count\(\*\).*mfa\.verification_failed`).
		WithArgs(testMonitorActorID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?s)UPDATE system_monitor\.mfa_credentials SET last_used_step`).
		WithArgs(testMonitorActorID, step).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.VerifyMFA(context.Background(), testMonitorActorID, code); err != nil {
		t.Fatalf("VerifyMFA() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMFARejectsReplayAndRecordsFailure(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	secret := base32NoPadding("12345678901234567890")
	encrypted, err := service.encrypt([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, step := generateTOTP(secret, now)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT secret_ciphertext, last_used_step.*FOR UPDATE`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"secret_ciphertext", "last_used_step"}).
			AddRow(encrypted, step))
	mock.ExpectQuery(`(?s)SELECT count\(\*\).*mfa\.verification_failed`).
		WithArgs(testMonitorActorID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.security_events`).
		WithArgs(testMonitorActorID, "mfa.verification_failed", "warning", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := service.VerifyMFA(context.Background(), testMonitorActorID, code); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("VerifyMFA() error = %v, want invalid MFA code", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMFAEnforcesPersistentRateLimit(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	secret := base32NoPadding("12345678901234567890")
	encrypted, err := service.encrypt([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT secret_ciphertext, last_used_step.*FOR UPDATE`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"secret_ciphertext", "last_used_step"}).AddRow(encrypted, 0))
	mock.ExpectQuery(`(?s)SELECT count\(\*\).*mfa\.verification_failed`).
		WithArgs(testMonitorActorID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(mfaFailureLimit))
	mock.ExpectRollback()
	if err := service.VerifyMFA(context.Background(), testMonitorActorID, "invalid"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatalf("VerifyMFA() error = %v, want rate limited", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmMFAEnrollmentEnablesMFAAndAuditsInOneTransaction(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	secret := base32NoPadding("12345678901234567890")
	encrypted, err := service.encrypt([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, step := generateTOTP(secret, now)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT secret_ciphertext, expires_at.*FOR UPDATE`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"secret_ciphertext", "expires_at"}).
			AddRow(encrypted, now.Add(time.Minute)))
	mock.ExpectQuery(`(?s)SELECT count\(\*\).*mfa\.enrollment_failed`).
		WithArgs(testMonitorActorID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.mfa_credentials`).
		WithArgs(testMonitorActorID, encrypted, step).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM system_monitor\.mfa_enrollment_challenges`).
		WithArgs(testMonitorActorID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.security_events`).
		WithArgs(testMonitorActorID, "mfa.enabled", "info", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`(?s)INSERT INTO audit\.audit_events`).
		WithArgs(sqlmock.AnyArg(), testMonitorActorID, "system_monitor.mfa.enabled",
			"system_monitor_mfa", testMonitorActorID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.ConfirmMFAEnrollment(context.Background(), testMonitorActorID, code); err != nil {
		t.Fatalf("ConfirmMFAEnrollment() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmMFAEnrollmentRateLimitsRepeatedFailures(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT secret_ciphertext, expires_at.*FOR UPDATE`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"secret_ciphertext", "expires_at"}).
			AddRow([]byte("encrypted"), now.Add(time.Minute)))
	mock.ExpectQuery(`(?s)SELECT count\(\*\).*mfa\.enrollment_failed`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(mfaFailureLimit))
	mock.ExpectRollback()
	if err := service.ConfirmMFAEnrollment(context.Background(), testMonitorActorID, "123456"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatalf("ConfirmMFAEnrollment() error = %v, want rate limited", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeSessionNeverRevokesCurrentSession(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)UPDATE identity\.sessions.*RETURNING id::text`).
		WithArgs(testMonitorActorID, testMonitorActorID, "current-session-hash").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*identity\.sessions`).
		WithArgs(testMonitorActorID, testMonitorActorID, "current-session-hash").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	err = service.RevokeSession(context.Background(), testMonitorActorID, testMonitorActorID,
		"current-session-hash", "session no longer needed")
	if !errors.Is(err, ErrCurrentSession) {
		t.Fatalf("RevokeSession() error = %v, want current session refusal", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDisableMFAWritesCriticalEventAndAuditAtomically(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := NewSecurityService(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)DELETE FROM system_monitor\.mfa_credentials`).
		WithArgs(testMonitorActorID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.security_events`).
		WithArgs(testMonitorActorID, "mfa.disabled", "critical", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`(?s)INSERT INTO audit\.audit_events`).
		WithArgs(sqlmock.AnyArg(), testMonitorActorID, "system_monitor.mfa.disabled",
			"system_monitor_mfa", testMonitorActorID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.DisableMFA(context.Background(), testMonitorActorID, "approved recovery request"); err != nil {
		t.Fatalf("DisableMFA() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func base32NoPadding(value string) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(value))
}

func generateTOTP(encodedSecret string, now time.Time) (string, int64) {
	decoded, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encodedSecret)
	step := now.Unix() / mfaStepSeconds
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, decoded)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), step
}
