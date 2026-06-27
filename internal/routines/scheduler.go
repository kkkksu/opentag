// Package routines runs standing, proactive work: on a fixed interval it fires
// a prompt at a channel's agent and posts the result without anyone asking —
// the "proactive teammate" behavior. Routines are listable and cancellable via
// `@OpenTag triggers` / `stop`.
package routines

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kkkksu/opentag/internal/config"
)

// FireFunc runs one routine occurrence: resolve the channel's agent, run the
// prompt, and post the result. Provided by the app layer.
type FireFunc func(ctx context.Context, channelID, prompt string)

// Info is a snapshot of a scheduled routine, for listing.
type Info struct {
	ID          string // routine name
	Kind        string // always "routine"
	Channel     string
	Description string
	Every       time.Duration
}

// Scheduler runs configured routines.
type Scheduler struct {
	fire FireFunc
	log  *slog.Logger

	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	routine config.Routine
	every   time.Duration
	cancel  context.CancelFunc
}

// New builds a Scheduler over the configured routines. Invalid intervals are
// skipped (config validation should prevent that).
func New(routines []config.Routine, fire FireFunc, log *slog.Logger) *Scheduler {
	s := &Scheduler{fire: fire, log: log, entries: make(map[string]*entry)}
	for _, r := range routines {
		every, err := time.ParseDuration(r.Every)
		if err != nil {
			log.Error("skip routine with invalid interval", "routine", r.Name, "every", r.Every, "err", err)
			continue
		}
		s.entries[r.Name] = &entry{routine: r, every: every}
	}
	return s
}

// Start launches all routines. They stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, e := range s.entries {
		rctx, cancel := context.WithCancel(ctx)
		e.cancel = cancel
		go s.run(rctx, e.routine, e.every)
		s.log.Info("routine scheduled", "routine", name, "channel", e.routine.ChannelID, "every", e.every)
	}
}

func (s *Scheduler) run(ctx context.Context, r config.Routine, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.fire(ctx, r.ChannelID, r.Prompt)
		}
	}
}

// List returns the routines scheduled for a channel.
func (s *Scheduler) List(channel string) []Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Info
	for name, e := range s.entries {
		if e.routine.ChannelID != channel {
			continue
		}
		out = append(out, Info{
			ID:          name,
			Kind:        "routine",
			Channel:     e.routine.ChannelID,
			Description: e.routine.Prompt,
			Every:       e.every,
		})
	}
	return out
}

// Stop cancels and removes a routine by name. Returns false if not found.
func (s *Scheduler) Stop(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[name]
	if !ok {
		return false
	}
	if e.cancel != nil {
		e.cancel()
	}
	delete(s.entries, name)
	return true
}
