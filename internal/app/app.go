// Package app wires OpenTag's pieces together and implements the coworker core:
// it turns a normalized chat.Event into a governed, identified, audited turn
// against the agent backend.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kkkksu/opentag/internal/async"
	"github.com/kkkksu/opentag/internal/audit"
	"github.com/kkkksu/opentag/internal/backend"
	"github.com/kkkksu/opentag/internal/chat"
	"github.com/kkkksu/opentag/internal/config"
	"github.com/kkkksu/opentag/internal/governance"
	"github.com/kkkksu/opentag/internal/identity"
	"github.com/kkkksu/opentag/internal/routines"
	"github.com/kkkksu/opentag/internal/routing"
)

// App is the coworker core.
type App struct {
	cfg     *config.Config
	router  *routing.Router
	backend backend.AgentBackend
	gov     *governance.Governor
	audit   audit.Sink
	log     *slog.Logger

	// Runtime collaborators, attached after construction (see Attach/SetTeam).
	poster    chat.Poster
	tracker   *async.Tracker
	scheduler *routines.Scheduler
	team      string
}

// New builds the core from its dependencies.
func New(cfg *config.Config, router *routing.Router, be backend.AgentBackend, gov *governance.Governor, auditSink audit.Sink, log *slog.Logger) *App {
	return &App{cfg: cfg, router: router, backend: be, gov: gov, audit: auditSink, log: log}
}

// Attach wires the runtime collaborators that depend on the chat provider
// (which in turn needs App.Handle), resolving the construction cycle.
func (a *App) Attach(poster chat.Poster, tracker *async.Tracker, scheduler *routines.Scheduler) {
	a.poster = poster
	a.tracker = tracker
	a.scheduler = scheduler
}

// SetTeam records the Slack team id so proactive routines use the same channel
// identity (and therefore memory) as interactive turns.
func (a *App) SetTeam(team string) { a.team = team }

// Handle implements chat.Handler. It resolves the agent + identity, enforces
// governance, ensures the session, and streams the agent's reply.
func (a *App) Handle(ctx context.Context, ev chat.Event, out chat.Streamer) {
	if cmd, arg, ok := parseCommand(ev.Text); ok {
		a.handleCommand(ctx, ev, out, cmd, arg)
		return
	}

	plan, ok := a.resolve(ev, out)
	if !ok {
		return
	}

	if ev.Text == "" {
		out.Append("Hi! Tag me with a request and I'll get to work.")
		return
	}

	base := audit.Entry{
		Surface: ev.Surface.String(), Team: ev.Team, Channel: ev.Channel,
		ThreadTS: ev.ThreadTS, User: ev.User, Identity: plan.id.UserID,
		Agent: plan.agent.String(), SessionID: plan.id.SessionID,
		MemoryShared: plan.shared,
	}
	a.write(base, "request", "ok")

	in := backend.EnsureSessionInput{
		UserID: plan.id.UserID, SessionID: plan.id.SessionID,
		Agent: plan.agent, Name: ev.ThreadTS, Source: "user",
	}
	if err := a.backend.EnsureSession(ctx, in); err != nil {
		a.log.Error("ensure session", "err", err, "agent", plan.agent.String())
		a.write(base, "error", "ensure_session: "+err.Error())
		out.Fail(err)
		return
	}

	res, err := a.backend.Stream(ctx, backend.StreamInput{
		UserID: plan.id.UserID, SessionID: plan.id.SessionID,
		Agent: plan.agent, Text: ev.Text,
	}, out.Append)
	if err != nil {
		a.log.Error("stream", "err", err, "agent", plan.agent.String())
		a.write(base, "error", "stream: "+err.Error())
		out.Fail(err)
		return
	}

	base.TaskID = res.TaskID
	if res.Done {
		a.write(base, "completed", "ok")
		return
	}

	// Long-running work: hand off to the tracker, which posts the result back to
	// this thread when the task finishes.
	a.write(base, "handoff", "task_pending")
	if res.TaskID != "" && a.tracker != nil {
		a.tracker.Track(async.TrackInfo{
			TaskID: res.TaskID, Channel: ev.Channel, ThreadTS: ev.ThreadTS,
			UserID: plan.id.UserID, Agent: plan.agent, Description: truncate(ev.Text, 80),
		})
		out.Append(fmt.Sprintf("\n\n_Working on this in the background — I'll post here when it's done. (id: `%s`)_", res.TaskID))
	}
}

// turnPlan is the resolved agent + identity for an event.
type turnPlan struct {
	agent  config.AgentRef
	id     identity.Identity
	shared bool // whether memory is shared across the channel
}

