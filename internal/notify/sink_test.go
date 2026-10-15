package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
)

func TestSlackAndWebhookFanOut(t *testing.T) {
	var slackBody, whBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/slack":
			slackBody = string(b)
		case "/wh":
			whBody = string(b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	senders := []Sender{
		&Slack{WebhookURL: srv.URL + "/slack", Client: srv.Client()},
		&Webhook{URL: srv.URL + "/wh", Body: "generic", Client: srv.Client()},
	}
	msg := BuildMessage(EventFiring, "c1", "cl", "r", metricCPUUtilReq, 0.9, 0.8, "ns", "p", "c", time.Now())
	if err := FanOut(context.Background(), senders, msg); err != nil {
		t.Fatal(err)
	}
	var slack map[string]string
	if err := json.Unmarshal([]byte(slackBody), &slack); err != nil || slack["text"] == "" {
		t.Fatalf("slack: %q err=%v", slackBody, err)
	}
	var gen map[string]any
	if err := json.Unmarshal([]byte(whBody), &gen); err != nil {
		t.Fatal(err)
	}
	if gen["event"] != EventFiring || gen["client.id"] != "c1" {
		t.Fatalf("generic payload: %#v", gen)
	}
}

func TestWebhookDiscordTeams(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	msg := BuildMessage(EventFiring, "c", "k", "r", metricCPUUtilReq, 1, 0.8, "n", "p", "c", time.Now())
	wh := &Webhook{URL: srv.URL, Body: "discord", Client: srv.Client()}
	if err := wh.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	var d map[string]string
	if err := json.Unmarshal([]byte(got), &d); err != nil || d["content"] == "" {
		t.Fatalf("discord: %s", got)
	}
	wh.Body = "teams"
	if err := wh.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	var teams map[string]any
	if err := json.Unmarshal([]byte(got), &teams); err != nil {
		t.Fatal(err)
	}
	if teams["@type"] != "MessageCard" {
		t.Fatalf("teams: %#v", teams)
	}
}

func TestBuildSendersFailClosed(t *testing.T) {
	_, err := BuildSenders([]config.NotifySink{{Type: "slack", WebhookURLEnv: "KPM_TEST_MISSING_SLACK"}})
	if err == nil {
		t.Fatal("expected fail-closed")
	}
}

func TestBuildSendersSlackOK(t *testing.T) {
	t.Setenv("KPM_TEST_SLACK_OK", "https://example.com/hook")
	s, err := BuildSenders([]config.NotifySink{{Type: "slack", WebhookURLEnv: "KPM_TEST_SLACK_OK"}})
	if err != nil || len(s) != 1 {
		t.Fatalf("err=%v n=%d", err, len(s))
	}
}

func TestSMTPDialOverride(t *testing.T) {
	var called bool
	s := &SMTP{
		Host: "smtp.example", Port: 587, From: "a@b.c", To: []string{"d@e.f"},
		DialAndSend: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			called = true
			if from != "a@b.c" || len(to) != 1 {
				t.Fatalf("args %s %v", from, to)
			}
			return nil
		},
	}
	msg := BuildMessage(EventFiring, "c", "k", "r", metricCPUUtilReq, 1, 0.8, "n", "p", "c", time.Now())
	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("DialAndSend not called")
	}
}

func TestSMTPSendHonorsTimeout(t *testing.T) {
	started := make(chan struct{})
	s := &SMTP{
		Host: "smtp.example", Port: 587, From: "a@b.c", To: []string{"d@e.f"},
		DialAndSend: func(string, smtp.Auth, string, []string, []byte) error {
			close(started)
			time.Sleep(3 * time.Second)
			return nil
		},
	}
	// Shrink default by wrapping: Send always uses DefaultHTTPTimeout; exercise
	// cancel via parent context shorter than the sleep.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := s.Send(ctx, Message{Text: "x", Title: "t"})
	<-started
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestDefaultHTTPClientTimeout(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := defaultHTTPClient()
	client.Timeout = 200 * time.Millisecond
	s := &Slack{WebhookURL: srv.URL, Client: client}
	err := s.Send(context.Background(), Message{Text: "x"})
	<-started
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestBuildSMTP(t *testing.T) {
	t.Setenv("KPM_SMTP_HOST", "smtp.example")
	t.Setenv("KPM_SMTP_PORT", "587")
	t.Setenv("KPM_SMTP_FROM", "from@ex.com")
	t.Setenv("KPM_SMTP_TO", "a@ex.com,b@ex.com")
	s, err := BuildSenders([]config.NotifySink{{
		Type:    "smtp",
		HostEnv: "KPM_SMTP_HOST", PortEnv: "KPM_SMTP_PORT",
		FromEnv: "KPM_SMTP_FROM", ToEnv: "KPM_SMTP_TO",
	}})
	if err != nil {
		t.Fatal(err)
	}
	smtpSender, ok := s[0].(*SMTP)
	if !ok || len(smtpSender.To) != 2 {
		t.Fatalf("%T %#v", s[0], s[0])
	}
}
