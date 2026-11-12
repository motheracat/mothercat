package systemmonitor

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	mfaCodeDigits      = 6
	mfaStepSeconds     = 30
	mfaEnrollmentTTL   = 10 * time.Minute
	mfaFailureLimit    = 10
	securityEventLimit = 200
)

var (
	ErrSecurityUnavailable = errors.New("System Monitor security is unavailable")
	ErrMFAAlreadyEnabled   = errors.New("System Monitor MFA is already enabled")
	ErrMFANotEnabled       = errors.New("System Monitor MFA is not enabled")
	ErrInvalidMFACode      = errors.New("invalid System Monitor MFA code")
	ErrMFARateLimited      = errors.New("System Monitor MFA verification is rate limited")
	ErrInvalidSecurityInput = errors.New("invalid System Monitor security input")
	ErrCurrentSession      = errors.New("cannot revoke the current session")
	ErrSessionUnavailable  = errors.New("session is unavailable")
)

type SecurityService struct {
	db  *sql.DB
	key []byte
}

type MFAEnrollment struct {
	Secret     string    `json:"secret"`
	OTPAuthURI string    `json:"otpauth_uri"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type SecurityStatus struct {
	MFAEnabled bool `json:"mfa_enabled"`
}

type MonitorSession struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Current   bool      `json:"current"`
}

type SecurityEvent struct {
	ID        int64           `json:"id"`
	AccountID *string         `json:"account_id,omitempty"`
	EventType string          `json:"event_type"`
	Severity  string          `json:"severity"`
	Details   json.RawMessage `json:"details"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type SecurityAlert struct {
	Source      string    `json:"source"`
	EventType   string    `json:"event_type"`
	Severity    string    `json:"severity"`
	AccountID   *string   `json:"account_id,omitempty"`
	FailureCount int64    `json:"failure_count,omitempty"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type SecurityAuditEvent struct {
	ActorType   string          `json:"actor_type"`
	ActorID     string          `json:"actor_id"`
	Action      string          `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID  string          `json:"resource_id"`
	Details     json.RawMessage `json:"details"`
	OccurredAt  time.Time       `json:"occurred_at"`
}

type MonitorAccessGrant struct {
	AccountID  string    `json:"account_id"`
	GrantedBy  string    `json:"granted_by"`
	Reason     string    `json:"reason"`
	GrantedAt  time.Time `json:"granted_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func NewSecurityService(db *sql.DB, encodedEncryptionKey string) (*SecurityService, error) {
	if db == nil {
		return nil, ErrSecurityUnavailable
	}
	key, err := base64.StdEncoding.DecodeString(encodedEncryptionKey)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(encodedEncryptionKey)
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%w: SYSTEM_MONITOR_MFA_ENCRYPTION_KEY must be base64-encoded 32-byte key",
			ErrSecurityUnavailable)
	}
	return &SecurityService{db: db, key: key}, nil
}

func (s *SecurityService) Status(ctx context.Context, accountID string) (SecurityStatus, error) {
	var enabled bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM system_monitor.mfa_credentials WHERE account_id = $1::uuid
		)
	`, accountID).Scan(&enabled); err != nil {
		return SecurityStatus{}, fmt.Errorf("read System Monitor MFA status: %w", err)
	}
	return SecurityStatus{MFAEnabled: enabled}, nil
}

