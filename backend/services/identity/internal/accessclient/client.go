package accessclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/observability"
)

const maximumResponseBytes = 32 << 10

type Client struct {
	baseURL string
	token   string
	http    *http.Client
	logger  *slog.Logger
}

func New(baseURL, token string, timeout time.Duration, loggers ...*slog.Logger) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	parsed, err := url.Parse(baseURL)
	isLoopback := false
	if parsed != nil {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		isLoopback = strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	}
	if err != nil || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopback)) ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || len(token) < 32 || strings.TrimSpace(token) != token {
		return nil, errors.New("Access service URL or credential is invalid")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		logger:  logger,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) GetAuthorization(ctx context.Context, accountID string) (application.Authorization, error) {
	if c == nil || c.http == nil || strings.TrimSpace(accountID) == "" {
		return application.Authorization{}, errors.New("Access authorization client is unavailable")
	}
	endpoint := c.baseURL + "/internal/v1/users/" + url.PathEscape(accountID) + "/authorization"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return application.Authorization{}, errors.New("create Access authorization request")
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	requestID := observability.RequestIDFromContext(ctx)
	if requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	started := time.Now()
	statusCode := 0
	outcome := "error"
	defer func() {
		c.logger.InfoContext(ctx, "Access authorization HTTP request",
			"request_id", requestID,
			"duration_ms", time.Since(started).Milliseconds(),
			"status", statusCode,
			"outcome", outcome,
		)
	}()
	response, err := c.http.Do(request)
	if err != nil {
		return application.Authorization{}, fmt.Errorf("request Access authorization: %w", err)
	}
	defer response.Body.Close()
	statusCode = response.StatusCode
	if response.StatusCode != http.StatusOK {
		return application.Authorization{}, fmt.Errorf("Access authorization returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, response.Body, maximumResponseBytes))
	decoder.DisallowUnknownFields()
	var authorization application.Authorization
	if err := decoder.Decode(&authorization); err != nil {
		return application.Authorization{}, fmt.Errorf("decode Access authorization response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return application.Authorization{}, errors.New("Access authorization response contains trailing data")
	}
	outcome = "success"
	return authorization, nil
}
