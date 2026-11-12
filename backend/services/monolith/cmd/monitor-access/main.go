package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/systemmonitor"
)

var accountIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
	workingDir, err := os.Getwd()
	if err != nil {
		return errors.New("could not determine current directory")
	}
	databaseURL, err := resolveAuthorizationDatabaseURL(os.LookupEnv, workingDir)
	if err != nil {
		return err
	}
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

func resolveAuthorizationDatabaseURL(
	lookup func(string) (string, bool),
	workingDir string,
) (string, error) {
	for _, key := range []string{"SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL", "DATABASE_URL"} {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}

	envPath, found, err := findBackendEnvFile(workingDir)
	if err != nil {
		return "", err
	}
	if found {
		values, err := readEnvFile(envPath)
		if err != nil {
			return "", fmt.Errorf("could not read backend/.env: %w", err)
		}
		for _, key := range []string{"SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL", "DATABASE_URL"} {
			if value := strings.TrimSpace(values[key]); value != "" {
				return value, nil
			}
		}
	}
	return "", errors.New("SYSTEM_MONITOR_AUTHORIZATION_DATABASE_URL is required; set it in the shell or define it (or DATABASE_URL) in backend/.env")
}

func findBackendEnvFile(startDir string) (string, bool, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false, errors.New("could not resolve current directory")
	}

	for {
		candidate := filepath.Join(dir, "backend", ".env")
		if filepath.Base(dir) == "backend" {
			candidate = filepath.Join(dir, ".env")
		}
		info, err := os.Stat(candidate)
		if err == nil {
			if !info.Mode().IsRegular() {
				return "", false, errors.New("backend/.env is not a regular file")
			}
			return candidate, true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", false, errors.New("could not access backend/.env")
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false, nil
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("file could not be opened")
	}
	defer func() { _ = file.Close() }()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, rawValue, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid assignment on line %d", lineNumber)
		}
		value, err := parseEnvValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("invalid value syntax on line %d", lineNumber)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("file could not be read")
	}
	return values, nil
}

func parseEnvValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	switch value[0] {
	case '"':
	case '\'':
	default:
		if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		return value, nil
	}

	quote := value[0]
	escaped := false
	for index := 1; index < len(value); index++ {
		character := value[index]
		if quote == '"' && escaped {
			escaped = false
			continue
		}
		if quote == '"' && character == '\\' {
			escaped = true
			continue
		}
		if character != quote {
			continue
		}
		suffix := strings.TrimSpace(value[index+1:])
		if suffix != "" && !strings.HasPrefix(suffix, "#") {
			return "", errors.New("unexpected text after quoted value")
		}
		if quote == '\'' {
			return value[1:index], nil
		}
		unquoted, err := strconv.Unquote(value[:index+1])
		if err != nil {
			return "", err
		}
		return unquoted, nil
	}
	return "", errors.New("unclosed quoted value")
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
