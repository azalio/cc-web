package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/user/cc-web/internal/config"
	"github.com/user/cc-web/internal/sessions"
)

type Server struct {
	cfg     *config.Config
	mgr     *sessions.Manager
	mux     *http.ServeMux
}

func NewServer(cfg *config.Config, mgr *sessions.Manager) *Server {
	s := &Server{
		cfg: cfg,
		mgr: mgr,
		mux: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Health check (no auth — used by Cloudflare Tunnel, load balancers, monitoring)
	s.mux.HandleFunc("/healthz", s.handleHealthz)

	// API routes (auth required)
	s.mux.HandleFunc("/api/sessions", s.authMiddleware(s.handleSessions))
	s.mux.HandleFunc("/api/sessions/", s.authMiddleware(s.handleSessionAction))

	// Terminal proxy (auth via cookie for WebSocket/iframe)
	s.mux.HandleFunc("/t/", s.authTerminal(s.handleTerminalProxy))

	// Static files (no auth - the PWA itself)
	s.mux.Handle("/", http.FileServer(http.Dir("web/static")))
}

// extractBearerToken extracts the token from the Authorization header.
// Handles "Bearer" prefix case-insensitively per RFC 7235.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	// RFC 7235: scheme is case-insensitive
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return auth[7:]
	}
	return ""
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			// Fall back to cookie (avoid query param on API routes — tokens leak via logs/referrers)
			if c, err := r.Cookie("auth_token"); err == nil {
				token = c.Value
			}
		}
		if !tokenMatch(token, s.cfg.AuthToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) authTerminal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check bearer header first, then cookie (for iframe/WebSocket)
		token := extractBearerToken(r)
		if token == "" {
			if c, err := r.Cookie("auth_token"); err == nil {
				token = c.Value
			}
		}
		if !tokenMatch(token, s.cfg.AuthToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// handleSessions handles GET /api/sessions and POST /api/sessions
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := s.mgr.List()
		writeJSON(w, http.StatusOK, list)

	case http.MethodPost:
		var req sessions.CreateRequest
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if req.CWD == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cwd is required"})
			return
		}

		sess, err := s.mgr.Create(req)
		if err != nil {
			writeCreateError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, sess)

	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleSessionAction handles /api/sessions/{id}/... routes
func (s *Server) handleSessionAction(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/sessions/{id} or /api/sessions/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session ID"})
		return
	}

	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "":
		// GET /api/sessions/{id}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		sess, ok := s.mgr.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusOK, sess)

	case "send":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Text == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
			return
		}
		if err := s.mgr.SendText(id, req.Text); err != nil {
			writeSessionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})

	case "interrupt":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if err := s.mgr.Interrupt(id); err != nil {
			writeSessionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "interrupted"})

	case "keys":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req struct {
			Keys []string `json:"keys"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if len(req.Keys) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "keys is required"})
			return
		}
		if err := s.mgr.SendKeys(id, req.Keys); err != nil {
			writeSessionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "keys sent"})

	case "kill":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if err := s.mgr.Kill(id); err != nil {
			writeSessionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})

	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
	}
}

// handleTerminalProxy proxies requests to the ttyd instance for a session.
func (s *Server) handleTerminalProxy(w http.ResponseWriter, r *http.Request) {
	// Path: /t/{session-id}/...
	path := strings.TrimPrefix(r.URL.Path, "/t/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing session ID", http.StatusBadRequest)
		return
	}

	sessionID := parts[0]
	port, ok := s.mgr.GetTtydPort(sessionID)
	if !ok || port == 0 {
		http.Error(w, "session not found or terminal unavailable", http.StatusNotFound)
		return
	}

	// Reverse proxy to ttyd
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		http.Error(w, "internal proxy error", http.StatusInternalServerError)
		return
	}
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			// Keep the full path for ttyd
		},
	}

	// Support WebSocket upgrade
	if isWebSocket(r) {
		proxy.ServeHTTP(w, r)
		return
	}

	proxy.ServeHTTP(w, r)
}

// handleHealthz returns service health status (no auth required).
// Used by Cloudflare Tunnel, load balancers, and monitoring.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	sessionList := s.mgr.List()
	running := 0
	for _, sess := range sessionList {
		if sess.Status == sessions.StatusRunning {
			running++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"sessions_total":   len(sessionList),
		"sessions_running": running,
	})
}

func isWebSocket(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// tokenMatch performs constant-time comparison to prevent timing attacks.
// Hashes both values first so the comparison is length-independent
// (subtle.ConstantTimeCompare returns early on length mismatch).
func tokenMatch(provided, expected string) bool {
	ph := sha256.Sum256([]byte(provided))
	eh := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(ph[:], eh[:]) == 1
}

// writeCreateError maps session creation errors to appropriate HTTP status codes.
// User errors (bad path, max sessions) get 400; internal errors (tmux/ttyd) get 500.
func writeCreateError(w http.ResponseWriter, err error) {
	msg := err.Error()
	if strings.Contains(msg, "not in allowed list") ||
		strings.Contains(msg, "does not exist or is not a directory") ||
		strings.Contains(msg, "max active sessions") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
	} else {
		log.Printf("create session error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

// writeSessionError maps session manager errors to appropriate HTTP status codes.
// Internal errors are logged but not exposed to clients.
func writeSessionError(w http.ResponseWriter, err error) {
	if sessions.IsNotFound(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	} else {
		log.Printf("session error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
