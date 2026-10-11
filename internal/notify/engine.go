package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/store"
)

type dedupeKey struct {
	Rule, Namespace, Pod, Container, Metric string
}

type keyState struct {
	trueSince  time.Time
	falseSince time.Time
	firing     bool
	lastNotify time.Time
	lastValue  float64
}

// Engine tracks hold/repeat/resolve and emits Messages.
type Engine struct {
	rules    []config.NotifyRule
	clientID string
	cluster  string
	states   map[dedupeKey]*keyState
	send     func(context.Context, Message) error
}

// NewEngine builds an evaluator. send may be FanOut wrapper; nil send is a no-op (tests).
func NewEngine(rules []config.NotifyRule, clientID, cluster string, send func(context.Context, Message) error) *Engine {
	if send == nil {
		send = func(context.Context, Message) error { return nil }
	}
	return &Engine{
		rules:    rules,
		clientID: clientID,
		cluster:  cluster,
		states:   make(map[dedupeKey]*keyState),
		send:     send,
	}
}

// Evaluate processes one sample tick. Returns joined send errors (best-effort).
func (e *Engine) Evaluate(ctx context.Context, records []store.Record, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	active := make(map[dedupeKey]float64)
	for _, rule := range e.rules {
		for _, rec := range records {
			if !nsMatch(rule.Namespaces, rec.Namespace) {
				continue
			}
			val, ok := MetricValue(rule.Metric, rec)
			if !ok {
				continue
			}
			if !Breach(rule.Metric, rule.ThresholdRatio, rule.Threshold, val) {
				continue
			}
			k := dedupeKey{
				Rule: rule.Name, Namespace: rec.Namespace, Pod: rec.Pod,
				Container: rec.Container, Metric: rule.Metric,
			}
			active[k] = val
		}
	}

	var errs []error
	seen := make(map[dedupeKey]bool, len(active))
	for k, val := range active {
		seen[k] = true
		if err := e.tickTrue(ctx, k, val, now); err != nil {
			errs = append(errs, err)
		}
	}
	for k, st := range e.states {
		if seen[k] {
			continue
		}
		if err := e.tickFalse(ctx, k, st, now); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrs(errs)
}

func (e *Engine) tickTrue(ctx context.Context, k dedupeKey, val float64, now time.Time) error {
	rule := e.ruleByName(k.Rule)
	if rule == nil {
		return nil
	}
	st := e.states[k]
	if st == nil {
		st = &keyState{}
		e.states[k] = st
	}
	if !st.firing && st.trueSince.IsZero() {
		st.trueSince = now
	}
	st.falseSince = time.Time{}
	st.lastValue = val

	hold := time.Duration(rule.HoldForSeconds) * time.Second
	if !st.firing {
		if now.Sub(st.trueSince) < hold {
			return nil
		}
		st.firing = true
		st.lastNotify = now
		return e.emit(ctx, EventFiring, *rule, k, val, now)
	}

	rep := ruleRepeatSeconds(*rule)
	if rep <= 0 {
		return nil
	}
	if now.Sub(st.lastNotify) < time.Duration(rep)*time.Second {
		return nil
	}
	st.lastNotify = now
	return e.emit(ctx, EventRepeat, *rule, k, val, now)
}

func (e *Engine) tickFalse(ctx context.Context, k dedupeKey, st *keyState, now time.Time) error {
	rule := e.ruleByName(k.Rule)
	if rule == nil {
		return nil
	}
	if !st.firing {
		// Not firing: reset trueSince so a brief blip does not accumulate across gaps.
		st.trueSince = time.Time{}
		return nil
	}
	if st.falseSince.IsZero() {
		st.falseSince = now
	}
	clearFor := time.Duration(rule.ClearAfterSeconds) * time.Second
	if now.Sub(st.falseSince) < clearFor {
		return nil
	}
	st.firing = false
	st.trueSince = time.Time{}
	st.falseSince = time.Time{}
	if !ResolveNotifyOnResolve(*rule) {
		return nil
	}
	return e.emit(ctx, EventResolved, *rule, k, st.lastValue, now)
}

func (e *Engine) emit(ctx context.Context, event string, rule config.NotifyRule, k dedupeKey, val float64, now time.Time) error {
	th := ThresholdForDisplay(rule.Metric, rule.ThresholdRatio, rule.Threshold)
	msg := BuildMessage(event, e.clientID, e.cluster, rule.Name, rule.Metric, val, th, k.Namespace, k.Pod, k.Container, now)
	return e.send(ctx, msg)
}

func (e *Engine) ruleByName(name string) *config.NotifyRule {
	for i := range e.rules {
		if e.rules[i].Name == name {
			return &e.rules[i]
		}
	}
	return nil
}

func nsMatch(allow []string, ns string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, a := range allow {
		if strings.TrimSpace(a) == ns {
			return true
		}
	}
	return false
}

func joinErrs(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		parts := make([]string, len(errs))
		for i, e := range errs {
			parts[i] = e.Error()
		}
		return fmt.Errorf("%s", strings.Join(parts, "; "))
	}
}
