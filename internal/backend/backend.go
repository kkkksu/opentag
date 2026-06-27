// Package backend defines the agent-runtime abstraction OpenTag talks to. The
// only implementation today is kagent (subpackage ./kagent), but the interface
// keeps the chat/coworker layer independent of the runtime.
package backend

import (
	"context"

	"github.com/kkkksu/opentag/internal/config"
)

// EnsureSessionInput describes the durable session a thread maps to.
type EnsureSessionInput struct {
	// UserID is sent as the kagent X-User-ID (the identity work runs as).
	UserID string
	// SessionID is the kagent session id (also the A2A contextID).
	SessionID string
	// Agent is the kagent agent that answers this session.
	Agent config.AgentRef
	// Name is a human-friendly session label (e.g. the Slack thread).
	Name string
	// Source tags the origin, e.g. "opentag-slack".
	Source string
}

// StreamInput is a single turn sent to an agent.
type StreamInput struct {
	UserID    string
	SessionID string
	Agent     config.AgentRef
	Text      string
}

// StreamResult summarizes a completed (or handed-off) turn.
type StreamResult struct {
	// SessionID is the (possibly server-assigned) context id for the turn.
	SessionID string
	// TaskID is the kagent task id, when the turn produced one (async).
	TaskID string
	// Done reports whether the agent finished within the stream. When false the
	// work continues asynchronously and should be tracked via GetTask.
	Done bool
}

// Task is the state of an async kagent task.
type Task struct {
	ID       string
	State    string
	Text     string
	Terminal bool
}

// AgentBackend is the runtime OpenTag delegates work to.
type AgentBackend interface {
	// EnsureSession idempotently creates the durable session for a thread.
	EnsureSession(ctx context.Context, in EnsureSessionInput) error
	// Stream sends one turn and invokes onChunk for each streamed text fragment.
	Stream(ctx context.Context, in StreamInput, onChunk func(string)) (StreamResult, error)
	// GetTask fetches the current state (and any result text) of a task.
	GetTask(ctx context.Context, userID string, agent config.AgentRef, taskID string) (Task, error)
	// CancelTask requests cancellation of a running task.
	CancelTask(ctx context.Context, userID string, agent config.AgentRef, taskID string) error
}
