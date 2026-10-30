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
	AppEnv                 string
	HTTPAddr               string
	DatabaseURL            string
	AccessServiceURL       string
	AccessServiceToken     string
	AccessServiceTimeout   time.Duration
	OnboardingServiceToken string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFrom               string
	PublicBaseURL          string
	SMTPTimeout            time.Duration
	DatabaseMaxConns       int32
	DBConnectTimeout       time.Duration
	HealthCheckTimeout     time.Duration
	ShutdownTimeout        time.Duration
}

// Load reads and validates the process environment.
func Load() (Config, error) {
	appEnv := strings.ToLower(strings.TrimSpace(env("APP_ENV", "development")))
	if appEnv != "development" && appEnv != "test" && appEnv != "staging" && appEnv != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, staging, or production")
	}

	httpAddr, err := loadHTTPAddr()
	if err != nil {
		return Config{}, err
	}

	databaseURL, err := loadDatabaseURL(appEnv)
	if err != nil {
		return Config{}, err
	}
	allowUnverifiedTLS, err := boolEnv("DATABASE_ALLOW_UNVERIFIED_TLS", false)
	if err != nil {
		return Config{}, fmt.Errorf("DATABASE_ALLOW_UNVERIFIED_TLS must be true or false")
	}
	if appEnv == "production" && allowUnverifiedTLS {
		parsed, err := url.Parse(databaseURL)
		if err != nil || parsed.Query().Get("sslmode") != "require" {
			return Config{}, fmt.Errorf("DATABASE_ALLOW_UNVERIFIED_TLS is only valid with sslmode=require")
		}
	}
	accessServiceURL, accessServiceToken, accessServiceTimeout, err := loadAccessService(appEnv)
	if err != nil {
		return Config{}, err
	}
	onboardingServiceToken, err := loadOnboardingServiceToken(appEnv)
	if err != nil {
		return Config{}, err
	}
	smtpConfig, err := loadSMTPConfig()
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
		AppEnv:                 appEnv,
		HTTPAddr:               httpAddr,
		DatabaseURL:            databaseURL,
		AccessServiceURL:       accessServiceURL,
		AccessServiceToken:     accessServiceToken,
		AccessServiceTimeout:   accessServiceTimeout,
		OnboardingServiceToken: onboardingServiceToken,
		SMTPHost:               smtpConfig.host,
		SMTPPort:               smtpConfig.port,
		SMTPUsername:           smtpConfig.username,
		SMTPPassword:           smtpConfig.password,
		SMTPFrom:               smtpConfig.from,
		PublicBaseURL:          smtpConfig.publicBaseURL,
		SMTPTimeout:            smtpConfig.timeout,
		DatabaseMaxConns:       maxConns,
		DBConnectTimeout:       dbConnectTimeout,
		HealthCheckTimeout:     healthTimeout,
		ShutdownTimeout:        shutdownTimeout,
	}, nil
}

