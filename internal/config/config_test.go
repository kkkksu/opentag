package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const validBody = `
slack:
  appToken: ${TEST_APP_TOKEN}
  botToken: xoxb-abc
kagent:
  baseURL: http://localhost:8083
org:
  id: acme
bindings:
  - channelId: C123
    agent: {namespace: kagent, name: k8s}
`

func TestLoad_ValidAndEnvExpansion(t *testing.T) {
	t.Setenv("TEST_APP_TOKEN", "xapp-xyz")
	cfg, err := Load(writeConfig(t, validBody))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Slack.AppToken != "xapp-xyz" {
		t.Errorf("env not expanded: got %q", cfg.Slack.AppToken)
	}
	if len(cfg.Bindings) != 1 || cfg.Bindings[0].Agent.Name != "k8s" {
		t.Errorf("binding not parsed: %+v", cfg.Bindings)
	}
	if !cfg.Governance.DMsAllowed() {
		t.Errorf("DMsAllowed default should be true")
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing appToken", "slack:\n  botToken: x\nkagent:\n  baseURL: u\norg:\n  id: a\nbindings:\n  - channelId: C\n    agent: {namespace: n, name: m}\n"},
		{"missing baseURL", "slack:\n  appToken: a\n  botToken: b\norg:\n  id: a\nbindings:\n  - channelId: C\n    agent: {namespace: n, name: m}\n"},
		{"missing org", "slack:\n  appToken: a\n  botToken: b\nkagent:\n  baseURL: u\nbindings:\n  - channelId: C\n    agent: {namespace: n, name: m}\n"},
		{"no bindings nor default", "slack:\n  appToken: a\n  botToken: b\nkagent:\n  baseURL: u\norg:\n  id: a\n"},
		{"binding without agent", "slack:\n  appToken: a\n  botToken: b\nkagent:\n  baseURL: u\norg:\n  id: a\nbindings:\n  - channelId: C\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.body)); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
