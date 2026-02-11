package sessions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/user/cc-web/internal/config"
)

// safeIDPattern matches session IDs that are safe for URLs and HTML.
var safeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ErrNotFound is returned when a session ID does not exist.
var ErrNotFound = errors.New("session not found")

// IsNotFound reports whether the error indicates a missing session.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// notFoundError wraps ErrNotFound with the session ID for context.
type notFoundError struct {
	id string
}

func (e *notFoundError) Error() string {
	return fmt.Sprintf("session %q not found", e.id)
}

func (e *notFoundError) Unwrap() error {
	return ErrNotFound
}

// Manager manages Claude Code sessions (tmux + ttyd).
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	cfg      *config.Config
	tmux     *TmuxRunner
	ttyd     *TtydManager
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		cfg:      cfg,
		tmux:     NewTmuxRunner(),
		ttyd:     NewTtydManager(cfg),
	}
}

// Recover scans tmux for existing sessions on startup.
func (m *Manager) Recover() error {
	// Load saved metadata
	m.loadFromFile()

	// Cross-check with tmux
	tmuxSessions, err := m.tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("recover: %w", err)
	}

	tmuxSet := make(map[string]bool)
	for _, name := range tmuxSessions {
		tmuxSet[name] = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Mark sessions not found in tmux as exited
	for id, s := range m.sessions {
		if !tmuxSet[s.TmuxName] {
			m.sessions[id].Status = StatusExited
		} else {
			m.sessions[id].Status = StatusRunning
			m.sessions[id].LastSeenAt = time.Now()
			// Restart ttyd for recovered running sessions
			if s.TtydPort > 0 {
				_ = m.ttyd.Start(s.TmuxName, s.TtydPort)
			}
		}
	}

	// Discover tmux sessions matching our prefix not yet tracked
	for _, name := range tmuxSessions {
		if !strings.HasPrefix(name, m.cfg.TmuxPrefix) {
			continue
		}
		// Validate that the tmux name is safe for use as an ID
		if !safeIDPattern.MatchString(name) {
			log.Printf("sessions: skipping recovered tmux session with unsafe name: %q", name)
			continue
		}
		found := false
		for _, s := range m.sessions {
			if s.TmuxName == name {
				found = true
				break
			}
		}
		if !found {
			id := name
			var port int
			var terminalURL string
			if m.ttyd.Available() {
				if p, err := m.ttyd.AllocatePort(); err == nil {
					if startErr := m.ttyd.Start(name, p); startErr == nil {
						port = p
						terminalURL = fmt.Sprintf("/t/%s/", id)
					} else {
						m.ttyd.ReleasePort(p)
					}
				}
			}
			m.sessions[id] = &Session{
				ID:          id,
				Name:        strings.TrimPrefix(name, m.cfg.TmuxPrefix),
				TmuxName:    name,
				TtydPort:    port,
				Status:      StatusRunning,
				CreatedAt:   time.Now(),
				LastSeenAt:  time.Now(),
				TerminalURL: terminalURL,
			}
		}
	}

	return nil
}

// List returns all sessions with refreshed status.
// Returns copies so callers cannot observe concurrent mutations.
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		// Refresh status
		if m.tmux.HasSession(s.TmuxName) {
			s.Status = StatusRunning
			s.LastSeenAt = time.Now()
		} else {
			s.Status = StatusExited
		}
		copy := *s
		result = append(result, &copy)
	}
	return result
}

// Get returns a session by ID.
// Returns a copy so callers cannot observe concurrent mutations.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	if m.tmux.HasSession(s.TmuxName) {
		s.Status = StatusRunning
		s.LastSeenAt = time.Now()
	} else {
		s.Status = StatusExited
	}
	copy := *s
	return &copy, true
}

type CreateRequest struct {
	Name     string `json:"name"`
	CWD      string `json:"cwd"`
	StartCmd string `json:"start_cmd"`
}

