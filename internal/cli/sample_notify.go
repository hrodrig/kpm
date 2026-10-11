package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/httpserver"
	"github.com/hrodrig/kpm/internal/notify"
	"github.com/hrodrig/kpm/internal/sample"
)

// wrapDaemonTick adds notify evaluation after logTick when notifications.enabled.
// metrics may be nil; when set, notify fan-out errors increment NotifyErrors.
func wrapDaemonTick(
	ctx context.Context,
	cfg *config.Config,
	clientID, clusterName string,
	logTick sample.TickFunc,
	errW io.Writer,
	metrics *httpserver.Counters,
) (sample.TickFunc, error) {
	if !cfg.Notifications.Enabled {
		return logTick, nil
	}
	senders, err := notify.BuildSenders(cfg.Notifications.Sinks)
	if err != nil {
		return nil, err
	}
	rules, usedBuiltins := notify.EffectiveRules(cfg.Notifications)
	if usedBuiltins {
		_, _ = fmt.Fprintln(errW, notify.FormatBuiltinLog())
	}
	eng := notify.NewEngine(rules, clientID, clusterName, notify.FanOutFunc(senders))
	return func(res sample.Result) {
		logTick(res)
		if err := eng.Evaluate(ctx, res.Records, time.Now().UTC()); err != nil {
			if metrics != nil {
				metrics.NotifyErrors.Add(1)
			}
			_, _ = fmt.Fprintf(errW, "notify: %v\n", err)
		}
	}, nil
}