func (s *SecurityService) BeginMFAEnrollment(ctx context.Context, accountID string) (MFAEnrollment, error) {
	var enabled bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM system_monitor.mfa_credentials WHERE account_id = $1::uuid
		)
	`, accountID).Scan(&enabled); err != nil {
		return MFAEnrollment{}, fmt.Errorf("check existing System Monitor MFA: %w", err)
	}
	if enabled {
		return MFAEnrollment{}, ErrMFAAlreadyEnabled
	}
	var secretBytes [20]byte
	if _, err := rand.Read(secretBytes[:]); err != nil {
		return MFAEnrollment{}, fmt.Errorf("generate System Monitor MFA secret: %w", err)
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes[:])
	encrypted, err := s.encrypt([]byte(secret))
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("encrypt System Monitor MFA secret: %w", err)
	}
	expiresAt := time.Now().UTC().Add(mfaEnrollmentTTL)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO system_monitor.mfa_enrollment_challenges (account_id, secret_ciphertext, expires_at)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (account_id) DO UPDATE SET
			secret_ciphertext = EXCLUDED.secret_ciphertext,
			expires_at = EXCLUDED.expires_at,
			created_at = now()
	`, accountID, encrypted, expiresAt)
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("save System Monitor MFA enrollment challenge: %w", err)
	}
	label := url.PathEscape("Mothercat:System Monitor " + accountID)
	issuer := url.QueryEscape("Mothercat System Monitor")
	uri := "otpauth://totp/" + label + "?secret=" + secret +
		"&issuer=" + issuer + "&algorithm=SHA1&digits=6&period=30"
	return MFAEnrollment{Secret: secret, OTPAuthURI: uri, ExpiresAt: expiresAt}, nil
}

func (s *SecurityService) ConfirmMFAEnrollment(ctx context.Context, accountID, code string) error {
	if !validMFACode(code) {
		return ErrInvalidMFACode
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin System Monitor MFA enrollment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var encrypted []byte
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT secret_ciphertext, expires_at
		FROM system_monitor.mfa_enrollment_challenges
		WHERE account_id = $1::uuid
		FOR UPDATE
	`, accountID).Scan(&encrypted, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !expiresAt.After(time.Now())) {
		return ErrInvalidMFACode
	}
	if err != nil {
		return fmt.Errorf("load System Monitor MFA enrollment: %w", err)
	}
	var failures int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM system_monitor.security_events
		WHERE account_id = $1::uuid AND event_type = 'mfa.enrollment_failed'
		  AND occurred_at > now() - interval '15 minutes'
	`, accountID).Scan(&failures); err != nil {
		return fmt.Errorf("check System Monitor MFA enrollment rate limit: %w", err)
	}
	if failures >= mfaFailureLimit {
		return ErrMFARateLimited
	}
	secret, err := s.decrypt(encrypted)
	if err != nil {
		return fmt.Errorf("decrypt System Monitor MFA enrollment: %w", err)
	}
	step, valid := verifyTOTP(string(secret), code, time.Now())
	if !valid {
		if err := recordSecurityEvent(ctx, tx, accountID, "mfa.enrollment_failed", "warning",
			map[string]string{"reason": "invalid_code"}); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit failed System Monitor MFA enrollment attempt: %w", err)
		}
		return ErrInvalidMFACode
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO system_monitor.mfa_credentials (account_id, secret_ciphertext, enabled_at, last_used_step)
		VALUES ($1::uuid, $2, now(), $3)
		ON CONFLICT (account_id) DO NOTHING
	`, accountID, encrypted, step)
	if err != nil {
		return fmt.Errorf("enable System Monitor MFA: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM system_monitor.mfa_enrollment_challenges WHERE account_id = $1::uuid
	`, accountID); err != nil {
		return fmt.Errorf("remove System Monitor MFA enrollment challenge: %w", err)
	}
	if err := recordSecurityEvent(ctx, tx, accountID, "mfa.enabled", "info", map[string]string{}); err != nil {
		return err
	}
	if err := recordSecurityAudit(ctx, tx, accountID, "system_monitor.mfa.enabled",
		"system_monitor_mfa", accountID, map[string]string{}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit System Monitor MFA enrollment: %w", err)
	}
	return nil
}

