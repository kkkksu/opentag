package async

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kkkksu/opentag/internal/backend"
	"github.com/kkkksu/opentag/internal/chat"
	"github.com/kkkksu/opentag/internal/config"
)

// fakeBackend reports a task as terminal after a set number of polls.
type fakeBackend struct {
	mu          sync.Mutex
	polls       int
	terminalAt  int
	text        string
	cancelled   bool
	cancelCalls int
}

func (f *fakeBackend) EnsureSession(context.Context, backend.EnsureSessionInput) error { return nil }
func (f *fakeBackend) Stream(context.Context, backend.StreamInput, func(string)) (backend.StreamResult, error) {
	return backend.StreamResult{}, nil
}
func (f *fakeBackend) GetTask(_ context.Context, _ string, _ config.AgentRef, id string) (backend.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	if f.polls >= f.terminalAt {
		return backend.Task{ID: id, State: "completed", Text: f.text, Terminal: true}, nil
	}
	return backend.Task{ID: id, State: "working"}, nil
}
func (f *fakeBackend) CancelTask(context.Context, string, config.AgentRef, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = true
	f.cancelCalls++
	return nil
}

type fakePoster struct {
	mu    sync.Mutex
	posts []chat.Target
	last  string
}

func (p *fakePoster) Post(_ context.Context, t chat.Target, text string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.posts = append(p.posts, t)
	p.last = text
	return nil
}
func (p *fakePoster) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.posts)
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestTracker_PostsResultOnCompletion(t *testing.T) {
	be := &fakeBackend{terminalAt: 2, text: "all done"}
	poster := &fakePoster{}
	tr := New(be, poster, discardLog()).WithPollInterval(5 * time.Millisecond)

	tr.Track(TrackInfo{TaskID: "task-1", Channel: "C1", ThreadTS: "1.1", Description: "do it"})

	waitFor(t, time.Second, func() bool { return poster.count() == 1 })
	if poster.last != "✅ all done" {
		t.Errorf("posted text = %q", poster.last)
	}
	if got := tr.List("C1"); len(got) != 0 {
		t.Errorf("completed task should be removed, got %d", len(got))
	}
}

func TestTracker_ListAndStop(t *testing.T) {
	be := &fakeBackend{terminalAt: 1000} // never completes during the test
	poster := &fakePoster{}
	tr := New(be, poster, discardLog()).WithPollInterval(time.Hour)

	tr.Track(TrackInfo{TaskID: "task-2", Channel: "C1", Description: "long job"})
	if got := tr.List("C1"); len(got) != 1 || got[0].ID != "task-2" {
		t.Fatalf("List = %+v", got)
	}
	if got := tr.List("other"); len(got) != 0 {
		t.Errorf("List for other channel should be empty")
	}

	if !tr.Stop(context.Background(), "task-2") {
		t.Errorf("Stop should report success")
	}
	if !be.cancelled {
		t.Errorf("Stop should cancel the backend task")
	}
	if tr.Stop(context.Background(), "missing") {
		t.Errorf("Stop of unknown id should be false")
	}
}

func TestTracker_StopsAfterRepeatedErrors(t *testing.T) {
	be := &errBackend{}
	poster := &fakePoster{}
	tr := New(be, poster, discardLog()).WithPollInterval(time.Millisecond)
	tr.Track(TrackInfo{TaskID: "bad", Channel: "C1"})
	waitFor(t, time.Second, func() bool { return len(tr.List("C1")) == 0 })
}

type errBackend struct{ fakeBackend }

func (e *errBackend) GetTask(context.Context, string, config.AgentRef, string) (backend.Task, error) {
	return backend.Task{}, errors.New("nope")
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}
