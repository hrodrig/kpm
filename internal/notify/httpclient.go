package notify

import (
	"net/http"
	"time"
)

// DefaultHTTPTimeout bounds Slack/webhook delivery so a hung sink cannot stall
// the sample daemon tick forever.
const DefaultHTTPTimeout = 15 * time.Second

// defaultHTTPClient returns a client with DefaultHTTPTimeout.
func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: DefaultHTTPTimeout}
}
