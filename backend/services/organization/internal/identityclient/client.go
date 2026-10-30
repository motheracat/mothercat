package identityclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/organization/internal/application"
)

const maximumResponseBytes = 32 << 10

type Client struct {
	sessionURL string
	http       *http.Client
}

func New(baseURL string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	isLoopback := false
	if parsed != nil {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		isLoopback = strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	}
	if err != nil || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopback)) ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Identity service URL is invalid")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		sessionURL: baseURL + "/api/v1/auth/session",
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (client *Client) Authenticate(ctx context.Context, cookie string) (application.Principal, error) {
	if client == nil || client.http == nil || strings.TrimSpace(cookie) == "" ||
		strings.ContainsAny(cookie, "\r\n;") {
		return application.Principal{}, application.ErrUnauthenticated
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.sessionURL, nil)
	if err != nil {
		return application.Principal{}, errors.New("create Identity session request")
	}
	request.Header.Set("Cookie", "mothercat_session="+cookie)
	request.Header.Set("Accept", "application/json")
	if requestID := strings.TrimSpace(application.RequestIDFromContext(ctx)); requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return application.Principal{}, fmt.Errorf("%w: %v", application.ErrAuthUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return application.Principal{}, application.ErrUnauthenticated
	}
	if response.StatusCode != http.StatusOK {
		return application.Principal{}, fmt.Errorf("%w: Identity returned HTTP %d", application.ErrAuthUnavailable, response.StatusCode)
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, response.Body, maximumResponseBytes))
	decoder.DisallowUnknownFields()
	var body struct {
		User struct {
			ID          string   `json:"id"`
			StaffID     string   `json:"staff_id"`
			DisplayName string   `json:"display_name"`
			Email       string   `json:"email"`
			Role        string   `json:"role"`
			Roles       []string `json:"roles"`
			Permissions []string `json:"permissions"`
		} `json:"user"`
	}
	if err := decoder.Decode(&body); err != nil {
		return application.Principal{}, fmt.Errorf("%w: decode Identity session response: %v", application.ErrAuthUnavailable, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return application.Principal{}, fmt.Errorf("%w: Identity session response contains trailing data", application.ErrAuthUnavailable)
	}
	if !validUUID(body.User.ID) {
		return application.Principal{}, fmt.Errorf("%w: Identity returned an invalid account identifier", application.ErrAuthUnavailable)
	}
	return application.Principal{ID: body.User.ID, StaffID: body.User.StaffID, Permissions: body.User.Permissions}, nil
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
