package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
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

	// RequireSTARTTLS refuses to send unless the relay advertises STARTTLS.
	// Without it the UseTLS=false path is opportunistic: smtp.SendMail silently
	// falls back to cleartext when the relay has no STARTTLS, so an operator who
	// needs transport security has no way to demand it.
	RequireSTARTTLS bool

	// DialAndSend overrides the network path (tests).
	DialAndSend func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

func (s *SMTP) Type() string { return "smtp" }

func (s *SMTP) Send(ctx context.Context, m Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(s.To) == 0 {
		return fmt.Errorf("smtp: no recipients")
	}
	// Bound every send so a hung MTA cannot stall the sample tick forever,
	// even when the daemon context has no deadline.
	ctx, cancel := context.WithTimeout(ctx, DefaultHTTPTimeout)
	defer cancel()

	// Header values must not carry CR/LF. A title containing "\r\n" closes the
	// header block and lets the message inject its own headers (a second To:,
	// Bcc:, ...). The value is rejected rather than escaped: it is not a
	// template, and silently rewriting it would hide the problem.
	for _, hv := range append([]string{s.From, m.Title}, s.To...) {
		if err := headerSafe(hv); err != nil {
			return err
		}
	}
	// Encode the subject as an RFC 2047 encoded-word. A non-ASCII title is
	// otherwise emitted raw and relays either reject the message or mangle the
	// subject. ASCII titles pass through unchanged.
	subject := mime.QEncoding.Encode("utf-8", m.Title)
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
	inner := s.DialAndSend
	if inner == nil {
		inner = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			if s.UseTLS {
				return s.sendImplicitTLS(addr, a, from, to, msg)
			}
			if s.RequireSTARTTLS {
				return s.sendSTARTTLSRequired(addr, a, from, to, msg)
			}
			return smtp.SendMail(addr, a, from, to, msg)
		}
	}
	if err := runSMTPBounded(ctx, inner, addr, auth, s.From, s.To, msg); err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	return nil
}

// headerSafe rejects a header value that would break out of the SMTP header
// block.
func headerSafe(v string) error {
	if strings.ContainsAny(v, "\r\n") {
		return fmt.Errorf("smtp: header value must not contain CR or LF")
	}
	return nil
}

// runSMTPBounded runs send in a goroutine so ctx cancel/deadline is honored.
func runSMTPBounded(
	ctx context.Context,
	send func(string, smtp.Auth, string, []string, []byte) error,
	addr string, a smtp.Auth, from string, to []string, msg []byte,
) error {
	errCh := make(chan error, 1)
	go func() { errCh <- send(addr, a, from, to, msg) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// sendImplicitTLS connects with TLS from the first byte (port 465).
func (s *SMTP) sendImplicitTLS(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	d := &net.Dialer{Timeout: DefaultHTTPTimeout}
	conn, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DefaultHTTPTimeout))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	return deliver(c, a, from, to, msg)
}

// sendSTARTTLSRequired dials in the clear but refuses to proceed unless the
// relay advertises STARTTLS, then upgrades before any credentials or message
// content are sent.
func (s *SMTP) sendSTARTTLSRequired(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	d := &net.Dialer{Timeout: DefaultHTTPTimeout}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DefaultHTTPTimeout))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return fmt.Errorf("smtp: %s does not advertise STARTTLS (require_starttls is set)", s.Host)
	}
	if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	return deliver(c, a, from, to, msg)
}

// deliver runs the shared SMTP envelope + DATA sequence on an established,
// optionally already-upgraded client.
func deliver(c *smtp.Client, a smtp.Auth, from string, to []string, msg []byte) error {
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
