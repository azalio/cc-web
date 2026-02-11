package sessions

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/user/cc-web/internal/config"
)

// TtydManager manages ttyd processes for terminal sessions.
type TtydManager struct {
	mu        sync.Mutex
	cfg       *config.Config
	processes map[string]*exec.Cmd // tmuxName -> ttyd process
	usedPorts map[int]bool
	ttydPath  string
}

func NewTtydManager(cfg *config.Config) *TtydManager {
	path := cfg.TtydPath
	if path == "" {
		// Try to find ttyd in PATH
		if p, err := exec.LookPath("ttyd"); err == nil {
			path = p
		}
	}
	return &TtydManager{
		cfg:       cfg,
		processes: make(map[string]*exec.Cmd),
		usedPorts: make(map[int]bool),
		ttydPath:  path,
	}
}

// Available returns whether ttyd is available.
func (t *TtydManager) Available() bool {
	return t.ttydPath != ""
}

// AllocatePort finds the next available port in the configured range.
func (t *TtydManager) AllocatePort() (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for port := t.cfg.TtydBasePort; port <= t.cfg.TtydMaxPort; port++ {
		if !t.usedPorts[port] {
			t.usedPorts[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", t.cfg.TtydBasePort, t.cfg.TtydMaxPort)
}

// Start launches a ttyd process that attaches to the given tmux session.
func (t *TtydManager) Start(tmuxName string, port int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.Available() {
		// ttyd not available — sessions will work without embedded terminal
		// Users can still use send/interrupt/keys APIs
		return nil
	}

	// Stop existing if any
	if cmd, ok := t.processes[tmuxName]; ok {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}

	cmd := exec.Command(t.ttydPath,
		"--port", fmt.Sprintf("%d", port),
		"--interface", "127.0.0.1",
		"--writable",
		"--base-path", fmt.Sprintf("/t/%s%s/", t.cfg.TmuxPrefix, tmuxName),
		"tmux", "attach-session", "-t", tmuxName,
	)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ttyd on port %d: %w", port, err)
	}

	t.processes[tmuxName] = cmd
	t.usedPorts[port] = true

	// Monitor process in background
	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		delete(t.processes, tmuxName)
		t.mu.Unlock()
	}()

	return nil
}

// Stop kills the ttyd process for a tmux session.
func (t *TtydManager) Stop(tmuxName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if cmd, ok := t.processes[tmuxName]; ok {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		delete(t.processes, tmuxName)
	}
}

// StopAll kills all ttyd processes.
func (t *TtydManager) StopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for name, cmd := range t.processes {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		delete(t.processes, name)
	}
}