func (s *SecurityService) DisableMFA(ctx context.Context, accountID, reason string) error {
	if !validReason(reason) {
		return ErrInvalidSecurityInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin System Monitor MFA removal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		DELETE FROM system_monitor.mfa_credentials WHERE account_id = $1::uuid
	`, accountID)
	if err != nil {
		return fmt.Errorf("remove System Monitor MFA: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count removed System Monitor MFA credentials: %w", err)
	}
	if count == 0 {
		return ErrMFANotEnabled
	}
	details := map[string]string{"reason": reason}
	if err := recordSecurityEvent(ctx, tx, accountID, "mfa.disabled", "critical", details); err != nil {
		return err
	}
	if err := recordSecurityAudit(ctx, tx, accountID, "system_monitor.mfa.disabled",
		"system_monitor_mfa", accountID, details); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit System Monitor MFA removal: %w", err)
	}
	return nil
}

func (s *SecurityService) VerifyMFA(ctx context.Context, accountID, code string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin System Monitor MFA verification: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var encrypted []byte
	var lastStep int64
	err = tx.QueryRowContext(ctx, `
		SELECT secret_ciphertext, last_used_step
		FROM system_monitor.mfa_credentials
		WHERE account_id = $1::uuid
		FOR UPDATE
	`, accountID).Scan(&encrypted, &lastStep)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMFANotEnabled
	}
	if err != nil {
		return fmt.Errorf("load System Monitor MFA credential: %w", err)
	}
	var failures int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM system_monitor.security_events
		WHERE account_id = $1::uuid AND event_type = 'mfa.verification_failed'
		  AND occurred_at > now() - interval '15 minutes'
	`, accountID).Scan(&failures); err != nil {
		return fmt.Errorf("check System Monitor MFA rate limit: %w", err)
	}
	if failures >= mfaFailureLimit {
		return ErrMFARateLimited
	}
	secret, err := s.decrypt(encrypted)
	if err != nil {
		return fmt.Errorf("decrypt System Monitor MFA credential: %w", err)
	}
	step, valid := verifyTOTP(string(secret), code, time.Now())
	if !valid || step <= lastStep {
		if err := recordSecurityEvent(ctx, tx, accountID, "mfa.verification_failed", "warning",
			map[string]string{"reason": "invalid_or_replayed_code"}); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit failed System Monitor MFA verification: %w", err)
		}
		return ErrInvalidMFACode
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE system_monitor.mfa_credentials SET last_used_step = $2
		WHERE account_id = $1::uuid
	`, accountID, step); err != nil {
		return fmt.Errorf("consume System Monitor MFA code: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit System Monitor MFA verification: %w", err)
	}
	return nil
}

func (s *SecurityService) ListSessions(ctx context.Context, accountID, currentTokenHash string) ([]MonitorSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, created_at, expires_at, token_hash = $2
		FROM identity.sessions
		WHERE account_id = $1::uuid AND revoked_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 100
	`, accountID, currentTokenHash)
	if err != nil {
		return nil, fmt.Errorf("list active System Monitor account sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	sessions := make([]MonitorSession, 0)
	for rows.Next() {
		var session MonitorSession
		if err := rows.Scan(&session.ID, &session.CreatedAt, &session.ExpiresAt, &session.Current); err != nil {
			return nil, fmt.Errorf("read System Monitor session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate System Monitor sessions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close System Monitor sessions: %w", err)
	}
	return sessions, nil
}

func (s *SecurityService) RevokeSession(ctx context.Context, accountID, sessionID, currentTokenHash, reason string) error {
	if !validOperator(accountID) || !validReason(reason) {
		return ErrInvalidSecurityInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin System Monitor session revocation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var revokedID string
	err = tx.QueryRowContext(ctx, `
		UPDATE identity.sessions
		SET revoked_at = now()
		WHERE id = $1::uuid AND account_id = $2::uuid AND token_hash <> $3
		  AND revoked_at IS NULL AND expires_at > now()
		RETURNING id::text
	`, sessionID, accountID, currentTokenHash).Scan(&revokedID)
	if errors.Is(err, sql.ErrNoRows) {
		var current bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM identity.sessions
				WHERE id = $1::uuid AND account_id = $2::uuid AND token_hash = $3
			)
		`, sessionID, accountID, currentTokenHash).Scan(&current); err != nil {
			return fmt.Errorf("check current System Monitor session: %w", err)
		}
		if current {
			return ErrCurrentSession
		}
		return ErrSessionUnavailable
	}
	if err != nil {
		return fmt.Errorf("revoke System Monitor account session: %w", err)
	}
	details := map[string]string{"session_id": revokedID, "reason": reason}
	if err := recordSecurityEvent(ctx, tx, accountID, "session.revoked", "warning", details); err != nil {
		return err
	}
	if err := recordSecurityAudit(ctx, tx, accountID, "system_monitor.session.revoked",
		"identity_session", revokedID, details); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit System Monitor session revocation: %w", err)
	}
	return nil
}

func (s *SecurityService) Events(ctx context.Context, limit int) ([]SecurityEvent, error) {
	if limit < 1 || limit > securityEventLimit {
		return nil, ErrInvalidSecurityInput
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id::text, event_type, severity, details, occurred_at
		FROM system_monitor.security_events
		ORDER BY occurred_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("read System Monitor security events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]SecurityEvent, 0)
	for rows.Next() {
		var event SecurityEvent
		var accountID sql.NullString
		if err := rows.Scan(&event.ID, &accountID, &event.EventType, &event.Severity,
			&event.Details, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan System Monitor security event: %w", err)
		}
		if accountID.Valid {
			event.AccountID = &accountID.String
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate System Monitor security events: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close System Monitor security events: %w", err)
	}
	return events, nil
}

func (s *SecurityService) Alerts(ctx context.Context, limit int) ([]SecurityAlert, error) {
	if limit < 1 || limit > securityEventLimit {
		return nil, ErrInvalidSecurityInput
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH authentication_bursts AS (
			SELECT account_id, event_type, count(*) AS failure_count, max(occurred_at) AS occurred_at
			FROM identity.authentication_events
			WHERE NOT successful AND occurred_at > now() - interval '24 hours'
			GROUP BY account_id, event_type
			HAVING count(*) >= 5
		),
		security_alerts AS (
			SELECT account_id, event_type, severity, occurred_at, 0::bigint AS failure_count
			FROM system_monitor.security_events
			WHERE severity IN ('warning', 'critical') AND occurred_at > now() - interval '24 hours'
		)
		SELECT 'authentication' AS source, event_type, 'warning' AS severity,
		       account_id::text, failure_count, occurred_at
		FROM authentication_bursts
		UNION ALL
		SELECT 'system_monitor', event_type, severity, account_id::text, failure_count, occurred_at
		FROM security_alerts
		ORDER BY occurred_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("read System Monitor security alerts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	alerts := make([]SecurityAlert, 0)
	for rows.Next() {
		var alert SecurityAlert
		var accountID sql.NullString
		if err := rows.Scan(&alert.Source, &alert.EventType, &alert.Severity,
			&accountID, &alert.FailureCount, &alert.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan System Monitor security alert: %w", err)
		}
		if accountID.Valid {
			alert.AccountID = &accountID.String
		}
		alerts = append(alerts, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate System Monitor security alerts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close System Monitor security alerts: %w", err)
	}
	return alerts, nil
}

func (s *SecurityService) AuditEvents(ctx context.Context, limit int) ([]SecurityAuditEvent, error) {
	if limit < 1 || limit > securityEventLimit {
		return nil, ErrInvalidSecurityInput
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH monitor_audit AS (
			SELECT id AS row_id, actor_type, actor_id, action, resource_type, resource_id, details, occurred_at
			FROM audit.audit_events
			WHERE resource_type LIKE 'system_monitor%' OR action LIKE 'system_monitor.%'
			UNION ALL
			SELECT id AS row_id, actor_type, COALESCE(actor_id::text, ''), action,
			       'endpoint_control', method || ' ' || route,
			       jsonb_build_object(
			           'reason', reason,
			           'response_status', response_status,
			           'expires_at', expires_at
			       ), changed_at
			FROM system_monitor.endpoint_control_history
		)
		SELECT actor_type, actor_id, action, resource_type, resource_id, details, occurred_at
		FROM monitor_audit
		ORDER BY occurred_at DESC, row_id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("read System Monitor privileged audit events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]SecurityAuditEvent, 0)
	for rows.Next() {
		var event SecurityAuditEvent
		if err := rows.Scan(&event.ActorType, &event.ActorID, &event.Action,
			&event.ResourceType, &event.ResourceID, &event.Details, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan System Monitor privileged audit event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate System Monitor privileged audit events: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close System Monitor privileged audit events: %w", err)
	}
	return events, nil
}

func (s *SecurityService) ActiveGrants(ctx context.Context, limit int) ([]MonitorAccessGrant, error) {
	if limit < 1 || limit > securityEventLimit {
		return nil, ErrInvalidSecurityInput
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.account_id::text, g.granted_by, g.grant_reason, g.granted_at, g.expires_at
		FROM system_monitor.access_grants g
		JOIN identity.user_accounts a ON a.id = g.account_id
		WHERE g.revoked_at IS NULL AND g.expires_at > now()
		  AND a.is_active AND a.suspended_at IS NULL
		ORDER BY g.expires_at, g.granted_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("read active System Monitor grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	grants := make([]MonitorAccessGrant, 0)
	for rows.Next() {
		var grant MonitorAccessGrant
		if err := rows.Scan(&grant.AccountID, &grant.GrantedBy, &grant.Reason,
			&grant.GrantedAt, &grant.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan active System Monitor grant: %w", err)
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active System Monitor grants: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close active System Monitor grants: %w", err)
	}
	return grants, nil
}

func (s *SecurityService) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (s *SecurityService) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted System Monitor MFA secret")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func verifyTOTP(encodedSecret, code string, now time.Time) (int64, bool) {
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encodedSecret))
	if err != nil || !validMFACode(code) {
		return 0, false
	}
	currentStep := now.Unix() / mfaStepSeconds
	for step := currentStep - 1; step <= currentStep+1; step++ {
		if step < 0 {
			continue
		}
		var counter [8]byte
		binary.BigEndian.PutUint64(counter[:], uint64(step))
		mac := hmac.New(sha1.New, secret)
		_, _ = mac.Write(counter[:])
		sum := mac.Sum(nil)
		offset := sum[len(sum)-1] & 0x0f
		value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
		expected := fmt.Sprintf("%06d", value%1000000)
		if hmac.Equal([]byte(expected), []byte(code)) {
			return step, true
		}
	}
	return 0, false
}

func validMFACode(code string) bool {
	if len(code) != mfaCodeDigits {
		return false
	}
	_, err := strconv.Atoi(code)
	return err == nil
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func recordSecurityEvent(ctx context.Context, tx *sql.Tx, accountID, eventType, severity string, details any) error {
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode System Monitor security event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO system_monitor.security_events (account_id, event_type, severity, details)
		VALUES ($1::uuid, $2, $3, $4::jsonb)
	`, accountID, eventType, severity, encoded); err != nil {
		return fmt.Errorf("record System Monitor security event: %w", err)
	}
	return nil
}

func recordSecurityAudit(ctx context.Context, tx *sql.Tx, actorID, action, resourceType, resourceID string, details any) error {
	encoded, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode System Monitor security audit details: %w", err)
	}
	eventKey, err := newEventKey()
	if err != nil {
		return fmt.Errorf("generate System Monitor security audit key: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit.audit_events (
			id, event_key, actor_type, actor_id, action, resource_type, resource_id, details
		) VALUES (gen_random_uuid(), $1, 'user', $2, $3, $4, $5, $6::jsonb)
	`, eventKey, actorID, action, resourceType, resourceID, encoded); err != nil {
		return fmt.Errorf("record System Monitor security audit event: %w", err)
	}
	return nil
}
