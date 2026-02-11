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
				if err := m.ttyd.Start(s.TmuxName, s.TtydPort); err != nil {
					log.Printf("sessions: failed to restart ttyd for %q: %v", s.TmuxName, err)
					m.sessions[id].TtydPort = 0
					m.sessions[id].TerminalURL = ""
				}
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
// Snapshots tmux names under lock, checks tmux outside the lock to avoid
// blocking other operations behind slow shell invocations.
func (m *Manager) List() []*Session {
	// Snapshot IDs and tmux names under read lock
	m.mu.RLock()
	type entry struct {
		id       string
		tmuxName string
	}
	entries := make([]entry, 0, len(m.sessions))
	for id, s := range m.sessions {
		entries = append(entries, entry{id: id, tmuxName: s.TmuxName})
	}
	m.mu.RUnlock()

	// Check tmux status without holding the lock
	alive := make(map[string]bool, len(entries))
	for _, e := range entries {
		alive[e.id] = m.tmux.HasSession(e.tmuxName)
	}

	// Build lookup of IDs that were in the snapshot
	snapshotIDs := make(map[string]bool, len(entries))
	for _, e := range entries {
		snapshotIDs[e.id] = true
	}

	// Re-lock to update status and build result
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	result := make([]*Session, 0, len(m.sessions))
	for id, s := range m.sessions {
		// Sessions created after the snapshot weren't checked — leave their status as-is
		if !snapshotIDs[id] {
			copy := *s
			result = append(result, &copy)
			continue
		}
		if alive[id] {
			s.Status = StatusRunning
			s.LastSeenAt = now
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
	// Read tmux name under read lock
	m.mu.RLock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.RUnlock()
		return nil, false
	}
	tmuxName := s.TmuxName
	m.mu.RUnlock()

	// Check tmux without holding the lock
	isAlive := m.tmux.HasSession(tmuxName)

	// Re-lock to update and copy
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok = m.sessions[id]
	if !ok {
		return nil, false
	}
	if isAlive {
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
	// Validate inputs outside the lock — no shared state needed
	if !m.cfg.IsPathAllowed(req.CWD) {
		return nil, fmt.Errorf("path %q is not in allowed list", req.CWD)
	}
	info, err := os.Stat(req.CWD)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("path %q does not exist or is not a directory", req.CWD)
	}
	if req.StartCmd == "" {
		req.StartCmd = "claude"
	}

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

	now := time.Now()
	suffix := randomSuffix()
	tmuxName := fmt.Sprintf("%s%s-%s-%s", m.cfg.TmuxPrefix, now.Format("20060102-1504"), sanitizeName(req.Name), suffix)
	id := tmuxName

	// Sanity check: generated ID must match safe pattern
	if !safeIDPattern.MatchString(id) {
		return nil, fmt.Errorf("generated session ID %q is not safe; check tmux_prefix in config", id)
	}

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
	if err := m.tmux.KillSession(s.TmuxName); err != nil {
		log.Printf("sessions: kill tmux %q: %v", s.TmuxName, err)
	}
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
	if sessions == nil {
		sessions = make(map[string]*Session)
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
