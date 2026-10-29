package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
)

var ErrInactiveAccount = errors.New("inactive_account")
var ErrAccountNotFound = errors.New("account_not_found")
var ErrSessionNotFound = errors.New("session_not_found")

const SessionTTL = time.Hour

// Repository defines the data access contract required for Identity auth flows.
type Repository interface {
	FindByStaffID(ctx context.Context, staffID string) (AccountRecord, error)
	FindBySessionToken(ctx context.Context, token string) (AccountRecord, error)
	CreateSession(ctx context.Context, accountID, staffID, token string, expiresAt time.Time) error
	RevokeSession(ctx context.Context, token string) error
	RecordAuthenticationEvent(ctx context.Context, staffID string, accountID *string, successful bool) error
}

// AccountRecord is the persistence representation of a user account. It keeps the
// password hash server-side and converts to the public domain account shape.
type AccountRecord struct {
	ID                 string
	StaffID            string
	Email              string
	DisplayName        string
	PasswordHash       string
	IsActive           bool
	MustChangePassword bool
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Login(ctx context.Context, staffID, password string) (identitydomain.Account, string, error) {
	if s == nil || s.repository == nil {
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}
	if strings.TrimSpace(staffID) == "" || password == "" {
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}

	accountRecord, err := s.repository.FindByStaffID(ctx, staffID)
	if err != nil {
		if !errors.Is(err, ErrAccountNotFound) {
			return identitydomain.Account{}, "", err
		}
		if eventErr := s.repository.RecordAuthenticationEvent(ctx, staffID, nil, false); eventErr != nil {
			return identitydomain.Account{}, "", eventErr
		}
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}
	if !accountRecord.IsActive {
		if eventErr := s.repository.RecordAuthenticationEvent(ctx, staffID, &accountRecord.ID, false); eventErr != nil {
			return identitydomain.Account{}, "", eventErr
		}
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}
	if err := verifyPasswordHash(accountRecord.PasswordHash, password); err != nil {
		if eventErr := s.repository.RecordAuthenticationEvent(ctx, staffID, &accountRecord.ID, false); eventErr != nil {
			return identitydomain.Account{}, "", eventErr
		}
		return identitydomain.Account{}, "", identitydomain.ErrInvalidCredentials
	}

	sessionToken, err := newSessionToken(rand.Reader)
	if err != nil {
		return identitydomain.Account{}, "", err
	}
	if err := s.repository.CreateSession(ctx, accountRecord.ID, accountRecord.StaffID, sessionToken, time.Now().Add(SessionTTL)); err != nil {
		return identitydomain.Account{}, "", err
	}

	return identitydomain.Account{
		ID:          accountRecord.ID,
		StaffID:     accountRecord.StaffID,
		DisplayName: accountRecord.DisplayName,
		Email:       accountRecord.Email,
		Roles:       nil,
	}, sessionToken, nil
}

func (s *Service) ResolveSession(ctx context.Context, sessionToken string) (identitydomain.Account, error) {
	if s == nil || s.repository == nil {
		return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
	}
	accountRecord, err := s.repository.FindBySessionToken(ctx, sessionToken)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return identitydomain.Account{}, identitydomain.ErrInvalidCredentials
		}
		return identitydomain.Account{}, err
	}
	return identitydomain.Account{
		ID:          accountRecord.ID,
		StaffID:     accountRecord.StaffID,
		DisplayName: accountRecord.DisplayName,
		Email:       accountRecord.Email,
		Roles:       nil,
	}, nil
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if s == nil || s.repository == nil {
		return identitydomain.ErrInvalidCredentials
	}
	return s.repository.RevokeSession(ctx, sessionToken)
}

func verifyPasswordHash(passwordHash, password string) error {
	if passwordHash == "" || password == "" {
		return identitydomain.ErrInvalidCredentials
	}
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return identitydomain.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return identitydomain.ErrInvalidCredentials
	}
	return nil
}

func newSessionToken(source io.Reader) (string, error) {
	if source == nil {
		return "", errors.New("secure random source unavailable")
	}
	buf := make([]byte, 32)
	if _, err := io.ReadFull(source, buf); err != nil {
		return "", errors.New("generate session token: secure random source failed")
	}
	return "sess_" + hex.EncodeToString(buf), nil
}
