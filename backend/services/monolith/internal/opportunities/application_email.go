package opportunities

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	applicationEmailBatchSize = 25
	applicationEmailInterval  = 2 * time.Second
	applicationEmailTimeout   = 15 * time.Second
)

type ApplicationEmailDispatcher struct {
	db     *sql.DB
	mailer ApplicationStatusMailer
	logger *slog.Logger
}

func NewApplicationEmailDispatcher(db *sql.DB, mailer ApplicationStatusMailer, logger *slog.Logger) *ApplicationEmailDispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &ApplicationEmailDispatcher{db: db, mailer: mailer, logger: logger}
}

func (d *ApplicationEmailDispatcher) Run(ctx context.Context) {
	if d == nil || d.db == nil || d.mailer == nil {
		if d != nil && d.mailer == nil {
			d.logger.WarnContext(ctx, "SIWES applicant email delivery is not configured; queued emails will remain pending")
		}
		return
	}
	ticker := time.NewTicker(applicationEmailInterval)
	defer ticker.Stop()
	for {
		if _, err := d.ProcessBatch(ctx); err != nil && ctx.Err() == nil {
			d.logger.ErrorContext(ctx, "process SIWES applicant email outbox batch", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *ApplicationEmailDispatcher) ProcessBatch(ctx context.Context) (int, error) {
	if d == nil || d.db == nil {
		return 0, errors.New("SIWES applicant email outbox database is unavailable")
	}
	if d.mailer == nil {
		return 0, errors.New("SIWES applicant email delivery is not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, applicationEmailTimeout)
	rows, err := d.db.QueryContext(queryCtx, `
		SELECT id::text FROM opportunities.application_email_outbox
		WHERE processed_at IS NULL AND available_at <= now()
		ORDER BY created_at, id
		LIMIT $1
	`, applicationEmailBatchSize)
	if err != nil {
		cancel()
		return 0, fmt.Errorf("select pending SIWES applicant emails: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			cancel()
			return 0, fmt.Errorf("scan pending SIWES applicant email: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		cancel()
		return 0, fmt.Errorf("read pending SIWES applicant emails: %w", err)
	}
	if err := rows.Close(); err != nil {
		cancel()
		return 0, fmt.Errorf("close pending SIWES applicant emails: %w", err)
	}
	cancel()

	processed := 0
	var failures []error
	for _, id := range ids {
		eventCtx, cancelEvent := context.WithTimeout(ctx, applicationEmailTimeout)
		err := d.processOne(eventCtx, id)
		cancelEvent()
		if err != nil {
			if retryErr := d.recordFailure(ctx, id, err); retryErr != nil {
				failures = append(failures, fmt.Errorf("record retry for SIWES email %s: %w", id, retryErr))
			}
			failures = append(failures, fmt.Errorf("deliver SIWES email %s: %w", id, err))
			continue
		}
		processed++
	}
	return processed, errors.Join(failures...)
}

func (d *ApplicationEmailDispatcher) processOne(ctx context.Context, id string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SIWES email delivery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var eventID, status, applicantName, recipient, opportunityTitle string
	err = tx.QueryRowContext(ctx, `
		SELECT e.id::text, e.status, a.full_name, a.email, o.title
		FROM opportunities.application_email_outbox e
		JOIN opportunities.siwes_applications a ON a.id=e.application_id
		JOIN opportunities.opportunities o ON o.id=a.opportunity_id
		WHERE e.id=$1::uuid AND e.processed_at IS NULL AND e.available_at <= now()
		FOR UPDATE OF e SKIP LOCKED
	`, id).Scan(&eventID, &status, &applicantName, &recipient, &opportunityTitle)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("finish skipped SIWES email: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock SIWES email event: %w", err)
	}
	if err := d.mailer.SendApplicationStatus(ctx, recipient, applicantName, opportunityTitle, status); err != nil {
		return fmt.Errorf("send SIWES status email: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE opportunities.application_email_outbox
		SET processed_at=now(), last_error=NULL
		WHERE id=$1::uuid AND processed_at IS NULL
	`, eventID); err != nil {
		return fmt.Errorf("mark SIWES email processed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SIWES email delivery: %w", err)
	}
	return nil
}

func (d *ApplicationEmailDispatcher) recordFailure(ctx context.Context, id string, deliveryErr error) error {
	message := strings.TrimSpace(deliveryErr.Error())
	if runes := []rune(message); len(runes) > 1000 {
		message = string(runes[:1000])
	}
	retryCtx, cancel := context.WithTimeout(ctx, applicationEmailTimeout)
	defer cancel()
	_, err := d.db.ExecContext(retryCtx, `
		UPDATE opportunities.application_email_outbox
		SET attempts=attempts+1,
		    available_at=now() + make_interval(secs =>
		        LEAST(3600, 5 * power(2, LEAST(attempts, 10)))::integer
		    ),
		    last_error=$2
		WHERE id=$1::uuid AND processed_at IS NULL
	`, id, message)
	if err != nil {
		return fmt.Errorf("update failed SIWES email event: %w", err)
	}
	return nil
}

func enqueueApplicationEmail(ctx context.Context, tx *sql.Tx, applicationID, status string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO opportunities.application_email_outbox (application_id, status)
		VALUES ($1::uuid, $2)
		ON CONFLICT (application_id, status) DO NOTHING
	`, applicationID, status); err != nil {
		return fmt.Errorf("insert SIWES email event: %w", err)
	}
	return nil
}
