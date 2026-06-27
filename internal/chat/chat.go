// Package chat defines the provider-agnostic contract between a chat platform
// (Slack today) and OpenTag's coworker core. A provider turns platform events
// into a normalized Event and hands the core a Streamer to reply through.
package chat

import "context"

// Surface is where an interaction originates.
type Surface int

const (
	// SurfaceChannel is a channel mention — runs under the org service identity.
	SurfaceChannel Surface = iota
	// SurfaceDM is a direct message — runs under the user's personal identity.
	SurfaceDM
)

func (s Surface) String() string {
	if s == SurfaceDM {
		return "dm"
	}
	return "channel"
}

// Event is a normalized inbound request (a mention or DM).
type Event struct {
	Surface  Surface
	Team     string // Slack team/workspace id
	Channel  string // channel id (or DM channel id)
	ThreadTS string // thread root timestamp; identifies the working thread
	User     string // id of the user who invoked OpenTag
	Text     string // message text with the bot mention stripped
}

// Streamer delivers a reply incrementally. Implementations may batch/throttle
// the underlying platform writes.
type Streamer interface {
	// Append adds a fragment of streamed text.
	Append(s string)
	// Fail reports an error to the user (terminal).
	Fail(err error)
	// Close finalizes the reply, flushing any buffered text.
	Close()
}

// Handler processes one Event, replying through out. It is invoked once per
// inbound event, typically in its own goroutine.
type Handler func(ctx context.Context, ev Event, out Streamer)

// Target identifies where an out-of-band message should be posted. A zero
// ThreadTS posts at the top of the channel rather than in a thread.
type Target struct {
	Channel  string
	ThreadTS string
}

// Poster sends a message outside the request/response flow — used for async
// task results and proactive routine posts.
type Poster interface {
	Post(ctx context.Context, target Target, text string) error
}

// Provider is a chat-platform integration (e.g. Slack Socket Mode).
type Provider interface {
	// Run blocks, dispatching events to the configured Handler until ctx ends.
	Run(ctx context.Context) error
}
