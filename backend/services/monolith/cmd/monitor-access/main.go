package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/systemmonitor"
)

var accountIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "monitor access operation failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: monitor-access grant|revoke [flags]")
	}
	request, err := parseRequest(args)
	if err != nil {
		return err
	}
	databaseURL := strings.TrimSpace(os.Getenv("SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL"))
	if err := validateDatabaseURL(databaseURL); err != nil {
		return err
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("could not configure authorization database connection")
	}
	defer func() { _ = db.Close() }()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return errors.New("could not connect to authorization database")
	}

	switch value := request.(type) {
	case systemmonitor.GrantRequest:
		if err := systemmonitor.GrantAccess(ctx, db, value); err != nil {
			return err
		}
		fmt.Printf("System Monitor access granted to account %s until %s.\n",
			value.AccountID, value.ExpiresAt.UTC().Format(time.RFC3339))
	case systemmonitor.RevokeRequest:
		if err := systemmonitor.RevokeAccess(ctx, db, value); err != nil {
			return err
		}
		fmt.Printf("Active System Monitor access revoked for account %s.\n", value.AccountID)
	default:
		return errors.New("unsupported monitor access operation")
	}
	return nil
}

func parseRequest(args []string) (any, error) {
	operation := args[0]
	flags := flag.NewFlagSet("monitor-access "+operation, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	accountID := flags.String("account-id", "", "Target account UUID")
	operator := flags.String("operator", "", "Trusted operator identifier from the approved change")
	reason := flags.String("reason", "", "Approval/ticket reference and reason")
	expiresAt := flags.String("expires-at", "", "Required grant expiry in RFC3339 format (maximum 30 days)")
	if err := flags.Parse(args[1:]); err != nil {
		return nil, err
	}
	if flags.NArg() != 0 || !accountIDPattern.MatchString(*accountID) ||
		strings.TrimSpace(*operator) == "" || strings.TrimSpace(*reason) == "" {
		return nil, errors.New("account-id, operator, and reason are required; account-id must be a UUID")
	}
	switch operation {
	case "grant":
		if *expiresAt == "" {
			return nil, errors.New("grant requires expires-at in RFC3339 format")
		}
		expiry, err := time.Parse(time.RFC3339, *expiresAt)
		if err != nil {
			return nil, errors.New("expires-at must be an RFC3339 timestamp")
		}
		return systemmonitor.GrantRequest{
			AccountID: *accountID, Operator: *operator, Reason: *reason, ExpiresAt: expiry,
		}, nil
	case "revoke":
		if *expiresAt != "" {
			return nil, errors.New("expires-at is only valid for grant")
		}
		return systemmonitor.RevokeRequest{
			AccountID: *accountID, Operator: *operator, Reason: *reason,
		}, nil
	default:
		return nil, errors.New("operation must be grant or revoke")
	}
}

func validateDatabaseURL(databaseURL string) error {
	if databaseURL == "" {
		return errors.New("SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL is required")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil {
		return errors.New("authorization database URL must be a valid PostgreSQL connection URL")
	}
	switch parsed.Query().Get("sslmode") {
	case "require", "verify-ca", "verify-full":
		return nil
	default:
		return errors.New("authorization database URL must require TLS")
	}
}
