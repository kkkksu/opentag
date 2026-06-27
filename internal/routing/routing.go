// Package routing resolves an inbound Slack channel to the ChannelBinding that
// governs it (which agent answers, plus per-channel policy).
package routing

import (
	"fmt"

	"github.com/kkkksu/opentag/internal/config"
)

// Router resolves channels to bindings.
type Router struct {
	byChannel map[string]config.ChannelBinding
	fallback  *config.ChannelBinding
}

// New builds a Router from the loaded configuration.
func New(cfg *config.Config) *Router {
	byChannel := make(map[string]config.ChannelBinding, len(cfg.Bindings))
	for _, b := range cfg.Bindings {
		byChannel[b.ChannelID] = b
	}
	return &Router{byChannel: byChannel, fallback: cfg.Default}
}

// Resolve returns the binding for channelID. The boolean reports whether the
// binding was explicit (true) or came from the default (false). When neither
// exists, an error is returned so governance can default-deny.
func (r *Router) Resolve(channelID string) (config.ChannelBinding, bool, error) {
	if b, ok := r.byChannel[channelID]; ok {
		return b, true, nil
	}
	if r.fallback != nil {
		b := *r.fallback
		b.ChannelID = channelID
		return b, false, nil
	}
	return config.ChannelBinding{}, false, fmt.Errorf("no binding for channel %s and no defaultBinding set", channelID)
}
