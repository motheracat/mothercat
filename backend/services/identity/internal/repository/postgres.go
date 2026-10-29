package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/config"
)

// NewPool creates the Identity service's PostgreSQL connection pool. Database
// availability is reported through readiness so the process can recover from a
// transient database outage without a restart. It does not apply migrations.
func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL connection configuration")
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = 0
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute
	poolConfig.ConnConfig.ConnectTimeout = cfg.DBConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL connection pool: %w", err)
	}
	return pool, nil
}
