package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrBootstrapComplete = errors.New("initial administrator bootstrap is already complete")
	ErrBootstrapSecret   = errors.New("bootstrap secret is invalid or expired")
	ErrAccountConflict   = errors.New("initial administrator account conflicts with an existing account")
)

const (
	minimumSecretLength = 32
	minimumPasswordSize = 14
	maximumPasswordSize = 72
	maximumSecretAge    = time.Hour
)

type InitialAdmin struct {
	StaffID     string
	Email       string
	DisplayName string
	Password    string
}

type database interface {
	Begin(context.Context) (pgx.Tx, error)
}

// Prepare creates the initial administrator as inactive. Repeating the operation
// with the same secret and account details is safe while the secret remains valid.
func Prepare(ctx context.Context, db database, secret string, expiresAt time.Time, admin InitialAdmin) (string, error) {
	if db == nil {
		return "", errors.New("identity bootstrap database is unavailable")
	}
	if err := validateSecret(secret, expiresAt, time.Now()); err != nil {
		return "", err
	}
	admin.StaffID = strings.TrimSpace(admin.StaffID)
	admin.Email = strings.ToLower(strings.TrimSpace(admin.Email))
	admin.DisplayName = strings.TrimSpace(admin.DisplayName)
	if err := validateAdmin(admin); err != nil {
		return "", err
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin Identity bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	secretHash := hashSecret(secret)
	if _, err := tx.Exec(ctx, `
		INSERT INTO identity.initial_admin_bootstrap (singleton, secret_hash, expires_at)
		VALUES (TRUE, $1, $2)
		ON CONFLICT (singleton) DO NOTHING
	`, secretHash, expiresAt); err != nil {
		return "", fmt.Errorf("initialize Identity bootstrap state: %w", err)
	}

	var storedHash *string
	var storedExpiry time.Time
	var accountID *string
	var completedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT secret_hash, expires_at, account_id, completed_at
		FROM identity.initial_admin_bootstrap
		WHERE singleton = TRUE
		FOR UPDATE
	`).Scan(&storedHash, &storedExpiry, &accountID, &completedAt); err != nil {
		return "", fmt.Errorf("read Identity bootstrap state: %w", err)
	}
	if completedAt != nil {
		return "", ErrBootstrapComplete
	}
	if !time.Now().Before(storedExpiry) {
		if storedHash != nil && *storedHash == secretHash {
			return "", ErrBootstrapSecret
		}
		if _, err := tx.Exec(ctx, `
			UPDATE identity.initial_admin_bootstrap
			SET secret_hash = $1, expires_at = $2
			WHERE singleton = TRUE
		`, secretHash, expiresAt); err != nil {
			return "", fmt.Errorf("rotate expired bootstrap secret: %w", err)
		}
		storedHash = &secretHash
		storedExpiry = expiresAt
	}
	if storedHash == nil || *storedHash != secretHash || !time.Now().Before(storedExpiry) {
		return "", ErrBootstrapSecret
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(admin.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash initial administrator password: %w", err)
	}

	if accountID == nil {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO identity.user_accounts
				(staff_id, email, password_hash, display_name, is_active, must_change_password)
			VALUES ($1, $2, $3, $4, FALSE, FALSE)
			RETURNING id
		`, admin.StaffID, admin.Email, string(passwordHash), admin.DisplayName).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("create pending initial administrator account: %w", err)
		}
		accountID = &id
		if _, err := tx.Exec(ctx, `
			UPDATE identity.initial_admin_bootstrap
			SET account_id = $1
			WHERE singleton = TRUE
		`, id); err != nil {
			return "", fmt.Errorf("record pending administrator account: %w", err)
		}
	} else {
		var staffID, email, displayName, existingHash string
		var active bool
		if err := tx.QueryRow(ctx, `
			SELECT staff_id, email, display_name, password_hash, is_active
			FROM identity.user_accounts
			WHERE id = $1
		`, *accountID).Scan(&staffID, &email, &displayName, &existingHash, &active); err != nil {
			return "", fmt.Errorf("read pending administrator account: %w", err)
		}
		if active || staffID != admin.StaffID || !strings.EqualFold(email, admin.Email) || displayName != admin.DisplayName {
			return "", ErrAccountConflict
		}
		if err := bcrypt.CompareHashAndPassword([]byte(existingHash), []byte(admin.Password)); err != nil {
			return "", errors.New("bootstrap retry must use the original password")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit pending administrator account: %w", err)
	}
	return *accountID, nil
}

