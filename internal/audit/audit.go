// Package audit records every interaction OpenTag handles — who asked, in which
// channel/thread, which agent ran, and the outcome. It writes JSON Lines to a
// file or stderr.
package audit

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Entry is one audit record.
type Entry struct {
	Time         time.Time `json:"time"`
	Surface      string    `json:"surface"`
	Team         string    `json:"team"`
	Channel      string    `json:"channel"`
	ThreadTS     string    `json:"thread_ts,omitempty"`
	User         string    `json:"user"`
	Identity     string    `json:"identity"` // X-User-ID the work ran as
	Agent        string    `json:"agent,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	TaskID       string    `json:"task_id,omitempty"`
	MemoryShared bool      `json:"memory_shared,omitempty"`
	Event        string    `json:"event"`   // e.g. "request", "completed", "denied"
	Outcome      string    `json:"outcome"` // e.g. "ok", "error", reason
}

// Sink records audit entries. Implementations must be safe for concurrent use.
type Sink interface {
	Write(Entry)
}

// writerSink emits JSONL to an io.Writer.
type writerSink struct {
	mu  sync.Mutex
	w   io.Writer
	enc *json.Encoder
}

// NewWriter returns a Sink writing JSONL to w.
func NewWriter(w io.Writer) Sink {
	return &writerSink{w: w, enc: json.NewEncoder(w)}
}

// NewFile returns a Sink appending JSONL to path, or stderr when path is empty.
func NewFile(path string) (Sink, error) {
	if path == "" {
		return NewWriter(os.Stderr), nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return NewWriter(f), nil
}

func (s *writerSink) Write(e Entry) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(e) // best-effort; never block the request path on audit IO
}
