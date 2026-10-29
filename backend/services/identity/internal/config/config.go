package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr          = ":8080"
	defaultDatabaseMaxConns  = int32(10)
	defaultDBConnectTimeout  = 10 * time.Second
	defaultHealthTimeout     = 2 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
	maximumDatabaseMaxConns  = 1000
	maximumConfiguredTimeout = 2 * time.Minute
)

// Config contains validated runtime settings. Secret values are never included in
// error messages or logged by the service.
type Config struct {
	AppEnv             string
	HTTPAddr           string
	DatabaseURL        string
	DatabaseMaxConns   int32
	DBConnectTimeout   time.Duration
	HealthCheckTimeout time.Duration
	ShutdownTimeout    time.Duration
}

// Load reads and validates the process environment.
func Load() (Config, error) {
	appEnv := strings.ToLower(strings.TrimSpace(env("APP_ENV", "development")))
	if appEnv != "development" && appEnv != "test" && appEnv != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, or production")
	}

	httpAddr := strings.TrimSpace(env("HTTP_ADDR", defaultHTTPAddr))
	if err := validateHTTPAddr(httpAddr); err != nil {
		return Config{}, err
	}

	databaseURL, err := loadDatabaseURL(appEnv)
	if err != nil {
		return Config{}, err
	}

	maxConns, err := int32Env("DATABASE_MAX_CONNS", defaultDatabaseMaxConns)
	if err != nil || maxConns < 1 || maxConns > maximumDatabaseMaxConns {
		return Config{}, fmt.Errorf("DATABASE_MAX_CONNS must be between 1 and %d", maximumDatabaseMaxConns)
	}

	dbConnectTimeout, err := durationEnv("DB_CONNECT_TIMEOUT", defaultDBConnectTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("DB_CONNECT_TIMEOUT must be a duration between 1s and %s", maximumConfiguredTimeout)
	}
	healthTimeout, err := durationEnv("HEALTH_CHECK_TIMEOUT", defaultHealthTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("HEALTH_CHECK_TIMEOUT must be a duration between 1s and %s", maximumConfiguredTimeout)
	}
	shutdownTimeout, err := durationEnv("SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT must be a duration between 1s and %s", maximumConfiguredTimeout)
	}

	return Config{
		AppEnv:             appEnv,
		HTTPAddr:           httpAddr,
		DatabaseURL:        databaseURL,
		DatabaseMaxConns:   maxConns,
		DBConnectTimeout:   dbConnectTimeout,
		HealthCheckTimeout: healthTimeout,
		ShutdownTimeout:    shutdownTimeout,
	}, nil
}

func loadDatabaseURL(appEnv string) (string, error) {
	connectionString := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if connectionString == "" {
		host := strings.TrimSpace(env("POSTGRES_HOST", "localhost"))
		port := strings.TrimSpace(env("POSTGRES_PORT", "5432"))
		database := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
		username := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
		password := os.Getenv("POSTGRES_PASSWORD")
		if database == "" || username == "" || password == "" {
			return "", fmt.Errorf("set DATABASE_URL or POSTGRES_DB, POSTGRES_USER, and POSTGRES_PASSWORD")
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("POSTGRES_PORT must be between 1 and 65535")
		}

		sslDefault := "disable"
		if appEnv == "production" {
			sslDefault = "verify-full"
		}
		sslMode := strings.TrimSpace(env("POSTGRES_SSLMODE", sslDefault))
		u := &url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(username, password),
			Host:   net.JoinHostPort(host, port),
			Path:   "/" + database,
		}
		query := url.Values{}
		query.Set("sslmode", sslMode)
		u.RawQuery = query.Encode()
		connectionString = u.String()
	}

	parsed, err := url.Parse(connectionString)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.User == nil || parsed.Path == "" {
		return "", fmt.Errorf("DATABASE_URL must be a valid PostgreSQL URL with a host, username, and database")
	}
	if username := parsed.User.Username(); username == "" {
		return "", fmt.Errorf("DATABASE_URL must include a database username")
	}
	if password, ok := parsed.User.Password(); !ok || password == "" {
		return "", fmt.Errorf("DATABASE_URL must include a database password")
	}

	sslModes := parsed.Query()["sslmode"]
	if len(sslModes) != 1 {
		return "", fmt.Errorf("PostgreSQL sslmode must be configured exactly once")
	}
	sslMode := sslModes[0]
	if !validSSLMode(sslMode) {
		return "", fmt.Errorf("PostgreSQL sslmode must be explicitly configured")
	}
	if appEnv == "production" && sslMode != "verify-full" {
		return "", fmt.Errorf("production PostgreSQL connections require sslmode=verify-full")
	}
	return connectionString, nil
}

func validSSLMode(mode string) bool {
	switch mode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

func validateHTTPAddr(address string) error {
	if address == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("HTTP_ADDR must be a host:port address")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("HTTP_ADDR port must be between 1 and 65535")
	}
	return nil
}

func int32Env(key string, fallback int32) (int32, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int32(parsed), err
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < time.Second || parsed > maximumConfiguredTimeout {
		return 0, fmt.Errorf("duration outside allowed range")
	}
	return parsed, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}