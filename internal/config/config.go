// Package config loads OpenTag's runtime configuration: Slack credentials, the
// kagent endpoint, org identity, and the per-channel bindings + governance that
// shape how the @OpenTag coworker behaves.
//
// Fields are present for the full roadmap (memory scope, ambient, spend caps),
// but only a subset is wired today; the rest is planned.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level OpenTag configuration.
type Config struct {
	Slack      SlackConfig      `yaml:"slack"`
	Kagent     KagentConfig     `yaml:"kagent"`
	Org        OrgConfig        `yaml:"org"`
	Governance Governance       `yaml:"governance"`
	Bindings   []ChannelBinding `yaml:"bindings"`
	// Default applies to channels with no explicit binding. When nil and a
	// channel is unbound, governance default-deny refuses the request.
	Default *ChannelBinding `yaml:"defaultBinding"`
	// Audit configures where the audit trail is written.
	Audit AuditConfig `yaml:"audit"`
}

// SlackConfig holds the two tokens required for Socket Mode operation.
type SlackConfig struct {
	// AppToken is the app-level token (xapp-...) used to open the Socket Mode connection.
	AppToken string `yaml:"appToken"`
	// BotToken is the bot user OAuth token (xoxb-...) used to call the Web API.
	BotToken string `yaml:"botToken"`
}

// KagentConfig points OpenTag at a running kagent HTTP/A2A server.
type KagentConfig struct {
	// BaseURL is the root of the kagent app server, e.g. http://localhost:8083.
	// Agents are reached at {BaseURL}/api/a2a/{namespace}/{name}.
	BaseURL string `yaml:"baseURL"`
}

// OrgConfig identifies the organization, used to mint the per-channel service
// identity that channel mentions run under.
type OrgConfig struct {
	// ID is a short, stable org slug, e.g. "acme".
	ID string `yaml:"id"`
}

// Governance holds workspace-wide policy knobs.
type Governance struct {
	// DMsEnabled controls whether @OpenTag responds in direct messages. When
	// false, DMs are refused (admins can disable DMs org-wide). Default true.
	DMsEnabled *bool `yaml:"dmsEnabled"`
	// SpendCap is a best-effort per-channel cap (turns/tasks). 0 = unlimited.
	// TODO(spend): replace turn/task proxy with real token-cost caps once kagent
	// or the model provider exposes usage.
	SpendCap SpendCap `yaml:"spendCap"`
}

// SpendCap is the best-effort metering policy (see TODO(spend)).
type SpendCap struct {
	// TurnsPerChannel caps assistant turns per channel per window. 0 = unlimited.
	TurnsPerChannel int `yaml:"turnsPerChannel"`
}

// AgentRef identifies a kagent Agent CRD by namespace and name.
type AgentRef struct {
	Namespace string `yaml:"namespace"`
	Name      string `yaml:"name"`
}

func (a AgentRef) String() string { return a.Namespace + "/" + a.Name }

// IsZero reports whether the ref is unset.
func (a AgentRef) IsZero() bool { return a.Namespace == "" && a.Name == "" }

// ChannelBinding maps a Slack channel to the kagent agent that answers it, plus
// the per-channel policy. One binding == one teammate per channel.
type ChannelBinding struct {
	ChannelID string   `yaml:"channelId"`
	Agent     AgentRef `yaml:"agent"`
	// Private marks a private channel: its memory is isolated (planned).
	Private bool `yaml:"private"`
	// Ambient opts the channel into proactive behavior (planned). Off by default.
	Ambient bool `yaml:"ambient"`
	// SharedMemory controls whether the whole channel shares one identity, and
	// therefore one memory, across threads. When true (default), kagent's
	// per-(agent,user) memory becomes channel-scoped: facts the agent learns in
	// one thread are recalled in others. When false, each thread is isolated.
	// (Actual recall/save is performed by the kagent agent's memory tools; this
	// only controls the identity the turn runs as.)
	SharedMemory *bool `yaml:"sharedMemory"`
}

// MemoryShared reports the effective shared-memory policy (default true).
func (b ChannelBinding) MemoryShared() bool {
	return b.SharedMemory == nil || *b.SharedMemory
}

// AuditConfig configures the audit sink.
type AuditConfig struct {
	// Path is a file to append JSONL audit entries to. Empty = stderr.
	Path string `yaml:"path"`
}

// DMsEnabled returns the effective DM policy (default true).
func (g Governance) DMsAllowed() bool {
	return g.DMsEnabled == nil || *g.DMsEnabled
}

// Load reads and validates a YAML config file. ${VAR} references in string
// values are expanded from the environment so secrets can live outside the file.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	expanded := os.ExpandEnv(string(raw))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Slack.AppToken) == "" {
		return fmt.Errorf("slack.appToken is required (xapp-...)")
	}
	if strings.TrimSpace(c.Slack.BotToken) == "" {
		return fmt.Errorf("slack.botToken is required (xoxb-...)")
	}
	if strings.TrimSpace(c.Kagent.BaseURL) == "" {
		return fmt.Errorf("kagent.baseURL is required")
	}
	if strings.TrimSpace(c.Org.ID) == "" {
		return fmt.Errorf("org.id is required (used to mint per-channel service identities)")
	}
	if len(c.Bindings) == 0 && c.Default == nil {
		return fmt.Errorf("at least one binding or a defaultBinding is required")
	}
	for i, b := range c.Bindings {
		if b.ChannelID == "" {
			return fmt.Errorf("bindings[%d].channelId is required", i)
		}
		if b.Agent.IsZero() {
			return fmt.Errorf("bindings[%d].agent must set namespace and name", i)
		}
	}
	if c.Default != nil && c.Default.Agent.IsZero() {
		return fmt.Errorf("defaultBinding.agent must set namespace and name")
	}
	return nil
}
