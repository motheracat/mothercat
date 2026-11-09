package opportunities

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

const (
	retentionCheckInterval = 24 * time.Hour
)

type RetentionJob struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewRetentionJob(db *sql.DB, logger *slog.Logger) *RetentionJob {
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionJob{db: db, logger: logger}
}

func (j *RetentionJob) Run(ctx context.Context) {
	ticker := time.NewTicker(retentionCheckInterval)
	defer ticker.Stop()
	for {
		if _, err := j.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
			j.logger.ErrorContext(ctx, "purge expired SIWES applications", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *RetentionJob) PurgeExpired(ctx context.Context) (int64, error) {
	if j == nil || j.db == nil {
		return 0, errors.New("SIWES application retention database is unavailable")
	}
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := j.db.ExecContext(queryCtx, `
		DELETE FROM opportunities.siwes_applications
		WHERE submitted_at < now() - interval '365 days'
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
