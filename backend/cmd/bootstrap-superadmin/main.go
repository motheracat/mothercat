package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accessbootstrap "github.com/mothercat-enterprise/mothercat/backend/services/access/bootstrap"
	auditbootstrap "github.com/mothercat-enterprise/mothercat/backend/services/audit/bootstrap"
	identitybootstrap "github.com/mothercat-enterprise/mothercat/backend/services/identity/bootstrap"
	"golang.org/x/term"
)

type options struct {
	databaseURL string
	secret      string
	expiresAt   time.Time
	appEnv      string
}

func main() {
	if err := run(); err != nil {
		slog.Error("initial SuperAdmin bootstrap failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("bootstrap requires an interactive terminal; do not pipe account credentials")
	}
	cfg, err := loadOptions()
	if err != nil {
		return err
	}
	admin, err := readInitialAdmin()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		return errors.New("create bootstrap database pool")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to bootstrap database: %w", err)
	}

	accountID, err := identitybootstrap.Prepare(ctx, pool, cfg.secret, cfg.expiresAt, admin)
	if err != nil {
		return err
	}
	if err := accessbootstrap.AssignInitialSuperAdmin(ctx, pool, accountID); err != nil {
		return fmt.Errorf("assign initial SuperAdmin permissions; account remains inactive and retry is safe: %w", err)
	}
	if err := auditbootstrap.RecordInitialSuperAdmin(ctx, pool, accountID, admin.StaffID); err != nil {
		return fmt.Errorf("record initial SuperAdmin audit event; account remains inactive and retry is safe: %w", err)
	}
	if err := identitybootstrap.Complete(ctx, pool, cfg.secret, accountID); err != nil {
		return fmt.Errorf("activate initial SuperAdmin; account remains inactive and retry is safe: %w", err)
	}

	fmt.Printf("Initial SuperAdmin created and activated. Account ID: %s\n", accountID)
	fmt.Println("The bootstrap secret has been consumed in the database; remove it from the deployment secret store now.")
	return nil
}

func loadOptions() (options, error) {
	cfg := options{
		databaseURL: strings.TrimSpace(os.Getenv("BOOTSTRAP_DATABASE_URL")),
		secret:      os.Getenv("BOOTSTRAP_SECRET"),
		appEnv:      strings.TrimSpace(os.Getenv("APP_ENV")),
	}
	if cfg.databaseURL == "" {
		return options{}, errors.New("set BOOTSTRAP_DATABASE_URL to the restricted administrative connection URL")
	}
	if cfg.secret == "" {
		return options{}, errors.New("set BOOTSTRAP_SECRET to a random one-time value")
	}
	expiresAtValue := strings.TrimSpace(os.Getenv("BOOTSTRAP_SECRET_EXPIRES_AT"))
	if expiresAtValue == "" {
		return options{}, errors.New("set BOOTSTRAP_SECRET_EXPIRES_AT in RFC3339 format")
	}
	expiresAt, err := time.Parse(time.RFC3339, expiresAtValue)
	if err != nil {
		return options{}, errors.New("BOOTSTRAP_SECRET_EXPIRES_AT must be an RFC3339 timestamp")
	}
	cfg.expiresAt = expiresAt
	if err := identitybootstrap.ValidateBootstrapSecret(cfg.secret, cfg.expiresAt); err != nil {
		return options{}, err
	}
	if cfg.appEnv != "production" && cfg.appEnv != "development" && cfg.appEnv != "test" {
		return options{}, errors.New("APP_ENV must be production, development, or test")
	}

	parsed, err := url.Parse(cfg.databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil || parsed.Path == "" {
		return options{}, errors.New("BOOTSTRAP_DATABASE_URL must be a PostgreSQL URL with host, user, and database")
	}
	if _, ok := parsed.User.Password(); !ok {
		return options{}, errors.New("BOOTSTRAP_DATABASE_URL must include a password")
	}
	modes := parsed.Query()["sslmode"]
	if len(modes) != 1 {
		return options{}, errors.New("BOOTSTRAP_DATABASE_URL must contain exactly one sslmode")
	}
	if cfg.appEnv == "production" && modes[0] != "verify-full" {
		allowUnverified := strings.EqualFold(strings.TrimSpace(os.Getenv("DATABASE_ALLOW_UNVERIFIED_TLS")), "true")
		if modes[0] != "require" || !allowUnverified {
			return options{}, errors.New("production bootstrap requires sslmode=verify-full, or sslmode=require with DATABASE_ALLOW_UNVERIFIED_TLS=true")
		}
	}
	if cfg.appEnv != "production" && modes[0] != "disable" && modes[0] != "require" &&
		modes[0] != "verify-ca" && modes[0] != "verify-full" {
		return options{}, errors.New("bootstrap database URL has an unsupported sslmode")
	}
	return cfg, nil
}

func readInitialAdmin() (identitybootstrap.InitialAdmin, error) {
	reader := bufio.NewReader(os.Stdin)
	staffID, err := readLine(reader, "Initial administrator staff ID: ")
	if err != nil {
		return identitybootstrap.InitialAdmin{}, err
	}
	email, err := readLine(reader, "Initial administrator email: ")
	if err != nil {
		return identitybootstrap.InitialAdmin{}, err
	}
	displayName, err := readLine(reader, "Initial administrator display name: ")
	if err != nil {
		return identitybootstrap.InitialAdmin{}, err
	}
	fmt.Fprint(os.Stderr, "Initial administrator password (hidden): ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return identitybootstrap.InitialAdmin{}, fmt.Errorf("read administrator password securely: %w", err)
	}
	fmt.Fprint(os.Stderr, "Confirm password (hidden): ")
	confirmation, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return identitybootstrap.InitialAdmin{}, fmt.Errorf("confirm administrator password securely: %w", err)
	}
	if string(password) != string(confirmation) {
		return identitybootstrap.InitialAdmin{}, errors.New("password confirmation does not match")
	}
	admin := identitybootstrap.InitialAdmin{
		StaffID:     strings.TrimSpace(staffID),
		Email:       strings.TrimSpace(email),
		DisplayName: strings.TrimSpace(displayName),
		Password:    string(password),
	}
	if _, err := mail.ParseAddress(admin.Email); err != nil {
		return identitybootstrap.InitialAdmin{}, errors.New("enter a valid administrator email address")
	}
	return admin, nil
}

func readLine(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read bootstrap input: %w", err)
	}
	return strings.TrimSpace(value), nil
}
