package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	accesshttp "github.com/mothercat-enterprise/mothercat/backend/services/access/http"
	accessstore "github.com/mothercat-enterprise/mothercat/backend/services/access/monolith"
	audithttp "github.com/mothercat-enterprise/mothercat/backend/services/audit/http"
	auditstore "github.com/mothercat-enterprise/mothercat/backend/services/audit/monolith"
	identityapp "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
	identityhttp "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/http"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/mail"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/notifications"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/opportunities"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/persistence"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/tasks"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/applicantidentity"
	organizationmonolith "github.com/mothercat-enterprise/mothercat/backend/services/organization/monolith"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "mothercat-api")
	if err := run(logger); err != nil {
		logger.Error("API server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	appEnv := strings.ToLower(strings.TrimSpace(env("APP_ENV", "development")))
	if appEnv != "development" && appEnv != "test" && appEnv != "staging" && appEnv != "production" {
		return errors.New("APP_ENV must be development, test, staging, or production")
	}
	address, err := listenAddress()
	if err != nil {
		return err
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if err := validateDatabaseSettings(databaseURL, appEnv); err != nil {
		return err
	}
	var profileUploader organizationmonolith.ProfileImageUploader
	cloudName, apiKey, apiSecret := strings.TrimSpace(os.Getenv("CLOUDINARY_CLOUD_NAME")),
		strings.TrimSpace(os.Getenv("CLOUDINARY_API_KEY")), strings.TrimSpace(os.Getenv("CLOUDINARY_API_SECRET"))
	if cloudName != "" || apiKey != "" || apiSecret != "" {
		configuredUploader, err := organizationmonolith.NewCloudinaryUploader(cloudName, apiKey, apiSecret)
		if err != nil {
			return fmt.Errorf("configure Cloudinary: %w", err)
		}
		profileUploader = configuredUploader
	}

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnect()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("configure PostgreSQL connection")
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close PostgreSQL connection pool", "error", err)
		}
	}()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.PingContext(connectCtx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}

	identityRepository := persistence.NewIdentityRepository(db)
	access := accessstore.NewStore(db)
	audit := auditstore.NewRecorder(db)
	authorizationProvider := localAuthorizationProvider{store: access}
	var mailer identityapp.Mailer
	brevoConfigured := os.Getenv("BREVO_API_KEY") != "" ||
		os.Getenv("BREVO_FROM_EMAIL") != "" ||
		os.Getenv("BREVO_FROM_NAME") != "" ||
		os.Getenv("PUBLIC_BASE_URL") != ""
	if brevoConfigured {
		sender, err := mail.NewBrevo(mail.BrevoConfig{
			APIKey: os.Getenv("BREVO_API_KEY"), From: os.Getenv("BREVO_FROM_EMAIL"),
			FromName: os.Getenv("BREVO_FROM_NAME"), PublicBaseURL: os.Getenv("PUBLIC_BASE_URL"),
			Timeout: 10 * time.Second,
		})
		if err != nil {
			return fmt.Errorf("configure email delivery: %w", err)
		}
		mailer = sender
	}
	authService := identityapp.NewService(identityRepository, authorizationProvider, mailer, logger)
	identityHandler := identityhttp.NewAuthHandler(
		authService, logger, identityhttp.AuthHandlerOptions{
			OnboardingServiceToken: os.Getenv("IDENTITY_ONBOARDING_TOKEN"),
			BootstrapToken:         os.Getenv("IDENTITY_BOOTSTRAP_TOKEN"),
		},
	)
	authenticator := localAuthenticator{service: authService}
	var profileUploaders []organizationmonolith.ProfileImageUploader
	if profileUploader != nil {
		profileUploaders = append(profileUploaders, profileUploader)
	}
	organizationHandler := organizationmonolith.NewHandler(
		db, authenticator, localAuditRecorder{recorder: audit},
		localAccountProvisioner{service: authService}, access, logger, profileUploaders...,
	)
	accessHandler := accesshttp.NewHandler(access, accessAuthenticator{service: authService}, logger)
	auditHandler := audithttp.NewHandler(
		auditstore.NewReader(db), auditAuthenticator{service: authService}, logger,
	)
	taskHandler := tasks.NewHandler(db, taskAuthenticator{service: authService}, logger)
	taskReminderJob := tasks.NewReminderJob(db, logger)
	notificationHandler := notifications.NewHandler(db, notificationAuthenticator{service: authService}, logger)
	notificationDispatcher := notifications.NewDispatcher(db, logger)
	var documentScanner opportunities.MalwareScanner
	if address := strings.TrimSpace(os.Getenv("SIWES_CLAMAV_ADDR")); address != "" {
		documentScanner, err = opportunities.NewClamAVScanner(address)
		if err != nil {
			return fmt.Errorf("configure SIWES malware scanner: %w", err)
		}
	}
	opportunitiesHandler := opportunities.NewHandler(
		db, opportunityAuthenticator{service: authService}, documentScanner, logger,
	)
	applicantIdentityHandler := applicantidentity.NewHandler(db, mailer, logger)
	applicationRetentionJob := opportunities.NewRetentionJob(db, logger)
	handler := newRouter(db, identityHandler, organizationHandler, accessHandler, auditHandler,
		taskHandler, notificationHandler, opportunitiesHandler, applicantIdentityHandler)
	server := &http.Server{
		Addr: address, Handler: handler, ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dispatcherDone := make(chan struct{})
	go func() {
		defer close(dispatcherDone)
		notificationDispatcher.Run(ctx)
	}()
	reminderDone := make(chan struct{})
	go func() {
		defer close(reminderDone)
		taskReminderJob.Run(ctx)
	}()
	retentionDone := make(chan struct{})
	go func() {
		defer close(retentionDone)
		applicationRetentionJob.Run(ctx)
	}()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	logger.Info("Mothercat API listening", "address", address)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			<-dispatcherDone
			<-reminderDone
			<-retentionDone
			return fmt.Errorf("graceful HTTP shutdown: %w", err)
		}
		<-dispatcherDone
		<-reminderDone
		<-retentionDone
		return nil
	case err := <-serverErrors:
		stop()
		<-dispatcherDone
		<-reminderDone
		<-retentionDone
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	}
}

