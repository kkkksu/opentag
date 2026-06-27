package routing

import (
	"testing"

	"github.com/kkkksu/opentag/internal/config"
)

func TestResolve(t *testing.T) {
	cfg := &config.Config{
		Bindings: []config.ChannelBinding{
			{ChannelID: "C1", Agent: config.AgentRef{Namespace: "ns", Name: "a"}},
		},
		Default: &config.ChannelBinding{Agent: config.AgentRef{Namespace: "ns", Name: "def"}},
	}
	r := New(cfg)

	t.Run("explicit binding", func(t *testing.T) {
		b, found, err := r.Resolve("C1")
		if err != nil || !found || b.Agent.Name != "a" {
			t.Fatalf("got %+v found=%v err=%v", b, found, err)
		}
	})

	t.Run("falls back to default", func(t *testing.T) {
		b, found, err := r.Resolve("CX")
		if err != nil || found || b.Agent.Name != "def" {
			t.Fatalf("got %+v found=%v err=%v", b, found, err)
		}
		if b.ChannelID != "CX" {
			t.Errorf("default binding should adopt channel id, got %q", b.ChannelID)
		}
	})
}

func TestResolve_DefaultDeny(t *testing.T) {
	r := New(&config.Config{Bindings: []config.ChannelBinding{{ChannelID: "C1", Agent: config.AgentRef{Namespace: "n", Name: "m"}}}})
	if _, _, err := r.Resolve("CX"); err == nil {
		t.Errorf("expected error for unbound channel with no default")
	}
}
