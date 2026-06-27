// Package governance enforces OpenTag's access policy: which surfaces are
// allowed, default-deny for unbound channels, DM policy, and a best-effort
// per-channel spend meter.
//
// TODO(spend): the meter counts turns as a proxy. Replace with real token-cost
// caps once kagent or the model provider exposes usage.
package governance

import (
	"fmt"
	"sync"

	"github.com/kkkksu/opentag/internal/config"
)

// Decision is the outcome of a policy check.
type Decision struct {
	Allowed bool
	// Reason is a user-facing explanation when Allowed is false.
	Reason string
}

func allow() Decision        { return Decision{Allowed: true} }
func deny(r string) Decision { return Decision{Allowed: false, Reason: r} }

// Governor applies policy. It is safe for concurrent use.
type Governor struct {
	dmsAllowed bool
	turnCap    int

	mu    sync.Mutex
	turns map[string]int // channelID -> turns this process lifetime
}

// New builds a Governor from config.
func New(cfg *config.Config) *Governor {
	return &Governor{
		dmsAllowed: cfg.Governance.DMsAllowed(),
		turnCap:    cfg.Governance.SpendCap.TurnsPerChannel,
		turns:      make(map[string]int),
	}
}

// AllowChannel decides whether a channel mention may proceed. found reports
// whether an explicit binding existed (vs. falling back to default).
func (g *Governor) AllowChannel(channelID string, found bool) Decision {
	if !found {
		// A default binding is configured, so this is allowed but noted. When no
		// default exists, routing errors before reaching here (default-deny).
		return allow()
	}
	_ = channelID
	return allow()
}

// AllowDM decides whether a DM may proceed.
func (g *Governor) AllowDM() Decision {
	if !g.dmsAllowed {
		return deny("Direct messages are disabled for this workspace.")
	}
	return allow()
}

// ChargeTurn records one turn against a channel's cap and reports whether it is
// permitted. The boolean alert is true when usage crosses a 75% or 95%
// threshold, so callers can surface a warning.
func (g *Governor) ChargeTurn(channelID string) (d Decision, alert string) {
	if g.turnCap <= 0 {
		return allow(), ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	used := g.turns[channelID]
	if used >= g.turnCap {
		return deny(fmt.Sprintf("Channel turn budget reached (%d). Work that would exceed the limit is declined.", g.turnCap)), ""
	}
	g.turns[channelID] = used + 1
	now := used + 1
	switch {
	case crossed(used, now, g.turnCap, 95):
		alert = fmt.Sprintf("Heads up: this channel is at 95%% of its turn budget (%d/%d).", now, g.turnCap)
	case crossed(used, now, g.turnCap, 75):
		alert = fmt.Sprintf("Heads up: this channel is at 75%% of its turn budget (%d/%d).", now, g.turnCap)
	}
	return allow(), alert
}

// crossed reports whether moving from prev to cur passes the pct% threshold of cap.
func crossed(prev, cur, cap, pct int) bool {
	threshold := cap * pct / 100
	return prev < threshold && cur >= threshold
}
