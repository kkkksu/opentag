package routines

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kkkksu/opentag/internal/config"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestScheduler_FiresAndStops(t *testing.T) {
	var fires int32
	var gotChannel, gotPrompt atomic.Value
	gotChannel.Store("")
	gotPrompt.Store("")

	fire := func(_ context.Context, channelID, prompt string) {
		atomic.AddInt32(&fires, 1)
		gotChannel.Store(channelID)
		gotPrompt.Store(prompt)
	}

	s := New([]config.Routine{
		{Name: "standup", ChannelID: "C1", Every: "5ms", Prompt: "summarize"},
	}, fire, discardLog())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	waitFor(t, time.Second, func() bool { return atomic.LoadInt32(&fires) >= 2 })
	if gotChannel.Load() != "C1" || gotPrompt.Load() != "summarize" {
		t.Errorf("fire args = %v / %v", gotChannel.Load(), gotPrompt.Load())
	}

	if got := s.List("C1"); len(got) != 1 || got[0].ID != "standup" {
		t.Fatalf("List = %+v", got)
	}

	if !s.Stop("standup") {
		t.Errorf("Stop should succeed")
	}
	if len(s.List("C1")) != 0 {
		t.Errorf("routine should be gone after Stop")
	}
	if s.Stop("standup") {
		t.Errorf("second Stop should be false")
	}
}

func TestScheduler_SkipsInvalidInterval(t *testing.T) {
	s := New([]config.Routine{{Name: "bad", ChannelID: "C1", Every: "notaduration", Prompt: "x"}}, func(context.Context, string, string) {}, discardLog())
	if len(s.List("C1")) != 0 {
		t.Errorf("invalid routine should be skipped")
	}
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
