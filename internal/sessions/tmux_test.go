package sessions

import (
	"strings"
	"testing"
)

func TestMapKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ESC", "Escape"},
		{"esc", "Escape"},
		{"UP", "Up"},
		{"DOWN", "Down"},
		{"TAB", "Tab"},
		{"ENTER", "Enter"},
		{"CTRL_C", "C-c"},
		{"CTRL+C", "C-c"},
		{"CTRL_D", "C-d"},
		{"BACKSPACE", "BSpace"},
		{"SPACE", "Space"},
		{"a", "a"},
		{"hello", "hello"},
	}

	for _, tt := range tests {
		got := mapKey(tt.input)
		if got != tt.expected {
			t.Errorf("mapKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"my-project", "my-project"},
		{"my project!@#", "myproject"},
		{"", "session"},
		{"a-very-long-name-that-exceeds-thirty-characters-limit", "a-very-long-name-that-exceeds-"},
	}

	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestTmuxRunnerListSessions(t *testing.T) {
	runner := NewTmuxRunner()
	// This should not error even if tmux server is not running
	sessions, err := runner.ListSessions()
	if err != nil {
		// "no server running" is acceptable — tmux just isn't running
		if !strings.Contains(err.Error(), "no server") {
			t.Errorf("ListSessions unexpected error: %v", err)
		}
	}
	_ = sessions
}
