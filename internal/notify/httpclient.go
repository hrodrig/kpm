package notify

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultHTTPTimeout bounds Slack/webhook delivery so a hung sink cannot stall
// the sample daemon tick forever.
const DefaultHTTPTimeout = 15 * time.Second

// defaultHTTPClient returns a client with DefaultHTTPTimeout that refuses
// HTTP redirects (prevents cross-origin custom header forwarding and redirect SSRF).
func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: DefaultHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// validateHTTPURL enforces http/https scheme and a non-empty host without
// echoing the raw URL (which may carry webhook secrets) in error messages.
func validateHTTPURL(raw string) error {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL scheme %q is not allowed (must be http or https)", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("URL missing host")
	}
	return nil
}

// redactURLError strips path, query, and userinfo from *url.Error so webhook
// bearer tokens embedded in Slack/Discord/Teams URLs never appear in logs.
func redactURLError(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	target := "[redacted]"
	if u, perr := url.Parse(ue.URL); perr == nil && u.Scheme != "" && u.Host != "" {
		target = u.Scheme + "://" + u.Host + "/[redacted]"
	}
	return fmt.Errorf("%s %q: %w", ue.Op, target, ue.Err)
}
