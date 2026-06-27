// Package app wires OpenTag's pieces together and implements the coworker core:
// it turns a normalized chat.Event into a governed, identified, audited turn
// against the agent backend.
package app

import (
	"context"
	"log/slog"

	"github.com/kkkksu/opentag/internal/audit"
	"github.com/kkkksu/opentag/internal/backend"
	"github.com/kkkksu/opentag/internal/chat"
	"github.com/kkkksu/opentag/internal/config"
	"github.com/kkkksu/opentag/internal/governance"
	"github.com/kkkksu/opentag/internal/identity"
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
}

// New builds the core from its dependencies.
func New(cfg *config.Config, router *routing.Router, be backend.AgentBackend, gov *governance.Governor, auditSink audit.Sink, log *slog.Logger) *App {
	return &App{cfg: cfg, router: router, backend: be, gov: gov, audit: auditSink, log: log}
}

// Handle implements chat.Handler. It resolves the agent + identity, enforces
// governance, ensures the session, and streams the agent's reply.
func (a *App) Handle(ctx context.Context, ev chat.Event, out chat.Streamer) {
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
	} else {
		// Long-running work continues asynchronously (full tracking is planned).
		a.write(base, "handoff", "task_pending")
		a.log.Info("task continues async", "task", res.TaskID, "agent", plan.agent.String())
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
