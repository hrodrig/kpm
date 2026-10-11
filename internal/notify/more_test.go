package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

type failSender struct{ name string }

func (f failSender) Type() string { return f.name }
func (f failSender) Send(context.Context, Message) error {
	return errors.New("boom")
}

func TestFanOutErrorsAndEmpty(t *testing.T) {
	if err := FanOut(context.Background(), nil, Message{}); err == nil {
		t.Fatal("empty senders")
	}
	err := FanOut(context.Background(), []Sender{failSender{"a"}, failSender{"b"}}, Message{})
	if err == nil || err.Error() == "boom" {
		t.Fatalf("want joined errs, got %v", err)
	}
	fn := FanOutFunc([]Sender{failSender{"x"}})
	if err := fn(context.Background(), Message{}); err == nil {
		t.Fatal("FanOutFunc")
	}
}

func TestMetricValueAll(t *testing.T) {
	rec := store.Record{
		CPUUtilMcores: 50, CPURequestMcores: 100, CPULimitMcores: 200,
		MemUtilBytes: 40, MemRequestBytes: 80, MemLimitBytes: 160,
	}
	cases := []string{
		metricCPUUtilReq, metricMemUtilReq, metricCPUUtilLim, metricMemUtilLim,
		metricCPUUtilAbs, metricMemUtilAbs,
	}
	for _, m := range cases {
		v, ok := MetricValue(m, rec)
		if !ok || v <= 0 {
			t.Fatalf("%s: %v %v", m, v, ok)
		}
		_ = ThresholdForDisplay(m, 0.8, 100)
		_ = Breach(m, 0.8, 100, v)
	}
	if _, ok := MetricValue("nope", rec); ok {
		t.Fatal("bad metric")
	}
	if ValidateMetricName(metricCPUUtilReq) != nil || ValidateMetricName("x") == nil {
		t.Fatal("ValidateMetricName")
	}
}

func TestEngineNamespaceAndResolveOff(t *testing.T) {
	off := false
	zero := 0
	rule := config.NotifyRule{
		Name: "ns-only", Namespaces: []string{"app"},
		Metric: metricCPUUtilAbs, Threshold: 10,
		HoldForSeconds: 1, ClearAfterSeconds: 1, RepeatSeconds: &zero,
		NotifyOnResolve: &off,
	}
	rules, _ := EffectiveRules(config.Notifications{Rules: []config.NotifyRule{rule}})
	var events []string
	eng := NewEngine(rules, "c", "k", func(_ context.Context, m Message) error {
		events = append(events, m.Event)
		return nil
	})
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	other := store.Record{Namespace: "other", Pod: "p", Container: "c", CPUUtilMcores: 99}
	hot := store.Record{Namespace: "app", Pod: "p", Container: "c", CPUUtilMcores: 99}
	_ = eng.Evaluate(context.Background(), []store.Record{other}, base)
	_ = eng.Evaluate(context.Background(), []store.Record{hot}, base)
	_ = eng.Evaluate(context.Background(), []store.Record{hot}, base.Add(time.Second))
	if len(events) != 1 || events[0] != EventFiring {
		t.Fatalf("events %v", events)
	}
	cool := store.Record{Namespace: "app", Pod: "p", Container: "c", CPUUtilMcores: 1}
	_ = eng.Evaluate(context.Background(), []store.Record{cool}, base.Add(2*time.Second))
	_ = eng.Evaluate(context.Background(), []store.Record{cool}, base.Add(3*time.Second))
	if len(events) != 1 {
		t.Fatalf("resolve should be off: %v", events)
	}
}

func TestBuildWebhookHeadersAndTypes(t *testing.T) {
	t.Setenv("KPM_WH", "https://example.com/h")
	t.Setenv("KPM_HDR", "Bearer x")
	s, err := BuildSenders([]config.NotifySink{{
		Type: "webhook", URLEnv: "KPM_WH", Body: "generic",
		HeadersEnv: map[string]string{"Authorization": "KPM_HDR"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wh := s[0].(*Webhook)
	if wh.Headers["Authorization"] != "Bearer x" {
		t.Fatalf("%v", wh.Headers)
	}
	if wh.Type() != "webhook" {
		t.Fatal(wh.Type())
	}
	if (&Slack{}).Type() != "slack" || (&SMTP{}).Type() != "smtp" {
		t.Fatal("types")
	}
}

func TestBuildSMTPWithAuth(t *testing.T) {
	t.Setenv("KPM_SMTP_HOST", "h")
	t.Setenv("KPM_SMTP_PORT", "465")
	t.Setenv("KPM_SMTP_FROM", "a@b.c")
	t.Setenv("KPM_SMTP_TO", "d@e.f")
	t.Setenv("KPM_SMTP_USER", "u")
	t.Setenv("KPM_SMTP_PASS", "p")
	s, err := BuildSenders([]config.NotifySink{{
		Type: "smtp", HostEnv: "KPM_SMTP_HOST", PortEnv: "KPM_SMTP_PORT",
		FromEnv: "KPM_SMTP_FROM", ToEnv: "KPM_SMTP_TO",
		UserEnv: "KPM_SMTP_USER", PasswordEnv: "KPM_SMTP_PASS", UseTLS: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	sm := s[0].(*SMTP)
	if sm.Username != "u" || !sm.UseTLS {
		t.Fatalf("%+v", sm)
	}
}

func TestWebhookBadBodyAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	wh := &Webhook{URL: srv.URL, Body: "nope", Client: srv.Client()}
	msg := BuildMessage(EventFiring, "c", "k", "r", metricCPUUtilReq, 1, 0.8, "n", "p", "c", time.Now())
	if err := wh.Send(context.Background(), msg); err == nil {
		t.Fatal("bad body")
	}
	wh.Body = "generic"
	if err := wh.Send(context.Background(), msg); err == nil {
		t.Fatal("500")
	}
}

func TestResolveNotifyOnResolveDefault(t *testing.T) {
	if !ResolveNotifyOnResolve(config.NotifyRule{}) {
		t.Fatal("default true")
	}
	v := true
	if !ResolveNotifyOnResolve(config.NotifyRule{NotifyOnResolve: &v}) {
		t.Fatal("explicit true")
	}
	if ruleRepeatSeconds(config.NotifyRule{}) != defaultRepeatSeconds {
		t.Fatal("repeat default")
	}
}

func TestNewEngineNilSend(t *testing.T) {
	eng := NewEngine(nil, "c", "k", nil)
	if err := eng.Evaluate(context.Background(), nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func TestSMTPEmptyToAndBadPort(t *testing.T) {
	s := &SMTP{From: "a@b.c"}
	if err := s.Send(context.Background(), Message{}); err == nil {
		t.Fatal("empty to")
	}
	t.Setenv("KPM_SMTP_HOST", "h")
	t.Setenv("KPM_SMTP_PORT", "nope")
	t.Setenv("KPM_SMTP_FROM", "a@b.c")
	t.Setenv("KPM_SMTP_TO", "d@e.f")
	_, err := BuildSenders([]config.NotifySink{{
		Type: "smtp", HostEnv: "KPM_SMTP_HOST", PortEnv: "KPM_SMTP_PORT",
		FromEnv: "KPM_SMTP_FROM", ToEnv: "KPM_SMTP_TO",
	}})
	if err == nil {
		t.Fatal("bad port")
	}
}
