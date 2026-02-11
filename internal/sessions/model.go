package sessions

import "time"

type Status string

const (
	StatusRunning Status = "running"
	StatusExited  Status = "exited"
	StatusUnknown Status = "unknown"
)

type Session struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CWD         string    `json:"cwd"`
	StartCmd    string    `json:"start_cmd"`
	CreatedAt   time.Time `json:"created_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	TmuxName    string    `json:"tmux_name"`
	TtydPort    int       `json:"ttyd_port"`
	Status      Status    `json:"status"`
	TerminalURL string    `json:"terminal_url"`
}
