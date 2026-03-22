package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/model"
)

func setupConfigTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	configPath := filepath.Join(t.TempDir(), "config.json")
	r := gin.New()
	h := NewHandler(NewConfigStore(configPath), VersionInfo{})
	h.RegisterAPIRoutes(r)
	return r, configPath
}

func TestConfigRoute_AccessibleWithoutAuthorization(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "[]" {
		t.Fatalf("body mismatch: got=%q want=%q", body, "[]")
	}
}

func TestSaveConfig_SavesValidPayload(t *testing.T) {
	r, configPath := setupConfigTestRouter(t)

	payload := `[{"id":101,"name":"node-a","url":"http://127.0.0.1:8005"}]`
	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "{\"status\":\"success\"}" {
		t.Fatalf("body mismatch: got=%q", body)
	}

	data := make([]model.AgentConfig, 0)
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("failed to parse saved config: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("length mismatch: got=%d want=%d", len(data), 1)
	}
	if got := data[0]; got.ID != 101 || got.Name != "node-a" || got.URL != "http://127.0.0.1:8005" {
		t.Fatalf("saved config mismatch: %+v", got)
	}
}

func TestSaveConfig_RejectsInvalidPayload(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"id":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusBadRequest)
	}
	if body := w.Body.String(); body != "{\"error\":\"invalid_payload\"}" {
		t.Fatalf("body mismatch: got=%q", body)
	}
}
