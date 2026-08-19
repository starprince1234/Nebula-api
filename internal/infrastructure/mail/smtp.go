package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Sender interface {
	Send(ctx context.Context, to, subject, text string) error
}

type SMTP struct {
	host     string
	port     int
	user     string
	password string
	from     string
	fromName string
	tlsMode  string
	timeout  time.Duration
}

func NewSMTP(host string, port int, user, password, from, fromName, tlsMode string, timeout time.Duration) (*SMTP, error) {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("SMTP_HOST and SMTP_FROM are required")
	}
	fromAddress, err := netmail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP_FROM: %w", err)
	}
	if hasHeaderInjection(fromName) {
		return nil, fmt.Errorf("invalid SMTP_FROM_NAME")
	}
	return &SMTP{
		host: host, port: port, user: user, password: password,
		from: fromAddress.Address, fromName: fromName, tlsMode: tlsMode, timeout: timeout,
	}, nil
}

func (s *SMTP) Send(ctx context.Context, to, subject, text string) error {
	if _, err := netmail.ParseAddress(to); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	if hasHeaderInjection(subject) || hasHeaderInjection(to) {
		return fmt.Errorf("invalid mail header")
	}
	address := net.JoinHostPort(s.host, fmt.Sprint(s.port))
	dialer := &net.Dialer{Timeout: s.timeout}
	var (
		client *smtp.Client
		conn   net.Conn
		err    error
	)
	if s.tlsMode == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: s.host,
		})
		if err != nil {
			return fmt.Errorf("dial SMTP TLS: %w", err)
		}
		client, err = smtp.NewClient(conn, s.host)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return fmt.Errorf("dial SMTP: %w", err)
		}
		client, err = smtp.NewClient(conn, s.host)
	}
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(s.timeout))
	}
	if s.tlsMode == "starttls" {
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if s.user != "" {
		if err := client.Auth(smtp.PlainAuth("", s.user, s.password, s.host)); err != nil {
			return fmt.Errorf("authenticate SMTP: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open SMTP body: %w", err)
	}
	fromHeader := (&netmail.Address{Name: s.fromName, Address: s.from}).String()
	message := "From: " + fromHeader + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" + text
	if _, err := io.WriteString(writer, message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close SMTP body: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("quit SMTP: %w", err)
	}
	return nil
}

func hasHeaderInjection(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}
