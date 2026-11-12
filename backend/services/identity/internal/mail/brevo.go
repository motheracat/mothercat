package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const brevoSendEmailURL = "https://api.brevo.com/v3/smtp/email"

type BrevoConfig struct {
	APIKey        string
	From          string
	FromName      string
	PublicBaseURL string
	Timeout       time.Duration
}

type Brevo struct {
	apiKey        string
	from          *mail.Address
	publicBaseURL string
	timeout       time.Duration
	httpClient    *http.Client
	endpoint      string
}

func NewBrevo(config BrevoConfig) (*Brevo, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.From = strings.TrimSpace(config.From)
	config.FromName = strings.TrimSpace(config.FromName)
	config.PublicBaseURL = strings.TrimRight(strings.TrimSpace(config.PublicBaseURL), "/")
	if config.APIKey == "" || config.Timeout <= 0 {
		return nil, errors.New("Brevo requires an API key and a positive timeout")
	}
	from, err := mail.ParseAddress(config.From)
	if err != nil || from.Address == "" || strings.ContainsAny(config.From, "\r\n") {
		return nil, errors.New("Brevo sender address is invalid")
	}
	baseURL, err := url.Parse(config.PublicBaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Hostname() == "" ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("public application URL must use HTTPS")
	}
	from.Name = config.FromName
	return &Brevo{
		apiKey:        config.APIKey,
		from:          from,
		publicBaseURL: config.PublicBaseURL,
		timeout:       config.Timeout,
		httpClient:    &http.Client{Timeout: config.Timeout},
		endpoint:      brevoSendEmailURL,
	}, nil
}

func (b *Brevo) SendRecovery(ctx context.Context, recipient, purpose, token string) error {
	if b == nil || b.httpClient == nil {
		return errors.New("Brevo sender is unavailable")
	}
	path := "/reset-password"
	subject := "Reset your password"
	if purpose == "applicant_verification" {
		path = "/applicant/verify-email"
		subject = "Verify your email address"
	} else if purpose == "activation" {
		path = "/activate"
		subject = "Activate your account"
	} else if purpose != "password_reset" {
		return errors.New("unknown recovery message purpose")
	}
	link := b.publicBaseURL + path + "?token=" + url.QueryEscape(token)
	return b.sendEmail(ctx, recipient, subject, fmt.Sprintf(
		"Use this one-time link to continue. It expires shortly and can only be used once:\n\n%s\n\nIf you did not request this, you can ignore this message.\n",
		link,
	))
}

func (b *Brevo) SendApplicationStatus(
	ctx context.Context,
	recipient, applicantName, opportunityTitle, status string,
) error {
	subject, stage, err := applicationStatusMessage(status)
	if err != nil {
		return err
	}
	applicantName = safeEmailText(applicantName)
	if applicantName == "" {
		applicantName = "Applicant"
	}
	opportunityTitle = safeEmailText(opportunityTitle)
	if opportunityTitle == "" {
		opportunityTitle = "SIWES placement"
	}
	return b.sendEmail(ctx, recipient, subject, fmt.Sprintf(
		"Hello %s,\n\nYour application status for %s is now %s.\n\n%s\n\nRegards,\nMothercat",
		applicantName, opportunityTitle, stage, applicationStatusDetail(status),
	))
}

func applicationStatusMessage(status string) (string, string, error) {
	switch status {
	case "submitted":
		return "SIWES application received", "submitted", nil
	case "under_review":
		return "Your SIWES application is under review", "under review", nil
	case "shortlisted":
		return "You have been shortlisted for a SIWES placement", "shortlisted", nil
	case "accepted":
		return "Update on your SIWES application", "accepted", nil
	case "rejected":
		return "Update on your SIWES application", "not selected", nil
	default:
		return "", "", errors.New("unknown SIWES application status")
	}
}

func applicationStatusDetail(status string) string {
	if status == "accepted" {
		return "Congratulations. The team will contact you with any next steps."
	}
	if status == "rejected" {
		return "Thank you for your interest. We are unable to proceed with your application at this time."
	}
	return "We will contact you if there are further updates."
}

func safeEmailText(value string) string {
	return strings.TrimSpace(strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value))
}

func (b *Brevo) sendEmail(ctx context.Context, recipient, subject, textContent string) error {
	if b == nil || b.httpClient == nil || b.from == nil {
		return errors.New("Brevo sender is unavailable")
	}
	to, err := mail.ParseAddress(recipient)
	if err != nil || to.Address == "" || strings.ContainsAny(recipient, "\r\n") {
		return errors.New("email recipient address is invalid")
	}
	payload := struct {
		Sender struct {
			Name  string `json:"name,omitempty"`
			Email string `json:"email"`
		} `json:"sender"`
		To []struct {
			Email string `json:"email"`
		} `json:"to"`
		Subject     string `json:"subject"`
		TextContent string `json:"textContent"`
	}{}
	payload.Sender.Name = b.from.Name
	payload.Sender.Email = b.from.Address
	payload.To = append(payload.To, struct {
		Email string `json:"email"`
	}{Email: to.Address})
	payload.Subject = subject
	payload.TextContent = textContent
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Brevo email request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Brevo email request: %w", err)
	}
	request.Header.Set("api-key", b.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := b.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send email through Brevo: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Brevo email request failed with status %d", response.StatusCode)
	}
	return nil
}