// Create creates a new session.
func (m *Manager) Create(req CreateRequest) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	activeCount := 0
	for _, s := range m.sessions {
		if s.Status != StatusExited {
			activeCount++
		}
	}
	if activeCount >= m.cfg.MaxSessions {
		return nil, fmt.Errorf("max active sessions (%d) reached", m.cfg.MaxSessions)
	}

	if !m.cfg.IsPathAllowed(req.CWD) {
		return nil, fmt.Errorf("path %q is not in allowed list", req.CWD)
	}

	// Verify directory exists
	info, err := os.Stat(req.CWD)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("path %q does not exist or is not a directory", req.CWD)
	}

	if req.StartCmd == "" {
		req.StartCmd = "claude"
	}

	now := time.Now()
	suffix := randomSuffix()
	tmuxName := fmt.Sprintf("%s%s-%s-%s", m.cfg.TmuxPrefix, now.Format("20060102-1504"), sanitizeName(req.Name), suffix)
	id := tmuxName

	// Create tmux session
	if err := m.tmux.CreateSession(tmuxName, req.CWD, req.StartCmd); err != nil {
		return nil, fmt.Errorf("create tmux session: %w", err)
	}

	// Allocate ttyd port only when ttyd is available
	var port int
	var terminalURL string
	if m.ttyd.Available() {
		p, err := m.ttyd.AllocatePort()
		if err != nil {
			_ = m.tmux.KillSession(tmuxName)
			return nil, fmt.Errorf("allocate port: %w", err)
		}
		if err := m.ttyd.Start(tmuxName, p); err != nil {
			m.ttyd.ReleasePort(p)
			_ = m.tmux.KillSession(tmuxName)
			return nil, fmt.Errorf("start ttyd: %w", err)
		}
		port = p
		terminalURL = fmt.Sprintf("/t/%s/", id)
	}

	s := &Session{
		ID:          id,
		Name:        req.Name,
		CWD:         req.CWD,
		StartCmd:    req.StartCmd,
		CreatedAt:   now,
		LastSeenAt:  now,
		TmuxName:    tmuxName,
		TtydPort:    port,
		Status:      StatusRunning,
		TerminalURL: terminalURL,
	}

	m.sessions[id] = s
	m.saveToFile()
	return s, nil
}

// Kill stops a session.
func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return &notFoundError{id: id}
	}

	m.ttyd.Stop(s.TmuxName)
	_ = m.tmux.KillSession(s.TmuxName)
	delete(m.sessions, id)
	m.saveToFile()
	return nil
}

// SendText sends text input to a session.
func (m *Manager) SendText(id, text string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return &notFoundError{id: id}
	}
	return m.tmux.SendKeys(s.TmuxName, text)
}

// Interrupt sends Ctrl+C to a session.
func (m *Manager) Interrupt(id string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return &notFoundError{id: id}
	}
	return m.tmux.Interrupt(s.TmuxName)
}

// SendKeys sends raw key tokens to a session.
func (m *Manager) SendKeys(id string, keys []string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return &notFoundError{id: id}
	}

	// Map friendly names to tmux key tokens
	mapped := make([]string, len(keys))
	for i, k := range keys {
		mapped[i] = mapKey(k)
	}
	return m.tmux.SendRawKeys(s.TmuxName, mapped)
}

// GetTtydPort returns the ttyd port for a session (for proxying).
func (m *Manager) GetTtydPort(id string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return 0, false
	}
	return s.TtydPort, true
}

// Cleanup stops all ttyd processes. Called during graceful shutdown.
// tmux sessions are left alive so they persist across gateway restarts.
func (m *Manager) Cleanup() {
	m.ttyd.StopAll()
}

func (m *Manager) saveToFile() {
	data, err := json.MarshalIndent(m.sessions, "", "  ")
	if err != nil {
		log.Printf("sessions: marshal error: %v", err)
		return
	}
	if err := os.WriteFile(m.cfg.SessionsFile, data, 0600); err != nil {
		log.Printf("sessions: save error: %v", err)
	}
}

func (m *Manager) loadFromFile() {
	data, err := os.ReadFile(m.cfg.SessionsFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("sessions: read error: %v", err)
		}
		return
	}
	var sessions map[string]*Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		log.Printf("sessions: corrupt sessions file: %v", err)
		return
	}
	m.sessions = sessions
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "session"
	}
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

func randomSuffix() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	}
	return hex.EncodeToString(b)
}

func mapKey(key string) string {
	switch strings.ToUpper(key) {
	case "ESC", "ESCAPE":
		return "Escape"
	case "UP":
		return "Up"
	case "DOWN":
		return "Down"
	case "LEFT":
		return "Left"
	case "RIGHT":
		return "Right"
	case "TAB":
		return "Tab"
	case "ENTER", "RETURN":
		return "Enter"
	case "CTRL_C", "CTRL+C":
		return "C-c"
	case "CTRL_D", "CTRL+D":
		return "C-d"
	case "CTRL_Z", "CTRL+Z":
		return "C-z"
	case "CTRL_L", "CTRL+L":
		return "C-l"
	case "BACKSPACE":
		return "BSpace"
	case "SPACE":
		return "Space"
	default:
		return key
	}
}
