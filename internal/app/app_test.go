package app

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		in  string
		cmd string
		arg string
		ok  bool
	}{
		{"triggers", "triggers", "", true},
		{"status", "triggers", "", true},
		{"  TRIGGERS  ", "triggers", "", true},
		{"stop task-123", "stop", "task-123", true},
		{"stop", "stop", "", true},
		{"summarize the incidents", "", "", false},
		{"", "", "", false},
		{"stopwatch the build", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			cmd, arg, ok := parseCommand(tt.in)
			if cmd != tt.cmd || arg != tt.arg || ok != tt.ok {
				t.Errorf("parseCommand(%q) = (%q,%q,%v), want (%q,%q,%v)", tt.in, cmd, arg, ok, tt.cmd, tt.arg, tt.ok)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("  hello  ", 10); got != "hello" {
		t.Errorf("truncate trim = %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcde…" {
		t.Errorf("truncate cut = %q", got)
	}
}
