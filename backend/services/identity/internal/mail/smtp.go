package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host          string
	Port          int
	Username      string
	Password      string
	From          string
	PublicBaseURL string
	Timeout       time.Duration
}

type SMTP struct {
	config SMTPConfig
	from   *mail.Address
}

func NewSMTP(config SMTPConfig) (*SMTP, error) {
	config.Host = strings.TrimSpace(config.Host)
	config.Username = strings.TrimSpace(config.Username)
	config.From = strings.TrimSpace(config.From)
	config.PublicBaseURL = strings.TrimRight(strings.TrimSpace(config.PublicBaseURL), "/")
	if config.Host == "" || config.Port != 587 || config.Username == "" ||
		config.Password == "" || config.Timeout <= 0 {
		return nil, errors.New("SMTP requires host, port 587, credentials, and timeout")
	}
	from, err := mail.ParseAddress(config.From)
	if err != nil || from.Address == "" || strings.ContainsAny(config.From, "\r\n") {
		return nil, errors.New("SMTP sender address is invalid")
	}
	baseURL, err := url.Parse(config.PublicBaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Hostname() == "" ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("public application URL must use HTTPS")
	}
	return &SMTP{config: config, from: from}, nil
}

func (s *SMTP) SendRecovery(ctx context.Context, recipient, purpose, token string) error {
	if s == nil {
		return errors.New("SMTP sender is unavailable")
	}
	to, err := mail.ParseAddress(recipient)
	if err != nil || to.Address == "" || strings.ContainsAny(recipient, "\r\n") {
		return errors.New("recovery recipient address is invalid")
	}
	path := "/reset-password"
	subject := "Reset your password"
	if purpose == "activation" {
		path = "/activate"
		subject = "Activate your account"
	} else if purpose != "password_reset" {
		return errors.New("unknown recovery message purpose")
	}
	link := s.config.PublicBaseURL + path + "?token=" + url.QueryEscape(token)
	body := fmt.Sprintf("Use this one-time link to continue. It expires shortly and can only be used once:\r\n\r\n%s\r\n\r\nIf you did not request this, you can ignore this message.\r\n", link)
	message := "From: " + s.from.String() + "\r\n" +
		"To: " + to.String() + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" + body

	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	dialer := net.Dialer{Timeout: s.config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(s.config.Timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}
	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		return fmt.Errorf("start SMTP client: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not advertise STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("establish SMTP TLS: %w", err)
	}
	if err := client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)); err != nil {
		return fmt.Errorf("authenticate to SMTP server: %w", err)
	}
	if err := client.Mail(s.from.Address); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("begin SMTP message: %w", err)
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("send SMTP message: %w", err)
	}
	return client.Quit()
}
