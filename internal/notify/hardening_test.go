package notify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hrodrig/kpm/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSlackAndWebhookRedactSecretsInErrors(t *testing.T) {
	secretURL := "https://hooks.slack.com/services/T111/B222/SUPERSECRETTOKEN?sig=LEAKME"
	failClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
	}

	slackErr := (&Slack{WebhookURL: secretURL, Client: failClient}).Send(context.Background(), Message{Text: "hi"})
	if slackErr == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(slackErr, context.DeadlineExceeded) {
		t.Fatalf("expected wrapped DeadlineExceeded, got %v", slackErr)
	}
	for _, secret := range []string{"SUPERSECRETTOKEN", "T111", "B222", "LEAKME"} {
		if strings.Contains(slackErr.Error(), secret) {
			t.Fatalf("slack error leaked secret %q: %v", secret, slackErr)
		}
	}
	if !strings.Contains(slackErr.Error(), "https://hooks.slack.com/[redacted]") {
		t.Fatalf("expected redacted host in slack error, got %v", slackErr)
	}

	whURL := "https://discord.com/api/webhooks/999/WHSECRET?wait=true"
	whErr := (&Webhook{URL: whURL, Body: "discord", Client: failClient}).Send(context.Background(), Message{Text: "hi"})
	if whErr == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(whErr, context.DeadlineExceeded) {
		t.Fatalf("expected wrapped DeadlineExceeded, got %v", whErr)
	}
	if strings.Contains(whErr.Error(), "WHSECRET") || strings.Contains(whErr.Error(), "999") {
		t.Fatalf("webhook error leaked secret: %v", whErr)
	}
}

func TestDefaultHTTPClientRefusesRedirects(t *testing.T) {
	calls := 0
	client := defaultHTTPClient()
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusFound,
			Status:     "302 Found",
			Header:     http.Header{"Location": []string{"http://169.254.169.254/latest/meta-data/"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	wh := &Webhook{
		URL:     "https://hooks.example.com/secret-path",
		Body:    "generic",
		Headers: map[string]string{"X-API-Key": "top-secret"},
		Client:  client,
	}
	err := wh.Send(context.Background(), Message{Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("expected 302 error without following redirect, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 request (no redirect follow), got %d", calls)
	}
}

func TestBuildSendersRejectsInvalidURLSchemes(t *testing.T) {
	badURLs := []string{
		"file:///etc/passwd",
		"ftp://example.com/secret",
		"javascript:alert(1)",
		"https:///missing-host/secret",
		"not-a-url",
	}
	for _, raw := range badURLs {
		t.Setenv("KPM_BAD_SLACK_URL", raw)
		_, err := BuildSenders([]config.NotifySink{{Type: "slack", WebhookURLEnv: "KPM_BAD_SLACK_URL"}})
		if err == nil {
			t.Fatalf("slack URL %q: expected scheme/host validation error", raw)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "/etc/passwd") {
			t.Fatalf("validation error leaked URL path: %v", err)
		}

		t.Setenv("KPM_BAD_WH_URL", raw)
		_, err = BuildSenders([]config.NotifySink{{Type: "webhook", URLEnv: "KPM_BAD_WH_URL"}})
		if err == nil {
			t.Fatalf("webhook URL %q: expected scheme/host validation error", raw)
		}
	}
}
