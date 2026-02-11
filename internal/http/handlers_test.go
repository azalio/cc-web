package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user/cc-web/internal/config"
	"github.com/user/cc-web/internal/sessions"
)

func testConfig() *config.Config {
	return &config.Config{
		ListenAddr:      "127.0.0.1:8787",
		AuthToken:       "test-token",
		TmuxPrefix:      "test-",
		TtydBasePort:    19000,
		TtydMaxPort:     19010,
		MaxSessions:     10,
		ProjectsAllowed: []string{"/tmp"},
		SessionsFile:    "/tmp/test-sessions.json",
	}
}

func TestListSessions_Unauthorized(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestListSessions_Authorized(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result []interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func TestCreateSession_BadCwd(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	body := `{"name":"test","cwd":"/etc/not-allowed","start_cmd":"echo hello"}`
	req := httptest.NewRequest("POST", "/api/sessions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "not in allowed list") {
		t.Errorf("error = %q, expected 'not in allowed list'", resp["error"])
	}
}

func TestCreateSession_MissingName(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	body := `{"cwd":"/tmp"}`
	req := httptest.NewRequest("POST", "/api/sessions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSendText_NotFound(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	body := `{"text":"hello"}`
	req := httptest.NewRequest("POST", "/api/sessions/nonexistent/send", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestInterrupt_NotFound(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	req := httptest.NewRequest("POST", "/api/sessions/nonexistent/interrupt", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHealthz_NoAuth(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
	if resp["sessions_total"].(float64) != 0 {
		t.Errorf("sessions_total = %v, want 0", resp["sessions_total"])
	}
	if resp["sessions_running"].(float64) != 0 {
		t.Errorf("sessions_running = %v, want 0", resp["sessions_running"])
	}
}

func TestTokenInQueryParam(t *testing.T) {
	cfg := testConfig()
	mgr := sessions.NewManager(cfg)
	srv := NewServer(cfg, mgr)

	req := httptest.NewRequest("GET", "/api/sessions?token=test-token", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
