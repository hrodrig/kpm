package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// SMTP sends plain-text email (gghstats-school).
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
	UseTLS   bool // true → implicit TLS (465); false → STARTTLS (typical 587)

	// DialAndSend overrides the network path (tests).
	DialAndSend func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

func (s *SMTP) Type() string { return "smtp" }

func (s *SMTP) Send(ctx context.Context, m Message) error {
	_ = ctx
	if len(s.To) == 0 {
		return fmt.Errorf("smtp: no recipients")
	}
	subject := m.Title
	body := m.Text
	msg := []byte("From: " + s.From + "\r\n" +
		"To: " + strings.Join(s.To, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body + "\r\n")

	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	send := s.DialAndSend
	if send == nil {
		if s.UseTLS {
			send = s.sendImplicitTLS
		} else {
			send = smtp.SendMail
		}
	}
	if err := send(addr, auth, s.From, s.To, msg); err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	return nil
}

func (s *SMTP) sendImplicitTLS(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if a != nil {
		if err := c.Auth(a); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
