package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Webhook POSTs JSON with a body preset (generic | discord | teams).
type Webhook struct {
	URL     string
	Body    string // generic | discord | teams
	Headers map[string]string
	Client  *http.Client
}

func (w *Webhook) Type() string { return "webhook" }

func (w *Webhook) Send(ctx context.Context, m Message) error {
	payload, err := w.encode(m)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}
	client := w.Client
	if client == nil {
		client = defaultHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: status %s", resp.Status)
	}
	return nil
}

func (w *Webhook) encode(m Message) ([]byte, error) {
	body := strings.ToLower(strings.TrimSpace(w.Body))
	if body == "" {
		body = "generic"
	}
	var v any
	switch body {
	case "discord":
		v = map[string]string{"content": m.Text}
	case "teams":
		// Simple MessageCard-compatible shape for Incoming Webhook.
		theme := "FF0000"
		if m.Event == EventResolved {
			theme = "00FF00"
		}
		v = map[string]any{
			"@type":      "MessageCard",
			"@context":   "https://schema.org/extensions",
			"summary":    m.Title,
			"themeColor": theme,
			"title":      m.Title,
			"text":       m.Text,
		}
	case "generic":
		v = GenericPayload(m)
	default:
		return nil, fmt.Errorf("webhook body preset %q", w.Body)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("webhook payload: %w", err)
	}
	return raw, nil
}