func validateDatabaseSettings(databaseURL, appEnv string) error {
	if strings.TrimSpace(databaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil {
		return errors.New("DATABASE_URL must be a valid PostgreSQL connection URL")
	}
	if appEnv == "production" || appEnv == "staging" {
		sslMode := parsed.Query().Get("sslmode")
		if sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full" {
			return errors.New("DATABASE_URL must require TLS in production or staging")
		}
		if appEnv == "production" {
			token := os.Getenv("IDENTITY_ONBOARDING_TOKEN")
			if len(token) < 32 || strings.TrimSpace(token) != token {
				return errors.New("IDENTITY_ONBOARDING_TOKEN must contain at least 32 characters")
			}
		}
	}
	bootstrapToken := os.Getenv("IDENTITY_BOOTSTRAP_TOKEN")
	if bootstrapToken != "" && (len(bootstrapToken) < 32 || strings.TrimSpace(bootstrapToken) != bootstrapToken) {
		return errors.New("IDENTITY_BOOTSTRAP_TOKEN must contain at least 32 characters without surrounding whitespace")
	}
	if bootstrapToken != "" && bootstrapToken == os.Getenv("IDENTITY_ONBOARDING_TOKEN") {
		return errors.New("IDENTITY_BOOTSTRAP_TOKEN must be different from IDENTITY_ONBOARDING_TOKEN")
	}
	return nil
}

func listenAddress() (string, error) {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		number, err := integerEnv("PORT", 0)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("PORT must be between 1 and 65535")
		}
		return fmt.Sprintf(":%d", number), nil
	}
	address := env("HTTP_ADDR", ":8080")
	if _, _, err := net.SplitHostPort(address); err != nil {
		return "", errors.New("HTTP_ADDR must be a valid host:port address")
	}
	return address, nil
}

func integerEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	var number int
	if _, err := fmt.Sscanf(value, "%d", &number); err != nil || fmt.Sprintf("%d", number) != value {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return number, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func newRouter(db *sql.DB, identity, organization, access, audit, tasks, notifications, opportunities, applicants http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "{\"error\":{\"code\":\"service_not_ready\"}}", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{\"status\":\"ready\"}\n"))
	})
	mux.Handle("/api/v1/staff", organization)
	mux.Handle("/api/v1/staff/", organization)
	mux.Handle("/api/v1/departments", organization)
	mux.Handle("/api/v1/departments/", organization)
	if access != nil {
		mux.Handle("/api/v1/roles", access)
		mux.Handle("/api/v1/roles/", access)
		mux.Handle("/api/v1/permissions", access)
		mux.Handle("/api/v1/users/", access)
	}
	if audit != nil {
		mux.Handle("/api/v1/audit-events", audit)
		mux.Handle("/api/v1/audit-events/", audit)
	}
	if tasks != nil {
		mux.Handle("/api/v1/tasks", tasks)
		mux.Handle("/api/v1/tasks/", tasks)
		mux.Handle("/api/v1/me/tasks", tasks)
	}
	if notifications != nil {
		mux.Handle("/api/v1/notifications", notifications)
		mux.Handle("/api/v1/notifications/", notifications)
	}
	if opportunities != nil {
		mux.Handle("/api/v1/opportunities", opportunities)
		mux.Handle("/api/v1/opportunities/", opportunities)
		mux.Handle("/api/v1/siwes-applications", opportunities)
		mux.Handle("/api/v1/siwes-applications/", opportunities)
	}
	if applicants != nil {
		mux.Handle("/api/v1/auth/applicant/", applicants)
	}
	mux.Handle("/api/v1/", identity)
	mux.Handle("/internal/v1/", identity)
	return mux
}

