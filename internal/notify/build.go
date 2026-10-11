package notify

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hrodrig/kpm/internal/config"
)

// BuildSenders resolves sink env vars (fail-closed) into Sender implementations.
func BuildSenders(sinks []config.NotifySink) ([]Sender, error) {
	if len(sinks) == 0 {
		return nil, fmt.Errorf("no notification sinks configured")
	}
	out := make([]Sender, 0, len(sinks))
	for i, s := range sinks {
		sender, err := buildOne(s)
		if err != nil {
			return nil, fmt.Errorf("notifications.sinks[%d]: %w", i, err)
		}
		out = append(out, sender)
	}
	return out, nil
}

func buildOne(s config.NotifySink) (Sender, error) {
	switch strings.ToLower(strings.TrimSpace(s.Type)) {
	case "slack":
		url, err := requireEnv(s.WebhookURLEnv)
		if err != nil {
			return nil, err
		}
		return &Slack{WebhookURL: url}, nil
	case "webhook":
		url, err := requireEnv(s.URLEnv)
		if err != nil {
			return nil, err
		}
		headers := map[string]string{}
		for hk, envName := range s.HeadersEnv {
			v, err := requireEnv(envName)
			if err != nil {
				return nil, fmt.Errorf("headers_env %q: %w", hk, err)
			}
			headers[hk] = v
		}
		body := strings.ToLower(strings.TrimSpace(s.Body))
		if body == "" {
			body = "generic"
		}
		return &Webhook{URL: url, Body: body, Headers: headers}, nil
	case "smtp":
		return buildSMTP(s)
	default:
		return nil, fmt.Errorf("unknown sink type %q", s.Type)
	}
}

func buildSMTP(s config.NotifySink) (Sender, error) {
	host, err := requireEnv(s.HostEnv)
	if err != nil {
		return nil, err
	}
	portStr, err := requireEnv(s.PortEnv)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port < 1 {
		return nil, fmt.Errorf("port_env %q: invalid port %q", s.PortEnv, portStr)
	}
	from, err := requireEnv(s.FromEnv)
	if err != nil {
		return nil, err
	}
	toRaw, err := requireEnv(s.ToEnv)
	if err != nil {
		return nil, err
	}
	to := splitAddrs(toRaw)
	if len(to) == 0 {
		return nil, fmt.Errorf("to_env %q: empty recipients", s.ToEnv)
	}
	user := ""
	pass := ""
	if strings.TrimSpace(s.UserEnv) != "" {
		user, err = requireEnv(s.UserEnv)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(s.PasswordEnv) != "" {
		pass, err = requireEnv(s.PasswordEnv)
		if err != nil {
			return nil, err
		}
	}
	return &SMTP{
		Host:     host,
		Port:     port,
		Username: user,
		Password: pass,
		From:     from,
		To:       to,
		UseTLS:   s.UseTLS,
	}, nil
}

func requireEnv(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("env var name empty")
	}
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("environment variable %s is missing or empty (fail-closed)", name)
	}
	return v, nil
}

func splitAddrs(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
