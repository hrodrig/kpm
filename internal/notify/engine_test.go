package notify

import (
	"context"
	"testing"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

func hotCPU() store.Record {
	return store.Record{
		Namespace: "app", Pod: "p1", Container: "c1",
		CPUUtilMcores: 90, CPURequestMcores: 100,
	}
}

func coolCPU() store.Record {
	return store.Record{
		Namespace: "app", Pod: "p1", Container: "c1",
		CPUUtilMcores: 10, CPURequestMcores: 100,
	}
}

func testRule(hold, clear, rep int) config.NotifyRule {
	return config.NotifyRule{
		Name:              "cpu-hot",
		Metric:            metricCPUUtilReq,
		ThresholdRatio:    0.80,
		HoldForSeconds:    hold,
		ClearAfterSeconds: clear,
		RepeatSeconds:     &rep,
	}
}

func TestEngineHoldBeforeFire(t *testing.T) {
	rules, _ := EffectiveRules(config.Notifications{Rules: []config.NotifyRule{testRule(10, 10, 30)}})
	var events []string
	eng := NewEngine(rules, "bastion-a", "prod", func(_ context.Context, m Message) error {
		events = append(events, m.Event)
		return nil
	})
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	rec := hotCPU()
	_ = eng.Evaluate(context.Background(), []store.Record{rec}, base)
	_ = eng.Evaluate(context.Background(), []store.Record{rec}, base.Add(5*time.Second))
	if len(events) != 0 {
		t.Fatalf("early fire: %v", events)
	}
	_ = eng.Evaluate(context.Background(), []store.Record{rec}, base.Add(10*time.Second))
	if len(events) != 1 || events[0] != EventFiring {
		t.Fatalf("want firing, got %v", events)
	}
}

func TestEngineRepeatAndResolve(t *testing.T) {
	rules, _ := EffectiveRules(config.Notifications{Rules: []config.NotifyRule{testRule(1, 10, 30)}})
	var events []string
	eng := NewEngine(rules, "bastion-a", "prod", func(_ context.Context, m Message) error {
		events = append(events, m.Event)
		return nil
	})
	base := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	rec := hotCPU()
	_ = eng.Evaluate(context.Background(), []store.Record{rec}, base)
	_ = eng.Evaluate(context.Background(), []store.Record{rec}, base.Add(time.Second))
	if len(events) != 1 {
		t.Fatalf("fire: %v", events)
	}
	_ = eng.Evaluate(context.Background(), []store.Record{rec}, base.Add(20*time.Second))
	if len(events) != 1 {
		t.Fatalf("no repeat yet: %v", events)
	}
	_ = eng.Evaluate(context.Background(), []store.Record{rec}, base.Add(40*time.Second))
	if len(events) != 2 || events[1] != EventRepeat {
		t.Fatalf("want repeat: %v", events)
	}
	cool := coolCPU()
	_ = eng.Evaluate(context.Background(), []store.Record{cool}, base.Add(41*time.Second))
	_ = eng.Evaluate(context.Background(), []store.Record{cool}, base.Add(51*time.Second))
	if len(events) != 3 || events[2] != EventResolved {
		t.Fatalf("want resolved: %v", events)
	}
}

func TestEffectiveRulesBuiltins(t *testing.T) {
	rules, used := EffectiveRules(config.Notifications{Enabled: true})
	if !used || len(rules) != 2 {
		t.Fatalf("builtins: used=%v n=%d", used, len(rules))
	}
	if FormatBuiltinLog() == "" {
		t.Fatal("empty log")
	}
}

func TestMetricBreach(t *testing.T) {
	rec := store.Record{CPUUtilMcores: 90, CPURequestMcores: 100, MemUtilBytes: 50, MemRequestBytes: 100}
	v, ok := MetricValue(metricCPUUtilReq, rec)
	if !ok || v != 0.9 {
		t.Fatalf("cpu ratio: %v %v", v, ok)
	}
	if !Breach(metricCPUUtilReq, 0.8, 0, v) {
		t.Fatal("expected breach")
	}
	_, ok = MetricValue(metricCPUUtilReq, store.Record{CPUUtilMcores: 10, CPURequestMcores: 0})
	if ok {
		t.Fatal("denom 0 should skip")
	}
}
