package auditclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
)

type Client struct {
	endpoint string
	token    string
	http     *http.Client
}

func New(baseURL, token string, timeout time.Duration) (*Client, error) {
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
		return nil, errors.New("Audit service URL or credential is invalid")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		endpoint: baseURL + "/internal/v1/audit-events",
		token:    token,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (client *Client) Record(ctx context.Context, event application.AuditEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode audit event: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("create Audit event request")
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if requestID := application.RequestIDFromContext(ctx); requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("send audit event: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return fmt.Errorf("Audit service returned HTTP %d", response.StatusCode)
	}
	return nil
}
