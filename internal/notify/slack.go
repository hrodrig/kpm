package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Slack sends via Incoming Webhook (plain text).
type Slack struct {
	WebhookURL string
	Client     *http.Client
}

func (s *Slack) Type() string { return "slack" }

func (s *Slack) Send(ctx context.Context, m Message) error {
	raw, err := json.Marshal(map[string]string{"text": m.Text})
	if err != nil {
		return fmt.Errorf("slack payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, strings.NewReader(string(raw)))
	if err != nil {
		return fmt.Errorf("slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("slack webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook: status %s", resp.Status)
	}
	return nil
}
