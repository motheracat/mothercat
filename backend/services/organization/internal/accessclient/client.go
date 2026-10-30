package accessclient

import (
	"context"
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
	baseURL string
	token   string
	http    *http.Client
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
		return nil, errors.New("Access service URL or credential is invalid")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (client *Client) AssignDefaultStaffRole(ctx context.Context, accountID string) error {
	if !validUUID(accountID) {
		return errors.New("account identifier is invalid")
	}
	endpoint := client.baseURL + "/internal/v1/users/" + url.PathEscape(accountID) + "/roles/staff"
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return errors.New("create Access role assignment request")
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Accept", "application/json")
	if requestID := strings.TrimSpace(application.RequestIDFromContext(ctx)); requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("send Access role assignment request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Access role assignment returned HTTP %d", response.StatusCode)
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
		} else if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') &&
			!(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}