// resolve applies routing + governance and computes identity. On denial it
// replies via out and returns ok=false.
func (a *App) resolve(ev chat.Event, out chat.Streamer) (turnPlan, bool) {
	switch ev.Surface {
	case chat.SurfaceChannel:
		binding, found, err := a.router.Resolve(ev.Channel)
		if err != nil {
			a.write(a.denyEntry(ev, ""), "denied", "no_binding")
			out.Append("I'm not set up to work in this channel yet. An admin can bind it to an agent.")
			return turnPlan{}, false
		}
		if d := a.gov.AllowChannel(ev.Channel, found); !d.Allowed {
			a.write(a.denyEntry(ev, binding.Agent.String()), "denied", d.Reason)
			out.Append(d.Reason)
			return turnPlan{}, false
		}
		if d, alert := a.gov.ChargeTurn(ev.Channel); !d.Allowed {
			a.write(a.denyEntry(ev, binding.Agent.String()), "denied", d.Reason)
			out.Append(d.Reason)
			return turnPlan{}, false
		} else if alert != "" {
			out.Append(alert + "\n\n")
		}
		return turnPlan{
			agent:  binding.Agent,
			id:     identity.ForChannelThread(a.cfg.Org.ID, ev.Team, ev.Channel, ev.ThreadTS, binding.MemoryShared()),
			shared: binding.MemoryShared(),
		}, true

	case chat.SurfaceDM:
		if d := a.gov.AllowDM(); !d.Allowed {
			a.write(a.denyEntry(ev, ""), "denied", d.Reason)
			out.Append(d.Reason)
			return turnPlan{}, false
		}
		if a.cfg.Default == nil {
			a.write(a.denyEntry(ev, ""), "denied", "no_dm_agent")
			out.Append("No default agent is configured for direct messages.")
			return turnPlan{}, false
		}
		return turnPlan{
			agent: a.cfg.Default.Agent,
			id:    identity.ForDM(ev.Team, ev.User, ev.ThreadTS),
		}, true
	}
	return turnPlan{}, false
}

func (a *App) denyEntry(ev chat.Event, agent string) audit.Entry {
	return audit.Entry{
		Surface: ev.Surface.String(), Team: ev.Team, Channel: ev.Channel,
		ThreadTS: ev.ThreadTS, User: ev.User, Agent: agent,
	}
}

func (a *App) write(e audit.Entry, event, outcome string) {
	e.Event = event
	e.Outcome = outcome
	a.audit.Write(e)
}

// parseCommand recognizes control commands ("triggers"/"status", "stop <id>").
// ok is false for ordinary requests, which go to the agent.
func parseCommand(text string) (cmd, arg string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", "", false
	}
	switch strings.ToLower(fields[0]) {
	case "triggers", "status":
		return "triggers", "", true
	case "stop":
		if len(fields) > 1 {
			arg = fields[1]
		}
		return "stop", arg, true
	}
	return "", "", false
}

func (a *App) handleCommand(ctx context.Context, ev chat.Event, out chat.Streamer, cmd, arg string) {
	switch cmd {
	case "triggers":
		out.Append(a.listTriggers(ev.Channel))
	case "stop":
		if arg == "" {
			out.Append("Usage: `stop <id>`. Run `triggers` to see ids.")
			return
		}
		if a.tracker != nil && a.tracker.Stop(ctx, arg) {
			out.Append("🛑 Stopped `" + arg + "`.")
			return
		}
		if a.scheduler != nil && a.scheduler.Stop(arg) {
			out.Append("🛑 Stopped routine `" + arg + "`.")
			return
		}
		out.Append("No standing work with id `" + arg + "` in this workspace.")
	}
}

// listTriggers summarizes the standing work (async tasks + routines) for a channel.
func (a *App) listTriggers(channel string) string {
	var lines []string
	if a.tracker != nil {
		for _, i := range a.tracker.List(channel) {
			lines = append(lines, fmt.Sprintf("• task `%s` — %s (since %s)", i.ID, i.Description, i.Started.Format("15:04")))
		}
	}
	if a.scheduler != nil {
		for _, i := range a.scheduler.List(channel) {
			lines = append(lines, fmt.Sprintf("• routine `%s` — every %s: %s", i.ID, i.Every, i.Description))
		}
	}
	if len(lines) == 0 {
		return "No standing work in this channel."
	}
	return "*Standing work here:*\n" + strings.Join(lines, "\n")
}

// RunRoutine executes one proactive routine occurrence: it runs prompt against
// the channel's agent under the channel identity and posts the result. It is the
// FireFunc handed to the routines scheduler.
func (a *App) RunRoutine(ctx context.Context, channelID, prompt string) {
	binding, _, err := a.router.Resolve(channelID)
	if err != nil {
		a.log.Warn("routine: channel not bound", "channel", channelID, "err", err)
		return
	}
	id := identity.ForChannelThread(a.cfg.Org.ID, a.team, channelID, "routine:"+prompt, binding.MemoryShared())
	base := audit.Entry{
		Surface: "routine", Team: a.team, Channel: channelID, Identity: id.UserID,
		Agent: binding.Agent.String(), SessionID: id.SessionID, MemoryShared: binding.MemoryShared(),
	}

	if err := a.backend.EnsureSession(ctx, backend.EnsureSessionInput{
		UserID: id.UserID, SessionID: id.SessionID, Agent: binding.Agent, Name: "routine", Source: "user",
	}); err != nil {
		a.log.Error("routine ensure session", "err", err, "channel", channelID)
		a.write(base, "error", "ensure_session: "+err.Error())
		return
	}

	var sb strings.Builder
	if _, err := a.backend.Stream(ctx, backend.StreamInput{
		UserID: id.UserID, SessionID: id.SessionID, Agent: binding.Agent, Text: prompt,
	}, func(s string) { sb.WriteString(s) }); err != nil {
		a.log.Error("routine stream", "err", err, "channel", channelID)
		a.write(base, "error", "stream: "+err.Error())
		return
	}

	text := strings.TrimSpace(sb.String())
	if text == "" {
		a.write(base, "completed", "empty")
		return
	}
	if a.poster != nil {
		if err := a.poster.Post(ctx, chat.Target{Channel: channelID}, text); err != nil {
			a.log.Error("routine post", "err", err, "channel", channelID)
		}
	}
	a.write(base, "proactive", "ok")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
