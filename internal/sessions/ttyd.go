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
	portMap   map[string]int // tmuxName -> port (for releasing on stop)
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
		portMap:   make(map[string]int),
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

// ReleasePort returns a port to the pool (e.g., on create failure).
func (t *TtydManager) ReleasePort(port int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.usedPorts, port)
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

	// Kill existing if any (don't Wait — monitor goroutine handles reaping)
	if cmd, ok := t.processes[tmuxName]; ok {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		// Unregister old entry so the old monitor goroutine becomes a no-op
		delete(t.processes, tmuxName)
		if oldPort, ok := t.portMap[tmuxName]; ok {
			delete(t.usedPorts, oldPort)
			delete(t.portMap, tmuxName)
		}
	}

	// tmuxName already contains the TmuxPrefix — use it directly in base-path
	cmd := exec.Command(t.ttydPath,
		"--port", fmt.Sprintf("%d", port),
		"--interface", "127.0.0.1",
		"--writable",
		"--base-path", fmt.Sprintf("/t/%s/", tmuxName),
		"tmux", "attach-session", "-t", tmuxName,
	)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ttyd on port %d: %w", port, err)
	}

	t.processes[tmuxName] = cmd
	t.usedPorts[port] = true
	t.portMap[tmuxName] = port

	// Monitor process in background — only place that calls Wait on this cmd
	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		defer t.mu.Unlock()
		// Only clean up if this is still the registered process (not replaced)
		if current, ok := t.processes[tmuxName]; ok && current == cmd {
			delete(t.processes, tmuxName)
			if p, ok := t.portMap[tmuxName]; ok {
				delete(t.usedPorts, p)
				delete(t.portMap, tmuxName)
			}
		}
	}()

	return nil
}

// Stop kills the ttyd process for a tmux session and releases its port.
// Unregisters immediately so the monitor goroutine's cleanup becomes a no-op.
func (t *TtydManager) Stop(tmuxName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if cmd, ok := t.processes[tmuxName]; ok {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		// Unregister immediately so monitor goroutine becomes a no-op
		delete(t.processes, tmuxName)
	}
	if port, ok := t.portMap[tmuxName]; ok {
		delete(t.usedPorts, port)
		delete(t.portMap, tmuxName)
	}
}

// StopAll kills all ttyd processes and releases all ports.
func (t *TtydManager) StopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for name, cmd := range t.processes {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		delete(t.processes, name)
	}
	for k := range t.usedPorts {
		delete(t.usedPorts, k)
	}
	for k := range t.portMap {
		delete(t.portMap, k)
	}
}
