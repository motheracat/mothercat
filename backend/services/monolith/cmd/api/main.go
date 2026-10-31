package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	accessstore "github.com/mothercat-enterprise/mothercat/backend/services/access/monolith"
	auditstore "github.com/mothercat-enterprise/mothercat/backend/services/audit/monolith"
	identityapp "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	identitydomain "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/domain"
	identityhttp "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/http"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/mail"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/persistence"
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
	uri := strings.TrimSpace(os.Getenv("MONGODB_URI"))
	databaseName := strings.TrimSpace(os.Getenv("MONGODB_DATABASE"))
	if err := validateMongoSettings(uri, databaseName, appEnv); err != nil {
		return err
	}

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnect()
	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(uri))
	if err != nil {
		return errors.New("connect to MongoDB")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			logger.Error("disconnect MongoDB client", "error", err)
		}
	}()
	if err := client.Ping(connectCtx, nil); err != nil {
		return errors.New("ping MongoDB")
	}

	db := client.Database(databaseName)
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
	organizationHandler := organizationmonolith.NewHandler(
		db, authenticator, localAuditRecorder{recorder: audit},
		localAccountProvisioner{service: authService}, access, logger,
	)
	handler := newRouter(db, identityHandler, organizationHandler)
	server := &http.Server{
		Addr: address, Handler: handler, ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	logger.Info("Mothercat API listening", "address", address)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("graceful HTTP shutdown: %w", err)
		}
		return nil
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	}
}

func validateMongoSettings(uri, database, appEnv string) error {
	if uri == "" || database == "" || strings.ContainsAny(database, "/\\.") {
		return errors.New("MONGODB_URI and a valid MONGODB_DATABASE are required")
	}
	clientOptions := options.Client().ApplyURI(uri)
	if err := clientOptions.Validate(); err != nil {
		return errors.New("MONGODB_URI is invalid")
	}
	if appEnv == "production" || appEnv == "staging" {
		if clientOptions.TLSConfig == nil || clientOptions.TLSConfig.InsecureSkipVerify {
			return errors.New("MongoDB TLS with certificate verification is required in production or staging")
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

func newRouter(db *mongo.Database, identity, organization http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Client().Ping(ctx, nil); err != nil {
			http.Error(w, "{\"error\":{\"code\":\"service_not_ready\"}}", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{\"status\":\"ready\"}\n"))
	})
	mux.Handle("/api/v1/staff", organization)
	mux.Handle("/api/v1/staff/", organization)
	mux.Handle("/api/v1/", identity)
	mux.Handle("/internal/v1/", identity)
	return mux
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

type localAuditRecorder struct {
	recorder *auditstore.Recorder
}

func (r localAuditRecorder) Record(ctx context.Context, event organizationmonolith.AuditEvent) error {
	return r.recorder.Record(ctx, auditstore.Event{
		EventKey: event.EventKey, ActorID: event.ActorID, Action: event.Action,
		ResourceID: event.ResourceID, Details: event.Details,
	})
}
