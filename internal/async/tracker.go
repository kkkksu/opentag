// Package async tracks long-running kagent tasks. When a turn doesn't finish
// inline, the tracker polls the task until it reaches a terminal state and posts
// the result back into the originating thread — so users can delegate and walk
// away.
package async

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kkkksu/opentag/internal/backend"
	"github.com/kkkksu/opentag/internal/chat"
	"github.com/kkkksu/opentag/internal/config"
)

const (
	defaultPollInterval = 10 * time.Second
	// maxConsecutiveErrors stops polling a task that keeps erroring (e.g. it was
	// deleted) so goroutines don't leak.
	maxConsecutiveErrors = 6
)

// TrackInfo describes a task to follow.
type TrackInfo struct {
	TaskID      string
	Channel     string
	ThreadTS    string
	UserID      string
	Agent       config.AgentRef
	Description string
}

// Info is a snapshot of standing work, for listing via `@OpenTag triggers`.
type Info struct {
	ID          string
	Kind        string // always "task" here
	Channel     string
	Description string
	Started     time.Time
}

// Tracker follows in-flight tasks.
type Tracker struct {
	be     backend.AgentBackend
	poster chat.Poster
	log    *slog.Logger

	pollInterval time.Duration

	mu     sync.Mutex
	active map[string]*tracked
}

type tracked struct {
	info    TrackInfo
	started time.Time
	cancel  context.CancelFunc
}

// New builds a Tracker.
func New(be backend.AgentBackend, poster chat.Poster, log *slog.Logger) *Tracker {
	return &Tracker{
		be:           be,
		poster:       poster,
		log:          log,
		pollInterval: defaultPollInterval,
		active:       make(map[string]*tracked),
	}
}

// WithPollInterval overrides the poll cadence (used in tests).
func (t *Tracker) WithPollInterval(d time.Duration) *Tracker {
	t.pollInterval = d
	return t
}

// Track begins following a task in the background.
func (t *Tracker) Track(info TrackInfo) {
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.active[info.TaskID] = &tracked{info: info, started: time.Now(), cancel: cancel}
	t.mu.Unlock()
	go t.run(ctx, info)
}

func (t *Tracker) run(ctx context.Context, info TrackInfo) {
	defer t.remove(info.TaskID)
	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	errs := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			task, err := t.be.GetTask(ctx, info.UserID, info.Agent, info.TaskID)
			if err != nil {
				errs++
				t.log.Warn("poll task", "task", info.TaskID, "err", err, "attempt", errs)
				if errs >= maxConsecutiveErrors {
					return
				}
				continue
			}
			errs = 0
			if task.Terminal {
				text := task.Text
				if text == "" {
					text = "Task finished."
				}
				if err := t.poster.Post(ctx, chat.Target{Channel: info.Channel, ThreadTS: info.ThreadTS}, "✅ "+text); err != nil {
					t.log.Error("post task result", "task", info.TaskID, "err", err)
				}
				return
			}
		}
	}
}

// List returns the active tasks for a channel.
func (t *Tracker) List(channel string) []Info {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Info
	for id, tr := range t.active {
		if tr.info.Channel != channel {
			continue
		}
		out = append(out, Info{
			ID:          id,
			Kind:        "task",
			Channel:     tr.info.Channel,
			Description: tr.info.Description,
			Started:     tr.started,
		})
	}
	return out
}

// Stop cancels tracking and requests task cancellation. Returns false if the id
// is not tracked here.
func (t *Tracker) Stop(ctx context.Context, id string) bool {
	t.mu.Lock()
	tr, ok := t.active[id]
	t.mu.Unlock()
	if !ok {
		return false
	}
	tr.cancel()
	if err := t.be.CancelTask(ctx, tr.info.UserID, tr.info.Agent, id); err != nil {
		t.log.Warn("cancel task", "task", id, "err", err)
	}
	t.remove(id)
	return true
}

func (t *Tracker) remove(id string) {
	t.mu.Lock()
	delete(t.active, id)
	t.mu.Unlock()
}
