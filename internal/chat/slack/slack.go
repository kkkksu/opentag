// Package slack implements chat.Provider over Slack Socket Mode, so OpenTag
// needs no public URL. It dispatches channel mentions and DMs to the core
// handler and streams replies back into the originating thread.
package slack

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/kkkksu/opentag/internal/chat"
)

// flushInterval throttles streamed message edits to respect Slack rate limits.
const flushInterval = 1100 * time.Millisecond

var mentionRE = regexp.MustCompile(`<@[A-Z0-9]+>`)

// Provider is a Slack Socket Mode chat provider.
type Provider struct {
	api     *slack.Client
	sm      *socketmode.Client
	handler chat.Handler
	log     *slog.Logger

	botUserID string
	teamID    string
}

// New constructs a Slack provider. handler is invoked once per inbound event.
func New(appToken, botToken string, handler chat.Handler, log *slog.Logger) *Provider {
	api := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	return &Provider{
		api:     api,
		sm:      socketmode.New(api),
		handler: handler,
		log:     log,
	}
}

// Run connects and dispatches events until ctx is cancelled.
func (p *Provider) Run(ctx context.Context) error {
	auth, err := p.api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack auth test: %w", err)
	}
	p.botUserID = auth.UserID
	p.teamID = auth.TeamID
	p.log.Info("slack connected", "bot_user", p.botUserID, "team", p.teamID)

	go p.loop(ctx)
	return p.sm.RunContext(ctx)
}

func (p *Provider) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-p.sm.Events:
			if !ok {
				return
			}
			if evt.Type != socketmode.EventTypeEventsAPI {
				continue
			}
			api, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			p.sm.Ack(*evt.Request)
			if api.Type != slackevents.CallbackEvent {
				continue
			}
			if ce := p.toEvent(api); ce != nil {
				go p.dispatch(ctx, *ce)
			}
		}
	}
}

// toEvent normalizes a Slack callback into a chat.Event, or nil to ignore.
func (p *Provider) toEvent(api slackevents.EventsAPIEvent) *chat.Event {
	team := api.TeamID
	switch ev := api.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		return &chat.Event{
			Surface:  chat.SurfaceChannel,
			Team:     team,
			Channel:  ev.Channel,
			ThreadTS: threadRoot(ev.ThreadTimeStamp, ev.TimeStamp),
			User:     ev.User,
			Text:     p.clean(ev.Text),
		}
	case *slackevents.MessageEvent:
		// Only handle genuine user DMs; ignore bot echoes, edits, and the bot's
		// own messages.
		if ev.ChannelType != "im" || ev.BotID != "" || ev.SubType != "" || ev.User == p.botUserID || ev.User == "" {
			return nil
		}
		return &chat.Event{
			Surface:  chat.SurfaceDM,
			Team:     team,
			Channel:  ev.Channel,
			ThreadTS: threadRoot(ev.ThreadTimeStamp, ev.TimeStamp),
			User:     ev.User,
			Text:     p.clean(ev.Text),
		}
	}
	return nil
}

func (p *Provider) dispatch(ctx context.Context, ev chat.Event) {
	defer func() {
		if r := recover(); r != nil {
			p.log.Error("panic handling event", "recover", r, "channel", ev.Channel)
		}
	}()
	out := newStreamer(p.api, ev.Channel, ev.ThreadTS)
	p.handler(ctx, ev, out)
	out.Close()
}

// clean strips bot mentions and trims whitespace.
func (p *Provider) clean(text string) string {
	return strings.TrimSpace(mentionRE.ReplaceAllString(text, ""))
}

func threadRoot(threadTS, ts string) string {
	if threadTS != "" {
		return threadTS
	}
	return ts
}

// streamer renders a single, incrementally-updated Slack message.
type streamer struct {
	api      *slack.Client
	channel  string
	threadTS string

	mu        sync.Mutex
	ts        string
	buf       strings.Builder
	lastFlush time.Time
	closed    bool
}

func newStreamer(api *slack.Client, channel, threadTS string) *streamer {
	return &streamer{api: api, channel: channel, threadTS: threadTS}
}

func (s *streamer) Append(text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.WriteString(text)
	s.render(false)
}

func (s *streamer) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
	s.buf.WriteString("⚠️ ")
	s.buf.WriteString(err.Error())
	s.render(true)
	s.closed = true
}

func (s *streamer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.render(true)
	s.closed = true
}

// render posts or edits the reply. Caller must hold s.mu.
func (s *streamer) render(force bool) {
	if s.closed {
		return
	}
	now := time.Now()
	if !force && now.Sub(s.lastFlush) < flushInterval {
		return
	}
	display := s.buf.String()
	if display == "" {
		display = "_…_"
	}
	opts := []slack.MsgOption{slack.MsgOptionText(display, false)}
	if s.threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(s.threadTS))
	}
	if s.ts == "" {
		if _, ts, err := s.api.PostMessage(s.channel, opts...); err == nil {
			s.ts = ts
		}
	} else {
		_, _, _, _ = s.api.UpdateMessage(s.channel, s.ts, slack.MsgOptionText(display, false))
	}
	s.lastFlush = now
}
