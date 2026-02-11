package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr      string   `yaml:"listen_addr"`
	BaseURL         string   `yaml:"base_url"`
	ProjectsAllowed []string `yaml:"projects_allowed"`
	TmuxPrefix      string   `yaml:"tmux_prefix"`
	TtydPath        string   `yaml:"ttyd_path"`
	TtydBasePort    int      `yaml:"ttyd_base_port"`
	TtydMaxPort     int      `yaml:"ttyd_max_port"`
	AuthToken       string   `yaml:"auth_token"`
	MaxSessions     int      `yaml:"max_sessions"`
	SessionsFile    string   `yaml:"sessions_file"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		ListenAddr:   "127.0.0.1:8787",
		TmuxPrefix:   "claude-",
		TtydBasePort: 9000,
		TtydMaxPort:  9099,
		MaxSessions:  10,
		SessionsFile: "sessions.json",
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.AuthToken == "" || cfg.AuthToken == "change-me-to-a-secure-token" {
		return nil, fmt.Errorf("auth_token must be set to a secure value in config")
	}

	if cfg.TtydBasePort > cfg.TtydMaxPort {
		return nil, fmt.Errorf("ttyd_base_port (%d) must be <= ttyd_max_port (%d)", cfg.TtydBasePort, cfg.TtydMaxPort)
	}

	return cfg, nil
}

func (c *Config) IsPathAllowed(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	// Try to resolve symlinks; fall back to cleaned abs path if path doesn't exist yet
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	abs = filepath.Clean(abs)

	for _, allowed := range c.ProjectsAllowed {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(allowedAbs); err == nil {
			allowedAbs = resolved
		}
		allowedAbs = filepath.Clean(allowedAbs)

		// Check if abs is equal to or under allowedAbs
		if abs == allowedAbs {
			return true
		}
		prefix := allowedAbs + string(filepath.Separator)
		if len(abs) > len(prefix) && abs[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
