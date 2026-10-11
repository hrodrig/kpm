package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hrodrig/kpm/internal/config"
	"github.com/hrodrig/kpm/internal/notify"
	"github.com/hrodrig/kpm/internal/sample"
)

// wrapDaemonTick adds notify evaluation after logTick when notifications.enabled.
func wrapDaemonTick(
	ctx context.Context,
	cfg *config.Config,
	clientID, clusterName string,
	logTick sample.TickFunc,
	errW io.Writer,
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
			_, _ = fmt.Fprintf(errW, "notify: %v\n", err)
		}
	}, nil
}
