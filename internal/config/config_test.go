package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
listen_addr: "127.0.0.1:9999"
auth_token: "test-secret-token-123"
projects_allowed:
  - "/tmp"
tmux_prefix: "test-"
ttyd_base_port: 9100
ttyd_max_port: 9110
max_sessions: 5
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ListenAddr != "127.0.0.1:9999" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:9999")
	}
	if cfg.AuthToken != "test-secret-token-123" {
		t.Errorf("AuthToken = %q, want %q", cfg.AuthToken, "test-secret-token-123")
	}
	if cfg.TmuxPrefix != "test-" {
		t.Errorf("TmuxPrefix = %q, want %q", cfg.TmuxPrefix, "test-")
	}
	if cfg.TtydBasePort != 9100 {
		t.Errorf("TtydBasePort = %d, want %d", cfg.TtydBasePort, 9100)
	}
	if cfg.MaxSessions != 5 {
		t.Errorf("MaxSessions = %d, want %d", cfg.MaxSessions, 5)
	}
}

func TestLoad_MissingToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `listen_addr: "127.0.0.1:9999"`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing auth_token")
	}
}

func TestLoad_InvalidPortRange(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
auth_token: "test-secret-token-123"
ttyd_base_port: 9100
ttyd_max_port: 9000
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid port range")
	}
}

func TestIsPathAllowed(t *testing.T) {
	cfg := &Config{
		ProjectsAllowed: []string{"/tmp"},
	}

	tests := []struct {
		path    string
		allowed bool
	}{
		{"/tmp", true},
		{"/tmp/foo/bar", true},
		{"/etc", false},
		{"/var", false},
		// Path traversal attempts
		{"/tmp/../etc", false},
		{"/tmp/../etc/passwd", false},
		{"/tmp/../../etc", false},
	}

	for _, tt := range tests {
		got := cfg.IsPathAllowed(tt.path)
		if got != tt.allowed {
			t.Errorf("IsPathAllowed(%q) = %v, want %v", tt.path, got, tt.allowed)
		}
	}
}
