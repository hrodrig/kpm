package notify

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
)

// stubSMTP returns a sender whose transport is replaced, plus a getter for the
// raw message handed to it. The message is nil when the transport was never
// reached.
func stubSMTP() (*SMTP, func() []byte) {
	var got []byte
	s := &SMTP{
		Host: "smtp.example", Port: 587,
		From: "from@example.com", To: []string{"to@example.com"},
		DialAndSend: func(_ string, _ smtp.Auth, _ string, _ []string, msg []byte) error {
			got = append([]byte(nil), msg...)
			return nil
		},
	}
	return s, func() []byte { return got }
}

// #31: a CRLF in the subject closes the header block, so the caller could
// append its own headers (a second To:, Bcc:, ...). It must be rejected before
// the transport is reached, not silently encoded away.
func TestSMTPSendRejectsCRLFInSubject(t *testing.T) {
	s, got := stubSMTP()
	err := s.Send(context.Background(), Message{
		Title: "cpu util high\r\nBcc: attacker@example.com",
		Text:  "body",
	})
	if err == nil {
		t.Fatal("expected a CRLF subject to be rejected")
	}
	if !strings.Contains(err.Error(), "CR or LF") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got() != nil {
		t.Fatal("transport must not be reached when a header value is unsafe")
	}
}

func TestSMTPSendRejectsCRLFInHeaderFields(t *testing.T) {
	cases := map[string]func(*SMTP){
		"recipient": func(s *SMTP) { s.To = []string{"to@example.com\r\nBcc: x@y.z"} },
		"sender":    func(s *SMTP) { s.From = "from@example.com\r\nBcc: x@y.z" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s, got := stubSMTP()
			mutate(s)
			err := s.Send(context.Background(), Message{Title: "t", Text: "b"})
			if err == nil || !strings.Contains(err.Error(), "CR or LF") {
				t.Fatalf("expected rejection, got %v", err)
			}
			if got() != nil {
				t.Fatal("transport must not be reached")
			}
		})
	}
}

// A non-ASCII title must come out as an RFC 2047 encoded-word; emitted raw,
// relays reject the message or mangle the subject.
func TestSMTPSendEncodesNonASCIISubject(t *testing.T) {
	s, got := stubSMTP()
	title := "Uso de CPU alto \u2014 reinicio de \u00abbusybox\u00bb"
	if err := s.Send(context.Background(), Message{Title: title, Text: "body"}); err != nil {
		t.Fatal(err)
	}
	raw := string(got())
	if raw == "" {
		t.Fatal("transport not reached")
	}
	if !strings.Contains(raw, "=?utf-8?") {
		t.Fatalf("subject not RFC 2047 encoded:\n%s", raw)
	}
	if strings.Contains(raw, title) {
		t.Fatalf("raw non-ASCII title leaked into the message:\n%s", raw)
	}
}

// ASCII titles are already valid, so they must not be needlessly re-encoded.
func TestSMTPSendLeavesASCIISubjectUnencoded(t *testing.T) {
	s, got := stubSMTP()
	const title = "cpu util_vs_request firing on api-7d9f8c"
	if err := s.Send(context.Background(), Message{Title: title, Text: "body"}); err != nil {
		t.Fatal(err)
	}
	raw := string(got())
	if !strings.Contains(raw, "Subject: "+title+"\r\n") {
		t.Fatalf("ASCII subject was rewritten:\n%s", raw)
	}
	if strings.Contains(raw, "=?utf-8?") {
		t.Fatalf("ASCII subject should not be encoded:\n%s", raw)
	}
}

// fakeRelay answers the minimum SMTP dialogue needed by net/smtp and reports
// whether it advertises STARTTLS.
func fakeRelay(t *testing.T, advertiseSTARTTLS bool) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = fmt.Fprint(conn, "220 fake ESMTP\r\n")
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				if advertiseSTARTTLS {
					_, _ = fmt.Fprint(conn, "250-fake\r\n250-STARTTLS\r\n250 SIZE 10240000\r\n")
				} else {
					_, _ = fmt.Fprint(conn, "250-fake\r\n250 SIZE 10240000\r\n")
				}
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				_, _ = fmt.Fprint(conn, "502 not implemented\r\n")
			}
		}
	}()
	h, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return h, n
}

// require_starttls must fail closed against a relay that does not offer it.
// Without this, use_tls:false is opportunistic and silently cleartext.
func TestSMTPRequireSTARTTLSRefusesRelayWithoutIt(t *testing.T) {
	host, port := fakeRelay(t, false)
	s := &SMTP{
		Host: host, Port: port,
		From: "from@example.com", To: []string{"to@example.com"},
		RequireSTARTTLS: true,
	}
	err := s.Send(context.Background(), Message{Title: "t", Text: "b"})
	if err == nil {
		t.Fatal("expected refusal against a relay without STARTTLS")
	}
	if !strings.Contains(err.Error(), "does not advertise STARTTLS") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// When the relay does advertise STARTTLS the code must attempt the upgrade
// rather than bail out. The fake cannot complete a TLS handshake, so the tell
// is which error comes back: a TLS failure, not the "not advertised" refusal.
func TestSMTPRequireSTARTTLSProceedsWhenAdvertised(t *testing.T) {
	host, port := fakeRelay(t, true)
	s := &SMTP{
		Host: host, Port: port,
		From: "from@example.com", To: []string{"to@example.com"},
		RequireSTARTTLS: true,
	}
	err := s.Send(context.Background(), Message{Title: "t", Text: "b"})
	if err == nil {
		t.Fatal("expected the TLS handshake against the fake to fail")
	}
	if strings.Contains(err.Error(), "does not advertise STARTTLS") {
		t.Fatalf("STARTTLS was advertised but the code refused: %v", err)
	}
}

// The option has to survive the config -> sink wiring.
func TestBuildSMTPCarriesRequireSTARTTLS(t *testing.T) {
	t.Setenv("KPM_TEST_SMTP_HOST", "smtp.example")
	t.Setenv("KPM_TEST_SMTP_PORT", "587")
	t.Setenv("KPM_TEST_SMTP_FROM", "from@ex.com")
	t.Setenv("KPM_TEST_SMTP_TO", "a@ex.com")
	senders, err := BuildSenders([]config.NotifySink{{
		Type: "smtp", HostEnv: "KPM_TEST_SMTP_HOST", PortEnv: "KPM_TEST_SMTP_PORT",
		FromEnv: "KPM_TEST_SMTP_FROM", ToEnv: "KPM_TEST_SMTP_TO",
		RequireSTARTTLS: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	smtpSender, ok := senders[0].(*SMTP)
	if !ok {
		t.Fatalf("got %T", senders[0])
	}
	if !smtpSender.RequireSTARTTLS {
		t.Fatal("require_starttls did not reach the sender")
	}
}
