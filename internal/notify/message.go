package notify

import (
	"fmt"
	"strings"
	"time"
)

// Event kinds for webhook JSON and titles.
const (
	EventFiring   = "firing"
	EventRepeat   = "repeat"
	EventResolved = "resolved"
)

const (
	LevelAlert = "alert"
	LevelOK    = "ok"
)

// Message is the shared notification payload (client.id + cluster.name required).
type Message struct {
	Event     string
	Level     string
	Title     string
	Text      string
	Rule      string
	Metric    string
	Value     float64
	Threshold float64
	Namespace string
	Pod       string
	Container string
	ClientID  string
	Cluster   string
	Timestamp time.Time
}

// BuildMessage builds a firing/repeat/resolved message for one dedupe key.
func BuildMessage(
	event, clientID, cluster, rule, metric string,
	value, threshold float64,
	ns, pod, container string,
	ts time.Time,
) Message {
	if ts.IsZero() {
		ts = time.Now().UTC()
	} else {
		ts = ts.UTC()
	}
	m := Message{
		Event:     event,
		Rule:      rule,
		Metric:    metric,
		Value:     value,
		Threshold: threshold,
		Namespace: ns,
		Pod:       pod,
		Container: container,
		ClientID:  clientID,
		Cluster:   cluster,
		Timestamp: ts,
	}
	switch event {
	case EventResolved:
		m.Level = LevelOK
		m.Title = fmt.Sprintf("kpm RESOLVED: %s %s/%s/%s", rule, ns, pod, container)
	case EventRepeat:
		m.Level = LevelAlert
		m.Title = fmt.Sprintf("kpm ALERT (ongoing): %s %s/%s/%s", rule, ns, pod, container)
	default:
		m.Level = LevelAlert
		m.Title = fmt.Sprintf("kpm ALERT: %s %s/%s/%s", rule, ns, pod, container)
	}
	m.Text = ComposeText(m)
	return m
}

// ComposeText encodes SPEC fields into plain text (Slack / SMTP body).
func ComposeText(m Message) string {
	ts := m.Timestamp.UTC().Format(time.RFC3339)
	var b strings.Builder
	b.WriteString(m.Title)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "metric=%s value=%.4g threshold=%.4g\n", m.Metric, m.Value, m.Threshold)
	fmt.Fprintf(&b, "event=%s level=%s client.id=%s cluster=%s ts=%s",
		m.Event, m.Level, m.ClientID, m.Cluster, ts)
	return b.String()
}

// GenericPayload is the JSON body for webhook body=generic.
func GenericPayload(m Message) map[string]any {
	return map[string]any{
		"event":        m.Event,
		"level":        m.Level,
		"title":        m.Title,
		"text":         m.Text,
		"rule":         m.Rule,
		"metric":       m.Metric,
		"value":        m.Value,
		"threshold":    m.Threshold,
		"namespace":    m.Namespace,
		"pod":          m.Pod,
		"container":    m.Container,
		"client.id":    m.ClientID,
		"cluster":      m.Cluster,
		"cluster.name": m.Cluster,
		"ts":           m.Timestamp.UTC().Format(time.RFC3339),
	}
}