type accessAuthenticator struct {
	service *identityapp.Service
}

func (a accessAuthenticator) Authenticate(ctx context.Context, token string) (accesshttp.Principal, error) {
	account, err := a.service.ResolveSession(ctx, token)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			return accesshttp.Principal{}, accesshttp.ErrUnauthenticated
		}
		return accesshttp.Principal{}, fmt.Errorf("authentication unavailable: %w", err)
	}
	return accesshttp.Principal{ID: account.ID, Permissions: account.Permissions}, nil
}

type auditAuthenticator struct {
	service *identityapp.Service
}

func (a auditAuthenticator) Authenticate(ctx context.Context, token string) (audithttp.Principal, error) {
	account, err := a.service.ResolveSession(ctx, token)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			return audithttp.Principal{}, audithttp.ErrUnauthenticated
		}
		return audithttp.Principal{}, fmt.Errorf("authentication unavailable: %w", err)
	}
	return audithttp.Principal{Permissions: account.Permissions}, nil
}

type taskAuthenticator struct {
	service *identityapp.Service
}

func (a taskAuthenticator) Authenticate(ctx context.Context, token string) (tasks.Principal, error) {
	account, err := a.service.ResolveSession(ctx, token)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			return tasks.Principal{}, tasks.ErrUnauthenticated
		}
		return tasks.Principal{}, fmt.Errorf("authentication unavailable: %w", err)
	}
	return tasks.Principal{ID: account.ID, Permissions: account.Permissions}, nil
}

type notificationAuthenticator struct {
	service *identityapp.Service
}

func (a notificationAuthenticator) Authenticate(ctx context.Context, token string) (notifications.Principal, error) {
	account, err := a.service.ResolveSession(ctx, token)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			return notifications.Principal{}, notifications.ErrUnauthenticated
		}
		return notifications.Principal{}, fmt.Errorf("authentication unavailable: %w", err)
	}
	return notifications.Principal{ID: account.ID}, nil
}

type opportunityAuthenticator struct {
	service *identityapp.Service
}

func (a opportunityAuthenticator) Authenticate(ctx context.Context, token string) (opportunities.Principal, error) {
	account, err := a.service.ResolveSession(ctx, token)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			return opportunities.Principal{}, opportunities.ErrUnauthenticated
		}
		return opportunities.Principal{}, fmt.Errorf("authentication unavailable: %w", err)
	}
	return opportunities.Principal{ID: account.ID, Permissions: account.Permissions}, nil
}

type localAuthorizationProvider struct {
	store *accessstore.Store
}

func (p localAuthorizationProvider) GetAuthorization(ctx context.Context, id string) (identityapp.Authorization, error) {
	result, err := p.store.GetUserAuthorization(ctx, id)
	if err != nil {
		return identityapp.Authorization{}, err
	}
	return identityapp.Authorization{Role: result.Role, Roles: result.Roles, Permissions: result.Permissions}, nil
}

type localAuthenticator struct {
	service *identityapp.Service
}

func (a localAuthenticator) Authenticate(ctx context.Context, token string) (organizationmonolith.Principal, error) {
	account, err := a.service.ResolveSession(ctx, token)
	if err != nil {
		if errors.Is(err, identitydomain.ErrInvalidCredentials) {
			return organizationmonolith.Principal{}, organizationmonolith.ErrUnauthenticated
		}
		return organizationmonolith.Principal{}, fmt.Errorf("authentication unavailable: %w", err)
	}
	return organizationmonolith.Principal{ID: account.ID, StaffID: account.StaffID, Permissions: account.Permissions}, nil
}

type localAccountProvisioner struct {
	service *identityapp.Service
}

func (p localAccountProvisioner) ProvisionAccount(ctx context.Context, staffID, email, displayName string) (string, error) {
	return p.service.ProvisionInactiveAccount(ctx, staffID, email, displayName)
}

func (p localAccountProvisioner) SendActivation(ctx context.Context, accountID string) error {
	return p.service.CreateActivationToken(ctx, accountID)
}

func (p localAccountProvisioner) SuspendAccount(ctx context.Context, staffID, actorID string) error {
	return p.service.SuspendAccount(ctx, staffID, actorID)
}

type localAuditRecorder struct {
	recorder *auditstore.Recorder
}

func (r localAuditRecorder) Record(ctx context.Context, event organizationmonolith.AuditEvent) error {
	return r.recorder.Record(ctx, auditstore.Event{
		EventKey: event.EventKey, ActorID: event.ActorID, Action: event.Action,
		ResourceID: event.ResourceID, Details: event.Details,
	})
}
