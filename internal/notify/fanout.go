package notify

import (
	"context"
	"fmt"
)

// Sender delivers one Message to a destination.
type Sender interface {
	Send(ctx context.Context, msg Message) error
	Type() string
}

// FanOut sends m to every sender, continuing past failures.
func FanOut(ctx context.Context, senders []Sender, m Message) error {
	if len(senders) == 0 {
		return fmt.Errorf("no notification senders configured")
	}
	var errs []error
	for _, s := range senders {
		if err := s.Send(ctx, m); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Type(), err))
		}
	}
	return joinErrs(errs)
}

// FanOutFunc adapts FanOut to Engine send callback.
func FanOutFunc(senders []Sender) func(context.Context, Message) error {
	return func(ctx context.Context, m Message) error {
		return FanOut(ctx, senders, m)
	}
}