// Complete activates the pending account and consumes the one-time secret.
func Complete(ctx context.Context, db database, secret, accountID string) error {
	if db == nil {
		return errors.New("identity bootstrap database is unavailable")
	}
	if len(secret) < minimumSecretLength || strings.TrimSpace(accountID) == "" {
		return ErrBootstrapSecret
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Identity bootstrap completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storedHash *string
	var storedExpiry time.Time
	var storedAccountID *string
	var completedAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT secret_hash, expires_at, account_id, completed_at
		FROM identity.initial_admin_bootstrap
		WHERE singleton = TRUE
		FOR UPDATE
	`).Scan(&storedHash, &storedExpiry, &storedAccountID, &completedAt); err != nil {
		return fmt.Errorf("read Identity bootstrap state: %w", err)
	}
	if completedAt != nil {
		return ErrBootstrapComplete
	}
	if storedHash == nil || *storedHash != hashSecret(secret) ||
		!time.Now().Before(storedExpiry) || storedAccountID == nil || *storedAccountID != accountID {
		return ErrBootstrapSecret
	}

	tag, err := tx.Exec(ctx, `
		UPDATE identity.user_accounts
		SET is_active = TRUE, updated_at = NOW()
		WHERE id = $1 AND is_active = FALSE
	`, accountID)
	if err != nil {
		return fmt.Errorf("activate initial administrator: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrAccountConflict
	}
	if _, err := tx.Exec(ctx, `
		UPDATE identity.initial_admin_bootstrap
		SET secret_hash = NULL, completed_at = NOW()
		WHERE singleton = TRUE
	`); err != nil {
		return fmt.Errorf("consume initial administrator bootstrap secret: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit initial administrator activation: %w", err)
	}
	return nil
}

func validateSecret(secret string, expiresAt, now time.Time) error {
	if len(secret) < minimumSecretLength || strings.TrimSpace(secret) != secret ||
		expiresAt.Before(now.Add(time.Minute)) || expiresAt.After(now.Add(maximumSecretAge)) {
		return ErrBootstrapSecret
	}
	return nil
}

// ValidateBootstrapSecret checks that an operator-provided secret has an
// acceptable length and expiry before prompting for account details.
func ValidateBootstrapSecret(secret string, expiresAt time.Time) error {
	return validateSecret(secret, expiresAt, time.Now())
}

func validateAdmin(admin InitialAdmin) error {
	admin.StaffID = strings.TrimSpace(admin.StaffID)
	admin.Email = strings.TrimSpace(admin.Email)
	admin.DisplayName = strings.TrimSpace(admin.DisplayName)
	if admin.StaffID == "" || len(admin.StaffID) > 80 ||
		admin.Email == "" || len(admin.Email) > 254 ||
		admin.DisplayName == "" || len(admin.DisplayName) > 160 {
		return errors.New("staff ID, email, and display name are required and must fit the account field limits")
	}
	parsedEmail, err := mail.ParseAddress(admin.Email)
	if err != nil || parsedEmail.Address != admin.Email {
		return errors.New("initial administrator email address is invalid")
	}
	if err := validatePassword(admin.Password); err != nil {
		return err
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minimumPasswordSize || len(password) > maximumPasswordSize {
		return fmt.Errorf("password must be between %d and %d bytes", minimumPasswordSize, maximumPasswordSize)
	}
	var upper, lower, digit, symbol bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsLower(char):
			lower = true
		case unicode.IsDigit(char):
			digit = true
		default:
			symbol = true
		}
	}
	classes := 0
	for _, present := range []bool{upper, lower, digit, symbol} {
		if present {
			classes++
		}
	}
	if classes < 3 {
		return errors.New("password must include characters from at least three of uppercase, lowercase, digits, and symbols")
	}
	return nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
