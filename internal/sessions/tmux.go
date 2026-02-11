package sessions

import (
	"fmt"
	"os/exec"
	"strings"
)

// TmuxRunner executes tmux commands for session management.
type TmuxRunner struct{}

func NewTmuxRunner() *TmuxRunner {
	return &TmuxRunner{}
}

// CreateSession creates a new tmux session with the given name, working directory, and command.
func (t *TmuxRunner) CreateSession(tmuxName, cwd, startCmd string) error {
	args := []string{"new-session", "-d", "-s", tmuxName, "-c", cwd}
	if startCmd != "" {
		args = append(args, startCmd)
	}
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux new-session: %s: %w", string(out), err)
	}
	return nil
}

// KillSession kills the tmux session.
func (t *TmuxRunner) KillSession(tmuxName string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", tmuxName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux kill-session: %s: %w", string(out), err)
	}
	return nil
}

// SendKeys sends text followed by Enter to the tmux session.
func (t *TmuxRunner) SendKeys(tmuxName, text string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", tmuxName, text, "Enter")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux send-keys: %s: %w", string(out), err)
	}
	return nil
}

// SendRawKeys sends raw key tokens to the tmux session (no implicit Enter).
func (t *TmuxRunner) SendRawKeys(tmuxName string, keys []string) error {
	args := []string{"send-keys", "-t", tmuxName}
	args = append(args, keys...)
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux send-keys: %s: %w", string(out), err)
	}
	return nil
}

// Interrupt sends Ctrl+C to the tmux session.
func (t *TmuxRunner) Interrupt(tmuxName string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", tmuxName, "C-c")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux interrupt: %s: %w", string(out), err)
	}
	return nil
}

// HasSession checks if a tmux session exists.
func (t *TmuxRunner) HasSession(tmuxName string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", tmuxName)
	return cmd.Run() == nil
}

// ListSessions returns all tmux session names.
func (t *TmuxRunner) ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// "no server running" means no sessions
		if strings.Contains(string(out), "no server running") ||
			strings.Contains(string(out), "no sessions") {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %s: %w", string(out), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var result []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result, nil
}