func loadAccessService(appEnv string) (string, string, time.Duration, error) {
	serviceURL := strings.TrimSpace(os.Getenv("ACCESS_SERVICE_URL"))
	token := strings.TrimSpace(os.Getenv("ACCESS_SERVICE_TOKEN"))
	if serviceURL == "" && token == "" && appEnv != "production" && appEnv != "staging" {
		return "", "", 3 * time.Second, nil
	}
	parsed, err := url.Parse(serviceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		len(token) < 32 || strings.TrimSpace(token) != token {
		return "", "", 0, fmt.Errorf("ACCESS_SERVICE_URL (HTTPS) and ACCESS_SERVICE_TOKEN (minimum 32 characters) are required together")
	}
	timeout, err := durationEnv("ACCESS_SERVICE_TIMEOUT", 3*time.Second)
	if err != nil {
		return "", "", 0, fmt.Errorf("ACCESS_SERVICE_TIMEOUT must be a duration between 1s and %s", maximumConfiguredTimeout)
	}
	return strings.TrimRight(serviceURL, "/"), token, timeout, nil
}

func loadOnboardingServiceToken(appEnv string) (string, error) {
	token := os.Getenv("IDENTITY_ONBOARDING_TOKEN")
	if token == "" && appEnv != "production" && appEnv != "staging" {
		return "", nil
	}
	if len(token) < 32 || strings.TrimSpace(token) != token {
		return "", fmt.Errorf("IDENTITY_ONBOARDING_TOKEN must contain at least 32 characters")
	}
	return token, nil
}

type smtpConfig struct {
	host          string
	port          int
	username      string
	password      string
	from          string
	publicBaseURL string
	timeout       time.Duration
}

func loadSMTPConfig() (smtpConfig, error) {
	config := smtpConfig{
		host:          strings.TrimSpace(os.Getenv("SMTP_HOST")),
		username:      strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		password:      os.Getenv("SMTP_PASSWORD"),
		from:          strings.TrimSpace(os.Getenv("SMTP_FROM")),
		publicBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/"),
		timeout:       10 * time.Second,
	}
	portValue := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if config.host == "" && config.username == "" && config.password == "" &&
		config.from == "" && config.publicBaseURL == "" && portValue == "" {
		return config, nil
	}
	if config.host == "" || config.username == "" || config.password == "" ||
		config.from == "" || config.publicBaseURL == "" || portValue == "" {
		return smtpConfig{}, fmt.Errorf("SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM, and PUBLIC_BASE_URL must be configured together")
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port != 587 {
		return smtpConfig{}, fmt.Errorf("SMTP_PORT must be 587 to require STARTTLS")
	}
	config.port = port
	config.timeout, err = durationEnv("SMTP_TIMEOUT", 10*time.Second)
	if err != nil {
		return smtpConfig{}, fmt.Errorf("SMTP_TIMEOUT must be a duration between 1s and %s", maximumConfiguredTimeout)
	}
	return config, nil
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
		} else if appEnv == "staging" {
			sslDefault = "require"
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

	query := parsed.Query()
	sslModes := query["sslmode"]
	if len(sslModes) == 0 {
		sslMode := strings.TrimSpace(os.Getenv("DATABASE_SSLMODE"))
		if sslMode == "" {
			return "", fmt.Errorf("PostgreSQL sslmode must be explicit or DATABASE_SSLMODE must be configured")
		}
		query.Set("sslmode", sslMode)
		parsed.RawQuery = query.Encode()
		connectionString = parsed.String()
		sslModes = query["sslmode"]
	}
	if len(sslModes) != 1 {
		return "", fmt.Errorf("PostgreSQL sslmode must be configured exactly once")
	}
	sslMode := sslModes[0]
	if !validSSLMode(sslMode) {
		return "", fmt.Errorf("PostgreSQL sslmode must be explicitly configured")
	}
	if appEnv == "production" && sslMode != "verify-full" {
		allowUnverifiedTLS, err := boolEnv("DATABASE_ALLOW_UNVERIFIED_TLS", false)
		if err != nil || sslMode != "require" || !allowUnverifiedTLS {
			return "", fmt.Errorf("production PostgreSQL connections require sslmode=verify-full, or sslmode=require with DATABASE_ALLOW_UNVERIFIED_TLS=true")
		}
	}
	if appEnv == "staging" && sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full" {
		return "", fmt.Errorf("staging PostgreSQL connections require TLS")
	}
	return connectionString, nil
}

func loadHTTPAddr() (string, error) {
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		address := ":" + port
		if err := validateHTTPAddr(address); err != nil {
			return "", fmt.Errorf("PORT must be between 1 and 65535")
		}
		return address, nil
	}

	address := strings.TrimSpace(env("HTTP_ADDR", defaultHTTPAddr))
	if err := validateHTTPAddr(address); err != nil {
		return "", err
	}
	return address, nil
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

func boolEnv(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	return parsed, err
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
